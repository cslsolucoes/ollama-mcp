package treino

// Fase 12, etapa 12.7: the retrato (C-4.13, C-4.14, C-5.1, C-5.7, C-5.13).

import (
	"bytes"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func confirmedEl(t *testing.T, s *Store, f *fakeSrc, n int, cat string) string {
	t.Helper()
	id := newEl(t, s, f, n, cat)
	must(t, s.Confirm(ctx, id, admin))
	return id
}

func itemIDs(rt Retrato) []string {
	var ids []string
	for _, it := range rt.Itens {
		ids = append(ids, it.OcorrenciaID)
	}
	sort.Strings(ids)
	return ids
}

func TestRetratoOnlyEligibleTestAndEvaluable(t *testing.T) { // C-5.1, C-5.7, C-4.13
	s := openStore(t, Options{})
	f := newFake()
	a := confirmedEl(t, s, f, 1, "omitiu")
	b := confirmedEl(t, s, f, 2, "formato_invalido")
	e := confirmedEl(t, s, f, 3, "numeracao_errada")
	c := confirmedEl(t, s, f, 4, CategoriaOutro)
	d := confirmedEl(t, s, f, 5, CategoriaIdioma)
	trunc := confirmedEl(t, s, f, 6, "omitiu")
	treinoSide := confirmedEl(t, s, f, 7, "omitiu")
	pending := newEl(t, s, f, 8, "omitiu") // never confirmed
	for _, id := range []string{a, b, e, c, d, trunc, treinoSide} {
		if got := sideOf(t, s, id); got != ParteTeste {
			t.Fatalf("setup: %s is in %q, want teste (quota)", id, got)
		}
	}
	if _, err := s.db.Exec(`UPDATE amostras SET prompt_truncado=1 WHERE job_id=?`, mkID(6)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, treinoSide); err != nil {
		t.Fatal(err)
	}
	rt, err := s.CreateRetrato(ctx, RetratoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, want := itemIDs(rt), []string{a, b, e}
	sort.Strings(want)
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("items %v, want %v (not: outro %s, idioma %s, truncated %s, treino %s, nova %s)", got, want, c, d, trunc, treinoSide, pending)
	}
	for _, it := range rt.Itens {
		var cat, ph string
		if err := s.db.QueryRow(`SELECT o.categoria, a.prompt_hmac FROM ocorrencias o JOIN amostras a ON a.job_id=o.job_do_erro WHERE o.id=?`, it.OcorrenciaID).Scan(&cat, &ph); err != nil {
			t.Fatal(err)
		}
		if it.Categoria != cat || it.PromptHMAC != ph || it.PromptHMAC == "" || it.JobID == "" {
			t.Errorf("item %+v does not carry the category and prompt_hmac of the sample", it)
		}
	}
	// C-4.13: the hashes of the index and of the instructions are "nenhum" until 12.8
	if rt.IndiceHash != NoneValue || rt.InstrucoesID != NoneValue || rt.InstrucoesHash != NoneValue || !rt.Valido {
		t.Errorf("retrato fields: %+v", rt)
	}
	back, err := s.GetRetrato(ctx, rt.ID)
	if err != nil || len(back.Itens) != 3 || !back.Valido || back.IndiceHash != NoneValue || back.CriadoEm != rt.CriadoEm {
		t.Fatalf("GetRetrato: %+v %v", back, err)
	}
	// the 12.8 hashes are recorded when given
	rt2, err := s.CreateRetrato(ctx, RetratoOptions{IndiceHash: "ih", InstrucoesID: "v3", InstrucoesHash: "hh"})
	if err != nil {
		t.Fatal(err)
	}
	if b2, _ := s.GetRetrato(ctx, rt2.ID); b2.IndiceHash != "ih" || b2.InstrucoesID != "v3" || b2.InstrucoesHash != "hh" {
		t.Errorf("hashes not recorded: %+v", b2)
	}
}

func TestRetratoNoCasesIsRefused(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	confirmedEl(t, s, f, 1, CategoriaOutro) // only a category without criterion
	if _, err := s.CreateRetrato(ctx, RetratoOptions{}); !errors.Is(err, ErrNoCases) {
		t.Fatalf("err = %v, want ErrNoCases", err)
	}
	if n := count(t, s, "retratos"); n != 0 {
		t.Errorf("%d retratos left behind", n)
	}
}

func TestRetratoIsFrozenAndOnePerEvaluation(t *testing.T) { // C-4.13, C-4.14
	s := openStore(t, Options{})
	f := newFake()
	confirmedEl(t, s, f, 1, "omitiu")
	rt, err := s.CreateRetrato(ctx, RetratoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	confirmedEl(t, s, f, 2, "omitiu") // a later confirmation does not enter the frozen list
	back, _ := s.GetRetrato(ctx, rt.ID)
	if len(back.Itens) != 1 {
		t.Fatalf("the retrato changed: %d items", len(back.Itens))
	}
	rt2, err := s.CreateRetrato(ctx, RetratoOptions{})
	if err != nil || rt2.ID == rt.ID || len(rt2.Itens) != 2 {
		t.Fatalf("a new evaluation needs a new retrato: %+v %v", rt2, err)
	}
}

func TestRetratoCasesContentAndInvalidRetratoIsRefused(t *testing.T) { // C-5.13, C-4.10, C-1.12b
	s := openStore(t, Options{})
	f := newFake()
	a := confirmedEl(t, s, f, 1, "omitiu")
	confirmedEl(t, s, f, 2, "formato_invalido")
	rt, err := s.CreateRetrato(ctx, RetratoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, cases, err := s.RetratoCases(ctx, rt.ID)
	if err != nil || len(cases) != 2 {
		t.Fatalf("RetratoCases: %d %v", len(cases), err)
	}
	for _, c := range cases {
		n := 1
		if c.Categoria == "formato_invalido" {
			n = 2
		}
		if !bytes.Equal(c.PromptJSON, pjOf(n)) || c.RespostaCorreta != "resposta certa "+string(rune('0'+n)) || c.Endpoint != "/api/generate" || c.Tool != "extract" {
			t.Errorf("case %+v does not carry the frozen sample", c)
		}
	}
	// discarding a listed occurrence invalidates the retrato (existing rule) and it is never used again
	must(t, s.Discard(ctx, a, admin))
	if _, _, err := s.RetratoCases(ctx, rt.ID); !errors.Is(err, ErrRetratoInvalido) {
		t.Fatalf("err = %v, want ErrRetratoInvalido", err)
	}
	if got, _ := s.GetRetrato(ctx, rt.ID); got.Valido {
		t.Error("valido must be 0")
	}
	if _, _, err := s.RetratoCases(ctx, "00000000000000000000000000000000"); !errors.Is(err, ErrRetratoNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestRetratoCasesNoticesAChangeAndInvalidates(t *testing.T) { // the check at reading time (defence in depth)
	tamper := []struct{ name, sql string }{
		{"prompt_hmac column", `UPDATE amostras SET prompt_hmac='deadbeef'`},
		{"prompt_json (key recomputes the hmac)", `UPDATE amostras SET prompt_json=CAST('{"model":"x","prompt":"outro"}' AS BLOB)`},
		{"category", `UPDATE ocorrencias SET categoria='truncado'`},
		{"partition", `UPDATE ocorrencias SET particao='treino'`},
		{"state", `UPDATE ocorrencias SET estado='nova'`},
		{"sample gone", `DELETE FROM amostras`},
		{"truncated sample", `UPDATE amostras SET resp_truncada=1`},
		{"occurrence erased", `DELETE FROM ocorrencias`},
	}
	for _, c := range tamper {
		s := openStore(t, Options{})
		f := newFake()
		confirmedEl(t, s, f, 1, "omitiu")
		rt, err := s.CreateRetrato(ctx, RetratoOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(c.sql); err != nil {
			t.Fatal(c.name, err)
		}
		_, _, err = s.RetratoCases(ctx, rt.ID)
		if !errors.Is(err, ErrRetratoInvalido) {
			t.Errorf("%s: err = %v, want ErrRetratoInvalido", c.name, err)
		}
		if got, _ := s.GetRetrato(ctx, rt.ID); got.Valido {
			t.Errorf("%s: valido still 1", c.name)
		}
	}
}

func TestRetratoWithValidoZeroIsNeverUsedEvenIfTheCasesAreIntact(t *testing.T) { // C-5.13
	s := openStore(t, Options{})
	f := newFake()
	confirmedEl(t, s, f, 1, "omitiu")
	rt, err := s.CreateRetrato(ctx, RetratoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE retratos SET valido=0 WHERE id=?`, rt.ID); err != nil { // e.g. the quarantine of C-4.10
		t.Fatal(err)
	}
	if _, cases, err := s.RetratoCases(ctx, rt.ID); !errors.Is(err, ErrRetratoInvalido) || len(cases) != 0 {
		t.Fatalf("a retrato with valido=0 was used: %d cases, err %v", len(cases), err)
	}
}

func TestTxLockImmediateOnlyWhenAsked(t *testing.T) { // the bake-off writes a retrato to the live file; the gateway keeps its DSN
	if d := dsn("x.db", Options{}); strings.Contains(d, "_txlock") {
		t.Errorf("the gateway DSN changed: %s", d)
	}
	if d := dsn("x.db", Options{TxLockImmediate: true}); !strings.HasSuffix(d, "&_txlock=immediate") {
		t.Errorf("DSN without the option: %s", d)
	}
	// a store opened with it works end to end, next to another store on the same file
	path := filepath.Join(t.TempDir(), "treino.db")
	gw, err := Open(path, Options{Key: testKey(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer gw.Close()
	bk, err := Open(path, Options{Key: testKey(t), TxLockImmediate: true})
	if err != nil {
		t.Fatal(err)
	}
	defer bk.Close()
	f := newFake()
	confirmedEl(t, gw, f, 1, "omitiu")
	if rt, err := bk.CreateRetrato(ctx, RetratoOptions{}); err != nil || len(rt.Itens) != 1 {
		t.Fatalf("CreateRetrato by the second store: %+v %v", rt, err)
	}
}
