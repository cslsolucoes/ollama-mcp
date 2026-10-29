package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// capture is a fake Ollama that records the body it receives.
type capture struct {
	mu     sync.Mutex
	bodies []string
	lens   []int64
	hdrs   []http.Header
	calls  atomic.Int64
}

func (c *capture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.bodies = append(c.bodies, string(b))
	c.lens = append(c.lens, r.ContentLength)
	c.hdrs = append(c.hdrs, r.Header.Clone())
	c.mu.Unlock()
	c.calls.Add(1)
	w.Header().Set("Content-Type", "application/x-ndjson")
	io.WriteString(w, finalChunk+"\n")
}

func (c *capture) last(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bodies) == 0 {
		t.Fatal("the Ollama got no call")
	}
	return c.bodies[len(c.bodies)-1]
}

func defaultPolicy(c *Config) {
	c.ThinkSuffix = "-think"
	c.StripOptions = DefaultStripOptions
}

// obj decodes a JSON object for comparison.
func obj(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not a JSON object: %v: %q", err, s)
	}
	return m
}

func sameJSON(t *testing.T, got, want string) {
	t.Helper()
	g, w := obj(t, got), obj(t, want)
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Fatalf("body sent to the Ollama:\n got  %s\n want %s", got, want)
	}
}

func TestPolicyNativeAndV1Table(t *testing.T) {
	type tc struct {
		name, path, in, want, think string
		hdr                         string
		mod                         func(*Config)
	}
	cases := []tc{
		{name: "native: injects think:false", path: "/api/generate", in: `{"model":"qwen3-ia","prompt":"oi"}`,
			want: `{"model":"qwen3-ia","prompt":"oi","think":false}`, think: ThinkOff},
		{name: "native chat: injects think:false", path: "/api/chat", in: `{"model":"qwen3-ia","messages":[{"role":"user","content":"oi"}]}`,
			want: `{"model":"qwen3-ia","messages":[{"role":"user","content":"oi"}],"think":false}`, think: ThinkOff},
		{name: "native: default-think true injects true", path: "/api/generate", in: `{"model":"qwen3-ia","prompt":"oi"}`,
			want: `{"model":"qwen3-ia","prompt":"oi","think":true}`, think: ThinkOn, mod: func(c *Config) { c.DefaultThink = true }},
		{name: "native: explicit think:true respected", path: "/api/generate", in: `{"model":"qwen3-ia","prompt":"oi","think":true}`,
			want: `{"model":"qwen3-ia","prompt":"oi","think":true}`, think: ThinkOn},
		{name: "native: explicit think:false respected over header on", path: "/api/generate", in: `{"model":"qwen3-ia","prompt":"oi","think":false}`,
			want: `{"model":"qwen3-ia","prompt":"oi","think":false}`, think: ThinkOff, hdr: "on"},
		{name: "native: think string (effort level) respected", path: "/api/chat", in: `{"model":"qwen3-ia","think":"high"}`,
			want: `{"model":"qwen3-ia","think":"high"}`, think: ThinkOn},
		{name: "native: think null is replaced", path: "/api/generate", in: `{"model":"qwen3-ia","think":null}`,
			want: `{"model":"qwen3-ia","think":false}`, think: ThinkOff},
		{name: "native: header on", path: "/api/generate", in: `{"model":"qwen3-ia","prompt":"x"}`,
			want: `{"model":"qwen3-ia","prompt":"x","think":true}`, think: ThinkOn, hdr: "on"},
		{name: "native: header off beats default-think true", path: "/api/generate", in: `{"model":"qwen3-ia","prompt":"x"}`,
			want: `{"model":"qwen3-ia","prompt":"x","think":false}`, think: ThinkOff, hdr: " OFF ", mod: func(c *Config) { c.DefaultThink = true }},
		{name: "native: invalid header ignored", path: "/api/generate", in: `{"model":"qwen3-ia","prompt":"x"}`,
			want: `{"model":"qwen3-ia","prompt":"x","think":false}`, think: ThinkOff, hdr: "maybe"},
		{name: "native: -think name -> base + think true", path: "/api/generate", in: `{"model":"qwen3-ia-think","prompt":"x"}`,
			want: `{"model":"qwen3-ia","prompt":"x","think":true}`, think: ThinkOn},
		{name: "native: -think name with explicit think:false keeps false", path: "/api/generate", in: `{"model":"qwen3-ia-think","think":false}`,
			want: `{"model":"qwen3-ia","think":false}`, think: ThinkOff},
		{name: "native: header off beats -think name", path: "/api/generate", in: `{"model":"qwen3-ia-think"}`,
			want: `{"model":"qwen3-ia","think":false}`, think: ThinkOff, hdr: "off"},
		{name: "native: suffix disabled leaves the name", path: "/api/generate", in: `{"model":"qwen3-ia-think"}`,
			want: `{"model":"qwen3-ia-think","think":false}`, think: ThinkOff, mod: func(c *Config) { c.ThinkSuffix = "" }},
		{name: "native: the bare suffix is not a model", path: "/api/generate", in: `{"model":"-think"}`,
			want: `{"model":"-think","think":false}`, think: ThinkOff},

		{name: "v1: injects reasoning_effort none", path: "/v1/chat/completions", in: `{"model":"qwen3-ia","messages":[]}`,
			want: `{"model":"qwen3-ia","messages":[],"reasoning_effort":"none"}`, think: ThinkOff},
		{name: "v1 completions: injects reasoning_effort none", path: "/v1/completions", in: `{"model":"qwen3-ia","prompt":"x"}`,
			want: `{"model":"qwen3-ia","prompt":"x","reasoning_effort":"none"}`, think: ThinkOff},
		{name: "v1: explicit reasoning_effort respected", path: "/v1/chat/completions", in: `{"model":"qwen3-ia","reasoning_effort":"high"}`,
			want: `{"model":"qwen3-ia","reasoning_effort":"high"}`, think: ThinkOn},
		{name: "v1: explicit reasoning_effort none is off", path: "/v1/chat/completions", in: `{"model":"qwen3-ia","reasoning_effort":"none"}`,
			want: `{"model":"qwen3-ia","reasoning_effort":"none"}`, think: ThinkOff},
		{name: "v1: reasoning object respected, nothing injected", path: "/v1/chat/completions", in: `{"model":"qwen3-ia","reasoning":{"effort":"low"}}`,
			want: `{"model":"qwen3-ia","reasoning":{"effort":"low"}}`, think: ThinkOn},
		{name: "v1: think:false in the body does NOT count (the Ollama ignores it)", path: "/v1/chat/completions", in: `{"model":"qwen3-ia","think":false}`,
			want: `{"model":"qwen3-ia","think":false,"reasoning_effort":"none"}`, think: ThinkOff},
		{name: "v1: default-think true leaves the model default", path: "/v1/chat/completions", in: `{"model":"qwen3-ia"}`,
			want: `{"model":"qwen3-ia"}`, think: ThinkModel, mod: func(c *Config) { c.DefaultThink = true }},
		{name: "v1: -think name -> base, no reasoning_effort injected", path: "/v1/chat/completions", in: `{"model":"qwen3-ia-think","messages":[]}`,
			want: `{"model":"qwen3-ia","messages":[]}`, think: ThinkOn},
		{name: "v1: -think name with explicit reasoning_effort respects it", path: "/v1/chat/completions", in: `{"model":"qwen3-ia-think","reasoning_effort":"none"}`,
			want: `{"model":"qwen3-ia","reasoning_effort":"none"}`, think: ThinkOff},
		{name: "v1: header on does not inject", path: "/v1/chat/completions", in: `{"model":"qwen3-ia"}`,
			want: `{"model":"qwen3-ia"}`, think: ThinkOn, hdr: "on"},
		{name: "v1: header off injects none (default-think true)", path: "/v1/chat/completions", in: `{"model":"qwen3-ia"}`,
			want: `{"model":"qwen3-ia","reasoning_effort":"none"}`, think: ThinkOff, hdr: "off", mod: func(c *Config) { c.DefaultThink = true }},
		{name: "v1: explicit body beats header", path: "/v1/chat/completions", in: `{"model":"qwen3-ia","reasoning_effort":"medium"}`,
			want: `{"model":"qwen3-ia","reasoning_effort":"medium"}`, think: ThinkOn, hdr: "off"},

		{name: "embed: no reasoning policy", path: "/api/embed", in: `{"model":"nomic-embed-text","input":"x"}`,
			want: `{"model":"nomic-embed-text","input":"x"}`, think: ""},
		{name: "v1 embeddings: no reasoning policy", path: "/v1/embeddings", in: `{"model":"nomic-embed-text","input":"x"}`,
			want: `{"model":"nomic-embed-text","input":"x"}`, think: ""},
		{name: "embed: a -think name is not rewritten", path: "/api/embed", in: `{"model":"nomic-think","input":"x"}`,
			want: `{"model":"nomic-think","input":"x"}`, think: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cp := &capture{}
			e := setup(t, cp, queue.Config{}, func(cfg *Config) {
				defaultPolicy(cfg)
				if c.mod != nil {
					c.mod(cfg)
				}
			})
			hdr := map[string]string{}
			if c.hdr != "" {
				hdr[HeaderThink] = c.hdr
			}
			resp, body := post(t, e, c.path, c.in, hdr)
			if resp.StatusCode != 200 {
				t.Fatalf("status %d: %s", resp.StatusCode, body)
			}
			sameJSON(t, cp.last(t), c.want)
			rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
			if rec.Think != c.think {
				t.Fatalf("record think = %q, want %q", rec.Think, c.think)
			}
			if cp.hdrs[0].Get(HeaderThink) != "" {
				t.Fatal("the X-Mcpollama-Think header reached the Ollama")
			}
		})
	}
}

