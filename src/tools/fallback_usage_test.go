package tools

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/ollama"
)

// Fase 10.4: when the fallback server answers, the [uso] line says
// backend=fallback; with the primary there is no such field at all.
func TestUsageLineNamesTheFallback(t *testing.T) {
	primary := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":"fila do servidor cheia"}`, http.StatusServiceUnavailable)
	})
	fb := newMock(t, generateOK)
	d := primary.deps(t, func(o *ollama.Options) { o.FallbackURL = fb.srv.URL })
	res, err := call(t, d, "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	line := lastLine(text(t, res))
	if !strings.HasPrefix(line, "[uso]") || !strings.Contains(line, " backend=fallback") {
		t.Fatalf("usage line: %q", line)
	}
	if meta, _ := res.Meta["mcpollama/usage"].(map[string]any); meta["backend"] != "fallback" {
		t.Fatalf("meta %+v", res.Meta)
	}
}

func TestUsageLineWithoutFallbackHasNoBackend(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	res, err := call(t, d, "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if line := lastLine(text(t, res)); strings.Contains(line, "backend=") {
		t.Fatalf("usage line: %q", line)
	}
}
