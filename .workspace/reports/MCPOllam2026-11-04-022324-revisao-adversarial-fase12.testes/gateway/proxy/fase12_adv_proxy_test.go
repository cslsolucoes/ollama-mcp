package proxy

// Revisão adversarial da etapa 12.9 (Fase 12), revisor P: pedido canônico (prompt_json,
// imagem -> imagem_hmac, X-Mcpollama-Doc) e a melhora no proxy (busca pela raia de embed).
// Contrato C-2.5, C-2.10, C-2.10a, C-2.11, C-4.2, C-8.x. Cada vetor imprime uma linha VEC.
//
// Dependências de outros _test.go do pacote: setup/env/post/postBg/finalChunk (proxy_test.go),
// capture/defaultPolicy (policy_test.go), testKey/testKeyHex (promptjson_test.go),
// newFakeOllama/withMelhora/fileReq/exampleIndex (melhora_test.go).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/ollama"
)

func vec12P(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res == "FALHA" {
		t.Errorf("%s: %s", id, desc)
	}
}

// adv12PImg returns a valid base64 image payload that is unique to tag.
func adv12PImg(tag string) string {
	return base64.StdEncoding.EncodeToString([]byte("PNGDATA-" + tag + "-" + strings.Repeat("z", 24)))
}

// adv12POllamaView decodes body the way the Ollama's Go structs do (case-folded field
// names, last duplicate wins) and lists the images it would hand to the model.
func adv12POllamaView(path string, body []byte) []string {
	var out []string
	switch path {
	case "/api/generate":
		var r struct {
			Images []string `json:"images"`
		}
		if json.Unmarshal(body, &r) == nil {
			out = append(out, r.Images...)
		}
	case "/api/chat":
		var r struct {
			Messages []struct {
				Images []string `json:"images"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &r) == nil {
			for _, m := range r.Messages {
				out = append(out, m.Images...)
			}
		}
	case "/v1/chat/completions":
		var r struct {
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &r) == nil {
			for _, m := range r.Messages {
				parts, _ := m.Content.([]any)
				for _, p := range parts {
					pm, _ := p.(map[string]any)
					if pm == nil || pm["type"] != "image_url" {
						continue
					}
					switch v := pm["image_url"].(type) {
					case string:
						out = append(out, v)
					case map[string]any:
						if s, ok := v["url"].(string); ok {
							out = append(out, s)
						}
					}
				}
			}
		}
	}
	return out
}

// adv12PPayload is the base64 part of an image as the Ollama sees it (data URI cut).
func adv12PPayload(img string) string {
	if i := strings.Index(img, "base64,"); i >= 0 {
		return img[i+len("base64,"):]
	}
	return img
}

// P1: every image the Ollama would see is replaced in the prompt_json and absent from the
// flat text, whatever the spelling of the keys (Unicode folding, escapes, duplicates).
func TestAdv12PImagesNeverInClear(t *testing.T) {
	k := testKey(t)
	type tc struct {
		id, path, body string
	}
	I := adv12PImg
	cases := []tc{
		{"P1.gen.plain", "/api/generate", `{"model":"m","prompt":"p","images":["` + I("a") + `"]}`},
		{"P1.gen.upper", "/api/generate", `{"model":"m","IMAGES":["` + I("b") + `"]}`},
		{"P1.gen.longS", "/api/generate", `{"model":"m","imageſ":["` + I("c") + `"]}`},
		{"P1.gen.longS.esc", "/api/generate", `{"model":"m","imageſ":["` + I("d") + `"]}`},
		{"P1.gen.esc.i", "/api/generate", `{"model":"m","images":["` + I("e") + `"]}`},
		{"P1.gen.esc.all", "/api/generate", `{"model":"m","IMAGES":["` + I("f") + `"]}`},
		{"P1.gen.dotlessI", "/api/generate", `{"model":"m","ımages":["` + I("g") + `"]}`},
		{"P1.gen.dottedI", "/api/generate", `{"model":"m","İmages":["` + I("h") + `"]}`},
		{"P1.gen.dup.last", "/api/generate", `{"model":"m","images":["` + I("i1") + `"],"images":["` + I("i2") + `"]}`},
		{"P1.gen.dup.case", "/api/generate", `{"model":"m","Images":["` + I("j1") + `"],"imageS":["` + I("j2") + `"]}`},
		{"P1.gen.escval", "/api/generate", `{"model":"m","images":["` + strings.ReplaceAll(I("k"), "A", `A`) + `"]}`},
		{"P1.gen.hugeNums", "/api/generate", `{"model":"m","options":{"x":1e400,"y":` + strings.Repeat("9", 300) + `,"z":-0.` + strings.Repeat("1", 300) + `e-999},"images":["` + I("l") + `"]}`},
		{"P1.gen.unknownNested", "/api/generate", `{"model":"m","zz":{"images":["ignored"],"deep":[[[{"messages":[{"images":["x"]}]}]]]},"images":["` + I("m") + `"]}`},
		{"P1.chat.history", "/api/chat", `{"model":"m","messages":[{"role":"user","content":"a","images":["` + I("n1") + `"]},{"role":"assistant","content":"b"},{"role":"user","content":"c","images":["` + I("n2") + `","` + I("n3") + `"]}]}`},
		{"P1.chat.foldedKeys", "/api/chat", `{"model":"m","meſſageſ":[{"role":"user","content":"a","IMAGEſ":["` + I("o") + `"]}]}`},
		{"P1.chat.dupMessages", "/api/chat", `{"model":"m","messages":[{"role":"user","content":"decoy"}],"Messages":[{"role":"user","content":"x","images":["` + I("p") + `"]}]}`},
		{"P1.chat.escKeys", "/api/chat", `{"model":"m","messages":[{"role":"user","images":["` + I("q") + `"]}]}`},
		{"P1.v1.dataURIobj", "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"veja"},{"type":"image_url","image_url":{"url":"data:image/png;base64,` + I("r") + `"}}]}]}`},
		{"P1.v1.dataURIstr", "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":"data:image/jpeg;base64,` + I("s") + `"}]}]}`},
		{"P1.v1.httpURL", "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://exemplo.invalid/` + I("t") + `.png"}}]}]}`},
		{"P1.v1.CONTENT", "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","CONTENT":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + I("u") + `"}}]}]}`},
		{"P1.v1.history", "/v1/chat/completions", `{"model":"m","messages":[{"role":"system","content":"s"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + I("v1") + `"}}]},{"role":"assistant","content":"ok"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + I("v2") + `"}}]}]}`},
	}
	for _, c := range cases {
		body := []byte(c.body)
		if !json.Valid(body) {
			t.Fatalf("%s: fixture is not JSON", c.id)
		}
		seen := adv12POllamaView(c.path, body)
		pj := buildPromptJSON(body, k)
		flat := extractPrompt(routeKinds[c.path], body, k)
		var leaks []string
		for _, img := range seen {
			p := adv12PPayload(img)
			if bytes.Contains(pj.JSON, []byte(p)) {
				leaks = append(leaks, "prompt_json")
			}
			if strings.Contains(flat, p) {
				leaks = append(leaks, "texto")
			}
		}
		switch {
		case len(seen) == 0 && (c.id == "P1.gen.dotlessI" || c.id == "P1.gen.dottedI"):
			// the Ollama's decoder does not fold ı/İ into i (encoding/json foldRune = unicode.SimpleFold):
			// not an image for it; the gateway is consistent if it also leaves it (no false positive needed)
			vec12P(t, c.id, "OK", fmt.Sprintf("chave com ı/İ (U+0131/U+0130) não é 'images' para o decoder Go do Ollama (0 imagens vistas, foldRune=SimpleFold); varredura consistente com ele (tem_imagem=%v)", pj.HasImage))
		case len(seen) == 0:
			vec12P(t, c.id, "FALHA", "Menor fixture: o decoder Go não viu imagem nenhuma, vetor não exercitado")
		case len(leaks) > 0 || !pj.HasImage || pj.Failed:
			vec12P(t, c.id, "FALHA", fmt.Sprintf("Bloqueador gateway/proxy/promptjson.go:190 imagem em claro (%v) tem_imagem=%v failed=%v", leaks, pj.HasImage, pj.Failed))
		default:
			vec12P(t, c.id, "OK", fmt.Sprintf("%d imagem(ns) vista(s) pelo decoder Go trocada(s) por imagem_hmac no prompt_json e ausentes do texto achatado", len(seen)))
		}
	}
}

// adv12PJournal records what the proxy hands to the journal.
type adv12PJournal struct {
	mu    sync.Mutex
	calls int
	texts []string
	pjs   []PromptJSON
}

func (j *adv12PJournal) Enqueued(rec queue.Record, prompt string) {
	j.mu.Lock()
	j.calls++
	j.texts = append(j.texts, prompt)
	j.mu.Unlock()
}
func (j *adv12PJournal) EnqueuedJSON(rec queue.Record, prompt string, pj PromptJSON) {
	j.mu.Lock()
	j.calls++
	j.texts = append(j.texts, prompt)
	j.pjs = append(j.pjs, pj)
	j.mu.Unlock()
}
func (j *adv12PJournal) WantsPromptJSON() bool                           { return true }
func (j *adv12PJournal) Started(id string, at time.Time, backend string) {}
func (j *adv12PJournal) Response(id string) ResponseSink                 { return nil }

// P1: bodies the Ollama's decoder might read differently (BOM, junk after the object,
// comments, NaN, two objects) never reach the Ollama nor the journal with the image.
func TestAdv12PMalformedBodiesNeverStored(t *testing.T) {
	k := testKey(t)
	cp := &capture{}
	jr := &adv12PJournal{}
	e := setup(t, cp, queue.Config{}, func(c *Config) { defaultPolicy(c); c.DocKey = k; c.Journal = jr })
	img := adv12PImg("malf")
	obj := `{"model":"qwen3-ia","prompt":"p","images":["` + img + `"]}`
	cases := map[string]string{
		"P1.bom":        string([]byte{0xEF, 0xBB, 0xBF}) + obj,
		"P1.lixoDepois": obj + `{"images":["` + img + `"]}`,
		"P1.lixoTexto":  obj + "xyz",
		"P1.comentario": `{"model":"qwen3-ia",/*c*/"images":["` + img + `"]}`,
		"P1.nan":        `{"model":"qwen3-ia","options":{"x":NaN},"images":["` + img + `"]}`,
		"P1.infinito":   `{"model":"qwen3-ia","options":{"x":Infinity},"images":["` + img + `"]}`,
	}
	for id, b := range cases {
		before := cp.calls.Load()
		jr.mu.Lock()
		jb := jr.calls
		jr.mu.Unlock()
		resp, _ := post(t, e, "/api/generate", b, nil)
		jr.mu.Lock()
		ja := jr.calls
		jr.mu.Unlock()
		if resp.StatusCode == 400 && cp.calls.Load() == before && ja == jb {
			vec12P(t, id, "OK", "corpo recusado com 400 pela política (json.Unmarshal estrito): não chega ao Ollama nem ao job log")
		} else {
			vec12P(t, id, "FALHA", fmt.Sprintf("Importante gateway/proxy/policy.go:131 corpo malformado aceito: status %d, ollama %d, journal %d", resp.StatusCode, cp.calls.Load()-before, ja-jb))
		}
	}
	// a valid body with an image: the journal gets the marker, the Ollama gets the image (control)
	resp, _ := post(t, e, "/api/generate", obj, nil)
	jr.mu.Lock()
	var pj PromptJSON
	var txt string
	if len(jr.pjs) > 0 {
		pj, txt = jr.pjs[len(jr.pjs)-1], jr.texts[len(jr.texts)-1]
	}
	jr.mu.Unlock()
	if resp.StatusCode == 200 && strings.Contains(cp.last(t), img) && !bytes.Contains(pj.JSON, []byte(img)) && !strings.Contains(txt, img) && pj.HasImage {
		vec12P(t, "P1.controle", "OK", "corpo válido: Ollama recebe a imagem; job log recebe só o marcador (prompt_json e texto)")
	} else {
		vec12P(t, "P1.controle", "FALHA", fmt.Sprintf("Bloqueador gateway/proxy/proxy.go:527 controle: status %d hasImage=%v", resp.StatusCode, pj.HasImage))
	}
}

// adv12PMeasure runs f and returns the time and the bytes allocated.
func adv12PMeasure(f func()) (time.Duration, uint64, uint64) {
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	var peak atomic.Uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				runtime.ReadMemStats(&m)
				if m.HeapInuse > peak.Load() {
					peak.Store(m.HeapInuse)
				}
			}
		}
	}()
	t0 := time.Now()
	f()
	d := time.Since(t0)
	close(stop)
	<-done
	runtime.ReadMemStats(&b)
	if b.HeapInuse > peak.Load() {
		peak.Store(b.HeapInuse)
	}
	return d, b.TotalAlloc - a.TotalAlloc, peak.Load()
}

// P2: the scan stays linear and bounded at the largest body the gateway accepts (32 MiB),
// for the shapes the 12.3 fix did not measure.
func TestAdv12PScanCostWorstCases(t *testing.T) {
	if testing.Short() {
		t.Skip("medição")
	}
	k := testKey(t)
	const size = 30 << 20
	pol := newPolicyCfg(Config{ThinkSuffix: "-think", StripOptions: DefaultStripOptions})
	img := adv12PImg("dos")
	type shape struct {
		id   string
		body func() []byte
	}
	fill := func(prefix, unit, suffix string) []byte {
		var b bytes.Buffer
		b.Grow(size + 64)
		b.WriteString(prefix)
		first := true
		for b.Len() < size {
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteString(unit)
		}
		b.WriteString(suffix)
		return b.Bytes()
	}
	shapes := []shape{
		{"P2.tokens", func() []byte { return fill(`{"model":"m","prompt":"p","zz":[`, `0`, `],"images":["`+img+`"]}`) }},
		{"P2.irmaosImages", func() []byte {
			return fill(`{"model":"m","messages":[`, `{"images":[]}`, `,{"role":"user","images":["`+img+`"]}]}`)
		}},
		{"P2.quaseImages", func() []byte {
			return fill(`{"model":"m","messages":[`, `{"imagez":["x"],"imageſ":[],"IMAGES":[]}`, `,{"images":["`+img+`"]}]}`)
		}},
		{"P2.escapesImagem", func() []byte {
			var b bytes.Buffer
			b.WriteString(`{"model":"m","images":["`)
			for b.Len() < size {
				b.WriteString(`A\uD800\n`)
			}
			b.WriteString(`"]}`)
			return b.Bytes()
		}},
		{"P2.chaveEnorme", func() []byte {
			return []byte(`{"model":"m","messages":[{"` + strings.Repeat("k", size) + `":1,"images":["` + img + `"]}]}`)
		}},
		{"P2.profundo", func() []byte {
			const d = 9990
			return []byte(`{"model":"m","zz":` + strings.Repeat("[", d) + strings.Repeat("]", d) + `,"messages":[{"images":["` + img + `"]}]}`)
		}},
		{"P2.profundoNaRegra", func() []byte {
			// deep nesting INSIDE a path whose rules are alive (messages/*/content/*)
			const d = 9980
			return []byte(`{"model":"m","messages":[{"content":[` + strings.Repeat("[", d) + strings.Repeat("]", d) + `,{"image_url":"` + img + `"}]}]}`)
		}},
	}
	for _, s := range shapes {
		body := s.body()
		if _, perr := pol.applyPolicy("/api/chat", body, ""); perr != nil {
			vec12P(t, s.id, "OK", fmt.Sprintf("MEDIDA corpo recusado pela política (%s) antes da varredura", perr.msg))
			continue
		}
		dPol, aPol, _ := adv12PMeasure(func() { _, _ = pol.applyPolicy("/api/chat", body, "") })
		var pj PromptJSON
		d, alloc, peak := adv12PMeasure(func() { pj = buildPromptJSON(body, k) })
		leak := bytes.Contains(pj.JSON, []byte(img))
		desc := fmt.Sprintf("MEDIDA %d MiB: varredura %s, %d MiB alocados, pico heap %d MiB (política: %s, %d MiB) failed=%v imagem=%v",
			len(body)>>20, d.Round(time.Millisecond), alloc>>20, peak>>20, dPol.Round(time.Millisecond), aPol>>20, pj.Failed, pj.HasImage)
		switch {
		case leak:
			vec12P(t, s.id, "FALHA", "Bloqueador gateway/proxy/promptjson.go:190 imagem em claro: "+desc)
		case d > 20*time.Second || peak > 1500<<20:
			vec12P(t, s.id, "FALHA", "Importante gateway/proxy/promptjson.go:190 custo fora do limite (20 s / 1,5 GiB): "+desc)
		default:
			vec12P(t, s.id, "OK", desc)
		}
	}
}

