package treino

// Fase 12, etapa 12.4, correções da revisão independente (contrato 1.1.2):
// C-1.4a, C-1.11a, C-1.12c, C-1.12d, C-4.11b, C-6.2, C-6.2a, M-1, M-2.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func httptestRequest(body, remote string) *http.Request {
	req := httptest.NewRequest("POST", "/api/feedback", strings.NewReader(body))
	req.RemoteAddr = remote
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-Ip", "10.0.0.7")
	req.Header.Set("X-Mcpollama-Station", "computer=PC01;user=ana")
	return req
}

func httptestRecord(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func ingestAs(t *testing.T, s *Store, r Report, o Origin, f Source) Result {
	t.Helper()
	res, err := s.Ingest(context.Background(), mustValid(t, r), o, "", f, testKey(t))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	return res
}

func histRows(t *testing.T, s *Store, id, acao, campo string) (antes, depois []string) {
	t.Helper()
	rows, err := s.db.Query(`SELECT COALESCE(antes,''), COALESCE(depois,'') FROM historico WHERE ocorrencia_id=? AND acao=? AND campo=? ORDER BY id`, id, acao, campo)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a, d string
		rows.Scan(&a, &d)
		antes, depois = append(antes, a), append(depois, d)
	}
	return
}

func jobRows(t *testing.T, s *Store, id string) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT papel, job_id, backend FROM ocorrencia_jobs WHERE ocorrencia_id=? ORDER BY ordem`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p, j, b string
		rows.Scan(&p, &j, &b)
		out = append(out, p+":"+j+"@"+b)
	}
	return out
}

// ---- C-1.12d: the replacement of the automatic candidate ----------------------------

func TestReplacementRecomputesTheStateOnAMixedList(t *testing.T) { // C-1.12d
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	auto := report(jr(1, "unico", "primario"))
	auto.Origem = "automatico"
	ra := ingest(t, s, auto, "", f)
	if ra.Estado != EstadoNova || count(t, s, "amostras") != 1 {
		t.Fatalf("automatic: %+v", ra)
	}
	// X (1) is local, Y (7) ran on the fallback: the new list is distribuida
	human := report(jr(7, "map1", "fallback"), jr(1, "final", "primario"))
	rh := ingest(t, s, human, "", f)
	if rh.OcorrenciaID != ra.OcorrenciaID || rh.Estado != EstadoDistrib || rh.AmostrasCopiadas != 0 || rh.JobsExternos != 1 {
		t.Fatalf("replacement: %+v", rh)
	}
	if estadoOf(t, s, ra.OcorrenciaID) != EstadoDistrib {
		t.Errorf("estado = %s", estadoOf(t, s, ra.OcorrenciaID))
	}
	if n := count(t, s, "amostras"); n != 0 {
		t.Errorf("a distribuida kept %d samples (the one the replaced copied must go)", n)
	}
	if got := strings.Join(jobRows(t, s, ra.OcorrenciaID), ","); got != "map1:"+mkID(7)+"@fallback,final:"+mkID(1)+"@primario" {
		t.Errorf("jobs = %s", got)
	}
	a, d := histRows(t, s, ra.OcorrenciaID, "alterou", "estado")
	if len(a) != 1 || a[0] != "nova" || d[0] != "distribuida" {
		t.Errorf("estado history: %v %v", a, d)
	}
	if a, _ := histRows(t, s, ra.OcorrenciaID, "alterou", "jobs"); len(a) != 1 {
		t.Errorf("jobs history rows = %d", len(a))
	}
}

func TestReplacementKeepsASampleAnotherOccurrenceUses(t *testing.T) { // C-1.12d, C-6.4
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	// Z uses job 1 as a part
	ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "final", "primario")), "", f)
	if count(t, s, "amostras") != 2 {
		t.Fatal("setup")
	}
	auto := report(jr(1, "unico", "primario"))
	auto.Origem = "automatico"
	ra := ingest(t, s, auto, "", f)
	human := report(jr(7, "map1", "fallback"), jr(1, "final", "primario"))
	if rh := ingest(t, s, human, "", f); rh.OcorrenciaID != ra.OcorrenciaID || rh.Estado != EstadoDistrib {
		t.Fatalf("replacement: %+v", rh)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM amostras WHERE job_id=?`, mkID(1)).Scan(&n)
	if n != 1 {
		t.Error("a sample that another occurrence still references was removed")
	}
}

