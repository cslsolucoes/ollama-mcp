package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/treino"
	"github.com/cslsoftwares/mcpollama/ollama"
)

// Fase 12.5: the reportar_erro tool against a fake gateway (httptest).

const fbToken = "tok-FALLBACK-77aa"

// fbReply is what the fake gateway answers to the n-th report (1-based).
type fbReply func(n int) (status int, body string, hdr map[string]string)

const fbOK = `{"ocorrencia_id":"0123456789abcdef0123456789abcdef","estado":"nova","amostras_copiadas":2,"sem_pedido":0,"jobs_externos":0,"jobs_outra_origem":0}`

func okReply(int) (int, string, map[string]string) { return 200, fbOK, nil }

// fbMock is a fake gateway that also answers /api/generate; it records the raw
// body and the headers of every POST /api/feedback.
type fbMock struct {
	*mock
	fmu   sync.Mutex
	raws  [][]byte
	heads []http.Header
}

func newFBMock(t *testing.T, reply fbReply) *fbMock {
	t.Helper()
	if reply == nil {
		reply = okReply
	}
	f := &fbMock{}
	f.mock = newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/feedback" {
			generateOK(w, r)
			return
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		f.fmu.Lock()
		f.raws = append(f.raws, buf.Bytes())
		f.heads = append(f.heads, r.Header.Clone())
		n := len(f.raws)
		f.fmu.Unlock()
		status, body, hdr := reply(n)
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintln(w, body)
	})
	return f
}

func (f *fbMock) reports() int {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	return len(f.raws)
}

// last returns the n-th (0-based) raw body decoded with the gateway's own strict
// decoder and validated by the gateway's own rules.
func (f *fbMock) report(t *testing.T, n int) treino.Report {
	t.Helper()
	f.fmu.Lock()
	raw := f.raws[n]
	f.fmu.Unlock()
	var rep treino.Report
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		t.Fatalf("the gateway would refuse the body: %v\n%s", err, raw)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("the gateway would refuse the report: %v", err)
	}
	return rep
}

