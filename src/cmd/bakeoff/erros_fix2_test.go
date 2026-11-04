package main

import (
	"strings"
	"testing"
)

func TestWantNumberHasToBeUnambiguous(t *testing.T) { // the number of the CORRECT answer
	inapplicable := []string{
		"12 (ver seção 3)", // two numbers in the prose
		"Somou 10 e 32",    // two numbers
		"Total de 5 itens em 2 listas",
		"sem número nenhum",
		"1,500", // pt-BR 1500 or English 1.5
		"1.500", // the same
		"RESULTADO: 1,500",
		"12,500",
		"Total: 3.250",
	}
	for _, w := range inapplicable {
		if v := scoreNumber("1500", w); v.Applicable {
			t.Errorf("want %q must be inapplicable (not a model error): %+v", w, v)
		}
		if ok, _ := wantApplicable("contagem_errada", w); ok {
			t.Errorf("wantApplicable(%q) = true", w)
		}
	}
	applicable := map[string]string{
		"42":                          "42",
		"RESULTADO: 12 (ver seção 3)": "12", // the marked result wins over the prose
		"O total é 12":                "12",
		"0,500":                       "0.5",
		"1,5":                         "1.5",
		"1.234.567":                   "1234567",
		"1.234,56":                    "1234.56",
		"-7":                          "-7",
		"em 2024-05 foram 3":          "", // 2024 and 05 and 3: three numbers
	}
	for w, got := range applicable {
		ok, _ := wantApplicable("calculo_errado", w)
		if got == "" {
			if ok {
				t.Errorf("%q has more than one number", w)
			}
			continue
		}
		if !ok {
			t.Errorf("want %q must be applicable", w)
			continue
		}
		if v := scoreNumber(got, w); !v.Applicable || !v.Pass {
			t.Errorf("scoreNumber(%q, %q) = %+v", got, w, v)
		}
	}
	// the model's own answer is read as before (the last number), only the CORRECT answer is strict
	if v := scoreNumber("Somando 10 + 32 = 42. Total: 42", "42"); !v.Pass {
		t.Errorf("%+v", v)
	}
}

func TestDuplicateKeysInTheAnswerFail(t *testing.T) { // json.Valid accepts them and the last value would win
	want := `{"a":2,"b":{"c":[1,2]}}`
	for name, got := range map[string]string{
		"top level":         `{"a":1,"a":2,"b":{"c":[1,2]}}`,
		"nested":            `{"a":2,"b":{"c":[1,2],"c":[1,2]}}`,
		"in an array":       `{"a":2,"b":{"c":[1,2]},"x":[{"k":1,"k":1}]}`,
		"same key, value 2": `{"a":1,"a":2,"b":{"c":[1,2]}}`,
	} {
		v := scoreJSON(got, want)
		if v.Pass || !v.Applicable || v.Why != "chave_duplicada" {
			t.Errorf("%s: %+v", name, v)
		}
	}
	// the same key in different objects is not a duplicate
	if v := scoreJSON(`{"a":2,"b":{"c":[1,2]},"x":[{"a":1},{"a":2}]}`, `{"a":2,"b":{"c":[1,2]},"x":[{"a":1},{"a":2}]}`); !v.Pass {
		t.Errorf("distinct objects with the same key: %+v", v)
	}
	if v := scoreJSON(want, `{"a":1,"a":2}`); v.Applicable {
		t.Errorf("a correct answer with duplicate keys is inapplicable: %+v", v)
	}
	if !hasDupKeys([]byte(`{"a":{"b":1,"b":2}}`)) || hasDupKeys([]byte(`{"a":{"b":1},"c":{"b":2}}`)) || hasDupKeys([]byte(`[{"a":1},{"a":1}]`)) {
		t.Error("hasDupKeys")
	}
}

func TestPendingAndApurarVerdicts(t *testing.T) {
	blind := func(n int) catRes { return catRes{Cat: "inventou", Mode: modeBlind, Blind: n} }
	human := func(n, a, c int) catRes {
		return catRes{Cat: "inventou", Mode: modeBlind, Human: true, Blind: n, Atual: modelStat{N: n, Pass: a}, Cand: modelStat{N: n, Pass: c}}
	}
	// the automatic part is fine, the blind part is waiting: pending, nothing accepted
	in := cmp("omitiu", cat("omitiu", 30, 0, 6), blind(40))
	o := evaluate(in)
	if o.Code != exitPending || o.Accepted {
		t.Fatalf("%+v", o)
	}
	rep := strings.ToLower(renderReport(in, o))
	if !strings.Contains(rep, "pendente da revisão às cegas") || strings.Contains(rep, "melhora aceita") || strings.Contains(rep, "sem regressão") {
		t.Errorf("wording:\n%s", rep)
	}
	// a broken automatic limit is a rejection even with the review pending
	in = cmp("omitiu", cat("omitiu", 30, 14, 6), blind(40))
	if o := evaluate(in); o.Code != exitRejected {
		t.Errorf("%+v", o)
	}
	// unmeasured cases fail the gate even when pending
	in = cmp("omitiu", cat("omitiu", 30, 0, 6), blind(40))
	in.Unmeasured = 2
	if o := evaluate(in); o.Code != exitGate {
		t.Errorf("%+v", o)
	}
	// only the blind category, pending, with the blind target: pending (not a gate failure)
	in = cmp("inventou", blind(40))
	if o := evaluate(in); o.Code != exitPending {
		t.Errorf("%+v", o)
	}
	// after the owner's review: the limits apply to the blind category
	for _, c := range []struct {
		name  string
		cat   catRes
		code  int
		alvo  string
		other bool
	}{
		{"target gains 100 points", human(30, 0, 30), exitOK, "inventou", false},
		{"target gains nothing", human(30, 15, 15), exitRejected, "inventou", false},
		{"target N=29 does not decide", human(29, 0, 29), exitGate, "inventou", false},
		{"non-target drops 1 of 30", human(30, 30, 29), exitRejected, "omitiu", true},
		{"non-target equal", human(30, 30, 30), exitOK, "omitiu", true},
	} {
		cats := []catRes{c.cat}
		if c.other {
			cats = append(cats, cat("omitiu", 30, 0, 6))
		}
		in := cmp(c.alvo, cats...)
		in.Apurado = true
		if o := evaluate(in); o.Code != c.code {
			t.Errorf("%s: code %d, want %d (%+v)", c.name, o.Code, c.code, o)
		}
	}
	// the limits of the human categories are not applied until the owner judged them
	in = cmp("omitiu", cat("omitiu", 30, 0, 6), blind(40))
	in.Cats[1].Atual = modelStat{N: 40, Pass: 40}
	in.Cats[1].Cand = modelStat{N: 40, Pass: 0}
	if o := evaluate(in); o.Code != exitPending {
		t.Errorf("counts of a category not yet judged were used: %+v", o)
	}
}
