package feed

// Fase 12, etapa 12.8: the background embeddings of the public examples (C-6.6), the
// load of a frozen index and the privacy of the examples (C-8.1, C-8.2).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

var ctx = context.Background()
var admin = treino.AdminActor("10.0.0.9")

func jobID(n int) string { return fmt.Sprintf("%032x", n) }

// src is a minimal treino.Source: every job is a finished extract of the same station.
type src struct {
	mu    sync.Mutex
	conts map[string]treino.Content
}

func (s *src) Job(_ context.Context, id string) (treino.Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.conts[id]; !ok {
		return treino.Job{}, false, nil
	}
	return treino.Job{Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia", Think: "off", Status: "ok", HTTPStatus: 200,
		StationComputer: "PC01", StationUser: "ana", ClientIP: "10.0.0.7"}, true, nil
}
func (s *src) Probe(_ context.Context, id string) (treino.Probe, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conts[id]
	return treino.Probe{OK: ok, HasJSON: ok, Bytes: int64(len(c.PromptJSON) + len(c.Response))}, nil
}
func (s *src) Content(_ context.Context, id string) (treino.Content, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conts[id]
	return c, ok, nil
}

type env struct {
	t    *testing.T
	st   *treino.Store
	src  *src
	path string
	raw  *sql.DB
	key  *hmackey.Key
}

func newEnv(t *testing.T) *env {
	t.Helper()
	key, err := hmackey.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "treino.db")
	st, err := treino.Open(path, treino.Options{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close(); st.Close() })
	return &env{t: t, st: st, src: &src{conts: map[string]treino.Content{}}, path: path, raw: raw, key: key}
}

// occurrence reports job n (request text `pedido`, model answer `errada`, right answer
// `certa`) and confirms it. part forces the partition; public marks it.
func (e *env) occurrence(n int, pedido, errada, certa, part string, public bool) string {
	e.t.Helper()
	pj, _ := json.Marshal(map[string]any{"model": "qwen3-ia", "system": "Você processa arquivos.", "prompt": pedido, "options": map[string]any{"seed": 1}})
	e.src.conts[jobID(n)] = treino.Content{Prompt: pedido, Response: errada, PromptJSON: pj, RespFinal: true}
	rep := treino.Report{Projeto: "X", Jobs: []treino.JobRef{{ID: jobID(n), Papel: "unico", Backend: "primario"}},
		Categoria: "omitiu", Descricao: "d", RespostaCorreta: certa, Severidade: "media", Origem: "claude"}
	if err := rep.Validate(); err != nil {
		e.t.Fatal(err)
	}
	res, err := e.st.Ingest(ctx, rep, treino.Origin{Computer: "PC01", User: "ana", Domain: "csl", MAC: "AABBCCDDEEFF", ClientIP: "10.0.0.7", ClientLabel: "PC01/ana"}, "", e.src, e.key)
	if err != nil {
		e.t.Fatal(err)
	}
	id := res.OcorrenciaID
	if err := e.st.Confirm(ctx, id, admin); err != nil {
		e.t.Fatal(err)
	}
	if part != "" {
		if _, err := e.raw.Exec(`UPDATE ocorrencias SET particao=? WHERE id=?`, part, id); err != nil {
			e.t.Fatal(err)
		}
	}
	if public {
		if err := e.st.SetExemploPublico(ctx, id, true, admin); err != nil {
			e.t.Fatal(err)
		}
	}
	return id
}

// fakeEmbed is deterministic: the text decides the vector.
func fakeEmbed(calls *[]string, mu *sync.Mutex) melhora.EmbedFunc {
	return func(_ context.Context, text string) ([]float32, error) {
		if mu != nil {
			mu.Lock()
			*calls = append(*calls, text)
			mu.Unlock()
		}
		switch {
		case strings.Contains(text, "ALFA"):
			return []float32{1, 0, 0}, nil
		case strings.Contains(text, "BETA"):
			return []float32{0, 1, 0}, nil
		}
		return []float32{0, 0, 1}, nil
	}
}

func vecOfRow(t *testing.T, e *env, id string) ([]float32, string) {
	t.Helper()
	var blob []byte
	var model sql.NullString
	if err := e.raw.QueryRow(`SELECT vetor, modelo FROM embeddings WHERE ocorrencia_id=?`, id).Scan(&blob, &model); err != nil {
		t.Fatal(err)
	}
	return treino.DecodeVec(blob), model.String
}

