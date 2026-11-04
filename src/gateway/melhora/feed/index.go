// Package feed connects the pure improvement code (package melhora) to treino.db:
// loading a frozen index into memory and computing, in the background, the
// embeddings of the public examples. It lives apart because treino depends on the
// proxy, and the proxy depends on melhora.
package feed

import (
	"context"
	"errors"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

// ErrIndexFormat: the index was made with another format of the examples (C-8.10a); freeze a new one.
var ErrIndexFormat = errors.New("o índice foi feito com outra versão do formato dos exemplos: congele de novo")

// LoadIndex loads a frozen index of treino.db into memory: the vectors, and the text
// of each example from its occurrence (the request of the sample, the answer the model
// gave and the right answer). ErrIndexInvalid / ErrIndexCorrupt come from the store.
func LoadIndex(ctx context.Context, st *treino.Store, id string) (*melhora.Index, error) {
	d, err := st.LoadIndex(ctx, id)
	if err != nil {
		return nil, err
	}
	items := make([]melhora.Item, 0, len(d.Items))
	for _, it := range d.Items {
		items = append(items, melhora.Item{
			OcorrenciaID: it.OcorrenciaID, Tool: it.Tool, GrupoID: it.GrupoID, DocKey: melhora.DocKey(it.DocKeyID, it.DocHash), Vec: it.Vec,
			Example: melhora.ExampleOf(it.Endpoint, it.PromptJSON, it.Resposta, it.RespostaCorreta),
		})
	}
	if d.Formato != melhora.FormatVersion {
		return nil, ErrIndexFormat
	}
	ix := melhora.NewIndex(d.ID, d.Hash, d.Modelo, d.K, d.Limiar, items)
	if len(ix.Items) != len(d.Items) {
		return nil, treino.ErrIndexCorrupt
	}
	return ix, nil
}
