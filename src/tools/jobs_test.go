package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cslsoftwares/mcpollama/ollama"
)

// Fase 12.2: the job id the gateway returns in X-Request-Id, the role of each
// call and the X-Mcpollama-Doc header (contract C-1.2, C-4.1).

// jobID is the id the fake gateway gives to its n-th answer: 32 lowercase hex.
func jobID(n int) string { return fmt.Sprintf("%032x", n) }

// gwMock answers /api/generate and /api/embed like the gateway would: a stream
// with a job id in X-Request-Id (when withID) and records the doc header of
// every request. idPrefix lets two servers (primary, fallback) differ.
type gwMock struct {
	*mock
	mu   sync.Mutex
	docs []string
	n    atomic.Int32
}

func newGWMock(t *testing.T, idBase int, withID bool, f reply) *gwMock {
	t.Helper()
	g := &gwMock{}
	g.mock = newMock(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		g.mu.Lock()
		g.docs = append(g.docs, r.Header.Get("X-Mcpollama-Doc"))
		g.mu.Unlock()
		k := g.n.Add(1)
		if withID {
			w.Header().Set("X-Request-Id", jobID(idBase+int(k)))
		}
		if r.URL.Path == "/api/embed" {
			_ = json.NewEncoder(w).Encode(map[string]any{"model": "nomic-embed-text", "embeddings": [][]float32{{1, 2}}})
			return
		}
		text := fmt.Sprintf("RESP-%d", k)
		if f != nil {
			text, _ = f(k, req.Prompt)
		}
		tb, _ := json.Marshal(text)
		if r.URL.Path == "/api/chat" {
			streamLines(w,
				`{"model":"qwen3-ia","message":{"role":"assistant","content":`+string(tb)+`},"done":false}`,
				`{"model":"qwen3-ia","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","total_duration":1000000000,"prompt_eval_count":10,"eval_count":2}`)
			return
		}
		streamLines(w,
			`{"model":"qwen3-ia","response":`+string(tb)+`,"done":false}`,
			`{"model":"qwen3-ia","response":"","done":true,"done_reason":"stop","total_duration":1000000000,"prompt_eval_count":10,"eval_count":2}`)
	})
	return g
}

func (g *gwMock) docHeaders() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.docs...)
}

func jobsMeta(t *testing.T, meta map[string]any) []map[string]any {
	t.Helper()
	u, _ := meta["mcpollama/usage"].(map[string]any)
	js, _ := u["jobs"].([]map[string]any)
	return js
}

func TestUsageLineSingleJob(t *testing.T) {
	g := newGWMock(t, 0, true, nil)
	res, err := call(t, g.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	line := lastLine(text(t, res))
	t.Logf("depois: %s", line)
	if !strings.HasSuffix(line, " job="+jobID(1)) || strings.Contains(line, "jobs=") {
		t.Fatalf("usage line: %q", line)
	}
	js := jobsMeta(t, res.Meta)
	if len(js) != 1 || js[0]["id"] != jobID(1) || js[0]["papel"] != "unico" || js[0]["backend"] != "primario" {
		t.Fatalf("meta jobs: %+v", res.Meta)
	}
}

func TestUsageLineWithoutRequestIDIsUnchanged(t *testing.T) {
	g := newGWMock(t, 0, false, nil)
	res, err := call(t, g.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	line := lastLine(text(t, res))
	t.Logf("antes (sem gateway): %s", line)
	if strings.Contains(line, "job") {
		t.Fatalf("usage line: %q", line)
	}
	if u, _ := res.Meta["mcpollama/usage"].(map[string]any); u["jobs"] != nil {
		t.Fatalf("meta must have no jobs: %+v", u)
	}
}

func TestUsageLineIgnoresInvalidRequestID(t *testing.T) {
	for _, bad := range []string{"abc", strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("a", 33), "../" + strings.Repeat("a", 29)} {
		m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", bad)
			generateOK(w, r)
		})
		res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
		if err != nil || res.IsError {
			t.Fatalf("%v %+v", err, res)
		}
		if line := lastLine(text(t, res)); strings.Contains(line, "job") {
			t.Fatalf("id %q must be ignored: %q", bad, line)
		}
	}
}

