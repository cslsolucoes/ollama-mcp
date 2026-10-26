package tools

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/ollama"
)

func TestGenerateSuccessUsageLineAndNoThinking(t *testing.T) {
	m := newMock(t, generateOK)
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"diga ola","system":"seja breve","temperature":0.2,"num_predict":50}`)
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	txt := text(t, res)
	if !strings.HasPrefix(txt, "Ola mundo!\n\n") {
		t.Fatalf("answer not aggregated: %q", txt)
	}
	// TOK-002: fixed last line with the counters.
	want := "[uso] modelo=qwen3-ia prompt_eval_count=17 eval_count=5 total_ms=2000 load_ms=1500 queue_ms=0 fim=stop"
	if got := lastLine(txt); got != want {
		t.Fatalf("usage line\n got %q\nwant %q", got, want)
	}
	// TOK-003: the same numbers in _meta.
	u, _ := res.Meta["mcpollama/usage"].(map[string]any)
	if u["prompt_eval_count"] != 17 || u["eval_count"] != 5 || u["load_ms"] != int64(1500) || u["done_reason"] != "stop" {
		t.Fatalf("meta: %+v", u)
	}
	// OLL-010: reasoning never reaches the client.
	if strings.Contains(txt, "PENSAMENTO") || strings.Contains(fmt.Sprint(res.Meta), "PENSAMENTO") {
		t.Fatal("thinking leaked")
	}
	// CFG-003 + TOOL-011: default model, system and sampling options on the wire.
	b := m.body(0)
	if b["model"] != "qwen3-ia" || b["system"] != "seja breve" || b["stream"] != true {
		t.Fatalf("wire body: %+v", b)
	}
	o, _ := b["options"].(map[string]any)
	if o["temperature"] != 0.2 || o["num_predict"] != float64(50) {
		t.Fatalf("options: %+v", o)
	}
	if m.auths[0] != "Bearer "+sentinel {
		t.Fatal("bearer header missing")
	}
}

func TestGenerateModelPerCall(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	d.AllowedModels = []string{"qwen3-ia", "outro:7b"}
	if _, err := call(t, d, "generate", `{"prompt":"x","model":"outro:7b"}`); err != nil {
		t.Fatal(err)
	}
	if m.body(0)["model"] != "outro:7b" {
		t.Fatalf("model per call ignored: %+v", m.body(0))
	}
}

func TestGenerateBadArgumentsNeverReachOllama(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	cases := map[string]string{
		`{}`:                                     "`prompt` is required",
		`{"prompt":123}`:                         "`prompt` must be a string",
		`{"prompt":"  "}`:                        "não pode ser vazio",
		`{"prompt":"x","system":5}`:              "`system` must be a string",
		`{"prompt":"x","model":[]}`:              "`model` must be a string",
		`{"prompt":"x","temperature":"hot"}`:     "`temperature` must be a number",
		`{"prompt":"x","temperature":3}`:         "`temperature` deve estar entre",
		`{"prompt":"x","num_predict":1.5}`:       "`num_predict` must be an integer",
		`{"prompt":"x","num_predict":0}`:         "`num_predict` deve estar entre",
		`{"prompt":"x","stream":true}`:           "argumento desconhecido",
		`{"prompt":"x","options":{"num_ctx":1}}`: "argumento desconhecido",
	}
	for in, want := range cases {
		_, err := call(t, d, "generate", in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s -> %v (want %q)", in, err, want)
		}
		if err != nil && strings.Contains(err.Error(), "is required") && in != `{}` {
			t.Errorf("%s: wrong type reported as required: %v", in, err)
		}
	}
	if m.hits.Load() != 0 {
		t.Fatalf("invalid arguments reached Ollama: %d requests", m.hits.Load())
	}
}

func TestGenerateOllamaErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   string
		kind   error
	}{
		{"401", 401, "token ausente ou inválido", ollama.ErrUnauthorized},
		{"403", 403, "bloqueado", ollama.ErrForbidden},
		{"404", 404, "não encontrado", ollama.ErrNotFound},
		{"500", 500, "erro do servidor", ollama.ErrServer},
		{"503", 503, "carga a frio", ollama.ErrServer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMock(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", tc.status) })
			_, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, tc.kind) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestGenerateConnectionDropped(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close() // the host restarting looks like a bare close
		}
	})
	_, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if !errors.Is(err, ollama.ErrConnectionDropped) || !strings.Contains(err.Error(), "encerrou a conexão") {
		t.Fatalf("err = %v", err)
	}
}

func TestGenerateMidStreamFailureKeepsPartialText(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"response":"comeco do texto","done":false}`, `{"error":"model runner has unexpectedly stopped"}`)
	})
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil {
		t.Fatalf("a partial answer is an isError result, not a Go error: %v", err)
	}
	txt := text(t, res)
	if !res.IsError || !strings.Contains(txt, "comeco do texto") || !strings.Contains(txt, "texto parcial") {
		t.Fatalf("res = %+v", res)
	}
}

