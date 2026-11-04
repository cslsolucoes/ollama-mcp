package main

// Scoring and statistics of the suite `erros` (Fase 12, stage 12.7; contract
// chapter 5). Pure functions: nothing here touches the network or the disk.

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strings"
)

// Modes of scoring of a category (C-5.2 to C-5.7).
const (
	modeItems   = "itens"    // C-5.2
	modeJSON    = "json"     // C-5.3
	modeNumber  = "numero"   // C-5.4
	modeTrunc   = "truncado" // C-5.4
	modeBlind   = "blind"    // C-5.5
	modeOutside = "fora"     // C-5.7 (and any category the contract does not know)
)

// categoryMode maps a category of C-1.16 to how the evaluation scores it.
func categoryMode(cat string) string {
	switch cat {
	case "numeracao_errada", "omitiu":
		return modeItems
	case "formato_invalido":
		return modeJSON
	case "contagem_errada", "calculo_errado":
		return modeNumber
	case "truncado":
		return modeTrunc
	case "resumo_infiel", "inventou", "traducao_errada", "ignorou_instrucao":
		return modeBlind
	}
	return modeOutside // outro, idioma_errado
}

// verdict is the outcome of scoring one answer. Applicable=false means the
// criterion cannot be applied (for instance the correct answer is not JSON in a
// `formato_invalido` case): the case is not counted as right or wrong.
type verdict struct {
	Pass       bool
	Applicable bool
	Why        string
	Precision  float64 // modeItems only
	Coverage   float64 // modeItems only
}

// reply is what one replayed call returned.
type reply struct {
	Text      string
	Fim       string // done_reason / finish_reason ("stop", "length")
	Final     bool   // the stream ended normally (resp_final)
	Truncated bool   // the answer hit the 16 MiB ceiling (resp_truncated)
}

// scoreReply applies the criterion of the category to the answer against the
// correct answer of the occurrence.
func scoreReply(cat string, got reply, want string) verdict {
	switch categoryMode(cat) {
	case modeItems:
		return scoreItems(got.Text, want)
	case modeJSON:
		return scoreJSON(got.Text, want)
	case modeNumber:
		return scoreNumber(got.Text, want)
	case modeTrunc:
		return scoreTruncado(got)
	}
	return verdict{Applicable: false, Why: "sem_criterio_automatico"}
}

// ---- C-5.4 truncado ----------------------------------------------------------------------------------

func scoreTruncado(r reply) verdict {
	return verdict{Applicable: true, Pass: r.Fim == "stop" && r.Final && !r.Truncated}
}

// ---- C-5.2 list of items -----------------------------------------------------------------------------

var errBulletRe = regexp.MustCompile(`^(?:[-*+•]\s+)`)

// listItems turns an answer into its items: the elements of a JSON array (after
// removing one code fence), each in canonical form, or else the non-empty lines
// without bullet markers and with single spaces. The numbering that is part of
// the item ("1.1 Escopo") stays: in `numeracao_errada` it is the point.
func listItems(text string) []string {
	body, _ := unfence(text)
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(body), &arr); err == nil {
		out := make([]string, 0, len(arr))
		for _, el := range arr {
			var s string
			if json.Unmarshal(el, &s) == nil {
				out = append(out, strings.Join(strings.Fields(s), " "))
				continue
			}
			c, err := canonicalJSON(el)
			if err != nil {
				c = string(el)
			}
			out = append(out, c)
		}
		return out
	}
	var out []string
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(strings.TrimRight(l, "\r"))
		l = errBulletRe.ReplaceAllString(l, "")
		l = strings.Join(strings.Fields(l), " ")
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// scoreItems compares the lists as multisets (the order is not part of the
// criterion): precision = right items / items given, coverage = right items /
// items expected; the case passes only with both at 100% (C-5.2).
func scoreItems(got, want string) verdict {
	g, w := listItems(got), listItems(want)
	if len(w) == 0 {
		return verdict{Why: "resposta_certa_sem_itens"} // not applicable
	}
	cnt := map[string]int{}
	for _, x := range w {
		cnt[x]++
	}
	hit := 0
	for _, x := range g {
		if cnt[x] > 0 {
			cnt[x]--
			hit++
		}
	}
	v := verdict{Applicable: true, Coverage: float64(hit) / float64(len(w))}
	if len(g) > 0 {
		v.Precision = float64(hit) / float64(len(g))
	}
	v.Pass = len(g) > 0 && hit == len(g) && hit == len(w)
	return v
}

// ---- C-5.3 JSON --------------------------------------------------------------------------------------

var errDupKey = errors.New("chave duplicada no objeto JSON")