// ---- P3: X-Mcpollama-Doc never reaches an Ollama ---------------------------------------------

// adv12PTapListener records every byte the fake Ollama reads.
type adv12PTapListener struct {
	net.Listener
	mu  sync.Mutex
	raw bytes.Buffer
}

type adv12PTapConn struct {
	net.Conn
	l *adv12PTapListener
}

func (c *adv12PTapConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.l.mu.Lock()
		c.l.raw.Write(p[:n])
		c.l.mu.Unlock()
	}
	return n, err
}

func (l *adv12PTapListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &adv12PTapConn{Conn: c, l: l}, nil
}

func (l *adv12PTapListener) bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.raw.Bytes()...)
}

// adv12PRawOllama is a fake Ollama whose raw input is recorded (headers, body, trailers).
func adv12PRawOllama(t *testing.T) (*url.URL, *adv12PTapListener) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tap := &adv12PTapListener{Listener: ln}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		switch {
		case strings.Contains(r.URL.Path, "embed"):
			io.WriteString(w, `{"model":"nomic-embed-text","embeddings":[[1,0,0]],"embedding":[1,0,0],"data":[{"embedding":[1,0,0]}]}`)
		case strings.HasPrefix(r.URL.Path, "/v1/"):
			io.WriteString(w, `{"id":"x","choices":[{"message":{"content":"ok"},"text":"ok","finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
		default:
			io.WriteString(w, finalChunk+"\n")
		}
	})}
	go srv.Serve(tap)
	t.Cleanup(func() { srv.Close() })
	u, _ := url.Parse("http://" + ln.Addr().String())
	return u, tap
}

// adv12PRawPost writes a raw HTTP/1.1 request to addr and returns the status line.
func adv12PRawPost(t *testing.T, addr, req string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, req); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	resp, err := http.ReadResponse(bufio.NewReader(io.MultiReader(strings.NewReader(line), br)), nil)
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return strings.TrimSpace(line)
}

func TestAdv12PDocHeaderNeverReachesTheOllama(t *testing.T) {
	k := testKey(t)
	up, tap := adv12PRawOllama(t)
	q := queue.New(queue.Config{})
	var recMu sync.Mutex
	var recs []queue.Record
	_ = recs
	h, err := New(Config{Upstream: up, Queue: q, DocKey: k, ThinkSuffix: "-think", StripOptions: DefaultStripOptions})
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(h)
	t.Cleanup(gw.Close)
	addr := strings.TrimPrefix(gw.URL, "http://")
	routes := map[string]string{
		"/api/generate":        `{"model":"qwen3-ia","prompt":"p"}`,
		"/api/chat":            `{"model":"qwen3-ia","messages":[{"role":"user","content":"p"}]}`,
		"/v1/chat/completions": `{"model":"qwen3-ia","messages":[{"role":"user","content":"p"}]}`,
		"/v1/completions":      `{"model":"qwen3-ia","prompt":"p"}`,
		"/api/embed":           `{"model":"nomic-embed-text","input":"p"}`,
		"/api/embeddings":      `{"model":"nomic-embed-text","prompt":"p"}`,
		"/v1/embeddings":       `{"model":"nomic-embed-text","input":"p"}`,
		"/api/show":            `{"model":"qwen3-ia"}`,
	}
	n := 0
	for path, body := range routes {
		n++
		sha := sha256.Sum256([]byte(fmt.Sprintf("documento-%d-%s", n, path)))
		v := hex.EncodeToString(sha[:])
		variants := []struct {
			name string
			req  string
		}{
			{"minusculas", "POST " + path + " HTTP/1.1\r\nHost: x\r\nx-mcpollama-doc: " + v + "\r\nContent-Length: " + fmt.Sprint(len(body)) + "\r\n\r\n" + body},
			{"maiusculas", "POST " + path + " HTTP/1.1\r\nHost: x\r\nX-MCPOLLAMA-DOC: " + v + "\r\nContent-Length: " + fmt.Sprint(len(body)) + "\r\n\r\n" + body},
			{"duplicado", "POST " + path + " HTTP/1.1\r\nHost: x\r\nX-Mcpollama-Doc: " + v + "\r\nX-Mcpollama-Doc: " + v + "\r\nContent-Length: " + fmt.Sprint(len(body)) + "\r\n\r\n" + body},
			{"trailer", "POST " + path + " HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTrailer: X-Mcpollama-Doc\r\n\r\n" +
				fmt.Sprintf("%x\r\n%s\r\n0\r\nX-Mcpollama-Doc: %s\r\n\r\n", len(body), body, v)},
			{"trailerSemAnuncio", "POST " + path + " HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n" +
				fmt.Sprintf("%x\r\n%s\r\n0\r\nX-Mcpollama-Doc: %s\r\n\r\n", len(body), body, v)},
		}
		for _, vr := range variants {
			before := len(tap.bytes())
			status := adv12PRawPost(t, addr, vr.req)
			time.Sleep(30 * time.Millisecond)
			after := tap.bytes()
			got := after[before:]
			id := "P3." + strings.Trim(strings.ReplaceAll(path, "/", "."), ".") + "." + vr.name
			switch {
			case len(got) == 0:
				vec12P(t, id, "OK", fmt.Sprintf("pedido não chegou ao Ollama (%s): nada a vazar", status))
			case bytes.Contains(bytes.ToLower(got), []byte(v)):
				vec12P(t, id, "FALHA", fmt.Sprintf("Bloqueador gateway/proxy/proxy.go:205 o sha256 do X-Mcpollama-Doc chegou ao Ollama (%s)", status))
			default:
				extra := ""
				if bytes.Contains(bytes.ToLower(got), []byte("x-mcpollama-doc")) {
					extra = " (só o NOME do cabeçalho aparece, numa linha Trailer:, sem o valor)"
				}
				vec12P(t, id, "OK", fmt.Sprintf("Ollama recebeu %d bytes sem o valor do cabeçalho (%s)%s", len(got), status, extra))
			}
		}
	}
	recMu.Lock()
	recMu.Unlock()
}

// P3 (pool): a secondary over https negotiates HTTP/2 (DefaultTransport clone with
// ForceAttemptHTTP2, ollama.NewTLSTransport, the transport the pool gives its secondaries).
// HTTP/2 sends request trailers whatever the Content-Length: does a trailer X-Mcpollama-Doc
// that the client sent reach the secondary with its value?
func TestAdv12PDocTrailerOverHTTP2Backend(t *testing.T) {
	var mu sync.Mutex
	var seenTrailer, seenHeader []string
	var proto string
	be := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		mu.Lock()
		proto = r.Proto
		for k, v := range r.Trailer {
			seenTrailer = append(seenTrailer, k+"="+strings.Join(v, ","))
		}
		for k, v := range r.Header {
			seenHeader = append(seenHeader, k+"="+strings.Join(v, ","))
		}
		mu.Unlock()
		io.WriteString(w, finalChunk+"\n")
	}))
	be.EnableHTTP2 = true
	be.StartTLS()
	t.Cleanup(be.Close)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: be.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	tr, err := ollama.NewTLSTransport(caPath)
	if err != nil {
		t.Fatal(err)
	}
	tr.Proxy = nil // like pool/registry.go:367
	u, _ := url.Parse(be.URL)
	h, err := New(Config{Upstream: u, Queue: queue.New(queue.Config{}), ThinkSuffix: "-think", StripOptions: DefaultStripOptions})
	if err != nil {
		t.Fatal(err)
	}
	h.proxy.Transport = backendTransport{def: tr} // the RoundTrip a secondary gets (backendTransport -> b.Transport)
	gw := httptest.NewServer(h)
	t.Cleanup(gw.Close)
	sha := sha256.Sum256([]byte("doc-h2"))
	v := hex.EncodeToString(sha[:])
	body := `{"model":"qwen3-ia","prompt":"p"}`
	type tv struct{ name, announce, lines string }
	tvs := []tv{
		{"soDoc", "X-Mcpollama-Doc", "X-Mcpollama-Doc: " + v + "\r\n"},
		{"docEEstacao", "X-Mcpollama-Doc, X-Mcpollama-Station", "X-Mcpollama-Doc: " + v + "\r\nX-Mcpollama-Station: computer=PC9;user=eva\r\n"},
		{"comAuthorization", "X-Mcpollama-Doc, Authorization", "X-Mcpollama-Doc: " + v + "\r\nAuthorization: Bearer tokendocliente123\r\n"},
	}
	for _, path := range []string{"/api/generate", "/api/show"} {
		for _, tvar := range tvs {
			mu.Lock()
			seenTrailer, seenHeader, proto = nil, nil, ""
			mu.Unlock()
			status := adv12PRawPost(t, strings.TrimPrefix(gw.URL, "http://"), "POST "+path+" HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTrailer: "+tvar.announce+"\r\n\r\n"+
				fmt.Sprintf("%x\r\n%s\r\n0\r\n%s\r\n", len(body), body, tvar.lines))
			mu.Lock()
			all := strings.ToLower(strings.Join(append(append([]string{}, seenTrailer...), seenHeader...), "|"))
			trl := strings.ToLower(strings.Join(seenTrailer, "|"))
			p := proto
			mu.Unlock()
			id := "P3.pool.h2" + strings.ReplaceAll(path, "/", ".") + "." + tvar.name
			switch {
			case p == "":
				vec12P(t, id, "FALHA", "Menor fixture: o backend h2 não recebeu o pedido ("+status+")")
			case strings.Contains(all, v):
				vec12P(t, id, "FALHA", fmt.Sprintf("Importante gateway/proxy/proxy.go:571 (out := r.WithContext mantém r.Trailer preenchido pelo io.ReadAll da linha 460; o Rewrite da linha 205 apaga só pr.Out.Header) "+
					"o valor do X-Mcpollama-Doc enviado como TRAILER chegou ao backend %s (transporte do pool: ollama.NewTLSTransport, HTTP/2); trailers recebidos: %s", p, trl))
			default:
				vec12P(t, id, "OK", fmt.Sprintf("backend %s não viu o valor do X-Mcpollama-Doc (%s); trailers recebidos: %q", p, status, trl))
			}
		}
	}
}

// ---- P9: the melhora search and the queue ----------------------------------------------------

// P9: the gateway's own embeddings are queued as 127.0.0.1 (proxy/embedder.go:79). The
// per-IP waiting limit is shared with every other client of the loopback (the bake-off and
// any local tool that does not pass through the nginx): a burst of searches caused by
// REMOTE clients fills it and a local client is refused with 503.
func TestAdv12PMelhoraSearchesChargeTheLoopbackQuota(t *testing.T) {
	up := newFakeOllama()
	up.embedDelay = 1500 * time.Millisecond
	mod, _ := withMelhora([]string{"extract"}, "", 3*time.Second, exampleIndex())
	e := setup(t, up, queue.Config{GenSlots: 5, EmbedSlots: 2, PerClient: 16}, mod)
	t.Cleanup(up.unblock)
	var wg sync.WaitGroup
	// 24 remote clients (distinct IPs through the nginx: X-Real-Ip from the loopback peer)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			postBg(e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract", "X-Real-Ip": fmt.Sprintf("10.0.1.%d", i+1)})
		}(i)
	}
	// wait until the embed lane is full of the gateway's own searches
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s := e.q.Snapshot()
		n := 0
		for _, w := range s.Waiting {
			if w.Lane == queue.LaneEmbed {
				n++
			}
		}
		if n >= 14 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// a local client (no X-Real-Ip: 127.0.0.1) asks for an embedding of its own
	resp, b := post(t, e, "/api/embed", `{"model":"nomic-embed-text","input":"local"}`, map[string]string{HeaderClient: "bakeoff/1"})
	wg.Wait()
	if resp.StatusCode == 503 {
		vec12P(t, "P9.cotaLoopback", "FALHA", "Menor gateway/proxy/embedder.go:79 as buscas da melhora disparadas por clientes REMOTOS contam na cota por IP de 127.0.0.1 (PerIP): um cliente local (bakeoff/ferramenta sem nginx) recebeu 503 "+strings.TrimSpace(b))
	} else {
		vec12P(t, "P9.cotaLoopback", "OK", fmt.Sprintf("cliente local atendido (%d) durante a rajada de buscas", resp.StatusCode))
	}
}

// P9: with --gen-slots 5 and --embed-slots 2 (production), a burst of 40 requests with the
// improvement on and a dead / slow embedding Ollama: no deadlock, every request finishes,
// each search costs at most the timeout, and the queue ends empty.
func TestAdv12PMelhoraBurstNoDeadlock(t *testing.T) {
	for _, mode := range []string{"lento", "morto", "cancelado"} {
		up := newFakeOllama()
		switch mode {
		case "lento":
			up.embedDelay = 10 * time.Second
		case "morto":
			up.embedStatus = 500
		case "cancelado":
			up.embedDelay = 10 * time.Second
		}
		mod, trf := withMelhora([]string{"extract"}, "INSTR", 400*time.Millisecond, exampleIndex())
		e := setup(t, up, queue.Config{GenSlots: 5, EmbedSlots: 2}, mod)
		var wg sync.WaitGroup
		var okN, badN atomic.Int64
		t0 := time.Now()
		for i := 0; i < 40; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if mode == "cancelado" {
					ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
					defer cancel()
					req, _ := http.NewRequestWithContext(ctx, "POST", e.gw.URL+"/api/generate", strings.NewReader(fileReq))
					req.Header.Set(HeaderTool, "extract")
					req.Header.Set("X-Real-Ip", fmt.Sprintf("10.0.2.%d", i+1))
					if resp, err := http.DefaultClient.Do(req); err == nil {
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
					return
				}
				resp, _ := postBg(e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract", "X-Real-Ip": fmt.Sprintf("10.0.2.%d", i+1)})
				if resp.StatusCode == 200 {
					okN.Add(1)
				} else {
					badN.Add(1)
				}
			}(i)
		}
		ch := make(chan struct{})
		go func() { wg.Wait(); close(ch) }()
		select {
		case <-ch:
		case <-time.After(30 * time.Second):
			vec12P(t, "P9.rajada."+mode, "FALHA", "Importante gateway/proxy/proxy.go:488 impasse: 40 pedidos não terminaram em 30 s")
			continue
		}
		d := time.Since(t0)
		// the queue must end empty (no leaked ticket)
		deadline := time.Now().Add(5 * time.Second)
		var s queue.Snapshot
		for time.Now().Before(deadline) {
			s = e.q.Snapshot()
			if len(s.Running) == 0 && len(s.Waiting) == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		st := trf().Stats()
		id := "P9.rajada." + mode
		switch {
		case len(s.Running) != 0 || len(s.Waiting) != 0:
			vec12P(t, id, "FALHA", fmt.Sprintf("Importante gateway/proxy/embedder.go:78 fila não esvaziou: %d rodando, %d esperando", len(s.Running), len(s.Waiting)))
		case mode != "cancelado" && okN.Load() != 40:
			vec12P(t, id, "FALHA", fmt.Sprintf("Importante gateway/proxy/proxy.go:488 %d de 40 pedidos falharam com a busca %s", badN.Load(), mode))
		case mode != "cancelado" && d > 12*time.Second:
			vec12P(t, id, "FALHA", fmt.Sprintf("Importante gateway/melhora/transformer.go:160 a busca %s atrasou os pedidos além do limite: %s", mode, d))
		default:
			vec12P(t, id, "OK", fmt.Sprintf("MEDIDA 40 pedidos (gen 5, embed 2, limite 400 ms, busca %s): ok=%d em %s; embed_falhas=%d exemplos=%d; fila vazia no fim",
				mode, okN.Load(), d.Round(time.Millisecond), st.EmbedFalhas, st.Exemplos))
		}
		up.unblock()
	}
}

// P10 (proxy): the index is swapped while a search is in flight. The examples come from the
// index that was searched; the trace must name THAT index (C-2.10a).
func TestAdv12PIndexSwappedDuringTheSearch(t *testing.T) {
	ixA := melhora.NewIndex("idx-A", "hA", "nomic-embed-text", 2, 0.5, []melhora.Item{
		{OcorrenciaID: "ocA", Tool: "extract", Vec: []float32{1, 0, 0}, Example: melhora.Example{Pedido: "PA", Errada: "EA", Certa: "CERTA-DE-A"}},
	})
	ixB := melhora.NewIndex("idx-B", "hB", "nomic-embed-text", 2, 0.5, []melhora.Item{
		{OcorrenciaID: "ocB", Tool: "extract", Vec: []float32{1, 0, 0}, Example: melhora.Example{Pedido: "PB", Errada: "EB", Certa: "CERTA-DE-B"}},
	})
	var tr *melhora.Transformer
	tr = melhora.NewTransformer(melhora.Config{Tools: []string{"extract"}, EmbedTimeout: time.Second,
		Embed: func(ctx context.Context, text string) ([]float32, error) {
			tr.SetIndex(ixB) // the admin swaps the index while this request's search runs
			return []float32{1, 0, 0}, nil
		}})
	tr.SetIndex(ixA)
	out, oc := tr.Transform(context.Background(), "extract", "/api/generate", []byte(fileReq), melhora.Exclusion{})
	fromA := bytes.Contains(out, []byte("CERTA-DE-A"))
	if fromA && oc.IndiceID != "idx-A" {
		vec12P(t, "P10.trocaDuranteBusca", "FALHA", fmt.Sprintf("Menor gateway/melhora/transformer.go:180 exemplos do índice idx-A, mas o rastro (melhora_indice_id) diz %q: t.index.Load() é lido de novo depois da busca (latente: hoje o gateway só chama SetIndex na partida, main.go:189; InvalidateIndex troca por um stub com o MESMO id)", oc.IndiceID))
	} else {
		vec12P(t, "P10.trocaDuranteBusca", "OK", fmt.Sprintf("rastro coerente: exemplos de A=%v, indice_id=%q", fromA, oc.IndiceID))
	}
	// index removed (SetIndex(nil)) during the search: the request must still go (no panic visible)
	var tr2 *melhora.Transformer
	tr2 = melhora.NewTransformer(melhora.Config{Tools: []string{"extract"}, Instr: nil, EmbedTimeout: time.Second,
		Embed: func(ctx context.Context, text string) ([]float32, error) {
			tr2.SetIndex(nil)
			return []float32{1, 0, 0}, nil
		}})
	tr2.SetIndex(ixA)
	out2, oc2 := tr2.Transform(context.Background(), "extract", "/api/generate", []byte(fileReq), melhora.Exclusion{})
	if oc2.Pulou && bytes.Equal(out2, []byte(fileReq)) {
		vec12P(t, "P10.removidoDuranteBusca", "OK", "índice retirado durante a busca: o pânico de t.index.Load().ID (nil) é recuperado e o pedido segue sem melhora (contado como corpo_pulado, não como falha de embed)")
	} else if bytes.Contains(out2, []byte("CERTA-DE-A")) {
		vec12P(t, "P10.removidoDuranteBusca", "OK", fmt.Sprintf("índice retirado durante a busca: exemplos da busca já feita, indice_id=%q", oc2.IndiceID))
	} else {
		vec12P(t, "P10.removidoDuranteBusca", "OK", fmt.Sprintf("índice retirado durante a busca: pedido segue (pulou=%v)", oc2.Pulou))
	}
}

// P3 (melhora): the search embedding is a request the gateway builds itself (embedder.go:84):
// neither it nor the generation that follows carries the client's X-Mcpollama-Doc, even as a trailer.
func TestAdv12PMelhoraEmbedCarriesNoDocHeader(t *testing.T) {
	up, tap := adv12PRawOllama(t)
	q := queue.New(queue.Config{GenSlots: 5, EmbedSlots: 2})
	cfg := Config{Upstream: up, Queue: q, DocKey: testKey(t)}
	mod, trf := withMelhora([]string{"extract"}, "", 2*time.Second, exampleIndex())
	mod(&cfg)
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(h)
	t.Cleanup(gw.Close)
	sha := sha256.Sum256([]byte("doc-melhora"))
	v := hex.EncodeToString(sha[:])
	status := adv12PRawPost(t, strings.TrimPrefix(gw.URL, "http://"), "POST /api/generate HTTP/1.1\r\nHost: x\r\nX-Mcpollama-Tool: extract\r\nX-Mcpollama-Doc: "+v+
		"\r\nTransfer-Encoding: chunked\r\nTrailer: X-Mcpollama-Doc\r\n\r\n"+fmt.Sprintf("%x\r\n%s\r\n0\r\nX-Mcpollama-Doc: %s\r\n\r\n", len(fileReq), fileReq, v))
	time.Sleep(50 * time.Millisecond)
	raw := tap.bytes()
	st := trf().Stats()
	switch {
	case !bytes.Contains(raw, []byte("POST /api/embed")) || st.Exemplos != 1:
		vec12P(t, "P3.melhoraEmbed", "FALHA", fmt.Sprintf("Menor fixture: a busca não rodou (%s, %+v)", status, st))
	case bytes.Contains(bytes.ToLower(raw), []byte(v)):
		vec12P(t, "P3.melhoraEmbed", "FALHA", "Bloqueador gateway/proxy/embedder.go:84 o valor do X-Mcpollama-Doc chegou ao Ollama no caminho da melhora")
	default:
		vec12P(t, "P3.melhoraEmbed", "OK", fmt.Sprintf("busca de embedding + geração com exemplo (%s): nenhum dos dois pedidos leva o valor do X-Mcpollama-Doc (cabeçalho e trailer)", status))
	}
}