// ---- the background embeddings (C-6.6) -------------------------------------------------------------

func TestEmbedderFillsPendingPublicExamplesOnlyAndOnlyOnce(t *testing.T) {
	e := newEnv(t)
	pub := e.occurrence(1, "ALFA pedido público", "errado", "certo", "treino", true)
	e.occurrence(2, "BETA pedido de cliente", "errado", "certo", "treino", false) // not public: never embedded
	var calls []string
	var mu sync.Mutex
	var ok, bad int
	w := &Embedder{Store: e.st, Embed: fakeEmbed(&calls, &mu), Note: func(o bool) {
		if o {
			ok++
		} else {
			bad++
		}
	}}
	if n := w.Pass(ctx); n != 1 || ok != 1 || bad != 0 {
		t.Fatalf("pass filled %d (ok %d, bad %d)", n, ok, bad)
	}
	v, model := vecOfRow(t, e, pub)
	if len(v) != 3 || v[0] != 1 || model != melhora.DefaultEmbedModel {
		t.Errorf("stored vector %v model %q", v, model)
	}
	if len(calls) != 1 || strings.Contains(calls[0], "BETA") || !strings.Contains(calls[0], "ALFA pedido público") || !strings.Contains(calls[0], "[sistema]") {
		t.Errorf("embedded text: %q", calls)
	}
	if n := w.Pass(ctx); n != 0 || len(calls) != 1 {
		t.Errorf("second pass embedded again: %d, %d calls", n, len(calls))
	}
	// the sample request of the client's occurrence was never given to the embedder
	for _, c := range calls {
		if strings.Contains(c, "cliente") {
			t.Errorf("a customer's text was embedded: %q", c)
		}
	}
}

func TestEmbedderRetriesAFailureAfterTheWaitAndNeverBlocksTheOthers(t *testing.T) {
	e := newEnv(t)
	a := e.occurrence(1, "ALFA um", "x", "y", "treino", true)
	b := e.occurrence(2, "BETA dois", "x", "y", "treino", true)
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	fail := map[string]bool{"ALFA": true}
	var mu sync.Mutex
	embedFn := func(_ context.Context, text string) ([]float32, error) {
		mu.Lock()
		defer mu.Unlock()
		for k := range fail {
			if strings.Contains(text, k) {
				return nil, errors.New("indisponível")
			}
		}
		return []float32{1, 1, 1}, nil
	}
	var bad int
	w := &Embedder{Store: e.st, Embed: embedFn, Retry: time.Minute, Now: func() time.Time { return now }, Note: func(o bool) {
		if !o {
			bad++
		}
	}}
	if n := w.Pass(ctx); n != 1 || bad != 1 {
		t.Fatalf("first pass: filled %d, failures %d", n, bad)
	}
	if v, _ := vecOfRow(t, e, b); v == nil {
		t.Error("the healthy one was not filled because of the failing one")
	}
	if v, _ := vecOfRow(t, e, a); v != nil {
		t.Error("the failing one got a vector")
	}
	if n := w.Pass(ctx); n != 0 || bad != 1 {
		t.Errorf("retried before the wait: %d, %d", n, bad)
	}
	now = now.Add(2 * time.Minute)
	mu.Lock()
	delete(fail, "ALFA")
	mu.Unlock()
	if n := w.Pass(ctx); n != 1 {
		t.Errorf("not retried after the wait: %d", n)
	}
	if v, _ := vecOfRow(t, e, a); v == nil {
		t.Error("never filled")
	}
}

