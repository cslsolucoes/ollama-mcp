package pool_test

// Fase 10.5 - revisao adversarial do pool de backends (10.4).
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado e FALHA quando o
// defeito existe. Os Ollamas sao falsos (httptest); nada sai da maquina.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func vec5(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// ---- Ollama falso configuravel ------------------------------------------------------

type advFake struct {
	srv    *httptest.Server
	mu     sync.Mutex
	hits   map[string]int
	seen   []string // cabecalhos e trailers de cada geracao (para procurar tokens)
	digest string
	gen    func(w http.ResponseWriter, r *http.Request, body string) // nil: responde ok
	health func(w http.ResponseWriter, r *http.Request) bool         // true: ja respondeu
}

func newAdvFake(t *testing.T, digest string) *advFake {
	t.Helper()
	f := &advFake{hits: map[string]int{}, digest: digest}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *advFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	gen, health, dg := f.gen, f.health, f.digest
	f.mu.Unlock()
	switch r.URL.Path {
	case "/api/version", "/api/ps", "/api/tags":
		if health != nil && health(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"version":"0.40.8"}`)
		case "/api/ps":
			fmt.Fprintf(w, `{"models":[{"name":"qwen3-ia","model":"qwen3-ia","size":100,"size_vram":100,"digest":%q}]}`, dg)
		default:
			fmt.Fprintf(w, `{"models":[{"name":"qwen3-ia","model":"qwen3-ia","size":100,"digest":%q}]}`, dg)
		}
		return
	}
	b, _ := io.ReadAll(r.Body)
	var sb strings.Builder
	for k, v := range r.Header {
		sb.WriteString(k + "=" + strings.Join(v, ",") + ";")
	}
	for k, v := range r.Trailer {
		sb.WriteString("TRAILER:" + k + "=" + strings.Join(v, ",") + ";")
	}
	f.mu.Lock()
	f.seen = append(f.seen, sb.String())
	f.mu.Unlock()
	if gen != nil {
		gen(w, r, string(b))
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	fmt.Fprint(w, `{"response":"ok","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
}

func (f *advFake) set(fn func(f *advFake)) { f.mu.Lock(); fn(f); f.mu.Unlock() }
func (f *advFake) hit(p string) int        { f.mu.Lock(); defer f.mu.Unlock(); return f.hits[p] }
func (f *advFake) headers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

// advRig: primario + 1 secundario reais (HTTP), fila, pool e o proxy de verdade.
type advRig struct {
	prim, sec *advFake
	pl        *pool.Pool
	q         *queue.Manager
	gw        *httptest.Server
	evs       *events
	mu        sync.Mutex
	recs      []queue.Record
}

type advOpt struct {
	secTipo     string
	secToken    string
	breakerOpen time.Duration
	runTimeout  time.Duration
	checkTO     time.Duration
	ovQueue     int
}

func newAdvRig(t *testing.T, o advOpt) *advRig {
	t.Helper()
	if o.secTipo == "" {
		o.secTipo = "gpu"
	}
	if o.checkTO == 0 {
		o.checkTO = time.Second
	}
	if o.ovQueue == 0 {
		o.ovQueue = 6
	}
	r := &advRig{evs: &events{}}
	r.prim = newAdvFake(t, "sha256-aaaa")
	r.sec = newAdvFake(t, "sha256-aaaa")
	sp := pool.Spec{ID: "sec-x", URL: r.sec.srv.URL, Tipo: o.secTipo, Slots: 2}
	if o.secToken != "" {
		p := filepath.Join(t.TempDir(), "sec.token")
		if err := os.WriteFile(p, []byte(o.secToken+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sp.TokenFile = p
	}
	secs, err := pool.Build([]pool.Spec{sp}, nil, "-think", o.checkTO, nil)
	if err != nil {
		t.Fatal(err)
	}
	pu := mustURL(t, r.prim.srv.URL)
	r.pl = pool.New(pool.Config{PrimarySlots: 1, PrimaryURL: pu, Secondaries: secs, QueueMax: 16, OverloadQueue: o.ovQueue,
		BreakerOpen: o.breakerOpen, OnEvent: r.evs.add})
	r.q = queue.New(queue.Config{Pool: r.pl, QueueMax: 16, PerClient: 16,
		OnRecord: func(rec queue.Record) { r.mu.Lock(); r.recs = append(r.recs, rec); r.mu.Unlock() }})
	px, err := proxy.New(proxy.Config{Upstream: pu, Queue: r.q, Pool: r.pl, RunTimeout: o.runTimeout, StripOptions: proxy.DefaultStripOptions})
	if err != nil {
		t.Fatal(err)
	}
	r.gw = httptest.NewServer(px)
	t.Cleanup(r.gw.Close)
	r.checkAll()
	return r
}

func (r *advRig) checkAll() {
	for _, b := range r.pl.Backends() {
		r.pl.CheckOnce(context.Background(), b)
	}
}

func (r *advRig) records() []queue.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]queue.Record(nil), r.recs...)
}

func (r *advRig) status(id string) pool.Status {
	for _, s := range r.q.PoolStatus() {
		if s.ID == id {
			return s
		}
	}
	return pool.Status{}
}

func (r *advRig) post(t *testing.T, path, body string, hdr map[string]string) (int, http.Header) {
	t.Helper()
	req, _ := http.NewRequest("POST", r.gw.URL+path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cl.Do(req)
	if err != nil {
		return 0, nil
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, resp.Header
}

// primaryDown forces S3 (the active check of the primary fails once).
func (r *advRig) primaryDown() {
	r.prim.set(func(f *advFake) {
		f.health = func(w http.ResponseWriter, r *http.Request) bool { http.Error(w, "down", 500); return true }
	})
	r.pl.CheckOnce(context.Background(), r.pl.Primary())
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func snapWith(digest string) models.Snapshot {
	return models.Snapshot{Installed: []models.Installed{{Name: "qwen3-ia", Digest: digest}}}
}

// ======================================================================================
// 3. Disjuntor e saude
// ======================================================================================

// B1: um secundario cujo disjuntor abriu (3 checagens falhas, p. ex. 15 s de
// rede fora) volta a ficar saudavel. Ele precisa voltar a receber trabalho.
func TestAdv5B1SecondaryBreakerRecovers(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, BreakerOpen: 40 * time.Millisecond, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 1}}})
	sec := r.pl.Backend("sec-gpu")
	r.secs["sec-gpu"].set(func(f *fake) { f.version = 500 })
	for i := 0; i < 3; i++ {
		r.pl.CheckOnce(context.Background(), sec)
	}
	r.secs["sec-gpu"].set(func(f *fake) { f.version = 200 })
	time.Sleep(60 * time.Millisecond)
	r.pl.CheckOnce(context.Background(), sec) // o backend voltou
	time.Sleep(60 * time.Millisecond)
	r.pl.CheckOnce(context.Background(), sec)
	a := r.submit(queue.Request{}).started(t) // ocupa o primario
	defer a.Release(queue.Result{})
	b := r.submit(queue.Request{})
	c := r.submit(queue.Request{}) // 2 esperando > OverloadQueue 1: S2
	got := ""
	dl := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(dl) && got == "" {
		r.q.Poke()
		for _, j := range []*job{b, c} {
			if j.isStarted() && j.err == nil {
				got = j.tk.Backend()
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	st := r.q.PoolStatus()[1]
	if got != "sec-gpu" {
		vec5(t, "B1-disjuntor-secundario-preso", "Media", "gateway/pool/pool.go:318", fmt.Sprintf("FALHA: o secundario saudavel ficou fora do pool para sempre (estado=%s disjuntor=%s); com S2 ligado nenhum pedido foi para ele", st.State, st.Breaker))
		t.Fail()
		return
	}
	vec5(t, "B1-disjuntor-secundario-preso", "Media", "gateway/pool/pool.go:318", "OK: o disjuntor do secundario fecha depois que a checagem volta e ele recebe o repasse (estado="+st.State+")")
}

// B2: nenhum padrao que o cliente controla abre o disjuntor do primario nem o
// tira da GPU: 500, 404 de modelo, corte do cliente, timeout de execucao,
// keep_alive:0 e options de placement (o proxy os remove).
func TestAdv5B2ClientCannotOpenTheBreaker(t *testing.T) {
	r := newAdvRig(t, advOpt{runTimeout: 80 * time.Millisecond, ovQueue: 12})
	r.prim.set(func(f *advFake) {
		f.gen = func(w http.ResponseWriter, rq *http.Request, body string) {
			switch {
			case strings.Contains(body, "P500"):
				http.Error(w, `{"error":"boom"}`, 500)
			case strings.Contains(body, "nao-existe"):
				http.Error(w, `{"error":"model not found"}`, 404)
			case strings.Contains(body, "LENTO"):
				select {
				case <-rq.Context().Done():
				case <-time.After(3 * time.Second):
				}
			case strings.Contains(body, "num_gpu") || strings.Contains(body, "keep_alive"):
				http.Error(w, "placement chegou ao Ollama", 418)
			default:
				w.Header().Set("Content-Type", "application/x-ndjson")
				fmt.Fprint(w, `{"response":"ok","done":true}`+"\n")
			}
		}
	})
	bad := 0
	for i := 0; i < 5; i++ {
		r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"P500"}`, nil)
		r.post(t, "/api/generate", `{"model":"nao-existe","prompt":"x"}`, nil)
		r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"LENTO"}`, nil) // run_timeout
		if c, _ := r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"x","keep_alive":0,"options":{"num_gpu":0,"main_gpu":3,"low_vram":true}}`, nil); c == 418 {
			bad++
		}
		// o cliente desiste no meio
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, "POST", r.gw.URL+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"LENTO"}`))
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
		cancel()
	}
	time.Sleep(100 * time.Millisecond)
	st := r.status(pool.DefaultPrimaryID)
	if st.Breaker != "fechado" || r.evs.has("breaker_open") || st.State != "ok" || bad > 0 {
		vec5(t, "B2-cliente-abre-disjuntor", "Alta", "gateway/queue/queue.go:652", fmt.Sprintf("FALHA: disjuntor=%s estado=%s placement=%d eventos=%v", st.Breaker, st.State, bad, r.evs.kinds()))
		t.Fail()
		return
	}
	vec5(t, "B2-cliente-abre-disjuntor", "Alta", "gateway/queue/queue.go:652", fmt.Sprintf("OK: 25 pedidos hostis (500, 404, run_timeout, keep_alive/num_gpu, corte) e o primario segue fechado/ok; %d registros", len(r.records())))
}

