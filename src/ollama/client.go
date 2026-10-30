// Package ollama is the HTTP client of the self-hosted Ollama (native
// /api/* endpoints). It has no tool logic and makes no call at construction.
//
// Design decisions (each one has a test):
//   - Every request carries a context.Context; the per-call deadline (Timeout)
//     is applied with context.WithTimeout. There is no http.Client.Timeout,
//     which would cut a stream in the middle (OLL-002).
//   - The token travels only in the Authorization header (SEC-002). URL
//     userinfo and query are stripped from the request URL.
//   - Every message that leaves this package is scrubbed: token and URL
//     credentials by exact value, URLs reduced to scheme+host+path (SEC-003/4).
//   - Admin endpoints (pull, push, create, delete, copy, blobs) have no code
//     here (SEC-012).
//   - There is no automatic retry: resending a generation would duplicate load
//     on the single shared GPU queue (OLL-014). The one exception is a 503
//     with Retry-After from the server queue (gateway): the request never ran,
//     so it is retried once, and only if the call deadline leaves room.
package ollama

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/redact"
)

const (
	defaultTimeout        = 120 * time.Second // above the 32-36 s cold load
	defaultMaxPromptChars = 128000
	defaultMaxBodyBytes   = 16 << 20
	defaultMaxLineBytes   = 8 << 20
	maxErrBodyBytes       = 4096 // SEC-005
	maxDetailRunes        = 200
	userAgent             = "mcpollama-mcp"
)

// Options configures the client. Zero values take the defaults above.
type Options struct {
	BaseURL  string // required; http(s) URL, trailing slash ignored
	APIKey   string // Bearer token; empty means no Authorization header
	TLSCA    string // extra CA bundle (PEM) added to the system pool
	Insecure bool   // skip TLS verification (the server logs the WARNING at start)

	// Fallback (optional, off by default): a second server tried ONCE when the
	// primary cannot be reached (connection error) or answers 503 "queue full".
	// The token and the CA are its own; --insecure never applies to it.
	FallbackURL    string
	FallbackAPIKey string
	FallbackTLSCA  string

	Model          string // default generation model
	EmbedModel     string // default embeddings model
	Timeout        time.Duration
	MaxPromptChars int // prompt ceiling in characters (PERF-004)
	MaxBodyBytes   int // ceiling of a non-stream response body (OLL-015)
	MaxLineBytes   int // ceiling of one NDJSON line (OLL-015)

	// ClientLabel names this agent to the server queue (X-Mcpollama-Client);
	// metadata only, never content. Empty omits the header.
	ClientLabel string

	// DisableStream switches generate/chat to a single "stream":false
	// response. The default is the internal stream (see Generate).
	DisableStream bool

	// Logf, when set, receives one scrubbed debug line per HTTP exchange:
	// method, path, status, duration and sizes only, never bodies.
	Logf func(format string, args ...any)
}

// Client talks to one Ollama server. It is safe for concurrent use.
type Client struct {
	base       *url.URL
	key        string
	model      string
	embedModel string
	timeout    time.Duration
	maxPrompt  int
	maxBody    int
	maxLine    int
	noStream   bool
	label      string

	http   *http.Client
	redact func(string) string
	logf   func(string, ...any)

	fb *Client // fallback server, nil when not configured (its own fb is always nil)
}

