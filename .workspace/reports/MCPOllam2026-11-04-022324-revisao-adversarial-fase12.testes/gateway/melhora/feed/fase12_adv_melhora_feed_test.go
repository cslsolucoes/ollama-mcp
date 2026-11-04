package feed

// Revisão adversarial da etapa 12.9 (Fase 12), revisor P: quem pode virar exemplo no pedido
// de OUTRO cliente (C-8.1, C-8.3, C-8.4, C-8.9, C-4.10). Cada vetor imprime uma linha VEC.
//
// Dependências de feed_test.go: env, src, jobID, admin, ctx, fakeEmbed (e o método env.occurrence).

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

func vec12P(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res == "FALHA" {
		t.Errorf("%s: %s", id, desc)
	}
}

// adv12PEnv is newEnv with the store wired to a Transformer, as main.go:164 does.
func adv12PEnv(t *testing.T) (*env, **melhora.Transformer) {
	t.Helper()
	key, err := hmackey.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	var trp *melhora.Transformer
	path := filepath.Join(t.TempDir(), "treino.db")
	st, err := treino.Open(path, treino.Options{Key: key, OnIndexInvalidated: func(id string) {
		if trp != nil {
			trp.InvalidateIndex(id)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close(); st.Close() })
	return &env{t: t, st: st, src: &src{conts: map[string]treino.Content{}}, path: path, raw: raw, key: key}, &trp
}

func adv12PAsk(t *testing.T, tr *melhora.Transformer) (string, melhora.Outcome) {
	t.Helper()
	out, oc := tr.Transform(ctx, "extract", "/api/generate", []byte(`{"model":"qwen3-ia","system":"Você processa arquivos.","prompt":"ALFA documento de OUTRO cliente"}`), melhora.Exclusion{})
	return string(out), oc
}

// P8: at freeze time only confirmed, public, training-part, exportable occurrences enter.
func TestAdv12POnlyConfirmedPublicTrainingEnterTheIndex(t *testing.T) {
	e, _ := adv12PEnv(t)
	ctl := e.occurrence(1, "ALFA controle público", "errCTL", "certaCTL", "treino", true)
	states := map[string]string{} // id -> how it was forced
	nums := map[string]int{}
	mk := func(n int, how, sqlSet string) {
		id := e.occurrence(n, fmt.Sprintf("ALFA SEGREDO-%d", n), fmt.Sprintf("SEGREDO-ERR-%d", n), fmt.Sprintf("SEGREDO-CERTA-%d", n), "treino", true)
		states[id] = how + "|" + sqlSet
		nums[id] = n
	}
	mk(2, "nova", `estado='nova'`)
	mk(3, "descartada", `estado='descartada'`)
	mk(4, "sem_pedido", `estado='sem_pedido'`)
	mk(5, "distribuida", `estado='distribuida'`)
	mk(6, "fallback", `estado='fallback'`)
	mk(7, "quarentena", `particao='quarentena'`)
	mk(8, "teste", `particao='teste'`)
	mk(9, "pendente", `particao=''`)
	mk(10, "apagada", `estado='apagada'`)
	w := &Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}
	if n := w.Pass(ctx); n != 10 {
		t.Fatalf("embedded %d, want 10", n)
	}
	for id, s := range states {
		set := strings.SplitN(s, "|", 2)[1]
		if _, err := e.raw.Exec(`UPDATE ocorrencias SET `+set+` WHERE id=?`, id); err != nil {
			// a CHECK may refuse a state outside the schema: that is already a defence
			states[id] = strings.SplitN(s, "|", 2)[0] + "|RECUSADO pelo esquema: " + err.Error()
		}
	}
	m, err := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 10, Limiar: 0.1, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
	if err != nil {
		t.Fatal(err)
	}
	ix, err := LoadIndex(ctx, e.st, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]bool{}
	for _, it := range ix.Items {
		in[it.OcorrenciaID] = true
	}
	tr := melhora.NewTransformer(melhora.Config{Tools: melhora.Tools, Embed: fakeEmbed(nil, nil)})
	tr.SetIndex(ix)
	out, oc := adv12PAsk(t, tr)
	for id := range in {
		if id != ctl {
			t.Logf("membro extra: %s = %s", id, states[id])
		}
	}
	if !in[ctl] || !strings.Contains(out, "certaCTL") {
		vec12P(t, "P8.controle", "FALHA", fmt.Sprintf("Menor fixture: o controle não virou exemplo (itens %v, %+v)", in, oc))
		return
	}
	vec12P(t, "P8.controle", "OK", "o exemplo público confirmado de treino entra e é servido")
	for id, s := range states {
		how := strings.SplitN(s, "|", 2)[0]
		vid := "P8.congela." + how
		switch {
		case strings.Contains(s, "RECUSADO"):
			vec12P(t, vid, "OK", "estado nem pode ser gravado com exemplo_publico: "+strings.SplitN(s, "|", 2)[1])
		case in[id] || strings.Contains(out, fmt.Sprintf("SEGREDO-CERTA-%d", nums[id])):
			vec12P(t, vid, "FALHA", fmt.Sprintf("Bloqueador gateway/treino/indice.go:229 ocorrência %s (exemplo_publico=1) entrou no índice e no pedido de outro cliente", how))
		default:
			vec12P(t, vid, "OK", fmt.Sprintf("ocorrência %s com exemplo_publico=1 e embedding pronto fica fora do índice congelado", how))
		}
	}
}

// P8: after the index is frozen and serving, every way a member stops being an example
// invalidates it AT ONCE in the running Transformer (OnIndexInvalidated -> InvalidateIndex).
func TestAdv12PMemberLeavesWhileServing(t *testing.T) {
	type act struct {
		id string
		do func(e *env, alvo string) error
	}
	rawThenWrite := func(set string) func(e *env, alvo string) error {
		return func(e *env, alvo string) error {
			// a state reached outside the store's own transitions (e.g. by the recompute of the groups):
			// the sweep of the NEXT write must catch it (treino/indice.go:131)
			if _, err := e.raw.Exec(`UPDATE ocorrencias SET `+set+` WHERE id=?`, alvo); err != nil {
				return err
			}
			e.occurrence(99, "BETA relato qualquer de outra estação", "x", "y", "treino", false)
			return nil
		}
	}
	nova := "nova resposta certa"
	acts := []act{
		{"P8.servindo.descartar", func(e *env, a string) error { return e.st.Discard(ctx, a, admin) }},
		{"P8.servindo.apagar", func(e *env, a string) error { return e.st.Delete(ctx, a, admin) }},
		{"P8.servindo.desmarcar", func(e *env, a string) error { return e.st.SetExemploPublico(ctx, a, false, admin) }},
		{"P8.servindo.editar", func(e *env, a string) error {
			_, err := e.st.Update(ctx, a, treino.Edit{RespostaCorreta: &nova}, admin)
			return err
		}},
		{"P8.servindo.quarentena", rawThenWrite(`particao='quarentena'`)},
		{"P8.servindo.teste", rawThenWrite(`particao='teste'`)},
	}
	for _, a := range acts {
		e, trp := adv12PEnv(t)
		alvo := e.occurrence(1, "ALFA alvo SEGREDO-ALVO", "SEGREDO-ALVO-ERR", "SEGREDO-ALVO-CERTA", "treino", true)
		e.occurrence(2, "ALFA controle", "errB", "certaB", "treino", true)
		(&Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}).Pass(ctx)
		m, err := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 5, Limiar: 0.5, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
		if err != nil {
			t.Fatal(err)
		}
		ix, err := LoadIndex(ctx, e.st, m.ID)
		if err != nil {
			t.Fatal(err)
		}
		tr := melhora.NewTransformer(melhora.Config{Tools: melhora.Tools, Embed: fakeEmbed(nil, nil)})
		tr.SetIndex(ix)
		*trp = tr
		if out, oc := adv12PAsk(t, tr); oc.Exemplos != 2 || !strings.Contains(out, "SEGREDO-ALVO-CERTA") {
			vec12P(t, a.id, "FALHA", fmt.Sprintf("Menor fixture: antes da ação o alvo não era servido (%+v)", oc))
			continue
		}
		if err := a.do(e, alvo); err != nil {
			vec12P(t, a.id, "FALHA", "Menor fixture: a ação falhou: "+err.Error())
			continue
		}
		out, oc := adv12PAsk(t, tr)
		var valido int
		e.raw.QueryRow(`SELECT COALESCE((SELECT valido FROM indices WHERE id=?), -1)`, m.ID).Scan(&valido)
		_, lerr := LoadIndex(ctx, e.st, m.ID)
		switch {
		case strings.Contains(out, "SEGREDO-ALVO"):
			vec12P(t, a.id, "FALHA", fmt.Sprintf("Bloqueador gateway/treino/indice.go:131 depois da ação o exemplo saído continua sendo servido no pedido de outro cliente (indices.valido=%d, exemplos=%d)", valido, oc.Exemplos))
		case oc.Exemplos != 0 || tr.Index().Valid():
			vec12P(t, a.id, "FALHA", fmt.Sprintf("Importante gateway/melhora/transformer.go:105 o índice em memória continua válido (exemplos=%d)", oc.Exemplos))
		case lerr == nil:
			vec12P(t, a.id, "FALHA", "Importante gateway/treino/indice.go:322 o índice ainda carrega na partida seguinte")
		default:
			vec12P(t, a.id, "OK", fmt.Sprintf("índice invalidado na hora em memória (0 exemplos) e no banco (valido=%d, LoadIndex: %v)", valido, lerr))
		}
	}
}