// B3: o backend fecha a conexao DEPOIS de recebido o pedido, antes dos
// cabecalhos (o que um defeito do Ollama acionado pelo corpo faria). Isso nao e
// "inalcancavel": a conexao foi feita. Contar como falha deixa um cliente abrir
// o disjuntor com 3 pedidos.
func TestAdv5B3DropBeforeHeadersIsNeutral(t *testing.T) {
	r := newAdvRig(t, advOpt{})
	r.prim.set(func(f *advFake) {
		f.gen = func(w http.ResponseWriter, rq *http.Request, body string) {
			if strings.Contains(body, "DERRUBA") {
				hj, _ := w.(http.Hijacker)
				c, _, _ := hj.Hijack()
				c.Close()
				return
			}
			fmt.Fprint(w, `{"response":"ok","done":true}`+"\n")
		}
	})
	codes := []int{}
	for i := 0; i < 4; i++ {
		c, _ := r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"DERRUBA"}`, nil)
		codes = append(codes, c)
	}
	st := r.status(pool.DefaultPrimaryID)
	if st.Breaker != "fechado" {
		vec5(t, "B3-corte-antes-dos-cabecalhos", "Baixa", "gateway/proxy/proxy.go:491", fmt.Sprintf("FALHA: 3 pedidos cujo corpo faz o backend fechar a conexao abriram o disjuntor do primario (%s; codigos %v)", st.Breaker, codes))
		t.Fail()
		return
	}
	// mas um backend que recusa a conexao continua contando
	r.prim.srv.Close()
	for i := 0; i < 3; i++ {
		r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"x"}`, nil)
	}
	if st := r.status(pool.DefaultPrimaryID); st.Breaker != "aberto" {
		vec5(t, "B3-corte-antes-dos-cabecalhos", "Baixa", "gateway/proxy/proxy.go:491", "FALHA: a correcao deixou de contar a recusa de conexao: "+st.Breaker)
		t.Fail()
		return
	}
	vec5(t, "B3-corte-antes-dos-cabecalhos", "Baixa", "gateway/proxy/proxy.go:491", fmt.Sprintf("OK: corte depois da conexao e neutro (codigos %v); recusa de conexao x3 abre", codes))
}

