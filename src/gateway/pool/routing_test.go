package pool_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// Three fake Ollamas stand for the primary (GPU), a secondary GPU and a "cpu"
// secondary. Every test below is one rule of the plan (F1..F8, S1..S3).

var twoSecs = []secCfg{
	{ID: "sec-gpu", Tipo: "gpu", Slots: 1, Order: 1},
	{ID: "sec-cpu", Tipo: "cpu", Slots: 1, Order: 2},
}

// F1: no overload, the primary takes the work; a secondary stays idle.
func TestF1NoOverloadStaysOnThePrimary(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 4, Secs: twoSecs})
	a := r.submit(queue.Request{}).started(t)
	if a.Backend() != pool.DefaultPrimaryID || a.FailoverRule() != "" {
		t.Fatalf("backend %q rule %q", a.Backend(), a.FailoverRule())
	}
	b := r.submit(queue.Request{}) // waits: the primary is full, but nothing is overloaded
	r.waitWaiting(1)
	time.Sleep(50 * time.Millisecond)
	if b.isStarted() {
		t.Fatal("not overloaded: the request must wait for the primary")
	}
	a.Release(queue.Result{})
	if got := b.started(t); got.Backend() != pool.DefaultPrimaryID {
		t.Fatalf("ran on %q", got.Backend())
	}
}

// S2 (queue above --overload-queue) hands the head of the line to the first
// eligible secondary, and the record names the backend and the rule.
func TestS2QueueOverloadGoesToTheSecondary(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Secs: twoSecs[:1]})
	a := r.submit(queue.Request{}).started(t) // takes the only primary slot
	b := r.submit(queue.Request{})
	r.waitWaiting(1)
	if b.isStarted() {
		t.Fatal("1 waiting is not above 1")
	}
	c := r.submit(queue.Request{}) // 2 waiting > 1: overload
	tb := b.started(t)
	if tb.Backend() != "sec-gpu" || tb.FailoverRule() != "F2:S2" {
		t.Fatalf("head of the line ran on %q rule %q", tb.Backend(), tb.FailoverRule())
	}
	if c.isStarted() {
		t.Fatal("the secondary has one slot: the third request waits")
	}
	tb.Release(queue.Result{})
	// the freed slot of sec-gpu takes the next one, still overloaded (hold)
	if tc := c.started(t); tc.Backend() != "sec-gpu" {
		t.Fatalf("ran on %q", tc.Backend())
	}
	a.Release(queue.Result{})
	waitFor(t, "records", func() bool { return len(r.records()) >= 2 })
	for _, rec := range r.records() {
		if rec.ID == a.ID && (rec.Backend != pool.DefaultPrimaryID || rec.FailoverRule != "") {
			t.Fatalf("record %+v", rec)
		}
		if rec.ID == tb.ID && (rec.Backend != "sec-gpu" || rec.FailoverRule != "F2:S2") {
			t.Fatalf("record %+v", rec)
		}
	}
}

// S1: the estimated wait (EWMA of the duration times the places ahead) above
// --overload-wait.
func TestS1EstimatedWaitGoesToTheSecondary(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, OverloadWait: 10 * time.Millisecond, Secs: twoSecs[:1]})
	// teach the EWMA: one primary generation that takes ~60 ms
	first := r.submit(queue.Request{}).started(t)
	time.Sleep(60 * time.Millisecond)
	first.Release(queue.Result{})
	a := r.submit(queue.Request{}).started(t)
	b := r.submit(queue.Request{}) // behind a full primary: estimate 2 x 60 ms > 10 ms
	tb := b.started(t)
	if tb.Backend() != "sec-gpu" || tb.FailoverRule() != "F2:S1" {
		t.Fatalf("ran on %q rule %q", tb.Backend(), tb.FailoverRule())
	}
	a.Release(queue.Result{})
}