func TestPolicyDangerousOptions(t *testing.T) {
	in := `{"model":"qwen3-ia","prompt":"x","keep_alive":0,"options":{"num_ctx":131072,"num_gpu":1,"main_gpu":2,"low_vram":true,"use_mmap":false,"use_mlock":true,"numa":true,"temperature":0.2,"num_predict":64}}`
	for _, path := range []string{"/api/generate", "/api/chat", "/v1/chat/completions", "/v1/completions", "/api/embed", "/api/embeddings", "/v1/embeddings"} {
		t.Run(path, func(t *testing.T) {
			cp := &capture{}
			e := setup(t, cp, queue.Config{}, defaultPolicy)
			if resp, b := post(t, e, path, in, nil); resp.StatusCode != 200 {
				t.Fatalf("%d %s", resp.StatusCode, b)
			}
			m := obj(t, cp.last(t))
			if _, ok := m["keep_alive"]; ok {
				t.Fatal("keep_alive reached the Ollama")
			}
			o := m["options"].(map[string]any)
			for _, k := range []string{"num_ctx", "num_gpu", "main_gpu", "low_vram", "use_mmap", "use_mlock", "numa"} {
				if _, ok := o[k]; ok {
					t.Fatalf("options.%s reached the Ollama: %v", k, o)
				}
			}
			if o["temperature"] != 0.2 || o["num_predict"] != float64(64) {
				t.Fatalf("harmless options lost: %v", o)
			}
		})
	}
}