// hasDupKeys reports an object that repeats a key at any depth: json.Valid and
// the decoder accept it, and the last value would silently win.
func hasDupKeys(raw []byte) bool {
	type frame struct {
		obj       bool
		keys      map[string]bool
		expectKey bool
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var st []frame
	for {
		t, err := dec.Token()
		if err != nil {
			return false
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '{':
				st = append(st, frame{obj: true, keys: map[string]bool{}, expectKey: true})
			case '[':
				st = append(st, frame{})
			default:
				if len(st) > 0 {
					st = st[:len(st)-1]
				}
				if n := len(st); n > 0 && st[n-1].obj {
					st[n-1].expectKey = true
				}
			}
			continue
		}
		if n := len(st); n > 0 && st[n-1].obj {
			if st[n-1].expectKey {
				k, _ := t.(string)
				if st[n-1].keys[k] {
					return true
				}
				st[n-1].keys[k] = true
				st[n-1].expectKey = false
			} else {
				st[n-1].expectKey = true
			}
		}
	}
}

// canonicalJSON writes a JSON value with sorted keys, no whitespace outside
// strings and numbers in their canonical rational form ("1.0" = "1" = "1e0").
func canonicalJSON(raw []byte) (string, error) {
	if hasDupKeys(raw) {
		return "", errDupKey
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	if dec.More() {
		return "", errors.New("dados depois do valor JSON")
	}
	var b strings.Builder
	writeCanon(&b, v)
	return b.String(), nil
}

func writeCanon(b *strings.Builder, v any) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			writeCanon(b, x[k])
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeCanon(b, e)
		}
		b.WriteByte(']')
	case json.Number:
		if r, ok := new(big.Rat).SetString(x.String()); ok {
			b.WriteString(r.RatString())
		} else {
			b.WriteString(x.String())
		}
	default: // string, bool, nil
		eb, _ := json.Marshal(x)
		b.Write(eb)
	}
}

// scoreJSON passes only if the text is valid JSON (as it is: no code fence is
// removed, C-5.3) AND its values equal those of the correct answer after
// normalising whitespace and key order. Comparing only the keys is not enough.
func scoreJSON(got, want string) verdict {
	wc, err := canonicalJSON([]byte(strings.TrimSpace(want)))
	if err != nil {
		return verdict{Why: "resposta_certa_nao_e_json"} // not applicable
	}
	g := strings.TrimSpace(got)
	if !json.Valid([]byte(g)) {
		return verdict{Applicable: true, Why: "json_invalido"}
	}
	gc, err := canonicalJSON([]byte(g))
	if errors.Is(err, errDupKey) {
		return verdict{Applicable: true, Why: "chave_duplicada"}
	}
	if err != nil {
		return verdict{Applicable: true, Why: "json_invalido"}
	}
	if gc != wc {
		return verdict{Applicable: true, Why: "valores_diferentes"}
	}
	return verdict{Applicable: true, Pass: true}
}

// ---- C-5.4 final number ------------------------------------------------------------------------------

var (
	errResultRe = regexp.MustCompile(`(?i)RESULTADO\s*[:=]\s*(-?\d[\d.,]*)`)
	errNumRe    = regexp.MustCompile(`-?\d[\d.,]*`)
)

// numberTokens lists the numbers of a text, in order. A hyphen glued to a word or
// digit ("2024-05", "item-3") is not a minus sign.
func numberTokens(text string) []string {
	var out []string
	for _, ix := range errNumRe.FindAllStringIndex(text, -1) {
		tok := text[ix[0]:ix[1]]
		if strings.HasPrefix(tok, "-") && ix[0] > 0 {
			if p := text[ix[0]-1]; p == '_' || (p >= '0' && p <= '9') || (p >= 'a' && p <= 'z') || (p >= 'A' && p <= 'Z') || p >= 0x80 {
				tok = tok[1:]
			}
		}
		out = append(out, tok)
	}
	return out
}

// finalNumber is the number the MODEL's answer ends with: the last
// "RESULTADO: <n>" if there is one, else the last number of the text.
func finalNumber(text string) (*big.Rat, bool) {
	tok := ""
	if m := errResultRe.FindAllStringSubmatch(text, -1); len(m) > 0 {
		tok = m[len(m)-1][1]
	} else if t := numberTokens(text); len(t) > 0 {
		tok = t[len(t)-1]
	}
	return parseNumber(tok)
}

// ambiguousNumber says whether a token can be read two ways: a lone separator
// followed by exactly three digits after an integer part of 1 to 3 digits other
// than 0 ("1,500" is 1500 in pt-BR and 1.5 in English).
func ambiguousNumber(tok string) bool {
	tok = strings.TrimRight(strings.TrimPrefix(tok, "-"), ".,")
	if strings.Count(tok, ".")+strings.Count(tok, ",") != 1 {
		return false
	}
	i := strings.IndexAny(tok, ".,")
	return i >= 1 && i <= 3 && len(tok)-i-1 == 3 && tok[:i] != "0"
}

