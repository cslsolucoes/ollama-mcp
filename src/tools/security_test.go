package tools

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/cslsoftwares/mcpollama/ollama"
)

// TEST-006 (tool layer): a mock that echoes the Bearer token in every error
// shape, with the client logging every exchange; the sentinel must not appear
// in any error, result, _meta or log line.
func TestSentinelNeverReachesResultsErrorsOrLogs(t *testing.T) {
	echo := "upstream rejected Authorization: Bearer " + sentinel + " (key=" + sentinel + ")"
	modes := map[string]http.HandlerFunc{
		"500-body": func(w http.ResponseWriter, _ *http.Request) { http.Error(w, echo, 500) },
		"401-body": func(w http.ResponseWriter, _ *http.Request) { http.Error(w, `{"error":"`+echo+`"}`, 401) },
		"stream-error": func(w http.ResponseWriter, _ *http.Request) {
			streamLines(w, `{"response":"antes","done":false}`, `{"error":"`+echo+`"}`)
		},
		"ok-echo": func(w http.ResponseWriter, _ *http.Request) {
			// a normal answer: nothing to scrub, nothing may be invented
			streamLines(w, `{"response":"ok","done":true,"prompt_eval_count":1,"eval_count":1}`)
		},
	}
	inputs := map[string]string{
		"generate":    `{"prompt":"p"}`,
		"chat":        `{"messages":[{"role":"user","content":"m"}]}`,
		"embed":       `{"input":"e"}`,
		"list_models": `{}`,
	}
	var (
		mu   sync.Mutex
		logs []string
	)
	for mode, h := range modes {
		m := newMock(t, h)
		d := m.deps(t, func(o *ollama.Options) {
			o.Logf = func(f string, a ...any) { mu.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); mu.Unlock() }
		})
		for name, in := range inputs {
			res, err := call(t, d, name, in)
			var all strings.Builder
			if err != nil {
				all.WriteString(err.Error())
			}
			for _, c := range res.Content {
				all.WriteString(c.Text)
			}
			fmt.Fprint(&all, res.Meta)
			if strings.Contains(all.String(), sentinel) {
				t.Errorf("%s/%s: token in %q", mode, name, all.String())
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(logs) == 0 {
		t.Fatal("the client logged nothing: the test would prove nothing")
	}
	for _, l := range logs {
		if strings.Contains(l, sentinel) || strings.Contains(l, "Bearer") {
			t.Errorf("secret in log line: %s", l)
		}
	}
}

// Prompt text and answers must not go to the client log (OBS-001/002 at the
// tool layer: the Logf lines carry method, path, status, durations, sizes).
func TestClientLogCarriesNoContent(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	m := newMock(t, generateOK)
	d := m.deps(t, func(o *ollama.Options) {
		o.Logf = func(f string, a ...any) { mu.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); mu.Unlock() }
	})
	if _, err := call(t, d, "generate", `{"prompt":"PROMPT-CONFIDENCIAL","system":"SISTEMA-CONFIDENCIAL"}`); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(logs) == 0 {
		t.Fatal("no log lines")
	}
	for _, l := range logs {
		for _, bad := range []string{"CONFIDENCIAL", "Ola", "mundo", "PENSAMENTO"} {
			if strings.Contains(l, bad) {
				t.Errorf("content in log: %s", l)
			}
		}
	}
}

// SEC-014: no tool writes to disk.
func TestToolsWriteNothingToDisk(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			embedOK(w, r)
		case "/api/tags", "/api/ps":
			tagsPS(w, r)
		default:
			generateOK(w, r)
		}
	})
	d := m.deps(t, nil)
	for name, in := range map[string]string{
		"generate": `{"prompt":"p"}`, "chat": `{"messages":[{"role":"user","content":"m"}]}`,
		"embed": `{"input":"e","include_vectors":true}`, "list_models": `{}`,
	} {
		if _, err := call(t, d, name, in); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("tools created files: %v", entries)
	}
}

// PROTO-011 / PERF-012 at the tool layer: building the registry calls nothing.
func TestRegistryConstructionIsLazy(t *testing.T) {
	built := 0
	d := Deps{Backend: func() (Backend, error) { built++; return nil, fmt.Errorf("must not be called") }}
	_ = New(d)
	_ = Defs(New(d))
	if built != 0 {
		t.Fatalf("backend built %d times at registry construction", built)
	}
	// With no backend a call fails as a tool error, not a panic.
	if _, err := call(t, Deps{}, "generate", `{"prompt":"x"}`); err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("err = %v", err)
	}
	if _, err := call(t, Deps{Backend: func() (Backend, error) { return nil, fmt.Errorf("configure --ollama-url") }}, "list_models", `{}`); err == nil ||
		!strings.Contains(err.Error(), "configure --ollama-url") {
		t.Fatalf("err = %v", err)
	}
}
