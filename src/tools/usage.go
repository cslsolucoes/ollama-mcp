package tools

import (
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
}

// line is the fixed last line of every result that called the model.
func (u usage) line() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[uso] modelo=%s prompt_eval_count=%d", u.model, u.promptEval)
	if u.hasEval {
		fmt.Fprintf(&b, " eval_count=%d", u.eval)
	}
	fmt.Fprintf(&b, " total_ms=%d load_ms=%d queue_ms=%d", u.total.Milliseconds(), u.load.Milliseconds(), u.queue.Milliseconds())
	if u.doneReason != "" {
		fmt.Fprintf(&b, " fim=%s", u.doneReason)
	}
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
