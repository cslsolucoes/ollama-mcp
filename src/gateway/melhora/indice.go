package melhora

import (
	"math"
	"sort"
	"sync/atomic"
)

// Item is one example of a loaded index: who it is, the vector (unit length) and the
// text the model will see.
type Item struct {
	OcorrenciaID string
	Tool         string
	GrupoID      string
	DocKey       string // "<key id>/<document hmac>", "" when it has no document
	Vec          []float32
	Example      Example
}

// Index is a frozen index in memory. It is immutable, except that it can be marked
// invalid (a member stopped being an example, C-4.10): an invalid index serves
// nothing. Safe for concurrent use.
type Index struct {
	ID, Hash, Modelo string
	K                int
	Limiar           float64
	Items            []Item
	dim              int
	byTool           map[string]int
	invalid          atomic.Bool
}

// Exclusion is what a search must leave out (C-8.4): the occurrence under
// evaluation, its group and its document.
type Exclusion struct {
	Ocorrencias []string
	Grupos      []string
	Docs        []string // DocKey values
}

// DocKey is the comparison key of a document: hashes only match under the same key id.
func DocKey(keyID, hash string) string {
	if hash == "" {
		return ""
	}
	return keyID + "/" + hash
}

// NewIndex builds an index from its parts. Vectors are normalized; an item with a
// zero or different-sized vector is dropped.
func NewIndex(id, hash, modelo string, k int, limiar float64, items []Item) *Index {
	ix := &Index{ID: id, Hash: hash, Modelo: modelo, K: k, Limiar: limiar, byTool: map[string]int{}}
	for _, it := range items {
		v, ok := unit(it.Vec)
		if !ok {
			continue
		}
		if ix.dim == 0 {
			ix.dim = len(v)
		}
		if len(v) != ix.dim {
			continue
		}
		it.Vec = v
		ix.Items = append(ix.Items, it)
		ix.byTool[it.Tool]++
	}
	return ix
}

// Invalidate marks the index as not usable.
func (ix *Index) Invalidate() { ix.invalid.Store(true) }

// Valid says whether the index may still serve.
func (ix *Index) Valid() bool { return ix != nil && !ix.invalid.Load() }

// HasTool says whether the index has examples of a tool (no point in embedding a
// query otherwise).
func (ix *Index) HasTool(tool string) bool { return ix != nil && ix.byTool[tool] > 0 }

func unit(v []float32) ([]float32, bool) {
	if len(v) == 0 {
		return nil, false
	}
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		return nil, false
	}
	n := math.Sqrt(s)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out, true
}

// Cosine is the cosine similarity of two vectors of the same size (0 otherwise).
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// Search returns the examples of the tool whose similarity to the query is at least
// limiar, best first, at most k, leaving out what ex names. Ties are broken by
// occurrence id, so the answer is deterministic. Nothing is returned for an invalid
// index, a query of another size, or k < 1.
func (ix *Index) Search(tool string, q []float32, k int, limiar float64, ex Exclusion) []Example {
	if !ix.Valid() || k < 1 || len(q) != ix.dim {
		return nil
	}
	qu, ok := unit(q)
	if !ok {
		return nil
	}
	skipOc, skipGr, skipDoc := set(ex.Ocorrencias), set(ex.Grupos), set(ex.Docs)
	type hit struct {
		score float64
		it    *Item
	}
	var hits []hit
	for i := range ix.Items {
		it := &ix.Items[i]
		if it.Tool != tool || skipOc[it.OcorrenciaID] || (it.GrupoID != "" && skipGr[it.GrupoID]) || (it.DocKey != "" && skipDoc[it.DocKey]) {
			continue
		}
		s := Cosine(qu, it.Vec)
		if s < limiar {
			continue
		}
		hits = append(hits, hit{s, it})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].it.OcorrenciaID < hits[j].it.OcorrenciaID
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	out := make([]Example, len(hits))
	for i, h := range hits {
		out[i] = h.it.Example
		out[i].ID = h.it.OcorrenciaID
	}
	return out
}

func set(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		if s != "" {
			m[s] = true
		}
	}
	return m
}
