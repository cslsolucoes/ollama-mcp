package ollama

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type captured struct {
	method, path, rawQuery, auth, body string
	header                             http.Header
}

func TestBearerOnlyInHeader(t *testing.T) {
	var mu sync.Mutex
	var reqs []captured
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, captured{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), string(b), r.Header.Clone()})
		mu.Unlock()
		switch r.URL.Path {
		case "/api/generate", "/api/chat":
			fmt.Fprint(w, `{"response":"ok","message":{"content":"ok"},"done":true}`+"\n")
		case "/api/embed":
			fmt.Fprint(w, `{"embeddings":[[1]]}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	})
	// userinfo and query in the configured URL must be neither sent nor used.
	u, _ := url.Parse(srv.URL)
	c := newClient(t, srv, func(o *Options) { o.BaseURL = "http://user:pw@" + u.Host + "/?apikey=QSECRET#frag" })
	ctx := context.Background()
	c.Tags(ctx)
	c.PS(ctx)
	c.Show(ctx, "")
	c.Generate(ctx, GenerateRequest{Prompt: "p"})
	c.Chat(ctx, ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "m"}}})
	c.Embed(ctx, EmbedRequest{Input: []string{"e"}})

	if len(reqs) != 6 {
		t.Fatalf("requests seen: %d", len(reqs))
	}
	for _, q := range reqs {
		if q.auth != "Bearer "+token {
			t.Errorf("%s %s: Authorization = %q", q.method, q.path, q.auth)
		}
		if q.rawQuery != "" {
			t.Errorf("%s: query %q", q.path, q.rawQuery)
		}
		if strings.Contains(q.body, token) || strings.Contains(q.body, "QSECRET") {
			t.Errorf("%s: secret in body", q.path)
		}
		for k, v := range q.header {
			if k != "Authorization" && strings.Contains(strings.Join(v, ","), token) {
				t.Errorf("%s: token in header %s", q.path, k)
			}
		}
		if strings.HasPrefix(q.auth, "Basic") {
			t.Errorf("userinfo was sent as Basic auth")
		}
	}
}

func TestNoTokenNoAuthorizationHeader(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			t.Error("Authorization sent without a token")
		}
		fmt.Fprint(w, `{}`)
	})
	if _, err := newClient(t, srv, func(o *Options) { o.APIKey = "" }).Tags(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTokenNeverInErrorsOrLogs(t *testing.T) {
	long := strings.Repeat("x", 190) + token + strings.Repeat("y", 50) // the cut at 200 would leave a prefix
	bodies := map[string]string{
		"echo in json":      `{"error":"bad key ` + token + `"}`,
		"echo in text":      "denied for Bearer " + token,
		"echo near the cut": long,
		"url with secrets":  `see http://user:pw@ia.example.invalid:8443/api/x?key=` + token + `#f`,
	}
	for name, body := range bodies {
		for _, status := range []int{401, 403, 404, 500} {
			t.Run(fmt.Sprintf("%s/%d", name, status), func(t *testing.T) {
				var logs strings.Builder
				srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					io.WriteString(w, body)
				})
				c := newClient(t, srv, func(o *Options) {
					o.Logf = func(f string, a ...any) { fmt.Fprintf(&logs, f+"\n", a...) }
				})
				_, err := c.Tags(context.Background())
				assertClean(t, err.Error()+"\n"+logs.String())
			})
		}
	}

	// Echo inside the stream, in a non-stream error field and in embed.
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			fmt.Fprint(w, `{"error":"key `+token+` rejected"}`)
		default:
			writeStream(w, `{"response":"a","done":false}`+"\n"+`{"error":"key `+token+` rejected"}`+"\n")
		}
	})
	c := newClient(t, srv, nil)
	_, err := c.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	assertClean(t, err.Error())
	_, err = c.Embed(context.Background(), EmbedRequest{Input: []string{"x"}})
	assertClean(t, err.Error())
	nc := newClient(t, srv, func(o *Options) { o.DisableStream = true })
	_, err = nc.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	assertClean(t, err.Error())
}

func assertClean(t *testing.T, text string) {
	t.Helper()
	for _, bad := range []string{token, token[:8], "user:pw", "pw@", "key=", "#f"} {
		if strings.Contains(text, bad) {
			t.Fatalf("%q leaked in: %s", bad, text)
		}
	}
}

