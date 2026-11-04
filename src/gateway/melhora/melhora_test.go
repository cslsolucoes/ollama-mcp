package melhora

// Fase 12, etapa 12.8: the improvement without training (contract chapter 8).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fileBody is the request of a file tool: the system text with the fixed rules and a
// prompt with the markers and a random id around the document. It has characters that a
// careless re-encoder would escape (<, >, &) and non-ASCII text.
func fileBody(nonce string) []byte {
	prompt := "Extraia os títulos.\n\n=====ARQUIVO_INICIO " + nonce + "=====\n1. Introdução <b>&</b>\n2. Método\n=====ARQUIVO_FIM " + nonce + "=====\n"
	pj, _ := json.Marshal(prompt)
	return []byte(`{"model":"qwen3-ia","system":"Você processa arquivos.","prompt":` + string(pj) + `,"options":{"seed":1},"think":false,"stream":true}`)
}

func rawOf(t *testing.T, body []byte, key string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, body)
	}
	return m[key]
}

func strOf(t *testing.T, body []byte, key string) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(rawOf(t, body, key), &s); err != nil {
		t.Fatalf("%s is not a string: %v", key, err)
	}
	return s
}

func allTools() Config { return Config{Tools: Tools} }

// ---- identity (C-8.8: nothing added means the very same body) ------------------------------------

func TestIdentityWithEmptyInstructionsAndNoIndex(t *testing.T) {
	v0, err := ParseInstrucoes([]byte(`{"tools":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := allTools()
	cfg.Instr = v0
	tr := NewTransformer(cfg)
	for _, path := range []string{"/api/generate", "/api/chat", "/v1/chat/completions", "/v1/completions"} {
		for _, tool := range Tools {
			in := fileBody("0123456789abcdef")
			out, oc := tr.Transform(context.Background(), tool, path, in, Exclusion{})
			if !bytes.Equal(out, in) || &out[0] != &in[0] || !zeroOutcome(oc) {
				t.Errorf("%s %s: the body changed or an outcome was reported: %+v", path, tool, oc)
			}
		}
	}
}

func TestIdentityWithAnEmptyIndex(t *testing.T) {
	var called atomic.Int32
	cfg := allTools()
	cfg.Embed = func(context.Context, string) ([]float32, error) { called.Add(1); return []float32{1, 0}, nil }
	tr := NewTransformer(cfg)
	tr.SetIndex(NewIndex("x", "h", "m", 3, 0.5, nil))
	in := fileBody("0123456789abcdef")
	out, _ := tr.Transform(context.Background(), "extract", "/api/generate", in, Exclusion{})
	if !bytes.Equal(out, in) {
		t.Fatalf("the body changed with an empty index")
	}
	if called.Load() != 0 {
		t.Errorf("a query was embedded although the index has no example of the tool")
	}
}

func TestOffByDefault(t *testing.T) { // C-8.8
	in, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"SEMPRE USE A NUMERAÇÃO DO CORPO"}}`))
	tr := NewTransformer(Config{Instr: in}) // no tool switched on
	body := fileBody("0123456789abcdef")
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if !bytes.Equal(out, body) || !zeroOutcome(oc) {
		t.Fatalf("something was added with the improvement off")
	}
	if s := tr.Stats(); s.Ligado || s.Pedidos != 0 {
		t.Errorf("stats of an off improvement: %+v", s)
	}
	var nilT *Transformer
	if out, _ := nilT.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{}); !bytes.Equal(out, body) {
		t.Errorf("a nil transformer changed the body")
	}
}

