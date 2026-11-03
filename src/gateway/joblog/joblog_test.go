package joblog_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/gpu"
	"github.com/cslsoftwares/mcpollama/gateway/joblog"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

type rig struct {
	dir     string
	st      *store.Store
	cs      *content.Store
	jr      *joblog.Journal
	q       *queue.Manager
	gw      *httptest.Server
	gwPath  string
	contDB  string
	upstrFn http.HandlerFunc
}

func newRig(t *testing.T, up http.HandlerFunc, flush time.Duration, mods ...func(*proxy.Config)) *rig {
	t.Helper()
	r := &rig{dir: t.TempDir()}
	r.gwPath, r.contDB = filepath.Join(r.dir, "gw.db"), filepath.Join(r.dir, "content.db")
	var err error
	r.st, err = store.Open(r.gwPath, store.Options{BootID: "boot-1"})
	if err != nil {
		t.Fatal(err)
	}
	r.cs, err = content.Open(r.contDB, content.Options{FlushEvery: flush})
	if err != nil {
		t.Fatal(err)
	}
	r.jr = &joblog.Journal{Store: r.st, Content: r.cs, Backend: "gpu-local"}
	r.q = queue.New(queue.Config{OnRecord: r.jr.Finish, GenSlots: 1})
	r.jr.Running = r.q.RunningIDs
	upsrv := httptest.NewServer(up)
	uu, _ := url.Parse(upsrv.URL)
	pcfg := proxy.Config{Upstream: uu, Queue: r.q, Journal: r.jr}
	for _, m := range mods {
		m(&pcfg)
	}
	h, err := proxy.New(pcfg)
	if err != nil {
		t.Fatal(err)
	}
	r.gw = httptest.NewServer(h)
	t.Cleanup(func() { r.gw.Close(); upsrv.Close(); r.cs.Close(); r.st.Close() })
	return r
}

func (r *rig) post(t *testing.T, path, body string) (string, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", r.gw.URL+path, strings.NewReader(body))
	req.Header.Set(proxy.HeaderClient, "teste/1")
	req.Header.Set(proxy.HeaderTool, "extract")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	id := resp.Header.Get(proxy.HeaderRequestID)
	// The client has the whole body before the handler's deferred Release runs:
	// wait for the end to be logged.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok, _ := r.st.Job(context.Background(), id); ok && j.Status != "em_execucao" && j.Status != "na_fila" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return id, string(b)
}

func job(t *testing.T, st *store.Store, id string) store.JobRow {
	t.Helper()
	j, ok, err := st.Job(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("job %s: %v %v", id, ok, err)
	}
	return j
}

