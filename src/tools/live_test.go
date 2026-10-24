//go:build live

// Live checks of the base tools against the real Ollama (TEST-016, TEST-017).
// Outside the default `go test ./...`. One call at a time (INTEG-012): four
// GPUs serve one resident model, parallel calls only queue, and the tests have
// no t.Parallel on purpose. Tiny prompts only (PERF-005).
//
// Environment (the token is never printed):
//
//	OLLAMA_HOST          base URL, e.g. https://ia.cslsolucoes.com.br:8443
//	OLLAMA_TLS_CA        path of the CA bundle (PEM)
//	OLLAMA_API_KEY       Bearer token, or
//	OLLAMA_API_KEY_FILE  path of a file holding only the token
//
// Run one test per command, for example:
//
//	go test -tags live -count=1 -v -run TestLive1ListModels ./tools
package tools

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/ollama"
)

func liveDeps(t *testing.T) Deps {
	t.Helper()
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		t.Skip("OLLAMA_HOST not set")
	}
	key := os.Getenv("OLLAMA_API_KEY")
	if f := os.Getenv("OLLAMA_API_KEY_FILE"); key == "" && f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal("cannot read OLLAMA_API_KEY_FILE")
		}
		key = strings.TrimSpace(string(b))
	}
	c, err := ollama.New(ollama.Options{
		BaseURL: host, APIKey: key, TLSCA: os.Getenv("OLLAMA_TLS_CA"),
		Model: "qwen3-ia", EmbedModel: "nomic-embed-text",
		Timeout: 3 * time.Minute, MaxPromptChars: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return Deps{Backend: func() (Backend, error) { return c, nil }, Queue: NewQueue(1), Model: "qwen3-ia", EmbedModel: "nomic-embed-text"}
}

func TestLive1ListModels(t *testing.T) {
	res, err := call(t, liveDeps(t), "list_models", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	txt := text(t, res)
	t.Log("\n" + txt)
	if !strings.Contains(txt, "Instalados (") {
		t.Fatal("no installed list")
	}
}

func TestLive2Generate(t *testing.T) {
	res, err := call(t, liveDeps(t), "generate", `{"prompt":"Responda apenas com a palavra: ok","temperature":0,"num_predict":8}`)
	if err != nil {
		t.Fatal(err)
	}
	txt := text(t, res)
	t.Log("\n" + txt)
	if !strings.HasPrefix(lastLine(txt), "[uso] modelo=") || !strings.Contains(txt, "prompt_eval_count=") {
		t.Fatal("usage line missing")
	}
}

func TestLive3Chat(t *testing.T) {
	res, err := call(t, liveDeps(t), "chat", `{"messages":[{"role":"user","content":"Responda apenas com a palavra: ok"}],"system":"Seja breve.","temperature":0,"num_predict":8}`)
	if err != nil {
		t.Fatal(err)
	}
	txt := text(t, res)
	t.Log("\n" + txt)
	if !strings.HasPrefix(lastLine(txt), "[uso] modelo=") {
		t.Fatal("usage line missing")
	}
}

func TestLive4Embed(t *testing.T) {
	res, err := call(t, liveDeps(t), "embed", `{"input":["texto curto de teste","outro texto"]}`)
	if err != nil {
		t.Fatal(err)
	}
	txt := text(t, res)
	t.Log("\n" + txt)
	if !strings.Contains(txt, "2 vetor(es) de 768 dimensões") {
		t.Fatal("expected 768 dimensions from nomic-embed-text")
	}
}
