package treino

// Fase 12, etapa 12.6: export predicate (chapter 3), groups, quota, partition and
// quarantine (chapter 4), public example (C-6.6), admin ingestion, summary, list
// and detail.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"
)

// newEl reports a job as an error of the given category, with a correct answer.
// The job has its own content, so its group is its own.
func newEl(t *testing.T, s *Store, f *fakeSrc, n int, cat string) string {
	t.Helper()
	if _, ok := f.jobs[mkID(n)]; !ok {
		f.add(n)
	}
	r := report(jr(n, "unico", "primario"))
	r.Categoria, r.RespostaCorreta = cat, fmt.Sprintf("resposta certa %d", n)
	return ingest(t, s, r, "", f).OcorrenciaID
}

func sideOf(t *testing.T, s *Store, id string) string {
	t.Helper()
	var p string
	if err := s.db.QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, id).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func gidOf(t *testing.T, s *Store, id string) string {
	t.Helper()
	var g string
	if err := s.db.QueryRow(`SELECT COALESCE(grupo_id,'') FROM ocorrencias WHERE id=?`, id).Scan(&g); err != nil {
		t.Fatal(err)
	}
	return g
}

func exportLines(t *testing.T, s *Store, parte string) []string {
	t.Helper()
	var b bytes.Buffer
	if _, err := s.Export(ctx, parte, admin, &b, nil); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ---- C-3.1 to C-3.10, C-3.16: one condition at a time ------------------------------------------------

func TestExportPredicateEachConditionAlone(t *testing.T) { // C-3.16
	cases := []struct{ name, sql string }{
		{"C-3.2 nova", `UPDATE ocorrencias SET estado='nova'`},
		{"C-3.2 descartada", `UPDATE ocorrencias SET estado='descartada'`},
		{"C-3.2 sem_pedido", `UPDATE ocorrencias SET estado='sem_pedido'`},
		{"C-3.2/3.9 distribuida", `UPDATE ocorrencias SET estado='distribuida'`},
		{"C-3.2/3.9 fallback", `UPDATE ocorrencias SET estado='fallback'`},
		{"C-3.3 sem amostra do job_do_erro", `DELETE FROM amostras`},
		{"C-3.4 resposta vazia", `UPDATE ocorrencias SET resposta_correta=''`},
		{"C-3.5 embeddings", `UPDATE amostras SET endpoint='/api/embeddings'`},
		{"C-3.5 status", `UPDATE amostras SET status='erro'`},
		{"C-3.5 http", `UPDATE amostras SET http_status=500`},
		{"C-3.6 prompt truncado", `UPDATE amostras SET prompt_truncado=1`},
		{"C-3.6 prompt_json truncado", `UPDATE amostras SET prompt_json_truncado=1`},
		{"C-3.6 resposta nao final", `UPDATE amostras SET resp_final=0`},
		{"C-3.6 resposta truncada", `UPDATE amostras SET resp_truncada=1`},
		{"C-3.7 tool", `UPDATE ocorrencias SET tool='embed'`},
		{"C-3.8 imagem", `UPDATE amostras SET tem_imagem=1`},
		{"C-3.6 sem pedido canonico", `UPDATE amostras SET prompt_json=NULL`},
		{"C-3.9 quarentena", `UPDATE ocorrencias SET particao='quarentena'`},
		{"C-3.10 translate montado (final)", `UPDATE ocorrencias SET tool='translate'; UPDATE ocorrencia_jobs SET papel='final'`},
		{"C-3.10 translate (reduce)", `UPDATE ocorrencias SET tool='translate'; UPDATE ocorrencia_jobs SET papel='reduce1'`},
	}
	for _, c := range cases {
		s := openStore(t, Options{})
		f := newFake()
		id := newEl(t, s, f, 1, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		if got := exportLines(t, s, ParteTeste); len(got) != 1 {
			t.Fatalf("%s: the baseline is not exportable (%d lines, side %q)", c.name, len(got), sideOf(t, s, id))
		}
		for _, q := range strings.Split(c.sql, ";") {
			if _, err := s.db.Exec(q); err != nil {
				t.Fatal(c.name, err)
			}
		}
		for _, parte := range []string{ParteTeste, ParteTreino} {
			if got := exportLines(t, s, parte); len(got) != 0 {
				t.Errorf("%s: exported in %s: %v", c.name, parte, got)
			}
		}
	}
	// the translate cases that DO export: a part (mapN) and the single call (unico)
	for _, papel := range []string{"map1", "map12", "unico"} {
		s := openStore(t, Options{})
		f := newFake()
		id := newEl(t, s, f, 1, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		s.db.Exec(`UPDATE ocorrencias SET tool='translate'`)
		s.db.Exec(`UPDATE ocorrencia_jobs SET papel=?`, papel)
		if got := exportLines(t, s, ParteTeste); len(got) != 1 {
			t.Errorf("translate %s must export: %v", papel, got)
		}
	}
	// the conditions are all met by the same row that the cases break one by one
	for _, tool := range []string{"generate", "chat", "summarize_file", "extract", "translate"} {
		s := openStore(t, Options{})
		f := newFake()
		id := newEl(t, s, f, 1, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		s.db.Exec(`UPDATE ocorrencias SET tool=?`, tool)
		s.db.Exec(`UPDATE ocorrencia_jobs SET papel='map1'`)
		if got := exportLines(t, s, ParteTeste); len(got) != 1 {
			t.Errorf("tool %s must export: %v", tool, got)
		}
	}
}

func TestStatisticalStatesCannotBeConfirmedNorExported(t *testing.T) { // C-1.14, C-3.9
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	f.add(3)
	delete(f.conts, mkID(3))
	cases := map[string]Report{
		EstadoDistrib:   report(jr(1, "map1", "fallback"), jr(2, "final", "primario")),
		EstadoSemPedido: report(jr(3, "unico", "primario")),
	}
	for want, r := range cases {
		r.RespostaCorreta = "certa"
		res := ingest(t, s, r, "", f)
		if res.Estado != want {
			t.Fatalf("%s: %+v", want, res)
		}
		if err := s.Confirm(ctx, res.OcorrenciaID, admin); !errors.Is(err, ErrTransition) {
			t.Errorf("%s confirmed: %v", want, err)
		}
	}
	s2 := openStore(t, Options{})
	r := report(jr(1, "unico", "fallback"))
	r.RespostaCorreta = "certa"
	res, err := s2.Ingest(ctx, mustValid(t, r), stationO, RoleFallback, f, testKey(t))
	if err != nil || res.Estado != EstadoFallback {
		t.Fatal(res, err)
	}
	if err := s2.Confirm(ctx, res.OcorrenciaID, admin); !errors.Is(err, ErrTransition) {
		t.Errorf("fallback confirmed: %v", err)
	}
	for _, st := range []*Store{s, s2} {
		for _, p := range []string{ParteTeste, ParteTreino} {
			if got := exportLines(t, st, p); len(got) != 0 {
				t.Errorf("exported %v", got)
			}
		}
	}
}

// ---- C-3.11 to C-3.15, C-7.7 ---------------------------------------------------------------------------

func TestExportFormatAndPartAndMeta(t *testing.T) { // C-3.11, C-3.13, C-3.14, C-3.15
	s := openStore(t, Options{})
	f := newFake()
	// a generate job with a system prompt
	id1 := f.add(1)
	c := f.conts[id1]
	c.PromptJSON = []byte(`{"model":"qwen3-ia","system":"Seja literal.","prompt":"liste os itens","options":{"seed":1}}`)
	f.conts[id1] = c
	// a chat job with history
	id2 := f.add(2)
	j := f.jobs[id2]
	j.Endpoint, j.Tool = "/api/chat", "chat"
	f.jobs[id2] = j
	c = f.conts[id2]
	c.PromptJSON = []byte(`{"model":"qwen3-ia","messages":[{"role":"system","content":"S"},{"role":"user","content":"u1"},{"role":"assistant","content":"a1"},{"role":"user","content":"u2"}]}`)
	f.conts[id2] = c
	o1, o2 := newEl(t, s, f, 1, "omitiu"), newEl(t, s, f, 2, "inventou")
	must(t, s.Confirm(ctx, o1, admin))
	must(t, s.Confirm(ctx, o2, admin))
	s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, o2)
	lt := exportLines(t, s, ParteTeste)
	if len(lt) != 1 {
		t.Fatalf("teste: %v", lt)
	}
	var line struct {
		Messages []map[string]string `json:"messages"`
		Meta     map[string]any      `json:"meta"`
	}
	if err := json.Unmarshal([]byte(lt[0]), &line); err != nil {
		t.Fatal(err)
	}
	if len(line.Messages) != 3 || line.Messages[0]["role"] != "system" || line.Messages[0]["content"] != "Seja literal." ||
		line.Messages[1]["role"] != "user" || line.Messages[1]["content"] != "liste os itens" ||
		line.Messages[2]["role"] != "assistant" || line.Messages[2]["content"] != "resposta certa 1" {
		t.Errorf("generate line: %v", line.Messages)
	}
	keys := []string{}
	for k := range line.Meta {
		keys = append(keys, k)
	}
	if len(keys) != 6 {
		t.Errorf("meta has %v, want exactly ocorrencia_id, job_id, papel, tool, categoria, particao", keys)
	}
	for k, want := range map[string]string{"ocorrencia_id": o1, "job_id": id1, "papel": "unico", "tool": "extract", "categoria": "omitiu", "particao": "teste"} {
		if line.Meta[k] != want {
			t.Errorf("meta.%s = %v, want %s", k, line.Meta[k], want)
		}
	}
	tr := exportLines(t, s, ParteTreino)
	if len(tr) != 1 {
		t.Fatalf("treino: %v", tr)
	}
	json.Unmarshal([]byte(tr[0]), &line)
	if len(line.Messages) != 5 || line.Messages[3]["content"] != "u2" || line.Messages[4]["role"] != "assistant" || line.Messages[4]["content"] != "resposta certa 2" ||
		line.Messages[1]["content"] != "u1" || line.Messages[2]["content"] != "a1" || line.Meta["particao"] != "treino" {
		t.Errorf("chat line (whole history, last = the answer): %v %v", line.Messages, line.Meta)
	}
	if _, err := s.Export(ctx, "quarentena", admin, io.Discard, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("the quarantine cannot be exported: %v", err)
	}
	if _, err := s.Export(ctx, "tudo", admin, io.Discard, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown part: %v", err)
	}
}

func TestExportRecordsOneRowPerExportedOccurrence(t *testing.T) { // C-7.7
	s := openStore(t, Options{})
	f := newFake()
	n := exportBatch + 5 // across a batch boundary
	var ids []string
	for i := 1; i <= n; i++ {
		id := newEl(t, s, f, i, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, id)
		ids = append(ids, id)
	}
	// one that is not exportable (image) must not be recorded
	f.add(500)
	cimg := f.conts[mkID(500)]
	_ = cimg
	bad := newEl(t, s, f, 500, "omitiu")
	must(t, s.Confirm(ctx, bad, admin))
	s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, bad)
	s.db.Exec(`UPDATE amostras SET tem_imagem=1 WHERE job_id=?`, mkID(500))
	who := Actor{Who: "admin", IP: "10.0.0.9"}
	var buf bytes.Buffer
	got, err := s.Export(ctx, ParteTreino, who, &buf, nil)
	if err != nil || got != n {
		t.Fatalf("exported %d: %v", got, err)
	}
	for _, id := range ids {
		var rows, other int
		s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='exportou' AND campo='treino' AND quem='admin' AND ip='10.0.0.9'`, id).Scan(&rows)
		s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='exportou'`, id).Scan(&other)
		if rows != 1 || other != 1 {
			t.Errorf("%s: %d matching rows, %d exportou rows", id, rows, other)
		}
	}
	var nb int
	s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='exportou'`, bad).Scan(&nb)
	if nb != 0 {
		t.Error("an occurrence that was not exported was recorded")
	}
	// an export that fails midway records only what was written
	s2 := openStore(t, Options{})
	f2 := newFake()
	for i := 1; i <= 5; i++ {
		id := newEl(t, s2, f2, i, "omitiu")
		must(t, s2.Confirm(ctx, id, admin))
		s2.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, id)
	}
	w := &failingWriter{failAfter: 2}
	wrote, err := s2.Export(ctx, ParteTreino, admin, w, nil)
	if err == nil || wrote != 2 {
		t.Fatalf("failing writer: %d %v", wrote, err)
	}
	var rec int
	s2.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE acao='exportou'`).Scan(&rec)
	if rec < 2 { // write-ahead: the batch is recorded before it is sent, so a failed delivery leaves extra rows
		t.Errorf("recorded %d exportou rows for 2 lines written", rec)
	}
}

