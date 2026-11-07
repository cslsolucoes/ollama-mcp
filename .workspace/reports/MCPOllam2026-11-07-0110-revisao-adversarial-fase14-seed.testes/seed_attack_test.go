package tools

// Revisão adversarial S4 da Fase 14 (seed em generate e chat). Arquivo de
// ataque: NÃO faz parte do repositório. Para rodar, copiar temporariamente
// para src/tools/zz_seed_attack_test.go, rodar
//   go test -count=1 -run TestAtkSeed -v ./tools
// e apagar a cópia. Cada linha "VEC|id|OK|..." registra um vetor que passou.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cslsoftwares/mcpollama/ollama"
)

// rawCapture wraps a mock handler and keeps the raw request bodies.
type rawCapture struct {
	mu   sync.Mutex
	raws []string
}

func (rc *rawCapture) wrap(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.raws = append(rc.raws, string(b))
		rc.mu.Unlock()
		h(w, r)
	}
}

func (rc *rawCapture) raw(n int) string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if n >= len(rc.raws) {
		return ""
	}
	return rc.raws[n]
}

// A01: the exact bytes on the wire. The seed is an integer literal (never
// 2.147483647e+09) and a seed-only call sends options with the seed only.
func TestAtkSeedA01RawWire(t *testing.T) {
	for _, tl := range seedTools() {
		for _, lit := range []string{"1", "2147483647", "1e3", "1.0"} {
			rc := &rawCapture{}
			m := newMock(t, rc.wrap(tl.handler))
			if _, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":`+lit)); err != nil {
				t.Fatalf("%s seed=%s: %v", tl.name, lit, err)
			}
			want := map[string]string{"1": "1", "2147483647": "2147483647", "1e3": "1000", "1.0": "1"}[lit]
			if !strings.Contains(rc.raw(0), `"options":{"seed":`+want+`}`) {
				t.Fatalf("%s seed=%s: raw body %s", tl.name, lit, rc.raw(0))
			}
		}
		t.Logf("VEC|A01|OK|%s: seed literal inteiro no corpo cru; seed sozinho => options={\"seed\":N}", tl.name)
	}
}

type atkCase struct {
	lit     string
	accept  bool
	value   float64 // when accept
	errPart string  // when !accept
}

// A02: literal table (numbers at the edges, types, invalid JSON).
func TestAtkSeedA02Literals(t *testing.T) {
	cases := []atkCase{
		// accepted, as documented in the plan (S3 e)
		{"1E3", true, 1000, ""},
		{"1.0e0", true, 1, ""},
		{"21474836.47e2", true, 2147483647, ""},
		// accepted by float64 rounding: SURPRISES (finding MENOR)
		{"2147483647.0000000001", true, 2147483647, ""},
		{"0.99999999999999999", true, 1, ""},
		// refused: out of range
		{"1e308", false, 0, "`seed` deve estar entre 1 e 2147483647"},
		{"9007199254740993", false, 0, "`seed` deve estar entre"},
		{"-9007199254740993", false, 0, "`seed` deve estar entre"},
		{"-2147483648", false, 0, "`seed` deve estar entre"},
		{"-0.0", false, 0, "`seed` deve estar entre"},
		{"0.0", false, 0, "`seed` deve estar entre"},
		{"0e10", false, 0, "`seed` deve estar entre"},
		{"1e-400", false, 0, "`seed` deve estar entre"}, // underflow decodes to 0
		// refused: not an integer
		{"2147483647.5", false, 0, "`seed` must be an integer (recebido: number)"},
		{"1.5e0", false, 0, "`seed` must be an integer"},
		{`"7"`, false, 0, "`seed` must be an integer (recebido: string)"},
		{`"7"`, false, 0, "`seed` must be an integer (recebido: string)"},
		{`[7]`, false, 0, "`seed` must be an integer (recebido: array)"},
		{`{"v":7}`, false, 0, "`seed` must be an integer (recebido: object)"},
		{`true`, false, 0, "`seed` must be an integer (recebido: boolean)"},
		{`false`, false, 0, "`seed` must be an integer (recebido: boolean)"},
		// refused: invalid JSON or number outside float64 -> generic message
		{"1e400", false, 0, "os argumentos devem ser um objeto JSON"},
		{"-1e400", false, 0, "os argumentos devem ser um objeto JSON"},
		{"NaN", false, 0, "os argumentos devem ser um objeto JSON"},
		{"Infinity", false, 0, "os argumentos devem ser um objeto JSON"},
		{"-Infinity", false, 0, "os argumentos devem ser um objeto JSON"},
		{"+1", false, 0, "os argumentos devem ser um objeto JSON"},
		{"01", false, 0, "os argumentos devem ser um objeto JSON"},
		{"0x10", false, 0, "os argumentos devem ser um objeto JSON"},
		{".5", false, 0, "os argumentos devem ser um objeto JSON"},
		{"1.", false, 0, "os argumentos devem ser um objeto JSON"},
	}
	for _, tl := range seedTools() {
		for _, c := range cases {
			m := newMock(t, tl.handler)
			_, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":`+c.lit))
			if c.accept {
				if err != nil {
					t.Errorf("%s seed=%s: want accepted, got %v", tl.name, c.lit, err)
					continue
				}
				if got, ok := seedOnWire(t, m); !ok || got != c.value {
					t.Errorf("%s seed=%s: wire %v (present %v), want %v", tl.name, c.lit, got, ok, c.value)
				}
				continue
			}
			if err == nil || !strings.Contains(err.Error(), c.errPart) {
				t.Errorf("%s seed=%s: err %v, want %q", tl.name, c.lit, err, c.errPart)
			}
			if m.hits.Load() != 0 {
				t.Errorf("%s seed=%s: reached Ollama", tl.name, c.lit)
			}
		}
		t.Logf("VEC|A02|OK|%s: %d literais de seed com o resultado esperado (inclui 2 aceitos por arredondamento de float64)", tl.name, len(cases))
	}
}

