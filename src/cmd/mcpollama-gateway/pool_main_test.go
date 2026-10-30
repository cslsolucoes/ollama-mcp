package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseConfigPoolFlags(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if c.Backends != "" || c.OverloadWait != time.Minute || c.OverloadQueue != 0 || c.OverloadHold != 30*time.Second ||
		c.HealthEvery != 5*time.Second || c.BreakerFails != 3 || c.BreakerOpen != 30*time.Second {
		t.Fatalf("defaults: %+v", c)
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"MCPGW_BACKENDS": "/etc/b.json", "MCPGW_OVERLOAD_WAIT": "90s", "MCPGW_OVERLOAD_QUEUE": "10", "MCPGW_BREAKER_FAILURES": "5"}
	c, err = parseConfig([]string{"--breaker-open", "1m", "--overload-wait", "30s"}, func(k string) string { return env[k] }, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if c.Backends != "/etc/b.json" || c.OverloadWait != 30*time.Second || c.OverloadQueue != 10 || c.BreakerFails != 5 || c.BreakerOpen != time.Minute {
		t.Fatalf("flag > env > default: %+v", c)
	}
	for _, a := range [][]string{{"--overload-wait", "0s"}, {"--breaker-failures", "0"}, {"--health-interval", "0s"}, {"--overload-hold", "-1s"}, {"--overload-queue", "999"}} {
		c, err := parseConfig(a, noEnv, &buf)
		if err == nil {
			err = c.validate()
		}
		if err == nil {
			t.Errorf("accepted %v", a)
		}
	}
}

// startGateway runs the gateway in-process and returns its base URL and a stop
// function that checks the exit code.
func startGateway(t *testing.T, args ...string) (base string, errb *bytes.Buffer, stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	errb = &bytes.Buffer{}
	sw := &syncWriter{w: errb}
	full := append([]string{"--listen", "127.0.0.1:0", "--db", filepath.Join(t.TempDir(), "gw.db"), "--gpu-interval", "0", "--aer-path", "-", "--start-stagger", "0"}, args...)
	go func() {
		done <- run(ctx, full, noEnv, strings.NewReader(""), io.Discard, sw, func(a string) { addr <- a })
	}()
	select {
	case a := <-addr:
		base = "http://" + a
	case code := <-done:
		cancel()
		t.Fatalf("exited with %d: %s", code, errb.String())
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatalf("did not start: %s", errb.String())
	}
	return base, errb, func() {
		cancel()
		select {
		case code := <-done:
			if code != 0 {
				t.Errorf("exit code %d", code)
			}
		case <-time.After(10 * time.Second):
			t.Error("did not shut down")
		}
	}
}

func getJSON(t *testing.T, url string, into any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
}

type stateBackends struct {
	Backends []struct {
		ID      string `json:"id"`
		State   string `json:"state"`
		Slots   int    `json:"slots"`
		Digest  string `json:"digest"`
		Breaker string `json:"breaker"`
	} `json:"backends"`
}

// Without --backends: one backend, gpu-local, and the job log says so. This is
// the behaviour of before the pool.
func TestRunWithoutBackendsIsOneGpuLocal(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"version":"0.40.8"}`)
		case "/api/ps", "/api/tags":
			fmt.Fprint(w, `{"models":[]}`)
		default:
			io.WriteString(w, `{"response":"ok","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
		}
	}))
	defer up.Close()
	base, _, stop := startGateway(t, "--ollama", up.URL, "--gen-slots", "3")
	defer stop()
	var st stateBackends
	getJSON(t, base+"/monitor/api/state", &st)
	if len(st.Backends) != 1 || st.Backends[0].ID != "gpu-local" || st.Backends[0].Slots != 3 {
		t.Fatalf("backends %+v", st.Backends)
	}
	resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"x"}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", err, resp)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		h, err := http.Get(base + "/monitor/api/history")
		if err == nil {
			hb, _ := io.ReadAll(h.Body)
			h.Body.Close()
			if strings.Contains(string(hb), `"backend":"gpu-local"`) && !strings.Contains(string(hb), `"failover_rule":"F`) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the job never reached the history with backend=gpu-local")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunRefusesABadRegistry(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := map[string]string{
		"missing":  filepath.Join(dir, "nao-existe.json"),
		"unknown":  write("u.json", `{"backends":[{"id":"a","url":"http://10.0.0.1:11434","tipo":"gpu","nope":1}]}`, 0o600),
		"bad url":  write("b.json", `{"backends":[{"id":"a","url":"file:///etc/passwd","tipo":"gpu"}]}`, 0o600),
		"no token": write("t.json", `{"backends":[{"id":"a","url":"http://127.0.0.1:1","tipo":"gpu","token_file":"`+filepath.ToSlash(filepath.Join(dir, "nada.tok"))+`"}]}`, 0o600),
	}
	if runtime.GOOS != "windows" {
		cases["mode 0644"] = write("m.json", `{"backends":[]}`, 0o644)
	}
	for name, path := range cases {
		var out, errb bytes.Buffer
		code := run(context.Background(), []string{"--listen", "127.0.0.1:0", "--db", filepath.Join(t.TempDir(), "g.db"), "--gpu-interval", "0", "--backends", path},
			noEnv, strings.NewReader(""), &out, &errb, nil)
		if code != 2 || !strings.Contains(errb.String(), "mcpollama-gateway:") {
			t.Errorf("%s: exit %d, stderr %q", name, code, errb.String())
		}
	}
}

