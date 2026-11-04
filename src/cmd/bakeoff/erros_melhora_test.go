package main

// Stage 12.8 in the bake-off: the candidate is the current model plus the frozen
// instructions and examples, built by the code the gateway uses (C-8.4, C-8.5, C-8.7,
// C-8.10). Synthetic treino.db and a fake gateway, nothing else.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/melhora/feed"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

const instrText = "USE A NUMERAÇÃO DO CORPO DO DOCUMENTO"

var indiceRe = regexp.MustCompile(`INDICE id=([0-9a-f]{32}) hash=([0-9a-f]{64}) itens=(\d+) k=(\d+) limiar=([0-9.]+) modelo=(\S+)`)

// exampleSet is the test cases plus the public training examples: 101 shares the document
// of case 1, 102 shares the group of case 2, 103 and 104 are clean.
func exampleSet() []synthCase {
	cs := syntheticSet()
	for n := 101; n <= 104; n++ {
		c := omitiu(n)
		c.Particao = "treino"
		cs = append(cs, c)
	}
	return cs
}

func (f *fixture) sqlExec(q string, args ...any) {
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

// seedExamples marks 101..104 as public, with an embedding, and links 101 to case 1 by the
// document and 102 to case 2 by the group.
func (f *fixture) seedExamples() {
	f.t.Helper()
	for n := 101; n <= 104; n++ {
		f.sqlExec(`UPDATE ocorrencias SET exemplo_publico=1 WHERE id=?`, synthID(n))
		f.sqlExec(`INSERT INTO embeddings(ocorrencia_id,vetor,modelo) VALUES(?,?,?)`, synthID(n), treino.EncodeVec([]float32{1, float32(n-100) / 100, 0}), melhora.DefaultEmbedModel)
	}
	f.sqlExec(`UPDATE ocorrencias SET doc_hash='DOC-1', doc_key_id='kid' WHERE id IN (?,?)`, synthID(1), synthID(101))
	f.sqlExec(`UPDATE ocorrencias SET grupo_id=? WHERE id=?`, synthID(2), synthID(102))
}

func (f *fixture) freezeIndex(k int, limiar float64) (id, hash string) {
	f.t.Helper()
	eo := f.opts("indice")
	eo.ExemplosK, eo.ExemplosLimiar = k, limiar
	code, out := f.run(eo)
	m := indiceRe.FindStringSubmatch(out)
	if code != 0 || m == nil {
		f.t.Fatalf("indice: code %d\n%s", code, out)
	}
	return m[1], m[2]
}

func (f *fixture) instrFile(content string) string {
	f.t.Helper()
	p := filepath.Join(f.dir, fmt.Sprintf("instr-%d.json", len(f.logs)))
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return p
}

var instrJSON = `{"tools":{"extract":"` + instrText + `"}}`

// retratoWith makes a retrato that records the index and the instructions.
func (f *fixture) retratoWith(idx, instr string) (rid string, out string) {
	f.t.Helper()
	ro := f.opts("retrato")
	ro.IndiceID, ro.InstrFile = idx, instr
	code, out := f.run(ro)
	m := retratoIDRe.FindStringSubmatch(out)
	if code != 0 || m == nil {
		f.t.Fatalf("retrato: code %d\n%s", code, out)
	}
	return m[1], out
}

func sysText(r gwReq) string {
	var s string
	_ = json.Unmarshal(r.Body["system"], &s)
	return s
}

func TestIndexModeFreezesAndReportsIdHashKAndFloor(t *testing.T) { // C-8.5, C-8.10
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	id, hash := f.freezeIndex(2, 0.7)
	st, err := treino.Open(f.db, treino.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m, err := st.GetIndexMeta(ctx0, id)
	if err != nil || m.Hash != hash || m.K != 2 || m.Limiar != 0.7 || m.Itens != 4 || !m.Valido || m.Modelo != melhora.DefaultEmbedModel {
		t.Fatalf("%+v %v", m, err)
	}
	// the defaults are the provisional ones
	id2, _ := f.freezeIndex(0, 0)
	m2, _ := st.GetIndexMeta(ctx0, id2)
	if m2.K != defaultExemplosK || m2.Limiar != defaultExemplosLimiar {
		t.Errorf("defaults: %+v", m2)
	}
	// invalid values
	for _, bad := range [][2]float64{{11, 0.8}, {3, 1.5}, {3, -0.1}} {
		eo := f.opts("indice")
		eo.ExemplosK, eo.ExemplosLimiar = int(bad[0]), bad[1]
		if code, _ := f.run(eo); code != exitUsage {
			t.Errorf("%v accepted (code %d)", bad, code)
		}
	}
	// no example: the gate-style failure, nothing created
	g := newFixture(t, syntheticSet(), scenarioPolicy(false))
	if code, out := g.run(g.opts("indice")); code != exitGate || !strings.Contains(out, "DADOS INSUFICIENTES") {
		t.Errorf("no examples: code %d\n%s", code, out)
	}
}

func TestRetratoRecordsTheHashesFromTheIndexAndTheInstructionsFile(t *testing.T) { // C-4.13, C-8.5, C-8.7, C-8.10
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, hash := f.freezeIndex(3, 0.8)
	instr := f.instrFile(instrJSON)
	rid, out := f.retratoWith(idx, instr)
	in, _ := melhora.LoadInstrucoes(instr)
	st, _ := treino.Open(f.db, treino.Options{})
	defer st.Close()
	rt, err := st.GetRetrato(ctx0, rid)
	if err != nil || rt.IndiceHash != hash || rt.InstrucoesID != in.ID || rt.InstrucoesHash != in.Hash || rt.ExemplosK != 3 || rt.ExemplosLimiar != 0.8 {
		t.Fatalf("%+v %v", rt, err)
	}
	if !strings.Contains(out, "exemplos_k=3") || !strings.Contains(out, "exemplos_limiar=0.8000") {
		t.Errorf("the log lacks k and the floor:\n%s", out)
	}
	// bad inputs are refused and leave no retrato
	n := func() int {
		var c int
		db, _ := sql.Open("sqlite", f.db)
		defer db.Close()
		db.QueryRow(`SELECT COUNT(*) FROM retratos`).Scan(&c)
		return c
	}
	before := n()
	for name, mod := range map[string]func(*errosOpts){
		"unknown index": func(o *errosOpts) { o.IndiceID = synthID(999) },
		"bad index id":  func(o *errosOpts) { o.IndiceID = "../x" },
		"missing file":  func(o *errosOpts) { o.IndiceID, o.InstrFile = "", filepath.Join(f.dir, "nao-existe.json") },
		"unknown tool":  func(o *errosOpts) { o.IndiceID, o.InstrFile = "", f.instrFile(`{"tools":{"outra":"x"}}`) },
		"invalid json":  func(o *errosOpts) { o.IndiceID, o.InstrFile = "", f.instrFile(`{`) },
	} {
		ro := f.opts("retrato")
		ro.IndiceID, ro.InstrFile = idx, instr
		mod(&ro)
		if code, _ := f.run(ro); code != exitUsage {
			t.Errorf("%s: code %d", name, code)
		}
	}
	if n() != before {
		t.Error("a refused retrato was left behind")
	}
}

func TestCandidateGetsTheInstructionsAndTheExamplesAndTheCurrentModelGetsTheRecordedRequest(t *testing.T) { // C-5.1, C-8.4
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(4, 0.5)
	rid, _ := f.retratoWith(idx, f.instrFile(instrJSON))
	f.gw.reset()
	ce := f.compareOpts(rid, "omitiu")
	ce.IndiceID, ce.InstrFile = idx, f.instrFile(instrJSON)
	code, out := f.run(ce)
	if code != exitOK && code != exitPending && code != exitRejected {
		t.Fatalf("code %d\n%s", code, out)
	}
	reqs := f.gw.requests()
	var gen, embeds int
	seenWithExample := map[string]bool{}
	for _, r := range reqs {
		if r.Path == "/api/embed" {
			continue
		}
		gen++
		if r.Model == "qwen3-ia" {
			// the current model: the recorded request, only the seed (no system added)
			if _, has := r.Body["system"]; has {
				t.Errorf("the current model got a system text: %+v", r.Key)
			}
			continue
		}
		sys := sysText(r)
		if !strings.Contains(sys, instrText) {
			t.Fatalf("the candidate got no instruction (case %s): %q", r.Key, sys)
		}
		if strings.Contains(sys, "PEDIDO-"+r.Key) {
			t.Errorf("the case %s is its own example (C-8.4)", r.Key)
		}
		// C-8.4: the document of case 1 and the group of case 2 are out of their own searches
		if r.Key == synthID(1) && strings.Contains(sys, "PEDIDO-"+synthID(101)) {
			t.Errorf("an example of the SAME DOCUMENT was shown for case 1")
		}
		if r.Key == synthID(2) && strings.Contains(sys, "PEDIDO-"+synthID(102)) {
			t.Errorf("an example of the SAME GROUP was shown for case 2")
		}
		for _, ex := range []int{103, 104} {
			if !strings.Contains(sys, "PEDIDO-"+synthID(ex)) {
				t.Errorf("case %s lacks the clean example %d", r.Key, ex)
			}
		}
		if r.Key == synthID(3) && strings.Contains(sys, "PEDIDO-"+synthID(101)) && strings.Contains(sys, "PEDIDO-"+synthID(102)) {
			seenWithExample["both"] = true
		}
		// the user text of the recorded request is untouched
		var prompt string
		_ = json.Unmarshal(r.Body["prompt"], &prompt)
		if !strings.HasPrefix(prompt, "PEDIDO-"+r.Key) {
			t.Errorf("the prompt changed: %q", prompt)
		}
	}
	if !seenWithExample["both"] {
		t.Error("a case with no link to 101/102 must get both of them")
	}
	for _, e := range f.gw.embeds {
		embeds++
		if e.Tool != replayTool || e.Model != melhora.DefaultEmbedModel {
			t.Errorf("embed call: %+v", e)
		}
		if strings.Contains(e.Text, instrText) || strings.Contains(e.Text, "EXEMPLOS_INICIO") {
			t.Errorf("the query was made from the improved request: %q", e.Text)
		}
	}
	if gen == 0 || embeds == 0 {
		t.Fatalf("calls: %d generations, %d embeddings", gen, embeds)
	}
	// the three rounds of a case use one embedding (a memo): no text repeats
	seen := map[string]bool{}
	for _, e := range f.gw.embeds {
		if seen[e.Text] {
			t.Errorf("the same query was embedded twice")
		}
		seen[e.Text] = true
	}
}

func TestInstructionsAloneAreAppliedWithoutAnyEmbedding(t *testing.T) { // v0 index "nenhum", instructions only
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid, _ := f.retratoWith("", f.instrFile(instrJSON))
	f.gw.reset()
	ce := f.compareOpts(rid, "omitiu")
	ce.InstrFile = f.instrFile(instrJSON)
	f.run(ce)
	if len(f.gw.embeds) != 0 {
		t.Errorf("%d embeddings without an index", len(f.gw.embeds))
	}
	n := 0
	for _, r := range f.gw.requests() {
		if r.Model != "qwen3-ia" {
			n++
			if sysText(r) != instrText {
				t.Errorf("system = %q", sysText(r))
			}
		} else if _, has := r.Body["system"]; has {
			t.Error("the current model got a system text")
		}
	}
	if n == 0 {
		t.Fatal("no candidate call")
	}
}

func TestEmptyInstructionsV0AndNoIndexGiveTheRecordedRequest(t *testing.T) { // identity
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	v0 := f.instrFile(`{"tools":{}}`)
	rid, _ := f.retratoWith("", v0)
	f.gw.reset()
	ce := f.compareOpts(rid, "omitiu")
	ce.InstrFile = v0
	f.run(ce)
	for _, r := range f.gw.requests() {
		if _, has := r.Body["system"]; has {
			t.Fatalf("v0 added a system text: %+v", r.Key)
		}
	}
	if len(f.gw.embeds) != 0 || len(f.gw.requests()) == 0 {
		t.Error("calls")
	}
}

func TestComparisonChecksWhatWasGivenAgainstTheRetrato(t *testing.T) { // C-8.5, C-8.7
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(3, 0.8)
	instr := f.instrFile(instrJSON)
	rid, _ := f.retratoWith(idx, instr)
	idx2, _ := f.freezeIndex(2, 0.8) // another index: another hash and another k
	otherInstr := f.instrFile(`{"tools":{"extract":"OUTRA INSTRUCAO"}}`)
	bare, _ := f.retratoWith("", "") // a retrato that records nothing

	cases := map[string]struct {
		rid, idx, instr string
		want            string
	}{
		"no index given":           {rid, "", instr, "passe -indice"},
		"no instructions given":    {rid, idx, "", "passe -instrucoes"},
		"another index":            {rid, idx2, instr, "o hash difere"},
		"another instructions":     {rid, idx, otherInstr, "o ID/hash diferem"},
		"unknown index":            {rid, synthID(77), instr, "inexistente"},
		"bad index id":             {rid, "x", instr, "-indice inválido"},
		"index for a bare retrato": {bare, idx, "", "não registra índice"},
		"instr for a bare retrato": {bare, "", instr, "não registra versão"},
	}
	for name, c := range cases {
		f.gw.reset()
		ce := f.compareOpts(c.rid, "omitiu")
		ce.IndiceID, ce.InstrFile = c.idx, c.instr
		code, out := f.run(ce)
		if code != exitUsage || !strings.Contains(out, c.want) {
			t.Errorf("%s: code %d (want %d), missing %q\n%s", name, code, exitUsage, c.want, out)
		}
		if n := len(f.gw.requests()) + len(f.gw.embeds); n != 0 {
			t.Errorf("%s: %d calls were made", name, n)
		}
	}
	// the right ones are accepted
	f.gw.reset()
	ce := f.compareOpts(rid, "omitiu")
	ce.IndiceID, ce.InstrFile = idx, instr
	if code, out := f.run(ce); code == exitUsage {
		t.Errorf("the right index and instructions were refused:\n%s", out)
	}
}

func TestComparisonRefusesAnIndexWhoseMemberStoppedBeingAnExample(t *testing.T) { // C-4.10, C-8.9
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(3, 0.8)
	rid, _ := f.retratoWith(idx, "")
	st, err := treino.Open(f.db, treino.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetExemploPublico(ctx0, synthID(103), false, treino.AdminActor("x")); err != nil {
		t.Fatal(err)
	}
	st.Close()
	f.gw.reset()
	ce := f.compareOpts(rid, "omitiu")
	ce.IndiceID = idx
	code, out := f.run(ce)
	if code != exitUsage || !strings.Contains(out, "inválido") {
		t.Errorf("code %d\n%s", code, out)
	}
	if len(f.gw.requests())+len(f.gw.embeds) != 0 {
		t.Error("calls with an invalid index")
	}
}

func TestAnIndexInvalidatedDuringTheRoundMakesTheGateFail(t *testing.T) { // C-4.10
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(3, 0.8)
	rid, _ := f.retratoWith(idx, "")
	done := false
	f.gw.onReq = func(n int) {
		if n == 20 && !done {
			done = true
			st, err := treino.Open(f.db, treino.Options{})
			if err != nil {
				t.Error(err)
				return
			}
			defer st.Close()
			if err := st.SetExemploPublico(ctx0, synthID(104), false, treino.AdminActor("x")); err != nil {
				t.Error(err)
			}
		}
	}
	ce := f.compareOpts(rid, "omitiu")
	ce.IndiceID = idx
	code, out := f.run(ce)
	if !done {
		t.Fatal("setup: the hook never ran")
	}
	if code != exitGate || !strings.Contains(out, "foi invalidado durante a rodada") {
		t.Errorf("code %d\n%s", code, out)
	}
}

func TestAFailedSearchLeavesTheCaseUnmeasuredNeverMeasuredWithoutExamples(t *testing.T) {
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(3, 0.8)
	rid, _ := f.retratoWith(idx, "")
	f.gw.embedStatus = 500
	f.gw.reset()
	ce := f.compareOpts(rid, "omitiu")
	ce.IndiceID = idx
	code, out := f.run(ce)
	if code != exitGate || !strings.Contains(out, "não medido") || !strings.Contains(out, "a busca de exemplos falhou") {
		t.Errorf("code %d\n%s", code, out)
	}
	for _, r := range f.gw.requests() {
		if r.Model != "qwen3-ia" {
			t.Fatalf("a candidate call went without its examples: %+v", r.Key)
		}
	}
}

func TestRequestTransformIsTheSharedMelhoraCode(t *testing.T) { // SSOT: the bake-off and the gateway use melhora.Apply
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(2, 0.5)
	instr := f.instrFile(instrJSON)
	rid, _ := f.retratoWith(idx, instr)
	st, err := treino.Open(f.db, treino.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rt, _ := st.GetRetrato(ctx0, rid)
	rc, err := newReplayClient(f.gw.srv.URL, false, 5e9)
	if err != nil {
		t.Fatal(err)
	}
	eo := f.opts("comparar")
	eo.IndiceID, eo.InstrFile = idx, instr
	tr, _, err := prepareCandidate(ctx0, st, eo, rt, rc)
	if err != nil || tr == nil {
		t.Fatalf("%v", err)
	}
	_, cases, _ := st.RetratoCases(ctx0, rid)
	c := cases[len(cases)-1]
	out, err := tr.Transform(c, c.PromptJSON)
	if err != nil {
		t.Fatal(err)
	}
	// the very bytes melhora.Apply makes for the same input
	in, _ := melhora.LoadInstrucoes(instr)
	ix := mustLoadIndex(t, st, idx)
	meta, _ := st.CaseMeta(ctx0, c.OcorrenciaID)
	exs := ix.Search(c.Tool, []float32{1, 0, 0}, ix.K, ix.Limiar, melhora.Exclusion{Ocorrencias: []string{c.OcorrenciaID}, Grupos: []string{meta.GrupoID}, Docs: []string{melhora.DocKey(meta.DocKeyID, meta.DocHash)}})
	want, changed, err := melhora.Apply(c.Endpoint, c.PromptJSON, in.For(c.Tool), exs, nonceOf(t, out))
	if err != nil || !changed || string(out) != string(want) {
		t.Errorf("the bake-off's request differs from melhora.Apply's:\n%s\n%s", out, want)
	}
}

var ctx0 = context.Background()

func mustLoadIndex(t *testing.T, st *treino.Store, id string) *melhora.Index {
	t.Helper()
	ix, err := feed.LoadIndex(ctx0, st, id)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

// C-8.4 defence: even if the case itself were in the index (it cannot be: the index holds
// the training part and the case is in the test part), it is never shown as its own example.
func TestTransformExcludesTheCaseItsGroupAndItsDocumentByTheCasesOwnMeta(t *testing.T) {
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.sqlExec(`UPDATE ocorrencias SET doc_hash='DOC-X', doc_key_id='kid', grupo_id='GRUPO-X' WHERE id=?`, synthID(5))
	st, err := treino.Open(f.db, treino.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	items := []melhora.Item{
		{OcorrenciaID: synthID(5), Tool: "extract", Vec: []float32{1, 0, 0}, Example: melhora.Example{Pedido: "P-SELF", Errada: "E", Certa: "C"}},
		{OcorrenciaID: "g", Tool: "extract", GrupoID: "GRUPO-X", Vec: []float32{1, 0, 0}, Example: melhora.Example{Pedido: "P-GROUP", Errada: "E", Certa: "C"}},
		{OcorrenciaID: "d", Tool: "extract", DocKey: "kid/DOC-X", Vec: []float32{1, 0, 0}, Example: melhora.Example{Pedido: "P-DOC", Errada: "E", Certa: "C"}},
		{OcorrenciaID: "ok", Tool: "extract", Vec: []float32{1, 0, 0}, Example: melhora.Example{Pedido: "P-OK", Errada: "E", Certa: "C"}},
	}
	tr := melhora.NewTransformer(melhora.Config{Tools: melhora.Tools, Embed: func(context.Context, string) ([]float32, error) { return []float32{1, 0, 0}, nil }})
	tr.SetIndex(melhora.NewIndex("i", "h", "m", 10, 0.1, items))
	_, cases, err := st.RetratoCases(ctx0, f.makeRetrato())
	if err != nil {
		t.Fatal(err)
	}
	var c treino.RetratoCase
	for _, x := range cases {
		if x.OcorrenciaID == synthID(5) {
			c = x
		}
	}
	out, err := melhoraTransform{t: tr, st: st, ctx: ctx0}.Transform(c, c.PromptJSON)
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ System string }
	json.Unmarshal(out, &m)
	for _, bad := range []string{"P-SELF", "P-GROUP", "P-DOC"} {
		if strings.Contains(m.System, bad) {
			t.Errorf("%s was shown for the case it belongs with", bad)
		}
	}
	if !strings.Contains(m.System, "P-OK") {
		t.Errorf("the clean example is missing: %q", m.System)
	}
}

func TestComparisonRefusesWhenKOrTheFloorOfTheRetratoDiffer(t *testing.T) { // C-8.10
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(3, 0.8)
	rid, _ := f.retratoWith(idx, "")
	f.sqlExec(`UPDATE retratos SET exemplos_k=5 WHERE id=?`, rid) // the table was changed behind our back
	f.gw.reset()
	ce := f.compareOpts(rid, "omitiu")
	ce.IndiceID = idx
	code, out := f.run(ce)
	if code != exitUsage || !strings.Contains(out, "k ou o limiar do índice diferem") {
		t.Errorf("code %d\n%s", code, out)
	}
	if len(f.gw.requests())+len(f.gw.embeds) != 0 {
		t.Error("calls were made")
	}
}

func TestRetratoWithAnInvalidIndexIsRefused(t *testing.T) { // C-4.10
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(3, 0.8)
	st, err := treino.Open(f.db, treino.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetExemploPublico(ctx0, synthID(101), false, treino.AdminActor("x")); err != nil {
		t.Fatal(err)
	}
	st.Close()
	ro := f.opts("retrato")
	ro.IndiceID = idx
	code, out := f.run(ro)
	if code != exitUsage || !strings.Contains(out, "inválido") {
		t.Errorf("code %d\n%s", code, out)
	}
}

var nonceRe = regexp.MustCompile(`EXEMPLOS_INICIO ([0-9a-f]{16})`)

// nonceOf reads the nonce of the block of examples (new for every request, C-8.11).
func nonceOf(t *testing.T, body []byte) string {
	t.Helper()
	m := nonceRe.FindSubmatch(body)
	if m == nil {
		t.Fatalf("no block of examples in %s", body)
	}
	return string(m[1])
}

func TestAJobChangedByTheImprovementIsNotInTheRetrato(t *testing.T) { // C-3.10a
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	var rid1 = f.makeRetrato()
	st, _ := treino.Open(f.db, treino.Options{})
	_, before, _ := st.RetratoCases(ctx0, rid1)
	st.Close()
	// the only change: the sample of case 5 was made by a request with instructions added
	f.sqlExec(`UPDATE amostras SET melhora=1, melhora_instrucoes_id='iabc' WHERE job_id=?`, synthJob(5))
	rid2 := f.makeRetrato()
	st, _ = treino.Open(f.db, treino.Options{})
	defer st.Close()
	_, after, err := st.RetratoCases(ctx0, rid2)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)-1 {
		t.Fatalf("%d cases before, %d after", len(before), len(after))
	}
	for _, c := range after {
		if c.OcorrenciaID == synthID(5) {
			t.Error("a job changed by the improvement is in the retrato")
		}
	}
	// the old retrato no longer reads: its case 5 is not eligible any more
	if _, _, err := st.RetratoCases(ctx0, rid1); err == nil {
		t.Error("a retrato that lists a job changed by the improvement is still used")
	}
}

func TestCleanupModeDeletesTheInvalidIndexes(t *testing.T) { // M-4
	f := newFixture(t, exampleSet(), scenarioPolicy(false))
	f.seedExamples()
	idx, _ := f.freezeIndex(3, 0.8)
	st, err := treino.Open(f.db, treino.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetExemploPublico(ctx0, synthID(103), false, treino.AdminActor("x")); err != nil {
		t.Fatal(err)
	}
	st.Close()
	code, out := f.run(f.opts("indices-limpar"))
	if code != exitOK || !strings.Contains(out, "removidos=1") {
		t.Fatalf("code %d\n%s", code, out)
	}
	st, _ = treino.Open(f.db, treino.Options{})
	defer st.Close()
	if _, err := st.GetIndexMeta(ctx0, idx); err == nil {
		t.Error("the invalid index is still there")
	}
	if code, out := f.run(f.opts("indices-limpar")); code != exitOK || !strings.Contains(out, "removidos=0") {
		t.Errorf("second cleanup: %d\n%s", code, out)
	}
}
