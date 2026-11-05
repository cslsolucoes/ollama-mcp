package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const psJSON = `{"models":[{"name":"qwen3-ia:latest","model":"qwen3-ia:latest","size":13958643712,"size_vram":13958643712,
 "context_length":65536,"expires_at":"2026-10-30T15:00:00-03:00","details":{"parameter_size":"9.7B","quantization_level":"Q4_K_M","family":"qwen35"}},
 {"name":"split","size":1000000000,"size_vram":300000000,"context_length":4096,"expires_at":""}]}`

const tagsJSON = `{"models":[
 {"name":"qwen3-ia:latest","size":6600000000,"modified_at":"2026-10-29T14:00:00Z","details":{"parameter_size":"9.7B","quantization_level":"Q4_K_M","family":"qwen35"}},
 {"name":"nomic-embed-text:latest","size":274000000,"modified_at":"2026-10-20T10:00:00Z","details":{"parameter_size":"137M","quantization_level":"F16","family":"nomic-bert"}},
 {"name":"other-model:7b","size":4000000000,"modified_at":"2026-10-01T10:00:00Z","details":{"parameter_size":"7B","quantization_level":"Q4_0","family":"llama"}}]}`

type fakeOllama struct {
	srv       *httptest.Server
	ps, tags  atomic.Int32
	mu        sync.Mutex
	psBody    string
	tagsBody  string
	psStatus  int
	methods   []string
	slowFor   time.Duration
	inflight  atomic.Int32
	maxFlight atomic.Int32
}

