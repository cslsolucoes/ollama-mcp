package tools

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/ollama"
)

// Phase 9.4 (plan section 7): the tools tag the call with the client label and
// the tool name, read the server queue wait from X-Queue-Ms and handle the
// gateway answers 503+Retry-After and 409.

func TestGatewayHeadersAndQueueMS(t *testing.T) {
	var client, tool atomic.Value
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		client.Store(r.Header.Get("X-Mcpollama-Client"))
		tool.Store(r.Header.Get("X-Mcpollama-Tool"))
		w.Header().Set("X-Queue-Ms", "1234")
		generateOK(w, r)
	})
	d := m.deps(t, func(o *ollama.Options) { o.ClientLabel = "estacao 7/pid#42" })
	res, err := call(t, d, "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if client.Load() != "estacao_7/pid_42" {
		t.Fatalf("client header %q (odd characters must become _)", client.Load())
	}
	if tool.Load() != "generate" {
		t.Fatalf("tool header %q", tool.Load())
	}
	// The server wait goes into queue_ms of the [uso] line.
	if !strings.Contains(lastLine(text(t, res)), "queue_ms=1234") {
		t.Fatalf("usage line: %q", lastLine(text(t, res)))
	}
}

func TestEmbedAndChatCarryToolAndQueue(t *testing.T) {
	var tools []string
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		tools = append(tools, r.Header.Get("X-Mcpollama-Tool"))
		w.Header().Set("X-Queue-Ms", "50")
		if r.URL.Path == "/api/embed" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"nomic-embed-text","embeddings":[[0.1,0.2]],"total_duration":1,"load_duration":1,"prompt_eval_count":2}`))
			return
		}
		streamLines(w, `{"model":"qwen3-ia","message":{"role":"assistant","content":"oi"},"done":true,"done_reason":"stop","eval_count":1,"prompt_eval_count":1}`)
	})
	d := m.deps(t, nil)
	r1, err := call(t, d, "embed", `{"input":["a"]}`)
	if err != nil || !strings.Contains(lastLine(text(t, r1)), "queue_ms=50") {
		t.Fatalf("embed: %v %q", err, text(t, r1))
	}
	r2, err := call(t, d, "chat", `{"messages":[{"role":"user","content":"oi"}]}`)
	if err != nil || !strings.Contains(lastLine(text(t, r2)), "queue_ms=50") {
		t.Fatalf("chat: %v %q", err, text(t, r2))
	}
	if len(tools) != 2 || tools[0] != "embed" || tools[1] != "chat" {
		t.Fatalf("tools = %v", tools)
	}
}

func TestQueueFullRetriesOnceThenSucceeds(t *testing.T) {
	var n atomic.Int32
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":"fila do servidor cheia"}`))
			return
		}
		generateOK(w, r)
	})
	start := time.Now()
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if n.Load() != 2 || time.Since(start) < 900*time.Millisecond {
		t.Fatalf("hits=%d after %v: want one retry after ~1 s", n.Load(), time.Since(start))
	}
}

func TestQueueFullGivesUpAfterOneRetry(t *testing.T) {
	var n atomic.Int32
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
	})
	_, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "fila do servidor está cheia") {
		t.Fatalf("err = %v", err)
	}
	if n.Load() != 2 {
		t.Fatalf("hits = %d, want exactly one retry", n.Load())
	}
}

func TestQueueFullNoRetryWhenItDoesNotFitTheTimeout(t *testing.T) {
	var n atomic.Int32
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(503)
	})
	d := m.deps(t, func(o *ollama.Options) { o.Timeout = 5 * time.Second })
	start := time.Now()
	_, err := call(t, d, "generate", `{"prompt":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "~30 s") {
		t.Fatalf("err = %v", err)
	}
	if n.Load() != 1 || time.Since(start) > 2*time.Second {
		t.Fatalf("hits=%d after %v: the retry did not fit and must not happen", n.Load(), time.Since(start))
	}
}

func TestBare503IsNotRetried(t *testing.T) {
	var n atomic.Int32
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(503) // the Ollama itself: no Retry-After
	})
	_, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err == nil || n.Load() != 1 {
		t.Fatalf("err=%v hits=%d", err, n.Load())
	}
}

func TestRemovedByAdministratorIsClearAndNotRetried(t *testing.T) {
	var n atomic.Int32
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"removido da fila pelo administrador"}`))
	})
	_, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "administrador do servidor removeu ou cancelou") || n.Load() != 1 {
		t.Fatalf("err=%v hits=%d", err, n.Load())
	}
}

func TestAdminCancelMidStreamShowsTheServerMessage(t *testing.T) {
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		streamLines(w,
			`{"model":"qwen3-ia","response":"par","done":false}`,
			`{"error":"cancelado pelo administrador"}`)
	})
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	// a partial result with the error text, or an error: either way the message is clear
	msg := ""
	if err != nil {
		msg = err.Error()
	} else {
		msg = text(t, res)
	}
	if !strings.Contains(msg, "cancelado pelo administrador") {
		t.Fatalf("message = %q", msg)
	}
}
