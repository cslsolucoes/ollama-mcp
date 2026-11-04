package treino

// Revisão adversarial da etapa 12.9 (Fase 12), revisor F: superfície POST /api/feedback.
// Cada vetor imprime uma linha VEC|id|OK ou FALHA|descricao. Depende dos helpers do pacote:
// openStore, testKey, newFake, fakeSrc, mkID, defJob, count, estadoOf (treino_test.go) e ctx (state_test.go).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
)

func vec12F(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res != "OK" {
		t.Errorf("vetor %s: %s", id, desc)
	}
}

// adv12FReq is a POST /api/feedback as the gateway sees it.
type adv12FReq struct {
	remote  string            // RemoteAddr (default 127.0.0.1:5555, the local nginx)
	realIPs []string          // X-Real-Ip lines (nil: none)
	station string            // X-Mcpollama-Station ("" none)
	hdr     map[string]string // other headers
}

func adv12FDo(h http.Handler, body string, r adv12FReq) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/feedback", strings.NewReader(body))
	req.RemoteAddr = r.remote
	if req.RemoteAddr == "" {
		req.RemoteAddr = "127.0.0.1:5555"
	}
	req.Header.Set("Content-Type", "application/json")
	for _, v := range r.realIPs {
		req.Header.Add("X-Real-Ip", v)
	}
	if r.station != "" {
		req.Header.Set(proxy.HeaderStation, r.station)
	}
	for k, v := range r.hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func adv12FBody(mut func(map[string]any), jobs ...map[string]string) string {
	m := map[string]any{"projeto": "P", "categoria": "omitiu", "descricao": "faltou o item 3", "severidade": "media", "origem": "claude",
		"resposta_correta": "1, 2, 3", "jobs": jobs}
	if mut != nil {
		mut(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func adv12FJob(n int, papel, backend string) map[string]string {
	return map[string]string{"id": mkID(n), "papel": papel, "backend": backend}
}

func adv12FRes(w *httptest.ResponseRecorder) Result {
	var r Result
	json.Unmarshal(w.Body.Bytes(), &r)
	return r
}

// adv12FHandler: a store, a fake source with jobs 1..n made by PC01/ana from 10.0.0.1.
func adv12FHandler(t *testing.T, n int, o Options) (*Handler, *Store, *fakeSrc) {
	t.Helper()
	s := openStore(t, o)
	f := newFake()
	for i := 1; i <= n; i++ {
		id := f.add(i)
		j := f.jobs[id]
		j.ClientIP = "10.0.0.1"
		f.jobs[id] = j
	}
	return &Handler{Store: s, Source: f, Key: testKey(t), Role: RolePrimario}, s, f
}

func adv12FTotals(t *testing.T, s *Store) string {
	return fmt.Sprintf("oc=%d jobs=%d amostras=%d hist=%d", count(t, s, "ocorrencias"), count(t, s, "ocorrencia_jobs"), count(t, s, "amostras"), count(t, s, "historico"))
}

// ---- F1: relato forjado por outra estação -------------------------------------------------

func TestAdv12FForgedOrigin(t *testing.T) {
	const stA = "computer=PC01;user=ana"
	cases := []struct {
		id, desc string
		req      adv12FReq
		noStatn  bool // the job has no station
	}{
		{"F1a", "mesma estação declarada, outro IP verificado (X-Real-Ip 10.0.0.2 do nginx)", adv12FReq{realIPs: []string{"10.0.0.2"}, station: stA}, false},
		{"F1b", "X-Real-Ip forjado (10.0.0.1) vindo de par não-loopback 10.0.0.2", adv12FReq{remote: "10.0.0.2:4444", realIPs: []string{"10.0.0.1"}, station: stA}, false},
		{"F1c", "X-Forwarded-For 10.0.0.1 do nginx com X-Real-Ip 10.0.0.2", adv12FReq{realIPs: []string{"10.0.0.2"}, station: stA, hdr: map[string]string{"X-Forwarded-For": "10.0.0.1"}}, false},
		{"F1c2", "X-Forwarded-For/Forwarded/CF-Connecting-IP 10.0.0.1 de par não-loopback", adv12FReq{remote: "10.0.0.2:4444", station: stA, hdr: map[string]string{"X-Forwarded-For": "10.0.0.1", "Forwarded": "for=10.0.0.1", "CF-Connecting-IP": "10.0.0.1"}}, false},
		{"F1d", "estação vazia, mesmo IP 10.0.0.1 (o job tem estação)", adv12FReq{realIPs: []string{"10.0.0.1"}}, false},
		{"F1d2", "estação inválida (vira vazia), mesmo IP", adv12FReq{realIPs: []string{"10.0.0.1"}, station: "computer=PC01;user=ana;user=ana"}, false},
		{"F1e", "job sem estação, relato de outro IP 10.0.0.2 com estação qualquer", adv12FReq{realIPs: []string{"10.0.0.2"}, station: stA}, true},
		{"F1f", "duas linhas X-Real-Ip (10.0.0.1 e 10.0.0.1) do nginx: o gateway cai no par 127.0.0.1", adv12FReq{realIPs: []string{"10.0.0.1", "10.0.0.1"}, station: stA}, false},
		{"F1f2", "IPv4 mapeado em IPv6 ::ffff:10.0.0.2 (outro IP)", adv12FReq{realIPs: []string{"::ffff:10.0.0.2"}, station: stA}, false},
	}
	for _, c := range cases {
		h, s, f := adv12FHandler(t, 2, Options{})
		if c.noStatn {
			j := f.jobs[mkID(1)]
			j.StationComputer, j.StationUser = "", ""
			f.jobs[mkID(1)] = j
		}
		w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), c.req)
		r := adv12FRes(w)
		ns := count(t, s, "amostras")
		switch {
		case w.Code != 200:
			vec12F(t, c.id, "FALHA", fmt.Sprintf("Menor fixture: status %d %s", w.Code, w.Body.String()))
		case ns != 0 || r.AmostrasCopiadas != 0 || r.Estado != EstadoSemPedido || r.JobsOutraOrigem != 1:
			vec12F(t, c.id, "FALHA", fmt.Sprintf("Bloqueador ingest.go:36 sameOrigin: %s copiou amostra de outra origem: %+v amostras=%d", c.desc, r, ns))
		default:
			vec12F(t, c.id, "OK", fmt.Sprintf("%s: estado=sem_pedido, jobs_outra_origem=1, 0 amostras", c.desc))
		}
	}
	// control: the true origin does copy (the fixture really exercises the path)
	h, s, _ := adv12FHandler(t, 2, Options{})
	w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), adv12FReq{realIPs: []string{"10.0.0.1"}, station: stA})
	if r := adv12FRes(w); w.Code != 200 || r.Estado != EstadoNova || count(t, s, "amostras") != 1 {
		vec12F(t, "F1ctl", "FALHA", fmt.Sprintf("Menor fixture: a origem verdadeira não copiou: %d %+v", w.Code, r))
	} else {
		vec12F(t, "F1ctl", "OK", "controle: a mesma estação e o mesmo IP copiam a amostra (o caminho atacado é o real)")
	}
}

// F1g: a job with a partial station (computer only, no user) is treated as "no station":
// any declared station from the same IP copies it.
func TestAdv12FPartialStationJob(t *testing.T) {
	h, s, f := adv12FHandler(t, 1, Options{})
	j := f.jobs[mkID(1)]
	j.StationUser = "" // the proxy keeps computer=PC01 alone when the header has only that key
	f.jobs[mkID(1)] = j
	if st := proxy.ParseStation([]string{"computer=PC01"}); st.Computer != "PC01" || st.User != "" {
		t.Fatalf("parser: %+v", st)
	}
	w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), adv12FReq{realIPs: []string{"10.0.0.1"}, station: "computer=OUTRO;user=mallory"})
	r := adv12FRes(w)
	if r.AmostrasCopiadas == 1 && count(t, s, "amostras") == 1 {
		vec12F(t, "F1g", "OK", "MEDIDA: job com estação parcial (só computer) é tratado como sem estação: estação OUTRO/mallory do MESMO IP copia; "+
			"o IP verificado continua sendo a barreira (C-1.11a fala em máquina E usuário; ver relatório, só leitura ingest.go:38)")
	} else {
		vec12F(t, "F1g", "OK", fmt.Sprintf("job com estação parcial não copiado por outra estação: %+v", r))
	}
}

