package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
)

// syncBuf is a goroutine-safe sink the tests can poll while Run is writing.
type syncBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuf) lines() []string {
	out := strings.TrimSpace(s.String())
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// hasID reports whether a response with this raw id was written.
func (s *syncBuf) hasID(id string) bool {
	for _, l := range s.lines() {
		var r struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal([]byte(l), &r) == nil && string(r.ID) == id {
			return true
		}
	}
	return false
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// pipeRun starts Run on a pipe so a test can feed lines while it is running.
type pipeRun struct {
	in     *io.PipeWriter
	out    *syncBuf
	errw   *syncBuf
	result chan error
}

func startRun(t *testing.T, cfg Config, tools []ToolEntry) *pipeRun {
	t.Helper()
	pr, pw := io.Pipe()
	p := &pipeRun{in: pw, out: &syncBuf{}, errw: &syncBuf{}, result: make(chan error, 1)}
	go func() { p.result <- Run(context.Background(), pr, p.out, p.errw, cfg, tools) }()
	t.Cleanup(func() { pw.Close() })
	return p
}

func (p *pipeRun) send(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(p.in, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

// finish closes stdin and waits for Run, which must answer what is running.
func (p *pipeRun) finish(t *testing.T) {
	t.Helper()
	p.in.Close()
	select {
	case err := <-p.result:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after EOF")
	}
}

func callLine(id, tool string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"tools/call","params":{"name":%q,"arguments":{}}}`, id, tool)
}

// blocker is a tool that signals when it starts and ends when released or when
// its context is cancelled.
type blocker struct {
	started chan struct{}
	release chan struct{}
	ctxDone chan struct{}
}

func newBlocker() *blocker {
	return &blocker{started: make(chan struct{}, 8), release: make(chan struct{}), ctxDone: make(chan struct{}, 8)}
}

func (b *blocker) entry(name string) ToolEntry {
	return ToolEntry{Def: mcp.Tool{Name: name}, Handler: func(ctx context.Context, _ json.RawMessage) (mcp.ToolCallResult, error) {
		b.started <- struct{}{}
		select {
		case <-b.release:
			return mcp.TextResult("done", false), nil
		case <-ctx.Done():
			b.ctxDone <- struct{}{}
			return mcp.ToolCallResult{}, ctx.Err()
		}
	}}
}

// PB5-9 / PERF-012: ping and tools/list answer while a tool is running.
func TestPingAndToolsListAnswerWhileAToolRuns(t *testing.T) {
	b := newBlocker()
	p := startRun(t, testConfig(), []ToolEntry{b.entry("slow")})
	p.send(t, callLine("1", "slow"))
	<-b.started
	p.send(t, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	p.send(t, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	eventually(t, "ping and tools/list answered", func() bool { return p.out.hasID("2") && p.out.hasID("3") })
	if p.out.hasID("1") {
		t.Fatal("the running call answered before being released")
	}
	close(b.release)
	p.finish(t)
	if !p.out.hasID("1") {
		t.Fatal("the call was never answered")
	}
}

// MCP cancellation: the context is cancelled and no response is sent.
func TestCancelledNotificationCancelsContextAndSuppressesResponse(t *testing.T) {
	for _, id := range []string{"7", `"abc"`} {
		t.Run(id, func(t *testing.T) {
			b := newBlocker()
			p := startRun(t, testConfig(), []ToolEntry{b.entry("slow")})
			p.send(t, callLine(id, "slow"))
			<-b.started
			p.send(t, fmt.Sprintf(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":%s,"reason":"user"}}`, id))
			select {
			case <-b.ctxDone:
			case <-time.After(3 * time.Second):
				t.Fatal("the tool context was not cancelled")
			}
			p.send(t, `{"jsonrpc":"2.0","id":99,"method":"ping"}`)
			eventually(t, "ping after cancel", func() bool { return p.out.hasID("99") })
			p.finish(t)
			if p.out.hasID(id) {
				t.Fatalf("a cancelled request got a response: %s", p.out.String())
			}
			if !strings.Contains(p.errw.String(), "status=cancelled") {
				t.Fatalf("cancellation not logged: %s", p.errw.String())
			}
		})
	}
}

// 7 (number) and "7" (string) are different requests; unknown ids are ignored.
func TestCancelOfAnotherIDOrUnknownIDIsIgnored(t *testing.T) {
	b := newBlocker()
	p := startRun(t, testConfig(), []ToolEntry{b.entry("slow")})
	p.send(t, callLine("7", "slow"))
	<-b.started
	p.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"7"}}`)
	p.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":12345}}`)
	p.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":"garbage"}`)
	p.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled"}`)
	p.send(t, `{"jsonrpc":"2.0","id":99,"method":"ping"}`)
	eventually(t, "ping", func() bool { return p.out.hasID("99") })
	select {
	case <-b.ctxDone:
		t.Fatal("an unrelated cancel stopped the call")
	default:
	}
	close(b.release)
	p.finish(t)
	if !p.out.hasID("7") {
		t.Fatal("the call must still be answered")
	}
	for _, want := range []string{"notification:unknown_request", "notification:ignored"} {
		if !strings.Contains(p.errw.String(), want) {
			t.Errorf("log lacks %s:\n%s", want, p.errw.String())
		}
	}
}

