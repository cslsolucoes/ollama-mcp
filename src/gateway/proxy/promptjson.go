package proxy

// The canonical request kept for the training data (Fase 12, contract C-2.1,
// C-2.2, C-2.5, C-2.10): the body that goes to the Ollama, after the policy,
// with every image replaced by a marker {"imagem_hmac":"<hex>"}. Everything
// else is kept byte for byte: the replacement is a splice of the original
// bytes, not a decode and re-encode.
//
// The scan fails CLOSED: if it cannot read the whole body, nothing of the body
// is kept (Failed), the job is marked as having an image and as truncated, so
// it can never be exported.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// imageMarker is the JSON that stands for an image: HMAC-SHA256 of the image as
// it arrived, with the key of the contract; empty without a key (never a plain
// sha256, C-2.5).
func imageMarker(key *hmackey.Key, raw []byte) []byte {
	return []byte(`{"imagem_hmac":"` + key.MAC(raw) + `"}`)
}

// PromptJSON is what the proxy hands to the journal besides the flat text.
type PromptJSON struct {
	JSON     []byte // the body with the images replaced by markers (the body itself when it has none); nil when Failed
	HasImage bool   // some image was replaced (tem_imagem); true too when Failed
	// Failed: the scan could not read the whole body. No JSON is kept; the job is
	// stored with prompt_json NULL, tem_imagem=1 and prompt_json_truncated=1.
	Failed bool
	// The trace of the improvement without training (C-2.10a), set only when it changed the request:
	// JSON above is then the body the Ollama received, Original the request before the improvement
	// (images replaced by markers too), and the ids say what was added.
	Melhora      bool
	Original     []byte
	InstrucoesID string
	IndiceID     string
	Exemplos     []string
}

// JSONJournal is the optional extension of Journal that receives the canonical
// request. The proxy calls EnqueuedJSON instead of Enqueued when the journal
// implements it and WantsPromptJSON is true.
type JSONJournal interface {
	Journal
	EnqueuedJSON(rec queue.Record, prompt string, pj PromptJSON)
	// WantsPromptJSON is false when the content is not recorded
	// (--log-content=false): the proxy then does not even scan the body.
	WantsPromptJSON() bool
}

// promptJSONBuilds counts buildPromptJSON calls (tests: with the content off the
// scan must not run).
var promptJSONBuilds atomic.Int64

type span struct {
	start, end int
	repl       []byte
}

// The image rules, as paths from the top-level object. "*" is any array element.
var imageRules = [...][]string{
	{"images", "*"},                                // /api/generate
	{"messages", "*", "images", "*"},               // /api/chat
	{"messages", "*", "content", "*", "image_url"}, // /v1/chat/completions
}

const (
	ruleAll    = 1<<len(imageRules) - 1
	ruleURLIdx = 2 // the rule whose target value is replaced as a whole
)

// buildPromptJSON returns the prompt_json of body (already the policy's output).
// Images are the strings of "images" (top level, /api/generate; and each
// message, /api/chat) and the "image_url" value of each content part of a
// message (/v1/chat/completions). Keys are matched like the Ollama's decoder
// does (Unicode case folding, escapes decoded) and every duplicate is replaced.
// A body that cannot be scanned to the end fails closed (Failed).
func buildPromptJSON(body []byte, key *hmackey.Key) PromptJSON {
	out, has, ok := spliceImages(body, key)
	if !ok {
		return PromptJSON{HasImage: true, Failed: true}
	}
	return PromptJSON{JSON: out, HasImage: has}
}

// spliceImages is the shared scan: body with the images replaced by markers
// (body itself when there are none), whether any was found, and ok=false when
// the scan could not read the whole body.
func spliceImages(body []byte, key *hmackey.Key) (out []byte, has, ok bool) {
	promptJSONBuilds.Add(1)
	spans, ok := findImages(body, key)
	if !ok {
		return nil, false, false
	}
	if len(spans) == 0 {
		return body, false, true
	}
	var b bytes.Buffer
	b.Grow(len(body))
	pos := 0
	for _, sp := range spans {
		b.Write(body[pos:sp.start])
		b.Write(sp.repl)
		pos = sp.end
	}
	b.Write(body[pos:])
	return b.Bytes(), true, true
}

// frame is one open container. mask holds the image rules still alive for its
// children and depth how many path segments were matched to reach it. Nothing
// else is tracked, so a body nested 10,000 levels deep costs one small frame
// per level and no path allocation.
type frame struct {
	obj       bool
	expectKey bool // an object waits for its next key
	mask      uint8
	depth     uint8
	key       string // current key, kept only while mask != 0
}

