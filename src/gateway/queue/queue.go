// Package queue is the waiting line of the gateway (Fase 9.1). It sits in
// front of the Ollama and decides who runs when.
//
// Rules (plan section 3):
//   - two independent lanes (geracao, embed), each with its own slots;
//   - three priorities (alta, normal, baixa); a baixa request that waits
//     longer than AgeAfter competes as normal (aging);
//   - inside one priority, fairness between clients (round-robin by IP+label,
//     FIFO inside one client);
//   - limits: total waiting, waiting per client, maximum wait;
//   - a request leaves the line when its context is cancelled (client gone);
//   - a minimum gap between two starts of the geracao lane (StartStagger);
//   - atomic administrator actions under the lock: promote, demote, remove (a
//     waiting request) and cancel (a running one).
//
// The line lives in memory and is the only truth about "now". Persistence is
// somebody else's job: the manager only calls the OnRecord/OnChange hooks,
// always outside its lock, and the hooks must not block.
package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Lane separates workloads that must not wait behind each other.
type Lane string

const (
	LaneGen   Lane = "geracao"
	LaneEmbed Lane = "embed"
)

// Priority orders requests; the zero value is not valid on purpose.
type Priority int

const (
	Low Priority = iota + 1
	Normal
	High
)

func (p Priority) String() string {
	switch p {
	case Low:
		return "baixa"
	case High:
		return "alta"
	case Normal:
		return "normal"
	}
	return "?"
}

// ParsePriority maps a header value to a Priority; ok is false for anything
// else (the caller decides the fallback).
func ParsePriority(s string) (Priority, bool) {
	switch s {
	case "alta":
		return High, true
	case "normal":
		return Normal, true
	case "baixa":
		return Low, true
	}
	return Normal, false
}

// Outcomes the callers branch on with errors.Is.
var (
	ErrFull          = errors.New("fila cheia")
	ErrClientFull    = errors.New("limite de pedidos em espera deste cliente")
	ErrWaitTimeout   = errors.New("tempo máximo de espera na fila esgotado")
	ErrRemoved       = errors.New("removido da fila pelo administrador")
	ErrAdminCanceled = errors.New("cancelado pelo administrador")
	ErrNotWaiting    = errors.New("não está mais na fila")
	ErrNotRunning    = errors.New("não está mais em execução")
)

// FullError carries the Retry-After hint of a rejection (503).
type FullError struct {
	Kind       error // ErrFull or ErrClientFull
	RetryAfter time.Duration
}

func (e *FullError) Error() string { return e.Kind.Error() }
func (e *FullError) Unwrap() error { return e.Kind }

// Status values of a Record.
const (
	StatusOK            = "ok"
	StatusError         = "erro"
	StatusCanceled      = "cancelado"       // the client gave up
	StatusAdminCanceled = "cancelado_admin" // running request cancelled by the administrator
	StatusRemoved       = "removido"        // waiting request removed by the administrator
	StatusRejected      = "rejeitado"       // queue full
	StatusExpired       = "expirado"        // MaxWait elapsed
)

// Config tunes a Manager. Zero values take the defaults below.
type Config struct {
	GenSlots     int           // default 1
	EmbedSlots   int           // default 2
	QueueMax     int           // total waiting requests, default 64
	PerClient    int           // waiting requests per IP+label, default 16
	PerIP        int           // waiting requests per IP, all labels together; default PerClient
	LabelsPerIP  int           // distinct labels per IP; more are folded into "<ip>/outros"; default 16
	MaxWait      time.Duration // default 10 min
	AgeAfter     time.Duration // baixa -> normal, default 5 min
	StartStagger time.Duration // minimum gap between two geracao starts; 0 disables

	// OnRecord receives one Record per request that leaves the system. OnChange
	// is called after every state change. Both run outside the lock and must
	// not block.
	OnRecord func(Record)
	OnChange func()
}