func TestURLSanitisedInErrors(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {})
	u, _ := url.Parse(srv.URL)
	c := newClient(t, srv, func(o *Options) { o.BaseURL = "http://alice:hunter2@" + u.Host + "/?secret=QSECRET" })
	srv.Close()
	_, err := c.Tags(context.Background())
	wantKind(t, err, ErrConnect)
	for _, bad := range []string{"alice", "hunter2", "QSECRET", "secret="} {
		if strings.Contains(err.Error(), bad) {
			t.Fatalf("%q leaked: %s", bad, err)
		}
	}
	if !strings.Contains(err.Error(), "http://"+u.Host) {
		t.Fatalf("sanitised host expected: %s", err)
	}
}

func TestSanitizeURL(t *testing.T) {
	cases := map[string]string{
		"https://u:p@h:8443/api/tags?x=1#f": "https://h:8443/api/tags",
		"http://h/a":                        "http://h/a",
		"https://h?token=abc":               "https://h",
		"http://u:p@h/%zz?k=v":              "http://h/%zz", // unparsable: cut by hand
	}
	for in, want := range cases {
		if got := sanitizeURL(in); got != want {
			t.Errorf("sanitizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPromptCeilingNeverReachesTheServer(t *testing.T) {
	srv, n := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":"ok","message":{"content":"ok"},"embeddings":[[1]],"done":true}`+"\n")
	})
	c := newClient(t, srv, func(o *Options) { o.MaxPromptChars = 10 })
	ctx := context.Background()

	_, err := c.Generate(ctx, GenerateRequest{Prompt: strings.Repeat("a", 11)})
	e := wantKind(t, err, ErrPromptTooLong)
	if !strings.Contains(e.Msg, "--max-prompt-chars") || !strings.Contains(e.Msg, "nada foi enviado") {
		t.Fatalf("msg: %q", e.Msg)
	}
	_, err = c.Generate(ctx, GenerateRequest{Prompt: strings.Repeat("a", 6), System: strings.Repeat("b", 5)})
	wantKind(t, err, ErrPromptTooLong) // system counts
	_, err = c.Chat(ctx, ChatRequest{Messages: []ChatMessage{{Role: "user", Content: strings.Repeat("a", 6)}, {Role: "assistant", Content: strings.Repeat("b", 5)}}})
	wantKind(t, err, ErrPromptTooLong)
	_, err = c.Embed(ctx, EmbedRequest{Input: []string{strings.Repeat("a", 6), strings.Repeat("b", 5)}})
	wantKind(t, err, ErrPromptTooLong)
	if n.Load() != 0 {
		t.Fatalf("the mock received %d request(s); an oversized prompt must not leave the client", n.Load())
	}

	// Characters, not bytes: 10 multi-byte runes are exactly at the ceiling.
	if _, err := c.Generate(ctx, GenerateRequest{Prompt: strings.Repeat("é", 10)}); err != nil {
		t.Fatalf("at the ceiling: %v", err)
	}
	if n.Load() != 1 {
		t.Fatalf("calls = %d", n.Load())
	}
}

func TestDefaultPromptCeiling(t *testing.T) {
	c, err := New(Options{BaseURL: "https://h.invalid"})
	if err != nil || c.maxPrompt != 128000 {
		t.Fatalf("default ceiling = %v (%v)", c.maxPrompt, err)
	}
}

func TestMissingModel(t *testing.T) {
	c, err := New(Options{BaseURL: "https://h.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	wantKind(t, err, ErrBadRequest)
}

// sourceFiles returns the non-test Go files of the package.
func sourceFiles(t *testing.T) map[string]string {
	t.Helper()
	files, _ := filepath.Glob("*.go")
	out := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(b)
	}
	if len(out) == 0 {
		t.Fatal("no source files found")
	}
	return out
}

func TestNoAdminEndpointsInCode(t *testing.T) { // SEC-012
	for f, src := range sourceFiles(t) {
		for _, p := range []string{"/api/pull", "/api/push", "/api/create", "/api/delete", "/api/copy", "/api/blobs"} {
			if strings.Contains(src, p) {
				t.Errorf("%s mentions %s", f, p)
			}
		}
	}
}

func TestNoContextSizeLiterals(t *testing.T) { // PERF-006
	for f, src := range sourceFiles(t) {
		for _, lit := range []string{"32768", "131072", "65536"} {
			if strings.Contains(src, lit) {
				t.Errorf("%s hard-codes %s", f, lit)
			}
		}
	}
}
