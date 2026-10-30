// Package monitor serves /monitor (Fase 9.4): one embedded HTML page, a JSON
// API, a server-sent-events stream and the administrator actions.
//
// Access (D9-1): every GET is open (the LAN restriction is the nginx's job);
// every POST needs the session of the single administrator plus a CSRF token.
// GET handlers have no side effects. Nothing that came from a client is ever
// written into the page as HTML: the page builds its DOM with textContent, the
// JSON is encoded by encoding/json (which escapes <, > and &), and a CSP with
// hashes of the inline script and style forbids anything else.
package monitor

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/cpu"
	"github.com/cslsoftwares/mcpollama/gateway/gpu"
	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

//go:embed index.html
var rawIndexHTML []byte

// indexHTML is the page with CRLF folded to LF. The HTML parser normalizes
// newlines before the browser hashes an inline script or style, so a page built
// from a CRLF checkout would get CSP hashes that never match and the browser
// would block both (seen live on 2026-10-30: page unstyled, "conectando...").
var indexHTML = bytes.ReplaceAll(rawIndexHTML, []byte("\r\n"), []byte("\n"))

// Hub fans a "something changed" signal out to the SSE clients. The queue's
// OnChange hook calls Notify.
type Hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

// NewHub returns an empty Hub.
func NewHub() *Hub { return &Hub{subs: map[chan struct{}]struct{}{}} }

// Notify wakes every subscriber; never blocks.
func (h *Hub) Notify() {
	if h == nil {
		return
	}
	h.mu.Lock()
	for c := range h.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *Hub) subscribe() chan struct{} {
	c := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) unsubscribe(c chan struct{}) {
	h.mu.Lock()
	delete(h.subs, c)
	h.mu.Unlock()
}

// Config configures the Handler.
type Config struct {
	Queue   *queue.Manager
	Store   *store.Store // may be nil (no history)
	GPU     *gpu.Collector
	CPU     *cpu.Collector    // may be nil (board shows "coletor desligado")
	Models  *models.Collector // may be nil (no "models" in the state)
	Hub     *Hub
	Content *content.Store // may be nil: the text of the jobs is not recorded (Fase 10.L)
	Admin   *Credentials   // nil disables the actions
	Version string
	BootID  string
	MaxSSE  int // concurrent event streams, default 32
	// MaxSSEPerIP bounds the streams of one client IP (Fase 9.5, M3), so one
	// machine cannot take every stream; default 4.
	MaxSSEPerIP int
	// StreamEvery is the periodic push of the event stream (follows the
	// sampling interval); default 1 s. Queue changes are still sent at once.
	StreamEvery time.Duration
	// KeepDone is how long a finished request stays in State.RecentDone (display
	// only; the queue itself is not touched). Zero disables the list.
	KeepDone time.Duration
	// Now is the clock of the done list and its cache; default time.Now (tests inject one).
	Now  func() time.Time
	Logf func(format string, args ...any)
}

// Handler serves everything under /monitor.
type Handler struct {
	cfg  Config
	mux  *http.ServeMux
	sess sessions
	lim  limiter
	// verifySem bounds the concurrent password checks.
	verifySem chan struct{}
	started   time.Time
	csp       string

	sseMu    sync.Mutex
	sseTotal int
	sseByIP  map[string]int

	jobs jobsCache // Fase 10.L

	statsMu    sync.Mutex
	statsCache map[string]statsEntry
	statsN     atomic.Int64

	done      chan struct{} // closed by Close: ends the event streams
	closeOnce sync.Once

	extraMu  sync.Mutex
	extraAt  time.Time
	extra    extras
	series   seriesRing
	bucketMu sync.Mutex
	bucket   bucketAcc
}

