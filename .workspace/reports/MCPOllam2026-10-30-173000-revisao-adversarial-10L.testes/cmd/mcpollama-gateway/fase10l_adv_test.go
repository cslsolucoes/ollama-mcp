package main

// Fase 10.L - revisao adversarial: vetores do entrypoint.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func vecL(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// R2: content.db corrompido (ou impossivel de abrir) nao pode derrubar o
// gateway: ele segue sem conteudo e registra um evento.
func TestAdvLCorruptContentDBKeepsGatewayUp(t *testing.T) {
	const sent = "SENTINELA-PROMPT-R2-10L"
	for _, mode := range []string{"lixo", "diretorio"} {
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			io.WriteString(w, `{"response":"ok","done":true,"prompt_eval_count":1,"eval_count":1}`+"\n")
		}))
		dir := t.TempDir()
		dbPath, contPath := filepath.Join(dir, "gw.db"), filepath.Join(dir, "content.db")
		if mode == "lixo" {
			os.WriteFile(contPath, []byte(strings.Repeat("isto nao e sqlite ", 4096)), 0o600)
		} else {
			os.MkdirAll(contPath, 0o700) // o caminho e um diretorio: nao abre
		}
		ctx, cancel := context.WithCancel(context.Background())
		addr := make(chan string, 1)
		done := make(chan int, 1)
		var errb bytes.Buffer
		go func() {
			done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", dbPath, "--content-db", contPath, "--gpu-interval", "0", "--aer-path", "-", "--debug"},
				noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
		}()
		res := ""
		select {
		case code := <-done:
			res = fmt.Sprintf("%s: o gateway saiu com %d na partida", mode, code)
		case a := <-addr:
			base := "http://" + a
			resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"`+sent+`"}`))
			code := 0
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				code = resp.StatusCode
			}
			sr, _ := http.Get(base + "/monitor/api/state")
			sb, _ := io.ReadAll(sr.Body)
			sr.Body.Close()
			cancel()
			<-done
			st, _ := store.Open(dbPath, store.Options{BootID: "x"})
			evs, _ := st.Events(context.Background(), 100)
			st.Close()
			ev := ""
			for _, e := range evs {
				if strings.Contains(e.Kind, "conteudo") {
					ev = e.Kind + " " + e.Detail
				}
			}
			if code != 200 || !strings.Contains(string(sb), `"content_enabled":false`) || ev == "" || strings.Contains(errb.String(), sent) {
				res = fmt.Sprintf("%s: subiu, mas pedido=%d content_enabled_false=%v evento=%q", mode, code, strings.Contains(string(sb), `"content_enabled":false`), ev)
			} else {
				res = "OK " + mode + ": pedido 200, content_enabled=false, evento " + ev
			}
		case <-time.After(15 * time.Second):
			res = mode + ": nao subiu em 15 s"
		}
		cancel()
		up.Close()
		if !strings.HasPrefix(res, "OK") {
			vecL(t, "R2-content-db-corrompido", "Media", "cmd/mcpollama-gateway/main.go:97", "FALHA: "+res)
			t.Error(res)
		} else {
			vecL(t, "R2-content-db-corrompido", "Media", "cmd/mcpollama-gateway/main.go:97", res)
		}
	}
}