type failingWriter struct {
	n, failAfter int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.n >= w.failAfter {
		return 0, errors.New("cliente saiu")
	}
	w.n++
	return len(p), nil
}

// C-6.10: a slow reader of the export never holds the writers' turn, so a report
// (and every admin action) goes through while the stream is stuck.
func TestExportDoesNotBlockReports(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	for i := 1; i <= 3; i++ {
		id := newEl(t, s, f, i, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, id)
	}
	f.add(9)
	gate := make(chan struct{})
	started := make(chan struct{})
	w := &blockingWriter{gate: gate, started: started}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Export(ctx, ParteTreino, admin, w, nil)
	}()
	<-started // the stream is stuck inside Write
	dctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	t0 := time.Now()
	if _, err := s.Ingest(dctx, mustValid(t, report(jr(9, "unico", "primario"))), stationO, "", f, testKey(t)); err != nil {
		t.Errorf("a report while the export is stuck: %v after %v", err, time.Since(t0))
	}
	if err := s.Confirm(dctx, mustOcc(t, s, mkID(9)), admin); err != nil && !errors.Is(err, ErrNeedsAnswer) {
		t.Errorf("an admin action while the export is stuck: %v", err)
	}
	close(gate)
	wg.Wait()
}

func mustOcc(t *testing.T, s *Store, job string) string {
	t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT id FROM ocorrencias WHERE job_do_erro=?`, job).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type blockingWriter struct {
	gate    chan struct{}
	started chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.gate
	return len(p), nil
}

// ---- groups (C-4.5, C-4.15) ---------------------------------------------------------------------------

func TestGroupsLinkByJobDocumentAndContent(t *testing.T) { // C-4.5, C-4.15
	s := openStore(t, Options{})
	k := testKey(t)
	f := newFake()
	for i := 1; i <= 12; i++ {
		f.add(i)
	}
	// 3 and 4 share the document hash; 5 and 6 have the same text under other options
	for _, n := range []int{3, 4} {
		j := f.jobs[mkID(n)]
		j.DocHash, j.DocKeyID = strings.Repeat("a", 64), k.ID()
		f.jobs[mkID(n)] = j
	}
	c6 := f.conts[mkID(6)]
	c6.PromptJSON = []byte(`{"model":"qwen3-ia","prompt":"pedido 5","options":{"seed":99,"temperature":0.1}}`)
	f.conts[mkID(6)] = c6
	r := func(jobs ...JobRef) string {
		x := report(jobs...)
		x.Origem = "claude"
		return ingest(t, s, x, "", f).OcorrenciaID
	}
	a := r(jr(1, "map1", "primario"), jr(2, "final", "primario")) // 1 and 2
	b := r(jr(1, "map1", "primario"), jr(7, "final", "primario")) // shares job 1 with a
	d3, d4 := r(jr(3, "unico", "primario")), r(jr(4, "unico", "primario"))
	c5, c6o := r(jr(5, "unico", "primario")), r(jr(6, "unico", "primario"))
	lone := r(jr(8, "unico", "primario"))
	// job 10 has the document hash too, but no occurrence has it as its job_do_erro or in common with others:
	// only the hash of the sample links this part to the group of the document
	j10 := f.jobs[mkID(10)]
	j10.DocHash, j10.DocKeyID = strings.Repeat("a", 64), k.ID()
	f.jobs[mkID(10)] = j10
	viaSample := r(jr(10, "map1", "primario"), jr(9, "final", "primario"))
	if gidOf(t, s, viaSample) != gidOf(t, s, d3) {
		t.Error("the document hash of a part (the sample) did not link")
	}
	if gidOf(t, s, a) != gidOf(t, s, b) {
		t.Error("a job in common did not link")
	}
	if gidOf(t, s, d3) != gidOf(t, s, d4) {
		t.Error("the same document hash did not link")
	}
	if gidOf(t, s, c5) != gidOf(t, s, c6o) {
		t.Error("the same content hash (other options) did not link (C-4.15)")
	}
	if g := gidOf(t, s, lone); g == gidOf(t, s, a) || g == gidOf(t, s, d3) || g == gidOf(t, s, c5) {
		t.Error("an unrelated occurrence joined a group")
	}
	// the same hash under another key id does not link (C-4.3a)
	s.db.Exec(`UPDATE amostras SET key_id='0000000000000000' WHERE job_id=?`, mkID(4))
	s.db.Exec(`UPDATE ocorrencias SET doc_key_id='0000000000000000' WHERE job_do_erro=?`, mkID(4))
	must(t, s.withTx(ctx, func(tx *sql.Tx) error { return s.recompute(ctx, tx, s.opts.Now()) }))
	if gidOf(t, s, d3) == gidOf(t, s, d4) {
		t.Error("hashes of different keys were compared")
	}
}

func TestDiscardedAndNewOccurrencesStayInTheGroups(t *testing.T) { // C-4.5
	s := openStore(t, Options{})
	f := newFake()
	for i := 1; i <= 4; i++ {
		f.add(i)
	}
	rep := func(jobs ...JobRef) string {
		x := report(jobs...)
		x.RespostaCorreta = "certa"
		return ingest(t, s, x, "", f).OcorrenciaID
	}
	x := rep(jr(1, "unico", "primario"))
	d := rep(jr(1, "map1", "primario"), jr(2, "final", "primario")) // links to x through job 1
	must(t, s.Discard(ctx, d, admin))
	y := rep(jr(2, "map1", "primario"), jr(3, "final", "primario")) // links to d only (job 2)
	if gidOf(t, s, x) != gidOf(t, s, y) {
		t.Error("a discarded occurrence stopped linking")
	}
	del := rep(jr(3, "map1", "primario"), jr(4, "final", "primario"))
	must(t, s.Delete(ctx, del, admin))
	z := rep(jr(4, "unico", "primario"))
	if gidOf(t, s, z) == gidOf(t, s, x) {
		t.Error("an erased occurrence still links")
	}
}

// ---- C-4.6, C-4.7: quota and pending -----------------------------------------------------------------

func TestQuotaCountsOnlyGroupsWithAnExportableSample(t *testing.T) { // C-4.6, C-1.8
	s := openStore(t, Options{})
	f := newFake()
	// 5 groups confirmed but not exportable (image): pending, no quota used
	for i := 1; i <= 5; i++ {
		id := newEl(t, s, f, i, "omitiu")
		s.db.Exec(`UPDATE amostras SET tem_imagem=1 WHERE job_id=?`, mkID(i))
		must(t, s.Confirm(ctx, id, admin))
		if p := sideOf(t, s, id); p != "" {
			t.Fatalf("a group with no exportable sample got %q", p)
		}
	}
	if n := count(t, s, "cota_grupos"); n != 0 {
		t.Fatalf("quota used by groups with no exportable sample: %d", n)
	}
	// 30 exportable groups of the category go to the test; the 31st follows the 15%
	var last string
	for i := 10; i < 10+CotaPorCategoria+1; i++ {
		id := newEl(t, s, f, i, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		last = id
		if i < 10+CotaPorCategoria && sideOf(t, s, id) != ParteTeste {
			t.Fatalf("group %d of the quota went to %q", i-9, sideOf(t, s, id))
		}
	}
	if want := hashSide(gidOf(t, s, last)); sideOf(t, s, last) != want {
		t.Errorf("after the quota: %q, the 15%% rule says %q", sideOf(t, s, last), want)
	}
	// another category has its own quota
	other := newEl(t, s, f, 100, "inventou")
	must(t, s.Confirm(ctx, other, admin))
	if sideOf(t, s, other) != ParteTeste {
		t.Error("the quota of a category took the vaga of another")
	}
	// a pending group does not become assigned by being pending
	pend := newEl(t, s, f, 200, "omitiu")
	if sideOf(t, s, pend) != "" {
		t.Errorf("an unconfirmed group has %q", sideOf(t, s, pend))
	}
}

func TestQuotaOrderIsTheOrderOfTheFirstExportableSample(t *testing.T) { // C-4.6
	s := openStore(t, Options{})
	f := newFake()
	n := CotaPorCategoria + 1
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = newEl(t, s, f, i+1, "omitiu")
	}
	for i := n - 1; i >= 0; i-- { // the last created is confirmed first
		must(t, s.Confirm(ctx, ids[i], admin))
	}
	for i := n - 1; i >= 1; i-- {
		if sideOf(t, s, ids[i]) != ParteTeste {
			t.Errorf("the %dth confirmed group is not in the quota: %q", n-i, sideOf(t, s, ids[i]))
		}
	}
	if want := hashSide(gidOf(t, s, ids[0])); sideOf(t, s, ids[0]) != want { // created first, confirmed last
		t.Errorf("the group confirmed last must follow the 15%% rule: %q want %q", sideOf(t, s, ids[0]), want)
	}
}

func TestQuotaCategoryIsThatOfTheFirstExportableSample(t *testing.T) { // C-4.6
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	ra := report(jr(1, "unico", "primario"))
	ra.Categoria, ra.RespostaCorreta = "omitiu", "certa"
	rb := ra
	rb.Categoria = "inventou"
	a := ingest(t, s, ra, "", f).OcorrenciaID
	b := ingest(t, s, rb, "", f).OcorrenciaID // same job: same group
	must(t, s.Confirm(ctx, b, admin))         // the first exportable one
	must(t, s.Confirm(ctx, a, admin))
	var cat string
	if err := s.db.QueryRow(`SELECT categoria FROM cota_grupos`).Scan(&cat); err != nil || cat != "inventou" || count(t, s, "cota_grupos") != 1 {
		t.Errorf("category of the group in the quota: %q %v", cat, err)
	}
}

func TestQuotaSlotIsNotFreedByDiscardUnconfirmOrDelete(t *testing.T) { // C-4.11a, C-4.11b
	s := openStore(t, Options{})
	f := newFake()
	var members []string
	for i := 1; i <= CotaPorCategoria; i++ {
		id := newEl(t, s, f, i, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
		members = append(members, id)
	}
	// seed a snapshot that lists three of them
	for _, q := range []string{`INSERT INTO retratos(id,criado_em,valido) VALUES('r1',1,1),('r2',1,1),('r3',1,1)`} {
		s.db.Exec(q)
	}
	for i, r := range []string{"r1", "r2", "r3"} {
		s.db.Exec(`INSERT INTO retrato_itens(retrato_id,ocorrencia_id,job_id) VALUES(?,?,'j')`, r, members[i])
	}
	nv := "outra"
	if _, err := s.Update(ctx, members[0], Edit{RespostaCorreta: &nv}, admin); err != nil { // unconfirm
		t.Fatal(err)
	}
	must(t, s.Discard(ctx, members[1], admin))
	must(t, s.Delete(ctx, members[2], admin))
	if n := count(t, s, "cota_grupos"); n != CotaPorCategoria {
		t.Errorf("the quota has %d rows after freeing attempts, want %d", n, CotaPorCategoria)
	}
	for _, r := range []string{"r1", "r2", "r3"} {
		if valid(t, s, r) != 0 {
			t.Errorf("snapshot %s still valid", r)
		}
	}
	// a new group that the 15% rule sends to the training part: if a vaga had been freed it would be a test case
	var cand string
	for i := 1000; i < 1100; i++ {
		id := newEl(t, s, f, i, "omitiu")
		if hashSide(gidOf(t, s, id)) == ParteTreino {
			cand = id
			break
		}
		must(t, s.Delete(ctx, id, admin))
	}
	if cand == "" {
		t.Skip("no candidate group for the training side in 100 tries")
	}
	must(t, s.Confirm(ctx, cand, admin))
	if sideOf(t, s, cand) != ParteTreino {
		t.Errorf("a vaga was freed: the new group went to %q", sideOf(t, s, cand))
	}
}

// ---- stability, fusion, quarantine (C-4.8 to C-4.11) -------------------------------------------------------

func TestPartitionIsStable(t *testing.T) { // C-4.8
	s := openStore(t, Options{})
	f := newFake()
	a := newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, a, admin))
	side, g := sideOf(t, s, a), gidOf(t, s, a)
	for i := 2; i <= 40; i++ { // many other groups come and go, the quota fills
		id := newEl(t, s, f, i, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
	}
	nv := "ajuste"
	if _, err := s.Update(ctx, a, Edit{Descricao: &nv}, admin); err != nil { // unconfirm and reconfirm
		t.Fatal(err)
	}
	must(t, s.Confirm(ctx, a, admin))
	if sideOf(t, s, a) != side || gidOf(t, s, a) != g {
		t.Errorf("the group changed: %q/%q -> %q/%q", side, g, sideOf(t, s, a), gidOf(t, s, a))
	}
}

// twoGroups makes two groups that already have their sides (A: teste, B: treino).
func twoGroups(t *testing.T, s *Store, f *fakeSrc) (a, b string) {
	t.Helper()
	a = newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, a, admin)) // the first of its category: the test
	b = newEl(t, s, f, 2, "inventou")
	must(t, s.Confirm(ctx, b, admin))
	if _, err := s.db.Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, b); err != nil {
		t.Fatal(err)
	}
	if sideOf(t, s, a) != ParteTeste || sideOf(t, s, b) != ParteTreino {
		t.Fatalf("setup: %q %q", sideOf(t, s, a), sideOf(t, s, b))
	}
	return
}

func TestFusionOfTheSameSideKeepsTheSmallestGroupAndDoesNotReassign(t *testing.T) { // C-4.8
	s := openStore(t, Options{})
	f := newFake()
	a := newEl(t, s, f, 1, "omitiu")
	must(t, s.Confirm(ctx, a, admin))
	b := newEl(t, s, f, 2, "inventou")
	must(t, s.Confirm(ctx, b, admin)) // also the first of its category: teste
	if sideOf(t, s, a) != ParteTeste || sideOf(t, s, b) != ParteTeste {
		t.Fatal("setup")
	}
	ga, gb := gidOf(t, s, a), gidOf(t, s, b)
	quota := count(t, s, "cota_grupos")
	x := report(jr(1, "map1", "primario"), jr(2, "final", "primario"), jr(1, "map2", "primario"))
	x.Jobs = x.Jobs[:2]
	x.Origem = "claude"
	c := ingest(t, s, x, "", f).OcorrenciaID // links the two groups
	want := ga
	if gb < ga {
		want = gb
	}
	for _, id := range []string{a, b, c} {
		if gidOf(t, s, id) != want || sideOf(t, s, id) != ParteTeste {
			t.Errorf("%s: group %q side %q, want %q teste", id, gidOf(t, s, id), sideOf(t, s, id), want)
		}
	}
	if count(t, s, "cota_grupos") != quota {
		t.Error("a fusion of the same side was evaluated again by the quota")
	}
}

func TestOppositeSidesGoToQuarantineAndInvalidateSnapshots(t *testing.T) { // C-4.9a, C-4.10, C-1.15
	s := openStore(t, Options{})
	f := newFake()
	a, b := twoGroups(t, s, f)
	// snapshots: one lists a member of the quarantined component, one lists someone else
	s.db.Exec(`INSERT INTO retratos(id,criado_em,indice_hash,valido) VALUES('r-a',1,'idx',1),('r-b',1,'idx',1),('r-x',1,'idx',1)`)
	s.db.Exec(`INSERT INTO retrato_itens(retrato_id,ocorrencia_id,job_id) VALUES('r-a',?,'j'),('r-b',?,'j'),('r-x','outra','j')`, a, b)
	x := report(jr(1, "map1", "primario"), jr(2, "final", "primario"))
	x.Origem = "claude"
	c := ingest(t, s, x, "", f).OcorrenciaID
	for _, id := range []string{a, b, c} {
		if sideOf(t, s, id) != ParteQuarentena {
			t.Errorf("%s is %q, want quarentena", id, sideOf(t, s, id))
		}
	}
	if valid(t, s, "r-a") != 0 || valid(t, s, "r-b") != 0 || valid(t, s, "r-x") != 1 {
		t.Errorf("snapshots: %d %d %d", valid(t, s, "r-a"), valid(t, s, "r-b"), valid(t, s, "r-x"))
	}
	for _, p := range []string{ParteTeste, ParteTreino} {
		if got := exportLines(t, s, p); len(got) != 0 {
			t.Errorf("quarantine exported in %s: %v", p, got)
		}
	}
	var antes, depois string
	if err := s.db.QueryRow(`SELECT antes, depois FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='particao' ORDER BY id DESC LIMIT 1`, a).Scan(&antes, &depois); err != nil || antes != ParteTeste || depois != ParteQuarentena {
		t.Errorf("history of the quarantine: %v %q %q", err, antes, depois)
	}
	// the quarantine is sticky: a later report that joins it is quarantined too
	f.add(3)
	y := report(jr(3, "map1", "primario"), jr(1, "final", "primario"))
	y.Origem = "claude"
	d := ingest(t, s, y, "", f).OcorrenciaID
	if sideOf(t, s, d) != ParteQuarentena {
		t.Errorf("joining a quarantined group: %q", sideOf(t, s, d))
	}
}

func TestOneSideIsInheritedAndNoSideIsPending(t *testing.T) { // C-4.9b, C-4.9c
	s := openStore(t, Options{})
	f := newFake()
	a, _ := twoGroups(t, s, f)
	f.add(3)
	x := report(jr(3, "map1", "primario"), jr(1, "final", "primario")) // joins only A (teste)
	x.Origem = "claude"
	c := ingest(t, s, x, "", f).OcorrenciaID
	if sideOf(t, s, c) != ParteTeste || gidOf(t, s, c) != gidOf(t, s, a) {
		t.Errorf("inheritance: %q group %q", sideOf(t, s, c), gidOf(t, s, c))
	}
	f.add(4)
	z := report(jr(4, "unico", "primario"))
	z.Origem = "claude"
	p := ingest(t, s, z, "", f).OcorrenciaID // links nothing
	if sideOf(t, s, p) != "" {
		t.Errorf("a group with no side is %q", sideOf(t, s, p))
	}
	// two pending groups that merge stay pending
	f.add(5)
	w := report(jr(4, "map1", "primario"), jr(5, "final", "primario"))
	w.Origem = "claude"
	q := ingest(t, s, w, "", f).OcorrenciaID
	if sideOf(t, s, q) != "" || sideOf(t, s, p) != "" {
		t.Errorf("merged pending groups: %q %q", sideOf(t, s, q), sideOf(t, s, p))
	}
}

func TestReleaseQuarantineChoosesASideForTheWholeComponent(t *testing.T) { // C-4.11
	s := openStore(t, Options{})
	f := newFake()
	a, b := twoGroups(t, s, f)
	x := report(jr(1, "map1", "primario"), jr(2, "final", "primario"))
	x.Origem = "claude"
	c := ingest(t, s, x, "", f).OcorrenciaID
	if err := s.ReleaseQuarantine(ctx, a, "quarentena", admin); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad side: %v", err)
	}
	if err := s.ReleaseQuarantine(ctx, mkID(77), ParteTreino, admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	must(t, s.ReleaseQuarantine(ctx, c, ParteTreino, admin))
	for _, id := range []string{a, b, c} {
		if sideOf(t, s, id) != ParteTreino {
			t.Errorf("%s is %q after the release", id, sideOf(t, s, id))
		}
		var antes, depois, quem string
		if err := s.db.QueryRow(`SELECT antes, depois, quem FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='particao' ORDER BY id DESC LIMIT 1`, id).Scan(&antes, &depois, &quem); err != nil ||
			antes != ParteQuarentena || depois != ParteTreino || quem != "admin" {
			t.Errorf("%s history: %v %q %q %q", id, err, antes, depois, quem)
		}
	}
	if err := s.ReleaseQuarantine(ctx, a, ParteTeste, admin); !errors.Is(err, ErrTransition) {
		t.Errorf("releasing what is not in quarantine: %v", err)
	}
	if got := exportLines(t, s, ParteTreino); len(got) != 2 { // a and b are confirmed; c is only nova
		t.Errorf("after the release the confirmed members export in their side: %d lines", len(got))
	}
	// stays released through later changes
	f.add(9)
	y := report(jr(9, "unico", "primario"))
	y.Origem = "claude"
	ingest(t, s, y, "", f)
	if sideOf(t, s, a) != ParteTreino {
		t.Error("the release did not hold")
	}
}

// ---- C-4.12: no leakage between the parts --------------------------------------------------------------

func TestNoJobDocumentOrGroupInBothParts(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	s := openStore(t, Options{})
	k := testKey(t)
	f := newFake()
	const jobs = 170
	for i := 1; i <= jobs; i++ {
		f.add(i)
		if i%5 == 0 { // some jobs share a document
			j := f.jobs[mkID(i)]
			j.DocHash, j.DocKeyID = fmt.Sprintf("%064x", i%3), k.ID()
			f.jobs[mkID(i)] = j
		}
		if i > 150 { // some jobs repeat the text of an earlier one
			c := f.conts[mkID(i)]
			c.PromptJSON = pjOf(i - 100)
			f.conts[mkID(i)] = c
		}
	}
	var ids []string
	perm := rng.Perm(jobs)
	for i := 0; i < 140; i++ {
		jobDoErro := perm[i] + 1
		other := rng.Intn(jobs) + 1
		var list []JobRef
		if other != jobDoErro && rng.Intn(5) == 0 {
			list = []JobRef{jr(other, "map1", "primario"), jr(jobDoErro, "final", "primario")}
		} else {
			list = []JobRef{jr(jobDoErro, "unico", "primario")}
		}
		r := report(list...)
		r.Origem = "claude"
		r.Categoria = []string{"omitiu", "omitiu", "omitiu", "inventou"}[rng.Intn(4)]
		r.RespostaCorreta = "certa"
		ids = append(ids, ingest(t, s, r, "", f).OcorrenciaID)
	}
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	for _, id := range ids {
		if err := s.Confirm(ctx, id, admin); err != nil {
			t.Fatal(err)
		}
	}
	type seen struct{ job, doc, group map[string]bool }
	got := map[string]*seen{ParteTreino: {map[string]bool{}, map[string]bool{}, map[string]bool{}}, ParteTeste: {map[string]bool{}, map[string]bool{}, map[string]bool{}}}
	for _, parte := range []string{ParteTreino, ParteTeste} {
		for _, l := range exportLines(t, s, parte) {
			var line struct {
				Meta exportMeta `json:"meta"`
			}
			json.Unmarshal([]byte(l), &line)
			var doc string
			s.db.QueryRow(`SELECT COALESCE(doc_hash,'') FROM amostras WHERE job_id=?`, line.Meta.JobID).Scan(&doc)
			got[parte].job[line.Meta.JobID] = true
			if doc != "" {
				got[parte].doc[doc] = true
			}
			got[parte].group[gidOf(t, s, line.Meta.OcorrenciaID)] = true
		}
	}
	if len(got[ParteTreino].job) == 0 || len(got[ParteTeste].job) == 0 {
		t.Fatalf("the scenario must fill both parts: %d treino, %d teste", len(got[ParteTreino].job), len(got[ParteTeste].job))
	}
	for _, kind := range []string{"job", "doc", "group"} {
		a, b := map[string]map[string]bool{ParteTreino: got[ParteTreino].job, ParteTeste: got[ParteTeste].job}, map[string]map[string]bool{}
		switch kind {
		case "doc":
			a = map[string]map[string]bool{ParteTreino: got[ParteTreino].doc, ParteTeste: got[ParteTeste].doc}
		case "group":
			a = map[string]map[string]bool{ParteTreino: got[ParteTreino].group, ParteTeste: got[ParteTeste].group}
		}
		_ = b
		for v := range a[ParteTreino] {
			if a[ParteTeste][v] {
				t.Errorf("%s %s is in training and test", kind, v)
			}
		}
	}
	// the same structure checked on the stored sides: every group has one side
	rows, _ := s.db.Query(`SELECT grupo_id, COUNT(DISTINCT particao) FROM ocorrencias WHERE estado!='apagada' AND grupo_id!='' GROUP BY 1 HAVING COUNT(DISTINCT particao)>1`)
	defer rows.Close()
	for rows.Next() {
		var g string
		var n int
		rows.Scan(&g, &n)
		t.Errorf("group %s has %d sides", g, n)
	}
}

// ---- C-6.6: public example -----------------------------------------------------------------------------

func TestPublicExampleOnlyForConfirmedAndLeavesTheEmbeddingPending(t *testing.T) { // C-6.6
	s := openStore(t, Options{})
	f := newFake()
	id := newEl(t, s, f, 1, "omitiu")
	if err := s.SetExemploPublico(ctx, id, true, admin); !errors.Is(err, ErrTransition) {
		t.Errorf("an unconfirmed occurrence became an example: %v", err)
	}
	must(t, s.Confirm(ctx, id, admin))
	must(t, s.SetExemploPublico(ctx, id, true, admin))
	var ex, pend int
	s.db.QueryRow(`SELECT exemplo_publico FROM ocorrencias WHERE id=?`, id).Scan(&ex)
	s.db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE ocorrencia_id=? AND vetor IS NULL AND modelo IS NULL`, id).Scan(&pend)
	if ex != 1 || pend != 1 {
		t.Errorf("mark: exemplo_publico=%d pending embedding rows=%d", ex, pend)
	}
	var a, d string
	if err := s.db.QueryRow(`SELECT antes, depois FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='exemplo_publico'`, id).Scan(&a, &d); err != nil || a != "0" || d != "1" {
		t.Errorf("history: %v %q %q", err, a, d)
	}
	sm, _ := s.Summary(ctx)
	if sm.EmbPendentes != 1 {
		t.Errorf("summary pending embeddings = %d", sm.EmbPendentes)
	}
	must(t, s.SetExemploPublico(ctx, id, false, admin))
	s.db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE ocorrencia_id=?`, id).Scan(&pend)
	if pend != 0 {
		t.Error("unmarking left the embedding row")
	}
	must(t, s.SetExemploPublico(ctx, id, false, admin)) // no change: no error, no row
	// discarding a marked one clears it (C-1.12b)
	must(t, s.SetExemploPublico(ctx, id, true, admin))
	must(t, s.Discard(ctx, id, admin))
	s.db.QueryRow(`SELECT exemplo_publico FROM ocorrencias WHERE id=?`, id).Scan(&ex)
	s.db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE ocorrencia_id=?`, id).Scan(&pend)
	if ex != 0 || pend != 0 {
		t.Errorf("after discard: %d %d", ex, pend)
	}
	if err := s.SetExemploPublico(ctx, mkID(77), true, admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

// ---- admin report ("Marcar erro") ---------------------------------------------------------------------

func TestAdminReportHasNoSameOriginRestriction(t *testing.T) { // C-1.11a (admin)
	s := openStore(t, Options{})
	f := newFake()
	id := f.add(1)
	j := f.jobs[id]
	j.StationComputer, j.StationUser, j.ClientIP = "OUTRO", "bob", "10.9.9.9" // another origin than the admin's browser
	f.jobs[id] = j
	r := report(jr(1, "unico", "primario"))
	r.Origem = "usuario"
	adminOrigin := Origin{ClientIP: "10.0.0.50", ClientLabel: "admin"}
	// the same report from a station of another origin has no sample
	if res := ingestAs(t, s, r, adminOrigin, f); res.Estado != EstadoSemPedido || res.JobsOutraOrigem != 1 {
		t.Fatalf("without the option: %+v", res)
	}
	s2 := openStore(t, Options{})
	res, err := s2.IngestWith(ctx, mustValid(t, r), adminOrigin, "", f, testKey(t), IngestOptions{Admin: true})
	if err != nil || res.Estado != EstadoNova || res.AmostrasCopiadas != 1 || res.JobsOutraOrigem != 0 {
		t.Fatalf("admin: %+v %v", res, err)
	}
	var origem, quem string
	s2.db.QueryRow(`SELECT origem FROM ocorrencias WHERE id=?`, res.OcorrenciaID).Scan(&origem)
	s2.db.QueryRow(`SELECT quem FROM historico WHERE ocorrencia_id=? AND acao='criou'`, res.OcorrenciaID).Scan(&quem)
	if origem != "usuario" || quem != "admin" {
		t.Errorf("origem=%q quem=%q", origem, quem)
	}
	// it replaces an automatic candidate of another origin, which a station could not
	s3 := openStore(t, Options{})
	auto := report(jr(1, "unico", "primario"))
	auto.Origem = "automatico"
	oa := Origin{Computer: "OUTRO", User: "bob", ClientIP: "10.9.9.9"}
	ra := ingestAs(t, s3, auto, oa, f)
	rb, err := s3.IngestWith(ctx, mustValid(t, r), adminOrigin, "", f, testKey(t), IngestOptions{Admin: true})
	if err != nil || rb.OcorrenciaID != ra.OcorrenciaID {
		t.Errorf("the admin report did not replace the candidate: %+v %v", rb, err)
	}
}

// ---- summary, list, detail ---------------------------------------------------------------------------

func TestSummaryListAndDetail(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	a := newEl(t, s, f, 1, "omitiu")
	b := newEl(t, s, f, 2, "inventou")
	must(t, s.Confirm(ctx, a, admin))
	must(t, s.Discard(ctx, b, admin))
	c := newEl(t, s, f, 3, "omitiu")
	del := newEl(t, s, f, 4, "omitiu")
	must(t, s.Delete(ctx, del, admin))
	sm, err := s.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sm.Total != 3 || sm.PorEstado[EstadoConfirmada] != 1 || sm.PorEstado[EstadoDescartada] != 1 || sm.PorEstado[EstadoNova] != 1 || sm.PorEstado[EstadoApagada] != 0 {
		t.Errorf("by state: %+v", sm.PorEstado)
	}
	if sm.PorCategoria["omitiu"] != 2 || sm.PorCategoria["inventou"] != 0 { // descartada is out of the error statistics (C-1.14)
		t.Errorf("by category: %+v", sm.PorCategoria)
	}
	if sm.ExportTeste != 1 || sm.ExportTreino != 0 || sm.MaxBytes != DefaultMaxBytes || sm.SizeBytes <= 0 || sm.PorParticao[ParteTeste] != 1 || sm.PorParticao["pendente"] != 2 {
		t.Errorf("summary: %+v", sm)
	}
	b2, _ := json.Marshal(sm)
	for _, secret := range []string{"resposta certa", "pulou a secao"} {
		if strings.Contains(string(b2), secret) {
			t.Errorf("the summary holds text: %s", secret)
		}
	}
	rows, err := s.List(ctx, Filter{})
	if err != nil || len(rows) != 3 {
		t.Fatalf("list: %d %v", len(rows), err)
	}
	for name, f := range map[string]Filter{"categoria": {Categoria: "inventou"}, "estado": {Estado: EstadoConfirmada}, "particao": {Particao: ParteTeste},
		"pendente": {Particao: "pendente"}, "tool": {Tool: "extract"}, "estacao": {Estacao: "ana"}, "limite": {Limit: 1}} {
		got, err := s.List(ctx, f)
		if err != nil {
			t.Fatal(name, err)
		}
		want := map[string]int{"categoria": 1, "estado": 1, "particao": 1, "pendente": 2, "tool": 3, "estacao": 3, "limite": 1}[name]
		if len(got) != want {
			t.Errorf("filter %s: %d rows, want %d", name, len(got), want)
		}
	}
	if got, _ := s.List(ctx, Filter{Estacao: "ninguem"}); len(got) != 0 {
		t.Error("the station filter matched nobody and returned rows")
	}
	if got, _ := s.List(ctx, Filter{Tool: "outra"}); len(got) != 0 {
		t.Error("a filter that matches nothing returned rows")
	}
	if got, _ := s.List(ctx, Filter{Desde: time.Now().Add(time.Hour).UnixMilli()}); len(got) != 0 {
		t.Error("period filter")
	}
	d, err := s.Detail(ctx, a)
	if err != nil || d.ID != a || len(d.Jobs) != 1 || len(d.Amostras) != 1 || !d.Exportavel || d.Amostras[0].Prompt == "" || len(d.Historico) < 2 {
		t.Fatalf("detail: %+v %v", d, err)
	}
	d, _ = s.Detail(ctx, c)
	if d.Exportavel || d.MotivoNaoExport != "estado" {
		t.Errorf("a nova: %v %q", d.Exportavel, d.MotivoNaoExport)
	}
	if _, err := s.Detail(ctx, del); !errors.Is(err, ErrNotFound) {
		t.Errorf("erased: %v", err)
	}
	if _, err := s.Detail(ctx, mkID(77)); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

// C-4.7: sha256(group id), read as a number, mod 100 < 15 is the test part. The expected
// values were computed outside Go (python hashlib), so the rule cannot agree with itself by accident.
func TestHashSideVectors(t *testing.T) {
	for g, want := range map[string]string{
		"grupo-126": ParteTeste,  // mod 100 = 0
		"grupo-104": ParteTeste,  // 14
		"grupo-10":  ParteTreino, // 15: the limit is exclusive
		"grupo-51":  ParteTreino, // 16
		"grupo-167": ParteTreino, // 99
	} {
		if got := hashSide(g); got != want {
			t.Errorf("hashSide(%s) = %s, want %s", g, got, want)
		}
	}
}

// C-4.6: when several groups get their first exportable sample in the same pass, the
// quota goes in the order of the confirmations, not in the order of the ids.
func TestQuotaOrderInOnePassFollowsTheConfirmations(t *testing.T) {
	for try := 0; try < 30; try++ {
		s := openStore(t, Options{})
		f := newFake()
		for i := 1; i <= 2; i++ {
			f.add(i)
		}
		a, b := newEl(t, s, f, 1, "omitiu"), newEl(t, s, f, 2, "omitiu")
		if hashSide(gidOf(t, s, a)) != ParteTreino || hashSide(gidOf(t, s, b)) != ParteTreino {
			continue // both must follow the training side by the 15% rule, so only the quota can make a test case
		}
		s.db.Exec(`UPDATE amostras SET tem_imagem=1`) // confirmed, not exportable yet
		must(t, s.Confirm(ctx, a, admin))             // a is confirmed before b
		must(t, s.Confirm(ctx, b, admin))
		for i := 0; i < CotaPorCategoria-1; i++ { // one vaga left in the quota
			s.db.Exec(`INSERT INTO cota_grupos(grupo_id,categoria,particao,atribuida_em) VALUES(?,?,?,?)`, fmt.Sprintf("x%d", i), "omitiu", ParteTeste, i)
		}
		s.db.Exec(`UPDATE amostras SET tem_imagem=0`) // both become exportable in the same pass
		must(t, s.withTx(ctx, func(tx *sql.Tx) error { return s.recompute(ctx, tx, s.opts.Now()) }))
		if sideOf(t, s, a) != ParteTeste || sideOf(t, s, b) != ParteTreino {
			t.Errorf("the vaga went to %q/%q, want the first confirmed (a) in the test", sideOf(t, s, a), sideOf(t, s, b))
		}
		return
	}
	t.Skip("no pair of groups on the training side in 30 tries")
}

// The contract fixes the quota at 30 groups of each category (C-4.6): the number is not derived from the constant.
func TestQuotaIsThirty(t *testing.T) {
	if CotaPorCategoria != 30 {
		t.Errorf("CotaPorCategoria = %d, the contract says 30", CotaPorCategoria)
	}
}

// The exportable counts use the predicate: a confirmed case that fails a condition is not counted.
func TestSummaryExportableCountsUseThePredicate(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	a, b := newEl(t, s, f, 1, "omitiu"), newEl(t, s, f, 2, "omitiu")
	must(t, s.Confirm(ctx, a, admin))
	must(t, s.Confirm(ctx, b, admin))
	s.db.Exec(`UPDATE amostras SET tem_imagem=1 WHERE job_id=?`, mkID(2))
	sm, err := s.Summary(ctx)
	if err != nil || sm.ExportTeste != 1 {
		t.Errorf("exportable in the test part: %d %v (the one with an image must not count)", sm.ExportTeste, err)
	}
}

// ---- I-1 of the review: the record of an export is write-ahead --------------------------------

// seedExportable makes n confirmed, exportable occurrences in the training part.
func seedExportable(t *testing.T, s *Store, n int) {
	t.Helper()
	f := newFake()
	for i := 1; i <= n; i++ {
		id := newEl(t, s, f, i, "omitiu")
		must(t, s.Confirm(ctx, id, admin))
	}
	s.db.Exec(`UPDATE ocorrencias SET particao='treino'`)
}

// delivered returns the occurrence ids of the lines that reached the client.
func delivered(t *testing.T, b []byte) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l == "" {
			continue
		}
		var line struct {
			Meta exportMeta `json:"meta"`
		}
		if err := json.Unmarshal([]byte(l), &line); err != nil {
			t.Fatalf("a delivered line is not JSON: %v", err)
		}
		out = append(out, line.Meta.OcorrenciaID)
	}
	return out
}

