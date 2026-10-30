package pool_test

import (
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
)

func newOverloadPool(t *testing.T, slots, queueMax int, ev *events) *pool.Pool {
	t.Helper()
	bs, _ := pool.Build([]pool.Spec{{ID: "sec", URL: "http://127.0.0.1:1", Tipo: "gpu"}}, nil, "-think", time.Second, nil)
	pl := pool.New(pool.Config{PrimarySlots: slots, Secondaries: bs, QueueMax: queueMax, OverloadWait: time.Minute, OnEvent: ev.add})
	now := time.Now()
	for i := 0; i < 20; i++ {
		pl.NoteEnd(pool.DefaultPrimaryID, pool.OutcomeOK, false, true, 40_000, now) // EWMA 40 s
	}
	return pl
}

// 10.7-1: the request being dispatched must not count itself. An idle primary
// with a single request (EWMA above half the threshold) is not overloaded.
func TestS1DoesNotCountTheArrivingRequest(t *testing.T) {
	ev := &events{}
	pl := newOverloadPool(t, 1, 8, ev)
	now := time.Now()
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 1, PrimaryUsed: 0}); o.On || o.S1 || o.EstWait != 0 {
		t.Fatalf("idle primary + 1 arriving request: %+v", o)
	}
	if ev.has("overload_on") {
		t.Fatalf("spurious event: %v", ev.evs)
	}
	// busy primary, nobody ahead: waits for the one running (40 s), under 60 s
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 1, PrimaryUsed: 1}); o.S1 || o.EstWait != 40*time.Second {
		t.Fatalf("1 running, 0 ahead: %+v", o)
	}
	// with requests ahead it turns on as before: (2 ahead + 1 running) * 40 s = 120 s
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 3, PrimaryUsed: 1}); !o.On || !o.S1 || o.EstWait != 120*time.Second {
		t.Fatalf("2 ahead: %+v", o)
	}
	if !ev.has("overload_on") {
		t.Fatal("overload_on expected")
	}
}

// S2 has the same flaw: requests that fit in the free slots are not a queue.
func TestS2IgnoresRequestsThatFitInFreeSlots(t *testing.T) {
	ev := &events{}
	bs, _ := pool.Build([]pool.Spec{{ID: "sec", URL: "http://127.0.0.1:1", Tipo: "gpu"}}, nil, "-think", time.Second, nil)
	pl := pool.New(pool.Config{PrimarySlots: 2, Secondaries: bs, QueueMax: 4, OverloadQueue: 3, OnEvent: ev.add})
	now := time.Now()
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 4, PrimaryUsed: 0}); o.On || o.S2 {
		t.Fatalf("4 waiting, 2 free slots = queue of 2: %+v", o)
	}
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 4, PrimaryUsed: 2}); !o.S2 {
		t.Fatalf("4 waiting, no free slot: S2 expected: %+v", o)
	}
	if got := strings.Join(ev.kinds(), ","); got != "overload_on" {
		t.Fatalf("events %s", got)
	}
}