func TestPolicyAllowFlagsLetThrough(t *testing.T) {
	in := `{"model":"qwen3-ia","prompt":"x","keep_alive":"5m","options":{"num_ctx":8192,"num_gpu":1}}`
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, func(c *Config) {
		defaultPolicy(c)
		c.AllowClientKeepAlive, c.AllowClientNumCtx = true, true
	})
	post(t, e, "/api/generate", in, nil)
	m := obj(t, cp.last(t))
	o := m["options"].(map[string]any)
	if m["keep_alive"] != "5m" || o["num_ctx"] != float64(8192) {
		t.Fatalf("allow flags did not let keep_alive/num_ctx through: %v", m)
	}
	if _, ok := o["num_gpu"]; ok {
		t.Fatal("num_gpu is still in --strip-options and must go")
	}
	// each flag alone
	cp2 := &capture{}
	e2 := setup(t, cp2, queue.Config{}, func(c *Config) { defaultPolicy(c); c.AllowClientKeepAlive = true })
	post(t, e2, "/api/generate", in, nil)
	m = obj(t, cp2.last(t))
	if m["keep_alive"] != "5m" {
		t.Fatalf("keep_alive lost: %v", m)
	}
	if _, ok := m["options"].(map[string]any)["num_ctx"]; ok {
		t.Fatal("num_ctx passed without --allow-client-num-ctx")
	}
}

func TestPolicyStripOptionsListIsConfigurable(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, func(c *Config) { c.StripOptions = []string{"seed"} })
	post(t, e, "/api/generate", `{"model":"m","options":{"seed":1,"num_gpu":2,"num_ctx":9}}`, nil)
	o := obj(t, cp.last(t))["options"].(map[string]any)
	if _, ok := o["seed"]; ok {
		t.Fatal("seed not stripped")
	}
	if o["num_gpu"] != float64(2) {
		t.Fatal("num_gpu stripped though not in the list")
	}
	if _, ok := o["num_ctx"]; ok {
		t.Fatal("num_ctx must go regardless of --strip-options")
	}
}

