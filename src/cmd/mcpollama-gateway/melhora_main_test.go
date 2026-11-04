package main

// Fase 12, etapa 12.8 through run(): the flags, off by default, the instructions and the
// frozen index on a real request, the background embeddings and the counters of the
// monitor (no text).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

// upstream is a fake Ollama: it records the generations and answers /api/embed.
type upstream struct {
	mu     sync.Mutex
	gen    []string
	embeds int
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	if r.URL.Path == "/api/embed" {
		u.mu.Lock()
		u.embeds++
		u.mu.Unlock()
		io.WriteString(w, `{"embeddings":[[1,0,0]]}`)
		return
	}
	u.mu.Lock()
	u.gen = append(u.gen, string(b))
	u.mu.Unlock()
	io.WriteString(w, `{"response":"ok","done":true,"eval_count":1}`+"\n")
}

func (u *upstream) last(t *testing.T) string {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.gen) == 0 {
		t.Fatal("no generation reached the Ollama")
	}
	return u.gen[len(u.gen)-1]
}

func startWith(t *testing.T, up http.Handler, extra ...string) (base, dir string) {
	t.Helper()
	srv := httptest.NewServer(up)
	t.Cleanup(srv.Close)
	dir = t.TempDir()
	args := append([]string{"--listen", "127.0.0.1:0", "--ollama", srv.URL, "--db", filepath.Join(dir, "gw.db"),
		"--content-db", filepath.Join(dir, "content.db"), "--gpu-interval", "0", "--aer-path", "-"}, extra...)
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, args, noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	select {
	case a := <-addr:
		base = "http://" + a
	case c := <-done:
		cancel()
		t.Fatalf("gateway exited %d: %s", c, errb.String())
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("gateway did not start")
	}
	t.Cleanup(func() { cancel(); <-done })
	return base, dir
}

