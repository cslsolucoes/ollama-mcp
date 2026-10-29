package proxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

const finalChunk = `{"model":"qwen3-ia","response":"","done":true,"done_reason":"stop","context":[1,2,3],"total_duration":536000000,"load_duration":4000000,"prompt_eval_count":44,"prompt_eval_duration":64000000,"eval_count":41,"eval_duration":424000000}`

type env struct {
	gw   *httptest.Server
	up   *httptest.Server
	q    *queue.Manager
	mu   sync.Mutex
	recs []queue.Record
}

func (e *env) records() []queue.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]queue.Record(nil), e.recs...)
}

func (e *env) waitRecord(t *testing.T, id string) queue.Record {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range e.records() {
			if r.ID == id {
				return r
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("no record for %s; have %+v", id, e.records())
	return queue.Record{}
}

func (e *env) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s := e.q.Snapshot()
		if len(s.Running) == 0 && len(s.Waiting) == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("slot not released: %+v", e.q.Snapshot())
}

func setup(t *testing.T, up http.Handler, qc queue.Config, mod func(*Config)) *env {
	t.Helper()
	e := &env{}
	e.up = httptest.NewServer(up)
	t.Cleanup(e.up.Close)
	qc.OnRecord = func(r queue.Record) { e.mu.Lock(); e.recs = append(e.recs, r); e.mu.Unlock() }
	e.q = queue.New(qc)
	u, _ := url.Parse(e.up.URL)
	cfg := Config{Upstream: u, Queue: e.q}
	if mod != nil {
		mod(&cfg)
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.gw = httptest.NewServer(h)
	t.Cleanup(e.gw.Close)
	return e
}

func post(t *testing.T, e *env, path, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.gw.URL+path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// postBg is post for goroutines: it never touches t.
func postBg(e *env, path, body string, hdr map[string]string) (*http.Response, string) {
	req, _ := http.NewRequest("POST", e.gw.URL+path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &http.Response{StatusCode: -1, Header: http.Header{}}, ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, l := range []string{`{"response":"Olá","done":false}`, `{"response":", mundo","done":false}`, finalChunk} {
		io.WriteString(w, l+"\n")
		w.(http.Flusher).Flush()
	}
}

var idRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestStreamPassThroughHeadersAndCounters(t *testing.T) {
	e := setup(t, http.HandlerFunc(streamHandler), queue.Config{}, nil)
	resp, body := post(t, e, "/api/generate", `{"model":"qwen3-ia","prompt":"oi"}`, map[string]string{HeaderClient: "host/42", HeaderTool: "generate"})
	if resp.StatusCode != 200 || !strings.Contains(body, `"response":", mundo"`) || !strings.HasSuffix(strings.TrimSpace(body), "}") {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
	id := resp.Header.Get(HeaderRequestID)
	if !idRe.MatchString(id) {
		t.Fatalf("X-Request-Id = %q", id)
	}
	if resp.Header.Get(HeaderQueueMS) == "" || resp.Header.Get(HeaderQueuePosition) != "0" {
		t.Fatalf("queue headers: %v", resp.Header)
	}
	rec := e.waitRecord(t, id)
	if rec.Status != queue.StatusOK || rec.PromptEval != 44 || rec.Eval != 41 || rec.LoadMS != 4 {
		t.Fatalf("record %+v", rec)
	}
	if rec.ClientLabel != "host/42" || rec.Tool != "generate" || rec.Model != "qwen3-ia" || rec.Lane != queue.LaneGen || rec.HTTPStatus != 200 {
		t.Fatalf("record meta %+v", rec)
	}
}

func TestNonStreamJSONCounters(t *testing.T) {
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"model":"nomic-embed-text","embeddings":[[0.1,0.2,0.3]],"total_duration":9000000,"load_duration":2000000,"prompt_eval_count":7}`)
	}), queue.Config{}, nil)
	resp, _ := post(t, e, "/api/embed", `{"model":"nomic-embed-text","input":["a"]}`, nil)
	rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if rec.Lane != queue.LaneEmbed || rec.PromptEval != 7 || rec.LoadMS != 2 || rec.Status != queue.StatusOK {
		t.Fatalf("record %+v", rec)
	}
	if rec.ClientLabel != "anonimo" {
		t.Fatalf("label %q", rec.ClientLabel)
	}
}

func TestDirectAndBlockedRoutes(t *testing.T) {
	var hits sync.Map
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Store(r.URL.Path, true)
		io.WriteString(w, `{"ok":true}`)
	}), queue.Config{GenSlots: 1}, nil)
	// Saturate the generation lane: direct routes must not wait.
	tk, err := e.q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "1.1.1.1", ClientLabel: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer tk.Release(queue.Result{})
	for _, p := range []string{"/api/tags", "/api/ps", "/api/version", "/v1/models"} {
		resp, err := http.Get(e.gw.URL + p)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", p, err, resp)
		}
		resp.Body.Close()
	}
	if resp, _ := post(t, e, "/api/show", `{"model":"x"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("show: %d", resp.StatusCode)
	}
	for _, p := range []string{"/api/pull", "/api/push", "/api/create", "/api/delete", "/api/copy", "/api/blobs/sha256:abc"} {
		for _, m := range []string{"POST", "DELETE", "GET"} {
			req, _ := http.NewRequest(m, e.gw.URL+p, strings.NewReader("{}"))
			resp, err := http.DefaultClient.Do(req)
			if err != nil || resp.StatusCode != 403 {
				t.Fatalf("%s %s: %v %v", m, p, err, resp)
			}
			resp.Body.Close()
		}
		if _, ok := hits.Load(p); ok {
			t.Fatalf("%s reached the upstream", p)
		}
	}
	if resp, _ := post(t, e, "/api/whatever", `{}`, nil); resp.StatusCode != 404 {
		t.Fatalf("unknown: %d", resp.StatusCode)
	}
	resp, err := http.Get(e.gw.URL + "/api/generate")
	if err != nil || resp.StatusCode != 405 {
		t.Fatalf("GET generate: %v %v", err, resp)
	}
	resp.Body.Close()
}

func TestUpstreamDoesNotSeeClientHeaders(t *testing.T) {
	var got http.Header
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		io.WriteString(w, finalChunk)
	}), queue.Config{}, nil)
	post(t, e, "/api/generate", `{"model":"m"}`, map[string]string{
		"Authorization": "Bearer SEGREDO", "X-Real-Ip": "9.9.9.9", "Cookie": "a=b", HeaderClient: "c", HeaderTool: "t", HeaderPriority: "alta",
	})
	for _, k := range []string{"Authorization", "X-Real-Ip", "Cookie", HeaderClient, HeaderTool, HeaderPriority} {
		if got.Get(k) != "" {
			t.Fatalf("header %s leaked to the Ollama: %q", k, got.Get(k))
		}
	}
}

func TestPriorityOnlyFromAllowedIPs(t *testing.T) {
	_, nets, _ := net.ParseCIDR("203.0.113.0/24")
	e := setup(t, http.HandlerFunc(streamHandler), queue.Config{}, func(c *Config) { c.PriorityNets = []*net.IPNet{nets} })
	r1, _ := post(t, e, "/api/generate", `{"model":"m"}`, map[string]string{"X-Real-Ip": "203.0.113.5", HeaderPriority: "alta"})
	r2, _ := post(t, e, "/api/generate", `{"model":"m"}`, map[string]string{"X-Real-Ip": "198.51.100.1", HeaderPriority: "alta"})
	r3, _ := post(t, e, "/api/generate", `{"model":"m"}`, map[string]string{"X-Real-Ip": "198.51.100.1", HeaderPriority: "lixo"})
	a, b, c := e.waitRecord(t, r1.Header.Get(HeaderRequestID)), e.waitRecord(t, r2.Header.Get(HeaderRequestID)), e.waitRecord(t, r3.Header.Get(HeaderRequestID))
	if a.PriorityIn != queue.High || a.PriorityFinal != queue.High || a.ClientIP != "203.0.113.5" {
		t.Fatalf("authorised: %+v", a)
	}
	if b.PriorityIn != queue.High || b.PriorityFinal != queue.Normal {
		t.Fatalf("unauthorised must be downgraded in silence: %+v", b)
	}
	if c.PriorityIn != queue.Normal || c.PriorityFinal != queue.Normal {
		t.Fatalf("garbage: %+v", c)
	}
}

func TestClientIPTrustsXRealIPOnlyFromLoopback(t *testing.T) {
	mk := func(remote string, vals ...string) *http.Request {
		r := httptest.NewRequest("POST", "/api/generate", nil)
		r.RemoteAddr = remote
		for _, v := range vals {
			r.Header.Add("X-Real-Ip", v)
		}
		return r
	}
	if got := clientIP(mk("127.0.0.1:5000", "10.1.2.3")); got != "10.1.2.3" {
		t.Fatalf("loopback peer: %q", got)
	}
	if got := clientIP(mk("198.51.100.9:5000", "10.1.2.3")); got != "198.51.100.9" {
		t.Fatalf("non-loopback peer must ignore X-Real-IP: %q", got)
	}
	if got := clientIP(mk("127.0.0.1:5000", "10.1.2.3", "10.9.9.9")); got != "127.0.0.1" {
		t.Fatalf("duplicate X-Real-IP must be ignored: %q", got)
	}
	if got := clientIP(mk("127.0.0.1:5000", "not-an-ip")); got != "127.0.0.1" {
		t.Fatalf("garbage X-Real-IP: %q", got)
	}
}

func TestHeaderValidation(t *testing.T) {
	e := setup(t, http.HandlerFunc(streamHandler), queue.Config{}, nil)
	cases := map[string]string{
		"ok/host:1":                        "ok/host:1",
		"<script>alert(1)</script>":        invalidLabel,
		"with space":                       invalidLabel,
		strings.Repeat("a", maxLabelLen+1): invalidLabel,
		strings.Repeat("a", maxLabelLen):   strings.Repeat("a", maxLabelLen),
		"quote\"x":                         invalidLabel,
	}
	for in, want := range cases {
		resp, _ := post(t, e, "/api/generate", `{"model":"m"}`, map[string]string{HeaderClient: in, HeaderTool: "Bad Tool!"})
		rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
		if rec.ClientLabel != want || rec.Tool != "invalido" {
			t.Fatalf("label %q -> %q (tool %q), want %q", in, rec.ClientLabel, rec.Tool, want)
		}
	}
}

func TestModelAllowlist(t *testing.T) {
	var hits int
	var mu sync.Mutex
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		io.WriteString(w, finalChunk)
	}), queue.Config{}, func(c *Config) { c.AllowedModels = []string{"qwen3-ia", "nomic-embed-text"} })
	if resp, _ := post(t, e, "/api/generate", `{"model":"llama3"}`, nil); resp.StatusCode != 403 {
		t.Fatalf("denied model: %d", resp.StatusCode)
	}
	if resp, _ := post(t, e, "/api/generate", `{"prompt":"x"}`, nil); resp.StatusCode != 403 {
		t.Fatalf("empty model: %d", resp.StatusCode)
	}
	if resp, _ := post(t, e, "/api/generate", `not json`, nil); resp.StatusCode != 400 {
		t.Fatalf("bad json: %d", resp.StatusCode)
	}
	mu.Lock()
	if hits != 0 {
		t.Fatalf("a denied request reached the Ollama (%d)", hits)
	}
	mu.Unlock()
	if resp, _ := post(t, e, "/api/generate", `{"model":"qwen3-ia:latest"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("allowed model: %d", resp.StatusCode)
	}
}

func TestBodyTooLarge(t *testing.T) {
	e := setup(t, http.HandlerFunc(streamHandler), queue.Config{}, func(c *Config) { c.MaxBodyBytes = 64 })
	resp, _ := post(t, e, "/api/generate", `{"model":"m","prompt":"`+strings.Repeat("x", 200)+`"}`, nil)
	if resp.StatusCode != 413 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestFullQueueReturns503WithRetryAfter(t *testing.T) {
	release := make(chan struct{})
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		io.WriteString(w, finalChunk)
	}), queue.Config{GenSlots: 1, QueueMax: 1, PerClient: 5, PerIP: 5}, nil)
	defer close(release)
	go postBg(e, "/api/generate", `{"model":"m"}`, nil) // runs
	waitFor(t, func() bool { return len(e.q.Snapshot().Running) == 1 })
	go postBg(e, "/api/generate", `{"model":"m"}`, nil) // waits
	waitFor(t, func() bool { return len(e.q.Snapshot().Waiting) == 1 })
	resp, body := post(t, e, "/api/generate", `{"model":"m"}`, nil)
	if resp.StatusCode != 503 || resp.Header.Get("Retry-After") == "" || !strings.Contains(body, "fila") {
		t.Fatalf("%d %v %q", resp.StatusCode, resp.Header, body)
	}
	if !idRe.MatchString(resp.Header.Get(HeaderRequestID)) {
		t.Fatal("rejection without X-Request-Id")
	}
	rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if rec.Status != queue.StatusRejected {
		t.Fatalf("record %+v", rec)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func TestSecondRequestWaitsAndReportsQueueHeaders(t *testing.T) {
	gate := make(chan struct{})
	first := true
	var mu sync.Mutex
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		f := first
		first = false
		mu.Unlock()
		if f {
			<-gate
		}
		io.WriteString(w, finalChunk)
	}), queue.Config{GenSlots: 1}, nil)
	done := make(chan struct{})
	go func() { postBg(e, "/api/generate", `{"model":"m"}`, nil); close(done) }()
	waitFor(t, func() bool { return len(e.q.Snapshot().Running) == 1 })
	res := make(chan *http.Response, 1)
	go func() { r, _ := postBg(e, "/api/generate", `{"model":"m"}`, nil); res <- r }()
	waitFor(t, func() bool { return len(e.q.Snapshot().Waiting) == 1 })
	time.Sleep(40 * time.Millisecond)
	close(gate)
	r := <-res
	<-done
	if r.Header.Get(HeaderQueuePosition) != "1" {
		t.Fatalf("position %q", r.Header.Get(HeaderQueuePosition))
	}
	ms := r.Header.Get(HeaderQueueMS)
	if ms == "" || ms == "0" {
		t.Fatalf("X-Queue-Ms %q", ms)
	}
}

// A client that leaves in the middle of the stream must free the slot and
// close the connection to the Ollama.
func TestClientDisconnectMidStreamFreesSlot(t *testing.T) {
	upCancelled := make(chan struct{})
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"response":"a","done":false}`+"\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(upCancelled)
		case <-time.After(10 * time.Second):
		}
	}), queue.Config{GenSlots: 1}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", e.gw.URL+"/api/generate", strings.NewReader(`{"model":"m"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(resp.Body).ReadString('\n')
	if !strings.Contains(line, `"response":"a"`) {
		t.Fatalf("first chunk %q", line)
	}
	id := resp.Header.Get(HeaderRequestID)
	cancel()
	resp.Body.Close()
	select {
	case <-upCancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("the Ollama connection was not closed")
	}
	e.waitIdle(t)
	rec := e.waitRecord(t, id)
	if rec.Status != queue.StatusCanceled {
		t.Fatalf("record %+v", rec)
	}
}

