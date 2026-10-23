package ollama

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A real Ollama /api/generate NDJSON stream (shape measured in VAL §5), with a
// reasoning chunk that must never reach the caller (OLL-010).
const genStream = `{"model":"qwen3-ia","created_at":"2026-10-23T19:00:00Z","response":"","thinking":"SECRET-REASONING","done":false}
{"model":"qwen3-ia","created_at":"2026-10-23T19:00:00Z","response":"Olá","done":false}

{"model":"qwen3-ia","created_at":"2026-10-23T19:00:00Z","response":", mundo","done":false}
{"model":"qwen3-ia","created_at":"2026-10-23T19:00:01Z","response":"","done":true,"done_reason":"stop","context":[1,2,3],"total_duration":536000000,"load_duration":4000000,"prompt_eval_count":44,"prompt_eval_duration":64000000,"eval_count":41,"eval_duration":424000000}
`

func writeStream(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, l := range strings.SplitAfter(body, "\n") {
		fmt.Fprint(w, l)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func TestGenerateStream(t *testing.T) {
	var seen generateWire
	var rawBody string
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/generate" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		mustJSON(t, r, &seen)
		rawBody = fmt.Sprint(seen)
		writeStream(w, genStream)
	})
	temp, n := 0.0, 8
	got, err := newClient(t, srv, nil).Generate(context.Background(),
		GenerateRequest{Prompt: "diga olá", System: "seja breve", Options: &ModelOptions{Temperature: &temp, NumPredict: &n}})
	if err != nil {
		t.Fatal(err)
	}
	if !seen.Stream || seen.Model != "qwen3-ia" || seen.System != "seja breve" || seen.Options == nil || *seen.Options.NumPredict != 8 {
		t.Fatalf("request: %s", rawBody)
	}
	if got.Response != "Olá, mundo" || !got.Done || got.DoneReason != "stop" {
		t.Fatalf("aggregation: %+v", got)
	}
	if got.PromptEvalCount != 44 || got.EvalCount != 41 || got.TotalDuration != 536000000 || got.LoadDuration != 4000000 ||
		got.PromptEvalDuration != 64000000 || got.EvalDuration != 424000000 {
		t.Fatalf("counters: %+v", got.Usage)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "SECRET-REASONING") {
		t.Fatal("thinking leaked into the result")
	}
}

func TestGenerateNoStream(t *testing.T) {
	var seen generateWire
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		mustJSON(t, r, &seen)
		fmt.Fprint(w, `{"model":"qwen3-ia","response":"ok","thinking":"SECRET-REASONING","done":true,"done_reason":"length","prompt_eval_count":44,"eval_count":2,"eval_duration":5}`)
	})
	got, err := newClient(t, srv, func(o *Options) { o.DisableStream = true }).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Stream || got.Response != "ok" || got.DoneReason != "length" || got.EvalCount != 2 || got.PromptEvalCount != 44 {
		t.Fatalf("seen=%+v got=%+v", seen, got)
	}
}