// S3: the primary is not healthy (active check fails): work goes to the
// secondary even though a primary slot is free.
func TestS3UnhealthyPrimaryGoesToTheSecondary(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, Secs: twoSecs[:1]})
	r.prim.set(func(f *fake) { f.version = 500 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	a := r.submit(queue.Request{}).started(t)
	if a.Backend() != "sec-gpu" || a.FailoverRule() != "F2:S3" {
		t.Fatalf("ran on %q rule %q", a.Backend(), a.FailoverRule())
	}
	if !r.evs.has("backend_down") || !r.evs.has("overload_on") {
		t.Fatalf("events %v", r.evs.kinds())
	}
	// the primary comes back: after the hold the work returns to it
	r.prim.set(func(f *fake) { f.version = 200 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	if !r.evs.has("backend_up") {
		t.Fatalf("events %v", r.evs.kinds())
	}
	a.Release(queue.Result{})
}

// The primary leaving 100% GPU (model pushed to the CPU for lack of VRAM) is S3
// too.
func TestS3PrimaryOffTheGPU(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, Secs: twoSecs[:1]})
	r.prim.set(func(f *fake) { f.processor = "70%/30% CPU/GPU" })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	st := r.q.PoolStatus()
	if st[0].State != "degradado" {
		t.Fatalf("state %q", st[0].State)
	}
	a := r.submit(queue.Request{}).started(t)
	if a.Backend() != "sec-gpu" {
		t.Fatalf("ran on %q", a.Backend())
	}
	a.Release(queue.Result{})
}

// A model all on the GPU whose /api/ps size counts a pinned host buffer
// (gemma4:26b, 2026-11-04: "degradado (fora da GPU)" was a false alarm) keeps
// the primary ok, and the work stays on it.
func TestHostBufferIsNotOffTheGPU(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, Secs: twoSecs[:1]})
	r.prim.set(func(f *fake) { f.processor = "host-buffer" })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	st := r.q.PoolStatus()
	if st[0].State != "ok" || st[0].LastError != "" {
		t.Fatalf("state %q last error %q", st[0].State, st[0].LastError)
	}
	a := r.submit(queue.Request{}).started(t)
	if a.Backend() != r.pl.Primary().ID {
		t.Fatalf("ran on %q", a.Backend())
	}
	a.Release(queue.Result{})
}

// The breaker: three passive failures on the primary open it; requests wait
// for the secondary; after the open time one probe goes to the primary.
func TestBreakerOpenPrimaryRoutesToSecondaryThenProbes(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, BreakerOpen: 80 * time.Millisecond, Secs: twoSecs[:1]})
	for i := 0; i < 3; i++ {
		tk := r.submit(queue.Request{}).started(t)
		if i == 0 && tk.Backend() != pool.DefaultPrimaryID {
			t.Fatalf("first on %q", tk.Backend())
		}
		if tk.Backend() == pool.DefaultPrimaryID {
			tk.Release(queue.Result{Status: queue.StatusError, ErrorClass: "upstream_unreachable", HTTPStatus: 502})
		} else {
			tk.Release(queue.Result{})
			i-- // not a primary failure: count again
		}
		if i >= 0 && tk.Backend() != pool.DefaultPrimaryID && r.pl.Primary() == nil {
			t.Fatal("unreachable")
		}
	}
	waitFor(t, "breaker_open", func() bool { return r.evs.has("breaker_open") })
	if st := r.q.PoolStatus(); st[0].Breaker != "aberto" || st[0].State != "disjuntor_aberto" {
		t.Fatalf("status %+v", st[0])
	}
	// open: the next request cannot start on the primary
	b := r.submit(queue.Request{}).started(t)
	if b.Backend() != "sec-gpu" {
		t.Fatalf("ran on %q while the breaker was open", b.Backend())
	}
	b.Release(queue.Result{})
	// after the open time the probe goes to the primary and closes the breaker
	time.Sleep(120 * time.Millisecond)
	r.check() // healthy active checks do not close it; only the probe does
	r.q.Poke()
	p := r.submit(queue.Request{}).started(t)
	if p.Backend() != pool.DefaultPrimaryID {
		t.Fatalf("the probe ran on %q", p.Backend())
	}
	p.Release(queue.Result{})
	waitFor(t, "breaker_close", func() bool { return r.evs.has("breaker_close") })
}

