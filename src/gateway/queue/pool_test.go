package queue

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/gateway/pool"
)

// Fase 10.4 non-regression. Without --backends the line has ONE backend,
// "gpu-local", with the slots of --gen-slots, and behaves as before the pool
// existed. The whole pre-existing suite of this package (queue_test, ip_test,
// accepted_test, fuzz_test, fase95_regress_test) runs unchanged against the
// pooled structure: New() always builds the pool. The tests below add the
// positive proof.

func TestDefaultIsOneGpuLocal(t *testing.T) {
	m := New(Config{GenSlots: 2, EmbedSlots: 3})
	bs := m.Pool().Backends()
	if len(bs) != 1 || bs[0].ID != pool.DefaultPrimaryID || !bs[0].Primary || bs[0].Slots != 2 || m.Pool().HasSecondaries() {
		t.Fatalf("backends %+v", bs)
	}
	if m.Slots(LaneGen) != 2 || m.Slots(LaneEmbed) != 3 {
		t.Fatalf("slots %d %d", m.Slots(LaneGen), m.Slots(LaneEmbed))
	}
	tk := mustGet(t, start(t, m, req("a", Normal)))
	if tk.Backend() != pool.DefaultPrimaryID || tk.FailoverRule() != "" {
		t.Fatalf("backend %q rule %q", tk.Backend(), tk.FailoverRule())
	}
	if it := m.Snapshot().Running[0]; it.Backend != pool.DefaultPrimaryID || it.FailoverRule != "" {
		t.Fatalf("item %+v", it)
	}
	e := mustGet(t, start(t, m, Request{Lane: LaneEmbed, ClientIP: "10.0.0.2", ClientLabel: "e", Model: "m"}))
	if e.Backend() != pool.DefaultPrimaryID {
		t.Fatalf("embed on %q", e.Backend())
	}
	tk.Release(Result{})
	e.Release(Result{})
	if s := m.checkInvariants(); s != "" {
		t.Fatal(s)
	}
	// no secondary, so no health gate: a run of failures never stops the line
	for i := 0; i < 6; i++ {
		x := mustGet(t, start(t, m, req("f", Normal)))
		x.Release(Result{Status: StatusError, ErrorClass: "upstream_unreachable", HTTPStatus: 502})
	}
	ok := mustGet(t, start(t, m, req("z", Normal)))
	ok.Release(Result{})
	if st := m.PoolStatus(); len(st) != 1 || st[0].ID != pool.DefaultPrimaryID || st[0].Slots != 2 {
		t.Fatalf("status %+v", st)
	}
}

// The same script on the implicit single backend and on an explicit pool of one
// produces the same line, step by step: who runs, who waits and in which order.
func TestExplicitLonePoolEqualsTheImplicitOne(t *testing.T) {
	cfg := Config{GenSlots: 2, EmbedSlots: 1, QueueMax: 12, PerClient: 4, PerIP: 5, LabelsPerIP: 3, AgeAfter: time.Hour}
	explicit := cfg
	explicit.Pool = pool.New(pool.Config{PrimarySlots: 2, QueueMax: 12})
	a, b := New(cfg), New(explicit)
	rnd := rand.New(rand.NewSource(42))
	var ta, tb []*Ticket
	var mu sync.Mutex
	var wg sync.WaitGroup
	launch := func(m *Manager, r Request, into *[]*Ticket) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tk, err := m.Acquire(context.Background(), r); err == nil {
				mu.Lock()
				*into = append(*into, tk)
				mu.Unlock()
			}
		}()
	}
	view := func(m *Manager) (run, wait []string) {
		s := m.Snapshot()
		for _, it := range s.Running {
			run = append(run, it.ID)
		}
		for _, it := range s.Waiting {
			wait = append(wait, fmt.Sprintf("%s@%d/%s", it.ID, it.Position, it.Priority))
		}
		sortStrings(run)
		return
	}
	settle := func() { time.Sleep(15 * time.Millisecond) }
	for step := 0; step < 80; step++ {
		switch op := rnd.Intn(10); {
		case op < 6: // enqueue the same request on both
			id := fmt.Sprintf("r%03d", step)
			r := Request{
				ID: id, Lane: LaneGen, ClientIP: fmt.Sprintf("10.0.0.%d", 1+rnd.Intn(3)), ClientLabel: string(rune('a' + rnd.Intn(4))),
				Model: "m", PriorityIn: Priority(1 + rnd.Intn(3)),
			}
			r.PriorityFinal = r.PriorityIn
			if rnd.Intn(8) == 0 {
				r.Lane = LaneEmbed
			}
			launch(a, r, &ta)
			launch(b, r, &tb)
		default: // release the same running request on both
			run, _ := view(a)
			if len(run) == 0 {
				continue
			}
			id := run[rnd.Intn(len(run))]
			settle()
			mu.Lock()
			for _, tk := range ta {
				if tk.ID == id {
					tk.Release(Result{})
				}
			}
			for _, tk := range tb {
				if tk.ID == id {
					tk.Release(Result{})
				}
			}
			mu.Unlock()
		}
		settle()
		ra, wa := view(a)
		rb, wb := view(b)
		if !reflect.DeepEqual(ra, rb) || !reflect.DeepEqual(wa, wb) {
			t.Fatalf("step %d: the implicit and the explicit single backend diverged\nimplicit: run %v wait %v\nexplicit: run %v wait %v", step, ra, wa, rb, wb)
		}
		if s := a.checkInvariants(); s != "" {
			t.Fatal("implicit: " + s)
		}
		if s := b.checkInvariants(); s != "" {
			t.Fatal("explicit: " + s)
		}
	}
	mu.Lock()
	for _, tk := range append(append([]*Ticket(nil), ta...), tb...) {
		tk.Release(Result{})
	}
	mu.Unlock()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---- routing fuzz ---------------------------------------------------------------

