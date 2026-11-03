package main

// Fase 12, etapa 12.4: --treino-db, --role and POST /api/feedback through run():
// the route is registered before the catch-all proxy, reads the real job log and
// content database, and never reaches the Ollama.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
)

type gw struct {
	base   string
	dir    string
	hits   *atomic.Int64 // requests that reached the fake Ollama, by path /api/feedback
	stop   func() (int, string)
	upSeen *atomic.Int64
}

// startGW runs the gateway against a fake Ollama. extra flags are added to the base ones.
func startGW(t *testing.T, extra ...string) *gw {
	t.Helper()
	g := &gw{hits: &atomic.Int64{}, upSeen: &atomic.Int64{}}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.upSeen.Add(1)
		if strings.HasPrefix(r.URL.Path, "/api/feedback") {
			g.hits.Add(1)
		}
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"response":"tudo certo","done":true,"eval_count":1}`+"\n")
	}))
	t.Cleanup(up.Close)
	g.dir = t.TempDir()
	args := append([]string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", filepath.Join(g.dir, "gw.db"),
		"--content-db", filepath.Join(g.dir, "content.db"), "--gpu-interval", "0", "--aer-path", "-"}, extra...)
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, args, noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	select {
	case a := <-addr:
		g.base = "http://" + a
	case c := <-done:
		cancel()
		t.Fatalf("gateway exited %d: %s", c, errb.String())
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("gateway did not start")
	}
	stopped := false
	g.stop = func() (int, string) {
		if !stopped {
			stopped = true
			cancel()
		}
		return <-done, errb.String()
	}
	t.Cleanup(func() {
		if !stopped {
			g.stop()
		}
	})
	return g
}

func (g *gw) generate(t *testing.T, prompt string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", g.base+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"`+prompt+`"}`))
	req.Header.Set("X-Mcpollama-Tool", "extract")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	id := resp.Header.Get("X-Request-Id")
	if len(id) != 32 {
		t.Fatalf("no job id: %q", id)
	}
	return id
}

// waitJobDone polls, with a deadline, until the job log says the job ended and the
// content database holds the final answer (both are written in the background).
func (g *gw) waitJobDone(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if g.jobDone(id) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish and reach the content database in time", id)
}

func (g *gw) jobDone(id string) bool {
	db, err := sql.Open("sqlite", filepath.Join(g.dir, "gw.db"))
	if err != nil {
		return false
	}
	defer db.Close()
	var st string
	if db.QueryRow(`SELECT status FROM requests WHERE id=?`, id).Scan(&st) != nil || st == "na_fila" || st == "em_execucao" {
		return false
	}
	cdb, err := sql.Open("sqlite", filepath.Join(g.dir, "content.db"))
	if err != nil {
		return false
	}
	defer cdb.Close()
	var rf int
	return cdb.QueryRow(`SELECT resp_final FROM job_content WHERE job_id=?`, id).Scan(&rf) == nil && rf == 1
}

func (g *gw) feedback(t *testing.T, method, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, g.base+"/api/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mcpollama-Station", "computer=PC01;user=ana")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	json.Unmarshal(b, &m)
	return resp.StatusCode, m
}

func fbBody(id string) string {
	b, _ := json.Marshal(map[string]any{"projeto": "GestorERP", "categoria": "omitiu", "descricao": "esqueceu o item 3", "severidade": "baixa", "origem": "claude",
		"resposta_correta": "1, 2, 3", "jobs": []map[string]string{{"id": id, "papel": "unico", "backend": "primario"}}})
	return string(b)
}