// wantNumber is the number of the CORRECT answer, which has to be unambiguous:
// "RESULTADO: <n>" or a single number in the whole text, and not ambiguous. Prose
// with two numbers ("12 (ver seção 3)") or "1,500" cannot be scored: ok=false.
func wantNumber(text string) (*big.Rat, bool) {
	tok := ""
	if m := errResultRe.FindAllStringSubmatch(text, -1); len(m) > 0 {
		tok = m[len(m)-1][1]
	} else if t := numberTokens(text); len(t) == 1 {
		tok = t[0]
	}
	if tok == "" || ambiguousNumber(tok) {
		return nil, false
	}
	return parseNumber(tok)
}

// parseNumber reads a number written in pt-BR or in English. A separator that
// appears more than once, or once followed by exactly three digits, is a
// thousands separator ("1.234", "1,234", "1.234.567"); when both appear the last
// one is the decimal separator ("1.234,56", "1,234.56"); otherwise a lone one is
// the decimal separator ("3,5", "3.5").
func parseNumber(tok string) (*big.Rat, bool) {
	tok = strings.TrimRight(strings.TrimSpace(tok), ".,")
	if tok == "" {
		return nil, false
	}
	neg := strings.HasPrefix(tok, "-")
	tok = strings.TrimPrefix(tok, "-")
	lastDot, lastCom := strings.LastIndex(tok, "."), strings.LastIndex(tok, ",")
	dec := -1
	switch {
	case lastDot >= 0 && lastCom >= 0:
		dec = lastDot
		if lastCom > lastDot {
			dec = lastCom
		}
	case lastDot >= 0 || lastCom >= 0:
		i, sep := lastDot, "."
		if lastCom >= 0 {
			i, sep = lastCom, ","
		}
		if strings.Count(tok, sep) == 1 && (len(tok)-i-1 != 3 || tok[:i] == "0") {
			dec = i
		}
	}
	var intPart, frac string
	if dec >= 0 {
		intPart, frac = tok[:dec], tok[dec+1:]
	} else {
		intPart = tok
	}
	intPart = strings.NewReplacer(".", "", ",", "").Replace(intPart)
	if intPart == "" {
		intPart = "0"
	}
	if strings.ContainsAny(frac, ".,") {
		return nil, false
	}
	s := intPart
	if frac != "" {
		s += "." + frac
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, false
	}
	if neg {
		r.Neg(r)
	}
	return r, true
}

func scoreNumber(got, want string) verdict {
	// m-15: the Unicode minus sign (U+2212, common in pasted text) is a minus sign
	got, want = strings.ReplaceAll(got, "−", "-"), strings.ReplaceAll(want, "−", "-")
	w, ok := wantNumber(want)
	if !ok {
		return verdict{Why: "resposta_certa_sem_numero_unico"} // not applicable
	}
	g, ok := finalNumber(got)
	if !ok {
		return verdict{Applicable: true, Why: "sem_numero"}
	}
	return verdict{Applicable: true, Pass: g.Cmp(w) == 0}
}

// ---- C-5.15 two of three -----------------------------------------------------------------------------

// casePasses: a case is right with 2 of the 3 rounds right.
func casePasses(rounds []bool) bool {
	n := 0
	for _, r := range rounds {
		if r {
			n++
		}
	}
	return n >= 2
}

// roundsDiffer says whether the case changed its result between rounds.
func roundsDiffer(rounds []bool) bool {
	for _, r := range rounds {
		if r != rounds[0] {
			return true
		}
	}
	return false
}

// ---- C-5.9 Wilson ------------------------------------------------------------------------------------

const zWilson95 = 1.959963984540054

// wilson returns the 95% Wilson score interval of k successes in n trials.
func wilson(k, n int) (lo, hi float64) {
	if n <= 0 {
		return 0, 0
	}
	z2 := zWilson95 * zWilson95
	nf, p := float64(n), float64(k)/float64(n)
	den := 1 + z2/nf
	centre := (p + z2/(2*nf)) / den
	half := zWilson95 * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / den
	lo, hi = centre-half, centre+half
	if lo < 0 || k == 0 {
		lo = 0
	}
	if hi > 1 || k == n {
		hi = 1
	}
	return lo, hi
}

// wantApplicable says, before any call is made, whether the correct answer of a
// case allows its category to be scored. A case that cannot be scored is not
// replayed and is not counted as a model error.
func wantApplicable(cat, want string) (bool, string) {
	switch categoryMode(cat) {
	case modeItems:
		if len(listItems(want)) == 0 {
			return false, "resposta_certa_sem_itens"
		}
	case modeJSON:
		if _, err := canonicalJSON([]byte(strings.TrimSpace(want))); err != nil {
			return false, "resposta_certa_nao_e_json"
		}
	case modeNumber:
		if _, ok := wantNumber(want); !ok {
			return false, "resposta_certa_sem_numero_unico"
		}
	}
	return true, ""
}
