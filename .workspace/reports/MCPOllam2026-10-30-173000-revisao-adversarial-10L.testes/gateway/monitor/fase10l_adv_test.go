package monitor

// Fase 10.L - revisao adversarial do log de jobs (monitor).
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado e FALHA quando o
// defeito existe.

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func vecL(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

const (
	advPrompt = "SENTINELA-PROMPT-10L-4f1e"
	advOdd    = 12345 // bytes_in exato, facil de achar
	advResp   = 4321  // resp_bytes exato
)

// mainMux reproduz o roteamento do entrypoint (main.go): /monitor e /monitor/
// para o monitor, o resto para um "proxy" que so responde 599.
func mainMux(h http.Handler) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("/monitor", h)
	mux.Handle("/monitor/", h)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(599) })
	return httptest.NewServer(mux)
}

// rawGet manda um pedido HTTP cru (o caminho vai exatamente como escrito).
func rawGet(t *testing.T, srv *httptest.Server, method, path, cookie string) (int, string) {
	t.Helper()
	c, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req := method + " " + path + " HTTP/1.1\r\nHost: x\r\nConnection: close\r\n"
	if cookie != "" {
		req += "Cookie: " + cookie + "\r\n"
	}
	req += "\r\n"
	io.WriteString(c, req)
	resp, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: method})
	if err != nil {
		return 0, err.Error()
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, string(b)
}

// addOddJob grava um job com bytes_in e resp_bytes exatos e "estranhos".
func (f *jobFixture) addOddJob(t *testing.T, id string) {
	t.Helper()
	now := time.Now()
	r := queue.Record{ID: id, ClientIP: "10.0.0.9", ClientLabel: "cli/1", Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia", Lane: queue.LaneGen,
		PriorityIn: queue.Normal, PriorityFinal: queue.Normal, BytesIn: advOdd, EnqueuedAt: now, Status: queue.StatusQueued}
	if err := f.st.JobEnqueued(r); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.cs.SavePrompt(id, advPrompt, now); err != nil {
		t.Fatal(err)
	}
	k := f.cs.NewSink(id)
	k.Write("RESPOSTA " + advPrompt)
	k.Finish("")
	r.Status, r.StartedAt, r.FinishedAt, r.RunMS, r.HTTPStatus, r.Responded, r.RespBytes = "ok", now, now.Add(time.Second), 1000, 200, true, advResp
	if err := f.st.Finish(r); err != nil {
		t.Fatal(err)
	}
}

// openGETs sao todas as rotas GET abertas (sem sessao) que carregam dados.
func openGETs(id string) []string {
	return []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/stats", "/monitor/api/audit", "/monitor/api/series",
		"/monitor/api/session", "/monitor/api/jobs", "/monitor/api/jobs/" + id, "/monitor/api/jobs/export?formato=jsonl",
		"/monitor/api/jobs/export?formato=csv", "/monitor"}
}

// H1 (achado do coordenador, revisao automatica de seguranca): GET
// /monitor/api/jobs/{id} sem login devolve content.Info com prompt_sha256 e os
// tamanhos exatos. Quem tem um documento candidato confirma que ele foi enviado.
func TestAdvLHashAndExactSizesOpen(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(41)
	f.addOddJob(t, id)
	c, _, _ := f.cs.Get(context.Background(), id)
	var leaks []string
	for _, p := range openGETs(id) {
		b := do(f.h, "GET", p, "", nil).Body.String()
		// a pagina estatica cita o nome do campo no JS; nela so conta o valor
		if (p != "/monitor" && strings.Contains(b, "prompt_sha256")) || strings.Contains(b, c.PromptSHA256) {
			leaks = append(leaks, p+":sha256")
		}
	}
	if len(leaks) > 0 {
		vecL(t, "H1-hash-aberto", "Media", "gateway/monitor/jobs.go:184", "FALHA: "+strings.Join(leaks, ", "))
		t.Error(leaks)
	} else {
		vecL(t, "H1-hash-aberto", "Media", "gateway/monitor/jobs.go:184", "OK: nenhum prompt_sha256 nas rotas GET abertas nem no export sem conteudo")
	}

	// H2: tamanhos exatos (bytes_in, resp_bytes, prompt_bytes, stored_bytes) nas rotas abertas.
	exact := regexp.MustCompile(fmt.Sprintf(`"(bytes_in|resp_bytes)":(%d|%d)\b|"(prompt_bytes|stored_bytes)"`, advOdd, advResp))
	var sz []string
	for _, p := range openGETs(id) {
		b := do(f.h, "GET", p, "", nil).Body.String()
		if exact.MatchString(b) {
			sz = append(sz, p+":"+exact.FindString(b))
		}
		if strings.Contains(p, "csv") {
			rd := csv.NewReader(strings.NewReader(b))
			recs, _ := rd.ReadAll()
			for _, rec := range recs[1:] {
				for _, cell := range rec {
					if cell == fmt.Sprint(advOdd) || cell == fmt.Sprint(advResp) {
						sz = append(sz, p+":celula "+cell)
					}
				}
			}
		}
	}
	if len(sz) > 0 {
		vecL(t, "H2-tamanho-exato-aberto", "Media", "gateway/monitor/jobs.go:184", "FALHA: "+strings.Join(sz, ", "))
		t.Error(sz)
	} else {
		vecL(t, "H2-tamanho-exato-aberto", "Media", "gateway/monitor/jobs.go:184", "OK: sem sessao os tamanhos saem arredondados a 1 KiB")
	}
}

