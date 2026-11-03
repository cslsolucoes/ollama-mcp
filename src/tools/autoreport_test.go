package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/treino"
	"github.com/cslsoftwares/mcpollama/ollama"
)

// Fase 12.5: the automatic candidates (D12-8, C-1.19, C-1.20).

// genReply is the model answer of the n-th generation/chat call (1-based): the
// text, the done_reason and the job id the fake gateway gives it ("" = none).
type genReply func(n int, prompt string) (text, done, id string)

func stopWith(text string) genReply {
	return func(n int, _ string) (string, string, string) { return text, "stop", jobID(n) }
}

// autoMock is a fake gateway for the automatic reports: it answers generate and
// chat with a job id and takes the reports on /api/feedback, which it can hold.
type autoMock struct {
	*mock
	gen genReply
	n   atomic.Int32

	fmu     sync.Mutex
	raws    [][]byte
	reply   fbReply
	entered chan struct{} // one value per report that arrived
	block   chan struct{} // when not nil, /api/feedback waits for it (or for the client to go)
}

func newAutoMock(t *testing.T, gen genReply) *autoMock {
	t.Helper()
	a := &autoMock{gen: gen, entered: make(chan struct{}, 64)}
	a.mock = newMock(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/feedback":
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r.Body)
			a.fmu.Lock()
			a.raws = append(a.raws, buf.Bytes())
			n, blk, reply := len(a.raws), a.block, a.reply
			a.fmu.Unlock()
			a.entered <- struct{}{}
			if blk != nil {
				select {
				case <-blk:
				case <-r.Context().Done():
					return
				}
			}
			status, body, hdr := 200, fbOK, map[string]string(nil)
			if reply != nil {
				status, body, hdr = reply(n)
			}
			for k, v := range hdr {
				w.Header().Set(k, v)
			}
			w.WriteHeader(status)
			fmt.Fprintln(w, body)
		default:
			var req struct {
				Prompt   string `json:"prompt"`
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			prompt := req.Prompt
			if len(req.Messages) > 0 {
				prompt = req.Messages[0].Content
			}
			text, done, id := a.gen(int(a.n.Add(1)), prompt)
			if id != "" {
				w.Header().Set("X-Request-Id", id)
			}
			tb, _ := json.Marshal(text)
			if r.URL.Path == "/api/chat" {
				streamLines(w,
					`{"model":"qwen3-ia","message":{"role":"assistant","content":`+string(tb)+`},"done":false}`,
					`{"model":"qwen3-ia","message":{"role":"assistant","content":""},"done":true,"done_reason":"`+done+`","total_duration":1000000000,"prompt_eval_count":10,"eval_count":2}`)
				return
			}
			streamLines(w,
				`{"model":"qwen3-ia","response":`+string(tb)+`,"done":false}`,
				`{"model":"qwen3-ia","response":"","done":true,"done_reason":"`+done+`","total_duration":1000000000,"prompt_eval_count":10,"eval_count":2}`)
		}
	})
	return a
}

// auto builds Deps with a running AutoReporter (closed with the test).
func (a *autoMock) auto(t *testing.T, size int, timeout time.Duration) Deps {
	t.Helper()
	d := a.deps(t, nil)
	r := newAutoReporter(d.Backend, nil, size, timeout)
	r.wait = func(time.Duration, int) time.Duration { return time.Millisecond }
	d.Auto = r
	t.Cleanup(r.Close)
	return d
}

func (a *autoMock) feedbacks() int {
	a.fmu.Lock()
	defer a.fmu.Unlock()
	return len(a.raws)
}

// waitReports waits until n reports have arrived (never more than 3 s).
func (a *autoMock) waitReports(t *testing.T, n int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for a.feedbacks() < n {
		select {
		case <-a.entered:
		case <-deadline:
			t.Fatalf("only %d of %d reports arrived", a.feedbacks(), n)
		}
	}
}

func (a *autoMock) report(t *testing.T, i int) treino.Report {
	t.Helper()
	a.fmu.Lock()
	raw := a.raws[i]
	a.fmu.Unlock()
	var rep treino.Report
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		t.Fatalf("the gateway would refuse the body: %v\n%s", err, raw)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("the gateway would refuse the report: %v", err)
	}
	return rep
}

// fire is a call that always hits a rule (empty answer, with a job id): used as
// the marker after a call that must NOT fire. The reporter is FIFO, so when the
// marker's report is the first and only one, nothing before it was sent.
func fire(t *testing.T, d Deps) {
	t.Helper()
	if _, err := call(t, d, "generate", `{"prompt":"MARCADOR"}`); err != nil {
		t.Fatal(err)
	}
}

