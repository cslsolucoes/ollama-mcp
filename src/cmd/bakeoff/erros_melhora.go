package main

// Stage 12.8 in the bake-off (contract C-8.4, C-8.5, C-8.7, C-8.10): the candidate of
// an evaluation is the CURRENT model with the frozen instructions and the examples of
// the frozen index added to its request, by the very code the gateway uses in
// production (package melhora). The current model always gets the recorded request as
// it is (C-5.1). A retrato that records an index or an instruction version can only be
// compared when the index and the file that match those hashes are given; anything
// else is refused, so the measure is always of what the retrato says it is.
//
//	bakeoff -suites erros -erros-modo indice  -treino-db D -exemplos-k 3 -exemplos-limiar 0.80 -out O
//	bakeoff -suites erros -erros-modo retrato -treino-db D -indice ID -instrucoes ARQ -out O
//	bakeoff -suites erros -erros-modo comparar ... -retrato R -indice ID -instrucoes ARQ

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/melhora/feed"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

// Provisional values of k and of the similarity floor: the contract fixes them in 12.8,
// measured by 12.7, and there is no data yet. They are only the default of the flags.
const (
	defaultExemplosK      = 3
	defaultExemplosLimiar = 0.80
)

// melhoraTransform is the requestTransform of stage 12.8.
type melhoraTransform struct {
	t   *melhora.Transformer
	st  *treino.Store
	ctx context.Context
}

// Transform builds the candidate's request. A search that cannot be made fails the
// case ("not measured"): unlike production, a measure must not silently drop the
// examples.
func (m melhoraTransform) Transform(c treino.RetratoCase, pj []byte) ([]byte, error) {
	meta, err := m.st.CaseMeta(m.ctx, c.OcorrenciaID)
	if err != nil {
		return nil, errors.New("metadados do caso ilegíveis")
	}
	// C-8.4: the case itself, its group and its document never serve as an example for it.
	ex := melhora.Exclusion{
		Ocorrencias: []string{c.OcorrenciaID},
		Grupos:      []string{meta.GrupoID},
		Docs:        []string{melhora.DocKey(meta.DocKeyID, meta.DocHash)},
	}
	out, oc := m.t.Transform(m.ctx, c.Tool, c.Endpoint, pj, ex)
	switch {
	case oc.EmbedFalhou:
		return nil, errors.New("a busca de exemplos falhou: o caso não é medido sem os exemplos")
	case oc.Pulou:
		return nil, errors.New("o pedido gravado não aceita as instruções/exemplos")
	}
	return out, nil
}

// memoEmbed caches the embeddings of one run: the three rounds of a case use the same
// text, and an embedding is deterministic.
func memoEmbed(f melhora.EmbedFunc) melhora.EmbedFunc {
	var mu sync.Mutex
	seen := map[[32]byte][]float32{}
	return func(ctx context.Context, text string) ([]float32, error) {
		k := sha256.Sum256([]byte(text))
		mu.Lock()
		v, ok := seen[k]
		mu.Unlock()
		if ok {
			return v, nil
		}
		v, err := f(ctx, text)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		seen[k] = v
		mu.Unlock()
		return v, nil
	}
}

// resolveHashes turns -indice and -instrucoes into what a new retrato records.
func resolveHashes(ctx context.Context, st *treino.Store, eo errosOpts) (treino.RetratoOptions, error) {
	o := eo.Hashes
	if eo.IndiceID != "" {
		if !validID(eo.IndiceID) {
			return o, errors.New("-indice inválido")
		}
		meta, err := st.GetIndexMeta(ctx, eo.IndiceID)
		if err != nil {
			return o, errors.New("índice de exemplos inexistente")
		}
		if !meta.Valido {
			return o, errors.New("o índice de exemplos está inválido (C-4.10)")
		}
		o.IndiceHash, o.ExemplosK, o.ExemplosLimiar = meta.Hash, meta.K, meta.Limiar
	}
	if eo.InstrFile != "" {
		in, err := melhora.LoadInstrucoes(eo.InstrFile)
		if err != nil {
			return o, err
		}
		o.InstrucoesID, o.InstrucoesHash = in.ID, in.Hash
	}
	return o, nil
}