func TestSwitchedOnPerTool(t *testing.T) { // C-8.8
	in, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"X-EXTRACT","translate":"X-TRANSLATE"}}`))
	tr := NewTransformer(Config{Tools: []string{"extract", "bakeoff-erros", "nonsense"}, Instr: in})
	body := fileBody("0123456789abcdef")
	if out, _ := tr.Transform(context.Background(), "translate", "/api/generate", body, Exclusion{}); !bytes.Equal(out, body) {
		t.Errorf("translate was changed but it is not switched on")
	}
	out, _ := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if !strings.Contains(strOf(t, out, "system"), "X-EXTRACT") || strings.Contains(strOf(t, out, "system"), "X-TRANSLATE") {
		t.Errorf("extract got %q", strOf(t, out, "system"))
	}
	// the bake-off's own tool, and anything that is not an eligible tool, can never be switched on
	for _, tool := range []string{"bakeoff-erros", "nonsense", "melhora-embed", ""} {
		if tr.Enabled(tool) {
			t.Errorf("%q is enabled", tool)
		}
	}
	if got := tr.Stats().Tools; len(got) != 1 || got[0] != "extract" {
		t.Errorf("tools in the stats: %v", got)
	}
}

// ---- where the instruction goes (and the markers stay) --------------------------------------------

func TestInstructionGoesInTheSystemTextAndTheMarkersStayIntact(t *testing.T) {
	const nonce = "a1b2c3d4e5f60718"
	body := fileBody(nonce)
	out, changed, err := Apply("/api/generate", body, "USE A NUMERAÇÃO DO CORPO <sempre> & nunca a do sumário", nil, testNonce)
	if err != nil || !changed {
		t.Fatalf("Apply: %v %v", changed, err)
	}
	want := "Você processa arquivos.\n\nUSE A NUMERAÇÃO DO CORPO <sempre> & nunca a do sumário"
	if got := strOf(t, out, "system"); got != want {
		t.Errorf("system = %q, want %q", got, want)
	}
	// the user text is byte-identical, markers and nonce included (not even a re-escape)
	if !bytes.Equal(rawOf(t, out, "prompt"), rawOf(t, body, "prompt")) {
		t.Errorf("the prompt value changed:\n%s\n%s", rawOf(t, out, "prompt"), rawOf(t, body, "prompt"))
	}
	if !strings.Contains(string(out), "ARQUIVO_INICIO "+nonce+"=====") || !strings.Contains(string(out), "ARQUIVO_FIM "+nonce+"=====") {
		t.Errorf("the markers are not intact")
	}
	for _, k := range []string{"model", "options", "think", "stream"} {
		if !bytes.Equal(rawOf(t, out, k), rawOf(t, body, k)) {
			t.Errorf("%s changed", k)
		}
	}
	// the new text is written without HTML escaping, like the policy does
	if !strings.Contains(string(out), `<sempre> & nunca`) {
		t.Errorf("the added text was HTML-escaped: %s", out)
	}
}

func TestGenerateWithoutSystemGetsOne(t *testing.T) {
	body := []byte(`{"model":"qwen3-ia","prompt":"oi"}`)
	out, changed, err := Apply("/api/generate", body, "INSTR", nil, testNonce)
	if err != nil || !changed || strOf(t, out, "system") != "INSTR" || strOf(t, out, "prompt") != "oi" {
		t.Fatalf("%s %v %v", out, changed, err)
	}
	// "system": null counts as absent
	out, _, err = Apply("/api/generate", []byte(`{"prompt":"oi","system":null}`), "INSTR", nil, testNonce)
	if err != nil || strOf(t, out, "system") != "INSTR" {
		t.Fatalf("null system: %s %v", out, err)
	}
}

func TestChatAppendsToTheFirstSystemMessageOrAddsOne(t *testing.T) {
	for _, path := range []string{"/api/chat", "/v1/chat/completions"} {
		withSys := []byte(`{"model":"m","messages":[{"role":"system","content":"regras"},{"role":"user","content":"pergunta <1>"}],"stream":false}`)
		out, changed, err := Apply(path, withSys, "INSTR", nil, testNonce)
		if err != nil || !changed {
			t.Fatalf("%s: %v %v", path, changed, err)
		}
		var m struct {
			Messages []struct{ Role, Content string }
		}
		json.Unmarshal(out, &m)
		if len(m.Messages) != 2 || m.Messages[0].Content != "regras\n\nINSTR" || m.Messages[1].Content != "pergunta <1>" {
			t.Errorf("%s with system: %+v", path, m.Messages)
		}
		noSys := []byte(`{"model":"m","messages":[{"role":"user","content":"pergunta"}]}`)
		out, changed, err = Apply(path, noSys, "INSTR", nil, testNonce)
		if err != nil || !changed {
			t.Fatal(err)
		}
		json.Unmarshal(out, &m)
		if len(m.Messages) != 2 || m.Messages[0].Role != "system" || m.Messages[0].Content != "INSTR" || m.Messages[1].Content != "pergunta" {
			t.Errorf("%s without system: %+v", path, m.Messages)
		}
		// a system message that is not the first is not the one extended
		late := []byte(`{"messages":[{"role":"user","content":"a"},{"role":"system","content":"tarde"}]}`)
		out, _, _ = Apply(path, late, "INSTR", nil, testNonce)
		json.Unmarshal(out, &m)
		if len(m.Messages) != 3 || m.Messages[0].Role != "system" || m.Messages[2].Content != "tarde" {
			t.Errorf("%s late system: %+v", path, m.Messages)
		}
	}
}

func TestShapesThatCannotBeExtendedAreLeftAlone(t *testing.T) {
	for name, tc := range map[string]struct{ path, body string }{
		"system not a string":     {"/api/generate", `{"system":{"x":1},"prompt":"a"}`},
		"messages not an array":   {"/api/chat", `{"messages":"oi"}`},
		"messages missing":        {"/api/chat", `{"model":"m"}`},
		"content as parts":        {"/v1/chat/completions", `{"messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`},
		"not an object":           {"/api/generate", `[1,2]`},
		"invalid json":            {"/api/generate", `{"a":`},
		"messages with a literal": {"/api/chat", `{"messages":5}`},
	} {
		in := []byte(tc.body)
		out, changed, err := Apply(tc.path, in, "INSTR", nil, testNonce)
		if changed || !bytes.Equal(out, in) || !errors.Is(err, ErrUnsupportedBody) {
			t.Errorf("%s: changed=%v err=%v out=%s", name, changed, err, out)
		}
	}
	// /v1/completions has no system text: untouched, not even an error
	in := []byte(`{"model":"m","prompt":"oi"}`)
	if out, changed, err := Apply("/v1/completions", in, "INSTR", nil, testNonce); changed || err != nil || !bytes.Equal(out, in) {
		t.Errorf("/v1/completions: %v %v", changed, err)
	}
}

