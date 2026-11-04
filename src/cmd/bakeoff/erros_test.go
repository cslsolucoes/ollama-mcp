package main

// Tests of the suite `erros` (Fase 12, stage 12.7) with a synthetic treino.db and
// a fake gateway (httptest): nothing here touches the real server or any network
// but the loopback of the test.

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

const testKeyHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

// secretMarkers appear only in the content of the cases (request, correct answer,
// the model outputs); no report or log may ever contain them (C-5.6).
var secretMarkers = []string{"SEGREDO", "OUT-"}

type synthCase struct {
	N                     int
	Cat, Estado, Particao string
	Correct, Right, Wrong string
	Endpoint              string
	Model                 string
	Truncated             bool // the recorded sample is truncated (not eligible)
}

func synthID(n int) string  { return fmt.Sprintf("%032x", n) }
func synthJob(n int) string { return fmt.Sprintf("%032x", 5000+n) }

func (c synthCase) promptJSON() []byte {
	m := c.Model
	if m == "" {
		m = "qwen3-ia"
	}
	return []byte(fmt.Sprintf(`{"model":%q,"prompt":"PEDIDO-%s texto do cliente SEGREDO-CLIENTE","options":{"temperature":0.2,"seed":99,"num_predict":300},"stream":true,"think":false}`, m, synthID(c.N)))
}

func omitiu(n int) synthCase {
	return synthCase{N: n, Cat: "omitiu", Correct: "item-SEGREDO-1\nitem-SEGREDO-2\nitem-SEGREDO-3", Right: "item-SEGREDO-3\nitem-SEGREDO-1\nitem-SEGREDO-2", Wrong: "item-SEGREDO-1\nitem-SEGREDO-2"}
}

func formato(n int) synthCase {
	return synthCase{N: n, Cat: "formato_invalido", Correct: `{"x":1,"y":"SEGREDO"}`, Right: `{"y":"SEGREDO","x":1}`, Wrong: `{"x":1,"y":"OUT-errado"}`}
}

func blindCase(n int) synthCase {
	return synthCase{N: n, Cat: "inventou", Correct: "resposta CERTA-SEGREDO", Right: "ATUAL-OUT", Wrong: "ATUAL-OUT"}
}

func fill(c synthCase) synthCase {
	if c.Estado == "" {
		c.Estado = "confirmada"
	}
	if c.Particao == "" {
		c.Particao = "teste"
	}
	if c.Endpoint == "" {
		c.Endpoint = "/api/generate"
	}
	return c
}

func testKey(t *testing.T) *hmackey.Key {
	t.Helper()
	raw, _ := hex.DecodeString(testKeyHex)
	k, err := hmackey.New(raw)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// seedDB creates a treino.db (schema by the package itself) and inserts the cases
// as confirmed occurrences of the test partition, with their samples.
func seedDB(t *testing.T, dir string, cs []synthCase) (dbPath, keyPath string) {
	t.Helper()
	dbPath = filepath.Join(dir, "treino.db")
	keyPath = filepath.Join(dir, "doc.key")
	if err := os.WriteFile(keyPath, []byte(testKeyHex+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := treino.Open(dbPath, treino.Options{})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := testKey(t)
	for _, c := range cs {
		c = fill(c)
		pj := c.promptJSON()
		tr := 0
		if c.Truncated {
			tr = 1
		}
		if _, err := db.Exec(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado,categoria,resposta_correta,tool,job_do_erro,particao,grupo_id) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			synthID(c.N), 1, 1, c.Estado, c.Cat, c.Correct, "extract", synthJob(c.N), c.Particao, synthID(c.N)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO ocorrencia_jobs(ocorrencia_id,job_id,papel,backend,ordem) VALUES(?,?,?,?,0)`, synthID(c.N), synthJob(c.N), "unico", "primario"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO amostras(job_id,copiada_em,endpoint,modelo,think,prompt_json,prompt_texto,resposta,prompt_hmac,conteudo_hmac,key_id,prompt_truncado,prompt_json_truncado,tem_imagem,doc_hash,resp_final,resp_truncada,status,http_status)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,0,0,'',1,0,'ok',200)`,
			synthJob(c.N), 1, c.Endpoint, "qwen3-ia", "off", pj, []byte("x"), []byte("y"), key.PromptHMAC(pj), "ch", key.ID(), tr); err != nil {
			t.Fatal(err)
		}
	}
	return dbPath, keyPath
}

// ---- fake gateway ------------------------------------------------------------------------------------

type gwReq struct {
	Path, Tool string
	Model      string
	Seed       int
	Key        string // the case id found in the prompt
	Body       map[string]json.RawMessage
}

type embedReq struct{ Model, Tool, Text string }

type fakeGW struct {
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []gwReq
	cases  map[string]synthCase
	policy func(model string, seed int, c synthCase) (text, reason string)
	status int // when set, every call fails with it
	// onReq runs on every request, with its number (1-based), before it is answered
	onReq func(n int)
	// cut makes an answer end badly: "nodone" (the stream stops without done), "abort" (the connection breaks)
	cut func(model string, seed int, c synthCase) string
	// stage 12.8: the embeddings asked for the examples of the candidate
	embeds      []embedReq
	embedStatus int
}

var caseKeyRe = regexp.MustCompile(`PEDIDO-([0-9a-f]{32})`)

func newGW(t *testing.T, cs []synthCase, policy func(model string, seed int, c synthCase) (string, string)) *fakeGW {
	g := &fakeGW{cases: map[string]synthCase{}, policy: policy}
	for _, c := range cs {
		g.cases[synthID(c.N)] = fill(c)
	}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/api/embed" { // stage 12.8: the search of the examples
			var eb struct{ Model, Input string }
			_ = json.Unmarshal(raw, &eb)
			g.mu.Lock()
			g.embeds = append(g.embeds, embedReq{Model: eb.Model, Tool: r.Header.Get("X-Mcpollama-Tool"), Text: eb.Input})
			st := g.embedStatus
			g.mu.Unlock()
			if st != 0 {
				w.WriteHeader(st)
				return
			}
			fmt.Fprint(w, `{"embeddings":[[1,0,0]]}`)
			return
		}
		var body map[string]json.RawMessage
		_ = json.Unmarshal(raw, &body)
		var model string
		_ = json.Unmarshal(body["model"], &model)
		var opts map[string]json.RawMessage
		_ = json.Unmarshal(body["options"], &opts)
		var seed int
		_ = json.Unmarshal(opts["seed"], &seed)
		key := ""
		if m := caseKeyRe.FindSubmatch(raw); m != nil {
			key = string(m[1])
		}
		g.mu.Lock()
		g.reqs = append(g.reqs, gwReq{Path: r.URL.Path, Tool: r.Header.Get("X-Mcpollama-Tool"), Model: model, Seed: seed, Key: key, Body: body})
		st, n := g.status, len(g.reqs)
		c, ok := g.cases[key]
		g.mu.Unlock()
		if g.onReq != nil {
			g.onReq(n)
		}
		if st != 0 {
			w.WriteHeader(st)
			return
		}
		if !ok {
			w.WriteHeader(404)
			return
		}
		text, reason := g.policy(model, seed, c)
		w.Header().Set("Content-Type", "application/x-ndjson")
		half := len(text) / 2
		fmt.Fprintf(w, `{"model":%q,"response":%q,"done":false}`+"\n", model, text[:half])
		mode := ""
		if g.cut != nil {
			mode = g.cut(model, seed, c)
		}
		if mode == "abort" {
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		fmt.Fprintf(w, `{"model":%q,"response":%q,"done":false}`+"\n", model, text[half:])
		if mode == "nodone" {
			return
		}
		fmt.Fprintf(w, `{"model":%q,"response":"","done":true,"done_reason":%q}`+"\n", model, reason)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGW) requests() []gwReq {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]gwReq(nil), g.reqs...)
}

