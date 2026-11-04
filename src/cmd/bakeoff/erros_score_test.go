package main

import (
	"math"
	"testing"
)

func TestCategoryMode(t *testing.T) { // C-5.2 to C-5.7
	want := map[string]string{
		"numeracao_errada": modeItems, "omitiu": modeItems, "formato_invalido": modeJSON,
		"contagem_errada": modeNumber, "calculo_errado": modeNumber, "truncado": modeTrunc,
		"resumo_infiel": modeBlind, "inventou": modeBlind, "traducao_errada": modeBlind, "ignorou_instrucao": modeBlind,
		"outro": modeOutside, "idioma_errado": modeOutside, "categoria_desconhecida": modeOutside,
	}
	for c, m := range want {
		if got := categoryMode(c); got != m {
			t.Errorf("%s: mode %s, want %s", c, got, m)
		}
	}
}

func TestScoreItems(t *testing.T) { // C-5.2: precision AND coverage at 100%
	want := "1 Introdução\n1.1 Escopo\n2 Modelo"
	cases := []struct {
		name, got string
		pass      bool
		prec, cov float64
	}{
		{"exact", "1 Introdução\n1.1 Escopo\n2 Modelo", true, 1, 1},
		{"other order, bullets, spaces", "- 2   Modelo\n* 1 Introdução\n- 1.1 Escopo\n", true, 1, 1},
		{"an extra item (precision < 1)", "1 Introdução\n1.1 Escopo\n2 Modelo\n3 Inventado", false, 0.75, 1},
		{"a missing item (coverage < 1)", "1 Introdução\n2 Modelo", false, 1, 2.0 / 3},
		{"a wrong number", "1 Introdução\n1.2 Escopo\n2 Modelo", false, 2.0 / 3, 2.0 / 3},
		{"empty answer", "", false, 0, 0},
		{"JSON array of strings", `["1 Introdução","1.1 Escopo","2 Modelo"]`, true, 1, 1},
		{"fenced JSON array", "```json\n[\"1 Introdução\",\"1.1 Escopo\",\"2 Modelo\"]\n```", true, 1, 1},
	}
	for _, c := range cases {
		v := scoreItems(c.got, want)
		if !v.Applicable || v.Pass != c.pass || math.Abs(v.Precision-c.prec) > 1e-9 || math.Abs(v.Coverage-c.cov) > 1e-9 {
			t.Errorf("%s: %+v, want pass=%v precision=%.3f coverage=%.3f", c.name, v, c.pass, c.prec, c.cov)
		}
	}
	// a repeated item counts twice
	if v := scoreItems("a\na", "a\nb"); v.Pass {
		t.Errorf("duplicate must not cover b: %+v", v)
	}
	// objects in the list are compared by value, not by key order
	if v := scoreItems(`[{"n":"1","t":"A"},{"n":"2","t":"B"}]`, `[{"t":"B","n":"2"},{"t":"A","n":"1"}]`); !v.Pass {
		t.Errorf("objects: %+v", v)
	}
	if v := scoreItems("x", ""); v.Applicable {
		t.Errorf("a correct answer without items is not applicable: %+v", v)
	}
}

func TestScoreJSON(t *testing.T) { // C-5.3
	want := `{"nome":"Ana","valor":10,"itens":[1,2]}`
	cases := []struct {
		name, got string
		pass, app bool
		why       string
	}{
		{"same", `{"nome":"Ana","valor":10,"itens":[1,2]}`, true, true, ""},
		{"other key order and whitespace", "{\n  \"itens\": [1, 2],\n \"valor\": 10, \"nome\": \"Ana\"\n}", true, true, ""},
		{"number written 10.0", `{"nome":"Ana","valor":10.0,"itens":[1,2]}`, true, true, ""},
		{"SAME KEYS, DIFFERENT VALUES", `{"nome":"Bia","valor":10,"itens":[1,2]}`, false, true, "valores_diferentes"},
		{"different value of a number", `{"nome":"Ana","valor":11,"itens":[1,2]}`, false, true, "valores_diferentes"},
		{"different array order", `{"nome":"Ana","valor":10,"itens":[2,1]}`, false, true, "valores_diferentes"},
		{"extra key", `{"nome":"Ana","valor":10,"itens":[1,2],"x":1}`, false, true, "valores_diferentes"},
		{"invalid JSON", `{"nome":"Ana","valor":10,`, false, true, "json_invalido"},
		{"prose around", "Aqui está: " + want, false, true, "json_invalido"},
		{"code fence is not valid JSON", "```json\n" + want + "\n```", false, true, "json_invalido"},
		{"two values", want + want, false, true, "json_invalido"},
	}
	for _, c := range cases {
		v := scoreJSON(c.got, want)
		if v.Pass != c.pass || v.Applicable != c.app || v.Why != c.why {
			t.Errorf("%s: %+v, want pass=%v why=%q", c.name, v, c.pass, c.why)
		}
	}
	if v := scoreJSON(want, "isto não é json"); v.Applicable {
		t.Errorf("correct answer that is not JSON must be inapplicable: %+v", v)
	}
}

