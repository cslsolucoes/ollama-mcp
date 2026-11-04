package main

// Suite `erros` of the bake-off (Fase 12, stage 12.7; contract chapter 5, C-4.13,
// C-4.14): the evaluation of a change (new instructions, examples, a model v2)
// against the cases of the test partition that the administrator confirmed.
//
//	bakeoff -suites erros -erros-modo retrato  -treino-db D -doc-key-file K -out O
//	bakeoff -suites erros -erros-modo base     -treino-db D -doc-key-file K -retrato ID -out O
//	bakeoff -suites erros -erros-modo comparar -treino-db D -doc-key-file K -retrato ID -candidato M -alvo CAT -out O
//
// WHERE IT RUNS: on the server, next to treino.db. The content of the cases (the
// request and the correct answer) is a customer's, so it must not leave the
// network (C-5.6): this program reads it from treino.db and replays it to the
// gateway of the same machine (127.0.0.1:11500), and everything it writes (the
// report, the baseline file) carries only ids, categories and numbers. The only
// file with text is the one of the blind review, written 0600 for the owner. The
// program never sends any text to anybody else: its HTTP client talks to the
// gateway host only.
//
// A retrato is made once (mode retrato) and then used by the baseline (mode base,
// the first round of the current model, C-5.14) and by every comparison (mode
// comparar; the current model is measured again each time, C-5.16).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

// Thresholds of the protocol (C-5.8, C-5.10 to C-5.12, C-5.16).
const (
	minReport     = 20 // a category is reported with at least this many cases
	minDecide     = 30 // and decides with at least this many
	gainPP        = 10 // C-5.10: gain of the target category, in points
	otherDropPP   = 3  // C-5.11
	s23DropPP     = 2  // C-5.12
	baselineDiffP = 3  // C-5.16
)

// Exit codes of the suite.
const (
	exitOK       = 0
	exitGate     = 1 // the gate FAILS: insufficient data (C-5.13)
	exitUsage    = 2
	exitRejected = 3 // valid evaluation, the change does not meet C-5.10 to C-5.12
	exitPending  = 4 // valid so far, waiting for the owner's blind review (C-5.5)
)

// errosOpts are the options of the suite.
type errosOpts struct {
	Mode        string // retrato | base | comparar | apurar
	DB, KeyFile string
	GatewayURL  string
	AllowRemote bool
	RetratoID   string
	Atual       string // name of the current model (label and folder of its S2/S3 score)
	Candidato   string
	CandScore   string // score.json of the S2/S3 of the candidate (I-9); empty: <out>/<candidato>/score.json
	Alvo        string // target category of the change
	Out         string // output root (the same -out of the other suites)
	Timeout     time.Duration
	RetryWait   time.Duration // 0 keeps the default (tests)
	Hashes      treino.RetratoOptions
	// Stage 12.8: the frozen index (id in treino.db) and the instructions file the
	// candidate uses; k, the floor and the embedding model of mode `indice`.
	IndiceID, InstrFile string
	ExemplosK           int
	ExemplosLimiar      float64
	EmbedModel          string
	Now                 func() time.Time
	ShuffleSeed         int64 // 0 = random (blind labels)
	// apurar: the result file of a comparison and the owner's answers to the blind review
	Resultado, BlindRespostas string
	S23MaxAge                 time.Duration // age limit of a score.json (default 48 h)
	Transform                 requestTransform
}

func (o options) errosOpts() errosOpts {
	return errosOpts{Mode: o.errMode, DB: o.treinoDB, KeyFile: o.keyFile, GatewayURL: o.gwURL, AllowRemote: o.gwRemote,
		RetratoID: o.retratoID, Atual: o.atual, Candidato: o.candidato, CandScore: o.candScore, Alvo: o.alvo, Out: o.outExplicit(), Timeout: o.timeout,
		Resultado: o.resultado, BlindRespostas: o.blindResp,
		IndiceID: o.indiceID, InstrFile: o.instrFile, ExemplosK: o.exemplosK, ExemplosLimiar: o.exemplosLimiar, EmbedModel: o.embedModel}
}

