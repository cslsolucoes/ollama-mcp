package store_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// Fase 9.6a: the sentinel stays out of the .db, the -wal, the log and the
// error answers with the policies on, including the rewritten body path, the
// "-think" virtual name and the 400 of an invalid body.
func TestSentinelSurvivesThePolicies(t *testing.T) {
	const (
		ok1 = "SENTINELA-POL-A-3d91e0b7-prompt-reescrito"
		ok2 = "SENTINELA-POL-B-a2c4f866-chat-v1"
		bad = "SENTINELA-POL-C-17be9d02-json-invalido"
		ctl = "CONTROLE-POLITICA-visivel"
	)
	dbPath := filepath.Join(t.TempDir(), "gw.db")
	st, err := store.Open(dbPath, store.Options{BootID: "boot-pol"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"response":"","done":true,"prompt_eval_count":1,"eval_count":2}`+"\n")
	}))
	defer up.Close()
	uu, _ := url.Parse(up.URL)
	var mu sync.Mutex
	var logs []string
	q := queue.New(queue.Config{OnRecord: st.Record})
	h, err := proxy.New(proxy.Config{
		Upstream: uu, Queue: q, AllowedModels: []string{"qwen3-ia"},
		ThinkSuffix: "-think", StripOptions: proxy.DefaultStripOptions,
		Logf: func(f string, a ...any) { mu.Lock(); logs = append(logs, f); mu.Unlock() },
	})
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(h)
	defer gw.Close()

	var answers []string
	do := func(path, body string) {
		req, _ := http.NewRequest("POST", gw.URL+path, strings.NewReader(body))
		req.Header.Set(proxy.HeaderClient, ctl)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		answers = append(answers, resp.Status+" "+string(b))
	}
	do("/api/generate", `{"model":"qwen3-ia-think","prompt":"`+ok1+`","keep_alive":0,"options":{"num_ctx":9}}`)
	do("/v1/chat/completions", `{"model":"qwen3-ia","messages":[{"role":"user","content":"`+ok2+`"}]}`)
	do("/api/generate", `{"model":"qwen3-ia","prompt":"`+bad+`"`)                 // truncated: 400
	do("/api/generate", `{"model":"foo-`+bad+`-think","prompt":"x"}`)             // not allowed: 403
	do("/api/chat", `{"model":"qwen3-ia","Keep_Alive":1,"keep_alive":"`+bad+`"}`) // case twins: 400

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st.Flush()
		if rows, _ := st.History(context.Background(), store.HistoryQuery{}); len(rows) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	rows, _ := st.History(context.Background(), store.HistoryQuery{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 stored requests, got %d", len(rows))
	}
	modes := map[string]bool{}
	for _, r := range rows {
		modes[r.Think+"/"+r.Model] = true
	}
	if !modes["on/qwen3-ia"] || !modes["off/qwen3-ia"] {
		t.Fatalf("think/model not stored as expected: %v", modes)
	}

	mu.Lock()
	logText := strings.Join(logs, "\n")
	mu.Unlock()
	for _, s := range []string{ok1, ok2, bad} {
		if strings.Contains(logText, s) || strings.Contains(strings.Join(answers, "\n"), s) && s != bad {
			t.Fatalf("sentinel %q leaked into the log or an answer", s)
		}
	}
	// 403 echoes the (sanitised) model name by design; the 400s must not echo anything.
	for _, a := range answers {
		if strings.HasPrefix(a, "400") && strings.Contains(a, bad) {
			t.Fatalf("a 400 echoed the body: %s", a)
		}
	}
	scan := func(label string) {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			b, err := os.ReadFile(dbPath + suffix)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				t.Fatal(err)
			}
			for _, s := range []string{ok1, ok2, bad} {
				if bytes.Contains(b, []byte(s)) {
					t.Fatalf("%s: sentinel %q in %s%s", label, s, filepath.Base(dbPath), suffix)
				}
			}
		}
	}
	scan("open")
	found := false
	for _, suffix := range []string{"", "-wal"} {
		if b, err := os.ReadFile(dbPath + suffix); err == nil && bytes.Contains(b, []byte(ctl)) {
			found = true
		}
	}
	if !found {
		t.Fatal("control string not found: the scan is blind")
	}
	st.Close()
	scan("closed")
}