func TestEmbedderDoesNotStoreAVectorForAnOccurrenceThatChangedMeanwhile(t *testing.T) {
	e := newEnv(t)
	id := e.occurrence(1, "ALFA um", "x", "y", "treino", true)
	w := &Embedder{Store: e.st, Embed: func(_ context.Context, _ string) ([]float32, error) {
		// the admin unmarks it while the vector is being made
		if err := e.st.SetExemploPublico(ctx, id, false, admin); err != nil {
			t.Error(err)
		}
		return []float32{1, 0, 0}, nil
	}}
	if n := w.Pass(ctx); n != 0 {
		t.Errorf("filled %d", n)
	}
	var n int
	e.raw.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE ocorrencia_id=?`, id).Scan(&n)
	if n != 0 {
		t.Errorf("the unmarked occurrence has an embedding row again (%d)", n)
	}
	// marked again, it is made fresh
	if err := e.st.SetExemploPublico(ctx, id, true, admin); err != nil {
		t.Fatal(err)
	}
	w2 := &Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}
	if w2.Pass(ctx) != 1 {
		t.Error("not embedded after being marked again")
	}
}

func TestEmbedderPassStopsWhenTheContextEndsInTheMiddle(t *testing.T) {
	e := newEnv(t)
	e.occurrence(1, "ALFA um", "x", "y", "treino", true)
	e.occurrence(2, "BETA dois", "x", "y", "treino", true)
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	calls := 0
	w := &Embedder{Store: e.st, Embed: func(context.Context, string) ([]float32, error) {
		calls++
		cancel() // the gateway is stopping
		return []float32{1, 0, 0}, nil
	}}
	w.Pass(c)
	if calls != 1 {
		t.Errorf("%d embeddings after the context ended, want 1", calls)
	}
}

// ---- the index and the privacy of the examples (C-8.1, C-8.2) ---------------------------------------------

func TestCustomerDocumentsNeverReachAnotherRequest(t *testing.T) {
	e := newEnv(t)
	const secret = "SEGREDO-DO-CLIENTE-X"
	pub := e.occurrence(1, "ALFA pedido público e anonimizado", "resposta pública errada", "resposta pública certa", "treino", true)
	// a customer's occurrence: confirmed, training part, the same kind of request, NOT public
	cust := e.occurrence(2, "ALFA "+secret+" contrato do cliente", secret+" resposta errada", secret+" resposta certa", "treino", false)
	_ = cust
	w := &Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}
	w.Pass(ctx)
	m, err := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 5, Limiar: 0.5, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
	if err != nil {
		t.Fatal(err)
	}
	if m.Itens != 1 {
		t.Fatalf("the index has %d members, want only the public one", m.Itens)
	}
	ix, err := LoadIndex(ctx, e.st, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Items) != 1 || ix.Items[0].OcorrenciaID != pub {
		t.Fatalf("items: %+v", ix.Items)
	}
	// nothing of the customer is in memory at all
	for _, it := range ix.Items {
		if strings.Contains(fmt.Sprintf("%+v", it), secret) {
			t.Errorf("the index holds the customer's text: %+v", it)
		}
	}
	tr := melhora.NewTransformer(melhora.Config{Tools: melhora.Tools, Embed: fakeEmbed(nil, nil)})
	tr.SetIndex(ix)
	// ANOTHER client's request, about the same kind of task
	req := []byte(`{"model":"qwen3-ia","system":"Você processa arquivos.","prompt":"ALFA outro documento, de outro cliente"}`)
	out, oc := tr.Transform(ctx, "extract", "/api/generate", req, melhora.Exclusion{})
	if oc.Exemplos != 1 {
		t.Fatalf("outcome %+v", oc)
	}
	if strings.Contains(string(out), secret) || !strings.Contains(string(out), "resposta pública certa") || !strings.Contains(string(out), "resposta pública errada") {
		t.Errorf("injected request:\n%s", out)
	}
}

func TestExampleTextComesFromTheSampleAndTheRightAnswer(t *testing.T) {
	e := newEnv(t)
	e.occurrence(1, "ALFA liste os títulos", "[raciocínio] hmm\n[resposta] 1, 2, 3", "1, 2, 4", "treino", true)
	(&Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}).Pass(ctx)
	m, err := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 1, Limiar: 0.5, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
	if err != nil {
		t.Fatal(err)
	}
	ix, err := LoadIndex(ctx, e.st, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	ex := ix.Items[0].Example
	if !strings.Contains(ex.Pedido, "ALFA liste os títulos") || ex.Errada != "1, 2, 3" || ex.Certa != "1, 2, 4" {
		t.Errorf("example: %+v", ex)
	}
	if ix.Hash != m.Hash || ix.K != 1 || ix.Limiar != 0.5 || ix.Modelo != melhora.DefaultEmbedModel || ix.ID != m.ID {
		t.Errorf("index: %+v vs %+v", ix, m)
	}
	if ix.Items[0].DocKey != "" && !strings.Contains(ix.Items[0].DocKey, "/") {
		t.Errorf("doc key: %q", ix.Items[0].DocKey)
	}
}

func TestLoadIndexRefusesAnInvalidOne(t *testing.T) {
	e := newEnv(t)
	id := e.occurrence(1, "ALFA x", "e", "c", "treino", true)
	(&Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}).Pass(ctx)
	m, _ := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 1, Limiar: 0.5, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
	if err := e.st.SetExemploPublico(ctx, id, false, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndex(ctx, e.st, m.ID); !errors.Is(err, treino.ErrIndexInvalid) {
		t.Errorf("err = %v", err)
	}
	if _, err := LoadIndex(ctx, e.st, "nope"); !errors.Is(err, treino.ErrIndexNotFound) {
		t.Errorf("err = %v", err)
	}
}

// ---- review of 12.8 ------------------------------------------------------------------------------

func TestBadPendingRowsNeverBlockTheQueueOfTwoHundred(t *testing.T) { // M-2
	e := newEnv(t)
	// 205 public examples whose request is gone (no sample), BEFORE the good one in the queue
	for i := 0; i < 205; i++ {
		id := fmt.Sprintf("%032x", 0xbad00000+i)
		if _, err := e.raw.Exec(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado,categoria,resposta_correta,tool,job_do_erro,particao,grupo_id,exemplo_publico) VALUES(?,1,1,'confirmada','omitiu','c','extract','sem-amostra','treino',?,1)`, id, id); err != nil {
			t.Fatal(err)
		}
		if _, err := e.raw.Exec(`INSERT INTO embeddings(ocorrencia_id,vetor,modelo) VALUES(?,NULL,NULL)`, id); err != nil {
			t.Fatal(err)
		}
	}
	good := e.occurrence(1, "ALFA pedido bom", "x", "y", "treino", true)
	calls := 0
	w := &Embedder{Store: e.st, Embed: func(_ context.Context, _ string) ([]float32, error) { calls++; return []float32{1, 0, 0}, nil }}
	if n := w.Pass(ctx); n != 1 {
		t.Fatalf("the good row was not filled behind 205 bad ones (filled %d)", n)
	}
	if v, _ := vecOfRow(t, e, good); v == nil {
		t.Error("no vector")
	}
	var pending, failed int
	e.raw.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE vetor IS NULL AND modelo IS NULL`).Scan(&pending)
	e.raw.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE modelo='falha:sem_pedido'`).Scan(&failed)
	if pending != 0 || failed != 205 {
		t.Errorf("pending %d, failed for good %d", pending, failed)
	}
	if calls != 1 {
		t.Errorf("%d embeddings for 1 good row", calls)
	}
	// the next pass has nothing to look at
	if n := w.Pass(ctx); n != 0 || calls != 1 {
		t.Errorf("second pass: %d, %d calls", n, calls)
	}
}