func (g *fakeGW) reset() {
	g.mu.Lock()
	g.reqs = nil
	g.mu.Unlock()
}

// answer returns the right or the wrong text of the case, stopping normally.
func answer(right bool, c synthCase) (string, string) {
	if right {
		return c.Right, "stop"
	}
	return c.Wrong, "stop"
}

// ---- runner ------------------------------------------------------------------------------------------

type fixture struct {
	t          *testing.T
	dir, out   string
	db, key    string
	gw         *fakeGW
	clock      int
	noBaseline bool
	logs       []string
	mu         sync.Mutex
}

func newFixture(t *testing.T, cs []synthCase, policy func(model string, seed int, c synthCase) (string, string)) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{t: t, dir: dir, out: filepath.Join(dir, "out")}
	f.db, f.key = seedDB(t, dir, cs)
	f.gw = newGW(t, cs, policy)
	return f
}

func (f *fixture) opts(mode string) errosOpts {
	return errosOpts{Mode: mode, DB: f.db, KeyFile: f.key, GatewayURL: f.gw.srv.URL, Out: f.out, Timeout: 30 * time.Second, RetryWait: time.Millisecond,
		ShuffleSeed: 42, Atual: "qwen3-ia",
		Now: func() time.Time {
			f.clock++
			return time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC).Add(time.Duration(f.clock) * time.Second)
		}}
}