func TestPromptCeilingAppliesToEveryTool(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, func(o *ollama.Options) { o.MaxPromptChars = 100 })
	big := strings.Repeat("a", 200)
	for name, in := range map[string]string{
		"generate": `{"prompt":"` + big + `"}`,
		"chat":     `{"messages":[{"role":"user","content":"` + big + `"}]}`,
		"embed":    `{"input":["` + big + `"]}`,
	} {
		_, err := call(t, d, name, in)
		if !errors.Is(err, ollama.ErrPromptTooLong) || !strings.Contains(err.Error(), "nada foi enviado") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if m.hits.Load() != 0 {
		t.Fatalf("oversized prompt reached Ollama: %d", m.hits.Load())
	}
}

func TestLongAnswerIsCappedAndUsageLineSurvives(t *testing.T) {
	long := strings.Repeat("x", maxOutputChars+500)
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"response":"`+long+`","done":true,"done_reason":"stop","prompt_eval_count":1,"eval_count":2}`)
	})
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	txt := text(t, res)
	if !strings.Contains(txt, "resposta truncada pelo servidor MCP") || !strings.HasPrefix(lastLine(txt), "[uso] ") {
		t.Fatalf("cap notice or usage line missing (len %d)", len(txt))
	}
	if strings.Count(txt, "x") != maxOutputChars {
		t.Fatalf("kept %d chars", strings.Count(txt, "x"))
	}
}

// PERF-001: a slow cold load is not a failure and shows up as load_ms; a real
// timeout names the cold load.
func TestColdLoadIsReportedAndTimeoutNamesIt(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		streamLines(w, `{"response":"ok","done":true,"done_reason":"stop","load_duration":36000000000,"total_duration":36500000000,"prompt_eval_count":3,"eval_count":1}`)
	})
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil || !strings.Contains(lastLine(text(t, res)), "load_ms=36000") {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	_, err = call(t, m.deps(t, func(o *ollama.Options) { o.Timeout = 50 * time.Millisecond }), "generate", `{"prompt":"x"}`)
	if !errors.Is(err, ollama.ErrTimeout) || !strings.Contains(err.Error(), "carga a frio") {
		t.Fatalf("timeout error: %v", err)
	}
}

func TestChatSuccessStatelessWithSystem(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w,
			`{"model":"qwen3-ia","message":{"role":"assistant","content":"Oi ","thinking":"RACIOCINIO"},"done":false}`,
			`{"model":"qwen3-ia","message":{"role":"assistant","content":"de volta"},"done":true,"done_reason":"stop","total_duration":1000000000,"load_duration":0,"prompt_eval_count":9,"eval_count":3}`)
	})
	d := m.deps(t, nil)
	res, err := call(t, d, "chat", `{"messages":[{"role":"user","content":"oi"},{"role":"assistant","content":"ola"},{"role":"user","content":"e ai"}],"system":"seja gentil"}`)
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	txt := text(t, res)
	if !strings.HasPrefix(txt, "Oi de volta\n\n") || strings.Contains(txt, "RACIOCINIO") {
		t.Fatalf("answer: %q", txt)
	}
	if got := lastLine(txt); got != "[uso] modelo=qwen3-ia prompt_eval_count=9 eval_count=3 total_ms=1000 load_ms=0 queue_ms=0 fim=stop" {
		t.Fatalf("usage line %q", got)
	}
	msgs, _ := m.body(0)["messages"].([]any)
	if len(msgs) != 4 || msgs[0].(map[string]any)["role"] != "system" || msgs[0].(map[string]any)["content"] != "seja gentil" {
		t.Fatalf("system must come first: %+v", msgs)
	}
	if m.body(0)["model"] != "qwen3-ia" {
		t.Fatalf("default model: %+v", m.body(0))
	}

	// No state on the server: the next call carries only its own messages.
	if _, err := call(t, d, "chat", `{"messages":[{"role":"user","content":"outra conversa"}]}`); err != nil {
		t.Fatal(err)
	}
	if msgs, _ := m.body(1)["messages"].([]any); len(msgs) != 1 {
		t.Fatalf("history leaked between calls: %+v", msgs)
	}
}

func TestChatValidation(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	manyMsgs := "[" + strings.TrimSuffix(strings.Repeat(`{"role":"user","content":"a"},`, maxChatMessages+1), ",") + "]"
	cases := map[string]string{
		`{}`:                  "`messages` is required",
		`{"messages":"oi"}`:   "`messages` must be an array",
		`{"messages":[]}`:     "não pode ser vazio",
		`{"messages":["oi"]}`: "`messages[0]` must be an object",
		`{"messages":[{"role":"robot","content":"x"}]}`:                       "role` inválido",
		`{"messages":[{"role":"tool","content":"x"}]}`:                        "role` inválido",
		`{"messages":[{"content":"x"}]}`:                                      "`role` is required",
		`{"messages":[{"role":5,"content":"x"}]}`:                             "`role` must be a string",
		`{"messages":[{"role":"user","content":""}]}`:                         "não pode ser vazio",
		`{"messages":[{"role":"user"}]}`:                                      "`content` is required",
		`{"messages":[{"role":"user","content":7}]}`:                          "`content` must be a string",
		`{"messages":[{"role":"user","content":"x","thinking":"y"}]}`:         "argumento desconhecido",
		`{"messages":[{"role":"user","content":"x"}],"system":9}`:             "`system` must be a string",
		`{"messages":[{"role":"user","content":"x"}],"temperature":"quente"}`: "`temperature` must be a number",
		`{"messages":` + manyMsgs + `}`:                                       "o máximo é",
	}
	for in, want := range cases {
		_, err := call(t, d, "chat", in)
		if err == nil || !strings.Contains(err.Error(), want) {
			short := in
			if len(short) > 90 {
				short = short[:90]
			}
			t.Errorf("%s -> %v (want %q)", short, err, want)
		}
	}
	if m.hits.Load() != 0 {
		t.Fatalf("invalid chat reached Ollama: %d", m.hits.Load())
	}
}

func TestChatOllamaErrorAndPartial(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "x", 500) })
	if _, err := call(t, m.deps(t, nil), "chat", `{"messages":[{"role":"user","content":"x"}]}`); !errors.Is(err, ollama.ErrServer) {
		t.Fatal(err)
	}
	m2 := newMock(t, func(w http.ResponseWriter, _ *http.Request) {
		streamLines(w, `{"message":{"role":"assistant","content":"meio"},"done":false}`, `{"error":"runner caiu"}`)
	})
	res, err := call(t, m2.deps(t, nil), "chat", `{"messages":[{"role":"user","content":"x"}]}`)
	if err != nil || !res.IsError || !strings.Contains(text(t, res), "meio") {
		t.Fatalf("err=%v res=%+v", err, res)
	}
}
