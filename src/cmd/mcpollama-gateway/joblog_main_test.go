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
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// TestMain lets the test binary double as a fake nvidia-smi (GW_FAKE_SMI=1).
func TestMain(m *testing.M) {
	if os.Getenv("GW_FAKE_SMI") == "1" {
		fmt.Print("0, 00000000:03:00.0, 95.5, 12000, 80, 55, 100.00, 250.00, 1395\n1, 00000000:04:00.0, 90.0, 12000, 75, 54, 100.00, 250.00, 1400\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Fase 10.L end to end through run(): the job log with a fake nvidia-smi, a job
// from a dead process closed as interrupted, and the text only in content.db.
func TestRunJobLogGPUExtractAndRecovery(t *testing.T) {
	t.Setenv("GW_FAKE_SMI", "1")
	dir := t.TempDir()
	dbPath, contPath := filepath.Join(dir, "gw.db"), filepath.Join(dir, "content.db")

	// a job left in flight by a previous boot
	old, err := store.Open(dbPath, store.Options{BootID: "boot-que-morreu"})
	if err != nil {
		t.Fatal(err)
	}
	old.JobEnqueued(queue.Record{ID: strings.Repeat("a", 32), ClientIP: "10.0.0.2", ClientLabel: "antigo", Endpoint: "/api/generate", Model: "qwen3-ia",
		Lane: queue.LaneGen, PriorityIn: queue.Normal, PriorityFinal: queue.Normal, EnqueuedAt: time.Now().Add(-time.Minute), Status: queue.StatusQueued})
	old.JobStarted(strings.Repeat("a", 32), time.Now().Add(-50*time.Second), "gpu-local")
	old.Close()

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.URL.Path != "/api/generate" { // the model collector asks /api/ps and /api/tags
			io.WriteString(w, `{"models":[]}`)
			return
		}
		started <- struct{}{}
		<-release
		io.WriteString(w, `{"response":"pronto","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
	}))
	defer up.Close()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", dbPath, "--content-db", contPath,
			"--gpu-interval", "100ms", "--nvidia-smi", os.Args[0], "--aer-path", "-", "--proc-root", dir, "--sys-root", dir},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	var base string
	select {
	case a := <-addr:
		base = "http://" + a
	case <-time.After(5 * time.Second):
		t.Fatalf("did not start: %s", errb.String())
	}
	getJSON := func(path string, v any) {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			t.Fatal(err)
		}
	}
	var jobs struct{ Rows []store.JobRow }
	getJSON("/monitor/api/jobs", &jobs)
	if len(jobs.Rows) != 1 || jobs.Rows[0].Status != "interrompido_reboot" {
		t.Fatalf("the job of the dead boot must be interrupted: %+v", jobs.Rows)
	}

	idc := make(chan string, 1)
	go func() {
		resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"PROMPT-DO-MAIN"}`))
		if err != nil {
			idc <- ""
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		idc <- resp.Header.Get("X-Request-Id")
	}()
	<-started
	// wait (generously: a race-instrumented fake nvidia-smi starts slowly) until the
	// extract of the running job has a few seconds
	var live struct{ Rows []store.JobRow }
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		getJSON("/monitor/api/jobs?status=em_execucao", &live)
		if len(live.Rows) == 1 && live.Rows[0].Samples >= 3 {
			break
		}
	}
	if len(live.Rows) != 1 || live.Rows[0].Backend != "gpu-local" || live.Rows[0].Samples < 3 {
		t.Fatalf("running job not in the log with its extract: %+v", live.Rows)
	}
	close(release)
	id := <-idc
	deadline := time.Now().Add(3 * time.Second)
	var fin store.JobRow
	for time.Now().Before(deadline) {
		var d struct{ Job store.JobRow }
		getJSON("/monitor/api/jobs/"+id, &d)
		if fin = d.Job; fin.Status == "ok" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if fin.Status != "ok" || !fin.Responded || fin.Samples < 3 || fin.PeakClockMHz != 1400 || fin.PeakPowerW != 95.5 {
		t.Fatalf("extract/peaks wrong: %+v", fin)
	}
	n1 := fin.Samples
	time.Sleep(1500 * time.Millisecond) // collector keeps running; nothing may be added without a job
	var d2 struct{ Job store.JobRow }
	getJSON("/monitor/api/jobs/"+id, &d2)
	if d2.Job.Samples != n1 {
		t.Fatalf("rows added after the job ended: %d -> %d", n1, d2.Job.Samples)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no shutdown")
	}
	// the text is in content.db and in no metadata file
	cs, err := content.Open(contPath, content.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, ok, _ := cs.Get(context.Background(), id)
	cs.Close()
	if !ok || !strings.Contains(c.Prompt, "PROMPT-DO-MAIN") || c.Response != "pronto" {
		t.Fatalf("%v %+v", ok, c)
	}
	for _, suf := range []string{"", "-wal", "-shm"} {
		if b, err := os.ReadFile(dbPath + suf); err == nil && (bytes.Contains(b, []byte("PROMPT-DO-MAIN")) || bytes.Contains(b, []byte("pronto"))) {
			t.Fatalf("text in the metadata database%s", suf)
		}
	}
}

// --log-content=false: the job log stays, the text file is never created.
func TestRunWithoutContentLog(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"response":"ok","done":true,"eval_count":1}`+"\n")
	}))
	defer up.Close()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", filepath.Join(dir, "gw.db"), "--log-content=false", "--gpu-interval", "0", "--aer-path", "-"},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	base := "http://" + <-addr
	resp, err := http.Post(base+"/api/generate", "application/json", strings.NewReader(`{"model":"qwen3-ia","prompt":"SEGREDO"}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	time.Sleep(200 * time.Millisecond)
	if c, _ := http.Get(base + "/monitor/api/jobs/" + resp.Header.Get("X-Request-Id") + "/content"); c.StatusCode != 401 {
		t.Fatalf("content without session: %d", c.StatusCode)
	}
	cancel()
	<-done
	if _, err := os.Stat(filepath.Join(dir, "content.db")); err == nil {
		t.Fatal("content.db was created with --log-content=false")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "gw.db")); bytes.Contains(b, []byte("SEGREDO")) {
		t.Fatal("text in the metadata database")
	}
}