func gen(t *testing.T, base, tool, body string) {
	t.Helper()
	req, _ := http.NewRequest("POST", base+"/api/generate", strings.NewReader(body))
	req.Header.Set("X-Mcpollama-Tool", tool)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

const fileBodyMain = `{"model":"qwen3-ia","system":"Regras.","prompt":"Extraia.\n=====ARQUIVO_INICIO 0123456789abcdef=====\ndoc\n=====ARQUIVO_FIM 0123456789abcdef====="}`

func writeInstr(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "instrucoes.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func state(t *testing.T, base string) map[string]json.RawMessage {
	t.Helper()
	resp, err := http.Get(base + "/monitor/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	m["_raw"] = b
	return m
}

func TestParseConfigMelhoraFlags(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil || len(c.MelhoraTools) != 0 || c.MelhoraInstr != "" || c.MelhoraIndice != "" || c.MelhoraBg ||
		c.MelhoraTimeout != 2*time.Second || c.MelhoraModel != "nomic-embed-text" || c.validate() != nil {
		t.Fatalf("defaults (off): %v %+v", err, c)
	}
	env := map[string]string{"MCPGW_MELHORA_TOOLS": "extract;translate", "MCPGW_MELHORA_INSTRUCOES": "/etc/g/i.json", "MCPGW_MELHORA_EMBEDDINGS": "true",
		"MCPGW_MELHORA_EMBED_TIMEOUT": "750ms", "MCPGW_TREINO_DB": "/var/t.db", "MCPGW_MELHORA_INDICE": "abc"}
	c, err = parseConfig(nil, func(k string) string { return env[k] }, &buf)
	if err != nil || strings.Join(c.MelhoraTools, ",") != "extract,translate" || c.MelhoraInstr != "/etc/g/i.json" || !c.MelhoraBg ||
		c.MelhoraTimeout != 750*time.Millisecond || c.MelhoraIndice != "abc" || c.validate() != nil {
		t.Fatalf("env: %v %+v", err, c)
	}
	c, _ = parseConfig([]string{"--melhora-tools", "chat", "--melhora-embeddings=false", "--melhora-embed-model", "outro"}, func(k string) string { return env[k] }, &buf)
	if strings.Join(c.MelhoraTools, ",") != "chat" || c.MelhoraBg || c.MelhoraModel != "outro" {
		t.Fatalf("flag must beat env: %+v", c)
	}
	for name, args := range map[string][]string{
		"unknown tool":          {"--melhora-tools", "extract,bakeoff-erros"},
		"relative instructions": {"--melhora-instrucoes", "i.json"},
		"index without treino":  {"--melhora-indice", "abc"},
		"zero timeout":          {"--melhora-embed-timeout", "0s"},
	} {
		c, _ := parseConfig(args, noEnv, &buf)
		if c.validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	var help bytes.Buffer
	parseConfig([]string{"-h"}, noEnv, &help)
	for _, w := range []string{"--melhora-tools", "--melhora-instrucoes", "--melhora-indice", "--melhora-embeddings", "MCPGW_MELHORA_TOOLS", "OFF (C-8.8)"} {
		if !strings.Contains(help.String(), w) {
			t.Errorf("--help lacks %q", w)
		}
	}
}

func TestRunMelhoraIsOffByDefault(t *testing.T) { // C-8.8
	up := &upstream{}
	base, dir := startWith(t, up, "--doc-key-file", writeDocKey(t, docKeyHex), "--treino-db", filepath.Join(t.TempDir(), "t.db"))
	_ = dir
	gen(t, base, "extract", fileBodyMain)
	got := up.last(t)
	var m struct{ System string }
	json.Unmarshal([]byte(got), &m)
	if m.System != "Regras." {
		t.Errorf("system = %q: something was added with the improvement off", m.System)
	}
	if up.embeds != 0 {
		t.Errorf("%d embeddings for a request with the improvement off", up.embeds)
	}
}

func TestRunMelhoraInstructionsForTheSwitchedOnToolAndCountersInTheState(t *testing.T) {
	up := &upstream{}
	dir := t.TempDir()
	instr := writeInstr(t, dir, `{"tools":{"extract":"INSTRUCAO-SECRETA-DO-EXTRACT"}}`)
	base, _ := startWith(t, up, "--melhora-tools", "extract", "--melhora-instrucoes", instr)
	gen(t, base, "extract", fileBodyMain)
	var m struct{ System string }
	json.Unmarshal([]byte(up.last(t)), &m)
	if m.System != "Regras.\n\nINSTRUCAO-SECRETA-DO-EXTRACT" {
		t.Errorf("system = %q", m.System)
	}
	gen(t, base, "translate", fileBodyMain)
	json.Unmarshal([]byte(up.last(t)), &m)
	if m.System != "Regras." {
		t.Errorf("an unselected tool was changed: %q", m.System)
	}
	st := state(t, base)
	in, _ := melhora.LoadInstrucoes(instr)
	var ms melhora.Stats
	if err := json.Unmarshal(st["melhora"], &ms); err != nil {
		t.Fatalf("no melhora in the state: %v", err)
	}
	if !ms.Ligado || ms.InstrucoesID != in.ID || ms.InstrucoesH != in.Hash || ms.Alterados != 1 || ms.Pedidos != 1 || len(ms.Tools) != 1 {
		t.Errorf("stats: %+v", ms)
	}
	if bytes.Contains(st["_raw"], []byte("INSTRUCAO-SECRETA")) {
		t.Error("the state carries the text of an instruction")
	}
}

func TestRunMelhoraAnInvalidInstructionsFileStopsTheStart(t *testing.T) {
	dir := t.TempDir()
	bad := writeInstr(t, dir, `{"tools":{"inexistente":"x"}}`)
	var errb bytes.Buffer
	code := run(context.Background(), []string{"--listen", "127.0.0.1:0", "--db", filepath.Join(dir, "g.db"), "--content-db", filepath.Join(dir, "c.db"), "--gpu-interval", "0",
		"--melhora-tools", "extract", "--melhora-instrucoes", bad}, noEnv, strings.NewReader(""), io.Discard, &errb, nil)
	if code != 2 || !strings.Contains(errb.String(), "--melhora-instrucoes") {
		t.Errorf("code %d: %s", code, errb.String())
	}
}

// seedTreino makes a treino.db with two public examples in the training part: 1 with its
// embedding computed and a frozen index of it, 2 still pending.
func seedTreino(t *testing.T, path string) (indexID string) {
	t.Helper()
	k, err := hmackey.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	st, err := treino.Open(path, treino.Options{Key: k})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for n, ex := range map[int]struct{ pedido, certa string }{1: {"pedido publico um", "CERTA-PUBLICA-UM"}, 2: {"pedido publico dois", "CERTA-PUBLICA-DOIS"}} {
		id := strings.Repeat(string(rune('0'+n)), 32)
		job := strings.Repeat(string(rune('a'+n)), 32)
		pj := []byte(`{"model":"qwen3-ia","system":"Regras.","prompt":"` + ex.pedido + `"}`)
		must := func(_ sql.Result, err error) {
			if err != nil {
				t.Fatal(err)
			}
		}
		must(db.Exec(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado,categoria,resposta_correta,tool,job_do_erro,particao,grupo_id,exemplo_publico) VALUES(?,1,1,'confirmada','omitiu',?,'extract',?,'treino',?,1)`, id, ex.certa, job, id))
		must(db.Exec(`INSERT INTO ocorrencia_jobs(ocorrencia_id,job_id,papel,backend,ordem) VALUES(?,?,'unico','primario',0)`, id, job))
		must(db.Exec(`INSERT INTO amostras(job_id,copiada_em,endpoint,modelo,think,prompt_json,prompt_texto,resposta,prompt_hmac,conteudo_hmac,key_id,prompt_truncado,prompt_json_truncado,tem_imagem,doc_hash,resp_final,resp_truncada,status,http_status)
			VALUES(?,1,'/api/generate','qwen3-ia','off',?,?,?, 'p','c','k',0,0,0,'',1,0,'ok',200)`, job, pj, []byte("x"), []byte("RESPOSTA-ERRADA")))
		if n == 1 {
			must(db.Exec(`INSERT INTO embeddings(ocorrencia_id,vetor,modelo) VALUES(?,?,?)`, id, treino.EncodeVec([]float32{1, 0, 0}), melhora.DefaultEmbedModel))
		} else {
			must(db.Exec(`INSERT INTO embeddings(ocorrencia_id,vetor,modelo) VALUES(?,NULL,NULL)`, id))
		}
	}
	st, err = treino.Open(path, treino.Options{Key: k})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m, err := st.FreezeIndex(context.Background(), treino.FreezeOptions{K: 2, Limiar: 0.9, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

func TestRunMelhoraFrozenIndexOnARealRequestAndTheBackgroundEmbedding(t *testing.T) {
	up := &upstream{}
	treinoPath := filepath.Join(t.TempDir(), "treino.db")
	idx := seedTreino(t, treinoPath)
	key := writeDocKey(t, docKeyHex)
	base, _ := startWith(t, up, "--doc-key-file", key, "--treino-db", treinoPath, "--melhora-tools", "extract", "--melhora-indice", idx, "--melhora-embeddings")

	// the pending embedding of example 2 is computed by the gateway, through its own queue
	deadline := time.Now().Add(10 * time.Second)
	filled := func() bool {
		db, err := sql.Open("sqlite", treinoPath)
		if err != nil {
			return false
		}
		defer db.Close()
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE vetor IS NOT NULL AND modelo=?`, melhora.DefaultEmbedModel).Scan(&n)
		return n == 2
	}
	for !filled() && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if !filled() {
		t.Fatal("the pending embedding of the public example was not computed")
	}

	gen(t, base, "extract", fileBodyMain)
	got := up.last(t)
	var m struct{ System string }
	json.Unmarshal([]byte(got), &m)
	if !strings.HasPrefix(m.System, "Regras.\n\nOs exemplos abaixo") || !strings.Contains(m.System, "CERTA-PUBLICA-UM") || !strings.Contains(m.System, "RESPOSTA-ERRADA") {
		t.Errorf("system = %q", m.System)
	}
	// the example 2 was embedded after the index was frozen: it is NOT in the frozen index (C-8.5)
	if strings.Contains(m.System, "CERTA-PUBLICA-DOIS") {
		t.Error("an example made after the freeze appeared in the frozen index")
	}
	if !strings.Contains(got, `=====ARQUIVO_INICIO 0123456789abcdef=====\ndoc\n=====ARQUIVO_FIM 0123456789abcdef=====`) {
		t.Errorf("the markers changed: %s", got)
	}
	st := state(t, base)
	var ms melhora.Stats
	json.Unmarshal(st["melhora"], &ms)
	if ms.IndiceID != idx || !ms.IndiceValido || ms.Exemplos != 1 || ms.K != 2 || ms.Limiar != 0.9 || ms.EmbCalculados < 1 || len(ms.IndiceHash) != 64 {
		t.Errorf("stats: %+v", ms)
	}
	for _, secret := range []string{"CERTA-PUBLICA", "RESPOSTA-ERRADA", "pedido publico"} {
		if bytes.Contains(st["_raw"], []byte(secret)) {
			t.Errorf("the state carries example text %q", secret)
		}
	}
}

func TestRunMelhoraABadIndexNeverTakesTheGatewayDown(t *testing.T) {
	up := &upstream{}
	treinoPath := filepath.Join(t.TempDir(), "treino.db")
	seedTreino(t, treinoPath)
	base, _ := startWith(t, up, "--doc-key-file", writeDocKey(t, docKeyHex), "--treino-db", treinoPath, "--melhora-tools", "extract", "--melhora-indice", strings.Repeat("e", 32))
	gen(t, base, "extract", fileBodyMain)
	var m struct{ System string }
	json.Unmarshal([]byte(up.last(t)), &m)
	if m.System != "Regras." {
		t.Errorf("system = %q", m.System)
	}
}

func TestRunMelhoraAnIndexOfAnotherEmbeddingModelIsNotUsed(t *testing.T) { // the vectors must come from the same model as the query
	up := &upstream{}
	treinoPath := filepath.Join(t.TempDir(), "treino.db")
	idx := seedTreino(t, treinoPath) // frozen with nomic-embed-text
	base, _ := startWith(t, up, "--doc-key-file", writeDocKey(t, docKeyHex), "--treino-db", treinoPath, "--melhora-tools", "extract", "--melhora-indice", idx,
		"--melhora-embed-model", "outro-modelo")
	gen(t, base, "extract", fileBodyMain)
	var m struct{ System string }
	json.Unmarshal([]byte(up.last(t)), &m)
	if m.System != "Regras." {
		t.Errorf("an index of another embedding model served: %q", m.System)
	}
}

func TestRunMelhoraEmbeddingsAreOffByDefault(t *testing.T) { // C-6.6
	up := &upstream{}
	treinoPath := filepath.Join(t.TempDir(), "treino.db")
	seedTreino(t, treinoPath) // example 2 is pending
	base, _ := startWith(t, up, "--doc-key-file", writeDocKey(t, docKeyHex), "--treino-db", treinoPath)
	gen(t, base, "extract", fileBodyMain)
	time.Sleep(700 * time.Millisecond) // a pass would have run at once
	db, err := sql.Open("sqlite", treinoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var pend, done int
	db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE vetor IS NULL`).Scan(&pend)
	db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE vetor IS NOT NULL`).Scan(&done)
	if pend != 1 || done != 1 {
		t.Errorf("pending %d, done %d: the embeddings ran without the flag", pend, done)
	}
	if up.embeds != 0 {
		t.Errorf("%d embedding calls without the flag", up.embeds)
	}
}

func TestRunMelhoraTheContentDBKeepsBothRequestsAndTheTrace(t *testing.T) { // C-2.10, C-2.10a
	up := &upstream{}
	dir := t.TempDir()
	instr := writeInstr(t, dir, `{"tools":{"extract":"INSTRUCAO-DO-EXTRACT"}}`)
	base, gdir := startWith(t, up, "--melhora-tools", "extract,translate", "--melhora-instrucoes", instr)
	in, _ := melhora.LoadInstrucoes(instr)
	post := func(tool string) string {
		req, _ := http.NewRequest("POST", base+"/api/generate", strings.NewReader(fileBodyMain))
		req.Header.Set("X-Mcpollama-Tool", tool)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.Header.Get("X-Request-Id")
	}
	changed, plain := post("extract"), post("translate") // translate is on, but has no instruction: unchanged
	db, err := sql.Open("sqlite", filepath.Join(gdir, "content.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	read := func(id string) (pj, orig string, ii, xi, ex sql.NullString) {
		var p, o []byte
		if err := db.QueryRow(`SELECT prompt_json, prompt_json_original, melhora_instrucoes_id, melhora_indice_id, melhora_exemplos FROM job_content WHERE job_id=?`, id).Scan(&p, &o, &ii, &xi, &ex); err != nil {
			t.Fatal(err)
		}
		return string(p), string(o), ii, xi, ex
	}
	pj, orig, ii, xi, ex := read(changed)
	if !strings.Contains(pj, "INSTRUCAO-DO-EXTRACT") || orig == "" || strings.Contains(orig, "INSTRUCAO-DO-EXTRACT") || ii.String != in.ID || xi.String != "" || ex.String != "" {
		t.Errorf("changed job: prompt_json has instr=%v, original=%q, ids %q %q %q", strings.Contains(pj, "INSTRUCAO-DO-EXTRACT"), orig, ii.String, xi.String, ex.String)
	}
	if sent := up.last(t); sent == "" {
		t.Fatal("no generation")
	}
	pj2, orig2, ii2, xi2, ex2 := read(plain)
	if strings.Contains(pj2, "INSTRUCAO") || orig2 != "" || ii2.String != "" || xi2.String != "" || ex2.String != "" {
		t.Errorf("an unchanged job has a trace: %q %q %q %q", orig2, ii2.String, xi2.String, ex2.String)
	}
}
