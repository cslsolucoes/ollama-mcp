package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// runner holds the state of one bake-off execution (one model, one process).
type runner struct {
	c       *client
	model   string
	outDir  string // <out>/<model>
	corpus  string
	reps    int
	timeout time.Duration
	log     *os.File // <outDir>/calls.jsonl
	logf    func(string, ...any)
	n       int
	nonce   int64
	errs    int
	protos  int
	http429 int
	queued  int // sequential calls that reported queue_ms > 0 (anomaly)
	// Risk gate (owner, 2026-10-28 21:03): skip any call whose estimated
	// prefill exceeds maxPrefill tokens (0 = off). The estimate is the prompt
	// size in characters divided by cpt (a deliberately low chars/token ratio).
	maxPrefill int
	cpt        float64
	refDir     string
	skipped    int
}

// promptOverhead is a generous allowance, in characters, for the system
// prompt, the task text and the markers the server adds around the file.
const promptOverhead = 1500

// estimateTokens returns the estimated prefill of one call: the text the model
// will receive (the doctext reference of a document, the file itself for text
// files, or the generate prompt) plus the instruction and an overhead.
func (r *runner) estimateTokens(args map[string]any) (int, string) {
	chars := promptOverhead
	src := "prompt"
	if p, ok := args["path"].(string); ok {
		base := filepath.Base(p)
		ext := strings.ToLower(filepath.Ext(base))
		file := p
		if ext == ".docx" || ext == ".pptx" || ext == ".pdf" {
			file = filepath.Join(r.refDir, base+".txt")
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return 1 << 30, "texto_indisponivel" // unknown size: treat as too large
		}
		chars += len([]rune(string(b)))
		src = "arquivo"
	}
	for _, k := range []string{"prompt", "instruction", "format", "focus"} {
		if v, ok := args[k].(string); ok {
			chars += len([]rune(v))
		}
	}
	return int(float64(chars) / r.cpt), src
}

// call paces the calls (1 s, nginx rate limit), runs one tool call, writes the
// answer to file (never to stdout) and logs the metrics line.
func (r *runner) call(suite, label, tool string, args map[string]any, save string) callRes {
	if r.maxPrefill > 0 {
		est, src := r.estimateTokens(args)
		if est > r.maxPrefill {
			r.skipped++
			res := callRes{Suite: suite, Label: label, Tool: tool, Err: fmt.Sprintf("pulado-risco: prefill estimado %d > %d tokens (%s)", est, r.maxPrefill, src)}
			if b, err := json.Marshal(res); err == nil && r.log != nil {
				_, _ = r.log.Write(append(b, byte(10)))
			}
			r.logf("PULADO-RISCO suite=%s label=%s tool=%s prefill_estimado=%d teto=%d fonte=%s", suite, label, tool, est, r.maxPrefill, src)
			return res
		}
		r.logf("PREFILL-ESTIMADO suite=%s label=%s tokens=%d teto=%d", suite, label, est, r.maxPrefill)
	}
	if r.n > 0 {
		time.Sleep(time.Second)
	}
	r.n++
	res := r.c.toolCall(r.timeout, tool, args)
	res.Suite, res.Label = suite, label
	if save != "" && res.text != "" {
		p := filepath.Join(r.outDir, suite, save)
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(res.text), 0o600); err == nil {
			res.BodyFile = filepath.ToSlash(filepath.Join(suite, save))
		}
	}
	if res.IsError {
		r.errs++
	}
	if res.Proto {
		r.protos++
	}
	if strings.Contains(res.Err, "429") {
		r.http429++
	}
	if res.QueueMS > 0 {
		r.queued++
	}
	if b, err := json.Marshal(res); err == nil && r.log != nil {
		_, _ = r.log.Write(append(b, '\n'))
	}
	r.logf("CALL suite=%s label=%s tool=%s wall_ms=%d isError=%v erro=%q %s", suite, label, tool, res.WallMS, res.IsError, res.Err, res.Uso)
	if res.IsError || res.Proto {
		// Owner's rule after the reboots of 2026-10-28: the first failed call
		// (connection, transport, timeout or any tool error) ends the whole
		// process. No next call, no next suite, no next model.
		panic(abortError{fmt.Sprintf("suite=%s label=%s erro=%q", suite, label, res.Err)})
	}
	return res
}