func TestKeysAreMatchedLikeTheOllamaDoes(t *testing.T) {
	out, _, err := Apply("/api/generate", []byte(`{"Prompt":"oi","SYSTEM":"regra"}`), "INSTR", nil, testNonce)
	if err != nil || strOf(t, out, "SYSTEM") != "regra\n\nINSTR" {
		t.Fatalf("%s %v", out, err)
	}
	if bytes.Contains(out, []byte(`"system"`)) {
		t.Errorf("a second system key was added: %s", out)
	}
}

func TestApplyIsDeterministic(t *testing.T) {
	body := fileBody("a1b2c3d4e5f60718")
	ex := []Example{{Pedido: "p", Errada: "e", Certa: "c"}}
	first, _, _ := Apply("/api/generate", body, "INSTR", ex, testNonce)
	for i := 0; i < 20; i++ {
		again, _, _ := Apply("/api/generate", body, "INSTR", ex, testNonce)
		if !bytes.Equal(first, again) {
			t.Fatal("two applications of the same input differ")
		}
	}
}

func TestExamplesBlockFormatAndLimits(t *testing.T) {
	if FormatExamples(nil, testNonce) != "" {
		t.Error("no examples must give an empty block")
	}
	b := FormatExamples([]Example{{Pedido: "PED1", Errada: "ERR1", Certa: "CER1"}, {Pedido: "PED2", Errada: "ERR2", Certa: "CER2"}}, testNonce)
	for _, s := range []string{"Exemplo 1", "Exemplo 2", "PED1", "ERR1", "CER1", "PED2", "não repita"} {
		if !strings.Contains(b, s) {
			t.Errorf("block lacks %q:\n%s", s, b)
		}
	}
	if strings.Index(b, "Exemplo 1") > strings.Index(b, "Exemplo 2") {
		t.Error("examples out of order")
	}
	long := strings.Repeat("ç", 5000)
	b = FormatExamples([]Example{{Pedido: long, Errada: long, Certa: long}, {Pedido: long, Errada: long, Certa: long}, {Pedido: long, Errada: long, Certa: long}}, testNonce)
	if n := len([]rune(b)); n > MaxBlockRunes+2*MaxExampleFieldRunes {
		t.Errorf("block of %d runes", n)
	}
	if strings.Count(b, "Exemplo ") < 1 {
		t.Error("the first example must always be kept")
	}
	if strings.Count(strings.Split(b, "Exemplo 1")[1], "ç") > 3*MaxExampleFieldRunes {
		t.Error("a field was not cut")
	}
}

// ---- instruction versions (C-8.7) ------------------------------------------------------------------

