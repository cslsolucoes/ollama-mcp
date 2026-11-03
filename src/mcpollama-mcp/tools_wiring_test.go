package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/tools"
)

const wiringToken = "tok-WIRING-4d5e6f"

// fakeOllama serves the native endpoints the tools use and counts requests.
type fakeOllama struct {
	srv  *httptest.Server
	hits atomic.Int32

	mu    sync.Mutex
	paths []string
}

func newFakeOllama(t *testing.T, generate http.HandlerFunc) *fakeOllama {
	t.Helper()
	f := &fakeOllama{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		io.Copy(io.Discard, r.Body)
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		switch r.URL.Path {
		case "/api/tags":
			w.Write([]byte(`{"models":[{"name":"qwen3-ia","size":1073741824,"details":{"parameter_size":"27B","quantization_level":"Q4_K_M"}}]}`))
		case "/api/ps":
			w.Write([]byte(`{"models":[]}`))
		case "/api/embed":
			w.Write([]byte(`{"model":"nomic-embed-text","embeddings":[[0.1,0.2]],"prompt_eval_count":2}`))
		case "/api/generate":
			generate(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func answerOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	fmt.Fprintln(w, `{"model":"qwen3-ia","response":"RESPOSTA-DO-MODELO","thinking":"RACIOCINIO","done":false}`)
	fmt.Fprintln(w, `{"model":"qwen3-ia","response":"","done":true,"done_reason":"stop","total_duration":1000000000,"load_duration":0,"prompt_eval_count":12,"eval_count":3}`)
}

func rpc(id int, method, params string) string {
	if params == "" {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q}`, id, method)
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)
}

func toolCall(id int, name, args string) string {
	return rpc(id, "tools/call", fmt.Sprintf(`{"name":%q,"arguments":%s}`, name, args))
}

// runMain drives realMain over the given lines and returns stdout lines,
// stderr and the exit code.
func runMain(t *testing.T, args []string, e map[string]string, lines ...string) ([]string, string, int) {
	t.Helper()
	var out, errw syncBuf
	code := realMain(args, env(e), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, &errw)
	return out.lines(), errw.String(), code
}

func resultOf(t *testing.T, line string) (mcp.ToolCallResult, rawResp) {
	t.Helper()
	r := decode(t, line)
	var res mcp.ToolCallResult
	if r.Error == nil {
		if err := json.Unmarshal(r.Result, &res); err != nil {
			t.Fatalf("result: %v: %s", err, r.Result)
		}
	}
	return res, r
}

func byID(t *testing.T, lines []string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, l := range lines {
		m[string(decode(t, l).ID)] = l
	}
	return m
}

// PROTO-011 / PERF-012: handshake, ping and tools/list never reach Ollama,
// and the real client is built only by the first tool call.
func TestRealRegistryIsLazy(t *testing.T) {
	f := newFakeOllama(t, answerOK)
	args := []string{"--ollama-url", f.srv.URL}
	e := map[string]string{"OLLAMA_API_KEY": wiringToken}
	start := time.Now()
	lines, _, code := runMain(t, args, e, rpc(1, "initialize", ""), rpc(2, "ping", ""), rpc(3, "tools/list", ""))
	if code != 0 || len(lines) != 3 {
		t.Fatalf("exit %d, %d lines", code, len(lines))
	}
	if f.hits.Load() != 0 {
		t.Fatalf("Ollama was contacted by the handshake: %d requests", f.hits.Load())
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("handshake took %s", d)
	}
	runMain(t, args, e, toolCall(4, "list_models", `{}`))
	if f.hits.Load() != 2 {
		t.Fatalf("list_models must make 2 requests, got %d (%v)", f.hits.Load(), f.paths)
	}
}

// With no URL the handshake still works, and only the call that needs the
// backend fails, as a tool error with an instruction.
func TestMissingURLFailsOnlyTheToolCall(t *testing.T) {
	lines, _, code := runMain(t, nil, nil, rpc(1, "initialize", ""), rpc(2, "tools/list", ""), toolCall(3, "generate", `{"prompt":"x"}`))
	if code != 0 || len(lines) != 3 {
		t.Fatalf("exit %d, %d lines", code, len(lines))
	}
	m := byID(t, lines)
	if decode(t, m["1"]).Error != nil || decode(t, m["2"]).Error != nil {
		t.Fatal("handshake must not fail without a URL")
	}
	res, r := resultOf(t, m["3"])
	if r.Error != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "--ollama-url") {
		t.Fatalf("got %s", m["3"])
	}
}

// PROTO-009/010/018 on the real registry.
func TestErrorChannelsOnTheRealRegistry(t *testing.T) {
	f := newFakeOllama(t, answerOK)
	lines, _, _ := runMain(t, []string{"--ollama-url", f.srv.URL}, nil,
		toolCall(1, "nao_existe", `{}`),
		rpc(2, "tools/call", ``),
		toolCall(3, "generate", `{"prompt":42}`),
		toolCall(4, "generate", `{}`),
	)
	m := byID(t, lines)
	if r := decode(t, m["1"]); r.Error == nil || r.Error.Code != -32602 || !strings.Contains(r.Error.Message, "Unknown tool") {
		t.Fatalf("unknown tool: %s", m["1"])
	}
	if r := decode(t, m["2"]); r.Error == nil || r.Error.Code != -32602 {
		t.Fatalf("missing params: %s", m["2"])
	}
	for _, id := range []string{"3", "4"} {
		res, r := resultOf(t, m[id])
		if r.Error != nil || !res.IsError {
			t.Fatalf("bad argument must be a tool error (isError), got %s", m[id])
		}
	}
	if res, _ := resultOf(t, m["3"]); !strings.Contains(res.Content[0].Text, "`prompt` must be a string") {
		t.Fatalf("wrong-type message: %s", m["3"])
	}
	if f.hits.Load() != 0 {
		t.Fatalf("invalid calls reached Ollama (%d)", f.hits.Load())
	}
}

// End to end through the dispatcher, the lazy backend and the GPU queue: the
// usage line, _meta, no thinking, and OBS-001/002 + TEST-006 on stderr.
func TestEndToEndGenerateUsageAndNoLeaks(t *testing.T) {
	f := newFakeOllama(t, answerOK)
	const secretPrompt = "PROMPT-SECRETO-DO-USUARIO"
	lines, errw, code := runMain(t, []string{"--ollama-url", f.srv.URL, "--debug"}, map[string]string{"OLLAMA_API_KEY": wiringToken},
		toolCall(1, "generate", fmt.Sprintf(`{"prompt":%q,"system":"SISTEMA-SECRETO"}`, secretPrompt)),
		toolCall(2, "list_models", `{}`),
		toolCall(3, "embed", `{"input":["TEXTO-SECRETO"]}`),
	)
	if code != 0 || len(lines) != 3 {
		t.Fatalf("exit %d, lines %q", code, lines)
	}
	m := byID(t, lines)
	res, _ := resultOf(t, m["1"])
	txt := res.Content[0].Text
	if !strings.HasPrefix(txt, "RESPOSTA-DO-MODELO\n\n[uso] modelo=qwen3-ia prompt_eval_count=12 eval_count=3 total_ms=1000 load_ms=0 queue_ms=") ||
		strings.Contains(txt, "RACIOCINIO") {
		t.Fatalf("generate text: %q", txt)
	}
	if u, _ := res.Meta["mcpollama/usage"].(map[string]any); u["prompt_eval_count"] != float64(12) {
		t.Fatalf("meta over the wire: %+v", res.Meta)
	}
	if r, _ := resultOf(t, m["3"]); !strings.Contains(r.Content[0].Text, "1 vetor(es) de 2 dimensões") {
		t.Fatalf("embed: %s", m["3"])
	}
	// OBS-001: counters and metadata in the log.
	for _, want := range []string{"tool=generate", "status=ok", "prompt_eval_count=12", "eval_count=3", "tool=embed", "tool=list_models"} {
		if !strings.Contains(errw, want) {
			t.Errorf("log lacks %q:\n%s", want, errw)
		}
	}
	// OBS-002 / TEST-006: no content, no thinking, no token, even with --debug.
	for _, bad := range []string{secretPrompt, "SISTEMA-SECRETO", "TEXTO-SECRETO", "RESPOSTA-DO-MODELO", "RACIOCINIO", wiringToken} {
		if strings.Contains(errw, bad) {
			t.Errorf("stderr leaks %q:\n%s", bad, errw)
		}
	}
	for _, l := range lines {
		if strings.Contains(l, wiringToken) {
			t.Fatal("token on stdout")
		}
	}
	if !strings.Contains(errw, "debug: GET /api/tags") {
		t.Fatalf("--debug must log the exchanges (method, path, status):\n%s", errw)
	}
}

// The token echoed by a failing upstream reaches neither stdout nor stderr.
func TestUpstreamEchoingTheTokenIsScrubbed(t *testing.T) {
	f := newFakeOllama(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied for Bearer "+wiringToken, http.StatusInternalServerError)
	})
	lines, errw, _ := runMain(t, []string{"--ollama-url", f.srv.URL, "--debug"}, map[string]string{"OLLAMA_API_KEY": wiringToken},
		toolCall(1, "generate", `{"prompt":"x"}`))
	res, r := resultOf(t, lines[0])
	if r.Error != nil || !res.IsError {
		t.Fatalf("got %s", lines[0])
	}
	if strings.Contains(lines[0], wiringToken) || strings.Contains(errw, wiringToken) {
		t.Fatalf("token leaked:\nstdout=%s\nstderr=%s", lines[0], errw)
	}
}

// Connection dropped by the server: an isError result with the Phase 2 message.
func TestDroppedConnectionIsAToolError(t *testing.T) {
	f := newFakeOllama(t, func(w http.ResponseWriter, _ *http.Request) {
		if c, _, err := w.(http.Hijacker).Hijack(); err == nil {
			c.Close()
		}
	})
	lines, _, _ := runMain(t, []string{"--ollama-url", f.srv.URL}, nil, toolCall(1, "generate", `{"prompt":"x"}`))
	res, r := resultOf(t, lines[0])
	if r.Error != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "encerrou a conexão") ||
		!strings.Contains(res.Content[0].Text, "carga a frio") {
		t.Fatalf("got %s", lines[0])
	}
}

// PB5-9 end to end: while a generation holds the GPU queue, ping and
// list_models answer; a second generation waits and reports it.
func TestPingAndListModelsAnswerWhileGenerating(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	f := newFakeOllama(t, func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		answerOK(w, r)
	})
	p := startRun(t, testConfigWith(f.srv.URL), buildToolsFor(t, testConfigWith(f.srv.URL)))
	p.send(t, toolCall(1, "generate", `{"prompt":"primeira"}`))
	<-started
	p.send(t, toolCall(2, "generate", `{"prompt":"segunda"}`)) // waits in the queue
	p.send(t, rpc(3, "ping", ""))
	p.send(t, toolCall(4, "list_models", `{}`))
	eventually(t, "ping and list_models during a generation", func() bool { return p.out.hasID("3") && p.out.hasID("4") })
	if p.out.hasID("1") || p.out.hasID("2") {
		t.Fatal("generations finished before being released")
	}
	if len(started) != 0 {
		t.Fatal("the second generation reached Ollama while the first held the queue")
	}
	close(release)
	p.finish(t)
	m := map[string]string{}
	for _, l := range p.out.lines() {
		m[string(decode(t, l).ID)] = l
	}
	if m["1"] == "" || m["2"] == "" {
		t.Fatalf("both generations must be answered: %q", p.out.lines())
	}
}

// PERF-011 end to end: notifications/cancelled aborts the HTTP call to Ollama
// and frees the GPU queue for the next generation.
func TestCancelAbortsTheUpstreamCallAndFreesTheQueue(t *testing.T) {
	upstreamGone := make(chan struct{}, 1)
	var first atomic.Bool
	f := newFakeOllama(t, func(w http.ResponseWriter, r *http.Request) {
		if first.CompareAndSwap(false, true) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			fmt.Fprintln(w, `{"response":"parcial","done":false}`)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			upstreamGone <- struct{}{}
			return
		}
		answerOK(w, r)
	})
	cfg := testConfigWith(f.srv.URL)
	p := startRun(t, cfg, buildToolsFor(t, cfg))
	p.send(t, toolCall(1, "generate", `{"prompt":"longa"}`))
	eventually(t, "generation started", func() bool { return f.hits.Load() == 1 })
	p.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`)
	select {
	case <-upstreamGone:
	case <-time.After(3 * time.Second):
		t.Fatal("the HTTP call to Ollama was not aborted")
	}
	p.send(t, toolCall(2, "generate", `{"prompt":"curta"}`))
	eventually(t, "second generation answered (queue freed)", func() bool { return p.out.hasID("2") })
	p.finish(t)
	if p.out.hasID("1") {
		t.Fatal("the cancelled call must not be answered")
	}
}

func testConfigWith(url string) Config {
	cfg := testConfig()
	cfg.OllamaURL = url
	cfg.APIKey = wiringToken
	return cfg
}

func buildToolsFor(t *testing.T, cfg Config) []ToolEntry {
	t.Helper()
	var sink syncBuf
	entries, closeTools := buildToolsWithClose(cfg, &logger{w: &sink, redact: func(s string) string { return s }})
	t.Cleanup(closeTools) // no background sender outlives the test (--auto-report)
	return entries
}

func realToolsList(t *testing.T, verbosity string, extra ...string) ([]mcp.Tool, int) {
	t.Helper()
	args := append([]string{"--ollama-url", "https://exemplo.invalid", "--tools-verbosity", verbosity}, extra...)
	lines, _, code := runMain(t, args, nil, rpc(1, "tools/list", ""))
	if code != 0 || len(lines) != 1 {
		t.Fatalf("exit %d, %d lines", code, len(lines))
	}
	var res mcp.ToolsListResult
	if err := json.Unmarshal(decode(t, lines[0]).Result, &res); err != nil {
		t.Fatal(err)
	}
	return res.Tools, len(lines[0])
}

// TEST-008: exact set on the real tools/list, full and compact.
func TestRealToolsListExactSetFullAndCompact(t *testing.T) {
	want := "chat,embed,generate,list_models,reportar_erro"
	for _, v := range []string{"full", "compact"} {
		list, size := realToolsList(t, v)
		var names []string
		for _, tl := range list {
			names = append(names, tl.Name)
		}
		if !sort.StringsAreSorted(names) || strings.Join(names, ",") != want {
			t.Errorf("%s: tools = %v", v, names)
		}
		t.Logf("tools/list %s: result line = %d bytes", v, size)
	}
	_, full := realToolsList(t, "full")
	_, compact := realToolsList(t, "compact")
	if compact >= full {
		t.Fatalf("compact (%d) must be smaller than full (%d)", compact, full)
	}
	// compact keeps the contract: required, type, enum and the annotations.
	list, _ := realToolsList(t, "compact")
	for _, tl := range list {
		if tl.Annotations == nil || tl.Annotations.OpenWorldHint || tl.Title == "" {
			t.Errorf("%s: compact lost title/annotations", tl.Name)
		}
		if strings.Contains(tl.Description, ". ") {
			t.Errorf("%s: compact description has more than one sentence: %q", tl.Name, tl.Description)
		}
		if tl.Name == "chat" {
			b, _ := json.Marshal(tl.InputSchema)
			for _, keep := range []string{`"required":["messages"]`, `"enum":["system","user","assistant"]`, `"additionalProperties":false`} {
				if !strings.Contains(string(b), keep) {
					t.Errorf("chat compact schema lost %s: %s", keep, b)
				}
			}
			if strings.Contains(string(b), `"description"`) {
				t.Errorf("chat compact schema kept a description: %s", b)
			}
		}
	}
}

// PROTO-014: two tools/list calls are byte-identical.
func TestRealToolsListIsDeterministic(t *testing.T) {
	args := []string{"--ollama-url", "https://exemplo.invalid"}
	a, _, _ := runMain(t, args, nil, rpc(1, "tools/list", ""))
	b, _, _ := runMain(t, args, nil, rpc(1, "tools/list", ""))
	if len(a) != 1 || a[0] != b[0] {
		t.Fatal("tools/list differs between runs")
	}
}

// BUILD-006 / TEST-010 against the REAL tools/list: render the catalog from
// what the server actually serves and compare with TOOLS.md.
func TestToolsMdMatchesTheRealToolsList(t *testing.T) {
	// the catalog documents every tool, so the real list is taken with --allow-root
	list, _ := realToolsList(t, "full", "--allow-root", t.TempDir())
	raw, err := os.ReadFile(filepath.Join("..", "..", "TOOLS.md"))
	if err != nil {
		t.Fatalf("TOOLS.md: %v (run: go run ./cmd/dumptools)", err)
	}
	if got, want := strings.ReplaceAll(string(raw), "\r\n", "\n"), tools.RenderCatalog(list); got != want {
		t.Fatalf("TOOLS.md differs from the real tools/list (run: go run ./cmd/dumptools)\nfile=%d bytes, tools/list=%d bytes", len(got), len(want))
	}
}

// CFG-011 / SEC-014 / SEC-015 / OBS-004: with the trace and audit flags set to
// paths and several calls made, nothing is created (their writers arrive in a
// later phase) and no tool writes to disk.
func TestNoFileIsCreated(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	f := newFakeOllama(t, answerOK)
	trace, audit := filepath.Join(dir, "trace.log"), filepath.Join(dir, "audit.jsonl")
	_, errw, code := runMain(t, []string{"--ollama-url", f.srv.URL, "--trace-file", trace, "--audit-log-path", audit, "--debug"}, nil,
		toolCall(1, "generate", `{"prompt":"x"}`), toolCall(2, "list_models", `{}`), toolCall(3, "embed", `{"input":"e"}`),
		toolCall(4, "chat", `{"messages":[{"role":"user","content":"m"}]}`))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errw)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("files created: %v", entries)
	}
	// And without any flag, the working directory stays empty too.
	runMain(t, []string{"--ollama-url", f.srv.URL}, nil, toolCall(1, "generate", `{"prompt":"x"}`))
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files created without flags: %v", entries)
	}
}