// P8 (C-8.2, by reading): what an example SHOWS of its request is the raw prompt_json of the sample
// (melhora/apply.go:168, texto.go:149), and the admin can edit only resposta_correta, descricao and
// correcao_aplicada (treino/state.go:113): there is no way to anonymize the request text of a real
// occurrence before marking it public. The vector proves that a request marked public reaches another
// client's request verbatim, so "anonimizada" depends wholly on the admin choosing only synthetic ones.
func TestAdv12PPublicExampleShowsTheOriginalRequestVerbatim(t *testing.T) {
	e, _ := adv12PEnv(t)
	const doc = "ALFA CNPJ 12.345.678/0001-90 contrato do cliente Fulano"
	id := e.occurrence(1, doc, "errado", "certo", "treino", true)
	(&Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}).Pass(ctx)
	// the admin "anonymizes" what he can edit
	anon := "resposta anonimizada"
	if err := e.st.SetExemploPublico(ctx, id, false, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Update(ctx, id, treino.Edit{RespostaCorreta: &anon}, admin); err != nil {
		t.Fatal(err)
	}
	if err := e.st.Confirm(ctx, id, admin); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetExemploPublico(ctx, id, true, admin); err != nil {
		t.Fatal(err)
	}
	(&Embedder{Store: e.st, Embed: fakeEmbed(nil, nil)}).Pass(ctx)
	m, err := e.st.FreezeIndex(ctx, treino.FreezeOptions{K: 5, Limiar: 0.5, Modelo: melhora.DefaultEmbedModel, Formato: melhora.FormatVersion})
	if err != nil {
		t.Fatal(err)
	}
	ix, err := LoadIndex(ctx, e.st, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	tr := melhora.NewTransformer(melhora.Config{Tools: melhora.Tools, Embed: fakeEmbed(nil, nil)})
	tr.SetIndex(ix)
	out, oc := adv12PAsk(t, tr)
	if oc.Exemplos == 1 && strings.Contains(out, "12.345.678/0001-90") {
		vec12P(t, "P8.pedidoOriginalNoExemplo", "OK", "MEDIDA (risco de projeto, não defeito de código): o texto do PEDIDO de uma ocorrência real marcada pública vai literal (até 1200 runas) ao pedido de outro cliente; "+
			"o admin só pode editar resposta_correta/descricao/correcao_aplicada (treino/state.go:113), então a C-8.1/C-8.2 depende só de o admin marcar apenas casos sintéticos")
	} else {
		vec12P(t, "P8.pedidoOriginalNoExemplo", "OK", fmt.Sprintf("o pedido original não aparece literal (%+v)", oc))
	}
}
