package tools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/ollama"
)

func embedOK(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte(`{"model":"nomic-embed-text","embeddings":[[0.123456,0.5,0.25],[0.75,0.0625,0.5]],"total_duration":900000000,"load_duration":800000000,"prompt_eval_count":6}`))
}

func TestEmbedSummaryByDefault(t *testing.T) {
	m := newMock(t, embedOK)
	res, err := call(t, m.deps(t, nil), "embed", `{"input":["um","dois"]}`)
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	txt := text(t, res)
	if !strings.Contains(txt, "2 vetor(es) de 3 dimensões, modelo nomic-embed-text") {
		t.Fatalf("summary: %q", txt)
	}
	// TOK-008: no vectors unless asked.
	if strings.Contains(txt, "0.123456") || strings.Contains(txt, "[[") {
		t.Fatalf("vectors leaked by default: %q", txt)
	}
	if got := lastLine(txt); got != "[uso] modelo=nomic-embed-text prompt_eval_count=6 total_ms=900 load_ms=800 queue_ms=0" {
		t.Fatalf("usage line %q", got)
	}
	if _, has := res.Meta["mcpollama/usage"].(map[string]any)["eval_count"]; has {
		t.Fatal("embed has no eval_count")
	}
	// CFG-004: the embeddings model, never the generation model.
	b := m.body(0)
	if b["model"] != "nomic-embed-text" {
		t.Fatalf("model on the wire: %+v", b)
	}
	if in, _ := b["input"].([]any); len(in) != 2 {
		t.Fatalf("batch input: %+v", b["input"])
	}
}

func TestEmbedStringInputAndVectors(t *testing.T) {
	m := newMock(t, embedOK)
	d := m.deps(t, nil)
	res, err := call(t, d, "embed", `{"input":"so um","include_vectors":true,"model":"outro-embed"}`)
	if err != nil {
		t.Fatal(err)
	}
	b := m.body(0)
	if in, _ := b["input"].([]any); len(in) != 1 || in[0] != "so um" || b["model"] != "outro-embed" {
		t.Fatalf("wire: %+v", b)
	}
	txt := text(t, res)
	if !strings.Contains(txt, "[[0.123456,0.5,0.25],[0.75,0.0625,0.5]]") {
		t.Fatalf("vectors missing: %q", txt)
	}
}

func TestEmbedValidation(t *testing.T) {
	m := newMock(t, embedOK)
	d := m.deps(t, nil)
	many := `[` + strings.TrimSuffix(strings.Repeat(`"a",`, maxVectorItems+1), ",") + `]`
	cases := map[string]string{
		`{}`:                                    "`input` is required",
		`{"input":5}`:                           "`input` must be a string or an array of strings",
		`{"input":["a",5]}`:                     "`input[1]` must be a string",
		`{"input":""}`:                          "não pode ser vazio",
		`{"input":["a",""]}`:                    "`input[1]` não pode ser vazio",
		`{"input":"a","include_vectors":"sim"}`: "`include_vectors` must be a boolean",
		`{"input":` + many + `,"include_vectors":true}`: "include_vectors aceita no máximo",
		`{"input":"a","dimensions":3}`:                  "argumento desconhecido",
	}
	for in, want := range cases {
		if _, err := call(t, d, "embed", in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s -> %v (want %q)", in, err, want)
		}
	}
	if m.hits.Load() != 0 {
		t.Fatalf("invalid embed reached Ollama: %d", m.hits.Load())
	}
	// OLL-011: an empty list reaches the client, which names the failure.
	if _, err := call(t, d, "embed", `{"input":[]}`); err == nil || !strings.Contains(err.Error(), "no embeddings returned") {
		t.Fatalf("empty list: %v", err)
	}
	// A batch above the vector ceiling is fine for the summary.
	if _, err := call(t, d, "embed", `{"input":`+many+`}`); err != nil {
		t.Fatalf("summary of a larger batch: %v", err)
	}
}

func TestEmbedOllamaErrors(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, `{"error":"model not found"}`, 404) })
	if _, err := call(t, m.deps(t, nil), "embed", `{"input":"a"}`); !errors.Is(err, ollama.ErrNotFound) {
		t.Fatal(err)
	}
	m2 := newMock(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"embeddings":[]}`)) })
	if _, err := call(t, m2.deps(t, nil), "embed", `{"input":"a"}`); err == nil || !strings.Contains(err.Error(), "no embeddings returned") {
		t.Fatal(err)
	}
}

func tagsPS(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/tags":
		w.Write([]byte(`{"models":[{"name":"qwen3-ia","size":21474836480,"details":{"parameter_size":"27B","quantization_level":"Q4_K_M"}},{"name":"nomic-embed-text","size":274000000,"details":{"parameter_size":"137M","quantization_level":"F16"}}]}`))
	case "/api/ps":
		w.Write([]byte(`{"models":[{"name":"qwen3-ia","size_vram":12884901888,"context_length":65536,"expires_at":"2026-10-23T21:00:00Z"}]}`))
	default:
		http.NotFound(w, r)
	}
}

func TestListModels(t *testing.T) {
	m := newMock(t, tagsPS)
	res, err := call(t, m.deps(t, nil), "list_models", `{}`)
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	txt := text(t, res)
	for _, want := range []string{"Instalados (2):", "- qwen3-ia  20.0 GiB  27B Q4_K_M", "- nomic-embed-text", "Em VRAM (1):",
		"vram 12.0 GiB  contexto 65536", "geração=qwen3-ia embeddings=nomic-embed-text"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "[uso]") {
		t.Fatal("list_models does not call the model: no usage line")
	}
	if len(m.paths) != 2 || m.paths[0] != "/api/tags" || m.paths[1] != "/api/ps" {
		t.Fatalf("endpoints: %v", m.paths)
	}
	// Null arguments and the empty object are the same call.
	if _, err := call(t, m.deps(t, nil), "list_models", ``); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, m.deps(t, nil), "list_models", `{"x":1}`); err == nil {
		t.Fatal("list_models takes no arguments")
	}
}

func TestListModelsPSFailureIsPartialTagsFailureIsError(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ps" {
			http.Error(w, "x", 500)
			return
		}
		tagsPS(w, r)
	})
	res, err := call(t, m.deps(t, nil), "list_models", `{}`)
	if err != nil || res.IsError || !strings.Contains(text(t, res), "Em VRAM: indisponível") || !strings.Contains(text(t, res), "Instalados (2)") {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	m2 := newMock(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "x", 401) })
	if _, err := call(t, m2.deps(t, nil), "list_models", `{}`); !errors.Is(err, ollama.ErrUnauthorized) {
		t.Fatal(err)
	}
	empty := newMock(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"models":[]}`)) })
	res, err = call(t, empty.deps(t, nil), "list_models", `{}`)
	if err != nil || !strings.Contains(text(t, res), "(nenhum)") || !strings.Contains(text(t, res), "carga a frio") {
		t.Fatalf("empty lists: %v %q", err, text(t, res))
	}
}

// list_models must answer while a generation owns the GPU queue: it is what one
// reads to see what is loaded.
func TestListModelsBypassesTheGPUQueue(t *testing.T) {
	m := newMock(t, tagsPS)
	d := m.deps(t, nil)
	release, _, err := d.Queue.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() { _, err := call(t, d, "list_models", `{}`); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("list_models waited in the GPU queue")
	}
}
