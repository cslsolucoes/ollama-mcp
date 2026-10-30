package content

// Fase 10.L - revisao adversarial do content.db.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado e FALHA quando o
// defeito existe.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func vecL(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// D2: prompt de 16 MiB + 1 e resposta acima do teto por job.
func TestAdvLPerJobCapBoundary(t *testing.T) {
	s, _ := open(t, Options{})
	big := strings.Repeat("a", DefaultMaxJobBytes) + "é" // 16 MiB + 2 bytes; o corte nao parte a runa
	n, tr, err := s.SavePrompt("j1", big, time.Now())
	k := s.NewSink("j1")
	chunk := strings.Repeat("b", 1<<20)
	for i := 0; i < 17; i++ {
		k.Write(chunk)
	}
	k.Finish("")
	c, ok, _ := s.Get(context.Background(), "j1")
	exact, _, _ := s.SavePrompt("j2", strings.Repeat("c", DefaultMaxJobBytes), time.Now())
	res := fmt.Sprintf("prompt %d bytes -> guardado %d truncado=%v; resposta 17 MiB -> %d truncada=%v resp_bytes=%d; exatamente 16 MiB -> %d", len(big), n, tr, len(c.Response), c.RespTruncated, c.RespBytes, exact)
	if err != nil || !ok || n != DefaultMaxJobBytes || !tr || !c.PromptTruncated || c.PromptBytes != int64(len(big)) ||
		len(c.Response) != DefaultMaxJobBytes || !c.RespTruncated || c.RespBytes != 17<<20 || exact != DefaultMaxJobBytes {
		vecL(t, "D2-teto-por-job", "Media", "gateway/content/content.go:160", "FALHA: "+res)
		t.Fail()
	} else {
		vecL(t, "D2-teto-por-job", "Media", "gateway/content/content.go:160", "OK: "+res)
	}
}

// D3: o teto total sob concorrencia: 16 goroutines gravando; no fim o total
// fica <= teto, o contador bate com o SUM real e os mais novos sobrevivem.
func TestAdvLTotalCapUnderConcurrency(t *testing.T) {
	var evMu sync.Mutex
	events := 0
	s, _ := open(t, Options{MaxBytes: 2 << 20, MaxJobBytes: 64 << 10, OnEvent: func(k, d string) { evMu.Lock(); events++; evMu.Unlock() }})
	var wg sync.WaitGroup
	base := time.Now().Add(-time.Hour)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				id := fmt.Sprintf("g%02d-%03d", g, i)
				s.SavePrompt(id, strings.Repeat("p", 8<<10), base.Add(time.Duration(i)*time.Second))
				k := s.NewSink(id)
				k.Write(strings.Repeat("r", 8<<10))
				k.Finish("")
			}
		}(g)
	}
	wg.Wait()
	s.WaitCap()
	var sum int64
	s.db.QueryRow(`SELECT COALESCE(SUM(COALESCE(length(prompt),0)+COALESCE(length(response),0)),0) FROM job_content`).Scan(&sum)
	var newest int
	s.db.QueryRow(`SELECT COUNT(*) FROM job_content WHERE job_id LIKE '%-039'`).Scan(&newest)
	res := fmt.Sprintf("640 jobs x 16 KiB (10 MiB) num teto de 2 MiB: SUM real %d, contador %d, eventos %d, jobs mais novos (i=39) presentes %d/16", sum, s.Total(), events, newest)
	if sum > 2<<20 || sum != s.Total() || events == 0 || newest < 16 {
		vecL(t, "D3-teto-total-concorrente", "Media", "gateway/content/content.go:188", "FALHA: "+res)
		t.Fail()
	} else {
		vecL(t, "D3-teto-total-concorrente", "Media", "gateway/content/content.go:188", "OK: "+res)
	}
}

