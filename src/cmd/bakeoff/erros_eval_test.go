package main

// Thresholds and minimums of the protocol (C-5.8 to C-5.13, C-5.16), on crafted
// numbers: no gateway, no database.

import (
	"strings"
	"testing"
)

func cat(name string, n, atual, cand int) catRes {
	return catRes{Cat: name, Mode: categoryMode(name), Atual: modelStat{N: n, Pass: atual}, Cand: modelStat{N: n, Pass: cand}}
}

var goodS23 = &s23{S2OK: 90, S2Tot: 100, S3OK: 28, S3Tot: 30}

func cmp(alvo string, cats ...catRes) reportInput {
	return reportInput{Mode: "comparar", RetratoID: strings.Repeat("a", 32), Atual: "qwen3-ia", Candidato: "v2", Alvo: alvo, Cats: cats, AtualS23: goodS23, CandS23: goodS23}
}

func TestTargetGainNeedsTenPointsAndALowerBoundAbovePoint(t *testing.T) { // C-5.10
	cases := []struct {
		name         string
		n, atual, cd int
		accepted     bool
		why          string
	}{
		{"+10 points and the lower bound above the point (0/30 -> 3/30)", 30, 0, 3, true, ""},
		{"+9 points (0/100 -> 9/100)", 100, 0, 9, false, "ganhou menos de 10 pontos"},
		{"+10 points but the lower bound does not pass the point (15/30 -> 18/30)", 30, 15, 18, false, "limite inferior"},
		{"+40 points", 30, 14, 26, true, ""},
		{"worse", 30, 14, 10, false, "ganhou menos de 10 pontos"},
	}
	for _, c := range cases {
		in := cmp("omitiu", cat("omitiu", c.n, c.atual, c.cd))
		o := evaluate(in)
		if len(o.GateFail) != 0 {
			t.Fatalf("%s: gate %v", c.name, o.GateFail)
		}
		if o.Accepted != c.accepted || (c.accepted && o.Code != exitOK) || (!c.accepted && o.Code != exitRejected) {
			t.Errorf("%s: %+v", c.name, o)
		}
		if c.why != "" && !strings.Contains(strings.Join(o.Rejects, "|"), c.why) {
			t.Errorf("%s: rejects %v, want %q", c.name, o.Rejects, c.why)
		}
	}
}

func TestOtherCategoriesMayNotDropMoreThanThreePoints(t *testing.T) { // C-5.11, only with N >= 30
	target := cat("omitiu", 30, 0, 6)
	cases := []struct {
		name     string
		other    catRes
		accepted bool
	}{
		{"one case of 30 is 3.33 points: rejected", cat("formato_invalido", 30, 20, 19), false},
		{"exactly 3 points (3 of 100) is allowed", cat("formato_invalido", 100, 50, 47), true},
		{"3.0x points (4 of 100) is rejected", cat("formato_invalido", 100, 50, 46), false},
		{"a rise is fine", cat("formato_invalido", 30, 20, 25), true},
		{"N = 29 only reports: a large drop does not decide", cat("formato_invalido", 29, 20, 5), true},
	}
	for _, c := range cases {
		o := evaluate(cmp("omitiu", target, c.other))
		if o.Accepted != c.accepted {
			t.Errorf("%s: %+v", c.name, o)
		}
	}
}

func TestS2S3MayNotDropMoreThanTwoPoints(t *testing.T) { // C-5.12
	for _, c := range []struct {
		name     string
		cand     *s23
		accepted bool
	}{
		{"same", &s23{90, 100, 28, 30}, true},
		{"S2 exactly -2 points", &s23{88, 100, 28, 30}, true},
		{"S2 -3 points", &s23{87, 100, 28, 30}, false},
		{"S3 -1 case of 30 (3.33 points)", &s23{90, 100, 27, 30}, false},
		{"S3 rises", &s23{90, 100, 30, 30}, true},
	} {
		in := cmp("omitiu", cat("omitiu", 30, 0, 6))
		in.CandS23 = c.cand
		if o := evaluate(in); o.Accepted != c.accepted {
			t.Errorf("%s: %+v", c.name, o)
		}
	}
}

func TestMinimumsReportAndDecide(t *testing.T) { // C-5.8
	// 19 cases: not reported (no rates in the report); 20 and 29: reported but do not decide; 30 decides
	in := cmp("omitiu", cat("omitiu", 30, 0, 6), cat("numeracao_errada", 19, 10, 12), cat("formato_invalido", 20, 10, 12), cat("contagem_errada", 29, 10, 12))
	rep := renderReport(in, evaluate(in))
	for _, want := range []string{"| numeracao_errada | 19 | - |", "N < 20: não reportada", "| formato_invalido | 20 | 50,0%", "| contagem_errada | 29 | 34,5%", "reportada; N < 30, não decide"} {
		if !strings.Contains(rep, want) {
			t.Errorf("report misses %q\n%s", want, rep)
		}
	}
	if strings.Contains(rep, "52,6%") || strings.Contains(rep, "63,2%") { // 10/19 and 12/19 must not show
		t.Error("a category with N < 20 shows its rates")
	}
	// the thresholds of C-5.11 are not applied to the categories under 30 (a 20-case drop of 50 points does not reject)
	in = cmp("omitiu", cat("omitiu", 30, 0, 6), cat("formato_invalido", 20, 20, 10))
	if o := evaluate(in); !o.Accepted {
		t.Errorf("a category under 30 decided: %+v", o)
	}
}

