package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const token = "tok-S3NT1NEL-0123456789"

// newMock starts a plain-HTTP Ollama mock and counts the requests it gets.
func newMock(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		// Read the body to EOF first: only then does the server watch the
		// connection, so r.Context() is cancelled when the client goes away.
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func newClient(t *testing.T, srv *httptest.Server, mod func(*Options)) *Client {
	t.Helper()
	o := Options{BaseURL: srv.URL, APIKey: token, Model: "qwen3-ia", EmbedModel: "nomic-embed-text", Timeout: 5 * time.Second}
	if mod != nil {
		mod(&o)
	}
	c, err := New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func mustJSON(t *testing.T, r *http.Request, v any) {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, v); err != nil {
		t.Errorf("mock cannot decode request %q: %v", b, err)
	}
}

func wantKind(t *testing.T, err error, kind error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if !errors.Is(err, kind) {
		t.Fatalf("want kind %v, got %v (%s)", kind, e.Kind, e.Msg)
	}
	return e
}

func TestTags(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/tags" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"models":[{"name":"qwen3-ia:latest","model":"qwen3-ia:latest","modified_at":"2026-10-23T15:36:22-03:00","size":18556701696,"digest":"abc","details":{"parent_model":"qwen3-coder:30b-a3b-q4_K_M","format":"gguf","family":"qwen3moe","parameter_size":"30.5B","quantization_level":"Q4_K_M"}},{"name":"nomic-embed-text:latest","size":274302450}]}`)
	})
	got, err := newClient(t, srv, nil).Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 2 || got.Models[0].Name != "qwen3-ia:latest" || got.Models[0].Details.QuantizationLevel != "Q4_K_M" || got.Models[1].Size != 274302450 {
		t.Fatalf("decoded: %+v", got)
	}
}

func TestPS(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/ps" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"models":[{"name":"qwen3-ia:latest","model":"qwen3-ia:latest","size":25562028438,"digest":"abc","expires_at":"0001-01-01T00:00:00Z","size_vram":25562028438,"context_length":4096}]}`)
	})
	got, err := newClient(t, srv, nil).PS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0].SizeVRAM != 25562028438 || got.Models[0].ContextLength != 4096 {
		t.Fatalf("decoded: %+v", got)
	}
}

func TestShow(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		mustJSON(t, r, &req)
		if r.Method != "POST" || r.URL.Path != "/api/show" || req["model"] != "qwen3-ia" {
			t.Errorf("got %s %s %v", r.Method, r.URL.Path, req)
		}
		fmt.Fprint(w, `{"modelfile":"HUGE","parameters":"temperature 0.7\ntop_k 20","details":{"family":"qwen3moe"},"model_info":{"qwen3moe.expert_count":128},"capabilities":["completion","tools"]}`)
	})
	got, err := newClient(t, srv, nil).Show(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Details.Family != "qwen3moe" || len(got.Capabilities) != 2 || got.ModelInfo["qwen3moe.expert_count"] != float64(128) {
		t.Fatalf("decoded: %+v", got)
	}
}

func TestEmbed(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		var req embedWire
		mustJSON(t, r, &req)
		if r.URL.Path != "/api/embed" || req.Model != "nomic-embed-text" || len(req.Input) != 2 {
			t.Errorf("got %s %+v", r.URL.Path, req)
		}
		fmt.Fprint(w, `{"model":"nomic-embed-text","embeddings":[[0.1,0.2,0.3],[0.4,0.5,0.6]],"total_duration":9936000000,"load_duration":3230000000,"prompt_eval_count":13}`)
	})
	got, err := newClient(t, srv, nil).Embed(context.Background(), EmbedRequest{Input: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Embeddings) != 2 || len(got.Embeddings[0]) != 3 || got.PromptEvalCount != 13 || got.LoadDuration != 3230000000 {
		t.Fatalf("decoded: %+v", got)
	}
}