func TestReplacementSwapsTheJobList(t *testing.T) { // C-1.12d: swapped, not summed
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	auto := report(jr(1, "unico", "primario"))
	auto.Origem = "automatico"
	ra := ingest(t, s, auto, "", f)
	rh := ingest(t, s, report(jr(2, "map1", "primario"), jr(1, "final", "primario")), "", f)
	if rh.OcorrenciaID != ra.OcorrenciaID || rh.Estado != EstadoNova || rh.AmostrasCopiadas != 1 {
		t.Fatalf("replacement: %+v", rh)
	}
	if got := strings.Join(jobRows(t, s, ra.OcorrenciaID), ","); got != "map1:"+mkID(2)+"@primario,final:"+mkID(1)+"@primario" {
		t.Errorf("jobs = %s (the old unico row must be gone)", got)
	}
}

func TestReplacementDropsTheSamplesOfJobsThatLeftTheList(t *testing.T) { // C-1.12d, C-6.4
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	auto := report(jr(1, "map1", "primario"), jr(2, "final", "primario"))
	auto.Origem = "automatico"
	ra := ingest(t, s, auto, "", f)
	if count(t, s, "amostras") != 2 {
		t.Fatal("setup")
	}
	rh := ingest(t, s, report(jr(2, "unico", "primario")), "", f)
	if rh.OcorrenciaID != ra.OcorrenciaID || rh.Estado != EstadoNova {
		t.Fatalf("replacement: %+v", rh)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM amostras WHERE job_id=?`, mkID(1)).Scan(&n)
	if n != 0 || count(t, s, "amostras") != 1 {
		t.Errorf("the sample of the job that left the list stayed (amostras=%d)", count(t, s, "amostras"))
	}
}

func TestReplacementHistoryCoversStationAndOrigin(t *testing.T) { // C-1.12d, C-1.19
	s := openStore(t, Options{})
	f := newFake()
	id := f.add(1)
	j := f.jobs[id]
	j.ClientIP = "2001:db8::/64" // the job log groups IPv6 by /64
	f.jobs[id] = j
	oa := Origin{Computer: "PC01", User: "ana", Domain: "d1", MAC: "AAAAAAAAAAAA", ClientIP: "2001:db8::1", ClientLabel: "L1"}
	ob := Origin{Computer: "PC01", User: "ana", Domain: "d2", MAC: "BBBBBBBBBBBB", ClientIP: "2001:db8::2", ClientLabel: "L2"}
	auto := report(jr(1, "unico", "primario"))
	auto.Origem = "automatico"
	ra := ingestAs(t, s, auto, oa, f)
	rh := ingestAs(t, s, report(jr(1, "unico", "primario")), ob, f)
	if rh.OcorrenciaID != ra.OcorrenciaID {
		t.Fatalf("not the same occurrence: %+v", rh)
	}
	for campo, want := range map[string][2]string{
		"origem": {"automatico", "claude"}, "estacao_domain": {"d1", "d2"}, "estacao_mac": {"AAAAAAAAAAAA", "BBBBBBBBBBBB"},
		"client_ip": {"2001:db8::1", "2001:db8::2"}, "client_label": {"L1", "L2"},
	} {
		a, d := histRows(t, s, ra.OcorrenciaID, "alterou", campo)
		if len(a) != 1 || a[0] != want[0] || d[0] != want[1] {
			t.Errorf("%s: %v %v, want %v", campo, a, d, want)
		}
	}
	for _, campo := range []string{"estacao_computer", "estacao_user"} {
		if a, _ := histRows(t, s, ra.OcorrenciaID, "alterou", campo); len(a) != 0 {
			t.Errorf("%s did not change but has history", campo)
		}
	}
	var dom, mac, ip, lb string
	s.db.QueryRow(`SELECT estacao_domain, estacao_mac, client_ip, client_label FROM ocorrencias WHERE id=?`, ra.OcorrenciaID).Scan(&dom, &mac, &ip, &lb)
	if dom != "d2" || mac != "BBBBBBBBBBBB" || ip != "2001:db8::2" || lb != "L2" {
		t.Errorf("fields: %s %s %s %s", dom, mac, ip, lb)
	}
}

// ---- C-1.4a: a job still in the line or running ---------------------------------------

func TestBusyJobIsRefusedWithoutWriting(t *testing.T) { // C-1.4a
	for _, st := range []string{"na_fila", "em_execucao"} {
		for _, which := range []string{"job_do_erro", "other local job"} {
			s := openStore(t, Options{})
			f := newFake()
			f.add(1)
			f.add(2)
			target := mkID(2)
			if which == "other local job" {
				target = mkID(1)
			}
			j := f.jobs[target]
			j.Status = st
			f.jobs[target] = j
			_, err := s.Ingest(ctx, mustValid(t, report(jr(1, "map1", "primario"), jr(2, "final", "primario"))), stationO, "", f, testKey(t))
			if !errors.Is(err, ErrJobBusy) {
				t.Errorf("%s %s: %v", st, which, err)
			}
			empty2(t, s)
		}
	}
	// a job of the other backend is not checked here, and an unknown local one is only a reference
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	if _, err := s.Ingest(ctx, mustValid(t, report(jr(1, "final", "primario"), jr(9, "map1", "fallback"))), stationO, "", f, testKey(t)); err != nil {
		t.Errorf("external job: %v", err)
	}
	if _, err := s.Ingest(ctx, mustValid(t, report(jr(1, "final", "primario"), jr(8, "map1", "primario"))), stationO, "", f, testKey(t)); err != nil {
		t.Errorf("unknown local job: %v", err)
	}
}

func empty2(t *testing.T, s *Store) {
	t.Helper()
	for _, tb := range []string{"ocorrencias", "ocorrencia_jobs", "amostras", "historico"} {
		if n := count(t, s, tb); n != 0 {
			t.Errorf("%s has %d rows", tb, n)
		}
	}
}

func TestFeedbackBusyJobIs409WithRetryAfter(t *testing.T) { // C-1.4a
	h, s, f := newHandler(t, "")
	j := f.jobs[mkID(3)]
	j.Status = "em_execucao"
	f.jobs[mkID(3)] = j
	w := do(h, "POST", goodBody(nil), nil)
	if w.Code != 409 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("%d retry-after=%q %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
	}
	empty(t, s)
}

// ---- C-1.11a: the same origin ------------------------------------------------------------

func TestSampleOnlyFromJobsOfTheSameOrigin(t *testing.T) { // C-1.11a
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	j := f.jobs[mkID(2)]
	j.StationComputer, j.StationUser = "OUTRO", "bob"
	f.jobs[mkID(2)] = j
	// a part of another origin: a reference, counted, no sample
	r := ingest(t, s, report(jr(2, "map1", "primario"), jr(1, "final", "primario")), "", f)
	if r.Estado != EstadoNova || r.AmostrasCopiadas != 1 || r.JobsOutraOrigem != 1 {
		t.Fatalf("result = %+v", r)
	}
	if got := len(jobRows(t, s, r.OcorrenciaID)); got != 2 {
		t.Errorf("references = %d", got)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM amostras WHERE job_id=?`, mkID(2)).Scan(&n)
	if n != 0 {
		t.Error("a sample of another origin was copied")
	}
	if len(f.contReqs) != 1 || f.contReqs[0] != mkID(1) {
		t.Errorf("content read for %v: a job of another origin must not be read", f.contReqs)
	}
	// the job_do_erro of another origin: sem_pedido
	s2 := openStore(t, Options{})
	f2 := newFake()
	f2.add(1)
	f2.add(2)
	j = f2.jobs[mkID(2)]
	j.StationComputer = "OUTRO"
	f2.jobs[mkID(2)] = j
	r = ingest(t, s2, report(jr(1, "map1", "primario"), jr(2, "final", "primario")), "", f2)
	if r.Estado != EstadoSemPedido || r.JobsOutraOrigem != 1 || r.AmostrasCopiadas != 1 {
		t.Errorf("job_do_erro of another origin: %+v", r)
	}
}

