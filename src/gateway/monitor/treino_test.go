package monitor

// Fase 12.6: the error board and its routes.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

func tid(n int) string { return fmt.Sprintf("%032x", n) }

// tsrc is the treino.Source of the tests.
type tsrc struct {
	mu    sync.Mutex
	jobs  map[string]treino.Job
	conts map[string]treino.Content
}

func (f *tsrc) Job(_ context.Context, id string) (treino.Job, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	return j, ok, nil
}

func (f *tsrc) Probe(_ context.Context, id string) (treino.Probe, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.conts[id]
	return treino.Probe{OK: ok, HasJSON: ok && c.PromptJSON != nil, Bytes: int64(len(c.PromptJSON) + len(c.Prompt) + len(c.Response))}, nil
}

func (f *tsrc) Content(_ context.Context, id string) (treino.Content, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.conts[id]
	return c, ok, nil
}

func (f *tsrc) add(n int) string {
	id := tid(n)
	f.jobs[id] = treino.Job{Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia", Think: "off", Status: "ok", HTTPStatus: 200,
		StationComputer: "PC01", StationUser: "ana", ClientIP: "10.0.0.7"}
	f.conts[id] = treino.Content{Prompt: fmt.Sprintf("TEXTO-PEDIDO-%d", n), Response: fmt.Sprintf("TEXTO-RESPOSTA-%d", n),
		PromptJSON: []byte(fmt.Sprintf(`{"model":"qwen3-ia","system":"S","prompt":"TEXTO-PEDIDO-%d"}`, n)), RespFinal: true}
	return id
}

type tfix struct {
	*fixture
	ts     *treino.Store
	dbPath string
	src    *tsrc
	key    *hmackey.Key
	cookie string
	csrf   string
}

var stOrigin = treino.Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.7", ClientLabel: "PC01/ana"}