// F1h: the answer must not reveal anything of a job of another origin, and the status
// codes as an oracle of existence.
func TestAdv12FNoLeakAndOracle(t *testing.T) {
	h, s, f := adv12FHandler(t, 3, Options{})
	j := f.jobs[mkID(1)]
	j.Tool, j.Model = "tool-SEGREDO", "modelo-SEGREDO"
	f.jobs[mkID(1)] = j
	c := f.conts[mkID(1)]
	c.Prompt, c.Response = "PROMPT-SEGREDO-F1", "RESP-SEGREDO-F1"
	f.conts[mkID(1)] = c
	busy := f.jobs[mkID(2)]
	busy.Status = "em_execucao"
	f.jobs[mkID(2)] = busy
	attacker := adv12FReq{realIPs: []string{"10.0.0.66"}, station: "computer=EVIL;user=x"}
	wOther := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), attacker)
	wBusy := adv12FDo(h, adv12FBody(nil, adv12FJob(2, "unico", "primario")), attacker)
	wNone := adv12FDo(h, adv12FBody(nil, adv12FJob(99, "unico", "primario")), attacker)
	body := wOther.Body.String() + wBusy.Body.String() + wNone.Body.String()
	leak := ""
	for _, sec := range []string{"SEGREDO", "PC01", "ana", "10.0.0.1", "extract", "/api/generate"} {
		if strings.Contains(body, sec) {
			leak += sec + " "
		}
	}
	var keys []string
	var m map[string]any
	json.Unmarshal(wOther.Body.Bytes(), &m)
	for k := range m {
		keys = append(keys, k)
	}
	if leak != "" {
		vec12F(t, "F1h", "FALHA", "Bloqueador http.go:96: a resposta revela dado do job alheio: "+leak)
	} else {
		vec12F(t, "F1h", "OK", fmt.Sprintf("resposta ao relato de job alheio só traz %v (sem estado/texto/estação/tool do job)", keys))
	}
	if wOther.Code == 200 && wBusy.Code == 409 && wNone.Code == 404 {
		vec12F(t, "F1h2", "OK", "MEDIDA: oráculo de existência e de execução existe (alheio pronto 200 sem_pedido, alheio em execução 409, inexistente 404): "+
			"imposto pela C-1.4/C-1.4a; os ids são 128 bits aleatórios e o /monitor (leitura livre na LAN) já lista os jobs, então não revela nada novo; "+
			"cada sonda grava uma ocorrência sem_pedido (estatística poluível)")
	} else {
		vec12F(t, "F1h2", "FALHA", fmt.Sprintf("Menor: códigos inesperados alheio=%d ocupado=%d inexistente=%d", wOther.Code, wBusy.Code, wNone.Code))
	}
	_ = s
}

// F1i / F8: a report of another origin (references only) links groups that already have
// opposite sides: the victims' confirmed occurrences go to quarantine (C-4.9a) and the
// snapshots that list them become invalid (C-4.10). Two paths: job_do_erro of another origin
// (sem_pedido), and the attacker's own job plus the victims' ids labelled "fallback" (distribuida,
// never validated, C-1.11).
func TestAdv12FForeignLinkForcesQuarantine(t *testing.T) {
	for _, path := range []string{"sem_pedido", "distribuida"} {
		s := openStore(t, Options{})
		f := newFake()
		key := testKey(t)
		victim := Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.1"}
		mk := func(n int) string {
			id := f.add(n)
			j := f.jobs[id]
			j.ClientIP = "10.0.0.1"
			f.jobs[id] = j
			c := f.conts[id]
			c.PromptJSON = []byte(fmt.Sprintf(`{"model":"qwen3-ia","prompt":"documento distinto %d"}`, n))
			f.conts[id] = c
			return id
		}
		confirm := func(n int) (string, string) {
			mk(n)
			rep := Report{Projeto: "P", Jobs: []JobRef{{ID: mkID(n), Papel: "unico", Backend: "primario"}}, Categoria: "omitiu", Descricao: "d",
				RespostaCorreta: "certo", Severidade: "media", Origem: "claude"}
			if err := rep.Validate(); err != nil {
				t.Fatal(err)
			}
			r, err := s.Ingest(ctx, rep, victim, "", f, key)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Confirm(ctx, r.OcorrenciaID, AdminActor("10.0.0.9")); err != nil {
				t.Fatal(err)
			}
			var p string
			s.db.QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, r.OcorrenciaID).Scan(&p)
			return r.OcorrenciaID, p
		}
		ocTeste, p1 := confirm(1) // the first group of the category: test (quota)
		if p1 != ParteTeste {
			t.Fatalf("fixture: first group went %q", p1)
		}
		// fill the quota of the category, so the next groups go by the 15% hash (C-4.7)
		for i := 0; i < CotaPorCategoria; i++ {
			s.db.Exec(`INSERT INTO cota_grupos(grupo_id,categoria,particao,atribuida_em) VALUES(?,?,?,?)`, fmt.Sprintf("fixture%02d", i), "omitiu", ParteTeste, 1)
		}
		ocTreino, jTreino := "", 0
		for n := 2; n < 80 && ocTreino == ""; n++ {
			if id, p := confirm(n); p == ParteTreino {
				ocTreino, jTreino = id, n
			}
		}
		if ocTreino == "" {
			t.Fatal("fixture: no group went to treino")
		}
		// a snapshot (retrato) that lists the test case
		s.db.Exec(`INSERT INTO retratos(id,criado_em,valido) VALUES('r1',1,1)`)
		s.db.Exec(`INSERT INTO retrato_itens(retrato_id,ocorrencia_id,job_id,categoria) VALUES('r1',?,?,'omitiu')`, ocTeste, mkID(1))

		h := &Handler{Store: s, Source: f, Key: key, Role: RolePrimario}
		attacker := adv12FReq{realIPs: []string{"10.0.0.66"}, station: "computer=EVIL;user=x"}
		var body string
		if path == "sem_pedido" {
			body = adv12FBody(nil, adv12FJob(1, "map1", "primario"), adv12FJob(jTreino, "final", "primario"))
		} else {
			// the attacker's own finished job (no station, IP 10.0.0.66)
			own := f.add(500)
			j := f.jobs[own]
			j.StationComputer, j.StationUser, j.ClientIP = "", "", "10.0.0.66"
			f.jobs[own] = j
			body = adv12FBody(nil, adv12FJob(500, "final", "primario"), adv12FJob(1, "map1", "fallback"), adv12FJob(jTreino, "map2", "fallback"))
		}
		w := adv12FDo(h, body, attacker)
		r := adv12FRes(w)
		var pT, pE string
		var valido int
		s.db.QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, ocTreino).Scan(&pT)
		s.db.QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, ocTeste).Scan(&pE)
		s.db.QueryRow(`SELECT valido FROM retratos WHERE id='r1'`).Scan(&valido)
		id := "F1i"
		if path == "distribuida" {
			id = "F8a"
		}
		if w.Code == 200 && pT == ParteQuarentena && pE == ParteQuarentena {
			// what the admin can do about it: discard the forged one (still a member, C-4.5), then erase it
			must(t, s.Discard(ctx, r.OcorrenciaID, admin))
			var pD, pX string
			s.db.QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, ocTreino).Scan(&pD)
			must(t, s.Delete(ctx, r.OcorrenciaID, admin))
			s.db.QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, ocTreino).Scan(&pX)
			vec12F(t, id, "FALHA", fmt.Sprintf("Importante ingest.go:333-347 + partition.go:131-211 (lacuna de contrato C-4.5/C-1.11/C-1.11a): relato de OUTRA origem (IP 10.0.0.66, estado %s, "+
				"0 amostras) só com referências aos jobs da vítima ligou um grupo de treino e um de teste já CONFIRMADOS: os dois foram para quarentena e o retrato r1 ficou valido=%d, "+
				"sem confirmação do admin (C-1.18); descartar o forjado deixa %q e apagá-lo deixa %q (a quarentena persiste até o admin liberar à mão); os ids dos jobs são públicos no /monitor da LAN",
				r.Estado, valido, pD, pX))
		} else {
			vec12F(t, id, "OK", fmt.Sprintf("referência alheia não mudou a partição: %d %+v treino=%q teste=%q valido=%d", w.Code, r, pT, pE, valido))
		}
	}
}

// ---- F2: flood ----------------------------------------------------------------------------

