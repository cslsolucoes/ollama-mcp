package main

// Phase 8.9 (plan MCPOllam2026-10-28-193200-fase8-bakeoff-modelos): real
// simultaneous load. N agents = N separate mcpollama processes (each Claude
// Code session has its own), each started with a random jitter and running K
// tasks. The first connection error aborts the whole step (degrau): every
// mcpollama process is killed and no new call is sent. Application errors
// (HTTP 429/5xx, timeout) are recorded and do not abort.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Error classes of one call under load.
const (
	clsOK      = "ok"
	clsHTTP429 = "http429"
	clsHTTP5xx = "http5xx"
	clsTimeout = "timeout"
	clsConn    = "conexao"   // aborts the step
	clsProto   = "protocolo" // the mcpollama process died or answered garbage: aborts the step
	clsOther   = "outro"
	clsAborted = "cancelado-abort" // in flight when the step was aborted
)

// classify maps a call outcome to an error class. The texts come from the
// mcpollama error map (src/ollama/errors.go) and from the driver (rpc.go).
func classify(r callRes) string {
	if !r.IsError && !r.Proto {
		return clsOK
	}
	e := r.Err
	switch {
	case r.Proto && strings.HasPrefix(e, "timeout_driver"):
		return clsTimeout
	case r.Proto:
		return clsProto
	case strings.Contains(e, "[HTTP 429"):
		return clsHTTP429
	case strings.Contains(e, "[HTTP 5"):
		return clsHTTP5xx
	case strings.Contains(e, "tempo esgotado"):
		return clsTimeout
	case strings.Contains(e, "encerrou a conexão"), strings.Contains(e, "não foi possível falar com"),
		strings.Contains(e, "certificado TLS"):
		return clsConn
	}
	return clsOther
}

// abortsStep says whether a class stops the whole step (owner's rule).
func abortsStep(cls string) bool { return cls == clsConn || cls == clsProto }

// task is one unit of work of an agent.
type task struct {
	suite, label, tool string
	args               map[string]any
}

// multiTasks builds the K tasks of agent i: S2 of the S2 document and S3 JSON
// of the S3 documents, alternating (agent i starts with S2 when i is even), so
// that at any moment the load mixes the two kinds. With big documents, the
// agent runs one summarize_file of big[i % len(big)] ("prompt grande").
func multiTasks(i, k int, corpus, s2Doc string, s3Docs, big []string) []task {
	doc := func(id string) string { return filepath.Join(corpus, id) }
	if len(big) > 0 {
		d := big[i%len(big)]
		return []task{{"grande", d, "summarize_file", map[string]any{"path": doc(d), "max_words": 150}}}
	}
	var out []task
	for j := 0; j < k; j++ {
		if (i+j)%2 == 0 {
			out = append(out, task{"s2", s2Doc, "extract",
				map[string]any{"path": doc(s2Doc), "instruction": s2Instruction, "format": s2Format}})
			continue
		}
		d := s3Docs[(i+j)%len(s3Docs)]
		out = append(out, task{"s3", d + "-json", "extract",
			map[string]any{"path": doc(d), "instruction": s3Instruction, "format": s3Formats[0].format}})
	}
	return out
}

// multiRes is one call of the load, with its quality score.
type multiRes struct {
	callRes
	Agent    int    `json:"agente"`
	Task     int    `json:"tarefa"`
	Class    string `json:"classe"`
	StartOff int64  `json:"inicio_ms"` // since the step started
	EndOff   int64  `json:"fim_ms"`
	S2NT     int    `json:"s2_numero_titulo_ok,omitempty"`
	S2Total  int    `json:"s2_total,omitempty"`
	S2Inv    int    `json:"s2_inventados,omitempty"`
	JSONOK   bool   `json:"json_ok"`
	Strict   bool   `json:"estrito"`
	Words    int    `json:"palavras,omitempty"`
}

// multiSummary aggregates one step.
type multiSummary struct {
	Model, Step                   string
	Agents, Calls, OK             int
	Classes                       map[string]int
	WallS                         float64
	PromptTok, EvalTok            int64
	TokS, PrefillTokS, GenTokS    float64
	P50S, P95S, MaxS              float64
	S2Runs, S2NT, S2Total, S2JSON int
	S3Runs, S3OK, S3Strict        int
	BigRuns, BigOK                int
	BigPromptMin, BigPromptMax    int64
	Aborted                       bool
	AbortWhy                      string
	FirstStart, LastEnd           string
}

// percentile returns the nearest-rank percentile p (0..1) of v.
func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	k := int(float64(len(s))*p+0.9999999) - 1
	if k < 0 {
		k = 0
	}
	if k >= len(s) {
		k = len(s) - 1
	}
	return s[k]
}