func TestGateConditions(t *testing.T) { // C-5.13: every one of them fails the gate and says so
	ok := cat("omitiu", 30, 0, 6)
	cases := []struct {
		name string
		mod  func(*reportInput)
		why  string
	}{
		{"0 cases", func(in *reportInput) { in.Cats = []catRes{cat("omitiu", 0, 0, 0)} }, "(0 casos)"},
		{"no categories at all", func(in *reportInput) { in.Cats = nil }, "(0 casos)"},
		{"no category with 30", func(in *reportInput) { in.Cats = []catRes{cat("omitiu", 29, 0, 6), cat("formato_invalido", 12, 0, 0)} }, "mínimo de 30"},
		{"blind category already judged but nothing automatic and N=12", func(in *reportInput) {
			in.Cats = []catRes{{Cat: "inventou", Mode: modeBlind, Human: true, Blind: 12, Atual: modelStat{N: 12}, Cand: modelStat{N: 12}}}
			in.Alvo = ""
		}, "mínimo de 30"},
		{"unmeasured cases", func(in *reportInput) { in.Unmeasured = 1 }, "não puderam ser medidos"},
		{"invalid retrato", func(in *reportInput) { in.GateExtra = []string{"o retrato está inválido (valido=0, C-5.13)"} }, "valido=0"},
		{"no S2/S3 for the candidate", func(in *reportInput) { in.CandS23 = nil }, "S2 e o S3"},
		{"target under 30", func(in *reportInput) {
			in.Alvo = "formato_invalido"
			in.Cats = append(in.Cats, cat("formato_invalido", 25, 0, 0))
		}, "categoria-alvo"},
	}
	for _, c := range cases {
		in := cmp("omitiu", ok)
		c.mod(&in)
		o := evaluate(in)
		if o.Code != exitGate || len(o.GateFail) == 0 || o.Accepted {
			t.Errorf("%s: %+v", c.name, o)
			continue
		}
		rep := strings.ToLower(renderReport(in, o))
		if !strings.Contains(rep, "dados insuficientes") || !strings.Contains(rep, "o gate falha") || strings.Contains(rep, "sem regressão") || strings.Contains(rep, "melhora aceita") {
			t.Errorf("%s: wrong wording\n%s", c.name, rep)
		}
		if !strings.Contains(strings.Join(o.GateFail, "|"), c.why) {
			t.Errorf("%s: reasons %v, want %q", c.name, o.GateFail, c.why)
		}
	}
	// and a valid input does not fail
	if o := evaluate(cmp("omitiu", ok)); o.Code != exitOK || !o.Accepted {
		t.Errorf("valid input: %+v", o)
	}
}

func TestWithoutATargetOnlyRegressionIsChecked(t *testing.T) {
	in := cmp("", cat("omitiu", 30, 14, 14))
	o := evaluate(in)
	rep := renderReport(in, o)
	if !o.Accepted || !strings.Contains(rep, "Sem regressão") || !strings.Contains(rep, "a melhora não foi avaliada") {
		t.Errorf("%+v\n%s", o, rep)
	}
}

func TestBaselineDivergenceWarning(t *testing.T) { // C-5.16: more than 3 points in a category
	base := func(acertos int) *baselineFile {
		return &baselineFile{Cats: map[string]baselineCt{"omitiu": {N: 100, Acertos: acertos}}}
	}
	for _, c := range []struct {
		acertos int
		warn    bool
	}{{50, false}, {53, false}, {47, false}, {54, true}, {46, true}, {80, true}} {
		in := cmp("", cat("omitiu", 100, 50, 50))
		in.Baseline = base(c.acertos)
		o := evaluate(in)
		if got := strings.Contains(strings.Join(o.Warnings, "|"), "C-5.16"); got != c.warn {
			t.Errorf("baseline %d/100 vs now 50/100: warn=%v, want %v", c.acertos, got, c.warn)
		}
	}
	// no baseline: a warning that the baseline is missing
	in := cmp("", cat("omitiu", 30, 14, 14))
	if o := evaluate(in); !strings.Contains(strings.Join(o.Warnings, "|"), "não há linha de base") {
		t.Errorf("%v", o.Warnings)
	}
}