// V1: variacoes de caminho e de metodo do /content e do export com conteudo=1,
// pelo mesmo roteamento do entrypoint.
func TestAdvLContentPathVariants(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(42)
	f.addOddJob(t, id)
	srv := mainMux(f.h)
	defer srv.Close()
	paths := []struct{ m, p string }{
		{"GET", "/monitor/api/jobs/" + id + "/content"},
		{"GET", "/monitor/api/jobs/" + id + "/content/"},
		{"GET", "/monitor/api/jobs/" + id + "%2Fcontent"},
		{"GET", "/monitor/api/jobs/" + id + "%2fcontent"},
		{"GET", "/monitor/api/jobs/" + strings.ToUpper(id) + "/content"},
		{"GET", "/MONITOR/api/jobs/" + id + "/content"},
		{"GET", "/monitor/API/jobs/" + id + "/content"},
		{"GET", "/monitor/api/jobs/" + id + "/Content"},
		{"GET", "/monitor//api/jobs/" + id + "/content"},
		{"GET", "/monitor/api/jobs/./" + id + "/content"},
		{"GET", "/monitor/api/x/../jobs/" + id + "/content"},
		{"GET", "/monitor/api/jobs/" + id + "/content?x=1"},
		{"GET", "/monitor/api/jobs/" + id + "/content;x"},
		{"GET", "/monitor/api/jobs/" + id + "/content%00"},
		{"HEAD", "/monitor/api/jobs/" + id + "/content"},
		{"OPTIONS", "/monitor/api/jobs/" + id + "/content"},
		{"POST", "/monitor/api/jobs/" + id + "/content"},
		{"GET", "/monitor/api/jobs/export?conteudo=1"},
		{"GET", "/monitor/api/jobs/export?conteudo=1&formato=csv"},
		{"GET", "/monitor/api/jobs/export?conteudo=0&conteudo=1"},
		{"GET", "/monitor/api/jobs/export?conteudo=1&conteudo=0"},
		{"GET", "/monitor/api/jobs/export?conteudo=true"},
		{"GET", "/monitor/api/jobs/export?conteudo=%31"},
		{"GET", "/monitor/api/jobs/export?CONTEUDO=1"},
		{"GET", "/monitor/api/jobs/export?conteudo=1&formato=xml"},
		{"GET", "/monitor/api/jobs/export?conteudo=1&desde=lixo"},
		{"GET", "/monitor/api/jobs/export?conteudo[]=1"},
		{"GET", "/monitor/api/jobs/export?formato=jsonl&conteudo=+1"},
		{"HEAD", "/monitor/api/jobs/export?conteudo=1"},
	}
	var bad []string
	for _, c := range paths {
		code, body := rawGet(t, srv, c.m, c.p, "")
		if strings.Contains(body, advPrompt) || strings.Contains(body, "RESPOSTA") || (code >= 200 && code < 300 && strings.Contains(c.p, "content") && c.m == "GET") {
			bad = append(bad, fmt.Sprintf("%s %s -> %d", c.m, c.p, code))
		}
	}
	if len(bad) > 0 {
		vecL(t, "V1-variantes-caminho", "Alta", "gateway/monitor/monitor.go:221", "FALHA: "+strings.Join(bad, "; "))
		t.Error(bad)
	} else {
		vecL(t, "V1-variantes-caminho", "Alta", "gateway/monitor/monitor.go:221", fmt.Sprintf("OK: %d variantes (caminho, %%2F, maiusculas, HEAD, OPTIONS, POST, conteudo duplicado/estranho) sem texto e sem 2xx", len(paths)))
	}
}