// B4: inanicao do primario pela histerese: com a sobrecarga ainda ligada
// ("hist"), um slot livre no primario saudavel e usado primeiro.
func TestAdv5B4NoPrimaryStarvation(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Hold: time.Hour, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 4}}})
	a := r.submit(queue.Request{}).started(t)
	b := r.submit(queue.Request{})
	c := r.submit(queue.Request{}) // S2: um deles vai para o secundario
	waitFor(t, "repasse", func() bool { return b.isStarted() || c.isStarted() })
	a.Release(queue.Result{}) // o primario libera; a sobrecarga segue (hold de 1 h)
	var on []string
	for i := 0; i < 4; i++ {
		tk := r.submit(queue.Request{}).started(t)
		on = append(on, tk.Backend())
		tk.Release(queue.Result{})
	}
	if on[0] != pool.DefaultPrimaryID {
		vec5(t, "B4-inanicao-histerese", "Media", "gateway/queue/queue.go:917", fmt.Sprintf("FALHA: com o primario livre e saudavel o trabalho foi para %v", on))
		t.Fail()
		return
	}
	vec5(t, "B4-inanicao-histerese", "Media", "gateway/queue/queue.go:917", fmt.Sprintf("OK: sob histerese o primario livre leva os pedidos: %v", on))
}