func TestParseConfigTreinoFlags(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil || c.TreinoDB != "" || c.Role != "primario" || c.validate() != nil {
		t.Fatalf("defaults: %v %q %q", err, c.TreinoDB, c.Role)
	}
	env := map[string]string{"MCPGW_TREINO_DB": "/var/lib/g/treino.db", "MCPGW_ROLE": "Fallback"}
	c, err = parseConfig(nil, func(k string) string { return env[k] }, &buf)
	if err != nil || c.TreinoDB != "/var/lib/g/treino.db" || c.Role != "fallback" || c.validate() != nil {
		t.Fatalf("env: %v %q %q", err, c.TreinoDB, c.Role)
	}
	c, _ = parseConfig([]string{"--treino-db", "/outro/t.db", "--role", "primario"}, func(k string) string { return env[k] }, &buf)
	if c.TreinoDB != "/outro/t.db" || c.Role != "primario" {
		t.Fatalf("flag must beat env: %q %q", c.TreinoDB, c.Role)
	}
	for name, args := range map[string][]string{
		"relative":  {"--treino-db", "treino.db"},
		"bad role":  {"--role", "secundario"},
		"same db":   {"--db", "/x/g.db", "--treino-db", "/x/g.db"},
		"same text": {"--content-db", "/x/c.db", "--treino-db", "/x/c.db"},
	} {
		c, _ := parseConfig(args, noEnv, &buf)
		if c.validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	var help bytes.Buffer
	parseConfig([]string{"-h"}, noEnv, &help)
	for _, w := range []string{"--treino-db", "MCPGW_TREINO_DB", "--role", "MCPGW_ROLE", "2 GiB", "507"} {
		if !strings.Contains(help.String(), w) {
			t.Errorf("--help lacks %q", w)
		}
	}
}

// The route is served by the gateway, before the proxy: a GET never reaches the
// Ollama, a report reads the real job log and content database and writes treino.db.
func TestRunFeedbackEndToEnd(t *testing.T) {
	keyPath := writeDocKey(t, docKeyHex)
	treinoPath := filepath.Join(t.TempDir(), "treino.db")
	g := startGW(t, "--doc-key-file", keyPath, "--treino-db", treinoPath)
	id := g.generate(t, "conte os itens")
	g.waitJobDone(t, id)

	before := g.upSeen.Load()
	if code, _ := g.feedback(t, "GET", ""); code != 405 {
		t.Errorf("GET: %d", code)
	}
	code, res := g.feedback(t, "POST", fbBody(id))
	if code != 200 || res["estado"] != "nova" || res["amostras_copiadas"].(float64) != 1 {
		t.Fatalf("report: %d %v", code, res)
	}
	if g.hits.Load() != 0 || g.upSeen.Load() != before {
		t.Errorf("the feedback route reached the Ollama (%d hits, %d requests)", g.hits.Load(), g.upSeen.Load()-before)
	}
	if code, m := g.feedback(t, "POST", fbBody(strings.Repeat("a", 32))); code != 404 {
		t.Errorf("unknown job: %d %v", code, m)
	}
	if code, _ := g.feedback(t, "POST", `{"descricao":"x"}`); code != 400 {
		t.Errorf("incomplete report: %d", code)
	}
	if c, msg := g.stop(); c != 0 {
		t.Fatalf("exit %d: %s", c, msg)
	}

	// what treino.db holds, read by a second connection after the gateway stopped
	tdb, err := sql.Open("sqlite", treinoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.Close()
	var pj, resp []byte
	var ph, kid, status string
	var img, rf, http int
	if err := tdb.QueryRow(`SELECT prompt_json, resposta, prompt_hmac, key_id, tem_imagem, resp_final, status, http_status FROM amostras WHERE job_id=?`, id).
		Scan(&pj, &resp, &ph, &kid, &img, &rf, &status, &http); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pj, []byte("conte os itens")) || !bytes.Contains(resp, []byte("tudo certo")) || img != 0 || rf != 1 || status != "ok" || http != 200 {
		t.Errorf("sample: pj=%s resp=%s img=%d rf=%d status=%s http=%d", pj, resp, img, rf, status, http)
	}
	cdb, err := sql.Open("sqlite", filepath.Join(g.dir, "content.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	var cpj []byte
	if err := cdb.QueryRow(`SELECT prompt_json FROM job_content WHERE job_id=?`, id).Scan(&cpj); err != nil {
		t.Fatal(err)
	}
	k, err := hmackey.Load(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cpj, pj) || ph != k.PromptHMAC(cpj) || kid != k.ID() {
		t.Errorf("the sample is not the content row: hmac %q key_id %q", ph, kid)
	}
	var tool, comp string
	if err := tdb.QueryRow(`SELECT tool, estacao_computer FROM ocorrencias`).Scan(&tool, &comp); err != nil || tool != "extract" || comp != "PC01" {
		t.Errorf("occurrence: %v %q %q", err, tool, comp)
	}
}

// Without --treino-db, or with it but without --doc-key-file, the route answers
// 503 and still never reaches the Ollama (it is the gateway's own route).
func TestRunFeedbackOffAndWithoutKey(t *testing.T) {
	keyPath := writeDocKey(t, docKeyHex)
	cases := map[string][]string{
		"no treino-db": {"--doc-key-file", keyPath},
		"no key":       {"--treino-db", filepath.Join(t.TempDir(), "treino.db")},
	}
	for name, args := range cases {
		g := startGW(t, args...)
		id := g.generate(t, "x")
		before := g.upSeen.Load()
		code, m := g.feedback(t, "POST", fbBody(id))
		if code != 503 || m["error"] == nil {
			t.Errorf("%s: %d %v", name, code, m)
		}
		if g.hits.Load() != 0 || g.upSeen.Load() != before {
			t.Errorf("%s: the route reached the Ollama", name)
		}
	}
}

// C-1.21 through the flag: a gateway with --role fallback records the report as
// estado fallback and copies no sample.
func TestRunFeedbackFallbackRole(t *testing.T) {
	keyPath := writeDocKey(t, docKeyHex)
	g := startGW(t, "--doc-key-file", keyPath, "--treino-db", filepath.Join(t.TempDir(), "treino.db"), "--role", "fallback")
	id := g.generate(t, "x")
	g.waitJobDone(t, id)
	body := strings.Replace(fbBody(id), `"backend":"primario"`, `"backend":"fallback"`, 1)
	code, res := g.feedback(t, "POST", body)
	if code != 200 || res["estado"] != "fallback" || res["amostras_copiadas"].(float64) != 0 {
		t.Errorf("%d %v", code, res)
	}
}
