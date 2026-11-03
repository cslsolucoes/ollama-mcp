package treino

// Fase 12, etapa 12.4: the state machine, the operations and the history
// (contract C-1.12, C-1.12a, C-1.12b, C-4.11a, C-6.3 to C-6.5, C-7.x).

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var ctx = context.Background()

var admin = AdminActor("10.0.0.9")

// nextJob gives each helper call its own job numbers.
var nextJob = 1000

// occIn creates an occurrence in the given state, with a correction and with its own jobs.
func occIn(t *testing.T, s *Store, state string) string {
	t.Helper()
	f := newFake()
	nextJob += 10
	a, b := nextJob, nextJob+1
	f.add(a)
	f.add(b)
	r := report(jr(a, "map1", "primario"), jr(b, "final", "primario"))
	r.RespostaCorreta = "resposta certa " + fmt.Sprint(a)
	role := ""
	switch state {
	case EstadoDistrib:
		r.Jobs[0].Backend = "fallback"
	case EstadoFallback:
		role = RoleFallback
	case EstadoSemPedido:
		delete(f.conts, mkID(b))
	}
	id := ingest(t, s, r, role, f).OcorrenciaID
	switch state {
	case EstadoNova, EstadoDistrib, EstadoFallback, EstadoSemPedido:
	case EstadoConfirmada:
		must(t, s.Confirm(ctx, id, admin))
	case EstadoDescartada:
		must(t, s.Discard(ctx, id, admin))
	case EstadoApagada:
		must(t, s.Delete(ctx, id, admin))
	default:
		t.Fatalf("state %q", state)
	}
	if got := estadoOf(t, s, id); got != state {
		t.Fatalf("occIn(%s) made %s", state, got)
	}
	return id
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransitionsAreExactlyTheContractOnes(t *testing.T) { // C-1.12
	states := []string{EstadoNova, EstadoConfirmada, EstadoDescartada, EstadoSemPedido, EstadoDistrib, EstadoFallback, EstadoApagada}
	allowed := map[string]map[string]string{ // action -> from -> to
		"confirmar": {EstadoNova: EstadoConfirmada},
		"descartar": {EstadoNova: EstadoDescartada, EstadoConfirmada: EstadoDescartada, EstadoSemPedido: EstadoDescartada,
			EstadoDistrib: EstadoDescartada, EstadoFallback: EstadoDescartada},
		"apagar":  {EstadoNova: EstadoApagada, EstadoConfirmada: EstadoApagada, EstadoDescartada: EstadoApagada, EstadoSemPedido: EstadoApagada, EstadoDistrib: EstadoApagada, EstadoFallback: EstadoApagada},
		"alterar": {EstadoNova: EstadoNova, EstadoConfirmada: EstadoNova}, // confirmada -> nova (C-6.5)
	}
	nova := "outra correcao"
	for _, action := range []string{"confirmar", "descartar", "apagar", "alterar"} {
		for _, from := range states {
			s := openStore(t, Options{})
			id := occIn(t, s, from)
			var err error
			switch action {
			case "confirmar":
				err = s.Confirm(ctx, id, admin)
			case "descartar":
				err = s.Discard(ctx, id, admin)
			case "apagar":
				err = s.Delete(ctx, id, admin)
			case "alterar":
				_, err = s.Update(ctx, id, Edit{RespostaCorreta: &nova}, admin)
			}
			to, ok := allowed[action][from]
			if ok {
				if err != nil {
					t.Errorf("%s from %s refused: %v", action, from, err)
				} else if got := estadoOf(t, s, id); got != to {
					t.Errorf("%s from %s -> %s, want %s", action, from, got, to)
				}
			} else {
				if !errors.Is(err, ErrTransition) {
					t.Errorf("%s from %s must be ErrTransition, got %v", action, from, err)
				}
				if got := estadoOf(t, s, id); got != from {
					t.Errorf("%s from %s changed the state to %s", action, from, got)
				}
			}
		}
	}
	s := openStore(t, Options{})
	if err := s.Confirm(ctx, mkID(77), admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestConfirmNeedsAnAnswer(t *testing.T) { // C-1.12a
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	r := report(jr(1, "unico", "primario"))
	r.Origem, r.RespostaCorreta = "automatico", ""
	id := ingest(t, s, r, "", f).OcorrenciaID
	if err := s.Confirm(ctx, id, admin); !errors.Is(err, ErrNeedsAnswer) {
		t.Fatalf("confirm without answer: %v", err)
	}
	if estadoOf(t, s, id) != EstadoNova || count(t, s, "historico") != 1 {
		t.Error("the refused confirm left traces")
	}
	blank := "  \n\t"
	if _, err := s.Update(ctx, id, Edit{RespostaCorreta: &blank}, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(ctx, id, admin); !errors.Is(err, ErrNeedsAnswer) {
		t.Fatalf("confirm with a blank answer: %v", err)
	}
	ok := "certa"
	if _, err := s.Update(ctx, id, Edit{RespostaCorreta: &ok}, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(ctx, id, admin); err != nil {
		t.Fatal(err)
	}
	var by string
	var at sql.NullInt64
	s.db.QueryRow(`SELECT revisada_por, revisada_em FROM ocorrencias WHERE id=?`, id).Scan(&by, &at)
	if by != "admin" || !at.Valid {
		t.Errorf("revision not recorded: %q %v", by, at)
	}
}

// seedExample makes the occurrence look like an example and a test case: a
// public mark, an embedding, a partition and a group, listed by one snapshot;
// another snapshot lists someone else.
func seedExample(t *testing.T, s *Store, id string) {
	t.Helper()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE ocorrencias SET exemplo_publico=1, particao='teste', grupo_id='g1' WHERE id=?`, []any{id}},
		{`INSERT INTO embeddings(ocorrencia_id,vetor,modelo) VALUES(?,?,?)`, []any{id, []byte{1, 2, 3}, "nomic-embed-text"}},
		{`INSERT INTO retratos(id,criado_em,valido) VALUES('r-lista',1,1),('r-outro',1,1)`, nil},
		{`INSERT INTO retrato_itens(retrato_id,ocorrencia_id,job_id,prompt_hmac,categoria) VALUES('r-lista',?,'j','h','omitiu'),('r-outro','outra','j2','h2','omitiu')`, []any{id}},
	} {
		if _, err := s.db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func valid(t *testing.T, s *Store, retrato string) int {
	t.Helper()
	var v int
	if err := s.db.QueryRow(`SELECT valido FROM retratos WHERE id=?`, retrato).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDiscardingAConfirmedDropsTheExample(t *testing.T) { // C-1.12b, C-4.11a
	s := openStore(t, Options{})
	id := occIn(t, s, EstadoConfirmada)
	seedExample(t, s, id)
	must(t, s.Discard(ctx, id, admin))
	if n := count(t, s, "embeddings"); n != 0 {
		t.Errorf("embeddings = %d", n)
	}
	var ex int
	var part, grupo string
	s.db.QueryRow(`SELECT exemplo_publico, particao, grupo_id FROM ocorrencias WHERE id=?`, id).Scan(&ex, &part, &grupo)
	if ex != 0 {
		t.Error("exemplo_publico was not zeroed")
	}
	if part != "teste" || grupo != "g1" {
		t.Errorf("the vaga in the quota was given back: particao=%q grupo=%q", part, grupo)
	}
	if valid(t, s, "r-lista") != 0 || valid(t, s, "r-outro") != 1 {
		t.Errorf("snapshots: listing=%d other=%d", valid(t, s, "r-lista"), valid(t, s, "r-outro"))
	}
	var antes, depois string
	if err := s.db.QueryRow(`SELECT antes, depois FROM historico WHERE ocorrencia_id=? AND acao='descartou'`, id).Scan(&antes, &depois); err != nil || antes != "confirmada" || depois != "descartada" {
		t.Errorf("history: %v %q %q", err, antes, depois)
	}
}

func TestUnconfirmAndDeleteAlsoInvalidateSnapshots(t *testing.T) { // C-4.11a, C-6.5
	s := openStore(t, Options{})
	id := occIn(t, s, EstadoConfirmada)
	seedExample(t, s, id)
	nv := "mudou"
	if _, err := s.Update(ctx, id, Edit{CorrecaoAplicada: &nv}, admin); err != nil {
		t.Fatal(err)
	}
	if valid(t, s, "r-lista") != 0 || valid(t, s, "r-outro") != 1 {
		t.Error("unconfirming did not invalidate exactly the snapshot that lists it")
	}
	id2 := occIn(t, s, EstadoNova)
	s.db.Exec(`UPDATE retratos SET valido=1`)
	s.db.Exec(`INSERT INTO retrato_itens(retrato_id,ocorrencia_id,job_id) VALUES('r-outro',?,'j3')`, id2)
	must(t, s.Delete(ctx, id2, admin))
	if valid(t, s, "r-outro") != 0 || valid(t, s, "r-lista") != 1 {
		t.Error("deleting did not invalidate the snapshot that lists it")
	}
}

func TestAlteringAConfirmedUndoesTheConfirmation(t *testing.T) { // C-6.5
	for _, field := range []string{"resposta_correta", "descricao", "correcao_aplicada"} {
		s := openStore(t, Options{})
		id := occIn(t, s, EstadoConfirmada)
		seedExample(t, s, id)
		nv := "novo valor de " + field
		var e Edit
		switch field {
		case "resposta_correta":
			e.RespostaCorreta = &nv
		case "descricao":
			e.Descricao = &nv
		default:
			e.CorrecaoAplicada = &nv
		}
		des, err := s.Update(ctx, id, e, admin)
		if err != nil || !des {
			t.Fatalf("%s: %v %v", field, des, err)
		}
		if estadoOf(t, s, id) != EstadoNova {
			t.Errorf("%s: estado = %s", field, estadoOf(t, s, id))
		}
		if count(t, s, "embeddings") != 0 {
			t.Errorf("%s: the embedding stayed", field)
		}
		var n int
		s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='desconfirmou'`, id).Scan(&n)
		if n != 1 {
			t.Errorf("%s: desconfirmou rows = %d", field, n)
		}
		s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo=?`, id, field).Scan(&n)
		if n != 1 {
			t.Errorf("%s: alterou rows = %d", field, n)
		}
		var rv sql.NullString
		s.db.QueryRow(`SELECT revisada_por FROM ocorrencias WHERE id=?`, id).Scan(&rv)
		if rv.Valid {
			t.Errorf("%s: the old review stayed", field)
		}
	}
	// the same value is no change: still confirmed, no row
	s := openStore(t, Options{})
	id := occIn(t, s, EstadoConfirmada)
	same := "resposta certa " + fmt.Sprint(nextJob)
	h := count(t, s, "historico")
	des, err := s.Update(ctx, id, Edit{RespostaCorreta: &same}, admin)
	if err != nil || des || estadoOf(t, s, id) != EstadoConfirmada || count(t, s, "historico") != h {
		t.Errorf("same value: %v %v %s", des, err, estadoOf(t, s, id))
	}
	// a nova stays nova, with an alterou row and no desconfirmou
	id = occIn(t, s, EstadoNova)
	nv := "x"
	des, err = s.Update(ctx, id, Edit{Descricao: &nv}, admin)
	if err != nil || des || estadoOf(t, s, id) != EstadoNova {
		t.Errorf("nova: %v %v", des, err)
	}
	// limits apply to the admin too
	big := strings.Repeat("a", MaxResposta+1)
	if _, err := s.Update(ctx, id, Edit{RespostaCorreta: &big}, admin); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversize: %v", err)
	}
}

func TestSubstitutionKeepsConfirmedUntouchedByOtherReports(t *testing.T) { // C-1.19 does not touch confirmed ones
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	h := report(jr(1, "unico", "primario"))
	h.RespostaCorreta = "ok"
	id := ingest(t, s, h, "", f).OcorrenciaID
	must(t, s.Confirm(ctx, id, admin))
	again := ingest(t, s, h, "", f)
	if again.OcorrenciaID == id || estadoOf(t, s, id) != EstadoConfirmada {
		t.Error("a new report altered a confirmed occurrence")
	}
}

func rowCols(t *testing.T, s *Store, id string) map[string]any {
	t.Helper()
	rows, err := s.db.Query(`SELECT * FROM ocorrencias WHERE id=?`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	if !rows.Next() {
		t.Fatal("no row")
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	for i, c := range cols {
		m[c] = vals[i]
	}
	return m
}

func TestDeleteLeavesOnlyTheMinimalRow(t *testing.T) { // C-6.3
	s := openStore(t, Options{})
	id := occIn(t, s, EstadoConfirmada)
	seedExample(t, s, id)
	must(t, s.Delete(ctx, id, admin))
	m := rowCols(t, s, id)
	keep := map[string]bool{"id": true, "estado": true, "criada_em": true, "atualizada_em": true, "particao": true, "exemplo_publico": true}
	for c, v := range m {
		if keep[c] {
			continue
		}
		if v != nil && v != "" {
			t.Errorf("column %s kept %v", c, v)
		}
	}
	if m["estado"] != "apagada" || m["id"] != id || m["particao"] != "" || m["exemplo_publico"].(int64) != 0 {
		t.Errorf("minimal row: %v", m)
	}
	if n := count(t, s, "ocorrencia_jobs"); n != 0 {
		t.Errorf("jobs left: %d", n)
	}
	if n := count(t, s, "amostras"); n != 0 {
		t.Errorf("samples left: %d", n)
	}
	if n := count(t, s, "embeddings"); n != 0 {
		t.Errorf("embeddings left: %d", n)
	}
}

func TestDeleteRespectsTheReferenceCount(t *testing.T) { // C-6.4
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	f.add(3)
	a := ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "final", "primario")), "", f).OcorrenciaID
	b := ingest(t, s, report(jr(1, "map1", "primario"), jr(3, "final", "primario")), "", f).OcorrenciaID
	must(t, s.Delete(ctx, a, admin))
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM amostras WHERE job_id=?`, mkID(1)).Scan(&n)
	if n != 1 {
		t.Error("a sample still referenced by another occurrence was removed")
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM amostras WHERE job_id=?`, mkID(2)).Scan(&n)
	if n != 0 {
		t.Error("an unreferenced sample stayed")
	}
	if count(t, s, "amostras") != 2 {
		t.Errorf("amostras = %d", count(t, s, "amostras"))
	}
	must(t, s.Delete(ctx, b, admin))
	if count(t, s, "amostras") != 0 {
		t.Errorf("after the last reference: amostras = %d", count(t, s, "amostras"))
	}
}

// ---- history ------------------------------------------------------------------------

var secrets = []string{"SEGREDO-DESC-7", "SEGREDO-RESP-7", "SEGREDO-CORR-7", "SEGREDO-PEDIDO-7", "SEGREDO-RESPMODELO-7", "SEGREDO-NOVA-7"}

func dumpTable(t *testing.T, s *Store, table string) string {
	t.Helper()
	rows, err := s.db.Query(`SELECT * FROM ` + table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var sb strings.Builder
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		for _, v := range vals {
			switch x := v.(type) {
			case []byte:
				sb.Write(x)
			default:
				fmt.Fprint(&sb, x)
			}
			sb.WriteByte('|')
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestHistoryNeverHoldsText(t *testing.T) { // C-7.4, C-7.6, C-7.8
	s := openStore(t, Options{})
	f := newFake()
	id1 := f.add(1)
	f.conts[id1] = Content{Prompt: "SEGREDO-PEDIDO-7", Response: "SEGREDO-RESPMODELO-7", PromptJSON: []byte(`{"prompt":"SEGREDO-PEDIDO-7"}`), RespFinal: true}
	r := report(jr(1, "unico", "primario"))
	r.Descricao, r.RespostaCorreta, r.CorrecaoAplicada = "SEGREDO-DESC-7", "SEGREDO-RESP-7", "SEGREDO-CORR-7"
	id := ingest(t, s, r, "", f).OcorrenciaID
	nova := "SEGREDO-NOVA-7"
	must(t, s.Confirm(ctx, id, admin))
	if _, err := s.Update(ctx, id, Edit{RespostaCorreta: &nova}, admin); err != nil { // desconfirma
		t.Fatal(err)
	}
	must(t, s.Confirm(ctx, id, admin))
	must(t, s.Discard(ctx, id, admin))
	// the whole history after a full cycle, apagou still to come
	h := dumpTable(t, s, "historico")
	for _, sec := range secrets {
		if strings.Contains(h, sec) {
			t.Errorf("historico holds %q:\n%s", sec, h)
		}
	}
	must(t, s.Delete(ctx, id, admin))
	h = dumpTable(t, s, "historico")
	for _, sec := range secrets {
		if strings.Contains(h, sec) {
			t.Errorf("historico holds %q after apagou:\n%s", sec, h)
		}
	}
	// after erasing, no table holds a secret
	for _, tb := range []string{"ocorrencias", "ocorrencia_jobs", "amostras", "historico", "embeddings", "retratos", "retrato_itens"} {
		d := dumpTable(t, s, tb)
		for _, sec := range secrets {
			if strings.Contains(d, sec) {
				t.Errorf("%s still holds %q after the deletion", tb, sec)
			}
		}
	}
	// the actions that must be there
	want := map[string]int{"criou": 1, "confirmou": 2, "desconfirmou": 1, "descartou": 1, "apagou": 1}
	for acao, n := range want {
		var got int
		s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao=?`, id, acao).Scan(&got)
		if got != n {
			t.Errorf("acao %s: %d rows, want %d", acao, got, n)
		}
	}
}

func TestHistoryHoldsHMACAndSizeOfContentFields(t *testing.T) { // C-7.4 (1.1.2)
	s := openStore(t, Options{})
	k := testKey(t)
	f := newFake()
	f.add(1)
	r := report(jr(1, "unico", "primario"))
	r.RespostaCorreta, r.Descricao, r.CorrecaoAplicada = "antes", "SEGREDO-DESC-7", "SEGREDO-CORR-7"
	id := ingest(t, s, r, "", f).OcorrenciaID
	nv := "depois do ajuste"
	if _, err := s.Update(ctx, id, Edit{RespostaCorreta: &nv}, admin); err != nil {
		t.Fatal(err)
	}
	var antes, depois string
	if err := s.db.QueryRow(`SELECT antes, depois FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='resposta_correta'`, id).Scan(&antes, &depois); err != nil {
		t.Fatal(err)
	}
	if want := "hmac=" + k.MACString("antes") + " key_id=" + k.ID() + " bytes=5"; antes != want {
		t.Errorf("antes = %q, want %q", antes, want)
	}
	if want := "hmac=" + k.MACString(nv) + " key_id=" + k.ID() + " bytes=" + fmt.Sprint(len(nv)); depois != want {
		t.Errorf("depois = %q, want %q", depois, want)
	}
	var criou string
	s.db.QueryRow(`SELECT depois FROM historico WHERE ocorrencia_id=? AND acao='criou'`, id).Scan(&criou)
	for _, txt := range []string{"SEGREDO-DESC-7", "antes", "SEGREDO-CORR-7"} {
		if !strings.Contains(criou, "hmac="+k.MACString(txt)+" key_id="+k.ID()) {
			t.Errorf("the creation row lacks the HMAC of %q: %q", txt, criou)
		}
	}
	if !regexp.MustCompile(`^hmac=[0-9a-f]{64} key_id=[0-9a-f]{16} bytes=\d+$`).MatchString(depois) {
		t.Errorf("format: %q", depois)
	}
	// the plain sha256 of a content text is in no table, in any form
	for _, tb := range []string{"ocorrencias", "ocorrencia_jobs", "amostras", "historico", "embeddings", "retratos", "retrato_itens", "cota_grupos"} {
		d := dumpTable(t, s, tb)
		for _, txt := range []string{"antes", nv, "SEGREDO-DESC-7", "SEGREDO-CORR-7"} {
			sum := sha256.Sum256([]byte(txt))
			if strings.Contains(strings.ToLower(d), hex.EncodeToString(sum[:])) {
				t.Errorf("%s holds the plain sha256 of %q", tb, txt)
			}
		}
	}
	// a store opened without the key keeps only the size, never a hash
	s2 := openStore(t, Options{})
	s2.opts.Key = nil
	id2 := occIn(t, s2, EstadoNova)
	if _, err := s2.Update(ctx, id2, Edit{Descricao: &nv}, admin); err != nil {
		t.Fatal(err)
	}
	var d2 string
	s2.db.QueryRow(`SELECT depois FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='descricao'`, id2).Scan(&d2)
	if d2 != "bytes="+fmt.Sprint(len(nv)) {
		t.Errorf("no key: %q", d2)
	}
}

func TestHistoryIsWrittenInTheSameTransaction(t *testing.T) { // C-7.3
	s := openStore(t, Options{})
	id := occIn(t, s, EstadoNova)
	n := count(t, s, "historico")
	if _, err := s.db.Exec(`CREATE TRIGGER falha_hist BEFORE INSERT ON historico WHEN NEW.acao IN ('confirmou','alterou','descartou','apagou') BEGIN SELECT RAISE(ABORT,'sem historico'); END`); err != nil {
		t.Fatal(err)
	}
	nv := "x"
	if err := s.Confirm(ctx, id, admin); err == nil {
		t.Error("Confirm succeeded without its history row")
	}
	if _, err := s.Update(ctx, id, Edit{Descricao: &nv}, admin); err == nil {
		t.Error("Update succeeded without its history row")
	}
	if err := s.Discard(ctx, id, admin); err == nil {
		t.Error("Discard succeeded without its history row")
	}
	if err := s.Delete(ctx, id, admin); err == nil {
		t.Error("Delete succeeded without its history row")
	}
	if estadoOf(t, s, id) != EstadoNova || count(t, s, "historico") != n {
		t.Error("a change stayed without its history row")
	}
	var d string
	s.db.QueryRow(`SELECT descricao FROM ocorrencias WHERE id=?`, id).Scan(&d)
	if d == "x" || d == "" {
		t.Errorf("a field changed without its history row: %q", d)
	}
	// and the creation: a report whose history row fails leaves no occurrence
	s2 := openStore(t, Options{})
	s2.db.Exec(`CREATE TRIGGER falha_criou BEFORE INSERT ON historico BEGIN SELECT RAISE(ABORT,'sem historico'); END`)
	f := newFake()
	f.add(1)
	if _, err := s2.Ingest(ctx, mustValid(t, report(jr(1, "unico", "primario"))), stationO, "", f, testKey(t)); err == nil {
		t.Error("Ingest succeeded without the history row")
	}
	if count(t, s2, "ocorrencias") != 0 || count(t, s2, "amostras") != 0 {
		t.Error("an occurrence stayed without its history row")
	}
}

func TestHistoryActorAndIP(t *testing.T) { // C-7.5
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	r := report(jr(1, "unico", "primario"))
	r.RespostaCorreta = "ok"
	id := ingest(t, s, r, "", f).OcorrenciaID
	must(t, s.Confirm(ctx, id, admin))
	rows, err := s.db.Query(`SELECT acao, quem, ip, quando FROM historico WHERE ocorrencia_id=? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type h struct {
		acao, quem, ip string
		quando         int64
	}
	var got []h
	for rows.Next() {
		var x h
		rows.Scan(&x.acao, &x.quem, &x.ip, &x.quando)
		got = append(got, x)
	}
	if len(got) == 3 && got[2].acao == "alterou" && got[2].quem == "sistema" { // the partition of the group (12.6)
		got = got[:2]
	}
	if len(got) != 2 || got[0].acao != "criou" || got[0].quem != "PC01/ana" || got[0].ip != "10.0.0.7" ||
		got[1].acao != "confirmou" || got[1].quem != "admin" || got[1].ip != "10.0.0.9" || got[1].quando <= got[0].quando {
		t.Errorf("history = %+v", got)
	}
	for in, want := range map[Origin]string{
		{Computer: "PC", User: "u"}: "PC/u", {Computer: "PC"}: "PC", {User: "u"}: "u", {ClientLabel: "rotulo"}: "rotulo", {}: "desconhecido",
	} {
		if in.Who() != want {
			t.Errorf("Who(%+v) = %q, want %q", in, in.Who(), want)
		}
	}
}

func TestHistoryNeverExpires(t *testing.T) { // C-7.9
	s := openStore(t, Options{})
	id := occIn(t, s, EstadoConfirmada)
	must(t, s.Delete(ctx, id, admin))
	n := count(t, s, "historico")
	if n < 3 {
		t.Fatalf("historico = %d", n)
	}
	path := s.path
	s.Close()
	s2, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got := count(t, s2, "historico"); got != n {
		t.Errorf("history after apagou and reopen: %d, want %d", got, n)
	}
	var m int
	s2.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=?`, id).Scan(&m)
	if m != n {
		t.Errorf("rows of the erased occurrence: %d of %d", m, n)
	}
}

func TestHistoryActionsAreClosed(t *testing.T) { // C-7.2, C-1.8
	s := openStore(t, Options{})
	if _, err := s.db.Exec(`INSERT INTO historico(quando,ocorrencia_id,acao) VALUES(1,'x','inventada')`); err == nil {
		t.Error("an action outside C-7.2 was accepted")
	}
	for _, a := range []string{"criou", "alterou", "confirmou", "desconfirmou", "descartou", "apagou", "exportou"} {
		if _, err := s.db.Exec(`INSERT INTO historico(quando,ocorrencia_id,acao) VALUES(1,'x',?)`, a); err != nil {
			t.Errorf("%s refused: %v", a, err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado) VALUES('x',1,1,'inventado')`); err == nil {
		t.Error("a state outside C-1.8 was accepted")
	}
	if _, err := s.db.Exec(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado,particao) VALUES('x',1,1,'nova','pendente')`); err == nil {
		t.Error("a partition outside C-1.8 was accepted")
	}
	if _, err := s.db.Exec(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado) VALUES('x',1,1,'nova')`); err != nil {
		t.Errorf("an empty (pending) partition must be allowed: %v", err)
	}
}
