package pool_test

// Regressões da revisão adversarial 10.5 (pool de backends). Cada teste falha
// no código da 10.4 (ed57add) e passa depois da correção.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

// B1 (Média): o disjuntor de um secundário abria e nunca mais fechava, porque
// um secundário meio-aberto não é elegível e por isso nunca recebia a prova.
func TestRegress105SecondaryBreakerClosesAfterTheActiveCheck(t *testing.T) {
	r := newRig(t, rigCfg{PrimarySlots: 1, OverloadQueue: 1, BreakerOpen: 30 * time.Millisecond, Secs: []secCfg{{ID: "sec-gpu", Tipo: "gpu", Slots: 1}}})
	sec := r.pl.Backend("sec-gpu")
	r.secs["sec-gpu"].set(func(f *fake) { f.version = 500 })
	for i := 0; i < 3; i++ {
		r.pl.CheckOnce(context.Background(), sec)
	}
	if st := r.q.PoolStatus()[1]; st.Breaker != "aberto" {
		t.Fatalf("setup: %+v", st)
	}
	r.secs["sec-gpu"].set(func(f *fake) { f.version = 200 })
	r.pl.CheckOnce(context.Background(), sec) // ok, mas ainda dentro do tempo aberto
	if st := r.q.PoolStatus()[1]; st.Breaker != "aberto" {
		t.Fatalf("o tempo aberto deve ser respeitado: %+v", st)
	}
	time.Sleep(50 * time.Millisecond)
	r.pl.CheckOnce(context.Background(), sec)
	if st := r.q.PoolStatus()[1]; st.Breaker != "fechado" || !r.evs.has("breaker_close") {
		t.Fatalf("a checagem ativa ok depois do tempo aberto deve fechar: %+v", st)
	}
	a := r.submit(queue.Request{}).started(t)
	defer a.Release(queue.Result{})
	b := r.submit(queue.Request{})
	c := r.submit(queue.Request{}) // 2 esperando > 1: S2; a justiça escolhe b ou c
	waitFor(t, "repasse ao secundário", func() bool {
		for _, j := range []*job{b, c} {
			if j.isStarted() && j.err == nil && j.tk.Backend() == "sec-gpu" {
				return true
			}
		}
		return false
	})
}

// S1 (Baixa): url.Parse só confere que a porta é feita de dígitos.
func TestRegress105RegistryPortRange(t *testing.T) {
	dir := t.TempDir()
	for i, u := range []string{"http://10.0.0.5:0", "http://10.0.0.5:65536", "http://10.0.0.5:99999", "http://10.0.0.5:11434?"} {
		p := filepath.Join(dir, fmt.Sprintf("b%d.json", i))
		os.WriteFile(p, []byte(fmt.Sprintf(`{"backends":[{"id":"s1","url":%q,"tipo":"gpu"}]}`, u)), 0o600)
		if _, err := pool.LoadFile(p); err == nil {
			t.Errorf("%s aceita", u)
		}
	}
	p := filepath.Join(dir, "ok.json")
	os.WriteFile(p, []byte(`{"backends":[{"id":"s1","url":"http://10.0.0.5:65535","tipo":"gpu"}]}`), 0o600)
	if _, err := pool.LoadFile(p); err != nil {
		t.Fatal(err)
	}
}

// T2 (Baixa): texto livre do secundário (version, nome do modelo) ia cru para o
// estado aberto do monitor, inclusive o próprio token se ele o ecoasse.
func TestRegress105EchoedTokenIsScrubbed(t *testing.T) {
	const tok = "TOKEN-REGRESS-T2"
	sec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprintf(w, `{"version":%q}`, a+"<b>")
		case "/api/ps":
			fmt.Fprintf(w, `{"models":[{"name":%q,"size":1,"size_vram":1,"digest":"d"}]}`, a+strings.Repeat("x", 3000))
		default:
			fmt.Fprint(w, `{"models":[]}`)
		}
	}))
	defer sec.Close()
	p := filepath.Join(t.TempDir(), "t")
	os.WriteFile(p, []byte(tok), 0o600)
	bs, err := pool.Build([]pool.Spec{{ID: "s", URL: sec.URL, Tipo: "gpu", TokenFile: p}}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	pl := pool.New(pool.Config{PrimarySlots: 1, Secondaries: bs})
	pl.CheckOnce(context.Background(), bs[0])
	st := pl.Status(time.Now(), nil)[1]
	if blob := fmt.Sprintf("%+v", st); strings.Contains(blob, tok) || len(st.Model) > 96 || len(st.Version) > 32 || strings.ContainsAny(st.Version, "<> ") {
		t.Fatalf("status %+v", st)
	}
}

