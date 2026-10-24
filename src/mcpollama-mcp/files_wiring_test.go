package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/mcp"
)

const fileSentinel = "CONTEUDO-SENTINELA-DO-ARQUIVO-7c1d"

func rootWithFiles(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "wr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ = filepath.EvalSymlinks(d)
	t.Cleanup(func() { os.RemoveAll(d) })
	root := filepath.Join(d, "root")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "nota.txt"), []byte("linha 1\n"+fileSentinel+"\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN="+fileSentinel+"\n"), 0o644)
	os.WriteFile(filepath.Join(d, "fora.txt"), []byte(fileSentinel+"-FORA\n"), 0o644)
	return root
}

func listNames(t *testing.T, line string) []string {
	t.Helper()
	r := decode(t, line)
	var res mcp.ToolsListResult
	if err := json.Unmarshal(r.Result, &res); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	sort.Strings(names)
	return names
}

// FILE-001 / TOOL-002 end to end: the file tools exist only with --allow-root.
func TestFileToolsExistOnlyWithAllowRoot(t *testing.T) {
	f := newFakeOllama(t, answerOK)
	root := rootWithFiles(t)

	lines, _, code := runMain(t, []string{"--ollama-url", f.srv.URL}, nil,
		rpc(1, "tools/list", ""), toolCall(2, "summarize_file", `{"path":"nota.txt"}`))
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	m := byID(t, lines)
	if got := strings.Join(listNames(t, m["1"]), ","); got != "chat,embed,generate,list_models" {
		t.Fatalf("without --allow-root: %s", got)
	}
	if r := decode(t, m["2"]); r.Error == nil || r.Error.Code != mcp.CodeInvalidParams {
		t.Fatalf("tools/call of an unregistered file tool must be -32602, got %+v", r.Error)
	}

	lines, _, _ = runMain(t, []string{"--ollama-url", f.srv.URL, "--allow-root", root}, nil, rpc(1, "tools/list", ""))
	if got := strings.Join(listNames(t, lines[0]), ","); got != "chat,embed,extract,generate,list_models,summarize_file,translate" {
		t.Fatalf("with --allow-root: %s", got)
	}

	// the environment variable works, and the flag beats it
	lines, _, _ = runMain(t, []string{"--ollama-url", f.srv.URL}, map[string]string{"MCPOLLAMA_ALLOW_ROOT": root}, rpc(1, "tools/list", ""))
	if got := listNames(t, lines[0]); len(got) != 7 {
		t.Fatalf("MCPOLLAMA_ALLOW_ROOT: %v", got)
	}
	if f.hits.Load() != 0 {
		t.Fatalf("listing must not reach Ollama: %d", f.hits.Load())
	}
}

// TEST-009 (flag side): a root that does not exist stops the start with exit 2
// and a message, before any JSON-RPC output.
func TestBadAllowRootStopsTheStart(t *testing.T) {
	root := rootWithFiles(t)
	for name, v := range map[string]string{
		"missing": filepath.Join(root, "nao-existe"),
		"file":    filepath.Join(root, "nota.txt"),
		"unc":     `\\host\share`,
		"one bad": root + "," + filepath.Join(root, "nao-existe"),
	} {
		lines, errw, code := runMain(t, []string{"--allow-root", v}, nil, rpc(1, "tools/list", ""))
		if code != 2 || len(lines) != 0 || !strings.Contains(errw, "--allow-root") {
			t.Fatalf("%s: exit %d, %d stdout lines, stderr %q", name, code, len(lines), errw)
		}
	}
	if _, errw, code := runMain(t, []string{"--allow-root", root, "--max-chunks", "0"}, nil); code != 2 || !strings.Contains(errw, "--max-chunks") {
		t.Fatalf("max-chunks 0: exit %d %q", code, errw)
	}
}