func TestInstructionVersionIDAndHashComeFromTheContent(t *testing.T) {
	a, err := ParseInstrucoes([]byte(`{"tools":{"extract":"A","translate":"B"}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseInstrucoes([]byte(`{ "tools": {"translate":"B", "extract":"A"} }`)) // other order and spacing
	if a.ID != b.ID || a.Hash != b.Hash || len(a.Hash) != 64 || !strings.HasPrefix(a.ID, "i") {
		t.Errorf("same content, different version: %s/%s vs %s/%s", a.ID, a.Hash, b.ID, b.Hash)
	}
	c, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"A","translate":"B2"}}`))
	d, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"A2","translate":"B"}}`))
	if c.Hash == a.Hash || d.Hash == a.Hash || c.Hash == d.Hash || c.ID == a.ID {
		t.Error("different content, same version")
	}
	// the same text under another tool is another version
	e, _ := ParseInstrucoes([]byte(`{"tools":{"chat":"A","translate":"B"}}`))
	if e.Hash == a.Hash {
		t.Error("the tool is not part of the hash")
	}
	// v0: empty, still has an ID and a hash, and a blank entry counts as none
	v0, _ := ParseInstrucoes([]byte(`{"tools":{}}`))
	v0b, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"   \n"}}`))
	v0c, _ := ParseInstrucoes([]byte(`{}`))
	if !v0.Empty() || v0.ID == "" || v0.Hash == "" || v0.Hash != v0b.Hash || v0.Hash != v0c.Hash || v0.Hash == a.Hash {
		t.Errorf("v0: %+v %+v %+v", v0, v0b, v0c)
	}
	if a.Empty() || a.For("extract") != "A" || a.For("chat") != "" {
		t.Error("For/Empty")
	}
	var nilI *Instrucoes
	if nilI.For("extract") != "" || !nilI.Empty() {
		t.Error("nil version must be empty")
	}
}

