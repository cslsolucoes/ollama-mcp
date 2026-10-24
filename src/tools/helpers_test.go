package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/ollama"
)

// sentinel is a fake token: it must never show up in a result, an error or a
// log line.
const sentinel = "tok-S3NTINEL-9f8e7d6c"

// mock is a fake Ollama. It records every request and counts hits.
type mock struct {
	srv  *httptest.Server
	hits atomic.Int32

	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
	auths  []string
}

func newMock(t *testing.T, h http.HandlerFunc) *mock {
	t.Helper()
	m := &mock{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		m.mu.Lock()
		m.paths = append(m.paths, r.URL.Path)
		m.bodies = append(m.bodies, body)
		m.auths = append(m.auths, r.Header.Get("Authorization"))
		m.mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// lastBody returns the decoded JSON body of the n-th request (0-based).
func (m *mock) body(n int) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n >= len(m.bodies) {
		return nil
	}
	return m.bodies[n]
}

// deps builds Deps backed by a real ollama.Client pointed at the mock.
func (m *mock) deps(t *testing.T, mod func(*ollama.Options)) Deps {
	t.Helper()
	o := ollama.Options{
		BaseURL: m.srv.URL, APIKey: sentinel, Model: "qwen3-ia", EmbedModel: "nomic-embed-text",
		Timeout: 10 * time.Second, MaxPromptChars: 128000,
	}
	if mod != nil {
		mod(&o)
	}
	c, err := ollama.New(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return Deps{
		Backend: func() (Backend, error) { return c, nil },
		Queue:   NewQueue(1), Model: o.Model, EmbedModel: o.EmbedModel,
	}
}

func entry(t *testing.T, d Deps, name string) mcp.ToolEntry {
	t.Helper()
	for _, e := range New(d) {
		if e.Def.Name == name {
			return e
		}
	}
	t.Fatalf("tool %q not registered", name)
	return mcp.ToolEntry{}
}

// call runs a tool handler with JSON arguments.
func call(t *testing.T, d Deps, name, argsJSON string) (mcp.ToolCallResult, error) {
	t.Helper()
	return entry(t, d, name).Handler(context.Background(), json.RawMessage(argsJSON))
}

func text(t *testing.T, res mcp.ToolCallResult) string {
	t.Helper()
	if len(res.Content) != 1 || res.Content[0].Type != "text" {
		t.Fatalf("want one text block, got %+v", res.Content)
	}
	return res.Content[0].Text
}

// streamLines writes NDJSON lines with a flush after each one.
func streamLines(w http.ResponseWriter, lines ...string) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, l := range lines {
		fmt.Fprintln(w, l)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// generateOK answers /api/generate with a three-chunk stream. The chunks carry
// a "thinking" field and the final one the counters.
func generateOK(w http.ResponseWriter, _ *http.Request) {
	streamLines(w,
		`{"model":"qwen3-ia","response":"Ola ","thinking":"PENSAMENTO-SECRETO","done":false}`,
		`{"model":"qwen3-ia","response":"mundo","thinking":"MAIS-PENSAMENTO","done":false}`,
		`{"model":"qwen3-ia","response":"!","done":true,"done_reason":"stop","total_duration":2000000000,"load_duration":1500000000,"prompt_eval_count":17,"eval_count":5,"prompt_eval_duration":100000000,"eval_duration":400000000}`)
}

func lastLine(s string) string {
	i := strings.LastIndex(s, "\n")
	return s[i+1:]
}