func TestOriginByIPWhenTheJobHasNoStation(t *testing.T) { // C-1.11a
	mk := func(station bool, ip string) *fakeSrc {
		f := newFake()
		id := f.add(1)
		j := f.jobs[id]
		if !station {
			j.StationComputer, j.StationUser = "", ""
		}
		j.ClientIP = ip
		f.jobs[id] = j
		return f
	}
	cases := []struct {
		name string
		f    *fakeSrc
		o    Origin
		want string
	}{
		{"no station, same IP", mk(false, "10.0.0.7"), Origin{ClientIP: "10.0.0.7"}, EstadoNova},
		{"no station, same IP, reporter has a station", mk(false, "10.0.0.7"), stationO, EstadoNova},
		{"no station, other IP", mk(false, "10.0.0.99"), stationO, EstadoSemPedido},
		{"station on the job, reporter without one", mk(true, "10.0.0.7"), Origin{ClientIP: "10.0.0.7"}, EstadoSemPedido},
		{"same machine, other user", mk(true, "10.0.0.7"), Origin{Computer: "PC01", User: "carla", ClientIP: "10.0.0.7"}, EstadoSemPedido},
		{"same user, other machine", mk(true, "10.0.0.7"), Origin{Computer: "PC09", User: "ana", ClientIP: "10.0.0.7"}, EstadoSemPedido},
		{"same machine and user, other IP", mk(true, "10.0.0.7"), Origin{Computer: "PC01", User: "ana", ClientIP: "10.9.9.9"}, EstadoSemPedido},
		{"same IP, other station", mk(true, "10.0.0.7"), Origin{Computer: "PC09", User: "bob", ClientIP: "10.0.0.7"}, EstadoSemPedido},
		{"same station and same IP", mk(true, "10.0.0.7"), Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.7"}, EstadoNova},
		{"same station, no IP on the report", mk(true, "10.0.0.7"), Origin{Computer: "PC01", User: "ana"}, EstadoSemPedido},
	}
	for _, c := range cases {
		s := openStore(t, Options{})
		if got := ingestAs(t, s, report(jr(1, "unico", "primario")), c.o, c.f).Estado; got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestDedupeNeverCrossesOrigins(t *testing.T) { // C-1.11a
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	auto := report(jr(1, "unico", "primario"))
	auto.Origem = "automatico"
	ra := ingestAs(t, s, auto, stationO, f)
	other := Origin{Computer: "PC02", User: "bia", ClientIP: "10.0.0.8"}
	// another origin reports the same job: it gets its own occurrence, never the id or state of A's
	rb := ingestAs(t, s, auto, other, f)
	if rb.OcorrenciaID == ra.OcorrenciaID || rb.Estado == ra.Estado {
		t.Errorf("the other origin saw A's occurrence: %+v vs %+v", rb, ra)
	}
	if count(t, s, "ocorrencias") != 2 {
		t.Errorf("occurrences = %d", count(t, s, "ocorrencias"))
	}
	// the same machine with another user is another origin too
	sameMachine := Origin{Computer: "PC01", User: "carla", ClientIP: "10.0.0.7"}
	rd := ingestAs(t, s, auto, sameMachine, f)
	if rd.OcorrenciaID == ra.OcorrenciaID || rd.Estado == ra.Estado {
		t.Errorf("another user of the same machine saw A's occurrence: %+v", rd)
	}
	// the same station from another IP is another origin (the station alone is declared)
	otherIP := Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.8"}
	re := ingestAs(t, s, auto, otherIP, f)
	if re.OcorrenciaID == ra.OcorrenciaID || re.Estado == ra.Estado {
		t.Errorf("the same station from another IP saw A's occurrence: %+v", re)
	}
	// without a station the origin is the IP
	s2 := openStore(t, Options{})
	f2 := newFake()
	id := f2.add(1)
	j := f2.jobs[id]
	j.StationComputer, j.StationUser = "", ""
	f2.jobs[id] = j
	a2 := ingestAs(t, s2, auto, Origin{ClientIP: "10.0.0.7"}, f2)
	b2 := ingestAs(t, s2, auto, Origin{ClientIP: "10.0.0.99"}, f2)
	if b2.OcorrenciaID == a2.OcorrenciaID || b2.Estado == a2.Estado {
		t.Errorf("another IP saw the occurrence: %+v vs %+v", b2, a2)
	}
	c2 := ingestAs(t, s2, auto, Origin{ClientIP: "10.0.0.7"}, f2)
	if c2.OcorrenciaID != a2.OcorrenciaID {
		t.Errorf("the same IP must get the same occurrence back: %+v", c2)
	}
	// and a human report from another origin does not replace A's candidate
	human := report(jr(1, "unico", "primario"))
	rc := ingestAs(t, s, human, other, f)
	var og string
	s.db.QueryRow(`SELECT origem FROM ocorrencias WHERE id=?`, ra.OcorrenciaID).Scan(&og)
	if rc.OcorrenciaID == ra.OcorrenciaID || og != "automatico" {
		t.Errorf("A's candidate was replaced by another origin: %+v origem=%s", rc, og)
	}
}

// ---- M-1: no reading in vain ----------------------------------------------------------------

func TestJobsWithASampleAreNotReadAgainAndSizeComesFirst(t *testing.T) { // M-1
	s := openStore(t, Options{})
	f := newFake()
	f.add(1)
	f.add(2)
	f.add(3)
	ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "final", "primario")), "", f)
	f.contReqs, f.probes = nil, nil
	ingest(t, s, report(jr(1, "map1", "primario"), jr(3, "final", "primario")), "", f)
	for _, id := range append(append([]string{}, f.contReqs...), f.probes...) {
		if id == mkID(1) {
			t.Error("job 1 already had a sample and was read again")
		}
	}
	// a sample too big for the budget is refused from its size, before its text is loaded
	s2 := openStore(t, Options{})
	f2 := newFake()
	id := f2.add(1)
	c := f2.conts[id]
	c.Response = strings.Repeat("x", 20000)
	f2.conts[id] = c
	s2.opts.MaxBytes = s2.Size() + 1000
	_, err := s2.Ingest(ctx, mustValid(t, report(jr(1, "unico", "primario"))), stationO, "", f2, testKey(t))
	if !errors.Is(err, ErrCapReached) {
		t.Fatalf("err = %v", err)
	}
	if len(f2.contReqs) != 0 {
		t.Errorf("the text was loaded before the budget check: %v", f2.contReqs)
	}
}