func TestGenerateNoStreamErrorField(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"error":"llama runner exited"}`) })
	_, err := newClient(t, srv, func(o *Options) { o.DisableStream = true }).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	wantKind(t, err, ErrStream)
}

func TestChatStream(t *testing.T) {
	var seen chatWire
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		mustJSON(t, r, &seen)
		writeStream(w, `{"model":"qwen3-ia","message":{"role":"assistant","content":"","thinking":"SECRET-REASONING"},"done":false}
{"model":"qwen3-ia","message":{"role":"assistant","content":"Oi"},"done":false}
{"model":"qwen3-ia","message":{"role":"assistant","content":"!"},"done":false}
{"model":"qwen3-ia","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","total_duration":9,"load_duration":3,"prompt_eval_count":12,"prompt_eval_duration":4,"eval_count":2,"eval_duration":5}
`)
	})
	got, err := newClient(t, srv, nil).Chat(context.Background(), ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "oi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !seen.Stream || len(seen.Messages) != 1 || seen.Messages[0].Role != "user" {
		t.Fatalf("request: %+v", seen)
	}
	if got.Message.Content != "Oi!" || got.Message.Role != "assistant" || got.DoneReason != "stop" || got.PromptEvalCount != 12 || got.EvalCount != 2 {
		t.Fatalf("result: %+v", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "SECRET-REASONING") {
		t.Fatal("thinking leaked")
	}
	if _, err := newClient(t, srv, nil).Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("chat without messages must fail")
	}
}

func TestStreamErrorMidStream(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		writeStream(w, `{"response":"parte um ","done":false}
{"error":"llama runner process has terminated: exit status 2"}
`)
	})
	got, err := newClient(t, srv, nil).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	e := wantKind(t, err, ErrStream)
	if !strings.Contains(e.Msg, "llama runner process has terminated") {
		t.Fatalf("msg: %q", e.Msg)
	}
	if got == nil || got.Response != "parte um " {
		t.Fatalf("the partial text must be returned with the error: %+v", got)
	}
}

func TestStreamEOFWithoutDone(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		writeStream(w, `{"response":"corta","done":false}`+"\n")
	})
	got, err := newClient(t, srv, nil).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	e := wantKind(t, err, ErrStream)
	if !strings.Contains(e.Msg, "done:true") || got == nil || got.Response != "corta" {
		t.Fatalf("msg=%q got=%+v", e.Msg, got)
	}
}

func TestStreamEmptyBodyIsAnError(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {})
	got, err := newClient(t, srv, nil).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	wantKind(t, err, ErrStream)
	if got != nil {
		t.Fatal("no partial text exists, result must be nil")
	}
}

func TestStreamInvalidLine(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		writeStream(w, `{"response":"a","done":false}`+"\n"+`this is not json`+"\n")
	})
	_, err := newClient(t, srv, nil).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	wantKind(t, err, ErrBadResponse)
}

func TestStreamLineAboveBuffer(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		writeStream(w, `{"response":"`+strings.Repeat("a", 5000)+`","done":false}`+"\n")
	})
	_, err := newClient(t, srv, func(o *Options) { o.MaxLineBytes = 1024 }).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	e := wantKind(t, err, ErrBadResponse)
	if !strings.Contains(e.Msg, "--max-line-bytes") {
		t.Fatalf("msg: %q", e.Msg)
	}
}

// hijackClose drops the TCP connection without writing anything; the client
// sees EOF/RST, like the host restart of 2026-10-23.
func hijackClose(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Errorf("hijack: %v", err)
		return
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetLinger(0) // close sends RST
	}
	conn.Close()
}

func TestConnectionDroppedBeforeHeaders(t *testing.T) {
	srv, n := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(120 * time.Millisecond)
		hijackClose(t, w)
	})
	got, err := newClient(t, srv, nil).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	e := wantKind(t, err, ErrConnectionDropped)
	for _, want := range []string{"encerrou a conexão", "reiniciando", "carga a frio", "--max-prompt-chars"} {
		if !strings.Contains(e.Msg, want) {
			t.Errorf("message lacks %q: %s", want, e.Msg)
		}
	}
	if got != nil {
		t.Fatal("an empty response must never be returned as success")
	}
	if n.Load() != 1 {
		t.Fatalf("generation was retried: %d attempts", n.Load())
	}
}

func TestConnectionDroppedOnGet(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { hijackClose(t, w) })
	_, err := newClient(t, srv, nil).Tags(context.Background())
	wantKind(t, err, ErrConnectionDropped)
}

func TestConnectionDroppedMidStream(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		writeStream(w, `{"response":"antes da queda ","done":false}`+"\n")
		hijackClose(t, w)
	})
	got, err := newClient(t, srv, nil).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	e := wantKind(t, err, ErrConnectionDropped)
	if !strings.Contains(e.Msg, "no meio da resposta") {
		t.Fatalf("msg: %q", e.Msg)
	}
	if got == nil || got.Response != "antes da queda " {
		t.Fatalf("partial: %+v", got)
	}
}

func TestStreamTimeoutKeepsPartial(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		writeStream(w, `{"response":"começo ","done":false}`+"\n")
		<-r.Context().Done()
	})
	got, err := newClient(t, srv, func(o *Options) { o.Timeout = 150 * time.Millisecond }).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	wantKind(t, err, ErrTimeout)
	if got == nil || got.Response != "começo " {
		t.Fatalf("partial: %+v", got)
	}
}
