package melhora

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"time"
)

// EmbedFunc makes the embedding of a text. The gateway's passes through its own
// queue (embed lane); the bake-off's goes to the local gateway.
type EmbedFunc func(ctx context.Context, text string) ([]float32, error)

// DefaultEmbedModel is the embedding model of the examples (C-6.6).
const DefaultEmbedModel = "nomic-embed-text"

// DefaultEmbedTimeout is how long a request may wait for its query embedding before
// it goes on without examples.
const DefaultEmbedTimeout = 2 * time.Second

// Config configures a Transformer.
type Config struct {
	// Tools the improvement is on for (C-8.8); empty means off for all. Only names in
	// melhora.Tools count.
	Tools []string
	// Instr is the version of the instructions; nil or empty adds nothing.
	Instr *Instrucoes
	// Embed makes the query embedding; nil disables the examples.
	Embed EmbedFunc
	// EmbedTimeout bounds the embedding of a query (default DefaultEmbedTimeout).
	EmbedTimeout time.Duration
}

// Outcome says what Transform did to one request. It never carries text.
type Outcome struct {
	Instrucao   bool // an instruction was added
	Exemplos    int  // examples added
	EmbedFalhou bool // the search could not be made (failure or too slow): sent without examples
	Pulou       bool // the body was left as it came because its shape cannot be extended
	// The trace of the job log (C-2.10a), set only when the request was changed: the version of the
	// instructions that were added, the index the examples came from, and the occurrences shown.
	InstrucoesID string
	IndiceID     string
	ExemploIDs   []string
}

// Stats are the counters and the identity of what is active, for the monitor. No text.
type Stats struct {
	Ligado        bool     `json:"ligado"`
	Tools         []string `json:"tools"`
	InstrucoesID  string   `json:"instrucoes_id"`
	InstrucoesH   string   `json:"instrucoes_hash"`
	IndiceID      string   `json:"indice_id"`
	IndiceHash    string   `json:"indice_hash"`
	IndiceValido  bool     `json:"indice_valido"`
	K             int      `json:"exemplos_k"`
	Limiar        float64  `json:"exemplos_limiar"`
	Pedidos       int64    `json:"pedidos"`          // requests the improvement looked at
	Alterados     int64    `json:"alterados"`        // requests that were changed
	Exemplos      int64    `json:"exemplos"`         // examples injected
	EmbedFalhas   int64    `json:"embed_falhas"`     // searches that could not be made
	EmbedLentos   int64    `json:"embed_lentos"`     // of those, the ones that hit the time limit
	CorpoPulado   int64    `json:"corpo_pulado"`     // bodies left alone
	SemExemplo    int64    `json:"busca_dispensada"` // searches skipped: no example of the tool
	IndiceCaiu    int64    `json:"indice_caiu"`      // times the index was invalidated
	EmbCalculados int64    `json:"emb_calculados"`   // embeddings of public examples computed in the background
	EmbFalhasBg   int64    `json:"emb_bg_falhas"`    // and the ones that failed
}

// Transformer is the improvement. Safe for concurrent use. The gateway builds one
// for production; the bake-off builds one to measure the candidate (C-8.4).
type Transformer struct {
	cfg   Config
	tools map[string]bool
	index atomic.Pointer[Index]

	pedidos, alterados, exemplos, embFalhas, embLentos, pulado, semExemplo, caiu atomic.Int64
	bgOK, bgFail                                                                 atomic.Int64
}

// NewTransformer builds it. The index is set apart, with SetIndex.
func NewTransformer(cfg Config) *Transformer {
	if cfg.EmbedTimeout <= 0 {
		cfg.EmbedTimeout = DefaultEmbedTimeout
	}
	t := &Transformer{cfg: cfg, tools: map[string]bool{}}
	for _, name := range cfg.Tools {
		if IsTool(name) {
			t.tools[name] = true
		}
	}
	return t
}

// SetIndex installs the frozen index served from now on (nil removes it). Swapping
// is the only way to change it (C-8.5).
func (t *Transformer) SetIndex(ix *Index) { t.index.Store(ix) }

// Index is the one installed.
func (t *Transformer) Index() *Index { return t.index.Load() }

// InvalidateIndex is called when the store says an index went invalid (C-4.10): if it
// is the installed one, it stops serving at once.
func (t *Transformer) InvalidateIndex(id string) {
	if ix := t.index.Load(); ix != nil && ix.ID == id {
		ix.Invalidate()
		// the vectors and the text of the examples are let go: a stub with the identity (for the
		// monitor) takes the place, already invalid (M-4 of the review)
		stub := &Index{ID: ix.ID, Hash: ix.Hash, Modelo: ix.Modelo, K: ix.K, Limiar: ix.Limiar}
		stub.invalid.Store(true)
		t.index.CompareAndSwap(ix, stub)
		t.caiu.Add(1)
	}
}

