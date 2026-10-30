package monitor

// Regressões da revisão adversarial da etapa 10.L (log de jobs). Cada teste
// falha no código do commit 6cbaf22 e passa depois da correção.

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

const regSentinel = "SENTINELA-REGRESSAO-10L"

func (f *jobFixture) addSized(t *testing.T, id string, bytesIn, respBytes int64) {
	t.Helper()
	now := time.Now()
	r := queue.Record{ID: id, ClientIP: "10.0.0.9", ClientLabel: "cli/1", Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia", Lane: queue.LaneGen,
		PriorityIn: queue.Normal, PriorityFinal: queue.Normal, BytesIn: bytesIn, EnqueuedAt: now, Status: queue.StatusQueued}
	if err := f.st.JobEnqueued(r); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.cs.SavePrompt(id, regSentinel, now); err != nil {
		t.Fatal(err)
	}
	r.Status, r.StartedAt, r.FinishedAt, r.HTTPStatus, r.Responded, r.RespBytes = queue.StatusOK, now, now.Add(time.Second), 200, true, respBytes
	if err := f.st.Finish(r); err != nil {
		t.Fatal(err)
	}
}

// H1: sem sessão, nenhuma rota GET aberta (nem o export sem conteudo=1) traz o
// prompt_sha256; com sessão o detalhe traz.
func TestRegress10LNoPromptHashWithoutSession(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(301)
	f.addSized(t, id, 12345, 4321)
	c, _, _ := f.cs.Get(context.Background(), id)
	for _, p := range []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/jobs", "/monitor/api/jobs/" + id,
		"/monitor/api/jobs/export?formato=jsonl", "/monitor/api/jobs/export?formato=csv", "/monitor/api/stats", "/monitor/api/audit"} {
		b := do(f.h, "GET", p, "", nil).Body.String()
		if strings.Contains(b, "prompt_sha256") || strings.Contains(b, c.PromptSHA256) {
			t.Fatalf("%s sem sessão traz o hash do prompt: %s", p, b)
		}
	}
	// a cross-site request with the cookie is not the administrator either
	cookie, _ := f.login(t)
	if b := do(f.h, "GET", "/monitor/api/jobs/"+id, "", map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "cross-site"}).Body.String(); strings.Contains(b, c.PromptSHA256) {
		t.Fatal("cross-site com cookie traz o hash")
	}
	if b := do(f.h, "GET", "/monitor/api/jobs/"+id, "", map[string]string{"Cookie": cookie}).Body.String(); !strings.Contains(b, c.PromptSHA256) {
		t.Fatalf("o administrador perdeu o hash no detalhe: %s", b)
	}
}

// H2: sem sessão, os tamanhos (bytes_in, resp_bytes) saem arredondados a 1 KiB
// em /state, /history, /jobs, /jobs/{id} e no export sem conteúdo.
func TestRegress10LSizesAreCoarseWithoutSession(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(302)
	f.addSized(t, id, 12345, 4321)
	for _, p := range []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/jobs", "/monitor/api/jobs/" + id, "/monitor/api/jobs/export?formato=jsonl"} {
		b := do(f.h, "GET", p, "", nil).Body.String()
		if strings.Contains(b, `"bytes_in":12345`) || strings.Contains(b, `"resp_bytes":4321`) || strings.Contains(b, `"prompt_bytes"`) {
			t.Fatalf("%s tem tamanho exato: %s", p, b)
		}
		if !strings.Contains(b, `"bytes_in":13312`) || !strings.Contains(b, `"resp_bytes":5120`) {
			t.Fatalf("%s sem o tamanho arredondado: %s", p, b)
		}
	}
	recs, err := csv.NewReader(strings.NewReader(do(f.h, "GET", "/monitor/api/jobs/export?formato=csv", "", nil).Body.String())).ReadAll()
	if err != nil || len(recs) != 2 {
		t.Fatal(err, recs)
	}
	for i, h := range recs[0] {
		if (h == "bytes_enviados" && recs[1][i] != "13312") || (h == "bytes_resposta" && recs[1][i] != "5120") {
			t.Fatalf("csv %s = %s", h, recs[1][i])
		}
	}
	// the queue snapshot (running/waiting) too
	tk, err := f.q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "10.0.0.7", ClientLabel: "x", BytesIn: 777})
	if err != nil {
		t.Fatal(err)
	}
	defer tk.Release(queue.Result{Status: queue.StatusOK})
	if b := do(f.h, "GET", "/monitor/api/state", "", nil).Body.String(); strings.Contains(b, `"bytes_in":777`) || !strings.Contains(b, `"bytes_in":1024`) {
		t.Fatalf("snapshot da fila com tamanho exato: %s", b)
	}
}

// A2: o conteúdo pedido de outro site, mesmo com o cookie, é 403.
func TestRegress10LCrossSiteContentRefused(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(303)
	f.addSized(t, id, 10, 10)
	cookie, _ := f.login(t)
	for _, hd := range []map[string]string{{"Sec-Fetch-Site": "cross-site"}, {"Sec-Fetch-Site": "same-site"}, {"Origin": "https://evil.example"}} {
		hd["Cookie"] = cookie
		for _, p := range []string{"/monitor/api/jobs/" + id + "/content", "/monitor/api/jobs/export?conteudo=1"} {
			if w := do(f.h, "GET", p, "", hd); w.Code != 403 || strings.Contains(w.Body.String(), regSentinel) {
				t.Fatalf("%s %v: %d", p, hd, w.Code)
			}
		}
	}
	// the page itself (same-origin, or a navigation without the header) still works
	if w := do(f.h, "GET", "/monitor/api/jobs/"+id+"/content", "", map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"}); w.Code != 200 {
		t.Fatalf("same-origin: %d", w.Code)
	}
}

// I1: status com dezenas de milhares de itens não vira 500; desconhecido não casa.
func TestRegress10LStatusListBounded(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	f.addSized(t, idN(304), 10, 10)
	if w := do(f.h, "GET", "/monitor/api/jobs?status="+strings.Repeat("ok,", 40000), "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), idN(304)) {
		t.Fatalf("lista grande: %d", w.Code)
	}
	if w := do(f.h, "GET", "/monitor/api/jobs?status="+strings.Repeat("zz,", 40000), "", nil); w.Code != 200 || strings.Contains(w.Body.String(), idN(304)) {
		t.Fatalf("status desconhecido casou: %d %s", w.Code, w.Body)
	}
	if w := do(f.h, "GET", "/monitor/api/jobs/export?status="+strings.Repeat("erro,", 40000), "", nil); w.Code != 200 {
		t.Fatalf("export: %d", w.Code)
	}
}

// I2: uma célula que começa com LF também é neutralizada.
func TestRegress10LCSVLeadingNewline(t *testing.T) {
	for _, s := range []string{"\n=1", "\r=1", "\t=1", "=1", "+1", "-1", "@x"} {
		if got := csvSafe(s); got[0] != '\'' {
			t.Fatalf("%q -> %q", s, got)
		}
	}
	if csvSafe("[user]") != "[user]" || csvSafe("") != "" {
		t.Fatal("csvSafe mexeu no que não devia")
	}
}
