package pool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// fake is a fake Ollama: /api/version, /api/ps, /api/tags and /api/generate.
type fake struct {
	srv *httptest.Server
	URL *url.URL

	mu        sync.Mutex
	model     string
	digest    string
	processor string // "100% GPU" unless a test changes it
	version   int    // HTTP status of /api/version
	genStatus int    // HTTP status of /api/generate
	hold      chan struct{}
	auths     []string // Authorization header of every generate
	gens      int
	maxHeld   int
}

func newFake(t *testing.T, model, digest string) *fake {
	t.Helper()
	f := &fake{model: model, digest: digest, processor: "100% GPU", version: 200, genStatus: 200}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	f.URL, _ = url.Parse(f.srv.URL)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	model, digest, proc, ver, gst, hold := f.model, f.digest, f.processor, f.version, f.genStatus, f.hold
	f.mu.Unlock()
	switch r.URL.Path {
	case "/api/version":
		if ver != 200 {
			http.Error(w, "down", ver)
			return
		}
		fmt.Fprint(w, `{"version":"0.40.8"}`)
	case "/api/ps":
		size, vram := int64(8<<30), int64(8<<30)
		switch proc {
		case "100% GPU":
		case "host-buffer": // gemma4:26b, 2026-11-04: every layer on the GPU, a CUDA_Host buffer counted in size
			size, vram = 6829175603, 6345979084
		default:
			vram = 2 << 30
		}
		fmt.Fprintf(w, `{"models":[{"name":%q,"model":%q,"size":%d,"size_vram":%d,"digest":%q,"context_length":65536}]}`, model, model, size, vram, digest)
	case "/api/tags":
		fmt.Fprintf(w, `{"models":[{"name":%q,"model":%q,"size":%d,"digest":%q,"details":{"family":"qwen","parameter_size":"9B","quantization_level":"Q4_K_M"}}]}`, model, model, int64(8<<30), digest)
	case "/api/generate", "/api/chat", "/api/embed":
		f.mu.Lock()
		f.gens++
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		f.mu.Unlock()
		_, _ = readAll(r)
		if hold != nil {
			select {
			case <-hold:
			case <-r.Context().Done():
				return
			}
		}
		if gst != 200 {
			http.Error(w, `{"error":"boom"}`, gst)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprint(w, `{"response":"ok","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
	default:
		http.NotFound(w, r)
	}
}

func readAll(r *http.Request) ([]byte, error) {
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return []byte(b.String()), nil
		}
	}
}

func (f *fake) set(fn func(f *fake)) { f.mu.Lock(); fn(f); f.mu.Unlock() }
func (f *fake) gensSeen() int        { f.mu.Lock(); defer f.mu.Unlock(); return f.gens }
func (f *fake) authsSeen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.auths...)
}

// ---- rig --------------------------------------------------------------------------

type secCfg struct {
	ID        string
	Tipo      string
	Slots     int
	Order     int
	Classes   []string
	Models    []string
	MaxTokens int
	Digest    string // digest the secondary reports; "" = same as the primary
	Token     string
}

type rigCfg struct {
	PrimarySlots  int
	QueueMax      int
	OverloadQueue int
	OverloadWait  time.Duration
	Hold          time.Duration
	BreakerOpen   time.Duration
	BreakerFails  int
	Secs          []secCfg
}

type events struct {
	mu  sync.Mutex
	evs []string
}

func (e *events) add(kind, detail string) {
	e.mu.Lock()
	e.evs = append(e.evs, kind+" "+detail)
	e.mu.Unlock()
}
func (e *events) kinds() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []string{}
	for _, s := range e.evs {
		out = append(out, strings.SplitN(s, " ", 2)[0])
	}
	return out
}
func (e *events) has(kind string) bool {
	for _, k := range e.kinds() {
		if k == kind {
			return true
		}
	}
	return false
}

type rig struct {
	t     *testing.T
	prim  *fake
	secs  map[string]*fake
	pl    *pool.Pool
	q     *queue.Manager
	evs   *events
	mu    sync.Mutex
	recs  []queue.Record
	nowMu sync.Mutex
	now   time.Time
}

const testModel = "qwen3-ia"

func newRig(t *testing.T, c rigCfg) *rig {
	t.Helper()
	if c.PrimarySlots == 0 {
		c.PrimarySlots = 1
	}
	if c.QueueMax == 0 {
		c.QueueMax = 8
	}
	r := &rig{t: t, secs: map[string]*fake{}, evs: &events{}, now: time.Now()}
	r.prim = newFake(t, testModel, "sha256-aaaa")
	var specs []pool.Spec
	dir := t.TempDir()
	for _, s := range c.Secs {
		d := s.Digest
		if d == "" {
			d = "sha256-aaaa"
		}
		f := newFake(t, testModel, d)
		r.secs[s.ID] = f
		sp := pool.Spec{ID: s.ID, URL: f.srv.URL, Tipo: s.Tipo, Slots: s.Slots, Ordem: s.Order, Classes: s.Classes, Modelos: s.Models, MaxPromptTokens: s.MaxTokens}
		if s.Token != "" {
			p := filepath.Join(dir, s.ID+".token")
			if err := os.WriteFile(p, []byte(s.Token+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			sp.TokenFile = p
		}
		specs = append(specs, sp)
	}
	secs, err := pool.Build(specs, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.pl = pool.New(pool.Config{
		PrimarySlots: c.PrimarySlots, PrimaryURL: r.prim.URL, Secondaries: secs, QueueMax: c.QueueMax,
		OverloadQueue: c.OverloadQueue, OverloadWait: c.OverloadWait, Hold: c.Hold, BreakerOpen: c.BreakerOpen, BreakerFails: c.BreakerFails,
		OnEvent: r.evs.add,
	})
	r.q = queue.New(queue.Config{
		Pool: r.pl, QueueMax: c.QueueMax,
		OnRecord: func(rec queue.Record) { r.mu.Lock(); r.recs = append(r.recs, rec); r.mu.Unlock() },
	})
	r.check()
	return r
}

// check runs one active check on every backend (synchronously).
func (r *rig) check() {
	for _, b := range r.pl.Backends() {
		r.pl.CheckOnce(context.Background(), b)
	}
}

func (r *rig) records() []queue.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]queue.Record(nil), r.recs...)
}

// job is a request in flight in the line.
type job struct {
	tk  *queue.Ticket
	err error
	got chan struct{}
}

var ipSeq int
var ipMu sync.Mutex

func nextIP() string {
	ipMu.Lock()
	defer ipMu.Unlock()
	ipSeq++
	return fmt.Sprintf("10.9.%d.%d", ipSeq/250, ipSeq%250+1)
}

func (r *rig) submit(req queue.Request) *job {
	r.t.Helper()
	if req.Model == "" {
		req.Model = testModel
	}
	if req.ClientIP == "" {
		req.ClientIP = nextIP()
	}
	if req.ClientLabel == "" {
		req.ClientLabel = "t"
	}
	if req.PriorityFinal == 0 {
		req.PriorityFinal = queue.Normal
	}
	req.PriorityIn = req.PriorityFinal
	j := &job{got: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	r.t.Cleanup(cancel)
	go func() {
		j.tk, j.err = r.q.Acquire(ctx, req)
		close(j.got)
	}()
	return j
}

// waiting waits until n requests are in the line (not started).
func (r *rig) waitWaiting(n int) {
	r.t.Helper()
	waitFor(r.t, fmt.Sprintf("%d waiting", n), func() bool { return len(r.q.Snapshot().Waiting) == n })
}

func (j *job) started(t *testing.T) *queue.Ticket {
	t.Helper()
	select {
	case <-j.got:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	if j.err != nil {
		t.Fatalf("acquire: %v", j.err)
	}
	return j.tk
}

func (j *job) isStarted() bool {
	select {
	case <-j.got:
		return true
	default:
		return false
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	dl := time.Now().Add(3 * time.Second)
	for time.Now().Before(dl) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func mustJSON(t *testing.T, v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
