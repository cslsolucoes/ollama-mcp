package main

// Corrections of the independent review of stage 12.7: the retrato checked again
// after the replay, cut streams, the blind review (pending and mode apurar), the
// remote gateway, the baseline of the same retrato, the number of the correct
// answer, the hashes of 12.8 and the extension point of the request.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

func blindSet(nOmitiu, nBlind int) []synthCase {
	var cs []synthCase
	for i := 1; i <= nOmitiu; i++ {
		cs = append(cs, omitiu(i))
	}
	for i := 1; i <= nBlind; i++ {
		cs = append(cs, blindCase(nOmitiu+i))
	}
	return cs
}

// runBase makes the retrato and registers its baseline.
func (f *fixture) runBase() string {
	f.t.Helper()
	rid := f.makeRetrato()
	be := f.opts("base")
	be.RetratoID = rid
	if code, out := f.run(be); code != 0 {
		f.t.Fatalf("base: code %d\n%s", code, out)
	}
	return rid
}

func (f *fixture) compareOpts(rid, alvo string) errosOpts {
	writeScore(f.t, f.out, "qwen3-ia", 90, 100, 28, 30)
	writeScore(f.t, f.out, "qwen3-ia-v2", 90, 100, 28, 30)
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", alvo
	return ce
}

func (f *fixture) dbExec(q string, args ...any) {
	f.t.Helper()
	db, err := sql.Open("sqlite", f.db)
	if err != nil {
		f.t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

// ---- 1. the retrato is checked again after the replay --------------------------------------------------

func TestRetratoChangedDuringTheRoundFailsTheGate(t *testing.T) {
	mutations := map[string]func(f *fixture, rid string){
		"retrato invalidated": func(f *fixture, rid string) { f.dbExec(`UPDATE retratos SET valido=0 WHERE id=?`, rid) },
		"occurrence discarded": func(f *fixture, rid string) {
			f.dbExec(`UPDATE ocorrencias SET estado='descartada' WHERE id=?`, synthID(3))
		},
		"correct answer edited": func(f *fixture, rid string) {
			f.dbExec(`UPDATE ocorrencias SET resposta_correta='item-SEGREDO-9' WHERE id=?`, synthID(3))
		},
		"item added to the frozen list": func(f *fixture, rid string) {
			f.dbExec(`INSERT INTO retrato_itens(retrato_id,ocorrencia_id,job_id,prompt_hmac,categoria) SELECT retrato_id,ocorrencia_id,job_id,prompt_hmac,categoria FROM retrato_itens WHERE retrato_id=? ORDER BY ocorrencia_id DESC LIMIT 1`, rid)
		},
		"sample replaced (same retrato id, other request)": func(f *fixture, rid string) {
			f.dbExec(`UPDATE amostras SET prompt_json=CAST('{"model":"qwen3-ia","prompt":"PEDIDO-x outro"}' AS BLOB) WHERE job_id=?`, synthJob(3))
		},
	}
	for name, mut := range mutations {
		for _, mode := range []string{"base", "comparar"} {
			f := newFixture(t, syntheticSet(), scenarioPolicy(false))
			rid := f.makeRetrato()
			if mode == "comparar" {
				f.ensureBaseline(rid)
			}
			eo := f.opts(mode)
			if mode == "comparar" {
				eo = f.compareOpts(rid, "omitiu")
			}
			eo.RetratoID = rid
			var once sync.Once
			f.gw.onReq = func(n int) {
				if n == 40 { // in the middle of the replay
					once.Do(func() { mut(f, rid) })
				}
			}
			code, out := f.run(eo)
			if code != exitGate || !strings.Contains(out, "retrato invalidado durante a rodada") {
				t.Errorf("%s/%s: code %d\n%s", name, mode, code, out)
				continue
			}
			gateText(t, out)
			if _, err := os.Stat(filepath.Join(f.out, "erros", rid, "linha-de-base.json")); mode == "base" && err == nil {
				t.Errorf("%s: a baseline was registered from a retrato that changed during the round", name)
			}
			if strings.Contains(out, "LINHA DE BASE REGISTRADA") || strings.Contains(out, "MELHORA ACEITA") {
				t.Errorf("%s/%s: accepted something", name, mode)
			}
		}
	}
}

// ---- 2. a cut stream is not measured -----------------------------------------------------------------------

func TestCutStreamIsNotMeasuredInAnyCategory(t *testing.T) {
	for _, how := range []string{"nodone", "abort"} {
		cs := blindSet(30, 0)
		for i := 31; i <= 40; i++ { // `truncado` cases: a cut stream must not count as a (failed) measure either
			cs = append(cs, synthCase{N: i, Cat: "truncado", Correct: "ok", Right: "resposta completa", Wrong: "resposta completa"})
		}
		f := newFixture(t, cs, func(model string, seed int, c synthCase) (string, string) { return answer(true, c) })
		rid := f.makeRetrato()
		f.gw.cut = func(model string, seed int, c synthCase) string {
			if c.Cat == "truncado" || c.N == 5 {
				return how
			}
			return ""
		}
		eo := f.opts("base")
		eo.RetratoID = rid
		code, out := f.run(eo)
		if code != exitGate || !strings.Contains(out, "11 caso(s) não puderam ser medidos") {
			t.Fatalf("%s: code %d\n%s", how, code, out)
		}
		gateText(t, out)
		if !strings.Contains(out, "| truncado | 0 |") {
			t.Errorf("%s: the cut `truncado` cases were counted:\n%s", how, out)
		}
	}
}

// ---- 3. the blind review: pending, then mode apurar ------------------------------------------------------

func answersFile(t *testing.T, keyPath string, judge func(cat string, i int, model string) bool) string {
	t.Helper()
	rows, err := readKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("# respostas do owner\n\n")
	for i, r := range rows {
		a, bb := "errada", "errada"
		if judge(r.Cat, i, r.A) {
			a = "certa"
		}
		if judge(r.Cat, i, r.B) {
			bb = "certa"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\n", r.Label, a, bb)
	}
	p := filepath.Join(t.TempDir(), "respostas.tsv")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *fixture) findOne(suffix string) string {
	f.t.Helper()
	for _, p := range sidecarFiles(f.t, f.out) {
		if strings.HasSuffix(p, suffix) {
			return p
		}
	}
	f.t.Fatalf("no file *%s", suffix)
	return ""
}

func (f *fixture) apurarOpts(resp string) errosOpts {
	ae := f.opts("apurar")
	ae.Resultado, ae.BlindRespostas = f.findOne(".json"), resp
	// the only .json besides the baseline is the result file
	for _, p := range sidecarFiles(f.t, f.out) {
		if strings.Contains(filepath.Base(p), "resultado-") {
			ae.Resultado = p
		}
	}
	return ae
}

func TestBlindPendingThenApurar(t *testing.T) {
	f := newFixture(t, blindSet(30, 30), scenarioPolicy(false))
	rid := f.runBase()
	ce := f.compareOpts(rid, "inventou") // the target is judged by the owner
	code, out := f.run(ce)
	if code != exitPending || !strings.Contains(out, "PENDENTE da revisão às cegas") {
		t.Fatalf("code %d\n%s", code, out)
	}
	low := strings.ToLower(out)
	if strings.Contains(low, "melhora aceita") || strings.Contains(low, "sem regressão") || strings.Contains(low, "dados insuficientes") {
		t.Errorf("a pending result must not accept anything:\n%s", out)
	}
	for _, want := range []string{"rótulo<TAB>avaliação de A<TAB>avaliação de B", "`certa`", "-erros-modo apurar", "resultado-"} {
		if !strings.Contains(out, want) {
			t.Errorf("the pending report misses %q (the format of the owner's file)", want)
		}
	}
	key := f.findOne(".chave.tsv")

	// the owner's judgement: the current model is wrong in every case, the candidate right in every case
	good := answersFile(t, key, func(cat string, i int, model string) bool { return model == "candidato" })
	code, out = f.run(f.apurarOpts(good))
	if code != exitOK || !strings.Contains(out, "MELHORA ACEITA") || !strings.Contains(out, "julgadas pelo owner") || !strings.Contains(out, "| inventou | 30 | 0,0% |") {
		t.Fatalf("apurar accept: code %d\n%s", code, out)
	}
	// no gateway call is made by mode apurar
	n := len(f.gw.requests())
	f.run(f.apurarOpts(good))
	if len(f.gw.requests()) != n {
		t.Error("mode apurar called the gateway")
	}

	// no gain for the target: rejected (C-5.10)
	same := answersFile(t, key, func(cat string, i int, model string) bool { return true })
	if code, out := f.run(f.apurarOpts(same)); code != exitRejected || !strings.Contains(out, "C-5.10: o alvo inventou ganhou menos de 10 pontos") {
		t.Fatalf("apurar no gain: code %d\n%s", code, out)
	}
	// an unanswered case: the review is incomplete
	partial := answersFile(t, key, func(cat string, i int, model string) bool { return true })
	b, _ := os.ReadFile(partial)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	os.WriteFile(partial, []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"), 0o600)
	if code, out := f.run(f.apurarOpts(partial)); code != exitGate || !strings.Contains(out, "avaliação às cegas incompleta: 1 caso(s) sem resposta") {
		t.Fatalf("apurar incomplete: code %d\n%s", code, out)
	} else {
		gateText(t, out)
	}
	// malformed answers are refused (usage), never guessed
	rows, _ := readKey(key)
	for name, body := range map[string]string{
		"unknown label": "C99\tcerta\tcerta\n",
		"duplicate":     rows[0].Label + "\tcerta\tcerta\n" + rows[0].Label + "\tcerta\tcerta\n",
		"bad value":     rows[0].Label + "\tmelhor\tcerta\n",
		"two columns":   rows[0].Label + "\tcerta\n",
	} {
		p := filepath.Join(t.TempDir(), "r.tsv")
		os.WriteFile(p, []byte(body), 0o600)
		if code, out := f.run(f.apurarOpts(p)); code != exitUsage {
			t.Errorf("%s: code %d\n%s", name, code, out)
		}
	}
	// the retrato went invalid after the comparison: the verdict does not stand
	f.dbExec(`UPDATE retratos SET valido=0 WHERE id=?`, rid)
	if code, out := f.run(f.apurarOpts(good)); code != exitGate || !strings.Contains(out, "valido=0") {
		t.Fatalf("apurar with an invalid retrato: code %d\n%s", code, out)
	}
	// nothing written holds text (the result file and the reports), only the blind file does
	for _, p := range sidecarFiles(t, f.out) {
		if strings.HasPrefix(filepath.Base(p), "blind-") || strings.HasSuffix(p, "score.json") {
			continue
		}
		body := readFile(t, p)
		for _, m := range secretMarkers {
			if strings.Contains(body, m) {
				t.Errorf("%s contains case text (%q)", p, m)
			}
		}
	}
	for _, l := range f.logs {
		for _, m := range secretMarkers {
			if strings.Contains(l, m) {
				t.Errorf("a log line contains case text (%q)", m)
			}
		}
	}
}

func TestApurarAppliesC511ToTheBlindCategoriesToo(t *testing.T) {
	f := newFixture(t, blindSet(30, 30), scenarioPolicy(false))
	rid := f.runBase()
	ce := f.compareOpts(rid, "omitiu") // the target is automatic (14 -> 27 of 30); the blind category must not drop
	if code, out := f.run(ce); code != exitPending {
		t.Fatalf("code %d\n%s", code, out)
	}
	key := f.findOne(".chave.tsv")
	// candidate right in 29 of 30, current right in all 30: one case of 30 is 3.33 points
	drop := answersFile(t, key, func(cat string, i int, model string) bool { return model == "atual" || i != 0 })
	if code, out := f.run(f.apurarOpts(drop)); code != exitRejected || !strings.Contains(out, "C-5.11: a categoria inventou caiu mais de 3 pontos") {
		t.Fatalf("code %d\n%s", code, out)
	}
	equal := answersFile(t, key, func(cat string, i int, model string) bool { return true })
	if code, out := f.run(f.apurarOpts(equal)); code != exitOK || !strings.Contains(out, "MELHORA ACEITA") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestPendingNeverHidesAnAutomaticRejection(t *testing.T) { // the owner's review cannot repair a broken limit
	cs := blindSet(30, 0)
	for i := 31; i <= 60; i++ {
		cs = append(cs, formato(i)) // formato_invalido, N = 30
	}
	for i := 101; i <= 103; i++ {
		cs = append(cs, blindCase(i))
	}
	f := newFixture(t, cs, scenarioPolicy(true)) // the candidate loses one formato_invalido case
	rid := f.runBase()
	code, out := f.run(f.compareOpts(rid, "omitiu"))
	if code != exitRejected || !strings.Contains(out, "MUDANÇA REPROVADA") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestBaselineWithBlindOnlyAndWithSmallAutomatic(t *testing.T) {
	// blind cases only: nothing automatic to record, the baseline is registered
	f := newFixture(t, blindSet(0, 5), scenarioPolicy(false))
	rid := f.makeRetrato()
	be := f.opts("base")
	be.RetratoID = rid
	if code, out := f.run(be); code != exitOK {
		t.Fatalf("blind only: code %d\n%s", code, out)
	}
	// automatic cases that exist but are under 30 do not make a baseline, even with blind cases next to them
	f2 := newFixture(t, blindSet(5, 5), scenarioPolicy(false))
	rid2 := f2.makeRetrato()
	be2 := f2.opts("base")
	be2.RetratoID = rid2
	code, out := f2.run(be2)
	if code != exitGate || !strings.Contains(out, "mínimo de 30") {
		t.Fatalf("small automatic: code %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(f2.out, "erros", rid2, "linha-de-base.json")); err == nil {
		t.Error("a baseline was registered from insufficient data")
	}
}

// ---- 4. the baseline of the same retrato -------------------------------------------------------------------

func TestComparisonNeedsTheBaselineOfTheSameRetratoAndModel(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	f.noBaseline = true
	rid := f.makeRetrato()
	ce := f.compareOpts(rid, "omitiu")
	if code, out := f.run(ce); code != exitUsage || !strings.Contains(out, "linha de base") || !strings.Contains(out, "arquivo ausente") {
		t.Fatalf("no baseline: code %d\n%s", code, out)
	}
	dir := filepath.Join(f.out, "erros", rid)
	base := filepath.Join(dir, "linha-de-base.json")
	os.WriteFile(base, mustJSON(baselineFile{RetratoID: strings.Repeat("b", 32), Modelo: "qwen3-ia"}), 0o600)
	if code, out := f.run(ce); code != exitUsage || !strings.Contains(out, "é de outro retrato") {
		t.Fatalf("other retrato: code %d\n%s", code, out)
	}
	os.WriteFile(base, mustJSON(baselineFile{RetratoID: rid, Modelo: "qwen3-ia-antigo"}), 0o600)
	if code, out := f.run(ce); code != exitUsage || !strings.Contains(out, "é do modelo") {
		t.Fatalf("other model: code %d\n%s", code, out)
	}
	if n := len(f.gw.requests()); n != 0 {
		t.Errorf("%d calls were made before the baseline was checked", n)
	}
	os.WriteFile(base, mustJSON(baselineFile{RetratoID: rid, Modelo: "qwen3-ia", Cats: map[string]baselineCt{}}), 0o600)
	if code, _ := f.run(ce); code == exitUsage {
		t.Error("a baseline of the same retrato and model was refused")
	}
}

// ---- 7. 12.8: hashes in the retrato, and the request transform -----------------------------------------

func TestComparisonRefusedForARetratoWithIndexOrInstructionsThatWereNotGiven(t *testing.T) {
	for name, tc := range map[string]struct {
		h    treino.RetratoOptions
		want string
	}{
		"index":        {treino.RetratoOptions{IndiceHash: "abc"}, "passe -indice"},
		"instructions": {treino.RetratoOptions{InstrucoesID: "v1", InstrucoesHash: "h"}, "passe -instrucoes"},
		"id only":      {treino.RetratoOptions{InstrucoesID: "v1"}, "inconsistente"},
	} {
		h := tc.h
		f := newFixture(t, syntheticSet(), scenarioPolicy(false))
		ro := f.opts("retrato")
		ro.Hashes = h
		code, out := f.run(ro)
		m := retratoIDRe.FindStringSubmatch(out)
		if code != 0 || m == nil {
			t.Fatalf("%s: %d\n%s", name, code, out)
		}
		// the baseline of the current model is fine in such a retrato
		be := f.opts("base")
		be.RetratoID = m[1]
		if code, out := f.run(be); code != 0 {
			t.Errorf("%s: base code %d\n%s", name, code, out)
		}
		f.gw.reset()
		ce := f.compareOpts(m[1], "omitiu")
		if code, out := f.run(ce); code != exitUsage || !strings.Contains(out, tc.want) {
			t.Errorf("%s: code %d\n%s", name, code, out)
		}
		if n := len(f.gw.requests()); n != 0 {
			t.Errorf("%s: %d calls", name, n)
		}
	}
}

type marker struct{}

func (marker) Transform(_ treino.RetratoCase, pj []byte) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(pj, &m); err != nil {
		return nil, err
	}
	m["system"] = json.RawMessage(`"INSTRUCOES-V1"`)
	return json.Marshal(m)
}

func TestRequestTransformIsAppliedToTheCandidateOnly(t *testing.T) {
	f := newFixture(t, blindSet(2, 2), scenarioPolicy(false))
	rid := f.makeRetrato()
	ce := f.compareOpts(rid, "")
	ce.Transform = marker{}
	f.run(ce)
	nc, na := 0, 0
	for _, r := range f.gw.requests() {
		_, has := r.Body["system"]
		if r.Model == "qwen3-ia" {
			na++
			if has {
				t.Errorf("the current model got the transformed request: %+v", r)
			}
		} else {
			nc++
			if !has {
				t.Errorf("the candidate got the recorded request untransformed: %+v", r)
			}
		}
	}
	if nc == 0 || na == 0 {
		t.Fatalf("calls: candidate %d, current %d", nc, na)
	}
	// the default is the identity: the recorded request, only seed and model change
	pj, err := identityTransform{}.Transform(treino.RetratoCase{}, []byte(`{"a":1}`))
	if err != nil || !bytes.Equal(pj, []byte(`{"a":1}`)) {
		t.Errorf("identity: %s %v", pj, err)
	}
}

// ---- minors ----------------------------------------------------------------------------------------------

func TestRecordedModelHasToBeTheCurrentModel(t *testing.T) {
	c := omitiu(1)
	c.Model = "qwen3-ia-think"
	f := newFixture(t, []synthCase{c}, scenarioPolicy(false))
	rid := f.makeRetrato()
	be := f.opts("base")
	be.RetratoID = rid
	code, out := f.run(be)
	if code != exitUsage || !strings.Contains(out, `foi gravado com o modelo "qwen3-ia-think" e -atual é "qwen3-ia"`) {
		t.Fatalf("code %d\n%s", code, out)
	}
	if n := len(f.gw.requests()); n != 0 {
		t.Errorf("%d calls", n)
	}
	be.Atual = "qwen3-ia-think"
	if code, out := f.run(be); code == exitUsage {
		t.Errorf("the right -atual was refused:\n%s", out)
	}
}

func TestOutIsRequiredBySuiteErros(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-suites", "erros", "-erros-modo", "retrato", "-treino-db", filepath.Join(t.TempDir(), "x.db")}, &out, &errb)
	if code != exitUsage || !strings.Contains(out.String(), "-out é obrigatório") {
		t.Fatalf("code %d\n%s", code, out.String())
	}
	out.Reset()
	code = run([]string{"-suites", "erros", "-erros-modo", "retrato", "-out", t.TempDir(), "-treino-db", filepath.Join(t.TempDir(), "x.db")}, &out, &errb)
	if code != exitUsage || strings.Contains(out.String(), "-out é obrigatório") || !strings.Contains(out.String(), "treino.db ilegível") {
		t.Fatalf("with -out: code %d\n%s", code, out.String())
	}
}

func TestDialControlRefusesAPublicAddress(t *testing.T) { // checked on the connection, before any packet; no connection is made here
	local, remote := connControl(false), connControl(true)
	for addr, want := range map[string][2]bool{
		"127.0.0.1:11500": {true, true}, "[::1]:9": {true, true}, "10.1.2.3:11500": {false, true}, "192.168.0.5:443": {false, true},
		"8.8.8.8:53": {false, false}, "192.0.2.1:80": {false, false}, "169.254.169.254:80": {false, false}, "noport": {false, false},
	} {
		if got := local("tcp", addr, nil) == nil; got != want[0] {
			t.Errorf("loopback-only client, %s: allowed=%v, want %v", addr, got, want[0])
		}
		if got := remote("tcp", addr, nil) == nil; got != want[1] {
			t.Errorf("-gateway-remoto client, %s: allowed=%v, want %v", addr, got, want[1])
		}
	}
	if err := local("tcp", "8.8.8.8:53", nil); err == nil || !strings.Contains(err.Error(), "recusada") {
		t.Errorf("err = %v", err)
	}
	// the clients are built with that control
	rc, _ := newReplayClient("http://127.0.0.1:1", false, time.Second)
	rr, _ := newReplayClient("https://10.1.2.3:1", true, time.Second)
	for name, c := range map[string]*replayClient{"local": rc, "remote": rr} {
		if c.dialer == nil || c.dialer.Control == nil {
			t.Errorf("%s client has no dialer control", name)
		}
	}
}