// New builds the Handler.
func New(cfg Config) (*Handler, error) {
	if cfg.Queue == nil {
		return nil, errors.New("monitor: queue is required")
	}
	if cfg.MaxSSE <= 0 {
		cfg.MaxSSE = 32
	}
	if cfg.StreamEvery <= 0 {
		cfg.StreamEvery = time.Second
	}
	if cfg.MaxSSEPerIP <= 0 {
		cfg.MaxSSEPerIP = 4
	}
	if cfg.Hub == nil {
		cfg.Hub = NewHub()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	csp, err := buildCSP(indexHTML)
	if err != nil {
		return nil, err
	}
	h := &Handler{cfg: cfg, mux: http.NewServeMux(), started: time.Now(), csp: csp, verifySem: make(chan struct{}, verifySlots),
		sseByIP: map[string]int{}, statsCache: map[string]statsEntry{}, done: make(chan struct{})}
	h.routes()
	return h, nil
}

var (
	scriptRe = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	styleRe  = regexp.MustCompile(`(?s)<style>(.*?)</style>`)
)

// buildCSP hashes the inline script and style, so the page needs no
// 'unsafe-inline' and any injected markup would not run.
func buildCSP(page []byte) (string, error) {
	sc, st := scriptRe.FindSubmatch(page), styleRe.FindSubmatch(page)
	if sc == nil || st == nil {
		return "", errors.New("monitor: index.html must have one <script> and one <style>")
	}
	hash := func(b []byte) string {
		sum := sha256.Sum256(b)
		return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	return "default-src 'none'; script-src " + hash(sc[1]) + "; style-src " + hash(st[1]) +
		"; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'", nil
}

// Close ends the open event streams. The server's Shutdown does not cancel the
// request contexts, so without this an open panel held a restart for the whole
// grace period (Fase 9.5, C5). Register it with http.Server.RegisterOnShutdown.
func (h *Handler) Close() { h.closeOnce.Do(func() { close(h.done) }) }

func (h *Handler) logf(format string, a ...any) {
	if h.cfg.Logf != nil {
		h.cfg.Logf(format, a...)
	}
}

func (h *Handler) routes() {
	m := h.mux
	m.HandleFunc("GET /monitor", h.page)
	m.HandleFunc("GET /monitor/{$}", h.page)
	m.HandleFunc("GET /monitor/api/state", h.apiState)
	m.HandleFunc("GET /monitor/api/stream", h.apiStream)
	m.HandleFunc("GET /monitor/api/series", h.apiSeries)
	m.HandleFunc("GET /monitor/api/history", h.apiHistory)
	m.HandleFunc("GET /monitor/api/stats", h.apiStats)
	m.HandleFunc("GET /monitor/api/audit", h.apiAudit)
	m.HandleFunc("GET /monitor/api/jobs", h.apiJobs)
	m.HandleFunc("GET /monitor/api/jobs/export", h.apiJobsExport)
	m.HandleFunc("GET /monitor/api/jobs/{id}", h.apiJob)
	m.HandleFunc("GET /monitor/api/jobs/{id}/content", h.apiJobContent)
	m.HandleFunc("GET /monitor/api/session", h.apiSession)
	m.HandleFunc("POST /monitor/api/login", h.apiLogin)
	m.HandleFunc("POST /monitor/api/logout", h.apiLogout)
	m.HandleFunc("POST /monitor/api/queue/{id}/promote", h.action("promote"))
	m.HandleFunc("POST /monitor/api/queue/{id}/demote", h.action("demote"))
	m.HandleFunc("POST /monitor/api/queue/{id}/remove", h.action("remove"))
	m.HandleFunc("POST /monitor/api/running/{id}/cancel", h.action("cancel"))
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("X-Frame-Options", "DENY")
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Security-Policy", h.csp)
	hd.Set("Cache-Control", "no-store")
	_, _ = w.Write(indexHTML)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"encode"}`, http.StatusInternalServerError)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "application/json; charset=utf-8")
	hd.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

func jsonErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---- state ---------------------------------------------------------------------

// Cards are the numbers of the top row.
type Cards struct {
	Running      int     `json:"running"`
	Waiting      int     `json:"waiting"`
	DoneToday    int     `json:"done_today"`
	ErrorsToday  int     `json:"errors_today"`
	WaitP50MS    int64   `json:"wait_p50_ms"`
	WaitP95MS    int64   `json:"wait_p95_ms"`
	TokensPerSec float64 `json:"tokens_per_sec"`
	PerMinute    float64 `json:"per_minute"`
}

type extras struct {
	cards  Cards
	recent []store.Row
	events []store.Event
}

// State is the photo the page shows.
type State struct {
	Now       time.Time        `json:"now"`
	Version   string           `json:"version"`
	BootID    string           `json:"boot_id"`
	UptimeS   int64            `json:"uptime_s"`
	Queue     queue.Snapshot   `json:"queue"`
	GPU       *gpu.Snapshot    `json:"gpu,omitempty"`
	CPU       *cpu.Sample      `json:"cpu,omitempty"`
	Models    *models.Snapshot `json:"models,omitempty"`
	Cards     Cards            `json:"cards"`
	Recent    []store.Row      `json:"recent"`
	Events    []store.Event    `json:"events"`
	Jobs      []store.JobRow   `json:"jobs"` // Fase 10.L: the live job log, newest first
	JobsLimit int              `json:"jobs_limit"`
	// RecentDone: requests that ended in the last KeepDone, newest first. Shown
	// next to the running ones so a request that came and went in a blink can
	// still be seen; it takes no slot and counts in no limit.
	RecentDone   []store.JobRow `json:"recent_done"`
	KeepDoneMS   int64          `json:"keep_done_ms"`
	ContentOn    bool           `json:"content_enabled"`
	Dropped      int64          `json:"dropped"`
	AdminEnabled bool           `json:"admin_enabled"`
}

const extrasTTL = 2 * time.Second

// refreshExtras recomputes the database-backed part at most every extrasTTL,
// however many viewers there are.
func (h *Handler) refreshExtras(ctx context.Context) extras {
	h.extraMu.Lock()
	defer h.extraMu.Unlock()
	if time.Since(h.extraAt) < extrasTTL {
		return h.extra
	}
	var e extras
	e.recent, e.events = []store.Row{}, []store.Event{}
	if s := h.cfg.Store; s != nil {
		now := time.Now()
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if st, err := s.Stats(ctx, now.Sub(midnight), "hoje"); err == nil {
			e.cards.DoneToday, e.cards.ErrorsToday = st.OK, st.Errors
		}
		if st, err := s.Stats(ctx, 15*time.Minute, "15m"); err == nil {
			e.cards.WaitP50MS, e.cards.WaitP95MS = st.WaitP50, st.WaitP95
			e.cards.TokensPerSec, e.cards.PerMinute = st.TokensPerSec, st.PerMinute
		}
		if rows, err := s.History(ctx, store.HistoryQuery{Limit: 50}); err == nil {
			e.recent = coarseRows(rows) // open: no exact size (revisão 10.L, H2)
		}
		if ev, err := s.Events(ctx, 10); err == nil {
			e.events = ev
		}
	}
	h.extra, h.extraAt = e, time.Now()
	return e
}

func (h *Handler) state(ctx context.Context) State {
	snap := h.cfg.Queue.Snapshot()
	snap.Running, snap.Waiting = coarseItems(snap.Running), coarseItems(snap.Waiting) // open: no exact size (revisão 10.L, H2)
	ex := h.refreshExtras(ctx)
	st := State{
		Now: snap.Now, Version: h.cfg.Version, BootID: h.cfg.BootID,
		UptimeS: int64(time.Since(h.started).Seconds()),
		Queue:   snap, Cards: ex.cards, Recent: ex.recent, Events: ex.events,
		AdminEnabled: h.cfg.Admin != nil, Jobs: h.liveJobs(ctx), JobsLimit: liveJobsLimit, RecentDone: h.recentDone(ctx), KeepDoneMS: h.cfg.KeepDone.Milliseconds(), ContentOn: h.cfg.Content != nil,
	}
	st.Cards.Running, st.Cards.Waiting = len(snap.Running), len(snap.Waiting)
	if h.cfg.GPU != nil {
		g := h.cfg.GPU.Latest()
		st.GPU = &g
	}
	if h.cfg.CPU != nil {
		c := h.cfg.CPU.Latest()
		st.CPU = &c
	}
	if h.cfg.Models != nil {
		m := h.cfg.Models.Latest() // cached: the Ollama is queried by the collector, never here
		st.Models = &m
	}
	if h.cfg.Store != nil {
		st.Dropped = h.cfg.Store.Dropped()
	}
	return st
}

func (h *Handler) apiState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.state(r.Context()))
}

// sseEnter takes one stream slot for ip; false when the global or the per-IP
// ceiling is reached.
func (h *Handler) sseEnter(ip string) bool {
	h.sseMu.Lock()
	defer h.sseMu.Unlock()
	if h.sseTotal >= h.cfg.MaxSSE || h.sseByIP[ip] >= h.cfg.MaxSSEPerIP {
		return false
	}
	h.sseTotal++
	h.sseByIP[ip]++
	return true
}

func (h *Handler) sseLeave(ip string) {
	h.sseMu.Lock()
	defer h.sseMu.Unlock()
	h.sseTotal--
	if n := h.sseByIP[ip] - 1; n > 0 {
		h.sseByIP[ip] = n
	} else {
		delete(h.sseByIP, ip)
	}
}

func (h *Handler) apiStream(w http.ResponseWriter, r *http.Request) {
	ip := proxy.ClientIP(r)
	if !h.sseEnter(ip) {
		w.Header().Set("Retry-After", "5")
		jsonErr(w, http.StatusServiceUnavailable, "muitos painéis abertos")
		return
	}
	defer h.sseLeave(ip)
	fl, ok := w.(http.Flusher)
	if !ok {
		jsonErr(w, http.StatusInternalServerError, "streaming não suportado")
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	rc := http.NewResponseController(w)
	send := func() bool {
		b, err := json.Marshal(h.state(r.Context())) // compact: newlines in data are escaped
		if err != nil {
			return false
		}
		_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	ch := h.cfg.Hub.subscribe()
	defer h.cfg.Hub.unsubscribe(ch)
	if !send() {
		return
	}
	tick := time.NewTicker(h.cfg.StreamEvery)
	defer tick.Stop()
	last := time.Now()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.done:
			return
		case <-ch:
			if d := time.Since(last); d < 150*time.Millisecond { // debounce bursts
				time.Sleep(150*time.Millisecond - d)
			}
		case <-tick.C:
		}
		if !send() {
			return
		}
		last = time.Now()
	}
}

// ---- series (last hour, in memory) ------------------------------------------------

// Point is one 5 s sample of the charts.
type Point struct {
	T        int64   `json:"t"` // unix ms
	Waiting  int     `json:"waiting"`
	Running  int     `json:"running"`
	Finished int     `json:"finished"` // requests finished in the interval
	WaitAvg  float64 `json:"wait_avg_ms"`
}

type bucketAcc struct {
	n       int
	waitSum int64
}

type seriesRing struct {
	mu  sync.Mutex
	pts []Point
}

const (
	seriesEvery = 5 * time.Second
	seriesMax   = 720 // one hour
)

// Observe feeds the charts with a finished request (call from the queue's
// OnRecord, next to the store).
func (h *Handler) Observe(r queue.Record) {
	if r.Status == queue.StatusRejected || r.StartedAt.IsZero() {
		return
	}
	h.bucketMu.Lock()
	h.bucket.n++
	h.bucket.waitSum += r.WaitMS
	h.bucketMu.Unlock()
}

func (h *Handler) samplePoint(now time.Time) {
	h.bucketMu.Lock()
	b := h.bucket
	h.bucket = bucketAcc{}
	h.bucketMu.Unlock()
	snap := h.cfg.Queue.Snapshot()
	p := Point{T: now.UnixMilli(), Waiting: len(snap.Waiting), Running: len(snap.Running), Finished: b.n}
	if b.n > 0 {
		p.WaitAvg = float64(b.waitSum) / float64(b.n)
	}
	h.series.mu.Lock()
	h.series.pts = append(h.series.pts, p)
	if len(h.series.pts) > seriesMax {
		h.series.pts = h.series.pts[len(h.series.pts)-seriesMax:]
	}
	h.series.mu.Unlock()
}

// Start runs the background sampler until ctx ends.
func (h *Handler) Start(ctx context.Context) {
	t := time.NewTicker(seriesEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			h.samplePoint(now)
		}
	}
}

func (h *Handler) apiSeries(w http.ResponseWriter, r *http.Request) {
	h.series.mu.Lock()
	pts := append([]Point{}, h.series.pts...)
	h.series.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"every_s": int(seriesEvery / time.Second), "points": pts})
}

// ---- history, stats, audit (read only) ----------------------------------------------

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.UnixMilli(n), nil
	}
	return time.Parse(time.RFC3339, s)
}

func atoiDef(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return def
}

func (h *Handler) apiHistory(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Store == nil {
		jsonErr(w, http.StatusServiceUnavailable, "histórico desabilitado")
		return
	}
	q := r.URL.Query()
	since, err1 := parseTime(q.Get("desde"))
	until, err2 := parseTime(q.Get("ate"))
	if err1 != nil || err2 != nil {
		jsonErr(w, http.StatusBadRequest, "desde/ate: use RFC 3339 ou epoch em ms")
		return
	}
	rows, err := h.cfg.Store.History(r.Context(), store.HistoryQuery{
		Since: since, Until: until, Client: q.Get("cliente"), Model: q.Get("modelo"),
		Limit: atoiDef(q.Get("limite"), 100), Offset: atoiDef(q.Get("offset"), 0),
	})
	if err != nil {
		h.logf("monitor: history: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar o histórico")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": coarseRows(rows)})
}

var windows = map[string]time.Duration{"15m": 15 * time.Minute, "1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}

func (h *Handler) apiStats(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Store == nil {
		jsonErr(w, http.StatusServiceUnavailable, "histórico desabilitado")
		return
	}
	label := r.URL.Query().Get("janela")
	if label == "" {
		label = "15m"
	}
	d, ok := windows[label]
	if !ok {
		jsonErr(w, http.StatusBadRequest, "janela: 15m, 1h, 24h ou 7d")
		return
	}
	st, err := h.stats(r.Context(), d, label)
	if err != nil {
		h.logf("monitor: stats: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao calcular as estatísticas")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// statsTTL is how long a computed window is served from memory. GET
// /monitor/api/stats is open (D9-1) and a 7d window reads up to 200 thousand
// rows; the cache and the mutex make N parallel requests cost one computation
// (Fase 9.5, M8).
const statsTTL = 5 * time.Second

type statsEntry struct {
	at time.Time
	st store.Stats
}

func (h *Handler) stats(ctx context.Context, d time.Duration, label string) (store.Stats, error) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	if e, ok := h.statsCache[label]; ok && time.Since(e.at) < statsTTL {
		return e.st, nil
	}
	h.statsN.Add(1)
	st, err := h.cfg.Store.Stats(ctx, d, label)
	if err != nil {
		return st, err
	}
	h.statsCache[label] = statsEntry{at: time.Now(), st: st}
	return st, nil
}

// statsComputed counts the real Stats computations (tests).
func (h *Handler) statsComputed() int64 { return h.statsN.Load() }

func (h *Handler) apiAudit(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []store.AuditEntry{}})
		return
	}
	a, err := h.cfg.Store.Audits(r.Context(), atoiDef(r.URL.Query().Get("limite"), 50))
	if err != nil {
		h.logf("monitor: audit: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar a auditoria")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": a})
}

// ---- session, login ---------------------------------------------------------------------

func (h *Handler) current(r *http.Request) (string, *session) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", nil
	}
	return c.Value, h.sess.get(c.Value, time.Now())
}

func (h *Handler) apiSession(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"admin_enabled": h.cfg.Admin != nil, "logged_in": false}
	if _, s := h.current(r); s != nil {
		out["logged_in"], out["user"], out["csrf"] = true, s.user, s.csrf
	}
	writeJSON(w, http.StatusOK, out)
}

// sameSite rejects a cross-site POST: Sec-Fetch-Site must be same-origin (or
// none) when sent, and Origin, when sent, must name this host. The SameSite
// cookie and the CSRF token are the main defences; this is a second layer.
func sameSite(r *http.Request) bool {
	if v := r.Header.Get("Sec-Fetch-Site"); v != "" && v != "same-origin" && v != "none" {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || u.Hostname() == "" {
			return false
		}
		host := r.Host
		if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i:], "]") {
			host = host[:i]
		}
		if !strings.EqualFold(u.Hostname(), strings.Trim(host, "[]")) {
			return false
		}
	}
	return true
}

func readSmallJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		jsonErr(w, http.StatusRequestEntityTooLarge, "corpo grande demais")
		return false
	}
	if len(b) == 0 {
		return true // an action with no reason
	}
	// JSON only: a plain HTML form cannot send this content type cross-site.
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		jsonErr(w, http.StatusUnsupportedMediaType, "use Content-Type: application/json")
		return false
	}
	if err := json.Unmarshal(b, v); err != nil {
		jsonErr(w, http.StatusBadRequest, "JSON inválido")
		return false
	}
	return true
}

func (h *Handler) apiLogin(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Admin == nil {
		jsonErr(w, http.StatusServiceUnavailable, "administração desabilitada (--admin-file)")
		return
	}
	if !sameSite(r) {
		jsonErr(w, http.StatusForbidden, "origem não permitida")
		return
	}
	ip := proxy.ClientIP(r)
	var in struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if !readSmallJSON(w, r, &in) {
		return
	}
	// Reserve the attempt before the slow check (Fase 9.5, M1): it counts
	// against the limit while it runs.
	ok, retry := h.lim.tryBegin(ip, time.Now())
	if !ok {
		secs := int((retry + time.Second - 1) / time.Second)
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		jsonErr(w, http.StatusTooManyRequests, "muitas tentativas; aguarde")
		return
	}
	// At most verifySlots PBKDF2 evaluations at a time (CPU of the AI server).
	select {
	case h.verifySem <- struct{}{}:
	case <-r.Context().Done():
		h.lim.abort(ip)
		return
	case <-time.After(10 * time.Second):
		h.lim.abort(ip)
		w.Header().Set("Retry-After", "5")
		jsonErr(w, http.StatusServiceUnavailable, "verificação de senha ocupada; tente de novo")
		return
	}
	good := h.cfg.Admin.Verify(in.User, in.Password)
	<-h.verifySem
	now := time.Now()
	h.lim.end(ip, now, good)
	if !good {
		h.logf("monitor: login recusado de %s", ip)
		jsonErr(w, http.StatusUnauthorized, "usuário ou senha inválidos")
		return
	}
	token, s := h.sess.create(h.cfg.Admin.User, now)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/monitor", MaxAge: int(sessionTTL / time.Second),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	h.logf("monitor: login de %s a partir de %s", s.user, ip)
	writeJSON(w, http.StatusOK, map[string]any{"user": s.user, "csrf": s.csrf})
}

// authorize checks the session and the CSRF token of a POST.
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (string, *session, bool) {
	if h.cfg.Admin == nil {
		jsonErr(w, http.StatusServiceUnavailable, "administração desabilitada (--admin-file)")
		return "", nil, false
	}
	if !sameSite(r) {
		jsonErr(w, http.StatusForbidden, "origem não permitida")
		return "", nil, false
	}
	tok, s := h.current(r)
	if s == nil {
		jsonErr(w, http.StatusUnauthorized, "faça login como administrador")
		return "", nil, false
	}
	if got := r.Header.Get("X-CSRF-Token"); got == "" || !constEq(got, s.csrf) {
		jsonErr(w, http.StatusForbidden, "token CSRF ausente ou inválido")
		return "", nil, false
	}
	return tok, s, true
}

func (h *Handler) apiLogout(w http.ResponseWriter, r *http.Request) {
	tok, _, ok := h.authorize(w, r)
	if !ok {
		return
	}
	h.sess.drop(tok)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/monitor", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "saiu"})
}

var idRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// action builds the handler of one administrator action.
func (h *Handler) action(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, s, ok := h.authorize(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !idRe.MatchString(id) {
			jsonErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !readSmallJSON(w, r, &in) {
			return
		}
		reason := in.Reason
		if len([]rune(reason)) > 200 {
			reason = string([]rune(reason)[:200])
		}
		var res queue.ActionResult
		var err error
		switch kind {
		case "promote":
			res, err = h.cfg.Queue.Promote(id)
		case "demote":
			res, err = h.cfg.Queue.Demote(id)
		case "remove":
			res, err = h.cfg.Queue.Remove(id)
		case "cancel":
			res, err = h.cfg.Queue.Cancel(id)
		}
		if err != nil {
			if errors.Is(err, queue.ErrNotWaiting) || errors.Is(err, queue.ErrNotRunning) {
				jsonErr(w, http.StatusConflict, err.Error())
				return
			}
			jsonErr(w, http.StatusInternalServerError, "erro")
			return
		}
		out := map[string]any{"ok": true, "acao": kind, "id": id, "antes": res.Before, "depois": res.After}
		if h.cfg.Store != nil {
			if aerr := h.cfg.Store.AddAudit(store.AuditEntry{
				User: s.user, IP: proxy.ClientIP(r), Action: kind, RequestID: id, Before: res.Before, After: res.After, Reason: reason,
			}); aerr != nil {
				h.logf("monitor: auditoria falhou: %v", aerr)
				out["audit_error"] = "a ação foi feita, mas a auditoria não foi gravada"
			}
		}
		h.cfg.Hub.Notify()
		writeJSON(w, http.StatusOK, out)
	}
}