// summarize computes the step metrics. Throughput uses the step window: from
// the first call start to the last call end. Latency percentiles use the
// calls that finished without error.
func summarize(model, step string, agents int, res []multiRes, t0 time.Time) multiSummary {
	s := multiSummary{Model: model, Step: step, Agents: agents, Calls: len(res), Classes: map[string]int{}, BigPromptMin: -1}
	var lat []float64
	first, last := int64(-1), int64(0)
	for _, r := range res {
		s.Classes[r.Class]++
		if r.Class == clsAborted {
			continue
		}
		if first < 0 || r.StartOff < first {
			first = r.StartOff
		}
		if r.EndOff > last {
			last = r.EndOff
		}
		if r.Class != clsOK {
			continue
		}
		s.OK++
		lat = append(lat, float64(r.WallMS)/1000)
		s.PromptTok += r.PromptE
		s.EvalTok += r.EvalE
		switch r.Suite {
		case "s2":
			s.S2Runs++
			s.S2NT += r.S2NT
			s.S2Total += r.S2Total
			if r.JSONOK {
				s.S2JSON++
			}
		case "s3":
			s.S3Runs++
			if r.JSONOK {
				s.S3OK++
			}
			if r.Strict {
				s.S3Strict++
			}
		case "grande":
			s.BigRuns++
			if r.JSONOK {
				s.BigOK++
			}
			if s.BigPromptMin < 0 || r.PromptE < s.BigPromptMin {
				s.BigPromptMin = r.PromptE
			}
			if r.PromptE > s.BigPromptMax {
				s.BigPromptMax = r.PromptE
			}
		}
	}
	if first >= 0 && last > first {
		s.WallS = float64(last-first) / 1000
		s.TokS = float64(s.PromptTok+s.EvalTok) / s.WallS
		s.PrefillTokS = float64(s.PromptTok) / s.WallS
		s.GenTokS = float64(s.EvalTok) / s.WallS
		s.FirstStart = t0.Add(time.Duration(first) * time.Millisecond).Format("15:04:05")
		s.LastEnd = t0.Add(time.Duration(last) * time.Millisecond).Format("15:04:05")
	}
	s.P50S, s.P95S = percentile(lat, 0.5), percentile(lat, 0.95)
	if len(lat) > 0 {
		s.MaxS = percentile(lat, 1)
	}
	if s.BigPromptMin < 0 {
		s.BigPromptMin = 0
	}
	return s
}

// scoreMulti fills the quality fields of one finished call.
func scoreMulti(r *multiRes, gab []gabItem) {
	if r.Class != clsOK {
		return
	}
	body := bodyOf(r.text)
	switch r.Suite {
	case "s2":
		sc := scoreS2(body, gab)
		r.S2NT, r.S2Total, r.S2Inv, r.JSONOK, r.Strict = sc.NumTitleOK, sc.Total, len(sc.Invented), sc.JSONValid, sc.Strict
	case "s3":
		sc := checkJSONStrings(body)
		r.JSONOK, r.Strict = sc.OK, sc.Strict
	case "grande":
		r.Words = len(strings.Fields(body))
		r.JSONOK = r.Words > 0 && r.Fim != "length" // a non-empty summary that was not cut
	}
}

