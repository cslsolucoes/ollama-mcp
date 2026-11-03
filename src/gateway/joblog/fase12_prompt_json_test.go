package joblog_test

// Fase 12, etapa 12.3: end to end, through the real proxy, queue, store and
// content database, with a fake Ollama that records the exact body it receives.
// Contract C-2.1, C-2.2, C-2.5, C-2.10, C-2.11, C-4.2.

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/joblog"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

const e2eKeyHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func e2eKey(t *testing.T) *hmackey.Key {
	t.Helper()
	p := filepath.Join(t.TempDir(), "doc.key")
	if err := os.WriteFile(p, []byte(e2eKeyHex), 0o600); err != nil {
		t.Fatal(err)
	}
	k, err := hmackey.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// fakeOllama records the body and the headers of every call.
type fakeOllama struct {
	mu      sync.Mutex
	bodies  [][]byte
	headers []http.Header
}

func (f *fakeOllama) handler(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.bodies = append(f.bodies, b)
	f.headers = append(f.headers, r.Header.Clone())
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/x-ndjson")
	io.WriteString(w, `{"response":"ok","done":true,"prompt_eval_count":1,"eval_count":1}`+"\n")
}

func (f *fakeOllama) last(t *testing.T) ([]byte, http.Header) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) == 0 {
		t.Fatal("the Ollama got no call")
	}
	return f.bodies[len(f.bodies)-1], f.headers[len(f.headers)-1]
}

func (r *rig) postHdr(t *testing.T, path, body string, hdr map[string]string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", r.gw.URL+path, strings.NewReader(body))
	req.Header.Set(proxy.HeaderClient, "teste/1")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	id := resp.Header.Get(proxy.HeaderRequestID)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok, _ := r.st.Job(context.Background(), id); ok && j.Status != "em_execucao" && j.Status != "na_fila" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return id
}

func (r *rig) pj(t *testing.T, id string) []byte {
	t.Helper()
	p, ok, err := r.cs.GetPromptJSON(context.Background(), id)
	if err != nil || !ok || p.JSON == nil {
		t.Fatalf("prompt_json of %s: ok=%v err=%v", id, ok, err)
	}
	return p.JSON
}

func e2eRig(t *testing.T, f *fakeOllama, key *hmackey.Key) *rig {
	t.Helper()
	return newRig(t, f.handler, time.Second, func(c *proxy.Config) {
		c.StripOptions = proxy.DefaultStripOptions
		c.ThinkSuffix = "-think"
		c.DocKey = key
	})
}