func runErros(o options, logf func(string, ...any)) int { return errosMain(o.errosOpts(), logf) }

// ---- results ----------------------------------------------------------------------------------------

type modelStat struct {
	N, Pass, Changed int // measured cases, right cases (2 of 3), cases that changed between rounds
}

type catRes struct {
	Cat          string
	Mode         string
	Atual, Cand  modelStat
	Inapplicable int
	Blind        int  // cases in the blind file (mode blind)
	Human        bool // judged by the owner (mode apurar): Atual and Cand hold the counts
}

type caseRes struct {
	OcorrenciaID string
	Categoria    string
	Rounds       map[string][]bool // "atual" / "candidato"
}

// baselineFile is the registered baseline (C-5.14): ids, categories and numbers.
type baselineFile struct {
	RetratoID string                `json:"retrato_id"`
	Modelo    string                `json:"modelo"`
	GeradaEm  string                `json:"gerada_em"`
	Cats      map[string]baselineCt `json:"categorias"`
	Casos     []baselineCase        `json:"casos"`
	S23       *s23                  `json:"s2_s3,omitempty"`
}
type baselineCt struct {
	N       int `json:"n"`
	Acertos int `json:"acertos"`
	Mudaram int `json:"mudaram"`
}
type baselineCase struct {
	OcorrenciaID string `json:"ocorrencia_id"`
	Categoria    string `json:"categoria"`
	Rodadas      []bool `json:"rodadas"`
}

// s23 are the S2 and S3 of Fase 8 read from the score.json of a model.
type s23 struct {
	S2OK  int `json:"s2_ok"`
	S2Tot int `json:"s2_total"`
	S3OK  int `json:"s3_ok"`
	S3Tot int `json:"s3_total"`
}

// readS23 reads <dir>/score.json written by `-suites score`: S2 is the numbers
// whose title also matches (the metric S2-TOTAL prints), S3 the formats that passed.
// The file has to name the model it was made for and the time it was made, and
// they are checked: the model is the one expected, and the file is not from the
// future nor older than maxAge (a score of another day is not the S2/S3 of this
// session, C-5.16).
func readS23(path, model string, now time.Time, maxAge time.Duration) (*s23, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc struct {
		Modelo   string `json:"modelo"`
		GeradoEm string `json:"gerado_em"`
		S2       []struct {
			NumTitleOK int `json:"numero_e_titulo_ok"`
			Total      int `json:"total_gabarito"`
		} `json:"s2"`
		S3 map[string]struct {
			OK bool `json:"ok"`
		} `json:"s3"`
	}
	if err := json.Unmarshal(b, &sc); err != nil {
		return nil, err
	}
	if sc.Modelo != model {
		return nil, fmt.Errorf("score.json é do modelo %q, esperado %q", sc.Modelo, model)
	}
	at, err := time.Parse(time.RFC3339, sc.GeradoEm)
	if err != nil {
		return nil, errors.New("score.json sem gerado_em: refaça -suites score")
	}
	if at.After(now.Add(5*time.Minute)) || now.Sub(at) > maxAge {
		return nil, fmt.Errorf("score.json de %s fora da janela de %s", sc.GeradoEm, maxAge)
	}
	r := &s23{}
	for _, x := range sc.S2 {
		r.S2OK += x.NumTitleOK
		r.S2Tot += x.Total
	}
	for _, x := range sc.S3 {
		r.S3Tot++
		if x.OK {
			r.S3OK++
		}
	}
	if r.S2Tot == 0 || r.S3Tot == 0 {
		return nil, errors.New("score.json sem S2 ou S3")
	}
	return r, nil
}

// ---- main -------------------------------------------------------------------------------------------