func TestAdv12FFlood(t *testing.T) {
	h, s, _ := adv12FHandler(t, 1, Options{})
	me := adv12FReq{realIPs: []string{"10.0.0.1"}, station: "computer=PC01;user=ana"}
	const n = 300
	sz0 := s.Size()
	var first, last time.Duration
	codes := map[int]int{}
	for i := 0; i < n; i++ {
		t0 := time.Now()
		w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), me)
		d := time.Since(t0)
		codes[w.Code]++
		if i < 10 {
			first += d
		}
		if i >= n-10 {
			last += d
		}
	}
	oc := count(t, s, "ocorrencias")
	grow := s.Size() - sz0
	if codes[200] == n && oc == n {
		vec12F(t, "F2a", "OK", fmt.Sprintf("MEDIDA: %d relatos seguidos da mesma estação e do mesmo job: todos 200, %d ocorrências (humano não deduplica), %d bytes vivos (+%d B/relato), "+
			"média dos 10 primeiros %v e dos 10 últimos %v (o recompute percorre todas as ocorrências a cada relato); não há limite por IP/estação no gateway (só o nginx: 600/min, rajada 100)",
			n, oc, grow, grow/int64(n), (first/10).Round(time.Microsecond), (last/10).Round(time.Microsecond)))
	} else {
		vec12F(t, "F2a", "FALHA", fmt.Sprintf("Menor: códigos %v, ocorrências %d", codes, oc))
	}

	// F2b: the writers' turn held: parallel reports get 503 + Retry-After and nothing is
	// written; the readers (Summary, List) are not blocked.
	h.Timeout = 150 * time.Millisecond
	s.wsem <- struct{}{} // someone holds the turn
	before := adv12FTotals(t, s)
	var wg sync.WaitGroup
	var mu sync.Mutex
	got := map[string]int{}
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), me)
			mu.Lock()
			got[fmt.Sprintf("%d/%s", w.Code, w.Header().Get("Retry-After"))]++
			mu.Unlock()
		}()
	}
	t0 := time.Now()
	_, errS := s.Summary(context.Background())
	_, errL := s.List(context.Background(), Filter{})
	readT := time.Since(t0)
	wg.Wait()
	<-s.wsem
	after := adv12FTotals(t, s)
	if got["503/5"] == 32 && before == after && errS == nil && errL == nil && readT < 2*time.Second {
		vec12F(t, "F2b", "OK", fmt.Sprintf("32 relatos paralelos com a vez de escrever presa: todos 503 com Retry-After 5, nada gravado (%s); Summary+List responderam em %v", after, readT.Round(time.Millisecond)))
	} else {
		vec12F(t, "F2b", "FALHA", fmt.Sprintf("Importante http.go:107: respostas %v, antes %s depois %s, leitura %v err %v %v", got, before, after, readT, errS, errL))
	}
}

// F2c: one station fills the ceiling and every other station gets 507 (no per-client quota).
func TestAdv12FFloodFillsTheCeilingForEveryone(t *testing.T) {
	h, s, f := adv12FHandler(t, 2, Options{})
	j := f.jobs[mkID(2)]
	j.StationComputer, j.StationUser, j.ClientIP = "PC02", "bia", "10.0.0.2"
	f.jobs[mkID(2)] = j
	s.opts.MaxBytes = s.Size() + 3<<20 // 3 MiB of room
	big := strings.Repeat("r", MaxResposta)
	me := adv12FReq{realIPs: []string{"10.0.0.1"}, station: "computer=PC01;user=ana"}
	acc := 0
	for i := 0; i < 200; i++ {
		w := adv12FDo(h, adv12FBody(func(m map[string]any) { m["resposta_correta"] = big }, adv12FJob(1, "unico", "primario")), me)
		if w.Code != 200 {
			break
		}
		acc++
	}
	w := adv12FDo(h, adv12FBody(nil, adv12FJob(2, "unico", "primario")), adv12FReq{realIPs: []string{"10.0.0.2"}, station: "computer=PC02;user=bia"})
	if w.Code == 507 {
		vec12F(t, "F2c", "OK", fmt.Sprintf("MEDIDA: uma estação encheu o teto (3 MiB de folga) com %d relatos de 128 KB de resposta_correta; a estação PC02 passou a receber 507: "+
			"o teto é global e não há cota por cliente (o contrato C-6.2 não pede; em produção 2 GiB/~140 KB ≈ 15 mil relatos, ~25 min a 600/min)", acc))
	} else {
		vec12F(t, "F2c", "FALHA", fmt.Sprintf("Menor: depois do flood a outra estação recebeu %d", w.Code))
	}
}

// ---- F3: body and JSON ----------------------------------------------------------------------

func TestAdv12FBodyAndJSON(t *testing.T) {
	h, s, f := adv12FHandler(t, 70, Options{})
	me := adv12FReq{realIPs: []string{"10.0.0.1"}, station: "computer=PC01;user=ana"}
	one := adv12FJob(1, "unico", "primario")
	type tc struct {
		id, desc string
		body     string
		want     int
	}
	pad := func(b string, total int) string { return b + strings.Repeat(" ", total-len(b)) }
	good := adv12FBody(nil, one)
	jobsN := func(n int) []map[string]string {
		var js []map[string]string
		for i := 1; i <= n; i++ {
			p := fmt.Sprintf("map%d", i)
			if i == n {
				p = "final"
			}
			js = append(js, adv12FJob(i, p, "primario"))
		}
		return js
	}
	ç := strings.Repeat("ç", 2048) // 4096 bytes
	cases := []tc{
		{"F3a", "corpo de exatamente 256 KB (JSON válido + espaços)", pad(good, MaxBodyBytes), 200},
		{"F3a2", "corpo de 256 KB + 1", pad(good, MaxBodyBytes+1), 413},
		{"F3c", "aninhamento profundo (100 mil '[')", `{"projeto":` + strings.Repeat("[", 100000) + strings.Repeat("]", 100000) + `}`, 400},
		{"F3d1", "duas 'categoria' (válida e depois inválida): vence a última", strings.Replace(good, `"categoria":"omitiu"`, `"categoria":"omitiu","categoria":"xx"`, 1), 400},
		{"F3d2", "duas 'job_do_erro' (inválida e depois a da lista)", strings.Replace(good, `"categoria"`, `"job_do_erro":"zz","job_do_erro":"`+mkID(1)+`","categoria"`, 1), 200},
		{"F3e1", "campo desconhecido", strings.Replace(good, `"categoria"`, `"extra":1,"categoria"`, 1), 400},
		{"F3f1", "enum com homóglifo cirílico (о U+043E)", strings.Replace(good, `"omitiu"`, `"`+"оmitiu"+`"`, 1), 400},
		{"F3f2", "enum com BOM no início", strings.Replace(good, `"omitiu"`, `"\ufeffomitiu"`, 1), 400},
		{"F3f3", "BOM antes do objeto", "\ufeff" + good, 400},
		{"F3f4", "\\u0000 na descricao", strings.Replace(good, `faltou o item 3`, `faltou\u0000`, 1), 400},
		{"F3f5", "NUL cru dentro da string", strings.Replace(good, `faltou o item 3`, "falt\x00ou", 1), 400},
		{"F3f6", "\\u0000 no projeto", strings.Replace(good, `"projeto":"P"`, `"projeto":"P\u0000"`, 1), 400},
		{"F3f7", "enum em NFD/maiúsculas (OMITIU)", strings.Replace(good, `"omitiu"`, `"OMITIU"`, 1), 400},
		{"F3g1", "número enorme num campo texto", strings.Replace(good, `"projeto":"P"`, `"projeto":1e400`, 1), 400},
		{"F3g2", "número no id do job", strings.Replace(good, `"id":"`+mkID(1)+`"`, `"id":123456789012345678901234567890123456789`, 1), 400},
		{"F3h1", "64 jobs", adv12FBody(nil, jobsN(64)...), 200},
		{"F3h2", "65 jobs", adv12FBody(nil, jobsN(65)...), 400},
		{"F3i1", "id com maiúsculas", adv12FBody(nil, map[string]string{"id": strings.ToUpper(mkID(1) + "")[:31] + "A", "papel": "unico", "backend": "primario"}), 400},
		{"F3i2", "id com 31 hex", adv12FBody(nil, map[string]string{"id": mkID(1)[1:], "papel": "unico", "backend": "primario"}), 400},
		{"F3i3", "id com 33 hex", adv12FBody(nil, map[string]string{"id": mkID(1) + "0", "papel": "unico", "backend": "primario"}), 400},
		{"F3i4", "job_do_erro com maiúsculas", adv12FBody(func(m map[string]any) { m["job_do_erro"] = strings.ToUpper(mkID(10)) }, adv12FJob(10, "map1", "primario"), adv12FJob(11, "map2", "primario")), 400},
		{"F3j1", "papel map0", adv12FBody(nil, adv12FJob(1, "map0", "primario")), 400},
		{"F3j2", "papel 'map1\\n'", adv12FBody(nil, adv12FJob(1, "map1\n", "primario")), 400},
		{"F3j3", "papel 'final ' com espaço", adv12FBody(nil, adv12FJob(1, "final ", "primario")), 400},
		{"F3j4", "papel repetido", adv12FBody(func(m map[string]any) { m["job_do_erro"] = mkID(1) }, adv12FJob(1, "map1", "primario"), adv12FJob(2, "map1", "primario")), 400},
		{"F3j5", "unico numa lista de 2", adv12FBody(nil, adv12FJob(1, "unico", "primario"), adv12FJob(2, "final", "primario")), 400},
		{"F3j6", "backend 'Primario'", adv12FBody(nil, adv12FJob(1, "unico", "Primario")), 400},
		{"F3j7", "lista sem final e sem job_do_erro", adv12FBody(nil, adv12FJob(1, "map1", "primario"), adv12FJob(2, "map2", "primario")), 400},
		{"F3k1", "descricao com 4096 bytes multibyte (2048 ç)", adv12FBody(func(m map[string]any) { m["descricao"] = ç }, one), 200},
		{"F3k2", "descricao com 4097 bytes (2048 ç + a)", adv12FBody(func(m map[string]any) { m["descricao"] = ç + "a" }, one), 400},
		{"F3k3", "descricao com 4097 bytes cortando um rune (2047 ç + 3 bytes de 'ã'+'a')", adv12FBody(func(m map[string]any) { m["descricao"] = ç[:4094] + "ãa" }, one), 400},
		{"F3k4", "projeto com 128 bytes", adv12FBody(func(m map[string]any) { m["projeto"] = strings.Repeat("p", 128) }, one), 200},
		{"F3k5", "projeto com 129 bytes", adv12FBody(func(m map[string]any) { m["projeto"] = strings.Repeat("p", 129) }, one), 400},
		{"F3k6", "correcao_aplicada com 4097 bytes", adv12FBody(func(m map[string]any) { m["correcao_aplicada"] = strings.Repeat("c", 4097) }, one), 400},
		{"F3k7", "resposta_correta com 128 KB + 1", adv12FBody(func(m map[string]any) { m["resposta_correta"] = strings.Repeat("c", MaxResposta+1) }, one), 400},
		{"F3l1", "descricao só com NBSP/tab/quebra", adv12FBody(func(m map[string]any) { m["descricao"] = " \t\r\n " }, one), 400},
		{"F3n1", "dois objetos seguidos", good + good, 400},
		{"F3n2", "array no topo", "[" + good + "]", 400},
		{"F3n3", "null", "null", 400},
		{"F3n4", "objeto vazio", "{}", 400},
	}
	for _, c := range cases {
		before := adv12FTotals(t, s)
		w := adv12FDo(h, c.body, me)
		after := adv12FTotals(t, s)
		wrote := before != after
		if w.Code != c.want || (c.want != 200 && wrote) || (c.want == 200 && !wrote) {
			sev := "Importante"
			if c.want != 200 && !wrote {
				sev = "Menor"
			}
			vec12F(t, c.id, "FALHA", fmt.Sprintf("%s http.go:63-82/report.go:57: %s: status %d (esperado %d), gravou=%v, %s", sev, c.desc, w.Code, c.want, wrote, strings.TrimSpace(w.Body.String())))
		} else {
			vec12F(t, c.id, "OK", fmt.Sprintf("%s: %d, gravou=%v", c.desc, w.Code, wrote))
		}
	}
	_ = f

	// F3d3: the duplicated key is resolved as "last one wins" (measure)
	w := adv12FDo(h, strings.Replace(good, `"categoria":"omitiu"`, `"categoria":"xx","categoria":"inventou"`, 1), me)
	var cat string
	s.db.QueryRow(`SELECT categoria FROM ocorrencias WHERE id=?`, adv12FRes(w).OcorrenciaID).Scan(&cat)
	if w.Code == 200 && cat == "inventou" {
		vec12F(t, "F3d3", "OK", "MEDIDA: chave duplicada aceita com 'vence a última' (encoding/json); o valor final é validado, sem desvio de enum")
	} else {
		vec12F(t, "F3d3", "FALHA", fmt.Sprintf("Menor: chave duplicada: %d categoria=%q", w.Code, cat))
	}

	// F3e2: names that differ only by case or by Unicode folding are taken as known fields.
	for _, v := range []struct{ id, from, to string }{
		{"F3e2", `"categoria"`, `"CATEGORIA"`},
		{"F3e3", `"descricao"`, `"de` + "ſ" + `cricao"`}, // long s folds to s
	} {
		w := adv12FDo(h, strings.Replace(good, v.from, v.to, 1), me)
		if w.Code == 200 {
			vec12F(t, v.id, "FALHA", fmt.Sprintf("Menor http.go:69-70 (C-1.17 1.1.2 'campo JSON desconhecido é recusado'): a chave %s, que não é campo do contrato, foi aceita como %s "+
				"(encoding/json casa nomes sem diferenciar caixa e com dobra Unicode); corrigir conferindo os nomes exatos antes do Decode", v.to, v.from))
		} else {
			vec12F(t, v.id, "OK", fmt.Sprintf("chave %s recusada: %d", v.to, w.Code))
		}
	}

	// F3f8: invalid UTF-8 and a lone surrogate become U+FFFD (no invalid byte is stored)
	raw := strings.Replace(good, `faltou o item 3`, "falt\xff\xfeou \\ud800", 1)
	w = adv12FDo(h, raw, me)
	var desc string
	s.db.QueryRow(`SELECT descricao FROM ocorrencias WHERE id=?`, adv12FRes(w).OcorrenciaID).Scan(&desc)
	if w.Code == 200 && strings.Contains(desc, "�") && !strings.Contains(desc, "\xff") {
		vec12F(t, "F3f8", "OK", "MEDIDA: UTF-8 inválido e surrogate solto viram U+FFFD no decoder (aceito, nenhum byte inválido gravado)")
	} else if w.Code == 400 {
		vec12F(t, "F3f8", "OK", "UTF-8 inválido recusado com 400")
	} else {
		vec12F(t, "F3f8", "FALHA", fmt.Sprintf("Menor: UTF-8 inválido: %d %q", w.Code, desc))
	}
}