// Keys are matched like the Go decoder of the Ollama matches them: ignoring
// case (and \u escapes, which json decodes before we see the key).
func TestPolicyKeyCaseAndEscapeTricks(t *testing.T) {
	// Keep_Alive and keep_alive are two spellings of one field and the order is
	// lost once decoded into a map: refused, never guessed.
	in := `{"model":"qwen3-ia","Keep_Alive":0,"keep_alive":0}`
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, defaultPolicy)
	if resp, b := post(t, e, "/api/generate", in, nil); resp.StatusCode != 400 {
		t.Fatalf("two spellings of keep_alive: status %d %s", resp.StatusCode, b)
	}
	if cp.calls.Load() != 0 {
		t.Fatal("reached the Ollama")
	}
	for _, in := range []string{
		`{"model":"qwen3-ia","Keep_Alive":0,"OPTIONS":{"Num_Ctx":999999,"NUM_GPU":3}}`,
		`{"model":"qwen3-ia","KEEP_ALIVE":"-1","Options":{"num\u005fctx":5,"num_ctx":6}}`,
		"{\"model\":\"qwen3-ia\",\"\u212aeep_alive\":0}", // Kelvin sign folds to k in Go's json
	} {
		cp := &capture{}
		e := setup(t, cp, queue.Config{}, defaultPolicy)
		if resp, b := post(t, e, "/api/chat", in, nil); resp.StatusCode != 200 {
			t.Fatalf("%q: %d %s", in, resp.StatusCode, b)
		}
		got := cp.last(t)
		var anyKeep bool
		var walk func(m map[string]any)
		walk = func(m map[string]any) {
			for k, v := range m {
				if strings.EqualFold(k, "keep_alive") || strings.EqualFold(k, "num_ctx") || strings.EqualFold(k, "num_gpu") {
					anyKeep = true
				}
				if s, ok := v.(map[string]any); ok {
					walk(s)
				}
			}
		}
		walk(obj(t, got))
		if anyKeep {
			t.Fatalf("%q -> %s still has a dangerous key", in, got)
		}
	}
}

func TestPolicyCaseVariantsOfPolicyKeysAreRefused(t *testing.T) {
	for _, in := range []string{
		`{"model":"a","Model":"b"}`,
		`{"model":"a","think":true,"THINK":false}`,
		`{"model":"a","options":{},"Options":{}}`,
	} {
		cp := &capture{}
		e := setup(t, cp, queue.Config{}, defaultPolicy)
		if resp, _ := post(t, e, "/api/generate", in, nil); resp.StatusCode != 400 {
			t.Fatalf("%s: status %d", in, resp.StatusCode)
		}
		if cp.calls.Load() != 0 {
			t.Fatalf("%s reached the Ollama", in)
		}
	}
}

func TestPolicyKeyCaseOfExplicitThinkIsKept(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, defaultPolicy)
	post(t, e, "/api/generate", `{"model":"m","Think":true}`, nil)
	if got := cp.last(t); strings.Contains(got, `"think"`) || !strings.Contains(got, `"Think":true`) {
		t.Fatalf("an explicit Think must be respected, not duplicated: %s", got)
	}
}

