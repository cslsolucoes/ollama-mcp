package main

// Fase 9.5 - revisao adversarial: vetores do entrypoint.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func vec(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// C1: --priority-ips com IPv4 mapeado em IPv6. "::ffff:10.0.0.5" vira
// "::ffff:10.0.0.5/32", que o net.ParseCIDR le como ::/32 (mascara IPv6 de 32
// bits): a entrada e aceita e nao casa com nada, nem com 10.0.0.5. A hipotese
// inicial (casar com todo IPv4) foi refutada pelo spike; o defeito real e a
// falha silenciosa.
func TestAdvPriorityIPsMappedV6(t *testing.T) {
	bad := []string{}
	for _, in := range []string{"::ffff:10.0.0.5", "::ffff:10.0.0.0/8", "::ffff:10.100.2.0/120"} {
		nets, err := parseNets(in)
		if err != nil {
			continue // refusing is acceptable
		}
		for _, other := range []string{"192.168.50.1", "8.8.8.8", "10.200.0.1"} {
			for _, n := range nets {
				if n.Contains(net.ParseIP(other)) {
					bad = append(bad, fmt.Sprintf("%s contem %s", in, other))
				}
			}
		}
	}
	// The intended address must match (or the entry must be refused): a
	// silent "matches nothing" leaves the administrator thinking alta works.
	for in, want := range map[string]string{"::ffff:10.0.0.5": "10.0.0.5", "::ffff:10.0.0.0/104": "10.9.9.9"} {
		nets, err := parseNets(in)
		if err != nil {
			continue
		}
		hit := false
		for _, n := range nets {
			hit = hit || n.Contains(net.ParseIP(want))
		}
		if !hit {
			bad = append(bad, fmt.Sprintf("%s aceito mas nao contem %s (falha silenciosa)", in, want))
		}
	}
	if len(bad) > 0 {
		vec(t, "C1-priority-ips-ipv4-mapeado", "Baixa", "cmd/mcpollama-gateway/config.go:245", "FALHA: "+strings.Join(bad, "; "))
		t.Error(bad)
	} else {
		vec(t, "C1-priority-ips-ipv4-mapeado", "Baixa", "cmd/mcpollama-gateway/config.go:245", "OK: formas mapeadas viram o prefixo IPv4 equivalente ou sao recusadas")
	}
}

// C2: CIDR malformado na lista de prioridade derruba a partida (nao vira
// "aceita tudo" em silencio).
func TestAdvPriorityIPsMalformed(t *testing.T) {
	acc := []string{}
	for _, in := range []string{"10.0.0.0/33", "10.0.0/8", "10.0.0.0/-1", "10.0.0.0/8/9", "*", "0.0.0.0/", "::/129", "10.0.0.1 10.0.0.2", "10.0.0.0/08x"} {
		if _, err := parseNets(in); err == nil {
			acc = append(acc, in)
		}
	}
	if len(acc) > 0 {
		vec(t, "C2-priority-ips-malformado", "Media", "cmd/mcpollama-gateway/config.go:237", fmt.Sprintf("FALHA: aceitos %q", acc))
		t.Error(acc)
	} else {
		vec(t, "C2-priority-ips-malformado", "Media", "cmd/mcpollama-gateway/config.go:237", "OK: 9 formas malformadas recusadas (partida falha)")
	}
}

// C3: hash-password nao ecoa a senha em stdout nem em stderr, inclusive nos
// erros; a senha nunca vai na linha de comando.
func TestAdvHashPasswordNoLeak(t *testing.T) {
	const pw = "SENHA-SECRETA-sentinela-42"
	leak := []string{}
	for _, args := range [][]string{{"hash-password", "-iterations", "1000"}, {"hash-password", "-iterations", "1"}, {"hash-password", "-user", "a:b"}} {
		var out, errb bytes.Buffer
		run(context.Background(), args, noEnv, strings.NewReader(pw+"\n"), &out, &errb, nil)
		if strings.Contains(out.String()+errb.String(), pw) {
			leak = append(leak, strings.Join(args, " "))
		}
	}
	var out, errb bytes.Buffer
	run(context.Background(), []string{"hash-password", pw}, noEnv, strings.NewReader("x\n"), &out, &errb, nil)
	if len(leak) > 0 {
		vec(t, "C3-hash-password", "Media", "cmd/mcpollama-gateway/main.go:209", fmt.Sprintf("FALHA: senha ecoada em %v", leak))
		t.Error(leak)
	} else {
		vec(t, "C3-hash-password", "Media", "cmd/mcpollama-gateway/main.go:209", "OK: senha so pelo stdin; nao aparece em stdout/stderr (sucesso e 2 erros). Risco residual: o terminal ecoa o que se digita (documentar `read -rs`)")
	}
}