func TestAutoRulesTable(t *testing.T) {
	n100, n255, n256, n4000 := 100, 255, 256, 4000
	cases := []struct {
		name string
		in   autoInput
		cat  string // "" = no hit
		desc string
	}{
		{"vazio", autoInput{Text: "", DoneReason: "stop"}, "outro", "resposta vazia"},
		{"só espaços", autoInput{Text: " \n\t  ", DoneReason: "stop"}, "outro", "resposta vazia"},
		{"texto com conteúdo", autoInput{Text: "ok", DoneReason: "stop"}, "", ""},
		{"JSON inválido com format", autoInput{Text: "{oops", DoneReason: "stop", JSONFormat: true}, "formato_invalido", ""},
		{"JSON válido com format", autoInput{Text: `{"a":1}`, DoneReason: "stop", JSONFormat: true}, "", ""},
		{"JSON inválido sem format", autoInput{Text: "{oops", DoneReason: "stop"}, "", ""},
		{"length sem num_predict", autoInput{Text: "corta", DoneReason: "length"}, "truncado", "resposta truncada: fim=length"},
		{"length com 256", autoInput{Text: "corta", DoneReason: "length", NumPredict: &n256}, "truncado", ""},
		{"length com 4000", autoInput{Text: "corta", DoneReason: "length", NumPredict: &n4000}, "truncado", ""},
		{"length com 255", autoInput{Text: "corta", DoneReason: "length", NumPredict: &n255}, "", ""},
		{"length com 100", autoInput{Text: "corta", DoneReason: "length", NumPredict: &n100}, "", ""},
		{"stop sem num_predict", autoInput{Text: "ok", DoneReason: "stop"}, "", ""},
	}
	for _, c := range cases {
		v, hit := autoRules(c.in)
		if hit != (c.cat != "") || v.categoria != c.cat {
			t.Errorf("%s: hit=%v cat=%q, want %q", c.name, hit, v.categoria, c.cat)
		}
		if c.desc != "" && v.descricao != c.desc {
			t.Errorf("%s: descricao %q", c.name, v.descricao)
		}
		if hit && (strings.Contains(v.descricao, c.in.Text) && c.in.Text != "" && len(c.in.Text) > 3) {
			t.Errorf("%s: the description carries the answer", c.name)
		}
	}
}

func TestAutoReportFiresForEmptyAnswer(t *testing.T) {
	for _, tool := range []struct{ name, args string }{
		{"generate", `{"prompt":"x"}`},
		{"chat", `{"messages":[{"role":"user","content":"x"}]}`},
	} {
		a := newAutoMock(t, stopWith("  \n "))
		d := a.auto(t, 4, 5*time.Second)
		res, err := call(t, d, tool.name, tool.args)
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", tool.name, err, res)
		}
		a.waitReports(t, 1)
		rep := a.report(t, 0)
		if rep.Origem != "automatico" || rep.Categoria != "outro" || rep.Descricao != "resposta vazia" || rep.RespostaCorreta != "" ||
			rep.JobDoErro != jobID(1) || len(rep.Jobs) != 1 || rep.Jobs[0].Papel != "unico" || rep.Jobs[0].Backend != "primario" || rep.Projeto == "" {
			t.Fatalf("%s: %+v", tool.name, rep)
		}
	}
}

func TestAutoReportFiresForTruncatedAnswer(t *testing.T) {
	for _, np := range []string{``, `,"num_predict":256`, `,"num_predict":4000`} {
		a := newAutoMock(t, func(n int, _ string) (string, string, string) { return "texto cortado", "length", jobID(n) })
		d := a.auto(t, 4, 5*time.Second)
		if _, err := call(t, d, "generate", `{"prompt":"x"`+np+`}`); err != nil {
			t.Fatal(err)
		}
		a.waitReports(t, 1)
		if rep := a.report(t, 0); rep.Categoria != "truncado" || rep.Origem != "automatico" || rep.RespostaCorreta != "" ||
			strings.Contains(rep.Descricao, "texto cortado") {
			t.Fatalf("num_predict %q: %+v", np, rep)
		}
	}
}

