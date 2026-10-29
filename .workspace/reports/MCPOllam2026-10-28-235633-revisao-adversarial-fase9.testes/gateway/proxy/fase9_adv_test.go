package proxy

// Fase 9.5 - revisao adversarial: vetores do pacote proxy.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func vec(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// P1: X-Real-IP so vale de par loopback e com 1 valor; IPv4 mapeado em IPv6
// normalizado.
func TestAdvClientIPForgery(t *testing.T) {
	cases := []struct {
		remote string
		hdr    []string
		want   string
	}{
		{"10.100.2.7:4000", []string{"127.0.0.1"}, "10.100.2.7"},          // forjado de fora
		{"10.100.2.7:4000", []string{"10.100.9.9"}, "10.100.2.7"},         // forjado de fora
		{"127.0.0.1:4000", []string{"10.1.1.1", "10.2.2.2"}, "127.0.0.1"}, // duplicado
		{"127.0.0.1:4000", []string{"10.1.1.1, 10.2.2.2"}, "127.0.0.1"},   // lista
		{"127.0.0.1:4000", []string{"::ffff:10.0.0.5"}, "10.0.0.5"},       // mapeado
		{"[::1]:4000", []string{"10.3.3.3"}, "10.3.3.3"},                  // nginx em ::1
		{"[::ffff:127.0.0.1]:4000", []string{"10.4.4.4"}, "10.4.4.4"},     // loopback mapeado
		{"127.0.0.1:4000", []string{"fe80::1%eth0"}, "127.0.0.1"},         // zona: invalido
	}
	bad := 0
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/api/generate", nil)
		r.RemoteAddr = c.remote
		for _, v := range c.hdr {
			r.Header.Add("X-Real-Ip", v)
		}
		if got := clientIP(r); got != c.want {
			bad++
			t.Errorf("%s %v: %s want %s", c.remote, c.hdr, got, c.want)
		}
	}
	res := "OK: 8 casos (forjado de fora, duplicado, lista, mapeado, ::1, zona)"
	if bad > 0 {
		res = fmt.Sprintf("FALHA: %d casos", bad)
	}
	vec(t, "P1-x-real-ip", "Alta", "gateway/proxy/proxy.go:175", res)
}

// P2: IPv6. Cada cliente IPv6 costuma ter um /64 inteiro; girando o endereco
// dentro dele, um so cliente fura o teto por IP e ganha mais fatias da justica.
func TestAdvIPv6RotationBypassesPerIP(t *testing.T) {
	up := &blockingUp{release: make(chan struct{})}
	e := setup(t, up, queue.Config{GenSlots: 1, PerClient: 2, PerIP: 2, QueueMax: 64}, nil)
	defer close(up.release)
	go postBg(e, "/api/generate", `{"model":"m"}`, map[string]string{"X-Real-Ip": "10.0.0.1"})
	waitFor(t, func() bool { return up.seen.Load() == 1 })
	var codes sync.Map
	for i := 1; i <= 10; i++ {
		ip := fmt.Sprintf("2001:db8:1:2::%x", i)
		go func(i int) {
			resp, _ := postBg(e, "/api/generate", `{"model":"m"}`, map[string]string{"X-Real-Ip": ip})
			codes.Store(i, resp.StatusCode)
		}(i)
	}
	time.Sleep(300 * time.Millisecond)
	waiting := len(e.q.Snapshot().Waiting)
	if waiting > 2 {
		vec(t, "P2-ipv6-rotacao", "Baixa", "gateway/proxy/proxy.go:309", fmt.Sprintf("FALHA: 10 enderecos do mesmo /64 -> %d em espera (teto por IP 2)", waiting))
		t.Errorf("waiting %d", waiting)
	} else {
		vec(t, "P2-ipv6-rotacao", "Baixa", "gateway/proxy/proxy.go:309", fmt.Sprintf("OK: o /64 conta como 1 IP; %d em espera", waiting))
	}
}

