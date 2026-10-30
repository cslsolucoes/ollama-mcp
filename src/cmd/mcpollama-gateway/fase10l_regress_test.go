package main

// Regressão da revisão adversarial da etapa 10.L (R2): falha no código do
// commit 6cbaf22 (run devolve 1 na partida) e passa depois da correção.

import (
	"bytes"
	"context"
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

func TestRegress10LCorruptContentDBDoesNotStopTheGateway(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"response":"ok","done":true}`+"\n")
	}))
	defer up.Close()
	dir := t.TempDir()
	dbPath, contPath := filepath.Join(dir, "gw.db"), filepath.Join(dir, "content.db")
	garbage := []byte(strings.Repeat("nao e sqlite ", 4096))
	os.WriteFile(contPath, garbage, 0o600)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", dbPath, "--content-db", contPath, "--gpu-interval", "0", "--aer-path", "-"},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	var base string
	select {
	case code := <-done:
		t.Fatalf("o gateway saiu com %d: %s", code, errb.String())
	case a := <-addr:
		base = "http://" + a
	case <-time.After(15 * time.Second):
		t.Fatal("não subiu")
	}
	resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"x"}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("pedido: %v", err)
	}
	resp.Body.Close()
	sr, err := http.Get(base + "/monitor/api/state")
	if err != nil {
		t.Fatal(err)
	}
	sb, _ := io.ReadAll(sr.Body)
	sr.Body.Close()
	if !strings.Contains(string(sb), `"content_enabled":false`) {
		t.Fatalf("o monitor não diz que o conteúdo está desligado: %s", sb)
	}
	cancel()
	<-done
	st, err := store.Open(dbPath, store.Options{BootID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	evs, _ := st.Events(context.Background(), 100)
	found := false
	for _, e := range evs {
		found = found || e.Kind == "conteudo_indisponivel"
	}
	if !found {
		t.Fatal("sem o evento conteudo_indisponivel")
	}
	// o arquivo corrompido fica como estava, para inspeção
	if b, _ := os.ReadFile(contPath); !bytes.Equal(b, garbage) {
		t.Fatal("o content.db corrompido foi alterado")
	}
}
