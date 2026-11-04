package main

// Revisão adversarial da etapa 12.9 (Fase 12), revisor P, ponta a ponta por run():
// imagem e sha256 puro em nenhum arquivo nem log (C-2.5, C-2.11, C-2.12, C-4.2), com a
// melhora ligada (C-2.10a: prompt_json_original também sem imagem).
//
// Dependências de outros _test.go do pacote: noEnv (main_test.go), docKeyHex (fase12_main_test.go).

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
)

func vec12P(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res == "FALHA" {
		t.Errorf("%s: %s", id, desc)
	}
}

type adv12PBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *adv12PBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}
func (w *adv12PBuf) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.b.String() }

type adv12PUp struct {
	mu   sync.Mutex
	hdrs []http.Header
	raw  [][]byte
}

func (u *adv12PUp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	u.mu.Lock()
	h := r.Header.Clone()
	for k, v := range r.Trailer {
		h["Trailer-"+k] = v
	}
	u.hdrs = append(u.hdrs, h)
	u.raw = append(u.raw, b)
	u.mu.Unlock()
	switch {
	case strings.Contains(r.URL.Path, "embed"):
		io.WriteString(w, `{"model":"nomic-embed-text","embeddings":[[1,0,0]]}`)
	case strings.HasPrefix(r.URL.Path, "/v1/"):
		io.WriteString(w, `{"id":"x","choices":[{"message":{"role":"assistant","content":"resposta v1"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	case r.URL.Path == "/api/chat":
		io.WriteString(w, `{"message":{"role":"assistant","content":"resposta chat"},"done":true,"eval_count":1}`+"\n")
	default:
		io.WriteString(w, `{"response":"resposta gen","done":true,"eval_count":1}`+"\n")
	}
}

func adv12PImg(tag string) (b64 string, rawImg []byte) {
	rawImg = []byte("\x89PNG\r\n\x1a\nIMAGEM-SECRETA-" + tag + "-" + strings.Repeat("q", 40))
	return base64.StdEncoding.EncodeToString(rawImg), rawImg
}

func TestAdv12PEndToEndNoImageNoPlainHash(t *testing.T) {
	up := &adv12PUp{}
	srv := httptest.NewServer(up)
	defer srv.Close()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "doc.key")
	if err := os.WriteFile(keyPath, []byte(docKeyHex+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	instr := filepath.Join(dir, "instr.json")
	os.WriteFile(instr, []byte(`{"tools":{"extract":"INSTRUCAO-ADV"}}`), 0o600)
	gwDB, contDB, trDB := filepath.Join(dir, "gw.db"), filepath.Join(dir, "content.db"), filepath.Join(dir, "treino.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := make(chan string, 1)
	done := make(chan int, 1)
	errb := &adv12PBuf{}
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", srv.URL, "--db", gwDB, "--content-db", contDB, "--treino-db", trDB,
			"--doc-key-file", keyPath, "--melhora-tools", "extract", "--melhora-instrucoes", instr, "--debug",
			"--gpu-interval", "0", "--aer-path", "-", "--gen-slots", "5", "--embed-slots", "2"}, noEnv, strings.NewReader(""), io.Discard, errb, func(a string) { addr <- a })
	}()
	var base string
	select {
	case a := <-addr:
		base = "http://" + a
	case c := <-done:
		t.Fatalf("exit %d: %s", c, errb.String())
	case <-time.After(10 * time.Second):
		t.Fatal("did not start")
	}
	docText := "CONTRATO-SIGILOSO-DO-CLIENTE linha 1"
	ds := sha256.Sum256([]byte(docText))
	doc := hex.EncodeToString(ds[:])
	i1, r1 := adv12PImg("1")
	i2, r2 := adv12PImg("2")
	i3, r3 := adv12PImg("3")
	i4, r4 := adv12PImg("4")
	i5, r5 := adv12PImg("5")
	imgs := map[string][]byte{i1: r1, i2: r2, i3: r3, i4: r4, i5: r5}
	reqs := []struct{ path, tool, body string }{
		{"/api/generate", "extract", `{"model":"qwen3-ia","system":"S","prompt":"veja","images":["` + i1 + `"]}`},
		{"/api/chat", "chat", `{"model":"qwen3-ia","messages":[{"role":"user","content":"a","images":["` + i2 + `"]},{"role":"assistant","content":"b"},{"role":"user","content":"c","imageſ":["` + i3 + `"]}]}`},
		{"/v1/chat/completions", "chat", `{"model":"qwen3-ia","messages":[{"role":"user","content":[{"type":"text","text":"t"},{"type":"image_url","image_url":{"url":"data:image/png;base64,` + i4 + `"}}]}]}`},
		{"/api/generate", "extract", `{"model":"qwen3-ia","prompt":"x","options":{"zz":1e400,"yy":` + strings.Repeat("7", 300) + `},"images":["` + i5 + `"]}`},
		{"/api/embed", "extract", `{"model":"nomic-embed-text","input":"` + docText + `"}`},
		{"/api/generate", "extract", `{"model":"qwen3-ia","system":"S","prompt":"conte os itens de ` + docText + `"}`},
	}
	var ids []string
	for _, r := range reqs {
		req, _ := http.NewRequest("POST", base+r.path, strings.NewReader(r.body))
		req.Header.Set("X-Mcpollama-Tool", r.tool)
		req.Header.Set("X-Mcpollama-Doc", doc)
		req.Header.Set("X-Mcpollama-Station", "computer=PC01;user=ana")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d", r.path, resp.StatusCode)
		}
		ids = append(ids, resp.Header.Get("X-Request-Id"))
	}
	// wait for the job log
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		db, _ := sql.Open("sqlite", contDB)
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM job_content WHERE resp_final=1`).Scan(&n)
		db.Close()
		if n >= len(reqs)-1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	// report the text-only job (sample copied, with doc_hash) and the image job
	for _, id := range []string{ids[5], ids[0]} {
		b, _ := json.Marshal(map[string]any{"projeto": "X", "categoria": "omitiu", "descricao": "d", "severidade": "baixa", "origem": "claude",
			"resposta_correta": "1, 2", "jobs": []map[string]string{{"id": id, "papel": "unico", "backend": "primario"}}})
		req, _ := http.NewRequest("POST", base+"/api/feedback", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mcpollama-Station", "computer=PC01;user=ana")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !bytes.Contains(rb, []byte(`"amostras_copiadas":1`)) {
			t.Fatalf("feedback %s: %d %s", id, resp.StatusCode, rb)
		}
	}
	// the canonical values of the text-only job, read BEFORE the shutdown
	cdb, _ := sql.Open("sqlite", contDB)
	var pj, orig []byte
	var temImg int
	if err := cdb.QueryRow(`SELECT prompt_json, prompt_json_original, tem_imagem FROM job_content WHERE job_id=?`, ids[5]).Scan(&pj, &orig, &temImg); err != nil {
		t.Fatal(err)
	}
	type imgRow struct {
		pj, orig []byte
		tem      int
	}
	rows := map[string]imgRow{}
	for i := 0; i < 4; i++ {
		var r imgRow
		cdb.QueryRow(`SELECT prompt_json, prompt_json_original, tem_imagem FROM job_content WHERE job_id=?`, ids[i]).Scan(&r.pj, &r.orig, &r.tem)
		rows[ids[i]] = r
	}
	cdb.Close()
	k, _ := hmackey.Load(keyPath)
	canon, _ := hmackey.CanonicalContent(pj)
	pjSum, cSum := sha256.Sum256(pj), sha256.Sum256(canon)
	var origSum [32]byte
	if len(orig) > 0 {
		origSum = sha256.Sum256(orig)
	}
	scanAll := func(phase string) {
		// every byte of every file and of the log
		files := []string{gwDB, gwDB + "-wal", contDB, contDB + "-wal", trDB, trDB + "-wal", gwDB + "-shm", contDB + "-shm", trDB + "-shm"}
		var blobs = map[string][]byte{"stderr(--debug)": []byte(errb.String())}
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil {
				blobs[filepath.Base(f)] = b
			}
		}
		needles := map[string][]byte{
			"sha256 do documento (hex)":           []byte(doc),
			"sha256 do documento (HEX)":           []byte(strings.ToUpper(doc)),
			"sha256 do documento (bruto)":         ds[:],
			"sha256 do prompt_json (hex)":         []byte(hex.EncodeToString(pjSum[:])),
			"sha256 do prompt_json (bruto)":       pjSum[:],
			"sha256 do conteúdo canônico (hex)":   []byte(hex.EncodeToString(cSum[:])),
			"sha256 do conteúdo canônico (bruto)": cSum[:],
			"chave HMAC (hex)":                    []byte(docKeyHex[:24]),
		}
		if len(orig) > 0 {
			needles["sha256 do prompt_json_original (hex)"] = []byte(hex.EncodeToString(origSum[:]))
		}
		for b64, raw := range imgs {
			needles["imagem base64 "+b64[:12]] = []byte(b64)
			needles["imagem bruta "+b64[:12]] = raw[8:]
		}
		var found []string
		for nn, nd := range needles {
			for fn, b := range blobs {
				if bytes.Contains(b, nd) {
					found = append(found, nn+" em "+fn)
				}
			}
		}
		scanned := make([]string, 0, len(blobs))
		for fn, b := range blobs {
			scanned = append(scanned, fmt.Sprintf("%s=%dB", fn, len(b)))
		}
		// the control: the HMAC IS there (so the scan looked at the right files)
		dh := k.DocHash(doc)
		ctl := false
		for _, b := range blobs {
			if bytes.Contains(b, []byte(dh)) {
				ctl = true
			}
		}
		switch {
		case !ctl:
			vec12P(t, "P4.e2e."+phase, "FALHA", "Menor fixture: o HMAC do documento não está em arquivo nenhum; varredura não exercitada")
		case len(found) > 0:
			vec12P(t, "P4.e2e."+phase, "FALHA", "Bloqueador valor proibido em claro: "+strings.Join(found, "; "))
		default:
			vec12P(t, "P4.e2e."+phase, "OK", fmt.Sprintf("nenhum sha256 puro (documento, prompt_json, original, conteúdo canônico; hex, HEX e bruto), nenhuma imagem (base64 e bruta) e nenhum pedaço da chave em %s; o doc_hash HMAC está lá (controle)", strings.Join(scanned, ", ")))
		}
	}
	scanAll("aoVivo")
	cancel()
	if c := <-done; c != 0 {
		t.Fatalf("exit %d", c)
	}
	scanAll("depoisDeParar")

	// what reached the Ollama
	up.mu.Lock()
	hdrLeak := 0
	for _, h := range up.hdrs {
		for name, v := range h {
			if strings.EqualFold(strings.TrimPrefix(name, "Trailer-"), "X-Mcpollama-Doc") || strings.Contains(strings.Join(v, ","), doc) {
				hdrLeak++
			}
		}
	}
	nUp := len(up.hdrs)
	up.mu.Unlock()
	if hdrLeak == 0 && nUp >= len(reqs) {
		vec12P(t, "P3.e2e", "OK", fmt.Sprintf("%d chamadas ao Ollama (generate, chat, /v1, embed, com melhora ligada) sem X-Mcpollama-Doc", nUp))
	} else {
		vec12P(t, "P3.e2e", "FALHA", fmt.Sprintf("Bloqueador gateway/proxy/proxy.go:205 X-Mcpollama-Doc chegou ao Ollama %d vez(es) em %d chamadas", hdrLeak, nUp))
	}

	// C-2.10a: the image job changed by the improvement keeps both requests without the image
	r0 := rows[ids[0]]
	if len(r0.orig) > 0 && bytes.Contains(r0.pj, []byte("INSTRUCAO-ADV")) && !bytes.Contains(r0.orig, []byte(i1)) && !bytes.Contains(r0.pj, []byte(i1)) && bytes.Contains(r0.orig, []byte(`"imagem_hmac":"`)) && r0.tem == 1 {
		vec12P(t, "P1.e2e.melhoraOriginal", "OK", "job alterado pela melhora: prompt_json (com a instrução) e prompt_json_original têm imagem_hmac, nenhum o base64; tem_imagem=1")
	} else {
		vec12P(t, "P1.e2e.melhoraOriginal", "FALHA", fmt.Sprintf("Bloqueador gateway/proxy/proxy.go:530 orig=%d bytes, instr=%v, b64 no orig=%v, no pj=%v, tem=%d", len(r0.orig), bytes.Contains(r0.pj, []byte("INSTRUCAO-ADV")), bytes.Contains(r0.orig, []byte(i1)), bytes.Contains(r0.pj, []byte(i1)), r0.tem))
	}
	bad := 0
	for i := 1; i < 4; i++ {
		if rows[ids[i]].tem != 1 {
			bad++
		}
	}
	if bad == 0 {
		vec12P(t, "P1.e2e.temImagem", "OK", "chat com histórico (imageſ), /v1 data-URI e número 1e400 gravados com tem_imagem=1")
	} else {
		vec12P(t, "P1.e2e.temImagem", "FALHA", fmt.Sprintf("Bloqueador gateway/proxy/promptjson.go:87 %d job(s) com imagem gravados com tem_imagem=0", bad))
	}

}
