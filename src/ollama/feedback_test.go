package ollama

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Fase 12.5: POST /api/feedback in the client.

const fbTok = "tok-FALLBACK-0042"

const (
	ocP = "11111111111111111111111111111111"
	ocF = "22222222222222222222222222222222"
	ocN = "33333333333333333333333333333333"
)

func fbReport() FeedbackReport {
	return FeedbackReport{
		Projeto: "p", Jobs: []FeedbackJob{{ID: strings.Repeat("a", 32), Papel: "unico", Backend: "primario"}},
		JobDoErro: strings.Repeat("a", 32), Categoria: "omitiu", Descricao: "d", Severidade: "baixa", Origem: "claude",
	}
}

func TestFeedbackUsesTheClientHeadersAndDecodesTheResult(t *testing.T) {
	var mu sync.Mutex
	var got http.Header
	var path string
	srv, n := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got, path = r.Header.Clone(), r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{"ocorrencia_id":"`+ocN+`","estado":"nova","amostras_copiadas":1,"sem_pedido":0,"jobs_externos":2,"jobs_outra_origem":3}`)
	})
	c := newClient(t, srv, func(o *Options) {
		o.StationInfo = "computer=PC;user=u;domain=D;mac=AA"
		o.ClientLabel = "pc/u"
	})
	res, err := c.Feedback(context.Background(), fbReport(), BackendPrimary)
	if err != nil {
		t.Fatal(err)
	}
	if res.OcorrenciaID != ocN || res.Estado != "nova" || res.AmostrasCopiadas != 1 || res.JobsExternos != 2 || res.JobsOutraOrigem != 3 {
		t.Fatalf("result %+v", res)
	}
	mu.Lock()
	defer mu.Unlock()
	if path != "/api/feedback" || got.Get("Authorization") != "Bearer "+token || got.Get("X-Mcpollama-Station") != "computer=PC;user=u;domain=D;mac=AA" ||
		got.Get("X-Mcpollama-Client") != "pc/u" || got.Get("Content-Type") != "application/json" || n.Load() != 1 {
		t.Fatalf("path %q headers %v", path, got)
	}
}

// Fallback job without a configured fallback: no request, a clear error.
func TestFeedbackFallbackJobWithoutFallbackSendsNothing(t *testing.T) {
	srv, n := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) })
	c := newClient(t, srv, nil)
	_, err := c.Feedback(context.Background(), fbReport(), BackendFallback)
	if err == nil || n.Load() != 0 || !strings.Contains(err.Error(), "fallback") {
		t.Fatalf("err=%v hits=%d", err, n.Load())
	}
	if strings.Contains(err.Error(), token) {
		t.Fatal("token in the error")
	}
	if _, err := c.Feedback(context.Background(), fbReport(), "outro"); err == nil || n.Load() != 0 {
		t.Fatalf("invalid backend must fail without a request: %v", err)
	}
}

// The fallback report uses the fallback's token only; the primary's never goes there.
func TestFeedbackTokensStayWithTheirServer(t *testing.T) {
	var mu sync.Mutex
	var pAuth, fAuth []string
	primary, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pAuth = append(pAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		fmt.Fprint(w, `{"ocorrencia_id":"`+ocP+`","estado":"nova"}`)
	})
	fb, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fAuth = append(fAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		fmt.Fprint(w, `{"ocorrencia_id":"`+ocF+`","estado":"fallback"}`)
	})
	c := newClient(t, primary, func(o *Options) { o.FallbackURL = fb.URL; o.FallbackAPIKey = fbTok })
	if r, err := c.Feedback(context.Background(), fbReport(), BackendFallback); err != nil || r.OcorrenciaID != ocF {
		t.Fatalf("%v %+v", err, r)
	}
	if r, err := c.Feedback(context.Background(), fbReport(), BackendPrimary); err != nil || r.OcorrenciaID != ocP {
		t.Fatalf("%v %+v", err, r)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(pAuth) != 1 || pAuth[0] != "Bearer "+token || len(fAuth) != 1 || fAuth[0] != "Bearer "+fbTok {
		t.Fatalf("primary %v, fallback %v", pAuth, fAuth)
	}
}

// Any non-200 status is an error that carries the status and Retry-After, never
// goes to the fallback, never is retried by the client, and is scrubbed.
func TestFeedbackRefusals(t *testing.T) {
	for _, st := range []int{400, 404, 409, 503, 507} {
		st := st
		primary, pn := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(st)
			fmt.Fprintf(w, `{"error":"recusado com %s dentro"}`, token)
		})
		fb, fn := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ocorrencia_id":"`+ocF+`","estado":"nova"}`)
		})
		c := newClient(t, primary, func(o *Options) { o.FallbackURL = fb.URL; o.FallbackAPIKey = fbTok })
		_, err := c.Feedback(context.Background(), fbReport(), BackendPrimary)
		var fe *FeedbackError
		if !errors.As(err, &fe) || fe.Status() != st {
			t.Fatalf("%d: %v", st, err)
		}
		if (st == 409 || st == 503) && fe.RetryAfter.Seconds() != 7 {
			t.Fatalf("%d: RetryAfter %v", st, fe.RetryAfter)
		}
		if pn.Load() != 1 || fn.Load() != 0 {
			t.Fatalf("%d: primary %d requests (no retry), fallback %d (none)", st, pn.Load(), fn.Load())
		}
		if strings.Contains(err.Error(), token) || strings.Contains(fe.Detail, token) {
			t.Fatalf("%d: token leaked: %q / %q", st, err.Error(), fe.Detail)
		}
		if !strings.Contains(fe.Detail, "recusado") {
			t.Fatalf("%d: detail %q", st, fe.Detail)
		}
	}
}

