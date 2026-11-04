package proxy

// Fase 12, etapa 12.8: the improvement without training in the proxy (D12-15, C-8.8):
// off by default, per tool, the log keeps the original request, the search is made
// before the slot of generation and never delays or fails the request.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// fakeOllama answers /api/generate (recording the bodies; a body with BLOQUEIA waits on
// release) and /api/embed (deterministic vector, with an optional delay or status).
type fakeOllama struct {
	mu          sync.Mutex
	gen         []string
	embedBodies []string
	embeds      atomic.Int64
	embedDelay  time.Duration
	embedStatus int
	release     chan struct{}
	started     chan struct{} // signalled when a BLOQUEIA request reaches the Ollama
	once        sync.Once
}

func newFakeOllama() *fakeOllama {
	return &fakeOllama{release: make(chan struct{}), started: make(chan struct{}, 8)}
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	if r.URL.Path == "/api/embed" {
		f.embeds.Add(1)
		f.mu.Lock()
		f.embedBodies = append(f.embedBodies, string(b))
		f.mu.Unlock()
		if f.embedDelay > 0 {
			select {
			case <-time.After(f.embedDelay):
			case <-r.Context().Done():
				return
			}
		}
		if f.embedStatus != 0 {
			w.WriteHeader(f.embedStatus)
			return
		}
		io.WriteString(w, `{"model":"nomic-embed-text","embeddings":[[1,0,0]]}`)
		return
	}
	f.mu.Lock()
	f.gen = append(f.gen, string(b))
	f.mu.Unlock()
	if strings.Contains(string(b), "BLOQUEIA") {
		f.started <- struct{}{}
		<-f.release
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	io.WriteString(w, finalChunk+"\n")
}

func (f *fakeOllama) unblock() { f.once.Do(func() { close(f.release) }) }

func (f *fakeOllama) bodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.gen...)
}

func (f *fakeOllama) lastGen(t *testing.T) string {
	t.Helper()
	b := f.bodies()
	if len(b) == 0 {
		t.Fatal("the Ollama got no generation")
	}
	return b[len(b)-1]
}

func exampleIndex() *melhora.Index {
	return melhora.NewIndex("idx-1", "hash-1", "nomic-embed-text", 2, 0.5, []melhora.Item{
		{OcorrenciaID: "a", Tool: "extract", Vec: []float32{1, 0, 0}, Example: melhora.Example{Pedido: "PED-A", Errada: "ERR-A", Certa: "CERTA-A"}},
	})
}

