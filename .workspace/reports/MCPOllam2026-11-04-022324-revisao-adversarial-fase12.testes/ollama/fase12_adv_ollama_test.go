package ollama

// Revisão adversarial da etapa 12.9 (Fase 12), revisor B: Client.Feedback (C-1.11,
// C-1.20a) e o fallback do cliente. Cada vetor imprime uma linha VEC. Depende de
// client_test.go (newMock, newClient, token) e feedback_test.go (fbReport, fbTok).

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func vec12B(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res != "OK" {
		t.Errorf("%s: %s", id, desc)
	}
}

const adv12BOK = `{"ocorrencia_id":"44444444444444444444444444444444","estado":"nova"}`

func adv12BClosedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a := l.Addr().String()
	l.Close()
	return a
}

func TestAdv12BFeedbackNeverFallsBackNorRetries(t *testing.T) {
	var bad []string
	fbSrv, fbHits := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, adv12BOK) })
	// (a) the primary cannot be reached: the generation path would try the fallback once; a report must not
	c := newClient(t, fbSrv, func(o *Options) {
		o.BaseURL = "http://" + adv12BClosedPort(t)
		o.FallbackURL, o.FallbackAPIKey = fbSrv.URL, fbTok
	})
	if _, err := c.Feedback(context.Background(), fbReport(), BackendPrimary); err == nil || fbHits.Load() != 0 {
		bad = append(bad, fmt.Sprintf("primário fora: err=%v, fallback recebeu %d", err, fbHits.Load()))
	}
	// (b) 503 queue full with Retry-After: the generation path retries once; a report never
	p503, pHits := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":"fila do servidor cheia"}`)
	})
	c2 := newClient(t, p503, func(o *Options) { o.FallbackURL, o.FallbackAPIKey = fbSrv.URL, fbTok })
	_, err := c2.Feedback(context.Background(), fbReport(), BackendPrimary)
	if err == nil || pHits.Load() != 1 || fbHits.Load() != 0 {
		bad = append(bad, fmt.Sprintf("503: err=%v primário %d fallback %d", err, pHits.Load(), fbHits.Load()))
	}
	// (c) invalid backend names never pick a server
	for _, b := range []string{"", "PRIMARIO", "fallback ", "Fallback", "outro"} {
		if _, err := c2.Feedback(context.Background(), fbReport(), b); err == nil {
			bad = append(bad, fmt.Sprintf("backend %q aceito", b))
		}
	}
	if pHits.Load() != 1 || fbHits.Load() != 0 {
		bad = append(bad, fmt.Sprintf("backend inválido enviou (primário %d, fallback %d)", pHits.Load(), fbHits.Load()))
	}
	if len(bad) > 0 {
		vec12B(t, "O-01", "FALHA", "Bloqueador feedback.go:79-91: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "O-01", "OK", "relato de job do primário com o primário fora do ar ou com 503+Retry-After: erro, 1 pedido no primário e 0 no fallback (não usa o fallback nem retenta como a geração); backend \"\", PRIMARIO, \"fallback \", Fallback, outro: recusado sem pedido")
}

func TestAdv12BFallbackGetsOnlyItsOwnCredential(t *testing.T) {
	var mu sync.Mutex
	var auths []string
	fbSrv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization")+"|q="+r.URL.RawQuery)
		mu.Unlock()
		fmt.Fprint(w, adv12BOK)
	})
	pSrv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, adv12BOK) })
	var bad []string
	// (a) fallback configured WITHOUT its own key: the primary key is never copied to it
	c := newClient(t, pSrv, func(o *Options) { o.FallbackURL = fbSrv.URL })
	if _, err := c.Feedback(context.Background(), fbReport(), BackendFallback); err != nil {
		bad = append(bad, "sem chave: "+err.Error())
	}
	// (b) fallback URL with userinfo and a query: they are stripped and redacted, never sent
	u := strings.Replace(fbSrv.URL, "http://", "http://fbuser:FBSENHA-77@", 1) + "/?k=QSEGREDO-88"
	var logMu sync.Mutex
	var logs []string
	c2 := newClient(t, pSrv, func(o *Options) {
		o.FallbackURL, o.FallbackAPIKey = u, fbTok
		o.Logf = func(f string, a ...any) { logMu.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); logMu.Unlock() }
	})
	if _, err := c2.Feedback(context.Background(), fbReport(), BackendFallback); err != nil {
		bad = append(bad, "userinfo: "+err.Error())
	}
	mu.Lock()
	for i, a := range auths {
		want := []string{"|q=", "Bearer " + fbTok + "|q="}[min(i, 1)]
		if a != want {
			bad = append(bad, fmt.Sprintf("pedido %d ao fallback com %q (esperado %q)", i, a, want))
		}
	}
	mu.Unlock()
	// (c) the same fallback down: the connect error names neither the userinfo, the query, nor a token
	down := "http://fbuser:FBSENHA-77@" + adv12BClosedPort(t) + "/?k=QSEGREDO-88"
	c3 := newClient(t, pSrv, func(o *Options) {
		o.FallbackURL, o.FallbackAPIKey = down, fbTok
		o.Logf = func(f string, a ...any) { logMu.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); logMu.Unlock() }
	})
	_, err := c3.Feedback(context.Background(), fbReport(), BackendFallback)
	msgs := []string{}
	if err != nil {
		msgs = append(msgs, err.Error())
	}
	logMu.Lock()
	msgs = append(msgs, logs...)
	logMu.Unlock()
	for _, m := range msgs {
		for _, s := range []string{"FBSENHA", "QSEGREDO", token, fbTok, "fbuser"} {
			if strings.Contains(m, s) {
				bad = append(bad, fmt.Sprintf("%q em %q", s, m))
			}
		}
	}
	// (d) a plain-http fallback outside loopback with a key is refused at start
	if _, err := New(Options{BaseURL: pSrv.URL, APIKey: token, FallbackURL: "http://10.0.0.9:11500", FallbackAPIKey: fbTok}); err == nil {
		bad = append(bad, "fallback http fora de loopback com chave aceito")
	}
	if len(bad) > 0 {
		vec12B(t, "O-02", "FALHA", "Bloqueador client.go:121-208 / feedback.go:104-123: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "O-02", "OK", "fallback sem chave própria recebe pedido SEM Authorization (a chave do primário não é copiada); userinfo e query da URL do fallback não vão no pedido (nem como Basic) e não aparecem no erro de conexão nem no debug; fallback http fora de loopback com chave recusado no New")
}

func TestAdv12BFeedbackHangingServerIsBounded(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c := newClient(t, srv, func(o *Options) { o.Timeout = 700 * time.Millisecond })
	start := time.Now()
	_, err := c.Feedback(context.Background(), fbReport(), BackendPrimary)
	el := time.Since(start)
	if err == nil || el > 3*time.Second {
		vec12B(t, "O-03", "FALHA", fmt.Sprintf("Importante feedback.go:109: servidor que não responde: err=%v após %v", err, el))
		return
	}
	vec12B(t, "O-03", "OK", fmt.Sprintf("MEDIDA servidor de feedback que nunca responde: erro de tempo esgotado em %v com --timeout 700ms (o relato manual usa o --timeout da geração, padrão 120 s; o automático tem teto próprio de 30 s)", el.Round(10*time.Millisecond)))
}
