//go:build live

// Live checks of the file tools (phase 4) against the real Ollama. Outside
// the default `go test ./...`; see live_test.go for the environment. One call
// at a time (INTEG-012) and a tiny file (PERF-005: no prompt anywhere near
// the 42,540 characters of the old failure). The client ceiling is 2000
// characters, so the test also proves the prompt stays under a small ceiling.
//
// Run one test per command, for example:
//
//	go test -tags live -count=1 -v -run TestLive5SummarizeFile ./tools
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/files"
)

const liveFileText = "Ata da reunião de 23/10/2026.\n" +
	"Presentes: Ana, Bruno e Carla.\n" +
	"Decisão 1: o servidor mcpollama passa a ler arquivos somente dentro de --allow-root.\n" +
	"Decisão 2: a lista de arquivos sensíveis (.env, chaves, credenciais) é fixa e não tem override.\n" +
	"Prazo: sexta-feira, responsável: Bruno.\n"

func liveFileDeps(t *testing.T) (Deps, string) {
	t.Helper()
	d := liveDeps(t)
	dir, err := os.MkdirTemp("", "livefiles")
	if err != nil {
		t.Fatal(err)
	}
	dir, _ = filepath.EvalSymlinks(dir)
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "ata.txt"), []byte(liveFileText), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := files.NewGuard([]string{dir}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	d.Files, d.MaxPromptChars, d.MaxChunks = g, 2000, 5
	return d, dir
}

func liveFileCheck(t *testing.T, tool string, extra map[string]any, mustContain string) {
	t.Helper()
	d, dir := liveFileDeps(t)
	a := map[string]any{"path": filepath.Join(dir, "ata.txt")}
	for k, v := range extra {
		a[k] = v
	}
	raw, _ := json.Marshal(a)
	res, err := call(t, d, tool, string(raw))
	if err != nil {
		t.Fatal(err)
	}
	txt := text(t, res)
	t.Log("\n" + txt)
	if !strings.HasPrefix(lastLine(txt), "[uso] modelo=") || !strings.Contains(lastLine(txt), "partes=1") {
		t.Fatal("usage line with partes=1 missing")
	}
	if !strings.Contains(txt, "[arquivo] nome=ata.txt") {
		t.Fatal("file line missing")
	}
	if mustContain != "" && !strings.Contains(strings.ToLower(txt), strings.ToLower(mustContain)) {
		t.Fatalf("the answer does not mention %q", mustContain)
	}
	if strings.Contains(txt, dir) {
		t.Fatal("the answer shows the directory")
	}
}

func TestLive5SummarizeFile(t *testing.T) {
	liveFileCheck(t, "summarize_file", map[string]any{"max_words": 60}, "allow-root")
}

func TestLive6Extract(t *testing.T) {
	liveFileCheck(t, "extract", map[string]any{
		"instruction": "liste os nomes das pessoas presentes", "format": "lista separada por vírgulas"}, "Bruno")
}

func TestLive7Translate(t *testing.T) {
	liveFileCheck(t, "translate", map[string]any{"target_language": "English"}, "meeting")
}