// ---- C-6.2, C-6.2a: live data, auto_vacuum -----------------------------------------------------

func TestCapCountsLiveDataAndRecoversAfterDelete(t *testing.T) { // C-6.2, C-6.2a
	s := openStore(t, Options{})
	var av, jsl int64
	s.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&av)
	s.db.QueryRow(`PRAGMA journal_size_limit`).Scan(&jsl)
	if av != 2 || jsl != 64<<20 {
		t.Errorf("auto_vacuum=%d journal_size_limit=%d", av, jsl)
	}
	big := func(f *fakeSrc, n int) {
		id := f.add(n)
		c := f.conts[id]
		c.Response = strings.Repeat("x", 100000)
		f.conts[id] = c
	}
	f := newFake()
	big(f, 1)
	big(f, 2)
	base := s.Size()
	s.opts.MaxBytes = base + 150000
	a := ingest(t, s, report(jr(1, "unico", "primario")), "", f)
	_, err := s.Ingest(ctx, mustValid(t, report(jr(2, "unico", "primario"))), stationO, "", f, testKey(t))
	if !errors.Is(err, ErrCapReached) {
		t.Fatalf("second big report: %v", err)
	}
	if count(t, s, "amostras") != 1 {
		t.Error("the refusal changed the data")
	}
	must(t, s.Delete(ctx, a.OcorrenciaID, admin))
	var fc int64
	s.db.QueryRow(`PRAGMA freelist_count`).Scan(&fc)
	if fc != 0 {
		t.Errorf("freelist_count = %d after the deletion: the space was not returned (incremental_vacuum)", fc)
	}
	if got := s.Size(); got > base+65536 {
		t.Errorf("live size after the deletion = %d, base %d", got, base)
	}
	if _, err := s.Ingest(ctx, mustValid(t, report(jr(2, "unico", "primario"))), stationO, "", f, testKey(t)); err != nil {
		t.Errorf("after deleting, the report must be accepted again: %v", err)
	}
	// Size is the live pages: (page_count - freelist_count) * page_size, no -wal
	var pc, ps int64
	s.db.QueryRow(`PRAGMA page_count`).Scan(&pc)
	s.db.QueryRow(`PRAGMA freelist_count`).Scan(&fc)
	s.db.QueryRow(`PRAGMA page_size`).Scan(&ps)
	if s.Size() != (pc-fc)*ps {
		t.Errorf("Size = %d, want %d", s.Size(), (pc-fc)*ps)
	}
}