func TestClientDisconnectWhileWaitingLeavesLine(t *testing.T) {
	gate := make(chan struct{})
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-gate
		io.WriteString(w, finalChunk)
	}), queue.Config{GenSlots: 1}, nil)
	defer close(gate)
	go postBg(e, "/api/generate", `{"model":"m"}`, nil)
	waitFor(t, func() bool { return len(e.q.Snapshot().Running) == 1 })
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", e.gw.URL+"/api/generate", strings.NewReader(`{"model":"m"}`))
	go http.DefaultClient.Do(req)
	waitFor(t, func() bool { return len(e.q.Snapshot().Waiting) == 1 })
	cancel()
	waitFor(t, func() bool { return len(e.q.Snapshot().Waiting) == 0 })
}

func TestAdminCancelRunningMidStream(t *testing.T) {
	upCancelled := make(chan struct{})
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"response":"a","done":false}`+"\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(upCancelled)
		case <-time.After(10 * time.Second):
		}
	}), queue.Config{GenSlots: 1}, nil)

	resp, err := http.Post(e.gw.URL+"/api/generate", "application/json", strings.NewReader(`{"model":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	id := resp.Header.Get(HeaderRequestID)
	if _, err := e.q.Cancel(id); err != nil {
		t.Fatal(err)
	}
	rest, _ := io.ReadAll(br)
	if !strings.Contains(string(rest), "cancelado pelo administrador") {
		t.Fatalf("the client did not get a clear error line: %q", rest)
	}
	select {
	case <-upCancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("the Ollama connection was not closed")
	}
	e.waitIdle(t)
	rec := e.waitRecord(t, id)
	if rec.Status != queue.StatusAdminCanceled {
		t.Fatalf("record %+v", rec)
	}
}

