package treino

// Fase 12, etapa 12.4: ingestion of a report and the schema (contract C-1.x, C-2.9,
// C-2.12, C-4.3a, C-6.2). The state machine is in state_test.go, the route in
// http_test.go.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
)

func mkID(n int) string { return fmt.Sprintf("%032x", n) }

func testKey(t *testing.T) *hmackey.Key {
	t.Helper()
	k, err := hmackey.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Second)
	return c.t
}

func openStore(t *testing.T, o Options) *Store {
	t.Helper()
	if o.Now == nil {
		o.Now = (&clock{t: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)}).Now
	}
	if o.Key == nil {
		o.Key = testKey(t)
	}
	s, err := Open(filepath.Join(t.TempDir(), "treino.db"), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// fakeSrc is a Source with the jobs and contents the test puts in it.
type fakeSrc struct {
	mu       sync.Mutex
	jobs     map[string]Job
	conts    map[string]Content
	failJob  map[string]error // Content(id) fails with this error
	probes   []string
	ctxSeen  []ctxSeen
	contReqs []string
}

func newFake() *fakeSrc {
	return &fakeSrc{jobs: map[string]Job{}, conts: map[string]Content{}, failJob: map[string]error{}}
}

func pjOf(n int) []byte {
	return []byte(fmt.Sprintf(`{"model":"qwen3-ia","prompt":"pedido %d","options":{"seed":1}}`, n))
}

// add puts a job with a full content in the source.
func (f *fakeSrc) add(n int) string {
	id := mkID(n)
	f.jobs[id] = defJob()
	f.conts[id] = Content{Prompt: fmt.Sprintf("texto achatado %d", n), Response: fmt.Sprintf("resposta %d", n), PromptJSON: pjOf(n), RespFinal: true}
	return id
}

func (f *fakeSrc) Job(ctx context.Context, id string) (Job, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxSeen = append(f.ctxSeen, seen(ctx))
	j, ok := f.jobs[id]
	return j, ok, nil
}

func (f *fakeSrc) Probe(ctx context.Context, id string) (Probe, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxSeen = append(f.ctxSeen, seen(ctx))
	f.probes = append(f.probes, id)
	c, ok := f.conts[id]
	return Probe{OK: ok, HasJSON: ok && c.PromptJSON != nil, Bytes: int64(len(c.PromptJSON) + len(c.Prompt) + len(c.Response))}, nil
}

// defJob is a finished job made by the same origin as stationO.
func defJob() Job {
	return Job{Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia", Think: "off", Status: "ok", HTTPStatus: 200,
		StationComputer: "PC01", StationUser: "ana", ClientIP: "10.0.0.7"}
}

func (f *fakeSrc) Content(ctx context.Context, id string) (Content, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxSeen = append(f.ctxSeen, seen(ctx))
	f.contReqs = append(f.contReqs, id)
	if err := f.failJob[id]; err != nil {
		return Content{}, false, err
	}
	c, ok := f.conts[id]
	return c, ok, nil
}

func report(jobs ...JobRef) Report {
	r := Report{Projeto: "GestorERP", Jobs: jobs, Categoria: "numeracao_errada", Descricao: "pulou a secao 7.2",
		Severidade: "media", Origem: "claude"}
	return r
}

func jr(n int, papel, backend string) JobRef {
	return JobRef{ID: mkID(n), Papel: papel, Backend: backend}
}

func mustValid(t *testing.T, r Report) Report {
	t.Helper()
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return r
}

var stationO = Origin{Computer: "PC01", User: "ana", Domain: "csl", MAC: "AABBCCDDEEFF", ClientIP: "10.0.0.7", ClientLabel: "PC01/ana"}

func ingest(t *testing.T, s *Store, r Report, role string, src Source) Result {
	t.Helper()
	res, err := s.Ingest(context.Background(), mustValid(t, r), stationO, role, src, testKey(t))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	return res
}

func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func estadoOf(t *testing.T, s *Store, id string) string {
	t.Helper()
	var e string
	if err := s.db.QueryRow(`SELECT estado FROM ocorrencias WHERE id=?`, id).Scan(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

// ---- C-1.16, C-1.17, C-1.3: validation ------------------------------------------

func TestValidateEnums(t *testing.T) { // C-1.16
	one := jr(1, "unico", "primario")
	for name, mut := range map[string]func(*Report){
		"categoria":  func(r *Report) { r.Categoria = "erro_qualquer" },
		"origem":     func(r *Report) { r.Origem = "admin" },
		"severidade": func(r *Report) { r.Severidade = "critica" },
		"vazia":      func(r *Report) { r.Categoria = "" },
	} {
		r := report(one)
		mut(&r)
		var ve *ValidationError
		if err := r.Validate(); !errors.As(err, &ve) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for c := range categorias {
		r := report(one)
		r.Categoria = c
		if err := r.Validate(); err != nil {
			t.Errorf("categoria %s refused: %v", c, err)
		}
	}
	for _, o := range []string{"claude", "usuario", "automatico"} {
		r := report(one)
		r.Origem = o
		if err := r.Validate(); err != nil {
			t.Errorf("origem %s refused: %v", o, err)
		}
	}
}

func TestValidateLimits(t *testing.T) { // C-1.17
	one := jr(1, "unico", "primario")
	r := report(one)
	r.Descricao = strings.Repeat("a", MaxDescricao)
	r.RespostaCorreta = strings.Repeat("b", MaxResposta)
	if err := r.Validate(); err != nil {
		t.Fatalf("at the limits: %v", err)
	}
	r.Descricao += "a"
	if r.Validate() == nil {
		t.Error("descricao of 4 KB + 1 accepted")
	}
	r = report(one)
	r.RespostaCorreta = strings.Repeat("b", MaxResposta+1)
	if r.Validate() == nil {
		t.Error("resposta_correta of 128 KB + 1 accepted")
	}
	var jobs []JobRef
	for i := 1; i <= MaxJobs; i++ {
		papel := fmt.Sprintf("map%d", i)
		if i == MaxJobs {
			papel = "final"
		}
		jobs = append(jobs, jr(i, papel, "primario"))
	}
	r = report(jobs...)
	if err := r.Validate(); err != nil {
		t.Fatalf("64 jobs refused: %v", err)
	}
	r = report(append(jobs, jr(99, "reduce1", "primario"))...)
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "64") {
		t.Errorf("65 jobs: %v", err)
	}
	r = report()
	if r.Validate() == nil {
		t.Error("no jobs accepted")
	}
}

func TestValidateJobList(t *testing.T) { // C-1.2
	bad := map[string][]JobRef{
		"id curto":       {{ID: "abc", Papel: "unico", Backend: "primario"}},
		"id maiusculo":   {{ID: strings.ToUpper(mkID(0xabc)), Papel: "unico", Backend: "primario"}},
		"papel":          {jr(1, "mapa", "primario")},
		"map0":           {jr(1, "map0", "primario")},
		"backend":        {jr(1, "unico", "gpu-local")},
		"id repetido":    {jr(1, "map1", "primario"), jr(1, "map2", "primario")},
		"papel repetido": {jr(1, "map1", "primario"), jr(2, "map1", "primario")},
		"dois finals":    {jr(1, "final", "primario"), jr(2, "final", "primario")},
		"unico com dois": {jr(1, "unico", "primario"), jr(2, "map1", "primario")},
	}
	for name, jobs := range bad {
		r := report(jobs...)
		r.JobDoErro = mkID(1)
		if r.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, p := range []string{"map1", "map12", "reduce3", "final", "unico"} {
		if !ValidPapel(p) {
			t.Errorf("papel %s refused", p)
		}
	}
}

func TestJobDoErroRules(t *testing.T) { // C-1.3
	r := report(jr(1, "map1", "primario"), jr(2, "map2", "primario"), jr(3, "reduce1", "primario"), jr(4, "final", "primario"))
	if err := r.Validate(); err != nil || r.JobDoErro != mkID(4) {
		t.Errorf("default must be the final: %v %q", err, r.JobDoErro)
	}
	r = report(jr(1, "unico", "primario"))
	if err := r.Validate(); err != nil || r.JobDoErro != mkID(1) {
		t.Errorf("a single job is the job_do_erro: %v %q", err, r.JobDoErro)
	}
	// translate: the parts have no final, so it is required
	r = report(jr(1, "map1", "primario"), jr(2, "map2", "primario"))
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "job_do_erro") {
		t.Errorf("a list without final and without job_do_erro was accepted: %v", err)
	}
	r = report(jr(1, "map1", "primario"), jr(2, "map2", "primario"), jr(3, "reduce1", "primario"))
	if r.Validate() == nil {
		t.Error("a list of several jobs without final and job_do_erro was accepted")
	}
	r = report(jr(1, "map1", "primario"), jr(2, "map2", "primario"))
	r.JobDoErro = mkID(2)
	if err := r.Validate(); err != nil || r.JobDoErro != mkID(2) {
		t.Errorf("explicit job_do_erro: %v", err)
	}
	r.JobDoErro = mkID(9)
	if r.Validate() == nil {
		t.Error("a job_do_erro outside the list was accepted")
	}
	r.JobDoErro = "xyz"
	if r.Validate() == nil {
		t.Error("a malformed job_do_erro was accepted")
	}
}

// ---- C-1.4: a job that this gateway does not know --------------------------------

func TestUnknownJobDoErroWritesNothing(t *testing.T) { // C-1.4
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	r := report(jr(1, "map1", "primario"), jr(7, "final", "primario")) // the final (7) is unknown
	_, err := s.Ingest(context.Background(), mustValid(t, r), stationO, "", f, testKey(t))
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v", err)
	}
	for _, tb := range []string{"ocorrencias", "ocorrencia_jobs", "amostras", "historico"} {
		if n := count(t, s, tb); n != 0 {
			t.Errorf("%s has %d rows after a 404", tb, n)
		}
	}
}

// ---- C-1.5, C-1.6, C-1.7, C-2.9, C-2.12: the samples ------------------------------

func TestIngestCopiesEverySampleInOneTransaction(t *testing.T) { // C-1.5
	s := openStore(t, Options{})
	f := newFake()
	for i := 1; i <= 3; i++ {
		f.add(i)
	}
	res := ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "map2", "primario"), jr(3, "final", "primario")), "", f)
	if res.Estado != EstadoNova || res.AmostrasCopiadas != 3 || res.SemPedido != 0 || res.JobsExternos != 0 {
		t.Fatalf("result = %+v", res)
	}
	if count(t, s, "amostras") != 3 || count(t, s, "ocorrencia_jobs") != 3 || count(t, s, "ocorrencias") != 1 {
		t.Fatalf("counts: amostras=%d jobs=%d oc=%d", count(t, s, "amostras"), count(t, s, "ocorrencia_jobs"), count(t, s, "ocorrencias"))
	}
	// a failure on the third sample leaves nothing at all
	s2 := openStore(t, Options{})
	f.failJob[mkID(3)] = errors.New("content.db caiu")
	_, err := s2.Ingest(context.Background(), mustValid(t, report(jr(1, "map1", "primario"), jr(2, "map2", "primario"), jr(3, "final", "primario"))),
		stationO, "", f, testKey(t))
	if err == nil {
		t.Fatal("the failure was swallowed")
	}
	for _, tb := range []string{"ocorrencias", "ocorrencia_jobs", "amostras", "historico"} {
		if n := count(t, s2, tb); n != 0 {
			t.Errorf("%s has %d rows after a failed transaction", tb, n)
		}
	}
}