func assertAllDeliveredRecorded(t *testing.T, s *Store, b []byte, wantAtLeast int) {
	t.Helper()
	ids := delivered(t, b)
	if len(ids) < wantAtLeast {
		t.Errorf("delivered %d lines, want at least %d", len(ids), wantAtLeast)
	}
	for _, id := range ids {
		var n int
		s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='exportou' AND campo='treino'`, id).Scan(&n)
		if n != 1 {
			t.Errorf("a delivered line of %s has %d exportou rows", id, n)
		}
	}
}

// buffer that can run a hook after each Write.
type hookWriter struct {
	bytes.Buffer
	after func(lines int)
	lines int
}

func (w *hookWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.lines++
	if w.after != nil {
		w.after(w.lines)
	}
	return n, err
}

func TestExportRecordIsWriteAheadWhenTheWriterFails(t *testing.T) {
	s := openStore(t, Options{})
	seedExportable(t, s, 25)
	w := &failingWriter{failAfter: 7}
	var b bytes.Buffer
	_, err := s.Export(ctx, ParteTreino, admin, io.MultiWriter(&b, w), nil)
	if err == nil {
		t.Fatal("the failing writer did not fail the export")
	}
	assertAllDeliveredRecorded(t, s, b.Bytes(), 7)
}

func TestExportRecordIsWriteAheadOnADatabaseError(t *testing.T) {
	s := openStore(t, Options{})
	seedExportable(t, s, exportBatch+5)
	// the second batch cannot be recorded: after the first batch the history refuses `exportou`
	if _, err := s.db.Exec(fmt.Sprintf(`CREATE TRIGGER sem_historico BEFORE INSERT ON historico WHEN NEW.acao='exportou' AND (SELECT COUNT(*) FROM historico WHERE acao='exportou')>=%d BEGIN SELECT RAISE(ABORT,'banco caiu'); END`, exportBatch)); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	n, err := s.Export(ctx, ParteTreino, admin, &b, nil)
	if err == nil || n != exportBatch {
		t.Fatalf("export: %d lines, %v (want the first batch only, then the error)", n, err)
	}
	assertAllDeliveredRecorded(t, s, b.Bytes(), exportBatch)
	if got := len(delivered(t, b.Bytes())); got != exportBatch {
		t.Errorf("a batch that could not be recorded was sent: %d lines", got)
	}
}

func TestExportRecordIsWriteAheadWhenTheContextExpires(t *testing.T) {
	s := openStore(t, Options{})
	seedExportable(t, s, exportBatch+5)
	cctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &hookWriter{after: func(lines int) {
		if lines == exportBatch {
			cancel() // the context ends right after the first batch was delivered
		}
	}}
	n, err := s.Export(cctx, ParteTreino, admin, w, nil)
	if err == nil || n != exportBatch {
		t.Fatalf("export: %d lines, %v", n, err)
	}
	assertAllDeliveredRecorded(t, s, w.Bytes(), exportBatch)
}

// A normal export records exactly the lines it sends, once each.
func TestExportRecordsExactlyWhatItSends(t *testing.T) {
	s := openStore(t, Options{})
	seedExportable(t, s, exportBatch*2+3)
	var b bytes.Buffer
	n, err := s.Export(ctx, ParteTreino, admin, &b, nil)
	if err != nil || n != exportBatch*2+3 {
		t.Fatal(n, err)
	}
	assertAllDeliveredRecorded(t, s, b.Bytes(), n)
	if c := count(t, s, "historico"); c < n {
		t.Error(c)
	}
	var rows int
	s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE acao='exportou'`).Scan(&rows)
	if rows != n {
		t.Errorf("%d exportou rows for %d lines", rows, n)
	}
}
