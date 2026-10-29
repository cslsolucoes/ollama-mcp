package proxy

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// Request policies (Fase 9.6a). They run in the proxy BEFORE the request is
// queued, on the routes that reach the model:
//
//  1. dangerous options are removed from the body (keep_alive, num_ctx and the
//     GPU/memory placement options): the Ollama keeps the model resident with
//     its own context, and any other value reloads (~20 s) or unloads it for
//     every client;
//  2. reasoning: the gateway decides it when the client did not (default,
//     X-Mcpollama-Think header, virtual model name "<model>-think");
//  3. the body is rewritten safely (see applyPolicy).
//
// Privacy: no text of the body is ever copied into a message, log or record.

// HeaderThink is the optional request header "X-Mcpollama-Think: on|off". It
// applies only when the body does not define the reasoning itself.
const HeaderThink = "X-Mcpollama-Think"

// DefaultStripOptions are the options removed from "options" by default.
var DefaultStripOptions = []string{"num_gpu", "main_gpu", "low_vram", "use_mmap", "use_mlock", "numa"}

// Reasoning modes stored for each request (metadata only).
const (
	ThinkOn    = "on"     // reasoning on (forced, or asked by the client)
	ThinkOff   = "off"    // reasoning off
	ThinkModel = "modelo" // left to the model default
)

type routeKind int

const (
	kindNative routeKind = iota + 1 // /api/generate, /api/chat
	kindV1                          // /v1/chat/completions, /v1/completions
	kindEmbed                       // /api/embed, /api/embeddings, /v1/embeddings
)

var routeKinds = map[string]routeKind{
	"/api/generate":        kindNative,
	"/api/chat":            kindNative,
	"/v1/chat/completions": kindV1,
	"/v1/completions":      kindV1,
	"/api/embed":           kindEmbed,
	"/api/embeddings":      kindEmbed,
	"/v1/embeddings":       kindEmbed,
}

// policyCfg is the part of Config the rewrite reads.
type policyCfg struct {
	defaultThink      bool
	thinkSuffix       string
	allowKeepAlive    bool
	allowNumCtx       bool
	stripOptions      []string
	stripOptionsLower map[string]bool
}

func newPolicyCfg(c Config) policyCfg {
	p := policyCfg{
		defaultThink: c.DefaultThink, thinkSuffix: c.ThinkSuffix,
		allowKeepAlive: c.AllowClientKeepAlive, allowNumCtx: c.AllowClientNumCtx,
		stripOptions: c.StripOptions, stripOptionsLower: map[string]bool{},
	}
	for _, o := range c.StripOptions {
		if o = strings.TrimSpace(o); o != "" {
			p.stripOptionsLower[strings.ToLower(o)] = true
		}
	}
	return p
}

// policyResult is what applyPolicy decides.
type policyResult struct {
	body  []byte // the body to forward (the input itself when nothing changed)
	model string // the BASE model name (suffix removed): what the allowlist checks
	think string // ThinkOn, ThinkOff, ThinkModel or "" (not applicable)
}

type policyError struct {
	status int
	msg    string
}

var (
	errNotObject = &policyError{400, "corpo JSON inválido: esperado um objeto"}
	errBadField  = &policyError{400, "corpo JSON inválido: campo com tipo errado"}
	errDupField  = &policyError{400, "corpo JSON inválido: campo repetido com maiúsculas diferentes"}
)

// body is a decoded request body. The JSON of each value is kept as it came
// (RawMessage), so the text of the prompt is never parsed or touched.
type body struct {
	m       map[string]json.RawMessage
	changed bool
}

// take removes the key matching name (case-insensitively, like the Go decoder
// of the Ollama does) and returns its key as written and its value. Two keys
// that differ only in case cannot be told apart (the order is lost in a map),
// so they are refused.
func (b *body) take(name string) (key string, raw json.RawMessage, ok bool, err *policyError) {
	for k, v := range b.m {
		if strings.EqualFold(k, name) {
			if ok {
				return "", nil, false, errDupField
			}
			key, raw, ok = k, v, true
		}
	}
	if ok {
		delete(b.m, key)
	}
	return key, raw, ok, nil
}

func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

