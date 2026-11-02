package tools

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cslsoftwares/mcpollama/mcp"
)

// usage holds the counters of one model call (PB5-5, TOK-002). The final text
// line and _meta carry the same numbers.
type usage struct {
	model      string
	promptEval int
	eval       int
	hasEval    bool // embed has no generated tokens
	total      time.Duration
	load       time.Duration
	queue      time.Duration
	doneReason string
	parts      int // file parts of a map-reduce request; 0 for the other tools
	think      bool
	backend    string // "fallback" when the fallback server answered; "" for the primary
	jobs       []jobRef
}

// Job roles (contract C-1.2): unico, map1..mapN, reduce1..reduceN, final.
const roleUnico = "unico"

// jobRef is one model call as the gateway knows it: the job id (header
// X-Request-Id; empty when there is no gateway), the role the call played in
// the request and the backend that answered ("primario" or "fallback").
type jobRef struct {
	ID      string
	Role    string
	Backend string
}

// newJobRef builds the reference of the response used by a call.
func newJobRef(id, role, respBackend string) jobRef {
	b := "primario"
	if respBackend == "fallback" {
		b = "fallback"
	}
	return jobRef{ID: id, Role: role, Backend: b}
}

// ref is the job id as written in the [uso] line; the fallback adds @fallback.
func (r jobRef) ref() string {
	if r.Backend == "fallback" {
		return r.ID + "@fallback"
	}
	return r.ID
}

// jobsField is the tail of the [uso] line: job=<id> for a request of one call,
// jobs=<role>:<id>,... for more than one (in call order). Calls without an id
// are left out; with no id at all nothing is written.
func (u usage) jobsField() string {
	var parts []string
	for _, j := range u.jobs {
		if j.ID == "" {
			continue
		}
		if len(u.jobs) == 1 {
			return " job=" + j.ref()
		}
		parts = append(parts, j.Role+":"+j.ref())
	}
	if len(parts) == 0 {
		return ""
	}
	return " jobs=" + strings.Join(parts, ",")
}

// line is the fixed last line of every result that called the model.
func (u usage) line() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[uso] modelo=%s prompt_eval_count=%d", u.model, u.promptEval)
	if u.hasEval {
		fmt.Fprintf(&b, " eval_count=%d", u.eval)
	}
	fmt.Fprintf(&b, " total_ms=%d load_ms=%d queue_ms=%d", u.total.Milliseconds(), u.load.Milliseconds(), u.queue.Milliseconds())
	if u.parts > 0 {
		fmt.Fprintf(&b, " partes=%d", u.parts)
	}
	if u.doneReason != "" {
		fmt.Fprintf(&b, " fim=%s", u.doneReason)
	}
	if u.think {
		b.WriteString(" think=on")
	}
	if u.backend != "" {
		b.WriteString(" backend=" + u.backend)
	}
	b.WriteString(u.jobsField())
	return b.String()
}

func (u usage) meta() map[string]any {
	m := map[string]any{
		"model":             u.model,
		"prompt_eval_count": u.promptEval,
		"total_ms":          u.total.Milliseconds(),
		"load_ms":           u.load.Milliseconds(),
		"queue_ms":          u.queue.Milliseconds(),
	}
	if u.hasEval {
		m["eval_count"] = u.eval
	}
	if u.doneReason != "" {
		m["done_reason"] = u.doneReason
	}
	if u.parts > 0 {
		m["parts"] = u.parts
	}
	if u.think {
		m["think"] = true
	}
	if u.backend != "" {
		m["backend"] = u.backend
	}
	var jobs []map[string]any
	for _, j := range u.jobs {
		if j.ID != "" {
			jobs = append(jobs, map[string]any{"id": j.ID, "papel": j.Role, "backend": j.Backend})
		}
	}
	if len(jobs) > 0 {
		m["jobs"] = jobs
	}
	return map[string]any{"mcpollama/usage": m}
}

// capText applies the output ceiling and says so when it cuts (no silent
// truncation).
func capText(s string) string {
	n := utf8.RuneCountInString(s)
	if n <= maxOutputChars {
		return s
	}
	cut := string([]rune(s)[:maxOutputChars])
	return fmt.Sprintf("%s\n\n[resposta truncada pelo servidor MCP: %d de %d caracteres]", cut, maxOutputChars, n)
}

// textResult is a model answer followed by the usage line.
func textResult(body string, u usage) mcp.ToolCallResult {
	res := mcp.TextResult(body+"\n\n"+u.line(), false)
	res.Meta = u.meta()
	return res
}

// thinkExhausted is the error for a call with think on whose reasoning ate
// the whole num_predict: empty answer and done_reason "length" (plan 8.8b).
// The thinking text is not decoded (OLL-010), so only that shape is detected.
func thinkExhausted(think bool, text, doneReason string) error {
	if think && doneReason == "length" && strings.TrimSpace(text) == "" {
		return errors.New("o raciocínio (think) consumiu o limite de tokens e a resposta veio vazia; aumente `num_predict` ou use `think=false`")
	}
	return nil
}