func TestInstructionFileIsStrict(t *testing.T) {
	for name, in := range map[string]string{
		"unknown tool":  `{"tools":{"bakeoff-erros":"x"}}`,
		"unknown key":   `{"tools":{},"versao":"v1"}`,
		"not an object": `[]`,
		"trailing":      `{"tools":{}} {}`,
		"too long":      `{"tools":{"extract":"` + strings.Repeat("a", MaxInstrucaoBytes+1) + `"}}`,
		"not json":      `oi`,
		"bad utf8":      "{\"tools\":{\"extract\":\"\xff\"}}",
	} {
		if _, err := ParseInstrucoes([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// ---- the embedded text -------------------------------------------------------------------------------

func TestRequestTextIgnoresTheNonceAndCuts(t *testing.T) {
	a, ok1 := RequestText("/api/generate", fileBody("0123456789abcdef"))
	b, ok2 := RequestText("/api/generate", fileBody("fedcba9876543210"))
	if !ok1 || !ok2 || a != b {
		t.Fatalf("the same task with another nonce gives another text:\n%q\n%q", a, b)
	}
	if strings.Contains(a, "0123456789abcdef") || !strings.Contains(a, "ARQUIVO_INICIO <id>=====") || !strings.Contains(a, "Você processa arquivos.") {
		t.Errorf("text = %q", a)
	}
	big, _ := json.Marshal(strings.Repeat("é", MaxEmbedRunes*3))
	txt, _ := RequestText("/api/generate", []byte(`{"prompt":`+string(big)+`}`))
	if n := len([]rune(txt)); n != MaxEmbedRunes {
		t.Errorf("cut at %d runes, want %d", n, MaxEmbedRunes)
	}
	if _, ok := RequestText("/api/generate", []byte(`{"prompt":"  "}`)); ok {
		t.Error("an empty request has no text")
	}
	if _, ok := RequestText("/api/generate", []byte(`oops`)); ok {
		t.Error("garbage has no text")
	}
	chat, _ := RequestText("/api/chat", []byte(`{"messages":[{"role":"system","content":"S"},{"role":"user","content":[{"type":"text","text":"U1"},{"type":"image_url"}]}]}`))
	if !strings.Contains(chat, "[system]\nS") || !strings.Contains(chat, "[user]\nU1") {
		t.Errorf("chat text = %q", chat)
	}
}

// ---- search (C-8.3, C-8.4, C-8.10) ----------------------------------------------------------------------

func vec(x, y float32) []float32 { return []float32{x, y, 0} }

func ix3(items ...Item) *Index { return NewIndex("idx", "hash", "m", 2, 0.5, items) }

func item(id, tool string, v []float32) Item {
	return Item{OcorrenciaID: id, Tool: tool, Vec: v, Example: Example{Pedido: "P-" + id, Errada: "E-" + id, Certa: "C-" + id}}
}

func ids(ex []Example) string {
	var s []string
	for _, e := range ex {
		s = append(s, strings.TrimPrefix(e.Pedido, "P-"))
	}
	return strings.Join(s, ",")
}

func TestSearchFiltersByToolLimiarAndK(t *testing.T) {
	ix := ix3(
		item("a", "extract", vec(1, 0)),          // cos 1.0
		item("b", "extract", vec(1, 0.5)),        // cos ~0.894
		item("c", "extract", vec(1, 1)),          // cos ~0.707
		item("d", "extract", vec(0, 1)),          // cos 0
		item("e", "translate", vec(1, 0)),        // other tool
		item("f", "extract", []float32{1}),       // other size: dropped when the index is built
		item("g", "extract", []float32{0, 0, 0}), // zero vector: dropped
	)
	if len(ix.Items) != 5 {
		t.Fatalf("%d items kept, want 5", len(ix.Items))
	}
	q := vec(1, 0)
	if got := ids(ix.Search("extract", q, 10, 0.5, Exclusion{})); got != "a,b,c" {
		t.Errorf("limiar 0.5: %s", got)
	}
	if got := ids(ix.Search("extract", q, 10, 0.8, Exclusion{})); got != "a,b" {
		t.Errorf("limiar 0.8: %s", got)
	}
	if got := ids(ix.Search("extract", q, 10, 0.95, Exclusion{})); got != "a" {
		t.Errorf("limiar 0.95: %s", got)
	}
	if got := ids(ix.Search("extract", q, 10, 1.0001, Exclusion{})); got != "" {
		t.Errorf("limiar above 1: %s", got)
	}
	if got := ids(ix.Search("extract", q, 2, 0.1, Exclusion{})); got != "a,b" {
		t.Errorf("k=2: %s", got)
	}
	if got := ids(ix.Search("extract", q, 1, 0.1, Exclusion{})); got != "a" {
		t.Errorf("k=1: %s", got)
	}
	if got := ids(ix.Search("extract", q, 0, 0.1, Exclusion{})); got != "" {
		t.Errorf("k=0: %s", got)
	}
	if got := ids(ix.Search("translate", q, 5, 0.1, Exclusion{})); got != "e" {
		t.Errorf("the tool filter: %s", got)
	}
	if got := ids(ix.Search("chat", q, 5, 0.1, Exclusion{})); got != "" {
		t.Errorf("a tool with no example: %s", got)
	}
	if got := ids(ix.Search("extract", []float32{1, 0}, 5, 0.1, Exclusion{})); got != "" {
		t.Errorf("a query of another size must find nothing: %s", got)
	}
	if got := ids(ix.Search("extract", []float32{0, 0, 0}, 5, 0.1, Exclusion{})); got != "" {
		t.Errorf("a zero query must find nothing: %s", got)
	}
}

func TestSearchOrderIsBySimilarityThenIDAndDeterministic(t *testing.T) {
	ix := ix3(item("z", "extract", vec(1, 0)), item("m", "extract", vec(1, 0)), item("a", "extract", vec(1, 0)), item("b", "extract", vec(1, 0.1)))
	for i := 0; i < 20; i++ {
		if got := ids(ix.Search("extract", vec(1, 0), 4, 0.1, Exclusion{})); got != "a,m,z,b" {
			t.Fatalf("order = %s", got)
		}
	}
}

func TestSearchExcludesTheCaseItsGroupAndItsDocument(t *testing.T) { // C-8.4
	mk := func(id, grp, doc string) Item {
		it := item(id, "extract", vec(1, 0))
		it.GrupoID, it.DocKey = grp, doc
		return it
	}
	ix := ix3(mk("self", "g1", "k1/d1"), mk("sameGroup", "g1", ""), mk("sameDoc", "g9", "k1/d1"), mk("otherKeyDoc", "g8", "k2/d1"), mk("clean", "g7", "k1/d7"), mk("noGroup", "", ""))
	q := vec(1, 0)
	all := func(e Exclusion) string { return ids(ix.Search("extract", q, 10, 0.1, e)) }
	if got := all(Exclusion{}); got != "clean,noGroup,otherKeyDoc,sameDoc,sameGroup,self" {
		t.Fatalf("no exclusion: %s", got)
	}
	if got := all(Exclusion{Ocorrencias: []string{"self"}}); strings.Contains(got, "self") || !strings.Contains(got, "sameGroup") {
		t.Errorf("occurrence: %s", got)
	}
	if got := all(Exclusion{Grupos: []string{"g1"}}); strings.Contains(got, "self") || strings.Contains(got, "sameGroup") || !strings.Contains(got, "sameDoc") {
		t.Errorf("group: %s", got)
	}
	if got := all(Exclusion{Docs: []string{DocKey("k1", "d1")}}); strings.Contains(got, "self") || strings.Contains(got, "sameDoc") || !strings.Contains(got, "otherKeyDoc") {
		t.Errorf("document (the hash only matches under the same key id): %s", got)
	}
	if got := all(Exclusion{Ocorrencias: []string{"self"}, Grupos: []string{"g1"}, Docs: []string{DocKey("k1", "d1")}}); got != "clean,noGroup,otherKeyDoc" {
		t.Errorf("all three: %s", got)
	}
	// an empty group or document in the exclusion must not exclude items that have none
	if got := all(Exclusion{Grupos: []string{""}, Docs: []string{"", DocKey("k", "")}}); !strings.Contains(got, "noGroup") {
		t.Errorf("empty values excluded the items without group: %s", got)
	}
	if DocKey("k", "") != "" {
		t.Error("a document with no hash has no key")
	}
}

func TestInvalidIndexServesNothing(t *testing.T) { // C-4.10
	ix := ix3(item("a", "extract", vec(1, 0)))
	if len(ix.Search("extract", vec(1, 0), 3, 0.1, Exclusion{})) != 1 || !ix.Valid() {
		t.Fatal("setup")
	}
	var embeds atomic.Int32
	cfg := allTools()
	cfg.Embed = func(context.Context, string) ([]float32, error) { embeds.Add(1); return vec(1, 0), nil }
	tr := NewTransformer(cfg)
	tr.SetIndex(ix)
	body := fileBody("0123456789abcdef")
	if _, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{}); oc.Exemplos != 1 {
		t.Fatalf("setup: %+v", oc)
	}
	tr.InvalidateIndex("another-id") // not this one
	if !ix.Valid() {
		t.Fatal("another id invalidated the index")
	}
	tr.InvalidateIndex("idx")
	if ix.Valid() || len(ix.Search("extract", vec(1, 0), 3, 0.1, Exclusion{})) != 0 {
		t.Error("an invalid index still searches")
	}
	embeds.Store(0)
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if !bytes.Equal(out, body) || oc.Exemplos != 0 || embeds.Load() != 0 {
		t.Errorf("an invalid index served: %+v embeds=%d", oc, embeds.Load())
	}
	if s := tr.Stats(); s.IndiceValido || s.IndiceCaiu != 1 || s.IndiceID != "idx" {
		t.Errorf("stats: %+v", s)
	}
}

// ---- the Transformer: examples, failures, slowness ---------------------------------------------------------

func exampleTransformer(embed EmbedFunc, timeout time.Duration) *Transformer {
	cfg := allTools()
	cfg.Embed, cfg.EmbedTimeout = embed, timeout
	tr := NewTransformer(cfg)
	tr.SetIndex(ix3(item("a", "extract", vec(1, 0)), item("b", "extract", vec(1, 0.3))))
	return tr
}

func TestTransformInjectsTheExamplesOfTheIndex(t *testing.T) {
	var text atomic.Value
	tr := exampleTransformer(func(_ context.Context, s string) ([]float32, error) { text.Store(s); return vec(1, 0), nil }, time.Second)
	body := fileBody("0123456789abcdef")
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if oc.Exemplos != 2 || oc.Instrucao || oc.EmbedFalhou {
		t.Fatalf("outcome: %+v", oc)
	}
	sys := strOf(t, out, "system")
	if !strings.HasPrefix(sys, "Você processa arquivos.\n\nOs exemplos abaixo, entre as marcações EXEMPLOS_INICIO ") || !strings.Contains(sys, "C-a") || !strings.Contains(sys, "C-b") {
		t.Errorf("system = %q", sys)
	}
	if strings.Index(sys, "C-a") > strings.Index(sys, "C-b") {
		t.Error("the best example must come first")
	}
	if !bytes.Equal(rawOf(t, out, "prompt"), rawOf(t, body, "prompt")) {
		t.Error("the prompt changed")
	}
	// the query is the text of the REQUEST as received (nonce-free), not of the improved one
	want, _ := RequestText("/api/generate", body)
	if text.Load() != want || strings.Contains(text.Load().(string), "EXEMPLOS_INICIO") {
		t.Errorf("embedded text = %q", text.Load())
	}
	s := tr.Stats()
	if s.Pedidos != 1 || s.Alterados != 1 || s.Exemplos != 2 || s.EmbedFalhas != 0 || !s.Ligado {
		t.Errorf("stats: %+v", s)
	}
}

func TestTransformUsesTheFrozenKAndLimiar(t *testing.T) { // C-8.10
	cfg := allTools()
	cfg.Embed = func(context.Context, string) ([]float32, error) { return vec(1, 0), nil }
	tr := NewTransformer(cfg)
	tr.SetIndex(NewIndex("i", "h", "m", 1, 0.5, []Item{item("a", "extract", vec(1, 0)), item("b", "extract", vec(1, 0.1))}))
	_, oc := tr.Transform(context.Background(), "extract", "/api/generate", fileBody("0123456789abcdef"), Exclusion{})
	if oc.Exemplos != 1 {
		t.Errorf("k=1 frozen with the index, got %d examples", oc.Exemplos)
	}
	tr.SetIndex(NewIndex("i2", "h", "m", 5, 0.999, []Item{item("a", "extract", vec(1, 0)), item("b", "extract", vec(1, 0.5))}))
	_, oc = tr.Transform(context.Background(), "extract", "/api/generate", fileBody("0123456789abcdef"), Exclusion{})
	if oc.Exemplos != 1 {
		t.Errorf("limiar 0.999 frozen with the index, got %d examples", oc.Exemplos)
	}
}

func TestEmbedFailureSendsTheRequestWithoutExamples(t *testing.T) {
	tr := exampleTransformer(func(context.Context, string) ([]float32, error) { return nil, errors.New("o Ollama caiu") }, time.Second)
	body := fileBody("0123456789abcdef")
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if !bytes.Equal(out, body) || !oc.EmbedFalhou || oc.Exemplos != 0 {
		t.Fatalf("outcome %+v, body changed=%v", oc, !bytes.Equal(out, body))
	}
	if s := tr.Stats(); s.EmbedFalhas != 1 || s.EmbedLentos != 0 || s.Alterados != 0 {
		t.Errorf("stats: %+v", s)
	}
}

func TestEmbedFailureStillAppliesTheInstruction(t *testing.T) {
	in, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"INSTR"}}`))
	cfg := allTools()
	cfg.Instr = in
	cfg.Embed = func(context.Context, string) ([]float32, error) { return nil, errors.New("x") }
	tr := NewTransformer(cfg)
	tr.SetIndex(ix3(item("a", "extract", vec(1, 0))))
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", fileBody("0123456789abcdef"), Exclusion{})
	if !oc.Instrucao || !oc.EmbedFalhou || oc.Exemplos != 0 || !strings.HasSuffix(strOf(t, out, "system"), "INSTR") {
		t.Errorf("%+v %q", oc, strOf(t, out, "system"))
	}
}

func TestSlowEmbedDoesNotDelayTheRequestBeyondTheTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	tr := exampleTransformer(func(ctx context.Context, _ string) ([]float32, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return vec(1, 0), nil
		}
	}, 150*time.Millisecond)
	body := fileBody("0123456789abcdef")
	t0 := time.Now()
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	d := time.Since(t0)
	if d < 100*time.Millisecond || d > 2*time.Second {
		t.Errorf("took %s with a 150 ms limit", d)
	}
	if !bytes.Equal(out, body) || !oc.EmbedFalhou {
		t.Errorf("a slow embedding must send the request without examples: %+v", oc)
	}
	if s := tr.Stats(); s.EmbedLentos != 1 || s.EmbedFalhas != 1 {
		t.Errorf("stats: %+v", s)
	}
}

func TestEmbedIgnoringItsContextStillBoundedByTheCallerContext(t *testing.T) {
	// the client leaving cancels the search at once
	tr := exampleTransformer(func(ctx context.Context, _ string) ([]float32, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	t0 := time.Now()
	_, oc := tr.Transform(ctx, "extract", "/api/generate", fileBody("0123456789abcdef"), Exclusion{})
	if time.Since(t0) > 2*time.Second || !oc.EmbedFalhou {
		t.Errorf("not bounded by the caller: %s %+v", time.Since(t0), oc)
	}
}

func TestQueryOfAnotherSizeFindsNothingButIsNotAFailure(t *testing.T) {
	tr := exampleTransformer(func(context.Context, string) ([]float32, error) { return []float32{1, 2}, nil }, time.Second)
	body := fileBody("0123456789abcdef")
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if !bytes.Equal(out, body) || oc.Exemplos != 0 {
		t.Errorf("%+v", oc)
	}
}

func TestSkippedShapesAreCountedAndTheRequestGoesAsItCame(t *testing.T) {
	in, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"INSTR"}}`))
	tr := NewTransformer(Config{Tools: []string{"extract"}, Instr: in})
	body := []byte(`{"system":{"x":1},"prompt":"a"}`)
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if !bytes.Equal(out, body) || !oc.Pulou {
		t.Errorf("%+v", oc)
	}
	if s := tr.Stats(); s.CorpoPulado != 1 || s.Alterados != 0 {
		t.Errorf("%+v", s)
	}
}