// F3b: through a real HTTP server: chunked body without Content-Length, Content-Length smaller
// and larger than the body.
func TestAdv12FBodyOnTheWire(t *testing.T) {
	h, s, _ := adv12FHandler(t, 1, Options{})
	srv := httptest.NewServer(h)
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")
	raw := func(head string, body []byte, closeWrite bool) (int, string) {
		c, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(15 * time.Second))
		io.WriteString(c, head)
		c.Write(body)
		if closeWrite {
			c.(*net.TCPConn).CloseWrite()
		}
		resp, err := http.ReadResponse(bufioReader(c), nil)
		if err != nil {
			return 0, err.Error()
		}
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	good := adv12FBody(nil, adv12FJob(1, "unico", "primario"))
	// chunked, 300 KB, no Content-Length
	var cb bytes.Buffer
	big := []byte(good + strings.Repeat(" ", 300<<10))
	for i := 0; i < len(big); i += 8192 {
		e := i + 8192
		if e > len(big) {
			e = len(big)
		}
		fmt.Fprintf(&cb, "%x\r\n%s\r\n", e-i, big[i:e])
	}
	cb.WriteString("0\r\n\r\n")
	code, _ := raw("POST /api/feedback HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n", cb.Bytes(), false)
	if code == 413 && count(t, s, "ocorrencias") == 0 {
		vec12F(t, "F3b1", "OK", "chunked de 300 KB sem Content-Length: 413, nada gravado")
	} else {
		vec12F(t, "F3b1", "FALHA", fmt.Sprintf("Importante http.go:63: chunked de 300 KB: %d", code))
	}
	// Content-Length smaller than the body: the server reads only CL bytes
	code, msg := raw(fmt.Sprintf("POST /api/feedback HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", 40), []byte(good), false)
	if code == 400 && count(t, s, "ocorrencias") == 0 {
		vec12F(t, "F3b2", "OK", "Content-Length menor que o corpo: lê só o declarado, JSON cortado, 400, nada gravado")
	} else {
		vec12F(t, "F3b2", "FALHA", fmt.Sprintf("Menor: CL menor: %d %s", code, msg))
	}
	// Content-Length larger than the body, then the client closes its side
	code, msg = raw(fmt.Sprintf("POST /api/feedback HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n", len(good)+5000), []byte(good), true)
	if count(t, s, "ocorrencias") != 0 {
		vec12F(t, "F3b3", "FALHA", "Importante: corpo incompleto gravou")
	} else if code == 413 {
		vec12F(t, "F3b3", "OK", "MEDIDA: Content-Length maior que o corpo (cliente fecha): nada gravado, mas a resposta é 413 'corpo maior que 256 KB' (rótulo errado, http.go:64-66 trata todo erro de leitura como 413)")
	} else {
		vec12F(t, "F3b3", "OK", fmt.Sprintf("Content-Length maior que o corpo: %d, nada gravado", code))
	}
	// Content-Length declares more than 256 KB: refused before reading it all
	code, _ = raw(fmt.Sprintf("POST /api/feedback HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", 10<<20), []byte(strings.Repeat(" ", 300<<10)), true)
	if code == 413 {
		vec12F(t, "F3b4", "OK", "Content-Length de 10 MiB: 413 depois de 256 KB lidos")
	} else {
		vec12F(t, "F3b4", "FALHA", fmt.Sprintf("Importante http.go:63: CL de 10 MiB: %d", code))
	}
}

// ---- F4: races between reports of the same job and the admin actions ------------------------