func newFake(t *testing.T) *fakeOllama {
	f := &fakeOllama{psBody: psJSON, tagsBody: tagsJSON, psStatus: 200}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := f.inflight.Add(1)
		defer f.inflight.Add(-1)
		if n > f.maxFlight.Load() {
			f.maxFlight.Store(n)
		}
		f.mu.Lock()
		f.methods = append(f.methods, r.Method)
		ps, tags, st, slow := f.psBody, f.tagsBody, f.psStatus, f.slowFor
		f.mu.Unlock()
		if slow > 0 {
			time.Sleep(slow)
		}
		switch r.URL.Path {
		case "/api/ps":
			f.ps.Add(1)
			if st != 200 {
				http.Error(w, "boom", st)
				return
			}
			w.Write([]byte(ps))
		case "/api/tags":
			f.tags.Add(1)
			w.Write([]byte(tags))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOllama) collector() *Collector {
	u, _ := url.Parse(f.srv.URL)
	return &Collector{Upstream: u, Allowed: []string{"qwen3-ia", "nomic-embed-text"}, ThinkSuffix: "-think"}
}

func TestSnapshotFromFakeOllama(t *testing.T) {
	f := newFake(t)
	c := f.collector()
	s := c.Once(context.Background())
	if s.Err != "" || len(s.Loaded) != 2 || len(s.Installed) != 3 {
		t.Fatalf("%+v", s)
	}
	l := s.Loaded[0]
	if l.Name != "qwen3-ia:latest" || l.SizeMiB != 13312 || l.SizeVRAMMiB != 13312 || l.Processor != "100% GPU" ||
		l.ContextLength != 65536 || l.ExpiresAt == "" || !l.Allowed {
		t.Fatalf("%+v", l)
	}
	if sp := s.Loaded[1]; sp.Processor != "70%/30% CPU/GPU" || sp.Allowed {
		t.Fatalf("split model %+v", sp)
	}
	i := s.Installed[0]
	if i.Name != "qwen3-ia:latest" || i.ParameterSize != "9.7B" || i.Quantization != "Q4_K_M" || i.Family != "qwen35" || !i.Allowed || i.SizeMiB != 6294 {
		t.Fatalf("%+v", i)
	}
	if !s.Installed[1].Allowed || s.Installed[2].Allowed {
		t.Fatalf("allowed flags %+v", s.Installed)
	}
	if s.AllowAll || s.ThinkSuffix != "-think" || len(s.Allowed) != 2 {
		t.Fatalf("%+v", s)
	}
	for _, m := range f.methods {
		if m != http.MethodGet {
			t.Fatalf("method %s", m)
		}
	}
}

func TestProcessor(t *testing.T) {
	for _, c := range []struct {
		size, vram int64
		want       string
	}{{100, 100, "100% GPU"}, {100, 0, "100% CPU"}, {100, 25, "75%/25% CPU/GPU"}, {0, 0, "-"}, {100, 150, "100% GPU"},
		// gemma4:26b, 2026-11-04: host buffer (CUDA_Host) with every layer on the GPU
		{6829175603, 6345979084, "100% GPU"},
		// qwen3-ia in the etapa B rodada 1: real offload (3.3 of 8.4 GiB in VRAM)
		{9019431321, 3543348019, "60%/40% CPU/GPU"},
		// a small model half out of the VRAM is under 1 GiB but over a quarter
		{858993459, 429496729, "50%/50% CPU/GPU"},
		// a big model 1 GiB + 1 byte out of the VRAM
		{20 << 30, 20<<30 - 1<<30 - 1, "5%/95% CPU/GPU"}} {
		if got := Processor(c.size, c.vram); got != c.want {
			t.Errorf("Processor(%d,%d) = %q want %q", c.size, c.vram, got, c.want)
		}
	}
}

func TestIsAllowed(t *testing.T) {
	if !IsAllowed(nil, "anything") {
		t.Fatal("no list = any model")
	}
	al := []string{"qwen3-ia", "x:1"}
	if !IsAllowed(al, "qwen3-ia:latest") || !IsAllowed(al, "x:1") || IsAllowed(al, "y") || IsAllowed(al, "qwen3-ia-think") {
		t.Fatal("rule")
	}
}

func TestErrorsLeaveListsEmptyWithShortMessage(t *testing.T) {
	f := newFake(t)
	f.psStatus = 500
	c := f.collector()
	s := c.Once(context.Background())
	if len(s.Loaded) != 0 || len(s.Installed) != 3 || !strings.HasPrefix(s.Err, "ps: HTTP 500") {
		t.Fatalf("%+v", s)
	}
	// Ollama down: both fail, no panic, lists empty and non-nil
	f.srv.Close()
	s = c.Once(context.Background())
	if s.Loaded == nil || s.Installed == nil || len(s.Loaded)+len(s.Installed) != 0 || !strings.Contains(s.Err, "ps:") || !strings.Contains(s.Err, "tags:") || len(s.Err) > 400 {
		t.Fatalf("%+v", s)
	}
	// garbage JSON
	g := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }))
	defer g.Close()
	u, _ := url.Parse(g.URL)
	s = (&Collector{Upstream: u}).Once(context.Background())
	if s.Err == "" || len(s.Loaded) != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestOversizedAnswerIsRejected(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[],"pad":"` + strings.Repeat("a", maxBody+10) + `"}`))
	}))
	defer big.Close()
	u, _ := url.Parse(big.URL)
	s := (&Collector{Upstream: u}).Once(context.Background())
	if !strings.Contains(s.Err, "grande demais") {
		t.Fatalf("%q", s.Err)
	}
}

// Reading the cache is free: many Latest calls make no query; only the loop
// queries, once per interval.
func TestLatestNeverQueriesAndLoopRespectsInterval(t *testing.T) {
	f := newFake(t)
	c := f.collector()
	c.Interval = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.RunLoop(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for f.ps.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	before := f.ps.Load()
	for i := 0; i < 500; i++ {
		_ = c.Latest()
	}
	if after := f.ps.Load(); after != before {
		t.Fatalf("Latest queried the Ollama: %d -> %d", before, after)
	}
	if len(c.Latest().Installed) != 3 {
		t.Fatal("cache empty")
	}
	time.Sleep(650 * time.Millisecond)
	cancel()
	<-done
	// first round at once + ticks at 200 ms in ~650 ms: well under 10, never one per call
	if n := f.ps.Load(); n < 2 || n > 6 {
		t.Fatalf("%d ps queries in ~0.65 s at a 200 ms interval", n)
	}
}

// A slow Ollama never gets two queries in flight.
func TestSlowOllamaNoOverlap(t *testing.T) {
	f := newFake(t)
	f.slowFor = 120 * time.Millisecond
	c := f.collector()
	c.Interval = 30 * time.Millisecond
	c.Timeout = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { c.RunLoop(ctx); close(done) }()
	<-done
	if f.maxFlight.Load() != 1 {
		t.Fatalf("%d queries in flight", f.maxFlight.Load())
	}
}

func TestOnChangeSkipsFirstReadingAndReportsLoadAndUnload(t *testing.T) {
	f := newFake(t)
	c := f.collector()
	var mu sync.Mutex
	var added, removed []string
	calls := 0
	c.OnChange = func(a, r []string) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		added, removed = a, r
	}
	c.Once(context.Background()) // first reading: baseline, no event
	if calls != 0 {
		t.Fatalf("first reading fired %d", calls)
	}
	c.Once(context.Background()) // same set
	if calls != 0 {
		t.Fatal("no change must not fire")
	}
	f.mu.Lock()
	f.psBody = `{"models":[{"name":"nomic-embed-text:latest","size":1,"size_vram":1}]}`
	f.mu.Unlock()
	c.Once(context.Background())
	if calls != 1 || len(added) != 1 || added[0] != "nomic-embed-text:latest" || len(removed) != 2 {
		t.Fatalf("calls %d added %v removed %v", calls, added, removed)
	}
	// a failing /api/ps must not look like "everything unloaded"
	f.mu.Lock()
	f.psStatus = 500
	f.mu.Unlock()
	c.Once(context.Background())
	if calls != 1 {
		t.Fatalf("an error fired an event: %d", calls)
	}
}

func TestLatestBeforeFirstReadingIsEmptyNotNull(t *testing.T) {
	s := (&Collector{}).Latest()
	if s.Loaded == nil || s.Installed == nil || s.Allowed == nil {
		t.Fatalf("%+v", s)
	}
}
