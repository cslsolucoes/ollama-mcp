package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/redact"
)

func testConfig() Config {
	return Config{
		Model: defaultModel, EmbedModel: defaultEmbedModel, Timeout: defaultTimeout,
		MaxPromptChars: defaultMaxPromptChars, MaxLineBytes: defaultMaxLineBytes,
		MaxFileBytes: defaultMaxFileBytes, ToolsVerbosity: "full", Version: "test",
	}
}

// run feeds input to Run and returns stdout lines and stderr.
func run(t *testing.T, cfg Config, tools []ToolEntry, input string) ([]string, string) {
	t.Helper()
	var out, errw bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(input), &out, &errw, cfg, tools); err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := out.String()
	if s != "" && !strings.HasSuffix(s, "\n") {
		t.Fatalf("stdout does not end with newline: %q", s)
	}
	var lines []string
	if s != "" {
		lines = strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	}
	return lines, errw.String()
}

type rawResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *mcp.Error      `json:"error"`
}

func decode(t *testing.T, line string) rawResp {
	t.Helper()
	var r rawResp
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("stdout line is not JSON: %v: %.200q", err, line)
	}
	if r.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q in %.200q", r.JSONRPC, line)
	}
	if _, ok := rawFields(t, line)["id"]; !ok {
		t.Fatalf("response without id field: %.200q", line)
	}
	return r
}

func rawFields(t *testing.T, line string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func one(t *testing.T, cfg Config, tools []ToolEntry, input string) rawResp {
	t.Helper()
	lines, _ := run(t, cfg, tools, input+"\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 response line, got %d: %q", len(lines), lines)
	}
	return decode(t, lines[0])
}

func TestInitializeNegotiation(t *testing.T) {
	cases := []struct{ params, want string }{
		{`{"protocolVersion":"2025-11-25"}`, "2025-11-25"},
		{`{"protocolVersion":"2025-06-18"}`, "2025-06-18"},
		{`{"protocolVersion":"2025-03-26"}`, "2025-03-26"},
		{`{"protocolVersion":"2024-11-05"}`, "2024-11-05"},
		{`{"protocolVersion":"1999-01-01"}`, "2025-11-25"}, // unknown -> newest
		{`{}`, "2025-11-25"},
		{``, "2025-11-25"}, // params absent
	}
	for _, c := range cases {
		req := `{"jsonrpc":"2.0","id":1,"method":"initialize"`
		if c.params != "" {
			req += `,"params":` + c.params
		}
		r := one(t, testConfig(), nil, req+`}`)
		var res mcp.InitializeResult
		if err := json.Unmarshal(r.Result, &res); err != nil {
			t.Fatalf("%s: %v (%s)", c.params, err, r.Result)
		}
		if res.ProtocolVersion != c.want {
			t.Errorf("params %q: version %q, want %q", c.params, res.ProtocolVersion, c.want)
		}
		if res.ServerInfo.Name != "mcpollama" || res.ServerInfo.Version != "test" {
			t.Errorf("serverInfo = %+v", res.ServerInfo)
		}
		if !strings.Contains(string(r.Result), `"tools":{"listChanged":false}`) {
			t.Errorf("capabilities.tools.listChanged:false missing: %s", r.Result)
		}
	}
}