// applyPolicy implements the policies for one request. hdr is the raw value of
// X-Mcpollama-Think. A nil error means the body is valid JSON (an object).
func (p policyCfg) applyPolicy(path string, in []byte, hdr string) (policyResult, *policyError) {
	kind := routeKinds[path]
	var m map[string]json.RawMessage
	if err := json.Unmarshal(in, &m); err != nil || m == nil {
		return policyResult{}, errNotObject
	}
	b := &body{m: m}
	res := policyResult{body: in}

	// model (and the virtual "-think" name)
	suffixed := false
	mk, mraw, mok, perr := b.take("model")
	if perr != nil {
		return policyResult{}, perr
	}
	if mok {
		var name string
		if !isNull(mraw) {
			if err := json.Unmarshal(mraw, &name); err != nil {
				return policyResult{}, errBadField
			}
		}
		res.model = name
		if kind != kindEmbed && p.thinkSuffix != "" && len(name) > len(p.thinkSuffix) && strings.HasSuffix(name, p.thinkSuffix) {
			res.model = strings.TrimSuffix(name, p.thinkSuffix)
			suffixed = true
			b.changed = true
			delete(b.m, mk)
			b.m["model"] = marshalString(res.model)
		} else {
			b.m[mk] = mraw
		}
	}

	// keep_alive (top level)
	if !p.allowKeepAlive {
		_, _, ok, perr := b.take("keep_alive")
		if perr != nil {
			return policyResult{}, perr
		}
		if ok {
			b.changed = true
		}
	}

	// options.*
	if perr := p.stripFromOptions(b); perr != nil {
		return policyResult{}, perr
	}

	// reasoning
	hdrMode := parseThinkHeader(hdr)
	switch kind {
	case kindNative:
		k, raw, ok, perr := b.take("think")
		if perr != nil {
			return policyResult{}, perr
		}
		if ok && !isNull(raw) {
			b.m[k] = raw // explicit: respected
			res.think = ThinkOn
			if string(bytes.TrimSpace(raw)) == "false" {
				res.think = ThinkOff
			}
			break
		}
		if ok {
			b.changed = true // a null think is replaced
		}
		on := p.defaultThink
		switch {
		case hdrMode != "":
			on = hdrMode == ThinkOn
		case suffixed:
			on = true
		}
		b.changed = true
		if on {
			b.m["think"], res.think = json.RawMessage("true"), ThinkOn
		} else {
			b.m["think"], res.think = json.RawMessage("false"), ThinkOff
		}
	case kindV1:
		ek, eraw, eok, perr := b.take("reasoning_effort")
		if perr != nil {
			return policyResult{}, perr
		}
		rk, rraw, rok, perr := b.take("reasoning")
		if perr != nil {
			return policyResult{}, perr
		}
		eok = eok && !isNull(eraw)
		rok = rok && !isNull(rraw)
		if eok || rok {
			res.think = ThinkOn
			if eok {
				b.m[ek] = eraw
				if effortIsNone(eraw) {
					res.think = ThinkOff
				}
			}
			if rok {
				b.m[rk] = rraw
				if res.think != ThinkOff && reasoningIsNone(rraw) {
					res.think = ThinkOff
				}
			}
			break
		}
		b.changed = b.changed || eraw != nil || rraw != nil // null ones are dropped
		off := !p.defaultThink
		switch {
		case hdrMode != "":
			off = hdrMode == ThinkOff
			res.think = hdrMode
		case suffixed:
			off, res.think = false, ThinkOn
		case off:
			res.think = ThinkOff
		default:
			res.think = ThinkModel
		}
		if off {
			b.changed = true
			b.m["reasoning_effort"] = json.RawMessage(`"none"`)
		}
	}

	if b.changed {
		res.body = b.marshal()
	}
	return res, nil
}

// stripFromOptions removes the dangerous keys from the "options" object.
func (p policyCfg) stripFromOptions(b *body) *policyError {
	k, raw, ok, perr := b.take("options")
	if perr != nil {
		return perr
	}
	if !ok {
		return nil
	}
	b.m[k] = raw
	var opts map[string]json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &opts) != nil || opts == nil {
		return nil // not an object: the Ollama will judge it
	}
	removed := false
	for name := range opts {
		if p.stripOptionsLower[strings.ToLower(name)] || (!p.allowNumCtx && strings.EqualFold(name, "num_ctx")) {
			delete(opts, name)
			removed = true
		}
	}
	if removed {
		b.changed = true
		b.m[k] = marshalObject(opts)
	}
	return nil
}

func parseThinkHeader(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on":
		return ThinkOn
	case "off":
		return ThinkOff
	}
	return "" // invalid values are ignored
}

// effortIsNone reports whether a reasoning_effort value is "none".
func effortIsNone(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && strings.EqualFold(strings.TrimSpace(s), "none")
}

// reasoningIsNone reports whether a "reasoning" object asks for none.
func reasoningIsNone(raw json.RawMessage) bool {
	var o struct {
		Effort string `json:"effort"`
	}
	return json.Unmarshal(raw, &o) == nil && strings.EqualFold(strings.TrimSpace(o.Effort), "none")
}

func marshalString(s string) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// marshalObject writes the object with its keys sorted and the values as they
// came (RawMessage), without HTML escaping.
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

func (b *body) marshal() []byte { return marshalObject(b.m) }
