// Package pool is the backend pool of the gateway (Fase 10.4): the primary
// Ollama (--ollama) plus the optional secondaries of the registry file
// (--backends). It owns the health of each backend (active checks and passive
// results), the circuit breaker, the overload state of the primary and the
// eligibility rules for handing a request that has not started to a secondary.
//
// The queue (package queue) stays the only truth about who waits and who runs;
// it asks this package three things: may backend X take a start now (CanStart),
// is the primary overloaded (Evaluate) and may a waiting request run on
// secondary Y (Eligible). Lock order is queue.mu then pool.mu; the pool never
// calls the queue while holding its lock.
//
// Without --backends the pool is one backend, "gpu-local", and none of the
// rules apply: the gateway behaves as it did before the pool existed.
package pool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/ollama"
)

// DefaultPrimaryID is the id of the primary Ollama (--ollama).
const DefaultPrimaryID = "gpu-local"

// Kind of a backend; it enters the routing rules.
type Kind string

const (
	KindGPU    Kind = "gpu"
	KindCPU    Kind = "cpu"
	KindRemote Kind = "remoto"
)

// Priority classes as the queue names them.
const (
	ClassHigh   = "alta"
	ClassNormal = "normal"
	ClassLow    = "baixa"
)

const (
	maxRegistryBytes = 1 << 20
	maxBackends      = 8
	maxTokenBytes    = 4096
)

var (
	idRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)
	tokenRe = regexp.MustCompile(`^[\x21-\x7e]+$`)
)

// IDValid tells whether s is a well-formed backend id (the monitor uses it to
// validate the path of the drain actions).
func IDValid(s string) bool { return idRe.MatchString(s) }

// Spec is one entry of the registry file, as the root wrote it.
type Spec struct {
	ID              string   `json:"id"`
	URL             string   `json:"url"`
	Tipo            string   `json:"tipo"`
	TokenFile       string   `json:"token_file"`
	TLSCA           string   `json:"tls_ca"`
	Slots           int      `json:"slots"`
	Modelos         []string `json:"modelos"`
	Classes         []string `json:"classes"`
	MaxPromptTokens int      `json:"max_prompt_tokens"`
	Ordem           int      `json:"ordem"`
}

// registryFile is the shape of backends.json.
type registryFile struct {
	Backends []Spec `json:"backends"`
}

// LoadFile reads and validates the registry file. On Unix the file must not be
// readable by group or others (0600), as with --admin-file; otherwise the start
// is refused. The URL of every backend comes from this file only: no request,
// header or monitor field can add or change one (no SSRF by the client).
func LoadFile(path string) ([]Spec, error) {
	if err := checkPerm(path, "--backends"); err != nil {
		return nil, err
	}
	b, err := readLimited(path, maxRegistryBytes)
	if err != nil {
		return nil, fmt.Errorf("--backends: %w", err)
	}
	var f registryFile
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("--backends %s: JSON inválido: %v", path, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("--backends %s: texto depois do JSON", path)
	}
	if len(f.Backends) > maxBackends {
		return nil, fmt.Errorf("--backends: no máximo %d backends", maxBackends)
	}
	seen := map[string]bool{DefaultPrimaryID: true}
	for i := range f.Backends {
		s := &f.Backends[i]
		if err := s.normalize(); err != nil {
			return nil, fmt.Errorf("--backends: backend %d (%q): %w", i+1, s.ID, err)
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("--backends: id %q repetido ou reservado (%s é o primário)", s.ID, DefaultPrimaryID)
		}
		seen[s.ID] = true
	}
	return f.Backends, nil
}

func checkPerm(path, flagName string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", flagName, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s %s não é um arquivo comum", flagName, path)
	}
	return CheckMode(path, flagName, fi.Mode(), runtime.GOOS)
}

// CheckMode refuses a file readable by group or others. Windows has no such
// mode bits, so the check is a Unix one; goos is a parameter so a test can
// exercise the Unix rule on any host.
func CheckMode(path, flagName string, mode os.FileMode, goos string) error {
	if goos != "windows" && mode.Perm()&0o077 != 0 {
		return fmt.Errorf("%s %s tem permissão %v: use 0600", flagName, path, mode.Perm())
	}
	return nil
}

func readLimited(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("arquivo grande demais")
	}
	return b, nil
}

func (s *Spec) normalize() error {
	s.ID = strings.TrimSpace(s.ID)
	if !idRe.MatchString(s.ID) {
		return errors.New("id inválido (a-z, 0-9, '.', '_', '-'; até 32)")
	}
	switch Kind(s.Tipo) {
	case KindGPU, KindCPU, KindRemote:
	default:
		return errors.New(`tipo deve ser "gpu", "cpu" ou "remoto"`)
	}
	u, err := parseURL(s.URL)
	if err != nil {
		return err
	}
	if s.TokenFile != "" && u.Scheme != "https" && !isLoopbackHost(u.Hostname()) {
		return errors.New("token_file só com https:// (ou loopback): o token não vai em texto puro pela rede")
	}
	if s.TLSCA != "" && u.Scheme != "https" {
		return errors.New("tls_ca só vale com https://")
	}
	if s.Slots == 0 {
		s.Slots = 1
	}
	if s.Slots < 1 || s.Slots > 64 {
		return errors.New("slots deve ficar entre 1 e 64")
	}
	if s.MaxPromptTokens < 0 {
		return errors.New("max_prompt_tokens não pode ser negativo")
	}
	if s.Ordem < 0 {
		return errors.New("ordem não pode ser negativa")
	}
	for _, c := range s.Classes {
		if c != ClassHigh && c != ClassNormal && c != ClassLow {
			return fmt.Errorf("classe %q desconhecida (alta, normal, baixa)", c)
		}
	}
	for _, m := range s.Modelos {
		if strings.TrimSpace(m) == "" || len(m) > 96 || strings.ContainsAny(m, "\r\n\x00") {
			return errors.New("modelos: nome inválido")
		}
	}
	return nil
}

