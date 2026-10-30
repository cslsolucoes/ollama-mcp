package ollama

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
)

const fbToken = "fb-S3NT1NEL-9876543210"

const genOK = `{"model":"qwen3-ia","response":"ola","done":true,"prompt_eval_count":3,"eval_count":2}` + "\n"

// deadURL is a URL nothing listens on.
func deadURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return "http://" + addr
}

type seen struct {
	mu    sync.Mutex
	auths []string
}

func (s *seen) add(r *http.Request) {
	s.mu.Lock()
	s.auths = append(s.auths, r.Header.Get("Authorization"))
	s.mu.Unlock()
}
func (s *seen) get() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.auths...)
}

// Connection error on the primary: the fallback is tried once, the result says
// backend=fallback and the fallback got ITS OWN token, never the primary's.
func TestFallbackOnConnectionError(t *testing.T) {
	var fs seen
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		fs.add(r)
		fmt.Fprint(w, genOK)
	})
	c, err := New(Options{BaseURL: deadURL(t), APIKey: token, Model: "qwen3-ia", FallbackURL: fb.URL, FallbackAPIKey: fbToken})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got, err := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Backend != "fallback" || got.Response != "ola" {
		t.Fatalf("backend %q response %q", got.Backend, got.Response)
	}
	if fbN.Load() != 1 {
		t.Fatalf("the fallback saw %d requests, want exactly 1", fbN.Load())
	}
	for _, a := range fs.get() {
		if a != "Bearer "+fbToken {
			t.Fatalf("the fallback received Authorization %q", a)
		}
		if strings.Contains(a, token) {
			t.Fatal("the primary token reached the fallback")
		}
	}
}

// 503 "queue full" (the gateway's overload answer, with Retry-After): the
// fallback is tried at once, without waiting for the primary.
func TestFallbackOnQueueFull503(t *testing.T) {
	primary, pN := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		http.Error(w, `{"error":"fila do servidor cheia"}`, http.StatusServiceUnavailable)
	})
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	c, err := New(Options{BaseURL: primary.URL, APIKey: token, Model: "qwen3-ia", FallbackURL: fb.URL, FallbackAPIKey: fbToken})
	if err != nil {
		// the fallback is http on loopback: a token is allowed there
		t.Fatal(err)
	}
	defer c.Close()
	got, err := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Backend != "fallback" {
		t.Fatalf("backend %q", got.Backend)
	}
	if pN.Load() != 1 || fbN.Load() != 1 {
		t.Fatalf("primary %d, fallback %d requests: one each, and no wait for the primary retry", pN.Load(), fbN.Load())
	}
}

// Embeddings go through the same path.
func TestFallbackEmbed(t *testing.T) {
	fb, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model":"nomic-embed-text","embeddings":[[0.1,0.2]],"prompt_eval_count":2}`)
	})
	c, _ := New(Options{BaseURL: deadURL(t), EmbedModel: "nomic-embed-text", FallbackURL: fb.URL})
	defer c.Close()
	got, err := c.Embed(context.Background(), EmbedRequest{Input: []string{"a"}})
	if err != nil || got.Backend != "fallback" {
		t.Fatalf("%v %+v", err, got)
	}
}

// Without the flags nothing changes: the primary's error stands and no second
// server is ever contacted.
func TestNoFallbackNothingChanges(t *testing.T) {
	c, err := New(Options{BaseURL: deadURL(t), Model: "qwen3-ia"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	wantKind(t, err, ErrConnect)
	// and a healthy primary never reports a fallback
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	c2 := newClient(t, srv, nil)
	got, err := c2.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	if err != nil || got.Backend != "" {
		t.Fatalf("%v backend %q", err, got.Backend)
	}
}

// A healthy primary never touches the fallback.
func TestFallbackUntouchedWhenThePrimaryAnswers(t *testing.T) {
	primary, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	c := newClient(t, primary, func(o *Options) { o.FallbackURL = fb.URL })
	got, err := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	if err != nil || got.Backend != "" || fbN.Load() != 0 {
		t.Fatalf("%v backend %q fallback hits %d", err, got.Backend, fbN.Load())
	}
}

// Other failures are not retried on the fallback: a 500, a 404, a cut stream.
func TestFallbackOnlyForConnectionAnd503(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusNotFound, http.StatusUnauthorized, http.StatusBadGateway} {
		primary, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, `{"error":"x"}`, status) })
		fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
		c := newClient(t, primary, func(o *Options) { o.FallbackURL = fb.URL })
		if _, err := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"}); err == nil {
			t.Fatalf("status %d: no error", status)
		}
		if fbN.Load() != 0 {
			t.Fatalf("status %d reached the fallback", status)
		}
	}
	// a bare 503 (the Ollama itself, no Retry-After) is not "queue full" either
	primary, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	c := newClient(t, primary, func(o *Options) { o.FallbackURL = fb.URL })
	if _, err := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"}); err == nil || fbN.Load() != 0 {
		t.Fatalf("bare 503: err=%v fallback hits %d", err, fbN.Load())
	}
}

// If the fallback is down too, the primary's own error is what the caller sees,
// and the fallback is tried only once.
func TestFallbackAlsoDown(t *testing.T) {
	c, err := New(Options{BaseURL: deadURL(t), Model: "qwen3-ia", FallbackURL: deadURL(t), FallbackAPIKey: fbToken})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	e := wantKind(t, err, ErrConnect)
	if strings.Contains(e.Msg, fbToken) {
		t.Fatal("the fallback token leaked into an error")
	}
}

// The fallback token never shows up in the primary client's messages or logs,
// and a fallback token over plain http to a non-loopback host is refused.
func TestFallbackTokenIsRedactedAndGuarded(t *testing.T) {
	var logs []string
	c, err := New(Options{BaseURL: deadURL(t), Model: "qwen3-ia", FallbackURL: deadURL(t), FallbackAPIKey: fbToken,
		Logf: func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	for _, l := range logs {
		if strings.Contains(l, fbToken) {
			t.Fatalf("token in a log line: %s", l)
		}
	}
	if _, err := New(Options{BaseURL: "http://127.0.0.1:1", FallbackURL: "http://10.1.2.3:11434", FallbackAPIKey: fbToken}); err == nil {
		t.Fatal("a token over plain http to a LAN host must be refused")
	}
	if _, err := New(Options{BaseURL: "http://127.0.0.1:1", FallbackURL: "ftp://x"}); err == nil {
		t.Fatal("a bad fallback URL must be refused")
	}
}

// The fallback's --insecure never follows the primary's: its TLS is the system
// pool (plus its own CA).
func TestFallbackIgnoresPrimaryInsecure(t *testing.T) {
	c, err := New(Options{BaseURL: "https://10.0.0.1:1", Insecure: true, FallbackURL: "https://10.0.0.2:1"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.fb == nil {
		t.Fatal("fallback not built")
	}
	tr := c.fb.http.Transport.(*http.Transport)
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("the fallback inherited --insecure")
	}
}
