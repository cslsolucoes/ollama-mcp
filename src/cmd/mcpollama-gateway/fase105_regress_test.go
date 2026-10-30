package main

// Regressão da revisão adversarial 10.5 (não regressão sem --backends). Falha
// no código da 10.4 (ed57add) e passa depois da correção.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// N1 (Média): sem --backends, com o Ollama fora, o quadro "Modelos" mostra o
// erro (como o RunLoop do coletor fazia antes da 10.4) em vez de seguir com o
// último retrato bom.
func TestRegress105ModelsBoardShowsTheOllamaDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"0.40.8","models":[{"name":"qwen3-ia","model":"qwen3-ia","size":1,"size_vram":1}]}`)
	}))
	base, errb, stop := startGateway(t, "--ollama", srv.URL, "--health-interval", "40ms")
	defer stop()
	var st struct {
		Models struct {
			Loaded []struct{} `json:"loaded"`
			Err    string     `json:"error"`
		} `json:"models"`
	}
	wait := func(ok func() bool) bool {
		for dl := time.Now().Add(3 * time.Second); time.Now().Before(dl); time.Sleep(30 * time.Millisecond) {
			getJSON(t, base+"/monitor/api/state", &st)
			if ok() {
				return true
			}
		}
		return false
	}
	if !wait(func() bool { return len(st.Models.Loaded) == 1 }) {
		t.Fatalf("o modelo nunca apareceu: %s", errb.String())
	}
	srv.Close()
	if !wait(func() bool { return st.Models.Err != "" && len(st.Models.Loaded) == 0 }) {
		t.Fatalf("Ollama fora e o quadro Modelos parado: %+v", st.Models)
	}
}
