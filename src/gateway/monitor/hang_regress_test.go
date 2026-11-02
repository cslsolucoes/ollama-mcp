package monitor

// Regression of the production incident of 2026-11-01 (gateway 0.12.5): a
// database read that never returns must not stop /monitor/api/state. The old
// refreshExtras ran the queries holding extraMu, so one stuck Stats made every
// panel request wait for the mutex until nginx answered 429.

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// hangSource is a readSource whose Stats blocks until release is closed,
// ignoring the context (the way the stuck database/sql read did).
type hangSource struct {
	readSource
	release chan struct{}
	calls   int
	mu      sync.Mutex
}

func (s *hangSource) Stats(ctx context.Context, window time.Duration, label string) (store.Stats, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	<-s.release
	return store.Stats{}, nil
}

func TestStateSurvivesHungDatabaseRead(t *testing.T) {
	old := extrasFirstWait
	extrasFirstWait = 100 * time.Millisecond
	defer func() { extrasFirstWait = old }()

	f := newJobFixture(t, time.Minute, false)
	hs := &hangSource{readSource: f.st, release: make(chan struct{})}
	f.h.src = hs
	defer close(hs.release)

	base := runtime.NumGoroutine()
	const workers, rounds = 20, 5
	var wg sync.WaitGroup
	slow := make(chan time.Duration, workers*rounds)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				t0 := time.Now()
				if rr := do(f.h, "GET", "/monitor/api/state", "", nil); rr.Code != http.StatusOK {
					t.Errorf("state: HTTP %d", rr.Code)
				}
				if d := time.Since(t0); d > 500*time.Millisecond {
					slow <- d
				}
			}
		}()
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatalf("state() travado: o mutex do monitor segura a leitura do banco (goroutines: %d)", runtime.NumGoroutine())
	}
	close(slow)
	for d := range slow {
		t.Errorf("state() levou %v (limite 500ms)", d)
	}
	hs.mu.Lock()
	calls := hs.calls
	hs.mu.Unlock()
	if calls != 1 {
		t.Errorf("Stats chamado %d vezes; esperado 1 refresh em curso", calls)
	}
	if extra := runtime.NumGoroutine() - base; extra > 5 {
		t.Errorf("%d goroutines a mais depois dos pedidos", extra)
	}
}
