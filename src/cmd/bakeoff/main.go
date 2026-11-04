// Command bakeoff runs the phase 8 comparison of local models (plan
// MCPOllam2026-10-28-193200-fase8-bakeoff-modelos): it starts an mcpollama
// binary with --model <candidate> --allowed-models "<candidate>;nomic-embed-text",
// runs the suites S1-S6 through the MCP tools, and writes every model answer to
// <out>/<model>/ (a git-ignored folder), never to stdout. stdout carries only
// metrics. The API token is read from a file into memory and scrubbed from all
// output.
//
//	bakeoff -exe X -model qwen3-ia -suites load,s5,s1,s2,s3,s4,s6
//	bakeoff -suites ref                 # reference texts (doctext) of the corpus
//	bakeoff -suites score -models a,b   # automatic scoring of S1-S4 outputs
//	bakeoff -suites blind -models a,b   # anonymised S1 packets for the human review
//	bakeoff -exe X -model m -suites multi -agents 5 -tasks 3 -step 5x32k   # 8.9: N concurrent agents
//	bakeoff -suites erros -erros-modo retrato|base|comparar ...             # 12.7: erros do modelo (roda no servidor)
//	bakeoff -suites monpeak -mon-file F -from HH:MM:SS -to HH:MM:SS         # 8.9: power/AER of a window
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

type options struct {
	exe, model, allowed, suites, out, corpus, env, url, ca, roots, repo, pdfCache string
	s1Docs, s2Doc, s3Docs, s4Pairs, models, gab, uptimeHost, s5Sizes              string
	reps, s3Reps, maxPrefill                                                      int
	cpt                                                                           float64
	s5Gen                                                                         bool
	timeout, srvTimeout                                                           time.Duration
	// 8.9 multi-agent load and monitor reading
	agents, tasks                                int
	jitter                                       time.Duration
	bigDocs, step, expectBoot, monFile, from, to string
	// Fase 12.7: suite erros (runs on the server, next to treino.db)
	errMode, treinoDB, keyFile, gwURL, retratoID, atual, candidato, alvo string
	candScore                                                            string
	indiceID, instrFile, embedModel                                      string
	exemplosK                                                            int
	exemplosLimiar                                                       float64
	gwRemote, outSet                                                     bool
	resultado, blindResp                                                 string
}

