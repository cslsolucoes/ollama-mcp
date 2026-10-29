package tools

import (
	"net/http"
	"strings"
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

func assertThink(t *testing.T, m *mock, n int, want bool) {
	t.Helper()
	v, ok := m.body(n)["think"]
	if !ok || v != want {
		t.Fatalf("request %d: want \"think\":%v on the wire, got %v (present=%v)", n, want, v, ok)
	}
}

func chatOKWith(reason, content string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"model":"qwen3-ia","message":{"role":"assistant","content":"`+content+`"},"done":true,"done_reason":"`+reason+`","prompt_eval_count":3,"eval_count":1}`)
	}
}

// Plan 8.8b: `think` is in the schema of the five generating tools only.
func TestThinkInSchemaOfGeneratingTools(t *testing.T) {
	with := map[string]bool{"generate": true, "chat": true, "summarize_file": true, "extract": true, "translate": true}
	for _, def := range Defs(New(CatalogDeps())) {
		props, _ := def.InputSchema["properties"].(schemaMap)
		p, has := props["think"].(schemaMap)
		if has != with[def.Name] {
			t.Errorf("%s: think in schema = %v, want %v", def.Name, has, with[def.Name])
		}
		if has && p["type"] != "boolean" {
			t.Errorf("%s: think type = %v, want boolean", def.Name, p["type"])
		}
	}
}

func TestThinkTrueReachesTheWire(t *testing.T) {
	m := newMock(t, generateOK)
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"oi","think":true}`)
	if err != nil {
		t.Fatal(err)
	}
	assertThink(t, m, 0, true)
	if !strings.HasSuffix(lastLine(text(t, res)), " think=on") {
		t.Fatalf("usage line without think=on: %q", lastLine(text(t, res)))
	}

	mc := newMock(t, chatOKWith("stop", "ok"))
	res, err = call(t, mc.deps(t, nil), "chat", `{"messages":[{"role":"user","content":"oi"}],"think":true}`)
	if err != nil {
		t.Fatal(err)
	}
	assertThink(t, mc, 0, true)
	if !strings.Contains(lastLine(text(t, res)), " think=on") {
		t.Fatalf("chat usage line without think=on: %q", lastLine(text(t, res)))
	}
}

func TestThinkOffKeepsUsageLine(t *testing.T) {
	m := newMock(t, generateOK)
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"oi","think":false}`)
	if err != nil {
		t.Fatal(err)
	}
	assertThinkFalse(t, m, 0)
	if strings.Contains(text(t, res), "think=") {
		t.Fatalf("usage line must not mention think when off: %q", lastLine(text(t, res)))
	}
}

func TestThinkMustBeBoolean(t *testing.T) {
	m := newMock(t, generateOK)
	if _, err := call(t, m.deps(t, nil), "generate", `{"prompt":"oi","think":"sim"}`); err == nil || !strings.Contains(err.Error(), "think") {
		t.Fatalf("want a type error for think, got %v", err)
	}
	if m.hits.Load() != 0 {
		t.Fatal("an invalid think must not reach the server")
	}
}

// Every call of a map-reduce request carries think (parts and the combine).
func TestFileToolThinkOnAllParts(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "grande.txt", strings.Repeat("linha de texto do arquivo grande\n", 120))
	m := newMock(t, generateOK)
	d := e.deps(t, m, 1500, 40)
	res, err := call(t, d, "summarize_file", `{"path":"grande.txt","think":true}`)
	if err != nil {
		t.Fatal(err)
	}
	n := len(m.bodies)
	if n < 3 {
		t.Fatalf("want 2+ parts plus the combine, got %d calls", n)
	}
	for i := 0; i < n; i++ {
		assertThink(t, m, i, true)
	}
	if !strings.Contains(lastLine(text(t, res)), " think=on") {
		t.Fatalf("usage line without think=on: %q", lastLine(text(t, res)))
	}

	m2 := newMock(t, generateOK)
	d2 := e.deps(t, m2, 1500, 40)
	if _, err := call(t, d2, "summarize_file", `{"path":"grande.txt"}`); err != nil {
		t.Fatal(err)
	}
	for i := range m2.bodies {
		assertThinkFalse(t, m2, i)
	}
}

func TestThinkExhaustedByReasoning(t *testing.T) {
	empty := func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"model":"qwen3-ia","response":"","thinking":"raciocinando","done":true,"done_reason":"length","prompt_eval_count":3,"eval_count":50}`)
	}
	m := newMock(t, empty)
	_, err := call(t, m.deps(t, nil), "generate", `{"prompt":"oi","think":true,"num_predict":50}`)
	if err == nil || !strings.Contains(err.Error(), "raciocínio") || !strings.Contains(err.Error(), "num_predict") {
		t.Fatalf("want the think-exhausted error, got %v", err)
	}

	mc := newMock(t, chatOKWith("length", ""))
	if _, err := call(t, mc.deps(t, nil), "chat", `{"messages":[{"role":"user","content":"oi"}],"think":true}`); err == nil || !strings.Contains(err.Error(), "raciocínio") {
		t.Fatalf("chat: want the think-exhausted error, got %v", err)
	}

	// Without think the current behaviour holds: an empty answer is a result.
	m2 := newMock(t, empty)
	res, err := call(t, m2.deps(t, nil), "generate", `{"prompt":"oi","num_predict":50}`)
	if err != nil {
		t.Fatalf("without think: %v", err)
	}
	if !strings.Contains(lastLine(text(t, res)), "fim=length") {
		t.Fatalf("usage: %q", lastLine(text(t, res)))
	}

	// Think on with a non-empty answer cut by length is still a result.
	m3 := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"model":"qwen3-ia","response":"parcial","done":true,"done_reason":"length","prompt_eval_count":3,"eval_count":50}`)
	})
	if _, err := call(t, m3.deps(t, nil), "generate", `{"prompt":"oi","think":true,"num_predict":50}`); err != nil {
		t.Fatalf("non-empty answer: %v", err)
	}
}

func TestFileToolThinkExhausted(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "nota.txt", "Primeira linha\n")
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"model":"qwen3-ia","response":"","done":true,"done_reason":"length","eval_count":9}`)
	})
	_, err := call(t, e.deps(t, m, 128000, 40), "extract", `{"path":"nota.txt","instruction":"x","think":true}`)
	if err == nil || !strings.Contains(err.Error(), "raciocínio") {
		t.Fatalf("want the think-exhausted error, got %v", err)
	}
}