func TestScoreNumber(t *testing.T) { // C-5.4
	cases := []struct {
		name, got, want string
		pass            bool
	}{
		{"plain", "42", "42", true},
		{"after the steps", "Somando 10 + 32 = 42.\nO total é 42.", "RESULTADO: 42", true},
		{"RESULTADO line wins over later numbers", "RESULTADO: 42\nObs: conferido em 3 passos", "42", true},
		{"wrong", "RESULTADO: 41", "42", false},
		{"pt-BR thousands and decimals", "RESULTADO: 1.234,56", "1234.56", true},
		{"en thousands and decimals", "Total: 1,234.56", "1.234,56", true},
		{"thousands only", "1.234", "1234", true},
		{"millions", "1.234.567", "1234567", true},
		{"decimal comma", "3,5", "3.5", true},
		{"decimal point", "3.5", "3,5", true},
		{"trailing zero", "2.50", "2,5", true},
		{"negative", "RESULTADO: -7", "-7", true},
		{"sign matters", "RESULTADO: 7", "-7", false},
		{"hyphen glued to a word is not a minus", "item-3", "3", true},
		{"no number", "não sei", "5", false},
		{"percent", "42%", "42", true},
	}
	for _, c := range cases {
		v := scoreNumber(c.got, c.want)
		if !v.Applicable || v.Pass != c.pass {
			t.Errorf("%s: %+v, want pass=%v", c.name, v, c.pass)
		}
	}
	if v := scoreNumber("42", "sem número"); v.Applicable {
		t.Errorf("a correct answer without a number is inapplicable: %+v", v)
	}
}

func TestScoreTruncado(t *testing.T) { // C-5.4: fim=stop, resp_final=1, resp_truncated=0
	ok := reply{Fim: "stop", Final: true}
	if v := scoreReply("truncado", ok, ""); !v.Pass || !v.Applicable {
		t.Fatalf("complete answer: %+v", v)
	}
	for name, r := range map[string]reply{
		"fim=length":     {Fim: "length", Final: true},
		"fim missing":    {Fim: "", Final: true},
		"not final":      {Fim: "stop", Final: false},
		"resp_truncated": {Fim: "stop", Final: true, Truncated: true},
	} {
		if v := scoreReply("truncado", r, ""); v.Pass {
			t.Errorf("%s must fail", name)
		}
	}
}

func TestScoreReplyBlindAndOutsideAreNotAutomatic(t *testing.T) { // C-5.5, C-5.7
	for _, c := range []string{"resumo_infiel", "inventou", "traducao_errada", "ignorou_instrucao", "outro", "idioma_errado"} {
		if v := scoreReply(c, reply{Text: "x", Fim: "stop", Final: true}, "x"); v.Applicable || v.Pass {
			t.Errorf("%s must not be scored by the program: %+v", c, v)
		}
	}
}

func TestTwoOfThree(t *testing.T) { // C-5.15
	for _, c := range []struct {
		r    []bool
		pass bool
	}{
		{[]bool{true, true, true}, true}, {[]bool{true, true, false}, true}, {[]bool{false, true, true}, true},
		{[]bool{true, false, false}, false}, {[]bool{false, false, false}, false}, {[]bool{false, true, false}, false},
	} {
		if casePasses(c.r) != c.pass {
			t.Errorf("%v: pass=%v, want %v", c.r, casePasses(c.r), c.pass)
		}
	}
	if roundsDiffer([]bool{true, true, true}) || !roundsDiffer([]bool{true, true, false}) || !roundsDiffer([]bool{false, true, true}) {
		t.Error("roundsDiffer")
	}
}

func TestWilsonFixedVectors(t *testing.T) { // C-5.9 (values computed independently, z = 1.959964)
	for _, c := range []struct {
		k, n   int
		lo, hi float64
	}{
		{8, 10, 0.4902, 0.9433}, {0, 10, 0, 0.2775}, {10, 10, 0.7225, 1}, {15, 30, 0.3315, 0.6685},
		{27, 30, 0.7438, 0.9654}, {24, 30, 0.6269, 0.9049}, {1, 20, 0.0089, 0.2361},
	} {
		lo, hi := wilson(c.k, c.n)
		if math.Abs(lo-c.lo) > 5e-5 || math.Abs(hi-c.hi) > 5e-5 {
			t.Errorf("wilson(%d,%d) = %.4f %.4f, want %.4f %.4f", c.k, c.n, lo, hi, c.lo, c.hi)
		}
	}
	if lo, hi := wilson(0, 0); lo != 0 || hi != 0 {
		t.Errorf("n=0: %v %v", lo, hi)
	}
}
