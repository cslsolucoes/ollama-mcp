package melhora

import (
	"encoding/json"
	"regexp"
	"strings"
)

// MaxEmbedRunes cuts the text an embedding is made of (nomic-embed-text reads 8192
// tokens; this keeps well inside it even for dense text).
const MaxEmbedRunes = 4000

// Limits of what an example shows (runes per field and for the whole block).
const (
	MaxExampleFieldRunes = 1200
	MaxBlockRunes        = 6000
)

// nonceRe finds the random id of the file markers (tools/filetools.go: 8 random
// bytes in hex, between "=====<LABEL>_INICIO " / "_FIM " and "=====").
var nonceRe = regexp.MustCompile(`(=====[A-Z]+_(?:INICIO|FIM)) [0-9a-f]{16}(=====)`)

// supportedPaths are the routes whose system text can be extended.
func supportedPath(path string) bool {
	switch path {
	case "/api/generate", "/api/chat", "/v1/chat/completions":
		return true
	}
	return false
}

type reqShape struct {
	System   json.RawMessage `json:"system"`
	Prompt   json.RawMessage `json:"prompt"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

// contentText reads a JSON string or an array of parts with "text".
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Text != "" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

// RequestText is the text a request is embedded by (see the package comment): the
// system text, then the prompt or the messages with their roles; the random id of
// the file markers is replaced by "<id>" so two requests about the same task get the
// same text; cut at MaxEmbedRunes. ok is false when there is nothing to embed.
func RequestText(path string, body []byte) (string, bool) {
	var r reqShape
	if json.Unmarshal(body, &r) != nil {
		return "", false
	}
	var sb strings.Builder
	if s := contentText(r.System); strings.TrimSpace(s) != "" {
		sb.WriteString("[sistema]\n" + s + "\n")
	}
	if s := contentText(r.Prompt); strings.TrimSpace(s) != "" {
		sb.WriteString("[pedido]\n" + s + "\n")
	}
	for _, m := range r.Messages {
		if s := contentText(m.Content); strings.TrimSpace(s) != "" {
			sb.WriteString("[" + m.Role + "]\n" + s + "\n")
		}
	}
	t := strings.TrimSpace(nonceRe.ReplaceAllString(sb.String(), "$1 <id>$2"))
	if t == "" {
		return "", false
	}
	return cutRunes(t, MaxEmbedRunes), true
}

// cutRunes keeps at most n runes.
func cutRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

// cleanAnswer is the stored answer of a job without the model's reasoning: the job
// log marks it "[raciocínio] ..." and "[resposta] ...".
func cleanAnswer(s string) string {
	if strings.HasPrefix(s, "[raciocínio] ") {
		if i := strings.Index(s, "\n[resposta] "); i >= 0 {
			return strings.TrimSpace(s[i+len("\n[resposta] "):])
		}
		return ""
	}
	return strings.TrimSpace(s)
}

// FormatVersion is the version of the format of the examples: what ExampleOf keeps of a
// sample, the limits of MaxExampleFieldRunes and MaxBlockRunes, and the layout of
// FormatExamples. It is part of the hash of a frozen index (C-8.10a), so an index made
// with another format is never served as if it were the same. Change it whenever any of
// those changes (TestFormatExamplesGolden fails to remind you).
const FormatVersion = "ex1"

// markerLineRe finds a line that looks like a marker of the file tools or of the block of
// examples: it starts and ends with "=====". Such lines are removed from the text of an
// example (C-8.11): a marker that came from a document, open or closed, never reaches the
// block, and the cut can never leave one open.
var markerLineRe = regexp.MustCompile(`(?m)^[ \t]*=====[^\n]*=====[ \t]*\r?$\n?`)

// sanitizeExampleText removes the marker lines. The distinct nonces of distinct markers are
// not unified: the lines are simply gone.
func sanitizeExampleText(s string) string { return markerLineRe.ReplaceAllString(s, "") }

// exampleRequestText is the request of a sample as an example shows it: the prompt or the
// messages with their roles, WITHOUT the system text (it is the same fixed text for every
// request of a tool and the model gets its own), with the marker lines removed, cut.
func exampleRequestText(body []byte) string {
	var r reqShape
	if json.Unmarshal(body, &r) != nil {
		return ""
	}
	var sb strings.Builder
	if s := contentText(r.Prompt); strings.TrimSpace(s) != "" {
		sb.WriteString(s + "\n")
	}
	for _, m := range r.Messages {
		if m.Role == "system" {
			continue
		}
		if s := contentText(m.Content); strings.TrimSpace(s) != "" {
			sb.WriteString("[" + m.Role + "]\n" + s + "\n")
		}
	}
	return cutRunes(strings.TrimSpace(sanitizeExampleText(sb.String())), MaxExampleFieldRunes)
}
