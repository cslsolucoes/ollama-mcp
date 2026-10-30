package pool

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cslsoftwares/mcpollama/gateway/models"
)

const ringSize = 64

// Defaults of the thresholds (flags of the entrypoint override them).
const (
	DefaultCheckEvery   = 5 * time.Second
	DefaultBreakerFails = 3
	DefaultBreakerOpen  = 30 * time.Second
	DefaultOverloadWait = 60 * time.Second
	DefaultHold         = 30 * time.Second
	ewmaAlpha           = 0.2
	// bytesPerToken turns the size of the body into a prompt-size estimate; 3 is
	// on the pessimistic side for Portuguese text (about 3.5 bytes a token).
	bytesPerToken = 3
)

type breakerState int

const (
	brClosed breakerState = iota
	brOpen
	brHalf
)

func (s breakerState) String() string {
	switch s {
	case brOpen:
		return "aberto"
	case brHalf:
		return "meio_aberto"
	}
	return "fechado"
}

// Outcome of a request, as the circuit breaker sees it.
type Outcome int

const (
	// OutcomeNeutral says nothing about the backend (the client left, the
	// administrator cancelled, the client sent a bad request).
	OutcomeNeutral Outcome = iota
	OutcomeOK
	// OutcomeFail is a connection error or a 5xx without any answer.
	OutcomeFail
)

// EstimateTokens is the prompt-size estimate used for max_prompt_tokens.
func EstimateTokens(bodyBytes int64) int64 {
	if bodyBytes <= 0 {
		return 0
	}
	return (bodyBytes + bytesPerToken - 1) / bytesPerToken
}

// Config tunes a Pool. Zero values take the defaults.
type Config struct {
	PrimarySlots     int
	PrimaryURL       *url.URL                  // nil in tests with no HTTP
	PrimaryCollector *models.Collector         // may be nil: the pool then makes its own
	Secondaries      []*Backend                // from Build
	QueueMax         int                       // 0: 64
	OverloadWait     time.Duration             // S1 threshold, default 60 s
	OverloadQueue    int                       // S2 threshold; 0 means 75% of QueueMax
	Hold             time.Duration             // calm time before the primary takes work back, default 30 s
	CheckEvery       time.Duration             // active check period, default 5 s
	BreakerFails     int                       // consecutive failures that open the breaker, default 3
	BreakerOpen      time.Duration             // time open before the probe, default 30 s
	EnergyHook       func() bool               // S4: true holds the primary and sends to a secondary; nil = off
	OnEvent          func(kind, detail string) // must not block nor call the pool or the queue
	OnChange         func()                    // state changed outside the queue (health, breaker, drain)
	Now              func() time.Time
	Logf             func(format string, args ...any)
}

// Pool is the set of backends. Safe for concurrent use.
type Pool struct {
	cfg     Config
	primary *Backend
	secs    []*Backend // by preference
	byID    map[string]*Backend

	mu       sync.Mutex
	over     bool
	overWhy  string
	calm     time.Time // since when the load is below half of the thresholds
	pending  []event
	lastEval Overload
}

type event struct{ kind, detail string }

