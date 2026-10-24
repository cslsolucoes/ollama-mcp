package tools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/ollama"
)

func TestQueueNilNeverBlocks(t *testing.T) {
	var q *Queue
	release, waited, err := q.Acquire(context.Background())
	if err != nil || waited != 0 {
		t.Fatal(err, waited)
	}
	release()
}

func TestQueueSlotsAndMinimum(t *testing.T) {
	q := NewQueue(0) // below 1 means 1
	r1, _, err := q.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := q.Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second acquire must wait: %v", err)
	}
	r1()
	r2, _, err := q.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r2()
	if _, _, err := NewQueue(2).Acquire(canceled()); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context must not take a slot: %v", err)
	}
}

func canceled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// PERF-002: two simultaneous generations never reach Ollama together; the
// second one waits and reports the time spent in line.
func TestGPUQueueSerialisesCalls(t *testing.T) {
	var inFlight, peak atomic.Int32
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
		inFlight.Add(-1)
		generateOK(w, r)
	})
	d := m.deps(t, nil)

	var wg sync.WaitGroup
	results := make([]mcp.ToolCallResult, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = call(t, d, "generate", `{"prompt":"x"}`)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if peak.Load() != 1 {
		t.Fatalf("peak concurrent requests at Ollama = %d, want 1", peak.Load())
	}
	var queued []int64
	for _, r := range results {
		queued = append(queued, r.Meta["mcpollama/usage"].(map[string]any)["queue_ms"].(int64))
	}
	max := queued[0]
	if queued[1] > max {
		max = queued[1]
	}
	if max < 150 {
		t.Fatalf("the waiting call must report its queue time, got queue_ms=%v", queued)
	}
	var withWait int
	for _, r := range results {
		if !strings.Contains(lastLine(text(t, r)), "queue_ms=") {
			t.Fatal("usage line without queue_ms")
		}
		if r.Meta["mcpollama/usage"].(map[string]any)["queue_ms"].(int64) >= 150 {
			withWait++
		}
	}
	if withWait != 1 {
		t.Fatalf("exactly one call waits, got %d", withWait)
	}
}

// PERF-011: a call cancelled while it waits leaves the line and does not leak
// the slot.
func TestCancelWhileWaitingFreesTheLine(t *testing.T) {
	release := make(chan struct{})
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
		generateOK(w, r)
	})
	d := m.deps(t, nil)
	first := make(chan error, 1)
	go func() { _, err := call(t, d, "generate", `{"prompt":"primeira"}`); first <- err }()
	waitFor(t, func() bool { return m.hits.Load() == 1 })

	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() {
		_, err := entry(t, d, "generate").Handler(ctx, []byte(`{"prompt":"segunda"}`))
		second <- err
	}()
	time.Sleep(100 * time.Millisecond) // the second call is parked in the queue
	cancel()
	select {
	case err := <-second:
		if err == nil || !strings.Contains(err.Error(), "cancelada enquanto esperava a fila") {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled call kept waiting")
	}
	if m.hits.Load() != 1 {
		t.Fatalf("the cancelled call must never reach Ollama (hits=%d)", m.hits.Load())
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	// The slot is free again.
	if _, err := call(t, d, "generate", `{"prompt":"terceira"}`); err != nil {
		t.Fatal(err)
	}
}

// Cancelling a running generation aborts the HTTP call and frees the queue.
func TestCancelDuringGenerationFreesTheQueue(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" && r.ContentLength > 0 {
			streamLines(w, `{"response":"parcial","done":false}`)
			once.Do(func() { close(started) })
			<-r.Context().Done() // hangs until the client drops the connection
			return
		}
		generateOK(w, r)
	})
	d := m.deps(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		res, err := entry(t, d, "generate").Handler(ctx, []byte(`{"prompt":"x"}`))
		if err == nil {
			err = errors.New("partial result: " + res.Content[0].Text)
		}
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !strings.Contains(err.Error(), "cancelada") && !errors.Is(err, ollama.ErrCanceled) && !strings.Contains(err.Error(), "partial result") {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not abort the generation")
	}
	release, _, err := d.Queue.Acquire(canceledAfter(t, 2*time.Second))
	if err != nil {
		t.Fatalf("queue slot leaked after a cancel: %v", err)
	}
	release()
}

func canceledAfter(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