func TestEmbedHasJobAndNoDocHeader(t *testing.T) {
	g := newGWMock(t, 0, true, nil)
	d := g.deps(t, nil)
	res, err := call(t, d, "embed", `{"input":["a"]}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if line := lastLine(text(t, res)); !strings.HasSuffix(line, " job="+jobID(1)) {
		t.Fatalf("usage line: %q", line)
	}
	for _, c := range [][2]string{{"generate", `{"prompt":"x"}`}, {"chat", `{"messages":[{"role":"user","content":"x"}]}`}} {
		r, err := call(t, d, c[0], c[1])
		if err != nil || r.IsError {
			t.Fatalf("%s: %v %+v", c[0], err, r)
		}
		if line := lastLine(text(t, r)); !strings.Contains(line, " job=") {
			t.Fatalf("%s usage line: %q", c[0], line)
		}
	}
	if n := len(g.docHeaders()); n != 3 {
		t.Fatalf("expected 3 requests (embed, generate, chat), got %d", n)
	}
	for i, h := range g.docHeaders() {
		if h != "" {
			t.Fatalf("request %d carries X-Mcpollama-Doc=%q", i, h)
		}
	}
}

func jobsOfLine(t *testing.T, line string) []string {
	t.Helper()
	i := strings.Index(line, " jobs=")
	if i < 0 {
		t.Fatalf("no jobs= in %q", line)
	}
	rest := line[i+len(" jobs="):]
	if j := strings.Index(rest, " "); j >= 0 {
		rest = rest[:j]
	}
	return strings.Split(rest, ",")
}

func TestMapReduceRolesAndDocHeader(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	body := lines(200, 40)
	e.put(t, "grande.txt", body)
	long := strings.Repeat("palavra ", 150)
	g := newGWMock(t, 0, true, func(n int32, prompt string) (string, int) {
		if strings.Contains(prompt, "PARCIAL_INICIO") {
			return fmt.Sprintf("COMBINADO-%d", n), 0
		}
		return fmt.Sprintf("RESP-%d %s", n, long), 0
	})
	d := e.deps(t, g.mock, ceiling, 40)
	res, err := call(t, d, "summarize_file", `{"path":"grande.txt"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	line := lastLine(text(t, res))
	t.Logf("depois: %s", line)
	got := jobsOfLine(t, line)

	// expected roles from the prompts: maps, then reduce rounds, last is final
	var want []string
	maps, reduces := 0, 0
	ps := g.prompts()
	for _, p := range ps {
		if strings.Contains(p, "PARCIAL_INICIO") {
			reduces++
		} else {
			maps++
		}
	}
	if reduces < 2 {
		t.Fatalf("need an intermediate reduce round, got %d reduce calls", reduces)
	}
	for i := 1; i <= maps; i++ {
		want = append(want, fmt.Sprintf("map%d", i))
	}
	for i := 1; i < reduces; i++ {
		want = append(want, fmt.Sprintf("reduce%d", i))
	}
	want = append(want, "final")
	if len(got) != len(want) {
		t.Fatalf("jobs %v, want %d roles %v", got, len(want), want)
	}
	for i, g := range got {
		if g != want[i]+":"+jobID(i+1) {
			t.Fatalf("job %d = %q, want %s:%s", i, g, want[i], jobID(i+1))
		}
	}
	if js := jobsMeta(t, res.Meta); len(js) != len(want) || js[len(js)-1]["papel"] != "final" {
		t.Fatalf("meta jobs: %+v", js)
	}

	// C-4.1: same hash, equal to the sha256 of the extracted text, in every call
	sum := sha256.Sum256([]byte(body))
	hash := hex.EncodeToString(sum[:])
	docs := g.docHeaders()
	if len(docs) != len(want) {
		t.Fatalf("docs %d vs calls %d", len(docs), len(want))
	}
	for i, h := range docs {
		if h != hash {
			t.Fatalf("call %d X-Mcpollama-Doc=%q, want %s", i, h, hash)
		}
	}
}

func TestSingleCallFileToolIsUnico(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "p.txt", "linha um\nlinha dois\n")
	g := newGWMock(t, 0, true, nil)
	res, err := call(t, e.deps(t, g.mock, 128000, 40), "extract", `{"path":"p.txt","instruction":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if line := lastLine(text(t, res)); !strings.HasSuffix(line, " job="+jobID(1)) {
		t.Fatalf("usage line: %q", line)
	}
	if js := jobsMeta(t, res.Meta); len(js) != 1 || js[0]["papel"] != "unico" {
		t.Fatalf("meta jobs: %+v", js)
	}
	sum := sha256.Sum256([]byte("linha um\nlinha dois\n"))
	if h := g.docHeaders(); len(h) != 1 || h[0] != hex.EncodeToString(sum[:]) {
		t.Fatalf("doc headers %v", h)
	}
}

func TestTranslateHasOnlyMapRoles(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	e.put(t, "t.txt", lines(200, 40))
	g := newGWMock(t, 0, true, nil)
	res, err := call(t, e.deps(t, g.mock, ceiling, 40), "translate", `{"path":"t.txt","target_language":"English"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	got := jobsOfLine(t, lastLine(text(t, res)))
	if len(got) < 3 {
		t.Fatalf("expected several parts, got %v", got)
	}
	for i, j := range got {
		if j != fmt.Sprintf("map%d:%s", i+1, jobID(i+1)) {
			t.Fatalf("job %d = %q (translate has only mapN, never final)", i, j)
		}
	}
	if strings.Contains(strings.Join(got, ","), "final") {
		t.Fatal("translate must not have final")
	}
}

func TestFallbackJobCarriesSuffixAndBackend(t *testing.T) {
	primary := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":"fila do servidor cheia"}`, http.StatusServiceUnavailable)
	})
	fb := newGWMock(t, 0x100, true, nil)
	d := primary.deps(t, func(o *ollama.Options) { o.FallbackURL = fb.srv.URL })
	res, err := call(t, d, "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	line := lastLine(text(t, res))
	t.Logf("depois (fallback): %s", line)
	if !strings.Contains(line, " backend=fallback") || !strings.HasSuffix(line, " job="+jobID(0x101)+"@fallback") {
		t.Fatalf("usage line: %q", line)
	}
	if js := jobsMeta(t, res.Meta); len(js) != 1 || js[0]["backend"] != "fallback" || js[0]["id"] != jobID(0x101) {
		t.Fatalf("meta jobs: %+v", res.Meta)
	}
}

func TestFallbackInMapReduceKeepsRoleAndDocHeader(t *testing.T) {
	e := newFileEnv(t)
	body := lines(200, 40)
	e.put(t, "g.txt", body)
	primary := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":"fila do servidor cheia"}`, http.StatusServiceUnavailable)
	})
	fb := newGWMock(t, 0x100, true, nil)
	d := e.deps(t, primary, 2600, 40)
	d2 := primary.deps(t, func(o *ollama.Options) { o.FallbackURL = fb.srv.URL; o.MaxPromptChars = 2600 })
	d.Backend = d2.Backend
	res, err := call(t, d, "translate", `{"path":"g.txt","target_language":"English"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	got := jobsOfLine(t, lastLine(text(t, res)))
	calls := int(fb.hits.Load())
	if calls < 3 || len(got) != calls {
		t.Fatalf("fallback received %d calls, line has %d jobs: %v", calls, len(got), got)
	}
	js := jobsMeta(t, res.Meta)
	if len(js) != calls {
		t.Fatalf("meta has %d jobs, want %d", len(js), calls)
	}
	for i := range got {
		role := fmt.Sprintf("map%d", i+1)
		if got[i] != role+":"+jobID(0x101+i)+"@fallback" {
			t.Fatalf("line job %d = %q", i, got[i])
		}
		if js[i]["papel"] != role || js[i]["backend"] != "fallback" || js[i]["id"] != jobID(0x101+i) {
			t.Fatalf("meta job %d = %+v", i, js[i])
		}
	}
	sum := sha256.Sum256([]byte(body))
	docs := fb.docHeaders()
	if len(docs) != calls {
		t.Fatalf("%d doc headers checked for %d fallback calls", len(docs), calls)
	}
	for i, h := range docs {
		if h != hex.EncodeToString(sum[:]) {
			t.Fatalf("fallback call %d lacks the doc header: %q", i, h)
		}
	}
}
