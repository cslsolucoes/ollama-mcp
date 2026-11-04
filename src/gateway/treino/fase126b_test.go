package treino

// Fase 12.6, correções da revisão independente: propriedades C-4.12 com operações intercaladas,
// divisão de grupo, linhas de /v1, aviso ao liberar, corte do detalhe.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// checkParts: every group (component of the links) has ONE side, and the exported lines of the
// two parts share no job, no document hash and no group (C-4.12).
func checkParts(t *testing.T, s *Store, step int) {
	t.Helper()
	var comps []component
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		comps, err = buildComponents(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range comps {
		sides := map[string]bool{}
		for _, m := range c.members {
			sides[m.particao] = true
		}
		if len(sides) > 1 {
			t.Fatalf("step %d: a group has several sides %v (members %d)", step, sides, len(c.members))
		}
	}
	got := map[string]map[string]map[string]bool{ParteTreino: {"job": {}, "doc": {}, "group": {}}, ParteTeste: {"job": {}, "doc": {}, "group": {}}}
	for _, parte := range []string{ParteTreino, ParteTeste} {
		for _, l := range exportLines(t, s, parte) {
			var line struct {
				Meta exportMeta `json:"meta"`
			}
			json.Unmarshal([]byte(l), &line)
			var doc string
			s.db.QueryRow(`SELECT COALESCE(doc_hash,'') FROM amostras WHERE job_id=?`, line.Meta.JobID).Scan(&doc)
			got[parte]["job"][line.Meta.JobID] = true
			if doc != "" {
				got[parte]["doc"][doc] = true
			}
			got[parte]["group"][gidOf(t, s, line.Meta.OcorrenciaID)] = true
		}
	}
	for kind := range got[ParteTreino] {
		for v := range got[ParteTreino][kind] {
			if got[ParteTeste][kind][v] {
				t.Fatalf("step %d: %s %s is in training and test", step, kind, v)
			}
		}
	}
}

func TestPartsStayDisjointUnderInterleavedOperations(t *testing.T) { // C-4.12
	rng := rand.New(rand.NewSource(7))
	s := openStore(t, Options{})
	k := testKey(t)
	f := newFake()
	const jobs = 300
	for i := 1; i <= jobs; i++ {
		f.add(i)
		if i%6 == 0 {
			j := f.jobs[mkID(i)]
			j.DocHash, j.DocKeyID = fmt.Sprintf("%064x", i%4), k.ID()
			f.jobs[mkID(i)] = j
		}
		if i > 260 {
			c := f.conts[mkID(i)]
			c.PromptJSON = pjOf(i - 250)
			f.conts[mkID(i)] = c
		}
	}
	var live []string
	quarantined, inherited := 0, 0
	for step := 1; step <= 900; step++ {
		switch op := rng.Intn(10); {
		case op < 4 || len(live) < 5: // a report, often linking to another job
			a := rng.Intn(jobs) + 1
			list := []JobRef{jr(a, "unico", "primario")}
			if b := rng.Intn(jobs) + 1; b != a && rng.Intn(3) == 0 {
				list = []JobRef{jr(b, "map1", "primario"), jr(a, "final", "primario")}
			}
			r := report(list...)
			r.Origem, r.Categoria, r.RespostaCorreta = "claude", "omitiu", "certa"
			id := ingest(t, s, r, "", f).OcorrenciaID
			live = append(live, id)
			if sideOf(t, s, id) == ParteQuarentena {
				quarantined++
			} else if sideOf(t, s, id) != "" {
				inherited++
			}
		case op < 7: // confirm
			s.Confirm(ctx, live[rng.Intn(len(live))], admin)
		case op < 8: // erase (may split groups)
			i := rng.Intn(len(live))
			s.Delete(ctx, live[i], admin)
			live = append(live[:i], live[i+1:]...)
		case op < 9: // discard
			s.Discard(ctx, live[rng.Intn(len(live))], admin)
		default: // unconfirm; a later confirm confirms again
			nv := fmt.Sprintf("ajuste %d", step)
			s.Update(ctx, live[rng.Intn(len(live))], Edit{RespostaCorreta: &nv}, admin)
		}
		if step%100 == 0 {
			checkParts(t, s, step)
		}
	}
	checkParts(t, s, 900)
	if quarantined == 0 || inherited == 0 {
		t.Errorf("the scenario did not exercise the rules: %d quarantined, %d inherited", quarantined, inherited)
	}
	t.Logf("reports that went to quarantine: %d, that inherited a side: %d", quarantined, inherited)
}

// The ids of the occurrences are random, and the split is only tricky when the id a part keeps
// is the id of a member of the other part: many rounds make every ordering likely.
func TestSplittingAGroupKeepsTheSideAndGivesDistinctIds(t *testing.T) {
	for i := 0; i < 30; i++ {
		splitOnce(t)
	}
}

func splitOnce(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	for i := 1; i <= 2; i++ {
		f.add(i)
	}
	a := newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, a, admin)) // the first group: the test
	br := report(jr(1, "map1", "primario"), jr(2, "final", "primario"))
	br.Origem, br.RespostaCorreta = "claude", "certa"
	b := ingest(t, s, br, "", f).OcorrenciaID // the bridge: job 1 with a, job 2 with c
	c := newEl(t, s, f, 2, "omitiu")
	if gidOf(t, s, a) != gidOf(t, s, b) || gidOf(t, s, a) != gidOf(t, s, c) {
		t.Fatalf("setup: one group expected: %q %q %q", gidOf(t, s, a), gidOf(t, s, b), gidOf(t, s, c))
	}
	side := sideOf(t, s, a)
	if side != ParteTeste || sideOf(t, s, c) != side {
		t.Fatalf("setup: side %q %q", side, sideOf(t, s, c))
	}
	must(t, s.Delete(ctx, b, admin)) // the bridge goes: two groups
	if sideOf(t, s, a) != side || sideOf(t, s, c) != side {
		t.Errorf("a part lost its side: %q %q", sideOf(t, s, a), sideOf(t, s, c))
	}
	if gidOf(t, s, a) == "" || gidOf(t, s, c) == "" || gidOf(t, s, a) == gidOf(t, s, c) {
		t.Errorf("the parts must have distinct group ids: %q %q", gidOf(t, s, a), gidOf(t, s, c))
	}
	if got := exportLines(t, s, ParteTeste); len(got) != 1 { // c is not confirmed; a is
		t.Errorf("export after the split: %d", len(got))
	}
}

