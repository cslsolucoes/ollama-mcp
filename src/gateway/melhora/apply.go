package melhora

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Example is one pair "error -> right" as the model sees it.
type Example struct {
	Pedido string // the request that went wrong (text, cut)
	Errada string // what the model answered
	Certa  string // what it should have answered
	// ID is the occurrence the example comes from. It is never shown to the model; it is the
	// trace of the job log (C-2.10a).
	ID string
}

// ErrUnsupportedBody is why Apply left a body alone (its system text has a shape
// that cannot be extended without changing its meaning).
var ErrUnsupportedBody = errors.New("melhora: forma do pedido não suportada")

// Apply appends the instruction and the examples to the system text of the body
// (see the package comment). With nothing to add it returns body itself, the same
// slice, and changed=false. On a body it cannot extend it returns the body unchanged
// and ErrUnsupportedBody (the caller sends the request as it came). Keys are
// matched case-insensitively, like the Ollama's decoder; every other value of the
// body is kept as it came, byte for byte, and the keys are written sorted.
func Apply(path string, body []byte, instr string, examples []Example, nonce string) (out []byte, changed bool, err error) {
	add := joinNonEmpty("\n\n", strings.TrimSpace(instr), FormatExamples(examples, nonce))
	if add == "" || !supportedPath(path) {
		return body, false, nil
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(body, &top) != nil || top == nil {
		return body, false, ErrUnsupportedBody
	}
	switch path {
	case "/api/generate":
		k, raw, ok := findKey(top, "system")
		if !ok {
			k = "system"
		}
		cur := ""
		if ok && !isNullRaw(raw) {
			if json.Unmarshal(raw, &cur) != nil {
				return body, false, ErrUnsupportedBody
			}
		}
		top[k] = marshalString(joinNonEmpty("\n\n", cur, add))
	default: // /api/chat and /v1/chat/completions
		k, raw, ok := findKey(top, "messages")
		if !ok {
			return body, false, ErrUnsupportedBody
		}
		var msgs []json.RawMessage
		if json.Unmarshal(raw, &msgs) != nil {
			return body, false, ErrUnsupportedBody
		}
		first := map[string]json.RawMessage(nil)
		if len(msgs) > 0 && json.Unmarshal(msgs[0], &first) == nil && first != nil {
			if _, rr, ok := findKey(first, "role"); ok {
				var role string
				if json.Unmarshal(rr, &role) == nil && role == "system" {
					ck, cr, cok := findKey(first, "content")
					cur := ""
					if cok && !isNullRaw(cr) {
						if json.Unmarshal(cr, &cur) != nil { // content as a list of parts: not extended
							return body, false, ErrUnsupportedBody
						}
					}
					if !cok {
						ck = "content"
					}
					first[ck] = marshalString(joinNonEmpty("\n\n", cur, add))
					msgs[0] = marshalObject(first)
					top[k] = marshalArray(msgs)
					return marshalObject(top), true, nil
				}
			}
		}
		sys := marshalObject(map[string]json.RawMessage{"role": json.RawMessage(`"system"`), "content": marshalString(add)})
		top[k] = marshalArray(append([]json.RawMessage{sys}, msgs...))
	}
	return marshalObject(top), true, nil
}

func joinNonEmpty(sep string, parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, sep)
}

func isNullRaw(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

// findKey is a case-insensitive lookup; the key is returned as written.
func findKey(m map[string]json.RawMessage, name string) (string, json.RawMessage, bool) {
	if v, ok := m[name]; ok {
		return name, v, true
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic when two keys differ only in case
	for _, k := range keys {
		if strings.EqualFold(k, name) {
			return k, m[k], true
		}
	}
	return "", nil, false
}

func marshalString(s string) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// marshalObject writes an object with its keys sorted and the values as they came,
// without HTML escaping.
func marshalObject(m map[string]json.RawMessage) json.RawMessage {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(marshalString(k))
		buf.WriteByte(':')
		buf.Write(m[k])
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.Bytes())
}

func marshalArray(items []json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, it := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(it)
	}
	buf.WriteByte(']')
	return json.RawMessage(buf.Bytes())
}

// ExampleOf builds the example of a sample: the text of its request, what the model
// answered (without the reasoning) and the right answer, each cut.
func ExampleOf(endpoint string, promptJSON, resposta []byte, certa string) Example {
	return Example{
		Pedido: exampleRequestText(promptJSON),
		Errada: cutRunes(strings.TrimSpace(sanitizeExampleText(cleanAnswer(string(resposta)))), MaxExampleFieldRunes),
		Certa:  cutRunes(strings.TrimSpace(sanitizeExampleText(certa)), MaxExampleFieldRunes),
	}
}