// outExplicit is -out only when the user typed it: the suite erros has no default folder.
func (o options) outExplicit() string {
	if !o.outSet {
		return ""
	}
	return o.out
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bakeoff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.exe, "exe", "", "mcpollama binary under test (never the one in D:\\MCP)")
	fs.StringVar(&o.model, "model", "", "model under test (--model)")
	fs.StringVar(&o.allowed, "allowed-models", "", "--allowed-models (default \"<model>;nomic-embed-text\")")
	fs.StringVar(&o.suites, "suites", "load,s5,s1,s2,s3,s4,s6", "comma list: load,ps,s1..s6 | ref | score | blind | multi | monpeak | erros")
	fs.StringVar(&o.out, "out", `E:\MCPOllam\.workspace\bakeoff\out`, "output root (git-ignored)")
	fs.StringVar(&o.corpus, "corpus", `E:\MCPOllam\.workspace\bakeoff\corpus`, "corpus folder")
	fs.StringVar(&o.env, "env", `D:\Users\claiton.linhares\.openclaude\csl-ollama.env`, "file with OPENAI_API_KEY=")
	fs.StringVar(&o.url, "ollama-url", "https://ia.cslsolucoes.com.br:8443", "--ollama-url")
	fs.StringVar(&o.ca, "tls-ca", `D:\Users\claiton.linhares\.openclaude\certs\cloudflare-origin-ca.pem`, "--tls-ca")
	fs.StringVar(&o.roots, "allow-root", "", "--allow-root (default: repo, or corpus;repo when the corpus is outside it)")
	fs.StringVar(&o.repo, "repo", `E:\MCPOllam`, "repository root (S6 count files)")
	fs.StringVar(&o.pdfCache, "pdf-cache-dir", "", "--pdf-cache-dir")
	fs.StringVar(&o.s1Docs, "s1-docs", "b01.docx,b02.pptx,b03.pdf,b05.pdf,b06.pdf", "S1 documents")
	fs.StringVar(&o.s2Doc, "s2-doc", "b01.docx", "S2 document")
	fs.StringVar(&o.s3Docs, "s3-docs", "b01.docx,b02.pptx,b03.pdf", "S3 documents")
	fs.StringVar(&o.s4Pairs, "s4", "", "S4 pairs id=language, comma separated")
	fs.StringVar(&o.models, "models", "", "score/blind: models (comma list)")
	fs.StringVar(&o.gab, "gab", `E:\MCPOllam\.workspace\bakeoff\gab\s2-word.tsv`, "S2 gabarito (Word COM)")
	fs.StringVar(&o.uptimeHost, "uptime-ssh", "", "ssh host alias: checks `uptime -s` before every suite block and stops on any change (reboot)")
	fs.StringVar(&o.s5Sizes, "s5-sizes", "8000,12000", "S5 prefill sizes in tokens (owner: no 16k/32k after the reboots)")
	fs.IntVar(&o.maxPrefill, "max-prefill-tokens", 0, "skip (pulado-risco) any call whose estimated prefill exceeds this (0 = off)")
	fs.Float64Var(&o.cpt, "chars-per-token", 3.0, "chars/token used by -max-prefill-tokens (low = conservative)")
	fs.BoolVar(&o.s5Gen, "s5-gen", true, "S5: also run the 1000-token generation")
	fs.IntVar(&o.reps, "reps", 5, "repetitions for S1 and S2")
	fs.IntVar(&o.s3Reps, "s3-reps", 2, "repetitions for S3")
	fs.DurationVar(&o.timeout, "call-timeout", 20*time.Minute, "driver timeout per tool call")
	fs.DurationVar(&o.srvTimeout, "server-timeout", 300*time.Second, "--timeout of the server")
	fs.IntVar(&o.agents, "agents", 2, "multi: concurrent agents (one mcpollama process each)")
	fs.IntVar(&o.tasks, "tasks", 3, "multi: tasks per agent (alternating S2 and S3 JSON)")
	fs.DurationVar(&o.jitter, "jitter", 2*time.Second, "multi: random delay 0..jitter before each task")
	fs.StringVar(&o.bigDocs, "big-docs", "", "multi: \"prompt grande\" documents (one summarize_file per agent, doc[i % len])")
	fs.StringVar(&o.step, "step", "", "multi: step label (output folder), e.g. 5x32k")
	fs.StringVar(&o.expectBoot, "expect-boot", "", "multi: expected `uptime -s`; any other boot aborts before the load")
	fs.StringVar(&o.monFile, "mon-file", "", "monpeak: monitor output file")
	fs.StringVar(&o.from, "from", "", "monpeak: window start HH:MM:SS")
	fs.StringVar(&o.to, "to", "", "monpeak: window end HH:MM:SS")
	fs.StringVar(&o.errMode, "erros-modo", "comparar", "erros: indices-limpar (apaga os índices inválidos) | indice (congela os exemplos públicos, 12.8) | retrato (congela o teste) | base (linha de base do modelo atual) | comparar (candidato x atual) | apurar (aplica as respostas do owner ao blind)")
	fs.StringVar(&o.treinoDB, "treino-db", "", "erros: caminho do treino.db (o replay roda no servidor; o conteúdo dos casos não sai da rede, C-5.6)")
	fs.StringVar(&o.keyFile, "doc-key-file", "", "erros: arquivo da chave HMAC (0600); com ele o retrato confere o prompt_hmac recalculado")
	fs.StringVar(&o.gwURL, "gateway-url", "http://127.0.0.1:11500", "erros: gateway do replay (loopback)")
	fs.BoolVar(&o.gwRemote, "gateway-remoto", false, "erros: aceita um gateway que não é loopback (rede interna, decisão do owner)")
	fs.StringVar(&o.retratoID, "retrato", "", "erros: id do retrato (modos base e comparar)")
	fs.StringVar(&o.atual, "atual", "qwen3-ia", "erros: nome do modelo atual (rótulo; pasta do score.json do S2/S3)")
	fs.StringVar(&o.candidato, "candidato", "", "erros: modelo candidato (modo comparar)")
	fs.StringVar(&o.candScore, "candidato-score", "", "erros/comparar: score.json do S2/S3 do candidato COM as instruções (vazio: <out>/<candidato>/score.json; igual ao do atual reprova o gate, C-5.12)")
	fs.StringVar(&o.alvo, "alvo", "", "erros: categoria-alvo da mudança (C-5.10)")
	fs.StringVar(&o.indiceID, "indice", "", "erros/retrato e comparar: id do índice de exemplos congelado em treino.db (12.8; vazio = nenhum)")
	fs.StringVar(&o.instrFile, "instrucoes", "", "erros/retrato e comparar: arquivo JSON da versão das instruções (12.8; vazio = nenhuma)")
	fs.IntVar(&o.exemplosK, "exemplos-k", 0, "erros/indice: k, quantos pares erro->certo no máximo (12.8, C-8.10; 0 = 3, provisório até haver dados medidos)")
	fs.Float64Var(&o.exemplosLimiar, "exemplos-limiar", 0, "erros/indice: limiar de similaridade (cosseno) dos exemplos (12.8, C-8.10; 0 = 0.80, provisório)")
	fs.StringVar(&o.embedModel, "embed-modelo", "", "erros/indice: modelo de embedding dos vetores do índice (padrão nomic-embed-text)")
	fs.StringVar(&o.resultado, "resultado", "", "erros/apurar: resultado-*.json escrito pelo modo comparar")
	fs.StringVar(&o.blindResp, "blind-respostas", "", "erros/apurar: arquivo de respostas do owner (rotulo<TAB>certa|errada<TAB>certa|errada)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "out" {
			o.outSet = true
		}
	})
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	var mu sync.Mutex
	token := ""
	logf := func(format string, a ...any) {
		s := fmt.Sprintf(format, a...)
		if token != "" {
			s = strings.ReplaceAll(s, token, "[omitido]")
		}
		mu.Lock()
		fmt.Fprintln(w, s)
		w.Flush()
		mu.Unlock()
	}
	switch o.suites {
	case "ref":
		return runRef(o, logf)
	case "score":
		return runScore(o, logf)
	case "blind":
		return runBlind(o, logf)
	case "monpeak":
		return runMonPeak(o, logf)
	case "erros":
		return runErros(o, logf)
	}
	if o.exe == "" || o.model == "" {
		fmt.Fprintln(stderr, "bakeoff: -exe e -model são obrigatórios")
		return 2
	}
	if strings.EqualFold(filepath.Dir(o.exe), `D:\MCP`) {
		fmt.Fprintln(stderr, "bakeoff: use um binário de teste, não o de D:\\MCP")
		return 2
	}
	t, err := readToken(o.env)
	if err != nil {
		fmt.Fprintln(stderr, "bakeoff: token:", err)
		return 2
	}
	token = t
	if o.allowed == "" {
		o.allowed = o.model + ";nomic-embed-text"
	}
	if o.roots == "" {
		// The server refuses nested roots; the corpus lives inside the repo.
		o.roots = o.repo
		if rel, err := filepath.Rel(o.repo, o.corpus); err != nil || strings.HasPrefix(rel, "..") {
			o.roots = o.corpus + ";" + o.repo
		}
	}
	srv := []string{"--ollama-url", o.url, "--tls-ca", o.ca, "--allow-root", o.roots, "--api-key", token,
		"--model", o.model, "--allowed-models", o.allowed, "--timeout", o.srvTimeout.String()}
	if o.pdfCache != "" {
		srv = append(srv, "--pdf-cache-dir", o.pdfCache)
	}
	if o.suites == "multi" {
		return runMulti(o, srv, logf)
	}
	outDir := filepath.Join(o.out, safeName(o.model))
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
	c, err := startClient(o.exe, srv, logf)
	if err != nil {
		logf("FATAL start: %v", err)
		return 2
	}
	defer c.close()
	if err := c.initialize(); err != nil {
		logf("FATAL initialize: %v", err)
		return 2
	}
	r := &runner{c: c, model: o.model, outDir: outDir, corpus: o.corpus, reps: o.reps, timeout: o.timeout, log: lf, logf: logf,
		maxPrefill: o.maxPrefill, cpt: o.cpt, refDir: filepath.Join(o.out, "ref")}
	if r.cpt <= 0 {
		r.cpt = 3.0
	}
	t0 := time.Now()
	logf("INIT ok modelo=%s suites=%s inicio=%s", o.model, o.suites, t0.Format("2006-01-02 15:04:05"))
	var sizes []int
	for _, v := range splitList(o.s5Sizes) {
		if n := atoiOr(v, 0); n > 0 {
			sizes = append(sizes, n)
		}
	}
	boot := ""
	if o.uptimeHost != "" {
		b, err := serverBoot(o.uptimeHost)
		if err != nil {
			logf("ABORT uptime inicial: %v", err)
			return 4
		}
		boot = b
		logf("UPTIME boot=%s (referência) aer_1b4_cor=%s", boot, lastAER)
	}
	if code := r.suites(o, sizes, boot, logf); code != 0 {
		logf("SUMMARY-ABORTADO modelo=%s chamadas=%d isError=%d erros_protocolo=%d duracao_s=%.0f",
			o.model, r.n, r.errs, r.protos, time.Since(t0).Seconds())
		return code
	}
	logf("SUMMARY modelo=%s chamadas=%d isError=%d erros_protocolo=%d http429=%d queue_ms_maior_que_0=%d linhas_stdout_invalidas=%d pulados_risco=%d duracao_s=%.0f",
		o.model, r.n, r.errs, r.protos, r.http429, r.queued, c.badLines.Load(), r.skipped, time.Since(t0).Seconds())
	return 0
}