func errosMain(eo errosOpts, logf func(string, ...any)) int {
	if eo.Now == nil {
		eo.Now = time.Now
	}
	if eo.Timeout <= 0 {
		eo.Timeout = 20 * time.Minute
	}
	if eo.Atual == "" {
		eo.Atual = "qwen3-ia"
	}
	if eo.S23MaxAge <= 0 {
		eo.S23MaxAge = 48 * time.Hour
	}
	if eo.Mode != "indice" && eo.Mode != "indices-limpar" && eo.Mode != "retrato" && eo.Mode != "base" && eo.Mode != "comparar" && eo.Mode != "apurar" {
		logf("FATAL -erros-modo deve ser indice, indices-limpar, retrato, base, comparar ou apurar")
		return exitUsage
	}
	if eo.Out == "" {
		logf("FATAL -out é obrigatório na suíte erros (os relatórios e a linha de base ficam nele)")
		return exitUsage
	}
	if eo.DB == "" {
		logf("FATAL -treino-db é obrigatório (o replay roda no servidor, ao lado do treino.db)")
		return exitUsage
	}
	if _, err := os.Stat(eo.DB); err != nil {
		logf("FATAL treino.db ilegível: %v", errors.Unwrap(err))
		return exitUsage
	}
	var key *hmackey.Key
	if eo.KeyFile != "" {
		k, err := hmackey.Load(eo.KeyFile)
		if err != nil {
			logf("FATAL chave: %v", err)
			return exitUsage
		}
		key = k
	}
	st, err := treino.Open(eo.DB, treinoOptions(key))
	if err != nil {
		logf("FATAL treino.db: %v", err)
		return exitUsage
	}
	defer st.Close()
	ctx := context.Background()

	if eo.Mode == "indice" {
		return freezeIndexMode(ctx, st, eo, logf)
	}
	if eo.Mode == "indices-limpar" { // M-4: the indexes that went invalid are of no use
		n, err := st.DeleteInvalidIndexes(ctx)
		if err != nil {
			logf("FATAL limpeza dos índices: %v", err)
			return exitUsage
		}
		logf("INDICES-LIMPOS removidos=%d", n)
		return exitOK
	}
	if eo.Mode == "retrato" {
		return makeRetrato(ctx, st, eo, logf)
	}
	if eo.Mode == "apurar" {
		return errosApurar(ctx, st, eo, logf)
	}
	if eo.RetratoID == "" {
		logf("FATAL -retrato é obrigatório nos modos base e comparar (C-4.14: um retrato por avaliação)")
		return exitUsage
	}
	if !validID(eo.RetratoID) {
		logf("FATAL -retrato inválido")
		return exitUsage
	}
	if eo.Mode == "comparar" && eo.Candidato == "" {
		logf("FATAL -candidato é obrigatório no modo comparar")
		return exitUsage
	}
	if eo.Alvo != "" && categoryMode(eo.Alvo) == modeOutside {
		logf("FATAL -alvo %q não tem pontuação (C-5.7)", eo.Alvo)
		return exitUsage
	}
	outDir := filepath.Join(eo.Out, "erros", eo.RetratoID)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		logf("FATAL pasta de saída: %v", err)
		return exitUsage
	}
	basePath := filepath.Join(outDir, "linha-de-base.json")
	if eo.Mode == "base" {
		if _, err := os.Stat(basePath); err == nil {
			logf("FATAL este retrato já tem linha de base registrada (C-5.14): é a referência histórica e não se sobrescreve; um retrato novo exige uma linha de base nova (C-4.14)")
			return exitUsage
		}
	}
	if eo.Mode == "comparar" {
		// C-4.14 / C-5.14: a comparison is always against the baseline of the SAME retrato
		bf, err := readBaseline(basePath, eo.RetratoID)
		if err != nil {
			logf("FATAL sem linha de base utilizável para este retrato (%v): rode antes o modo base no mesmo retrato (C-4.14, C-5.14)", err)
			return exitUsage
		}
		if bf.Modelo != eo.Atual {
			logf("FATAL a linha de base é do modelo %q e -atual é %q (C-5.14)", bf.Modelo, eo.Atual)
			return exitUsage
		}
	}
	rt, cases, err := st.RetratoCases(ctx, eo.RetratoID)
	if errors.Is(err, treino.ErrRetratoNotFound) {
		logf("FATAL retrato inexistente")
		return exitUsage
	}
	if errors.Is(err, treino.ErrRetratoInvalido) {
		// C-5.13: an invalid retrato is never used; the gate fails
		in := reportInput{Mode: eo.Mode, Retrato: rt, RetratoID: eo.RetratoID, Atual: eo.Atual, Candidato: eo.Candidato, Alvo: eo.Alvo,
			GateExtra: []string{"o retrato está inválido (valido=0, C-5.13): " + err.Error()}}
		return finish(eo, outDir, in, nil, nil, logf)
	}
	if err != nil {
		logf("FATAL retrato: %v", err)
		return exitUsage
	}
	for _, c := range cases { // the recorded model has to be the current one
		var m struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(c.PromptJSON, &m)
		if m.Model != eo.Atual {
			logf("FATAL o caso %s foi gravado com o modelo %q e -atual é %q: o modelo atual é medido com o pedido gravado, então os dois têm de coincidir", c.OcorrenciaID, m.Model, eo.Atual)
			return exitUsage
		}
	}
	rc, err := newReplayClient(eo.GatewayURL, eo.AllowRemote, eo.Timeout)
	if err != nil {
		logf("FATAL %v", err)
		return exitUsage
	}
	if eo.RetryWait > 0 {
		rc.retryWait = eo.RetryWait
	}
	// 12.8: the candidate is the current model with the instructions and examples the
	// retrato records, built by the shared code of the gateway. What the retrato records
	// and what was given must match, or the comparison is refused (C-8.5, C-8.7).
	indexID := ""
	if eo.Mode == "comparar" {
		tr, idx, err := prepareCandidate(ctx, st, eo, rt, rc)
		if err != nil {
			logf("FATAL %v", err)
			return exitUsage
		}
		if tr != nil && eo.Transform == nil {
			eo.Transform = tr
		}
		indexID = idx
	}
	if eo.Transform == nil {
		eo.Transform = identityTransform{}
	}
	logf("ERROS modo=%s retrato=%s casos=%d atual=%s candidato=%s", eo.Mode, rt.ID, len(cases), eo.Atual, eo.Candidato)

	// ---- the replay: one call at a time, the two models interleaved (C-5.15) ----
	cats := map[string]*catRes{}
	cat := func(c string) *catRes {
		if cats[c] == nil {
			cats[c] = &catRes{Cat: c, Mode: categoryMode(c)}
		}
		return cats[c]
	}
	in := reportInput{Mode: eo.Mode, Retrato: rt, RetratoID: rt.ID, Atual: eo.Atual, Candidato: eo.Candidato, Alvo: eo.Alvo}
	var results []caseRes
	var blind []blindItem
	for _, c := range cases {
		mode := categoryMode(c.Categoria)
		cr := cat(c.Categoria)
		if mode == modeOutside {
			in.Fora++
			continue
		}
		if ok, why := wantApplicable(c.Categoria, c.RespostaCorreta); !ok {
			cr.Inapplicable++ // not replayed, not a model error
			logf("CASO %s categoria=%s inaplicável: %s", c.OcorrenciaID, c.Categoria, why)
			continue
		}
		if mode == modeBlind {
			if eo.Mode != "comparar" {
				cr.Blind++ // counted, measured only in a comparison
				continue
			}
			bi, err := replayBlind(ctx, rc, c, eo.Candidato, eo.Transform)
			if err != nil {
				in.Unmeasured++
				logf("CASO %s categoria=%s não medido: %v", c.OcorrenciaID, c.Categoria, err)
				continue
			}
			blind = append(blind, bi)
			cr.Blind++
			continue
		}
		models := []string{"atual"}
		if eo.Mode == "comparar" {
			models = append(models, "candidato")
		}
		rounds := map[string][]bool{}
		var inapplicable, failed bool
		var failWhy string
	rounds:
		for _, seed := range replaySeeds {
			for _, m := range models { // atual then candidato, same seed
				model, pj := "", c.PromptJSON
				if m == "candidato" {
					model = eo.Candidato
					var terr error
					if pj, terr = eo.Transform.Transform(c, c.PromptJSON); terr != nil {
						failed, failWhy = true, oneLine(terr.Error(), 120)
						break rounds
					}
				}
				body, err := withSeedAndModel(pj, c.Endpoint, seed, model)
				var rp reply
				if err == nil {
					rp, err = rc.do(ctx, c.Endpoint, body)
					if errors.Is(err, errAnswerTooLong) && categoryMode(c.Categoria) == modeTrunc {
						err = nil // m-14: an answer past the ceiling reproves `truncado`; in any other category it is not measured
					}
				}
				if err != nil {
					failed, failWhy = true, oneLine(err.Error(), 120)
					break rounds
				}
				v := scoreReply(c.Categoria, rp, c.RespostaCorreta)
				if !v.Applicable {
					inapplicable = true
					break rounds
				}
				rounds[m] = append(rounds[m], v.Pass)
			}
		}
		switch {
		case failed:
			in.Unmeasured++
			logf("CASO %s categoria=%s não medido: %s", c.OcorrenciaID, c.Categoria, failWhy)
		case inapplicable:
			cr.Inapplicable++
			logf("CASO %s categoria=%s critério inaplicável à resposta certa", c.OcorrenciaID, c.Categoria)
		default:
			results = append(results, caseRes{OcorrenciaID: c.OcorrenciaID, Categoria: c.Categoria, Rounds: rounds})
			for m, r := range rounds {
				ms := &cr.Atual
				if m == "candidato" {
					ms = &cr.Cand
				}
				ms.N++
				if casePasses(r) {
					ms.Pass++
				}
				if roundsDiffer(r) {
					ms.Changed++
				}
			}
		}
	}
	in.Calls = rc.calls
	for _, cr := range cats {
		in.Cats = append(in.Cats, *cr)
	}
	sort.Slice(in.Cats, func(i, j int) bool { return in.Cats[i].Cat < in.Cats[j].Cat })
	// The replay takes hours: the retrato is checked again, case by case, before
	// anything is written (an occurrence discarded or edited in the meantime, a
	// quarantine). A retrato that changed or went invalid is no measure at all.
	if why := recheckRetrato(ctx, st, rt, cases); why != "" {
		in.GateExtra = append(in.GateExtra, why)
		in.Retrato.Valido = false
	}
	if why := recheckIndex(ctx, st, indexID, rt.IndiceHash); why != "" {
		in.GateExtra = append(in.GateExtra, why)
		in.Retrato.Valido = false
	}
	in.Calls += rc.embedCalls // the searches of the examples are calls to the gateway too
	return finish(eo, outDir, in, results, blind, logf)
}

