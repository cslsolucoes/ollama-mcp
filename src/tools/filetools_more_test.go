package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cslsoftwares/mcpollama/files"
)

// Partials that do not fit one combine prompt are regrouped and combined in
// rounds; still nothing above the ceiling.
func TestReduceInRounds(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	long := strings.Repeat("palavra ", 150) // 1,200 runes per map answer
	m := genMock(t, func(n int32, prompt string) (string, int) {
		if strings.Contains(prompt, "PARCIAL_INICIO") {
			return fmt.Sprintf("COMBINADO-%d", n), 0 // short: the rounds shrink
		}
		return fmt.Sprintf("RESP-%d %s", n, long), 0
	})
	d := e.deps(t, m, ceiling, 40)
	res, err := call(t, d, "summarize_file", `{"path":"grande.txt"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v", err)
	}
	assertWithinCeiling(t, m, ceiling)
	combines := 0
	for _, p := range m.prompts() {
		if strings.Contains(p, "PARCIAL_INICIO") {
			combines++
		}
	}
	if combines < 2 {
		t.Fatalf("expected an intermediate round plus the final combine, got %d combine calls", combines)
	}
	if !strings.HasPrefix(text(t, res), fmt.Sprintf("COMBINADO-%d", m.hits.Load())) {
		t.Fatalf("the answer must come from the last combine call: %q", text(t, res))
	}
}

// A combine that never shrinks the text must end in an error, not a loop.
func TestReduceThatDoesNotShrinkStops(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	long := strings.Repeat("palavra ", 150)
	m := genMock(t, func(n int32, _ string) (string, int) { return fmt.Sprintf("R%d %s", n, long), 0 })
	d := e.deps(t, m, ceiling, 40)
	_, err := call(t, d, "summarize_file", `{"path":"grande.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "combinar") {
		t.Fatalf("err = %v", err)
	}
	assertWithinCeiling(t, m, ceiling)
	if m.hits.Load() > 40 {
		t.Fatalf("calls = %d", m.hits.Load())
	}
}

func TestExtractMapReduce(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	m := genMock(t, nil)
	d := e.deps(t, m, ceiling, 40)
	res, err := call(t, d, "extract", `{"path":"grande.txt","instruction":"liste os números","format":"JSON"}`)
	if err != nil || res.IsError {
		t.Fatal(err)
	}
	assertWithinCeiling(t, m, ceiling)
	ps := m.prompts()
	n := len(ps)
	if n < 4 {
		t.Fatalf("calls = %d", n)
	}
	if !strings.Contains(ps[0], "liste os números") || !strings.Contains(ps[0], "JSON") || !strings.Contains(ps[0], "SOMENTE ao trecho 1 de") {
		t.Fatalf("map prompt: %q", ps[0][:300])
	}
	last := ps[n-1]
	if !strings.Contains(last, "Tarefa original: liste os números") || !strings.Contains(last, "PARCIAL_INICIO") || !strings.Contains(last, "RESP-1") {
		t.Fatalf("combine prompt: %q", last[:300])
	}
	if !strings.Contains(lastLine(text(t, res)), fmt.Sprintf("partes=%d", n-1)) {
		t.Fatalf("usage line: %q", lastLine(text(t, res)))
	}
}

// translate has nothing to combine: the translated parts follow one another.
func TestTranslateMapConcatenates(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	m := genMock(t, nil)
	d := e.deps(t, m, ceiling, 40)
	res, err := call(t, d, "translate", `{"path":"grande.txt","target_language":"English"}`)
	if err != nil || res.IsError {
		t.Fatal(err)
	}
	assertWithinCeiling(t, m, ceiling)
	n := int(m.hits.Load())
	if n < 4 {
		t.Fatalf("calls = %d", n)
	}
	for _, p := range m.prompts() {
		if strings.Contains(p, "PARCIAL_INICIO") {
			t.Fatal("translate must not run a combine call")
		}
	}
	out := text(t, res)
	prev := -1
	for i := 1; i <= n; i++ {
		k := strings.Index(out, fmt.Sprintf("RESP-%d", i))
		if k < 0 || k < prev {
			t.Fatalf("part %d missing or out of order", i)
		}
		prev = k
	}
	if !strings.Contains(out, fmt.Sprintf("partes=%d chamadas=%d omitidas=0", n, n)) || !strings.Contains(lastLine(out), fmt.Sprintf("partes=%d", n)) {
		t.Fatalf("counters: %q", out[len(out)-200:])
	}
}

// FILE-010: when the translation would pass the output ceiling the rest is
// not translated, and the answer says how many parts were left out.
func TestTranslateStopsAtOutputCeilingAndSaysSo(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	big := strings.Repeat("a", 20000)
	m := genMock(t, func(n int32, _ string) (string, int) { return fmt.Sprintf("P%d %s", n, big), 0 })
	d := e.deps(t, m, ceiling, 40)
	res, err := call(t, d, "translate", `{"path":"grande.txt","target_language":"English"}`)
	if err != nil || res.IsError {
		t.Fatal(err)
	}
	out := text(t, res)
	if m.hits.Load() != 2 {
		t.Fatalf("must stop after the part that passes the ceiling: %d calls", m.hits.Load())
	}
	if !strings.Contains(out, "tradução interrompida") || !strings.Contains(out, "omitidas=") || strings.Contains(out, "omitidas=0") {
		t.Fatalf("omission not reported: %q", out[len(out)-400:])
	}
	if !strings.Contains(out, "resposta truncada pelo servidor MCP") {
		t.Fatal("the output cut must be announced too")
	}
}

// FILE-011: a file that needs more calls than --max-chunks is refused before
// any call.
func TestMaxChunksRefusesBeforeCalling(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	m := genMock(t, nil)
	d := e.deps(t, m, 2600, 3)
	for tool, args := range map[string]string{
		"summarize_file": `{"path":"grande.txt"}`,
		"extract":        `{"path":"grande.txt","instruction":"x"}`,
		"translate":      `{"path":"grande.txt","target_language":"English"}`,
	} {
		_, err := call(t, d, tool, args)
		if err == nil || !strings.Contains(err.Error(), "--max-chunks") {
			t.Fatalf("%s: err = %v", tool, err)
		}
	}
	if m.hits.Load() != 0 {
		t.Fatalf("%d calls reached the server", m.hits.Load())
	}
}

func TestMaxPromptCharsTooLowForTheInstruction(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "a.txt", "hello\n")
	m := genMock(t, nil)
	d := e.deps(t, m, 300, 40) // smaller than the fixed system prompt
	_, err := call(t, d, "summarize_file", `{"path":"a.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "--max-prompt-chars") {
		t.Fatalf("err = %v", err)
	}
	if m.hits.Load() != 0 {
		t.Fatal("nothing may be sent")
	}
}

func TestFailureOnAMiddlePartNamesIt(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	m := genMock(t, func(n int32, _ string) (string, int) {
		if n == 3 {
			return "", 500
		}
		return "ok", 0
	})
	d := e.deps(t, m, 2600, 40)
	_, err := call(t, d, "summarize_file", `{"path":"grande.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "falha na parte 3 de ") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), e.dir) {
		t.Fatalf("leak in %q", err)
	}
}

func TestCancellationStopsTheMapReduce(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := genMock(t, func(n int32, _ string) (string, int) {
		if n == 2 {
			cancel()
		}
		return "ok", 0
	})
	d := e.deps(t, m, 2600, 40)
	_, err := entry(t, d, "summarize_file").Handler(ctx, json.RawMessage(`{"path":"grande.txt"}`))
	if err == nil {
		t.Fatal("a cancelled request must fail")
	}
	if h := m.hits.Load(); h > 3 {
		t.Fatalf("the work went on after the cancellation: %d calls", h)
	}
}

// Every part waits its turn in the single GPU slot: two requests never run
// model calls at the same time.
func TestPartsGoThroughTheGPUQueue(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	var inflight, peak atomic.Int32
	m := genMock(t, func(n int32, _ string) (string, int) {
		cur := inflight.Add(1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		inflight.Add(-1)
		return "ok", 0
	})
	d := e.deps(t, m, 2600, 40)
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := call(t, d, "summarize_file", `{"path":"grande.txt"}`)
			done <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if peak.Load() != 1 {
		t.Fatalf("peak concurrent model calls = %d, want 1", peak.Load())
	}
}

// ---- refusals, arguments, disk -----------------------------------------------

func TestRefusalsNeverReachTheModel(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, ".env", "TOKEN=SENSITIVE-VALUE")
	e.put(t, "ok.txt", "x")
	e.put(t, "bin.dat", "a\x00b")
	outside := filepath.Join(filepath.Dir(e.dir), "fora.txt")
	os.WriteFile(outside, []byte("OUTSIDE-CONTENT"), 0o644)
	os.MkdirAll(filepath.Join(e.dir, "sub"), 0o755)
	m := genMock(t, nil)
	d := e.deps(t, m, 128000, 40)
	jsonPath := func(p string) string { b, _ := json.Marshal(p); return string(b) }
	for name, p := range map[string]string{
		"denylist":    ".env",
		"traversal":   "../fora.txt",
		"absolute":    outside,
		"missing":     "nao-existe.txt",
		"directory":   "sub",
		"binary":      "bin.dat",
		"unc":         `\\host\share\f.txt`,
		"windows ads": "ok.txt:stream",
	} {
		for _, tool := range []string{"summarize_file", "extract", "translate"} {
			args := `{"path":` + jsonPath(p)
			switch tool {
			case "extract":
				args += `,"instruction":"x"`
			case "translate":
				args += `,"target_language":"English"`
			}
			res, err := call(t, d, tool, args+"}")
			if err == nil {
				t.Fatalf("%s/%s: no error: %+v", name, tool, res)
			}
			msg := err.Error()
			for _, bad := range []string{"SENSITIVE-VALUE", "OUTSIDE-CONTENT", e.dir, filepath.Dir(e.dir), sentinel} {
				if strings.Contains(msg, bad) {
					t.Fatalf("%s/%s: message leaks %q: %s", name, tool, bad, msg)
				}
			}
		}
	}
	if m.hits.Load() != 0 {
		t.Fatalf("%d refused requests reached Ollama", m.hits.Load())
	}
}

func TestEmptyFileIsRefused(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "vazio.txt", " \n\n ")
	m := genMock(t, nil)
	d := e.deps(t, m, 128000, 40)
	if _, err := call(t, d, "summarize_file", `{"path":"vazio.txt"}`); err == nil || !strings.Contains(err.Error(), "vazio") {
		t.Fatalf("err = %v", err)
	}
	if m.hits.Load() != 0 {
		t.Fatal("an empty file must not cost a model call")
	}
}