// A03: duplicate keys: encoding/json keeps the LAST one.
func TestAtkSeedA03DuplicateKeys(t *testing.T) {
	for _, tl := range seedTools() {
		m := newMock(t, tl.handler)
		d := m.deps(t, nil)
		if _, err := call(t, d, tl.name, tl.args(`"seed":0,"seed":5`)); err != nil {
			t.Fatalf("%s {seed:0,seed:5}: %v", tl.name, err)
		}
		if got, _ := seedOnWire(t, m); got != float64(5) {
			t.Fatalf("%s: last key must win, got %v", tl.name, got)
		}
		if _, err := call(t, d, tl.name, tl.args(`"seed":5,"seed":0`)); err == nil || !strings.Contains(err.Error(), "deve estar entre") {
			t.Fatalf("%s {seed:5,seed:0}: %v", tl.name, err)
		}
		if _, err := call(t, d, tl.name, tl.args(`"seed":5,"seed":null`)); err != nil {
			t.Fatalf("%s {seed:5,seed:null}: %v", tl.name, err)
		}
		if _, has := m.body(1)["options"]; has {
			t.Fatalf("%s {seed:5,seed:null}: options present %+v", tl.name, m.body(1))
		}
		t.Logf("VEC|A03|OK|%s: chave repetida => vale a ÚLTIMA (0,5 aceito como 5; 5,0 recusado; 5,null = omitido)", tl.name)
	}
}

// A04: key spelling: map keys are case-sensitive in parseArgs.
func TestAtkSeedA04KeyCase(t *testing.T) {
	for _, tl := range seedTools() {
		m := newMock(t, tl.handler)
		d := m.deps(t, nil)
		for _, k := range []string{`"Seed"`, `"SEED"`, `"seed "`, `"ѕeed"`, `"seed"`} {
			_, err := call(t, d, tl.name, tl.args(k+`:7`))
			if k == `"seed"` { // the escape decodes to "seed": accepted
				if err != nil {
					t.Fatalf("%s %s: %v", tl.name, k, err)
				}
				continue
			}
			if err == nil || !strings.Contains(err.Error(), "argumento desconhecido") {
				t.Fatalf("%s key %s: %v", tl.name, k, err)
			}
		}
		if m.hits.Load() != 1 {
			t.Fatalf("%s: hits %d, want 1 (only the escaped spelling)", tl.name, m.hits.Load())
		}
		t.Logf("VEC|A04|OK|%s: Seed/SEED/\"seed \"/homóglifo cirílico recusados como desconhecidos; \\u0065 escapado decodifica para seed e é aceito", tl.name)
	}
}

// A05: the arguments object itself: BOM, array, null, seed first.
func TestAtkSeedA05ArgumentsShape(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	for _, in := range []string{"\xEF\xBB\xBF{\"prompt\":\"oi\",\"seed\":3}", `[{"prompt":"oi","seed":3}]`, `"seed"`, `3`} {
		if _, err := call(t, d, "generate", in); err == nil || !strings.Contains(err.Error(), "os argumentos devem ser um objeto JSON") {
			t.Fatalf("%q: %v", in, err)
		}
	}
	if _, err := call(t, d, "generate", `null`); err == nil || !strings.Contains(err.Error(), "`prompt` is required") {
		t.Fatalf("null arguments: %v", err)
	}
	if _, err := call(t, d, "generate", `{"seed":3,"prompt":"oi"}`); err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if got, ok := seedOnWire(t, m); !ok || got != float64(3) {
		t.Fatalf("seed first: wire %v", got)
	}
	t.Log("VEC|A05|OK|BOM, array, string e número como argumentos => mensagem genérica; null => prompt obrigatório; seed antes de prompt funciona")
}

