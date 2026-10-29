package tools

// Fase 9.5 - revisao adversarial: cliente mcpollama diante do gateway.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado.

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func vec(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// O1: so 503 com Retry-After inteiro >= 1 vale nova tentativa; 429, 409 sem
// "administrador", 502 e Retry-After invalido nao repetem; nunca mais de 1
// repeticao, em generate (stream) e embed (nao stream).
func TestAdvRetryOnlyOn503RetryAfter(t *testing.T) {
	type c struct {
		name, tool, args string
		status           int
		ra, body         string
		want             int32
	}
	cases := []c{
		{"503 RA=0", "generate", `{"prompt":"x"}`, 503, "0", "", 1},
		{"503 RA=-3", "generate", `{"prompt":"x"}`, 503, "-3", "", 1},
		{"503 RA=data HTTP", "generate", `{"prompt":"x"}`, 503, "Wed, 21 Oct 2015 07:28:00 GMT", "", 1},
		{"503 RA=1.5", "generate", `{"prompt":"x"}`, 503, "1.5", "", 1},
		{"429 RA=1", "generate", `{"prompt":"x"}`, 429, "1", "", 1},
		{"502 RA=1", "generate", `{"prompt":"x"}`, 502, "1", "", 1},
		{"409 generico", "generate", `{"prompt":"x"}`, 409, "", `{"error":"conflict"}`, 1},
		{"409 administrador RA=1", "generate", `{"prompt":"x"}`, 409, "1", `{"error":"cancelado pelo administrador"}`, 1},
		{"503 RA=1 sempre (embed)", "embed", `{"input":["a"]}`, 503, "1", "", 2},
	}
	bad := []string{}
	res := []string{}
	for _, x := range cases {
		var n atomic.Int32
		m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			if x.ra != "" {
				w.Header().Set("Retry-After", x.ra)
			}
			w.WriteHeader(x.status)
			_, _ = w.Write([]byte(x.body))
		})
		_, err := call(t, m.deps(t, nil), x.tool, x.args)
		res = append(res, fmt.Sprintf("%s=%d", x.name, n.Load()))
		if err == nil || n.Load() != x.want {
			bad = append(bad, fmt.Sprintf("%s: %d tentativas (want %d) err=%v", x.name, n.Load(), x.want, err))
		}
		if strings.Contains(x.name, "administrador") && (err == nil || !strings.Contains(err.Error(), "não foi repetida")) {
			bad = append(bad, x.name+": mensagem")
		}
	}
	if len(bad) > 0 {
		vec(t, "O1-retry-503", "Media", "ollama/api.go:323", "FALHA: "+strings.Join(bad, "; "))
		t.Error(bad)
	} else {
		vec(t, "O1-retry-503", "Media", "ollama/api.go:323", "OK: tentativas "+strings.Join(res, "; "))
	}
}