// C4: ponta a ponta com --debug: prompt, resposta e token nao aparecem no log
// nem em nenhuma rota do /monitor; o banco e conferido pelo teste-sentinela.
func TestAdvEndToEndNoSecretInLogsOrMonitor(t *testing.T) {
	const (
		tok    = "SENTINELA-TOKEN-e2e-77aa"
		prompt = "SENTINELA-PROMPT-e2e-31cc"
		answer = "SENTINELA-RESPOSTA-e2e-90dd"
	)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(500)
			io.WriteString(w, `{"error":"token chegou ao Ollama"}`)
			return
		}
		io.WriteString(w, `{"response":"`+answer+`","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
	}))
	defer up.Close()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer // run wraps stderr in a syncWriter; read only after done
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", filepath.Join(dir, "gw.db"), "--gpu-interval", "0", "--aer-path", "-", "--debug"},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	base := "http://" + <-addr
	req, _ := http.NewRequest("POST", base+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"`+prompt+`"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	time.Sleep(400 * time.Millisecond)
	var all strings.Builder
	for _, p := range []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/stats", "/monitor/api/audit", "/monitor/api/series", "/monitor/api/session", "/monitor"} {
		r, err := http.Get(base + p)
		if err == nil {
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			all.Write(b)
		}
	}
	cancel()
	<-done
	leak := []string{}
	for _, s := range []string{tok, prompt, answer} {
		if strings.Contains(all.String(), s) {
			leak = append(leak, "monitor:"+s)
		}
		if strings.Contains(errb.String(), s) {
			leak = append(leak, "log:"+s)
		}
	}
	for _, f := range []string{"gw.db", "gw.db-wal"} {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		for _, s := range []string{tok, prompt, answer} {
			if bytes.Contains(b, []byte(s)) {
				leak = append(leak, f+":"+s)
			}
		}
	}
	if resp.StatusCode != 200 || len(leak) > 0 {
		vec(t, "C4-e2e-sem-segredo", "Alta", "cmd/mcpollama-gateway/main.go:123", fmt.Sprintf("FALHA: status %d vazamentos %v", resp.StatusCode, leak))
		t.Error(leak)
	} else {
		vec(t, "C4-e2e-sem-segredo", "Alta", "cmd/mcpollama-gateway/main.go:123", "OK: token removido antes do Ollama; prompt/resposta/token ausentes do log --debug, do /monitor e do .db/-wal")
	}
}

// C5 (pendencia 8 da 9.4): com um painel SSE aberto, o encerramento por
// SIGTERM espera os 30 s de tolerancia inteiros, porque o stream nunca termina
// sozinho (Shutdown nao cancela o contexto dos pedidos). Um reinicio do
// gateway com o monitor aberto fica 30 s sem atender.
func TestAdvShutdownWithOpenSSE(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer up.Close()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", filepath.Join(t.TempDir(), "gw.db"), "--gpu-interval", "0", "--aer-path", "-"},
			noEnv, strings.NewReader(""), io.Discard, io.Discard, func(a string) { addr <- a })
	}()
	base := "http://" + <-addr
	resp, err := http.Get(base + "/monitor/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	go io.Copy(io.Discard, resp.Body)
	t0 := time.Now()
	cancel()
	select {
	case <-done:
		d := time.Since(t0)
		if d > 3*time.Second {
			vec(t, "C5-shutdown-sse", "Baixa", "cmd/mcpollama-gateway/main.go:183", fmt.Sprintf("FALHA: encerrou em %v com 1 painel aberto", d))
			t.Errorf("shutdown %v", d)
		} else {
			vec(t, "C5-shutdown-sse", "Baixa", "cmd/mcpollama-gateway/main.go:183", fmt.Sprintf("OK: encerrou em %v com 1 painel aberto", d.Round(time.Millisecond)))
		}
	case <-time.After(10 * time.Second):
		vec(t, "C5-shutdown-sse", "Baixa", "cmd/mcpollama-gateway/main.go:183", "FALHA: nao encerrou em 10 s com 1 painel SSE aberto (tolerancia de 30 s)")
		t.Error("shutdown blocked by the event stream")
	}
}