func (c Config) withDefaults() Config {
	if c.GenSlots < 1 {
		c.GenSlots = 1
	}
	if c.EmbedSlots < 1 {
		c.EmbedSlots = 2
	}
	if c.QueueMax < 1 {
		c.QueueMax = 64
	}
	if c.PerClient < 1 {
		c.PerClient = 16
	}
	if c.PerIP < 1 {
		c.PerIP = c.PerClient
	}
	if c.LabelsPerIP < 1 {
		c.LabelsPerIP = 16
	}
	if c.MaxWait <= 0 {
		c.MaxWait = 10 * time.Minute
	}
	if c.AgeAfter <= 0 {
		c.AgeAfter = 5 * time.Minute
	}
	return c
}

// Request describes who is asking for a slot. Strings are already validated
// by the proxy.
type Request struct {
	ID            string // optional; generated when empty
	Lane          Lane
	ClientIP      string
	ClientLabel   string
	Tool          string
	Endpoint      string
	Model         string
	PriorityIn    Priority // what the client asked
	PriorityFinal Priority // what the gateway granted
	BytesIn       int64
}

// Result is what the proxy reports when the request ends.
type Result struct {
	Status     string // empty means StatusOK
	HTTPStatus int
	ErrorClass string
	PromptEval int64
	Eval       int64
	LoadMS     int64
}

// Record is the metadata row of a request (no text, plan section 5).
type Record struct {
	ID            string
	ClientIP      string
	ClientLabel   string
	Tool          string
	Endpoint      string
	Model         string
	Lane          Lane
	PriorityIn    Priority
	PriorityFinal Priority
	BytesIn       int64
	EnqueuedAt    time.Time
	StartedAt     time.Time // zero when it never ran
	FinishedAt    time.Time
	WaitMS        int64
	RunMS         int64
	Status        string
	HTTPStatus    int
	ErrorClass    string
	PromptEval    int64
	Eval          int64
	LoadMS        int64
}

type state int

const (
	stWaiting state = iota
	stRunning
	stDone // left the line or released the slot
)

// Ticket is a granted slot. The holder must call Release (more calls are
// harmless) and should run its upstream call with Ctx, which the administrator
// can cancel.
type Ticket struct {
	ID       string
	Ctx      context.Context
	Waited   time.Duration
	Position int // place in the line when it entered; 0 when it started at once

	m      *Manager
	req    Request
	cancel context.CancelCauseFunc

	// guarded by m.mu
	st        state
	err       error
	ready     chan struct{}
	level     Priority // current level (promote/demote change it)
	prio      int64    // FIFO key inside the level
	rank      int64    // <0 pinned on top, >0 pushed to the bottom
	enqueued  time.Time
	started   time.Time
	finalPrio Priority
	adminCx   bool
	recorded  bool
	key       string
}

// Manager is the line. Safe for concurrent use.
type Manager struct {
	cfg Config

	mu        sync.Mutex
	waiting   map[Lane][]*Ticket // arrival order
	running   map[Lane]map[string]*Ticket
	byID      map[string]*Ticket
	perClient map[string]int            // waiting requests per IP+label
	perIP     map[string]int            // waiting requests per IP (all labels)
	labels    map[string]map[string]int // active tickets per IP and label (bounds distinct labels)
	served    map[string]served
	seq       int64       // FIFO keys
	topSeq    int64       // pin counter (decreasing)
	botSeq    int64       // push-to-bottom counter (increasing)
	startSeq  uint64      // fairness clock
	lastStart time.Time   // last geracao start
	timer     *time.Timer // pending stagger wake-up
	after     []func()    // hooks to run after unlock
	totals    Totals
}

type served struct {
	seq  uint64
	last time.Time
	ip   string
}

// Totals are counters since the start of the process.
type Totals struct {
	Started  int64 `json:"started"`
	Finished int64 `json:"finished"`
	Rejected int64 `json:"rejected"`
	Expired  int64 `json:"expired"`
	Canceled int64 `json:"canceled"`
	Removed  int64 `json:"removed"`
}