// B5: flapping: a checagem alterna falha/ok. O disjuntor nao pode abrir (nunca
// 3 falhas seguidas); mede-se o custo em eventos.
func TestAdv5B5Flapping(t *testing.T) {
	ev := &events{}
	pl, sec := newBare(t, ev)
	for i := 0; i < 40; i++ {
		var err error
		if i%2 == 0 {
			err = fmt.Errorf("dial tcp: recusada")
		}
		pl.Observe(sec.ID, "0.40.8", snapWith("sha256-aaaa"), err)
	}
	n := 0
	for _, k := range ev.kinds() {
		if k == "backend_down" || k == "backend_up" {
			n++
		}
	}
	if ev.has("breaker_open") {
		vec5(t, "B5-flapping", "Baixa", "gateway/pool/pool.go:206", "FALHA: falhas alternadas abriram o disjuntor")
		t.Fail()
		return
	}
	vec5(t, "B5-flapping", "Baixa", "gateway/pool/pool.go:686", fmt.Sprintf("MEDIDA: 40 checagens alternadas -> disjuntor fechado e %d eventos backend_down/up (1 por troca; a 5 s isso daria ate 17.280/dia por backend)", n))
}

// B6: custo da checagem ativa e backend que nunca responde.
func TestAdv5B6ActiveCheckCost(t *testing.T) {
	r := newAdvRig(t, advOpt{checkTO: 200 * time.Millisecond})
	before := r.sec.hit("/api/version") + r.sec.hit("/api/ps") + r.sec.hit("/api/tags")
	r.pl.CheckOnce(context.Background(), r.pl.Backend("sec-x"))
	per := r.sec.hit("/api/version") + r.sec.hit("/api/ps") + r.sec.hit("/api/tags") - before
	release := make(chan struct{})
	r.sec.set(func(f *advFake) {
		f.health = func(w http.ResponseWriter, rq *http.Request) bool {
			select {
			case <-rq.Context().Done():
			case <-release:
			}
			return true
		}
	})
	t0 := time.Now()
	r.pl.CheckOnce(context.Background(), r.pl.Backend("sec-x"))
	d := time.Since(t0)
	close(release)
	st := r.status("sec-x")
	if per > 3 || d > 900*time.Millisecond || st.State != "fora" {
		vec5(t, "B6-custo-checagem", "Baixa", "gateway/pool/pool.go:650", fmt.Sprintf("FALHA: %d pedidos por rodada; rodada presa durou %s; estado %s", per, d.Round(time.Millisecond), st.State))
		t.Fail()
		return
	}
	vec5(t, "B6-custo-checagem", "Baixa", "gateway/pool/pool.go:650", fmt.Sprintf("MEDIDA: %d GET por rodada (version+ps+tags); backend mudo: rodada de %s com timeout de 200 ms por GET, estado %s", per, d.Round(time.Millisecond), st.State))
}

// ======================================================================================
// 4. Repasse
// ======================================================================================

// R1: um pedido em execucao nunca muda de backend (drenar, disjuntor, sobrecarga).
func TestAdv5R1RunningRequestNeverMoves(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 2}}})
	a := r.submit(queue.Request{}).started(t)
	r.pl.SetDrained(pool.DefaultPrimaryID, true)
	r.prim.set(func(f *fake) { f.version = 500 })
	for i := 0; i < 3; i++ {
		r.pl.CheckOnce(context.Background(), r.pl.Primary())
	}
	for i := 0; i < 3; i++ {
		r.submit(queue.Request{})
	}
	r.q.Poke()
	if a.Backend() != pool.DefaultPrimaryID || a.FailoverRule() != "" {
		vec5(t, "R1-execucao-repassada", "Alta", "gateway/queue/queue.go:851", "FALHA: o pedido em execucao mudou para "+a.Backend())
		t.Fail()
		return
	}
	a.Release(queue.Result{})
	vec5(t, "R1-execucao-repassada", "Alta", "gateway/queue/queue.go:851", "OK: drenar, abrir o disjuntor e sobrecarregar nao movem o pedido que ja comecou")
}

// R2: "alta" nunca vai para cpu, mesmo que o arquivo liste a classe.
func TestAdv5R2HighNeverOnCPU(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, Secs: []secCfg{{ID: "sec-cpu", Tipo: "cpu", Slots: 4, Classes: []string{"alta", "normal", "baixa"}}}})
	r.prim.set(func(f *fake) { f.version = 500 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary()) // S3
	var bad []string
	for i := 0; i < 6; i++ {
		j := r.submit(queue.Request{PriorityFinal: queue.High})
		time.Sleep(10 * time.Millisecond)
		r.q.Poke()
		if j.isStarted() && j.err == nil && j.tk.Backend() == "sec-cpu" {
			bad = append(bad, j.tk.ID)
		}
	}
	if len(bad) > 0 {
		vec5(t, "R2-alta-na-cpu", "Media", "gateway/pool/pool.go:434", fmt.Sprintf("FALHA: %d pedido(s) alta na cpu", len(bad)))
		t.Fail()
		return
	}
	vec5(t, "R2-alta-na-cpu", "Media", "gateway/pool/pool.go:434", "OK: com o primario fora e a cpu listando 'alta', nenhum pedido alta foi para a cpu")
}

