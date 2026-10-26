package tools

import (
	"net/http"
	"strings"
	"testing"
)

func TestModelAllowedLatestAndCase(t *testing.T) {
	l := []string{"qwen3-ia", "nomic-embed-text:latest"}
	for name, want := range map[string]bool{
		"qwen3-ia": true, "qwen3-ia:latest": true, "nomic-embed-text": true, "nomic-embed-text:latest": true,
		"Qwen3-IA": false, "qwen3-ia:7b": false, "qwen3.8:27b-64k": false, "": false,
	} {
		if got := ModelAllowed(l, name); got != want {
			t.Errorf("ModelAllowed(%q)=%v want %v", name, got, want)
		}
	}
}

func TestSplitModels(t *testing.T) {
	got := SplitModels(" a ; b,c;;a , ")
	if strings.Join(got, "|") != "a|b|c" {
		t.Fatalf("%v", got)
	}
	if len(SplitModels(" ; ,")) != 0 {
		t.Fatal("blank list must be empty")
	}
}

// Every tool with a `model` argument refuses a model outside the allowlist
// with an isError-bound error and never reaches Ollama (0 hits).
func TestDisallowedModelNeverCallsOllama(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	d = newFileEnv(t).deps(t, m, 128000, 40)
	cases := map[string]string{
		"generate":       `{"prompt":"x","model":"qwen3.8:27b-64k"}`,
		"chat":           `{"messages":[{"role":"user","content":"x"}],"model":"qwen3.8:27b-64k"}`,
		"embed":          `{"input":"x","model":"qwen3.8:27b-64k"}`,
		"summarize_file": `{"path":"a.txt","model":"qwen3.8:27b-64k"}`,
		"extract":        `{"path":"a.txt","instruction":"x","model":"qwen3.8:27b-64k"}`,
		"translate":      `{"path":"a.txt","target_language":"en","model":"qwen3.8:27b-64k"}`,
	}
	for name, a := range cases {
		_, err := call(t, d, name, a)
		if err == nil || !strings.Contains(err.Error(), "modelo qwen3.8:27b-64k não permitido neste servidor; permitidos: qwen3-ia, nomic-embed-text") {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	if n := m.hits.Load(); n != 0 {
		t.Fatalf("Ollama received %d requests, want 0", n)
	}
}

func TestAllowedModelPassesThroughWithLatest(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	if _, err := call(t, d, "generate", `{"prompt":"x","model":"qwen3-ia:latest"}`); err != nil {
		t.Fatal(err)
	}
	if m.hits.Load() != 1 {
		t.Fatalf("hits=%d", m.hits.Load())
	}
}

func TestExplicitAllowlist(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	d.AllowedModels = []string{"qwen3-ia", "outro"}
	if _, err := call(t, d, "generate", `{"prompt":"x","model":"outro"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, d, "generate", `{"prompt":"x","model":"nomic-embed-text"}`); err == nil {
		t.Fatal("nomic-embed-text is not in the explicit list")
	}
}

func TestListModelsFiltersAndShowsOutsideVRAM(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Write([]byte(`{"models":[{"name":"qwen3-ia:latest","size":1},{"name":"qwen3.8:27b-64k","size":2},{"name":"qwen3-coder:30b-a3b-q4_K_M","size":3}]}`))
		case "/api/ps":
			w.Write([]byte(`{"models":[{"name":"qwen3.8:27b-64k","size_vram":8589934592,"context_length":65536}]}`))
		}
	})
	res, err := call(t, m.deps(t, nil), "list_models", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	s := text(t, res)
	if !strings.Contains(s, "Instalados (1):") || !strings.Contains(s, "- qwen3-ia:latest") {
		t.Fatalf("installed: %s", s)
	}
	if strings.Contains(s, "- qwen3.8:27b-64k") || strings.Contains(s, "qwen3-coder") {
		t.Fatalf("disallowed model offered: %s", s)
	}
	if !strings.Contains(s, "Em VRAM (0):") || !strings.Contains(s, "Fora da allowlist (ocupa VRAM): qwen3.8:27b-64k (vram 8.0 GiB)") {
		t.Fatalf("vram: %s", s)
	}
}