// ensureBaseline writes an (empty) baseline of the retrato, which a comparison requires.
func (f *fixture) ensureBaseline(rid string) {
	dir := filepath.Join(f.out, "erros", rid)
	os.MkdirAll(dir, 0o700)
	p := filepath.Join(dir, "linha-de-base.json")
	if _, err := os.Stat(p); err == nil {
		return
	}
	if err := os.WriteFile(p, mustJSON(baselineFile{RetratoID: rid, Modelo: "qwen3-ia", Cats: map[string]baselineCt{}}), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) run(eo errosOpts) (int, string) {
	f.t.Helper()
	var lines []string
	logf := func(format string, a ...any) {
		f.mu.Lock()
		lines = append(lines, fmt.Sprintf(format, a...))
		f.mu.Unlock()
	}
	if eo.Mode == "comparar" && eo.RetratoID != "" && !f.noBaseline {
		f.ensureBaseline(eo.RetratoID)
	}
	code := errosMain(eo, logf)
	out := strings.Join(lines, "\n")
	f.mu.Lock()
	f.logs = append(f.logs, out)
	f.mu.Unlock()
	return code, out
}

var retratoIDRe = regexp.MustCompile(`RETRATO id=([0-9a-f]{32})`)

func (f *fixture) makeRetrato() string {
	f.t.Helper()
	code, out := f.run(f.opts("retrato"))
	m := retratoIDRe.FindStringSubmatch(out)
	if code != 0 || m == nil {
		f.t.Fatalf("retrato: code %d\n%s", code, out)
	}
	return m[1]
}

func writeScore(t *testing.T, out, model string, s2ok, s2tot, s3ok, s3tot int) {
	t.Helper()
	dir := filepath.Join(out, safeName(model))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s3 := map[string]map[string]bool{}
	for i := 0; i < s3tot; i++ {
		s3[fmt.Sprintf("k%d", i)] = map[string]bool{"ok": i < s3ok}
	}
	b, _ := json.Marshal(map[string]any{"modelo": model, "gerado_em": "2026-11-03T14:00:00Z", "s2": []map[string]int{{"numero_e_titulo_ok": s2ok, "total_gabarito": s2tot}}, "s3": s3})
	if err := os.WriteFile(filepath.Join(dir, "score.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func syntheticSet() []synthCase {
	var cs []synthCase
	for i := 1; i <= 30; i++ {
		cs = append(cs, omitiu(i))
	}
	for i := 31; i <= 60; i++ {
		cs = append(cs, formato(i))
	}
	cs = append(cs, blindCase(61), blindCase(62))
	o := fill(omitiu(63))
	o.Cat = "outro"
	cs = append(cs, o)
	i := fill(omitiu(64))
	i.Cat = "idioma_errado"
	cs = append(cs, i)
	return cs
}

// scenario policy: the current model gets omitiu right in 14 of 30 cases (case 1
// and 2 right in 2 of 3 rounds, case 3 right in 1 of 3), the candidate in 27;
// formato_invalido is the same in both unless dropOne is set.
func scenarioPolicy(dropOne bool) func(model string, seed int, c synthCase) (string, string) {
	return func(model string, seed int, c synthCase) (string, string) {
		cand := model != "qwen3-ia"
		switch c.Cat {
		case "omitiu":
			if cand {
				return answer(c.N <= 27, c)
			}
			switch {
			case c.N <= 2:
				return answer(seed != 3, c)
			case c.N == 3:
				return answer(seed == 1, c)
			}
			return answer(c.N <= 15, c)
		case "formato_invalido":
			n := c.N - 30
			if cand && dropOne {
				return answer(n <= 19, c)
			}
			return answer(n <= 20, c)
		}
		return "ATUAL-OUT", "stop" // blind cases
	}
}

func sidecarFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ---- the flow ----------------------------------------------------------------------------------------

func TestErrosFullFlow(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()

	// C-5.7, C-4.13: the retrato holds the 62 evaluable cases, never outro or idioma_errado
	db, _ := sql.Open("sqlite", f.db)
	defer db.Close()
	var n, bad int
	db.QueryRow(`SELECT COUNT(*) FROM retrato_itens WHERE retrato_id=?`, rid).Scan(&n)
	db.QueryRow(`SELECT COUNT(*) FROM retrato_itens WHERE retrato_id=? AND categoria IN ('outro','idioma_errado')`, rid).Scan(&bad)
	if n != 62 || bad != 0 {
		t.Fatalf("retrato has %d items, %d without criterion; want 62 and 0", n, bad)
	}

	// baseline (C-5.14): only the current model, three rounds, blind cases skipped
	writeScore(t, f.out, "qwen3-ia", 90, 100, 28, 30)
	eo := f.opts("base")
	eo.RetratoID = rid
	code, out := f.run(eo)
	if code != 0 || !strings.Contains(out, "LINHA DE BASE REGISTRADA") {
		t.Fatalf("base: code %d\n%s", code, out)
	}
	reqs := f.gw.requests()
	if len(reqs) != 60*3 {
		t.Fatalf("baseline made %d calls, want %d (60 scored cases x 3 seeds; blind skipped)", len(reqs), 60*3)
	}
	for _, r := range reqs {
		if r.Model != "qwen3-ia" || r.Tool != "bakeoff-erros" {
			t.Fatalf("baseline call %+v", r)
		}
	}
	basePath := filepath.Join(f.out, "erros", rid, "linha-de-base.json")
	var bf baselineFile
	if err := json.Unmarshal([]byte(readFile(t, basePath)), &bf); err != nil || bf.RetratoID != rid || bf.Cats["omitiu"].N != 30 || bf.Cats["omitiu"].Acertos != 14 || bf.Cats["omitiu"].Mudaram != 3 || len(bf.Casos) != 60 {
		t.Fatalf("baseline file: %+v %v", bf.Cats, err)
	}
	// the baseline is the first round: a second one is refused and changes nothing
	before := readFile(t, basePath)
	if code, out := f.run(eo); code != exitUsage || !strings.Contains(out, "já tem linha de base") {
		t.Fatalf("second baseline: code %d\n%s", code, out)
	}
	if readFile(t, basePath) != before {
		t.Fatal("the baseline was overwritten")
	}

	// comparison (C-5.10 to C-5.12, C-5.15, C-5.16)
	writeScore(t, f.out, "qwen3-ia-v2", 90, 100, 28, 30)
	f.gw.reset()
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "omitiu"
	code, out = f.run(ce)
	if code != exitPending || !strings.Contains(out, "PENDENTE da revisão às cegas") || strings.Contains(out, "MELHORA ACEITA") || strings.Contains(strings.ToLower(out), "sem regressão") {
		t.Fatalf("comparar with blind cases must be pending, not accepted: code %d\n%s", code, out)
	}
	for _, want := range []string{rid, "| omitiu | 30 | 46,7% | [30,2–63,9] | 3 | 90,0% | [74,4–96,5] | 0 | +43,3 | decide |", "| 3 | 90,0%", "| formato_invalido | 30 | 66,7%", "| decide |"} {
		if !strings.Contains(out, want) {
			t.Errorf("report misses %q\n%s", want, out)
		}
	}
	t.Logf("relatório do modo comparar (sintético):\n%s", out)
	reqs = f.gw.requests()
	if len(reqs) != 60*6+2*2 {
		t.Fatalf("comparison made %d calls, want %d (60x6 scored + 2 blind x 2)", len(reqs), 60*6+2*2)
	}
	// the current model is measured again in every comparison (C-5.16)
	nAtual := 0
	for _, r := range reqs {
		if r.Model == "qwen3-ia" {
			nAtual++
		}
	}
	if nAtual != 60*3+2 {
		t.Errorf("the current model was called %d times, want %d", nAtual, 60*3+2)
	}
}

func TestErrosReplayInterleavedSeedsAndBodies(t *testing.T) { // C-5.1, C-5.15, C-5.2
	cs := []synthCase{omitiu(1), omitiu(2)}
	f := newFixture(t, cs, scenarioPolicy(false))
	rid := f.makeRetrato()
	writeScore(t, f.out, "qwen3-ia", 9, 10, 9, 10)
	writeScore(t, f.out, "qwen3-ia-v2", 9, 10, 9, 10)
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato = rid, "qwen3-ia-v2"
	f.run(ce)
	byCase := map[string][]gwReq{}
	for _, r := range f.gw.requests() {
		byCase[r.Key] = append(byCase[r.Key], r)
	}
	for _, c := range cs {
		rs := byCase[synthID(c.N)]
		if len(rs) != 6 {
			t.Fatalf("case %d: %d calls, want 6", c.N, len(rs))
		}
		wantModels := []string{"qwen3-ia", "qwen3-ia-v2", "qwen3-ia", "qwen3-ia-v2", "qwen3-ia", "qwen3-ia-v2"}
		wantSeeds := []int{1, 1, 2, 2, 3, 3}
		for i, r := range rs {
			if r.Model != wantModels[i] || r.Seed != wantSeeds[i] {
				t.Errorf("case %d call %d: model %s seed %d, want %s seed %d (interleaved, paired seeds)", c.N, i, r.Model, r.Seed, wantModels[i], wantSeeds[i])
			}
			if r.Tool != "bakeoff-erros" || r.Path != "/api/generate" {
				t.Errorf("call %+v: tool/path", r)
			}
			// the body equals the recorded prompt_json except options.seed (and model of the candidate)
			var rec map[string]json.RawMessage
			json.Unmarshal(c.promptJSON(), &rec)
			sent := map[string]json.RawMessage{}
			for k, v := range r.Body {
				sent[k] = v
			}
			var recOpts, sentOpts map[string]json.RawMessage
			json.Unmarshal(rec["options"], &recOpts)
			json.Unmarshal(sent["options"], &sentOpts)
			if string(sentOpts["seed"]) != fmt.Sprint(r.Seed) {
				t.Errorf("options.seed = %s", sentOpts["seed"])
			}
			delete(recOpts, "seed")
			delete(sentOpts, "seed")
			if len(recOpts) != 2 || fmt.Sprint(canon(t, recOpts)) != fmt.Sprint(canon(t, sentOpts)) {
				t.Errorf("options differ beyond the seed: %v vs %v", recOpts, sentOpts)
			}
			delete(rec, "options")
			delete(sent, "options")
			if r.Model != "qwen3-ia" { // the candidate: only the model changes
				delete(rec, "model")
				delete(sent, "model")
			}
			if fmt.Sprint(canon(t, rec)) != fmt.Sprint(canon(t, sent)) {
				t.Errorf("body differs beyond seed/model:\n%v\n%v", rec, sent)
			}
		}
	}
}

func canon(t *testing.T, m map[string]json.RawMessage) string {
	t.Helper()
	c, err := canonicalJSON(mustJSON(m))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestWithSeedAndModelUnits(t *testing.T) {
	pj := []byte(`{"model":"qwen3-ia","messages":[{"role":"user","content":"oi"}],"options":{"temperature":0.1,"seed":99},"format":"json","think":false}`)
	b, err := withSeedAndModel(pj, "/api/chat", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	var o map[string]float64
	json.Unmarshal(m["options"], &o)
	if o["seed"] != 2 || o["temperature"] != 0.1 || string(m["model"]) != `"qwen3-ia"` || string(m["format"]) != `"json"` || string(m["think"]) != "false" {
		t.Errorf("body: %s", b)
	}
	// no options at all: created
	b, _ = withSeedAndModel([]byte(`{"model":"m","prompt":"p"}`), "/api/generate", 3, "m2")
	if !strings.Contains(string(b), `"options":{"seed":3}`) || !strings.Contains(string(b), `"model":"m2"`) {
		t.Errorf("no options: %s", b)
	}
	// /v1 endpoints carry the seed at the top level
	b, _ = withSeedAndModel([]byte(`{"model":"m","messages":[]}`), "/v1/chat/completions", 1, "")
	if !strings.Contains(string(b), `"seed":1`) || strings.Contains(string(b), "options") {
		t.Errorf("/v1: %s", b)
	}
	if _, err := withSeedAndModel([]byte(`[1]`), "/api/chat", 1, ""); err == nil {
		t.Error("a request that is not an object must fail")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestParseReplyShapes(t *testing.T) {
	r, err := parseReply("/api/chat", strings.NewReader(`{"message":{"content":"ol"},"done":false}`+"\n"+`{"message":{"content":"á","thinking":"x"},"done":false}`+"\n"+`{"message":{"content":""},"done":true,"done_reason":"length"}`+"\n"))
	if err != nil || r.Text != "olá" || r.Fim != "length" || !r.Final {
		t.Errorf("chat stream: %+v %v", r, err)
	}
	r, err = parseReply("/v1/chat/completions", strings.NewReader(`{"choices":[{"message":{"content":"oi"},"finish_reason":"stop"}]}`))
	if err != nil || r.Text != "oi" || r.Fim != "stop" || !r.Final {
		t.Errorf("v1 json: %+v %v", r, err)
	}
	r, err = parseReply("/v1/chat/completions", strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"b\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	if err != nil || r.Text != "ab" || r.Fim != "stop" || !r.Final {
		t.Errorf("v1 sse: %+v %v", r, err)
	}
	if _, err := parseReply("/api/generate", strings.NewReader(`{"error":"modelo ausente"}`+"\n")); err == nil {
		t.Error("an error in the body must fail")
	}
	// a stream that ends without done is a CUT stream: an error, never a (short) answer
	if r, err := parseReply("/api/generate", strings.NewReader(`{"response":"parcial","done":false}`+"\n")); err == nil {
		t.Errorf("cut stream accepted: %+v", r)
	}
	if _, err := parseReply("/v1/chat/completions", strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n")); err == nil {
		t.Error("v1 stream without [DONE] or finish_reason accepted")
	}
	// a read error that is not EOF is an error too
	if _, err := parseReply("/api/generate", io.MultiReader(strings.NewReader(`{"response":"a","done":false}`+"\n"), errReader{})); err == nil {
		t.Error("a broken read accepted")
	}
	// even after a done line: a connection that broke while the body was being read is not a complete answer
	if r, err := parseReply("/api/generate", io.MultiReader(strings.NewReader(`{"response":"a","done":true,"done_reason":"stop"}`+"\n"), errReader{})); err == nil {
		t.Errorf("a broken read after done accepted: %+v", r)
	}
}

// ---- gate (C-5.13) -----------------------------------------------------------------------------------

func gateText(t *testing.T, out string) {
	t.Helper()
	low := strings.ToLower(out)
	if !strings.Contains(low, "dados insuficientes") {
		t.Errorf("the report must say \"dados insuficientes\":\n%s", out)
	}
	if strings.Contains(low, "sem regressão") || strings.Contains(low, "melhora aceita") {
		t.Errorf("a failed gate must never say \"sem regressão\" or accept anything:\n%s", out)
	}
}

func TestGateFailsWithNoCategoryAtThirty(t *testing.T) {
	var cs []synthCase
	for i := 1; i <= 29; i++ { // 29 < 30: reported, does not decide
		cs = append(cs, omitiu(i))
	}
	f := newFixture(t, cs, scenarioPolicy(false))
	rid := f.makeRetrato()
	eo := f.opts("base")
	eo.RetratoID = rid
	code, out := f.run(eo)
	if code != exitGate {
		t.Fatalf("code %d, want %d\n%s", code, exitGate, out)
	}
	gateText(t, out)
	if !strings.Contains(out, "nenhuma categoria atingiu o mínimo de 30") || !strings.Contains(out, "reportada; N < 30, não decide") {
		t.Errorf("reasons:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(f.out, "erros", rid, "linha-de-base.json")); err == nil {
		t.Error("a baseline from insufficient data must NOT be registered")
	}
}

func TestGateFailsWithZeroCasesEvaluated(t *testing.T) {
	f := newFixture(t, []synthCase{omitiu(1), omitiu(2)}, scenarioPolicy(false))
	rid := f.makeRetrato()
	f.gw.status = 500 // the suite cannot run
	eo := f.opts("base")
	eo.RetratoID = rid
	code, out := f.run(eo)
	if code != exitGate {
		t.Fatalf("code %d\n%s", code, out)
	}
	gateText(t, out)
	if !strings.Contains(out, "(0 casos)") || !strings.Contains(out, "não puderam ser medidos") {
		t.Errorf("reasons:\n%s", out)
	}
}

func TestGateFailsWithAnUnmeasuredCase(t *testing.T) { // a partial measure is never "ok"
	set := syntheticSet()
	f := newFixture(t, set, scenarioPolicy(false))
	rid := f.makeRetrato()
	delete(f.gw.cases, synthID(7)) // the gateway answers 404 for this case
	eo := f.opts("base")
	eo.RetratoID = rid
	code, out := f.run(eo)
	if code != exitGate || !strings.Contains(out, "1 caso(s) não puderam ser medidos") {
		t.Fatalf("code %d\n%s", code, out)
	}
	gateText(t, out)
}

func TestGateFailsWithAnInvalidRetratoAndNeverCallsTheGateway(t *testing.T) { // C-5.13, C-4.10
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	db, _ := sql.Open("sqlite", f.db)
	defer db.Close()
	if _, err := db.Exec(`UPDATE retratos SET valido=0 WHERE id=?`, rid); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"base", "comparar"} {
		eo := f.opts(mode)
		eo.RetratoID, eo.Candidato = rid, "qwen3-ia-v2"
		code, out := f.run(eo)
		if code != exitGate || !strings.Contains(out, "valido=0") {
			t.Fatalf("%s: code %d\n%s", mode, code, out)
		}
		gateText(t, out)
	}
	if n := len(f.gw.requests()); n != 0 {
		t.Errorf("an invalid retrato must not be used: %d calls", n)
	}
	// and the retrato that goes invalid between two runs is noticed at reading time
	rid2 := func() string {
		db.Exec(`DELETE FROM retratos`)
		return f.makeRetrato()
	}()
	if _, err := db.Exec(`UPDATE ocorrencias SET estado='descartada' WHERE id=?`, synthID(3)); err != nil {
		t.Fatal(err)
	}
	eo := f.opts("base")
	eo.RetratoID = rid2
	if code, out := f.run(eo); code != exitGate || !strings.Contains(out, "mudou desde o congelamento") {
		t.Fatalf("changed case: code %d\n%s", code, out)
	}
}

func TestGateFailsWithoutS2S3ForTheComparison(t *testing.T) { // C-5.12 cannot be checked
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "omitiu"
	code, out := f.run(ce)
	if code != exitGate || !strings.Contains(out, "S2 e o S3 da Fase 8 não foram medidos") {
		t.Fatalf("code %d\n%s", code, out)
	}
	gateText(t, out)
}

func TestGateFailsWhenTheTargetCannotDecide(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	writeScore(t, f.out, "qwen3-ia", 9, 10, 9, 10)
	writeScore(t, f.out, "qwen3-ia-v2", 9, 10, 9, 10)
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "truncado" // no case of that category
	code, out := f.run(ce)
	if code != exitGate || !strings.Contains(out, "a categoria-alvo truncado tem N=0") {
		t.Fatalf("code %d\n%s", code, out)
	}
	// a target judged by the owner is allowed: the result is pending until mode apurar
	if code, out := f.run(func() errosOpts { e := ce; e.Alvo = "inventou"; return e }()); code != exitPending {
		t.Fatalf("a blind target: code %d\n%s", code, out)
	}
	if code, out := f.run(func() errosOpts { e := ce; e.Alvo = "outro"; return e }()); code != exitUsage || !strings.Contains(out, "não tem pontuação") {
		t.Fatalf("a target without criterion: code %d\n%s", code, out)
	}
}

func TestComparisonRejectsAChangeThatDropsAnotherCategory(t *testing.T) { // C-5.11 end to end
	f := newFixture(t, syntheticSet(), scenarioPolicy(true)) // the candidate loses one formato_invalido case (3.3 points)
	rid := f.makeRetrato()
	writeScore(t, f.out, "qwen3-ia", 9, 10, 9, 10)
	writeScore(t, f.out, "qwen3-ia-v2", 9, 10, 9, 10)
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "omitiu"
	code, out := f.run(ce)
	if code != exitRejected || !strings.Contains(out, "MUDANÇA REPROVADA") || !strings.Contains(out, "C-5.11: a categoria formato_invalido caiu mais de 3 pontos") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if strings.Contains(strings.ToLower(out), "sem regressão") {
		t.Error("a rejected change cannot say \"sem regressão\"")
	}
}

func TestComparisonWarnsWhenTheBaselineAndTheNewMeasureDiffer(t *testing.T) { // C-5.16
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	writeScore(t, f.out, "qwen3-ia", 9, 10, 9, 10)
	writeScore(t, f.out, "qwen3-ia-v2", 9, 10, 9, 10)
	// a baseline from "last month": omitiu was 80%, formato_invalido the same as now (20/30)
	dir := filepath.Join(f.out, "erros", rid)
	os.MkdirAll(dir, 0o700)
	bf := baselineFile{RetratoID: rid, Modelo: "qwen3-ia", Cats: map[string]baselineCt{"omitiu": {N: 30, Acertos: 24}, "formato_invalido": {N: 30, Acertos: 20}}}
	os.WriteFile(filepath.Join(dir, "linha-de-base.json"), mustJSON(bf), 0o600)
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "omitiu"
	_, out := f.run(ce)
	if !strings.Contains(out, "C-5.16: categoria omitiu: linha de base 80,0% (N=30) e nova medição do modelo atual 46,7% (N=30)") {
		t.Errorf("no divergence warning:\n%s", out)
	}
	if strings.Contains(out, "C-5.16: categoria formato_invalido") {
		t.Errorf("a category within 3 points must not warn:\n%s", out)
	}
}

// ---- the report holds no text (C-5.6) -------------------------------------------------------------------

func TestReportsAndLogsNeverHoldCaseText(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	writeScore(t, f.out, "qwen3-ia", 9, 10, 9, 10)
	writeScore(t, f.out, "qwen3-ia-v2", 9, 10, 9, 10)
	be := f.opts("base")
	be.RetratoID = rid
	f.run(be)
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "omitiu"
	f.run(ce)
	f.gw.status = 500
	f.run(ce) // a failing run too
	files := sidecarFiles(t, f.out)
	scanned := 0
	for _, p := range files {
		if strings.HasPrefix(filepath.Base(p), "blind-") || strings.HasSuffix(p, "score.json") {
			continue // the blind files are the one place with text; the S2/S3 scores are from Fase 8
		}
		scanned++
		body := readFile(t, p)
		for _, m := range secretMarkers {
			if strings.Contains(body, m) {
				t.Errorf("%s contains case text (%q)", p, m)
			}
		}
	}
	if scanned < 3 {
		t.Fatalf("only %d files scanned", scanned)
	}
	for _, l := range f.logs {
		for _, m := range secretMarkers {
			if strings.Contains(l, m) {
				t.Errorf("a log line contains case text (%q)", m)
			}
		}
	}
}

// ---- blind (C-5.5, C-5.6) -----------------------------------------------------------------------------------

func TestBlindFileIsPrivateShuffledAndTheOnlyOneWithText(t *testing.T) {
	var cs []synthCase
	for i := 1; i <= 30; i++ {
		cs = append(cs, omitiu(i)) // so the gate has a deciding category
	}
	for i := 31; i <= 40; i++ {
		b := blindCase(i)
		cs = append(cs, b)
	}
	pol := func(model string, seed int, c synthCase) (string, string) {
		if c.Cat == "inventou" {
			if model == "qwen3-ia" {
				return "ATUAL-OUT-" + synthID(c.N)[28:], "stop"
			}
			return "CAND-OUT-" + synthID(c.N)[28:], "stop"
		}
		return answer(true, c)
	}
	f := newFixture(t, cs, pol)
	rid := f.makeRetrato()
	writeScore(t, f.out, "qwen3-ia", 9, 10, 9, 10)
	writeScore(t, f.out, "qwen3-ia-v2", 9, 10, 9, 10)
	ce := f.opts("comparar")
	ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "omitiu"
	code, out := f.run(ce)
	if code == exitUsage || !strings.Contains(out, "Avaliação humana às cegas") || !strings.Contains(out, "inventou: 10 caso(s)") {
		t.Fatalf("code %d\n%s", code, out)
	}
	// blind runs one round (seed 1), current then candidate, and nothing else
	var blindReqs []gwReq
	for _, r := range f.gw.requests() {
		if c := f.gw.cases[r.Key]; c.Cat == "inventou" {
			blindReqs = append(blindReqs, r)
		}
	}
	if len(blindReqs) != 20 {
		t.Fatalf("%d blind calls, want 20", len(blindReqs))
	}
	for i, r := range blindReqs {
		if r.Seed != 1 || (i%2 == 0) != (r.Model == "qwen3-ia") {
			t.Fatalf("blind call %d: %+v", i, r)
		}
	}
	var blindTxt, keyTSV string
	for _, p := range sidecarFiles(t, f.out) {
		switch {
		case strings.HasSuffix(p, ".chave.tsv"):
			keyTSV = p
		case strings.HasPrefix(filepath.Base(p), "blind-") && strings.HasSuffix(p, ".txt"):
			blindTxt = p
		}
	}
	if blindTxt == "" || keyTSV == "" {
		t.Fatal("blind files not written")
	}
	txt := readFile(t, blindTxt)
	for _, want := range []string{"SEGREDO-CLIENTE", "ATUAL-OUT-", "CAND-OUT-", "--- SAÍDA A", "--- SAÍDA B", "Só o owner lê"} {
		if !strings.Contains(txt, want) {
			t.Errorf("blind file misses %q", want)
		}
	}
	// the outputs are labelled A/B only; the file never says which is the current model
	if strings.Contains(txt, "qwen3-ia") || strings.Contains(txt, "candidato") || strings.Contains(txt, "atual") {
		t.Error("the blind file reveals which output is which")
	}
	// the key tells which is which, and the order is shuffled (both positions occur)
	key := readFile(t, keyTSV)
	if !strings.Contains(key, "atual\tcandidato") || !strings.Contains(key, "candidato\tatual") {
		t.Errorf("the A/B positions were not shuffled:\n%s", key)
	}
	if strings.Contains(key, "OUT-") || strings.Contains(key, "SEGREDO") {
		t.Error("the key file must not hold text")
	}
	// 0600 (C-5.6): not provable on Windows, where file modes do not exist
	for _, p := range []string{blindTxt, keyTSV} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS == "windows" {
			t.Logf("Windows: o modo 0600 de %s não é provável aqui (modos de arquivo não existem); a prova é feita no Linux", filepath.Base(p))
			continue
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s has mode %v, want 0600", p, fi.Mode().Perm())
		}
	}
	if privateFileMode != 0o600 {
		t.Errorf("privateFileMode = %v, want 0600 (the mode cannot be observed on Windows)", privateFileMode)
	}
	// the labels are not in the order of the retrato: the cases were shuffled too
	var ids []string
	for _, l := range strings.Split(strings.TrimSpace(key), "\n")[1:] {
		ids = append(ids, strings.Split(l, "\t")[1])
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	if strings.Join(ids, ",") == strings.Join(sorted, ",") || len(ids) != 10 {
		t.Errorf("the cases were not shuffled: %v", ids)
	}
	// the report names the blind file but holds no text
	for _, m := range secretMarkers {
		if strings.Contains(out, m) {
			t.Errorf("report contains %q", m)
		}
	}
}

func TestReplayClientNeverSendsToAnotherHost(t *testing.T) { // C-5.6
	// 1. a non-loopback gateway is refused unless the owner allows it
	if _, err := newReplayClient("http://10.1.2.3:11500", false, time.Second); err == nil {
		t.Error("a non-loopback gateway must be refused")
	}
	// -gateway-remoto: only a LITERAL private IP over https (never a name, never a public IP, never plain http)
	for _, ok := range []string{"https://10.1.2.3:11500", "https://192.168.0.9", "https://[fd00::1]:8443"} {
		if _, err := newReplayClient(ok, true, time.Second); err != nil {
			t.Errorf("-gateway-remoto %s: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://10.1.2.3:11500", "https://8.8.8.8", "https://203.0.113.5:1", "https://gateway.interno", "https://ia.cslsolucoes.com.br:8443", "http://gateway.interno"} {
		if _, err := newReplayClient(bad, true, time.Second); err == nil {
			t.Errorf("-gateway-remoto must refuse %s", bad)
		}
	}
	// the address is checked again on the connection: loopback always, private only with -gateway-remoto
	for ip, want := range map[string][2]bool{"127.0.0.1": {true, true}, "::1": {true, true}, "10.0.0.1": {false, true}, "192.168.1.1": {false, true}, "8.8.8.8": {false, false}, "169.254.1.1": {false, false}, "0.0.0.0": {false, false}} {
		if allowedConnIP(net.ParseIP(ip), false) != want[0] || allowedConnIP(net.ParseIP(ip), true) != want[1] {
			t.Errorf("allowedConnIP(%s): local %v remote %v, want %v", ip, allowedConnIP(net.ParseIP(ip), false), allowedConnIP(net.ParseIP(ip), true), want)
		}
	}
	if allowedConnIP(nil, true) {
		t.Error("a nil address was allowed")
	}
	for _, bad := range []string{"ftp://127.0.0.1", "127.0.0.1:11500", "http://", ""} {
		if _, err := newReplayClient(bad, true, time.Second); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	for _, ok := range []string{"http://127.0.0.1:11500", "http://localhost:1", "http://[::1]:9"} {
		if _, err := newReplayClient(ok, false, time.Second); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	// 2. a redirect to another host is not followed
	var hits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer other.Close()
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x", http.StatusTemporaryRedirect)
	}))
	defer gw.Close()
	rc, err := newReplayClient(gw.URL, false, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rc.do(t.Context(), "/api/generate", []byte(`{"prompt":"SEGREDO"}`)); err == nil || hits != 0 {
		t.Errorf("redirect: err=%v, hits at the other host=%d", err, hits)
	}
	// 3. the transport itself refuses any other host (defence if a URL slips through)
	req, _ := http.NewRequest("POST", other.URL+"/api/generate", strings.NewReader("SEGREDO"))
	if _, err := rc.hc.Do(req); err == nil || hits != 0 {
		t.Errorf("transport guard: err=%v hits=%d", err, hits)
	}
	// 4. only generation endpoints are ever called (a plain gateway that counts what reaches it)
	var gwHits int
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gwHits++ }))
	defer plain.Close()
	rp, _ := newReplayClient(plain.URL, false, 5*time.Second)
	for _, ep := range []string{"/api/pull", "/api/embeddings", "/api/delete", "/x"} {
		if _, err := rp.do(t.Context(), ep, []byte(`{}`)); err == nil {
			t.Errorf("%s must be refused", ep)
		}
	}
	if gwHits != 0 {
		t.Errorf("a non-generation endpoint reached the gateway (%d hits)", gwHits)
	}
	// 5. no proxy: Go skips the proxy for loopback hosts, so the proof is the transport itself
	tr, ok := rc.hc.Transport.(guardTransport)
	if !ok {
		t.Fatalf("transport is %T, want guardTransport", rc.hc.Transport)
	}
	if ht, ok := tr.rt.(*http.Transport); !ok || ht.Proxy != nil {
		t.Errorf("the replay transport must not use a proxy: %#v", tr.rt)
	}
	if rc.hc.CheckRedirect == nil || rc.hc.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Error("the replay client must not follow redirects")
	}
}

func TestErrosRefusesARemoteGatewayAndMissingInputs(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	eo := f.opts("base")
	eo.RetratoID, eo.GatewayURL, eo.Timeout = rid, "http://192.0.2.1:11500", 200*time.Millisecond // TEST-NET-1: never routed; the short timeout keeps a regression fast
	if code, out := f.run(eo); code != exitUsage || !strings.Contains(out, "loopback") {
		t.Errorf("remote gateway: code %d\n%s", code, out)
	}
	if n := len(f.gw.requests()); n != 0 {
		t.Errorf("%d calls", n)
	}
	for name, mod := range map[string]func(*errosOpts){
		"no mode":           func(e *errosOpts) { e.Mode = "x" },
		"no db":             func(e *errosOpts) { e.DB = "" },
		"missing db":        func(e *errosOpts) { e.DB = filepath.Join(f.dir, "nao-existe.db") },
		"no retrato":        func(e *errosOpts) { e.RetratoID = "" },
		"retrato not an id": func(e *errosOpts) { e.RetratoID = "../../x" },
		"unknown retrato":   func(e *errosOpts) { e.RetratoID = strings.Repeat("0", 32) },
		"compare no model":  func(e *errosOpts) { e.Mode, e.Candidato = "comparar", "" },
	} {
		e := f.opts("base")
		e.RetratoID = rid
		mod(&e)
		if code, out := f.run(e); code != exitUsage {
			t.Errorf("%s: code %d\n%s", name, code, out)
		}
	}
	if _, err := os.Stat(filepath.Join(f.dir, "x")); err == nil { // "../../x" under out/erros would be dir/x
		t.Error("a -retrato that is not an id escaped the output folder")
	}
	if _, err := os.Stat(filepath.Join(f.dir, "nao-existe.db")); err == nil {
		t.Error("a mistyped -treino-db must not create an empty database")
	}
}

func TestRetratoModeWithoutCasesFailsWithInsufficientData(t *testing.T) {
	o := fill(omitiu(1))
	o.Cat = "outro"
	f := newFixture(t, []synthCase{o}, scenarioPolicy(false))
	code, out := f.run(f.opts("retrato"))
	if code != exitGate {
		t.Fatalf("code %d\n%s", code, out)
	}
	gateText(t, out)
}

func TestSecretsAreNeverWritten(t *testing.T) { // the program has no token and never prints a key
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	be := f.opts("base")
	be.RetratoID = rid
	f.run(be)
	for _, p := range sidecarFiles(t, f.out) {
		if strings.Contains(readFile(t, p), testKeyHex) {
			t.Errorf("%s holds the HMAC key", p)
		}
	}
	for _, l := range f.logs {
		if strings.Contains(l, testKeyHex) {
			t.Error("a log holds the HMAC key")
		}
	}
}

func TestReadS23(t *testing.T) { // the S2 (number and title) and S3 (formats) of Fase 8, from score.json
	dir := t.TempDir()
	writeScore(t, dir, "m", 83, 100, 25, 30)
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC)
	got, err := readS23(filepath.Join(dir, "m", "score.json"), "m", now, 48*time.Hour)
	if err != nil || *got != (s23{83, 100, 25, 30}) {
		t.Fatalf("%+v %v", got, err)
	}
	for name, body := range map[string]string{"empty": `{}`, "no s3": `{"s2":[{"numero_e_titulo_ok":1,"total_gabarito":2}]}`, "not json": "x"} {
		p := filepath.Join(dir, name+".json")
		os.WriteFile(p, []byte(body), 0o600)
		if _, err := readS23(p, "m", now, 48*time.Hour); err == nil {
			t.Errorf("%s must fail (the gate treats it as not measured)", name)
		}
	}
	if _, err := readS23(filepath.Join(dir, "missing.json"), "m", now, 48*time.Hour); err == nil {
		t.Error("a missing score.json must fail")
	}
	p := filepath.Join(dir, "m", "score.json")
	for name, c := range map[string]struct {
		model string
		now   time.Time
	}{
		"another model":   {"outro", now},
		"older than 48 h": {"m", now.Add(72 * time.Hour)},
		"from the future": {"m", now.Add(-24 * time.Hour)},
	} {
		if _, err := readS23(p, c.model, c.now, 48*time.Hour); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	// an old score.json (no model and no date written) is not accepted
	os.WriteFile(filepath.Join(dir, "old.json"), []byte(`{"s2":[{"numero_e_titulo_ok":1,"total_gabarito":2}],"s3":{"a":{"ok":true}}}`), 0o600)
	if _, err := readS23(filepath.Join(dir, "old.json"), "m", now, 48*time.Hour); err == nil {
		t.Error("a score.json without model and date must be refused")
	}
}

func TestWriteNewNeverOverwrites(t *testing.T) { // the baseline, reports and blind files are never replaced
	p := filepath.Join(t.TempDir(), "x.txt")
	if err := writeNew(p, []byte("primeiro")); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(p, []byte("segundo")); err == nil {
		t.Fatal("an existing file was overwritten")
	}
	if readFile(t, p) != "primeiro" {
		t.Error("the first content was lost")
	}
}