// The whole wiring with a primary and a secondary: the primary fails its health
// check, the job goes to the secondary with the secondary's own token, and the
// board, the job log and the events say so.
func TestRunWithASecondaryEndToEnd(t *testing.T) {
	var primaryDown atomic.Bool
	var primaryGens, secGens atomic.Int32
	var secAuth atomic.Value
	fake := func(sec bool) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			switch r.URL.Path {
			case "/api/version":
				if !sec && primaryDown.Load() {
					http.Error(w, "down", 500)
					return
				}
				fmt.Fprint(w, `{"version":"0.40.8"}`)
			case "/api/ps":
				fmt.Fprint(w, `{"models":[]}`)
			case "/api/tags":
				fmt.Fprint(w, `{"models":[{"name":"qwen3-ia","model":"qwen3-ia","size":1,"digest":"sha256-igual"}]}`)
			default:
				if sec {
					secGens.Add(1)
					secAuth.Store(r.Header.Get("Authorization"))
				} else {
					primaryGens.Add(1)
				}
				io.WriteString(w, `{"response":"ok","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
			}
		})
	}
	prim := httptest.NewServer(fake(false))
	defer prim.Close()
	secSrv := httptest.NewServer(fake(true))
	defer secSrv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "sec.token")
	os.WriteFile(tok, []byte("TOKEN-DO-SECUNDARIO\n"), 0o600)
	reg := filepath.Join(dir, "backends.json")
	os.WriteFile(reg, []byte(fmt.Sprintf(`{"backends":[{"id":"sec-ws01","url":%q,"tipo":"remoto","slots":2,"token_file":%q}]}`, secSrv.URL, filepath.ToSlash(tok))), 0o600)

	base, errb, stop := startGateway(t, "--ollama", prim.URL, "--backends", reg, "--health-interval", "40ms", "--gen-slots", "1")
	defer stop()
	var st stateBackends
	waitState := func(what string, ok func() bool) {
		t.Helper()
		dl := time.Now().Add(5 * time.Second)
		for time.Now().Before(dl) {
			st = stateBackends{}
			getJSON(t, base+"/monitor/api/state", &st)
			if ok() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("timeout waiting for %s: %+v\n%s", what, st.Backends, errb.String())
	}
	waitState("both backends known", func() bool {
		return len(st.Backends) == 2 && st.Backends[0].Digest != "" && st.Backends[1].Digest != ""
	})
	// F1: healthy primary, the job runs there
	post := func() {
		t.Helper()
		resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"x"}`))
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%v %v", err, resp)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	post()
	if primaryGens.Load() != 1 || secGens.Load() != 0 {
		t.Fatalf("primary %d secondary %d", primaryGens.Load(), secGens.Load())
	}
	// S3: the primary goes down
	primaryDown.Store(true)
	waitState("primary out", func() bool { return st.Backends[0].State == "fora" || st.Backends[0].State == "disjuntor_aberto" })
	post()
	if secGens.Load() != 1 {
		t.Fatalf("the job did not go to the secondary (primary %d secondary %d)", primaryGens.Load(), secGens.Load())
	}
	if a, _ := secAuth.Load().(string); a != "Bearer TOKEN-DO-SECUNDARIO" {
		t.Fatalf("secondary Authorization %q", a)
	}
	if strings.Contains(errb.String(), "TOKEN-DO-SECUNDARIO") {
		t.Fatal("the token is in the log")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		h, err := http.Get(base + "/monitor/api/history")
		hb := ""
		if err == nil {
			b, _ := io.ReadAll(h.Body)
			h.Body.Close()
			hb = string(b)
		}
		ev, err := http.Get(base + "/monitor/api/state")
		evs := ""
		if err == nil {
			b, _ := io.ReadAll(ev.Body)
			ev.Body.Close()
			evs = string(b)
		}
		if strings.Contains(hb, `"backend":"sec-ws01"`) && strings.Contains(hb, `"failover_rule":"F2:S3"`) && strings.Contains(evs, "backend_down") && strings.Contains(evs, "overload_on") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("history/events lack the secondary job:\n%s\n%s", hb, evs)
		}
		time.Sleep(30 * time.Millisecond)
	}
}