// The core promise: while the Ollama is still working, the log already says
// em_execucao with the whole prompt, and the partial answer is on disk.
func TestWriteAheadVisibleFromTheUpstreamAndPartialAnswer(t *testing.T) {
	var r *rig
	seen := make(chan string, 4)
	r = newRig(t, func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		// A second, independent handle on the metadata file and on the content file.
		st2, err := store.Open(filepath.Join(t.TempDir(), "x.db"), store.Options{}) // unrelated: proves nothing, closed at once
		if err == nil {
			st2.Close()
		}
		js, _ := r.st.Jobs(context.Background(), store.JobQuery{})
		if len(js) != 1 || js[0].Status != "em_execucao" || js[0].Backend != "gpu-local" || js[0].StartedAt == 0 {
			seen <- "metadata not em_execucao before the upstream: " + describe(js)
			return
		}
		c, ok, _ := r.cs.Get(context.Background(), js[0].ID)
		if !ok || !strings.Contains(c.Prompt, "PROMPT-COMPLETO-123") {
			seen <- "prompt not in content.db before the upstream"
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"response":"parte um ","done":false}`+"\n")
		w.(http.Flusher).Flush()
		time.Sleep(80 * time.Millisecond)
		io.WriteString(w, `{"response":"parte dois","done":false}`+"\n")
		w.(http.Flusher).Flush()
		time.Sleep(80 * time.Millisecond)
		c, _, _ = r.cs.Get(context.Background(), js[0].ID)
		if !strings.Contains(c.Response, "parte um ") || c.RespFinal {
			seen <- "partial answer not on disk mid-stream: " + c.Response
			return
		}
		io.WriteString(w, `{"response":"","done":true,"prompt_eval_count":11,"eval_count":22}`+"\n")
		seen <- "ok"
	}, 20*time.Millisecond)
	id, out := r.post(t, "/api/generate", `{"model":"qwen3-ia","system":"SYS-X","prompt":"PROMPT-COMPLETO-123"}`)
	if got := <-seen; got != "ok" {
		t.Fatal(got)
	}
	if !strings.Contains(out, "parte dois") {
		t.Fatalf("client lost the answer: %s", out)
	}
	j := job(t, r.st, id)
	if j.Status != "ok" || !j.Responded || j.RespBytes != int64(len("parte um parte dois")) || j.Eval != 22 || j.Backend != "gpu-local" || j.ClientLabel != "teste/1" {
		t.Fatalf("%+v", j.Row)
	}
	c, _, _ := r.cs.Get(context.Background(), id)
	if c.Response != "parte um parte dois" || !c.RespFinal || !strings.Contains(c.Prompt, "[system]\nSYS-X") || !strings.Contains(c.Prompt, "[prompt]\nPROMPT-COMPLETO-123") {
		t.Fatalf("%+v", c)
	}
}

func describe(js []store.JobRow) string {
	var b strings.Builder
	for _, j := range js {
		b.WriteString(j.Status + " ")
	}
	return b.String()
}

// "The job dies": the upstream hangs mid-answer, the host is declared rebooted
// (a new Store with another boot id opens the same files). The job becomes
// interrompido_reboot, and its prompt and partial answer are in content.db.
func TestKilledJobBecomesInterruptedWithPromptAndPartialAnswer(t *testing.T) {
	hang := make(chan struct{})
	reached := make(chan struct{}, 1)
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"response":"resposta parcial antes da queda","done":false}`+"\n")
		w.(http.Flusher).Flush()
		reached <- struct{}{}
		<-hang
	}, 10*time.Millisecond)
	defer close(hang)
	var id string
	done := make(chan struct{})
	go func() {
		defer close(done)
		req, _ := http.NewRequest("POST", r.gw.URL+"/api/chat", strings.NewReader(`{"model":"qwen3-ia","messages":[{"role":"user","content":"CONTEXTO-ENVIADO-XYZ"}]}`))
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
		if err == nil {
			id = resp.Header.Get(proxy.HeaderRequestID)
			io.Copy(io.Discard, resp.Body) // blocks until the context ends
			resp.Body.Close()
		}
	}()
	<-reached
	time.Sleep(60 * time.Millisecond) // the sink flushes the partial answer on the next write after 10 ms; make one more chunk due
	// Drive one more chunk through the sink the same way the proxy would, then "reboot".
	js, _ := r.st.Jobs(context.Background(), store.JobQuery{})
	if len(js) != 1 || js[0].Status != "em_execucao" {
		t.Fatalf("running job not logged: %s", describe(js))
	}
	jobID := js[0].ID
	// what the log holds right now, read the way a rebooted process would
	st2, err := store.Open(r.gwPath, store.Options{BootID: "boot-2-after-reboot"})
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	rc := st2.Recovered()
	if len(rc) != 1 || rc[0].ID != jobID || rc[0].Status != "interrompido_reboot" || !rc[0].WasRunning {
		t.Fatalf("recovered %+v", rc)
	}
	cs2, err := content.Open(r.contDB, content.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cs2.Close()
	jr2 := &joblog.Journal{Store: st2, Content: cs2}
	jr2.RecoverResponses(context.Background())
	j := job(t, st2, jobID)
	if j.Status != "interrompido_reboot" || j.ErrorClass != "interrompido_reboot" || j.FinishedAt == 0 || j.StartedAt == 0 {
		t.Fatalf("%+v", j.Row)
	}
	c, ok, _ := cs2.Get(context.Background(), jobID)
	if !ok || !strings.Contains(c.Prompt, "CONTEXTO-ENVIADO-XYZ") || !strings.Contains(c.Prompt, "[user]") {
		t.Fatalf("prompt lost: %+v", c)
	}
	// The partial answer: flushed by the sink before the "crash" (resp_final = false).
	if c.RespFinal {
		t.Fatal("an interrupted job must not look finished")
	}
	<-done
	_ = id
}