// V2: o SSE (/stream), o /state e o /jobs/{id} nunca carregam o texto, com ou
// sem sessao; erros nao ecoam o prompt.
func TestAdvLNoTextInStreamStateDetail(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(43)
	f.addOddJob(t, id)
	cookie, _ := f.login(t)
	srv := mainMux(f.h)
	defer srv.Close()
	var bad []string
	for _, ck := range []string{"", cookie} {
		req, _ := http.NewRequest("GET", srv.URL+"/monitor/api/stream", nil)
		if ck != "" {
			req.Header.Set("Cookie", ck)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if !strings.Contains(string(b), "data:") {
				bad = append(bad, "stream vazio")
			}
			if strings.Contains(string(b), advPrompt) || strings.Contains(string(b), "RESPOSTA") {
				bad = append(bad, "stream sessao="+fmt.Sprint(ck != ""))
			}
		}
		cancel()
		for _, p := range []string{"/monitor/api/state", "/monitor/api/jobs/" + id, "/monitor/api/jobs?status=" + advPrompt, "/monitor/api/jobs?desde=" + advPrompt,
			"/monitor/api/jobs/" + advPrompt, "/monitor/api/jobs/export?formato=" + advPrompt, "/monitor/api/history?cliente=" + advPrompt} {
			hd := map[string]string{}
			if ck != "" {
				hd["Cookie"] = ck
			}
			b := do(f.h, "GET", p, "", hd).Body.String()
			if strings.Contains(b, advPrompt) || strings.Contains(b, "RESPOSTA") {
				bad = append(bad, p)
			}
		}
	}
	if len(bad) > 0 {
		vecL(t, "V2-sse-state-detalhe", "Alta", "gateway/monitor/monitor.go:349", "FALHA: "+strings.Join(bad, ", "))
		t.Error(bad)
	} else {
		vecL(t, "V2-sse-state-detalhe", "Alta", "gateway/monitor/monitor.go:349", "OK: SSE, /state, /jobs/{id} e erros (id, filtro, formato) sem o texto, com e sem sessao")
	}
}

// A1: sessao expirada, cookie de outro gateway, cookie da sessao encerrada.
func TestAdvLSessionEdges(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	g := newJobFixture(t, time.Minute, true) // "outro gateway"
	id := idN(44)
	f.addOddJob(t, id)
	g.addOddJob(t, id)
	path := "/monitor/api/jobs/" + id + "/content"
	var bad []string
	// cookie valido em g, usado em f
	cg, _ := g.login(t)
	if w := do(f.h, "GET", path, "", map[string]string{"Cookie": cg}); w.Code != 401 {
		bad = append(bad, fmt.Sprintf("cookie de outro gateway: %d", w.Code))
	}
	// sessao expirada
	ck, csrf := f.login(t)
	tok := strings.TrimPrefix(ck, cookieName+"=")
	f.h.sess.mu.Lock()
	f.h.sess.m[tok].exp = time.Now().Add(-time.Second)
	f.h.sess.mu.Unlock()
	if w := do(f.h, "GET", path, "", map[string]string{"Cookie": ck}); w.Code != 401 {
		bad = append(bad, fmt.Sprintf("sessao expirada: %d", w.Code))
	}
	if w := do(f.h, "GET", "/monitor/api/jobs/export?conteudo=1", "", map[string]string{"Cookie": ck}); w.Code != 401 {
		bad = append(bad, fmt.Sprintf("export com sessao expirada: %d", w.Code))
	}
	// sessao encerrada (logout)
	ck2, csrf2 := f.login(t)
	do(f.h, "POST", "/monitor/api/logout", "", map[string]string{"Cookie": ck2, "X-CSRF-Token": csrf2})
	if w := do(f.h, "GET", path, "", map[string]string{"Cookie": ck2}); w.Code != 401 {
		bad = append(bad, fmt.Sprintf("depois do logout: %d", w.Code))
	}
	// o token CSRF nao serve de cookie
	if w := do(f.h, "GET", path, "", map[string]string{"Cookie": cookieName + "=" + csrf}); w.Code != 401 {
		bad = append(bad, fmt.Sprintf("csrf como cookie: %d", w.Code))
	}
	// cookie duplicado: o primeiro invalido, o segundo valido de outro gateway
	if w := do(f.h, "GET", path, "", map[string]string{"Cookie": cookieName + "=x; " + cg}); w.Code != 401 {
		bad = append(bad, fmt.Sprintf("cookie duplicado: %d", w.Code))
	}
	if len(bad) > 0 {
		vecL(t, "A1-sessao", "Alta", "gateway/monitor/jobs.go:193", "FALHA: "+strings.Join(bad, ", "))
		t.Error(bad)
	} else {
		vecL(t, "A1-sessao", "Alta", "gateway/monitor/jobs.go:193", "OK: expirada, de outro gateway, apos logout, csrf como cookie e cookie duplicado: todos 401")
	}
}