// C-2.1, C-2.2, C-2.10: the prompt_json stored for the job is the body that
// reached the Ollama, byte for byte: the body AFTER the policy (num_gpu and
// keep_alive stripped, think decided), not what the client sent and not the
// flat text. /api/generate, /api/chat with a long history, /v1/chat/completions
// and the embeddings route.
func TestPromptJSONIsTheBodyTheOllamaReceived(t *testing.T) {
	cases := []struct{ name, path, body string }{
		{"generate", "/api/generate",
			`{"model":"qwen3-ia","system":"Seja breve.","prompt":"Resuma: áé \"x\"\n fim","stream":false,"keep_alive":0,"options":{"seed":3,"num_gpu":1,"temperature":0.1,"num_predict":64},"format":"json"}`},
		{"chat history", "/api/chat",
			`{"model":"qwen3-ia","messages":[{"role":"system","content":"S"},{"role":"user","content":"um"},{"role":"assistant","content":"dois"},{"role":"user","content":"tres 😀"},{"role":"assistant","content":"quatro"},{"role":"user","content":"cinco"}],"options":{"num_ctx":131072,"seed":1},"think":false}`},
		{"v1 chat", "/v1/chat/completions",
			`{ "model" : "qwen3-ia", "messages":[{"role":"system","content":"s"},{"role":"user","content":"u1"},{"role":"assistant","content":"a1"},{"role":"user","content":"u2"}], "temperature":0.2 , "max_tokens":50 }`},
		{"v1 completions", "/v1/completions", `{"model":"qwen3-ia","prompt":"era uma vez","max_tokens":10}`},
		{"embed", "/api/embed", `{"model":"nomic-embed-text","input":["a","b"]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeOllama{}
			r := e2eRig(t, f, e2eKey(t))
			id := r.postHdr(t, c.path, c.body, nil)
			received, _ := f.last(t)
			got := r.pj(t, id)
			if !bytes.Equal(got, received) {
				t.Fatalf("prompt_json differs from the body the Ollama received\nstored:   %s\nreceived: %s", got, received)
			}
			if c.name != "embed" && !strings.Contains(string(received), `"think"`) && !strings.Contains(string(received), `"reasoning_effort"`) {
				t.Fatalf("the fixture is wrong: the policy did not decide think: %s", received)
			}
			if strings.Contains(string(got), `"num_gpu"`) || strings.Contains(string(got), `"keep_alive"`) {
				t.Fatalf("the stored JSON still has what the policy removed: %s", got)
			}
			if strings.HasPrefix(string(got), "[") || strings.Contains(string(got), "[prompt]") || strings.Contains(string(got), "[user]") {
				t.Fatalf("the stored JSON looks like the flat text: %s", got)
			}
			// The flat text is still kept as before.
			if cont, ok, _ := r.cs.Get(context.Background(), id); !ok || cont.Prompt == "" {
				t.Fatal("the flat prompt is gone")
			}
			if p, _, _ := r.cs.GetPromptJSON(context.Background(), id); p.Truncated || p.HasImage {
				t.Fatalf("marks: %+v", p)
			}
		})
	}
}

// C-2.5, C-2.10: with images the stored JSON equals the received body except
// that each image is the marker; the Ollama still gets the real image; the
// job has tem_imagem=1.
func TestPromptJSONImagesBecomeHMACMarkers(t *testing.T) {
	key := e2eKey(t)
	cases := []struct{ name, path, body string }{
		{"generate", "/api/generate", `{"model":"qwen3-ia","prompt":"o que e?","images":["QUJDREVGRw==","SEVMTE8="],"stream":false}`},
		{"chat", "/api/chat", `{"model":"qwen3-ia","messages":[{"role":"user","content":"a","images":["QUJDREVGRw=="]},{"role":"assistant","content":"b"},{"role":"user","content":"c","images":["SEVMTE8="]}]}`},
		{"v1", "/v1/chat/completions", `{"model":"qwen3-ia","messages":[{"role":"user","content":[{"type":"text","text":"x"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDREVGRw=="}}]}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeOllama{}
			r := e2eRig(t, f, key)
			id := r.postHdr(t, c.path, c.body, nil)
			received, _ := f.last(t)
			if !strings.Contains(string(received), "QUJDREVGRw==") {
				t.Fatalf("the Ollama did not get the real image: %s", received)
			}
			got := r.pj(t, id)
			if strings.Contains(string(got), "QUJDREVGRw") || strings.Contains(string(got), "SEVMTE8") {
				t.Fatalf("image data in the stored JSON: %s", got)
			}
			if !strings.Contains(string(got), `{"imagem_hmac":"`) {
				t.Fatalf("no marker: %s", got)
			}
			// Put the images back into the stored JSON by hand and compare with
			// what the Ollama got: they must be identical.
			re := string(got)
			for _, img := range []string{"QUJDREVGRw==", "SEVMTE8="} {
				if c.name == "v1" {
					continue
				}
				re = strings.Replace(re, `{"imagem_hmac":"`+key.MACString(img)+`"}`, `"`+img+`"`, 1)
			}
			if c.name == "v1" {
				re = strings.Replace(re, `{"imagem_hmac":"`+key.MACString(`{"url":"data:image/png;base64,QUJDREVGRw=="}`)+`"}`, `{"url":"data:image/png;base64,QUJDREVGRw=="}`, 1)
			}
			if re != string(received) {
				t.Fatalf("outside the images the stored JSON differs from the received body\nrestored: %s\nreceived: %s", re, received)
			}
			p, _, _ := r.cs.GetPromptJSON(context.Background(), id)
			if !p.HasImage {
				t.Fatal("tem_imagem not set")
			}
			// The flat text never carries the image data, on any route (M5: /v1 parts too).
			if cont, _, _ := r.cs.Get(context.Background(), id); strings.Contains(cont.Prompt, "QUJDREVGRw") || strings.Contains(cont.Prompt, "SEVMTE8") {
				t.Fatal("image data in the flat text")
			}
		})
	}
}

// C-2.5: without a key the marker is {"imagem_hmac":""} and tem_imagem is 1.
func TestPromptJSONImageWithoutKey(t *testing.T) {
	f := &fakeOllama{}
	r := e2eRig(t, f, nil)
	id := r.postHdr(t, "/api/generate", `{"model":"qwen3-ia","prompt":"p","images":["QUJDREVGRw=="]}`, nil)
	got := string(r.pj(t, id))
	if !strings.Contains(got, `"images":[{"imagem_hmac":""}]`) || strings.Contains(got, "QUJD") {
		t.Fatalf("got %s", got)
	}
	if p, _, _ := r.cs.GetPromptJSON(context.Background(), id); !p.HasImage {
		t.Fatal("tem_imagem not set without a key")
	}
}

// C-2.11, C-4.2: the header is never forwarded and never in the stored JSON;
// requests.doc_hash is the HMAC with the key id; the plain value is nowhere in
// either database file.
func TestDocHeaderEndToEnd(t *testing.T) {
	key := e2eKey(t)
	f := &fakeOllama{}
	r := e2eRig(t, f, key)
	v := strings.Repeat("0123456789abcdef", 4)
	id := r.postHdr(t, "/api/generate", `{"model":"qwen3-ia","prompt":"p"}`, map[string]string{proxy.HeaderDoc: v})
	received, hdr := f.last(t)
	if hdr.Get(proxy.HeaderDoc) != "" {
		t.Fatal("X-Mcpollama-Doc reached the Ollama")
	}
	if strings.Contains(string(r.pj(t, id)), v) || strings.Contains(string(received), v) {
		t.Fatal("the header value is in a body")
	}
	h, k, err := r.st.JobDoc(context.Background(), id)
	if err != nil || h != key.MACString(v) || k != key.ID() {
		t.Fatalf("doc_hash=%q key_id=%q err=%v", h, k, err)
	}
	// Close both databases (checkpoint the WAL) and scan the files: the plain
	// sha256 must not be anywhere.
	r.cs.Close()
	r.st.Close()
	for _, p := range []string{r.gwPath, r.contDB, r.gwPath + "-wal", r.contDB + "-wal"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if bytes.Contains(b, []byte(v)) {
			t.Fatalf("the plain header value is in %s", filepath.Base(p))
		}
	}
}

// B1 end to end: a number the float parser refuses in an unknown field passes the
// policy and the Ollama gets the body intact, but it used to make the image scan
// give up and keep the body with the image in clear and tem_imagem=0. Now the
// number is only text: the image is replaced and the base64 is nowhere in
// content.db.
func TestHugeNumberDoesNotLeakTheImage(t *testing.T) {
	key := e2eKey(t)
	f := &fakeOllama{}
	r := e2eRig(t, f, key)
	body := `{"model":"qwen3-ia","prompt":"x","images":["SEGREDOBASE64"],"zz":1e400}`
	id := r.postHdr(t, "/api/generate", body, nil)
	received, _ := f.last(t)
	if !strings.Contains(string(received), "SEGREDOBASE64") || !strings.Contains(string(received), `1e400`) {
		t.Fatalf("the Ollama did not get the body intact: %s", received)
	}
	got := r.pj(t, id)
	if strings.Contains(string(got), "SEGREDOBASE64") || !strings.Contains(string(got), `"zz":1e400`) {
		t.Fatalf("prompt_json: %s", got)
	}
	if p, _, _ := r.cs.GetPromptJSON(context.Background(), id); !p.HasImage || p.Truncated {
		t.Fatalf("marks: %+v", p)
	}
	r.cs.Close()
	for _, p := range []string{r.contDB, r.contDB + "-wal"} {
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte("SEGREDOBASE64")) {
			t.Fatalf("the image text is in %s", filepath.Base(p))
		}
	}
}

// B1: if the scan cannot finish, nothing of the body is kept: prompt_json NULL,
// tem_imagem=1, prompt_json_truncated=1 (never exportable), and the image text is
// not in content.db either.
func TestScanFailureStoresNoBodyAndMarksTheJob(t *testing.T) {
	f := &fakeOllama{}
	r := e2eRig(t, f, e2eKey(t))
	id := "f12failclosed00000000000000000001"
	rec := queueRecord(id)
	r.jr.EnqueuedJSON(rec, "texto", proxy.PromptJSON{HasImage: true, Failed: true})
	p, ok, err := r.cs.GetPromptJSON(context.Background(), id)
	if err != nil || !ok || p.JSON != nil || !p.HasImage || !p.Truncated {
		t.Fatalf("%+v ok=%v err=%v", p, ok, err)
	}
}

// I2: with --log-content=false the journal says it does not want the JSON.
func TestJournalWithoutContentDoesNotWantTheJSON(t *testing.T) {
	r := newRig(t, (&fakeOllama{}).handler, time.Second)
	if !r.jr.WantsPromptJSON() {
		t.Fatal("a journal with content must want it")
	}
	off := &joblog.Journal{Store: r.st}
	if off.WantsPromptJSON() {
		t.Fatal("a journal without content must not want it")
	}
}

func queueRecord(id string) queue.Record {
	return queue.Record{ID: id, ClientIP: "10.0.0.2", ClientLabel: "t", Endpoint: "/api/generate", Model: "qwen3-ia", Lane: queue.LaneGen,
		PriorityIn: queue.Normal, PriorityFinal: queue.Normal, EnqueuedAt: time.Now(), Status: queue.StatusQueued}
}
