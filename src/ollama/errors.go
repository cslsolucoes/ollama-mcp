package ollama

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
)

// Error kinds. Every error returned by the client is an *Error whose Kind is
// one of these, so callers (and tests) can branch with errors.Is.
var (
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrNotFound          = errors.New("not found")
	ErrBadRequest        = errors.New("bad request")
	ErrServer            = errors.New("server error")
	ErrTimeout           = errors.New("timeout")
	ErrCanceled          = errors.New("canceled")
	ErrConnect           = errors.New("cannot connect")
	ErrConnectionDropped = errors.New("connection dropped")
	ErrTLS               = errors.New("tls verification failed")
	ErrBadResponse       = errors.New("bad response")
	ErrStream            = errors.New("stream failed")
	ErrPromptTooLong     = errors.New("prompt too long")
	ErrQueueFull         = errors.New("server queue full")
	ErrRemoved           = errors.New("removed by the server administrator")
)

// Error is the single error type of the package. Msg is final: it was already
// scrubbed (token by exact value, URLs reduced to scheme+host+path) when the
// error was built, so it is safe to show to the MCP client and to log.
type Error struct {
	Kind   error
	Status int // HTTP status, 0 when the failure was not an HTTP status
	Msg    string
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Kind }

// fail builds an *Error with the scrubbed message.
func (c *Client) fail(kind error, status int, format string, a ...any) *Error {
	return &Error{Kind: kind, Status: status, Msg: c.scrub(fmt.Sprintf(format, a...))}
}

var urlRe = regexp.MustCompile(`https?://[^\s"'<>)]+`)

// sanitizeURL is the single URL sanitiser (SEC-004): it drops userinfo, query
// and fragment. Unparsable input is cut at the first '?' or '#'.
func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			raw = raw[:i]
		}
		if i := strings.Index(raw, "@"); i >= 0 {
			if j := strings.Index(raw, "://"); j >= 0 && j < i {
				raw = raw[:j+3] + raw[i+1:]
			}
		}
		return raw
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return u.String()
}

// scrub reduces every URL in the text to scheme+host+path, then masks the
// token and the URL credentials by exact value.
func (c *Client) scrub(s string) string {
	// URLs first: dropping userinfo and query also drops anything secret in
	// them; the exact-value redaction then catches every other occurrence.
	s = urlRe.ReplaceAllStringFunc(s, sanitizeURL)
	return c.redact(s)
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// detail turns a server-supplied body (JSON {"error":...}, plain text or the
// nginx HTML page) into one short line. Redaction runs BEFORE the truncation,
// so a token cannot survive as a cut prefix.
func (c *Client) detail(body []byte) string {
	s := string(body)
	var j struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &j) == nil && j.Error != "" {
		s = j.Error
	}
	s = c.scrub(s)
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxDetailRunes {
		s = string(r[:maxDetailRunes]) + "..."
	}
	return s
}

// statusError is the OLL-004 map. Short, actionable, no server HTML.
func (c *Client) statusError(method, path string, status int, body []byte) *Error {
	d := c.detail(body)
	suffix := fmt.Sprintf(" [HTTP %d %s %s]", status, method, path)
	if d != "" {
		suffix += ": " + d
	}
	switch {
	case status == 401:
		return c.fail(ErrUnauthorized, status, "token ausente ou inválido (OLLAMA_API_KEY)%s", suffix)
	case status == 403:
		return c.fail(ErrForbidden, status, "endpoint bloqueado pelo nginx%s", suffix)
	case status == 404:
		return c.fail(ErrNotFound, status, "modelo ou endpoint não encontrado%s", suffix)
	case status >= 500:
		extra := ""
		if status == 502 || status == 503 || status == 504 {
			extra = "; o servidor pode estar reiniciando ou carregando o modelo (carga a frio ~36 s)"
		}
		return c.fail(ErrServer, status, "erro do servidor Ollama%s%s", extra, suffix)
	case status >= 300 && status < 400:
		return c.fail(ErrBadResponse, status, "redirecionamento não seguido%s", suffix)
	case status >= 400:
		return c.fail(ErrBadRequest, status, "requisição recusada pelo servidor%s", suffix)
	}
	return c.fail(ErrBadResponse, status, "resposta inesperada do servidor%s", suffix)
}

func isDropped(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	// Windows WSAECONNRESET / WSAECONNABORTED (the syscall constants differ there).
	var errno syscall.Errno
	if errors.As(err, &errno) && (errno == 10054 || errno == 10053) {
		return true
	}
	// The message is localised on Windows ("wsarecv: Foi forçado o
	// cancelamento..."), so match the stable tokens only.
	m := strings.ToLower(err.Error())
	for _, t := range []string{"wsarecv", "wsasend", "forcibly closed", "connection reset", "connection aborted", "broken pipe"} {
		if strings.Contains(m, t) {
			return true
		}
	}
	return false
}

// transportErr classifies a failure of the HTTP exchange itself: deadline,
// cancellation, TLS, dropped connection (the host restarting shows up as a
// bare RST/EOF with no body), or plain connect failure. midStream is true
// when part of the response had already arrived (received bytes).
func (c *Client) transportErr(ctx context.Context, err error, start time.Time, midStream bool, received int) *Error {
	elapsed := time.Since(start).Round(100 * time.Millisecond)
	inner := err
	var ue *url.Error
	if errors.As(err, &ue) {
		inner = ue.Err // the *url.Error text embeds the full URL
	}
	var ne net.Error
	switch {
	case ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled):
		return c.fail(ErrCanceled, 0, "chamada cancelada após %s", elapsed)
	case ctx.Err() != nil, errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return c.fail(ErrTimeout, 0,
			"tempo esgotado após %s (--timeout %s); a primeira chamada depois de o modelo sair da VRAM leva ~36 s (carga a frio): tente de novo ou aumente --timeout",
			elapsed, c.timeout)
	case isTLSVerify(err):
		return c.fail(ErrTLS, 0,
			"certificado TLS do servidor não é confiável (%s); informe a CA com --tls-ca (OLLAMA_TLS_CA) ou, só em último caso, --insecure",
			inner)
	case isDropped(err):
		where := "sem resposta"
		if midStream {
			where = fmt.Sprintf("no meio da resposta (%d bytes recebidos)", received)
		}
		return c.fail(ErrConnectionDropped, 0,
			"o servidor encerrou a conexão após %s %s; o servidor Ollama pode estar reiniciando ou carregando o modelo (carga a frio ~36 s), ou o prompt é grande demais (ver --max-prompt-chars); a chamada não foi repetida",
			elapsed, where)
	}
	return c.fail(ErrConnect, 0, "não foi possível falar com %s: %v", sanitizeURL(c.base.String()), inner)
}

func isTLSVerify(err error) bool {
	var cve *tls.CertificateVerificationError
	var ua x509.UnknownAuthorityError
	var he x509.HostnameError
	var ci x509.CertificateInvalidError
	return errors.As(err, &cve) || errors.As(err, &ua) || errors.As(err, &he) || errors.As(err, &ci)
}
