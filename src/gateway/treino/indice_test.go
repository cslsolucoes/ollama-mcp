package treino

// Fase 12, etapa 12.8: the frozen index of examples and the embeddings of the public
// examples (C-6.6, C-8.1 to C-8.5, C-8.9, C-8.10, C-4.10).

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

const testModel = "nomic-embed-text"
const testFmt = "f1"

func vecOf(n int) []float32 { return []float32{float32(n), 1, 0.5} }

// publicEx makes a confirmed occurrence in the TRAINING part, marked public, with its
// embedding computed.
func publicEx(t *testing.T, s *Store, f *fakeSrc, n int, cat string) string {
	t.Helper()
	id := newEl(t, s, f, n, cat)
	must(t, s.Confirm(ctx, id, admin))
	if _, err := s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	must(t, s.SetExemploPublico(ctx, id, true, admin))
	embed(t, s, id, vecOf(n))
	return id
}

func embed(t *testing.T, s *Store, id string, v []float32) {
	t.Helper()
	pend, err := s.PendingEmbeddings(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pend {
		if p.OcorrenciaID == id {
			must(t, s.SetEmbedding(ctx, id, p.Version, v, testModel))
			return
		}
	}
	t.Fatalf("%s is not pending", id)
}

func freeze(t *testing.T, s *Store) IndexMeta {
	t.Helper()
	m, err := s.FreezeIndex(ctx, FreezeOptions{K: 3, Limiar: 0.8, Modelo: testModel, Formato: testFmt})
	if err != nil {
		t.Fatalf("FreezeIndex: %v", err)
	}
	return m
}

func memberIDs(t *testing.T, s *Store, idx string) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT ocorrencia_id FROM indice_itens WHERE indice_id=? ORDER BY ocorrencia_id`, idx)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		out = append(out, id)
	}
	return out
}

func sortedEq(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, ",") == strings.Join(b, ",")
}

func idxValid(t *testing.T, s *Store, id string) bool {
	t.Helper()
	m, err := s.GetIndexMeta(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return m.Valido
}

// ---- C-8.1, C-8.3, C-8.4, C-8.9: who may be in the index -------------------------------------------

func TestFreezeTakesOnlyPublicConfirmedTrainingExamplesWithAVector(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	good1 := publicEx(t, s, f, 1, "omitiu")
	good2 := publicEx(t, s, f, 2, "inventou")

	notPublic := newEl(t, s, f, 3, "omitiu") // confirmed, training part, NOT marked public (a customer's)
	must(t, s.Confirm(ctx, notPublic, admin))
	s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, notPublic)
	// a vector left behind (a bug elsewhere, an old file) must not be enough: the mark of the admin decides
	s.db.Exec(`INSERT INTO embeddings(ocorrencia_id,vetor,modelo) VALUES(?,?,?)`, notPublic, EncodeVec(vecOf(3)), testModel)

	pending := newEl(t, s, f, 4, "omitiu") // public, but the embedding is still pending
	must(t, s.Confirm(ctx, pending, admin))
	s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, pending)
	must(t, s.SetExemploPublico(ctx, pending, true, admin))

	inTest := newEl(t, s, f, 5, "omitiu") // public with a vector, but in the TEST part (C-8.3)
	must(t, s.Confirm(ctx, inTest, admin))
	must(t, s.SetExemploPublico(ctx, inTest, true, admin))
	embed(t, s, inTest, vecOf(5))
	if sideOf(t, s, inTest) != ParteTeste {
		t.Fatalf("setup: %q", sideOf(t, s, inTest))
	}

	quar := publicEx(t, s, f, 6, "omitiu") // in quarantine (C-8.4)
	s.db.Exec(`UPDATE ocorrencias SET particao='quarentena' WHERE id=?`, quar)

	truncated := publicEx(t, s, f, 7, "omitiu") // the sample is cut: not exportable, so not an example
	s.db.Exec(`UPDATE amostras SET prompt_truncado=1 WHERE job_id=?`, mkID(7))

	otherModel := newEl(t, s, f, 8, "omitiu") // vector from another model
	must(t, s.Confirm(ctx, otherModel, admin))
	s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, otherModel)
	must(t, s.SetExemploPublico(ctx, otherModel, true, admin))
	pend, _ := s.PendingEmbeddings(ctx, 100)
	for _, p := range pend {
		if p.OcorrenciaID == otherModel {
			must(t, s.SetEmbedding(ctx, otherModel, p.Version, vecOf(8), "outro-modelo"))
		}
	}

	m := freeze(t, s)
	got := memberIDs(t, s, m.ID)
	if !sortedEq(got, []string{good1, good2}) || m.Itens != 2 {
		t.Fatalf("members %v (itens %d), want %v; not: notPublic %s pending %s inTest %s quarantine %s truncated %s otherModel %s",
			got, m.Itens, []string{good1, good2}, notPublic, pending, inTest, quar, truncated, otherModel)
	}
	if !m.Valido || m.K != 3 || m.Limiar != 0.8 || m.Modelo != testModel || len(m.Hash) != 64 {
		t.Errorf("meta: %+v", m)
	}
}

func TestFreezeWithNoExampleIsRefusedAndLeavesNothing(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	if _, err := s.FreezeIndex(ctx, FreezeOptions{K: 3, Limiar: 0.8, Modelo: testModel, Formato: testFmt}); !errors.Is(err, ErrNoExamples) {
		t.Fatalf("err = %v", err)
	}
	id := newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, id, admin))
	must(t, s.SetExemploPublico(ctx, id, true, admin)) // pending only
	if _, err := s.FreezeIndex(ctx, FreezeOptions{K: 3, Limiar: 0.8, Modelo: testModel, Formato: testFmt}); !errors.Is(err, ErrNoExamples) {
		t.Fatalf("err = %v", err)
	}
	if count(t, s, "indices") != 0 || count(t, s, "indice_itens") != 0 {
		t.Error("a refused freeze left rows behind")
	}
}

func TestFreezeValidatesKAndLimiar(t *testing.T) { // C-8.10
	s := openStore(t, Options{})
	f := newFake()
	publicEx(t, s, f, 1, "omitiu")
	for _, o := range []FreezeOptions{
		{K: 0, Limiar: 0.8, Modelo: testModel}, {K: MaxExamplesK + 1, Limiar: 0.8, Modelo: testModel},
		{K: 3, Limiar: 0, Modelo: testModel}, {K: 3, Limiar: 1.01, Modelo: testModel}, {K: 3, Limiar: -1, Modelo: testModel}, {K: 3, Limiar: 0.8},
	} {
		if _, err := s.FreezeIndex(ctx, o); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", o, err)
		}
	}
}

// ---- C-8.5: the index is frozen, with an id and a hash ------------------------------------------------

func TestIndexIsFrozenAndTheHashIsRecomputable(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	b := publicEx(t, s, f, 2, "omitiu")
	m := freeze(t, s)
	want := IndexHash(testModel, testFmt, 3, 0.8, []HashItem{{ID: b, Vec: EncodeVec(vecOf(2))}, {ID: a, Vec: EncodeVec(vecOf(1))}}) // any order
	if m.Hash != want {
		t.Errorf("hash %s, want %s", m.Hash, want)
	}
	c := publicEx(t, s, f, 3, "omitiu") // a later public example does not enter the frozen one
	if got := memberIDs(t, s, m.ID); !sortedEq(got, []string{a, b}) {
		t.Fatalf("the frozen index changed: %v (new: %s)", got, c)
	}
	d, err := s.LoadIndex(ctx, m.ID)
	if err != nil || len(d.Items) != 2 || d.Hash != m.Hash {
		t.Fatalf("LoadIndex: %+v %v", d.IndexMeta, err)
	}
	for _, it := range d.Items {
		if it.Tool != "extract" || len(it.Vec) != 3 || it.RespostaCorreta == "" || len(it.PromptJSON) == 0 || len(it.Resposta) == 0 || it.Endpoint != "/api/generate" {
			t.Errorf("item %+v lacks content", it)
		}
	}
	// a second freeze is another index, with another id and (now with 3 members) another hash
	m2 := freeze(t, s)
	if m2.ID == m.ID || m2.Hash == m.Hash || m2.Itens != 3 {
		t.Errorf("second freeze: %+v", m2)
	}
}

func TestIndexHashCoversModelKLimiarMembersAndVectors(t *testing.T) { // C-8.5, C-8.10
	base := []HashItem{{ID: "a", Vec: EncodeVec([]float32{1, 2})}, {ID: "b", Vec: EncodeVec([]float32{3, 4})}}
	h0 := IndexHash("m", testFmt, 3, 0.8, base)
	if IndexHash("m", testFmt, 3, 0.8, []HashItem{base[1], base[0]}) != h0 {
		t.Error("the member order matters")
	}
	variants := map[string]string{
		"model":   IndexHash("m2", testFmt, 3, 0.8, base),
		"k":       IndexHash("m", testFmt, 4, 0.8, base),
		"limiar":  IndexHash("m", testFmt, 3, 0.81, base),
		"member":  IndexHash("m", testFmt, 3, 0.8, base[:1]),
		"id":      IndexHash("m", testFmt, 3, 0.8, []HashItem{{ID: "a2", Vec: base[0].Vec}, base[1]}),
		"vector":  IndexHash("m", testFmt, 3, 0.8, []HashItem{{ID: "a", Vec: EncodeVec([]float32{1, 2.0001})}, base[1]}),
		"swapped": IndexHash("m", testFmt, 3, 0.8, []HashItem{{ID: "a", Vec: base[1].Vec}, {ID: "b", Vec: base[0].Vec}}),
	}
	for name, h := range variants {
		if h == h0 {
			t.Errorf("the hash ignores the %s", name)
		}
	}
}

func TestLoadIndexRefusesATamperedOne(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	publicEx(t, s, f, 1, "omitiu")
	publicEx(t, s, f, 2, "omitiu")
	m := freeze(t, s)
	if _, err := s.db.Exec(`UPDATE indice_itens SET vetor=? WHERE indice_id=? AND ocorrencia_id=(SELECT MIN(ocorrencia_id) FROM indice_itens WHERE indice_id=?)`,
		EncodeVec([]float32{9, 9, 9}), m.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadIndex(ctx, m.ID); !errors.Is(err, ErrIndexCorrupt) {
		t.Errorf("tampered vector: %v", err)
	}
	m2 := freeze(t, s)
	s.db.Exec(`DELETE FROM indice_itens WHERE indice_id=? AND ocorrencia_id=(SELECT MIN(ocorrencia_id) FROM indice_itens WHERE indice_id=?)`, m2.ID, m2.ID)
	if _, err := s.LoadIndex(ctx, m2.ID); !errors.Is(err, ErrIndexCorrupt) {
		t.Errorf("a member removed: %v", err)
	}
	if _, err := s.LoadIndex(ctx, "nope"); !errors.Is(err, ErrIndexNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

// ---- C-4.10, C-8.9: the index goes invalid when a member stops being an example ----------------------------

type invLog struct {
	mu  sync.Mutex
	ids []string
}

func (l *invLog) add(id string) { l.mu.Lock(); l.ids = append(l.ids, id); l.mu.Unlock() }
func (l *invLog) got() string   { l.mu.Lock(); defer l.mu.Unlock(); return strings.Join(l.ids, ",") }

func TestIndexGoesInvalidWhenAMemberStopsBeingAnExample(t *testing.T) {
	cases := map[string]func(t *testing.T, s *Store, member string){
		"unmarked as public": func(t *testing.T, s *Store, id string) { must(t, s.SetExemploPublico(ctx, id, false, admin)) },
		"discarded":          func(t *testing.T, s *Store, id string) { must(t, s.Discard(ctx, id, admin)) },
		"erased":             func(t *testing.T, s *Store, id string) { must(t, s.Delete(ctx, id, admin)) },
		"edited (unconfirmed)": func(t *testing.T, s *Store, id string) {
			x := "outra resposta"
			if _, err := s.Update(ctx, id, Edit{RespostaCorreta: &x}, admin); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, act := range cases {
		t.Run(name, func(t *testing.T) {
			var l invLog
			s := openStore(t, Options{OnIndexInvalidated: l.add})
			f := newFake()
			a := publicEx(t, s, f, 1, "omitiu")
			publicEx(t, s, f, 2, "omitiu")
			m := freeze(t, s)
			act(t, s, a)
			if idxValid(t, s, m.ID) {
				t.Errorf("the index still valid after the member was %s", name)
			}
			if !strings.Contains(l.got(), m.ID) {
				t.Errorf("the process was not told (got %q)", l.got())
			}
			if _, err := s.LoadIndex(ctx, m.ID); !errors.Is(err, ErrIndexInvalid) {
				t.Errorf("LoadIndex of an invalid index: %v", err)
			}
		})
	}
}

func TestIndexNotListingTheMemberStaysValid(t *testing.T) {
	var l invLog
	s := openStore(t, Options{OnIndexInvalidated: l.add})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	b := publicEx(t, s, f, 2, "omitiu")
	m := freeze(t, s)
	c := publicEx(t, s, f, 3, "omitiu") // enters later, so it is not a member
	must(t, s.SetExemploPublico(ctx, c, false, admin))
	must(t, s.Discard(ctx, c, admin))
	if !idxValid(t, s, m.ID) || l.got() != "" {
		t.Errorf("an unrelated occurrence invalidated the index: %q", l.got())
	}
	_ = a
	_ = b
	// ordinary writes (a new report, a confirmation) do not touch it either
	d := newEl(t, s, f, 4, "omitiu")
	must(t, s.Confirm(ctx, d, admin))
	if !idxValid(t, s, m.ID) {
		t.Error("an ordinary write invalidated the index")
	}
}

func TestIndexGoesInvalidWhenAMemberEntersQuarantineByTheRealPath(t *testing.T) { // C-4.10
	var l invLog
	s := openStore(t, Options{OnIndexInvalidated: l.add})
	f := newFake()
	// A: test side (first of its category); B: a public training example
	a := newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, a, admin))
	b := publicEx(t, s, f, 2, "inventou")
	if sideOf(t, s, a) != ParteTeste || sideOf(t, s, b) != ParteTreino {
		t.Fatalf("setup %q %q", sideOf(t, s, a), sideOf(t, s, b))
	}
	m := freeze(t, s)
	// a report that links A's group to B's puts the whole component in quarantine
	x := report(jr(1, "map1", "primario"), jr(2, "final", "primario"))
	x.Origem = "claude"
	ingest(t, s, x, "", f)
	if sideOf(t, s, b) != ParteQuarentena {
		t.Fatalf("setup: B is %q", sideOf(t, s, b))
	}
	if idxValid(t, s, m.ID) || !strings.Contains(l.got(), m.ID) {
		t.Errorf("quarantined member: valid=%v told=%q", idxValid(t, s, m.ID), l.got())
	}
	if _, err := s.FreezeIndex(ctx, FreezeOptions{K: 3, Limiar: 0.8, Modelo: testModel, Formato: testFmt}); !errors.Is(err, ErrNoExamples) {
		t.Errorf("a quarantined occurrence entered a new index: %v", err)
	}
}

func TestIndexGoesInvalidWhenAMemberLeavesTheTrainingPartBySQL(t *testing.T) {
	// defence in depth: even a change this package does not make is caught by the next write
	var l invLog
	s := openStore(t, Options{OnIndexInvalidated: l.add})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	m := freeze(t, s)
	s.db.Exec(`UPDATE ocorrencias SET particao='teste' WHERE id=?`, a)
	if !idxValid(t, s, m.ID) {
		t.Fatal("nothing ran yet")
	}
	if _, err := s.LoadIndex(ctx, m.ID); !errors.Is(err, ErrIndexInvalid) { // the load re-checks the members
		t.Errorf("LoadIndex: %v", err)
	}
	if idxValid(t, s, m.ID) || !strings.Contains(l.got(), m.ID) {
		t.Errorf("the load did not invalidate: %q", l.got())
	}
}

func TestErasingAMemberDropsItsVectorFromTheIndex(t *testing.T) { // C-6.3
	s := openStore(t, Options{})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	m := freeze(t, s)
	must(t, s.Delete(ctx, a, admin))
	var blob []byte
	if err := s.db.QueryRow(`SELECT vetor FROM indice_itens WHERE indice_id=? AND ocorrencia_id=?`, m.ID, a).Scan(&blob); err != nil || len(blob) != 0 {
		t.Errorf("the vector copy is still there: %v %d bytes", err, len(blob))
	}
	var emb int
	s.db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE ocorrencia_id=?`, a).Scan(&emb)
	if emb != 0 {
		t.Error("the embedding of an erased occurrence is still there")
	}
}

