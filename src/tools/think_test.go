package tools

import (
	"net/http"
	"testing"
)

// assertThinkFalse requires the decoded request body to carry "think":false
// (key present, boolean false): plan 8.8, the project never wants reasoning.
func assertThinkFalse(t *testing.T, m *mock, n int) {
	t.Helper()
	v, ok := m.body(n)["think"]
	if !ok || v != false {
		t.Fatalf("request %d: want \"think\":false on the wire, got %v (present=%v)", n, v, ok)
	}
}

func TestGenerateSendsThinkFalse(t *testing.T) {
	m := newMock(t, generateOK)
	if _, err := call(t, m.deps(t, nil), "generate", `{"prompt":"oi"}`); err != nil {
		t.Fatal(err)
	}
	if m.paths[0] != "/api/generate" {
		t.Fatalf("path %q", m.paths[0])
	}
	assertThinkFalse(t, m, 0)
}

func TestChatSendsThinkFalse(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"model":"qwen3-ia","message":{"role":"assistant","content":"ok"},"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":1}`)
	})
	if _, err := call(t, m.deps(t, nil), "chat", `{"messages":[{"role":"user","content":"oi"}]}`); err != nil {
		t.Fatal(err)
	}
	if m.paths[0] != "/api/chat" {
		t.Fatalf("path %q", m.paths[0])
	}
	assertThinkFalse(t, m, 0)
}

func TestFileToolsSendThinkFalse(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "nota.txt", "Primeira linha\nSegunda linha\n")
	m := newMock(t, generateOK)
	d := e.deps(t, m, 128000, 40)
	if _, err := call(t, d, "summarize_file", `{"path":"nota.txt"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, d, "extract", `{"path":"nota.txt","instruction":"liste as linhas"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, d, "translate", `{"path":"nota.txt","target_language":"inglês"}`); err != nil {
		t.Fatal(err)
	}
	if got := len(m.bodies); got != 3 {
		t.Fatalf("want 3 requests, got %d", got)
	}
	for i := range m.bodies {
		assertThinkFalse(t, m, i)
	}
}

func TestEmbedDoesNotSendThink(t *testing.T) {
	m := newMock(t, embedOK)
	if _, err := call(t, m.deps(t, nil), "embed", `{"input":["um"]}`); err != nil {
		t.Fatal(err)
	}
	if m.paths[0] != "/api/embed" {
		t.Fatalf("path %q", m.paths[0])
	}
	if _, ok := m.body(0)["think"]; ok {
		t.Fatalf("/api/embed must not receive think: %+v", m.body(0))
	}
}
