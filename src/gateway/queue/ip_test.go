package queue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// reqIP builds a request from an explicit IP (the label is client-controlled).
func reqIP(ip, label string, p Priority) Request {
	r := req(label, p)
	r.ClientIP = ip
	return r
}

// Regression (alert MEDIUM, Fase 9.1): varying X-Mcpollama-Client must not get
// past the per-IP ceiling.
func TestManyLabelsSameIPHitPerIPCeiling(t *testing.T) {
	m := New(Config{GenSlots: 1, QueueMax: 200, PerClient: 2, PerIP: 5, LabelsPerIP: 100})
	_ = mustGet(t, start(t, m, reqIP("10.0.0.9", "run", Normal)))
	for i := 0; i < 5; i++ {
		start(t, m, reqIP("10.0.0.1", fmt.Sprintf("label-%d", i), Normal))
	}
	rejected := 0
	for i := 5; i < 50; i++ {
		_, err := m.Acquire(context.Background(), reqIP("10.0.0.1", fmt.Sprintf("label-%d", i), Normal))
		if errors.Is(err, ErrClientFull) {
			rejected++
		}
	}
	if rejected != 45 {
		t.Fatalf("rejected = %d, want 45", rejected)
	}
	if n := len(m.Snapshot().Waiting); n != 5 {
		t.Fatalf("waiting = %d, want the per-IP ceiling 5", n)
	}
	if msg := m.checkInvariants(); msg != "" {
		t.Fatal(msg)
	}
}

func TestPerIPCeilingDoesNotAffectOtherIP(t *testing.T) {
	m := New(Config{GenSlots: 1, QueueMax: 200, PerClient: 10, PerIP: 3})
	_ = mustGet(t, start(t, m, reqIP("10.0.0.9", "run", Normal)))
	for i := 0; i < 3; i++ {
		start(t, m, reqIP("10.0.0.1", fmt.Sprintf("l%d", i), Normal))
	}
	if _, err := m.Acquire(context.Background(), reqIP("10.0.0.1", "another", Normal)); !errors.Is(err, ErrClientFull) {
		t.Fatalf("4th label of the same IP: %v", err)
	}
	start(t, m, reqIP("10.0.0.2", "x", Normal))
}

func TestLabelsPerIPFoldedIntoOutros(t *testing.T) {
	m := New(Config{GenSlots: 1, QueueMax: 200, PerClient: 100, PerIP: 100, LabelsPerIP: 4})
	_ = mustGet(t, start(t, m, reqIP("10.0.0.9", "run", Normal)))
	for i := 0; i < 50; i++ {
		start(t, m, reqIP("10.0.0.1", fmt.Sprintf("lab%d", i), Normal))
	}
	labels := map[string]int{}
	for _, it := range m.Snapshot().Waiting {
		labels[it.ClientLabel]++
	}
	if len(labels) != 5 || labels["10.0.0.1/outros"] != 46 {
		t.Fatalf("labels = %v", labels)
	}
	if msg := m.checkInvariants(); msg != "" {
		t.Fatal(msg)
	}
}

// One IP with 20 labels gets one slice per round against another IP.
func TestFairnessAnchoredOnIPNotLabel(t *testing.T) {
	m := New(Config{GenSlots: 1, QueueMax: 200, PerClient: 100, PerIP: 100, LabelsPerIP: 100})
	cur := mustGet(t, start(t, m, reqIP("10.0.0.9", "run", Normal)))
	for i := 0; i < 20; i++ {
		start(t, m, reqIP("10.0.0.1", fmt.Sprintf("lab%d", i), Normal))
	}
	for i := 0; i < 20; i++ {
		start(t, m, reqIP("10.0.0.2", "single", Normal))
	}
	counts := map[string]int{}
	for i := 0; i < 20; i++ {
		cur.Release(Result{})
		var next *Ticket
		deadline := time.Now().Add(2 * time.Second)
		for next == nil && time.Now().Before(deadline) {
			if r := m.Snapshot().Running; len(r) == 1 {
				m.mu.Lock()
				next = m.byID[r[0].ID]
				m.mu.Unlock()
			} else {
				time.Sleep(time.Millisecond)
			}
		}
		if next == nil {
			t.Fatal("nothing started")
		}
		counts[next.Req().ClientIP]++
		cur = next
	}
	if d := counts["10.0.0.1"] - counts["10.0.0.2"]; d < -1 || d > 1 {
		t.Fatalf("unfair share between IPs: %v", counts)
	}
}