// New builds the pool: the primary (always) and the secondaries.
func New(cfg Config) *Pool {
	if cfg.PrimarySlots < 1 {
		cfg.PrimarySlots = 2
	}
	if cfg.QueueMax < 1 {
		cfg.QueueMax = 64
	}
	if cfg.OverloadWait <= 0 {
		cfg.OverloadWait = DefaultOverloadWait
	}
	if cfg.OverloadQueue <= 0 {
		cfg.OverloadQueue = cfg.QueueMax * 3 / 4
	}
	if cfg.Hold <= 0 {
		cfg.Hold = DefaultHold
	}
	if cfg.CheckEvery <= 0 {
		cfg.CheckEvery = DefaultCheckEvery
	}
	if cfg.BreakerFails < 1 {
		cfg.BreakerFails = DefaultBreakerFails
	}
	if cfg.BreakerOpen <= 0 {
		cfg.BreakerOpen = DefaultBreakerOpen
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	p := &Pool{cfg: cfg, byID: map[string]*Backend{}}
	p.primary = &Backend{
		Spec:    Spec{ID: DefaultPrimaryID, Tipo: string(KindGPU), Slots: cfg.PrimarySlots},
		URL:     cfg.PrimaryURL,
		Primary: true,
		col:     cfg.PrimaryCollector,
	}
	if p.primary.col == nil && cfg.PrimaryURL != nil {
		p.primary.col = &models.Collector{Upstream: cfg.PrimaryURL, Logf: cfg.Logf}
	}
	p.byID[p.primary.ID] = p.primary
	p.secs = append([]*Backend(nil), cfg.Secondaries...)
	sortByOrder(p.secs)
	for _, b := range p.secs {
		p.byID[b.ID] = b
	}
	return p
}

// Primary returns the primary backend.
func (p *Pool) Primary() *Backend { return p.primary }

// Secondaries returns the secondaries by preference. Do not modify the slice.
func (p *Pool) Secondaries() []*Backend { return p.secs }

// HasSecondaries tells whether the pool is more than the primary.
func (p *Pool) HasSecondaries() bool { return len(p.secs) > 0 }

// Backends returns the primary followed by the secondaries.
func (p *Pool) Backends() []*Backend { return append([]*Backend{p.primary}, p.secs...) }

// Backend finds a backend by id (nil when unknown).
func (p *Pool) Backend(id string) *Backend { return p.byID[id] }

// Kind of the backend.
func (b *Backend) Kind() Kind { return Kind(b.Tipo) }

// ---- locking with deferred events ---------------------------------------------

func (p *Pool) lock() { p.mu.Lock() }

// unlock releases the lock and delivers the events queued while it was held.
func (p *Pool) unlock() {
	evs := p.pending
	p.pending = nil
	p.mu.Unlock()
	if f := p.cfg.OnEvent; f != nil {
		for _, e := range evs {
			f(e.kind, e.detail)
		}
	}
}

func (p *Pool) emit(kind, detail string) { p.pending = append(p.pending, event{kind, detail}) }

func (p *Pool) changed() {
	if f := p.cfg.OnChange; f != nil {
		f()
	}
}

// ---- circuit breaker ------------------------------------------------------------

// advance moves an open breaker to half-open when its time is up.
func (p *Pool) advance(b *Backend, now time.Time) {
	if b.br == brOpen && now.Sub(b.openedAt) >= p.cfg.BreakerOpen {
		b.br = brHalf
		b.probing = false
	}
}

func (p *Pool) failLocked(b *Backend, now time.Time, active bool) {
	b.fails++
	b.failActive = active
	switch {
	case b.br == brHalf:
		b.br, b.openedAt = brOpen, now
		p.emit("breaker_open", b.ID+": falhou no meio-aberto")
	case b.br == brClosed && b.fails >= p.cfg.BreakerFails:
		b.br, b.openedAt = brOpen, now
		p.emit("breaker_open", b.ID+": "+itoa(b.fails)+" falhas seguidas")
	}
}

// CanStart tells whether backend id may take a new start right now: it is not
// drained, and its breaker is closed or half-open without a probe in flight.
// With no secondary in the pool only the drain counts: the breaker is tracked
// and shown, but the gateway keeps dispatching to its only Ollama as it always
// did.
func (p *Pool) CanStart(id string, now time.Time) bool {
	p.lock()
	defer p.unlock()
	b := p.byID[id]
	if b == nil || b.drained {
		return false
	}
	if len(p.secs) == 0 {
		return true
	}
	if b.Primary && p.cfg.EnergyHook != nil && p.cfg.EnergyHook() {
		return false // S4: hold new starts on the primary (protects the power supply)
	}
	p.advance(b, now)
	switch b.br {
	case brOpen:
		return false
	case brHalf:
		return !b.probing
	}
	return true
}

// NoteStart is called under the queue lock when a request starts on id; probe
// is true when the request is the half-open probe (hand it back to NoteEnd).
func (p *Pool) NoteStart(id string, now time.Time) (probe bool) {
	p.lock()
	defer p.unlock()
	b := p.byID[id]
	if b == nil || len(p.secs) == 0 {
		return false
	}
	p.advance(b, now)
	if b.br == brHalf && !b.probing {
		b.probing = true
		return true
	}
	return false
}

// NoteEnd records how a request that ran on id ended (passive health). gen says
// whether it was a generation, runMS its duration (used for p50 and, on the
// primary, the EWMA of the wait estimate).
func (p *Pool) NoteEnd(id string, o Outcome, probe, gen bool, runMS int64, now time.Time) {
	p.lock()
	defer p.unlock()
	b := p.byID[id]
	if b == nil {
		return
	}
	if probe {
		b.probing = false
	}
	switch o {
	case OutcomeOK:
		switch {
		case b.br == brClosed:
			b.fails = 0
		case b.br == brHalf && probe:
			b.br, b.fails = brClosed, 0
			p.emit("breaker_close", b.ID+": a prova deu certo")
		}
		if gen && runMS > 0 {
			b.ring[b.ringN%ringSize] = runMS
			b.ringN++
			if b.ewma == 0 {
				b.ewma = float64(runMS)
			} else {
				b.ewma = (1-ewmaAlpha)*b.ewma + ewmaAlpha*float64(runMS)
			}
		}
	case OutcomeFail:
		p.failLocked(b, now, false)
	}
}

// ---- overload ---------------------------------------------------------------------

// LoadInput is what the queue knows about the primary.
type LoadInput struct {
	WaitingGen  int // generation requests waiting (all of them)
	PrimaryUsed int // generation slots the primary is using
}

// Overload is the verdict of Evaluate.
type Overload struct {
	On      bool
	Rules   string // active triggers, "S1+S3", "hist" while only the hold keeps it on
	S1, S2  bool
	S3, S4  bool
	EstWait time.Duration
}

// unhealthy is the soft health of a backend (not the hard gate of CanStart).
func (b *Backend) unhealthy() bool { return b.checkFailed || b.degraded || b.br != brClosed }

// queued is the real queue seen by the dispatcher: the waiting generation
// requests that do not fit in the free slots of the primary. A request that is
// about to start (it is in the line only because the dispatcher is looking at
// it) is not queued.
func (p *Pool) queued(in LoadInput) int {
	free := p.primary.Slots - in.PrimaryUsed
	if free < 0 {
		free = 0
	}
	if q := in.WaitingGen - free; q > 0 {
		return q
	}
	return 0
}

// estWait is the estimated wait of the newest waiting request: EWMA of the
// duration of a primary generation times the requests ahead of it (the waiting
// ones before it and the ones running), spread over the primary slots. The
// request itself is not counted. It is 0 while it fits in a free slot.
func (p *Pool) estWait(in LoadInput) time.Duration {
	if p.queued(in) == 0 || p.primary.ewma == 0 {
		return 0
	}
	ahead := in.WaitingGen - 1 + in.PrimaryUsed
	return time.Duration(p.primary.ewma * float64(ahead) / float64(p.primary.Slots) * float64(time.Millisecond))
}

// Evaluate computes the overload state of the primary (S1..S4) with hysteresis:
// it turns on at once and turns off only after the load stayed under half of
// the thresholds, and the primary stayed healthy, for Hold. Without
// secondaries nothing is evaluated (there is nowhere to send work).
func (p *Pool) Evaluate(now time.Time, in LoadInput) Overload {
	p.lock()
	defer p.unlock()
	if len(p.secs) == 0 {
		return Overload{}
	}
	est := p.estWait(in)
	o := Overload{EstWait: est}
	o.S1 = est > p.cfg.OverloadWait
	q := p.queued(in)
	o.S2 = q > p.cfg.OverloadQueue
	pr := p.primary
	p.advance(pr, now)
	o.S3 = pr.unhealthy() || pr.drained
	o.S4 = p.cfg.EnergyHook != nil && p.cfg.EnergyHook()
	var rules []string
	for _, r := range []struct {
		on bool
		n  string
	}{{o.S1, "S1"}, {o.S2, "S2"}, {o.S3, "S3"}, {o.S4, "S4"}} {
		if r.on {
			rules = append(rules, r.n)
		}
	}
	trig := len(rules) > 0
	switch {
	case !p.over && trig:
		p.over, p.overWhy, p.calm = true, strings.Join(rules, "+"), time.Time{}
		p.emit("overload_on", strings.Join(rules, "+")+" espera_estimada="+est.Round(time.Second).String()+" fila="+itoa(q))
	case p.over && !trig:
		calm := est < p.cfg.OverloadWait/2 && q*2 < p.cfg.OverloadQueue
		switch {
		case !calm:
			p.calm = time.Time{}
		case p.calm.IsZero():
			p.calm = now
		case now.Sub(p.calm) >= p.cfg.Hold:
			p.over, p.overWhy, p.calm = false, "", time.Time{}
			p.emit("overload_off", "carga abaixo de 50% dos limiares por "+p.cfg.Hold.String())
		}
	case p.over && trig:
		p.calm = time.Time{}
		p.overWhy = strings.Join(rules, "+")
	}
	o.On = p.over
	o.Rules = strings.Join(rules, "+")
	if o.On && o.Rules == "" {
		o.Rules = "hist"
	}
	p.lastEval = o
	return o
}

// PrimaryYields tells whether the dispatcher should try the secondaries before
// the primary: the primary is unhealthy (down, off the GPU, breaker open) and
// is not offering its half-open probe. The probe must reach the primary, or an
// open breaker could never close while a secondary is available.
func (p *Pool) PrimaryYields(now time.Time) bool {
	p.lock()
	defer p.unlock()
	pr := p.primary
	p.advance(pr, now)
	if pr.br == brHalf && !pr.probing && !pr.checkFailed && !pr.degraded {
		return false
	}
	return pr.unhealthy()
}

// ---- routing eligibility ----------------------------------------------------------

// Cand is the routing view of a secondary.
type Cand struct {
	Kind      Kind
	Drained   bool
	Unhealthy bool
	Models    []string // empty: any model whose digest matches
	Classes   []string // empty: the defaults of the kind
	MaxTokens int      // 0: no limit
	Digest    string   // digest of the model on this backend ("" when not installed)
}

// Req is the routing view of a request that has not started.
type Req struct {
	Model         string
	Class         string
	Tokens        int64
	PrimaryDigest string // digest of the model on the primary ("" when unknown)
}

// NormModel drops the implicit ":latest".
func NormModel(m string) string { return strings.TrimSuffix(strings.TrimSpace(m), ":latest") }

// Eligible is the pure rule F2/F3/F4 of the plan: a secondary may take the
// request when it is usable, serves the same model with the same digest as the
// primary, accepts the priority class and the prompt fits. "alta" never goes to
// a cpu backend, whatever the file says.
func Eligible(c Cand, r Req) bool {
	if c.Drained || c.Unhealthy {
		return false
	}
	if c.Kind == KindCPU && r.Class == ClassHigh {
		return false
	}
	classes := c.Classes
	if len(classes) == 0 {
		if c.Kind == KindCPU {
			classes = []string{ClassNormal, ClassLow}
		} else {
			classes = []string{ClassHigh, ClassNormal, ClassLow}
		}
	}
	okClass := false
	for _, k := range classes {
		if k == r.Class {
			okClass = true
		}
	}
	if !okClass {
		return false
	}
	if len(c.Models) > 0 {
		found := false
		for _, m := range c.Models {
			if NormModel(m) == NormModel(r.Model) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if c.MaxTokens > 0 && r.Tokens > int64(c.MaxTokens) {
		return false
	}
	// Same model, same digest: without a known digest on both sides there is no
	// proof, and the answer must not change with the backend.
	return r.PrimaryDigest != "" && c.Digest != "" && r.PrimaryDigest == c.Digest
}

// Eligible applies the pure rule to the live state of secondary sec.
func (p *Pool) Eligible(sec *Backend, model, class string, tokens int64) bool {
	p.lock()
	defer p.unlock()
	if sec == nil || sec.Primary {
		return false
	}
	m := NormModel(model)
	return Eligible(Cand{
		Kind: sec.Kind(), Drained: sec.drained, Unhealthy: sec.unhealthy(),
		Models: sec.Modelos, Classes: sec.Classes, MaxTokens: sec.MaxPromptTokens, Digest: sec.installed[m],
	}, Req{Model: model, Class: class, Tokens: tokens, PrimaryDigest: p.primary.installed[m]})
}

// ---- drain ------------------------------------------------------------------------

// SetDrained stops (or resumes) new starts on a backend; what is running
// finishes. ok is false for an unknown id; changed is false when nothing
// changed.
func (p *Pool) SetDrained(id string, v bool) (changed, ok bool) {
	p.lock()
	defer p.unlock()
	b := p.byID[id]
	if b == nil {
		return false, false
	}
	if b.drained == v {
		return false, true
	}
	b.drained = v
	if v {
		p.emit("backend_drenado", id)
	} else {
		p.emit("backend_retomado", id)
	}
	return true, true
}

// ---- status -----------------------------------------------------------------------

// Load is what the queue knows about one backend, for the status board.
type Load struct {
	Used    int // slots in use
	Waiting int // waiting requests this backend could take
}

// Status is one row of the "Backends" board.
type Status struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Primary   bool      `json:"primary"`
	Host      string    `json:"host"`
	State     string    `json:"state"` // ok, sobrecarregado, disjuntor_aberto, fora, degradado, drenado
	Breaker   string    `json:"breaker"`
	Drained   bool      `json:"drained"`
	Slots     int       `json:"slots"`
	Used      int       `json:"used"`
	Free      int       `json:"free"`
	Queue     int       `json:"queue"`
	P50MS     int64     `json:"p50_ms"`
	Model     string    `json:"model"`
	Digest    string    `json:"digest"`
	Processor string    `json:"processor"`
	LastError string    `json:"last_error"`
	LastCheck time.Time `json:"last_check"`
	Version   string    `json:"version"`
	Order     int       `json:"order"`
	Rules     string    `json:"rules,omitempty"` // active overload triggers (primary)
}

func (b *Backend) p50() int64 {
	n := b.ringN
	if n > ringSize {
		n = ringSize
	}
	if n == 0 {
		return 0
	}
	v := append([]int64(nil), b.ring[:n]...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	return v[(n-1)/2]
}

// Status returns the board, primary first. loads maps a backend id to what the
// queue sees.
func (p *Pool) Status(now time.Time, loads map[string]Load) []Status {
	p.lock()
	defer p.unlock()
	out := make([]Status, 0, 1+len(p.secs))
	for _, b := range append([]*Backend{p.primary}, p.secs...) {
		p.advance(b, now)
		ld := loads[b.ID]
		s := Status{
			ID: b.ID, Type: b.Tipo, Primary: b.Primary, Host: b.Host(), Breaker: b.br.String(), Drained: b.drained,
			Slots: b.Slots, Used: ld.Used, Queue: ld.Waiting, P50MS: b.p50(), LastError: b.lastErr, LastCheck: b.lastCheck,
			Version: b.version, Order: b.Ordem,
		}
		if s.Free = b.Slots - ld.Used; s.Free < 0 {
			s.Free = 0
		}
		// The model: the resident one, otherwise the first one the registry names, otherwise the first installed.
		if len(b.loaded) > 0 {
			l := b.loaded[0]
			s.Model, s.Processor, s.Digest = b.scrub(l.Name, 96), b.scrub(l.Processor, 32), b.scrub(l.Digest, 80)
		} else if len(b.Modelos) > 0 {
			s.Model = b.Modelos[0]
		} else if len(b.installed) > 0 { // nothing resident: show the first installed model
			names := make([]string, 0, len(b.installed))
			for n := range b.installed {
				names = append(names, n)
			}
			sort.Strings(names)
			s.Model = b.scrub(names[0], 96)
		}
		if s.Digest == "" && s.Model != "" {
			s.Digest = b.installed[NormModel(s.Model)]
		}
		if len(s.Digest) > 12 {
			s.Digest = s.Digest[:12]
		}
		switch {
		case b.drained:
			s.State = "drenado"
		case b.checkFailed:
			s.State = "fora"
		case b.br != brClosed:
			s.State = "disjuntor_aberto"
		case b.degraded:
			s.State = "degradado"
		case b.Primary && p.over && len(p.secs) > 0:
			s.State, s.Rules = "sobrecarregado", p.lastEval.Rules
		default:
			s.State = "ok"
		}
		out = append(out, s)
	}
	return out
}

// ---- active health ----------------------------------------------------------------

// Run checks every backend that has a URL on a timer (a first check at once)
// until ctx ends. The primary's models collector is driven from here, so the
// "Modelos" board and the health share one reading.
func (p *Pool) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, b := range p.Backends() {
		if b.col == nil || b.URL == nil {
			continue
		}
		wg.Add(1)
		go func(b *Backend) {
			defer wg.Done()
			t := time.NewTicker(p.cfg.CheckEvery)
			defer t.Stop()
			for {
				start := time.Now()
				p.CheckOnce(ctx, b)
				if time.Since(start) >= p.cfg.CheckEvery { // a slow round drops the tick that came due
					select {
					case <-t.C:
					default:
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}(b)
	}
	wg.Wait()
}

// CheckOnce runs one active check of b: /api/version, then /api/ps and
// /api/tags (the resident model, whether it left 100% GPU, the digests).
//
// The reading of the models (/api/ps, /api/tags) comes first and always runs:
// on the primary it is also the "Modelos" board, which must show the error when
// the Ollama is down, as the collector's own loop did before the pool (revisão
// 10.5, N1). /api/version only runs when that reading worked.
func (p *Pool) CheckOnce(ctx context.Context, b *Backend) {
	if b.col == nil {
		return
	}
	var v struct {
		Version string `json:"version"`
	}
	var err error
	snap := b.col.Once(ctx)
	if snap.Err != "" {
		err = errors.New(snap.Err)
	} else {
		err = b.col.Get(ctx, "/api/version", &v)
	}
	p.observe(b, v.Version, snap, err)
}

// Observe feeds the result of an active check to the pool as CheckOnce would:
// err is the failure of the check (nil when it worked), snap what /api/ps and
// /api/tags returned. Tests and fuzzers use it to set the state of a backend
// without a server.
func (p *Pool) Observe(id, version string, snap models.Snapshot, err error) {
	if b := p.byID[id]; b != nil {
		p.observe(b, version, snap, err)
	}
}

func (p *Pool) observe(b *Backend, version string, snap models.Snapshot, err error) {
	now := p.cfg.Now()
	p.lock()
	b.checked, b.lastCheck = true, now
	was := b.checkFailed
	if err != nil {
		msg := b.scrub(shortErr(err), 160)
		b.checkFailed, b.lastErr = true, msg
		if !was {
			p.emit("backend_down", b.ID+": "+msg)
		}
		p.failLocked(b, now, true)
	} else {
		b.checkFailed, b.version = false, cleanVersion(b.scrub(version, 64))
		b.loaded = snap.Loaded
		b.installed = make(map[string]string, len(snap.Installed))
		for _, m := range snap.Installed {
			if m.Digest != "" {
				b.installed[NormModel(m.Name)] = m.Digest
			}
		}
		b.degraded = false
		if b.Kind() != KindCPU {
			for _, l := range snap.Loaded {
				if l.Processor != "100% GPU" && l.Processor != "-" {
					b.degraded = true
					b.lastErr = b.scrub("modelo "+l.Name+" fora de 100% GPU: "+l.Processor, 160)
				}
			}
		}
		if was {
			p.emit("backend_up", b.ID)
		}
		if b.failActive && b.br == brClosed {
			b.fails = 0
		}
		// A successful active check is a probe too: it reached the backend
		// (version, ps and tags). Without this a secondary whose breaker opened
		// stayed half-open forever, because a half-open secondary is not eligible
		// and so never got the probe request (revisão 10.5, B1). The open period is
		// kept: only an open breaker whose time is up closes here.
		p.advance(b, now)
		if b.br == brHalf && !b.probing {
			b.br, b.fails = brClosed, 0
			p.emit("breaker_close", b.ID+": a checagem ativa deu certo")
		}
	}
	p.unlock()
	p.changed()
}

// scrub removes the backend's own token from text that came from the backend
// (a misconfigured or hostile secondary could echo the Authorization header in
// its answers) and clips it, so it is safe for the open monitor state.
func (b *Backend) scrub(s string, max int) string {
	if b.token != "" {
		s = strings.ReplaceAll(s, b.token, "[redigido]")
	}
	if len(s) > max {
		s = s[:max]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// cleanVersion keeps a version string to the characters a version has.
func cleanVersion(v string) string {
	out := make([]byte, 0, len(v))
	for i := 0; i < len(v) && len(out) < 32; i++ {
		c := v[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '.' || c == '-' || c == '+' || c == '_' {
			out = append(out, c)
		}
	}
	return string(out)
}

func shortErr(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

func itoa(n int) string { return strconv.Itoa(n) }
