package ollama

// Fase 10.5 - revisao adversarial do fallback do mcpollama (10.4).
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado e FALHA quando o
// defeito existe.

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func vec5o(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// rawServer aceita a conexao, le o pedido inteiro e responde com bytes crus
// (resp == "" fecha sem responder).
func rawServer(t *testing.T, resp string) (string, *atomic.Int32) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	var n atomic.Int32
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				buf := make([]byte, 1<<16)
				for {
					if _, err := req.Body.Read(buf); err != nil {
						break
					}
				}
				n.Add(1)
				if resp != "" {
					c.Write([]byte(resp))
				}
			}(c)
		}
	}()
	return "http://" + l.Addr().String(), &n
}

// FB1: o fallback so pode disparar quando o primario NAO viu o pedido. Uma
// resposta malformada (ou um GOAWAY/erro de stream do HTTP/2) chega depois de o
// primario ler o pedido inteiro, mas cai em ErrConnect: o mesmo trabalho roda
// duas vezes (custo dobrado; e o principio "a chamada nao foi repetida" quebra).
func TestAdv5FB1NoFallbackAfterThePrimarySawTheRequest(t *testing.T) {
	prim, primN := rawServer(t, "ISTO-NAO-E-HTTP\r\n\r\n")
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	c, err := New(Options{BaseURL: prim, Model: "qwen3-ia", FallbackURL: fb.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, gerr := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	if primN.Load() == 1 && fbN.Load() > 0 {
		vec5o(t, "FB1-fallback-duplica", "Baixa", "ollama/api.go:323", fmt.Sprintf("FALHA: o primario leu o pedido (%d) e o fallback rodou de novo (%d); erro=%v", primN.Load(), fbN.Load(), gerr))
		t.Fail()
		return
	}
	// mas a recusa de conexao continua indo ao fallback
	c2, _ := New(Options{BaseURL: deadURL(t), Model: "qwen3-ia", FallbackURL: fb.URL})
	defer c2.Close()
	got, err := c2.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	if err != nil || got.Backend != "fallback" {
		vec5o(t, "FB1-fallback-duplica", "Baixa", "ollama/api.go:323", fmt.Sprintf("FALHA: a correcao parou o fallback na recusa de conexao: %v", err))
		t.Fail()
		return
	}
	vec5o(t, "FB1-fallback-duplica", "Baixa", "ollama/api.go:323", fmt.Sprintf("OK: resposta malformada depois de o primario ler o pedido -> sem fallback (%v); recusa de conexao -> fallback", kindOf(gerr)))
}

func kindOf(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Kind.Error()
	}
	if err == nil {
		return "nil"
	}
	return err.Error()
}

// FB2: 4xx, 5xx, erro de modelo, 503 sem Retry-After, TLS e timeout nunca
// disparam o fallback.
func TestAdv5FB2OnlyConnectionAnd503(t *testing.T) {
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	var out []string
	for _, st := range []int{400, 401, 403, 404, 409, 413, 429, 500, 502, 503, 504} {
		st := st
		prim, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, `{"error":"model 'x' not found"}`, st) })
		c, _ := New(Options{BaseURL: prim.URL, Model: "qwen3-ia", FallbackURL: fb.URL})
		c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
		c.Close()
		out = append(out, fmt.Sprint(st))
	}
	// erro do modelo dentro do stream (200 + {"error":...})
	prim, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"error":"model runner crashed"}`+"\n") })
	c, _ := New(Options{BaseURL: prim.URL, Model: "qwen3-ia", FallbackURL: fb.URL})
	c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	c.Close()
	// timeout do primario
	slow, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(400 * time.Millisecond) })
	c, _ = New(Options{BaseURL: slow.URL, Model: "qwen3-ia", Timeout: 100 * time.Millisecond, FallbackURL: fb.URL})
	c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	c.Close()
	// corte no meio do stream
	cut, _ := rawServer(t, "HTTP/1.1 200 OK\r\nContent-Type: application/x-ndjson\r\nContent-Length: 999\r\n\r\n{\"response\":\"me")
	c, _ = New(Options{BaseURL: cut, Model: "qwen3-ia", FallbackURL: fb.URL})
	c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	c.Close()
	if fbN.Load() != 0 {
		vec5o(t, "FB2-fallback-so-conexao-503", "Media", "ollama/api.go:392", fmt.Sprintf("FALHA: o fallback recebeu %d pedido(s)", fbN.Load()))
		t.Fail()
		return
	}
	vec5o(t, "FB2-fallback-so-conexao-503", "Media", "ollama/api.go:392", "OK: HTTP "+strings.Join(out, "/")+", erro no stream, timeout e corte no meio -> 0 pedidos ao fallback")
}

// FB3: sem laco; o fallback e tentado uma vez por chamada, inclusive quando ele
// tambem responde 503 de fila cheia; tokens separados nos dois sentidos.
func TestAdv5FB3NoLoopAndTokens(t *testing.T) {
	var primAuth, fbAuth atomic.Value
	prim, primN := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		primAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":"fila cheia"}`, 503)
	})
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		fbAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":"fila cheia"}`, 503)
	})
	c, err := New(Options{BaseURL: prim.URL, APIKey: token, Model: "qwen3-ia", FallbackURL: fb.URL, FallbackAPIKey: fbToken})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, gerr := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	pa, _ := primAuth.Load().(string)
	fa, _ := fbAuth.Load().(string)
	// fallback sem token proprio: nada de Authorization (o do primario nunca vai)
	var fb2Auth atomic.Value
	fb2Auth.Store("<nenhum pedido>")
	fb2, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		fb2Auth.Store(r.Header.Get("Authorization"))
		fmt.Fprint(w, genOK)
	})
	c2, _ := New(Options{BaseURL: deadURL(t), APIKey: token, Model: "qwen3-ia", FallbackURL: fb2.URL})
	defer c2.Close()
	c2.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	f2, _ := fb2Auth.Load().(string)
	if primN.Load() != 2 || fbN.Load() != 1 || pa != "Bearer "+token || fa != "Bearer "+fbToken || f2 != "" {
		vec5o(t, "FB3-laco-e-tokens", "Alta", "ollama/client.go:158", fmt.Sprintf("FALHA: primario=%d fallback=%d auth primario=%q fallback=%q fallback-sem-token=%q err=%v", primN.Load(), fbN.Load(), pa, fa, f2, kindOf(gerr)))
		t.Fail()
		return
	}
	vec5o(t, "FB3-laco-e-tokens", "Alta", "ollama/client.go:158", fmt.Sprintf("OK: 503+503 -> primario 2 (1 nova tentativa), fallback 1, erro %s; cada um so com o proprio token; fallback sem token nao recebe Authorization", kindOf(gerr)))
}

// FB4: o tempo total (--timeout) vale para primario + fallback.
func TestAdv5FB4TotalTimeout(t *testing.T) {
	hang, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c, _ := New(Options{BaseURL: deadURL(t), Model: "qwen3-ia", Timeout: 300 * time.Millisecond, FallbackURL: hang.URL})
	defer c.Close()
	t0 := time.Now()
	_, err := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"})
	d := time.Since(t0)
	if d > 1500*time.Millisecond || err == nil {
		vec5o(t, "FB4-timeout-total", "Media", "ollama/api.go:115", fmt.Sprintf("FALHA: %s, err=%v", d, err))
		t.Fail()
		return
	}
	vec5o(t, "FB4-timeout-total", "Media", "ollama/api.go:115", fmt.Sprintf("OK: primario fora + fallback mudo -> %s com --timeout 300ms (%s)", d.Round(10*time.Millisecond), kindOf(err)))
}