func newTreinoFixture(t *testing.T) *tfix {
	t.Helper()
	key, err := hmackey.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "treino.db")
	ts, err := treino.Open(dbPath, treino.Options{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ts.Close() })
	st, err := store.Open(filepath.Join(dir, "gw.db"), store.Options{BootID: "boot-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hub := NewHub()
	q := queue.New(queue.Config{GenSlots: 1, EmbedSlots: 1, OnChange: hub.Notify, OnRecord: st.Record})
	line, err := HashPassword(adminPass, minIterations)
	if err != nil {
		t.Fatal(err)
	}
	cr, err := ParseCredentials(adminUser + ":" + line)
	if err != nil {
		t.Fatal(err)
	}
	src := &tsrc{jobs: map[string]treino.Job{}, conts: map[string]treino.Content{}}
	h, err := New(Config{Queue: q, Store: st, Hub: hub, Version: "test", BootID: "boot-test", Admin: cr, Treino: ts, TreinoSource: src, TreinoKey: key, Role: "primario"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{h: h, q: q, st: st, hub: hub}
	tf := &tfix{fixture: f, ts: ts, dbPath: dbPath, src: src, key: key}
	tf.cookie, tf.csrf = f.login(t)
	return tf
}

// occ reports a job as an error (from its own station) and returns the occurrence id.
func (f *tfix) occ(t *testing.T, n int, cat string, resp string) string {
	t.Helper()
	if _, ok := f.src.jobs[tid(n)]; !ok {
		f.src.add(n)
	}
	rep := treino.Report{Projeto: "GestorERP", Jobs: []treino.JobRef{{ID: tid(n), Papel: "unico", Backend: "primario"}}, Categoria: cat,
		Descricao: fmt.Sprintf("SEGREDO-DESCRICAO-%d", n), RespostaCorreta: resp, Severidade: "media", Origem: "claude"}
	if err := rep.Validate(); err != nil {
		t.Fatal(err)
	}
	res, err := f.ts.Ingest(context.Background(), rep, stOrigin, "", f.src, f.key)
	if err != nil {
		t.Fatal(err)
	}
	return res.OcorrenciaID
}

func (f *tfix) adm(method, path, body string) *httptest.ResponseRecorder {
	return do(f.h, method, path, body, map[string]string{"Cookie": f.cookie, "X-CSRF-Token": f.csrf})
}

func (f *tfix) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	// the history is read through the store's own pool: a second connection on the file
	db := f.openDB(t)
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// openDB opens a second connection on treino.db, to read what the routes wrote.
func (f *tfix) openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---- access ------------------------------------------------------------------------------------------

func TestTreinoRoutesNeedTheAdminSessionCSRFAndSameSite(t *testing.T) {
	f := newTreinoFixture(t)
	id := f.occ(t, 1, "omitiu", "certa")
	post := []string{"confirmar", "descartar", "completar", "apagar", "exemplo-publico", "liberar-quarentena"}
	bodies := map[string]string{"completar": `{"descricao":"x"}`, "exemplo-publico": `{"marcar":true}`, "liberar-quarentena": `{"lado":"treino"}`}
	for _, kind := range post {
		path := "/monitor/api/treino/" + id + "/" + kind
		body := bodies[kind]
		if body == "" {
			body = "{}"
		}
		if w := do(f.h, "POST", path, body, nil); w.Code != 401 {
			t.Errorf("%s without session: %d", kind, w.Code)
		}
		if w := do(f.h, "POST", path, body, map[string]string{"Cookie": f.cookie}); w.Code != 403 {
			t.Errorf("%s without CSRF: %d", kind, w.Code)
		}
		if w := do(f.h, "POST", path, body, map[string]string{"Cookie": f.cookie, "X-CSRF-Token": "errado"}); w.Code != 403 {
			t.Errorf("%s with a wrong CSRF: %d", kind, w.Code)
		}
		if w := do(f.h, "POST", path, body, map[string]string{"Cookie": f.cookie, "X-CSRF-Token": f.csrf, "Sec-Fetch-Site": "cross-site"}); w.Code != 403 {
			t.Errorf("%s cross-site: %d", kind, w.Code)
		}
		if w := do(f.h, "POST", path, body, map[string]string{"Cookie": f.cookie, "X-CSRF-Token": f.csrf, "Origin": "https://outro.exemplo"}); w.Code != 403 {
			t.Errorf("%s from another origin: %d", kind, w.Code)
		}
	}
	mark := "/monitor/api/jobs/" + tid(1) + "/marcar-erro"
	mb := `{"categoria":"omitiu","severidade":"baixa","descricao":"x"}`
	for name, hdr := range map[string]map[string]string{"no session": nil, "no CSRF": {"Cookie": f.cookie}, "cross-site": {"Cookie": f.cookie, "X-CSRF-Token": f.csrf, "Sec-Fetch-Site": "cross-site"}} {
		if w := do(f.h, "POST", mark, mb, hdr); w.Code != 401 && w.Code != 403 {
			t.Errorf("marcar-erro %s: %d", name, w.Code)
		}
	}
	for _, p := range []string{"/monitor/api/treino/" + id, "/monitor/api/treino/export?parte=teste"} {
		if w := do(f.h, "GET", p, "", nil); w.Code != 401 {
			t.Errorf("GET %s without session: %d", p, w.Code)
		}
		if w := do(f.h, "GET", p, "", map[string]string{"Cookie": f.cookie, "Sec-Fetch-Site": "cross-site"}); w.Code != 403 {
			t.Errorf("GET %s cross-site: %d", p, w.Code)
		}
	}
	// nothing changed in any of those refusals
	if got := f.count(t, `SELECT COUNT(*) FROM ocorrencias WHERE estado='nova'`); got != 1 {
		t.Errorf("a refused request changed the occurrence (%d nova)", got)
	}
	// without --admin-file there is no action at all
	nf := newFixture(t, false, queue.Config{GenSlots: 1})
	_ = nf
}

func TestVisitorSeesCountsAndNoText(t *testing.T) {
	f := newTreinoFixture(t)
	f.occ(t, 1, "omitiu", "TEXTO-CORRECAO-1")
	f.occ(t, 2, "inventou", "")
	for _, p := range []string{"/monitor/api/treino", "/monitor/api/state"} {
		w := do(f.h, "GET", p, "", nil)
		if w.Code != 200 {
			t.Fatalf("%s: %d", p, w.Code)
		}
		body := w.Body.String()
		for _, secret := range []string{"SEGREDO", "TEXTO-", "GestorERP", "PC01"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s shows %q to a visitor", p, secret)
			}
		}
		if !strings.Contains(body, `"por_estado"`) || !strings.Contains(body, `"max_bytes"`) {
			t.Errorf("%s lacks the counts: %s", p, body)
		}
	}
	var out struct {
		Admin   bool            `json:"admin"`
		Rows    json.RawMessage `json:"rows"`
		Summary struct{ Total int }
	}
	json.Unmarshal(do(f.h, "GET", "/monitor/api/treino", "", nil).Body.Bytes(), &out)
	if out.Admin || out.Rows != nil || out.Summary.Total != 2 {
		t.Errorf("visitor view: %+v", out)
	}
	// a cross-site request that carries the cookie is a visitor too
	w := do(f.h, "GET", "/monitor/api/treino", "", map[string]string{"Cookie": f.cookie, "Sec-Fetch-Site": "cross-site"})
	if strings.Contains(w.Body.String(), "SEGREDO") || strings.Contains(w.Body.String(), `"rows"`) {
		t.Error("a cross-site read got the list")
	}
}

func TestAdminListAndDetailShowTheText(t *testing.T) {
	f := newTreinoFixture(t)
	id := f.occ(t, 1, "omitiu", "TEXTO-CORRECAO-1")
	f.occ(t, 2, "inventou", "")
	w := f.adm("GET", "/monitor/api/treino?categoria=omitiu", "")
	var out struct {
		Admin bool
		Rows  []treino.ListRow
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if !out.Admin || len(out.Rows) != 1 || out.Rows[0].ID != id || !strings.Contains(out.Rows[0].Descricao, "SEGREDO-DESCRICAO-1") {
		t.Fatalf("admin list: %s", w.Body)
	}
	w = f.adm("GET", "/monitor/api/treino/"+id, "")
	var d treino.Detail
	json.Unmarshal(w.Body.Bytes(), &d)
	if w.Code != 200 || d.ID != id || d.RespostaCorreta != "TEXTO-CORRECAO-1" || len(d.Jobs) != 1 || len(d.Amostras) != 1 ||
		d.Amostras[0].Prompt != "TEXTO-PEDIDO-1" || d.Amostras[0].Resposta != "TEXTO-RESPOSTA-1" || len(d.Historico) == 0 {
		t.Errorf("detail: %d %s", w.Code, w.Body)
	}
	if w := f.adm("GET", "/monitor/api/treino/"+tid(99), ""); w.Code != 404 {
		t.Errorf("unknown: %d", w.Code)
	}
	if w := f.adm("GET", "/monitor/api/treino/xyz", ""); w.Code != 400 && w.Code != 404 {
		t.Errorf("bad id: %d", w.Code)
	}
	if w := f.adm("GET", "/monitor/api/treino?desde=ontem", ""); w.Code != 400 {
		t.Errorf("bad period: %d", w.Code)
	}
}

// ---- actions ------------------------------------------------------------------------------------------

func TestAdminActionsChangeAndLeaveHistory(t *testing.T) {
	f := newTreinoFixture(t)
	id := f.occ(t, 1, "omitiu", "")
	url := func(kind string) string { return "/monitor/api/treino/" + id + "/" + kind }
	if w := f.adm("POST", url("confirmar"), "{}"); w.Code != 422 { // C-1.12a
		t.Errorf("confirm without answer: %d %s", w.Code, w.Body)
	}
	if w := f.adm("POST", url("completar"), `{"resposta_correta":"SEGREDO-CERTA"}`); w.Code != 200 {
		t.Fatalf("completar: %d %s", w.Code, w.Body)
	}
	if w := f.adm("POST", url("completar"), `{}`); w.Code != 400 {
		t.Errorf("completar with nothing: %d", w.Code)
	}
	if w := f.adm("POST", url("completar"), `{"resposta_correta":"x","campo":1}`); w.Code != 400 {
		t.Errorf("unknown field: %d", w.Code)
	}
	if w := f.adm("POST", url("exemplo-publico"), `{"marcar":true}`); w.Code != 409 {
		t.Errorf("example of an unconfirmed: %d", w.Code)
	}
	if w := f.adm("POST", url("confirmar"), "{}"); w.Code != 200 {
		t.Fatalf("confirmar: %d %s", w.Code, w.Body)
	}
	if w := f.adm("POST", url("confirmar"), "{}"); w.Code != 409 {
		t.Errorf("confirm twice: %d", w.Code)
	}
	if w := f.adm("POST", url("exemplo-publico"), `{"marcar":true}`); w.Code != 200 {
		t.Errorf("example: %d %s", w.Code, w.Body)
	}
	if w := f.adm("POST", url("exemplo-publico"), `{}`); w.Code != 400 {
		t.Errorf("example without marcar: %d", w.Code)
	}
	var des struct{ Desconfirmou bool }
	w := f.adm("POST", url("completar"), `{"descricao":"mudou"}`)
	json.Unmarshal(w.Body.Bytes(), &des)
	if w.Code != 200 || !des.Desconfirmou {
		t.Errorf("editing a confirmed one: %d %s", w.Code, w.Body)
	}
	if w := f.adm("POST", url("liberar-quarentena"), `{"lado":"treino"}`); w.Code != 409 {
		t.Errorf("release of what is not in quarantine: %d", w.Code)
	}
	if w := f.adm("POST", url("liberar-quarentena"), `{"lado":"nenhum"}`); w.Code != 400 {
		t.Errorf("release with a bad side: %d", w.Code)
	}
	if w := f.adm("POST", url("descartar"), "{}"); w.Code != 200 {
		t.Errorf("descartar: %d", w.Code)
	}
	if w := f.adm("POST", url("completar"), `{"descricao":"y"}`); w.Code != 409 { // C-1.12c
		t.Errorf("editing a discarded one: %d", w.Code)
	}
	if w := f.adm("POST", url("apagar"), "{}"); w.Code != 200 {
		t.Errorf("apagar: %d", w.Code)
	}
	if w := f.adm("POST", url("apagar"), "{}"); w.Code != 409 {
		t.Errorf("apagar twice: %d", w.Code)
	}
	if w := f.adm("POST", "/monitor/api/treino/"+tid(99)+"/confirmar", "{}"); w.Code != 404 {
		t.Errorf("unknown: %d", w.Code)
	}
	if w := f.adm("POST", "/monitor/api/treino/zz/confirmar", "{}"); w.Code != 400 && w.Code != 404 {
		t.Errorf("bad id: %d", w.Code)
	}
	// every action is in the history, by "admin", with the IP of the request
	for _, acao := range []string{"alterou", "confirmou", "desconfirmou", "descartou", "apagou"} {
		if n := f.count(t, `SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao=? AND quem='admin' AND ip='10.100.2.50'`, id, acao); n == 0 {
			t.Errorf("no %s row by admin in the history", acao)
		}
	}
	// the minimal row remains, with no text
	var desc *string
	f.openDB(t).QueryRow(`SELECT descricao FROM ocorrencias WHERE id=?`, id).Scan(&desc)
	if desc != nil {
		t.Errorf("the erased occurrence kept its text: %q", *desc)
	}
}

// The HMAC key given to the store reaches the admin actions (pending of 12.4): the
// history of an edit made through the route holds an HMAC, never a plain hash.
func TestAdminEditHistoryUsesTheKey(t *testing.T) {
	f := newTreinoFixture(t)
	id := f.occ(t, 1, "omitiu", "antes")
	if w := f.adm("POST", "/monitor/api/treino/"+id+"/completar", `{"resposta_correta":"depois do ajuste"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	var depois string
	f.openDB(t).QueryRow(`SELECT depois FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='resposta_correta'`, id).Scan(&depois)
	if want := "hmac=" + f.key.MACString("depois do ajuste") + " key_id=" + f.key.ID() + " bytes=16"; depois != want {
		t.Errorf("history = %q, want %q", depois, want)
	}
}

// ---- "Marcar erro" -------------------------------------------------------------------------------------

func TestMarkErrorFromTheJobDetail(t *testing.T) {
	f := newTreinoFixture(t)
	id := f.src.add(1)
	j := f.src.jobs[id]
	j.StationComputer, j.StationUser, j.ClientIP = "OUTRO", "bob", "10.9.9.9" // another origin than the administrator's
	f.src.jobs[id] = j
	body := `{"categoria":"numeracao_errada","severidade":"alta","descricao":"pulou a secao 7.2","resposta_correta":"certa","projeto":"GestorERP"}`
	w := f.adm("POST", "/monitor/api/jobs/"+id+"/marcar-erro", body)
	var res treino.Result
	json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != 200 || res.Estado != "nova" || res.AmostrasCopiadas != 1 || res.JobsOutraOrigem != 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var origem string
	f.openDB(t).QueryRow(`SELECT origem FROM ocorrencias WHERE id=?`, res.OcorrenciaID).Scan(&origem)
	if origem != "usuario" || f.count(t, `SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='criou' AND quem='admin'`, res.OcorrenciaID) != 1 {
		t.Errorf("origem=%q", origem)
	}
	for name, c := range map[string]struct {
		path, body string
		want       int
	}{
		"unknown job":  {"/monitor/api/jobs/" + tid(77) + "/marcar-erro", body, 404},
		"bad category": {"/monitor/api/jobs/" + id + "/marcar-erro", `{"categoria":"x","severidade":"alta","descricao":"d"}`, 400},
		"no text":      {"/monitor/api/jobs/" + id + "/marcar-erro", `{"categoria":"omitiu","severidade":"alta"}`, 400},
		"bad id":       {"/monitor/api/jobs/zz/marcar-erro", body, 400},
	} {
		if w := f.adm("POST", c.path, c.body); w.Code != c.want && !(c.want == 400 && w.Code == 404) {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	// a job still running: 409 with Retry-After
	busy := f.src.add(2)
	bj := f.src.jobs[busy]
	bj.Status = "em_execucao"
	f.src.jobs[busy] = bj
	w = f.adm("POST", "/monitor/api/jobs/"+busy+"/marcar-erro", body)
	if w.Code != 409 || w.Header().Get("Retry-After") == "" {
		t.Errorf("running job: %d %q", w.Code, w.Header().Get("Retry-After"))
	}
}

// ---- export ----------------------------------------------------------------------------------------------

func TestExportRouteStreamsAndRecordsEachOccurrence(t *testing.T) {
	f := newTreinoFixture(t)
	var ids []string
	for i := 1; i <= 3; i++ {
		id := f.occ(t, i, "omitiu", fmt.Sprintf("certa %d", i))
		if w := f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}"); w.Code != 200 {
			t.Fatal(w.Body)
		}
		ids = append(ids, id)
	}
	f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino'`)
	if w := f.adm("GET", "/monitor/api/treino/export?parte=quarentena", ""); w.Code != 400 {
		t.Errorf("bad part: %d", w.Code)
	}
	w := f.adm("GET", "/monitor/api/treino/export?parte=treino", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/x-ndjson") || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers: %d %v", w.Code, w.Header())
	}
	lines := strings.Split(strings.TrimRight(w.Body.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines: %s", len(lines), w.Body)
	}
	var l struct {
		Messages []map[string]string `json:"messages"`
		Meta     map[string]string   `json:"meta"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &l); err != nil || len(l.Messages) != 3 || l.Meta["particao"] != "treino" || len(l.Meta) != 6 {
		t.Errorf("line: %v %v", err, lines[0])
	}
	for _, id := range ids {
		if n := f.count(t, `SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='exportou' AND campo='treino' AND quem='admin'`, id); n != 1 {
			t.Errorf("%s: %d exportou rows", id, n)
		}
	}
	if w := f.adm("GET", "/monitor/api/treino/export?parte=teste", ""); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "" {
		t.Errorf("the test part is empty: %d %q", w.Code, w.Body)
	}
}

// ---- C-6.10: own contexts, no lock held while streaming --------------------------------------------------------

func TestTreinoReadsDoNotUseTheVisitorContext(t *testing.T) {
	f := newTreinoFixture(t)
	id := f.occ(t, 1, "omitiu", "certa")
	if w := f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}"); w.Code != 200 { // exportable, in the test part
		t.Fatal(w.Body)
	}
	cctx, cancel := context.WithCancel(context.Background())
	cancel() // the visitor is gone before the work starts
	hdr := map[string]string{"Cookie": f.cookie, "X-CSRF-Token": f.csrf}
	for name, c := range map[string]struct{ method, path, body string }{
		"summary": {"GET", "/monitor/api/treino", ""}, "detail": {"GET", "/monitor/api/treino/" + id, ""},
		"state": {"GET", "/monitor/api/state", ""}, "action": {"POST", "/monitor/api/treino/" + id + "/exemplo-publico", `{"marcar":true}`},
		"export": {"GET", "/monitor/api/treino/export?parte=teste", ""},
	} {
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)).WithContext(cctx)
		r.RemoteAddr = "127.0.0.1:5555"
		r.Header.Set("Content-Type", "application/json")
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("%s with a cancelled request context: %d %s", name, w.Code, w.Body)
		}
		if name == "export" && !strings.Contains(w.Body.String(), `"ocorrencia_id"`) {
			t.Errorf("the export with a cancelled request context sent nothing: %q", w.Body)
		}
	}
}

// slowResponse is a ResponseWriter whose Write blocks until released: a slow client.
type slowResponse struct {
	hdr     http.Header
	gate    chan struct{}
	started chan struct{}
	once    sync.Once
}

func (s *slowResponse) Header() http.Header { return s.hdr }
func (s *slowResponse) WriteHeader(int)     {}
func (s *slowResponse) Write(p []byte) (int, error) {
	s.once.Do(func() { close(s.started) })
	<-s.gate
	return len(p), nil
}

func TestStuckExportDoesNotBlockTheMonitorOrTheReports(t *testing.T) {
	f := newTreinoFixture(t)
	for i := 1; i <= 3; i++ {
		id := f.occ(t, i, "omitiu", "certa")
		f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}")
	}
	f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino'`)
	sw := &slowResponse{hdr: http.Header{}, gate: make(chan struct{}), started: make(chan struct{})}
	r := httptest.NewRequest("GET", "/monitor/api/treino/export?parte=treino", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("Cookie", f.cookie)
	done := make(chan struct{})
	go func() { f.h.ServeHTTP(sw, r); close(done) }()
	select {
	case <-sw.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the export did not start")
	}
	t0 := time.Now()
	for _, p := range []string{"/monitor/api/state", "/monitor/api/treino"} {
		if w := f.adm("GET", p, ""); w.Code != 200 {
			t.Errorf("%s while the export is stuck: %d", p, w.Code)
		}
	}
	id := f.occ(t, 9, "inventou", "certa") // a report goes through
	if w := f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}"); w.Code != 200 {
		t.Errorf("an action while the export is stuck: %d %s", w.Code, w.Body)
	}
	if time.Since(t0) > 4*time.Second {
		t.Errorf("took %v", time.Since(t0))
	}
	close(sw.gate)
	<-done
}

// ---- the page and the off switch ---------------------------------------------------------------------------

func TestTreinoOffAnswers503AndStateHasNoSummary(t *testing.T) {
	f := newFixture(t, true, queue.Config{GenSlots: 1})
	cookie, csrf := f.login(t)
	hdr := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	for _, c := range []struct{ m, p string }{{"GET", "/monitor/api/treino"}, {"GET", "/monitor/api/treino/" + tid(1)}, {"GET", "/monitor/api/treino/export?parte=teste"},
		{"POST", "/monitor/api/treino/" + tid(1) + "/confirmar"}, {"POST", "/monitor/api/jobs/" + tid(1) + "/marcar-erro"}} {
		if w := do(f.h, c.m, c.p, "{}", hdr); w.Code != 503 {
			t.Errorf("%s %s: %d", c.m, c.p, w.Code)
		}
	}
	if w := do(f.h, "GET", "/monitor/api/state", "", nil); strings.Contains(w.Body.String(), `"treino"`) {
		t.Error("the state shows a treino summary with the feature off")
	}
}

func TestPageHasTheErrorsTabAndAValidCSP(t *testing.T) {
	page := string(indexHTML)
	for _, want := range []string{`data-tab="erros"`, `id="tab-erros"`, `Erros (treino)`, `/monitor/api/treino`, `marcar-erro`, `liberar-quarentena`, `exemplo-publico`, `Marcar erro`} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	if strings.Contains(string(rawIndexHTML), "\r") {
		t.Error("index.html must stay in LF (the CSP hashes would not match a CRLF file)")
	}
	f := newTreinoFixture(t)
	w := do(f.h, "GET", "/monitor", "", nil)
	if w.Header().Get("Content-Security-Policy") == "" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'sha256-") {
		t.Error("no CSP hash on the page")
	}
	want, err := buildCSP(indexHTML)
	if err != nil || w.Header().Get("Content-Security-Policy") != want {
		t.Errorf("the CSP is not the one of the served page: %v", err)
	}
}

// ---- review of 12.6 ---------------------------------------------------------------------------

// I-3: the read paths of the board hold no context with a Done channel (pattern of 0.12.6): no
// WithTimeout, WithCancel or WithDeadline in the summary, the list, the detail and the export. The
// write routes keep their deadline, which only bounds the wait for the writers' turn.
func TestReadPathsUseNoContextWithDone(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "treino.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	reads := map[string]bool{"treinoSummary": true, "apiTreino": true, "apiTreinoDetail": true, "apiTreinoExport": true}
	seen := 0
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || !reads[fd.Name.Name] {
			continue
		}
		seen++
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					switch sel.Sel.Name {
					case "WithTimeout", "WithCancel", "WithDeadline":
						t.Errorf("%s calls context.%s", fd.Name.Name, sel.Sel.Name)
					case "Context":
						t.Errorf("%s uses the request context", fd.Name.Name)
					}
				}
			}
			return true
		})
	}
	if seen != len(reads) {
		t.Fatalf("found %d of the %d read handlers", seen, len(reads))
	}
}

