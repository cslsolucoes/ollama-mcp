package feed

import (
	"context"
	"errors"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

// Embedder computes, in the background, the embedding of the occurrences the admin
// marked as public examples and that are still waiting for it (C-6.6). It goes through
// the EmbedFunc of the gateway, which uses the queue's embed lane and its slots, so
// it never takes a slot of generation. The vector is stored only if the occurrence is
// still the same one that was read (SetEmbedding); an occurrence unmarked, unconfirmed
// or erased in the meantime has no row left and nothing is written.
type Embedder struct {
	Store   *treino.Store
	Embed   melhora.EmbedFunc
	Model   string        // default melhora.DefaultEmbedModel
	Every   time.Duration // period of a pass (default 10 s)
	Timeout time.Duration // per embedding (default 60 s: the model may be loading)
	Batch   int           // pending rows looked at per pass (default 200)
	Retry   time.Duration // wait before trying a failed row again (default 1 min)
	Note    func(ok bool) // optional: counts a success or a failure (Transformer.NoteBackground)
	Logf    func(format string, args ...any)
	Now     func() time.Time

	retryAt map[string]time.Time
}

func (w *Embedder) defaults() {
	if w.Model == "" {
		w.Model = melhora.DefaultEmbedModel
	}
	if w.Every <= 0 {
		w.Every = 10 * time.Second
	}
	if w.Timeout <= 0 {
		w.Timeout = 60 * time.Second
	}
	if w.Batch <= 0 {
		w.Batch = 200
	}
	if w.Retry <= 0 {
		w.Retry = time.Minute
	}
	if w.Now == nil {
		w.Now = time.Now
	}
	if w.retryAt == nil {
		w.retryAt = map[string]time.Time{}
	}
}

func (w *Embedder) logf(format string, a ...any) {
	if w.Logf != nil {
		w.Logf(format, a...)
	}
}

// Pass makes one round over the pending rows and returns how many it filled. A row
// that fails (no request to embed, the embedding failed) is tried again after Retry.
// Logs carry only ids and error classes, never text.
func (w *Embedder) Pass(ctx context.Context) int {
	w.defaults()
	done := 0
	var cursor int64
	for { // pages by cursor: rows that fail for now or for good never hold the head of the queue (M-2)
		pend, err := w.Store.PendingEmbeddingsAfter(ctx, cursor, w.Batch)
		if err != nil {
			w.logf("melhora: embeddings pendentes: %v", err)
			return done
		}
		if len(pend) == 0 {
			return done
		}
		done += w.page(ctx, pend)
		cursor = pend[len(pend)-1].Rowid
		if ctx.Err() != nil || len(pend) < w.Batch {
			return done
		}
	}
}

func (w *Embedder) page(ctx context.Context, pend []treino.PendingEmb) int {
	done := 0
	now := w.Now()
	for _, p := range pend {
		if ctx.Err() != nil {
			return done
		}
		if at, ok := w.retryAt[p.OcorrenciaID]; ok && now.Before(at) {
			continue
		}
		fail := func() {
			w.retryAt[p.OcorrenciaID] = w.Now().Add(w.Retry)
			if w.Note != nil {
				w.Note(false)
			}
		}
		src, err := w.Store.EmbeddingSource(ctx, p.OcorrenciaID)
		if errors.Is(err, treino.ErrNoSample) {
			// a request that is not there will not appear: the row is marked and leaves the queue
			_ = w.Store.MarkEmbeddingFailed(ctx, p.OcorrenciaID, p.Version, "sem_pedido")
			continue
		}
		if err != nil {
			fail()
			continue
		}
		text, ok := melhora.RequestText(src.Endpoint, src.PromptJSON)
		if !ok {
			_ = w.Store.MarkEmbeddingFailed(ctx, p.OcorrenciaID, src.Version, "sem_texto")
			continue
		}
		ectx, cancel := context.WithTimeout(ctx, w.Timeout)
		vec, err := w.Embed(ectx, text)
		cancel()
		if err != nil || len(vec) == 0 {
			w.logf("melhora: embedding da ocorrência %s falhou (%s)", p.OcorrenciaID, errClass(err))
			fail()
			continue
		}
		// src.Version is the version read together with the request: if the occurrence
		// changed while the vector was being made, SetEmbedding refuses it.
		switch err := w.Store.SetEmbedding(ctx, p.OcorrenciaID, src.Version, vec, w.Model); {
		case err == nil:
			delete(w.retryAt, p.OcorrenciaID)
			done++
			if w.Note != nil {
				w.Note(true)
			}
		case errors.Is(err, treino.ErrStaleEmbed):
			// changed in the meantime: the next pass reads it again
		default:
			w.logf("melhora: gravar o embedding da ocorrência %s: %v", p.OcorrenciaID, err)
			fail()
		}
	}
	return done
}

func errClass(err error) string {
	switch {
	case err == nil:
		return "vazio"
	case errors.Is(err, context.DeadlineExceeded):
		return "tempo"
	case errors.Is(err, context.Canceled):
		return "cancelado"
	}
	return "erro"
}

// Run does a pass every Every until ctx ends.
func (w *Embedder) Run(ctx context.Context) {
	w.defaults()
	t := time.NewTicker(w.Every)
	defer t.Stop()
	for {
		w.Pass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