// D4: quem estoura o teto paga o apagamento no caminho do pedido. Com jobs
// pequenos, liberar 10% do teto sao centenas de DELETE, cada um com o seu
// commit FULL (fsync), dentro da gravacao do prompt (que acontece com o pedido
// ja aceito) ou do Write do stream (com o slot da GPU preso).
func TestAdvLCapEnforcementOnRequestPath(t *testing.T) {
	s, _ := open(t, Options{MaxBytes: 4 << 20, MaxJobBytes: 1 << 20})
	base := time.Now().Add(-time.Hour)
	// enche ate perto do teto com 4.000 jobs de 1 KiB, numa transacao so (preparo)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4000; i++ {
		_, err = tx.Exec(`INSERT INTO job_content(job_id,created_at,updated_at,prompt,prompt_bytes,prompt_sha256,prompt_truncated,response,resp_bytes,resp_truncated,resp_final) VALUES(?,?,?,?,1024,'',0,x'',0,0,1)`,
			fmt.Sprintf("old-%05d", i), base.Add(time.Duration(i)*time.Millisecond).UnixMilli(), 0, make([]byte, 1024))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var sum int64
	s.db.QueryRow(`SELECT SUM(length(prompt)) FROM job_content`).Scan(&sum)
	s.total.Store(sum)
	start := time.Now()
	s.SavePrompt("novo", strings.Repeat("x", 300<<10), time.Now()) // passa do teto
	el := time.Since(start)
	s.WaitCap()
	var left int
	s.db.QueryRow(`SELECT COUNT(*) FROM job_content`).Scan(&left)
	res := fmt.Sprintf("a gravacao que passou do teto (%d bytes antes, teto %d) levou %s (restaram %d de 4001 jobs)", sum, 4<<20, el.Round(time.Millisecond), left)
	if left >= 4001 {
		t.Fatalf("o teto nao agiu: %s", res)
	}
	if el > 300*time.Millisecond {
		vecL(t, "D4-teto-no-caminho-do-pedido", "Media", "gateway/content/content.go:180", "FALHA: "+res)
		t.Fail()
	} else {
		vecL(t, "D4-teto-no-caminho-do-pedido", "Media", "gateway/content/content.go:180", "OK: "+res)
	}
}

// D5: custo da gravacao periodica da resposta parcial: o UPDATE response =
// response || ? reescreve a linha inteira a cada flush.
func TestAdvLPartialFlushCost(t *testing.T) {
	s, _ := open(t, Options{FlushEvery: time.Nanosecond})
	s.SavePrompt("j", "p", time.Now())
	k := s.NewSink("j")
	chunk := strings.Repeat("r", 64<<10)
	var line []string
	var last time.Duration
	for i := 1; i <= 256; i++ { // 16 MiB em 256 flushes
		st := time.Now()
		k.Write(chunk)
		last = time.Since(st)
		if i == 16 || i == 64 || i == 256 {
			line = append(line, fmt.Sprintf("%d MiB: %s", i/16, last.Round(100*time.Microsecond)))
		}
	}
	k.Finish("")
	res := "custo de 1 flush com a resposta ja em " + strings.Join(line, ", ")
	// informativo: com FlushEvery=2 s, 1 flush a cada 2 s
	vecL(t, "D5-flush-parcial", "Baixa", "gateway/content/content.go:322", "MEDIDA: "+res)
}

// P1: permissoes. O content.db, o -wal e o -shm nascem 0600, e um content.db
// que ja existia com 0644 volta a 0600. So faz sentido em POSIX.
func TestAdvLPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		vecL(t, "P1-permissoes", "Media", "gateway/content/content.go:89", "N/A no Windows (sem bits POSIX); rodar o binario de teste Linux no servidor")
		t.Skip("POSIX")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "content.db")
	os.WriteFile(p, nil, 0o644)
	os.Chmod(p, 0o644)
	s, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s.SavePrompt("a", "x", time.Now())
	var bad []string
	check := func(tag string) {
		for _, f := range []string{p, p + "-wal", p + "-shm"} {
			if fi, err := os.Stat(f); err == nil && fi.Mode().Perm() != 0o600 {
				bad = append(bad, fmt.Sprintf("%s %s=%v", tag, filepath.Base(f), fi.Mode().Perm()))
			}
		}
	}
	check("pre-existente")
	s.Close()
	os.Remove(p + "-wal")
	os.Remove(p + "-shm")
	s, _ = Open(p, Options{})
	s.SavePrompt("b", "x", time.Now())
	check("recriado")
	s.Close()
	if len(bad) > 0 {
		vecL(t, "P1-permissoes", "Media", "gateway/content/content.go:89", "FALHA: "+strings.Join(bad, ", "))
		t.Error(bad)
	} else {
		vecL(t, "P1-permissoes", "Media", "gateway/content/content.go:89", "OK: content.db/-wal/-shm 0600, inclusive arquivo pre-existente 0644 e -wal/-shm recriados")
	}
}

// R1: content.db corrompido: Open devolve erro (o gateway decide o que fazer;
// o vetor do entrypoint confere que ele segue de pe).
func TestAdvLCorruptOpenErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "content.db")
	os.WriteFile(p, []byte(strings.Repeat("lixo nao e sqlite ", 512)), 0o600)
	_, err := Open(p, Options{})
	if err == nil {
		vecL(t, "R1-corrompido-erro", "Baixa", "gateway/content/content.go:100", "FALHA: abriu um arquivo que nao e SQLite")
		t.Fail()
	} else {
		vecL(t, "R1-corrompido-erro", "Baixa", "gateway/content/content.go:100", "OK: Open recusa: "+err.Error())
	}
}
