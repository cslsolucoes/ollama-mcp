package pool_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/gateway/pool"
)

// 10.7 cosmetic (a): the last error is cleared when the backend is healthy again.
func TestLastErrorIsClearedWhenTheBackendRecovers(t *testing.T) {
	bs, _ := pool.Build([]pool.Spec{{ID: "sec", URL: "http://127.0.0.1:1", Tipo: "cpu"}}, nil, "-think", time.Second, nil)
	pl := pool.New(pool.Config{PrimarySlots: 1, Secondaries: bs})
	pl.Observe("sec", "", models.Snapshot{}, errors.New("connection refused"))
	if st := pl.Status(time.Now(), nil); st[1].LastError == "" {
		t.Fatalf("setup: error expected: %+v", st[1])
	}
	pl.Observe("sec", "0.40.8", models.Snapshot{}, nil)
	if st := pl.Status(time.Now(), nil); st[1].LastError != "" || st[1].State != "ok" {
		t.Fatalf("stale error after recovery: %+v", st[1])
	}
}

// 10.7 cosmetic (c): a dead backend gives at most 1 breaker event per minute
// per kind, and the next one carries the count of the suppressed ones.
func TestBreakerEventsAreRateLimited(t *testing.T) {
	ev := &events{}
	bs, _ := pool.Build([]pool.Spec{{ID: "sec", URL: "http://127.0.0.1:1", Tipo: "gpu"}}, nil, "-think", time.Second, nil)
	pl := pool.New(pool.Config{PrimarySlots: 1, Secondaries: bs, BreakerFails: 1, BreakerOpen: 20 * time.Second, OnEvent: ev.add})
	t0 := time.Now()
	for i := 0; i < 9; i++ { // fail, then half-open after 20 s, fail again...
		now := t0.Add(time.Duration(i) * 21 * time.Second)
		if !pl.CanStart("sec", now) && i > 0 {
			t.Fatalf("step %d: half-open expected", i)
		}
		pl.NoteStart("sec", now)
		pl.NoteEnd("sec", pool.OutcomeFail, i > 0, true, 1, now)
	}
	// 9 failures over 168 s: events at 0 s, 63 s and 126 s only
	n := 0
	for _, k := range ev.kinds() {
		if k == "breaker_open" {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("breaker_open events = %d, want 3: %v", n, ev.evs)
	}
	if !strings.Contains(ev.evs[1], "suprimidos") || !strings.Contains(ev.evs[1], "+2") {
		t.Fatalf("second event must count the suppressed ones: %q", ev.evs[1])
	}
}