func TestFeedbackNoServerIsAConnectError(t *testing.T) {
	srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {})
	c := newClient(t, srv, nil)
	srv.Close()
	_, err := c.Feedback(context.Background(), fbReport(), BackendPrimary)
	var fe *FeedbackError
	if !errors.As(err, &fe) || fe.Status() != 0 || !errors.Is(err, ErrConnect) {
		t.Fatalf("%v", err)
	}
}

// M-5: an answer that does not have the promised shape is refused without
// echoing what the server sent.
func TestFeedbackRefusesAMalformedAnswer(t *testing.T) {
	bad := []string{
		`{"ocorrencia_id":"<script>alert(1)</script>","estado":"nova"}`,
		`{"ocorrencia_id":"` + strings.Repeat("A", 32) + `","estado":"nova"}`,
		`{"ocorrencia_id":"` + ocN + `","estado":"INJETADO-ESTADO"}`,
		`{"ocorrencia_id":"` + ocN + `","estado":""}`,
		`{"ocorrencia_id":"` + ocN + `","estado":"nova","amostras_copiadas":-1}`,
		`{"estado":"nova"}`,
		`[]`,
	}
	for _, body := range bad {
		body := body
		srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		c := newClient(t, srv, nil)
		_, err := c.Feedback(context.Background(), fbReport(), BackendPrimary)
		if err == nil || !errors.Is(err, ErrBadResponse) {
			t.Fatalf("%s: %v", body, err)
		}
		for _, echo := range []string{"script", "INJETADO", strings.Repeat("A", 32)} {
			if strings.Contains(err.Error(), echo) {
				t.Fatalf("%s: the error echoes %q: %s", body, echo, err)
			}
		}
	}
	// every state of C-1.8 is accepted
	for _, st := range []string{"nova", "confirmada", "descartada", "sem_pedido", "distribuida", "fallback", "apagada"} {
		st := st
		srv, _ := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"ocorrencia_id":"%s","estado":"%s"}`, ocN, st)
		})
		if r, err := newClient(t, srv, nil).Feedback(context.Background(), fbReport(), BackendPrimary); err != nil || r.Estado != st {
			t.Fatalf("%s: %v", st, err)
		}
	}
}
