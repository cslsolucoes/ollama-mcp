package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

// runOps interprets data as a script of queue actions and runs it from four
// goroutines at once. Op byte layout: low 4 bits = action, high 4 bits =
// argument. Invariants are checked while it runs and the system must drain.
func runOps(t *testing.T, data []byte) {
	m := New(Config{GenSlots: 2, EmbedSlots: 1, QueueMax: 6, PerClient: 3, MaxWait: 300 * time.Millisecond, AgeAfter: 20 * time.Millisecond})

	var (
		mu      sync.Mutex
		ids     []string // ids ever seen in snapshots
		tickets []*Ticket
		cancels []context.CancelFunc
		wg      sync.WaitGroup
	)
	pickID := func(arg byte) string {
		s := m.Snapshot()
		all := append(append([]Item{}, s.Waiting...), s.Running...)
		if len(all) == 0 {
			return "none"
		}
		return all[int(arg)%len(all)].ID
	}
	enqueue := func(arg byte) {
		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		cancels = append(cancels, cancel)
		mu.Unlock()
		r := Request{
			Lane:        LaneGen,
			ClientIP:    "10.0.0.1",
			ClientLabel: string(rune('a' + arg%3)),
			PriorityIn:  Priority(1 + arg%3), PriorityFinal: Priority(1 + arg%3),
		}
		if arg%5 == 0 {
			r.Lane = LaneEmbed
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, err := m.Acquire(ctx, r)
			if err != nil {
				return
			}
			mu.Lock()
			tickets = append(tickets, tk)
			mu.Unlock()
		}()
	}
	releaseOne := func(arg byte) {
		mu.Lock()
		var tk *Ticket
		if len(tickets) > 0 {
			i := int(arg) % len(tickets)
			tk = tickets[i]
			tickets = append(tickets[:i:i], tickets[i+1:]...)
		}
		mu.Unlock()
		if tk != nil {
			tk.Release(Result{HTTPStatus: 200})
		}
	}
	apply := func(op byte) {
		arg := op >> 4
		switch op & 0x0f {
		case 0, 1, 2, 3:
			enqueue(arg)
		case 4:
			m.Promote(pickID(arg))
		case 5:
			m.Demote(pickID(arg))
		case 6:
			m.Remove(pickID(arg))
		case 7:
			m.Cancel(pickID(arg))
		case 8, 9:
			releaseOne(arg)
		case 10:
			mu.Lock()
			if len(cancels) > 0 {
				cancels[int(arg)%len(cancels)]()
			}
			mu.Unlock()
		case 11:
			m.Snapshot()
		case 12:
			time.Sleep(time.Duration(arg) * time.Millisecond)
		}
		if msg := m.checkInvariants(); msg != "" {
			t.Errorf("invariant: %s", msg)
		}
	}

	const workers = 4
	var ow sync.WaitGroup
	for w := 0; w < workers; w++ {
		ow.Add(1)
		go func(w int) {
			defer ow.Done()
			for i := w; i < len(data); i += workers {
				apply(data[i])
			}
		}(w)
	}
	ow.Wait()

	// Drain: release every held ticket, cancel every context, wait for all.
	_ = ids
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		held := tickets
		tickets = nil
		for _, c := range cancels {
			c()
		}
		mu.Unlock()
		for _, tk := range held {
			tk.Release(Result{})
		}
		s := m.Snapshot()
		if len(s.Waiting) == 0 && len(s.Running) == 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("goroutines did not finish: deadlock or leak")
	}
	mu.Lock()
	for _, tk := range tickets {
		tk.Release(Result{})
	}
	mu.Unlock()
	if s := m.Snapshot(); len(s.Waiting) != 0 || len(s.Running) != 0 {
		t.Fatalf("not drained: %d waiting, %d running", len(s.Waiting), len(s.Running))
	}
	if msg := m.checkInvariants(); msg != "" {
		t.Fatalf("final invariant: %s", msg)
	}
}

func FuzzConcurrentActions(f *testing.F) {
	f.Add([]byte{0x00, 0x10, 0x20, 0x04, 0x05, 0x06, 0x07, 0x08, 0x00, 0x00, 0x10, 0x4a})
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0x00, 0x01, 0x02, 0x03, 0x00, 0x01, 0x02, 0x03, 0x08, 0x08, 0x08, 0x08})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x70, 0x60, 0x50, 0x40, 0x7c, 0x0b, 0x88})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 200 {
			data = data[:200]
		}
		runOps(t, data)
	})
}

// TestRandomActions is the deterministic companion of the fuzz target: a
// seeded pseudo-random script, many rounds, run under -race.
func TestRandomActions(t *testing.T) {
	var x uint32 = 2463534242
	next := func() byte {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		return byte(x)
	}
	for round := 0; round < 40; round++ {
		data := make([]byte, 120)
		for i := range data {
			data[i] = next()
		}
		runOps(t, data)
	}
}