// suites runs the suite blocks. Before each block (except ps) it checks the
// server boot time; a change or a failed check stops everything (exit 4). The
// first failed call stops everything too (exit 3, abortError).
func (r *runner) suites(o options, sizes []int, boot string, logf func(string, ...any)) (code int) {
	defer func() {
		if v := recover(); v != nil {
			ae, ok := v.(abortError)
			if !ok {
				panic(v)
			}
			logf("ABORT primeira falha: %s", ae.why)
			code = 3
		}
	}()
	for _, s := range splitList(o.suites) {
		if boot != "" && s != "ps" {
			b, err := serverBoot(o.uptimeHost)
			if err != nil {
				logf("ABORT uptime antes de %s: %v", s, err)
				return 4
			}
			if b != boot {
				logf("ABORT reboot detectado antes de %s: boot %s -> %s", s, boot, b)
				return 4
			}
			logf("UPTIME ok antes de %s boot=%s aer_1b4_cor=%s", s, b, lastAER)
		}
		st := time.Now()
		logf("SUITE %s inicio=%s", s, st.Format("15:04:05"))
		switch s {
		case "load":
			r.suiteLoad()
		case "ps":
			r.suitePS(time.Now().Format("150405"))
		case "s1":
			r.suiteS1(splitList(o.s1Docs))
		case "s2":
			r.suiteS2(o.s2Doc)
		case "s3":
			r.suiteS3(splitList(o.s3Docs), o.s3Reps)
		case "s4":
			r.suiteS4(splitList(o.s4Pairs))
		case "s5":
			r.suiteS5(sizes, o.s5Gen)
		case "s6":
			r.suiteS6(defaultCountCases(o.repo))
		default:
			logf("suite desconhecida: %s", s)
		}
		logf("SUITE %s fim duracao_s=%.0f", s, time.Since(st).Seconds())
	}
	if boot != "" {
		b, err := serverBoot(o.uptimeHost)
		if err != nil {
			logf("ABORT uptime final: %v", err)
			return 4
		}
		if b != boot {
			logf("ABORT reboot detectado no fim: boot %s -> %s", boot, b)
			return 4
		}
		logf("UPTIME ok fim boot=%s aer_1b4_cor=%s", b, lastAER)
	}
	return 0
}

