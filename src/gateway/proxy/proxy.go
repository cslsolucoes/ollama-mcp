// Package proxy is the HTTP front of the gateway (Fase 9.2): it puts the
// generation and embedding calls in the queue and forwards them to the Ollama
// on the loopback. Everything else of the Ollama API is either forwarded
// straight (read-only endpoints) or refused.
//
// Privacy: the body is read to apply the request policies (policy.go: dangerous
// options removed, reasoning decided, base model name) and forwarded with
// those keys changed and everything else as it came; only counters (never
// text) are kept from the response, and only from the last chunk.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// Request headers set by mcpollama (all optional).
const (
	HeaderClient   = "X-Mcpollama-Client"
	HeaderTool     = "X-Mcpollama-Tool"
	HeaderPriority = "X-Mcpollama-Priority"
)

// Response headers set by the gateway.
const (
	HeaderRequestID     = "X-Request-Id"
	HeaderQueueMS       = "X-Queue-Ms"
	HeaderQueuePosition = "X-Queue-Position"
)

const (
	defaultMaxBody = 32 << 20
	tailBytes      = 16 << 10
	maxLabelLen    = 64
	maxToolLen     = 32
	invalidLabel   = "invalido"
)

var (
	labelRe = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]{1,64}$`)
	toolRe  = regexp.MustCompile(`^[a-z0-9_.-]{1,32}$`)
)

// Config configures the Handler.
type Config struct {
	Upstream      *url.URL // the Ollama, e.g. http://127.0.0.1:11434
	Queue         *queue.Manager
	PriorityNets  []*net.IPNet // IPs allowed to ask for priority alta
	AllowedModels []string     // empty means any model
	MaxBodyBytes  int64        // request body ceiling, default 32 MiB
	// RunTimeout bounds how long a started request may hold its slot (Fase
	// 9.5, P6): an Ollama that never answers or a stream that never ends is
	// cut, the slot comes back and the client gets a clear error. Zero means
	// the queue MaxWait (the entrypoint flag --run-timeout defaults to
	// --max-wait, 10 min).
	RunTimeout time.Duration

	// Request policies (Fase 9.6a), applied before queuing. See policy.go.
	DefaultThink         bool     // reasoning when the client does not say (false = off)
	ThinkSuffix          string   // virtual name "<model><suffix>" turns reasoning on; "" disables
	AllowClientKeepAlive bool     // let the client's top-level keep_alive through
	AllowClientNumCtx    bool     // let the client's options.num_ctx through
	StripOptions         []string // options removed from "options" (no defaulting here; see DefaultStripOptions)

	Logf func(format string, args ...any)
}

// ErrRunTimeout is the cause of a request cut by RunTimeout.
var ErrRunTimeout = errors.New("tempo máximo de execução no servidor esgotado")

// Handler is the gateway front. Safe for concurrent use.
type Handler struct {
	cfg   Config
	pol   policyCfg
	proxy *httputil.ReverseProxy
}

type route struct {
	lane   queue.Lane
	queued bool
	direct bool
}

var routes = map[string]route{
	"/api/generate":        {lane: queue.LaneGen, queued: true},
	"/api/chat":            {lane: queue.LaneGen, queued: true},
	"/api/embed":           {lane: queue.LaneEmbed, queued: true},
	"/api/embeddings":      {lane: queue.LaneEmbed, queued: true},
	"/v1/chat/completions": {lane: queue.LaneGen, queued: true},
	"/v1/completions":      {lane: queue.LaneGen, queued: true},
	"/v1/embeddings":       {lane: queue.LaneEmbed, queued: true},
	"/api/tags":            {direct: true},
	"/api/ps":              {direct: true},
	"/api/show":            {direct: true},
	"/api/version":         {direct: true},
	"/v1/models":           {direct: true},
}

var blockedPrefixes = []string{"/api/pull", "/api/push", "/api/create", "/api/delete", "/api/copy", "/api/blobs"}

type ctxKey struct{}

// exchange is the per-request state shared with the ReverseProxy callbacks.
type exchange struct {
	status  int
	body    *countingBody
	proxyEr error
	runCtx  context.Context
}

// New builds the Handler.
func New(cfg Config) (*Handler, error) {
	if cfg.Upstream == nil || cfg.Upstream.Host == "" {
		return nil, errors.New("proxy: upstream is required")
	}
	if cfg.Queue == nil {
		return nil, errors.New("proxy: queue is required")
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBody
	}
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = cfg.Queue.Config().MaxWait
	}
	h := &Handler{cfg: cfg, pol: newPolicyCfg(cfg)}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil // the Ollama is on the loopback; never go through an environment proxy
	tr.MaxIdleConnsPerHost = 16
	h.proxy = &httputil.ReverseProxy{
		FlushInterval: -1,
		Transport:     tr,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(cfg.Upstream)
			pr.Out.Host = cfg.Upstream.Host
			for _, k := range []string{"X-Real-Ip", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded",
				"Authorization", "Cookie", HeaderClient, HeaderTool, HeaderPriority, HeaderThink, HeaderRequestID} {
				pr.Out.Header.Del(k)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			ex, _ := resp.Request.Context().Value(ctxKey{}).(*exchange)
			if ex == nil {
				return nil
			}
			ex.status = resp.StatusCode
			ex.body = &countingBody{rc: resp.Body, ctx: resp.Request.Context()}
			resp.Body = ex.body
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			ex, _ := r.Context().Value(ctxKey{}).(*exchange)
			if ex != nil {
				ex.proxyEr = err
			}
			switch {
			case queue.IsAdminCancel(r.Context()):
				writeJSON(w, http.StatusConflict, "cancelado pelo administrador")
			case isRunTimeout(r.Context()):
				writeJSON(w, http.StatusGatewayTimeout, ErrRunTimeout.Error())
			case r.Context().Err() != nil:
				// The client is gone: nobody to answer.
			default:
				writeJSON(w, http.StatusBadGateway, "Ollama indisponível")
			}
		},
		ErrorLog: log.New(io.Discard, "", 0),
	}
	return h, nil
}

func (h *Handler) logf(format string, a ...any) {
	if h.cfg.Logf != nil {
		h.cfg.Logf(format, a...)
	}
}

func writeJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(append(b, '\n'))
}

// clientIP trusts X-Real-IP only when the peer is the local nginx.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer != nil && peer.IsLoopback() {
		if v := r.Header.Values("X-Real-Ip"); len(v) == 1 {
			if ip := net.ParseIP(strings.TrimSpace(v[0])); ip != nil {
				return ip.String()
			}
		}
	}
	if peer != nil {
		return peer.String()
	}
	return host
}

// ClientIP is exported for the monitor, which applies the same rule.
func ClientIP(r *http.Request) string { return clientIP(r) }

func cleanLabel(v string) string {
	if v == "" {
		return ""
	}
	if !labelRe.MatchString(v) {
		return invalidLabel
	}
	return v
}

func cleanTool(v string) string {
	if v == "" {
		return ""
	}
	if !toolRe.MatchString(v) {
		return "invalido"
	}
	return v
}

func (h *Handler) priorityFor(ip, header string) (in, final queue.Priority) {
	in = queue.Normal
	if p, ok := queue.ParsePriority(header); ok {
		in = p
	}
	final = in
	if in == queue.High && !h.priorityAllowed(ip) {
		final = queue.Normal // ignored in silence (D9-8)
	}
	return in, final
}

func (h *Handler) priorityAllowed(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	for _, n := range h.cfg.PriorityNets {
		if n.Contains(p) {
			return true
		}
	}
	return false
}

func (h *Handler) modelAllowed(model string) bool {
	if len(h.cfg.AllowedModels) == 0 {
		return true
	}
	n := strings.TrimSuffix(model, ":latest")
	for _, m := range h.cfg.AllowedModels {
		if strings.TrimSuffix(m, ":latest") == n {
			return true
		}
	}
	return false
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := queue.NewID()
	w.Header().Set(HeaderRequestID, id)
	path := r.URL.Path

	for _, p := range blockedPrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			writeJSON(w, http.StatusForbidden, "endpoint administrativo bloqueado")
			return
		}
	}
	rt, ok := routes[path]
	if !ok {
		writeJSON(w, http.StatusNotFound, "endpoint desconhecido")
		return
	}
	if rt.direct {
		if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodHead {
			writeJSON(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}
		h.proxy.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.cfg.MaxBodyBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSON(w, http.StatusRequestEntityTooLarge, "corpo da requisição grande demais")
		}
		return // otherwise the client is gone
	}
	bytesIn := int64(len(body))
	pol, perr := h.pol.applyPolicy(path, body, r.Header.Get(HeaderThink))
	if perr != nil {
		writeJSON(w, perr.status, perr.msg)
		return
	}
	// The allowlist (D9-9) checks the BASE name, after the "-think" rewrite.
	if len(h.cfg.AllowedModels) > 0 && !h.modelAllowed(pol.model) {
		writeJSON(w, http.StatusForbidden, fmt.Sprintf("modelo %q não permitido neste servidor", sanitizeForMsg(pol.model)))
		return
	}
	body = pol.body

	ip := clientIP(r)
	pin, pfinal := h.priorityFor(ip, r.Header.Get(HeaderPriority))
	qreq := queue.Request{
		ID: id, Lane: rt.lane, ClientIP: fairIP(ip),
		ClientLabel: cleanLabel(r.Header.Get(HeaderClient)),
		Tool:        cleanTool(r.Header.Get(HeaderTool)),
		Endpoint:    path, Model: truncate(pol.model, 96), Think: pol.think,
		PriorityIn: pin, PriorityFinal: pfinal, BytesIn: bytesIn,
	}
	if qreq.ClientLabel == "" {
		qreq.ClientLabel = "anonimo"
	}

	tk, err := h.cfg.Queue.Acquire(r.Context(), qreq)
	if err != nil {
		h.rejectAcquire(w, r, err)
		return
	}
	runCtx, cancelRun := context.WithTimeoutCause(tk.Ctx, h.cfg.RunTimeout, ErrRunTimeout)
	ex := &exchange{runCtx: runCtx}
	defer func() {
		// Also runs when ReverseProxy aborts the handler (client gone mid-stream).
		tk.Release(h.result(r.Context(), tk, ex))
	}()
	defer cancelRun() // runs first: the cause stays ErrRunTimeout only if the deadline hit
	w.Header().Set(HeaderQueueMS, strconv.FormatInt(tk.Waited.Milliseconds(), 10))
	w.Header().Set(HeaderQueuePosition, strconv.Itoa(tk.Position))

	ctx := context.WithValue(runCtx, ctxKey{}, ex)
	out := r.WithContext(ctx)
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.ContentLength = int64(len(body))
	out.TransferEncoding = nil
	h.proxy.ServeHTTP(w, out)
}

func (h *Handler) rejectAcquire(w http.ResponseWriter, r *http.Request, err error) {
	var fe *queue.FullError
	switch {
	case errors.As(err, &fe):
		secs := int(fe.RetryAfter.Round(time.Second) / time.Second)
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusServiceUnavailable, "fila do servidor cheia ("+fe.Error()+"); tente em "+strconv.Itoa(secs)+" s")
	case errors.Is(err, queue.ErrWaitTimeout):
		writeJSON(w, http.StatusGatewayTimeout, "tempo máximo de espera na fila esgotado")
	case errors.Is(err, queue.ErrRemoved):
		writeJSON(w, http.StatusConflict, "removido da fila pelo administrador")
	default:
		// The client left (context cancelled): nothing to write.
	}
}

// result classifies how the exchange ended.
func (h *Handler) result(reqCtx context.Context, tk *queue.Ticket, ex *exchange) queue.Result {
	res := queue.Result{HTTPStatus: ex.status}
	var c counters
	if ex.body != nil {
		c = parseCounters(ex.body.tail())
		res.PromptEval, res.Eval, res.LoadMS = c.promptEval, c.eval, c.loadNS/1e6
	}
	complete := ex.body != nil && ex.body.eof
	switch {
	case queue.IsAdminCancel(tk.Ctx):
		res.Status = queue.StatusAdminCanceled
		res.ErrorClass = "admin_cancel"
	case ex.status >= 200 && ex.status < 300 && complete && !c.errorLine:
		res.Status = queue.StatusOK // finished before the run timeout could matter
	case ex.status >= 200 && ex.status < 300 && c.errorLine:
		res.Status, res.ErrorClass = queue.StatusError, "stream_error"
	case ex.runCtx != nil && isRunTimeout(ex.runCtx):
		res.Status, res.ErrorClass = queue.StatusError, "run_timeout"
		if res.HTTPStatus == 0 {
			res.HTTPStatus = http.StatusGatewayTimeout
		}
	case reqCtx.Err() != nil:
		res.Status, res.ErrorClass = queue.StatusCanceled, "client_gone"
		if res.HTTPStatus == 0 || res.HTTPStatus < 400 {
			res.HTTPStatus = 499
		}
	case ex.proxyEr != nil:
		res.Status, res.ErrorClass, res.HTTPStatus = queue.StatusError, "upstream_unreachable", 502
	case ex.status >= 400:
		res.Status, res.ErrorClass = queue.StatusError, "upstream_"+strconv.Itoa(ex.status)
	case ex.body != nil && !complete && ex.body.readErr == nil:
		// The write to the client failed before the end of the body.
		res.Status, res.ErrorClass = queue.StatusCanceled, "client_gone"
		res.HTTPStatus = 499
	default:
		res.Status, res.ErrorClass = queue.StatusError, "upstream_dropped"
		if res.HTTPStatus == 0 {
			res.HTTPStatus = 502
		}
	}
	h.logf("req %s %s %s status=%d class=%s wait_ms=%d", tk.ID, tk.Req().Lane, tk.Req().Endpoint, res.HTTPStatus, res.ErrorClass, tk.Waited.Milliseconds())
	return res
}

func isRunTimeout(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrRunTimeout)
}

// fairIP is the key of the per-IP limits and of the fairness. An IPv6 client
// usually owns a whole /64 and can rotate its address inside it, so IPv6
// addresses are grouped by /64 (Fase 9.5, P2). IPv4 (also IPv4-mapped) stays
// as is.
func fairIP(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.Unmap()
	if a.Is4() {
		return a.String()
	}
	p, err := a.WithZone("").Prefix(64)
	if err != nil {
		return ip
	}
	return p.String()
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// sanitizeForMsg keeps a client-supplied model name printable and short before
// it goes back into an error message.
func sanitizeForMsg(s string) string {
	s = truncate(s, 64)
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, s)
}

// ---- response body tee -----------------------------------------------------

// countingBody forwards the upstream body, remembers whether it ended cleanly
// and keeps only the last tailBytes bytes (to read the final chunk counters).
// When the administrator cancels a running request mid-stream, it ends the
// body with an NDJSON error line so the client gets a clear message instead of
// a cut connection.
type countingBody struct {
	rc      io.ReadCloser
	ctx     context.Context
	buf     []byte
	eof     bool
	readErr error
	synth   []byte
	done    bool
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.synth != nil {
		n := copy(p, b.synth)
		b.synth = b.synth[n:]
		if len(b.synth) == 0 {
			b.synth = nil
			b.done = true
		}
		return n, nil
	}
	if b.done {
		return 0, io.EOF
	}
	n, err := b.rc.Read(p)
	if n > 0 {
		b.buf = append(b.buf, p[:n]...)
		if len(b.buf) > 2*tailBytes {
			b.buf = append(b.buf[:0], b.buf[len(b.buf)-tailBytes:]...)
		}
	}
	if err == io.EOF {
		b.eof = true
	} else if err != nil {
		b.readErr = err
		if msg := cutMessage(b.ctx); msg != "" {
			line, _ := json.Marshal(map[string]string{"error": msg})
			b.synth = append(append([]byte("\n"), line...), '\n')
			n2 := copy(p[n:], b.synth)
			b.synth = b.synth[n2:]
			if len(b.synth) == 0 {
				b.synth, b.done = nil, true
			}
			return n + n2, nil
		}
	}
	return n, err
}

func (b *countingBody) Close() error { return b.rc.Close() }

// cutMessage is the NDJSON error closing a stream the gateway cut on purpose.
func cutMessage(ctx context.Context) string {
	switch {
	case queue.IsAdminCancel(ctx):
		return "cancelado pelo administrador"
	case isRunTimeout(ctx):
		return ErrRunTimeout.Error()
	}
	return ""
}

func (b *countingBody) tail() []byte { return b.buf }

// ---- counters ----------------------------------------------------------------

type counters struct {
	promptEval, eval, loadNS, totalNS int64
	errorLine                         bool
}

var (
	reNum = func(name string) *regexp.Regexp { return regexp.MustCompile(`"` + name + `"\s*:\s*(\d+)`) }
	rePE  = reNum("prompt_eval_count")
	reE   = reNum("eval_count")
	reLD  = reNum("load_duration")
	reTD  = reNum("total_duration")
	rePT  = reNum("prompt_tokens")
	reCT  = reNum("completion_tokens")
)

func lastNum(re *regexp.Regexp, b []byte) (int64, bool) {
	m := re.FindAllSubmatch(b, -1)
	if len(m) == 0 {
		return 0, false
	}
	v, err := strconv.ParseInt(string(m[len(m)-1][1]), 10, 64)
	return v, err == nil
}

// parseCounters reads the numbers of the LAST line of the body (the final
// chunk with done:true, or the whole non-stream JSON). Escaped quotes inside
// model text cannot match: the patterns need a bare quote before the name.
func parseCounters(tail []byte) counters {
	t := bytes.TrimRight(tail, " \r\n\t")
	for bytes.HasSuffix(t, []byte("[DONE]")) {
		t = bytes.TrimRight(bytes.TrimSuffix(t, []byte("[DONE]")), " \r\n\t")
		t = bytes.TrimSuffix(t, []byte("data:"))
		t = bytes.TrimRight(t, " \r\n\t")
	}
	if i := bytes.LastIndexByte(t, '\n'); i >= 0 {
		t = t[i+1:]
	}
	var c counters
	if bytes.HasPrefix(bytes.TrimSpace(t), []byte(`{"error"`)) {
		c.errorLine = true
		return c
	}
	if v, ok := lastNum(rePE, t); ok {
		c.promptEval = v
	} else if v, ok := lastNum(rePT, t); ok {
		c.promptEval = v
	}
	if v, ok := lastNum(reE, t); ok {
		c.eval = v
	} else if v, ok := lastNum(reCT, t); ok {
		c.eval = v
	}
	c.loadNS, _ = lastNum(reLD, t)
	c.totalNS, _ = lastNum(reTD, t)
	return c
}