func TestStatsCarryNoText(t *testing.T) {
	in, _ := ParseInstrucoes([]byte(`{"tools":{"extract":"TEXTO-SECRETO-DA-INSTRUCAO"}}`))
	cfg := allTools()
	cfg.Instr = in
	cfg.Embed = func(context.Context, string) ([]float32, error) { return vec(1, 0), nil }
	tr := NewTransformer(cfg)
	tr.SetIndex(ix3(item("a", "extract", vec(1, 0))))
	tr.Transform(context.Background(), "extract", "/api/generate", fileBody("0123456789abcdef"), Exclusion{})
	b, _ := json.Marshal(tr.Stats())
	for _, bad := range []string{"TEXTO-SECRETO", "P-a", "C-a", "E-a", "Introdução", "ARQUIVO"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("the stats carry %q: %s", bad, b)
		}
	}
	s := tr.Stats()
	if s.InstrucoesID != in.ID || s.InstrucoesH != in.Hash || s.IndiceID != "idx" || s.IndiceHash != "hash" || !s.IndiceValido {
		t.Errorf("identity missing: %+v", s)
	}
}

func TestCosine(t *testing.T) {
	if c := Cosine([]float32{1, 0}, []float32{1, 0}); c < 0.999999 {
		t.Error(c)
	}
	if c := Cosine([]float32{1, 0}, []float32{0, 1}); c != 0 {
		t.Error(c)
	}
	if Cosine([]float32{1}, []float32{1, 2}) != 0 || Cosine(nil, nil) != 0 || Cosine([]float32{0, 0}, []float32{1, 1}) != 0 {
		t.Error("degenerate cosines must be 0")
	}
	if c := Cosine([]float32{3, 4}, []float32{6, 8}); fmt.Sprintf("%.4f", c) != "1.0000" {
		t.Error("cosine is not scale-free", c)
	}
}