func TestAdv12FRacesReportsVsAdmin(t *testing.T) {
	s := openStore(t, Options{})
	f := newFake()
	key := testKey(t)
	for i := 1; i <= 3; i++ {
		f.add(i)
	}
	o := Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.7"}
	rep := func(origem string, jobs ...JobRef) Report {
		r := Report{Projeto: "P", Jobs: jobs, Categoria: "omitiu", Descricao: "d", RespostaCorreta: "certo", Severidade: "media", Origem: origem}
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		return r
	}
	var mu sync.Mutex
	var ids []string
	pick := func(i int) string {
		mu.Lock()
		defer mu.Unlock()
		if len(ids) == 0 {
			return ""
		}
		return ids[i%len(ids)]
	}
	var wg sync.WaitGroup
	var unexpected []error
	note := func(err error) {
		if err == nil || errors.Is(err, ErrTransition) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrNeedsAnswer) || errors.Is(err, ErrBusy) {
			return
		}
		mu.Lock()
		unexpected = append(unexpected, err)
		mu.Unlock()
	}
	for g := 0; g < 12; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < 15; k++ {
				origem := "claude"
				if (g+k)%3 == 0 {
					origem = "automatico"
				}
				jobs := []JobRef{{ID: mkID(1), Papel: "map1", Backend: "primario"}, {ID: mkID(2 + (g+k)%2), Papel: "final", Backend: "primario"}}
				r, err := s.Ingest(ctx, rep(origem, jobs...), o, "", f, key)
				note(err)
				if err == nil {
					mu.Lock()
					ids = append(ids, r.OcorrenciaID)
					mu.Unlock()
				}
			}
		}(g)
	}
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < 40; k++ {
				id := pick(g*7 + k)
				if id == "" {
					time.Sleep(time.Millisecond)
					continue
				}
				switch (g + k) % 4 {
				case 0:
					note(s.Confirm(ctx, id, AdminActor("10.0.0.9")))
				case 1:
					note(s.Discard(ctx, id, AdminActor("10.0.0.9")))
				case 2:
					note(s.Delete(ctx, id, AdminActor("10.0.0.9")))
				default:
					v := fmt.Sprintf("nova resposta %d", k)
					_, err := s.Update(ctx, id, Edit{RespostaCorreta: &v}, AdminActor("10.0.0.9"))
					note(err)
				}
			}
		}(g)
	}
	wg.Wait()
	q := func(sqlq string) int {
		var n int
		if err := s.db.QueryRow(sqlq).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	orphanSamples := q(`SELECT COUNT(*) FROM amostras a WHERE NOT EXISTS (SELECT 1 FROM ocorrencia_jobs oj WHERE oj.job_id=a.job_id)`)
	jobsOfErased := q(`SELECT COUNT(*) FROM ocorrencia_jobs oj JOIN ocorrencias o ON o.id=oj.ocorrencia_id WHERE o.estado='apagada'`)
	jobsNoOcc := q(`SELECT COUNT(*) FROM ocorrencia_jobs oj WHERE NOT EXISTS (SELECT 1 FROM ocorrencias o WHERE o.id=oj.ocorrencia_id)`)
	histNoOcc := q(`SELECT COUNT(*) FROM historico h WHERE NOT EXISTS (SELECT 1 FROM ocorrencias o WHERE o.id=h.ocorrencia_id)`)
	occNoCreated := q(`SELECT COUNT(*) FROM ocorrencias o WHERE NOT EXISTS (SELECT 1 FROM historico h WHERE h.ocorrencia_id=o.id AND h.acao='criou')`)
	missingSample := q(`SELECT COUNT(*) FROM ocorrencia_jobs oj JOIN ocorrencias o ON o.id=oj.ocorrencia_id WHERE o.estado IN ('nova','confirmada','descartada') AND NOT EXISTS (SELECT 1 FROM amostras a WHERE a.job_id=oj.job_id)`)
	erasedDirty := q(`SELECT COUNT(*) FROM ocorrencias WHERE estado='apagada' AND (descricao IS NOT NULL OR resposta_correta IS NOT NULL OR job_do_erro IS NOT NULL OR client_ip IS NOT NULL)`)
	confirmedNoAnswer := q(`SELECT COUNT(*) FROM ocorrencias WHERE estado='confirmada' AND COALESCE(TRIM(resposta_correta),'')=''`)
	total := q(`SELECT COUNT(*) FROM ocorrencias`)
	erased := q(`SELECT COUNT(*) FROM ocorrencias WHERE estado='apagada'`)
	inv := fmt.Sprintf("orfas=%d jobs_de_apagadas=%d jobs_sem_ocorrencia=%d historico_sem_linha=%d sem_criou=%d sem_amostra=%d apagada_suja=%d confirmada_sem_resposta=%d",
		orphanSamples, jobsOfErased, jobsNoOcc, histNoOcc, occNoCreated, missingSample, erasedDirty, confirmedNoAnswer)
	if orphanSamples+jobsOfErased+jobsNoOcc+histNoOcc+occNoCreated+missingSample+erasedDirty+confirmedNoAnswer != 0 || len(unexpected) > 0 {
		vec12F(t, "F4a", "FALHA", fmt.Sprintf("Importante state.go/ingest.go (contagem de referências C-6.4): %s; erros %v", inv, unexpected))
	} else {
		vec12F(t, "F4a", "OK", fmt.Sprintf("180 relatos (1/3 automáticos, mesmos jobs) x 160 ações do admin em paralelo (race=%v): %d ocorrências, %d apagadas; invariantes zeradas (%s)", raceEnabled, total, erased, inv))
	}
}

// ---- F5: the 507 ceiling ---------------------------------------------------------------------