// abortError stops the run at the first failed call; run recovers it.
type abortError struct{ why string }

func (r *runner) doc(id string) string { return filepath.Join(r.corpus, id) }

// ---------- load / ps ----------

func (r *runner) suiteLoad() {
	res := r.call("load", "frio", "generate", map[string]any{"prompt": "Responda apenas: OK", "num_predict": 8}, "frio.txt")
	r.logf("LOAD modelo=%s load_ms=%d wall_ms=%d", r.model, res.LoadMS, res.WallMS)
	res = r.call("load", "quente", "generate", map[string]any{"prompt": "Responda apenas: OK", "num_predict": 8}, "quente.txt")
	r.logf("LOAD-QUENTE modelo=%s load_ms=%d wall_ms=%d", r.model, res.LoadMS, res.WallMS)
}

func (r *runner) suitePS(tag string) {
	r.call("ps", tag, "list_models", map[string]any{}, "list_models-"+tag+".txt")
}

// ---------- S1: summary fidelity ----------

func (r *runner) suiteS1(docs []string) {
	for _, d := range docs {
		for k := 1; k <= r.reps; k++ {
			r.call("s1", fmt.Sprintf("%s-r%d", d, k), "summarize_file",
				map[string]any{"path": r.doc(d), "max_words": 150}, fmt.Sprintf("%s-r%d.txt", d, k))
		}
	}
}

// ---------- S2: numbered sections ----------

const s2Instruction = "Liste TODAS as seções numeradas do documento (os títulos que têm numeração, em todos os níveis, por exemplo 1, 1.1 e 1.1.1), " +
	"na ordem em que aparecem, com o número exatamente como aparece no documento e o título da seção"
const s2Format = `um array JSON de objetos {"numero": string, "titulo": string}, sem nenhum texto fora do array`

func (r *runner) suiteS2(doc string) {
	for k := 1; k <= r.reps; k++ {
		r.call("s2", fmt.Sprintf("%s-r%d", doc, k), "extract",
			map[string]any{"path": r.doc(doc), "instruction": s2Instruction, "format": s2Format}, fmt.Sprintf("%s-r%d.txt", doc, k))
	}
}

// ---------- S3: format ----------

var s3Formats = []struct{ key, format string }{
	{"json", "um array JSON de strings, sem nenhum texto fora do array"},
	{"lista", "uma lista com marcadores, um item por linha, cada linha começando com \"- \", sem nenhum texto antes ou depois da lista"},
	{"tabela", "uma tabela markdown com exatamente duas colunas (| item | descrição |), com linha de cabeçalho e linha separadora, sem nenhum texto antes ou depois da tabela"},
}

const s3Instruction = "Liste os principais tópicos tratados no documento (de 4 a 8 tópicos), cada um com uma descrição curta"

func (r *runner) suiteS3(docs []string, reps int) {
	for _, d := range docs {
		for _, f := range s3Formats {
			for k := 1; k <= reps; k++ {
				r.call("s3", fmt.Sprintf("%s-%s-r%d", d, f.key, k), "extract",
					map[string]any{"path": r.doc(d), "instruction": s3Instruction, "format": f.format}, fmt.Sprintf("%s-%s-r%d.txt", d, f.key, k))
			}
		}
	}
}

// ---------- S4: translation ----------

// suiteS4 translates "id=language" pairs: e.g. "b01-trecho.txt=inglês".
func (r *runner) suiteS4(pairs []string) {
	for _, p := range pairs {
		id, lang, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		r.call("s4", id, "translate", map[string]any{"path": r.doc(id), "target_language": lang}, id+".traducao.txt")
	}
}

