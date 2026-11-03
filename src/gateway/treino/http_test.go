package treino

// Fase 12, etapa 12.4: POST /api/feedback (D12-5, §8.3).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newHandler(t *testing.T, role string) (*Handler, *Store, *fakeSrc) {
	t.Helper()
	s := openStore(t, Options{})
	f := newFake()
	for i := 1; i <= 3; i++ {
		f.add(i)
	}
	return &Handler{Store: s, Source: f, Key: testKey(t), Role: role}, s, f
}

func do(h http.Handler, method, body string, hdr map[string]string) *httptest.ResponseRecorder {
	return doCtx(context.Background(), h, method, body, hdr)
}

func doCtx(ctx context.Context, h http.Handler, method, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/feedback", strings.NewReader(body)).WithContext(ctx)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Real-Ip", "10.1.2.3")
	if _, ok := hdr["Content-Type"]; !ok {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func goodBody(mut func(map[string]any)) string {
	m := map[string]any{
		"projeto": "GestorERP", "categoria": "numeracao_errada", "descricao": "pulou a secao 7.2", "severidade": "media", "origem": "claude",
		"jobs": []map[string]string{{"id": mkID(1), "papel": "map1", "backend": "primario"}, {"id": mkID(3), "papel": "final", "backend": "primario"}},
	}
	if mut != nil {
		mut(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func empty(t *testing.T, s *Store) {
	t.Helper()
	for _, tb := range []string{"ocorrencias", "ocorrencia_jobs", "amostras", "historico"} {
		if n := count(t, s, tb); n != 0 {
			t.Errorf("%s has %d rows", tb, n)
		}
	}
}

func errMsg(w *httptest.ResponseRecorder) string {
	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	s, _ := m["error"].(string)
	return s
}

func TestFeedbackAcceptsOnlyPostWithJSON(t *testing.T) {
	h, s, _ := newHandler(t, "")
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		w := do(h, m, goodBody(nil), nil)
		if w.Code != 405 || w.Header().Get("Allow") != "POST" {
			t.Errorf("%s: %d allow=%q", m, w.Code, w.Header().Get("Allow"))
		}
	}
	if w := do(h, "POST", goodBody(nil), map[string]string{"Content-Type": "text/plain"}); w.Code != 415 {
		t.Errorf("text/plain: %d", w.Code)
	}
	if w := do(h, "POST", goodBody(nil), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}); w.Code != 415 {
		t.Errorf("form: %d", w.Code)
	}
	for name, body := range map[string]string{
		"vazio":      ``,
		"nao json":   `isto nao e json`,
		"campo novo": goodBody(func(m map[string]any) { m["extra"] = 1 }),
		"lixo":       goodBody(nil) + ` {"x":1}`,
		"array":      `[1,2]`,
		"tipo":       goodBody(func(m map[string]any) { m["descricao"] = 5 }),
	} {
		if w := do(h, "POST", body, nil); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	empty(t, s)
}

func TestFeedbackRejectsBadValuesWithAClearMessage(t *testing.T) { // C-1.16, C-1.17
	h, s, _ := newHandler(t, "")
	cases := map[string]struct {
		mut  func(map[string]any)
		want string
	}{
		"categoria":  {func(m map[string]any) { m["categoria"] = "bobagem" }, "categoria"},
		"severidade": {func(m map[string]any) { m["severidade"] = "grave" }, "severidade"},
		"origem":     {func(m map[string]any) { m["origem"] = "root" }, "origem"},
		"descricao":  {func(m map[string]any) { m["descricao"] = strings.Repeat("a", MaxDescricao+1) }, "descricao"},
		"resposta":   {func(m map[string]any) { m["resposta_correta"] = strings.Repeat("a", MaxResposta+1) }, "resposta_correta"},
		"jobs 65": {func(m map[string]any) {
			var js []map[string]string
			for i := 1; i <= 65; i++ {
				js = append(js, map[string]string{"id": mkID(i), "papel": fmt.Sprintf("map%d", i), "backend": "primario"})
			}
			m["jobs"] = js
		}, "64"},
		"sem jobs":    {func(m map[string]any) { m["jobs"] = []any{} }, "jobs"},
		"job_do_erro": {func(m map[string]any) { m["job_do_erro"] = mkID(2) }, "job_do_erro"},
	}
	for name, c := range cases {
		w := do(h, "POST", goodBody(c.mut), nil)
		if w.Code != 400 || !strings.Contains(errMsg(w), c.want) {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	empty(t, s)
}

func TestFeedbackBodyLimit(t *testing.T) { // C-1.17: 256 KB
	h, s, _ := newHandler(t, "")
	big := `{"descricao":"` + strings.Repeat("a", MaxBodyBytes) + `"}`
	if w := do(h, "POST", big, nil); w.Code != 413 {
		t.Errorf("body over 256 KB: %d", w.Code)
	}
	// a body that fits but with the biggest allowed fields is accepted
	body := goodBody(func(m map[string]any) {
		m["resposta_correta"] = strings.Repeat("r", MaxResposta)
		m["descricao"] = strings.Repeat("d", MaxDescricao)
	})
	if len(body) >= MaxBodyBytes {
		t.Fatalf("test body too big: %d", len(body))
	}
	if w := do(h, "POST", body, nil); w.Code != 200 {
		t.Errorf("max fields: %d %s", w.Code, w.Body.String())
	}
	_ = s
}

func TestFeedbackAnswerAndStation(t *testing.T) { // §8.3, C-1.18
	h, s, _ := newHandler(t, "")
	w := do(h, "POST", goodBody(func(m map[string]any) { m["resposta_correta"] = "certa" }), map[string]string{
		"X-Mcpollama-Station": "computer=PC01;user=ana;domain=csl;mac=AABBCCDDEEFF", "X-Mcpollama-Client": "PC01/ana"})
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ocorrencia_id", "estado", "amostras_copiadas", "sem_pedido", "jobs_externos", "jobs_outra_origem"} {
		if _, ok := res[k]; !ok {
			t.Errorf("answer lacks %s: %v", k, res)
		}
	}
	if res["estado"] != "nova" || res["amostras_copiadas"].(float64) != 2 || res["jobs_externos"].(float64) != 0 || res["sem_pedido"].(float64) != 0 {
		t.Errorf("answer = %v", res)
	}
	var c, u, d, m, ip, lb, tool, job string
	if err := s.db.QueryRow(`SELECT estacao_computer,estacao_user,estacao_domain,estacao_mac,client_ip,client_label,tool,job_do_erro FROM ocorrencias WHERE id=?`, res["ocorrencia_id"]).
		Scan(&c, &u, &d, &m, &ip, &lb, &tool, &job); err != nil {
		t.Fatal(err)
	}
	if c != "PC01" || u != "ana" || d != "csl" || m != "AABBCCDDEEFF" || ip != "10.1.2.3" || lb != "PC01/ana" || tool != "extract" || job != mkID(3) {
		t.Errorf("origin: %s %s %s %s %s %s %s %s", c, u, d, m, ip, lb, tool, job)
	}
	// a malformed station header is ignored, never a refusal
	if w := do(h, "POST", goodBody(func(m map[string]any) { m["job_do_erro"] = mkID(1) }), map[string]string{"X-Mcpollama-Station": "isso;nao=vale"}); w.Code != 200 {
		t.Errorf("bad station header: %d", w.Code)
	}
}

func TestFeedbackUnknownJobIs404(t *testing.T) { // C-1.4
	h, s, _ := newHandler(t, "")
	w := do(h, "POST", goodBody(func(m map[string]any) {
		m["jobs"] = []map[string]string{{"id": mkID(1), "papel": "map1", "backend": "primario"}, {"id": mkID(99), "papel": "final", "backend": "primario"}}
	}), nil)
	if w.Code != 404 || !strings.Contains(errMsg(w), "job_do_erro") {
		t.Errorf("%d %s", w.Code, w.Body.String())
	}
	empty(t, s)
}

func TestFeedbackWithoutKeyOrDatabaseIs503(t *testing.T) { // C-4.3a
	h, s, _ := newHandler(t, "")
	h.Key = nil
	w := do(h, "POST", goodBody(nil), nil)
	if w.Code != 503 || !strings.Contains(errMsg(w), "--doc-key-file") {
		t.Errorf("no key: %d %s", w.Code, w.Body.String())
	}
	empty(t, s)
	off := &Handler{Source: newFake(), Key: testKey(t)}
	w = do(off, "POST", goodBody(nil), nil)
	if w.Code != 503 || !strings.Contains(errMsg(w), "--treino-db") {
		t.Errorf("no database: %d %s", w.Code, w.Body.String())
	}
}

func TestFeedbackAtTheCapIs507(t *testing.T) { // C-6.2
	h, s, _ := newHandler(t, "")
	if w := do(h, "POST", goodBody(nil), nil); w.Code != 200 {
		t.Fatalf("first: %d", w.Code)
	}
	before := [3]int{count(t, s, "ocorrencias"), count(t, s, "amostras"), count(t, s, "historico")}
	s.opts.MaxBytes = s.Size()
	w := do(h, "POST", goodBody(func(m map[string]any) { m["job_do_erro"] = mkID(1) }), nil)
	if w.Code != 507 || !strings.Contains(errMsg(w), "teto") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	if m["teto_bytes"].(float64) <= 0 || m["tamanho_bytes"].(float64) <= 0 {
		t.Errorf("507 body: %v", m)
	}
	if after := [3]int{count(t, s, "ocorrencias"), count(t, s, "amostras"), count(t, s, "historico")}; after != before {
		t.Errorf("507 changed the data: %v -> %v", before, after)
	}
}

func TestFeedbackRoles(t *testing.T) { // C-1.21
	h, s, _ := newHandler(t, RoleFallback)
	w := do(h, "POST", goodBody(func(m map[string]any) {
		m["jobs"] = []map[string]string{{"id": mkID(1), "papel": "map1", "backend": "fallback"}, {"id": mkID(3), "papel": "final", "backend": "fallback"}}
	}), nil)
	var res Result
	json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != 200 || res.Estado != "fallback" || res.AmostrasCopiadas != 0 || count(t, s, "amostras") != 0 {
		t.Errorf("%d %+v", w.Code, res)
	}
}

// C-6.10: the database work does not use the request's context. A request whose
// context is already cancelled still completes, and the source sees a live
// context with a deadline of its own.
func TestFeedbackUsesItsOwnContext(t *testing.T) {
	h, _, f := newHandler(t, "")
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := doCtx(cctx, h, "POST", goodBody(nil), nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if len(f.ctxSeen) == 0 {
		t.Fatal("the source was not called")
	}
	for i, c := range f.ctxSeen {
		if c.err != nil || !c.hasDeadline {
			t.Errorf("call %d saw err=%v deadline=%v", i, c.err, c.hasDeadline)
		}
	}
}