// C-3.13, C-3.14: the OpenAI-compatible endpoints export like the native ones.
func TestExportOfTheV1Endpoints(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	id1 := f.add(1)
	j := f.jobs[id1]
	j.Endpoint = "/v1/chat/completions"
	f.jobs[id1] = j
	c := f.conts[id1]
	c.PromptJSON = []byte(`{"model":"qwen3-ia","messages":[{"role":"system","content":"S"},{"role":"user","content":"u1"},{"role":"assistant","content":"a1"},{"role":"user","content":"u2"}]}`)
	f.conts[id1] = c
	id2 := f.add(2)
	j = f.jobs[id2]
	j.Endpoint = "/v1/completions"
	f.jobs[id2] = j
	c = f.conts[id2]
	c.PromptJSON = []byte(`{"model":"qwen3-ia","prompt":"complete isto"}`)
	f.conts[id2] = c
	o1, o2 := newEl(t, s, f, 1, "omitiu"), newEl(t, s, f, 2, "inventou")
	must(t, s.Confirm(ctx, o1, admin))
	must(t, s.Confirm(ctx, o2, admin))
	lines := exportLines(t, s, ParteTeste)
	if len(lines) != 2 {
		t.Fatalf("%d lines: %v", len(lines), lines)
	}
	byOcc := map[string][]map[string]string{}
	for _, l := range lines {
		var line struct {
			Messages []map[string]string `json:"messages"`
			Meta     exportMeta          `json:"meta"`
		}
		if err := json.Unmarshal([]byte(l), &line); err != nil {
			t.Fatal(err)
		}
		byOcc[line.Meta.OcorrenciaID] = line.Messages
	}
	chat := byOcc[o1]
	if len(chat) != 5 || chat[0]["role"] != "system" || chat[3]["content"] != "u2" || chat[4]["role"] != "assistant" || chat[4]["content"] != "resposta certa 1" {
		t.Errorf("/v1/chat/completions: %v", chat)
	}
	comp := byOcc[o2]
	if len(comp) != 2 || comp[0]["role"] != "user" || comp[0]["content"] != "complete isto" || comp[1]["role"] != "assistant" || comp[1]["content"] != "resposta certa 2" {
		t.Errorf("/v1/completions: %v", comp)
	}
}

