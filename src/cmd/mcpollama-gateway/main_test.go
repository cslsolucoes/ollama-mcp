package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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
		c.MaxBodyBytes != 32<<20 || c.RetentionReq != 90*24*time.Hour || c.RetentionGPU != 7*24*time.Hour || c.GPUInterval != time.Second || c.SampleStore != 5*time.Second {
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

// Fase 9.6a: the policy flags, their env fallbacks and the --help text.
func TestParseConfigPolicyFlags(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultThink || c.ThinkSuffix != "-think" || c.AllowKeepAlive || c.AllowNumCtx ||
		strings.Join(c.StripOptions, ",") != "num_gpu,main_gpu,low_vram,use_mmap,use_mlock,numa" {
		t.Fatalf("defaults: %+v", c)
	}
	env := map[string]string{
		"MCPGW_DEFAULT_THINK": "true", "MCPGW_THINK_SUFFIX": "-raciocina", "MCPGW_ALLOW_CLIENT_KEEPALIVE": "1",
		"MCPGW_ALLOW_CLIENT_NUM_CTX": "yes", "MCPGW_STRIP_OPTIONS": "num_gpu; seed",
	}
	c, err = parseConfig(nil, func(k string) string { return env[k] }, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !c.DefaultThink || c.ThinkSuffix != "-raciocina" || !c.AllowKeepAlive || !c.AllowNumCtx || strings.Join(c.StripOptions, ",") != "num_gpu,seed" {
		t.Fatalf("env: %+v", c)
	}
	// the flag wins; "none" disables the suffix and the strip list
	c, err = parseConfig([]string{"--think-suffix", "none", "--strip-options", "none", "--default-think=false"}, func(k string) string { return env[k] }, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if c.ThinkSuffix != "" || c.StripOptions == nil || len(c.StripOptions) != 0 || c.DefaultThink != false {
		t.Fatalf("flags: %+v", c)
	}
	if _, err := parseConfig([]string{"--think-suffix", "a b"}, noEnv, &buf); err == nil {
		t.Fatal("a suffix with a space must be refused")
	}
	for _, f := range []string{"--default-think", "--think-suffix", "--allow-client-keepalive", "--allow-client-num-ctx", "--strip-options",
		"MCPGW_DEFAULT_THINK", "MCPGW_THINK_SUFFIX", "MCPGW_ALLOW_CLIENT_KEEPALIVE", "MCPGW_ALLOW_CLIENT_NUM_CTX", "MCPGW_STRIP_OPTIONS"} {
		if !strings.Contains(usageText, f) {
			t.Errorf("--help does not document %s", f)
		}
	}
}

// Fase 9.6a end to end: the policies run through run(); the Ollama gets the
// rewritten body, the monitor history shows the reasoning mode.
func TestRunAppliesPolicies(t *testing.T) {
	var mu sync.Mutex
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, string(b))
		mu.Unlock()
		io.WriteString(w, `{"response":"ok","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
	}))
	defer up.Close()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", filepath.Join(t.TempDir(), "gw.db"), "--gpu-interval", "0", "--aer-path", "-",
			"--allowed-models", "qwen3-ia"},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	var base string
	select {
	case a := <-addr:
		base = "http://" + a
	case <-time.After(5 * time.Second):
		t.Fatalf("did not start: %s", errb.String())
	}
	resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia-think","prompt":"x","keep_alive":0,"options":{"num_ctx":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	mu.Lock()
	sent := strings.Join(got, "|")
	mu.Unlock()
	if strings.Contains(sent, "keep_alive") || strings.Contains(sent, "num_ctx") || !strings.Contains(sent, `"think":true`) || !strings.Contains(sent, `"model":"qwen3-ia"`) {
		t.Fatalf("body at the Ollama: %s", sent)
	}
	if r, _ := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":`)); r.StatusCode != 400 {
		t.Fatalf("invalid json: %d", r.StatusCode)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		h, err := http.Get(base + "/monitor/api/history")
		if err == nil {
			hb, _ := io.ReadAll(h.Body)
			h.Body.Close()
			if strings.Contains(string(hb), `"think":"on"`) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the reasoning mode never reached the history")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("did not shut down")
	}
}

// Fase 10.L flags: defaults, env, validation, --help text.
func TestParseConfigJobLogFlags(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !c.LogContent || c.ContentMax != 2<<30 || c.ContentMaxJob != 16<<20 || c.RetentionCont != 7*24*time.Hour ||
		c.RetentionJobGP != 7*24*time.Hour || c.KeepDone != 5*time.Minute || filepath.ToSlash(c.ContentDB) != "/var/lib/mcpollama-gateway/content.db" {
		t.Fatalf("defaults: %+v", c)
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"MCPGW_LOG_CONTENT": "false", "MCPGW_CONTENT_MAX_BYTES": "1048576", "MCPGW_MONITOR_KEEP_DONE": "90s", "MCPGW_RETENTION_JOB_GPU": "24h"}
	c, err = parseConfig([]string{"--content-max-job-bytes", "4096"}, func(k string) string { return env[k] }, &buf)
	if err != nil || c.LogContent || c.ContentMax != 1<<20 || c.ContentMaxJob != 4096 || c.KeepDone != 90*time.Second || c.RetentionJobGP != 24*time.Hour {
		t.Fatalf("env: %v %+v", err, c)
	}
	// the flag beats the env, and --log-content=true turns it back on
	c, _ = parseConfig([]string{"--log-content=true", "--db", "/x/gw.db"}, func(k string) string { return env[k] }, &buf)
	if !c.LogContent || filepath.ToSlash(c.ContentDB) != "/x/content.db" {
		t.Fatalf("%+v", c)
	}
	bad := [][]string{{"--content-db", "/var/lib/mcpollama-gateway/gateway.db", "--db", "/var/lib/mcpollama-gateway/gateway.db"}, {"--content-max-job-bytes", "999999999999"}, {"--content-max-bytes", "10"}, {"--monitor-keep-done", "-1s"}}
	for _, a := range bad {
		c, err := parseConfig(a, noEnv, &buf)
		if err == nil {
			err = c.validate()
		}
		if err == nil {
			t.Errorf("%v accepted", a)
		}
	}
	// --log-content=false does not need the content flags to be sane
	c, _ = parseConfig([]string{"--log-content=false", "--content-max-bytes", "10"}, noEnv, &buf)
	if err := c.validate(); err != nil {
		t.Errorf("content off must ignore the content caps: %v", err)
	}
	var help bytes.Buffer
	parseConfig([]string{"-h"}, noEnv, &help)
	for _, w := range []string{"--log-content", "--content-db", "--content-max-bytes", "--content-max-job-bytes", "--retention-content", "--retention-job-gpu", "--monitor-keep-done", "MCPGW_LOG_CONTENT"} {
		if !strings.Contains(help.String(), w) {
			t.Errorf("--help lacks %s", w)
		}
	}
}