func TestSampleIsSharedBetweenOccurrences(t *testing.T) { // C-1.6
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	f.add(3)
	r1 := ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "final", "primario")), "", f)
	r2 := ingest(t, s, report(jr(1, "map1", "primario"), jr(3, "final", "primario")), "", f)
	if r1.OcorrenciaID == r2.OcorrenciaID {
		t.Fatal("two reports made one occurrence")
	}
	if n := count(t, s, "amostras"); n != 3 {
		t.Errorf("amostras = %d, want 3 (job 1 shared)", n)
	}
	if n := count(t, s, "ocorrencia_jobs"); n != 4 {
		t.Errorf("ocorrencia_jobs = %d, want 4", n)
	}
	if r2.AmostrasCopiadas != 1 {
		t.Errorf("the second report copied %d, want 1 (the shared one is not copied again)", r2.AmostrasCopiadas)
	}
}

func TestSampleKeepsEveryMarkAndHMACs(t *testing.T) { // C-1.7, C-2.9, C-2.12
	s := openStore(t, Options{})
	f := newFake()
	id := f.add(1)
	k := testKey(t)
	f.jobs[id] = Job{Tool: "summarize_file", Endpoint: "/api/chat", Model: "qwen3-ia", Think: "on", Status: "erro", HTTPStatus: 500,
		DocHash: strings.Repeat("d", 64), DocKeyID: k.ID(), StationComputer: "PC01", StationUser: "ana", ClientIP: "10.0.0.7"}
	pj := []byte(`{"model":"qwen3-ia","options":{"seed":3},"messages":[{"role":"user","content":"oi"}],"think":true}`)
	f.conts[id] = Content{Prompt: "oi achatado", Response: "parcial", PromptJSON: pj, PromptTruncated: true, PromptJSONTruncated: true,
		HasImage: true, RespFinal: false, RespTruncated: true}
	ingest(t, s, report(jr(1, "unico", "primario")), "", f)
	var a struct {
		endpoint, modelo, think, ph, ch, kid, dh, status string
		pt, pjt, img, rf, rt, http                       int
		pjb, texto, resp                                 []byte
	}
	err := s.db.QueryRow(`SELECT endpoint,modelo,think,prompt_json,prompt_texto,resposta,prompt_hmac,conteudo_hmac,key_id,prompt_truncado,
		prompt_json_truncado,tem_imagem,doc_hash,resp_final,resp_truncada,status,http_status FROM amostras WHERE job_id=?`, id).
		Scan(&a.endpoint, &a.modelo, &a.think, &a.pjb, &a.texto, &a.resp, &a.ph, &a.ch, &a.kid, &a.pt, &a.pjt, &a.img, &a.dh, &a.rf, &a.rt, &a.status, &a.http)
	if err != nil {
		t.Fatal(err)
	}
	if a.endpoint != "/api/chat" || a.modelo != "qwen3-ia" || a.think != "on" || string(a.pjb) != string(pj) || string(a.texto) != "oi achatado" || string(a.resp) != "parcial" {
		t.Errorf("copied columns wrong: %+v", a)
	}
	if a.pt != 1 || a.pjt != 1 || a.img != 1 || a.rf != 0 || a.rt != 1 || a.status != "erro" || a.http != 500 {
		t.Errorf("marks not copied: pt=%d pjt=%d img=%d rf=%d rt=%d status=%q http=%d", a.pt, a.pjt, a.img, a.rf, a.rt, a.status, a.http)
	}
	wantCH, err := k.ContentHMAC(pj)
	if err != nil {
		t.Fatal(err)
	}
	if a.ph != k.PromptHMAC(pj) || a.ch != wantCH || a.kid != k.ID() || a.ph == "" || a.ch == "" {
		t.Errorf("hmacs: prompt=%q content=%q key_id=%q", a.ph, a.ch, a.kid)
	}
	if a.dh != strings.Repeat("d", 64) {
		t.Errorf("doc_hash = %q", a.dh)
	}
	// every mark true -> false: a second job with the other values
	id2 := f.add(2)
	f.conts[id2] = Content{Prompt: "p", Response: "r", PromptJSON: pjOf(2), RespFinal: true}
	ingest(t, s, report(jr(2, "unico", "primario")), "", f)
	err = s.db.QueryRow(`SELECT prompt_truncado,prompt_json_truncado,tem_imagem,resp_final,resp_truncada,status,http_status FROM amostras WHERE job_id=?`, id2).
		Scan(&a.pt, &a.pjt, &a.img, &a.rf, &a.rt, &a.status, &a.http)
	if err != nil || a.pt != 0 || a.pjt != 0 || a.img != 0 || a.rf != 1 || a.rt != 0 || a.status != "ok" || a.http != 200 {
		t.Errorf("clean marks: %v %+v", err, a)
	}
}