func TestEmbedEmpty(t *testing.T) {
	srv, n := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"embeddings":[]}`) })
	c := newClient(t, srv, nil)
	if _, err := c.Embed(context.Background(), EmbedRequest{}); err == nil || !strings.Contains(err.Error(), "no embeddings returned") || n.Load() != 0 {
		t.Fatalf("empty input: %v, calls=%d", err, n.Load())
	}
	_, err := c.Embed(context.Background(), EmbedRequest{Input: []string{"x"}})
	wantKind(t, err, ErrBadResponse)
}

func TestHTTPStatusMap(t *testing.T) {
	html := "<html><head><title>403 Forbidden</title></head><body><center><h1>403 Forbidden</h1></center><hr><center>nginx</center></body></html>"
	cases := []struct {
		name   string
		status int
		body   string
		kind   error
		want   string
	}{
		{"401 json", 401, `{"error":"unauthorized"}`, ErrUnauthorized, "token ausente ou inválido (OLLAMA_API_KEY)"},
		{"403 nginx html", 403, html, ErrForbidden, "endpoint bloqueado pelo nginx"},
		{"403 gateway allowlist", 403, `{"error":"modelo \"x\" não permitido neste servidor"}`, ErrModelNotAllowed, "modelo não permitido pelo servidor"},
		{"404 model", 404, `{"error":"model 'x' not found"}`, ErrNotFound, "modelo ou endpoint não encontrado"},
		{"400", 400, `{"error":"invalid options"}`, ErrBadRequest, "invalid options"},
		{"500", 500, `{"error":"boom"}`, ErrServer, "erro do servidor Ollama"},
		{"502 restarting", 502, "<html>Bad Gateway</html>", ErrServer, "carga a frio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, n := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			_, err := newClient(t, srv, nil).Tags(context.Background())
			e := wantKind(t, err, tc.kind)
			if e.Status != tc.status || !strings.Contains(e.Msg, tc.want) {
				t.Fatalf("status=%d msg=%q", e.Status, e.Msg)
			}
			if strings.ContainsAny(e.Msg, "<>") {
				t.Fatalf("HTML leaked into the message: %q", e.Msg)
			}
			if n.Load() != 1 {
				t.Fatalf("attempts = %d, want 1", n.Load())
			}
		})
	}
}

func TestStatusOnPost(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"model 'nope' not found"}`)
	})
	_, err := newClient(t, srv, nil).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	e := wantKind(t, err, ErrNotFound)
	if !strings.Contains(e.Msg, "model 'nope' not found") {
		t.Fatalf("detail lost: %q", e.Msg)
	}
}

func TestErrorBodyIsLimited(t *testing.T) {
	var written atomic.Int64
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		chunk := []byte(strings.Repeat("A", 1<<16))
		for i := 0; i < 16; i++ { // 1 MiB
			n, err := w.Write(chunk)
			written.Add(int64(n))
			if err != nil {
				return
			}
		}
	})
	_, err := newClient(t, srv, nil).Tags(context.Background())
	e := wantKind(t, err, ErrServer)
	if len(e.Msg) > 600 {
		t.Fatalf("message is %d bytes; the body must be capped", len(e.Msg))
	}
}

func TestInvalidJSON(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>not json</html>") })
	_, err := newClient(t, srv, nil).Tags(context.Background())
	e := wantKind(t, err, ErrBadResponse)
	if strings.ContainsAny(e.Msg, "<>") {
		t.Fatalf("body echoed: %q", e.Msg)
	}
}