// routingRig is a line with a primary and two secondaries (gpu and cpu) whose
// health, digests and drain state the fuzzer drives directly.
type routingRig struct {
	m  *Manager
	pl *pool.Pool
}

func newRoutingRig(t testing.TB) *routingRig {
	bs, err := pool.Build([]pool.Spec{
		{ID: "sec-gpu", URL: "http://127.0.0.1:1", Tipo: "gpu", Slots: 1, Ordem: 1, MaxPromptTokens: 2000},
		{ID: "sec-cpu", URL: "http://127.0.0.1:2", Tipo: "cpu", Slots: 1, Ordem: 2, Classes: []string{"alta", "normal", "baixa"}},
	}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	pl := pool.New(pool.Config{PrimarySlots: 1, Secondaries: bs, QueueMax: 8, OverloadQueue: 1, Hold: 5 * time.Millisecond, BreakerFails: 2, BreakerOpen: 10 * time.Millisecond})
	r := &routingRig{pl: pl, m: New(Config{Pool: pl, QueueMax: 8, PerClient: 8, PerIP: 8, MaxWait: 400 * time.Millisecond, AgeAfter: 30 * time.Millisecond})}
	r.observe(true, true, true)
	return r
}

// observe sets the active-check result of the three backends: healthy or not,
// and whether the secondaries hold the same digest as the primary.
func (r *routingRig) observe(primaryUp, secSame, gpuSecUp bool) {
	// model "n" never has the same digest on the primary and on a secondary
	snapPrimary := func() models.Snapshot {
		return models.Snapshot{Installed: []models.Installed{{Name: "m", Digest: "d1"}, {Name: "n", Digest: "dn"}}}
	}
	snapSec := func(digest string) models.Snapshot {
		return models.Snapshot{Installed: []models.Installed{{Name: "m", Digest: digest}, {Name: "n", Digest: "dn-other"}}}
	}
	prim := error(nil)
	if !primaryUp {
		prim = fmt.Errorf("down")
	}
	r.pl.Observe(pool.DefaultPrimaryID, "v", snapPrimary(), prim)
	d := "d1"
	if !secSame {
		d = "d2"
	}
	g := error(nil)
	if !gpuSecUp {
		g = fmt.Errorf("down")
	}
	r.pl.Observe("sec-gpu", "v", snapSec(d), g)
	r.pl.Observe("sec-cpu", "v", snapSec(d), nil)
}

// violations checks, under the lock of the line, every rule that must hold at
// every instant for the requests that are running.
func (r *routingRig) violations(drainedAtStart map[string]bool) string {
	if s := r.m.checkInvariants(); s != "" {
		return s
	}
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	perBackend := map[string]int{}
	for _, t := range r.m.running[LaneGen] {
		perBackend[t.backend]++
		b := r.pl.Backend(t.backend)
		if b == nil {
			return "running on an unknown backend " + t.backend
		}
		if b.Primary {
			if t.rule != "" {
				return "a primary start carries a failover rule"
			}
			continue
		}
		if t.rule == "" {
			return "a secondary start without a rule"
		}
		if t.req.Model != "m" {
			return fmt.Sprintf("model %q (digest unknown on the primary) ran on a secondary", t.req.Model)
		}
		if b.Kind() == pool.KindCPU && t.finalPrio == High {
			return "alta ran on a cpu backend"
		}
		if b.MaxPromptTokens > 0 && pool.EstimateTokens(t.req.BytesIn) > int64(b.MaxPromptTokens) {
			return "a prompt over max_prompt_tokens ran on a secondary"
		}
	}
	for id, n := range perBackend {
		if n > r.pl.Backend(id).Slots {
			return fmt.Sprintf("backend %s over its slots", id)
		}
	}
	for _, t := range r.m.running[LaneEmbed] {
		if t.backend != pool.DefaultPrimaryID {
			return "an embedding ran off the primary"
		}
	}
	return ""
}

func runRouting(t *testing.T, data []byte) {
	r := newRoutingRig(t)
	var (
		mu      sync.Mutex
		tickets []*Ticket
		cancels []context.CancelFunc
		wg      sync.WaitGroup
	)
	enqueue := func(arg, arg2 byte) {
		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		cancels = append(cancels, cancel)
		mu.Unlock()
		prio := Priority(1 + arg%3)
		req := Request{
			Lane: LaneGen, ClientIP: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}[(arg/3)%3], ClientLabel: string(rune('a' + arg%5)),
			Model: []string{"m", "m", "n"}[arg2%3], PriorityIn: prio, PriorityFinal: prio,
			BytesIn: int64(arg2%4) * 4000, // 0, 4000, 8000, 12000 bytes: 0..4000 tokens
		}
		if arg%11 == 0 {
			req.Lane = LaneEmbed
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, err := r.m.Acquire(ctx, req)
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
			tickets = append(tickets[:i], tickets[i+1:]...)
		}
		mu.Unlock()
		if tk == nil {
			return
		}
		switch arg % 4 {
		case 0:
			tk.Release(Result{Status: StatusError, ErrorClass: "upstream_unreachable", HTTPStatus: 502})
		case 1:
			tk.Release(Result{Status: StatusError, ErrorClass: "upstream_500", HTTPStatus: 500})
		default:
			tk.Release(Result{})
		}
	}
	primaryUp, secSame, gpuUp := true, true, true
	for i := 0; i+1 < len(data); i += 2 {
		op, arg := data[i]&0x0f, data[i+1]
		switch op {
		case 0, 1, 2, 3:
			enqueue(arg, data[i])
		case 4, 5:
			releaseOne(arg)
		case 6:
			primaryUp = !primaryUp
			r.observe(primaryUp, secSame, gpuUp)
		case 7:
			secSame = !secSame
			r.observe(primaryUp, secSame, gpuUp)
		case 8:
			gpuUp = !gpuUp
			r.observe(primaryUp, secSame, gpuUp)
		case 9:
			r.pl.SetDrained([]string{pool.DefaultPrimaryID, "sec-gpu", "sec-cpu"}[arg%3], arg&0x80 != 0)
		case 10:
			time.Sleep(time.Duration(arg%8) * time.Millisecond) // lets the breaker and the hold run
		case 11:
			r.m.Poke()
		}
		if s := r.violations(nil); s != "" {
			t.Fatalf("after op %d: %s", i/2, s)
		}
	}
	// drain: everything healthy and undrained, release what runs; the line must empty
	for _, id := range []string{pool.DefaultPrimaryID, "sec-gpu", "sec-cpu"} {
		r.pl.SetDrained(id, false)
	}
	r.observe(true, true, true)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		cur := append([]*Ticket(nil), tickets...)
		tickets = nil
		mu.Unlock()
		for _, tk := range cur {
			tk.Release(Result{})
		}
		r.m.Poke()
		s := r.m.Snapshot()
		if len(s.Running) == 0 && len(s.Waiting) == 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	for _, c := range cancels {
		c()
	}
	wg.Wait()
	mu.Lock()
	for _, tk := range tickets {
		tk.Release(Result{})
	}
	mu.Unlock()
	if s := r.violations(nil); s != "" {
		t.Fatal(s)
	}
}

func TestRoutingScriptsSeeds(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	for i := 0; i < 40; i++ {
		data := make([]byte, 120)
		rnd.Read(data)
		runRouting(t, data)
	}
}

// FuzzRouting drives the routing decision with random request mixes, health,
// digest and drain changes and breaker failures, and checks the invariants of
// F1..F8 after every step. Run it for the 5-minute gate with
// go test -run=^$ -fuzz=FuzzRouting -fuzztime=5m ./gateway/queue
func FuzzRouting(f *testing.F) {
	f.Add([]byte{0, 1, 0, 2, 6, 0, 0, 3, 4, 1, 0, 4, 7, 0, 0, 5, 5, 2})
	f.Add([]byte{9, 0x81, 0, 7, 0, 8, 0, 9, 9, 0x80, 4, 0, 1, 0xb, 0})
	f.Add([]byte{0, 3, 0, 3, 0, 3, 0, 3, 6, 0, 8, 0, 10, 7, 11, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 400 {
			data = data[:400]
		}
		runRouting(t, data)
	})
}