func TestRollbackDoesNotAnnounceAnInvalidation(t *testing.T) {
	var l invLog
	s := openStore(t, Options{OnIndexInvalidated: l.add})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	m := freeze(t, s)
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE ocorrencias SET exemplo_publico=0 WHERE id=?`, a); err != nil {
			return err
		}
		return errors.New("falha no meio")
	})
	if err == nil || !idxValid(t, s, m.ID) || l.got() != "" {
		t.Errorf("a rolled-back write invalidated: err=%v valid=%v told=%q", err, idxValid(t, s, m.ID), l.got())
	}
}

// ---- C-6.6: the embeddings of the public examples ----------------------------------------------------------

func TestPendingEmbeddingsSetAndStale(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	id := newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, id, admin))
	if p, _ := s.PendingEmbeddings(ctx, 10); len(p) != 0 {
		t.Fatalf("an occurrence that is not public is pending: %v", p)
	}
	must(t, s.SetExemploPublico(ctx, id, true, admin))
	pend, err := s.PendingEmbeddings(ctx, 10)
	if err != nil || len(pend) != 1 || pend[0].OcorrenciaID != id {
		t.Fatalf("pending: %v %v", pend, err)
	}
	src, err := s.EmbeddingSource(ctx, id)
	if err != nil || src.Version != pend[0].Version || src.Tool != "extract" || src.Endpoint != "/api/generate" || !strings.Contains(string(src.PromptJSON), "pedido 1") {
		t.Fatalf("source: %+v %v", src, err)
	}
	// the occurrence changes while the vector is being made: the write is refused
	must(t, s.SetExemploPublico(ctx, id, false, admin))
	must(t, s.SetExemploPublico(ctx, id, true, admin))
	if err := s.SetEmbedding(ctx, id, src.Version, vecOf(1), testModel); !errors.Is(err, ErrStaleEmbed) {
		t.Errorf("a vector for an old version was accepted: %v", err)
	}
	pend, _ = s.PendingEmbeddings(ctx, 10)
	if len(pend) != 1 || pend[0].Version == src.Version {
		t.Fatalf("pending after the change: %v", pend)
	}
	must(t, s.SetEmbedding(ctx, id, pend[0].Version, vecOf(1), testModel))
	if p, _ := s.PendingEmbeddings(ctx, 10); len(p) != 0 {
		t.Errorf("still pending: %v", p)
	}
	if err := s.SetEmbedding(ctx, id, pend[0].Version, vecOf(2), testModel); !errors.Is(err, ErrStaleEmbed) {
		t.Errorf("a vector over a computed one: %v", err)
	}
	var blob []byte
	var model string
	s.db.QueryRow(`SELECT vetor, modelo FROM embeddings WHERE ocorrencia_id=?`, id).Scan(&blob, &model)
	if v := DecodeVec(blob); len(v) != 3 || v[0] != 1 || model != testModel {
		t.Errorf("stored: %v %q", v, model)
	}
	if err := s.SetEmbedding(ctx, id, 1, nil, testModel); !errors.Is(err, ErrInvalid) {
		t.Errorf("an empty vector: %v", err)
	}
	if err := s.SetEmbedding(ctx, mkID(99), 1, vecOf(1), testModel); !errors.Is(err, ErrStaleEmbed) {
		t.Errorf("unknown occurrence: %v", err)
	}
}

func TestEmbeddingIsDeletedWhenTheMarkLeavesOrTheConfirmationIsUndone(t *testing.T) { // C-6.6
	s := openStore(t, Options{})
	f := newFake()
	a := publicEx(t, s, f, 1, "omitiu")
	b := publicEx(t, s, f, 2, "omitiu")
	must(t, s.SetExemploPublico(ctx, a, false, admin))
	x := "resposta nova"
	if _, err := s.Update(ctx, b, Edit{RespostaCorreta: &x}, admin); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "embeddings"); n != 0 {
		t.Errorf("%d embedding rows left", n)
	}
}

func TestEmbeddingSourceWithoutASample(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	id := newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, id, admin))
	must(t, s.SetExemploPublico(ctx, id, true, admin))
	s.db.Exec(`UPDATE amostras SET prompt_json=NULL`)
	if _, err := s.EmbeddingSource(ctx, id); !errors.Is(err, ErrNoSample) {
		t.Errorf("err = %v", err)
	}
	if _, err := s.EmbeddingSource(ctx, mkID(5)); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestVecRoundTrip(t *testing.T) {
	v := []float32{0, -1.5, 3.25e10, 1e-20}
	got := DecodeVec(EncodeVec(v))
	if len(got) != len(v) {
		t.Fatal(got)
	}
	for i := range v {
		if got[i] != v[i] {
			t.Errorf("[%d] %v != %v", i, got[i], v[i])
		}
	}
	if DecodeVec(nil) != nil || DecodeVec([]byte{1, 2, 3}) != nil || DecodeVec([]byte{0, 0, 0xc0, 0x7f}) != nil /* NaN */ {
		t.Error("a bad blob must decode to nil")
	}
}

// ---- C-8.10: k and the floor in the retrato; the migration ---------------------------------------------------

func TestRetratoRecordsKAndLimiarWithTheIndex(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	confirmedEl(t, s, f, 1, "omitiu")
	rt, err := s.CreateRetrato(ctx, RetratoOptions{IndiceHash: "ih", InstrucoesID: "iabc", InstrucoesHash: "hh", ExemplosK: 4, ExemplosLimiar: 0.77})
	if err != nil {
		t.Fatal(err)
	}
	back, err := s.GetRetrato(ctx, rt.ID)
	if err != nil || back.IndiceHash != "ih" || back.ExemplosK != 4 || back.ExemplosLimiar != 0.77 || rt.ExemplosK != 4 {
		t.Fatalf("retrato: %+v %v", back, err)
	}
	// without an index, k and the floor are not recorded, even if given
	rt2, _ := s.CreateRetrato(ctx, RetratoOptions{ExemplosK: 4, ExemplosLimiar: 0.77})
	back2, _ := s.GetRetrato(ctx, rt2.ID)
	if back2.ExemplosK != 0 || back2.ExemplosLimiar != 0 || back2.IndiceHash != NoneValue {
		t.Errorf("k recorded without an index: %+v", back2)
	}
	var n sql.NullInt64
	s.db.QueryRow(`SELECT exemplos_k FROM retratos WHERE id=?`, rt2.ID).Scan(&n)
	if n.Valid {
		t.Error("exemplos_k should be NULL without an index")
	}
}

func TestMigrationOfAnOldFileIsIdempotentAndKeepsTheData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "treino.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// the schema of version 1 (before stage 12.8): retratos without the two columns, no index tables
	for _, q := range []string{
		`CREATE TABLE retratos(id TEXT PRIMARY KEY, criado_em INTEGER NOT NULL, indice_hash TEXT, instrucoes_id TEXT, instrucoes_hash TEXT, valido INTEGER NOT NULL DEFAULT 1)`,
		`INSERT INTO retratos(id,criado_em,indice_hash,instrucoes_id,instrucoes_hash,valido) VALUES('velho',7,'nenhum','nenhum','nenhum',1)`,
		`PRAGMA user_version=1`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()
	for i := 0; i < 3; i++ { // opening again changes nothing and loses nothing
		s, err := Open(path, Options{Now: openStore(t, Options{}).opts.Now})
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		rt, err := s.GetRetrato(ctx, "velho")
		if err != nil || rt.CriadoEm != 7 || rt.ExemplosK != 0 || rt.IndiceHash != "nenhum" {
			t.Fatalf("open %d: %+v %v", i, rt, err)
		}
		for _, col := range []string{"exemplos_k", "exemplos_limiar"} {
			if has, _ := hasColumn(s.db, "retratos", col); !has {
				t.Errorf("open %d: no column %s", i, col)
			}
		}
		var v int
		s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != schemaVersion {
			t.Errorf("user_version %d", v)
		}
		for _, tb := range []string{"indices", "indice_itens"} {
			if _, err := s.db.Exec(fmt.Sprintf(`SELECT 1 FROM %s LIMIT 1`, tb)); err != nil {
				t.Errorf("table %s: %v", tb, err)
			}
		}
		s.Close()
	}
}

func TestCaseMetaGivesTheGroupAndTheDocument(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	id := newEl(t, s, f, 1, "omitiu")
	s.db.Exec(`UPDATE ocorrencias SET doc_hash='dh', doc_key_id='kid' WHERE id=?`, id)
	m, err := s.CaseMeta(ctx, id)
	if err != nil || m.GrupoID == "" || m.DocHash != "dh" || m.DocKeyID != "kid" {
		t.Errorf("%+v %v", m, err)
	}
	if _, err := s.CaseMeta(ctx, mkID(9)); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}