// F4: a secondary with another digest of the same model is refused; the
// request waits for the primary (F5), no error.
func TestF4DifferentDigestIsRefused(t *testing.T) {
	cfg := []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 1, Order: 1, Digest: "sha256-OTHER"}}
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 0, Secs: cfg})
	// S3 forces the overload; the only secondary has another digest
	r.prim.set(func(f *fake) { f.version = 500 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	a := r.submit(queue.Request{}).started(t)
	if a.Backend() != pool.DefaultPrimaryID {
		t.Fatalf("a secondary with another digest took the request (%q)", a.Backend())
	}
	if n := r.secs["sec-gpu"].gensSeen(); n != 0 {
		t.Fatalf("the secondary saw %d generations", n)
	}
	a.Release(queue.Result{})
}

// F3: alta never goes to a cpu backend; normal does.
func TestF3AltaNeverGoesToCPU(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Secs: []secCfg{{ID: "sec-cpu", Tipo: "cpu", Slots: 2, Classes: []string{"alta", "normal", "baixa"}}}})
	hold := r.submit(queue.Request{}).started(t) // primary busy
	alta1 := r.submit(queue.Request{PriorityFinal: queue.High})
	alta2 := r.submit(queue.Request{PriorityFinal: queue.High})
	r.waitWaiting(2) // 2 waiting > 1: overload; the cpu backend has free slots but refuses alta
	time.Sleep(60 * time.Millisecond)
	if alta1.isStarted() || alta2.isStarted() {
		t.Fatal("an alta request ran on the cpu backend")
	}
	n := r.submit(queue.Request{PriorityFinal: queue.Normal})
	tn := n.started(t)
	if tn.Backend() != "sec-cpu" {
		t.Fatalf("a normal request should take the cpu backend, ran on %q", tn.Backend())
	}
	if alta1.isStarted() || alta2.isStarted() {
		t.Fatal("alta jumped to cpu")
	}
	hold.Release(queue.Result{})
	waitFor(t, "an alta request on the primary", func() bool { return alta1.isStarted() || alta2.isStarted() })
	first := alta1
	if !alta1.isStarted() {
		first = alta2
	}
	if a := first.started(t); a.Backend() != pool.DefaultPrimaryID {
		t.Fatalf("alta ran on %q", a.Backend())
	}
	tn.Release(queue.Result{})
}

// A prompt above max_prompt_tokens stays on the primary.
func TestMaxPromptTokens(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 2, MaxTokens: 1000}}})
	hold := r.submit(queue.Request{}).started(t)
	big := r.submit(queue.Request{BytesIn: 300_000}) // ~100k tokens
	small := r.submit(queue.Request{BytesIn: 3_000}) // ~1k tokens, fits
	r.waitWaiting(1)                                 // big waits; small went to sec-gpu (2 waiting > 1 at its arrival)
	ts := small.started(t)
	if ts.Backend() != "sec-gpu" {
		t.Fatalf("small ran on %q", ts.Backend())
	}
	time.Sleep(40 * time.Millisecond)
	if big.isStarted() {
		t.Fatal("a prompt above max_prompt_tokens ran on the secondary")
	}
	hold.Release(queue.Result{})
	if tb := big.started(t); tb.Backend() != pool.DefaultPrimaryID {
		t.Fatalf("big ran on %q", tb.Backend())
	}
}