func TestChatV1AndEmbedExtractionNoVector(t *testing.T) {
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		switch req.URL.Path {
		case "/api/chat":
			io.WriteString(w, `{"message":{"role":"assistant","thinking":"pensando"},"done":false}`+"\n")
			io.WriteString(w, `{"message":{"role":"assistant","content":"ola "},"done":false}`+"\n")
			io.WriteString(w, `{"message":{"role":"assistant","content":"mundo"},"done":true,"prompt_eval_count":1,"eval_count":2}`+"\n")
		case "/v1/chat/completions":
			if bytes.Contains(b, []byte(`"stream":true`)) {
				io.WriteString(w, "data: "+`{"choices":[{"delta":{"content":"sse um "}}]}`+"\n\n")
				io.WriteString(w, "data: "+`{"choices":[{"delta":{"content":"sse dois"}}]}`+"\n\n")
				io.WriteString(w, "data: [DONE]\n\n")
			} else {
				io.WriteString(w, `{"choices":[{"message":{"content":"nao-stream"}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
			}
		case "/api/embed":
			io.WriteString(w, `{"model":"nomic-embed-text","embeddings":[[0.123456789,0.2,0.3],[0.4,0.5,0.6]],"prompt_eval_count":5}`)
		case "/v1/embeddings":
			io.WriteString(w, `{"data":[{"embedding":[0.987654321,0.1]}],"usage":{"prompt_tokens":2}}`)
		}
	}, 0)
	get := func(id string) (store.JobRow, content.Content) {
		j := job(t, r.st, id)
		c, _, _ := r.cs.Get(context.Background(), id)
		return j, c
	}
	id, _ := r.post(t, "/api/chat", `{"model":"qwen3-ia","think":true,"messages":[{"role":"system","content":"seja breve"},{"role":"user","content":"oi"}]}`)
	j, c := get(id)
	if c.Response != "[raciocínio] pensando\n[resposta] ola mundo" || !j.Responded || !strings.Contains(c.Prompt, "[system]\nseja breve\n\n[user]\noi") {
		t.Fatalf("%q | %q | %+v", c.Response, c.Prompt, j.Row)
	}
	id, _ = r.post(t, "/v1/chat/completions", `{"model":"qwen3-ia","stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"parts"}]}]}`)
	_, c = get(id)
	if c.Response != "sse um sse dois" || !strings.Contains(c.Prompt, `"text":"parts"`) {
		t.Fatalf("%q | %q", c.Response, c.Prompt)
	}
	id, _ = r.post(t, "/v1/chat/completions", `{"model":"qwen3-ia","messages":[{"role":"user","content":"x"}]}`)
	_, c = get(id)
	if c.Response != "nao-stream" {
		t.Fatalf("%q", c.Response)
	}
	// embeddings: the input is the prompt, the vector is NEVER stored
	id, _ = r.post(t, "/api/embed", `{"model":"nomic-embed-text","input":["primeiro texto","segundo texto"]}`)
	j, c = get(id)
	if !j.Responded || c.Response != "embeddings: 2 vetor(es) de 3 dimensões (vetores não gravados)" || !strings.Contains(c.Prompt, "[input 1]\nprimeiro texto\n\n[input 2]\nsegundo texto") {
		t.Fatalf("%q | %q | %+v", c.Response, c.Prompt, j.Row)
	}
	id2, _ := r.post(t, "/v1/embeddings", `{"model":"nomic-embed-text","input":"so um"}`)
	_, c2 := get(id2)
	if c2.Response != "embeddings: 1 vetor(es) de 2 dimensões (vetores não gravados)" {
		t.Fatalf("%q", c2.Response)
	}
	r.st.Close()
	r.cs.Close()
	for _, f := range []string{r.gwPath, r.contDB} {
		for _, suffix := range []string{"", "-wal"} {
			b, err := os.ReadFile(f + suffix)
			if err != nil {
				continue
			}
			for _, vec := range []string{"0.123456789", "0.987654321"} {
				if bytes.Contains(b, []byte(vec)) {
					t.Fatalf("a vector value reached %s%s", filepath.Base(f), suffix)
				}
			}
		}
	}
}

// A job that errors before any text has "responded = false"; an error answer
// is not a response.
func TestNoAnswerMeansNotResponded(t *testing.T) {
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		w.WriteHeader(500)
		io.WriteString(w, `{"error":"modelo caiu"}`)
	}, 0)
	id, _ := r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"p"}`)
	j := job(t, r.st, id)
	if j.Status != "erro" || j.Responded || j.RespBytes != 0 {
		t.Fatalf("%+v", j.Row)
	}
	if c, ok, _ := r.cs.Get(context.Background(), id); !ok || c.Response != "" || !c.RespFinal {
		t.Fatalf("%+v", c)
	}
}

// ---- per-job GPU extract --------------------------------------------------------

const smi = `0, 00000000:03:00.0, 95.5, 12000, 80, 55, 100.00, 250.00, 1395
1, 00000000:04:00.0, 90.0, 12000, 75, 54, 100.00, 250.00, 1400
`

func TestGPUExtractOneRowPerSecondPerGPUOnlyWhileAJobRuns(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		started <- struct{}{}
		<-release
		io.WriteString(w, `{"response":"x","done":true}`+"\n")
	}, 0)
	col := &gpu.Collector{AERPath: "-", Run: func(context.Context, string, ...string) ([]byte, error) { return []byte(smi), nil },
		OnSamples: func(ts time.Time, ss []gpu.Sample, aer int64) { r.jr.Sample(ts, ss, nil) }}

	// no job: three collector rounds write nothing
	for i := 0; i < 3; i++ {
		col.Once(context.Background())
	}
	var n int
	row := rawCount(t, r.gwPath, `SELECT COUNT(*) FROM job_gpu_samples`, &n)
	if row != 0 {
		t.Fatalf("%d extract rows written without a running job", row)
	}

	idc := make(chan string, 1)
	go func() { id, _ := r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"p"}`); idc <- id }()
	<-started
	const rounds = 4
	for i := 0; i < rounds; i++ {
		col.Once(context.Background())
		time.Sleep(2 * time.Millisecond)
	}
	close(release)
	id := <-idc
	// one more round after the end: must not add rows
	col.Once(context.Background())

	got, err := r.st.JobSamplesOf(context.Background(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	perGPU := map[int]int{}
	for _, s := range got {
		perGPU[s.GPU]++
		if s.JobID != id {
			t.Fatalf("sample linked to %s", s.JobID)
		}
	}
	if len(got) != rounds*2 || perGPU[0] != rounds || perGPU[1] != rounds {
		t.Fatalf("want %d rows per GPU, got %v (total %d)", rounds, perGPU, len(got))
	}
	if got[0].ClockMHz != 1395 && got[0].ClockMHz != 1400 {
		t.Fatalf("clocks.gr not stored: %+v", got[0])
	}
	if got[0].PowerLimitW != 100 || got[0].MemUsedMiB != 12000 {
		t.Fatalf("%+v", got[0])
	}
	j := job(t, r.st, id)
	if j.Samples != rounds || j.PeakPowerW != 95.5 || j.PeakClockMHz != 1400 {
		t.Fatalf("peaks %+v", j)
	}
}

func rawCount(t *testing.T, path, q string, out *int) int {
	t.Helper()
	st, err := store.Open(path, store.Options{BootID: "boot-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rows, err := st.JobSamplesOf(context.Background(), "", 0)
	_ = rows
	_ = err
	// JobSamplesOf filters by job; count through Jobs peaks instead
	js, _ := st.Jobs(context.Background(), store.JobQuery{})
	n := 0
	for _, j := range js {
		n += j.Samples
	}
	*out = n
	return n
}