// withMelhora wires a Transformer whose embedding goes through the real queue and the fake Ollama.
func withMelhora(tools []string, instr string, timeout time.Duration, ix *melhora.Index) (func(*Config), func() *melhora.Transformer) {
	var tr *melhora.Transformer
	return func(c *Config) {
		defaultPolicy(c)
		var in *melhora.Instrucoes
		if instr != "" {
			in, _ = melhora.ParseInstrucoes([]byte(`{"tools":{"extract":` + jsonStr(instr) + `}}`))
		}
		tr = melhora.NewTransformer(melhora.Config{Tools: tools, Instr: in, EmbedTimeout: timeout,
			Embed: NewEmbedder(EmbedderConfig{Upstream: c.Upstream, Queue: c.Queue})})
		if ix != nil {
			tr.SetIndex(ix)
		}
		c.Melhora = tr
	}, func() *melhora.Transformer { return tr }
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

const fileReq = `{"model":"qwen3-ia","system":"Regras fixas.","prompt":"Extraia.\n\n=====ARQUIVO_INICIO 0123456789abcdef=====\ndoc <x> & y\n=====ARQUIVO_FIM 0123456789abcdef=====\n"}`

func sysOf(t *testing.T, body string) string {
	t.Helper()
	var m struct{ System string }
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return m.System
}

func TestMelhoraOffByDefaultTheBodyIsUntouched(t *testing.T) { // C-8.8
	base := newFakeOllama()
	e0 := setup(t, base, queue.Config{}, defaultPolicy)
	post(t, e0, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	want := base.lastGen(t)

	for name, mod := range map[string]func(*Config){
		"no transformer":          func(c *Config) { defaultPolicy(c) },
		"transformer, no tool on": func(c *Config) { defaultPolicy(c); c.Melhora = melhora.NewTransformer(melhora.Config{}) },
		"tool on, nothing to add": func(c *Config) {
			defaultPolicy(c)
			c.Melhora = melhora.NewTransformer(melhora.Config{Tools: melhora.Tools})
		},
		"instructions, tool off":     nil,
		"instructions, another tool": nil,
	} {
		if mod == nil {
			in, _ := melhora.ParseInstrucoes([]byte(`{"tools":{"extract":"SEMPRE"}}`))
			tools := []string(nil)
			if name == "instructions, another tool" {
				tools = []string{"translate"}
			}
			mod = func(c *Config) {
				defaultPolicy(c)
				c.Melhora = melhora.NewTransformer(melhora.Config{Tools: tools, Instr: in})
			}
		}
		up := newFakeOllama()
		e := setup(t, up, queue.Config{}, mod)
		post(t, e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
		if got := up.lastGen(t); got != want {
			t.Errorf("%s: the body changed:\n%s\n%s", name, got, want)
		}
	}
}

func TestMelhoraAddsTheInstructionOnlyForTheSwitchedOnTool(t *testing.T) {
	up := newFakeOllama()
	mod, trf := withMelhora([]string{"extract"}, "USE A NUMERAÇÃO DO CORPO", time.Second, nil)
	e := setup(t, up, queue.Config{}, mod)
	post(t, e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	got := up.lastGen(t)
	if s := sysOf(t, got); s != "Regras fixas.\n\nUSE A NUMERAÇÃO DO CORPO" {
		t.Errorf("system = %q", s)
	}
	// the user text and the markers arrive byte for byte
	if !strings.Contains(got, `=====ARQUIVO_INICIO 0123456789abcdef=====\ndoc <x> & y\n=====ARQUIVO_FIM 0123456789abcdef=====`) {
		t.Errorf("the prompt changed: %s", got)
	}
	// Content-Length follows the new body
	cap := &capture{}
	e2 := setup(t, cap, queue.Config{}, mod)
	post(t, e2, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	if cap.lens[0] != int64(len(cap.bodies[0])) {
		t.Errorf("Content-Length %d, body %d", cap.lens[0], len(cap.bodies[0]))
	}
	n := len(up.bodies())
	for _, hdr := range []map[string]string{{HeaderTool: "generate"}, {HeaderTool: "bakeoff-erros"}, {}, {HeaderTool: "Extract"}} {
		post(t, e, "/api/generate", fileReq, hdr)
		if s := sysOf(t, up.lastGen(t)); s != "Regras fixas." {
			t.Errorf("%v: system = %q", hdr, s)
		}
	}
	if len(up.bodies()) != n+4 {
		t.Fatal("setup")
	}
	if st := trf().Stats(); st.Alterados != 1 || st.Pedidos != 1 {
		t.Errorf("stats: %+v", st)
	}
}

func TestMelhoraDoesNotTouchEmbeddingsOrOtherRoutes(t *testing.T) {
	up := newFakeOllama()
	mod, trf := withMelhora(melhora.Tools, "INSTR", time.Second, nil)
	e := setup(t, up, queue.Config{}, mod)
	body := `{"model":"nomic-embed-text","input":"texto"}`
	post(t, e, "/api/embed", body, map[string]string{HeaderTool: "extract"})
	up.mu.Lock()
	got := up.embedBodies
	up.mu.Unlock()
	if len(got) != 1 || got[0] != body {
		t.Errorf("an embedding request was changed: %v", got)
	}
	if trf().Stats().Pedidos != 0 {
		t.Error("the improvement looked at an embedding request")
	}
}

func TestMelhoraJobLogKeepsBothRequestsAndTheTrace(t *testing.T) { // C-2.10, C-2.10a
	baseUp := newFakeOllama()
	fj0 := &fakeJournal{wants: true}
	k := testKey(t)
	e0 := setup(t, baseUp, queue.Config{}, func(c *Config) { defaultPolicy(c); c.Journal = fj0; c.DocKey = k })
	post(t, e0, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	e0.waitIdle(t)
	plain := baseUp.lastGen(t) // what the Ollama gets with the improvement off = the request after the policy

	fj := &fakeJournal{wants: true}
	up := newFakeOllama()
	mod, _ := withMelhora([]string{"extract"}, "INSTRUCAO-NOVA", time.Second, exampleIndex())
	e := setup(t, up, queue.Config{}, func(c *Config) { mod(c); c.Journal = fj; c.DocKey = k })
	post(t, e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	e.waitIdle(t)
	sent := up.lastGen(t)
	if !strings.Contains(sent, "INSTRUCAO-NOVA") || !strings.Contains(sent, "CERTA-A") {
		t.Fatalf("setup: the Ollama did not get the improvement: %s", sent)
	}
	fj.mu.Lock()
	defer fj.mu.Unlock()
	if len(fj.withPJ) != 1 {
		t.Fatalf("journal got %d requests", len(fj.withPJ))
	}
	pj := fj.withPJ[0]
	// C-2.10 stays whole: prompt_json IS the body that reached the Ollama
	if string(pj.JSON) != sent {
		t.Errorf("prompt_json is not what the Ollama received:\n%s\n%s", pj.JSON, sent)
	}
	// C-2.10a: the request before the improvement is kept apart, and the trace says what was added
	if string(pj.Original) != plain || strings.Contains(string(pj.Original), "INSTRUCAO-NOVA") || strings.Contains(string(pj.Original), "CERTA-A") {
		t.Errorf("prompt_json_original is not the request before the improvement:\n%s", pj.Original)
	}
	if !pj.Melhora || pj.IndiceID != "idx-1" || pj.InstrucoesID == "" || len(pj.Exemplos) != 1 || pj.Exemplos[0] != "a" {
		t.Errorf("trace: %+v", pj)
	}
}

func TestMelhoraLeavesNoTraceWhenItChangesNothing(t *testing.T) {
	fj := &fakeJournal{wants: true}
	k := testKey(t)
	mod, _ := withMelhora([]string{"extract"}, "", time.Second, nil) // on, but nothing to add
	e := setup(t, newFakeOllama(), queue.Config{}, func(c *Config) { mod(c); c.Journal = fj; c.DocKey = k })
	post(t, e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	e.waitIdle(t)
	fj.mu.Lock()
	defer fj.mu.Unlock()
	if len(fj.withPJ) != 1 || fj.withPJ[0].Melhora || fj.withPJ[0].Original != nil || fj.withPJ[0].InstrucoesID != "" || fj.withPJ[0].IndiceID != "" || len(fj.withPJ[0].Exemplos) != 0 {
		t.Errorf("trace without an improvement: %+v", fj.withPJ)
	}
}

func TestMelhoraSearchUsesTheEmbedLaneAndNeverDeadlocksWithTheGenerationSlot(t *testing.T) {
	up := newFakeOllama()
	mod, trf := withMelhora([]string{"extract"}, "", 5*time.Second, exampleIndex())
	e := setup(t, up, queue.Config{GenSlots: 1, EmbedSlots: 1}, mod)
	t.Cleanup(up.unblock) // a failing test must not leave request 1 blocked (the server would never close)

	// request 1 takes the only generation slot and holds it
	done1 := make(chan struct{})
	go func() {
		postBg(e, "/api/generate", `{"model":"qwen3-ia","prompt":"BLOQUEIA"}`, map[string]string{HeaderTool: "generate"})
		close(done1)
	}()
	select {
	case <-up.started:
	case <-time.After(3 * time.Second):
		t.Fatal("request 1 never reached the Ollama")
	}
	// request 2 (with a search) arrives while the slot is taken
	done2 := make(chan string, 1)
	go func() {
		_, _ = postBg(e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
		done2 <- "ok"
	}()
	// its embedding is served NOW, by the embed lane, while generation is still held
	deadline := time.Now().Add(3 * time.Second)
	for up.embeds.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if up.embeds.Load() != 1 {
		t.Fatalf("the search embedding did not run while the generation slot was busy (%d)", up.embeds.Load())
	}
	// ... and the request waits for its generation turn only after the search is done
	deadline = time.Now().Add(3 * time.Second)
	for len(e.q.Snapshot().Waiting) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	snap := e.q.Snapshot()
	if len(snap.Waiting) != 1 || snap.Waiting[0].Lane != queue.LaneGen || snap.InUse[queue.LaneGen] != 1 {
		t.Fatalf("snapshot: waiting %+v in use %+v", snap.Waiting, snap.InUse)
	}
	up.unblock()
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("request 2 never finished")
	}
	<-done1
	e.waitIdle(t)
	if !strings.Contains(up.lastGen(t), "CERTA-A") {
		t.Errorf("request 2 did not get its example: %s", up.lastGen(t))
	}
	// the embedding went through the queue like any other, tagged as the gateway's own
	var emb *queue.Record
	for _, r := range e.records() {
		if r.Lane == queue.LaneEmbed {
			r := r
			emb = &r
		}
	}
	if emb == nil || emb.Tool != EmbedTool || emb.ClientLabel != EmbedLabel || emb.Status != queue.StatusOK || emb.Endpoint != "/api/embed" {
		t.Errorf("embed record: %+v", emb)
	}
	if st := trf().Stats(); st.Exemplos != 1 || st.EmbedFalhas != 0 {
		t.Errorf("stats %+v", st)
	}
}

func TestMelhoraSlowEmbeddingSendsTheRequestWithoutExamplesInTime(t *testing.T) {
	up := newFakeOllama()
	up.embedDelay = 3 * time.Second
	mod, trf := withMelhora([]string{"extract"}, "INSTR", 200*time.Millisecond, exampleIndex())
	e := setup(t, up, queue.Config{}, mod)
	t0 := time.Now()
	resp, _ := post(t, e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	if d := time.Since(t0); d > 2*time.Second || resp.StatusCode != 200 {
		t.Fatalf("status %d after %s with a 200 ms limit", resp.StatusCode, d)
	}
	got := up.lastGen(t)
	if strings.Contains(got, "CERTA-A") || !strings.Contains(sysOf(t, got), "INSTR") {
		t.Errorf("a slow search must go on without examples (and with the instruction): %s", got)
	}
	if st := trf().Stats(); st.EmbedFalhas != 1 || st.EmbedLentos != 1 || st.Exemplos != 0 {
		t.Errorf("stats %+v", st)
	}
	e.waitIdle(t)
}

func TestMelhoraEmbeddingErrorSendsTheRequestWithoutExamples(t *testing.T) {
	up := newFakeOllama()
	up.embedStatus = 500
	mod, trf := withMelhora([]string{"extract"}, "", time.Second, exampleIndex())
	e := setup(t, up, queue.Config{}, mod)
	resp, _ := post(t, e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if strings.Contains(up.lastGen(t), "CERTA-A") {
		t.Error("examples were added after a failed search")
	}
	if st := trf().Stats(); st.EmbedFalhas != 1 || st.EmbedLentos != 0 {
		t.Errorf("stats %+v", st)
	}
	e.waitIdle(t)
}

func TestMelhoraConcurrentSlowSearchesAllGoOnWithoutExamples(t *testing.T) {
	// with the only embed slot busy, a search that waits past its limit gives up
	// and the request goes on; the generation slot is never involved
	up := newFakeOllama()
	up.embedDelay = 600 * time.Millisecond
	mod, trf := withMelhora([]string{"extract"}, "", 150*time.Millisecond, exampleIndex())
	e := setup(t, up, queue.Config{GenSlots: 1, EmbedSlots: 1}, mod)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			postBg(e, "/api/generate", fileReq, map[string]string{HeaderTool: "extract"})
		}()
	}
	wg.Wait()
	e.waitIdle(t)
	if len(up.bodies()) != 3 {
		t.Fatalf("%d generations reached the Ollama, want 3", len(up.bodies()))
	}
	if st := trf().Stats(); st.EmbedFalhas < 2 || st.Pedidos != 3 {
		t.Errorf("stats %+v", st)
	}
}