// Embeddings run on the primary only, whatever the overload.
func TestEmbedStaysOnThePrimary(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Secs: twoSecs[:1]})
	r.prim.set(func(f *fake) { f.version = 500 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	e := r.submit(queue.Request{Lane: queue.LaneEmbed, Model: "nomic-embed-text"}).started(t)
	if e.Backend() != pool.DefaultPrimaryID {
		t.Fatalf("embedding ran on %q", e.Backend())
	}
	e.Release(queue.Result{})
}

// Drain: a drained secondary takes nothing; what runs there finishes; resumed,
// it takes work again.
func TestDrain(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Secs: twoSecs[:1]})
	hold := r.submit(queue.Request{}).started(t)
	if ch, ok := r.pl.SetDrained("sec-gpu", true); !ch || !ok {
		t.Fatal("drain")
	}
	b := r.submit(queue.Request{})
	c := r.submit(queue.Request{})
	r.waitWaiting(2)
	time.Sleep(60 * time.Millisecond)
	if b.isStarted() || c.isStarted() {
		t.Fatal("a drained backend took work")
	}
	if st := r.q.PoolStatus(); st[1].State != "drenado" {
		t.Fatalf("state %q", st[1].State)
	}
	if _, ok := r.pl.SetDrained("nope", true); ok {
		t.Fatal("unknown backend")
	}
	r.pl.SetDrained("sec-gpu", false)
	r.q.Poke()
	waitFor(t, "a request on the resumed backend", func() bool { return b.isStarted() || c.isStarted() })
	first := b
	if !b.isStarted() {
		first = c
	}
	if tb := first.started(t); tb.Backend() != "sec-gpu" {
		t.Fatalf("resumed backend ran nothing: %q", tb.Backend())
	}
	hold.Release(queue.Result{})
	if !r.evs.has("backend_drenado") || !r.evs.has("backend_retomado") {
		t.Fatalf("events %v", r.evs.kinds())
	}
}

// Draining the primary sends the work to the secondary (maintenance), and what
// already runs on the primary finishes.
func TestDrainThePrimary(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 2, OverloadQueue: 6, Secs: twoSecs[:1]})
	run := r.submit(queue.Request{}).started(t)
	r.pl.SetDrained(pool.DefaultPrimaryID, true)
	next := r.submit(queue.Request{}).started(t)
	if next.Backend() != "sec-gpu" {
		t.Fatalf("ran on %q", next.Backend())
	}
	run.Release(queue.Result{}) // the running one finishes where it was
	next.Release(queue.Result{})
}

// F8: the per-IP limit and the queue size belong to the pool, not to a backend.
func TestF8LimitsHoldForTheWholePool(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, QueueMax: 3, Secs: twoSecs[:1]})
	r.q = queue.New(queue.Config{Pool: r.pl, QueueMax: 3, PerIP: 2, PerClient: 2})
	hold := r.submit(queue.Request{ClientIP: "10.0.0.1"}).started(t)
	r.submit(queue.Request{ClientIP: "10.0.0.2"})
	r.submit(queue.Request{ClientIP: "10.0.0.2"})
	r.waitWaiting(2)
	_, err := r.q.Acquire(context.Background(), queue.Request{ClientIP: "10.0.0.2", Model: testModel, ClientLabel: "t", Lane: queue.LaneGen})
	if err == nil {
		t.Fatal("a third waiting request of one IP must be refused, with a secondary in the pool")
	}
	hold.Release(queue.Result{})
}

// ---- through the proxy: the token of the client is never forwarded -------------------

