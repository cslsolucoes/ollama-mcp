package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/tools"
)

// Fase 12.5: --auto-report (env MCPOLLAMA_AUTO_REPORT), OFF by default (C-1.20a).
func TestAutoReportFlagEnvAndDefault(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  map[string]string
		want bool
	}{
		{"default off", nil, nil, false},
		{"flag false", []string{"--auto-report=false"}, nil, false},
		{"flag true", []string{"--auto-report"}, nil, true},
		{"env 0", nil, map[string]string{"MCPOLLAMA_AUTO_REPORT": "0"}, false},
		{"env false", nil, map[string]string{"MCPOLLAMA_AUTO_REPORT": "false"}, false},
		{"env 1", nil, map[string]string{"MCPOLLAMA_AUTO_REPORT": "1"}, true},
		{"flag beats env", []string{"--auto-report=false"}, map[string]string{"MCPOLLAMA_AUTO_REPORT": "1"}, false},
		{"flag true beats env", []string{"--auto-report=true"}, map[string]string{"MCPOLLAMA_AUTO_REPORT": "0"}, true},
	}
	for _, c := range cases {
		if got := parse(t, c.args, c.env).AutoReport; got != c.want {
			t.Errorf("%s: AutoReport = %v, want %v", c.name, got, c.want)
		}
	}
}

// With the real wiring: an empty answer with a job id is reported in the
// background with --auto-report, and not at all by default or with =false.
func TestAutoReportWiring(t *testing.T) {
	run := func(extra ...string) (reported bool, text string) {
		f := newFakeOllama(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Request-Id", strings.Repeat("ab", 16))
			w.Header().Set("Content-Type", "application/x-ndjson")
			fmt.Fprintln(w, `{"model":"qwen3-ia","response":"   ","done":false}`)
			fmt.Fprintln(w, `{"model":"qwen3-ia","response":"","done":true,"done_reason":"stop","total_duration":1,"prompt_eval_count":1,"eval_count":1}`)
		})
		cfg := parse(t, append([]string{"--ollama-url", f.srv.URL}, extra...), nil)
		var b bytes.Buffer
		entries, closeTools := buildToolsWithClose(cfg, &logger{w: &b, redact: func(s string) string { return s }})
		defer closeTools()
		for _, e := range entries {
			if e.Def.Name != "generate" {
				continue
			}
			res, err := e.Handler(context.Background(), json.RawMessage(`{"prompt":"x"}`))
			if err != nil || res.IsError {
				t.Fatalf("generate: %v %+v", err, res)
			}
			text = res.Content[0].Text
		}
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			f.mu.Lock()
			for _, p := range f.paths {
				if p == "/api/feedback" {
					reported = true
				}
			}
			f.mu.Unlock()
			if reported {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		return reported, text
	}
	on, textOn := run("--auto-report")
	if !on {
		t.Fatal("--auto-report: the empty answer was not reported")
	}
	if def, _ := run(); def {
		t.Fatal("default: a report was sent (the feature is off by default)")
	}
	off, textOff := run("--auto-report=false")
	if off {
		t.Fatal("--auto-report=false: a report was sent")
	}
	// the answer is the same with and without the feature (same [uso] line, job id included)
	if textOn != textOff {
		t.Fatalf("the answer changed with the feature:\n%q\n%q", textOn, textOff)
	}
}

// M-8: the closer returned with the tools stops the background sender, so a
// test (or the server) that builds the tools with --auto-report leaves no
// goroutine behind.
func TestBuildToolsCloseStopsTheSender(t *testing.T) {
	// The sender's own "done" channel, not runtime.NumGoroutine: other tests of
	// the package leave goroutines winding down, which made a global count flaky.
	var started *tools.AutoReporter
	orig := newAutoReporter
	newAutoReporter = func(b func() (tools.Backend, error), logf func(string, ...any)) *tools.AutoReporter {
		started = orig(b, logf)
		return started
	}
	t.Cleanup(func() { newAutoReporter = orig })

	cfg := parse(t, []string{"--ollama-url", "https://exemplo.invalid", "--auto-report"}, nil)
	_, closeTools := buildToolsWithClose(cfg, &logger{w: &bytes.Buffer{}, redact: func(s string) string { return s }})
	if started == nil {
		t.Fatal("--auto-report did not start the sender")
	}
	select {
	case <-started.Done():
		t.Fatal("the sender ended before Close")
	case <-time.After(50 * time.Millisecond):
	}
	closeTools()
	select {
	case <-started.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the sender goroutine is still running after Close")
	}
	closeTools() // idempotent
}
