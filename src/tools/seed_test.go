package tools

import (
	"net/http"
	"strings"
	"testing"
)

// Fase 14: `seed` on generate and chat (D14-3, D14-9, D14-12, D14-13a). The
// tool -> Ollama wire body is proved here; gateway -> prompt_json is proved
// by gateway/joblog/fase12_prompt_json_test.go (C-2.10).

// seedTool describes one tool under test: a mock handler, a valid argument
// object body (without the closing brace) and how to build the call.
type seedTool struct {
	name    string
	handler http.HandlerFunc
	base    string // JSON members, no braces
}

func seedTools() []seedTool {
	return []seedTool{
		{"generate", generateOK, `"prompt":"oi"`},
		{"chat", chatOKWith("stop", "ok"), `"messages":[{"role":"user","content":"oi"}]`},
	}
}

// args builds the JSON arguments: the base members plus extra (may be empty).
func (s seedTool) args(extra string) string {
	if extra == "" {
		return "{" + s.base + "}"
	}
	return "{" + s.base + "," + extra + "}"
}

// seedOnWire returns options.seed of the first request and whether it is present.
func seedOnWire(t *testing.T, m *mock) (any, bool) {
	t.Helper()
	b := m.body(0)
	if b == nil {
		t.Fatal("no request reached the mock")
	}
	o, _ := b["options"].(map[string]any)
	v, ok := o["seed"]
	return v, ok
}

// (a) + (c): the exact value reaches options.seed, boundaries included.
func TestSeedReachesTheWire(t *testing.T) {
	for _, tl := range seedTools() {
		for _, lit := range []string{"7", "1", "2147483647"} {
			t.Run(tl.name+"/"+lit, func(t *testing.T) {
				m := newMock(t, tl.handler)
				if _, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":`+lit)); err != nil {
					t.Fatal(err)
				}
				got, ok := seedOnWire(t, m)
				want := map[string]float64{"7": 7, "1": 1, "2147483647": 2147483647}[lit]
				if !ok || got != want {
					t.Fatalf("options.seed = %v (present %v), want %v; body %+v", got, ok, want, m.body(0))
				}
			})
		}
	}
}

// (b): without seed the key is absent and options behaves as before.
func TestSeedAbsentWhenOmitted(t *testing.T) {
	for _, tl := range seedTools() {
		t.Run(tl.name, func(t *testing.T) {
			m := newMock(t, tl.handler)
			d := m.deps(t, nil)
			if _, err := call(t, d, tl.name, tl.args("")); err != nil {
				t.Fatal(err)
			}
			if _, has := m.body(0)["options"]; has {
				t.Fatalf("options must be absent with no sampling argument: %+v", m.body(0))
			}
			if _, err := call(t, d, tl.name, tl.args(`"temperature":0.5`)); err != nil {
				t.Fatal(err)
			}
			o, has := m.body(1)["options"].(map[string]any)
			if !has || o["temperature"] != 0.5 {
				t.Fatalf("options lost temperature: %+v", m.body(1))
			}
			if _, has := o["seed"]; has {
				t.Fatalf("seed key present without the argument: %+v", o)
			}
		})
	}
}

// (d): invalid values are refused with the standard message and never reach Ollama.
func TestSeedInvalidNeverReachesOllama(t *testing.T) {
	cases := map[string]string{
		`0`:          "`seed` deve estar entre",
		`-0`:         "`seed` deve estar entre",
		`-1`:         "`seed` deve estar entre",
		`2147483648`: "`seed` deve estar entre",
		`1.5`:        "`seed` must be an integer",
		`"7"`:        "`seed` must be an integer",
		`true`:       "`seed` must be an integer",
		`{}`:         "`seed` must be an integer",
		`[]`:         "`seed` must be an integer",
	}
	for _, tl := range seedTools() {
		m := newMock(t, tl.handler)
		d := m.deps(t, nil)
		for lit, want := range cases {
			_, err := call(t, d, tl.name, tl.args(`"seed":`+lit))
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%s seed=%s -> %v (want %q)", tl.name, lit, err, want)
			}
		}
		if n := m.hits.Load(); n != 0 {
			t.Errorf("%s: invalid seed reached Ollama: %d requests", tl.name, n)
		}
	}
}

// (e): 1.0 and 1e3 are integers once decoded and must not startle.
func TestSeedIntegralFloatsAccepted(t *testing.T) {
	for _, tl := range seedTools() {
		for lit, want := range map[string]float64{"1.0": 1, "1e3": 1000} {
			t.Run(tl.name+"/"+lit, func(t *testing.T) {
				m := newMock(t, tl.handler)
				if _, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":`+lit)); err != nil {
					t.Fatal(err)
				}
				if got, ok := seedOnWire(t, m); !ok || got != want {
					t.Fatalf("options.seed = %v (present %v), want %v", got, ok, want)
				}
			})
		}
	}
}