// R3: corrida entre a escolha do backend e o drenar: depois de SetDrained +
// Poke (o que o monitor faz), nenhuma decisao nova escolhe o drenado.
func TestAdv5R3DrainRace(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, QueueMax: 64, OverloadQueue: 1, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 4}}})
	r.prim.set(func(f *fake) { f.version = 500 })
	r.pl.CheckOnce(context.Background(), r.pl.Primary()) // S3: tudo vai ao secundario
	var stop atomic.Bool
	var mu sync.Mutex
	var started []*queue.Ticket
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				tk, err := r.q.Acquire(ctx, queue.Request{Model: testModel, ClientIP: fmt.Sprintf("10.66.%d.%d", g, i%200+1), ClientLabel: "t"})
				cancel()
				if err == nil {
					mu.Lock()
					started = append(started, tk)
					mu.Unlock()
					tk.Release(queue.Result{})
				}
			}
		}(g)
	}
	time.Sleep(50 * time.Millisecond)
	r.pl.SetDrained("sec-gpu", true)
	r.q.Poke()
	cut := time.Now()
	time.Sleep(150 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
	late := 0
	mu.Lock()
	for _, tk := range started {
		if tk.Backend() == "sec-gpu" && tk.StartedAt().After(cut) {
			late++
		}
	}
	n := len(started)
	mu.Unlock()
	if late > 0 {
		vec5(t, "R3-corrida-drenar", "Media", "gateway/monitor/backends.go:46", fmt.Sprintf("FALHA: %d inicio(s) no drenado depois de SetDrained+Poke", late))
		t.Fail()
		return
	}
	vec5(t, "R3-corrida-drenar", "Media", "gateway/monitor/backends.go:46", fmt.Sprintf("OK: %d pedidos em 4 goroutines; 0 inicios no drenado depois de SetDrained+Poke (um inicio decidido antes do drenar ainda pode cair nele: e o mesmo que ter comecado um instante antes)", n))
}

// R4: justica no pool: um IP que inunda a fila nao passa na frente do IP novo
// quando o secundario abre vaga.
func TestAdv5R4FairnessAcrossBackends(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, QueueMax: 16, OverloadQueue: 2, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 1}}})
	ipA := "10.44.0.1"
	a1 := r.submit(queue.Request{ClientIP: ipA}).started(t)
	defer a1.Release(queue.Result{})
	var as []*job
	for i := 0; i < 3; i++ {
		as = append(as, r.submit(queue.Request{ClientIP: ipA}))
		time.Sleep(5 * time.Millisecond)
	}
	waitFor(t, "um A no secundario", func() bool {
		for _, j := range as {
			if j.isStarted() {
				return true
			}
		}
		return false
	})
	var onSec *queue.Ticket
	for _, j := range as {
		if j.isStarted() {
			onSec = j.tk
		}
	}
	b := r.submit(queue.Request{ClientIP: "10.44.0.2"})
	r.waitWaiting(3)
	onSec.Release(queue.Result{})
	r.q.Poke()
	waitFor(t, "proximo no secundario", func() bool {
		if b.isStarted() {
			return true
		}
		for _, j := range as {
			if j.isStarted() && j.tk != onSec {
				return true
			}
		}
		return false
	})
	if !b.isStarted() {
		vec5(t, "R4-justica-no-pool", "Media", "gateway/queue/queue.go:929", "FALHA: o IP que inunda a fila levou a vaga do secundario antes do IP novo")
		t.Fail()
		return
	}
	vec5(t, "R4-justica-no-pool", "Media", "gateway/queue/queue.go:929", "OK: a vaga do secundario foi para o IP novo (a justica e a da fila unica); o limite por IP ja e o da fila (TestF8)")
}

// R5: max_prompt_tokens pela estimativa bytes/3: um corpo de digitos (o Qwen
// tokeniza cada digito como 1 token) tem ~3x mais tokens do que a estimativa.
func TestAdv5R5TokenEstimateIsManipulable(t *testing.T) {
	digits := strings.Repeat("7", 3000)
	body := `{"model":"qwen3-ia","prompt":"` + digits + `"}`
	est := pool.EstimateTokens(int64(len(body)))
	// cota superior honesta de um BPE de bytes: 1 token por byte do texto
	upper := int64(len(digits))
	vec5(t, "R5-estimativa-tokens", "Baixa", "gateway/pool/pool.go:62", fmt.Sprintf("MEDIDA: %d bytes de corpo -> estimativa %d tokens; so os %d digitos podem dar %d tokens (%.1fx). Um max_prompt_tokens=1000 da cpu aceita este pedido. Efeito: prefill mais longo na cpu, limitado pelo --run-timeout; decisao do owner (bytes/1 e a cota segura)", len(body), est, len(digits), upper, float64(upper)/float64(est)))
}

