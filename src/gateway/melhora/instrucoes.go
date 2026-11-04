// Package melhora is the improvement without training (Fase 12, stage 12.8;
// contract chapter 8, D12-15): what the gateway adds to a request before the model
// sees it, and the same code the bake-off uses to measure it.
//
// Two things can be added, both off by default and switched on per tool:
//
//   - the instructions of the tool (a versioned text, C-8.6, C-8.7), and
//   - up to k examples "error -> right" taken from a frozen index of public
//     examples, found by embedding similarity (C-8.1 to C-8.5, C-8.8 to C-8.10).
//
// WHERE THEY GO. Both are appended to the system text of the request, after what
// the client wrote (native /api/generate: the "system" string; /api/chat and
// /v1/chat/completions: the first message when it is a system message, otherwise a
// new system message in front). The user text ("prompt" or the other messages) is
// never touched, so the markers of the file tools (ARQUIVO_INICIO <nonce> ...
// ARQUIVO_FIM <nonce>) and the document between them reach the model byte for
// byte. /v1/completions has no system text and is left alone. The examples are one block
// delimited by a marker with a NEW nonce for every request and declared as data, never as
// instruction (C-8.11); the text of an example has no system text and no marker lines.
//
// WHAT THE EMBEDDING IS MADE OF. The text of the request (system, then prompt or
// the messages with their roles) with the random id of the file markers replaced by
// "<id>", cut at MaxEmbedRunes. The same function makes the vector of an example
// (from the request of its sample) and the vector of the query. (What an example SHOWS is a
// different text, see ExampleOf.)
//
// Apply is deterministic for the same input (nonce included), and with nothing to add the
// body comes back as the very same bytes.
package melhora

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

// Tools are the tools the improvement may be switched on for (the tools whose jobs
// can be exported, C-3.7). The bake-off's own tool name is not one of them: its
// requests are transformed by the bake-off, never by the gateway.
var Tools = []string{"chat", "extract", "generate", "summarize_file", "translate"}

// IsTool says whether name is one of Tools.
func IsTool(name string) bool {
	for _, t := range Tools {
		if t == name {
			return true
		}
	}
	return false
}

// MaxInstrucaoBytes is the ceiling of the text of one tool and of the file.
const (
	MaxInstrucaoBytes = 16 << 10
	maxFileBytes      = 256 << 10
)

// Instrucoes is one version of the instructions per tool. ID and Hash come from the
// content, so the same content is always the same version (C-8.7).
type Instrucoes struct {
	ID   string
	Hash string
	text map[string]string
}

// fileFormat is the instructions file: {"tools": {"extract": "...", ...}}.
type fileFormat struct {
	Tools map[string]string `json:"tools"`
}

// ParseInstrucoes reads the instructions file. Unknown tools, a text over
// MaxInstrucaoBytes, invalid UTF-8 and unknown keys are errors. An entry that is
// empty or only spaces counts as no instruction. A file with no instruction at all is
// the empty version (v0): it changes nothing, and still has an ID and a hash.
func ParseInstrucoes(b []byte) (*Instrucoes, error) {
	if len(b) > maxFileBytes {
		return nil, errors.New("arquivo de instruções grande demais")
	}
	if !utf8.Valid(b) {
		return nil, errors.New("arquivo de instruções não é UTF-8")
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var f fileFormat
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("arquivo de instruções inválido: %w", err)
	}
	if dec.More() {
		return nil, errors.New("arquivo de instruções inválido: texto depois do objeto")
	}
	in := &Instrucoes{text: map[string]string{}}
	for tool, txt := range f.Tools {
		if !IsTool(tool) {
			return nil, fmt.Errorf("instrução para a tool desconhecida %q", tool)
		}
		if len(txt) > MaxInstrucaoBytes {
			return nil, fmt.Errorf("instrução da tool %s acima de %d bytes", tool, MaxInstrucaoBytes)
		}
		if !utf8.ValidString(txt) {
			return nil, fmt.Errorf("instrução da tool %s não é UTF-8", tool)
		}
		if strings.TrimSpace(txt) == "" {
			continue
		}
		in.text[tool] = txt
	}
	in.Hash = hashOf(in.text)
	in.ID = "i" + in.Hash[:12]
	return in, nil
}

// LoadInstrucoes reads the file at path.
func LoadInstrucoes(path string) (*Instrucoes, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("instruções: %w", errors.Unwrap(err))
	}
	if fi.Size() > maxFileBytes {
		return nil, errors.New("arquivo de instruções grande demais")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("instruções: %w", errors.Unwrap(err))
	}
	return ParseInstrucoes(b)
}

// hashOf is the sha256 of the entries in tool order, each as tool NUL text NUL.
func hashOf(m map[string]string) string {
	tools := make([]string, 0, len(m))
	for t := range m {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	h := sha256.New()
	h.Write([]byte("mcpollama-instrucoes/1\n"))
	for _, t := range tools {
		fmt.Fprintf(h, "%s\x00%d\x00%s\x00", t, len(m[t]), m[t])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// For is the instruction of a tool, "" when it has none. Safe on a nil receiver.
func (i *Instrucoes) For(tool string) string {
	if i == nil {
		return ""
	}
	return i.text[tool]
}

// Empty says whether the version changes nothing.
func (i *Instrucoes) Empty() bool { return i == nil || len(i.text) == 0 }

// ToolsWithText lists the tools that have an instruction, sorted.
func (i *Instrucoes) ToolsWithText() []string {
	if i == nil {
		return nil
	}
	out := make([]string, 0, len(i.text))
	for t := range i.text {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