func TestInitializeBadParams(t *testing.T) {
	r := one(t, testConfig(), nil, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":"x"}`)
	if r.Error == nil || r.Error.Code != -32602 {
		t.Fatalf("got %+v", r)
	}
}

func TestNotificationsGetNoResponse(t *testing.T) {
	in := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","method":"ping"}` + "\n" + // notification form of a request
		`{"jsonrpc":"2.0","method":"no/such"}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"nope"}}` + "\n"
	lines, _ := run(t, testConfig(), nil, in)
	if len(lines) != 0 {
		t.Fatalf("notifications must not be answered: %q", lines)
	}
}

func TestPingReturnsEmptyObject(t *testing.T) {
	lines, _ := run(t, testConfig(), nil, `{"jsonrpc":"2.0","id":7,"method":"ping"}`+"\n")
	if len(lines) != 1 || lines[0] != `{"jsonrpc":"2.0","id":7,"result":{}}` {
		t.Fatalf("got %q", lines)
	}
}

func TestToolsListEmptyIsArray(t *testing.T) {
	lines, _ := run(t, testConfig(), nil, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n")
	if len(lines) != 1 || lines[0] != `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}` {
		t.Fatalf("got %q", lines)
	}
}

func noop(context.Context, json.RawMessage) (string, error) { return "ok", nil }

func TestToolsListSortedDeterministic(t *testing.T) {
	var tools []ToolEntry
	for _, n := range []string{"zeta", "alpha", "mid", "beta"} {
		tools = append(tools, ToolEntry{Def: mcp.Tool{Name: n, InputSchema: map[string]any{"type": "object"}}, Handler: noop})
	}
	first := ""
	for i := 0; i < 5; i++ {
		lines, _ := run(t, testConfig(), tools, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n")
		if i == 0 {
			first = lines[0]
			var res struct {
				Result mcp.ToolsListResult `json:"result"`
			}
			json.Unmarshal([]byte(lines[0]), &res)
			var got []string
			for _, tl := range res.Result.Tools {
				got = append(got, tl.Name)
			}
			if strings.Join(got, ",") != "alpha,beta,mid,zeta" {
				t.Fatalf("order = %v", got)
			}
		} else if lines[0] != first {
			t.Fatal("tools/list not deterministic")
		}
	}
}

func TestToolsVerbosityCompact(t *testing.T) {
	tool := ToolEntry{Def: mcp.Tool{Name: "t", Description: "First sentence. Second sentence with detail.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"p": map[string]any{"type": "string", "description": "long text"}}}}, Handler: noop}
	cfg := testConfig()
	cfg.ToolsVerbosity = "compact"
	lines, _ := run(t, cfg, []ToolEntry{tool}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n")
	if strings.Contains(lines[0], "Second") || strings.Contains(lines[0], "long text") {
		t.Fatalf("compact not compact: %s", lines[0])
	}
	lines, _ = run(t, testConfig(), []ToolEntry{tool}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n")
	if !strings.Contains(lines[0], "Second") || !strings.Contains(lines[0], "long text") {
		t.Fatalf("full lost text: %s", lines[0])
	}
}

func TestUnknownMethod(t *testing.T) {
	r := one(t, testConfig(), nil, `{"jsonrpc":"2.0","id":3,"method":"resources/list"}`)
	if r.Error == nil || r.Error.Code != -32601 || string(r.ID) != "3" {
		t.Fatalf("got %+v", r)
	}
}

func TestInvalidJSONParseError(t *testing.T) {
	lines, _ := run(t, testConfig(), nil, "{not json\n"+`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")
	if len(lines) != 2 {
		t.Fatalf("got %q", lines)
	}
	if lines[0] != `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}` {
		t.Fatalf("parse error line: %s", lines[0])
	}
	if decode(t, lines[1]).Error != nil {
		t.Fatal("loop must continue after a parse error")
	}
}

func TestValidJSONNotAnObjectIsInvalidRequest(t *testing.T) {
	for _, in := range []string{`[1,2]`, `5`, `"x"`, `null`} {
		r := one(t, testConfig(), nil, in)
		if r.Error == nil || r.Error.Code != -32600 || string(r.ID) != "null" {
			t.Errorf("%s -> %+v", in, r)
		}
	}
}

func TestMissingMethodOrVersion(t *testing.T) {
	for _, in := range []string{`{"jsonrpc":"2.0","id":1}`, `{"id":1,"method":"ping"}`, `{"jsonrpc":"1.0","id":1,"method":"ping"}`} {
		r := one(t, testConfig(), nil, in)
		if r.Error == nil || r.Error.Code != -32600 || string(r.ID) != "1" {
			t.Errorf("%s -> %+v", in, r)
		}
	}
}

func TestEmptyLinesIgnored(t *testing.T) {
	lines, _ := run(t, testConfig(), nil, "\n   \n\r\n"+`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\r\n\n")
	if len(lines) != 1 {
		t.Fatalf("got %q", lines)
	}
}

func TestIDPreserved(t *testing.T) {
	for _, id := range []string{`0`, `1`, `-5`, `1.5`, `"abc"`, `"0"`, `null`, `9007199254740993`, `"a\"b"`} {
		r := one(t, testConfig(), nil, `{"jsonrpc":"2.0","id":`+id+`,"method":"ping"}`)
		if string(r.ID) != id {
			t.Errorf("id %s came back as %s", id, r.ID)
		}
	}
}

func TestLastLineWithoutNewline(t *testing.T) {
	lines, _ := run(t, testConfig(), nil, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if len(lines) != 1 {
		t.Fatalf("got %q", lines)
	}
}

func pingWithPadding(n int) string {
	return `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` + strings.Repeat("x", n) + `"}}`
}

func TestLongLines(t *testing.T) {
	cfg := testConfig()
	cfg.MaxLineBytes = 2 << 20
	for _, n := range []int{30 << 10, 70 << 10, 1 << 20} {
		in := pingWithPadding(n) + "\n" + `{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n"
		lines, _ := run(t, cfg, nil, in)
		if len(lines) != 2 {
			t.Fatalf("%d bytes: got %d responses", n, len(lines))
		}
		if r := decode(t, lines[0]); r.Error != nil || string(r.ID) != "1" {
			t.Fatalf("%d bytes: %+v", n, r)
		}
	}
}

func TestLineCeilingBoundary(t *testing.T) {
	cfg := testConfig()
	line := pingWithPadding(100)
	cfg.MaxLineBytes = len(line)
	lines, _ := run(t, cfg, nil, line+"\n"+line+"x\n"+`{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\n")
	if len(lines) != 3 {
		t.Fatalf("got %q", lines)
	}
	if decode(t, lines[0]).Error != nil {
		t.Fatal("line of exactly the ceiling must pass")
	}
	r := decode(t, lines[1])
	if r.Error == nil || r.Error.Code != -32600 || string(r.ID) != "null" {
		t.Fatalf("ceiling+1: %+v", r)
	}
	if r := decode(t, lines[2]); r.Error != nil || string(r.ID) != "2" {
		t.Fatalf("loop must continue after an oversized line: %+v", r)
	}
}

func TestHugeLineDoesNotKillServer(t *testing.T) {
	cfg := testConfig()
	cfg.MaxLineBytes = 1024
	in := strings.Repeat("y", 5<<20) + "\n" + `{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n"
	lines, _ := run(t, cfg, nil, in)
	if len(lines) != 2 || decode(t, lines[0]).Error == nil || decode(t, lines[1]).Error != nil {
		t.Fatalf("got %q", lines)
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	r := one(t, testConfig(), nil, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	if r.Error == nil || r.Error.Code != -32602 || !strings.Contains(r.Error.Message, "Unknown tool: nope") {
		t.Fatalf("got %+v", r)
	}
}

func TestToolsCallBadParams(t *testing.T) {
	for _, in := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":null}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"x"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":[]}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":5}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"t","arguments":"str"}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"t","arguments":[1]}}`,
	} {
		r := one(t, testConfig(), []ToolEntry{{Def: mcp.Tool{Name: "t"}, Handler: noop}}, in)
		if r.Error == nil || r.Error.Code != -32602 {
			t.Errorf("%s -> %+v", in, r)
		}
	}
}

func TestToolsCallArgumentsDefaultToEmptyObject(t *testing.T) {
	var got []string
	h := func(_ context.Context, a json.RawMessage) (string, error) {
		got = append(got, string(a))
		return "ok", nil
	}
	tools := []ToolEntry{{Def: mcp.Tool{Name: "t"}, Handler: h}}
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"t"}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"t","arguments":null}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"t","arguments":{"a":1}}}` + "\n"
	run(t, testConfig(), tools, in)
	if strings.Join(got, "|") != `{}|{}|{"a":1}` {
		t.Fatalf("args = %v", got)
	}
}

func TestToolsCallSuccessAndIsError(t *testing.T) {
	tools := []ToolEntry{
		{Def: mcp.Tool{Name: "good"}, Handler: func(context.Context, json.RawMessage) (string, error) { return "hello", nil }},
		{Def: mcp.Tool{Name: "bad"}, Handler: func(context.Context, json.RawMessage) (string, error) { return "", errors.New("ollama 503") }},
	}
	r := one(t, testConfig(), tools, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"good"}}`)
	if r.Error != nil || string(r.Result) != `{"content":[{"type":"text","text":"hello"}]}` {
		t.Fatalf("good: %+v %s", r.Error, r.Result)
	}
	r = one(t, testConfig(), tools, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"bad"}}`)
	if r.Error != nil || string(r.Result) != `{"content":[{"type":"text","text":"ollama 503"}],"isError":true}` {
		t.Fatalf("bad: %+v %s", r.Error, r.Result)
	}
}

func TestToolPanicRecovered(t *testing.T) {
	tools := []ToolEntry{{Def: mcp.Tool{Name: "boom"}, Handler: func(context.Context, json.RawMessage) (string, error) {
		panic("kaboom")
	}}}
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"boom"}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n"
	lines, errw := run(t, testConfig(), tools, in)
	if len(lines) != 2 {
		t.Fatalf("got %q", lines)
	}
	if !strings.Contains(lines[0], `"isError":true`) {
		t.Fatalf("panic must become isError: %s", lines[0])
	}
	if decode(t, lines[1]).Error != nil {
		t.Fatal("server must keep serving after a panic")
	}
	if !strings.Contains(errw, "panic recovered") {
		t.Fatalf("panic not logged: %s", errw)
	}
}

func TestStdoutIsOnlyJSONRPC(t *testing.T) {
	tools := []ToolEntry{{Def: mcp.Tool{Name: "t"}, Handler: noop}}
	cfg := testConfig()
	cfg.Debug = true
	cfg.Insecure = true
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"nope"}`,
		`garbage`,
		``,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"t"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"zz"}}`,
		pingWithPadding(100 << 10),
	}, "\n") + "\n"
	lines, errw := run(t, cfg, tools, in)
	if len(lines) != 8 {
		t.Fatalf("want 8 responses, got %d: %q", len(lines), lines)
	}
	for _, l := range lines {
		decode(t, l)
		if strings.ContainsAny(l, "\r") {
			t.Fatalf("CR in output line: %q", l)
		}
	}
	if errw == "" {
		t.Fatal("expected log output on stderr")
	}
}

func TestLogsDefaultToMetadataOnly(t *testing.T) {
	tools := []ToolEntry{{Def: mcp.Tool{Name: "t"}, Handler: noop}}
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"t","arguments":{"prompt":"TOPSECRETPROMPT"}}}` + "\n"
	for _, debug := range []bool{false, true} {
		cfg := testConfig()
		cfg.Debug = debug
		_, errw := run(t, cfg, tools, in)
		if strings.Contains(errw, "TOPSECRETPROMPT") {
			t.Fatalf("debug=%v: arguments leaked to the log: %s", debug, errw)
		}
		if !strings.Contains(errw, "method=tools/call tool=t status=ok") {
			t.Fatalf("debug=%v: metadata missing: %s", debug, errw)
		}
	}
}

func TestLogLineCap(t *testing.T) {
	var b bytes.Buffer
	l := &logger{w: &b, redact: redact.New("SECRET")}
	l.logf("%s SECRET %s", strings.Repeat("a", 3000), strings.Repeat("é", 3000))
	out := b.String()
	if len(out) > maxLogLine {
		t.Fatalf("log line is %d bytes, cap %d", len(out), maxLogLine)
	}
	if strings.Contains(out, "SECRET") || !strings.Contains(out, redact.Mask) {
		t.Fatal("redaction must happen before the cap")
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatal("exactly one newline expected")
	}
	b.Reset()
	l.logf("a\nb\r\nc")
	if b.String() != "a b  c\n" {
		t.Fatalf("newlines must be neutralised: %q", b.String())
	}
}

func TestRedactorIgnoresEmptySecret(t *testing.T) {
	if got := redact.New("")("abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestTokenNeverInAnyOutput(t *testing.T) {
	const token = "tok-S3CR3T-value-123"
	tools := []ToolEntry{
		{Def: mcp.Tool{Name: "err"}, Handler: func(context.Context, json.RawMessage) (string, error) {
			return "", fmt.Errorf("upstream said Authorization: Bearer %s rejected", token)
		}},
		{Def: mcp.Tool{Name: "boom"}, Handler: func(context.Context, json.RawMessage) (string, error) {
			panic("leaking " + token)
		}},
	}
	cfg := testConfig()
	cfg.Debug = true
	cfg.APIKey = token
	cfg.OllamaURL = "https://user:" + token + "@host.invalid:8443/x?k=" + token
	cfg.Insecure = true
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"err"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"boom"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"` + token + `"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"` + token + `"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"initialize"}`,
	}, "\n") + "\n"
	var out, errw bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(in), &out, &errw, cfg, tools); err != nil {
		t.Fatal(err)
	}
	// The client itself sent the token as method/tool name, so the protocol
	// echo of those two strings is the client's own text; every other byte
	// (tool errors, panics, logs, startup) must be clean.
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if strings.Contains(l, token) && !strings.Contains(l, "method not found") && !strings.Contains(l, "Unknown tool") {
			t.Fatalf("token in stdout: %s", l)
		}
	}
	if strings.Contains(errw.String(), token) {
		t.Fatalf("token in stderr:\n%s", errw.String())
	}
}

func TestInsecureWarnings(t *testing.T) {
	cfg := testConfig()
	cfg.Insecure = true
	_, errw := run(t, cfg, nil, "")
	if !strings.Contains(errw, "WARNING") || strings.Contains(errw, "tls-ca") {
		t.Fatalf("insecure only: %s", errw)
	}
	cfg.TLSCA = "ca.pem"
	_, errw = run(t, cfg, nil, "")
	if !strings.Contains(errw, "--tls-ca is ignored") {
		t.Fatalf("insecure+ca: %s", errw)
	}
	_, errw = run(t, testConfig(), nil, "")
	if strings.Contains(errw, "WARNING") {
		t.Fatalf("no warning expected: %s", errw)
	}
}

func TestDuplicateToolRejected(t *testing.T) {
	tl := ToolEntry{Def: mcp.Tool{Name: "a"}, Handler: noop}
	err := Run(context.Background(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, testConfig(), []ToolEntry{tl, tl})
	if err == nil {
		t.Fatal("duplicate tool names must fail")
	}
}

// PERF-012 / PROTO-011: no backend is ever contacted and the protocol answers
// fast even when the configured Ollama is unreachable.
func TestFastWithoutBackend(t *testing.T) {
	cfg := testConfig()
	cfg.OllamaURL = "http://127.0.0.1:1"
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}` + "\n"
	start := time.Now()
	lines, _ := run(t, cfg, nil, in)
	if d := time.Since(start); d > time.Second || len(lines) != 3 {
		t.Fatalf("took %s, %d responses", d, len(lines))
	}
}

func TestContextCancelStopsLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &bytes.Buffer{}, &bytes.Buffer{}, testConfig(), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestLeadingBOMTolerated(t *testing.T) {
	lines, _ := run(t, testConfig(), nil, "\xEF\xBB\xBF"+`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")
	if len(lines) != 1 || decode(t, lines[0]).Error != nil {
		t.Fatalf("got %q", lines)
	}
}