func TestFreePagesDoNotCountAgainstTheCeiling(t *testing.T) { // C-6.2: live data only
	s := openStore(t, Options{})
	f := newFake()
	id := f.add(1)
	c := f.conts[id]
	c.Response = strings.Repeat("x", 200000)
	f.conts[id] = c
	ingest(t, s, report(jr(1, "unico", "primario")), "", f)
	// delete without the vacuum: the pages are free, not live
	must(t, s.withTx(ctx, func(tx2 *sql.Tx) error {
		_, err := tx2.ExecContext(ctx, `DELETE FROM amostras`)
		return err
	}))
	var fc int64
	s.db.QueryRow(`PRAGMA freelist_count`).Scan(&fc)
	if fc == 0 {
		t.Fatal("setup: no free pages")
	}
	// room for the 200 KB sample only if the free pages do not count; the live
	// size is computed here from the pragmas, not through Size
	var pc, ps int64
	s.db.QueryRow(`PRAGMA page_count`).Scan(&pc)
	s.db.QueryRow(`PRAGMA freelist_count`).Scan(&fc)
	s.db.QueryRow(`PRAGMA page_size`).Scan(&ps)
	s.opts.MaxBytes = (pc-fc)*ps + 250000
	if _, err := s.Ingest(ctx, mustValid(t, report(jr(1, "unico", "primario"))), stationO, "", f, testKey(t)); err != nil {
		t.Errorf("free pages were counted against the ceiling (or the report failed): %v", err)
	}
}