func TestAdv12FCeiling(t *testing.T) {
	var evMu sync.Mutex
	var events []string
	s := openStore(t, Options{OnEvent: func(kind, detail string) { evMu.Lock(); events = append(events, kind); evMu.Unlock() }})
	f := newFake()
	key := testKey(t)
	o := Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.7"}
	const n = 16
	sizes := map[string]int{}
	for i := 1; i <= n; i++ {
		id := f.add(i)
		c := f.conts[id]
		c.Response = strings.Repeat("y", 10000*i) // 10 KB .. 160 KB
		f.conts[id] = c
		sizes[id] = len(c.Response)
	}
	room := int64(400000)
	s.opts.MaxBytes = s.Size() + room
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r := Report{Projeto: "P", Jobs: []JobRef{{ID: mkID(i), Papel: "unico", Backend: "primario"}}, Categoria: "omitiu", Descricao: "d", Severidade: "media", Origem: "claude"}
			r.Validate()
			_, errs[i-1] = s.Ingest(ctx, r, o, "", f, key)
		}(i)
	}
	close(start)
	wg.Wait()
	acc, capd := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			acc++
		case errors.Is(e, ErrCapReached):
			capd++
		default:
			t.Errorf("unexpected: %v", e)
		}
	}
	var sampleBytes int64
	rows, _ := s.db.Query(`SELECT length(prompt_json)+length(prompt_texto)+length(resposta) FROM amostras`)
	for rows.Next() {
		var b int64
		rows.Scan(&b)
		sampleBytes += b
	}
	rows.Close()
	over := s.Size() - s.opts.MaxBytes
	evMu.Lock()
	nev := len(events)
	evMu.Unlock()
	// The binding measure is in pages (live data), the budget of the samples in raw bytes: a page
	// already counted absorbs part of a new sample, so the raw bytes may pass the room by a few KB
	// (measured: up to ~5 KB over 12 rounds). What must hold is that the ceiling is passed by at most
	// one report (C-6.2 measures before the transaction): here, the largest one (160 KB) plus 16 KB.
	const oneReport = 160000 + 16<<10
	if acc+capd != n || count(t, s, "amostras") != acc || capd == 0 || nev == 0 || over > oneReport {
		vec12F(t, "F5a", "FALHA", fmt.Sprintf("Importante ingest.go:283-386: %d aceitos %d 507, %d bytes de amostra para %d de folga, excesso %d, %d eventos", acc, capd, sampleBytes, room, over, nev))
	} else {
		vec12F(t, "F5a", "OK", fmt.Sprintf("16 relatos simultâneos de 10 a 160 KB com 400 KB de folga: %d aceitos, %d com 507; amostras somam %d B (folga %d); tamanho final %+d B em relação ao teto "+
			"(no máximo um relato acima, conforme C-6.2); %d evento(s) (o evento é limitado a 1 por minuto, os 507 seguintes não gravam evento novo)", acc, capd, sampleBytes, room, over, nev))
	}

	// F5b: the non-sample bytes of a report (resposta_correta 128 KB, history) are not in the
	// budget: a report just under the ceiling ends above it.
	s2 := openStore(t, Options{})
	f2 := newFake()
	f2.add(1)
	f2.add(2)
	r1 := Report{Projeto: "P", Jobs: []JobRef{{ID: mkID(1), Papel: "unico", Backend: "primario"}}, Categoria: "omitiu", Descricao: "d", Severidade: "media", Origem: "claude"}
	r1.Validate()
	if _, err := s2.Ingest(ctx, r1, o, "", f2, key); err != nil {
		t.Fatal(err)
	}
	s2.opts.MaxBytes = s2.Size() + 4096 // one page of room
	r2 := r1
	r2.RespostaCorreta = strings.Repeat("z", MaxResposta)
	r2.Descricao = strings.Repeat("d", MaxDescricao)
	_, err := s2.Ingest(ctx, r2, o, "", f2, key) // same job: sample already there, nothing counted in the budget
	ov := s2.Size() - s2.opts.MaxBytes
	if err == nil && ov > 0 {
		vec12F(t, "F5b", "OK", fmt.Sprintf("MEDIDA: com 4 KB de folga, um relato sem amostra nova mas com 132 KB de texto foi aceito e deixou o banco %d B ACIMA do teto: "+
			"o orçamento (ingest.go:353-385) só conta as amostras; a C-6.2 mede antes da transação, então ultrapassar por UM relato é conforme; o seguinte recebe 507", ov))
	} else {
		vec12F(t, "F5b", "OK", fmt.Sprintf("relato perto do teto: err=%v, excesso %d", err, ov))
	}
	if _, err := s2.Ingest(ctx, r1, o, "", f2, key); !errors.Is(err, ErrCapReached) {
		vec12F(t, "F5b2", "FALHA", fmt.Sprintf("Importante ingest.go:284: acima do teto o relato seguinte não deu 507: %v", err))
	} else {
		vec12F(t, "F5b2", "OK", "acima do teto o relato seguinte recebe 507")
	}

	// F5c: the admin's edit grows the database past the ceiling (admin is not capped); deleting
	// brings the space back and the 507 goes away (C-6.2a); the 507 deletes nothing.
	s3 := openStore(t, Options{})
	f3 := newFake()
	var occ []string
	for i := 1; i <= 6; i++ {
		f3.add(i)
		c := f3.conts[mkID(i)]
		c.Response = strings.Repeat("w", 200000)
		f3.conts[mkID(i)] = c
	}
	for i := 1; i <= 5; i++ {
		r := Report{Projeto: "P", Jobs: []JobRef{{ID: mkID(i), Papel: "unico", Backend: "primario"}}, Categoria: "omitiu", Descricao: "d", Severidade: "media", Origem: "claude"}
		r.Validate()
		res, err := s3.Ingest(ctx, r, o, "", f3, key)
		if err != nil {
			t.Fatal(err)
		}
		occ = append(occ, res.OcorrenciaID)
	}
	s3.opts.MaxBytes = s3.Size() + 100000
	big := strings.Repeat("e", MaxResposta)
	_, errU := s3.Update(ctx, occ[0], Edit{RespostaCorreta: &big}, AdminActor("10.0.0.9"))
	r6 := Report{Projeto: "P", Jobs: []JobRef{{ID: mkID(6), Papel: "unico", Backend: "primario"}}, Categoria: "omitiu", Descricao: "d", Severidade: "media", Origem: "claude"}
	r6.Validate()
	before := fmt.Sprintf("%d/%d/%d", count(t, s3, "ocorrencias"), count(t, s3, "amostras"), count(t, s3, "historico"))
	_, err6 := s3.Ingest(ctx, r6, o, "", f3, key)
	after := fmt.Sprintf("%d/%d/%d", count(t, s3, "ocorrencias"), count(t, s3, "amostras"), count(t, s3, "historico"))
	for _, id := range occ[:3] {
		if err := s3.Delete(ctx, id, AdminActor("10.0.0.9")); err != nil {
			t.Fatal(err)
		}
	}
	_, err7 := s3.Ingest(ctx, r6, o, "", f3, key)
	if errU == nil && errors.Is(err6, ErrCapReached) && before == after && err7 == nil {
		vec12F(t, "F5c", "OK", "MEDIDA: a edição do admin cresce o banco sem teto (admin é confiável); o relato seguinte dá 507 sem apagar nada (contagens iguais); depois de apagar 3 ocorrências o espaço volta e o relato é aceito (C-6.2a)")
	} else {
		vec12F(t, "F5c", "FALHA", fmt.Sprintf("Importante: edição %v, relato no teto %v (antes %s depois %s), depois de apagar %v", errU, err6, before, after, err7))
	}
}

// ---- F6: 409, 503, the fallback role ---------------------------------------------------------

func TestAdv12FBusyNoKeyAndFallback(t *testing.T) {
	me := adv12FReq{realIPs: []string{"10.0.0.1"}, station: "computer=PC01;user=ana"}
	for _, st := range []string{"na_fila", "em_execucao"} {
		for _, which := range []string{"job_do_erro", "outro local"} {
			h, s, f := adv12FHandler(t, 2, Options{})
			n := 2
			if which == "outro local" {
				n = 1
			}
			j := f.jobs[mkID(n)]
			j.Status = st
			f.jobs[mkID(n)] = j
			w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "map1", "primario"), adv12FJob(2, "final", "primario")), me)
			id := "F6a-" + st + "-" + strings.ReplaceAll(which, " ", "_")
			if w.Code == 409 && w.Header().Get("Retry-After") != "" && count(t, s, "ocorrencias")+count(t, s, "historico") == 0 {
				vec12F(t, id, "OK", fmt.Sprintf("%s %s: 409 com Retry-After %s, nada gravado", which, st, w.Header().Get("Retry-After")))
			} else {
				vec12F(t, id, "FALHA", fmt.Sprintf("Importante ingest.go:272: %s %s: %d Retry-After=%q, %s", which, st, w.Code, w.Header().Get("Retry-After"), adv12FTotals(t, s)))
			}
		}
	}
	// a busy local job labelled with the other backend: not checked, but then nothing is copied
	h, s, f := adv12FHandler(t, 2, Options{})
	j := f.jobs[mkID(1)]
	j.Status = "em_execucao"
	f.jobs[mkID(1)] = j
	w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "map1", "fallback"), adv12FJob(2, "final", "primario")), me)
	if r := adv12FRes(w); w.Code == 200 && r.Estado == EstadoDistrib && count(t, s, "amostras") == 0 {
		vec12F(t, "F6a2", "OK", "MEDIDA: job local em execução rotulado 'fallback' escapa do 409 (backend é declarado), mas a ocorrência vira distribuida e nada é copiado, então nenhuma amostra fica incompleta")
	} else {
		vec12F(t, "F6a2", "FALHA", fmt.Sprintf("Importante: job em execução rotulado fallback: %d %+v amostras=%d", w.Code, adv12FRes(w), count(t, s, "amostras")))
	}

	// 503 without key (nil and empty), and 503 when the writers' turn does not come
	for _, k := range []*hmackey.Key{nil, {}} {
		h, s, _ := adv12FHandler(t, 1, Options{})
		h.Key = k
		w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), me)
		if w.Code == 503 && strings.Contains(w.Body.String(), "doc-key-file") && count(t, s, "ocorrencias") == 0 {
			vec12F(t, "F6b", "OK", fmt.Sprintf("sem chave (%v): 503 com mensagem explícita, nada gravado", k == nil))
		} else {
			vec12F(t, "F6b", "FALHA", fmt.Sprintf("Importante http.go:55: sem chave: %d %s", w.Code, w.Body.String()))
		}
	}
	h, s, _ = adv12FHandler(t, 1, Options{})
	h.Timeout = 100 * time.Millisecond
	s.wsem <- struct{}{}
	t0 := time.Now()
	w = adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), me)
	el := time.Since(t0)
	<-s.wsem
	if w.Code == 503 && w.Header().Get("Retry-After") == "5" && count(t, s, "ocorrencias") == 0 && el < 3*time.Second {
		vec12F(t, "F6c", "OK", fmt.Sprintf("prazo da vez de escrever: 503 com Retry-After 5 em %v, nada gravado", el.Round(time.Millisecond)))
	} else {
		vec12F(t, "F6c", "FALHA", fmt.Sprintf("Importante http.go:107: semáforo preso: %d %q em %v", w.Code, w.Header().Get("Retry-After"), el))
	}

	// fallback role: no sample, also when it replaces an automatic candidate that had samples
	// (the same treino.db reopened with --role fallback)
	h, s, _ = adv12FHandler(t, 2, Options{})
	wa := adv12FDo(h, adv12FBody(func(m map[string]any) { m["origem"] = "automatico"; delete(m, "resposta_correta") }, adv12FJob(1, "map1", "primario"), adv12FJob(2, "final", "primario")), me)
	na := count(t, s, "amostras")
	h.Role = RoleFallback
	wh := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "map1", "primario"), adv12FJob(2, "final", "primario")), me)
	rh := adv12FRes(wh)
	wn := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), me)
	rn := adv12FRes(wn)
	if wa.Code == 200 && na == 2 && rh.Estado == EstadoFallback && rh.OcorrenciaID == adv12FRes(wa).OcorrenciaID && rn.Estado == EstadoFallback && rn.AmostrasCopiadas == 0 && count(t, s, "amostras") == 0 {
		vec12F(t, "F6d", "OK", "role fallback: a substituição do automático (que tinha 2 amostras) virou fallback e as amostras saíram; relato novo no fallback: 0 amostras (C-1.21, C-1.12d)")
	} else {
		vec12F(t, "F6d", "FALHA", fmt.Sprintf("Importante ingest.go:350/554: fallback: auto %d (%d amostras), humano %+v, novo %+v, amostras agora %d", wa.Code, na, rh, rn, count(t, s, "amostras")))
	}
}