func TestDuplicateInflightIDIsRejected(t *testing.T) {
	b := newBlocker()
	p := startRun(t, testConfig(), []ToolEntry{b.entry("slow")})
	p.send(t, callLine("5", "slow"))
	<-b.started
	p.send(t, callLine("5", "slow"))
	eventually(t, "duplicate rejected", func() bool { return strings.Contains(p.out.String(), "still running") })
	close(b.release)
	p.finish(t)
}

// EOF on stdin answers the calls still running before Run returns.
func TestEOFWaitsForRunningCalls(t *testing.T) {
	tool := ToolEntry{Def: mcp.Tool{Name: "slow"}, Handler: func(context.Context, json.RawMessage) (mcp.ToolCallResult, error) {
		time.Sleep(150 * time.Millisecond)
		return mcp.TextResult("late", false), nil
	}}
	lines, _ := run(t, testConfig(), []ToolEntry{tool}, callLine("1", "slow")+"\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"text":"late"`) {
		t.Fatalf("got %q", lines)
	}
}

// -race: many concurrent calls, every response is one whole JSON line.
func TestManyConcurrentCallsAreAllAnsweredIntact(t *testing.T) {
	const n = 60
	tool := ToolEntry{Def: mcp.Tool{Name: "work"}, Handler: func(_ context.Context, _ json.RawMessage) (mcp.ToolCallResult, error) {
		time.Sleep(time.Duration(5+n%7) * time.Millisecond)
		return mcp.TextResult(strings.Repeat("x", 2000), false), nil
	}}
	var in strings.Builder
	for i := 1; i <= n; i++ {
		in.WriteString(callLine(fmt.Sprint(i), "work") + "\n")
		in.WriteString(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping"}`+"\n", 1000+i))
	}
	lines, _ := run(t, testConfig(), []ToolEntry{tool}, in.String())
	if len(lines) != 2*n {
		t.Fatalf("got %d responses, want %d", len(lines), 2*n)
	}
	seen := map[string]bool{}
	for _, l := range lines {
		r := decode(t, l) // fails on a torn line
		if seen[string(r.ID)] {
			t.Fatalf("id %s answered twice", r.ID)
		}
		seen[string(r.ID)] = true
	}
	for i := 1; i <= n; i++ {
		if !seen[fmt.Sprint(i)] || !seen[fmt.Sprint(1000+i)] {
			t.Fatalf("missing response for %d", i)
		}
	}
}

// A handler that fails after the context was cancelled by the client does not
// produce a second message; a normal failure is still a result with isError.
func TestToolErrorStillAnswersWhenNotCancelled(t *testing.T) {
	tool := ToolEntry{Def: mcp.Tool{Name: "bad"}, Handler: func(context.Context, json.RawMessage) (mcp.ToolCallResult, error) {
		return mcp.ToolCallResult{}, fmt.Errorf("falhou")
	}}
	lines, errw := run(t, testConfig(), []ToolEntry{tool}, callLine("1", "bad")+"\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"isError":true`) || !strings.Contains(errw, "status=tool_error") {
		t.Fatalf("lines=%q log=%s", lines, errw)
	}
}

// OBS-001: the call record carries the counters from _meta, never content.
func TestCallLogCarriesCountersFromMeta(t *testing.T) {
	tool := ToolEntry{Def: mcp.Tool{Name: "m"}, Handler: func(context.Context, json.RawMessage) (mcp.ToolCallResult, error) {
		r := mcp.TextResult("CONTEUDO-SECRETO", false)
		r.Meta = map[string]any{"mcpollama/usage": map[string]any{"prompt_eval_count": 11, "eval_count": 4, "total_ms": int64(900), "load_ms": int64(0), "queue_ms": int64(3)}}
		return r, nil
	}}
	_, errw := run(t, testConfig(), []ToolEntry{tool}, callLine("1", "m")+"\n")
	for _, want := range []string{"method=tools/call", "tool=m", "status=ok", "prompt_eval_count=11", "eval_count=4", "queue_ms=3"} {
		if !strings.Contains(errw, want) {
			t.Errorf("log lacks %q: %s", want, errw)
		}
	}
	if strings.Contains(errw, "CONTEUDO-SECRETO") {
		t.Fatal("content in the log")
	}
}

// The text returned by a tool is redacted by the dispatcher as a last defence.
func TestResultTextIsRedactedByTheDispatcher(t *testing.T) {
	const token = "tok-DISPATCH-123"
	tool := ToolEntry{Def: mcp.Tool{Name: "echo"}, Handler: func(context.Context, json.RawMessage) (mcp.ToolCallResult, error) {
		return mcp.TextResult("the model said "+token, false), nil
	}}
	cfg := testConfig()
	cfg.APIKey = token
	lines, _ := run(t, cfg, []ToolEntry{tool}, callLine("1", "echo")+"\n")
	if len(lines) != 1 || strings.Contains(lines[0], token) || !strings.Contains(lines[0], "[REDACTED]") {
		t.Fatalf("got %q", lines)
	}
}
