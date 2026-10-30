package store_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/joblog"
	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// Sentinel test of the pool (Fase 10.4): a job that is handed to a SECONDARY
// carries the secondary's token on the wire. Neither that token, nor the
// client's, nor the prompt and the answer may reach the metadata database
// (gateway.db, its -wal and -shm); the text lives only in content.db. The
// metadata does name the backend and the rule (positive control).
func TestPoolSentinelNeverReachesTheMetadataDatabase(t *testing.T) {
	const (
		promptSentinel = "SENTINELA-POOL-PROMPT-91ab77c0-peticao"
		answerSentinel = "SENTINELA-POOL-RESPOSTA-3be0d1f2-sentenca"
		clientToken    = "SENTINELA-POOL-TOKEN-CLIENTE-5d2f"
		secToken       = "SENTINELA-POOL-TOKEN-SECUNDARIO-8a1c"
	)
	dir := t.TempDir()
	dbPath, contentPath := filepath.Join(dir, "gw.db"), filepath.Join(dir, "content.db")
	st, err := store.Open(dbPath, store.Options{BootID: "boot-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cs, err := content.Open(contentPath, content.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	jr := &joblog.Journal{Store: st, Content: cs}

	var secAuth atomic.Value
	var primaryDown atomic.Bool
	handler := func(sec bool) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/version":
				if !sec && primaryDown.Load() {
					http.Error(w, "down", 500) // the primary is down: S3 sends the job to the secondary
					return
				}
				fmt.Fprint(w, `{"version":"0.40.8"}`)
			case "/api/ps":
				fmt.Fprint(w, `{"models":[]}`)
			case "/api/tags":
				fmt.Fprint(w, `{"models":[{"name":"qwen3-ia","model":"qwen3-ia","size":1,"digest":"sha256-same"}]}`)
			default:
				io.Copy(io.Discard, r.Body)
				if sec {
					secAuth.Store(r.Header.Get("Authorization"))
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				io.WriteString(w, `{"response":"`+answerSentinel+`","done":false}`+"\n")
				io.WriteString(w, `{"response":"","done":true,"prompt_eval_count":12,"eval_count":34}`+"\n")
			}
		})
	}
	prim := httptest.NewServer(handler(false))
	defer prim.Close()
	secSrv := httptest.NewServer(handler(true))
	defer secSrv.Close()
	tokFile := filepath.Join(dir, "sec.token")
	if err := os.WriteFile(tokFile, []byte(secToken), 0o600); err != nil {
		t.Fatal(err)
	}
	secs, err := pool.Build([]pool.Spec{{ID: "sec-ws01", URL: secSrv.URL, Tipo: "remoto", Slots: 1, TokenFile: tokFile}}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	pu, _ := url.Parse(prim.URL)
	pl := pool.New(pool.Config{PrimarySlots: 1, PrimaryURL: pu, Secondaries: secs})
	for _, b := range pl.Backends() {
		pl.CheckOnce(context.Background(), b)
	}
	primaryDown.Store(true) // known digests from the first round, then the primary fails its check
	pl.CheckOnce(context.Background(), pl.Primary())
	q := queue.New(queue.Config{Pool: pl, OnRecord: jr.Finish})
	jr.Running = q.RunningIDs
	h, err := proxy.New(proxy.Config{Upstream: pu, Queue: q, Journal: jr, Pool: pl})
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(h)
	defer gw.Close()

	req, _ := http.NewRequest("POST", gw.URL+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"`+promptSentinel+`"}`))
	req.Header.Set("Authorization", "Bearer "+clientToken)
	req.Header.Set(proxy.HeaderClient, "CONTROLE-POOL-label")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(got, []byte(answerSentinel)) {
		t.Fatalf("the client did not get the answer: %s", got)
	}
	if a, _ := secAuth.Load().(string); a != "Bearer "+secToken {
		t.Fatalf("the secondary got Authorization %q", a)
	}
	id := resp.Header.Get(proxy.HeaderRequestID)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st.Flush()
		if j, ok, _ := st.Job(context.Background(), id); ok && j.Status == "ok" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	j, ok, err := st.Job(context.Background(), id)
	if err != nil || !ok || j.Backend != "sec-ws01" || j.FailoverRule != "F2:S3" {
		t.Fatalf("the metadata must name the backend and the rule: %v %v %+v", err, ok, j.Row)
	}
	scan := func(label string) {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			for _, f := range []string{dbPath, contentPath} {
				b, err := os.ReadFile(f + suffix)
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					t.Fatal(err)
				}
				banned := []string{clientToken, secToken}
				if f == dbPath {
					banned = append(banned, promptSentinel, answerSentinel)
				}
				for _, s := range banned {
					if bytes.Contains(b, []byte(s)) {
						t.Fatalf("%s: sentinel %q found in %s%s", label, s, filepath.Base(f), suffix)
					}
				}
			}
		}
	}
	scan("open")
	// positive controls: the scan can see the label (metadata) and the prompt (content)
	seen := func(path string, needle string) bool {
		for _, suffix := range []string{"", "-wal"} {
			if b, err := os.ReadFile(path + suffix); err == nil && bytes.Contains(b, []byte(needle)) {
				return true
			}
		}
		return false
	}
	if !seen(dbPath, "sec-ws01") || !seen(dbPath, "CONTROLE-POOL-label") || !seen(contentPath, promptSentinel) {
		t.Fatal("control strings not found: the scan is blind")
	}
	// a board snapshot, a backend sample and an event hold no token either
	st.Backends([]store.BackendRow{{TS: time.Now(), BackendID: "sec-ws01", State: "ok"}})
	for _, s := range q.PoolStatus() {
		if b, _ := fmt.Sprintf("%+v", s), 0; strings.Contains(b, secToken) {
			t.Fatalf("token in the board: %s", b)
		}
	}
	st.Flush()
	st.Close()
	cs.Close()
	scan("closed")
}