// runMulti runs one step: N agents x K tasks against the model under test.
func runMulti(o options, srvArgs []string, logf func(string, ...any)) int {
	if o.agents <= 0 || o.tasks <= 0 {
		logf("FATAL multi: -agents e -tasks devem ser > 0")
		return 2
	}
	gab, err := readGab(o.gab)
	if err != nil {
		logf("FATAL gabarito: %v", err)
		return 2
	}
	step := o.step
	if step == "" {
		step = fmt.Sprintf("%dagentes", o.agents)
	}
	outDir := filepath.Join(o.out, "multi", safeName(o.model), step)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		logf("FATAL out: %v", err)
		return 2
	}
	lf, err := os.OpenFile(filepath.Join(outDir, "calls.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		logf("FATAL log: %v", err)
		return 2
	}
	defer lf.Close()
	boot := ""
	if o.uptimeHost != "" {
		b, err := serverBoot(o.uptimeHost)
		if err != nil {
			logf("ABORT uptime inicial: %v", err)
			return 4
		}
		if o.expectBoot != "" && b != o.expectBoot {
			logf("ABORT boot diferente do esperado: %s != %s", b, o.expectBoot)
			return 4
		}
		boot = b
		logf("UPTIME boot=%s aer_1b4_cor=%s", boot, lastAER)
	}
	big := splitList(o.bigDocs)
	clients := make([]*client, o.agents)
	closeAll := func() {
		for _, c := range clients {
			if c != nil {
				c.close()
			}
		}
	}
	for i := range clients {
		i := i
		alog := func(f string, a ...any) { logf(fmt.Sprintf("A%02d ", i+1)+f, a...) }
		c, err := startClient(o.exe, srvArgs, alog)
		if err == nil {
			c.think = o.think
		}
		if err != nil {
			logf("FATAL start agente %d: %v", i+1, err)
			closeAll()
			return 2
		}
		clients[i] = c
		if err := c.initialize(); err != nil {
			logf("FATAL initialize agente %d: %v", i+1, err)
			closeAll()
			return 2
		}
	}
	logf("MULTI-INICIO modelo=%s degrau=%s agentes=%d tarefas=%d jitter=%s grande=%v inicio=%s",
		o.model, step, o.agents, o.tasks, o.jitter, big, time.Now().Format("15:04:05"))
	var (
		mu      sync.Mutex
		results []multiRes
		aborted atomic.Bool
		why     atomic.Value
		killOne sync.Once
		wg      sync.WaitGroup
	)
	killAll := func() {
		killOne.Do(func() {
			for _, c := range clients {
				_ = c.cmd.Process.Kill()
			}
		})
	}
	t0 := time.Now()
	for i := 0; i < o.agents; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)*7919))
			for j, tk := range multiTasks(i, o.tasks, o.corpus, o.s2Doc, splitList(o.s3Docs), big) {
				if o.jitter > 0 {
					time.Sleep(time.Duration(rng.Int63n(int64(o.jitter))))
				}
				if aborted.Load() {
					return
				}
				st := time.Since(t0)
				cr := clients[i].toolCall(o.timeout, tk.tool, tk.args)
				en := time.Since(t0)
				cr.Suite, cr.Label = tk.suite, tk.label
				r := multiRes{callRes: cr, Agent: i + 1, Task: j + 1, StartOff: st.Milliseconds(), EndOff: en.Milliseconds()}
				r.Class = classify(cr)
				if aborted.Load() && r.Class != clsOK {
					r.Class = clsAborted
				}
				scoreMulti(&r, gab)
				if cr.text != "" {
					name := fmt.Sprintf("a%02d-t%d-%s-%s.txt", i+1, j+1, tk.suite, tk.label)
					if os.WriteFile(filepath.Join(outDir, name), []byte(cr.text), 0o600) == nil {
						r.BodyFile = name
					}
				}
				mu.Lock()
				results = append(results, r)
				if b, err := json.Marshal(r); err == nil {
					_, _ = lf.Write(append(b, '\n'))
				}
				mu.Unlock()
				logf("CALL agente=%d tarefa=%d suite=%s label=%s classe=%s wall_ms=%d inicio_ms=%d prompt=%d eval=%d s2=%d/%d json=%v estrito=%v erro=%q",
					i+1, j+1, tk.suite, tk.label, r.Class, r.WallMS, r.StartOff, r.PromptE, r.EvalE, r.S2NT, r.S2Total, r.JSONOK, r.Strict, r.Err)
				if abortsStep(r.Class) && aborted.CompareAndSwap(false, true) {
					why.Store(fmt.Sprintf("agente=%d tarefa=%d classe=%s erro=%q", i+1, j+1, r.Class, r.Err))
					logf("ABORT degrau: 1º erro de conexão/protocolo (agente %d); encerrando todos os mcpollama", i+1)
					killAll()
					return
				}
			}
		}(i)
	}
	wg.Wait()
	closeAll()
	s := summarize(o.model, step, o.agents, results, t0)
	s.Aborted = aborted.Load()
	if w, ok := why.Load().(string); ok {
		s.AbortWhy = w
	}
	code := 0
	if s.Aborted {
		code = 3
	}
	if boot != "" {
		b, err := serverBoot(o.uptimeHost)
		switch {
		case err != nil:
			logf("ABORT uptime final: %v", err)
			code = 4
		case b != boot:
			logf("ABORT reboot detectado: boot %s -> %s", boot, b)
			code = 4
		default:
			logf("UPTIME ok fim boot=%s aer_1b4_cor=%s", b, lastAER)
		}
	}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(outDir, "summary.json"), b, 0o600)
	}
	logf("SUMMARY-MULTI modelo=%s degrau=%s agentes=%d chamadas=%d ok=%d classes=%v janela=%s-%s janela_s=%.1f prompt_tok=%d eval_tok=%d tok_s_total=%.0f prefill_tok_s=%.0f geracao_tok_s=%.1f p50_s=%.1f p95_s=%.1f max_s=%.1f s2=%d/%d (%.1f%%) s2_json=%d/%d s3_json=%d/%d s3_estrito=%d/%d grande_ok=%d/%d grande_prompt=%d-%d abortado=%v motivo=%q",
		s.Model, s.Step, s.Agents, s.Calls, s.OK, s.Classes, s.FirstStart, s.LastEnd, s.WallS, s.PromptTok, s.EvalTok, s.TokS, s.PrefillTokS, s.GenTokS,
		s.P50S, s.P95S, s.MaxS, s.S2NT, s.S2Total, pctf(s.S2NT, s.S2Total), s.S2JSON, s.S2Runs, s.S3OK, s.S3Runs, s.S3Strict, s.S3Runs,
		s.BigOK, s.BigRuns, s.BigPromptMin, s.BigPromptMax, s.Aborted, s.AbortWhy)
	return code
}