// The page builds its DOM with textContent only: no HTML sinks and no inline handler or style attribute.
func TestIndexHasNoHTMLSinksNorInlineAttributes(t *testing.T) {
	page := string(indexHTML)
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "setAttribute(\"style\"", "setAttribute('style'", "eval(", "new Function("} {
		if strings.Contains(page, bad) {
			t.Errorf("index.html contains %s", bad)
		}
	}
	for _, cb := range []string{`setAttribute("on`, `setAttribute('on`} {
		if strings.Contains(page, cb) {
			t.Errorf("index.html sets an inline handler with %s", cb)
		}
	}
	// the markup outside <script> and <style>: no on*= and no style= attributes
	markup := regexp.MustCompile(`(?s)<script>.*?</script>|<style>.*?</style>`).ReplaceAllString(page, "")
	if m := regexp.MustCompile(`(?i)<[a-z][^>]*\s(on[a-z]+|style)\s*=`).FindString(markup); m != "" {
		t.Errorf("a tag carries an inline attribute: %s", m)
	}
}

// M-1: releasing a quarantine warns, in the answer, about the members already exported to the other part.
func TestReleaseAnswerWarnsAboutExportsToTheOtherPart(t *testing.T) {
	f := newTreinoFixture(t)
	a := f.occ(t, 1, "omitiu", "certa")
	b := f.occ(t, 2, "inventou", "certa")
	for _, id := range []string{a, b} {
		if w := f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}"); w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, b)
	f.adm("GET", "/monitor/api/treino/export?parte=teste", "")  // a went to the test file
	f.adm("GET", "/monitor/api/treino/export?parte=treino", "") // b to the training file
	rep := treino.Report{Jobs: []treino.JobRef{{ID: tid(1), Papel: "map1", Backend: "primario"}, {ID: tid(2), Papel: "final", Backend: "primario"}},
		Categoria: "omitiu", Descricao: "liga os dois", Severidade: "baixa", Origem: "claude"}
	if err := rep.Validate(); err != nil {
		t.Fatal(err)
	}
	res, err := f.ts.Ingest(context.Background(), rep, stOrigin, "", f.src, f.key)
	if err != nil {
		t.Fatal(err)
	}
	w := f.adm("POST", "/monitor/api/treino/"+res.OcorrenciaID+"/liberar-quarentena", `{"lado":"treino"}`)
	var out struct {
		Aviso string
		N     int `json:"exportados_na_outra_parte"`
		M     int `json:"membros"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.N != 1 || out.M != 3 || !strings.Contains(out.Aviso, "OUTRA parte") {
		t.Errorf("release answer: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(string(indexHTML), "r.body.aviso") {
		t.Error("the page does not show the warning")
	}
}

// The export has a deadline of its own, checked between the lines (the request context has no Done channel).
func TestExportRouteStopsAtItsDeadline(t *testing.T) {
	f := newTreinoFixture(t)
	for i := 1; i <= 3; i++ {
		id := f.occ(t, i, "omitiu", "certa")
		f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}")
	}
	f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino'`)
	old := treinoExportMax
	defer func() { treinoExportMax = old }()
	treinoExportMax = -time.Second // already past
	if w := f.adm("GET", "/monitor/api/treino/export?parte=treino", ""); strings.TrimSpace(w.Body.String()) != "" {
		t.Errorf("an export past its deadline sent lines: %q", w.Body)
	}
	treinoExportMax = old
	if w := f.adm("GET", "/monitor/api/treino/export?parte=treino", ""); len(strings.Split(strings.TrimSpace(w.Body.String()), "\n")) != 3 {
		t.Errorf("a normal export: %q", w.Body)
	}
}
