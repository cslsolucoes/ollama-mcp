package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func noEnv(string) string { return "" }

func TestParseConfigDefaults(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:11500" || c.Ollama.Host != "127.0.0.1:11434" || c.GenSlots != 2 || c.EmbedSlots != 2 ||
		c.QueueMax != 64 || c.PerClient != 16 || c.PerIP != 16 || c.LabelsPerIP != 16 ||
		c.MaxWait != 10*time.Minute || c.AgeAfter != 5*time.Minute || c.StartStagger != 300*time.Millisecond ||
		c.MaxBodyBytes != 32<<20 || c.RetentionReq != 90*24*time.Hour || c.RetentionGPU != 7*24*time.Hour || c.GPUInterval != 5*time.Second {
		t.Fatalf("%+v", c)
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestParseConfigFlagBeatsEnvAndPerIPFollowsPerClient(t *testing.T) {
	env := map[string]string{"MCPGW_GEN_SLOTS": "4", "MCPGW_QUEUE_PER_CLIENT": "9", "MCPGW_PRIORITY_IPS": "10.100.2.0/24, 10.100.3.7"}
	var buf bytes.Buffer
	c, err := parseConfig([]string{"--gen-slots", "3", "--labels-per-ip", "5", "--allowed-models", "a;b,c"}, func(k string) string { return env[k] }, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if c.GenSlots != 3 || c.PerClient != 9 || c.PerIP != 9 || c.LabelsPerIP != 5 || len(c.PriorityNets) != 2 || len(c.AllowedModels) != 3 {
		t.Fatalf("%+v", c)
	}
	if !c.PriorityNets[0].Contains([]byte{10, 100, 2, 77}) || !c.PriorityNets[1].Contains([]byte{10, 100, 3, 7}) || c.PriorityNets[1].Contains([]byte{10, 100, 3, 8}) {
		t.Fatalf("nets %v", c.PriorityNets)
	}
	c2, _ := parseConfig([]string{"--queue-per-client", "9", "--queue-per-ip", "30"}, noEnv, &buf)
	if c2.PerIP != 30 {
		t.Fatalf("explicit --queue-per-ip lost: %d", c2.PerIP)
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := [][]string{
		{"--priority-ips", "not-an-ip"},
		{"--ollama", "https://x:1"},
		{"--max-wait", "abc"},
		{"extra"},
	}
	for _, a := range cases {
		var buf bytes.Buffer
		if _, err := parseConfig(a, noEnv, &buf); err == nil {
			t.Errorf("accepted %v", a)
		}
	}
	var buf bytes.Buffer
	c, _ := parseConfig([]string{"--listen", "0.0.0.0:11500"}, noEnv, &buf)
	if err := c.validate(); err == nil {
		t.Error("non-loopback listen accepted")
	}
	c, _ = parseConfig([]string{"--listen", "0.0.0.0:11500", "--non-loopback-listen"}, noEnv, &buf)
	if err := c.validate(); err != nil {
		t.Errorf("explicit override refused: %v", err)
	}
	c, _ = parseConfig([]string{"--gen-slots", "0"}, noEnv, &buf)
	if err := c.validate(); err == nil {
		t.Error("zero slots accepted")
	}
}

func TestVersionAndBadConfigExitCodes(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, noEnv, strings.NewReader(""), &out, &errb, nil); code != 0 || !strings.Contains(out.String(), "mcpollama-gateway version") {
		t.Fatalf("%d %q", code, out.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"--listen", "0.0.0.0:1"}, noEnv, strings.NewReader(""), &out, &errb, nil); code != 2 {
		t.Fatalf("bad config exit code %d", code)
	}
}

func TestHashPasswordSubcommand(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"hash-password", "-user", "claiton", "-iterations", "1000"}, noEnv, strings.NewReader("senha-de-teste-1\n"), &out, &errb, nil)
	if code != 0 || !strings.HasPrefix(out.String(), "claiton:pbkdf2-sha256$1000$") {
		t.Fatalf("%d %q %q", code, out.String(), errb.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"hash-password"}, noEnv, strings.NewReader("curta\n"), &out, &errb, nil); code != 2 {
		t.Fatalf("short password exit code %d", code)
	}
}

// End to end: a real process-like run with a fake Ollama, the proxy path, the
// monitor page, the SQLite history and a clean shutdown.
func TestRunEndToEnd(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"response":"ok","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
	}))
	defer up.Close()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", filepath.Join(dir, "gw.db"), "--gpu-interval", "0", "--aer-path", "-"},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	var base string
	select {
	case a := <-addr:
		base = "http://" + a
	case <-time.After(5 * time.Second):
		t.Fatalf("did not start: %s", errb.String())
	}
	if resp, err := http.Get(base + "/healthz"); err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", err, resp)
	}
	resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"response":"ok"`) || resp.Header.Get("X-Request-Id") == "" {
		t.Fatalf("proxy: %d %s %v", resp.StatusCode, b, resp.Header)
	}
	if resp, _ := http.Post(base+"/api/pull", "application/json", strings.NewReader(`{}`)); resp.StatusCode != 403 {
		t.Fatalf("pull: %d", resp.StatusCode)
	}
	page, err := http.Get(base + "/monitor")
	if err != nil || page.StatusCode != 200 {
		t.Fatalf("monitor: %v %v", err, page)
	}
	page.Body.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		h, err := http.Get(base + "/monitor/api/history")
		if err == nil {
			hb, _ := io.ReadAll(h.Body)
			h.Body.Close()
			if strings.Contains(string(hb), `"eval_count":4`) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("request never reached the history")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code %d: %s", code, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("did not shut down")
	}
}
