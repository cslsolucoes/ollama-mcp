//go:build live

// Live checks against the real Ollama server (TEST-016, TEST-017). They are
// outside the default `go test ./...` and run strictly one call at a time
// (INTEG-012: four GPUs serve one resident model, parallel calls queue).
//
// Environment (the token is never printed):
//
//	OLLAMA_HOST          base URL, e.g. https://ia.cslsolucoes.com.br:8443
//	OLLAMA_TLS_CA        path of the CA bundle (PEM)
//	OLLAMA_API_KEY       Bearer token, or
//	OLLAMA_API_KEY_FILE  path of a file holding only the token
//
// Run: go test -tags live -count=1 -v -run TestLive ./ollama
//
// The prompts are tiny on purpose (PERF-005): never send a large prompt here.
package ollama

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func liveOptions(t *testing.T) Options {
	t.Helper()
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		t.Skip("OLLAMA_HOST not set")
	}
	key := os.Getenv("OLLAMA_API_KEY")
	if f := os.Getenv("OLLAMA_API_KEY_FILE"); key == "" && f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("cannot read OLLAMA_API_KEY_FILE")
		}
		key = strings.TrimSpace(string(b))
	}
	return Options{BaseURL: host, APIKey: key, TLSCA: os.Getenv("OLLAMA_TLS_CA"), Model: "qwen3-ia", EmbedModel: "nomic-embed-text",
		Timeout: 3 * time.Minute, MaxPromptChars: 2000}
}

func liveClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(liveOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestLive1Tags(t *testing.T) {
	got, err := liveClient(t).Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got.Models {
		t.Logf("model %s size=%d", m.Name, m.Size)
	}
	if len(got.Models) == 0 {
		t.Fatal("no models")
	}
}

func TestLive2PS(t *testing.T) {
	got, err := liveClient(t).PS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got.Models {
		t.Logf("resident %s size_vram=%d context_length=%d", m.Name, m.SizeVRAM, m.ContextLength)
	}
}

func TestLive3Unauthorized(t *testing.T) {
	o := liveOptions(t)
	o.APIKey = ""
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Tags(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	t.Logf("401 mapped: %v", err)
}

func TestLive4GenerateShort(t *testing.T) {
	temp, n := 0.0, 8
	got, err := liveClient(t).Generate(context.Background(), GenerateRequest{
		Prompt:  "Responda apenas com a palavra: ok",
		Options: &ModelOptions{Temperature: &temp, NumPredict: &n},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("response=%q done_reason=%s prompt_eval=%d eval=%d load_ms=%d total_ms=%d",
		got.Response, got.DoneReason, got.PromptEvalCount, got.EvalCount, got.LoadDuration/1e6, got.TotalDuration/1e6)
	if !got.Done || got.PromptEvalCount == 0 {
		t.Fatalf("counters missing: %+v", got.Usage)
	}
}