func TestPolicyAllowlistChecksTheBaseName(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, func(c *Config) {
		defaultPolicy(c)
		c.AllowedModels = []string{"qwen3-ia", "nomic-embed-text"}
	})
	// the virtual name passes because its base is allowed
	if resp, b := post(t, e, "/api/generate", `{"model":"qwen3-ia-think","prompt":"x"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("base allowed: %d %s", resp.StatusCode, b)
	}
	if !strings.Contains(cp.last(t), `"model":"qwen3-ia"`) {
		t.Fatalf("not rewritten: %s", cp.last(t))
	}
	n := cp.calls.Load()
	// foo-think with foo outside the list: refused
	resp, b := post(t, e, "/api/generate", `{"model":"foo-think","prompt":"x"}`, nil)
	if resp.StatusCode != 403 || !strings.Contains(b, "foo") {
		t.Fatalf("foo-think: %d %s", resp.StatusCode, b)
	}
	// only one suffix is removed: "qwen3-ia-think-think" has base qwen3-ia-think
	if resp, _ := post(t, e, "/api/generate", `{"model":"qwen3-ia-think-think"}`, nil); resp.StatusCode != 403 {
		t.Fatalf("double suffix: %d", resp.StatusCode)
	}
	if cp.calls.Load() != n {
		t.Fatal("a refused request reached the Ollama")
	}
}

func TestPolicyInvalidJSONIs400WithoutCallingTheOllama(t *testing.T) {
	const secret = "SEGREDO-NO-PROMPT-91c2"
	for _, path := range []string{"/api/generate", "/api/chat", "/api/embed", "/v1/chat/completions", "/v1/completions", "/v1/embeddings"} {
		for _, in := range []string{
			`not json`, ``, `null`, `[1,2]`, `"` + secret + `"`, `{"model":"a","prompt":"` + secret + `"`, // truncated
			`{"model":"a"}` + secret, `{"model":7}`, `{"model":["a"]}`,
		} {
			cp := &capture{}
			var logs []string
			e := setup(t, cp, queue.Config{}, func(c *Config) {
				defaultPolicy(c)
				c.Logf = func(f string, a ...any) { logs = append(logs, f) }
			})
			resp, b := post(t, e, path, in, nil)
			if resp.StatusCode != 400 {
				t.Fatalf("%s %q: status %d %s", path, in, resp.StatusCode, b)
			}
			if strings.Contains(b, secret) || strings.Contains(strings.Join(logs, "\n"), secret) {
				t.Fatalf("%s: prompt text leaked into the error or the log: %s", path, b)
			}
			if cp.calls.Load() != 0 {
				t.Fatalf("%s %q reached the Ollama", path, in)
			}
			if s := e.q.Snapshot(); len(s.Waiting)+len(s.Running) != 0 {
				t.Fatal("a rejected body took a place in the line")
			}
		}
	}
}

func TestPolicyContentLengthIsRecalculatedAndUntouchedBodyIsByteExact(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, defaultPolicy)
	post(t, e, "/api/generate", `{"model":"qwen3-ia","prompt":"x","keep_alive":0,"options":{"num_ctx":1}}`, nil)
	if got := cp.last(t); cp.lens[0] != int64(len(got)) {
		t.Fatalf("Content-Length %d, body %d bytes", cp.lens[0], len(got))
	}
	// Nothing to change (explicit think, no dangerous key): the very same bytes,
	// including the odd spacing and the HTML characters of the prompt.
	in := "{ \"model\" : \"qwen3-ia\",\n \"prompt\":\"<b>&</b>  \\u00e9\", \"think\" : false }"
	post(t, e, "/api/generate", in, nil)
	if got := cp.last(t); got != in {
		t.Fatalf("untouched body changed:\n got %q\nwant %q", got, in)
	}
	// Rewritten: the values keep their bytes (no HTML escaping, no number reformat).
	post(t, e, "/api/generate", `{"model":"qwen3-ia","prompt":"<b>&</b> \u00e9","temperature":1.50,"keep_alive":0}`, nil)
	got := cp.last(t)
	if !strings.Contains(got, `"prompt":"<b>&</b> \u00e9"`) || !strings.Contains(got, `"temperature":1.50`) {
		t.Fatalf("values changed: %s", got)
	}
}

func TestPolicyBodyLimitBehaviourUnchanged(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, func(c *Config) { defaultPolicy(c); c.MaxBodyBytes = 64 })
	big := `{"model":"m","prompt":"` + strings.Repeat("a", 200) + `"}`
	if resp, _ := post(t, e, "/api/generate", big, nil); resp.StatusCode != 413 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if cp.calls.Load() != 0 {
		t.Fatal("reached the Ollama")
	}
}

func TestPolicyBytesInIsTheOriginalSize(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, defaultPolicy)
	in := `{"model":"qwen3-ia","prompt":"x","keep_alive":0}`
	resp, _ := post(t, e, "/api/generate", in, nil)
	if rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID)); rec.BytesIn != int64(len(in)) || rec.Model != "qwen3-ia" {
		t.Fatalf("record %+v", rec)
	}
}

func TestPolicyRecordModelIsTheBaseName(t *testing.T) {
	cp := &capture{}
	e := setup(t, cp, queue.Config{}, defaultPolicy)
	resp, _ := post(t, e, "/api/generate", `{"model":"qwen3-ia-think"}`, nil)
	if rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID)); rec.Model != "qwen3-ia" || rec.Think != ThinkOn {
		t.Fatalf("record %+v", rec)
	}
}

// FuzzPolicyRewrite (Fase 9.6a): any body, any route, any header, any option
// set: never panics; when accepted, the result is a valid JSON object with no
// keep_alive, no options.num_ctx/num_gpu (case-insensitive, like Go's decoder)
// and the reasoning defined; the model is the base name.
func FuzzPolicyRewrite(f *testing.F) {
	seeds := []string{
		`{"model":"qwen3-ia","prompt":"x"}`, `{"model":"qwen3-ia-think","keep_alive":0}`,
		`{"model":"m","options":{"num_ctx":1,"num_gpu":2,"Num_Ctx":3,"temperature":0}}`,
		`{"model":"m","think":null,"reasoning_effort":null,"reasoning":null}`, `{"model":"m","Think":true}`,
		`{"model":"m","KEEP_ALIVE":"5m","OPTIONS":{"NUM_GPU":1}}`, `{"model":"m","options":null}`, `{"model":"m","options":[1]}`,
		"{\"\u212aeep_alive\":1,\"model\":\"m\"}", `{"model":"a","Model":"b"}`, `null`, `[]`, ``, `{}`,
		`{"model":"m","reasoning":{"effort":"none"}}`, `{"model":"m","reasoning_effort":"high"}`, `{"model":"-think"}`,
	}
	for i, s := range seeds {
		f.Add(s, uint8(i), uint8(i), uint8(i))
	}
	paths := []string{"/api/generate", "/api/chat", "/v1/chat/completions", "/v1/completions", "/api/embed", "/api/embeddings", "/v1/embeddings"}
	hdrs := []string{"", "on", "off", "ON", "x", " off "}
	f.Fuzz(func(t *testing.T, body string, pi, hi, flags uint8) {
		cfg := Config{DefaultThink: flags&1 != 0, StripOptions: DefaultStripOptions}
		if flags&2 != 0 {
			cfg.ThinkSuffix = "-think"
		}
		p := newPolicyCfg(cfg)
		path := paths[int(pi)%len(paths)]
		res, perr := p.applyPolicy(path, []byte(body), hdrs[int(hi)%len(hdrs)])
		if perr != nil {
			if perr.status != 400 || (len(body) > 3 && strings.Contains(perr.msg, body)) {
				t.Fatalf("bad error %+v", perr)
			}
			return
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(res.body, &m); err != nil || m == nil {
			t.Fatalf("accepted but not a JSON object: %v: %q -> %q", err, body, res.body)
		}
		for k, v := range m {
			if strings.EqualFold(k, "keep_alive") {
				t.Fatalf("keep_alive left: %q -> %q", body, res.body)
			}
			if strings.EqualFold(k, "options") {
				var o map[string]json.RawMessage
				if json.Unmarshal(v, &o) == nil {
					for ok := range o {
						for _, bad := range []string{"num_ctx", "num_gpu", "main_gpu", "low_vram", "use_mmap", "use_mlock", "numa"} {
							if strings.EqualFold(ok, bad) {
								t.Fatalf("options.%s left: %q -> %q", ok, body, res.body)
							}
						}
					}
				}
			}
		}
		kind := routeKinds[path]
		has := func(name string) bool {
			for k, v := range m {
				if strings.EqualFold(k, name) && !isNull(v) {
					return true
				}
			}
			return false
		}
		switch kind {
		case kindNative:
			if !has("think") || (res.think != ThinkOn && res.think != ThinkOff) {
				t.Fatalf("native without think: %q -> %q (%s)", body, res.body, res.think)
			}
		case kindV1:
			if res.think == "" {
				t.Fatalf("v1 without a reasoning mode: %q", body)
			}
			if res.think == ThinkOff && !has("reasoning_effort") && !has("reasoning") {
				t.Fatalf("v1 off without reasoning_effort: %q -> %q", body, res.body)
			}
		case kindEmbed:
			if res.think != "" {
				t.Fatalf("embed got a reasoning mode: %q", res.think)
			}
		}
		// the model written in the body is the base name the allowlist sees
		if has("model") {
			var name string
			for k, v := range m {
				if strings.EqualFold(k, "model") {
					_ = json.Unmarshal(v, &name)
				}
			}
			if name != res.model {
				t.Fatalf("model in the body %q, base name %q", name, res.model)
			}
		}
		// a second pass over the result without the suffix rule decides nothing new
		if cfg.ThinkSuffix == "" && kind != kindV1 {
			again, perr := p.applyPolicy(path, res.body, "")
			if perr != nil || string(again.body) != string(res.body) {
				t.Fatalf("not idempotent: %q -> %q -> %q (%v)", body, res.body, again.body, perr)
			}
		}
	})
}
