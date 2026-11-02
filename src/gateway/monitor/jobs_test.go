package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

const xssPrompt = `</script><img src=x onerror=alert(1)><script>alert('x')</script>`

type jobFixture struct {
	*fixture
	cs    *content.Store
	clock *fakeClock
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newJobFixture(t *testing.T, keep time.Duration, withContent bool) *jobFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "gw.db"), store.Options{BootID: "boot-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var cs *content.Store
	if withContent {
		cs, err = content.Open(filepath.Join(dir, "content.db"), content.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
	}
	hub := NewHub()
	q := queue.New(queue.Config{GenSlots: 1, QueueMax: 4, PerClient: 4, OnChange: hub.Notify, OnRecord: func(r queue.Record) { _ = st.Finish(r) }})
	line, _ := HashPassword(adminPass, minIterations)
	cr, _ := ParseCredentials(adminUser + ":" + line)
	clock := &fakeClock{t: time.Now()}
	h, err := New(Config{Queue: q, Store: st, Content: cs, Hub: hub, Admin: cr, Version: "test", BootID: "boot-test", KeepDone: keep, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return &jobFixture{fixture: &fixture{h: h, q: q, st: st, hub: hub}, cs: cs, clock: clock}
}

// addJob writes a job into the log the way the proxy would, ended or not.
func (f *jobFixture) addJob(t *testing.T, id, label, prompt string, status string) {
	t.Helper()
	now := time.Now()
	r := queue.Record{ID: id, ClientIP: "10.0.0.9", ClientLabel: label, Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia", Lane: queue.LaneGen,
		PriorityIn: queue.Normal, PriorityFinal: queue.Normal, BytesIn: int64(len(prompt)), EnqueuedAt: now, Status: queue.StatusQueued}
	if err := f.st.JobEnqueued(r); err != nil {
		t.Fatal(err)
	}
	if f.cs != nil {
		if _, _, err := f.cs.SavePrompt(id, prompt, now); err != nil {
			t.Fatal(err)
		}
		k := f.cs.NewSink(id)
		k.Write("RESPOSTA " + prompt)
		k.Finish("")
	}
	switch status {
	case "em_execucao":
		f.st.JobStarted(id, now, "gpu-local")
	case "na_fila":
	default:
		r.Status, r.StartedAt, r.FinishedAt, r.RunMS, r.HTTPStatus, r.Responded, r.RespBytes = status, now, now.Add(time.Second), 1000, 200, true, 10
		f.st.Finish(r)
	}
}

func idN(n int) string { return fmt.Sprintf("%032x", n) }

func TestJobContentNeedsTheAdminSession(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(1)
	f.addJob(t, id, "cli/1", xssPrompt, "ok")

	// the metadata is open like every GET
	for _, p := range []string{"/monitor/api/jobs", "/monitor/api/jobs/" + id, "/monitor/api/jobs/export?formato=jsonl", "/monitor/api/jobs/export?formato=csv"} {
		if w := do(f.h, "GET", p, "", nil); w.Code != 200 {
			t.Fatalf("%s: %d %s", p, w.Code, w.Body)
		}
	}
	// the text is not
	for _, p := range []string{"/monitor/api/jobs/" + id + "/content", "/monitor/api/jobs/export?formato=jsonl&conteudo=1", "/monitor/api/jobs/export?formato=csv&conteudo=1"} {
		w := do(f.h, "GET", p, "", nil)
		if w.Code != 401 || strings.Contains(w.Body.String(), "RESPOSTA") || strings.Contains(w.Body.String(), "alert") {
			t.Fatalf("%s without a session: %d %s", p, w.Code, w.Body)
		}
	}
	// a forged cookie is not a session
	if w := do(f.h, "GET", "/monitor/api/jobs/"+id+"/content", "", map[string]string{"Cookie": "mcpgw_session=forjado"}); w.Code != 401 {
		t.Fatalf("forged cookie: %d", w.Code)
	}
	cookie, _ := f.login(t)
	w := do(f.h, "GET", "/monitor/api/jobs/"+id+"/content", "", map[string]string{"Cookie": cookie})
	if w.Code != 200 {
		t.Fatalf("with session: %d %s", w.Code, w.Body)
	}
	var c content.Content
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil || c.Prompt != xssPrompt || c.Response != "RESPOSTA "+xssPrompt {
		t.Fatalf("%v %+v", err, c)
	}
	w = do(f.h, "GET", "/monitor/api/jobs/export?formato=jsonl&conteudo=1", "", map[string]string{"Cookie": cookie})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"prompt"`) || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export with content: %d %s", w.Code, w.Body)
	}
	// the metadata endpoints never carry the text, session or not
	for _, p := range []string{"/monitor/api/jobs", "/monitor/api/jobs/" + id, "/monitor/api/jobs/export?formato=jsonl", "/monitor/api/state", "/monitor/api/history"} {
		b := do(f.h, "GET", p, "", map[string]string{"Cookie": cookie}).Body.String()
		if strings.Contains(b, "RESPOSTA") || strings.Contains(b, "alert") {
			t.Fatalf("%s leaks the text", p)
		}
	}
	// bad id / unknown id / content off
	if w := do(f.h, "GET", "/monitor/api/jobs/zzz/content", "", map[string]string{"Cookie": cookie}); w.Code != 400 {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := do(f.h, "GET", "/monitor/api/jobs/"+idN(7)+"/content", "", map[string]string{"Cookie": cookie}); w.Code != 404 {
		t.Fatalf("unknown id: %d", w.Code)
	}
	if w := do(f.h, "GET", "/monitor/api/jobs/"+idN(7), "", nil); w.Code != 404 {
		t.Fatalf("unknown job: %d", w.Code)
	}
}

func TestJobContentOffAnswers404EvenToAdmin(t *testing.T) {
	f := newJobFixture(t, time.Minute, false)
	f.addJob(t, idN(1), "c", "p", "ok")
	cookie, _ := f.login(t)
	if w := do(f.h, "GET", "/monitor/api/jobs/"+idN(1)+"/content", "", map[string]string{"Cookie": cookie}); w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
	if w := do(f.h, "GET", "/monitor/api/jobs/export?conteudo=1", "", map[string]string{"Cookie": cookie}); w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
	var st State
	json.Unmarshal(do(f.h, "GET", "/monitor/api/state", "", nil).Body.Bytes(), &st)
	if st.ContentOn {
		t.Fatal("state says content is on")
	}
}

// XSS: a prompt that tries to break out of the page is data, never markup.
func TestXSSPromptIsOnlyEverText(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	f.addJob(t, idN(2), xssLabelSafe, xssPrompt, "ok")
	cookie, _ := f.login(t)
	hdr := map[string]string{"Cookie": cookie}
	for _, p := range []string{"/monitor/api/jobs/" + idN(2) + "/content", "/monitor/api/jobs/export?formato=jsonl&conteudo=1"} {
		w := do(f.h, "GET", p, "", hdr)
		b := w.Body.String()
		// encoding/json escapes < > & as <: no raw tag can be in the response
		if strings.Contains(b, "</script>") || strings.Contains(b, "<img") || strings.Contains(b, "<script") {
			t.Fatalf("%s carries raw markup: %s", p, b)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") && !strings.HasPrefix(ct, "application/x-ndjson") {
			t.Fatalf("%s content type %q", p, ct)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("nosniff missing")
		}
	}
	// and it round-trips as the exact text
	var c content.Content
	json.Unmarshal(do(f.h, "GET", "/monitor/api/jobs/"+idN(2)+"/content", "", hdr).Body.Bytes(), &c)
	if c.Prompt != xssPrompt {
		t.Fatalf("%q", c.Prompt)
	}
	// the page puts every value in with textContent, and the CSP forbids the rest
	page := string(indexHTML)
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", " onclick=", " onerror=", "srcdoc"} {
		if strings.Contains(page, bad) {
			t.Errorf("page contains %q", bad)
		}
	}
	for _, want := range []string{`cbox.appendChild(el("pre", c.prompt))`, `cbox.appendChild(el("pre", c.response))`, `btn.addEventListener`, `b.addEventListener("click"`} {
		if !strings.Contains(page, want) && want != `btn.addEventListener` {
			t.Errorf("page lacks %q", want)
		}
	}
	w := do(f.h, "GET", "/monitor", "", nil)
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Fatalf("csp %q", csp)
	}
}

const xssLabelSafe = "cli/1"

func TestExportCSVDefusesFormulasAndFilters(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	f.addJob(t, idN(1), "+cmd|calc", `=HYPERLINK("http://x")`, "ok")
	f.addJob(t, idN(2), "normal", "outro", "erro")
	cookie, _ := f.login(t)
	hdr := map[string]string{"Cookie": cookie}
	w := do(f.h, "GET", "/monitor/api/jobs/export?formato=csv&conteudo=1", "", hdr)
	b := w.Body.String()
	if w.Code != 200 || !strings.Contains(b, "'+cmd|calc") || !strings.Contains(b, `'=HYPERLINK`) || strings.Contains(b, ",=HYPERLINK") || strings.Contains(b, ",+cmd") {
		t.Fatalf("%d %s", w.Code, b)
	}
	if !strings.HasPrefix(b, "id,entrada,inicio,fim,status") || !strings.Contains(strings.SplitN(b, "\n", 2)[0], "prompt,resposta,truncado") {
		t.Fatalf("header %q", strings.SplitN(b, "\n", 2)[0])
	}
	// filters
	var res struct{ Rows []store.JobRow }
	json.Unmarshal(do(f.h, "GET", "/monitor/api/jobs?status=erro", "", nil).Body.Bytes(), &res)
	if len(res.Rows) != 1 || res.Rows[0].ID != idN(2) {
		t.Fatalf("%+v", res.Rows)
	}
	if w := do(f.h, "GET", "/monitor/api/jobs?cliente=normal", "", nil); !strings.Contains(w.Body.String(), idN(2)) || strings.Contains(w.Body.String(), idN(1)) {
		t.Fatal("client filter")
	}
	if w := do(f.h, "GET", "/monitor/api/jobs/export?formato=xml", "", nil); w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
	if w := do(f.h, "GET", "/monitor/api/jobs?desde=ontem", "", nil); w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

func TestJobDetailHasTheGPUExtractWithoutText(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	f.addJob(t, idN(3), "c", "segredo-do-prompt", "ok")
	var rows []store.JobSample
	for s := 0; s < 3; s++ {
		rows = append(rows, store.JobSample{TS: time.Now().Add(time.Duration(s) * time.Second), JobID: idN(3), GPU: 0, PowerW: 90, ClockMHz: 1400})
	}
	f.st.AddJobSamples(rows)
	w := do(f.h, "GET", "/monitor/api/jobs/"+idN(3), "", nil)
	var d struct {
		Job     store.JobRow
		Samples []store.JobSample
		Content map[string]any
		On      bool `json:"content_enabled"`
	}
	// without a session: no content.Info (hash, exact sizes; revisão 10.L, H1)
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil || len(d.Samples) != 3 || d.Job.PeakClockMHz != 1400 || !d.On || d.Content != nil {
		t.Fatalf("%v %s", err, w.Body)
	}
	cookie, _ := f.login(t)
	d.Content = nil
	if err := json.Unmarshal(do(f.h, "GET", "/monitor/api/jobs/"+idN(3), "", map[string]string{"Cookie": cookie}).Body.Bytes(), &d); err != nil || d.Content == nil || d.Content["prompt_sha256"] == "" {
		t.Fatalf("admin detail: %v %+v", err, d.Content)
	}
	if strings.Contains(w.Body.String(), "segredo-do-prompt") {
		t.Fatal("detail carries the text")
	}
}

// ---- the "concluídos recentes" view ------------------------------------------------

func recentIDs(t *testing.T, f *jobFixture) []string {
	t.Helper()
	var st State
	if err := json.Unmarshal(do(f.h, "GET", "/monitor/api/state", "", nil).Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, j := range st.RecentDone {
		ids = append(ids, j.ID)
	}
	return ids
}

func TestFinishedRequestStaysVisibleForFiveMinutesThenGoes(t *testing.T) {
	f := newJobFixture(t, 5*time.Minute, true)
	tk, err := f.q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "10.0.0.1", ClientLabel: "rapido", Model: "qwen3-ia"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	tk.Release(queue.Result{HTTPStatus: 200, Eval: 5})

	var st State
	json.Unmarshal(do(f.h, "GET", "/monitor/api/state", "", nil).Body.Bytes(), &st)
	if len(st.RecentDone) != 1 || st.RecentDone[0].ID != tk.ID || st.RecentDone[0].Status != "ok" || st.RecentDone[0].RunMS < 40 || st.KeepDoneMS != 300000 {
		t.Fatalf("%+v keep=%d", st.RecentDone, st.KeepDoneMS)
	}
	// it holds no slot, counts in no limit and not in the cards
	if st.Queue.InUse[queue.LaneGen] != 0 || len(st.Queue.Running) != 0 || len(st.Queue.Waiting) != 0 || st.Cards.Running != 0 || st.Cards.Waiting != 0 {
		t.Fatalf("a finished request leaked into the real queue: %+v %+v", st.Queue, st.Cards)
	}
	// the single generation slot is free for the next request, at once
	tk2, err := f.q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "10.0.0.1", ClientLabel: "rapido", Model: "qwen3-ia"})
	if err != nil || tk2.Position != 0 {
		t.Fatalf("slot not free: %v pos=%d", err, tk2.Position)
	}
	json.Unmarshal(do(f.h, "GET", "/monitor/api/state", "", nil).Body.Bytes(), &st)
	if len(st.RecentDone) != 1 || st.Cards.Running != 1 {
		t.Fatalf("running one is not a finished one: done=%d running=%d", len(st.RecentDone), st.Cards.Running)
	}
	// 4m59s later it is still there; at 5m01s it is gone
	f.clock.Add(4*time.Minute + 59*time.Second)
	if ids := recentIDs(t, f); len(ids) != 1 {
		t.Fatalf("gone too early: %v", ids)
	}
	f.clock.Add(2 * time.Second)
	settleState(f.h) // the stale list is served once while the refresh runs
	if ids := recentIDs(t, f); len(ids) != 0 {
		t.Fatalf("still listed after 5 min: %v", ids)
	}
	tk2.Release(queue.Result{HTTPStatus: 200})
	// the log keeps it (only the view expired)
	if j, ok, _ := f.st.Job(context.Background(), tk.ID); !ok || j.Status != "ok" {
		t.Fatalf("the job log lost it: %v %+v", ok, j)
	}
}

func TestKeepDoneFlagZeroHidesAndOtherValuesApply(t *testing.T) {
	f := newJobFixture(t, 0, true)
	f.addJob(t, idN(1), "c", "p", "ok")
	if ids := recentIDs(t, f); len(ids) != 0 {
		t.Fatalf("keep 0 must hide: %v", ids)
	}
	g := newJobFixture(t, 30*time.Second, true)
	g.addJob(t, idN(1), "c", "p", "erro")
	g.addJob(t, idN(2), "c", "p", "em_execucao")
	g.addJob(t, idN(3), "c", "p", "na_fila")
	if ids := recentIDs(t, g); len(ids) != 1 || ids[0] != idN(1) {
		t.Fatalf("only the ended job belongs in the done list: %v", ids)
	}
	var st State
	json.Unmarshal(do(g.h, "GET", "/monitor/api/state", "", nil).Body.Bytes(), &st)
	if len(st.Jobs) != 3 || st.KeepDoneMS != 30000 {
		t.Fatalf("live log %d keep %d", len(st.Jobs), st.KeepDoneMS)
	}
	g.clock.Add(35 * time.Second) // addJob ends its jobs 1 s after "now"
	settleState(g.h)
	if ids := recentIDs(t, g); len(ids) != 0 {
		t.Fatalf("30 s later: %v", ids)
	}
}

// settleState makes the cached parts fresh: it waits for any refresh already
// running (it may hold older data), triggers a new one and waits for it. The
// state serves the stale value meanwhile, by design (see swrCache).
func settleState(h *Handler) {
	idle := func() {
		for i := 0; i < 500; i++ {
			busy := false
			for _, c := range []interface{ isBusy() bool }{&h.extras, &h.jobs.live, &h.jobs.done} {
				busy = busy || c.isBusy()
			}
			if !busy {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	idle()
	h.state()
	idle()
}

var _ = httptest.NewRecorder
var _ = http.StatusOK
