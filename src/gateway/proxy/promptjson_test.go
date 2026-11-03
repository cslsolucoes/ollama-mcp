package proxy

// Fase 12, etapa 12.3: the prompt_json of the training data (contract C-2.5,
// C-2.10) and the X-Mcpollama-Doc header (C-2.11, C-4.2).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

const testKeyHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func testKey(t *testing.T) *hmackey.Key {
	t.Helper()
	p := filepath.Join(t.TempDir(), "k")
	if err := os.WriteFile(p, []byte(testKeyHex), 0o600); err != nil {
		t.Fatal(err)
	}
	k, err := hmackey.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func validJSON(t *testing.T, b []byte) {
	t.Helper()
	if !json.Valid(b) {
		t.Fatalf("not valid JSON: %s", b)
	}
}

// C-2.10: without images the prompt_json IS the body, byte for byte, whatever
// the formatting (spaces, key order, unicode escapes, odd numbers).
func TestBuildPromptJSONWithoutImagesIsTheBody(t *testing.T) {
	k := testKey(t)
	for _, body := range []string{
		`{"model":"qwen3-ia","prompt":"oi","stream":false}`,
		"{\n  \"model\" : \"qwen3-ia\" ,\r\n \"messages\":[{\"role\":\"user\",\"content\":\"a\\u00e9\\n\"}] , \"options\":{\"seed\":1.50e0,\"images\":[\"x\"]},\"images\":[] }",
		`{"prompt":"😀 x","think":true,"format":{"type":"object"}}`,
	} {
		pj := buildPromptJSON([]byte(body), k)
		if pj.HasImage || !bytes.Equal(pj.JSON, []byte(body)) {
			t.Errorf("changed:\n%s\n%s (image=%v)", body, pj.JSON, pj.HasImage)
		}
	}
}

// C-2.5: /api/generate images (top level) become markers; everything else, spaces
// included, is untouched; no base64 text survives.
func TestBuildPromptJSONGenerateImages(t *testing.T) {
	k := testKey(t)
	body := `{ "model":"m", "prompt":"descreva",  "images": [ "QUJD" , "REVG" ], "stream":false }`
	pj := buildPromptJSON([]byte(body), k)
	want := `{ "model":"m", "prompt":"descreva",  "images": [ {"imagem_hmac":"` + k.MACString("QUJD") + `"} , {"imagem_hmac":"` + k.MACString("REVG") + `"} ], "stream":false }`
	if !pj.HasImage || string(pj.JSON) != want {
		t.Fatalf("got  %s\nwant %s", pj.JSON, want)
	}
	validJSON(t, pj.JSON)
	if strings.Contains(string(pj.JSON), "QUJD") || strings.Contains(string(pj.JSON), "REVG") {
		t.Fatal("image text survived")
	}
}

// C-2.5: /api/chat images inside each message; the history around them stays.
func TestBuildPromptJSONChatImagesInMessages(t *testing.T) {
	k := testKey(t)
	body := `{"model":"m","messages":[{"role":"system","content":"s"},{"role":"user","content":"olha","images":["AAAA"]},{"role":"assistant","content":"ok"},{"role":"user","content":"e esta?","images":["BBBB","CCCC"]}]}`
	pj := buildPromptJSON([]byte(body), k)
	m := func(s string) string { return `{"imagem_hmac":"` + k.MACString(s) + `"}` }
	want := `{"model":"m","messages":[{"role":"system","content":"s"},{"role":"user","content":"olha","images":[` + m("AAAA") + `]},{"role":"assistant","content":"ok"},{"role":"user","content":"e esta?","images":[` + m("BBBB") + `,` + m("CCCC") + `]}]}`
	if !pj.HasImage || string(pj.JSON) != want {
		t.Fatalf("got  %s\nwant %s", pj.JSON, want)
	}
	validJSON(t, pj.JSON)
}

// C-2.5: /v1 content parts: the image_url value (object or string) is replaced.
func TestBuildPromptJSONV1ImageURLParts(t *testing.T) {
	k := testKey(t)
	url1 := `data:image/png;base64,iVBORw0KGgo=`
	body := `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"veja"},{"type":"image_url","image_url":{"url":"` + url1 + `","detail":"low"}},{"image_url":"http://x/y.png","type":"image_url"}]}]}`
	pj := buildPromptJSON([]byte(body), k)
	obj := `{"url":"` + url1 + `","detail":"low"}`
	want := `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"veja"},{"type":"image_url","image_url":{"imagem_hmac":"` + k.MACString(obj) + `"}},{"image_url":{"imagem_hmac":"` + k.MACString("http://x/y.png") + `"},"type":"image_url"}]}]}`
	if !pj.HasImage || string(pj.JSON) != want {
		t.Fatalf("got  %s\nwant %s", pj.JSON, want)
	}
	validJSON(t, pj.JSON)
	if strings.Contains(string(pj.JSON), "iVBOR") || strings.Contains(string(pj.JSON), "y.png") {
		t.Fatal("image data survived")
	}
}

// C-2.5: the Ollama decodes keys without regard to case and understands escapes
// in them; the redaction must too, or an image would be kept in clear.
func TestBuildPromptJSONKeysCaseAndEscapes(t *testing.T) {
	k := testKey(t)
	for _, body := range []string{
		`{"model":"m","Images":["SEGREDO"]}`,
		`{"model":"m","IMAGES":["SEGREDO"]}`,
		`{"model":"m","images":["SEGREDO"]}`,
		`{"model":"m","Messages":[{"Role":"user","IMAGES":["SEGREDO"]}]}`,
		`{"model":"m","images":["SEGREDO"],"images":["SEGREDO"]}`,
		`{"model":"m","messages":[{"role":"user","Content":[{"Image_URL":"SEGREDO"}]}]}`,
	} {
		pj := buildPromptJSON([]byte(body), k)
		if !pj.HasImage || strings.Contains(string(pj.JSON), "SEGREDO") {
			t.Errorf("%s -> %s (image=%v)", body, pj.JSON, pj.HasImage)
		}
		validJSON(t, pj.JSON)
	}
}

// Other keys that merely look like image fields are not touched, and an empty
// images array is not an image.
func TestBuildPromptJSONLeavesLookalikesAndEmptyImages(t *testing.T) {
	k := testKey(t)
	for _, body := range []string{
		`{"prompt":"p","images":[]}`,
		`{"messages":[{"role":"user","content":"c","images":[]}]}`,
		`{"options":{"images":["x"]},"prompt":"images"}`,
		`{"messages":[{"role":"user","content":"images: [\"x\"]","image_url":"nao eh parte"}]}`,
	} {
		pj := buildPromptJSON([]byte(body), k)
		if pj.HasImage || string(pj.JSON) != body {
			t.Errorf("changed or flagged: %s -> %s", body, pj.JSON)
		}
	}
}

// C-2.5: without a key the marker carries an empty hash and tem_imagem is still
// set; there is never a plain sha256 (nor any digest of the image).
func TestBuildPromptJSONWithoutKeyHasEmptyMarker(t *testing.T) {
	pj := buildPromptJSON([]byte(`{"prompt":"p","images":["QUJD"]}`), nil)
	if !pj.HasImage || string(pj.JSON) != `{"prompt":"p","images":[{"imagem_hmac":""}]}` {
		t.Fatalf("got %s (image=%v)", pj.JSON, pj.HasImage)
	}
}

// B1: a body the scan cannot read to the end fails CLOSED: no JSON is kept and
// the job counts as having an image. Valid non-object JSON is left alone.
func TestBuildPromptJSONFailsClosed(t *testing.T) {
	k := testKey(t)
	for _, body := range []string{``, `not json`, `{"images":["SEGREDO"`, `[1,2`, `{"images":["SEGREDO"]} trailing`, `{"a":}`, `{"images":["SEGREDO"],"x":tru}`} {
		pj := buildPromptJSON([]byte(body), k)
		if !pj.Failed || !pj.HasImage || pj.JSON != nil {
			t.Errorf("%q: not failed closed: %+v", body, pj)
		}
	}
	for _, body := range []string{`null`, `"just a string"`, `[1,2]`, `7`} {
		pj := buildPromptJSON([]byte(body), k)
		if pj.Failed || pj.HasImage || string(pj.JSON) != body {
			t.Errorf("%q: %+v", body, pj)
		}
	}
}

// B1: numbers the float parser refuses (1e400 ...) are text for the scan: the
// image is still replaced and the number kept as it came.
func TestBuildPromptJSONHugeNumbersDoNotBreakTheScan(t *testing.T) {
	k := testKey(t)
	for _, num := range []string{"1e400", "-1e999", "1E+4000", "123456789012345678901234567890e500", "1e-99999"} {
		body := `{"model":"qwen3-ia","prompt":"x","images":["SEGREDO"],"zz":` + num + `,"options":{"yy":[` + num + `]}}`
		pj := buildPromptJSON([]byte(body), k)
		if pj.Failed || !pj.HasImage || strings.Contains(string(pj.JSON), "SEGREDO") {
			t.Fatalf("%s: %+v", num, pj)
		}
		want := `{"model":"qwen3-ia","prompt":"x","images":[{"imagem_hmac":"` + k.MACString("SEGREDO") + `"}],"zz":` + num + `,"options":{"yy":[` + num + `]}}`
		if string(pj.JSON) != want {
			t.Fatalf("%s:\n got %s\nwant %s", num, pj.JSON, want)
		}
	}
}

// I1: the Ollama's decoder matches names by Unicode case folding, so "imageſ"
// (long s) and "meſſages" are the image and message keys for it. The scan uses
// strings.EqualFold, not ToLower.
func TestBuildPromptJSONUnicodeFoldedKeys(t *testing.T) {
	k := testKey(t)
	for _, body := range []string{
		`{"model":"m","imageſ":["SEGREDO"]}`,
		`{"model":"m","meſſages":[{"role":"user","imageſ":["SEGREDO"]}]}`,
		`{"model":"m","messages":[{"role":"user","conteNt":[{"image_URL":"SEGREDO"}]}]}`,
		`{"model":"m","meſſages":[{"role":"user","content":[{"image_urL":{"url":"SEGREDO"}}]}]}`,
	} {
		pj := buildPromptJSON([]byte(body), k)
		if !pj.HasImage || strings.Contains(string(pj.JSON), "SEGREDO") {
			t.Errorf("%s -> %s", body, pj.JSON)
		}
		validJSON(t, pj.JSON)
	}
}

// I2: a body nested ~9,000 levels deep with hundreds of thousands of tokens is
// scanned in linear time and memory (no path allocated per token).
func TestBuildPromptJSONDeepNestingIsLinear(t *testing.T) {
	k := testKey(t)
	const depth = 9000
	var b strings.Builder
	b.WriteString(`{"model":"m","deep":`)
	b.WriteString(strings.Repeat("[", depth))
	b.WriteString(strings.Repeat("1,", 300000))
	b.WriteString("1")
	b.WriteString(strings.Repeat("]", depth))
	b.WriteString(`,"images":["SEGREDO"]}`)
	body := []byte(b.String())
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	start := time.Now()
	pj := buildPromptJSON(body, k)
	el := time.Since(start)
	runtime.ReadMemStats(&m1)
	if pj.Failed || !pj.HasImage || strings.Contains(string(pj.JSON), "SEGREDO") {
		t.Fatalf("scan failed: failed=%v image=%v", pj.Failed, pj.HasImage)
	}
	if el > 2*time.Second {
		t.Fatalf("scan took %v", el)
	}
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 400<<20 {
		t.Fatalf("scan allocated %d MiB", alloc>>20)
	}
}

// I2: with the content off the proxy does not scan the body at all, and with it
// on the scan runs for the accepted request.
type fakeJournal struct {
	wants  bool
	mu     sync.Mutex
	plain  []string
	withPJ []PromptJSON
}

func (f *fakeJournal) Enqueued(rec queue.Record, prompt string) {
	f.mu.Lock()
	f.plain = append(f.plain, prompt)
	f.mu.Unlock()
}
func (f *fakeJournal) EnqueuedJSON(rec queue.Record, prompt string, pj PromptJSON) {
	f.mu.Lock()
	f.withPJ = append(f.withPJ, pj)
	f.mu.Unlock()
}
func (f *fakeJournal) WantsPromptJSON() bool                           { return f.wants }
func (f *fakeJournal) Started(id string, at time.Time, backend string) {}
func (f *fakeJournal) Response(id string) ResponseSink                 { return nil }

func TestContentOffDoesNotScanTheBody(t *testing.T) {
	for _, wants := range []bool{false, true} {
		fj := &fakeJournal{wants: wants}
		k := testKey(t)
		e := setup(t, &capture{}, queue.Config{}, func(c *Config) { defaultPolicy(c); c.Journal = fj; c.DocKey = k })
		before := promptJSONBuilds.Load()
		post(t, e, "/v1/chat/completions", `{"model":"qwen3-ia","messages":[{"role":"user","content":[{"type":"image_url","image_url":"SEGREDO"}]}]}`, nil)
		e.waitIdle(t)
		scans := promptJSONBuilds.Load() - before
		fj.mu.Lock()
		if !wants && (scans != 0 || len(fj.plain) != 1 || len(fj.withPJ) != 0 || fj.plain[0] != "") {
			t.Errorf("content off: %d scans, plain=%q, json=%d", scans, fj.plain, len(fj.withPJ))
		}
		if wants && (scans < 1 || len(fj.withPJ) != 1 || len(fj.plain) != 0 || !fj.withPJ[0].HasImage) {
			t.Errorf("content on: %d scans, plain=%q, json=%+v", scans, fj.plain, fj.withPJ)
		}
		fj.mu.Unlock()
	}
}

// M1: the HMAC of a string image is taken over the slice of the body, with no
// copy of a large image; an escaped string hashes its decoded value.
func TestStringMACNoCopyAndSameValue(t *testing.T) {
	k := testKey(t)
	big := bytes.Repeat([]byte("A"), 16<<20)
	body := append(append([]byte(`"`), big...), '"')
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	got := stringMAC(k, body, 0, len(body), "")
	runtime.ReadMemStats(&m1)
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 1<<20 {
		t.Fatalf("stringMAC allocated %d KiB for a 16 MiB image", alloc>>10)
	}
	if want := `{"imagem_hmac":"` + k.MAC(big) + `"}`; string(got) != want {
		t.Fatal("marker differs from the HMAC of the image bytes")
	}
	esc := []byte(`"QU\/JD"`)
	if string(stringMAC(k, esc, 0, len(esc), "QU/JD")) != `{"imagem_hmac":"`+k.MACString("QU/JD")+`"}` {
		t.Fatal("an escaped string must hash its decoded value")
	}
}

// M5: the flat text of /v1 never shows the data URI of an image part; it shows
// the marker (empty hash without a key) and keeps the other parts.
func TestExtractPromptRedactsV1ImageParts(t *testing.T) {
	k := testKey(t)
	body := []byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"veja isto"},{"type":"image_url","image_url":{"url":"data:image/png;base64,SEGREDOSEGREDO"}}]},{"role":"assistant","content":"ok"}]}`)
	txt := extractPrompt(kindV1, body, k)
	if strings.Contains(txt, "SEGREDO") || !strings.Contains(txt, `{"imagem_hmac":"`) || !strings.Contains(txt, "veja isto") || !strings.Contains(txt, "[assistant]\nok") {
		t.Fatalf("flat text: %s", txt)
	}
	want := `{"imagem_hmac":"` + k.MACString(`{"url":"data:image/png;base64,SEGREDOSEGREDO"}`) + `"}`
	if !strings.Contains(txt, want) {
		t.Fatalf("marker is not the one of the prompt JSON: %s", txt)
	}
	nokey := extractPrompt(kindV1, body, nil)
	if strings.Contains(nokey, "SEGREDO") || !strings.Contains(nokey, `{"imagem_hmac":""}`) {
		t.Fatalf("without key: %s", nokey)
	}
	plain := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"a"}]},{"role":"user","content":"b"}]}`)
	if got := extractPrompt(kindV1, plain, k); !strings.Contains(got, `[{"type":"text","text":"a"}]`) || !strings.Contains(got, "[user]\nb") {
		t.Fatalf("plain parts changed: %s", got)
	}
}

// The marker hashes the image as it arrived: the same image gives the same
// marker, another image another one.
func TestImageMarkerIsStableAndImageSpecific(t *testing.T) {
	k := testKey(t)
	a := buildPromptJSON([]byte(`{"images":["QUJD"]}`), k).JSON
	b := buildPromptJSON([]byte(`{ "images" : [ "QUJD" ] }`), k).JSON
	c := buildPromptJSON([]byte(`{"images":["QUJE"]}`), k).JSON
	if !strings.Contains(string(b), k.MACString("QUJD")) || !strings.Contains(string(a), k.MACString("QUJD")) {
		t.Fatal("marker is not the HMAC of the image")
	}
	if bytes.Equal(a, c) {
		t.Fatal("different images gave the same marker")
	}
}

// C-2.11: the header is dropped before the Ollama, whatever its value.
func TestDocHeaderNeverReachesTheOllama(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, defaultPolicy)
	hdr := map[string]string{HeaderDoc: strings.Repeat("ab", 32), HeaderClient: "c/1"}
	if resp, b := post(t, e, "/api/generate", `{"model":"qwen3-ia","prompt":"x"}`, hdr); resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if v := cp.hdrs[0].Get(HeaderDoc); v != "" {
		t.Fatalf("X-Mcpollama-Doc reached the Ollama: %q", v)
	}
	for k := range cp.hdrs[0] {
		if strings.EqualFold(k, HeaderDoc) {
			t.Fatalf("header %s reached the Ollama", k)
		}
	}
	// An invalid value is dropped too (not forwarded as junk).
	cp2 := &capture{}
	e2 := setup(t, cp2, queue.Config{}, defaultPolicy)
	post(t, e2, "/api/generate", `{"model":"qwen3-ia","prompt":"x"}`, map[string]string{HeaderDoc: "lixo"})
	if cp2.hdrs[0].Get(HeaderDoc) != "" {
		t.Fatal("an invalid X-Mcpollama-Doc reached the Ollama")
	}
	// It is also dropped on the pass-through routes.
	var got http.Header
	e3 := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Clone(); w.Write([]byte("{}")) }), queue.Config{}, defaultPolicy)
	req, _ := http.NewRequest("GET", e3.gw.URL+"/api/tags", nil)
	req.Header.Set(HeaderDoc, strings.Repeat("cd", 32))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Get(HeaderDoc) != "" {
		t.Fatal("X-Mcpollama-Doc reached the Ollama on /api/tags")
	}
}

func postDoc(t *testing.T, e *env, vals ...string) queue.Record {
	t.Helper()
	req, _ := http.NewRequest("POST", e.gw.URL+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"x"}`))
	for _, v := range vals {
		req.Header.Add(HeaderDoc, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return e.waitRecord(t, resp.Header.Get(HeaderRequestID))
}

// C-4.2, C-4.3a: the record carries only the HMAC of the header and the key id;
// never the header value itself.
func TestDocHashIsHMACWithKeyID(t *testing.T) {
	k := testKey(t)
	e := setup(t, &capture{}, queue.Config{}, func(c *Config) { defaultPolicy(c); c.DocKey = k })
	v := strings.Repeat("ab", 32)
	rec := postDoc(t, e, v)
	if rec.DocHash != k.MACString(v) || rec.DocKeyID != k.ID() || rec.DocKeyID != "630dcd2966c43366" {
		t.Fatalf("doc_hash=%q key_id=%q", rec.DocHash, rec.DocKeyID)
	}
	if rec.DocHash == v || strings.Contains(rec.DocHash, v) {
		t.Fatal("the plain sha256 was recorded")
	}
	e.waitIdle(t)
	for _, r := range e.records() { // no other field of any record carries the value
		if strings.Contains(strings.Join([]string{r.ClientLabel, r.Tool, r.Endpoint, r.Model, r.ErrorClass, r.StationComputer}, "|"), v) {
			t.Fatal("the header value leaked into another field")
		}
	}
}

// C-4.2: only one well-formed header counts; anything else is ignored.
func TestDocHeaderInvalidValuesAreIgnored(t *testing.T) {
	k := testKey(t)
	e := setup(t, &capture{}, queue.Config{}, func(c *Config) { defaultPolicy(c); c.DocKey = k })
	good := strings.Repeat("ab", 32)
	for name, vals := range map[string][]string{
		"upper case": {strings.ToUpper(good)}, "short": {good[:62]}, "long": {good + "a"}, "not hex": {strings.Repeat("g", 64)},
		"two lines": {good, good}, "padded": {" " + good[1:]}, "empty": {""},
	} {
		if rec := postDoc(t, e, vals...); rec.DocHash != "" || rec.DocKeyID != "" {
			t.Errorf("%s: doc_hash=%q key_id=%q", name, rec.DocHash, rec.DocKeyID)
		}
	}
	if rec := postDoc(t, e); rec.DocHash != "" || rec.DocKeyID != "" {
		t.Errorf("no header: %q %q", rec.DocHash, rec.DocKeyID)
	}
}

// C-4.2: without a key the gateway records no document hash at all.
func TestDocHashEmptyWithoutKey(t *testing.T) {
	e := setup(t, &capture{}, queue.Config{}, defaultPolicy)
	if rec := postDoc(t, e, strings.Repeat("ab", 32)); rec.DocHash != "" || rec.DocKeyID != "" {
		t.Fatalf("recorded without a key: %q %q", rec.DocHash, rec.DocKeyID)
	}
}