func TestAdminCancelBeforeResponseHeaders(t *testing.T) {
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body) // like the real Ollama: only a handler that read the body sees the disconnect
		<-r.Context().Done()
	}), queue.Config{GenSlots: 1}, nil)
	res := make(chan *http.Response, 1)
	go func() { r, _ := postBg(e, "/api/generate", `{"model":"m"}`, nil); res <- r }()
	waitFor(t, func() bool { return len(e.q.Snapshot().Running) == 1 })
	id := e.q.Snapshot().Running[0].ID
	if _, err := e.q.Cancel(id); err != nil {
		t.Fatal(err)
	}
	r := <-res
	if r.StatusCode != 409 {
		t.Fatalf("status %d", r.StatusCode)
	}
	e.waitIdle(t)
}

func TestAdminRemoveWaitingGets409(t *testing.T) {
	gate := make(chan struct{})
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-gate
		io.WriteString(w, finalChunk)
	}), queue.Config{GenSlots: 1}, nil)
	defer close(gate)
	go postBg(e, "/api/generate", `{"model":"m"}`, nil)
	waitFor(t, func() bool { return len(e.q.Snapshot().Running) == 1 })
	res := make(chan *http.Response, 1)
	body := make(chan string, 1)
	go func() { r, b := postBg(e, "/api/generate", `{"model":"m"}`, nil); res <- r; body <- b }()
	waitFor(t, func() bool { return len(e.q.Snapshot().Waiting) == 1 })
	if _, err := e.q.Remove(e.q.Snapshot().Waiting[0].ID); err != nil {
		t.Fatal(err)
	}
	r := <-res
	if r.StatusCode != 409 || !strings.Contains(<-body, "removido") {
		t.Fatalf("status %d", r.StatusCode)
	}
}

