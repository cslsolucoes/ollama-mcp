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
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/joblog"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// Sentinel test (plan section 5, adapted in Fase 10.L by owner decision): a
// prompt, an answer, a token and a system text with unique strings go through
// the proxy with the job log ON. The prompt, the answer and the system text
// may appear ONLY in the content database (content.db); none of them may
// appear in the bytes of the metadata .db, its -wal or its -shm. The token
// (a header, never part of the body) may appear nowhere. Control strings that
// are stored prove both scans can see data: the client label in the metadata
// file, and the prompt in the content file.
func TestSentinelNeverReachesTheDatabase(t *testing.T) {
	const (
		promptSentinel = "SENTINELA-PROMPT-7f3a9c1e-contrato-sigiloso"
		answerSentinel = "SENTINELA-RESPOSTA-b41d52aa-parecer-privado"
		tokenSentinel  = "SENTINELA-TOKEN-0e9d77c3-bearer"
		systemSentinel = "SENTINELA-SYSTEM-55c0de11"
		labelControl   = "CONTROLE-LABEL-visivel"
	)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gw.db")
	st, err := store.Open(dbPath, store.Options{BootID: "boot-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	contentPath := filepath.Join(dir, "content.db")
	cs, err := content.Open(contentPath, content.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	jr := &joblog.Journal{Store: st, Content: cs}

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"response":"`+answerSentinel+`","done":false}`+"\n")
		io.WriteString(w, `{"response":"","done":true,"total_duration":5,"load_duration":1000000,"prompt_eval_count":12,"eval_count":34}`+"\n")
	}))
	defer up.Close()
	uu, _ := url.Parse(up.URL)
	q := queue.New(queue.Config{OnRecord: jr.Finish})
	jr.Running = q.RunningIDs
	h, err := proxy.New(proxy.Config{Upstream: uu, Queue: q, Journal: jr})
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(h)
	defer gw.Close()

	body := `{"model":"qwen3-ia","prompt":"` + promptSentinel + `","system":"` + systemSentinel + `"}`
	req, _ := http.NewRequest("POST", gw.URL+"/api/generate", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenSentinel)
	req.Header.Set(proxy.HeaderClient, labelControl)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(got, []byte(answerSentinel)) {
		t.Fatal("the client did not get the answer")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st.Flush()
		if rows, _ := st.History(context.Background(), store.HistoryQuery{}); len(rows) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if rows, _ := st.History(context.Background(), store.HistoryQuery{}); len(rows) != 1 || rows[0].Eval != 34 {
		t.Fatalf("record not stored: %+v", rows)
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
				// the token is in neither file; the texts are not in the metadata file
				banned := []string{tokenSentinel}
				if f == dbPath {
					banned = append(banned, promptSentinel, answerSentinel, systemSentinel)
				}
				for _, s := range banned {
					if bytes.Contains(b, []byte(s)) {
						t.Fatalf("%s: sentinel %q found in %s%s", label, s, filepath.Base(f), suffix)
					}
				}
			}
		}
	}
	// the texts DO live in the content database (read through the API and raw)
	cj, ok, err := cs.Get(context.Background(), resp.Header.Get(proxy.HeaderRequestID))
	if err != nil || !ok || !strings.Contains(cj.Prompt, promptSentinel) || !strings.Contains(cj.Prompt, systemSentinel) || !strings.Contains(cj.Response, answerSentinel) {
		t.Fatalf("content.db lacks the job text: %v %v %+v", err, ok, cj)
	}
	scan("open")
	// Positive control: the label (metadata) must be in the .db or the -wal.
	found := false
	for _, suffix := range []string{"", "-wal"} {
		if b, err := os.ReadFile(dbPath + suffix); err == nil && bytes.Contains(b, []byte(labelControl)) {
			found = true
		}
	}
	if !found {
		t.Fatal("control string not found: the scan cannot see stored data")
	}
	// positive control of the content scan: the prompt is in content.db or its -wal
	foundC := false
	for _, suffix := range []string{"", "-wal"} {
		if b, err := os.ReadFile(contentPath + suffix); err == nil && bytes.Contains(b, []byte(promptSentinel)) {
			foundC = true
		}
	}
	if !foundC {
		t.Fatal("content control not found: the content scan is blind")
	}
	st.Close()
	cs.Close()
	scan("closed")
}