// FILE-012 / SEC-015: file content, the file path and the token reach neither
// stderr (with --debug) nor any error; refusals never call Ollama.
func TestFileContentNeverInLogsOrErrors(t *testing.T) {
	f := newFakeOllama(t, answerOK)
	root := rootWithFiles(t)
	outside := filepath.Join(filepath.Dir(root), "fora.txt")
	args := []string{"--ollama-url", f.srv.URL, "--allow-root", root, "--debug"}
	e := map[string]string{"OLLAMA_API_KEY": wiringToken}

	lines, errw, code := runMain(t, args, e,
		toolCall(1, "summarize_file", `{"path":"nota.txt"}`),
		toolCall(2, "extract", `{"path":".env","instruction":"x"}`),
		toolCall(3, "translate", mustJSONString(map[string]string{"path": outside, "target_language": "English"})),
		toolCall(4, "summarize_file", `{"path":"..\\fora.txt"}`),
		toolCall(5, "summarize_file", `{"path":"..\\..\\..\\Windows\\win.ini"}`),
	)
	if code != 0 || len(lines) != 5 {
		t.Fatalf("exit %d, %d lines\n%s", code, len(lines), errw)
	}
	m := byID(t, lines)

	res, _ := resultOf(t, m["1"])
	if res.IsError || !strings.Contains(res.Content[0].Text, "RESPOSTA-DO-MODELO") || !strings.Contains(res.Content[0].Text, "partes=1") {
		t.Fatalf("summarize_file: %+v", res)
	}
	if f.hits.Load() != 1 {
		t.Fatalf("only the allowed file may reach Ollama: %d requests (%v)", f.hits.Load(), f.paths)
	}
	for _, id := range []string{"2", "3", "4", "5"} {
		res, r := resultOf(t, m[id])
		if r.Error != nil || !res.IsError {
			t.Fatalf("call %s must be a tool error: %+v %+v", id, res, r.Error)
		}
		msg := res.Content[0].Text
		if strings.Contains(msg, fileSentinel) || strings.Contains(msg, root) || strings.Contains(msg, wiringToken) {
			t.Fatalf("call %s: error leaks: %s", id, msg)
		}
	}
	for _, bad := range []string{fileSentinel, root, filepath.Dir(root), wiringToken, "nota.txt"} {
		if strings.Contains(errw, bad) {
			t.Fatalf("stderr contains %q:\n%s", bad, errw)
		}
	}
	for _, l := range lines {
		if strings.Contains(l, wiringToken) {
			t.Fatal("token on stdout")
		}
	}
	if !strings.Contains(errw, "file tools enabled: 1 allowed root(s)") {
		t.Fatalf("the risk banner is missing:\n%s", errw)
	}
	// nothing was written next to the file
	entries, _ := os.ReadDir(root)
	names := []string{}
	for _, en := range entries {
		names = append(names, en.Name())
	}
	sort.Strings(names)
	if strings.Join(names, ",") != ".env,nota.txt" {
		t.Fatalf("files in the root: %v", names)
	}
}

func mustJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TEST-008 with --allow-root: the exact set of seven, full and compact, and
// the compact list is smaller (TOK-006).
func TestRealToolsListWithAllowRoot(t *testing.T) {
	var sizes []int
	for _, v := range []string{"full", "compact"} {
		list, size := realToolsList(t, v, "--allow-root", t.TempDir())
		var names []string
		for _, tl := range list {
			names = append(names, tl.Name)
		}
		if got := strings.Join(names, ","); got != "chat,embed,extract,generate,list_models,summarize_file,translate" {
			t.Errorf("%s: %s", v, got)
		}
		sizes = append(sizes, size)
		t.Logf("%s tools/list with --allow-root: %d bytes", v, size)
	}
	if sizes[1] >= sizes[0] {
		t.Fatalf("compact (%d) must be smaller than full (%d)", sizes[1], sizes[0])
	}
}

// CFG-009: --allow-root is empty by default, comes from MCPOLLAMA_ALLOW_ROOT
// and the flag wins; --max-chunks defaults to 40.
func TestAllowRootConfig(t *testing.T) {
	c := parse(t, nil, nil)
	if c.AllowRoot != "" || c.MaxChunks != 40 {
		t.Fatalf("defaults: %+v", c)
	}
	c = parse(t, nil, map[string]string{"MCPOLLAMA_ALLOW_ROOT": "/a,/b", "MCPOLLAMA_MAX_CHUNKS": "7"})
	if c.AllowRoot != "/a,/b" || c.MaxChunks != 7 {
		t.Fatalf("env: %+v", c)
	}
	c = parse(t, []string{"--allow-root", "/flag", "--max-chunks", "9"}, map[string]string{"MCPOLLAMA_ALLOW_ROOT": "/env"})
	if c.AllowRoot != "/flag" || c.MaxChunks != 9 {
		t.Fatalf("flag: %+v", c)
	}
}