func TestClientTokenIsNeverForwardedToABackend(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 2, Token: "SEC-OWN-TOKEN"}}})
	px, err := proxy.New(proxy.Config{Upstream: r.prim.URL, Queue: r.q, Pool: r.pl})
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(px)
	defer gw.Close()
	post := func() int {
		req, _ := http.NewRequest("POST", gw.URL+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"oi"}`))
		req.Header.Set("Authorization", "Bearer CLIENT-SECRET-TOKEN")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	// 1) on the primary
	if post() != 200 {
		t.Fatal("primary")
	}
	// 2) S3 forces the secondary
	r.prim.set(func(f *fake) { f.version = 500 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	if post() != 200 {
		t.Fatal("secondary")
	}
	if n := r.secs["sec-gpu"].gensSeen(); n != 1 {
		t.Fatalf("secondary saw %d generations", n)
	}
	for _, a := range r.secs["sec-gpu"].authsSeen() {
		if a != "Bearer SEC-OWN-TOKEN" {
			t.Fatalf("the secondary received Authorization %q", a)
		}
	}
	for _, a := range r.prim.authsSeen() {
		if strings.Contains(a, "CLIENT-SECRET-TOKEN") {
			t.Fatalf("the primary received the client token: %q", a)
		}
	}
	// the record carries the backend
	waitFor(t, "records", func() bool { return len(r.records()) == 2 })
	if rec := r.records()[1]; rec.Backend != "sec-gpu" || rec.FailoverRule != "F2:S3" {
		t.Fatalf("record %+v", rec)
	}
}

// A request given to a secondary that is down at the connection level is an
// error for the client (no migration during the run) and counts toward the
// breaker.
func TestFailureOnASecondaryIsReportedNotMigrated(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 2}}})
	px, _ := proxy.New(proxy.Config{Upstream: r.prim.URL, Queue: r.q, Pool: r.pl})
	gw := httptest.NewServer(px)
	defer gw.Close()
	r.prim.set(func(f *fake) { f.version = 500 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
	r.secs["sec-gpu"].srv.CloseClientConnections()
	r.secs["sec-gpu"].set(func(f *fake) { f.genStatus = 500 })
	resp, err := http.Post(gw.URL+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"oi"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d: the error must reach the client", resp.StatusCode)
	}
	if n := r.prim.gensSeen(); n != 0 {
		t.Fatalf("the request was migrated to the primary (%d)", n)
	}
}

var _ = net.IPv4len

// Revisao de seguranca (MEDIUM): a client must not be able to open the breaker.
// 5xx answers and client give-ups are neutral; a refused connection and a failing
// active check open it.
func TestBreakerCannotBeForcedByAClient(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, Secs: twoSecs[:1]})
	for i := 0; i < 50; i++ { // one client, 50 requests that make the Ollama answer 500
		tk := r.submit(queue.Request{ClientIP: "10.7.7.7"}).started(t)
		tk.Release(queue.Result{Status: queue.StatusError, ErrorClass: "upstream_500", HTTPStatus: 500})
	}
	for i := 0; i < 50; i++ { // the same client dropping its connection mid-stream
		tk := r.submit(queue.Request{ClientIP: "10.7.7.7"}).started(t)
		tk.Release(queue.Result{Status: queue.StatusCanceled, ErrorClass: "client_gone", HTTPStatus: 499})
		tk2 := r.submit(queue.Request{ClientIP: "10.7.7.7"}).started(t)
		tk2.Release(queue.Result{Status: queue.StatusError, ErrorClass: "upstream_dropped", HTTPStatus: 502})
	}
	if st := r.q.PoolStatus(); st[0].Breaker != "fechado" || r.evs.has("breaker_open") {
		t.Fatalf("a client forced the breaker: %+v", st[0])
	}
	// a backend that refuses the connection opens it
	for i := 0; i < 3; i++ {
		tk := r.submit(queue.Request{}).started(t)
		tk.Release(queue.Result{Status: queue.StatusError, ErrorClass: "upstream_unreachable", HTTPStatus: 502})
	}
	if st := r.q.PoolStatus(); st[0].Breaker != "aberto" {
		t.Fatalf("unreachable x3 must open: %+v", st[0])
	}
}

func TestFailingActiveCheckOpensTheBreaker(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 6, Secs: twoSecs[:1]})
	r.prim.set(func(f *fake) { f.version = 500 })
	for i := 0; i < 3; i++ {
		r.pl.CheckOnce(context.Background(), r.pl.Primary())
	}
	if st := r.q.PoolStatus(); st[0].Breaker != "aberto" || !r.evs.has("breaker_open") {
		t.Fatalf("3 failed active checks must open: %+v", st[0])
	}
}