// New builds a Manager.
func New(cfg Config) *Manager {
	cfg = cfg.withDefaults()
	return &Manager{
		cfg:       cfg,
		waiting:   map[Lane][]*Ticket{LaneGen: nil, LaneEmbed: nil},
		running:   map[Lane]map[string]*Ticket{LaneGen: {}, LaneEmbed: {}},
		byID:      map[string]*Ticket{},
		perClient: map[string]int{},
		perIP:     map[string]int{},
		labels:    map[string]map[string]int{},
		served:    map[string]served{},
	}
}

// Slots returns the number of slots of a lane.
func (m *Manager) Slots(l Lane) int {
	if l == LaneEmbed {
		return m.cfg.EmbedSlots
	}
	return m.cfg.GenSlots
}

// Config returns the effective configuration (hooks included).
func (m *Manager) Config() Config { return m.cfg }

// NewID returns a random 128-bit id in hex.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("queue: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

func clientKey(ip, label string) string { return ip + "|" + label }

func (m *Manager) lockOp() { m.mu.Lock() }

// unlockOp unlocks and runs the hooks queued while the lock was held.
func (m *Manager) unlockOp() {
	fns := m.after
	m.after = nil
	m.mu.Unlock()
	for _, f := range fns {
		f()
	}
}

func (m *Manager) changed() {
	if f := m.cfg.OnChange; f != nil {
		m.after = append(m.after, f)
	}
}

func (m *Manager) record(r Record) {
	if f := m.cfg.OnRecord; f != nil {
		m.after = append(m.after, func() { f(r) })
	}
}