// FILE-001 at the handler: a Guard with no roots refuses; the registry does
// not even list the tools without a Guard (TestExactToolSet).
func TestHandlerWithoutRootsRefuses(t *testing.T) {
	m := genMock(t, nil)
	d := m.deps(t, nil)
	d.Files = &files.Guard{}
	for tool, args := range map[string]string{
		"summarize_file": `{"path":"a.txt"}`,
		"extract":        `{"path":"a.txt","instruction":"x"}`,
		"translate":      `{"path":"a.txt","target_language":"y"}`,
	} {
		if _, err := call(t, d, tool, args); err == nil || !strings.Contains(err.Error(), "--allow-root") {
			t.Fatalf("%s: err = %v", tool, err)
		}
	}
	if m.hits.Load() != 0 {
		t.Fatal("no call may be made")
	}
}

func TestFileToolArguments(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "a.txt", "x\n")
	m := genMock(t, nil)
	d := e.deps(t, m, 128000, 40)
	long := strings.Repeat("a", maxInstructionRune+1)
	cases := []struct{ tool, args, want string }{
		{"summarize_file", `{}`, "`path` is required"},
		{"summarize_file", `{"path":5}`, "`path` must be a string"},
		{"summarize_file", `{"path":"a.txt","max_words":5}`, "entre 20 e 2000"},
		{"summarize_file", `{"path":"a.txt","max_words":"x"}`, "an integer"},
		{"summarize_file", `{"path":"a.txt","text":"inline"}`, "argumento desconhecido"},
		{"summarize_file", `{"path":"a.txt","content":"inline"}`, "argumento desconhecido"},
		{"extract", `{"path":"a.txt"}`, "`instruction` is required"},
		{"extract", `{"path":"a.txt","instruction":"` + long + `"}`, "longo demais"},
		{"translate", `{"path":"a.txt"}`, "`target_language` is required"},
		{"translate", `{"path":"a.txt","target_language":"  "}`, "não pode ser vazio"},
	}
	for _, c := range cases {
		_, err := call(t, d, c.tool, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %s: err = %v, want %q", c.tool, c.args, err, c.want)
		}
	}
	if m.hits.Load() != 0 {
		t.Fatal("invalid arguments reached Ollama")
	}
}