func TestExampleOfCleansTheReasoning(t *testing.T) {
	ex := ExampleOf("/api/generate", []byte(`{"prompt":"pedido?"}`), []byte("[raciocínio] penso...\n[resposta] errei"), "certo")
	if ex.Errada != "errei" || ex.Certa != "certo" || !strings.Contains(ex.Pedido, "pedido?") {
		t.Errorf("%+v", ex)
	}
	if ExampleOf("/api/generate", []byte(`{"prompt":"x"}`), []byte("[raciocínio] só pensei"), "c").Errada != "" {
		t.Error("a reasoning-only answer is not an example answer")
	}
	if ExampleOf("/api/generate", []byte(`{"prompt":"x"}`), []byte("  resposta simples \n"), "c").Errada != "resposta simples" {
		t.Error("plain answer not kept")
	}
}

func TestAPanickingEmbedNeverTakesTheRequestDown(t *testing.T) {
	cfg := allTools()
	cfg.Embed = func(context.Context, string) ([]float32, error) { panic("boom") }
	tr := NewTransformer(cfg)
	tr.SetIndex(ix3(item("a", "extract", vec(1, 0))))
	body := fileBody("0123456789abcdef")
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", body, Exclusion{})
	if !bytes.Equal(out, body) || !oc.Pulou {
		t.Errorf("%+v", oc)
	}
	if tr.Stats().CorpoPulado != 1 {
		t.Error("the skip was not counted")
	}
}