// treinoOptions are the options this program opens treino.db with. It writes (the
// retrato) to a file that the gateway keeps open, so its transactions take the
// write lock when they begin (TxLockImmediate); the gateway's own DSN is unchanged.
func treinoOptions(key *hmackey.Key) treino.Options {
	return treino.Options{Key: key, TxLockImmediate: true}
}

// recheckRetrato repeats, after the replay, the check of RetratoCases and compares
// what it returns with what the replay used. It returns the reason, or "".
func recheckRetrato(ctx context.Context, st *treino.Store, rt treino.Retrato, cases []treino.RetratoCase) string {
	const why = "retrato invalidado durante a rodada (C-5.13): a medição não vale"
	rt2, cases2, err := st.RetratoCases(ctx, rt.ID)
	if err != nil {
		return why
	}
	if !rt2.Valido || len(rt2.Itens) != len(rt.Itens) || len(cases2) != len(cases) {
		return why
	}
	for i, c := range cases {
		d := cases2[i]
		if d.OcorrenciaID != c.OcorrenciaID || d.JobID != c.JobID || d.PromptHMAC != c.PromptHMAC || d.Categoria != c.Categoria ||
			d.RespostaCorreta != c.RespostaCorreta || !bytes.Equal(d.PromptJSON, c.PromptJSON) {
			return why
		}
	}
	return ""
}