// prepareCandidate checks that what was given matches what the retrato recorded and
// builds the transform. nil transform with nil error means "nothing to add" (the
// retrato records none, and none was given). The returned index id (or "") is the one
// to re-check after the replay.
func prepareCandidate(ctx context.Context, st *treino.Store, eo errosOpts, rt treino.Retrato, rc *replayClient) (requestTransform, string, error) {
	hasIdx := rt.IndiceHash != treino.NoneValue
	hasInstr := rt.InstrucoesID != treino.NoneValue || rt.InstrucoesHash != treino.NoneValue
	if hasInstr && (rt.InstrucoesID == treino.NoneValue || rt.InstrucoesHash == treino.NoneValue) {
		return nil, "", errors.New("o retrato registra só uma parte da versão das instruções (id ou hash): inconsistente (C-8.7)")
	}
	switch {
	case !hasIdx && eo.IndiceID != "":
		return nil, "", errors.New("-indice foi dado, mas o retrato não registra índice de exemplos: um retrato por avaliação (C-4.13, C-8.5)")
	case !hasInstr && eo.InstrFile != "":
		return nil, "", errors.New("-instrucoes foi dado, mas o retrato não registra versão das instruções (C-4.13, C-8.7)")
	case hasIdx && eo.IndiceID == "":
		return nil, "", fmt.Errorf("o retrato registra o índice de exemplos %s: passe -indice com o índice congelado que confere com esse hash (C-8.5)", rt.IndiceHash)
	case hasInstr && eo.InstrFile == "":
		return nil, "", fmt.Errorf("o retrato registra as instruções %s / %s: passe -instrucoes com o arquivo dessa versão (C-8.7)", rt.InstrucoesID, rt.InstrucoesHash)
	}
	if !hasIdx && !hasInstr {
		return nil, "", nil
	}
	cfg := melhora.Config{Tools: melhora.Tools}
	idxID := ""
	var ix *melhora.Index
	if hasIdx {
		if !validID(eo.IndiceID) {
			return nil, "", errors.New("-indice inválido")
		}
		var err error
		ix, err = feed.LoadIndex(ctx, st, eo.IndiceID)
		switch {
		case errors.Is(err, treino.ErrIndexNotFound):
			return nil, "", errors.New("índice de exemplos inexistente")
		case errors.Is(err, treino.ErrIndexInvalid):
			return nil, "", errors.New("o índice de exemplos está inválido (C-4.10)")
		case err != nil:
			return nil, "", errors.New("o índice de exemplos não confere com o hash gravado nele")
		}
		if ix.Hash != rt.IndiceHash {
			return nil, "", errors.New("o índice dado não é o do retrato: o hash difere do registrado (C-8.5)")
		}
		if ix.K != rt.ExemplosK || ix.Limiar != rt.ExemplosLimiar {
			return nil, "", errors.New("k ou o limiar do índice diferem dos registrados no retrato (C-8.10)")
		}
		idxID = ix.ID
		cfg.Embed = memoEmbed(func(c context.Context, text string) ([]float32, error) { return rc.embed(c, ix.Modelo, text) })
		cfg.EmbedTimeout = eo.Timeout
	}
	if hasInstr {
		in, err := melhora.LoadInstrucoes(eo.InstrFile)
		if err != nil {
			return nil, "", err
		}
		if in.ID != rt.InstrucoesID || in.Hash != rt.InstrucoesHash {
			return nil, "", errors.New("o arquivo de instruções não é o do retrato: o ID/hash diferem dos registrados (C-8.7)")
		}
		cfg.Instr = in
	}
	t := melhora.NewTransformer(cfg)
	if ix != nil {
		t.SetIndex(ix)
	}
	return melhoraTransform{t: t, st: st, ctx: ctx}, idxID, nil
}

// freezeIndexMode is the mode `indice`: it makes the snapshot of the public examples
// (C-8.5) with k and the floor chosen here (C-8.10), and prints its id and hash.
func freezeIndexMode(ctx context.Context, st *treino.Store, eo errosOpts, logf func(string, ...any)) int {
	k, lim := eo.ExemplosK, eo.ExemplosLimiar
	if k == 0 {
		k = defaultExemplosK
	}
	if lim == 0 {
		lim = defaultExemplosLimiar
	}
	model := eo.EmbedModel
	if model == "" {
		model = melhora.DefaultEmbedModel
	}
	m, err := st.FreezeIndex(ctx, treino.FreezeOptions{K: k, Limiar: lim, Modelo: model, Formato: melhora.FormatVersion})
	switch {
	case errors.Is(err, treino.ErrNoExamples):
		logf("DADOS INSUFICIENTES: nenhum exemplo público confirmado, da partição de treino, com embedding calculado; nenhum índice foi criado")
		return exitGate
	case errors.Is(err, treino.ErrInvalid):
		logf("FATAL -exemplos-k deve estar entre 1 e %d e -exemplos-limiar entre 0 (exclusive) e 1", treino.MaxExamplesK)
		return exitUsage
	case err != nil:
		logf("FATAL índice: %v", err)
		return exitUsage
	}
	logf("INDICE id=%s hash=%s itens=%d k=%d limiar=%.4f modelo=%s formato=%s valido=1", m.ID, m.Hash, m.Itens, m.K, m.Limiar, m.Modelo, m.Formato)
	return exitOK
}

// recheckIndex repeats, after the replay, the validity of the index: a quarantine
// during the round invalidates it, and a measure with an index that went invalid does
// not count (C-4.10). It returns the reason, or "".
func recheckIndex(ctx context.Context, st *treino.Store, id, hash string) string {
	if id == "" {
		return ""
	}
	const why = "o índice de exemplos foi invalidado durante a rodada (C-4.10): a medição não vale"
	m, err := st.GetIndexMeta(ctx, id)
	if err != nil || !m.Valido || m.Hash != hash {
		return why
	}
	return ""
}