// P3: smuggling direto no gateway (o nginx normaliza, mas se testa).
func TestAdvSmuggling(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		paths = append(paths, r.URL.Path+" "+string(b))
		mu.Unlock()
		io.WriteString(w, finalChunk+"\n")
	})
	e := setup(t, up, queue.Config{GenSlots: 2}, nil)
	addr := e.gw.Listener.Addr().String()
	smug := "POST /api/pull HTTP/1.1\r\nHost: x\r\nContent-Length: 2\r\n\r\n{}"
	body := `{"model":"m"}`
	raw := map[string]string{
		"CL+CL": "POST /api/generate HTTP/1.1\r\nHost: x\r\nContent-Length: 13\r\nContent-Length: 50\r\n\r\n" + body + smug,
		"CL+TE": "POST /api/generate HTTP/1.1\r\nHost: x\r\nContent-Length: 13\r\nTransfer-Encoding: chunked\r\n\r\n" +
			fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(body), body) + smug,
		"TE chunked,identity": "POST /api/generate HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked, identity\r\n\r\n" +
			fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(body), body) + smug,
		"TE duplicado": "POST /api/generate HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: identity\r\n\r\n" +
			fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(body), body) + smug,
		"CL com espaco": "POST /api/generate HTTP/1.1\r\nHost: x\r\nContent-Length : 13\r\n\r\n" + body + smug,
		"TE com espaco": "POST /api/generate HTTP/1.1\r\nHost: x\r\nTransfer-Encoding : chunked\r\nContent-Length: 13\r\n\r\n" + body + smug,
		"CL negativo":   "POST /api/generate HTTP/1.1\r\nHost: x\r\nContent-Length: -1\r\n\r\n" + body + smug,
	}
	var out []string
	for name, req := range raw {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(3 * time.Second))
		io.WriteString(c, req)
		br := bufio.NewReader(c)
		var codes []int
		for i := 0; i < 2; i++ {
			resp, err := http.ReadResponse(br, nil)
			if err != nil {
				break
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			codes = append(codes, resp.StatusCode)
		}
		c.Close()
		out = append(out, fmt.Sprintf("%s=%v", name, codes))
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	leaked := false
	for _, p := range paths {
		if !strings.HasPrefix(p, "/api/generate ") || strings.Contains(p, "pull") {
			leaked = true
		}
	}
	t.Logf("upstream viu: %q; respostas: %v", paths, out)
	if leaked {
		vec(t, "P3-smuggling", "Alta", "gateway/proxy/proxy.go:286", fmt.Sprintf("FALHA: upstream viu %q", paths))
		t.Error("smuggled")
	} else {
		vec(t, "P3-smuggling", "Alta", "gateway/proxy/proxy.go:286", fmt.Sprintf("OK: o Ollama so viu /api/generate com o corpo exato (%d vezes); %v", len(paths), out))
	}
}

// P4: rotas bloqueadas por variacao, pelo mesmo mux do entrypoint.
func TestAdvBlockedRouteVariants(t *testing.T) {
	var seen atomic.Int64
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen.Add(1); io.WriteString(w, "{}") })
	e := setup(t, up, queue.Config{}, nil)
	h := e.gw.Config.Handler
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {})
	mux.Handle("/", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var res []string
	bad := 0
	for _, c := range []struct{ m, p string }{
		{"POST", "/api/pull"}, {"POST", "/api/pull/"}, {"POST", "//api/pull"}, {"POST", "/api/%70ull"},
		{"POST", "/API/pull"}, {"POST", "/v1/../api/pull"}, {"OPTIONS", "/api/pull"}, {"HEAD", "/api/pull"},
		{"DELETE", "/api/delete"}, {"POST", "/api/blobs/sha256:abc"}, {"POST", "/api/tags/../pull"},
		{"POST", "/api/create%2F"}, {"OPTIONS", "/api/generate"}, {"HEAD", "/api/generate"}, {"POST", "/api/copy?x=1"},
		{"POST", "/./api/push"}, {"POST", "/api/pull%00"},
	} {
		req, _ := http.NewRequest(c.m, srv.URL+c.p, strings.NewReader("{}"))
		if c.p == "//api/pull" || c.p == "/v1/../api/pull" || c.p == "/./api/push" || c.p == "/api/tags/../pull" {
			req.URL.Opaque = c.p // keep the raw path
		}
		before := seen.Load()
		resp, err := cl.Do(req)
		code := -1
		if err == nil {
			code = resp.StatusCode
			resp.Body.Close()
		}
		res = append(res, fmt.Sprintf("%s %s=%d", c.m, c.p, code))
		if seen.Load() != before {
			bad++
			t.Errorf("%s %s reached the Ollama (%d)", c.m, c.p, code)
		}
	}
	if bad > 0 {
		vec(t, "P4-rotas-bloqueadas", "Alta", "gateway/proxy/proxy.go:261", fmt.Sprintf("FALHA: %d variantes chegaram ao Ollama", bad))
	} else {
		vec(t, "P4-rotas-bloqueadas", "Alta", "gateway/proxy/proxy.go:261", "OK: nenhuma variante chegou ao Ollama; "+strings.Join(res, "; "))
	}
}