// Acquire waits for a slot in req.Lane. It returns when the request starts
// (a Ticket), when the line rejects it (*FullError, 503), when MaxWait passes
// (ErrWaitTimeout, 504), when the administrator removes it (ErrRemoved, 409)
// or when ctx ends (the ctx error: the client left).
func (m *Manager) Acquire(ctx context.Context, req Request) (*Ticket, error) {
	if req.Lane != LaneEmbed {
		req.Lane = LaneGen
	}
	if req.PriorityFinal < Low || req.PriorityFinal > High {
		req.PriorityFinal = Normal
	}
	if req.PriorityIn < Low || req.PriorityIn > High {
		req.PriorityIn = req.PriorityFinal
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now()
	if req.ID == "" {
		req.ID = NewID()
	}
	t := &Ticket{
		ID: req.ID, m: m, req: req, ready: make(chan struct{}),
		level: req.PriorityFinal, enqueued: now,
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	t.Ctx, t.cancel = runCtx, cancel

	m.lockOp()
	reject := func(kind error) (*Ticket, error) {
		m.totals.Rejected++
		m.record(m.endRecord(t, now, Result{Status: StatusRejected, HTTPStatus: 503, ErrorClass: kind.Error()}))
		m.changed()
		m.unlockOp()
		cancel(nil)
		return nil, &FullError{Kind: kind, RetryAfter: retryAfter}
	}
	// The IP comes from the proxy (not forgeable by the client); the label is
	// client-controlled. Limits and fairness anchor on the IP, and the number
	// of distinct labels per IP is bounded.
	t.req.ClientLabel = m.normLabelLocked(req.ClientIP, req.ClientLabel)
	t.key = clientKey(t.req.ClientIP, t.req.ClientLabel)
	if m.perIP[req.ClientIP] >= m.cfg.PerIP || m.perClient[t.key] >= m.cfg.PerClient {
		return reject(ErrClientFull)
	}
	if m.totalWaitingLocked() >= m.cfg.QueueMax {
		return reject(ErrFull)
	}
	m.seq++
	t.prio = m.seq
	t.Position = len(m.waiting[req.Lane]) + 1
	m.waiting[req.Lane] = append(m.waiting[req.Lane], t)
	m.byID[t.ID] = t
	m.perClient[t.key]++
	m.perIP[req.ClientIP]++
	m.refLabelLocked(t, +1)
	m.dispatchLocked()
	if t.st == stRunning {
		t.Position = 0 // started at once
	}
	m.changed()
	m.unlockOp()

	timer := time.NewTimer(m.cfg.MaxWait)
	defer timer.Stop()
	select {
	case <-t.ready:
	case <-ctx.Done():
		return nil, m.abandon(t, ctx.Err(), StatusCanceled)
	case <-timer.C:
		return nil, m.abandon(t, ErrWaitTimeout, StatusExpired)
	}
	if t.err != nil {
		cancel(nil)
		return nil, t.err
	}
	t.Waited = t.started.Sub(t.enqueued)
	return t, nil
}

// retryAfter is a simple, honest hint for a full line; clients add jitter.
const retryAfter = 5 * time.Second

// abandon takes a request out of the line because the caller stopped waiting.
// If the request was started in the meantime, the slot is given back.
func (m *Manager) abandon(t *Ticket, cause error, status string) error {
	m.lockOp()
	defer m.unlockOp()
	switch t.st {
	case stWaiting:
		m.removeWaitingLocked(t)
		t.st = stDone
		t.err = cause
		if status == StatusExpired {
			m.totals.Expired++
		} else {
			m.totals.Canceled++
		}
		m.record(m.endRecord(t, time.Now(), Result{Status: status, HTTPStatus: abandonHTTP(status), ErrorClass: status}))
		t.cancel(nil)
		m.changed()
		return cause
	case stRunning:
		// Started between the wake-up and now: give the slot back.
		m.finishLocked(t, time.Now(), Result{Status: status, HTTPStatus: abandonHTTP(status), ErrorClass: status})
		return cause
	}
	// Already gone. A running request cancelled by the administrator between
	// the wake-up and now never reaches the proxy (no ticket, no Release):
	// emit its record here, once (Fase 9.5, Q1).
	if t.adminCx && !t.recorded {
		t.recorded = true
		m.record(m.endRecord(t, time.Now(), Result{Status: StatusAdminCanceled, HTTPStatus: 409, ErrorClass: "admin_cancel"}))
		m.changed()
	}
	if t.err != nil { // removed by the administrator
		return t.err
	}
	return cause
}

func abandonHTTP(status string) int {
	if status == StatusExpired {
		return 504
	}
	return 499
}

func (m *Manager) endRecord(t *Ticket, now time.Time, r Result) Record {
	rec := Record{
		ID: t.ID, ClientIP: t.req.ClientIP, ClientLabel: t.req.ClientLabel, Tool: t.req.Tool,
		Endpoint: t.req.Endpoint, Model: t.req.Model, Lane: t.req.Lane,
		PriorityIn: t.req.PriorityIn, PriorityFinal: t.finalPrio,
		BytesIn: t.req.BytesIn, EnqueuedAt: t.enqueued, FinishedAt: now,
		Status: r.Status, HTTPStatus: r.HTTPStatus, ErrorClass: r.ErrorClass,
		PromptEval: r.PromptEval, Eval: r.Eval, LoadMS: r.LoadMS,
	}
	if rec.PriorityFinal == 0 {
		rec.PriorityFinal = t.level
	}
	if rec.Status == "" {
		rec.Status = StatusOK
	}
	if t.started.IsZero() {
		rec.WaitMS = now.Sub(t.enqueued).Milliseconds()
	} else {
		rec.StartedAt = t.started
		rec.WaitMS = t.started.Sub(t.enqueued).Milliseconds()
		rec.RunMS = now.Sub(t.started).Milliseconds()
	}
	return rec
}

// Release gives the slot back and reports the outcome. Calling it again does
// nothing.
func (t *Ticket) Release(r Result) {
	m := t.m
	m.lockOp()
	defer m.unlockOp()
	if t.st == stRunning {
		m.finishLocked(t, time.Now(), r)
		return
	}
	// Cancel by the administrator freed the slot already; the record is
	// emitted here, once, with what the proxy saw.
	if t.st == stDone && t.adminCx && !t.recorded {
		t.recorded = true
		r.Status = StatusAdminCanceled
		m.record(m.endRecord(t, time.Now(), r))
		m.changed()
	}
}

// Req returns the request data of the ticket.
func (t *Ticket) Req() Request { return t.req }

// FinalPriority is the priority the request had when it started.
func (t *Ticket) FinalPriority() Priority {
	t.m.mu.Lock()
	defer t.m.mu.Unlock()
	return t.finalPrio
}

func (m *Manager) finishLocked(t *Ticket, now time.Time, r Result) {
	delete(m.running[t.req.Lane], t.ID)
	delete(m.byID, t.ID)
	m.refLabelLocked(t, -1)
	t.st = stDone
	t.recorded = true
	m.totals.Finished++
	m.record(m.endRecord(t, now, r))
	t.cancel(nil)
	m.dispatchLocked()
	m.changed()
}

func (m *Manager) totalWaitingLocked() int {
	n := 0
	for _, w := range m.waiting {
		n += len(w)
	}
	return n
}

func (m *Manager) removeWaitingLocked(t *Ticket) {
	w := m.waiting[t.req.Lane]
	for i, x := range w {
		if x == t {
			m.waiting[t.req.Lane] = append(w[:i:i], w[i+1:]...)
			break
		}
	}
	delete(m.byID, t.ID)
	m.dropPerClientLocked(t)
	m.refLabelLocked(t, -1)
}

// normLabelLocked folds a label into "<ip>/outros" when the IP already uses
// LabelsPerIP distinct labels, so varying the label cannot multiply the
// client's share. The request is never rejected for this.
func (m *Manager) normLabelLocked(ip, label string) string {
	set := m.labels[ip]
	if _, ok := set[label]; ok || len(set) < m.cfg.LabelsPerIP {
		return label
	}
	return ip + "/outros"
}

// refLabelLocked counts the active tickets that use a label of an IP.
func (m *Manager) refLabelLocked(t *Ticket, d int) {
	ip, l := t.req.ClientIP, t.req.ClientLabel
	set := m.labels[ip]
	if set == nil {
		set = map[string]int{}
		m.labels[ip] = set
	}
	if n := set[l] + d; n > 0 {
		set[l] = n
	} else {
		delete(set, l)
	}
	if len(set) == 0 {
		delete(m.labels, ip)
	}
}

func (m *Manager) dropPerClientLocked(t *Ticket) {
	if n := m.perIP[t.req.ClientIP] - 1; n > 0 {
		m.perIP[t.req.ClientIP] = n
	} else {
		delete(m.perIP, t.req.ClientIP)
	}
	if n := m.perClient[t.key] - 1; n > 0 {
		m.perClient[t.key] = n
	} else {
		delete(m.perClient, t.key)
	}
}

// effectiveLocked is the level a waiting request competes at (aging).
func (m *Manager) effectiveLocked(t *Ticket, now time.Time) Priority {
	if t.level == Low && now.Sub(t.enqueued) >= m.cfg.AgeAfter {
		return Normal
	}
	return t.level
}

// pickLocked returns the index of the next request of w. served is the
// fairness memory (read only).
func (m *Manager) pickLocked(w []*Ticket, now time.Time, served map[string]uint64) int {
	if len(w) == 0 {
		return -1
	}
	top := Priority(0)
	for _, t := range w {
		if e := m.effectiveLocked(t, now); e > top {
			top = e
		}
	}
	best := -1
	// 1) pinned on top: the most recent pin wins.
	for i, t := range w {
		if m.effectiveLocked(t, now) == top && t.rank < 0 && (best < 0 || t.rank < w[best].rank) {
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	// 2) fairness, two levels: first between IPs (the one served longest ago,
	// then the one with the oldest request), then between the labels of that
	// IP, then FIFO. The IP is not forgeable; the label is.
	oldest := map[string]int64{}
	for _, t := range w {
		if m.effectiveLocked(t, now) != top || t.rank != 0 {
			continue
		}
		if p, ok := oldest[t.req.ClientIP]; !ok || t.prio < p {
			oldest[t.req.ClientIP] = t.prio
		}
	}
	if len(oldest) > 0 {
		winIP, winSeen, winPrio := "", uint64(0), int64(0)
		first := true
		for ip, p := range oldest {
			sv := served[ip]
			if first || sv < winSeen || (sv == winSeen && p < winPrio) {
				winIP, winSeen, winPrio, first = ip, sv, p, false
			}
		}
		for i, t := range w {
			if t.req.ClientIP != winIP || m.effectiveLocked(t, now) != top || t.rank != 0 {
				continue
			}
			if best < 0 {
				best = i
				continue
			}
			b := w[best]
			sa, sb := served[t.key], served[b.key]
			if sa < sb || (sa == sb && t.prio < b.prio) {
				best = i
			}
		}
		return best
	}
	// 3) pushed to the bottom: the earliest push goes first.
	for i, t := range w {
		if m.effectiveLocked(t, now) == top && (best < 0 || t.rank < w[best].rank) {
			best = i
		}
	}
	return best
}

func (m *Manager) servedSnapshotLocked() map[string]uint64 {
	s := make(map[string]uint64, len(m.served))
	for k, v := range m.served {
		s[k] = v.seq
	}
	return s
}

// dispatchLocked starts as many waiting requests as the free slots and the
// stagger allow.
func (m *Manager) dispatchLocked() {
	now := time.Now()
	for _, lane := range []Lane{LaneEmbed, LaneGen} {
		for len(m.running[lane]) < m.Slots(lane) && len(m.waiting[lane]) > 0 {
			if lane == LaneGen && m.cfg.StartStagger > 0 && !m.lastStart.IsZero() {
				if next := m.lastStart.Add(m.cfg.StartStagger); now.Before(next) {
					m.armTimerLocked(next.Sub(now))
					break
				}
			}
			i := m.pickLocked(m.waiting[lane], now, m.servedSnapshotLocked())
			t := m.waiting[lane][i]
			m.waiting[lane] = append(m.waiting[lane][:i:i], m.waiting[lane][i+1:]...)
			m.dropPerClientLocked(t)
			t.st = stRunning
			t.started = now
			t.finalPrio = m.effectiveLocked(t, now)
			m.running[lane][t.ID] = t
			m.startSeq++
			m.served[t.key] = served{seq: m.startSeq, last: now, ip: t.req.ClientIP}
			m.served[t.req.ClientIP] = served{seq: m.startSeq, last: now, ip: t.req.ClientIP}
			if lane == LaneGen {
				m.lastStart = now
			}
			m.totals.Started++
			close(t.ready)
		}
	}
}

func (m *Manager) armTimerLocked(d time.Duration) {
	if m.timer != nil {
		return // one wake-up is enough: dispatch re-arms if needed
	}
	m.timer = time.AfterFunc(d, func() {
		m.lockOp()
		m.timer = nil
		m.dispatchLocked()
		m.changed()
		m.unlockOp()
	})
}

// ---- administrator actions -------------------------------------------------

// ActionResult describes what an action changed, for the audit trail.
type ActionResult struct {
	Before string
	After  string
}

func (m *Manager) waitingTicket(id string) (*Ticket, error) {
	t, ok := m.byID[id]
	if !ok || t.st != stWaiting {
		return nil, ErrNotWaiting
	}
	return t, nil
}

// Promote raises a waiting request one level; one already at alta goes to the
// top of its lane. A promoted request joins the end of the new level (it does
// not jump over requests that were already waiting there).
func (m *Manager) Promote(id string) (ActionResult, error) {
	m.lockOp()
	defer m.unlockOp()
	t, err := m.waitingTicket(id)
	if err != nil {
		return ActionResult{}, err
	}
	before := t.describe()
	if t.level < High {
		t.level++
		m.seq++
		t.prio = m.seq
		t.rank = 0
	} else {
		m.topSeq--
		t.rank = m.topSeq
	}
	m.changed()
	return ActionResult{Before: before, After: t.describe()}, nil
}

// Demote lowers a waiting request one level; one already at baixa goes to the
// end of its lane.
func (m *Manager) Demote(id string) (ActionResult, error) {
	m.lockOp()
	defer m.unlockOp()
	t, err := m.waitingTicket(id)
	if err != nil {
		return ActionResult{}, err
	}
	before := t.describe()
	if t.level > Low {
		t.level--
		m.seq++
		t.prio = m.seq
		t.rank = 0
	} else {
		m.botSeq++
		t.rank = m.botSeq
	}
	m.changed()
	return ActionResult{Before: before, After: t.describe()}, nil
}

// Remove takes a waiting request out of the line; its holder gets ErrRemoved.
func (m *Manager) Remove(id string) (ActionResult, error) {
	m.lockOp()
	defer m.unlockOp()
	t, err := m.waitingTicket(id)
	if err != nil {
		return ActionResult{}, err
	}
	before := t.describe()
	m.removeWaitingLocked(t)
	t.st = stDone
	t.err = ErrRemoved
	m.totals.Removed++
	m.record(m.endRecord(t, time.Now(), Result{Status: StatusRemoved, HTTPStatus: 409, ErrorClass: StatusRemoved}))
	t.cancel(nil)
	close(t.ready)
	m.changed()
	return ActionResult{Before: before, After: "removido"}, nil
}

// Cancel stops a running request: its context is cancelled (the proxy then
// closes the upstream connection) and the slot is freed at once.
func (m *Manager) Cancel(id string) (ActionResult, error) {
	m.lockOp()
	defer m.unlockOp()
	t, ok := m.byID[id]
	if !ok || t.st != stRunning {
		return ActionResult{}, ErrNotRunning
	}
	before := t.describe()
	t.adminCx = true
	delete(m.running[t.req.Lane], t.ID)
	delete(m.byID, t.ID)
	m.refLabelLocked(t, -1)
	t.st = stDone
	m.totals.Finished++
	t.cancel(ErrAdminCanceled)
	m.dispatchLocked()
	m.changed()
	return ActionResult{Before: before, After: "cancelado"}, nil
}

// IsAdminCancel tells whether ctx ended because of Cancel.
func IsAdminCancel(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrAdminCanceled)
}

func (t *Ticket) describe() string {
	pos := ""
	switch {
	case t.rank < 0:
		pos = " (topo)"
	case t.rank > 0:
		pos = " (fim)"
	}
	return fmt.Sprintf("%s/%s%s", t.req.Lane, t.level, pos)
}

// ---- snapshots ---------------------------------------------------------------

// Item is one request as the monitor shows it.
type Item struct {
	ID          string    `json:"id"`
	Lane        Lane      `json:"lane"`
	ClientIP    string    `json:"client_ip"`
	ClientLabel string    `json:"client_label"`
	Tool        string    `json:"tool"`
	Endpoint    string    `json:"endpoint"`
	Model       string    `json:"model"`
	PriorityIn  string    `json:"priority_in"`
	Priority    string    `json:"priority"` // current level (waiting) or granted (running)
	Position    int       `json:"position"` // 1-based place in the lane order; 0 when running
	EnqueuedAt  time.Time `json:"enqueued_at"`
	StartedAt   time.Time `json:"started_at"`
	WaitingMS   int64     `json:"waiting_ms"`
	RunningMS   int64     `json:"running_ms"`
	BytesIn     int64     `json:"bytes_in"`
}

// Snapshot is a consistent photo of the line.
type Snapshot struct {
	Now     time.Time    `json:"now"`
	Running []Item       `json:"running"`
	Waiting []Item       `json:"waiting"`
	Slots   map[Lane]int `json:"slots"`
	InUse   map[Lane]int `json:"in_use"`
	Totals  Totals       `json:"totals"`
}

// Snapshot returns the running requests (oldest first) and the waiting ones in
// the order they will start, lane by lane.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	s := Snapshot{
		Now: now, Slots: map[Lane]int{}, InUse: map[Lane]int{}, Totals: m.totals,
		Running: []Item{}, Waiting: []Item{},
	}
	for _, lane := range []Lane{LaneGen, LaneEmbed} {
		s.Slots[lane] = m.Slots(lane)
		s.InUse[lane] = len(m.running[lane])
		for _, t := range m.running[lane] {
			s.Running = append(s.Running, m.itemLocked(t, now, 0))
		}
		// Simulate the dispatch order on a copy of the fairness memory.
		w := append([]*Ticket(nil), m.waiting[lane]...)
		sv := m.servedSnapshotLocked()
		var clock uint64
		for _, v := range sv {
			if v > clock {
				clock = v
			}
		}
		for pos := 1; len(w) > 0; pos++ {
			i := m.pickLocked(w, now, sv)
			t := w[i]
			w = append(w[:i:i], w[i+1:]...)
			clock++
			sv[t.key] = clock
			sv[t.req.ClientIP] = clock
			s.Waiting = append(s.Waiting, m.itemLocked(t, now, pos))
		}
	}
	sort.SliceStable(s.Running, func(i, j int) bool { return s.Running[i].StartedAt.Before(s.Running[j].StartedAt) })
	return s
}

func (m *Manager) itemLocked(t *Ticket, now time.Time, pos int) Item {
	it := Item{
		ID: t.ID, Lane: t.req.Lane, ClientIP: t.req.ClientIP, ClientLabel: t.req.ClientLabel,
		Tool: t.req.Tool, Endpoint: t.req.Endpoint, Model: t.req.Model,
		PriorityIn: t.req.PriorityIn.String(), Position: pos,
		EnqueuedAt: t.enqueued, BytesIn: t.req.BytesIn,
	}
	if t.st == stRunning {
		it.Priority = t.finalPrio.String()
		it.StartedAt = t.started
		it.WaitingMS = t.started.Sub(t.enqueued).Milliseconds()
		it.RunningMS = now.Sub(t.started).Milliseconds()
	} else {
		it.Priority = m.effectiveLocked(t, now).String()
		it.WaitingMS = now.Sub(t.enqueued).Milliseconds()
	}
	return it
}

// Sweep drops fairness memory of clients idle for longer than idle. Call it
// from a slow ticker.
func (m *Manager) Sweep(idle time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cut := time.Now().Add(-idle)
	for k, v := range m.served {
		if v.last.Before(cut) && m.perIP[v.ip] == 0 {
			delete(m.served, k)
		}
	}
}

// checkInvariants is used by the tests: slots never exceeded, indexes
// consistent. It returns a description of the first violation or "".
func (m *Manager) checkInvariants() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	nWait, nRun := 0, 0
	perClient := map[string]int{}
	for lane, w := range m.waiting {
		for _, t := range w {
			nWait++
			perClient[t.key]++
			if t.st != stWaiting {
				return fmt.Sprintf("waiting ticket in state %d", t.st)
			}
			if m.byID[t.ID] != t {
				return "waiting ticket missing in byID"
			}
			if t.req.Lane != lane {
				return "ticket in wrong lane"
			}
		}
	}
	for lane, r := range m.running {
		if len(r) > m.Slots(lane) {
			return fmt.Sprintf("lane %s over capacity: %d > %d", lane, len(r), m.Slots(lane))
		}
		for _, t := range r {
			nRun++
			if t.st != stRunning || m.byID[t.ID] != t {
				return "running ticket inconsistent"
			}
		}
	}
	if len(m.byID) != nWait+nRun {
		return fmt.Sprintf("byID %d != waiting %d + running %d", len(m.byID), nWait, nRun)
	}
	perIP := map[string]int{}
	for _, w := range m.waiting {
		for _, t := range w {
			perIP[t.req.ClientIP]++
		}
	}
	for ip, v := range perIP {
		if m.perIP[ip] != v {
			return "perIP count mismatch"
		}
		if v > m.cfg.PerIP {
			return fmt.Sprintf("ip %s has %d waiting, over PerIP %d", ip, v, m.cfg.PerIP)
		}
	}
	if len(perIP) != len(m.perIP) {
		return "perIP size mismatch"
	}
	active := 0
	for ip, set := range m.labels {
		if len(set) > m.cfg.LabelsPerIP+1 {
			return fmt.Sprintf("ip %s uses %d labels", ip, len(set))
		}
		for _, n := range set {
			active += n
		}
	}
	if active != nWait+nRun {
		return fmt.Sprintf("label refs %d != active %d", active, nWait+nRun)
	}
	if len(perClient) != len(m.perClient) {
		return "perClient size mismatch"
	}
	for k, v := range perClient {
		if m.perClient[k] != v {
			return "perClient count mismatch"
		}
	}
	return ""
}
