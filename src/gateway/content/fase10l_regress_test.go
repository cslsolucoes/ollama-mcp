package content

// Regressões da revisão adversarial da etapa 10.L. Cada teste falha no código
// do commit 6cbaf22 e passa depois da correção.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// D4: a gravação que passa do teto não paga o apagamento dos jobs antigos (ele
// roda em segundo plano, em transações de capBatch).
func TestRegress10LCapNotOnTheWritePath(t *testing.T) {
	s, _ := open(t, Options{MaxBytes: 4 << 20, MaxJobBytes: 1 << 20})
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 4000; i++ {
		if _, err := tx.Exec(`INSERT INTO job_content(job_id,created_at,updated_at,prompt,prompt_bytes,prompt_sha256,prompt_truncated,response,resp_bytes,resp_truncated,resp_final) VALUES(?,?,0,?,1024,'',0,x'',0,0,1)`,
			fmt.Sprintf("old-%05d", i), base.Add(time.Duration(i)*time.Millisecond).UnixMilli(), make([]byte, 1024)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s.total.Store(4000 * 1024)
	start := time.Now()
	if _, _, err := s.SavePrompt("novo", strings.Repeat("x", 300<<10), time.Now()); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 300*time.Millisecond {
		t.Fatalf("a gravação que passou do teto levou %s", el)
	}
	s.WaitCap()
	var left int
	var sum int64
	s.db.QueryRow(`SELECT COUNT(*), SUM(length(prompt)+length(response)) FROM job_content`).Scan(&left, &sum)
	if sum > 4<<20 || sum != s.Total() || left >= 4001 {
		t.Fatalf("teto não aplicado: %d jobs, %d bytes, contador %d", left, sum, s.Total())
	}
	if _, ok, _ := s.Info(t.Context(), "novo"); !ok {
		t.Fatal("o job novo foi apagado")
	}
}

// D4 (cont.): Close espera uma rodada do teto em andamento; nada roda depois.
func TestRegress10LCloseWaitsForCap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "content.db")
	s, err := Open(p, Options{MaxBytes: 1 << 20, MaxJobBytes: 64 << 10})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				s.SavePrompt(fmt.Sprintf("%d-%d", g, i), strings.Repeat("z", 16<<10), time.Now())
			}
		}(g)
	}
	time.Sleep(20 * time.Millisecond)
	s.Close() // concorrente com as gravações e com o teto em segundo plano
	wg.Wait()
	s.WaitCap()
	s.triggerCap() // depois de fechado: não sobe goroutine
	s.WaitCap()
}

// P1: um content.db que já existia com 0644 volta a 0600 (o -wal e o -shm
// copiam o modo dele). Só em POSIX.
func TestRegress10LExistingFileBecomesPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sem bits POSIX no Windows; rodar o binário de teste Linux")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "content.db")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(p, 0o644)
	for round := 0; round < 2; round++ {
		s, err := Open(p, Options{})
		if err != nil {
			t.Fatal(err)
		}
		s.SavePrompt(fmt.Sprint(round), "x", time.Now())
		for _, f := range []string{p, p + "-wal", p + "-shm"} {
			if fi, err := os.Stat(f); err == nil && fi.Mode().Perm() != 0o600 {
				t.Fatalf("rodada %d: %s = %v", round, filepath.Base(f), fi.Mode().Perm())
			}
		}
		s.Close()
		os.Remove(p + "-wal")
		os.Remove(p + "-shm")
	}
}

// M1: SavePrompt de um prompt de 32 MiB não copia o prompt inteiro de novo.
func TestRegress10LSavePromptDoesNotCopyTheWholePrompt(t *testing.T) {
	s, _ := open(t, Options{})
	big := strings.Repeat("w", 32<<20)
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	if _, tr, err := s.SavePrompt("big", big, time.Now()); err != nil || !tr {
		t.Fatal(err, tr)
	}
	runtime.ReadMemStats(&m1)
	if a := m1.TotalAlloc - m0.TotalAlloc; a > 28<<20 {
		t.Fatalf("SavePrompt alocou %d MiB para guardar 16 MiB", a>>20)
	}
}