// NoteBackground counts an embedding computed (or failed) by the background worker.
func (t *Transformer) NoteBackground(ok bool) {
	if ok {
		t.bgOK.Add(1)
	} else {
		t.bgFail.Add(1)
	}
}

// Enabled says whether the improvement is on for a tool.
func (t *Transformer) Enabled(tool string) bool { return t != nil && t.tools[tool] }

// Transform returns the body the model should receive. Without the improvement on for
// the tool, with nothing to add, or on any failure, it returns body itself. The search
// of the examples (the query embedding) happens here, BEFORE the caller takes a slot of
// generation, and is bounded by EmbedTimeout: a failure or a slow embedding sends the
// request without examples, counts, and never fails the request.
func (t *Transformer) Transform(ctx context.Context, tool, path string, body []byte, ex Exclusion) (out []byte, oc Outcome) {
	out = body
	if !t.Enabled(tool) {
		return
	}
	defer func() {
		if r := recover(); r != nil { // the improvement must never take a request down
			out, oc = body, Outcome{Pulou: true}
			t.pulado.Add(1)
		}
	}()
	t.pedidos.Add(1)
	instr := t.cfg.Instr.For(tool)
	var examples []Example
	var usedIx *Index // the index the examples came from: the trace names this one even if another is installed meanwhile (m-9)
	if ix := t.index.Load(); ix.Valid() && t.cfg.Embed != nil && supportedPath(path) {
		if ix.HasTool(tool) {
			if text, ok := RequestText(path, body); ok {
				ectx, cancel := context.WithTimeout(ctx, t.cfg.EmbedTimeout)
				vec, err := t.cfg.Embed(ectx, text)
				cancel()
				switch {
				case err != nil:
					t.embFalhas.Add(1)
					if errors.Is(err, context.DeadlineExceeded) || errors.Is(ectx.Err(), context.DeadlineExceeded) {
						t.embLentos.Add(1)
					}
					oc.EmbedFalhou = true
				default:
					usedIx = ix
					examples = ix.Search(tool, vec, ix.K, ix.Limiar, ex)
				}
			}
		} else {
			t.semExemplo.Add(1)
		}
	}
	nonce := NewNonce(examples) // new for every request (C-8.11)
	res, changed, err := Apply(path, body, instr, examples, nonce)
	if err != nil {
		t.pulado.Add(1)
		return body, Outcome{Pulou: true, EmbedFalhou: oc.EmbedFalhou}
	}
	if changed {
		out = res
		oc.Instrucao = instr != ""
		oc.Exemplos = len(examples)
		if oc.Instrucao {
			oc.InstrucoesID = t.cfg.Instr.ID
		}
		if len(examples) > 0 {
			oc.IndiceID = usedIx.ID
			for _, e := range examples {
				oc.ExemploIDs = append(oc.ExemploIDs, e.ID)
			}
		}
		t.alterados.Add(1)
		t.exemplos.Add(int64(len(examples)))
	}
	return
}

// Stats reads the counters and what is active.
func (t *Transformer) Stats() Stats {
	if t == nil {
		return Stats{}
	}
	s := Stats{Ligado: len(t.tools) > 0}
	for name := range t.tools {
		s.Tools = append(s.Tools, name)
	}
	sort.Strings(s.Tools)
	if t.cfg.Instr != nil {
		s.InstrucoesID, s.InstrucoesH = t.cfg.Instr.ID, t.cfg.Instr.Hash
	}
	if ix := t.index.Load(); ix != nil {
		s.IndiceID, s.IndiceHash, s.IndiceValido, s.K, s.Limiar = ix.ID, ix.Hash, ix.Valid(), ix.K, ix.Limiar
	}
	s.Pedidos, s.Alterados, s.Exemplos = t.pedidos.Load(), t.alterados.Load(), t.exemplos.Load()
	s.EmbedFalhas, s.EmbedLentos, s.CorpoPulado = t.embFalhas.Load(), t.embLentos.Load(), t.pulado.Load()
	s.SemExemplo, s.IndiceCaiu = t.semExemplo.Load(), t.caiu.Load()
	s.EmbCalculados, s.EmbFalhasBg = t.bgOK.Load(), t.bgFail.Load()
	return s
}
