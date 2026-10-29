package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type outcome struct {
	t   *Ticket
	err error
}

type waiter struct {
	res    chan outcome
	cancel context.CancelFunc
}

func req(client string, p Priority) Request {
	return Request{Lane: LaneGen, ClientIP: "10.0.0.1", ClientLabel: client, Tool: "generate", Endpoint: "/api/generate", Model: "m", PriorityIn: p, PriorityFinal: p}
}

// start launches Acquire in a goroutine and waits until it is either waiting
// in the line or already done.
func start(t *testing.T, m *Manager, r Request) *waiter {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	w := &waiter{res: make(chan outcome, 1), cancel: cancel}
	before := m.Snapshot()
	nb := len(before.Waiting) + len(before.Running)
	go func() {
		tk, err := m.Acquire(ctx, r)
		w.res <- outcome{tk, err}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := m.Snapshot()
		if len(s.Waiting)+len(s.Running) > nb || len(w.res) > 0 {
			return w
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("request never entered the line")
	return nil
}

func mustGet(t *testing.T, w *waiter) *Ticket {
	t.Helper()
	select {
	case o := <-w.res:
		if o.err != nil {
			t.Fatalf("Acquire: %v", o.err)
		}
		return o.t
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire did not return")
	}
	return nil
}

func mustNot(t *testing.T, w *waiter) {
	t.Helper()
	select {
	case o := <-w.res:
		t.Fatalf("returned early: %+v", o)
	case <-time.After(30 * time.Millisecond):
	}
}

func mustErr(t *testing.T, w *waiter, want error) {
	t.Helper()
	select {
	case o := <-w.res:
		if !errors.Is(o.err, want) {
			t.Fatalf("err = %v, want %v", o.err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire did not return")
	}
}

func TestSlotsLimitAndRelease(t *testing.T) {
	m := New(Config{GenSlots: 2})
	a := mustGet(t, start(t, m, req("a", Normal)))
	_ = mustGet(t, start(t, m, req("b", Normal)))
	c := start(t, m, req("c", Normal))
	mustNot(t, c)
	if got := m.Snapshot().InUse[LaneGen]; got != 2 {
		t.Fatalf("in use = %d", got)
	}
	a.Release(Result{})
	mustGet(t, c)
	if msg := m.checkInvariants(); msg != "" {
		t.Fatal(msg)
	}
}

func TestLanesAreIndependent(t *testing.T) {
	m := New(Config{GenSlots: 1, EmbedSlots: 1})
	_ = mustGet(t, start(t, m, req("a", Normal)))
	blocked := start(t, m, req("b", Normal))
	mustNot(t, blocked)
	er := req("e", Normal)
	er.Lane = LaneEmbed
	e := mustGet(t, start(t, m, er))
	if e.Position != 0 {
		t.Fatalf("embed should start at once, position %d", e.Position)
	}
}

func TestPriorityOrder(t *testing.T) {
	m := New(Config{GenSlots: 1})
	first := mustGet(t, start(t, m, req("x", Normal)))
	low := start(t, m, req("l", Low))
	norm := start(t, m, req("n", Normal))
	high := start(t, m, req("h", High))
	first.Release(Result{})
	h := mustGet(t, high)
	mustNot(t, norm)
	mustNot(t, low)
	h.Release(Result{})
	n := mustGet(t, norm)
	mustNot(t, low)
	n.Release(Result{})
	mustGet(t, low)
}

func TestFairnessRoundRobinAndFIFOInsideClient(t *testing.T) {
	m := New(Config{GenSlots: 1})
	first := mustGet(t, start(t, m, req("A", Normal)))
	a1 := start(t, m, req("A", Normal))
	a2 := start(t, m, req("A", Normal))
	a3 := start(t, m, req("A", Normal))
	b1 := start(t, m, req("B", Normal))
	first.Release(Result{})
	// A was served last, so B goes first even though it arrived after the A's.
	tb := mustGet(t, b1)
	mustNot(t, a1)
	tb.Release(Result{})
	t1 := mustGet(t, a1) // FIFO inside A
	mustNot(t, a2)
	t1.Release(Result{})
	t2 := mustGet(t, a2)
	t2.Release(Result{})
	mustGet(t, a3)
}

func TestAgingLowBecomesNormal(t *testing.T) {
	m := New(Config{GenSlots: 1, AgeAfter: 40 * time.Millisecond})
	first := mustGet(t, start(t, m, req("x", Normal)))
	low := start(t, m, req("l", Low))
	time.Sleep(60 * time.Millisecond)
	norm := start(t, m, req("n", Normal)) // arrives later, same effective level
	first.Release(Result{})
	got := mustGet(t, low) // aged: competes as normal and is older
	if got.FinalPriority() != Normal {
		t.Fatalf("final priority %v, want normal", got.FinalPriority())
	}
	mustNot(t, norm)
}

func TestLimits(t *testing.T) {
	m := New(Config{GenSlots: 1, QueueMax: 2, PerClient: 1, PerIP: 10})
	_ = mustGet(t, start(t, m, req("run", Normal)))
	_ = start(t, m, req("a", Normal))
	// per-client limit
	_, err := m.Acquire(context.Background(), req("a", Normal))
	var fe *FullError
	if !errors.As(err, &fe) || !errors.Is(err, ErrClientFull) || fe.RetryAfter <= 0 {
		t.Fatalf("per client: %v", err)
	}
	_ = start(t, m, req("b", Normal))
	// total limit
	_, err = m.Acquire(context.Background(), req("c", Normal))
	if !errors.As(err, &fe) || !errors.Is(err, ErrFull) {
		t.Fatalf("total: %v", err)
	}
	if m.Snapshot().Totals.Rejected != 2 {
		t.Fatalf("rejected = %d", m.Snapshot().Totals.Rejected)
	}
}

func TestMaxWaitExpires(t *testing.T) {
	m := New(Config{GenSlots: 1, MaxWait: 40 * time.Millisecond})
	_ = mustGet(t, start(t, m, req("run", Normal)))
	w := start(t, m, req("a", Normal))
	mustErr(t, w, ErrWaitTimeout)
	if n := len(m.Snapshot().Waiting); n != 0 {
		t.Fatalf("waiting = %d", n)
	}
	if msg := m.checkInvariants(); msg != "" {
		t.Fatal(msg)
	}
}

func TestClientGoneLeavesLineWithoutSlot(t *testing.T) {
	var mu sync.Mutex
	var recs []Record
	m := New(Config{GenSlots: 1, OnRecord: func(r Record) { mu.Lock(); recs = append(recs, r); mu.Unlock() }})
	run := mustGet(t, start(t, m, req("run", Normal)))
	w := start(t, m, req("a", Normal))
	w.cancel()
	mustErr(t, w, context.Canceled)
	if n := len(m.Snapshot().Waiting); n != 0 {
		t.Fatalf("waiting = %d", n)
	}
	run.Release(Result{})
	if got := m.Snapshot().InUse[LaneGen]; got != 0 {
		t.Fatalf("slot leaked: %d", got)
	}
	mu.Lock()
	defer mu.Unlock()
	var sawCanceled bool
	for _, r := range recs {
		if r.Status == StatusCanceled {
			sawCanceled = true
		}
	}
	if !sawCanceled {
		t.Fatalf("no cancelado record: %+v", recs)
	}
}

func TestStartStagger(t *testing.T) {
	m := New(Config{GenSlots: 4, StartStagger: 80 * time.Millisecond})
	t0 := time.Now()
	a := mustGet(t, start(t, m, req("a", Normal)))
	b := start(t, m, req("b", Normal))
	tb := mustGet(t, b)
	if d := tb.started.Sub(a.started); d < 70*time.Millisecond {
		t.Fatalf("gap between starts = %v", d)
	}
	_ = t0
	if tb.Waited < 60*time.Millisecond {
		t.Fatalf("waited %v", tb.Waited)
	}
}

func TestPromoteDemoteOrder(t *testing.T) {
	m := New(Config{GenSlots: 1})
	run := mustGet(t, start(t, m, req("run", Normal)))
	n1 := start(t, m, req("n1", Normal))
	l1 := start(t, m, req("l1", Low))
	snap := m.Snapshot()
	if len(snap.Waiting) != 2 || snap.Waiting[0].ClientLabel != "n1" {
		t.Fatalf("order: %+v", snap.Waiting)
	}
	lowID := snap.Waiting[1].ID
	// promote low -> normal: joins the end of normal, behind n1
	res, err := m.Promote(lowID)
	if err != nil || res.Before == res.After {
		t.Fatalf("promote: %v %+v", err, res)
	}
	if s := m.Snapshot(); s.Waiting[0].ClientLabel != "n1" || s.Waiting[1].Priority != "normal" {
		t.Fatalf("after promote: %+v", s.Waiting)
	}
	// promote again -> alta: goes first
	if _, err := m.Promote(lowID); err != nil {
		t.Fatal(err)
	}
	if s := m.Snapshot(); s.Waiting[0].ClientLabel != "l1" || s.Waiting[0].Priority != "alta" {
		t.Fatalf("after 2nd promote: %+v", s.Waiting)
	}
	// demote twice -> back to baixa, last
	if _, err := m.Demote(lowID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Demote(lowID); err != nil {
		t.Fatal(err)
	}
	if s := m.Snapshot(); s.Waiting[1].ClientLabel != "l1" || s.Waiting[1].Priority != "baixa" {
		t.Fatalf("after demotes: %+v", s.Waiting)
	}
	run.Release(Result{})
	mustGet(t, n1)
	mustNot(t, l1)
}

func TestPromoteAtHighGoesToTop(t *testing.T) {
	m := New(Config{GenSlots: 1})
	run := mustGet(t, start(t, m, req("run", Normal)))
	h1 := start(t, m, req("h1", High))
	h2 := start(t, m, req("h2", High))
	s := m.Snapshot()
	if s.Waiting[0].ClientLabel != "h1" {
		t.Fatalf("order %+v", s.Waiting)
	}
	if _, err := m.Promote(s.Waiting[1].ID); err != nil {
		t.Fatal(err)
	}
	run.Release(Result{})
	mustGet(t, h2)
	mustNot(t, h1)
}

func TestDemoteAtLowGoesToEnd(t *testing.T) {
	m := New(Config{GenSlots: 1})
	run := mustGet(t, start(t, m, req("run", Normal)))
	_ = start(t, m, req("l1", Low))
	l2 := start(t, m, req("l2", Low))
	s := m.Snapshot()
	if _, err := m.Demote(s.Waiting[0].ID); err != nil {
		t.Fatal(err)
	}
	if s2 := m.Snapshot(); s2.Waiting[0].ClientLabel != "l2" {
		t.Fatalf("order %+v", s2.Waiting)
	}
	run.Release(Result{})
	mustGet(t, l2)
}

func TestRemoveWaiting(t *testing.T) {
	var mu sync.Mutex
	var recs []Record
	m := New(Config{GenSlots: 1, OnRecord: func(r Record) { mu.Lock(); recs = append(recs, r); mu.Unlock() }})
	run := mustGet(t, start(t, m, req("run", Normal)))
	w := start(t, m, req("a", Normal))
	id := m.Snapshot().Waiting[0].ID
	if _, err := m.Remove(id); err != nil {
		t.Fatal(err)
	}
	mustErr(t, w, ErrRemoved)
	if _, err := m.Remove(id); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("second remove: %v", err)
	}
	if _, err := m.Promote("nope"); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("promote unknown: %v", err)
	}
	run.Release(Result{})
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, r := range recs {
		if r.ID == id && r.Status == StatusRemoved && r.HTTPStatus == 409 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no removido record: %+v", recs)
	}
	if msg := m.checkInvariants(); msg != "" {
		t.Fatal(msg)
	}
}

func TestActionOnRunningReturnsNotWaiting(t *testing.T) {
	m := New(Config{GenSlots: 1})
	run := mustGet(t, start(t, m, req("run", Normal)))
	for name, f := range map[string]func(string) (ActionResult, error){"promote": m.Promote, "demote": m.Demote, "remove": m.Remove} {
		if _, err := f(run.ID); !errors.Is(err, ErrNotWaiting) {
			t.Fatalf("%s on running: %v", name, err)
		}
	}
	if _, err := m.Cancel("nope"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("cancel unknown: %v", err)
	}
}

func TestCancelRunningFreesSlotAndCancelsContext(t *testing.T) {
	var mu sync.Mutex
	var recs []Record
	m := New(Config{GenSlots: 1, OnRecord: func(r Record) { mu.Lock(); recs = append(recs, r); mu.Unlock() }})
	run := mustGet(t, start(t, m, req("run", Normal)))
	next := start(t, m, req("next", Normal))
	mustNot(t, next)
	if _, err := m.Cancel(run.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-run.Ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context not cancelled")
	}
	if !IsAdminCancel(run.Ctx) {
		t.Fatal("cause is not admin cancel")
	}
	mustGet(t, next) // slot was freed at once
	if _, err := m.Cancel(run.ID); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("second cancel: %v", err)
	}
	run.Release(Result{HTTPStatus: 499})
	run.Release(Result{}) // idempotent
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for _, r := range recs {
		if r.ID == run.ID {
			n++
			if r.Status != StatusAdminCanceled {
				t.Fatalf("status %q", r.Status)
			}
		}
	}
	if n != 1 {
		t.Fatalf("records for cancelled ticket = %d", n)
	}
	if msg := m.checkInvariants(); msg != "" {
		t.Fatal(msg)
	}
}

func TestRecordCarriesResultAndPriorities(t *testing.T) {
	var got Record
	m := New(Config{GenSlots: 1, OnRecord: func(r Record) { got = r }})
	r := req("a", Normal)
	r.PriorityIn = High // asked alta, granted normal
	tk := mustGet(t, start(t, m, r))
	tk.Release(Result{HTTPStatus: 200, PromptEval: 10, Eval: 20, LoadMS: 5})
	if got.Status != StatusOK || got.PromptEval != 10 || got.Eval != 20 || got.LoadMS != 5 {
		t.Fatalf("record %+v", got)
	}
	if got.PriorityIn != High || got.PriorityFinal != Normal {
		t.Fatalf("priorities in=%v final=%v", got.PriorityIn, got.PriorityFinal)
	}
	if len(got.ID) != 32 {
		t.Fatalf("id %q is not 128 bits of hex", got.ID)
	}
}

func TestIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		id := NewID()
		if seen[id] {
			t.Fatal("duplicate id")
		}
		seen[id] = true
	}
}

func TestOnChangeFires(t *testing.T) {
	var mu sync.Mutex
	n := 0
	m := New(Config{OnChange: func() { mu.Lock(); n++; mu.Unlock() }})
	tk := mustGet(t, start(t, m, req("a", Normal)))
	tk.Release(Result{})
	mu.Lock()
	defer mu.Unlock()
	if n < 2 {
		t.Fatalf("OnChange calls = %d", n)
	}
}

func TestSweepKeepsActiveClients(t *testing.T) {
	m := New(Config{GenSlots: 1})
	tk := mustGet(t, start(t, m, req("a", Normal)))
	tk.Release(Result{})
	m.Sweep(time.Hour)
	if len(m.served) != 2 {
		t.Fatal("recent client dropped")
	}
	time.Sleep(5 * time.Millisecond)
	m.Sweep(time.Millisecond)
	if len(m.served) != 0 {
		t.Fatal("idle client kept")
	}
}