func TestTransientFailuresBeyondOnePageDoNotStarveTheRest(t *testing.T) { // M-2: the cursor pages on
	e := newEnv(t)
	for i := 1; i <= 5; i++ {
		e.occurrence(i, fmt.Sprintf("ALFA pedido %d", i), "x", "y", "treino", true)
	}
	n := 0
	w := &Embedder{Store: e.st, Batch: 2, Embed: func(_ context.Context, text string) ([]float32, error) {
		n++
		if n <= 3 { // the first three rows (a page and a half) fail
			return nil, errors.New("indisponível")
		}
		return []float32{1, 0, 0}, nil
	}}
	if got := w.Pass(ctx); got != 2 {
		t.Errorf("filled %d, want the last two", got)
	}
}

func TestLoadIndexRefusesAnotherFormatOfTheExamples(t *testing.T) { // C-8.10a, M-3
	e := newEnv(t)
	e.occurrence(1, "ALFA x", "e", "c", "treino", true)
	(&Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}).Pass(ctx)
	old, err := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 1, Limiar: 0.5, Modelo: melhora.DefaultEmbedModel, Formato: "ex0-velho"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndex(ctx, e.st, old.ID); !errors.Is(err, ErrIndexFormat) {
		t.Errorf("err = %v", err)
	}
	cur, err := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 1, Limiar: 0.5, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
	if err != nil {
		t.Fatal(err)
	}
	if cur.Hash == old.Hash {
		t.Error("the format is not in the hash")
	}
	if _, err := LoadIndex(ctx, e.st, cur.ID); err != nil {
		t.Errorf("current format: %v", err)
	}
}