// A2: o conteudo pedido de outro site (Sec-Fetch-Site: cross-site) com o cookie
// presente. O SameSite=Strict ja impede o navegador de mandar o cookie; esta e a
// segunda camada, como nos POSTs (sameSite).
func TestAdvLCrossSiteGETContent(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	id := idN(45)
	f.addOddJob(t, id)
	ck, _ := f.login(t)
	w := do(f.h, "GET", "/monitor/api/jobs/"+id+"/content", "", map[string]string{"Cookie": ck, "Sec-Fetch-Site": "cross-site"})
	w2 := do(f.h, "GET", "/monitor/api/jobs/export?conteudo=1", "", map[string]string{"Cookie": ck, "Origin": "https://evil.example"})
	if w.Code == 200 || w2.Code == 200 {
		vecL(t, "A2-get-cross-site", "Baixa", "gateway/monitor/jobs.go:193", fmt.Sprintf("FALHA: cross-site com cookie -> %d / export com Origin alheio -> %d", w.Code, w2.Code))
		t.Fail()
	} else {
		vecL(t, "A2-get-cross-site", "Baixa", "gateway/monitor/jobs.go:193", fmt.Sprintf("OK: %d / %d", w.Code, w2.Code))
	}
}

// A3: ids aleatorios de 128 bits; nenhum POST novo sem CSRF.
func TestAdvLIDsAndNoNewPOST(t *testing.T) {
	seen := map[string]bool{}
	re := regexp.MustCompile(`^[0-9a-f]{32}$`)
	ok := true
	for i := 0; i < 20000; i++ {
		id := queue.NewID()
		if !re.MatchString(id) || seen[id] {
			ok = false
		}
		seen[id] = true
	}
	src, _ := os.ReadFile("monitor.go")
	posts := regexp.MustCompile(`HandleFunc\("POST ([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	var list []string
	for _, p := range posts {
		list = append(list, p[1])
	}
	// Fase 12.6: as 7 rotas novas (acoes do admin sobre o registro de erros e "Marcar erro") foram revisadas:
	// todas passam por authorize (sessao + CSRF + mesma origem), com teste proprio no pacote monitor
	// (TestTreinoRoutesNeedTheAdminSessionCSRFAndSameSite). O vetor continua exigindo a lista exata.
	// Fase 10.5: a 10.4 acrescentou drain/undrain (sessao + CSRF + mesma origem,
	// TestDrainNeedsAdminCSRFAndSameSite e o vetor M1 da 10.5). A lista exata
	// continua sendo a protecao: qualquer POST novo sem revisao faz este vetor falhar.
	want := "/monitor/api/treino/{id}/confirmar /monitor/api/treino/{id}/descartar /monitor/api/treino/{id}/completar /monitor/api/treino/{id}/apagar /monitor/api/treino/{id}/exemplo-publico /monitor/api/treino/{id}/liberar-quarentena /monitor/api/jobs/{id}/marcar-erro /monitor/api/login /monitor/api/logout /monitor/api/queue/{id}/promote /monitor/api/queue/{id}/demote /monitor/api/queue/{id}/remove /monitor/api/running/{id}/cancel /monitor/api/backends/{id}/drain /monitor/api/backends/{id}/undrain"
	f := newJobFixture(t, time.Minute, true)
	ck, _ := f.login(t)
	var csrf []string
	for _, p := range []string{"/monitor/api/jobs", "/monitor/api/jobs/export", "/monitor/api/jobs/" + idN(1), "/monitor/api/jobs/" + idN(1) + "/content"} {
		if w := do(f.h, "POST", p, "{}", map[string]string{"Cookie": ck}); w.Code != 405 {
			csrf = append(csrf, fmt.Sprintf("%s=%d", p, w.Code))
		}
	}
	if !ok || strings.Join(list, " ") != want || len(csrf) > 0 {
		vecL(t, "A3-ids-csrf", "Media", "gateway/monitor/monitor.go:208", fmt.Sprintf("FALHA: ids ok=%v posts=%v nao-405=%v", ok, list, csrf))
		t.Fail()
	} else {
		vecL(t, "A3-ids-csrf", "Media", "gateway/monitor/monitor.go:208", "OK: 20.000 ids hex de 128 bits sem repeticao (crypto/rand); POSTs = lista exata (10.L + drain/undrain da 10.4); POST nas rotas de jobs = 405")
	}
}

// I1: SQL nos filtros e no export; lista de status gigante.
func TestAdvLSQLInjectionFilters(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	for i := 0; i < 3; i++ {
		f.addJob(t, idN(100+i), "cli/1", "p", "ok")
	}
	inj := []string{"'", "' OR 1=1--", "ok') OR ('1'='1", "x\" OR \"1\"=\"1", "erro'); DROP TABLE requests;--", "1; DELETE FROM requests"}
	var bad []string
	for _, s := range inj {
		for _, k := range []string{"status", "cliente", "desde", "ate", "limite", "offset"} {
			for _, p := range []string{"/monitor/api/jobs?", "/monitor/api/jobs/export?formato=jsonl&"} {
				w := do(f.h, "GET", p+k+"="+urlq(s), "", nil)
				if w.Code == 500 {
					bad = append(bad, k+"=500")
				}
				if k == "cliente" || k == "status" {
					var out struct{ Rows []json.RawMessage }
					if strings.HasPrefix(p, "/monitor/api/jobs?") && json.Unmarshal(w.Body.Bytes(), &out) == nil && len(out.Rows) > 0 {
						bad = append(bad, k+"="+s+" devolveu linhas")
					}
				}
			}
		}
	}
	if js, _ := f.st.Jobs(context.Background(), storeQueryAll()); len(js) != 3 {
		bad = append(bad, fmt.Sprintf("tabela alterada: %d linhas", len(js)))
	}
	// status com 40 mil itens: o SQLite tem teto de variaveis (32766)
	big := strings.Repeat("ok,", 40000)
	start := time.Now()
	w := do(f.h, "GET", "/monitor/api/jobs?status="+big, "", nil)
	el := time.Since(start)
	if w.Code == 500 {
		bad = append(bad, fmt.Sprintf("status com 40 mil itens -> 500 em %s", el.Round(time.Millisecond)))
	}
	if len(bad) > 0 {
		vecL(t, "I1-sql-filtros", "Baixa", "gateway/monitor/jobs.go:114", "FALHA: "+strings.Join(bad, ", "))
		t.Error(bad)
	} else {
		vecL(t, "I1-sql-filtros", "Baixa", "gateway/monitor/jobs.go:114", "OK: parametros sempre ligados; status desconhecido ignorado; tabela intacta")
	}
}

// I2: CSV injection, inclusive com tab/CR/LF a frente; JSONL com newline no texto.
func TestAdvLCSVAndJSONL(t *testing.T) {
	f := newJobFixture(t, time.Minute, true)
	evil := []string{"=1+1", "+1", "-1", "@SUM(A1)", "\t=1", "\r=1", "\n=1", " =1", " =1", "=HYPERLINK(\"http://x\")"}
	for i, e := range evil {
		id := idN(200 + i)
		r := queue.Record{ID: id, ClientIP: "10.0.0.9", ClientLabel: e, Tool: e, Endpoint: "/api/generate", Model: e, Lane: queue.LaneGen,
			PriorityIn: queue.Normal, PriorityFinal: queue.Normal, EnqueuedAt: time.Now(), Status: queue.StatusQueued}
		f.st.JobEnqueued(r)
		f.cs.SavePrompt(id, e+"\nlinha2\r\n{\"x\":1}", time.Now())
		k := f.cs.NewSink(id)
		k.Write(e)
		k.Finish("")
	}
	ck, _ := f.login(t)
	var bad []string
	b := do(f.h, "GET", "/monitor/api/jobs/export?formato=csv&conteudo=1", "", map[string]string{"Cookie": ck}).Body.String()
	recs, err := csv.NewReader(strings.NewReader(b)).ReadAll()
	if err != nil {
		bad = append(bad, "csv invalido: "+err.Error())
	}
	for _, rec := range recs {
		for _, cell := range rec {
			if cell != "" && strings.ContainsRune("=+-@\t\r\n", rune(cell[0])) {
				bad = append(bad, fmt.Sprintf("celula %q", cell))
			}
		}
	}
	jb := do(f.h, "GET", "/monitor/api/jobs/export?formato=jsonl&conteudo=1", "", map[string]string{"Cookie": ck}).Body.String()
	lines := strings.Split(strings.TrimSuffix(jb, "\n"), "\n")
	for _, l := range lines {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil {
			bad = append(bad, "linha jsonl quebrada")
			break
		}
	}
	if len(lines) != len(evil) {
		bad = append(bad, fmt.Sprintf("jsonl com %d linhas para %d jobs", len(lines), len(evil)))
	}
	if len(bad) > 0 {
		vecL(t, "I2-csv-jsonl", "Baixa", "gateway/monitor/jobs.go:229", "FALHA: "+strings.Join(bad, ", "))
		t.Error(bad)
	} else {
		vecL(t, "I2-csv-jsonl", "Baixa", "gateway/monitor/jobs.go:229", "OK: nenhuma celula comeca com = + - @ TAB CR LF; JSONL com 1 objeto por linha")
	}
}

// I3: XSS. O index.html nunca usa innerHTML/insertAdjacentHTML/document.write/
// eval, e todo texto vai por textContent (o Playwright do commit cobre o render).
func TestAdvLXSSStatic(t *testing.T) {
	b, _ := os.ReadFile("index.html")
	s := string(b)
	var bad []string
	for _, k := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "setAttribute(\"on", "srcdoc", " onclick=", " onerror=", " onload=", "style=\""} {
		if strings.Contains(s, k) {
			bad = append(bad, k)
		}
	}
	if strings.Contains(s, "\r") {
		bad = append(bad, "CRLF")
	}
	if len(bad) > 0 {
		vecL(t, "I3-xss-estatico", "Alta", "gateway/monitor/index.html", "FALHA: "+strings.Join(bad, ","))
		t.Error(bad)
	} else {
		vecL(t, "I3-xss-estatico", "Alta", "gateway/monitor/index.html", "OK: sem sumidouros de HTML, sem handlers/estilo inline, LF")
	}
}

// D1: export de 7 dias sem conteudo: memoria limitada (pagina de 500).
func TestAdvLExportMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	f := newJobFixture(t, time.Minute, false)
	now := time.Now()
	tx := f.st
	for i := 0; i < 20000; i++ {
		r := queue.Record{ID: fmt.Sprintf("%032x", 900000+i), ClientIP: "10.0.0.9", ClientLabel: "cli/1", Tool: "t", Endpoint: "/api/generate", Model: "m", Lane: queue.LaneGen,
			PriorityIn: queue.Normal, PriorityFinal: queue.Normal, EnqueuedAt: now.Add(-time.Duration(i) * 30 * time.Second), Status: "ok", FinishedAt: now}
		tx.Record(r)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if js, _ := f.st.Jobs(context.Background(), storeQueryAll()); len(js) >= 1000 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(time.Second)
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	srv := mainMux(f.h)
	defer srv.Close()
	stop := make(chan struct{})
	var peak uint64
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.HeapInuse > peak {
				peak = m.HeapInuse
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	start := time.Now()
	resp, err := http.Get(srv.URL + "/monitor/api/jobs/export?formato=csv")
	n := int64(0)
	if err == nil {
		n, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	el := time.Since(start)
	close(stop)
	runtime.ReadMemStats(&m1)
	grow := int64(peak) - int64(m0.HeapInuse)
	res := fmt.Sprintf("20.000 linhas, %d bytes em %s, pico de heap +%.1f MiB", n, el.Round(time.Millisecond), float64(grow)/(1<<20))
	if grow > 64<<20 {
		vecL(t, "D1-export-memoria", "Media", "gateway/monitor/jobs.go:315", "FALHA: "+res)
		t.Error(res)
	} else {
		vecL(t, "D1-export-memoria", "Media", "gateway/monitor/jobs.go:315", "OK: "+res)
	}
}

func urlq(s string) string {
	r := strings.NewReplacer("%", "%25", " ", "%20", "'", "%27", "\"", "%22", ";", "%3B", "=", "%3D", "(", "%28", ")", "%29", "#", "%23", "&", "%26", "+", "%2B", ",", "%2C")
	return r.Replace(s)
}

func storeQueryAll() store.JobQuery { return store.JobQuery{Limit: 1000} }