func repArgs(over map[string]any) string {
	m := map[string]any{
		"projeto": "GestorERP", "jobs": "job=" + jobID(1), "categoria": "omitiu", "erro": "pulou a seção 7.2",
		"resposta_correta": "RESPOSTA-CERTA-SECRETA", "correcao_aplicada": "corrigi à mão", "severidade": "media", "origem": "claude",
	}
	for k, v := range over {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func je(id, papel, backend string) jobEntry { return jobEntry{ID: id, Papel: papel, Backend: backend} }

func TestParseJobsArgForms(t *testing.T) {
	a, b := jobID(0xa), jobID(0xb)
	cases := []struct {
		in   string
		want []jobEntry
	}{
		{"job=" + a, []jobEntry{je(a, "unico", "primario")}},
		{a, []jobEntry{je(a, "unico", "primario")}},
		{"job=" + a + "@fallback", []jobEntry{je(a, "unico", "fallback")}},
		{a + "@fallback", []jobEntry{je(a, "unico", "fallback")}},
		{"jobs=map1:" + a + ",final:" + b + "@fallback", []jobEntry{je(a, "map1", "primario"), je(b, "final", "fallback")}},
		{"map1:" + a + ",reduce2:" + b, []jobEntry{je(a, "map1", "primario"), je(b, "reduce2", "primario")}},
		{"[uso] modelo=qwen3-ia prompt_eval_count=10 total_ms=1 fim=stop job=" + a, []jobEntry{je(a, "unico", "primario")}},
		{"[uso] modelo=qwen3-ia partes=2 fim=stop jobs=map1:" + a + ",final:" + b + "\n", []jobEntry{je(a, "map1", "primario"), je(b, "final", "primario")}},
		{"  jobs=map1:" + a + ",map2:" + b + "@fallback  ", []jobEntry{je(a, "map1", "primario"), je(b, "map2", "fallback")}},
	}
	for _, c := range cases {
		got, err := parseJobsArg(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Fatalf("%q:\n got %v\nwant %v", c.in, got, c.want)
		}
	}
	bad := []string{
		"", "   ", "job=abc", "job=" + strings.ToUpper(a), "map1:" + a + "," + b, // bare id in a list
		"[uso] modelo=x total_ms=1 fim=stop", // no job field
		"jobs=zzz:" + a + "," + "final:" + b, // invalid role
		"jobs=map0:" + a + ",final:" + b,
	}
	for _, in := range bad {
		if _, err := parseJobsArg(in); err == nil {
			t.Fatalf("%q must be refused", in)
		}
	}
	var many []string
	for i := 1; i <= 65; i++ {
		many = append(many, fmt.Sprintf("map%d:%s", i, jobID(i)))
	}
	if _, err := parseJobsArg("jobs=" + strings.Join(many, ",")); err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("65 jobs must be refused with the limit in the message: %v", err)
	}
	if got, err := parseJobsArg("jobs=" + strings.Join(many[:64], ",")); err != nil || len(got) != 64 {
		t.Fatalf("64 jobs must pass: %v %d", err, len(got))
	}
}

// The body is exactly the gateway's: its strict decoder and Validate accept it,
// and every field arrives.
func TestReportBodyIsExactlyTheGatewayFormat(t *testing.T) {
	g := newFBMock(t, nil)
	d := g.deps(t, func(o *ollama.Options) { o.StationInfo = "computer=PC1;user=ana;domain=D;mac=AABBCCDDEEFF" })
	res, err := call(t, d, "reportar_erro", repArgs(map[string]any{
		"jobs": "jobs=map1:" + jobID(1) + ",final:" + jobID(2), "job_do_erro": "map1", "origem": "usuario", "severidade": "alta", "categoria": "numeracao_errada",
	}))
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	rep := g.report(t, 0)
	want := treino.Report{
		Projeto: "GestorERP", JobDoErro: jobID(1), Categoria: "numeracao_errada", Descricao: "pulou a seção 7.2",
		RespostaCorreta: "RESPOSTA-CERTA-SECRETA", CorrecaoAplicada: "corrigi à mão", Severidade: "alta", Origem: "usuario",
		Jobs: []treino.JobRef{{ID: jobID(1), Papel: "map1", Backend: "primario"}, {ID: jobID(2), Papel: "final", Backend: "primario"}},
	}
	if fmt.Sprintf("%+v", rep) != fmt.Sprintf("%+v", want) {
		t.Fatalf("body\n got %+v\nwant %+v", rep, want)
	}
	h := g.heads[0]
	if h.Get("Authorization") != "Bearer "+sentinel || h.Get("X-Mcpollama-Station") != "computer=PC1;user=ana;domain=D;mac=AABBCCDDEEFF" ||
		h.Get("Content-Type") != "application/json" {
		t.Fatalf("headers %v", h)
	}
	out := text(t, res)
	if !strings.Contains(out, "0123456789abcdef0123456789abcdef") || !strings.Contains(out, "estado nova") {
		t.Fatalf("result: %q", out)
	}
	for _, secret := range []string{"RESPOSTA-CERTA-SECRETA", "pulou a seção", sentinel} {
		if strings.Contains(out, secret) {
			t.Fatalf("the result echoes %q", secret)
		}
	}
	if u, _ := res.Meta["mcpollama/reportar_erro"].(map[string]any); u["estado"] != "nova" {
		t.Fatalf("meta %+v", res.Meta)
	}
}

// Everything refused before sending: no request reaches the gateway.
func TestReportRefusedBeforeSending(t *testing.T) {
	g := newFBMock(t, nil)
	d := g.deps(t, nil)
	two := "jobs=map1:" + jobID(1) + ",map2:" + jobID(2)
	var many []string
	for i := 1; i <= 65; i++ {
		many = append(many, fmt.Sprintf("map%d:%s", i, jobID(i)))
	}
	cases := map[string]string{
		"categoria fora do enum":           repArgs(map[string]any{"categoria": "burrice"}),
		"severidade fora do enum":          repArgs(map[string]any{"severidade": "gravissima"}),
		"origem automatico":                repArgs(map[string]any{"origem": "automatico"}),
		"origem fora do enum":              repArgs(map[string]any{"origem": "ia"}),
		"erro vazio":                       repArgs(map[string]any{"erro": "  "}),
		"erro ausente":                     repArgs(map[string]any{"erro": nil}),
		"erro acima de 4 KB":               repArgs(map[string]any{"erro": strings.Repeat("x", 4097)}),
		"resposta acima de 128 KB":         repArgs(map[string]any{"resposta_correta": strings.Repeat("x", 128*1024+1)}),
		"correcao acima de 4 KB":           repArgs(map[string]any{"correcao_aplicada": strings.Repeat("x", 4097)}),
		"projeto acima de 128 B":           repArgs(map[string]any{"projeto": strings.Repeat("p", 129)}),
		"projeto ausente":                  repArgs(map[string]any{"projeto": nil}),
		"correcao ausente":                 repArgs(map[string]any{"correcao_aplicada": nil}),
		"65 jobs":                          repArgs(map[string]any{"jobs": "jobs=" + strings.Join(many, ","), "job_do_erro": "map1"}),
		"job_do_erro fora da lista":        repArgs(map[string]any{"jobs": two, "job_do_erro": "map9"}),
		"job_do_erro id de fora":           repArgs(map[string]any{"jobs": two, "job_do_erro": jobID(9)}),
		"sem final e sem job_do_erro":      repArgs(map[string]any{"jobs": two}),
		"jobs sem campo":                   repArgs(map[string]any{"jobs": "[uso] modelo=x total_ms=1"}),
		"id repetido":                      repArgs(map[string]any{"jobs": "jobs=map1:" + jobID(1) + ",map2:" + jobID(1), "job_do_erro": "map1"}),
		"papel repetido (map1 duas vezes)": repArgs(map[string]any{"jobs": "jobs=map1:" + jobID(1) + ",map1:" + jobID(2), "job_do_erro": "map1"}),
		"dois final":                       repArgs(map[string]any{"jobs": "jobs=final:" + jobID(1) + ",final:" + jobID(2)}),
		"ids soltos em lista":              repArgs(map[string]any{"jobs": jobID(1) + "," + jobID(2), "job_do_erro": jobID(1)}),
		"duas linhas [uso]":                repArgs(map[string]any{"jobs": "[uso] a=1 job=" + jobID(1) + "\n[uso] b=2 job=" + jobID(2)}),
		"job= e jobs= juntos":              repArgs(map[string]any{"jobs": "[uso] x=1 job=" + jobID(1) + " jobs=map1:" + jobID(2) + ",final:" + jobID(3)}),
		"unico numa lista":                 repArgs(map[string]any{"jobs": "jobs=unico:" + jobID(1) + ",final:" + jobID(2)}),
		"corpo acima de 256 KB":            repArgs(map[string]any{"resposta_correta": strings.Repeat("\x01", 100000)}),
		"nulo no texto":                    repArgs(map[string]any{"erro": "a\x00b"}),
		"argumento desconhecido":           `{"projeto":"p","jobs":"job=` + jobID(1) + `","categoria":"omitiu","erro":"e","correcao_aplicada":"c","severidade":"baixa","origem":"claude","extra":1}`,
	}
	for name, args := range cases {
		res, err := call(t, d, "reportar_erro", args)
		if err == nil && !res.IsError {
			t.Errorf("%s: must be refused", name)
		}
		if n := g.hits.Load(); n != 0 {
			t.Fatalf("%s: %d request(s) reached the gateway", name, n)
		}
	}
}

func TestJobDoErroDefaultAndRequired(t *testing.T) {
	g := newFBMock(t, nil)
	d := g.deps(t, nil)
	ids := func(n int) string { return jobID(n) }
	three := "jobs=map1:" + ids(1) + ",map2:" + ids(2) + ",final:" + ids(3)
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"final por padrão", map[string]any{"jobs": three}, ids(3)},
		{"unico com um job", map[string]any{"jobs": "job=" + ids(1)}, ids(1)},
		{"papel explícito", map[string]any{"jobs": three, "job_do_erro": "map2"}, ids(2)},
		{"id explícito", map[string]any{"jobs": three, "job_do_erro": ids(1)}, ids(1)},
		{"translate com a parte", map[string]any{"jobs": "jobs=map1:" + ids(4) + ",map2:" + ids(5), "job_do_erro": "map2"}, ids(5)},
		{"unico explícito", map[string]any{"jobs": "job=" + ids(6), "job_do_erro": "unico"}, ids(6)},
	}
	for i, c := range cases {
		if res, err := call(t, d, "reportar_erro", repArgs(c.args)); err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", c.name, err, res)
		}
		if got := g.report(t, i).JobDoErro; got != c.want {
			t.Fatalf("%s: job_do_erro = %s, want %s", c.name, got, c.want)
		}
	}
	// translate (no final) without job_do_erro is refused, and says why
	_, err := call(t, d, "reportar_erro", repArgs(map[string]any{"jobs": "jobs=map1:" + ids(7) + ",map2:" + ids(8)}))
	if err == nil || !strings.Contains(err.Error(), "obrigatório") || !strings.Contains(err.Error(), "map1, map2") {
		t.Fatalf("translate without job_do_erro: %v", err)
	}
	if g.reports() != len(cases) {
		t.Fatalf("reports sent: %d", g.reports())
	}
}