// M-1 (changed by C-4.11c, 1.1.6): releasing a quarantined component is REFUSED when a member was already exported to
// the other part, and nothing changes.
func TestReleaseIsRefusedForMembersExportedToTheOtherPart(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	twoGroups(t, s, f)             // a: test, b: training; both confirmed
	exportLines(t, s, ParteTeste)  // a was exported to the test part before the quarantine
	exportLines(t, s, ParteTreino) // and b to the training part
	x := report(jr(1, "map1", "primario"), jr(2, "final", "primario"))
	x.Origem = "claude"
	c := ingest(t, s, x, "", f).OcorrenciaID
	if sideOf(t, s, c) != ParteQuarentena {
		t.Fatal("setup")
	}
	info, err := s.ReleaseQuarantineInfo(ctx, c, ParteTreino, admin) // a (exported to the test) would go to the training
	if !errors.Is(err, ErrReleaseExported) || info.Membros != 3 || info.ExportadosNaOutraParte != 1 {
		t.Errorf("release to training: %+v %v", info, err)
	}
	if _, err = s.ReleaseQuarantineInfo(ctx, c, ParteTeste, admin); !errors.Is(err, ErrReleaseExported) { // b would go to the test
		t.Errorf("release to test: %v", err)
	}
	if sideOf(t, s, c) != ParteQuarentena {
		t.Errorf("a refused release changed the side: %q", sideOf(t, s, c))
	}
	// without any earlier export there is nothing to refuse
	s2 := openStore(t, Options{})
	f2 := newFake()
	twoGroups(t, s2, f2)
	c2 := ingest(t, s2, x, "", f2).OcorrenciaID
	info, err = s2.ReleaseQuarantineInfo(ctx, c2, ParteTeste, admin)
	if err != nil || info.ExportadosNaOutraParte != 0 {
		t.Errorf("no export before: %+v %v", info, err)
	}
}

// M-2: the detail cuts each text field and says how much was cut.
func TestDetailCutsLongTextsAndMarksIt(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	id := f.add(1)
	c := f.conts[id]
	c.Prompt = strings.Repeat("p", DetailTextMax+1000)
	c.Response = strings.Repeat("€", DetailTextMax) // 3 bytes each: the cut falls inside a character
	f.conts[id] = c
	o := newEl(t, s, f, 1, "omitiu")
	d, err := s.Detail(ctx, o)
	if err != nil || len(d.Amostras) != 1 {
		t.Fatal(err)
	}
	a := d.Amostras[0]
	if !a.PromptCortado || !strings.Contains(a.Prompt, fmt.Sprintf("[cortado (%d bytes)]", DetailTextMax+1000)) || len(a.Prompt) > DetailTextMax+100 || a.PromptBytes != DetailTextMax+1000 {
		t.Errorf("prompt: cut=%v bytes=%d len=%d", a.PromptCortado, a.PromptBytes, len(a.Prompt))
	}
	if !a.RespCortada || !strings.Contains(a.Resposta, "[cortado (") || a.RespBytes != 3*DetailTextMax {
		t.Errorf("resposta: cut=%v bytes=%d", a.RespCortada, a.RespBytes)
	}
	body := a.Resposta[:strings.Index(a.Resposta, "\n[cortado")]
	if strings.ContainsRune(body, '�') {
		t.Error("the cut split a character")
	}
	// a short one is shown whole, with no mark
	f.add(2)
	o2 := newEl(t, s, f, 2, "omitiu")
	d2, _ := s.Detail(ctx, o2)
	if d2.Amostras[0].PromptCortado || strings.Contains(d2.Amostras[0].Prompt, "cortado") {
		t.Error("a short text was marked as cut")
	}
	if t2, cut := cutText(strings.Repeat("x", DetailTextMax+5)); !cut || !strings.Contains(t2, "cortado") {
		t.Error("cutText")
	}
}

// ExportUntil: the deadline is checked between the lines and stops the export with its own error.
func TestExportUntilStopsAtTheDeadline(t *testing.T) {
	s := openStore(t, Options{})
	seedExportable(t, s, 5)
	var b bytes.Buffer
	n, err := s.ExportUntil(context.Background(), ParteTreino, admin, &b, nil, time.Now().Add(-time.Second))
	if !errors.Is(err, ErrExportDeadline) || n != 0 || b.Len() != 0 {
		t.Errorf("past deadline: %d lines, %v", n, err)
	}
	b.Reset()
	n, err = s.ExportUntil(context.Background(), ParteTreino, admin, &b, nil, time.Now().Add(time.Minute))
	if err != nil || n != 5 {
		t.Errorf("future deadline: %d lines, %v", n, err)
	}
	assertAllDeliveredRecorded(t, s, b.Bytes(), 5)
}