// R6: o digest vem do proprio secundario (/api/tags). Um secundario mal-
// intencionado declara o digest do primario e serve outro modelo. Modelo de
// ameaca: o secundario esta no backends.json do root e ja recebe o prompt; o
// digest e uma checagem de consistencia (engano de configuracao), nao uma
// fronteira de seguranca.
func TestAdv5R6DigestIsSelfReported(t *testing.T) {
	r := newAdvRig(t, advOpt{})
	r.sec.set(func(f *advFake) {
		f.gen = func(w http.ResponseWriter, rq *http.Request, body string) {
			fmt.Fprint(w, `{"response":"resposta de OUTRO modelo","done":true}`+"\n")
		}
	})
	r.primaryDown()
	c, _ := r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"x"}`, nil)
	vec5(t, "R6-digest-forjado", "Info", "gateway/pool/pool.go:470", fmt.Sprintf("ACEITO (modelo de ameaca): o secundario que declara o digest do primario recebe o repasse (HTTP %d, %d geracao no secundario). Protecao real = confianca no host do backends.json (root, 0600) + TLS + token proprio", c, r.sec.hit("/api/generate")))
}

// ======================================================================================
// 1. SSRF e confianca no backends.json
// ======================================================================================

// S1: url com userinfo, esquema estranho ou porta invalida.
func TestAdv5S1RegistryURL(t *testing.T) {
	cases := map[string]bool{ // url -> deve ser aceita
		"http://10.0.0.5:11434":            true,
		"https://ia2.example:8443/":        true,
		"http://user:pw@10.0.0.5:11434":    false,
		"file:///etc/passwd":               false,
		"unix:///var/run/ollama.sock":      false,
		"gopher://10.0.0.5:70":             false,
		"http://10.0.0.5:0":                false,
		"http://10.0.0.5:99999":            false,
		"http://10.0.0.5:-1":               false,
		"http://10.0.0.5:65536":            false,
		"http://169.254.169.254":           false,
		"http://[fe80::1]:11434":           false,
		"http://0.0.0.0:11434":             false,
		"http://10.0.0.5:11434/api":        false,
		"http://10.0.0.5:11434?x=1":        false,
		"http://10.0.0.5:11434#f":          false,
		"//10.0.0.5:11434":                 false,
		"http:/10.0.0.5":                   false,
		"http://10.0.0.5:11434\r\nX-A: b":  false,
		"http://%31%30.0.0.5:11434":        false,
		"http://[::ffff:169.254.169.254]/": false,
	}
	dir := t.TempDir()
	var wrong []string
	keys := make([]string, 0, len(cases))
	for k := range cases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, u := range keys {
		p := filepath.Join(dir, fmt.Sprintf("b%d.json", i))
		js := fmt.Sprintf(`{"backends":[{"id":"s1","url":%q,"tipo":"gpu"}]}`, u)
		if err := os.WriteFile(p, []byte(js), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := pool.LoadFile(p)
		if (err == nil) != cases[u] {
			wrong = append(wrong, fmt.Sprintf("%q aceita=%v", u, err == nil))
		}
	}
	if len(wrong) > 0 {
		vec5(t, "S1-url-registro", "Baixa", "gateway/pool/registry.go:214", "FALHA: "+strings.Join(wrong, "; "))
		t.Fail()
		return
	}
	vec5(t, "S1-url-registro", "Baixa", "gateway/pool/registry.go:214", fmt.Sprintf("OK: %d URLs (userinfo, file:, unix:, gopher:, porta 0/-1/65536/99999, link-local, 0.0.0.0, caminho, query, CRLF) com o resultado esperado", len(cases)))
}

// S2: o secundario responde 3xx (para outro host, ou para o loopback de outro
// servico) e poe Set-Cookie. O gateway nao segue (o ReverseProxy nunca segue),
// mas repassava o 3xx e o cookie ao cliente: um SDK que segue redirect mandaria
// o prompt para o Location, e o cookie cairia na origem do monitor.
func TestAdv5S2RedirectFromASecondary(t *testing.T) {
	var evilHits atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { evilHits.Add(1) }))
	defer evil.Close()
	r := newAdvRig(t, advOpt{secToken: "TOKEN-SEC-S2"})
	r.sec.set(func(f *advFake) {
		f.gen = func(w http.ResponseWriter, rq *http.Request, body string) {
			w.Header().Set("Set-Cookie", "mcpgw_session=forjado; Path=/monitor")
			http.Redirect(w, rq, evil.URL+"/roubar", http.StatusTemporaryRedirect)
		}
	})
	r.primaryDown()
	code, hdr := r.post(t, "/api/generate", `{"model":"qwen3-ia","prompt":"segredo do cliente"}`, nil)
	if r.sec.hit("/api/generate") != 1 {
		t.Fatalf("o pedido nao chegou ao secundario (%d)", r.sec.hit("/api/generate"))
	}
	// a checagem ativa tambem nao segue redirect
	r.sec.set(func(f *advFake) {
		f.health = func(w http.ResponseWriter, rq *http.Request) bool {
			http.Redirect(w, rq, evil.URL+"/api/version", http.StatusFound)
			return true
		}
	})
	r.pl.CheckOnce(context.Background(), r.pl.Backend("sec-x"))
	st := r.status("sec-x")
	if code >= 300 && code < 400 || hdr.Get("Location") != "" || hdr.Get("Set-Cookie") != "" || evilHits.Load() != 0 || st.State != "fora" {
		vec5(t, "S2-redirect-secundario", "Baixa", "gateway/proxy/proxy.go:187", fmt.Sprintf("FALHA: cliente recebeu HTTP %d Location=%q Set-Cookie=%q; evil=%d; checagem com 302 -> %s", code, hdr.Get("Location"), hdr.Get("Set-Cookie"), evilHits.Load(), st.State))
		t.Fail()
		return
	}
	vec5(t, "S2-redirect-secundario", "Baixa", "gateway/proxy/proxy.go:187", fmt.Sprintf("OK: 3xx do secundario vira HTTP %d sem Location/Set-Cookie; nenhum pedido ao destino do redirect; checagem com 302 -> %s", code, st.State))
}

// S3: o arquivo (e o token_file) trocado depois da partida nao muda nada ate o
// proximo reinicio (nao ha recarga a quente).
func TestAdv5S3RegistrySwappedAfterStart(t *testing.T) {
	dir := t.TempDir()
	tok := filepath.Join(dir, "t")
	os.WriteFile(tok, []byte("TOKEN-ORIGINAL"), 0o600)
	reg := filepath.Join(dir, "b.json")
	os.WriteFile(reg, []byte(fmt.Sprintf(`{"backends":[{"id":"s1","url":"https://10.0.0.5:11434","tipo":"gpu","token_file":%q}]}`, filepath.ToSlash(tok))), 0o600)
	specs, err := pool.LoadFile(reg)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := pool.Build(specs, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(reg, []byte(`{"backends":[{"id":"s1","url":"http://203.0.113.9:80","tipo":"gpu"}]}`), 0o600)
	os.WriteFile(tok, []byte("TOKEN-TROCADO"), 0o600)
	if bs[0].Host() != "10.0.0.5:11434" || bs[0].Token() != "TOKEN-ORIGINAL" {
		vec5(t, "S3-registro-trocado", "Baixa", "gateway/pool/registry.go:305", "FALHA: a troca do arquivo depois da partida mudou o backend")
		t.Fail()
		return
	}
	vec5(t, "S3-registro-trocado", "Baixa", "gateway/pool/registry.go:305", "OK: URL e token lidos uma vez na partida; trocar os arquivos depois nao muda o pool (sem recarga a quente)")
}

// ======================================================================================
// 2. Token do secundario e do cliente
// ======================================================================================

// T2: um secundario que ecoa o Authorization no "version" (ou no nome do
// modelo) poria o token no /monitor/api/state, que e aberto.
func TestAdv5T2EchoedTokenNeverInTheBoard(t *testing.T) {
	const tok = "TOKEN-SEC-ECO-77aa"
	r := newAdvRig(t, advOpt{secToken: tok})
	r.sec.set(func(f *advFake) {
		f.health = func(w http.ResponseWriter, rq *http.Request) bool {
			a := rq.Header.Get("Authorization")
			switch rq.URL.Path {
			case "/api/version":
				fmt.Fprintf(w, `{"version":%q}`, a+" <img src=x onerror=alert(1)>")
			case "/api/ps":
				fmt.Fprintf(w, `{"models":[{"name":%q,"model":"m","size":1,"size_vram":1,"digest":"sha256-aaaa"}]}`, a+strings.Repeat("A", 5000))
			default:
				return false
			}
			return true
		}
	})
	r.pl.CheckOnce(context.Background(), r.pl.Backend("sec-x"))
	st := r.status("sec-x")
	blob := fmt.Sprintf("%+v", r.q.PoolStatus())
	if strings.Contains(blob, tok) || len(st.Model) > 200 || len(st.Version) > 64 {
		vec5(t, "T2-token-ecoado-no-quadro", "Baixa", "gateway/pool/pool.go:691", fmt.Sprintf("FALHA: token no quadro=%v; version=%d bytes; model=%d bytes", strings.Contains(blob, tok), len(st.Version), len(st.Model)))
		t.Fail()
		return
	}
	vec5(t, "T2-token-ecoado-no-quadro", "Baixa", "gateway/pool/pool.go:691", fmt.Sprintf("OK: o texto livre do secundario (version %q, modelo de %d bytes) e limitado e o token sai redigido", st.Version, len(st.Model)))
}

// T3: o token do cliente nunca chega a um backend: Authorization em minusculas,
// duplicado e como TRAILER de corpo chunked; em /api/* e /v1/*. E o token do
// secundario nunca vai para o primario.
func TestAdv5T3ClientTokenNeverReachesABackend(t *testing.T) {
	const secTok = "TOKEN-SEC-T3-55bb"
	r := newAdvRig(t, advOpt{secToken: secTok})
	raw := func(path string) string {
		c, err := net.Dial("tcp", strings.TrimPrefix(r.gw.URL, "http://"))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		body := `{"model":"qwen3-ia","prompt":"x","messages":[{"role":"user","content":"x"}]}`
		req := "POST " + path + " HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\n" +
			"authorization: Bearer CLIENTE-MINUSCULA\r\nAuthorization: Bearer CLIENTE-DUP1\r\nAuthorization: Bearer CLIENTE-DUP2\r\n" +
			"Proxy-Authorization: Basic CLIENTE-PROXY\r\nCookie: mcpgw_session=CLIENTE-COOKIE\r\n" +
			"Transfer-Encoding: chunked\r\nTrailer: Authorization\r\nConnection: close\r\n\r\n" +
			fmt.Sprintf("%x\r\n%s\r\n0\r\nAuthorization: Bearer CLIENTE-TRAILER\r\n\r\n", len(body), body)
		c.Write([]byte(req))
		st, _ := bufio.NewReader(c).ReadString('\n')
		return strings.TrimSpace(st)
	}
	var lines []string
	for _, p := range []string{"/api/generate", "/v1/chat/completions"} {
		lines = append(lines, p+" primario: "+raw(p))
	}
	r.primaryDown()
	for _, p := range []string{"/api/generate", "/api/chat", "/v1/chat/completions"} {
		lines = append(lines, p+" secundario: "+raw(p))
	}
	var leaks []string
	for _, h := range append(r.prim.headers(), r.sec.headers()...) {
		if strings.Contains(h, "CLIENTE-") {
			leaks = append(leaks, h)
		}
	}
	for _, h := range r.prim.headers() {
		if strings.Contains(h, secTok) {
			leaks = append(leaks, "token do secundario no primario: "+h)
		}
	}
	secOK := 0
	for _, h := range r.sec.headers() {
		if strings.Contains(h, "Authorization=Bearer "+secTok+";") {
			secOK++
		}
	}
	if len(leaks) > 0 || r.sec.hit("/api/generate")+r.sec.hit("/api/chat")+r.sec.hit("/v1/chat/completions") != 3 || secOK != 3 {
		vec5(t, "T3-token-do-cliente", "Alta", "gateway/proxy/proxy.go:176", fmt.Sprintf("FALHA: vazamentos=%v; secundario com token proprio=%d; %v", leaks, secOK, lines))
		t.Fail()
		return
	}
	vec5(t, "T3-token-do-cliente", "Alta", "gateway/proxy/proxy.go:176", fmt.Sprintf("OK: minusculo, duplicado, Proxy-Authorization, Cookie e trailer chunked nunca chegam a backend algum (5 pedidos: %d no primario, 3 no secundario, todos so com o token proprio); o primario nunca viu o token do secundario", len(r.prim.headers())))
}

// S4 (so Linux): modo real do backends.json e do token_file, symlink para um
// arquivo 0644, e o mesmo arquivo conferido na leitura (TOCTOU, por inspecao:
// readSecret compara os.SameFile do nome com o descritor aberto).
func TestAdv5S4FileModesLinux(t *testing.T) {
	if runtime.GOOS == "windows" {
		vec5(t, "S4-modo-symlink-dono", "Baixa", "gateway/pool/registry.go:149", "N/A no Windows: rodar no Linux do servidor (pendencia); dono do arquivo nao e conferido (igual ao --admin-file)")
		t.Skip("sem modo POSIX")
	}
	dir := t.TempDir()
	js := `{"backends":[{"id":"s1","url":"http://10.0.0.5:11434","tipo":"gpu"}]}`
	open := filepath.Join(dir, "aberto.json")
	os.WriteFile(open, []byte(js), 0o644)
	os.Chmod(open, 0o644)
	link := filepath.Join(dir, "link.json")
	os.Symlink(open, link)
	g := filepath.Join(dir, "grupo.json")
	os.WriteFile(g, []byte(js), 0o640)
	os.Chmod(g, 0o640)
	var bad []string
	for _, p := range []string{open, link, g} {
		if _, err := pool.LoadFile(p); err == nil {
			bad = append(bad, filepath.Base(p))
		}
	}
	tok := filepath.Join(dir, "tok")
	os.WriteFile(tok, []byte("x"), 0o644)
	os.Chmod(tok, 0o644)
	if _, err := pool.Build([]pool.Spec{{ID: "s1", URL: "https://10.0.0.5:1", Tipo: "gpu", TokenFile: tok}}, nil, "", time.Second, nil); err == nil {
		bad = append(bad, "token_file 0644")
	}
	if len(bad) > 0 {
		vec5(t, "S4-modo-symlink-dono", "Baixa", "gateway/pool/registry.go:149", "FALHA: aceitos "+strings.Join(bad, ", "))
		t.Fail()
		return
	}
	vec5(t, "S4-modo-symlink-dono", "Baixa", "gateway/pool/registry.go:149", "OK: 0644, 0640, symlink para 0644 e token_file 0644 recusados")
}