// Look-alikes do NOT fire: a marker call fires last and must be the only report.
func TestAutoReportDoesNotFireForLookAlikes(t *testing.T) {
	type lookalike struct{ name, tool, args, text, done string }
	cases := []lookalike{
		{"length com num_predict 100", "generate", `{"prompt":"x","num_predict":100}`, "curto de propósito", "length"},
		{"length com num_predict 255", "generate", `{"prompt":"x","num_predict":255}`, "curto de propósito", "length"},
		{"chat length com num_predict 100", "chat", `{"messages":[{"role":"user","content":"x"}],"num_predict":100}`, "curto", "length"},
		{"texto com conteúdo", "generate", `{"prompt":"x"}`, "uma resposta normal", "stop"},
		{"JSON inválido sem format", "generate", `{"prompt":"x"}`, "{isto não é json", "stop"},
		{"texto pedindo JSON na instrução", "generate", `{"prompt":"responda em JSON"}`, "não é json", "stop"},
	}
	for _, c := range cases {
		c := c
		a := newAutoMock(t, func(n int, prompt string) (string, string, string) {
			if prompt == "MARCADOR" {
				return "", "stop", jobID(n)
			}
			return c.text, c.done, jobID(n)
		})
		d := a.auto(t, 4, 5*time.Second)
		if res, err := call(t, d, c.tool, c.args); err != nil || res.IsError {
			t.Fatalf("%s: %v", c.name, err)
		}
		fire(t, d)
		a.waitReports(t, 1)
		time.Sleep(20 * time.Millisecond)
		if n := a.feedbacks(); n != 1 {
			t.Fatalf("%s: %d reports, want only the marker's", c.name, n)
		}
		if rep := a.report(t, 0); rep.JobDoErro != jobID(2) {
			t.Fatalf("%s: the report is not the marker's: %+v", c.name, rep)
		}
	}
}

// File tools: the rules look at the last call (the final of a map-reduce, the
// last part of a translation), which is the job in error.
func TestAutoReportFileToolsPointAtTheLastCall(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "pequeno.txt", "linha um\nlinha dois\n")
	e.put(t, "grande.txt", lines(200, 40))

	// one part, answer cut by the token limit: unico
	a := newAutoMock(t, func(n int, _ string) (string, string, string) { return "resumo cortado", "length", jobID(n) })
	d := e.deps(t, a.mock, 128000, 40)
	d.Auto = newAutoReporter(d.Backend, nil, 4, 5*time.Second)
	t.Cleanup(d.Auto.Close)
	if res, err := call(t, d, "summarize_file", `{"path":"pequeno.txt"}`); err != nil || res.IsError {
		t.Fatalf("%v", err)
	}
	a.waitReports(t, 1)
	if rep := a.report(t, 0); rep.Categoria != "truncado" || rep.JobDoErro != jobID(1) || len(rep.Jobs) != 1 || rep.Jobs[0].Papel != "unico" {
		t.Fatalf("single part: %+v", rep)
	}

	// map-reduce: only the final is cut -> job_do_erro is the final, the list has every job
	a2 := newAutoMock(t, func(n int, prompt string) (string, string, string) {
		if strings.Contains(prompt, "PARCIAL_INICIO") {
			return "combinado cortado", "length", jobID(n)
		}
		return "parcial", "stop", jobID(n)
	})
	d2 := e.deps(t, a2.mock, 2600, 40)
	d2.Auto = newAutoReporter(d2.Backend, nil, 4, 5*time.Second)
	t.Cleanup(d2.Auto.Close)
	if res, err := call(t, d2, "summarize_file", `{"path":"grande.txt"}`); err != nil || res.IsError {
		t.Fatalf("%v", err)
	}
	a2.waitReports(t, 1)
	rep := a2.report(t, 0)
	last := rep.Jobs[len(rep.Jobs)-1]
	if rep.Categoria != "truncado" || last.Papel != "final" || rep.JobDoErro != last.ID || len(rep.Jobs) < 3 {
		t.Fatalf("map-reduce: %+v", rep)
	}

	// translate (no final): the last part is the job in error
	a3 := newAutoMock(t, func(n int, _ string) (string, string, string) { return "parte", "length", jobID(n) })
	d3 := e.deps(t, a3.mock, 2600, 40)
	d3.Auto = newAutoReporter(d3.Backend, nil, 4, 5*time.Second)
	t.Cleanup(d3.Auto.Close)
	if res, err := call(t, d3, "translate", `{"path":"grande.txt","target_language":"English"}`); err != nil || res.IsError {
		t.Fatalf("%v", err)
	}
	a3.waitReports(t, 1)
	rep3 := a3.report(t, 0)
	last3 := rep3.Jobs[len(rep3.Jobs)-1]
	if !strings.HasPrefix(last3.Papel, "map") || rep3.JobDoErro != last3.ID || len(rep3.Jobs) < 3 {
		t.Fatalf("translate: %+v", rep3)
	}
	for _, j := range rep3.Jobs {
		if j.Papel == "final" {
			t.Fatal("translate has no final")
		}
	}
}