// TOK-001: the schemas take a path, never the file text.
func TestFileToolSchemasTakeAPathNotText(t *testing.T) {
	seen := 0
	for _, e := range New(CatalogDeps()) {
		if !isFileTool(e.Def.Name) {
			continue
		}
		seen++
		props, _ := e.Def.InputSchema["properties"].(schemaMap)
		if _, ok := props["path"]; !ok {
			t.Errorf("%s has no path", e.Def.Name)
		}
		for _, banned := range []string{"text", "content", "body", "data"} {
			if _, ok := props[banned]; ok {
				t.Errorf("%s accepts %q: the file text must not pass through the client", e.Def.Name, banned)
			}
		}
		req, _ := e.Def.InputSchema["required"].([]string)
		if len(req) == 0 || req[0] != "path" {
			t.Errorf("%s: required = %v", e.Def.Name, req)
		}
		a := e.Def.Annotations
		if a == nil || !a.ReadOnlyHint || a.DestructiveHint || a.OpenWorldHint {
			t.Errorf("%s: annotations %+v", e.Def.Name, a)
		}
	}
	if seen != 3 {
		t.Fatalf("file tools registered: %d", seen)
	}
}

// FILE-013 / SEC-014: nothing is created, changed or removed, even through a
// full map-reduce.
func TestFileToolsWriteNothing(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	e.put(t, ".env", "S")
	base := filepath.Dir(e.dir)
	cwd := t.TempDir()
	t.Chdir(cwd)
	before := snapshotDir(t, base)
	m := genMock(t, nil)
	d := e.deps(t, m, 2600, 40)
	for _, tc := range [][2]string{
		{"summarize_file", `{"path":"grande.txt"}`},
		{"extract", `{"path":"grande.txt","instruction":"x"}`},
		{"translate", `{"path":"grande.txt","target_language":"English"}`},
		{"summarize_file", `{"path":".env"}`},
		{"summarize_file", `{"path":"../x"}`},
	} {
		call(t, d, tc[0], tc[1])
	}
	if after := snapshotDir(t, base); after != before {
		t.Fatalf("the file system changed:\n%s\n---\n%s", before, after)
	}
	if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
		t.Fatalf("files created in the working directory: %v", entries)
	}
}