// C-1.11: the report goes to the server of the job_do_erro, with that server's
// own token; the other server never sees it, nor the token.
func TestReportRoutingByBackendOfTheJobInError(t *testing.T) {
	primary := newFBMock(t, nil)
	fb := newFBMock(t, nil)
	d := primary.deps(t, func(o *ollama.Options) { o.FallbackURL = fb.srv.URL; o.FallbackAPIKey = fbToken })

	// job of the primary
	if _, err := call(t, d, "reportar_erro", repArgs(map[string]any{"jobs": "job=" + jobID(1)})); err != nil {
		t.Fatal(err)
	}
	if primary.reports() != 1 || fb.reports() != 0 {
		t.Fatalf("primary job: primary %d, fallback %d", primary.reports(), fb.reports())
	}
	// job of the fallback
	if _, err := call(t, d, "reportar_erro", repArgs(map[string]any{"jobs": "job=" + jobID(2) + "@fallback"})); err != nil {
		t.Fatal(err)
	}
	if primary.reports() != 1 || fb.reports() != 1 {
		t.Fatalf("fallback job: primary %d, fallback %d", primary.reports(), fb.reports())
	}
	if got := fb.report(t, 0).Jobs[0].Backend; got != "fallback" {
		t.Fatalf("backend sent: %s", got)
	}
	// distributed list: the final ran on the fallback, so it goes there
	if _, err := call(t, d, "reportar_erro", repArgs(map[string]any{"jobs": "jobs=map1:" + jobID(3) + ",final:" + jobID(4) + "@fallback"})); err != nil {
		t.Fatal(err)
	}
	if primary.reports() != 1 || fb.reports() != 2 {
		t.Fatalf("distributed list: primary %d, fallback %d", primary.reports(), fb.reports())
	}
	// tokens never cross
	for _, h := range primary.heads {
		if h.Get("Authorization") != "Bearer "+sentinel {
			t.Fatalf("primary got %q", h.Get("Authorization"))
		}
	}
	for _, h := range fb.heads {
		if h.Get("Authorization") != "Bearer "+fbToken {
			t.Fatalf("fallback got %q", h.Get("Authorization"))
		}
	}
	// a primary job whose report is refused does not spill to the fallback
	primary2 := newFBMock(t, func(int) (int, string, map[string]string) {
		return 503, `{"error":"treino.db ocupado"}`, map[string]string{"Retry-After": "5"}
	})
	fb2 := newFBMock(t, nil)
	d2 := primary2.deps(t, func(o *ollama.Options) { o.FallbackURL = fb2.srv.URL; o.FallbackAPIKey = fbToken })
	if _, err := call(t, d2, "reportar_erro", repArgs(nil)); err == nil {
		t.Fatal("503 must be an error")
	}
	if primary2.reports() != 1 || fb2.reports() != 0 {
		t.Fatalf("503 on the primary: primary %d (no retry), fallback %d (none)", primary2.reports(), fb2.reports())
	}
}