// A06: validation order and message consistency with the sibling arguments.
func TestAtkSeedA06OrderAndMessages(t *testing.T) {
	m := newMock(t, generateOK)
	d := m.deps(t, nil)
	_, err := call(t, d, "generate", `{"prompt":"x","seed":0,"temperature":5}`)
	if err == nil || !strings.Contains(err.Error(), "`temperature`") {
		t.Fatalf("temperature is validated before seed: %v", err)
	}
	_, err = call(t, d, "generate", `{"prompt":"x","seed":0,"num_predict":0}`)
	if err == nil || !strings.Contains(err.Error(), "`num_predict`") {
		t.Fatalf("num_predict is validated before seed: %v", err)
	}
	_, err = call(t, d, "generate", `{"prompt":"x","seed":0,"think":"sim"}`)
	if err == nil || !strings.Contains(err.Error(), "`seed`") {
		t.Fatalf("seed is validated before think: %v", err)
	}
	_, e1 := call(t, d, "generate", `{"prompt":"x","seed":0}`)
	_, e2 := call(t, d, "generate", `{"prompt":"x","num_predict":0}`)
	n1 := strings.Replace(e1.Error(), "`seed` deve estar entre 1 e 2147483647", "X", 1)
	n2 := strings.Replace(e2.Error(), "`num_predict` deve estar entre 1 e ", "X", 1)
	if n1 != "X" || !strings.HasPrefix(n2, "X") {
		t.Fatalf("message shape differs: %q vs %q", e1, e2)
	}
	t.Logf("VEC|A06|OK|ordem: temperature, num_predict, seed, think; mensagem igual à dos irmãos: %q", e1.Error())
}

// A07: the fallback server receives the same seed (same wire bytes).
func TestAtkSeedA07Fallback(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close() // connection refused: neverSent -> fallback
	for _, tl := range seedTools() {
		rc := &rawCapture{}
		fb := newMock(t, rc.wrap(tl.handler))
		d := fb.deps(t, func(o *ollama.Options) {
			o.BaseURL = deadURL
			o.FallbackURL = fb.srv.URL
			o.FallbackAPIKey = "fb-" + sentinel
		})
		res, err := call(t, d, tl.name, tl.args(`"seed":77`))
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", tl.name, err, res)
		}
		if !strings.Contains(rc.raw(0), `"options":{"seed":77}`) {
			t.Fatalf("%s: fallback body %s", tl.name, rc.raw(0))
		}
		t.Logf("VEC|A07|OK|%s: o fallback recebe o mesmo options.seed", tl.name)
	}
}

// A08: seed with the explicit think:false and with a model per call.
func TestAtkSeedA08ThinkFalseAndModel(t *testing.T) {
	for _, tl := range seedTools() {
		m := newMock(t, tl.handler)
		d := m.deps(t, nil)
		d.AllowedModels = []string{"qwen3-ia", "outro:7b"}
		if _, err := call(t, d, tl.name, tl.args(`"seed":9,"think":false,"model":"outro:7b"`)); err != nil {
			t.Fatal(err)
		}
		b := m.body(0)
		if b["model"] != "outro:7b" || b["think"] != false {
			t.Fatalf("%s: %+v", tl.name, b)
		}
		if got, _ := seedOnWire(t, m); got != float64(9) {
			t.Fatalf("%s: seed %v", tl.name, got)
		}
		// a disallowed model with a valid seed: refused before Ollama
		if _, err := call(t, d, tl.name, tl.args(`"seed":9,"model":"proibido"`)); err == nil {
			t.Fatalf("%s: disallowed model accepted", tl.name)
		}
		if m.hits.Load() != 1 {
			t.Fatalf("%s: hits %d", tl.name, m.hits.Load())
		}
		t.Logf("VEC|A08|OK|%s: seed + think:false + modelo por chamada; modelo proibido com seed válido não chega ao Ollama", tl.name)
	}
}

// A09: samplingOptions directly: every presence combination.
func TestAtkSeedA09SamplingCombos(t *testing.T) {
	for _, in := range []string{`{}`, `{"temperature":0}`, `{"num_predict":1}`, `{"seed":1}`,
		`{"temperature":0,"seed":1}`, `{"num_predict":1,"seed":1}`, `{"temperature":0,"num_predict":1,"seed":1}`,
		`{"temperature":null,"num_predict":null,"seed":null}`} {
		a, err := parseArgs([]byte(in), "temperature", "num_predict", "seed")
		if err != nil {
			t.Fatal(err)
		}
		o, err := samplingOptions(a)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		_, ht := a.get("temperature")
		_, hn := a.get("num_predict")
		_, hs := a.get("seed")
		if (o == nil) != (!ht && !hn && !hs) {
			t.Fatalf("%s: options %+v", in, o)
		}
		if o != nil && ((o.Temperature != nil) != ht || (o.NumPredict != nil) != hn || (o.Seed != nil) != hs || o.TopP != nil) {
			t.Fatalf("%s: fields %+v", in, o)
		}
		if o != nil && numPredictOf(o) != o.NumPredict {
			t.Fatalf("%s: numPredictOf", in)
		}
	}
	// temperature 0 is kept (pointer, not omitempty on the value)
	a, _ := parseArgs([]byte(`{"temperature":0,"seed":1}`), "temperature", "seed")
	o, _ := samplingOptions(a)
	if o.Temperature == nil || *o.Temperature != 0 {
		t.Fatal("temperature 0 lost")
	}
	t.Log("VEC|A09|OK|samplingOptions: 8 combinações de presença; nil só quando os três faltam; top_p nunca preenchido; numPredictOf inalterado")
}