// ---------- S5: performance ----------

func (r *runner) numberedPrompt(tokens int) string {
	r.nonce++
	chars := int(float64(tokens) * 2.67)
	var b strings.Builder
	fmt.Fprintf(&b, "Nonce %d-%d. Abaixo há um log numerado.\n", time.Now().UnixNano(), r.nonce)
	for n := 1; b.Len() < chars; n++ {
		fmt.Fprintf(&b, "Linha %05d: o servidor processou a requisição %d do cliente e devolveu status 200.\n", n, n)
	}
	b.WriteString("Responda apenas no formato 'primeira=NNNNN ultima=NNNNN'.")
	return b.String()
}

func (r *runner) suiteS5(sizes []int, withGen bool) {
	for _, t := range sizes {
		for k := 1; k <= 2; k++ {
			p := r.numberedPrompt(t)
			res := r.call("s5", fmt.Sprintf("prefill-%dk-r%d", t/1000, k), "generate", map[string]any{"prompt": p, "num_predict": 16}, "")
			gen := res.TotalMS - res.LoadMS
			rate := 0.0
			if gen > 0 {
				rate = float64(res.PromptE) / (float64(gen) / 1000)
			}
			r.logf("S5 tipo=prefill alvo_tokens=%d prompt_eval_count=%d eval_count=%d total_ms=%d load_ms=%d prefill_tok_s_aprox=%.0f", t, res.PromptE, res.EvalE, res.TotalMS, res.LoadMS, rate)
		}
	}
	if !withGen {
		return
	}
	for k := 1; k <= 2; k++ {
		r.nonce++
		p := fmt.Sprintf("[id %d] Escreva um texto técnico longo e detalhado, em português do Brasil (cerca de 1000 palavras), sobre balanceamento de carga em servidores de inferência.", r.nonce)
		res := r.call("s5", fmt.Sprintf("geracao-r%d", k), "generate", map[string]any{"prompt": p, "num_predict": 1000}, fmt.Sprintf("geracao-r%d.txt", k))
		gen := res.TotalMS - res.LoadMS
		rate := 0.0
		if gen > 0 {
			rate = float64(res.EvalE) / (float64(gen) / 1000)
		}
		r.logf("S5 tipo=geracao prompt_eval_count=%d eval_count=%d total_ms=%d load_ms=%d geracao_tok_s_aprox=%.1f fim=%s", res.PromptE, res.EvalE, res.TotalMS, res.LoadMS, rate, res.Fim)
	}
}

// ---------- S6: known limits (informative) ----------

var firstIntRe = regexp.MustCompile(`-?\d+`)
var resultRe = regexp.MustCompile(`(?i)RESULTADO:\s*\**\s*(-?\d+)`)

