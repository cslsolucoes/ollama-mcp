package ollama

// Regressão da revisão adversarial 10.5 (fallback). Falha no código da 10.4
// (ed57add) e passa depois da correção.

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
)

// FB1 (Baixa): o fallback disparava em qualquer ErrConnect, inclusive quando o
// primário já tinha lido o pedido inteiro (resposta malformada, GOAWAY do
// HTTP/2): o mesmo trabalho rodava duas vezes.
func TestRegress105NoFallbackOnceThePrimaryReadTheRequest(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var primN atomic.Int32
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				req, err := http.ReadRequest(bufio.NewReader(c))
				if err != nil {
					return
				}
				buf := make([]byte, 4096)
				for {
					if _, err := req.Body.Read(buf); err != nil {
						break
					}
				}
				primN.Add(1)
				c.Write([]byte("ISTO-NAO-E-HTTP\r\n\r\n"))
			}(c)
		}
	}()
	fb, fbN := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, genOK) })
	c, err := New(Options{BaseURL: "http://" + l.Addr().String(), Model: "qwen3-ia", FallbackURL: fb.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Generate(context.Background(), GenerateRequest{Prompt: "oi"}); err == nil {
		t.Fatal("a resposta malformada não deu erro")
	}
	if primN.Load() != 1 || fbN.Load() != 0 {
		t.Fatalf("primário leu %d, fallback rodou %d (deve ser 1 e 0)", primN.Load(), fbN.Load())
	}
	// a recusa de conexão continua indo ao fallback
	c2, _ := New(Options{BaseURL: deadURL(t), Model: "qwen3-ia", FallbackURL: fb.URL})
	defer c2.Close()
	if got, err := c2.Generate(context.Background(), GenerateRequest{Prompt: "oi"}); err != nil || got.Backend != "fallback" {
		t.Fatalf("recusa de conexão: %v %+v", err, got)
	}
}