// parseURL accepts http(s)://host[:port] with an empty (or "/") path, no
// credentials, query or fragment, and refuses the addresses a registry file has
// no business pointing at (the unspecified address and link-local ranges, where
// cloud metadata services live).
func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, errors.New("url deve ser http(s)://host[:porta]")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("url sem credenciais, caminho, query ou fragmento")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()) {
		return nil, errors.New("url aponta para um endereço não permitido")
	}
	u.Path, u.RawPath = "", ""
	return u, nil
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// Backend is one Ollama of the pool. The exported fields never change after
// the pool is built; the runtime state is private and guarded by the Pool.
type Backend struct {
	Spec
	URL     *url.URL
	Primary bool

	// Transport is what the proxy uses to reach this backend (nil for the
	// primary: the proxy keeps its own loopback transport). It carries the TLS
	// policy only; the proxy sets the Authorization header (Token) itself.
	Transport http.RoundTripper

	token string // never logged, never in any String()
	col   *models.Collector

	// runtime state, guarded by Pool.mu
	drained     bool
	fails       int
	failActive  bool // the last failure was an active check
	br          breakerState
	openedAt    time.Time
	probing     bool
	checked     bool
	checkFailed bool
	lastErr     string
	lastCheck   time.Time
	version     string
	degraded    bool
	installed   map[string]string // normalized model name -> digest
	loaded      []models.Loaded
	ring        [ringSize]int64
	ringN       int
	ewma        float64 // ms, generation runs that ended ok (primary only)
}

// Token returns the bearer token of this backend ("" when it has none). It is
// the only way to read it; callers must not log it.
func (b *Backend) Token() string { return b.token }

// String names the backend without anything secret.
func (b *Backend) String() string { return b.ID }

// Host is host[:port] of the backend, for display.
func (b *Backend) Host() string {
	if b.URL == nil {
		return ""
	}
	return b.URL.Host
}

// bearerRT adds the backend token to the requests of the health checks.
type bearerRT struct {
	rt    http.RoundTripper
	token string
}

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	if b.token != "" {
		r2.Header.Set("Authorization", "Bearer "+b.token)
	}
	return b.rt.RoundTrip(r2)
}

// Build turns the validated specs into backends: reads the token files (0600),
// builds the TLS transport (system pool plus the CA, the same code as the
// mcpollama client) and the collector that polls /api/ps and /api/tags. allowed
// is the gateway --allowed-models list.
func Build(specs []Spec, allowed []string, think string, timeout time.Duration, logf func(string, ...any)) ([]*Backend, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	out := make([]*Backend, 0, len(specs))
	for _, s := range specs {
		if err := s.normalize(); err != nil {
			return nil, fmt.Errorf("backend %q: %w", s.ID, err)
		}
		u, _ := parseURL(s.URL)
		b := &Backend{Spec: s, URL: u}
		if s.TokenFile != "" {
			if err := checkPerm(s.TokenFile, "token_file de "+s.ID); err != nil {
				return nil, err
			}
			raw, err := readLimited(s.TokenFile, maxTokenBytes)
			if err != nil {
				return nil, fmt.Errorf("token_file de %s: %v", s.ID, err)
			}
			tok := strings.TrimSpace(string(raw))
			if !tokenRe.MatchString(tok) {
				return nil, fmt.Errorf("token_file de %s: o token deve ser texto ASCII visível, sem espaços", s.ID)
			}
			b.token = tok
		}
		var tr *http.Transport
		if u.Scheme == "https" {
			t, err := ollama.NewTLSTransport(s.TLSCA)
			if err != nil {
				return nil, fmt.Errorf("backend %s: %v", s.ID, strings.TrimPrefix(err.Error(), "--tls-ca: "))
			}
			tr = t
		} else {
			tr = http.DefaultTransport.(*http.Transport).Clone()
		}
		tr.Proxy = nil // the pool is on the LAN; never go through an environment proxy
		b.Transport = tr
		cl := &http.Client{
			Transport: bearerRT{rt: tr, token: b.token}, Timeout: timeout,
			// A redirect could carry the token (or the probe) somewhere else.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
		b.col = &models.Collector{Upstream: u, Allowed: allowed, ThinkSuffix: think, Client: cl, Timeout: timeout, Logf: logf}
		out = append(out, b)
	}
	return out, nil
}

// sortByOrder orders secondaries by preference: ordem, then id.
func sortByOrder(bs []*Backend) {
	sort.SliceStable(bs, func(i, j int) bool {
		if bs[i].Ordem != bs[j].Ordem {
			return bs[i].Ordem < bs[j].Ordem
		}
		return bs[i].ID < bs[j].ID
	})
}
