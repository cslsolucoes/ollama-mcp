package monitor

// Fase 12.8: the counters of the improvement without training in the state (no text, no
// database read: the state is answered while the database is hung).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func TestStateCarriesTheCountersOfTheImprovementAndNoText(t *testing.T) {
	st, err := store.Open(t.TempDir()+"/gw.db", store.Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := queue.New(queue.Config{OnRecord: st.Record})
	in, _ := melhora.ParseInstrucoes([]byte(`{"tools":{"extract":"TEXTO-SECRETO-DA-INSTRUCAO"}}`))
	tr := melhora.NewTransformer(melhora.Config{Tools: []string{"extract"}, Instr: in})
	tr.SetIndex(melhora.NewIndex("indice-xyz", "hash-xyz", "m", 3, 0.8, []melhora.Item{
		{OcorrenciaID: "a", Tool: "extract", Vec: []float32{1, 0}, Example: melhora.Example{Pedido: "PEDIDO-SECRETO", Errada: "ERRADA-SECRETA", Certa: "CERTA-SECRETA"}}}))
	tr.Transform(t.Context(), "extract", "/api/generate", []byte(`{"prompt":"x"}`), melhora.Exclusion{})
	h, err := New(Config{Queue: q, Store: st, Melhora: tr})
	if err != nil {
		t.Fatal(err)
	}
	s := h.state()
	if s.Melhora == nil || !s.Melhora.Ligado || s.Melhora.InstrucoesID != in.ID || s.Melhora.IndiceID != "indice-xyz" || !s.Melhora.IndiceValido || s.Melhora.Alterados != 1 {
		t.Fatalf("melhora: %+v", s.Melhora)
	}
	b, _ := json.Marshal(s)
	for _, secret := range []string{"TEXTO-SECRETO", "PEDIDO-SECRETO", "ERRADA-SECRETA", "CERTA-SECRETA"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("the state carries %q", secret)
		}
	}
	tr.InvalidateIndex("indice-xyz")
	if s := h.state(); s.Melhora.IndiceValido || s.Melhora.IndiceCaiu != 1 {
		t.Errorf("after the invalidation: %+v", s.Melhora)
	}

	// without a transformer the key is absent
	h2, _ := New(Config{Queue: q, Store: st})
	b2, _ := json.Marshal(h2.state())
	if strings.Contains(string(b2), `"melhora"`) {
		t.Error("the state of a gateway without the feature has the key")
	}
}
