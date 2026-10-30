package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Fase 10.L: OnAccepted fires once per accepted request, before it waits, never
// for a rejection; RunningIDs and StartedAt serve the job log.
func TestOnAcceptedAndRunningIDs(t *testing.T) {
	var mu sync.Mutex
	var got []Record
	m := New(Config{GenSlots: 1, QueueMax: 1, PerClient: 4})
	mk := func(label string) Request {
		return Request{Lane: LaneGen, ClientIP: "10.0.0.1", ClientLabel: label, Tool: "t", Model: "m", BytesIn: 7,
			OnAccepted: func(r Record) { mu.Lock(); got = append(got, r); mu.Unlock() }}
	}
	t1, err := m.Acquire(context.Background(), mk("a")) // starts at once
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != StatusQueued || got[0].ID != t1.ID || got[0].ClientLabel != "a" || got[0].BytesIn != 7 ||
		!got[0].StartedAt.IsZero() || !got[0].FinishedAt.IsZero() || got[0].EnqueuedAt.IsZero() {
		t.Fatalf("%+v", got)
	}
	if ids := m.RunningIDs(); len(ids) != 1 || ids[0] != t1.ID {
		t.Fatalf("running %v", ids)
	}
	if t1.StartedAt().IsZero() {
		t.Fatal("StartedAt not set")
	}
	// second waits (OnAccepted already fired while it waits), third is rejected (queue of 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := m.Acquire(ctx, mk("b")); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("OnAccepted not called while the request waits")
		}
		time.Sleep(2 * time.Millisecond)
	}
	var fe *FullError
	if _, err := m.Acquire(context.Background(), mk("c")); !errors.As(err, &fe) {
		t.Fatalf("expected a rejection, got %v", err)
	}
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 2 {
		t.Fatalf("OnAccepted fired for a rejected request: %d", n)
	}
	cancel()
	<-done
	t1.Release(Result{})
	if len(m.RunningIDs()) != 0 {
		t.Fatal("running ids not empty")
	}
}
