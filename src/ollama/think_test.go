package ollama

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// rawBodies records the exact bytes sent to each path.
func rawBodies(t *testing.T, out map[string]string, reply string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		out[r.URL.Path] = string(b)
		writeStream(w, reply)
	}
}

func TestWireCarriesThinkFalse(t *testing.T) {
	for _, noStream := range []bool{false, true} {
		got := map[string]string{}
		srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			got[r.URL.Path] = string(b)
			if r.URL.Path == "/api/chat" {
				writeStream(w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
				return
			}
			writeStream(w, `{"response":"ok","done":true}`)
		})
		c := newClient(t, srv, func(o *Options) { o.DisableStream = noStream })
		if _, err := c.Generate(context.Background(), GenerateRequest{Prompt: "x"}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Chat(context.Background(), ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}}); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"/api/generate", "/api/chat"} {
			if !strings.Contains(got[p], `"think":false`) {
				t.Fatalf("noStream=%v %s body lacks \"think\":false: %s", noStream, p, got[p])
			}
		}
	}
}

func TestEmbedWireHasNoThink(t *testing.T) {
	got := map[string]string{}
	srv, _ := newMock(t, rawBodies(t, got, `{"embeddings":[[0.5]]}`))
	if _, err := newClient(t, srv, nil).Embed(context.Background(), EmbedRequest{Input: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if b, ok := got["/api/embed"]; !ok || strings.Contains(b, "think") {
		t.Fatalf("embed body: %q", b)
	}
}