// Without a gateway job id nothing is sent.
func TestAutoReportWithoutJobIDSendsNothing(t *testing.T) {
	a := newAutoMock(t, func(n int, prompt string) (string, string, string) {
		if prompt == "MARCADOR" {
			return "", "stop", jobID(n)
		}
		return "", "stop", "" // no X-Request-Id: no gateway
	})
	d := a.auto(t, 4, 5*time.Second)
	if _, err := call(t, d, "generate", `{"prompt":"sem id"}`); err != nil {
		t.Fatal(err)
	}
	fire(t, d)
	a.waitReports(t, 1)
	time.Sleep(20 * time.Millisecond)
	if a.feedbacks() != 1 || a.report(t, 0).JobDoErro != jobID(2) {
		t.Fatalf("%d reports; the one without id must not be sent", a.feedbacks())
	}
}

// --auto-report=false (Deps.Auto nil): the same empty answer sends nothing.
func TestAutoReportOffSendsNothing(t *testing.T) {
	a := newAutoMock(t, stopWith(""))
	d := a.deps(t, nil) // Auto is nil
	if res, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil || res.IsError {
		t.Fatalf("%v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if a.feedbacks() != 0 {
		t.Fatalf("%d reports with the feature off", a.feedbacks())
	}
}

// The answer of the tool does not wait for the report, even when the gateway
// holds the report for ever, and is identical to the one without the feature.
func TestAutoReportNeverDelaysOrChangesTheAnswer(t *testing.T) {
	plain := newAutoMock(t, stopWith(""))
	want, err := call(t, plain.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil {
		t.Fatal(err)
	}

	a := newAutoMock(t, stopWith(""))
	a.block = make(chan struct{}) // never released: the report hangs
	d := a.auto(t, 4, 30*time.Second)
	start := time.Now()
	got, err := call(t, d, "generate", `{"prompt":"x"}`)
	took := time.Since(start)
	if err != nil || got.IsError {
		t.Fatalf("%v %+v", err, got)
	}
	if took > 700*time.Millisecond {
		t.Fatalf("the tool waited %v for the report", took)
	}
	if text(t, got) != text(t, want) || got.IsError != want.IsError {
		t.Fatalf("answer changed:\n%q\n%q", text(t, got), text(t, want))
	}
	// the sender did try (so the hang is real, not a missing send)
	select {
	case <-a.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("the report never reached the gateway")
	}
}

// A failing gateway (500, closed port) never fails the tool.
func TestAutoReportFailureNeverFailsTheTool(t *testing.T) {
	a := newAutoMock(t, stopWith(""))
	a.reply = func(int) (int, string, map[string]string) { return 500, `{"error":"boom"}`, nil }
	d := a.auto(t, 4, 5*time.Second)
	for i := 0; i < 3; i++ {
		if res, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil || res.IsError {
			t.Fatalf("%v %+v", err, res)
		}
	}
	a.waitReports(t, 3)

	// backend that cannot be built
	b := newAutoMock(t, stopWith(""))
	d2 := b.deps(t, nil)
	r := newAutoReporter(func() (Backend, error) { return nil, fmt.Errorf("sem cliente") }, nil, 4, time.Second)
	t.Cleanup(r.Close)
	d2.Auto = r
	if res, err := call(t, d2, "generate", `{"prompt":"x"}`); err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	deadline := time.Now().Add(2 * time.Second) // the sender really ran into the error
	for r.failed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if r.failed.Load() != 1 {
		t.Fatalf("failed = %d, want 1", r.failed.Load())
	}

	// a bug in the sender (here: the client builder panics) is contained: the
	// tool still answers and the process survives
	c := newAutoMock(t, stopWith(""))
	d3 := c.deps(t, nil)
	r3 := newAutoReporter(func() (Backend, error) { panic("bug no envio") }, nil, 4, time.Second)
	t.Cleanup(r3.Close)
	d3.Auto = r3
	if res, err := call(t, d3, "generate", `{"prompt":"x"}`); err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	deadline = time.Now().Add(2 * time.Second)
	for r3.failed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if r3.failed.Load() != 1 {
		t.Fatalf("the panic was not contained: failed = %d", r3.failed.Load())
	}
	if res, err := call(t, d3, "generate", `{"prompt":"x"}`); err != nil || res.IsError {
		t.Fatalf("the tool must keep working after the panic: %v", err)
	}
}

// A full queue drops the candidate (and says so on stderr/debug), never blocks.
func TestAutoReportFullQueueDrops(t *testing.T) {
	a := newAutoMock(t, stopWith(""))
	a.block = make(chan struct{})
	d := a.deps(t, nil)
	var mu sync.Mutex
	var logs []string
	r := newAutoReporter(d.Backend, func(f string, args ...any) { mu.Lock(); logs = append(logs, fmt.Sprintf(f, args...)); mu.Unlock() }, 2, 30*time.Second)
	t.Cleanup(r.Close)
	d.Auto = r

	if _, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil {
		t.Fatal(err)
	}
	select { // the sender holds the first report: the queue is empty again
	case <-a.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first report never arrived")
	}
	start := time.Now()
	for i := 0; i < 5; i++ {
		if res, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil || res.IsError {
			t.Fatalf("%v", err)
		}
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("calls blocked on a full queue: %v", time.Since(start))
	}
	if got := r.dropped.Load(); got != 3 { // 5 submitted, 2 fit in the queue
		t.Fatalf("dropped = %d, want 3", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(logs) != 3 || !strings.Contains(logs[0], "fila cheia") {
		t.Fatalf("logs: %v", logs)
	}
}

// 409 on an automatic report: up to 3 retries after the first try.
func TestAutoReport409RetriesUpToThree(t *testing.T) {
	a := newAutoMock(t, stopWith(""))
	a.reply = func(n int) (int, string, map[string]string) {
		if n <= 2 {
			return 409, `{"error":"job em execução"}`, map[string]string{"Retry-After": "1"}
		}
		return 200, fbOK, nil
	}
	d := a.auto(t, 4, 5*time.Second)
	if _, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil {
		t.Fatal(err)
	}
	a.waitReports(t, 3)
	deadline := time.Now().Add(2 * time.Second)
	for d.Auto.sent.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if d.Auto.sent.Load() != 1 || a.feedbacks() != 3 {
		t.Fatalf("sent=%d requests=%d, want 1 and 3", d.Auto.sent.Load(), a.feedbacks())
	}

	// always 409: the first try plus exactly three retries, then it gives up
	b := newAutoMock(t, stopWith(""))
	b.reply = func(int) (int, string, map[string]string) {
		return 409, `{"error":"job em execução"}`, map[string]string{"Retry-After": "1"}
	}
	d2 := b.auto(t, 4, 5*time.Second)
	if _, err := call(t, d2, "generate", `{"prompt":"x"}`); err != nil {
		t.Fatal(err)
	}
	b.waitReports(t, 4)
	time.Sleep(60 * time.Millisecond)
	if b.feedbacks() != 4 || d2.Auto.sent.Load() != 0 {
		t.Fatalf("requests=%d sent=%d, want 4 and 0", b.feedbacks(), d2.Auto.sent.Load())
	}
}

// Close ends the goroutine even with a report hanging at the gateway.
func TestAutoReporterCloseEndsTheGoroutine(t *testing.T) {
	a := newAutoMock(t, stopWith(""))
	a.block = make(chan struct{})
	d := a.deps(t, nil)
	r := newAutoReporter(d.Backend, nil, 4, 30*time.Second)
	d.Auto = r
	if _, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil {
		t.Fatal(err)
	}
	<-a.entered // a send is in flight and blocked
	start := time.Now()
	r.Close()
	select {
	case <-r.done:
	default:
		t.Fatal("the goroutine is still running after Close")
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("Close took %v", time.Since(start))
	}
	r.Close() // idempotent
	// after Close a new candidate is dropped, not queued
	before := a.feedbacks()
	if _, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if a.feedbacks() != before {
		t.Fatal("a report was sent after Close")
	}
}

// Automatic reports go to the backend of the job in error, like the manual ones.
func TestAutoReportGoesToTheFallbackOfTheJob(t *testing.T) {
	primary := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":"fila do servidor cheia"}`, http.StatusServiceUnavailable)
	})
	fb := newAutoMock(t, func(n int, _ string) (string, string, string) { return "", "stop", jobID(0x100 + n) })
	d := primary.deps(t, func(o *ollama.Options) { o.FallbackURL = fb.srv.URL; o.FallbackAPIKey = fbToken })
	r := newAutoReporter(d.Backend, nil, 4, 5*time.Second)
	t.Cleanup(r.Close)
	d.Auto = r
	if res, err := call(t, d, "generate", `{"prompt":"x"}`); err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	fb.waitReports(t, 1)
	rep := fb.report(t, 0)
	if rep.Jobs[0].Backend != "fallback" || rep.JobDoErro != jobID(0x101) {
		t.Fatalf("%+v", rep)
	}
}