// serverBoot returns `uptime -s` of the server over ssh (BatchMode, 20 s).
func serverBoot(host string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=20", host,
		"uptime -s; awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable").Output()
	if err != nil {
		return "", fmt.Errorf("ssh falhou: %v", err)
	}
	// Output: "YYYY-MM-DD HH:MM:SS" (two fields), then the AER total.
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return "", errors.New("uptime vazio ou incompleto")
	}
	lastAER = "?"
	if len(f) >= 3 {
		lastAER = f[2]
	}
	return f[0] + " " + f[1], nil
}

// lastAER is TOTAL_ERR_COR of the root port 00:1b.4 (link of GPU 03:00.0) read
// by the last serverBoot; it is logged before and after every block.
var lastAER = "?"

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// safeName turns a model tag into a folder name: "gemma4:12b" -> "gemma4_12b".
func safeName(m string) string {
	r := strings.NewReplacer(":", "_", "/", "_", "\\", "_")
	return r.Replace(m)
}

func readToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("arquivo de token ilegível")
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "OPENAI_API_KEY=") {
			if v := strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "OPENAI_API_KEY=")), `"'`); v != "" {
				return v, nil
			}
		}
	}
	return "", errors.New("OPENAI_API_KEY não encontrada")
}

func atoiOr(s string, d int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return d
}