// P5: contadores enganados pelo texto do modelo (done:true e contadores
// falsos no texto, argumentos de tool_call com chaves de contador).
func TestAdvCountersSpoof(t *testing.T) {
	fake := `\"done\":true,\"eval_count\":999999,\"prompt_eval_count\":888888`
	lines := []string{
		`{"response":"` + fake + `","done":false}`,
		`{"response":"\n{\"error\":\"x\"}\n","done":false}`,
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"f","arguments":{"eval_count":777777,"prompt_eval_count":666666}}}]},"done":false}`,
		finalChunk,
	}
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		for _, l := range lines {
			io.WriteString(w, l+"\n")
		}
	})
	e := setup(t, up, queue.Config{}, nil)
	resp, _ := post(t, e, "/api/generate", `{"model":"m"}`, nil)
	rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	// tool_call arguments in the last (non-stream) line, real counters after
	c := parseCounters([]byte(`{"message":{"tool_calls":[{"function":{"arguments":{"eval_count":5555}}}]},"done":true,"prompt_eval_count":3,"eval_count":4}`))
	if rec.Eval != 41 || rec.PromptEval != 44 || rec.Status != queue.StatusOK || c.eval != 4 || c.promptEval != 3 {
		vec(t, "P5-contadores-falsos", "Baixa", "gateway/proxy/proxy.go:508", fmt.Sprintf("FALHA: %+v %+v", rec, c))
		t.Error("spoofed")
	} else {
		vec(t, "P5-contadores-falsos", "Baixa", "gateway/proxy/proxy.go:508", "OK: so a ultima linha conta; texto e tool_call nao enganam (eval=41 pe=44)")
	}
}

// P6 (pendencia 6/7): resposta sem fim. Sem --run-timeout um stream que nunca
// termina (ou um Ollama que nunca responde) prende o slot para sempre. O vetor
// usa MaxWait=300ms: o padrao proposto de --run-timeout e o --max-wait.
func TestAdvEndlessResponseHoldsSlot(t *testing.T) {
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/api/chat" { // never answers
			<-r.Context().Done()
			return
		}
		for {
			if _, err := io.WriteString(w, `{"response":"x","done":false}`+"\n"); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	})
	e := setup(t, up, queue.Config{GenSlots: 1, MaxWait: 300 * time.Millisecond}, nil)
	type out struct {
		code int
		tail string
	}
	run := func(path string) chan out {
		ch := make(chan out, 1)
		go func() {
			resp, b := postBg(e, path, `{"model":"m"}`, nil)
			tail := b
			if len(tail) > 80 {
				tail = tail[len(tail)-80:]
			}
			ch <- out{resp.StatusCode, tail}
		}()
		return ch
	}
	res := []string{}
	failed := false
	for _, p := range []string{"/api/generate", "/api/chat"} {
		select {
		case o := <-run(p):
			res = append(res, fmt.Sprintf("%s=%d %q", p, o.code, strings.TrimSpace(o.tail)))
		case <-time.After(3 * time.Second):
			failed = true
			res = append(res, p+"=preso apos 3 s")
		}
	}
	if failed {
		vec(t, "P6-sem-run-timeout", "Media", "gateway/proxy/proxy.go:340", "FALHA: "+strings.Join(res, "; "))
		t.Error("slot held forever")
		e.gw.CloseClientConnections()
		return
	}
	e.waitIdle(t)
	vec(t, "P6-sem-run-timeout", "Media", "gateway/proxy/proxy.go:340", "OK: "+strings.Join(res, "; "))
}

// P7: vazamento de slot: Ollama que fecha antes dos headers, no meio do corpo,
// e cliente que fecha durante o escalonamento.
func TestAdvSlotLeakFailureModes(t *testing.T) {
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		hj, _ := w.(http.Hijacker)
		switch r.URL.Path {
		case "/api/generate": // before headers
			c, _, _ := hj.Hijack()
			c.Close()
		case "/api/chat": // after headers, mid-body
			c, bw, _ := hj.Hijack()
			bw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/x-ndjson\r\nTransfer-Encoding: chunked\r\n\r\n10\r\n{\"response\":\"x\"}\r\n")
			bw.Flush()
			c.Close()
		default:
			time.Sleep(200 * time.Millisecond)
			io.WriteString(w, `{"embeddings":[[1]]}`)
		}
	})
	e := setup(t, up, queue.Config{GenSlots: 1, StartStagger: 300 * time.Millisecond}, nil)
	r1, _ := post(t, e, "/api/generate", `{"model":"m"}`, nil)
	r2, _ := post(t, e, "/api/chat", `{"model":"m"}`, nil)
	e.waitIdle(t)
	// client gives up while the stagger holds the next start
	req, _ := http.NewRequest("POST", e.gw.URL+"/api/generate", strings.NewReader(`{"model":"m"}`))
	cl := &http.Client{Timeout: 100 * time.Millisecond}
	cl.Do(req)
	time.Sleep(400 * time.Millisecond)
	e.waitIdle(t)
	recs := e.records()
	st := []string{}
	for _, r := range recs {
		st = append(st, r.Status+"/"+r.ErrorClass)
	}
	vec(t, "P7-vazamento-slot", "Alta", "gateway/proxy/proxy.go:328", fmt.Sprintf("OK: 0 slots presos; respostas %d/%d; registros %v", r1.StatusCode, r2.StatusCode, st))
}