// readBaseline reads the registered baseline of a retrato.
func readBaseline(path, retrato string) (*baselineFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("arquivo ausente")
	}
	var bf baselineFile
	if err := json.Unmarshal(b, &bf); err != nil {
		return nil, errors.New("arquivo ilegível")
	}
	if bf.RetratoID != retrato {
		return nil, errors.New("é de outro retrato")
	}
	return &bf, nil
}

// requestTransform is the extension point of stage 12.8: it rewrites the recorded
// prompt_json into the request the CANDIDATE receives (frozen instructions and the
// examples of the frozen index), with code shared with mcpollama. It is applied to
// the candidate only; the current model always gets the recorded request as it is.
type requestTransform interface {
	Transform(c treino.RetratoCase, promptJSON []byte) ([]byte, error)
}

// identityTransform is the implementation of stage 12.7: the candidate gets the
// recorded request unchanged (only seed and model differ, C-5.1).
type identityTransform struct{}

func (identityTransform) Transform(_ treino.RetratoCase, pj []byte) ([]byte, error) { return pj, nil }

// validID accepts only the 32 hex digits of a retrato id: it becomes a folder name.
func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func makeRetrato(ctx context.Context, st *treino.Store, eo errosOpts, logf func(string, ...any)) int {
	hashes, err := resolveHashes(ctx, st, eo)
	if err != nil {
		logf("FATAL %v", err)
		return exitUsage
	}
	rt, err := st.CreateRetrato(ctx, hashes)
	if errors.Is(err, treino.ErrNoCases) {
		logf("DADOS INSUFICIENTES: nenhum caso elegível na partição de teste; nenhum retrato foi criado")
		return exitGate
	}
	if err != nil {
		logf("FATAL retrato: %v", err)
		return exitUsage
	}
	n := map[string]int{}
	for _, it := range rt.Itens {
		n[it.Categoria]++
	}
	var names []string
	for c := range n {
		names = append(names, c)
	}
	sort.Strings(names)
	logf("RETRATO id=%s itens=%d indice_hash=%s instrucoes_id=%s instrucoes_hash=%s exemplos_k=%d exemplos_limiar=%.4f valido=1", rt.ID, len(rt.Itens), rt.IndiceHash, rt.InstrucoesID, rt.InstrucoesHash, rt.ExemplosK, rt.ExemplosLimiar)
	for _, c := range names {
		note := ""
		if n[c] < minDecide {
			note = fmt.Sprintf(" (abaixo de %d: ainda não decide)", minDecide)
		}
		logf("RETRATO categoria=%s casos=%d modo=%s%s", c, n[c], categoryMode(c), note)
	}
	return exitOK
}

