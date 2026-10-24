package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/cslsoftwares/mcpollama/files"
	"github.com/cslsoftwares/mcpollama/ollama"
)

// fileEnv is a temporary --allow-root directory (made by hand: t.TempDir puts
// the test name in the path, which could trip the denylist on its own).
type fileEnv struct {
	dir string
}

func newFileEnv(t *testing.T) fileEnv {
	t.Helper()
	d, err := os.MkdirTemp("", "ft")
	if err != nil {
		t.Fatal(err)
	}
	d, _ = filepath.EvalSymlinks(d)
	t.Cleanup(func() { os.RemoveAll(d) })
	root := filepath.Join(d, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return fileEnv{dir: root}
}

func (e fileEnv) put(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(e.dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// deps builds Deps for the file tools backed by the mock, with a prompt
// ceiling that the real client enforces too (a prompt above it would fail
// there, so "never above the ceiling" is checked twice).
func (e fileEnv) deps(t *testing.T, m *mock, ceiling, maxChunks int) Deps {
	t.Helper()
	g, err := files.NewGuard([]string{e.dir}, 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	d := m.deps(t, func(o *ollama.Options) { o.MaxPromptChars = ceiling })
	d.Files, d.MaxPromptChars, d.MaxChunks = g, ceiling, maxChunks
	return d
}

// reply is what a fake generate call answers: text and an HTTP status (0 = 200).
type reply func(n int32, prompt string) (string, int)

// genMock is a mock whose /api/generate handler sees the prompt and numbers
// its calls. Every call reports 10 prompt tokens, 2 generated and 1 s.
func genMock(t *testing.T, f reply) *mock {
	t.Helper()
	var n atomic.Int32
	return newMock(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		k := n.Add(1)
		text, status := fmt.Sprintf("RESP-%d", k), 0
		if f != nil {
			text, status = f(k, req.Prompt)
		}
		if status != 0 {
			http.Error(w, `{"error":"boom"}`, status)
			return
		}
		b, _ := json.Marshal(text)
		streamLines(w,
			`{"model":"qwen3-ia","response":`+string(b)+`,"done":false}`,
			`{"model":"qwen3-ia","response":"","done":true,"done_reason":"stop","total_duration":1000000000,"load_duration":100000000,"prompt_eval_count":10,"eval_count":2}`)
	})
}

func (m *mock) prompts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, b := range m.bodies {
		p, _ := b["prompt"].(string)
		out = append(out, p)
	}
	return out
}

func (m *mock) systems() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, b := range m.bodies {
		s, _ := b["system"].(string)
		out = append(out, s)
	}
	return out
}

// assertWithinCeiling checks every request that reached the mock: system plus
// prompt never exceed the ceiling (PERF-004).
func assertWithinCeiling(t *testing.T, m *mock, ceiling int) {
	t.Helper()
	ps, ss := m.prompts(), m.systems()
	for i := range ps {
		if n := utf8.RuneCountInString(ps[i]) + utf8.RuneCountInString(ss[i]); n > ceiling {
			t.Fatalf("request %d has %d chars > ceiling %d", i, n, ceiling)
		}
	}
}

func lines(n, width int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%04d %s\n", i, strings.Repeat("x", width))
	}
	return b.String()
}

func snapshotDir(t *testing.T, dir string) string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil {
			st, _ := os.Lstat(p)
			out = append(out, fmt.Sprintf("%s|%d|%s", p, st.Size(), st.ModTime()))
		}
		return nil
	})
	sort.Strings(out)
	return strings.Join(out, "\n")
}

var markerRe = regexp.MustCompile(`=====(ARQUIVO|PARCIAL)_(INICIO|FIM) ([0-9a-f]{16})=====`)