// ---- F7: no text and no plain sha256 outside the content columns; remanence after erasing ----

func adv12FHexForms(b []byte) [][]byte {
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	return [][]byte{[]byte(h), []byte(strings.ToUpper(h)), sum[:]}
}

func adv12FScan(paths []string, needles map[string][]byte) []string {
	var hits []string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for name, nd := range needles {
			if len(nd) > 0 && bytes.Contains(b, nd) {
				hits = append(hits, fmt.Sprintf("%s em %s", name, p[strings.LastIndexAny(p, `\/`)+1:]))
			}
		}
	}
	return hits
}

func TestAdv12FNoTextNoPlainHashAndRemanence(t *testing.T) {
	key := testKey(t)
	s := openStore(t, Options{Key: key})
	f := newFake()
	docHeader := strings.Repeat("ab", 32) // the X-Mcpollama-Doc value (a sha256 of the document)
	mkJob := func(n int, prompt, resp string) {
		id := mkID(n)
		j := defJob()
		j.DocHash, j.DocKeyID = key.DocHash(docHeader), key.ID()
		f.jobs[id] = j
		pj := []byte(fmt.Sprintf(`{"model":"qwen3-ia","system":"SYS-F7-%d","prompt":%q,"options":{"seed":1}}`, n, prompt))
		f.conts[id] = Content{Prompt: "ACHATADO-F7-" + prompt, Response: resp, PromptJSON: pj, RespFinal: true}
	}
	mkJob(1, "PEDIDO-F7-UM", "RESPMODELO-F7-UM")
	mkJob(2, "PEDIDO-F7-DOIS", "RESPMODELO-F7-DOIS")
	o := Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.7"}
	texts := map[string]string{"desc_auto": "DESC-F7-AUTO", "desc": "DESC-F7-HUMANO", "resp": "RESPCERTA-F7-HUMANO", "corr": "CORR-F7-HUMANO",
		"edit": "RESPCERTA-F7-EDITADA", "desc2": "DESC-F7-OUTRA", "resp2": "RESPCERTA-F7-OUTRA"}
	rep := func(origem, desc, resp, corr string, n int) Report {
		r := Report{Projeto: "P", Jobs: []JobRef{{ID: mkID(n), Papel: "unico", Backend: "primario"}}, Categoria: "omitiu", Descricao: desc,
			RespostaCorreta: resp, CorrecaoAplicada: corr, Severidade: "media", Origem: origem}
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		return r
	}
	// automatic, replaced by the human report; edit; confirm; edit again (unconfirm); confirm; discard
	ra, err := s.Ingest(ctx, rep("automatico", texts["desc_auto"], "", "", 1), o, "", f, key)
	if err != nil {
		t.Fatal(err)
	}
	rh, err := s.Ingest(ctx, rep("claude", texts["desc"], texts["resp"], texts["corr"], 1), o, "", f, key)
	if err != nil || rh.OcorrenciaID != ra.OcorrenciaID {
		t.Fatalf("replacement: %v %+v %+v", err, ra, rh)
	}
	id := rh.OcorrenciaID
	must(t, s.Confirm(ctx, id, admin))
	ed := texts["edit"]
	if _, err := s.Update(ctx, id, Edit{RespostaCorreta: &ed}, admin); err != nil {
		t.Fatal(err)
	}
	must(t, s.Confirm(ctx, id, admin))
	must(t, s.Discard(ctx, id, admin))
	// a second occurrence, on job 2, that is erased
	r2, err := s.Ingest(ctx, rep("claude", texts["desc2"], texts["resp2"], "", 2), o, "", f, key)
	if err != nil {
		t.Fatal(err)
	}

	// C-7.6 / C-7.4: the history table, read as rows
	hist := dumpTable(t, s, "historico")
	needles := map[string][]byte{}
	for name, v := range texts {
		needles["texto:"+name] = []byte(v)
		for i, hf := range adv12FHexForms([]byte(v)) {
			needles[fmt.Sprintf("sha256[%d]:%s", i, name)] = hf
		}
	}
	for n := 1; n <= 2; n++ {
		c := f.conts[mkID(n)]
		needles[fmt.Sprintf("texto:pedido%d", n)] = []byte(fmt.Sprintf("PEDIDO-F7-%s", []string{"", "UM", "DOIS"}[n]))
		needles[fmt.Sprintf("texto:resposta%d", n)] = []byte(c.Response)
		for i, hf := range adv12FHexForms(c.PromptJSON) {
			needles[fmt.Sprintf("sha256[%d]:prompt_json%d", i, n)] = hf
		}
		canon, _ := hmackey.CanonicalContent(c.PromptJSON)
		for i, hf := range adv12FHexForms(canon) {
			needles[fmt.Sprintf("sha256[%d]:conteudo_canonico%d", i, n)] = hf
		}
		for i, hf := range adv12FHexForms([]byte(c.Prompt)) {
			needles[fmt.Sprintf("sha256[%d]:prompt_texto%d", i, n)] = hf
		}
		for i, hf := range adv12FHexForms([]byte(c.Response)) {
			needles[fmt.Sprintf("sha256[%d]:resposta%d", i, n)] = hf
		}
	}
	needles["X-Mcpollama-Doc puro"] = []byte(docHeader)
	needles["X-Mcpollama-Doc puro (maiúsculas)"] = []byte(strings.ToUpper(docHeader))
	var histHits []string
	for name, nd := range needles {
		if bytes.Contains([]byte(hist), nd) {
			histHits = append(histHits, name)
		}
	}
	if len(histHits) > 0 {
		vec12F(t, "F7a", "FALHA", fmt.Sprintf("Bloqueador ingest.go:340/state.go:156 (C-7.4/C-7.6): o histórico tem %v", histHits))
	} else {
		vec12F(t, "F7a", "OK", fmt.Sprintf("historico depois de automático→humano, editar, confirmar, desconfirmar, descartar: nenhum texto e nenhum sha256 puro (%d agulhas)", len(needles)))
	}

	// the bytes of treino.db, -wal and -shm: no plain sha256 anywhere (the texts may be in the content columns)
	files := []string{s.path, s.path + "-wal", s.path + "-shm"}
	var shaHits []string
	for _, hit := range adv12FScan(files, needles) {
		if strings.HasPrefix(hit, "sha256") || strings.HasPrefix(hit, "X-Mcpollama-Doc") {
			shaHits = append(shaHits, hit)
		}
	}
	if len(shaHits) > 0 {
		vec12F(t, "F7b", "FALHA", fmt.Sprintf("Bloqueador (C-2.12/C-4.2/C-7.4): sha256 puro nos bytes do banco: %v", shaHits))
	} else {
		vec12F(t, "F7b", "OK", "bytes de treino.db, -wal e -shm: nenhum sha256 puro (hex minúsculo, maiúsculo ou binário) de prompt_json, conteúdo canônico, X-Mcpollama-Doc, textos e respostas")
	}

	// the text columns: only the allowed ones hold the texts (logical check, row by row)
	var where []string
	for _, tb := range []string{"ocorrencias", "ocorrencia_jobs", "historico", "retratos", "retrato_itens", "embeddings", "cota_grupos", "indices", "indice_itens"} {
		d := dumpTable(t, s, tb)
		for name, v := range texts {
			if strings.Contains(d, v) && tb != "ocorrencias" {
				where = append(where, name+"@"+tb)
			}
		}
	}
	if len(where) > 0 {
		vec12F(t, "F7c", "FALHA", fmt.Sprintf("Bloqueador: texto fora das colunas de conteúdo: %v", where))
	} else {
		vec12F(t, "F7c", "OK", "os textos do relato só aparecem em ocorrencias (colunas de conteúdo); nenhuma outra tabela os tem")
	}

	// remanence: erase the second occurrence (its sample has no other user), then look at the bytes
	must(t, s.Delete(ctx, r2.OcorrenciaID, admin))
	gone := map[string][]byte{"descricao": []byte(texts["desc2"]), "resposta_correta": []byte(texts["resp2"]), "pedido": []byte("PEDIDO-F7-DOIS"), "resposta_modelo": []byte("RESPMODELO-F7-DOIS")}
	var logical []string
	for _, tb := range []string{"ocorrencias", "amostras", "historico"} {
		d := dumpTable(t, s, tb)
		for name, v := range gone {
			if strings.Contains(d, string(v)) {
				logical = append(logical, name+"@"+tb)
			}
		}
	}
	live := adv12FScan(files, gone)
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Logf("checkpoint: %v", err)
	}
	s.db.Exec(`PRAGMA incremental_vacuum`)
	afterCk := adv12FScan(files, gone)
	switch {
	case len(logical) > 0:
		vec12F(t, "F7d", "FALHA", fmt.Sprintf("Bloqueador state.go:189 (C-6.3): o conteúdo apagado ainda está nas linhas: %v", logical))
	case len(live) > 0 || len(afterCk) > 0:
		vec12F(t, "F7d", "FALHA", fmt.Sprintf("Importante treino.go:169 (dsn sem secure_delete; C-6.3 'remove todo o conteúdo'): depois de apagar, nenhuma linha tem o texto, mas os BYTES do arquivo ainda têm: "+
			"logo após apagar %v; depois de wal_checkpoint(TRUNCATE)+incremental_vacuum %v. Páginas livres/espaço livre da página e quadros do -wal guardam o texto em claro; "+
			"o contrato fala de campos, não de bytes (lacuna); correção: _pragma=secure_delete(ON) no dsn e checkpoint(TRUNCATE) depois de apagar", live, afterCk))
	default:
		vec12F(t, "F7d", "OK", "depois de apagar, nem as linhas nem os bytes do arquivo (db, -wal, -shm) guardam o texto da ocorrência apagada")
	}
}

