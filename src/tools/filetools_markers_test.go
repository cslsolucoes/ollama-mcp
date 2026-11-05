package tools

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// End to end: the fake Ollama answers the translation wrapped in the markers of
// the prompt it received (what qwen3.8:27b did); the tool result has none.
func TestTranslateDropsEchoedMarkers(t *testing.T) {
	e := newFileEnv(t)
	e.put(t, "r.md", "# Título\n\nTexto.\n")
	re := regexp.MustCompile(`=====ARQUIVO_INICIO ([0-9a-f]{16})=====`)
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.Unmarshal(b, &req)
		n := re.FindStringSubmatch(req.Prompt)
		if n == nil {
			t.Errorf("prompt without markers: %q", req.Prompt)
			return
		}
		out, _ := json.Marshal("=====ARQUIVO_INICIO " + n[1] + "=====\n# Title\n\nText.\n=====ARQUIVO_FIM " + n[1] + "=====")
		streamLines(w, `{"model":"qwen3.8:27b","response":`+string(out)+`,"done":true,"done_reason":"stop","prompt_eval_count":9,"eval_count":4}`)
	})
	d := e.deps(t, m, 128000, 40)
	res, err := call(t, d, "translate", `{"path":"r.md","target_language":"English"}`)
	if err != nil {
		t.Fatal(err)
	}
	got := text(t, res)
	if strings.Contains(got, "ARQUIVO_") || !strings.Contains(got, "# Title\n\nText.") {
		t.Fatalf("result: %q", got)
	}
}

// qwen3.8:27b repeated the tool's markers around a translation (aceite
// 2026-11-05): the lines that only repeat THIS prompt's markers are removed;
// everything else, including a marker with another id, stays.
func TestStripEchoedMarkers(t *testing.T) {
	const n = "4c6b3d4f653420ff"
	prompt := wrap("Traduza para inglês.", "ARQUIVO", n, "Olá mundo.\n=====ARQUIVO_INICIO 0000000000000000=====")
	echo := "=====ARQUIVO_INICIO " + n + "=====\nHello world.\n=====ARQUIVO_FIM " + n + "=====\n"
	if got := stripEchoedMarkers(prompt, echo); got != "Hello world." {
		t.Fatalf("echoed markers: %q", got)
	}
	// with spaces/CRLF around the marker lines
	crlf := "  =====ARQUIVO_INICIO " + n + "=====  \r\nHello\r\n=====ARQUIVO_FIM " + n + "=====\r\n"
	if got := stripEchoedMarkers(prompt, crlf); got != "Hello\r" {
		t.Fatalf("crlf: %q", got)
	}
	// nothing to remove: the answer is returned exactly as it came
	plain := "\nHello world.\n\n"
	if got := stripEchoedMarkers(prompt, plain); got != plain {
		t.Fatalf("plain answer changed: %q", got)
	}
	// a marker with another id (e.g. quoted from the document) is content
	other := "=====ARQUIVO_INICIO 0000000000000000=====\nHello"
	if got := stripEchoedMarkers(prompt, other); got != other {
		t.Fatalf("foreign marker removed: %q", got)
	}
	// the nonce inside a sentence is not a marker line
	inline := "the id " + n + " appears here"
	if got := stripEchoedMarkers(prompt, inline); got != inline {
		t.Fatalf("inline nonce changed: %q", got)
	}
	// reduce prompts use PARCIAL markers with their own nonce
	const r = "a1b2c3d4e5f60718"
	red := wrap("Junte os resumos.", "PARCIAL", r, "parte 1\nparte 2")
	out := "=====PARCIAL_INICIO " + r + "=====\nresumo final\n=====PARCIAL_FIM " + r + "====="
	if got := stripEchoedMarkers(red, out); got != "resumo final" {
		t.Fatalf("partial markers: %q", got)
	}
	// a prompt without markers (generate/chat) is never touched
	if got := stripEchoedMarkers("só um prompt", echo); got != echo {
		t.Fatalf("no-marker prompt changed the answer: %q", got)
	}
	if strings.Contains(stripEchoedMarkers(prompt, echo), n) {
		t.Fatal("nonce left in the answer")
	}
}