func TestReportForFallbackJobWithoutFallbackIsRefusedBeforeSending(t *testing.T) {
	g := newFBMock(t, nil)
	_, err := call(t, g.deps(t, nil), "reportar_erro", repArgs(map[string]any{"jobs": "job=" + jobID(1) + "@fallback"}))
	if err == nil || !strings.Contains(err.Error(), "fallback") || g.hits.Load() != 0 {
		t.Fatalf("err=%v hits=%d", err, g.hits.Load())
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatal("token in the error")
	}
}

func TestReport409RetriesOnceAfterRetryAfter(t *testing.T) {
	var waited []time.Duration
	g := newFBMock(t, func(n int) (int, string, map[string]string) {
		if n == 1 {
			return 409, `{"error":"o job ainda está na fila"}`, map[string]string{"Retry-After": "5"}
		}
		return 200, fbOK, nil
	})
	d := g.deps(t, nil)
	d.feedbackWait = func(ra time.Duration) time.Duration { waited = append(waited, ra); return time.Millisecond }
	res, err := call(t, d, "reportar_erro", repArgs(nil))
	if err != nil || res.IsError || g.reports() != 2 {
		t.Fatalf("%v %+v reports=%d", err, res, g.reports())
	}
	if len(waited) != 1 || waited[0] != 5*time.Second {
		t.Fatalf("Retry-After given to the wait: %v", waited)
	}

	// persistent 409: exactly one retry, then a clear message
	g2 := newFBMock(t, func(int) (int, string, map[string]string) {
		return 409, `{"error":"o job ainda está na fila"}`, map[string]string{"Retry-After": "1"}
	})
	d2 := g2.deps(t, nil)
	d2.feedbackWait = func(time.Duration) time.Duration { return time.Millisecond }
	_, err = call(t, d2, "reportar_erro", repArgs(nil))
	if err == nil || g2.reports() != 2 || !strings.Contains(err.Error(), "mais tarde") {
		t.Fatalf("persistent 409: err=%v reports=%d", err, g2.reports())
	}
}