func (r *runner) suiteS6(countFiles []countCase) {
	exprs := []struct {
		q    string
		want int
	}{{"17*23 + 144/12", 403}, {"48*12 - 96/8", 564}, {"125*4 + 81/9", 509}, {"36*27 - 512/16", 940}, {"19*19 + 210/14", 376}}
	totD, totS := 0, 0
	// With -think the reasoning counts in num_predict: 50 tokens leave the
	// answer empty (qwen3.8:27b, 2026-11-05: ~70 tokens of reasoning).
	npD, npS := 50, 1500
	if r.c.think {
		npD, npS = 4096, 4096
	}
	for i, e := range exprs {
		okD, okS := 0, 0
		var gotD, gotS []string
		for k := 1; k <= 3; k++ {
			res := r.call("s6", fmt.Sprintf("p3d-%d-r%d", i, k), "generate", map[string]any{"prompt": "Responda apenas com o número, sem texto: quanto é " + e.q + "?", "temperature": 0, "num_predict": npD}, fmt.Sprintf("p3d-%d-r%d.txt", i, k))
			g := firstIntRe.FindString(bodyOf(res.text))
			gotD = append(gotD, g)
			if g == strconv.Itoa(e.want) {
				okD++
			}
		}
		for k := 1; k <= 2; k++ {
			res := r.call("s6", fmt.Sprintf("p3s-%d-r%d", i, k), "generate", map[string]any{"prompt": "Calcule passo a passo e termine com uma linha no formato RESULTADO: <número>. Quanto é " + e.q + "?", "temperature": 0, "num_predict": npS}, fmt.Sprintf("p3s-%d-r%d.txt", i, k))
			g := ""
			if m := resultRe.FindStringSubmatch(bodyOf(res.text)); m != nil {
				g = m[1]
			}
			gotS = append(gotS, g)
			if g == strconv.Itoa(e.want) {
				okS++
			}
		}
		totD += okD
		totS += okS
		r.logf("S6 P3 expr=%q esperado=%d direto_ok=%d/3 direto=%v passo_ok=%d/2 passo=%v", e.q, e.want, okD, gotD, okS, gotS)
	}
	r.logf("S6 P3 TOTAL direto_ok=%d/15 passo_ok=%d/10", totD, totS)
	totC, totL, n := 0, 0, 0
	for _, cs := range countFiles {
		raw, err := os.ReadFile(cs.path)
		if err != nil {
			r.logf("S6 R1 caso=%s erro_leitura", cs.name)
			continue
		}
		want := cs.truth(string(raw))
		okC, okL := 0, 0
		var gotC, gotL []string
		for k := 1; k <= 3; k++ {
			res := r.call("s6", fmt.Sprintf("r1c-%s-r%d", cs.name, k), "extract", map[string]any{"path": cs.path, "instruction": "Conte " + cs.what + ". Responda apenas com o número."}, "")
			g := firstIntRe.FindString(bodyOf(res.text))
			gotC = append(gotC, g)
			if g == strconv.Itoa(want) {
				okC++
			}
		}
		for k := 1; k <= 3; k++ {
			res := r.call("s6", fmt.Sprintf("r1l-%s-r%d", cs.name, k), "extract", map[string]any{"path": cs.path, "instruction": "Liste " + cs.what + ", um item por elemento, na ordem do arquivo.", "format": "um array JSON de strings, sem texto fora do array"}, "")
			b, _ := unfence(bodyOf(res.text))
			if i := strings.Index(b, "["); i >= 0 {
				if j := strings.LastIndex(b, "]"); j > i {
					b = b[i : j+1]
				}
			}
			var arr []any
			g := "parse_erro"
			if json.Unmarshal([]byte(b), &arr) == nil {
				g = strconv.Itoa(len(arr))
				if len(arr) == want {
					okL++
				}
			}
			gotL = append(gotL, g)
		}
		n += 3
		totC += okC
		totL += okL
		r.logf("S6 R1 caso=%s esperado=%d contar_ok=%d/3 contar=%v listar_ok=%d/3 listar=%v", cs.name, want, okC, gotC, okL, gotL)
	}
	r.logf("S6 R1 TOTAL contar_ok=%d/%d listar_ok=%d/%d", totC, n, totL, n)
}

type countCase struct {
	name, path, what string
	truth            func(string) int
}

func linesWith(text, prefix string) int {
	n := 0
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimRight(l, "\r"), prefix) {
			n++
		}
	}
	return n
}

func defaultCountCases(repo string) []countCase {
	return []countCase{
		{"readme-flags", filepath.Join(repo, "README.md"),
			"as linhas da tabela de configuração que começam com uma flag de linha de comando (| `--...`)",
			func(t string) int { return linesWith(t, "| `--") }},
		{"tools-headers", filepath.Join(repo, "TOOLS.md"),
			"os cabeçalhos de seção de nível 2 que trazem o nome de uma tool entre crases (## `nome`)",
			func(t string) int { return linesWith(t, "## `") }},
	}
}

// shuffled returns a copy of s in a random order (used nowhere in scoring;
// kept deterministic by seed for reproducible blind labels).
func shuffled(s []string, seed int64) []string {
	out := append([]string(nil), s...)
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