// ---- M-2: the writers' turn honours the deadline ---------------------------------------------------

func TestWriterTurnHonoursTheDeadline(t *testing.T) { // M-2
	h, s, _ := newHandler(t, "")
	s.wsem <- struct{}{} // another writer holds the turn
	t0 := time.Now()
	cctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	f := newFake()
	f.add(1)
	_, err := s.Ingest(cctx, mustValid(t, report(jr(1, "unico", "primario"))), stationO, "", f, testKey(t))
	if !errors.Is(err, ErrBusy) || time.Since(t0) > 3*time.Second {
		t.Errorf("err = %v after %v", err, time.Since(t0))
	}
	h.Timeout = 60 * time.Millisecond
	w := do(h, "POST", goodBody(nil), nil)
	if w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Errorf("handler: %d retry-after=%q %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
	}
	<-s.wsem
	if w := do(h, "POST", goodBody(nil), nil); w.Code != 200 {
		t.Errorf("after the turn is free: %d %s", w.Code, w.Body.String())
	}
}

// ---- C-1.12c: only nova and confirmada are editable -----------------------------------------------------

func TestEditIsRefusedOutsideNovaAndConfirmada(t *testing.T) { // C-1.12c
	for _, st := range []string{EstadoSemPedido, EstadoDistrib, EstadoFallback, EstadoDescartada, EstadoApagada} {
		s := openStore(t, Options{})
		id := occIn(t, s, st)
		nv := "tentativa de edicao"
		before := count(t, s, "historico")
		for name, e := range map[string]Edit{"resposta": {RespostaCorreta: &nv}, "descricao": {Descricao: &nv}, "correcao": {CorrecaoAplicada: &nv}} {
			if _, err := s.Update(ctx, id, e, admin); !errors.Is(err, ErrTransition) {
				t.Errorf("%s %s: %v", st, name, err)
			}
		}
		if count(t, s, "historico") != before {
			t.Errorf("%s: a refused edit wrote history", st)
		}
		if st != EstadoApagada {
			var d, r, c string
			s.db.QueryRow(`SELECT descricao, resposta_correta, correcao_aplicada FROM ocorrencias WHERE id=?`, id).Scan(&d, &r, &c)
			if d == nv || r == nv || c == nv {
				t.Errorf("%s: a refused edit changed the field", st)
			}
		}
	}
}

// ---- C-4.11b: the quota table --------------------------------------------------------------------------

func TestQuotaTableExistsAndNothingTouchesIt(t *testing.T) { // C-4.11b
	s := openStore(t, Options{})
	rows, err := s.db.Query(`PRAGMA table_info(cota_grupos)`)
	if err != nil {
		t.Fatal(err)
	}
	var cols []string
	for rows.Next() {
		var cid, nn, pk int
		var name, typ string
		var d *string
		rows.Scan(&cid, &name, &typ, &nn, &d, &pk)
		cols = append(cols, name)
	}
	rows.Close()
	if strings.Join(cols, ",") != "grupo_id,categoria,particao,atribuida_em" {
		t.Fatalf("columns = %v", cols)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.db.Exec(`INSERT INTO cota_grupos(grupo_id,categoria,particao,atribuida_em) VALUES(?,?,?,?)`, fmt.Sprintf("g%d", i), "omitiu", "teste", 100+i); err != nil {
			t.Fatal(err)
		}
	}
	before := dumpTable(t, s, "cota_grupos")
	// unconfirm, discard and delete occurrences that sit in a test group
	a, b, c := occIn(t, s, EstadoConfirmada), occIn(t, s, EstadoConfirmada), occIn(t, s, EstadoConfirmada)
	for _, id := range []string{a, b, c} {
		s.db.Exec(`UPDATE ocorrencias SET particao='teste', grupo_id='g1' WHERE id=?`, id)
	}
	nv := "mudou"
	if _, err := s.Update(ctx, a, Edit{RespostaCorreta: &nv}, admin); err != nil {
		t.Fatal(err)
	}
	must(t, s.Discard(ctx, b, admin))
	must(t, s.Delete(ctx, c, admin))
	if after := dumpTable(t, s, "cota_grupos"); after != before {
		t.Errorf("cota_grupos changed:\n%s\n--\n%s", before, after)
	}
	// the minimal row of an erased occurrence still holds only id, estado and dates
	m := rowCols(t, s, c)
	if m["grupo_id"] != nil && m["grupo_id"] != "" || m["particao"] != "" {
		t.Errorf("the erased row kept group data: %v", m)
	}
}