func TestWaitFeedbackIsCappedAt10s(t *testing.T) {
	var d Deps
	for _, c := range []struct{ in, want time.Duration }{
		{0, 5 * time.Second}, {3 * time.Second, 3 * time.Second}, {10 * time.Second, 10 * time.Second}, {60 * time.Second, 10 * time.Second},
	} {
		if got := d.waitFeedback(c.in); got != c.want {
			t.Fatalf("waitFeedback(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestReportRefusalMessages(t *testing.T) {
	cases := []struct {
		status int
		body   string
		hdr    map[string]string
		want   []string
	}{
		{400, `{"error":"categoria \"x\" inválida"}`, nil, []string{"HTTP 400", "categoria", "nada foi gravado"}},
		{404, `{"error":"job_do_erro não existe neste gateway"}`, nil, []string{"HTTP 404", "não existe", "nada foi gravado"}},
		{503, `{"error":"falha x"}`, nil, []string{"HTTP 503", "desligado ou ocupado", "falha x"}},
		{507, `{"error":"treino.db no teto"}`, nil, []string{"HTTP 507", "cheio", "administrador"}},
		{401, `{"error":"x"}`, nil, []string{"token ausente ou inválido"}},
	}
	for _, c := range cases {
		g := newFBMock(t, func(int) (int, string, map[string]string) { return c.status, c.body, c.hdr })
		d := g.deps(t, nil)
		_, err := call(t, d, "reportar_erro", repArgs(nil))
		if err == nil {
			t.Fatalf("%d: must be an error", c.status)
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%d: %q lacks %q", c.status, err.Error(), w)
			}
		}
		if g.reports() != 1 {
			t.Errorf("%d: %d reports sent, want 1 (no retry)", c.status, g.reports())
		}
		if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "RESPOSTA-CERTA-SECRETA") {
			t.Errorf("%d: secret in the error", c.status)
		}
	}
}

func TestReportSummaryExplainsTheState(t *testing.T) {
	cases := []struct {
		body string
		want []string
	}{
		{`{"ocorrencia_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","estado":"nova","amostras_copiadas":3,"sem_pedido":0,"jobs_externos":0,"jobs_outra_origem":0}`, []string{"ocorrência " + strings.Repeat("a", 32), "estado nova", "administrador", "amostras copiadas: 3"}},
		{`{"ocorrencia_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","estado":"sem_pedido","amostras_copiadas":0,"sem_pedido":1,"jobs_externos":0,"jobs_outra_origem":0}`, []string{"estado sem_pedido", "expirou", "só na estatística"}},
		{`{"ocorrencia_id":"cccccccccccccccccccccccccccccccc","estado":"distribuida","amostras_copiadas":0,"sem_pedido":0,"jobs_externos":2,"jobs_outra_origem":0}`, []string{"estado distribuida", "só na estatística", "2 job(s) de outro servidor"}},
		{`{"ocorrencia_id":"dddddddddddddddddddddddddddddddd","estado":"fallback","amostras_copiadas":0,"sem_pedido":0,"jobs_externos":0,"jobs_outra_origem":0}`, []string{"estado fallback", "fallback", "só na estatística"}},
		{`{"ocorrencia_id":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","estado":"nova","amostras_copiadas":1,"sem_pedido":2,"jobs_externos":0,"jobs_outra_origem":1}`, []string{"1 job(s) de outra estação", "2 job(s) sem conteúdo"}},
	}
	for _, c := range cases {
		body := c.body
		g := newFBMock(t, func(int) (int, string, map[string]string) { return 200, body, nil })
		res, err := call(t, g.deps(t, nil), "reportar_erro", repArgs(nil))
		if err != nil || res.IsError {
			t.Fatal(err)
		}
		for _, w := range c.want {
			if !strings.Contains(text(t, res), w) {
				t.Errorf("%q lacks %q", text(t, res), w)
			}
		}
	}
}

// The tool needs no --allow-root and is not read-only.
func TestReportarErroIsRegisteredWithoutFilesAndNotReadOnly(t *testing.T) {
	e := entry(t, Deps{}, "reportar_erro")
	if e.Def.Annotations == nil || e.Def.Annotations.ReadOnlyHint || e.Def.Annotations.OpenWorldHint {
		t.Fatalf("annotations %+v", e.Def.Annotations)
	}
	props, _ := e.Def.InputSchema["properties"].(schemaMap)
	for _, k := range []string{"categoria", "severidade", "origem"} {
		if p, _ := props[k].(schemaMap); p["enum"] == nil {
			t.Errorf("%s has no enum in the schema", k)
		}
	}
	if origem, _ := props["origem"].(schemaMap); fmt.Sprint(origem["enum"]) != "[claude usuario]" {
		t.Errorf("origem enum: %v", origem["enum"])
	}
}

// M-1: a cancelled context during the wait after a 409 ends the call at once;
// the second request is never sent.
func TestReport409CancelledDuringTheWaitSendsNoSecondRequest(t *testing.T) {
	g := newFBMock(t, func(int) (int, string, map[string]string) {
		return 409, `{"error":"o job ainda está na fila"}`, map[string]string{"Retry-After": "5"}
	})
	d := g.deps(t, nil) // default wait: Retry-After 5 s
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for g.reports() == 0 {
			time.Sleep(2 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := entry(t, d, "reportar_erro").Handler(ctx, json.RawMessage(repArgs(nil)))
	if err == nil || !strings.Contains(err.Error(), "cancelada") {
		t.Fatalf("err = %v", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("the call waited %v after the cancel", took)
	}
	if g.reports() != 1 {
		t.Fatalf("%d requests, want 1", g.reports())
	}
}

// M-3: a bare id (no job=/jobs=) is accepted only as a single, unambiguous job.
func TestBareJobIDIsAcceptedOnlyAlone(t *testing.T) {
	g := newFBMock(t, nil)
	d := g.deps(t, nil)
	if res, err := call(t, d, "reportar_erro", repArgs(map[string]any{"jobs": jobID(5)})); err != nil || res.IsError {
		t.Fatalf("%v", err)
	}
	if rep := g.report(t, 0); len(rep.Jobs) != 1 || rep.Jobs[0].Papel != "unico" {
		t.Fatalf("%+v", rep)
	}
	_, err := call(t, d, "reportar_erro", repArgs(map[string]any{"jobs": jobID(5) + "," + jobID(6), "job_do_erro": jobID(5)}))
	if err == nil || !strings.Contains(err.Error(), "linha [uso]") || g.reports() != 1 {
		t.Fatalf("err=%v reports=%d", err, g.reports())
	}
}