// childMask returns the rules alive for the value that sits in f under the
// current key (object) or at any index (array), and the rules that are
// complete with exactly that value.
func childMask(f *frame) (alive, complete uint8) {
	for r := range imageRules {
		bit := uint8(1) << r
		if f.mask&bit == 0 {
			continue
		}
		pat := imageRules[r]
		d := int(f.depth)
		if d >= len(pat) {
			continue
		}
		seg := pat[d]
		if f.obj {
			if seg == "*" || !strings.EqualFold(f.key, seg) {
				continue
			}
		} else if seg != "*" {
			continue
		}
		if d == len(pat)-1 {
			complete |= bit
		} else {
			alive |= bit
		}
	}
	return alive, complete
}

// valueStart finds where the value that begins at or after off starts.
func valueStart(body []byte, off int) int {
	for off < len(body) {
		switch body[off] {
		case ' ', '\t', '\r', '\n', ',', ':':
			off++
		default:
			return off
		}
	}
	return off
}

// stringMAC hashes the decoded string value of the token spanning
// body[start:end] (quotes included). Without a backslash the decoded value is
// the slice between the quotes, so no copy of a large image is made.
func stringMAC(key *hmackey.Key, body []byte, start, end int, decoded string) []byte {
	if end-start >= 2 {
		inner := body[start+1 : end-1]
		if bytes.IndexByte(inner, '\\') < 0 {
			return imageMarker(key, inner)
		}
	}
	return imageMarker(key, []byte(decoded))
}

// findImages scans body once and lists the byte spans to replace. ok is false
// when the scan could not reach the end of a single well-formed JSON value.
func findImages(body []byte, key *hmackey.Key) (spans []span, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // a number such as 1e400 is text here, not a float that fails to parse
	var st []frame
	seen := false
	done := func() { // a value ended in its parent
		if n := len(st); n > 0 && st[n-1].obj {
			st[n-1].expectKey = true
		}
	}
	for {
		off0 := int(dec.InputOffset())
		tok, err := dec.Token()
		if err != nil {
			// An empty body is not JSON: at least one value must have been read.
			return spans, seen && len(st) == 0 && errors.Is(err, io.EOF)
		}
		seen = true
		n := len(st)
		if n > 0 && st[n-1].obj && st[n-1].expectKey && tok != json.Delim('}') {
			s, isStr := tok.(string)
			if !isStr {
				return nil, false
			}
			f := &st[n-1]
			f.expectKey = false
			if f.mask != 0 {
				f.key = s
			}
			continue
		}
		if d, isD := tok.(json.Delim); isD && (d == '}' || d == ']') {
			st = st[:n-1]
			done()
			continue
		}
		// A value starts. Which rules does it satisfy?
		var alive, complete uint8
		if n == 0 {
			alive = ruleAll // the top-level value: its children start the paths
		} else {
			alive, complete = childMask(&st[n-1])
		}
		switch t := tok.(type) {
		case json.Delim: // '{' or '['
			if complete&(1<<ruleURLIdx) != 0 {
				// The whole value (an object, or an array) becomes a marker.
				start := valueStart(body, off0)
				depth := 1
				for depth > 0 {
					tk, err := dec.Token()
					if err != nil {
						return nil, false
					}
					if dd, isD := tk.(json.Delim); isD {
						if dd == '{' || dd == '[' {
							depth++
						} else {
							depth--
						}
					}
				}
				end := int(dec.InputOffset())
				spans = append(spans, span{start, end, imageMarker(key, body[start:end])})
				done()
				continue
			}
			fr := frame{obj: t == '{', expectKey: t == '{', mask: alive}
			if n > 0 && fr.mask != 0 { // depth only matters while a rule is alive
				fr.depth = st[n-1].depth + 1
			}
			st = append(st, fr)
		case string:
			if complete != 0 {
				start := valueStart(body, off0)
				end := int(dec.InputOffset())
				spans = append(spans, span{start, end, stringMAC(key, body, start, end, t)})
			}
			done()
		default: // number, bool, null
			if complete&(1<<ruleURLIdx) != 0 {
				start := valueStart(body, off0)
				end := int(dec.InputOffset())
				spans = append(spans, span{start, end, imageMarker(key, body[start:end])})
			}
			done()
		}
	}
}