func TestTransformPassesTheExclusionToTheSearch(t *testing.T) { // C-8.4
	cfg := allTools()
	cfg.Embed = func(context.Context, string) ([]float32, error) { return vec(1, 0), nil }
	tr := NewTransformer(cfg)
	a, b := item("a", "extract", vec(1, 0)), item("b", "extract", vec(1, 0))
	b.GrupoID = "g-b"
	tr.SetIndex(NewIndex("i", "h", "m", 5, 0.1, []Item{a, b}))
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", fileBody("0123456789abcdef"), Exclusion{Ocorrencias: []string{"a"}, Grupos: []string{"g-b"}})
	if oc.Exemplos != 0 || strings.Contains(strOf(t, out, "system"), "C-a") || strings.Contains(strOf(t, out, "system"), "C-b") {
		t.Errorf("the exclusion did not reach the search: %+v", oc)
	}
	_, oc = tr.Transform(context.Background(), "extract", "/api/generate", fileBody("0123456789abcdef"), Exclusion{})
	if oc.Exemplos != 2 {
		t.Errorf("without exclusion: %+v", oc)
	}
}

const testNonce = "00112233445566ff"

func zeroOutcome(oc Outcome) bool {
	return !oc.Instrucao && oc.Exemplos == 0 && !oc.EmbedFalhou && !oc.Pulou && oc.InstrucoesID == "" && oc.IndiceID == "" && len(oc.ExemploIDs) == 0
}