// (f): null is omitted (D14-9), exactly like temperature and num_predict.
func TestSeedNullIsOmitted(t *testing.T) {
	for _, tl := range seedTools() {
		t.Run(tl.name, func(t *testing.T) {
			m := newMock(t, tl.handler)
			if _, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":null`)); err != nil {
				t.Fatalf("seed:null must not be an error: %v", err)
			}
			if _, has := m.body(0)["options"]; has {
				t.Fatalf("seed:null alone must leave options absent: %+v", m.body(0))
			}
			if _, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":null,"num_predict":9`)); err != nil {
				t.Fatal(err)
			}
			if o, _ := m.body(1)["options"].(map[string]any); o["num_predict"] != float64(9) {
				t.Fatalf("num_predict lost: %+v", m.body(1))
			} else if _, has := o["seed"]; has {
				t.Fatalf("seed:null leaked into options: %+v", o)
			}
		})
	}
}

// (g): unknown arguments are still refused next to a valid seed.
func TestSeedKeepsUnknownArgumentsRefused(t *testing.T) {
	for _, tl := range seedTools() {
		m := newMock(t, tl.handler)
		_, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":3,"top_p":0.9`))
		if err == nil || !strings.Contains(err.Error(), "argumento desconhecido") {
			t.Errorf("%s: unknown argument accepted: %v", tl.name, err)
		} else {
			// Only top_p may be listed as unknown: if seed were missing from the
			// accepted list it would show up here too (S4 I-1, mutation proof).
			unknown, _, _ := strings.Cut(err.Error(), " (aceitos:")
			if !strings.Contains(unknown, "top_p") || strings.Contains(unknown, "seed") {
				t.Errorf("%s: wrong unknown list: %v", tl.name, err)
			}
		}
		if m.hits.Load() != 0 {
			t.Errorf("%s: reached Ollama", tl.name)
		}
	}
}

// (h): seed + temperature + num_predict arrive together; think and
// numPredictOf are unaffected.
func TestSeedCombinedWithOtherOptionsAndThink(t *testing.T) {
	for _, tl := range seedTools() {
		t.Run(tl.name, func(t *testing.T) {
			m := newMock(t, tl.handler)
			if _, err := call(t, m.deps(t, nil), tl.name, tl.args(`"seed":42,"temperature":0.7,"num_predict":33,"think":true`)); err != nil {
				t.Fatal(err)
			}
			o, _ := m.body(0)["options"].(map[string]any)
			if o["seed"] != float64(42) || o["temperature"] != 0.7 || o["num_predict"] != float64(33) {
				t.Fatalf("options: %+v", o)
			}
			assertThink(t, m, 0, true)
		})
	}
	// numPredictOf only reflects num_predict, seed or not.
	a, _ := parseArgs([]byte(`{"seed":5}`), "seed", "temperature", "num_predict")
	o, err := samplingOptions(a)
	if err != nil || o == nil || o.Seed == nil || *o.Seed != 5 {
		t.Fatalf("samplingOptions seed-only: %+v err=%v", o, err)
	}
	if numPredictOf(o) != nil {
		t.Fatalf("numPredictOf must stay nil without num_predict: %v", *numPredictOf(o))
	}
}

// (i): chat with system and several messages keeps working with a seed.
func TestChatWithSystemAndMessagesAndSeed(t *testing.T) {
	m := newMock(t, chatOKWith("stop", "ok"))
	res, err := call(t, m.deps(t, nil), "chat", `{"messages":[{"role":"user","content":"oi"},{"role":"assistant","content":"ola"},{"role":"user","content":"e ai"}],"system":"seja gentil","seed":11}`)
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	msgs, _ := m.body(0)["messages"].([]any)
	if len(msgs) != 4 || msgs[0].(map[string]any)["role"] != "system" {
		t.Fatalf("messages: %+v", msgs)
	}
	if got, ok := seedOnWire(t, m); !ok || got != float64(11) {
		t.Fatalf("options.seed = %v (present %v)", got, ok)
	}
}

// The schema advertises seed on generate and chat only, with the same bounds
// the validation enforces.
func TestSeedInSchemaOfGenerateAndChatOnly(t *testing.T) {
	for _, def := range Defs(New(CatalogDeps())) {
		props, _ := def.InputSchema["properties"].(schemaMap)
		p, has := props["seed"].(schemaMap)
		want := def.Name == "generate" || def.Name == "chat"
		if has != want {
			t.Errorf("%s: seed in schema = %v, want %v", def.Name, has, want)
			continue
		}
		if has && (p["type"] != "integer" || p["minimum"] != 1 || p["maximum"] != maxSeed) {
			t.Errorf("%s: seed schema %+v", def.Name, p)
		}
	}
}