func TestBodyAboveLimit(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[{"name":"`+strings.Repeat("a", 5000)+`"}]}`)
	})
	_, err := newClient(t, srv, func(o *Options) { o.MaxBodyBytes = 1000 }).Tags(context.Background())
	wantKind(t, err, ErrBadResponse)
}

func TestTimeoutMentionsColdLoad(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})
	start := time.Now()
	_, err := newClient(t, srv, func(o *Options) { o.Timeout = 80 * time.Millisecond }).
		Generate(context.Background(), GenerateRequest{Prompt: "x"})
	e := wantKind(t, err, ErrTimeout)
	if !strings.Contains(e.Msg, "carga a frio") || !strings.Contains(e.Msg, "--timeout") {
		t.Fatalf("msg: %q", e.Msg)
	}
	if time.Since(start) > time.Second {
		t.Fatal("deadline was not enforced")
	}
}

func TestColdLoadSlowerThanLoadIsNotAFailure(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond) // the model loading
		fmt.Fprint(w, `{"model":"qwen3-ia","response":"ok","done":true,"done_reason":"stop","load_duration":36449000000,"total_duration":36675000000,"eval_count":1}`+"\n")
	})
	got, err := newClient(t, srv, func(o *Options) { o.Timeout = time.Second }).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if err != nil || got.LoadDuration != 36449000000 || got.Response != "ok" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestContextCancel(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := newClient(t, srv, nil).Generate(ctx, GenerateRequest{Prompt: "x"})
	wantKind(t, err, ErrCanceled)
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer other.Close()
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	})
	_, err := newClient(t, srv, nil).Tags(context.Background())
	wantKind(t, err, ErrBadResponse)
	if hits.Load() != 0 {
		t.Fatal("redirect target was contacted")
	}
}

func TestConnectRefused(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {})
	c := newClient(t, srv, nil)
	srv.Close()
	_, err := c.Tags(context.Background())
	e := wantKind(t, err, ErrConnect)
	if !strings.Contains(e.Msg, "não foi possível falar com") {
		t.Fatalf("msg: %q", e.Msg)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(Options{}); err == nil || !strings.Contains(err.Error(), "--ollama-url") {
		t.Fatalf("empty url: %v", err)
	}
	if _, err := New(Options{BaseURL: "host:11434"}); err == nil {
		t.Fatal("scheme-less url must fail")
	}
	c, err := New(Options{BaseURL: "https://h.invalid:8443/"})
	if err != nil || c.base.String() != "https://h.invalid:8443" {
		t.Fatalf("trailing slash: %v %v", c, err)
	}
}

func TestBearerRefusedOnRemoteHTTP(t *testing.T) {
	if _, err := New(Options{BaseURL: "http://ia.example.invalid:11434", APIKey: token}); err == nil {
		t.Fatal("Bearer over remote http must be refused")
	} else if strings.Contains(err.Error(), token) {
		t.Fatal("token in error")
	}
	for _, u := range []string{"http://127.0.0.1:11434", "http://localhost:11434", "http://[::1]:11434", "https://ia.example.invalid"} {
		if _, err := New(Options{BaseURL: u, APIKey: token}); err != nil {
			t.Errorf("%s must be accepted: %v", u, err)
		}
	}
	if _, err := New(Options{BaseURL: "http://ia.example.invalid:11434"}); err != nil {
		t.Errorf("no token, no refusal: %v", err)
	}
}

func TestTransportIsClonedFromDefault(t *testing.T) {
	c, err := New(Options{BaseURL: "https://h.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	tr := c.http.Transport.(*http.Transport)
	if tr.Proxy == nil || tr.MaxIdleConns == 0 || tr.IdleConnTimeout == 0 || !tr.ForceAttemptHTTP2 {
		t.Fatalf("transport lost the DefaultTransport settings: %+v", tr)
	}
	if tr == http.DefaultTransport.(*http.Transport) {
		t.Fatal("must be a clone, not the shared default")
	}
	if c.http.Timeout != 0 {
		t.Fatal("http.Client.Timeout must stay zero (it would cut streams)")
	}
}

// 10.7 cosmetic: a 403 from the gateway allowlist must not blame the nginx.
func TestGatewayAllowlist403IsNotTheNginxBlock(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `{"error":"modelo \"x\" não permitido neste servidor"}`)
	})
	_, err := newClient(t, srv, nil).Tags(context.Background())
	e := wantKind(t, err, ErrModelNotAllowed)
	if !errors.Is(err, ErrForbidden) || strings.Contains(e.Msg, "nginx") {
		t.Fatalf("msg=%q forbidden=%v", e.Msg, errors.Is(err, ErrForbidden))
	}
}