func TestSampleDocHashOnlyWithTheCurrentKey(t *testing.T) { // C-1.7 (single key_id), C-4.3a
	s := openStore(t, Options{})
	f := newFake()
	id := f.add(1)
	j := f.jobs[id]
	j.DocHash, j.DocKeyID = strings.Repeat("e", 64), "0000000000000000" // made with another key
	f.jobs[id] = j
	ingest(t, s, report(jr(1, "unico", "primario")), "", f)
	var dh sql.NullString
	if err := s.db.QueryRow(`SELECT doc_hash FROM amostras WHERE job_id=?`, id).Scan(&dh); err != nil || dh.String != "" {
		t.Errorf("a doc_hash of another key was kept: %v %q", err, dh.String)
	}
	var och, okid sql.NullString
	if err := s.db.QueryRow(`SELECT doc_hash, doc_key_id FROM ocorrencias`).Scan(&och, &okid); err != nil || och.String == "" || okid.String != "0000000000000000" {
		t.Errorf("the occurrence keeps the pair as the log has it: %v %q %q", err, och.String, okid.String)
	}
}

// ---- C-1.9, C-1.9a, C-1.10, C-1.21: the state --------------------------------------

func TestSemPedidoOnlyByTheJobDoErro(t *testing.T) { // C-1.9, C-1.9a
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	f.add(3)
	// the job_do_erro (the final, 3) expired from the content database
	delete(f.conts, mkID(3))
	r := ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "map2", "primario"), jr(3, "final", "primario")), "", f)
	if r.Estado != EstadoSemPedido || r.SemPedido != 1 || r.AmostrasCopiadas != 2 {
		t.Errorf("job_do_erro expired: %+v", r)
	}
	// the job_do_erro has content but no canonical request (job before 0.13.0)
	f2 := newFake()
	id := f2.add(1)
	c := f2.conts[id]
	c.PromptJSON = nil
	f2.conts[id] = c
	s = openStore(t, Options{}) // a job that already has a sample is not read again (M-1)
	r = ingest(t, s, report(jr(1, "unico", "primario")), "", f2)
	if r.Estado != EstadoSemPedido || r.AmostrasCopiadas != 0 || r.SemPedido != 1 {
		t.Errorf("no prompt_json: %+v", r)
	}
	// only jobs that are NOT the job_do_erro expired: nova, existing samples copied, references kept (C-1.9a)
	f3 := newFake()
	f3.add(1)
	f3.add(2)
	f3.add(3)
	delete(f3.conts, mkID(1))
	delete(f3.conts, mkID(2))
	s = openStore(t, Options{})
	r = ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "map2", "primario"), jr(3, "final", "primario")), "", f3)
	if r.Estado != EstadoNova || r.AmostrasCopiadas != 1 || r.SemPedido != 2 {
		t.Errorf("only the others expired: %+v", r)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ocorrencia_jobs WHERE ocorrencia_id=?`, r.OcorrenciaID).Scan(&n); err != nil || n != 3 {
		t.Errorf("the expired jobs must stay as references: %d %v", n, err)
	}
}

func TestDistribuidaCopiesNoSample(t *testing.T) { // C-1.10
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	f.add(3)
	r := ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "map2", "fallback"), jr(3, "final", "primario")), "", f)
	if r.Estado != EstadoDistrib || r.AmostrasCopiadas != 0 || r.JobsExternos != 1 {
		t.Fatalf("result = %+v", r)
	}
	if count(t, s, "amostras") != 0 {
		t.Error("a distribuida copied a sample, not even the local ones")
	}
	rows, err := s.db.Query(`SELECT job_id, backend FROM ocorrencia_jobs WHERE ocorrencia_id=? ORDER BY ordem`, r.OcorrenciaID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id, b string
		rows.Scan(&id, &b)
		got = append(got, b)
	}
	if strings.Join(got, ",") != "primario,fallback,primario" {
		t.Errorf("references = %v", got)
	}
	for _, id := range f.contReqs {
		t.Errorf("content of %s was read for a distribuida", id)
	}
	// the jobs of the other backend are references without local validation: only the job_do_erro must exist
	r = ingest(t, s, report(jr(3, "final", "primario"), jr(500, "map1", "fallback"), jr(501, "map2", "fallback")), "", f)
	if r.Estado != EstadoDistrib || r.JobsExternos != 2 {
		t.Errorf("external jobs unknown here: %+v", r)
	}
}

func TestFallbackRoleCopiesNoSample(t *testing.T) { // C-1.21
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	r := ingest(t, s, report(jr(1, "map1", "fallback"), jr(2, "final", "fallback")), RoleFallback, f)
	if r.Estado != EstadoFallback || r.AmostrasCopiadas != 0 || r.JobsExternos != 0 {
		t.Fatalf("result = %+v", r)
	}
	if count(t, s, "amostras") != 0 || count(t, s, "ocorrencia_jobs") != 2 {
		t.Errorf("amostras=%d jobs=%d", count(t, s, "amostras"), count(t, s, "ocorrencia_jobs"))
	}
	// the same report on the primary gateway is a nova with samples
	s2 := openStore(t, Options{})
	r = ingest(t, s2, report(jr(1, "map1", "primario"), jr(2, "final", "primario")), "", f)
	if r.Estado != EstadoNova || r.AmostrasCopiadas != 2 {
		t.Errorf("primary: %+v", r)
	}
	// an unknown role is the primary
	s3 := openStore(t, Options{})
	r = ingest(t, s3, report(jr(1, "map1", "primario"), jr(2, "final", "primario")), "qualquer", f)
	if r.Estado != EstadoNova {
		t.Errorf("unknown role: %+v", r)
	}
}

func TestStatePrecedence(t *testing.T) { // C-1.9: distribuida > fallback > sem_pedido > nova
	f := newFake()
	f.add(1)
	f.add(2)
	delete(f.conts, mkID(2)) // the job_do_erro (final) has no content
	mixed := report(jr(1, "map1", "primario"), jr(2, "final", "fallback"))
	ok := report(jr(1, "map1", "primario"), jr(2, "final", "primario"))
	cases := []struct {
		name string
		r    Report
		role string
		want string
	}{
		{"distribuida beats fallback", mixed, RoleFallback, EstadoDistrib},
		{"distribuida beats sem_pedido", mixed, RolePrimario, EstadoDistrib},
		{"fallback beats sem_pedido", ok, RoleFallback, EstadoFallback},
		{"sem_pedido beats nova", ok, RolePrimario, EstadoSemPedido},
	}
	for _, c := range cases {
		s := openStore(t, Options{})
		if got := ingest(t, s, c.r, c.role, f).Estado; got != c.want {
			t.Errorf("%s: %s", c.name, got)
		}
	}
}

// ---- C-1.19: the automatic candidate ------------------------------------------------

func TestHumanReportReplacesTheAutomaticCandidate(t *testing.T) { // C-1.19
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	auto := report(jr(1, "unico", "primario"))
	auto.Origem, auto.Descricao = "automatico", "SEGREDO-AUTO-7"
	ra := ingest(t, s, auto, "", f)
	var resp string
	s.db.QueryRow(`SELECT COALESCE(resposta_correta,'') FROM ocorrencias WHERE id=?`, ra.OcorrenciaID).Scan(&resp)
	if resp != "" || ra.Estado != EstadoNova {
		t.Fatalf("automatic: %q %+v", resp, ra)
	}
	// the same automatic rule again: the same occurrence, nothing written
	hn := count(t, s, "historico")
	auto2 := auto
	auto2.Descricao = "outra descricao do mesmo candidato"
	again := ingest(t, s, auto2, "", f)
	if again.OcorrenciaID != ra.OcorrenciaID || count(t, s, "ocorrencias") != 1 || count(t, s, "historico") != hn {
		t.Errorf("duplicate automatic: %+v oc=%d", again, count(t, s, "ocorrencias"))
	}
	var desc string
	s.db.QueryRow(`SELECT descricao FROM ocorrencias WHERE id=?`, ra.OcorrenciaID).Scan(&desc)
	if desc != "SEGREDO-AUTO-7" {
		t.Errorf("a repeated automatic candidate changed the first one: %q", desc)
	}
	human := report(jr(1, "unico", "primario"))
	human.RespostaCorreta = "{\"ok\":true}"
	human.Descricao = "SEGREDO-HUM-7"
	rh := ingest(t, s, human, "", f)
	if rh.OcorrenciaID != ra.OcorrenciaID {
		t.Fatalf("the human report made a new occurrence: %s vs %s", rh.OcorrenciaID, ra.OcorrenciaID)
	}
	if count(t, s, "ocorrencias") != 1 {
		t.Errorf("occurrences = %d", count(t, s, "ocorrencias"))
	}
	var origem, antes, depois, quem string
	err := s.db.QueryRow(`SELECT o.origem, h.antes, h.depois, h.quem FROM ocorrencias o JOIN historico h ON h.ocorrencia_id=o.id
		WHERE o.id=? AND h.acao='alterou' AND h.campo='origem'`, ra.OcorrenciaID).Scan(&origem, &antes, &depois, &quem)
	if err != nil || origem != "claude" || antes != "automatico" || depois != "claude" || quem != "PC01/ana" {
		t.Errorf("history of the replacement: %v %q %q %q %q", err, origem, antes, depois, quem)
	}
	s.db.QueryRow(`SELECT resposta_correta FROM ocorrencias WHERE id=?`, ra.OcorrenciaID).Scan(&resp)
	if resp != human.RespostaCorreta {
		t.Errorf("resposta_correta not taken from the human report: %q", resp)
	}
	// the replacement is in the history as hash and size only (C-7.4)
	if h := dumpTable(t, s, "historico"); strings.Contains(h, "SEGREDO") || strings.Contains(h, "{\"ok\":true}") {
		t.Errorf("the history of the replacement holds text: %s", h)
	}
	var dh string
	s.db.QueryRow(`SELECT depois FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='descricao'`, ra.OcorrenciaID).Scan(&dh)
	if !strings.HasPrefix(dh, "hmac="+testKey(t).MACString("SEGREDO-HUM-7")+" key_id=") {
		t.Errorf("the replacement history of descricao is not an HMAC: %q", dh)
	}
	// a human report of a job whose occurrence is already human is a new occurrence (no replacement)
	rh2 := ingest(t, s, human, "", f)
	if rh2.OcorrenciaID == ra.OcorrenciaID {
		t.Error("a human report replaced a human one")
	}
}

// ---- C-4.3a, C-6.2: refusals -----------------------------------------------------------

func TestIngestWithoutKeyIsRefused(t *testing.T) { // C-4.3a
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	for _, k := range []*hmackey.Key{nil} {
		if _, err := s.Ingest(context.Background(), mustValid(t, report(jr(1, "unico", "primario"))), stationO, "", f, k); !errors.Is(err, ErrNoKey) {
			t.Errorf("err = %v", err)
		}
	}
	if count(t, s, "ocorrencias") != 0 {
		t.Error("wrote without a key")
	}
}

func TestCapRefusesWithoutDeletingAnything(t *testing.T) { // C-6.2
	var events []string
	s := openStore(t, Options{OnEvent: func(k, d string) { events = append(events, k) }})
	f := newFake()
	f.add(1)
	f.add(2)
	f.add(3)
	r1 := ingest(t, s, report(jr(1, "unico", "primario")), "", f)
	before := [4]int{count(t, s, "ocorrencias"), count(t, s, "amostras"), count(t, s, "historico"), count(t, s, "ocorrencia_jobs")}
	// the file with its -wal counts: a ceiling at the current size refuses
	s.opts.MaxBytes = s.Size()
	_, err := s.Ingest(context.Background(), mustValid(t, report(jr(2, "unico", "primario"))), stationO, "", f, testKey(t))
	if !errors.Is(err, ErrCapReached) {
		t.Fatalf("err = %v", err)
	}
	after := [4]int{count(t, s, "ocorrencias"), count(t, s, "amostras"), count(t, s, "historico"), count(t, s, "ocorrencia_jobs")}
	if before != after {
		t.Errorf("the refusal changed the data: %v -> %v", before, after)
	}
	if estadoOf(t, s, r1.OcorrenciaID) != EstadoNova {
		t.Error("an existing occurrence changed")
	}
	if len(events) != 1 || events[0] != "treino_teto_atingido" {
		t.Errorf("events = %v", events)
	}
	// the ceiling refuses before reading anything: a report that would copy no sample is refused too
	mixed := report(jr(2, "final", "primario"), jr(501, "map1", "fallback"))
	if _, err := s.Ingest(context.Background(), mustValid(t, mixed), stationO, "", f, testKey(t)); !errors.Is(err, ErrCapReached) {
		t.Errorf("a distribuida (no sample) at the ceiling: %v", err)
	}
	// a second refusal within the minute does not repeat the event
	s.Ingest(context.Background(), mustValid(t, report(jr(3, "unico", "primario"))), stationO, "", f, testKey(t))
	if len(events) != 1 {
		t.Errorf("event repeated: %v", events)
	}
	// the ceiling also stops a report that would cross it halfway: nothing of it stays
	s2 := openStore(t, Options{})
	f2 := newFake()
	f2.add(1)
	f2.add(2)
	c := f2.conts[mkID(2)]
	c.Response = strings.Repeat("x", 4096)
	f2.conts[mkID(2)] = c
	s2.opts.MaxBytes = s2.Size() + 2048
	_, err = s2.Ingest(context.Background(), mustValid(t, report(jr(1, "map1", "primario"), jr(2, "final", "primario"))), stationO, "", f2, testKey(t))
	if !errors.Is(err, ErrCapReached) {
		t.Fatalf("mid-transaction: %v", err)
	}
	if count(t, s2, "ocorrencias") != 0 || count(t, s2, "amostras") != 0 || count(t, s2, "historico") != 0 {
		t.Error("a report that crossed the ceiling left rows behind")
	}
}

func TestFileIs0600AndMigrationIsIdempotent(t *testing.T) { // C-6.1, §8.2
	path := filepath.Join(t.TempDir(), "treino.db")
	s, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	f := newFake()
	f.add(1)
	res := ingest(t, s, report(jr(1, "unico", "primario")), "", f)
	s.Close()
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", fi.Mode().Perm())
		}
	}
	for i := 0; i < 3; i++ {
		s, err = Open(path, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if estadoOf(t, s, res.OcorrenciaID) != EstadoNova || count(t, s, "amostras") != 1 || count(t, s, "historico") != 1 {
			t.Fatalf("reopen %d lost data", i)
		}
		var v int
		s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != schemaVersion {
			t.Errorf("user_version = %d", v)
		}
		s.Close()
	}
	// a database of a newer version is refused, not changed
	db, _ := sql.Open("sqlite", path)
	db.Exec(`PRAGMA user_version=99`)
	db.Close()
	if _, err := Open(path, Options{}); err == nil {
		t.Error("a newer treino.db was opened")
	}
}

// ctxSeen is what a Source saw about the context it was called with, at call time.
type ctxSeen struct {
	err         error
	hasDeadline bool
}

func seen(c context.Context) ctxSeen {
	_, d := c.Deadline()
	return ctxSeen{err: c.Err(), hasDeadline: d}
}