func TestUpstreamDownReturns502AndFreesSlot(t *testing.T) {
	e := setup(t, http.HandlerFunc(streamHandler), queue.Config{GenSlots: 1}, nil)
	e.up.Close()
	resp, _ := post(t, e, "/api/generate", `{"model":"m"}`, nil)
	if resp.StatusCode != 502 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if rec.Status != queue.StatusError || rec.ErrorClass != "upstream_unreachable" {
		t.Fatalf("record %+v", rec)
	}
	e.waitIdle(t)
}

func TestUpstreamErrorStatusIsRecorded(t *testing.T) {
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"model not found"}`)
	}), queue.Config{}, nil)
	resp, _ := post(t, e, "/api/generate", `{"model":"m"}`, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if rec.Status != queue.StatusError || rec.ErrorClass != "upstream_404" {
		t.Fatalf("record %+v", rec)
	}
}

func TestParseCounters(t *testing.T) {
	cases := []struct {
		name, in string
		pe, e, l int64
		errLine  bool
	}{
		{"final chunk", "{\"response\":\"x\",\"done\":false}\n" + finalChunk + "\n", 44, 41, 4000000, false},
		{"decoy in earlier line", "{\"response\":\"\\\"eval_count\\\":999\",\"done\":false}\n" + finalChunk, 44, 41, 4000000, false},
		{"decoy escaped in the final line", `{"response":"\"eval_count\":999","done":true,"eval_count":3,"prompt_eval_count":2}`, 2, 3, 0, false},
		{"error line", "{\"response\":\"x\"}\n{\"error\":\"boom\"}\n", 0, 0, 0, true},
		{"openai usage", "data: {\"x\":1}\n\ndata: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":6}}\n\ndata: [DONE]\n\n", 5, 6, 0, false},
		{"empty", "", 0, 0, 0, false},
	}
	for _, c := range cases {
		got := parseCounters([]byte(c.in))
		if got.promptEval != c.pe || got.eval != c.e || got.loadNS != c.l || got.errorLine != c.errLine {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}

func TestCountersFromLastChunkOnlyEvenWithHugeContext(t *testing.T) {
	big := strings.Repeat("123456,", 20000) // > 2x tailBytes of "context"
	line := `{"response":"","done":true,"context":[` + big + `1],"total_duration":5,"load_duration":6,"prompt_eval_count":7,"eval_count":8}`
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"response":"a","done":false,"eval_count":1}`+"\n"+line+"\n")
	}), queue.Config{}, nil)
	resp, body := post(t, e, "/api/generate", `{"model":"m"}`, nil)
	if !strings.Contains(body, `"eval_count":8`) {
		t.Fatal("body altered")
	}
	rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if rec.PromptEval != 7 || rec.Eval != 8 {
		t.Fatalf("record %+v", rec)
	}
}
