package treino

// Fase 12.8 (review): C-3.10a, the copy of the trace (C-2.9, C-2.10a), the copies of an unmarked example
// (I-3), the failed pending rows (M-2), the version of the format (M-3) and the cleanup of invalid
// indexes (M-4).

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
)

func TestAJobChangedByTheImprovementIsNeverExportedNorFrozenInARetrato(t *testing.T) { // C-3.10a, C-3.16
	s := openStore(t, Options{})
	f := newFake()
	a := confirmedEl(t, s, f, 1, "omitiu")
	b := confirmedEl(t, s, f, 2, "omitiu")
	if sideOf(t, s, a) != ParteTeste || sideOf(t, s, b) != ParteTeste {
		t.Fatal("setup")
	}
	if ok, err := s.eligibleOnce(ctx, s.db, a); err != nil || !ok {
		t.Fatalf("setup: the case is not eligible before the trace: %v %v", ok, err)
	}
	if got := exportLines(t, s, ParteTeste); len(got) != 2 {
		t.Fatalf("setup: %d lines", len(got))
	}
	// the ONLY difference: the sample of b says the improvement changed its request
	if _, err := s.db.Exec(`UPDATE amostras SET melhora=1, melhora_instrucoes_id='iabc' WHERE job_id=?`, mkID(2)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.eligibleOnce(ctx, s.db, b); ok {
		t.Error("a job changed by the improvement passes the export predicate")
	}
	if ok, _ := s.eligibleOnce(ctx, s.db, a); !ok {
		t.Error("the unchanged one stopped being eligible")
	}
	if got := exportLines(t, s, ParteTeste); len(got) != 1 || !strings.Contains(got[0], b[:0]) || strings.Contains(got[0], `"ocorrencia_id":"`+b+`"`) {
		t.Errorf("export: %v", got)
	}
	rt, err := s.CreateRetrato(ctx, RetratoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ids := itemIDs(rt); len(ids) != 1 || ids[0] != a {
		t.Errorf("the retrato lists %v, want only %s", ids, a)
	}
	// the predicate reports it by name
	r, err := scanEl(s.db.QueryRowContext(ctx, elSelect+`WHERE o.id=?`, b))
	if err != nil {
		t.Fatal(err)
	}
	if ok, why := Eligible(r, false); ok || why != "melhora" || !r.Melhora {
		t.Errorf("%v %q", ok, why)
	}
	// it still counts in the statistics (the occurrence is there)
	if sm, _ := s.Summary(ctx); sm.Total == 0 {
		t.Error("summary lost the occurrence")
	}
}

func TestIngestCopiesTheTraceToTheSample(t *testing.T) { // C-2.9, C-2.10a
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	c := f.conts[mkID(1)]
	c.PromptJSONOriginal, c.PromptJSONOriginalTruncated = []byte(`{"antes":1}`), true
	c.MelhoraInstrucoesID, c.MelhoraIndiceID, c.MelhoraExemplos = "iabc", "idx9", "o1,o2"
	f.conts[mkID(1)] = c
	f.add(2) // a job the improvement did not change
	r1 := report(jr(1, "unico", "primario"))
	r1.Categoria, r1.RespostaCorreta = "omitiu", "certa 1"
	id1 := ingest(t, s, r1, "", f).OcorrenciaID
	id2 := ingest(t, s, report(jr(2, "unico", "primario")), "", f).OcorrenciaID
	_ = id2
	var orig []byte
	var otr, mel int
	var ii, xi, ex string
	if err := s.db.QueryRow(`SELECT prompt_json_original, prompt_json_original_truncado, melhora, COALESCE(melhora_instrucoes_id,''), COALESCE(melhora_indice_id,''), COALESCE(melhora_exemplos,'') FROM amostras WHERE job_id=?`, mkID(1)).
		Scan(&orig, &otr, &mel, &ii, &xi, &ex); err != nil {
		t.Fatal(err)
	}
	if string(orig) != `{"antes":1}` || otr != 1 || mel != 1 || ii != "iabc" || xi != "idx9" || ex != "o1,o2" {
		t.Errorf("sample 1: %q %d %d %q %q %q", orig, otr, mel, ii, xi, ex)
	}
	if err := s.db.QueryRow(`SELECT prompt_json_original, prompt_json_original_truncado, melhora, COALESCE(melhora_instrucoes_id,''), COALESCE(melhora_indice_id,''), COALESCE(melhora_exemplos,'') FROM amostras WHERE job_id=?`, mkID(2)).
		Scan(&orig, &otr, &mel, &ii, &xi, &ex); err != nil {
		t.Fatal(err)
	}
	if orig != nil || otr != 0 || mel != 0 || ii != "" || xi != "" || ex != "" {
		t.Errorf("sample 2 has a trace: %q %d %d %q %q %q", orig, otr, mel, ii, xi, ex)
	}
	// and the one with the trace is out of the predicate once confirmed
	must(t, s.Confirm(ctx, id1, admin))
	if ok, _ := s.eligibleOnce(ctx, s.db, id1); ok {
		t.Error("the sample copied with a trace is exportable")
	}
}

func TestUnmarkingDeletesTheCopiesInTheFrozenIndexesAndInvalidatesThem(t *testing.T) { // C-6.6, I-3
	var l invLog
	s := openStore(t, Options{OnIndexInvalidated: l.add})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	publicEx(t, s, f, 2, "omitiu")
	m := freeze(t, s)
	must(t, s.SetExemploPublico(ctx, a, false, admin))
	if idxValid(t, s, m.ID) || !strings.Contains(l.got(), m.ID) {
		t.Errorf("the index is still valid after the unmark (told %q)", l.got())
	}
	var blob []byte
	if err := s.db.QueryRow(`SELECT vetor FROM indice_itens WHERE indice_id=? AND ocorrencia_id=?`, m.ID, a).Scan(&blob); err != nil || len(blob) != 0 {
		t.Errorf("the copy of the unmarked example is still in the index: %v, %d bytes", err, len(blob))
	}
	if n := count(t, s, "embeddings"); n != 1 {
		t.Errorf("%d embedding rows, want only the other example's", n)
	}
}

func TestFailedPendingRowsLeaveTheQueueForGood(t *testing.T) { // M-2
	s := openStore(t, Options{})
	f := newFake()
	var ids []string
	for i := 1; i <= 3; i++ {
		id := newEl(t, s, f, i, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		must(t, s.SetExemploPublico(ctx, id, true, admin))
		ids = append(ids, id)
	}
	pend, _ := s.PendingEmbeddings(ctx, 10)
	if len(pend) != 3 {
		t.Fatalf("%d pending", len(pend))
	}
	must(t, s.MarkEmbeddingFailed(ctx, pend[0].OcorrenciaID, pend[0].Version, "sem_pedido"))
	after, _ := s.PendingEmbeddings(ctx, 10)
	if len(after) != 2 || after[0].OcorrenciaID == pend[0].OcorrenciaID {
		t.Errorf("the failed row is still pending: %v", after)
	}
	// the cursor: the rows after a rowid
	rest, _ := s.PendingEmbeddingsAfter(ctx, after[0].Rowid, 10)
	if len(rest) != 1 || rest[0].OcorrenciaID != after[1].OcorrenciaID {
		t.Errorf("cursor: %v", rest)
	}
	// a stale version does not mark (the occurrence changed: it deserves a fresh try)
	must(t, s.MarkEmbeddingFailed(ctx, after[0].OcorrenciaID, after[0].Version+999, "sem_pedido"))
	if again, _ := s.PendingEmbeddings(ctx, 10); len(again) != 2 {
		t.Errorf("a stale mark removed a row: %v", again)
	}
	// a failed row is not in a new index either, and marking it public again makes a fresh row
	must(t, s.SetExemploPublico(ctx, pend[0].OcorrenciaID, false, admin))
	must(t, s.SetExemploPublico(ctx, pend[0].OcorrenciaID, true, admin))
	if again, _ := s.PendingEmbeddings(ctx, 10); len(again) != 3 {
		t.Errorf("the unmark/mark did not make a fresh row: %v", again)
	}
	_ = ids
}

func TestTheFormatVersionIsPartOfTheHash(t *testing.T) { // C-8.10a, M-3
	s := openStore(t, Options{})
	f := newFake()
	publicEx(t, s, f, 1, "omitiu")
	a, err := s.FreezeIndex(ctx, FreezeOptions{K: 3, Limiar: 0.8, Modelo: testModel, Formato: "ex1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.FreezeIndex(ctx, FreezeOptions{K: 3, Limiar: 0.8, Modelo: testModel, Formato: "ex2"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Hash == b.Hash || a.Formato != "ex1" || b.Formato != "ex2" {
		t.Errorf("the format does not change the hash: %+v %+v", a, b)
	}
	if IndexHash("m", "ex1", 3, 0.8, nil) == IndexHash("m", "ex2", 3, 0.8, nil) {
		t.Error("IndexHash ignores the format")
	}
	d, err := s.LoadIndex(ctx, b.ID)
	if err != nil || d.Formato != "ex2" || d.Hash != b.Hash {
		t.Errorf("LoadIndex: %+v %v", d.IndexMeta, err)
	}
	if _, err := s.FreezeIndex(ctx, FreezeOptions{K: 3, Limiar: 0.8, Modelo: testModel}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a freeze without a format: %v", err)
	}
}

func TestInvalidIndexesAreDeletedAtTheNextFreezeAndByTheCleanup(t *testing.T) { // M-4
	s := openStore(t, Options{})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	publicEx(t, s, f, 2, "omitiu")
	old := freeze(t, s)
	must(t, s.SetExemploPublico(ctx, a, false, admin))
	if idxValid(t, s, old.ID) {
		t.Fatal("setup")
	}
	fresh := freeze(t, s) // the freeze cleans up the invalid ones
	if _, err := s.GetIndexMeta(ctx, old.ID); !errors.Is(err, ErrIndexNotFound) {
		t.Errorf("the invalid index survived the next freeze: %v", err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM indice_itens WHERE indice_id=?`, old.ID).Scan(&n)
	if n != 0 || !idxValid(t, s, fresh.ID) || len(memberIDs(t, s, fresh.ID)) != 1 {
		t.Errorf("members of the deleted index: %d; fresh valid: %v", n, idxValid(t, s, fresh.ID))
	}
	// the cleanup command
	c := publicEx(t, s, f, 3, "omitiu")
	keep := freeze(t, s)
	must(t, s.SetExemploPublico(ctx, c, false, admin)) // invalidates `keep` and `fresh`? only those that list it
	del, err := s.DeleteInvalidIndexes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if del < 1 {
		t.Errorf("nothing was deleted (n=%d)", del)
	}
	if _, err := s.GetIndexMeta(ctx, keep.ID); !errors.Is(err, ErrIndexNotFound) {
		t.Errorf("the invalid index is still there: %v", err)
	}
	if !idxValid(t, s, fresh.ID) {
		t.Error("a valid index was deleted or invalidated by the cleanup")
	}
	if again, _ := s.DeleteInvalidIndexes(ctx); again != 0 {
		t.Errorf("a second cleanup deleted %d", again)
	}
}

func TestTheSweepOnlyLooksAtValidIndexes(t *testing.T) { // M-4
	var l invLog
	s := openStore(t, Options{OnIndexInvalidated: l.add})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	m := freeze(t, s)
	must(t, s.SetExemploPublico(ctx, a, false, admin))
	first := l.got()
	if !strings.Contains(first, m.ID) {
		t.Fatal("setup")
	}
	// further writes do not announce the dead index again
	b := newEl(t, s, f, 2, "omitiu")
	must(t, s.Confirm(ctx, b, admin))
	if l.got() != first {
		t.Errorf("an invalid index is announced again: %q", l.got())
	}
}

func TestTheRealSourceReadsTheTraceFromTheContentDatabase(t *testing.T) { // C-2.9, C-2.10a
	cs, err := content.Open(filepath.Join(t.TempDir(), "c.db"), content.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if _, err := cs.SavePromptJSONMelhora("job-1", "texto", []byte(`{"enviado":1}`), false,
		content.MelhoraTrace{Original: []byte(`{"antes":1}`), InstrucoesID: "iabc", IndiceID: "idx9", Exemplos: []string{"o1", "o2"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.SavePromptJSON("job-2", "texto", []byte(`{"enviado":2}`), false, time.Now()); err != nil {
		t.Fatal(err)
	}
	src := NewSource(nil, cs)
	c, ok, err := src.Content(ctx, "job-1")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if string(c.PromptJSON) != `{"enviado":1}` || string(c.PromptJSONOriginal) != `{"antes":1}` || c.MelhoraInstrucoesID != "iabc" || c.MelhoraIndiceID != "idx9" || c.MelhoraExemplos != "o1,o2" || !c.Melhora() {
		t.Errorf("%+v", c)
	}
	c2, _, _ := src.Content(ctx, "job-2")
	if c2.Melhora() || c2.PromptJSONOriginal != nil || c2.MelhoraInstrucoesID != "" {
		t.Errorf("a plain job reads with a trace: %+v", c2)
	}
}
