package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testGab = []gabItem{{"1.", "Introdução"}, {"1.1", "Convenções, termos e abreviações"}, {"2.", "Modelo do negócio"}}

func TestScoreS2(t *testing.T) {
	body := "```json\n[{\"numero\":\"1\",\"titulo\":\"Introdução\"},{\"numero\":\"1.1\",\"titulo\":\"Visão geral do documento\"},{\"numero\":\"1.2\",\"titulo\":\"Convenções\"}]\n```"
	sc := scoreS2(body, testGab)
	if !sc.JSONValid || sc.Strict {
		t.Fatalf("json=%v strict=%v, want valid and fenced", sc.JSONValid, sc.Strict)
	}
	if sc.NumOK != 2 || sc.NumTitleOK != 1 {
		t.Fatalf("NumOK=%d NumTitleOK=%d, want 2 and 1", sc.NumOK, sc.NumTitleOK)
	}
	if len(sc.Invented) != 1 || sc.Invented[0] != "1.2" {
		t.Fatalf("Invented=%v, want [1.2]", sc.Invented)
	}
	if len(sc.Missing) != 1 || sc.Missing[0] != "2" {
		t.Fatalf("Missing=%v, want [2]", sc.Missing)
	}
	if bad := scoreS2("Aqui está: [", testGab); bad.JSONValid || len(bad.Missing) != 3 {
		t.Fatalf("invalid JSON: %+v", bad)
	}
}

func TestFormats(t *testing.T) {
	cases := []struct {
		name   string
		f      func(string) fmtScore
		body   string
		ok, st bool
	}{
		{"json", checkJSONStrings, `["a","b"]`, true, true},
		{"json fenced", checkJSONStrings, "```json\n[\"a\"]\n```", true, false},
		{"json prose", checkJSONStrings, "Segue:\n[\"a\"]", false, false},
		{"list", checkDashList, "- a\n- b", true, true},
		{"list prose", checkDashList, "Tópicos:\n- a", false, false},
		{"list star", checkDashList, "* a\n* b", false, false},
		{"table", checkTable2, "| item | descrição |\n|---|---|\n| a | b |", true, true},
		{"table 3 cols", checkTable2, "| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |", false, false},
		{"table prose", checkTable2, "Tabela:\n| a | b |\n|---|---|\n| 1 | 2 |", false, false},
	}
	for _, c := range cases {
		got := c.f(c.body)
		if got.OK != c.ok || got.Strict != c.st {
			t.Errorf("%s: got ok=%v strict=%v (%s), want ok=%v strict=%v", c.name, got.OK, got.Strict, got.Why, c.ok, c.st)
		}
	}
}

func TestScoreStruct(t *testing.T) {
	src := "1.1\tIntrodução\n- O requisito [RFXX001] usa CM6502S.\n- Veja https://x.y/z"
	dst := "1.1\tIntroduction\n- Requirement [RFXX001] uses CM6502S.\n- See https://x.y/z"
	sc := scoreStruct(src, dst)
	if sc.SrcLines != 3 || sc.DstLines != 3 || sc.SrcMarkers != 3 || sc.DstMarkers != 3 {
		t.Fatalf("lines/markers: %+v", sc)
	}
	if sc.IdsTotal == 0 || sc.IdsKept != sc.IdsTotal {
		t.Fatalf("ids: %+v", sc)
	}
}

func TestScreenSummary(t *testing.T) {
	src := "O CRM foi criado em 1.999 pela Empresa Alfa."
	sc := screenSummary("Segundo a Empresa Alfa, o CRM surgiu em 1999 e custa 300 reais na Beta.", src)
	if len(sc.NumsAbsent) != 1 || sc.NumsAbsent[0] != "300" {
		t.Fatalf("NumsAbsent=%v, want [300]", sc.NumsAbsent)
	}
	found := strings.Join(sc.NameAbsent, ",")
	if !strings.Contains(found, "Beta") || strings.Contains(found, "Alfa") {
		t.Fatalf("NameAbsent=%v, want Beta and not Alfa", sc.NameAbsent)
	}
}

func TestBodyOfAndUso(t *testing.T) {
	text := "resumo aqui\n\n[arquivo] nome=x formato=pdf\n\n[uso] modelo=m prompt_eval_count=10 eval_count=5 total_ms=100 load_ms=0 queue_ms=0 partes=1 fim=stop\n"
	if b := bodyOf(text); b != "resumo aqui" {
		t.Fatalf("bodyOf=%q", b)
	}
	_, kv := parseUso(text)
	if kv["eval_count"] != "5" || kv["fim"] != "stop" || kv["modelo"] != "m" {
		t.Fatalf("kv=%v", kv)
	}
}

func TestRunRefusesProductionBinary(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-exe", `D:\MCP\mcpollama-mcp.exe`, "-model", "x"}, &out, &errb); code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	if !strings.Contains(errb.String(), "binário de teste") {
		t.Fatalf("stderr=%q", errb.String())
	}
}

func TestSafeName(t *testing.T) {
	if got := safeName("gemma4:12b"); got != "gemma4_12b" {
		t.Fatalf("safeName=%q", got)
	}
}

func TestRiskGateSkipsLargePrefill(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("palavra ", 4000) // 32000 chars
	if err := os.WriteFile(filepath.Join(dir, "b03.pdf.txt"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	var lines []string
	r := &runner{maxPrefill: 8000, cpt: 3.0, refDir: dir, logf: func(f string, a ...any) { lines = append(lines, fmt.Sprintf(f, a...)) }}
	// No client: a call that is not skipped would panic on the nil client.
	res := r.call("s1", "b03.pdf-r1", "summarize_file", map[string]any{"path": `E:\x\b03.pdf`}, "")
	if r.skipped != 1 || !strings.HasPrefix(res.Err, "pulado-risco") || res.IsError {
		t.Fatalf("skipped=%d res=%+v", r.skipped, res)
	}
	if est, _ := r.estimateTokens(map[string]any{"prompt": "curto"}); est > 1000 {
		t.Fatalf("small prompt estimated at %d tokens", est)
	}
	if est, src := r.estimateTokens(map[string]any{"path": `E:\x\naoexiste.docx`}); est <= 8000 || src != "texto_indisponivel" {
		t.Fatalf("unknown file must count as too large: %d %s", est, src)
	}
}