// N1 (Média, não regressão): a checagem do primário é também a leitura do quadro
// "Modelos"; com o Ollama fora, o quadro precisa mostrar o erro.
func TestRegress105ModelsReadingRunsWhenTheOllamaIsDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"1","models":[{"name":"qwen3-ia","size":1,"size_vram":1}]}`)
	}))
	u, _ := url.Parse(srv.URL)
	col := &models.Collector{Upstream: u}
	pl := pool.New(pool.Config{PrimarySlots: 1, PrimaryURL: u, PrimaryCollector: col})
	pl.CheckOnce(context.Background(), pl.Primary())
	if len(col.Latest().Loaded) != 1 {
		t.Fatalf("setup: %+v", col.Latest())
	}
	srv.Close()
	pl.CheckOnce(context.Background(), pl.Primary())
	if l := col.Latest(); l.Err == "" || len(l.Loaded) != 0 {
		t.Fatalf("o quadro Modelos ficou parado com o Ollama fora: %+v", l)
	}
}

// B3 (Baixa) e S2 (Baixa), pelo proxy de verdade.
func TestRegress105ProxyDropAfterConnectAndRedirect(t *testing.T) {
	var primDown atomic.Bool
	health := func(w http.ResponseWriter, rq *http.Request) bool {
		switch rq.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"version":"0.40.8"}`)
		case "/api/ps", "/api/tags":
			fmt.Fprint(w, `{"models":[{"name":"qwen3-ia","size":1,"size_vram":1,"digest":"sha256-r"}]}`)
		default:
			return false
		}
		return true
	}
	prim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
		if primDown.Load() && rq.URL.Path == "/api/version" {
			http.Error(w, "down", 500)
			return
		}
		if health(w, rq) {
			return
		}
		io.Copy(io.Discard, rq.Body)
		c, _, _ := w.(http.Hijacker).Hijack() // a conexão foi feita e cai antes da resposta
		c.Close()
	}))
	defer prim.Close()
	sec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
		if health(w, rq) {
			return
		}
		w.Header().Set("Set-Cookie", "mcpgw_session=x")
		http.Redirect(w, rq, "http://127.0.0.1:1/x", http.StatusTemporaryRedirect)
	}))
	defer sec.Close()
	bs, err := pool.Build([]pool.Spec{{ID: "sec-gpu", URL: sec.URL, Tipo: "gpu", Slots: 2}}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	pu, _ := url.Parse(prim.URL)
	pl := pool.New(pool.Config{PrimarySlots: 1, PrimaryURL: pu, Secondaries: bs, QueueMax: 8, OverloadQueue: 6})
	var mu sync.Mutex
	var recs []queue.Record
	q := queue.New(queue.Config{Pool: pl, QueueMax: 8, OnRecord: func(rec queue.Record) { mu.Lock(); recs = append(recs, rec); mu.Unlock() }})
	nrecs := func() []queue.Record { mu.Lock(); defer mu.Unlock(); return append([]queue.Record(nil), recs...) }
	for _, b := range pl.Backends() {
		pl.CheckOnce(context.Background(), b)
	}
	px, err := proxy.New(proxy.Config{Upstream: pu, Queue: q, Pool: pl})
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(px)
	defer gw.Close()
	post := func() *http.Response {
		cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := cl.Post(gw.URL+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"x"}`))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp
	}
	for i := 0; i < 4; i++ {
		if resp := post(); resp.StatusCode != 502 {
			t.Fatalf("status %d", resp.StatusCode)
		}
	}
	if st := q.PoolStatus()[0]; st.Breaker != "fechado" {
		t.Fatalf("corte depois da conexão abriu o disjuntor: %+v", st)
	}
	waitFor(t, "registros", func() bool { return len(nrecs()) == 4 })
	if rec := nrecs()[0]; rec.ErrorClass != "upstream_unreachable" {
		t.Fatalf("a classe do registro mudou: %+v", rec)
	}
	primDown.Store(true)
	pl.CheckOnce(context.Background(), pl.Primary()) // S3: vai ao secundário
	resp := post()
	if resp.StatusCode != 502 || resp.Header.Get("Location") != "" || resp.Header.Get("Set-Cookie") != "" {
		t.Fatalf("redirect do secundário repassado: %d %v", resp.StatusCode, resp.Header)
	}
	waitFor(t, "registro do redirect", func() bool { return len(nrecs()) == 5 })
	if rec := nrecs()[4]; rec.ErrorClass != "upstream_redirect" || rec.Backend != "sec-gpu" {
		t.Fatalf("registro %+v", rec)
	}
	if st := q.PoolStatus()[1]; st.Breaker != "fechado" {
		t.Fatalf("o redirect é neutro para o disjuntor: %+v", st)
	}
}
