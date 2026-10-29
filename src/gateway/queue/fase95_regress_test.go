package queue

import (
	"context"
	"sync"
	"testing"
)

// Fase 9.5, Q1 (Baixa): Acquire woke up through ctx.Done at the same moment
// the slot was granted, and the administrator cancelled the running request
// before abandon took the lock. The record must still be written, once.
func TestRegressAbandonAfterAdminCancelRecordsOnce(t *testing.T) {
	var mu sync.Mutex
	var recs []Record
	m := New(Config{GenSlots: 1, OnRecord: func(r Record) { mu.Lock(); recs = append(recs, r); mu.Unlock() }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tk, err := m.Acquire(ctx, Request{ClientIP: "10.0.0.1", ClientLabel: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Cancel(tk.ID); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = m.abandon(tk, context.Canceled, StatusCanceled)
	_ = m.abandon(tk, context.Canceled, StatusCanceled) // a second call must not duplicate
	mu.Lock()
	defer mu.Unlock()
	if len(recs) != 1 || recs[0].Status != StatusAdminCanceled {
		t.Fatalf("records %+v", recs)
	}
	if s := m.checkInvariants(); s != "" {
		t.Fatal(s)
	}
}