// ---- blind (C-5.5, C-5.6) ---------------------------------------------------------------------------

type blindItem struct {
	OcorrenciaID, Categoria string
	Prompt                  string // text of the request, for the owner to judge
	Atual, Cand             string
}

// replayBlind runs the current model and then the candidate, one round (seed=1,
// C-5.15), and keeps both texts for the owner (never for anybody else, C-5.6).
func replayBlind(ctx context.Context, rc *replayClient, c treino.RetratoCase, candidate string, tr requestTransform) (blindItem, error) {
	bi := blindItem{OcorrenciaID: c.OcorrenciaID, Categoria: c.Categoria, Prompt: promptText(c.PromptJSON)}
	for _, model := range []string{"", candidate} { // "" = the model recorded in the request (the current one)
		pj := c.PromptJSON
		if model != "" {
			var err error
			if pj, err = tr.Transform(c, c.PromptJSON); err != nil {
				return bi, err
			}
		}
		body, err := withSeedAndModel(pj, c.Endpoint, 1, model)
		if err != nil {
			return bi, err
		}
		r, err := rc.do(ctx, c.Endpoint, body)
		if err != nil {
			return bi, err
		}
		if model == "" {
			bi.Atual = r.Text
		} else {
			bi.Cand = r.Text
		}
	}
	return bi, nil
}

// promptText flattens a request for the human reader: system, prompt or messages.
func promptText(pj []byte) string {
	var b struct {
		System   string `json:"system"`
		Prompt   string `json:"prompt"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(pj, &b) != nil {
		return ""
	}
	var sb strings.Builder
	if b.System != "" {
		sb.WriteString("[system]\n" + b.System + "\n")
	}
	if b.Prompt != "" {
		sb.WriteString("[prompt]\n" + b.Prompt + "\n")
	}
	for _, m := range b.Messages {
		sb.WriteString("[" + m.Role + "]\n" + m.Content + "\n")
	}
	return sb.String()
}
