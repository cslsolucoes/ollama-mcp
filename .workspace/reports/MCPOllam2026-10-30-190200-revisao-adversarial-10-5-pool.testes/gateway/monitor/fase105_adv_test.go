package monitor

// Fase 10.5 - revisao adversarial do pool de backends (monitor).
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado e FALHA quando o
// defeito existe.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func vec5m(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

const advSecTok = "TOKEN-SEC-MON-31cc"

func poolFixtureWithToken(t *testing.T, withAdmin bool) (*fixture, string) {
	t.Helper()
	dir := t.TempDir()
	tok := filepath.Join(dir, "sec.token")
	if err := os.WriteFile(tok, []byte(advSecTok), 0o600); err != nil {
		t.Fatal(err)
	}
	bs, err := pool.Build([]pool.Spec{{ID: "sec-ws01", URL: "https://10.100.2.50:8443", Tipo: "remoto", Slots: 2, TokenFile: tok}}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	pl := pool.New(pool.Config{PrimarySlots: 2, Secondaries: bs})
	f := newFixture(t, withAdmin, queue.Config{GenSlots: 2, Pool: pl})
	return f, tok
}

// M1: drain/undrain exigem sessao, CSRF e mesma origem; id malicioso nao
// drena nada nem ecoa HTML.
func TestAdv5M1DrainGate(t *testing.T) {
	f, _ := poolFixtureWithToken(t, true)
	ck, csrf := f.login(t)
	type c struct {
		method, path string
		hdr          map[string]string
	}
	ok := map[string]string{"Cookie": ck, "X-CSRF-Token": csrf}
	with := func(k, v string) map[string]string {
		m := map[string]string{"Cookie": ck, "X-CSRF-Token": csrf}
		m[k] = v
		return m
	}
	var bad []string
	gates := []c{
		{"POST", "/monitor/api/backends/sec-ws01/drain", nil},
		{"POST", "/monitor/api/backends/sec-ws01/drain", map[string]string{"Cookie": ck}},
		{"POST", "/monitor/api/backends/sec-ws01/drain", with("X-CSRF-Token", csrf+"0")},
		{"POST", "/monitor/api/backends/sec-ws01/drain", with("Origin", "https://evil.example")},
		{"POST", "/monitor/api/backends/sec-ws01/drain", with("Sec-Fetch-Site", "cross-site")},
		{"POST", "/monitor/api/backends/sec-ws01/drain", with("Sec-Fetch-Site", "same-site")},
		{"GET", "/monitor/api/backends/sec-ws01/drain", ok},
		{"PUT", "/monitor/api/backends/sec-ws01/drain", ok},
		{"POST", "/monitor/api/backends/sec-ws01/undrain", nil},
		{"POST", "/monitor/api/backends/gpu-local/drain", map[string]string{"Cookie": ck}},
	}
	for _, g := range gates {
		if w := do(f.h, g.method, g.path, "{}", g.hdr); w.Code == 200 {
			bad = append(bad, fmt.Sprintf("%s %s %v -> 200", g.method, g.path, g.hdr))
		}
	}
	ids := []string{"%3Cscript%3Ealert(1)%3C%2Fscript%3E", "SEC-WS01", "sec-ws01%00", "sec-ws01%0d%0aX:1", strings.Repeat("a", 33), "..%2Fgpu-local", "sec-ws01%2F..", "nao-existe", "gpu-local%20"}
	for _, id := range ids {
		w := do(f.h, "POST", "/monitor/api/backends/"+id+"/drain", "{}", ok)
		if w.Code == 200 || strings.Contains(w.Body.String(), "<script") {
			bad = append(bad, fmt.Sprintf("id %q -> %d %s", id, w.Code, strings.TrimSpace(w.Body.String())))
		}
	}
	for _, b := range backendsOf(t, f) {
		if b.Drained {
			bad = append(bad, b.ID+" ficou drenado")
		}
	}
	// o caminho valido funciona e fica na auditoria
	w1 := do(f.h, "POST", "/monitor/api/backends/sec-ws01/drain", `{"reason":"<b>x</b>"}`, ok)
	w2 := do(f.h, "POST", "/monitor/api/backends/sec-ws01/undrain", `{}`, ok)
	aud, _ := f.st.Audits(context.Background(), 10)
	if w1.Code != 200 || w2.Code != 200 || len(aud) < 2 {
		bad = append(bad, fmt.Sprintf("caminho valido: %d/%d auditoria %d", w1.Code, w2.Code, len(aud)))
	}
	if len(bad) > 0 {
		vec5m(t, "M1-drain-sessao-csrf-origem", "Media", "gateway/monitor/backends.go:17", "FALHA: "+strings.Join(bad, " | "))
		t.Fail()
		return
	}
	vec5m(t, "M1-drain-sessao-csrf-origem", "Media", "gateway/monitor/backends.go:17", fmt.Sprintf("OK: %d tentativas sem sessao/CSRF/origem ou com metodo errado e %d ids maliciosos -> nenhum 200, nada drenado; o caminho valido drena, retoma e audita", len(gates), len(ids)))
}

// M2: nada sensivel (token, caminho do token_file, URL completa) no state, no
// SSE, no history nem no export; a pagina nao tem sink de HTML e esta em LF.
func TestAdv5M2NothingSensitiveInTheMonitor(t *testing.T) {
	f, tokPath := poolFixtureWithToken(t, true)
	f.st.Event("backend_down", "sec-ws01: teste")
	f.st.Backends([]store.BackendRow{{TS: time.Now(), BackendID: "sec-ws01", State: "fora"}})
	srv := httptest.NewServer(f.h)
	defer srv.Close()
	var blobs []string
	for _, p := range []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/jobs/export?formato=csv", "/monitor/api/jobs/export"} {
		blobs = append(blobs, p+": "+do(f.h, "GET", p, "", nil).Body.String())
	}
	ck, _ := f.login(t)
	blobs = append(blobs, "export admin: "+do(f.h, "GET", "/monitor/api/jobs/export?conteudo=1", "", map[string]string{"Cookie": ck}).Body.String())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/monitor/api/stream", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data:") {
				blobs = append(blobs, "sse: "+sc.Text())
				break
			}
		}
		resp.Body.Close()
	}
	html, _ := os.ReadFile("index.html")
	var bad []string
	for _, b := range blobs {
		for _, s := range []string{advSecTok, filepath.Base(tokPath), "https://10.100.2.50", "token_file"} {
			if strings.Contains(b, s) {
				bad = append(bad, fmt.Sprintf("%q em %.40s", s, b))
			}
		}
	}
	for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "\r"} {
		if strings.Contains(string(html), sink) {
			bad = append(bad, "index.html tem "+fmt.Sprintf("%q", sink))
		}
	}
	if len(blobs) < 6 {
		bad = append(bad, fmt.Sprintf("so %d fontes lidas (SSE nao respondeu?)", len(blobs)))
	}
	if len(bad) > 0 {
		vec5m(t, "M2-nada-sensivel-no-monitor", "Alta", "gateway/monitor/monitor.go:359", "FALHA: "+strings.Join(bad, " | "))
		t.Fail()
		return
	}
	vec5m(t, "M2-nada-sensivel-no-monitor", "Alta", "gateway/monitor/monitor.go:359", fmt.Sprintf("OK: %d fontes (state, history, export csv/jsonl, export admin, SSE) sem token, sem caminho do token_file e sem URL; index.html sem sink de HTML e sem CR", len(blobs)))
	_ = io.Discard
}