// C-1.11a (1.1.3): in a list, a part of the same station but another IP, or of the
// same IP but another station, is a reference only; the part that matches on both is copied.
func TestPartsAreCopiedOnlyWhenStationAndIPBothMatch(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	for i := 1; i <= 4; i++ {
		f.add(i)
	}
	set := func(n int, mut func(*Job)) {
		j := f.jobs[mkID(n)]
		mut(&j)
		f.jobs[mkID(n)] = j
	}
	set(1, func(j *Job) { j.ClientIP = "10.0.0.99" })                    // same station, other IP
	set(2, func(j *Job) { j.StationComputer, j.StationUser = "X", "y" }) // same IP, other station
	r := ingest(t, s, report(jr(1, "map1", "primario"), jr(2, "map2", "primario"), jr(3, "map3", "primario"), jr(4, "final", "primario")), "", f)
	if r.Estado != EstadoNova || r.AmostrasCopiadas != 2 || r.JobsOutraOrigem != 2 {
		t.Fatalf("result = %+v", r)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM amostras WHERE job_id IN (?,?)`, mkID(1), mkID(2)).Scan(&n)
	if n != 0 || len(jobRows(t, s, r.OcorrenciaID)) != 4 {
		t.Errorf("samples of the other origins: %d", n)
	}
}

// The IP that counts is the one the gateway verifies: X-Real-Ip is believed only
// from a loopback peer (the local nginx), so a client cannot claim another IP.
func TestRealIPIsTrustedOnlyFromLoopback(t *testing.T) {
	h, s, _ := newHandler(t, "")
	via := func(remote string) Result {
		req := httptestRequest(goodBody(nil), remote)
		w := httptestRecord(h, req)
		var res Result
		json.Unmarshal(w.Body.Bytes(), &res)
		return res
	}
	if r := via("127.0.0.1:5555"); r.Estado != EstadoNova { // the nginx says 10.0.0.7, the job's IP
		t.Errorf("through the loopback nginx: %+v", r)
	}
	if r := via("10.9.9.9:5555"); r.Estado != EstadoSemPedido || r.JobsOutraOrigem == 0 { // the same header from a stranger is ignored
		t.Errorf("X-Real-Ip from a non-loopback peer was believed: %+v", r)
	}
	_ = s
}

// ---- C-6.2 (1.1.3): the ceiling is measured in the writer's turn -------------------------------

func TestConcurrentReportsNearTheCeilingDoNotOvershoot(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	const n = 8
	for i := 1; i <= n; i++ {
		id := f.add(i)
		c := f.conts[id]
		c.Response = strings.Repeat("x", 100000)
		f.conts[id] = c
	}
	s.opts.MaxBytes = s.Size() + 150000 // room for one 100 KB sample, not for two
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i-1] = s.Ingest(ctx, mustValid(t, report(jr(i, "unico", "primario"))), stationO, "", f, testKey(t))
		}(i)
	}
	close(start)
	wg.Wait()
	ok, capped := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, ErrCapReached):
			capped++
		default:
			t.Errorf("unexpected error: %v", e)
		}
	}
	if ok != 1 || capped != n-1 {
		t.Errorf("%d accepted and %d refused with 507; want 1 and %d", ok, capped, n-1)
	}
	if got := count(t, s, "amostras"); got != 1 {
		t.Errorf("amostras = %d", got)
	}
}