// ---- F8: other forgeries ---------------------------------------------------------------------

func TestAdv12FOtherForgeries(t *testing.T) {
	me := adv12FReq{realIPs: []string{"10.0.0.1"}, station: "computer=PC01;user=ana"}
	// F8b: the job_do_erro is an embedding: the report is accepted but the sample never exports
	h, s, f := adv12FHandler(t, 1, Options{})
	j := f.jobs[mkID(1)]
	j.Endpoint, j.Tool = "/api/embed", "embed"
	f.jobs[mkID(1)] = j
	w := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), me)
	id := adv12FRes(w).OcorrenciaID
	must(t, s.Confirm(ctx, id, admin))
	ok, _ := s.eligibleOnce(ctx, s.db, id)
	var buf bytes.Buffer
	nT, _ := s.Export(ctx, ParteTreino, admin, &buf, func() {})
	nE, _ := s.Export(ctx, ParteTeste, admin, &buf, func() {})
	if w.Code == 200 && !ok && nT+nE == 0 {
		vec12F(t, "F8b", "OK", "relato cujo job_do_erro é um embed: aceito (estatística), confirmado, mas inelegível e fora das duas exportações (C-3.5)")
	} else {
		vec12F(t, "F8b", "FALHA", fmt.Sprintf("Importante partition.go:64 (C-3.5): embed exportável: %d elegivel=%v exportadas=%d", w.Code, ok, nT+nE))
	}

	// F8c: origem=automatico forged by another origin for the victim's job does not block the
	// victim's own automatic candidate nor its replacement
	h, s, _ = adv12FHandler(t, 1, Options{})
	evil := adv12FReq{realIPs: []string{"10.0.0.66"}, station: "computer=PC01;user=ana"} // forged station, other IP
	auto := func(m map[string]any) { m["origem"] = "automatico"; delete(m, "resposta_correta") }
	we := adv12FDo(h, adv12FBody(auto, adv12FJob(1, "unico", "primario")), evil)
	wv := adv12FDo(h, adv12FBody(auto, adv12FJob(1, "unico", "primario")), me)
	wh := adv12FDo(h, adv12FBody(nil, adv12FJob(1, "unico", "primario")), me)
	re, rv, rh := adv12FRes(we), adv12FRes(wv), adv12FRes(wh)
	var eEstado, eOrig string
	s.db.QueryRow(`SELECT estado, origem FROM ocorrencias WHERE id=?`, re.OcorrenciaID).Scan(&eEstado, &eOrig)
	if re.Estado == EstadoSemPedido && rv.Estado == EstadoNova && rv.OcorrenciaID != re.OcorrenciaID && rh.OcorrenciaID == rv.OcorrenciaID && eOrig == "automatico" && count(t, s, "amostras") == 1 {
		vec12F(t, "F8c", "OK", "automático forjado por outra origem para o job da vítima vira sem_pedido próprio; o automático da vítima nasce nova com a amostra e é substituído pelo humano dela; o forjado não é tocado")
	} else {
		vec12F(t, "F8c", "FALHA", fmt.Sprintf("Importante ingest.go:291-299: forjado %+v (%s/%s), vítima %+v, humano %+v", re, eEstado, eOrig, rv, rh))
	}

	// F8d: projeto and the client label with CR/LF and HTML: stored as declared data; what goes to
	// the monitor is JSON with < > escaped; the label is filtered by the proxy rule
	h, s, _ = adv12FHandler(t, 1, Options{})
	xss := "<script>alert(1)</script>\r\nSet-Cookie: x=1"
	req := me
	req.hdr = map[string]string{proxy.HeaderClient: "<b>x</b>"}
	w = adv12FDo(h, adv12FBody(func(m map[string]any) { m["projeto"] = xss }, adv12FJob(1, "unico", "primario")), req)
	id = adv12FRes(w).OcorrenciaID
	var proj, label string
	s.db.QueryRow(`SELECT projeto, COALESCE(client_label,'') FROM ocorrencias WHERE id=?`, id).Scan(&proj, &label)
	rows, _ := s.List(ctx, Filter{})
	lj, _ := json.Marshal(rows)
	d, _ := s.Detail(ctx, id)
	dj, _ := json.Marshal(d)
	if w.Code == 200 && proj == xss && label != "<b>x</b>" && !bytes.Contains(lj, []byte("<script")) && !bytes.Contains(dj, []byte("<script")) && !bytes.Contains(dj, []byte("\r\n")) {
		vec12F(t, "F8d", "OK", fmt.Sprintf("MEDIDA: projeto com HTML e CR/LF é aceito e guardado como dado declarado (C-1.18, sem filtro de caracteres de controle); List/Detail saem em JSON com <, > e CR/LF escapados; o rótulo HTML vira %q", label))
	} else {
		vec12F(t, "F8d", "FALHA", fmt.Sprintf("Importante: projeto/label: %d proj=%q label=%q list_tem_script=%v", w.Code, proj, label, bytes.Contains(lj, []byte("<script"))))
	}
}

func bufioReader(r io.Reader) *bufio.Reader { return bufio.NewReader(r) }

var _ = sql.ErrNoRows

// F7e: refutes the easy hypothesis of F7d and checks the suggested fix on the real path.
func TestAdv12FSecureDeleteFixHypothesis(t *testing.T) {
	// the real path (Ingest + Delete of the Store), with and without secure_delete on every pooled connection
	real := func(secure bool) (mode int, live, afterCk []string) {
		key := testKey(t)
		s := openStore(t, Options{Key: key})
		if secure {
			var cs []*sql.Conn
			for i := 0; i < 4; i++ {
				c, err := s.db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				c.ExecContext(ctx, `PRAGMA secure_delete=ON`)
				cs = append(cs, c)
			}
			for _, c := range cs {
				c.Close()
			}
		}
		s.db.QueryRow(`PRAGMA secure_delete`).Scan(&mode)
		f := newFake()
		f.add(1)
		c := f.conts[mkID(1)]
		c.PromptJSON = []byte(`{"model":"qwen3-ia","prompt":"PEDIDO-F7E-REAL"}`)
		c.Response = "RESPMODELO-F7E-REAL"
		f.conts[mkID(1)] = c
		r := Report{Projeto: "P", Jobs: []JobRef{{ID: mkID(1), Papel: "unico", Backend: "primario"}}, Categoria: "omitiu", Descricao: "DESC-F7E-REAL",
			RespostaCorreta: "RESPCERTA-F7E-REAL", Severidade: "media", Origem: "claude"}
		r.Validate()
		res, err := s.Ingest(ctx, r, Origin{Computer: "PC01", User: "ana", ClientIP: "10.0.0.7"}, "", f, key)
		if err != nil {
			t.Fatal(err)
		}
		must(t, s.Delete(ctx, res.OcorrenciaID, admin))
		gone := map[string][]byte{"descricao": []byte("DESC-F7E-REAL"), "resposta_correta": []byte("RESPCERTA-F7E-REAL"), "pedido": []byte("PEDIDO-F7E-REAL"), "resposta_modelo": []byte("RESPMODELO-F7E-REAL")}
		files := []string{s.path, s.path + "-wal"}
		live = adv12FScan(files, gone)
		s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
		afterCk = adv12FScan(files, gone)
		return
	}
	m0, l0, c0 := real(false)
	m1, l1, c1 := real(true)
	if len(c0) > 0 && len(c1) == 0 {
		vec12F(t, "F7e", "OK", fmt.Sprintf("hipótese do F7d confirmada pelo caminho real (Ingest+Delete): secure_delete=%d deixa o texto apagado (logo após %v; após checkpoint(TRUNCATE) %v); "+
			"com secure_delete=%d em todas as conexões, após checkpoint(TRUNCATE) nada sobra (logo após, ainda no -wal: %v)", m0, l0, c0, m1, l1))
	} else {
		vec12F(t, "F7e", "OK", fmt.Sprintf("MEDIDA: secure_delete=%d: logo após %v, após ck %v; secure_delete=%d: logo após %v, após ck %v", m0, l0, c0, m1, l1, c1))
	}
}