// New validates the options and builds the client. It performs no network I/O.
func New(o Options) (*Client, error) {
	if strings.TrimSpace(o.BaseURL) == "" {
		return nil, errors.New("configure --ollama-url (OLLAMA_HOST)")
	}
	u, err := url.Parse(strings.TrimSpace(o.BaseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("a URL do Ollama deve ser http(s)://host[:porta]")
	}

	// Credentials embedded in the URL are secrets too, and are never sent:
	// the only credential on the wire is the Bearer header.
	secrets := []string{o.APIKey, o.FallbackAPIKey}
	if u.User != nil {
		secrets = append(secrets, u.User.Username())
		if pw, ok := u.User.Password(); ok {
			secrets = append(secrets, pw)
		}
	}
	for _, v := range u.Query() {
		secrets = append(secrets, v...)
	}
	base := *u
	base.User, base.RawQuery, base.Fragment, base.ForceQuery = nil, "", "", false
	base.Path = strings.TrimRight(base.Path, "/")
	base.RawPath = ""

	if o.APIKey != "" && base.Scheme == "http" && !isLoopback(base.Hostname()) {
		return nil, errors.New("recusado: o token Bearer não é enviado por http:// para um host que não é loopback; use https://")
	}

	tr, err := newTransport(o.Insecure, o.TLSCA)
	if err != nil {
		return nil, err
	}
	c := &Client{
		base:       &base,
		key:        o.APIKey,
		model:      o.Model,
		embedModel: o.EmbedModel,
		timeout:    orDur(o.Timeout, defaultTimeout),
		maxPrompt:  orInt(o.MaxPromptChars, defaultMaxPromptChars),
		maxBody:    orInt(o.MaxBodyBytes, defaultMaxBodyBytes),
		maxLine:    orInt(o.MaxLineBytes, defaultMaxLineBytes),
		noStream:   o.DisableStream,
		label:      cleanHeader(o.ClientLabel, 64, labelOK),
		redact:     redact.New(secrets...),
		http: &http.Client{
			Transport: tr,
			// A redirect could carry the request somewhere else; never follow.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	if o.Logf != nil {
		logf := o.Logf
		c.logf = func(format string, a ...any) { logf("%s", c.scrub(fmt.Sprintf(format, a...))) }
	}
	if strings.TrimSpace(o.FallbackURL) != "" {
		fo := o
		fo.BaseURL, fo.APIKey, fo.TLSCA = o.FallbackURL, o.FallbackAPIKey, o.FallbackTLSCA
		fo.Insecure = false
		fo.FallbackURL, fo.FallbackAPIKey, fo.FallbackTLSCA = "", "", ""
		fb, err := New(fo)
		if err != nil {
			return nil, fmt.Errorf("fallback: %w", err)
		}
		c.fb = fb
	}
	return c, nil
}

// Close releases idle keep-alive connections.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
	if c.fb != nil {
		c.fb.Close()
	}
}

func orDur(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

func orInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newTransport clones http.DefaultTransport (proxy from environment, dial and
// idle defaults, HTTP/2) and sets only the TLS configuration (SEC-006).
func newTransport(insecure bool, caPath string) (*http.Transport, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	switch {
	case insecure:
		cfg.InsecureSkipVerify = true // opt-in; with --insecure the CA is ignored
	case caPath != "":
		pem, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("--tls-ca: não foi possível ler %s", caPath)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("--tls-ca: nenhum certificado PEM em %s", caPath)
		}
		cfg.RootCAs = pool
	}
	tr.TLSClientConfig = cfg
	return tr, nil
}

// NewTLSTransport is the transport the client uses for an https endpoint: the
// system pool plus the PEM bundle at caPath (empty: system pool only). The
// gateway pool (Fase 10.4) reuses it for its secondary backends, so there is
// one TLS policy in the repository. Redirects and the environment proxy are the
// caller's business.
func NewTLSTransport(caPath string) (*http.Transport, error) { return newTransport(false, caPath) }

func (c *Client) debugf(format string, a ...any) {
	if c.logf != nil {
		c.logf(format, a...)
	}
}

type toolKey struct{}

// WithTool tags the calls made with ctx with the MCP tool name (header
// X-Mcpollama-Tool). Metadata only.
func WithTool(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, toolKey{}, name)
}

func toolFrom(ctx context.Context) string {
	n, _ := ctx.Value(toolKey{}).(string)
	return cleanHeader(n, 32, toolOK)
}

func labelOK(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:/@+-", r)
}

func toolOK(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-'
}

// cleanHeader maps every character outside the allowed set to '_' and cuts
// the value, so a hostname with odd characters still makes a valid label.
func cleanHeader(v string, max int, ok func(rune) bool) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	out := make([]rune, 0, len(v))
	for _, r := range v {
		if !ok(r) {
			r = '_'
		}
		out = append(out, r)
		if len(out) == max {
			break
		}
	}
	return string(out)
}

// queueMS reads the server-side queue wait from the gateway header.
func queueMS(resp *http.Response) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("X-Queue-Ms")), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
