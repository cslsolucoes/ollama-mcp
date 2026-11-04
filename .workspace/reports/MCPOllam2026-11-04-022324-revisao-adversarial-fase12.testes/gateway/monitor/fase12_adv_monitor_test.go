package monitor

// Fase 12.9 - revisao adversarial da Fase 12, superficie "monitor" (sufixo M):
// aba "Erros (treino)", rotas do admin (12.6 e 12.8), exportacao, "Marcar erro".
// Cada vetor imprime exatamente uma linha VEC|id|OK ou FALHA|descricao; FALHA chama t.Errorf.
//
// Depende de helpers de outros _test.go do pacote: newFixture/do/fixture.login (monitor_test.go),
// tid/tsrc/tfix/newTreinoFixture/tfix.occ/tfix.adm/tfix.count/tfix.openDB/stOrigin (treino_test.go).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

func vec12M(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res != "OK" {
		t.Errorf("%s: %s", id, desc)
	}
}

// ---- helpers ----------------------------------------------------------------------------------------------

type adv12MRoute struct{ method, pattern string }

// adv12MRoutes lists every route registered by the non-test files of the package (go/ast), so a new
// route enters the vectors by itself.
func adv12MRoutes(t *testing.T) []adv12MRoute {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []adv12MRoute
	for _, p := range pkgs {
		for _, f := range p.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) < 2 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle") {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				m, pth, found := strings.Cut(s, " ")
				if !found {
					m, pth = "ANY", s
				}
				out = append(out, adv12MRoute{m, pth})
				return true
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].method+out[i].pattern < out[j].method+out[j].pattern })
	return out
}

func adv12MFill(pattern, id string) string {
	p := strings.ReplaceAll(pattern, "{$}", "")
	return strings.ReplaceAll(p, "{id}", id)
}

func adv12MMerge(base map[string]string, kv ...string) map[string]string {
	m := map[string]string{}
	for k, v := range base {
		m[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == "" {
			delete(m, kv[i])
			continue
		}
		m[kv[i]] = kv[i+1]
	}
	return m
}

// adv12MSnap is the whole persistent state an action could touch: occurrences (estado, particao,
// exemplo), history and the queue.
func (f *tfix) adv12MSnap(t *testing.T) string {
	t.Helper()
	db := f.openDB(t)
	var a, b string
	var nh int
	db.QueryRow(`SELECT COALESCE(group_concat(id||':'||estado||':'||particao||':'||exemplo_publico, ','),'') FROM (SELECT * FROM ocorrencias ORDER BY id)`).Scan(&a)
	db.QueryRow(`SELECT COUNT(*) FROM historico`).Scan(&nh)
	db.QueryRow(`SELECT COALESCE(group_concat(job_id, ','),'') FROM (SELECT job_id FROM amostras ORDER BY job_id)`).Scan(&b)
	q := f.q.Snapshot()
	return fmt.Sprintf("%s|%d|%s|%d|%d", a, nh, b, len(q.Running), len(q.Waiting))
}

// adv12MBodyFor gives a valid body for each POST route, so a refusal is the gate, not the body.
func adv12MBodyFor(pattern string) string {
	switch {
	case strings.HasSuffix(pattern, "/completar"):
		return `{"descricao":"nova descricao"}`
	case strings.HasSuffix(pattern, "/exemplo-publico"):
		return `{"marcar":true}`
	case strings.HasSuffix(pattern, "/liberar-quarentena"):
		return `{"lado":"treino"}`
	case strings.HasSuffix(pattern, "/marcar-erro"):
		return `{"categoria":"omitiu","severidade":"baixa","descricao":"x"}`
	case strings.HasSuffix(pattern, "/login"):
		return `{"user":"admin","password":"errada-mas-longa"}`
	}
	return "{}"
}

// adv12MFlip returns tok with position i replaced by a character that is ALWAYS different.
func adv12MFlip(tok string, i int) string {
	b := []byte(tok)
	if b[i] == 'A' {
		b[i] = 'B'
	} else {
		b[i] = 'A'
	}
	return string(b)
}

// ---- M1: session, CSRF and same-site on every route --------------------------------------------------------

func TestAdv12MM1GatesOnEveryRoute(t *testing.T) {
	f := newTreinoFixture(t)
	occ := f.occ(t, 1, "omitiu", "certa")
	routes := adv12MRoutes(t)
	var posts, gets []adv12MRoute
	for _, r := range routes {
		switch r.method {
		case "POST":
			posts = append(posts, r)
		case "GET":
			gets = append(gets, r)
		default:
			vec12M(t, "M1.0", "FALHA", "Importante monitor.go:228 rota sem metodo explicito: "+r.method+" "+r.pattern)
		}
	}
	idFor := func(p string) string {
		if strings.Contains(p, "/treino/") {
			return occ
		}
		return tid(1)
	}
	hostHdr := map[string]string{"Host": "ia.cslsolucoes.com.br:8443"}
	okHdr := adv12MMerge(hostHdr, "Cookie", f.cookie, "X-CSRF-Token", f.csrf)
	before := f.adv12MSnap(t)

	type probe struct {
		name string
		hdr  map[string]string
		ct   string // "" keeps application/json
		want []int
	}
	var probes []probe
	probes = append(probes,
		probe{"sem cookie", adv12MMerge(hostHdr, "X-CSRF-Token", f.csrf), "", []int{401}},
		probe{"cookie sem CSRF", adv12MMerge(hostHdr, "Cookie", f.cookie), "", []int{403}},
		probe{"cookie de outra sessao inventada", adv12MMerge(okHdr, "Cookie", cookieName+"="+newToken()), "", []int{401}},
	)
	for _, i := range []int{0, len(f.csrf) / 2, len(f.csrf) - 1} {
		probes = append(probes, probe{fmt.Sprintf("CSRF trocado na posicao %d", i), adv12MMerge(okHdr, "X-CSRF-Token", adv12MFlip(f.csrf, i)), "", []int{403}})
	}
	probes = append(probes,
		probe{"CSRF com sufixo", adv12MMerge(okHdr, "X-CSRF-Token", f.csrf+"x"), "", []int{403}},
		probe{"CSRF prefixo", adv12MMerge(okHdr, "X-CSRF-Token", f.csrf[:len(f.csrf)-1]), "", []int{403}},
		probe{"Origin null", adv12MMerge(okHdr, "Origin", "null"), "", []int{403}},
		probe{"Origin sufixo", adv12MMerge(okHdr, "Origin", "https://ia.cslsolucoes.com.br.evil.example"), "", []int{403}},
		probe{"Origin subdominio", adv12MMerge(okHdr, "Origin", "https://x.ia.cslsolucoes.com.br"), "", []int{403}},
		probe{"Origin irmao", adv12MMerge(okHdr, "Origin", "https://evil.cslsolucoes.com.br"), "", []int{403}},
		probe{"Origin prefixo colado", adv12MMerge(okHdr, "Origin", "https://evilia.cslsolucoes.com.br"), "", []int{403}},
		probe{"Sec-Fetch-Site same-site", adv12MMerge(okHdr, "Sec-Fetch-Site", "same-site"), "", []int{403}},
		probe{"Sec-Fetch-Site cross-site", adv12MMerge(okHdr, "Sec-Fetch-Site", "cross-site"), "", []int{403}},
		probe{"form simples sem CSRF (text/plain)", adv12MMerge(hostHdr, "Cookie", f.cookie), "text/plain", []int{403}},
		probe{"form simples sem CSRF (urlencoded)", adv12MMerge(hostHdr, "Cookie", f.cookie), "application/x-www-form-urlencoded", []int{403}},
		probe{"text/plain com CSRF", okHdr, "text/plain", []int{415}},
		probe{"multipart com CSRF", okHdr, "multipart/form-data; boundary=x", []int{415}},
	)
	var bad []string
	nreq := 0
	for _, r := range posts {
		if r.pattern == "/monitor/api/login" {
			continue // the authentication itself (same-site checked by TestAdv12MM1Login below)
		}
		path := adv12MFill(r.pattern, idFor(r.pattern))
		mine := strings.Contains(r.pattern, "/treino/") || strings.HasSuffix(r.pattern, "/marcar-erro")
		for _, p := range probes {
			if p.want[0] == 415 && !mine {
				continue // the 415 probes are authorized requests: only the routes that read a JSON body of this surface
			}
			hdr := p.hdr
			if p.ct != "" {
				hdr = adv12MMerge(hdr, "Content-Type", p.ct)
			}
			w := do(f.h, "POST", path, adv12MBodyFor(r.pattern), hdr)
			nreq++
			okc := false
			for _, c := range p.want {
				okc = okc || w.Code == c
			}
			// the logout without CSRF and the queue actions on an unknown id: only the code matters
			if !okc {
				bad = append(bad, fmt.Sprintf("%s [%s] -> %d", r.pattern, p.name, w.Code))
			}
		}
	}
	after := f.adv12MSnap(t)
	switch {
	case len(posts) < 15:
		vec12M(t, "M1.1", "FALHA", fmt.Sprintf("Menor fixture: so %d rotas POST enumeradas", len(posts)))
	case len(bad) > 0:
		vec12M(t, "M1.1", "FALHA", "Bloqueador monitor.go:845 (authorize) rotas POST sem portao: "+strings.Join(bad, "; "))
	case before != after:
		vec12M(t, "M1.1", "FALHA", "Bloqueador recusas mudaram o estado: "+before+" -> "+after)
	default:
		vec12M(t, "M1.1", "OK", fmt.Sprintf("%d rotas POST (enumeradas por go/ast, inclusive as 7 da 12.6) x %d sondas = %d pedidos: sem cookie 401; sem CSRF/CSRF trocado por caractere sempre diferente/sufixo/prefixo 403; Origin null/sufixo/subdominio/irmao 403; Sec-Fetch-Site same-site e cross-site 403; form simples sem CSRF 403; text/plain e multipart com CSRF 415; estado identico", len(posts), len(probes), nreq))
	}

	// expired session and the cookie after logout
	{
		ck, cs := f.login(t)
		tok := strings.TrimPrefix(ck, cookieName+"=")
		f.h.sess.mu.Lock()
		f.h.sess.m[tok].exp = time.Now().Add(-time.Second)
		f.h.sess.mu.Unlock()
		w1 := do(f.h, "POST", "/monitor/api/treino/"+occ+"/confirmar", "{}", map[string]string{"Cookie": ck, "X-CSRF-Token": cs})
		w1b := do(f.h, "GET", "/monitor/api/treino/export?parte=teste", "", map[string]string{"Cookie": ck})
		ck2, cs2 := f.login(t)
		if w := do(f.h, "POST", "/monitor/api/logout", "", map[string]string{"Cookie": ck2, "X-CSRF-Token": cs2}); w.Code != 200 {
			t.Fatalf("logout: %d", w.Code)
		}
		w2 := do(f.h, "POST", "/monitor/api/treino/"+occ+"/apagar", "{}", map[string]string{"Cookie": ck2, "X-CSRF-Token": cs2})
		w3 := do(f.h, "GET", "/monitor/api/treino/"+occ, "", map[string]string{"Cookie": ck2})
		w4 := do(f.h, "GET", "/monitor/api/treino", "", map[string]string{"Cookie": ck2})
		if w1.Code == 401 && w1b.Code == 401 && w2.Code == 401 && w3.Code == 401 && !strings.Contains(w4.Body.String(), `"rows"`) && f.adv12MSnap(t) == after {
			vec12M(t, "M1.2", "OK", "sessao expirada: POST 401 e export 401; cookie depois do logout: POST apagar 401, detalhe 401, lista volta a visao de visitante; nada mudou")
		} else {
			vec12M(t, "M1.2", "FALHA", fmt.Sprintf("Bloqueador admin.go:181/monitor.go:725 sessao expirada ou apos logout aceita: %d %d %d %d", w1.Code, w1b.Code, w2.Code, w3.Code))
		}
	}

	// GET/HEAD/PUT/DELETE/PATCH on the POST routes
	{
		var wrong []string
		for _, r := range posts {
			path := adv12MFill(r.pattern, idFor(r.pattern))
			for _, m := range []string{"GET", "HEAD", "PUT", "DELETE", "PATCH", "OPTIONS"} {
				if w := do(f.h, m, path, "{}", okHdr); w.Code != 405 {
					wrong = append(wrong, fmt.Sprintf("%s %s -> %d", m, r.pattern, w.Code))
				}
			}
		}
		if len(wrong) == 0 && f.adv12MSnap(t) == after {
			vec12M(t, "M1.3", "OK", fmt.Sprintf("%d rotas POST x GET/HEAD/PUT/DELETE/PATCH/OPTIONS (com cookie e CSRF validos) = 405; nada mudou", len(posts)))
		} else {
			vec12M(t, "M1.3", "FALHA", "Importante monitor.go:247 metodo errado aceito numa rota de acao: "+strings.Join(wrong, "; "))
		}
	}

	// the GET routes that only the administrator may use: export, detail, content of a job
	{
		exp := "/monitor/api/treino/export?parte=teste"
		det := "/monitor/api/treino/" + occ
		type g struct {
			name string
			hdr  map[string]string
			want int
		}
		var wrong []string
		for _, p := range []string{exp, det, "/monitor/api/jobs/" + tid(1) + "/content"} {
			for _, c := range []g{
				{"sem cookie", hostHdr, 401},
				{"cross-site", adv12MMerge(hostHdr, "Cookie", f.cookie, "Sec-Fetch-Site", "cross-site"), 403},
				{"same-site (subdominio irmao, <img>/<a>)", adv12MMerge(hostHdr, "Cookie", f.cookie, "Sec-Fetch-Site", "same-site"), 403},
				{"Origin estranho", adv12MMerge(hostHdr, "Cookie", f.cookie, "Origin", "https://evil.example"), 403},
				{"Origin null", adv12MMerge(hostHdr, "Cookie", f.cookie, "Origin", "null"), 403},
			} {
				if w := do(f.h, "GET", p, "", c.hdr); w.Code != c.want {
					wrong = append(wrong, fmt.Sprintf("%s [%s] -> %d", p, c.name, w.Code))
				}
			}
		}
		// the cookie that backs the "<a>/<img> cross-site" defense
		var cookieOK bool
		w := do(f.h, "POST", "/monitor/api/login", `{"user":"`+adminUser+`","password":"`+adminPass+`"}`, nil)
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName {
				cookieOK = c.SameSite == http.SameSiteStrictMode && c.HttpOnly && c.Secure && c.Path == "/monitor"
			}
		}
		// MEDIDA: an old browser (no Sec-Fetch-Site, no Origin on a GET) with the cookie passes; only SameSite=Strict holds it
		old := do(f.h, "GET", exp, "", adv12MMerge(hostHdr, "Cookie", f.cookie))
		// MEDIDA: Origin of the same host on another port passes the Origin layer (sameSite ignores the port)
		port := do(f.h, "POST", "/monitor/api/treino/"+tid(98)+"/confirmar", "{}", adv12MMerge(okHdr, "Origin", "https://ia.cslsolucoes.com.br:9999"))
		if len(wrong) == 0 && cookieOK {
			vec12M(t, "M1.4", "OK", fmt.Sprintf("export (GET), detalhe e conteudo: sem cookie 401; Sec-Fetch-Site cross-site/same-site e Origin estranho/null 403; cookie SameSite=Strict+HttpOnly+Secure+Path=/monitor. MEDIDA: GET do export com cookie e sem Fetch Metadata (navegador antigo) = %d, so o SameSite=Strict barra o <a>/<img> cross-site; Origin do mesmo host em outra porta passa a camada Origin (POST com CSRF valido = %d, monitor.go:757 compara so o hostname)", old.Code, port.Code))
		} else {
			vec12M(t, "M1.4", "FALHA", fmt.Sprintf("Bloqueador treino.go:138 (requireSessionMsg) GET do admin sem portao: %s; cookieOK=%v", strings.Join(wrong, "; "), cookieOK))
		}
	}
}

// The login itself: same-site and JSON only (it creates the session the other routes trust).
func TestAdv12MM1Login(t *testing.T) {
	f := newTreinoFixture(t)
	body := `{"user":"` + adminUser + `","password":"` + adminPass + `"}`
	var wrong []string
	for name, hdr := range map[string]map[string]string{
		"cross-site":   {"Sec-Fetch-Site": "cross-site"},
		"same-site":    {"Sec-Fetch-Site": "same-site"},
		"Origin null":  {"Origin": "null"},
		"text/plain":   {"Content-Type": "text/plain"},
		"Origin outro": {"Origin": "https://evil.example"},
	} {
		w := do(f.h, "POST", "/monitor/api/login", body, hdr)
		if w.Code == 200 {
			wrong = append(wrong, name)
		}
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName && c.Value != "" {
				wrong = append(wrong, name+" (cookie)")
			}
		}
	}
	if len(wrong) == 0 {
		vec12M(t, "M1.5", "OK", "login cross-site, same-site, Origin null/outro e text/plain recusados sem cookie (login CSRF barrado)")
	} else {
		vec12M(t, "M1.5", "FALHA", "Importante monitor.go:785 login aceito em: "+strings.Join(wrong, ", "))
	}
}

// HEAD on the export: the GET pattern answers HEAD, the handler runs and the server eats the body.
func TestAdv12MM1HeadExport(t *testing.T) {
	f := newTreinoFixture(t)
	for i := 1; i <= 3; i++ {
		id := f.occ(t, i, "omitiu", "certa")
		if w := f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}"); w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino'`)
	srv := httptest.NewServer(f.h)
	defer srv.Close()
	req, _ := http.NewRequest("HEAD", srv.URL+"/monitor/api/treino/export?parte=treino", nil)
	req.Header.Set("Cookie", f.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	n := f.count(t, `SELECT COUNT(*) FROM historico WHERE acao='exportou'`)
	vec12M(t, "M1.6", "OK", fmt.Sprintf("MEDIDA: HEAD /monitor/api/treino/export (com sessao) = %d, %d bytes de corpo entregues e %d linhas `exportou` gravadas (treino.go:154 roda o export num HEAD; write-ahead fica do lado seguro: registro sem entrega, nunca entrega sem registro; HEAD exige a mesma sessao + same-site)", resp.StatusCode, len(b), n))
}

// ---- M2: what a visitor (no session) sees -------------------------------------------------------------------

const adv12MJob = "deadbeefcafebabe0123456789abcdef"

func TestAdv12MM2VisitorLeaks(t *testing.T) {
	f := newTreinoFixture(t)
	f.src.mu.Lock()
	f.src.jobs[adv12MJob] = treino.Job{Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia", Think: "off", Status: "ok", HTTPStatus: 200,
		StationComputer: "CANARIOPC", StationUser: "CANARIOUSER", ClientIP: "10.0.0.7"}
	f.src.conts[adv12MJob] = treino.Content{Prompt: "CANARIO-PROMPT", Response: "CANARIO-RESPMOD", RespFinal: true,
		PromptJSON: []byte(`{"model":"qwen3-ia","system":"CANARIO-SYS","prompt":"CANARIO-PROMPT"}`)}
	f.src.mu.Unlock()
	rep := treino.Report{Projeto: "CANARIO-PROJ", Jobs: []treino.JobRef{{ID: adv12MJob, Papel: "unico", Backend: "primario"}}, Categoria: "omitiu",
		Descricao: "CANARIO-DESC", RespostaCorreta: "CANARIO-CERTA", CorrecaoAplicada: "CANARIO-CORR", Severidade: "alta", Origem: "claude"}
	if err := rep.Validate(); err != nil {
		t.Fatal(err)
	}
	res, err := f.ts.Ingest(context.Background(), rep, treino.Origin{Computer: "CANARIOPC", User: "CANARIOUSER", Domain: "CANARIODOM", ClientIP: "10.0.0.7", ClientLabel: "CANARIOPC/CANARIOUSER"}, "", f.src, f.key)
	if err != nil || res.AmostrasCopiadas != 1 {
		t.Fatalf("ingest: %+v %v", res, err)
	}
	occ := res.OcorrenciaID
	if w := f.adm("POST", "/monitor/api/treino/"+occ+"/confirmar", "{}"); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := f.adm("POST", "/monitor/api/treino/"+occ+"/exemplo-publico", `{"marcar":true}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	var phmac, chmac, grupo, hist string
	db := f.openDB(t)
	db.QueryRow(`SELECT prompt_hmac, COALESCE(conteudo_hmac,'') FROM amostras WHERE job_id=?`, adv12MJob).Scan(&phmac, &chmac)
	db.QueryRow(`SELECT COALESCE(grupo_id,'') FROM ocorrencias WHERE id=?`, occ).Scan(&grupo)
	db.QueryRow(`SELECT COALESCE(depois,'') FROM historico WHERE ocorrencia_id=? AND depois LIKE 'hmac=%' LIMIT 1`, occ).Scan(&hist)
	secrets := []string{"CANARIO", occ, adv12MJob, f.key.ID()}
	for _, s := range []string{phmac, chmac, grupo} {
		if len(s) >= 16 {
			secrets = append(secrets, s[:16])
		}
	}
	if m := regexp.MustCompile(`hmac=([0-9a-f]{16})`).FindStringSubmatch(hist); m != nil {
		secrets = append(secrets, m[1])
	}
	// sanity: the administrator does see them (the fixture is real)
	if w := f.adm("GET", "/monitor/api/treino/"+occ, ""); !strings.Contains(w.Body.String(), "CANARIO-CERTA") || !strings.Contains(w.Body.String(), "CANARIO-PROMPT") {
		t.Fatalf("fixture: o admin nao ve o conteudo: %s", w.Body)
	}
	var leaks []string
	seen := 0
	check := func(name, body string) {
		seen++
		for _, s := range secrets {
			if s != "" && strings.Contains(body, s) {
				leaks = append(leaks, fmt.Sprintf("%s mostra %q", name, s))
			}
		}
	}
	visitors := map[string]map[string]string{
		"anonimo":               nil,
		"cookie mas cross-site": {"Cookie": f.cookie, "Sec-Fetch-Site": "cross-site"},
		"cookie mas same-site":  {"Cookie": f.cookie, "Sec-Fetch-Site": "same-site"},
		"cookie invalido":       {"Cookie": cookieName + "=" + newToken()},
	}
	for _, r := range adv12MRoutes(t) {
		if r.method != "GET" || strings.HasSuffix(r.pattern, "/stream") { // the stream is read below over a real server
			continue
		}
		for _, id := range []string{occ, adv12MJob} {
			p := adv12MFill(r.pattern, id)
			for _, q := range []string{"", "?parte=teste", "?parte=treino", "?estado=confirmada&limite=500", "?janela=7d"} {
				for vn, hdr := range visitors {
					w := do(f.h, "GET", p+q, "", hdr)
					check(fmt.Sprintf("%s%s [%s]", p, q, vn), w.Body.String()+fmt.Sprint(w.Header()))
				}
			}
		}
	}
	// the event stream, over a real server (the recorder never ends a stream)
	srv := httptest.NewServer(f.h)
	defer srv.Close()
	defer f.h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/monitor/api/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	sse := ""
	if err == nil {
		br := bufio.NewReader(resp.Body)
		for {
			l, err := br.ReadString('\n')
			if strings.HasPrefix(l, "data: ") {
				sse = l
				break
			}
			if err != nil {
				break
			}
		}
		resp.Body.Close()
	}
	cancel()
	if !strings.Contains(sse, `"treino"`) {
		t.Fatalf("fixture: o SSE nao trouxe o resumo: %q", sse)
	}
	check("SSE", sse)
	// what the visitor DOES see (for the record): the summary keys
	var st struct {
		Treino map[string]any `json:"treino"`
	}
	json.Unmarshal(do(f.h, "GET", "/monitor/api/state", "", nil).Body.Bytes(), &st)
	var keys []string
	for k := range st.Treino {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(leaks) == 0 && seen > 40 {
		vec12M(t, "M2.1", "OK", fmt.Sprintf("%d respostas de visitante (todas as rotas GET por go/ast, id de ocorrencia e de job, 4 tipos de visitante, filtros) + 1 evento SSE: nenhum texto (descricao, resposta_correta, correcao, prompt, resposta, system), projeto, estacao, dominio, id de ocorrencia/job, grupo, prompt_hmac/conteudo_hmac/hmac do historico nem key_id. O visitante ve so o resumo: %s", seen, strings.Join(keys, ",")))
	} else {
		vec12M(t, "M2.1", "FALHA", fmt.Sprintf("Bloqueador treino.go:74 vazamento ao visitante (%d respostas): %s", seen, strings.Join(leaks, "; ")))
	}
}

// ---- M3: XSS through the declared fields --------------------------------------------------------------------

const adv12MXSS = "</script><script>alert(1)</script><img src=x onerror=alert(2)>\"'javascript:alert(3)&amp;  "

func adv12MRawSafe(b []byte) []string {
	var bad []string
	for _, c := range []string{"<", ">", " ", " "} {
		if bytes.Contains(b, []byte(c)) {
			bad = append(bad, strconv.Quote(c))
		}
	}
	if bytes.Contains(b, []byte("&amp;")) || bytes.Contains(b, []byte("&")) {
		bad = append(bad, `"&"`)
	}
	return bad
}

func TestAdv12MM3XSS(t *testing.T) {
	f := newTreinoFixture(t)
	small := adv12MXSS[:60] // projeto <= 128 bytes
	f.src.mu.Lock()
	f.src.jobs[adv12MJob] = treino.Job{Tool: "chat", Endpoint: "/api/chat", Model: "qwen3-ia", Think: "off", Status: "ok", HTTPStatus: 200,
		StationComputer: small, StationUser: small, ClientIP: "10.0.0.7"}
	pj, _ := json.Marshal(map[string]any{"model": "qwen3-ia", "messages": []map[string]string{{"role": "user", "content": adv12MXSS}}})
	f.src.conts[adv12MJob] = treino.Content{Prompt: adv12MXSS, Response: adv12MXSS, RespFinal: true, PromptJSON: pj}
	f.src.mu.Unlock()
	// a report with every declared field poisoned, from the station (feedback path)
	rep := treino.Report{Projeto: small, Jobs: []treino.JobRef{{ID: adv12MJob, Papel: "unico", Backend: "primario"}}, Categoria: "inventou",
		Descricao: adv12MXSS, RespostaCorreta: adv12MXSS, CorrecaoAplicada: adv12MXSS, Severidade: "media", Origem: "claude"}
	if err := rep.Validate(); err != nil {
		t.Fatal(err)
	}
	res, err := f.ts.Ingest(context.Background(), rep, treino.Origin{Computer: small, User: small, Domain: small, ClientIP: "10.0.0.7", ClientLabel: "x"}, "", f.src, f.key)
	if err != nil || res.AmostrasCopiadas != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	// and the same through the administrator's "Marcar erro", on a second job with the same poisoned content
	const job2 = "deadbeefcafebabe0123456789abcdee"
	f.src.mu.Lock()
	f.src.jobs[job2] = f.src.jobs[adv12MJob]
	f.src.conts[job2] = f.src.conts[adv12MJob]
	f.src.mu.Unlock()
	mb, _ := json.Marshal(map[string]string{"categoria": "omitiu", "severidade": "alta", "descricao": adv12MXSS, "resposta_correta": adv12MXSS, "correcao_aplicada": adv12MXSS, "projeto": small})
	wm := f.adm("POST", "/monitor/api/jobs/"+job2+"/marcar-erro", string(mb))
	var mr treino.Result
	json.Unmarshal(wm.Body.Bytes(), &mr)
	if wm.Code != 200 {
		t.Fatalf("marcar-erro: %d %s", wm.Code, wm.Body)
	}
	for _, id := range []string{res.OcorrenciaID, mr.OcorrenciaID} {
		if w := f.adm("POST", "/monitor/api/treino/"+id+"/confirmar", "{}"); w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino'`)
	var bad []string
	roundTrip := 0
	look := func(name string, w *httptest.ResponseRecorder, ctype string) {
		if w.Code != 200 && w.Code != 400 {
			bad = append(bad, fmt.Sprintf("%s: %d", name, w.Code))
		}
		if v := adv12MRawSafe(w.Body.Bytes()); len(v) > 0 {
			bad = append(bad, name+" tem "+strings.Join(v, ","))
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), ctype) || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			bad = append(bad, fmt.Sprintf("%s: Content-Type %q nosniff %q", name, w.Header().Get("Content-Type"), w.Header().Get("X-Content-Type-Options")))
		}
		// the payload is really in there (no false OK): decoded back, byte for byte
		for _, line := range strings.Split(strings.TrimSpace(w.Body.String()), "\n") {
			var v any
			if json.Unmarshal([]byte(line), &v) == nil && strings.Contains(fmt.Sprint(v), adv12MXSS) {
				roundTrip++
				break
			}
		}
	}
	look("detalhe(relato)", f.adm("GET", "/monitor/api/treino/"+res.OcorrenciaID, ""), "application/json")
	look("detalhe(marcar)", f.adm("GET", "/monitor/api/treino/"+mr.OcorrenciaID, ""), "application/json")
	look("lista", f.adm("GET", "/monitor/api/treino?limite=500", ""), "application/json")
	look("export", f.adm("GET", "/monitor/api/treino/export?parte=treino", ""), "application/x-ndjson")
	// an invalid category that is echoed in the error (clip)
	eb, _ := json.Marshal(map[string]string{"categoria": "<img src=x onerror=alert(1)>", "severidade": "alta", "descricao": "d"})
	we := f.adm("POST", "/monitor/api/jobs/"+adv12MJob+"/marcar-erro", string(eb))
	if we.Code != 400 {
		bad = append(bad, fmt.Sprintf("categoria invalida: %d", we.Code))
	}
	if v := adv12MRawSafe(we.Body.Bytes()); len(v) > 0 {
		bad = append(bad, "erro ecoado tem "+strings.Join(v, ","))
	}
	exp := f.adm("GET", "/monitor/api/treino/export?parte=treino", "")
	if cd := exp.Header().Get("Content-Disposition"); !regexp.MustCompile(`^attachment; filename="erros-treino-\d{8}-\d{6}\.jsonl"$`).MatchString(cd) {
		bad = append(bad, "Content-Disposition "+cd)
	}
	if len(bad) == 0 && roundTrip >= 3 {
		vec12M(t, "M3.1", "OK", fmt.Sprintf("payload </script>, <img onerror>, javascript:, aspas, &, U+2028/2029 em descricao, resposta_correta, correcao_aplicada, projeto, estacao (maquina/usuario/dominio), prompt e resposta (relato e Marcar erro): detalhe, lista, export e erro ecoado sem <, >, & nem U+2028/2029 crus (encoding/json escapa), Content-Type JSON/ndjson + nosniff, attachment com nome fixo; o payload volta intacto ao decodificar (%d respostas)", roundTrip))
	} else {
		vec12M(t, "M3.1", "FALHA", fmt.Sprintf("Bloqueador monitor.go:282 resposta com HTML cru: %s (roundTrip=%d)", strings.Join(bad, "; "), roundTrip))
	}
}

// The page: only textContent; every href/src/location built from data is a finding.
func TestAdv12MM3PageSinks(t *testing.T) {
	page := string(indexHTML)
	m := scriptRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatal("sem script")
	}
	js := m[1]
	var bad []string
	for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "setTimeout(\"", "setInterval(\"",
		"srcdoc", ".action", "formAction", "window.open", "location.assign", "location.replace", "createContextualFragment", "DOMParser", "javascript:"} {
		if strings.Contains(js, sink) {
			bad = append(bad, sink)
		}
	}
	// setAttribute: only the SVG geometry and aria-selected, never href/src/on*/style
	for _, sa := range regexp.MustCompile(`setAttribute\(\s*"([^"]+)"`).FindAllStringSubmatch(js, -1) {
		switch sa[1] {
		case "viewBox", "preserveAspectRatio", "x1", "x2", "y1", "y2", "class", "points", "x", "y", "aria-selected":
		default:
			bad = append(bad, "setAttribute("+sa[1]+")")
		}
	}
	if regexp.MustCompile(`setAttribute\(\s*[^"\s]`).MatchString(js) {
		bad = append(bad, "setAttribute com nome dinamico")
	}
	// every assignment to href/src/location: only the two known exports, with fixed prefixes
	allowed := map[string]bool{
		`window.location.href = "/monitor/api/jobs/export?" + q.join("&");`:        true,
		`window.location.href = "/monitor/api/treino/export?parte=" + parte; }`: true,
	}
	nURL, nFetch := 0, 0
	for _, l := range strings.Split(js, "\n") {
		if regexp.MustCompile(`\.(href|src)\s*=|location\s*=`).MatchString(l) {
			k := strings.TrimSpace(l)
			if i := strings.Index(k, "window.location"); i >= 0 {
				k = k[i:]
			}
			nURL++
			if !allowed[k] {
				bad = append(bad, "atribuicao de URL: "+strings.TrimSpace(l))
			}
		}
	}
	for _, call := range regexp.MustCompile(`trExport\(([^)]*)\)`).FindAllStringSubmatch(js, -1) {
		if call[1] != `"treino"` && call[1] != `"teste"` && call[1] != "parte" {
			bad = append(bad, "trExport("+call[1]+")")
		}
	}
	// the Erros section: fetch/post paths only from server ids (validated by idRe on the server) and constants
	sec := js[strings.Index(js, "// ---- Erros (treino)"):]
	for _, u := range regexp.MustCompile(`(?:fetch|post)\(("[^"]*")\s*\+\s*([a-zA-Z_.]+)`).FindAllStringSubmatch(sec, -1) {
		nFetch++
		switch u[2] {
		case "trQuery", "id", "j.id":
		default:
			bad = append(bad, "URL com "+u[2])
		}
	}
	csp, _ := buildCSP(indexHTML)
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") || !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "form-action 'none'") {
		bad = append(bad, "CSP "+csp)
	}
	if nURL != 2 || nFetch < 4 {
		t.Fatalf("fixture: a varredura nao achou o que devia (location.href=%d, fetch/post com dado=%d)", nURL, nFetch)
	}
	if len(bad) == 0 {
		vec12M(t, "M3.2", "OK", "index.html (script inteiro e a secao Erros): nenhum innerHTML/outerHTML/insertAdjacentHTML/document.write/eval/new Function/srcdoc/window.open/javascript:; setAttribute so de geometria SVG e aria; as 2 unicas atribuicoes de location.href tem prefixo fixo (trExport so com \"treino\"/\"teste\"); URLs da aba Erros so com id do servidor; CSP sem unsafe-*, default-src/form-action 'none'. Observacao: className vem de r.severidade/estado (enum validado no servidor), sem efeito de script")
	} else {
		vec12M(t, "M3.2", "FALHA", "Bloqueador index.html sink com dado: "+strings.Join(bad, "; "))
	}
}

// ---- M4 and M5: a long export over a real server ------------------------------------------------------------------

type adv12MSrv struct {
	f    *tfix
	srv  *httptest.Server
	mu   sync.Mutex
	done map[string]chan struct{} // closed when the export with ?adv=<tag> returns
}

func (s *adv12MSrv) doneCh(tag string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.done[tag]
	if !ok {
		c = make(chan struct{})
		s.done[tag] = c
	}
	return c
}

func adv12MServer(t *testing.T, f *tfix) *adv12MSrv {
	s := &adv12MSrv{f: f, done: map[string]chan struct{}{}}
	mux := http.NewServeMux()
	fb := &treino.Handler{Store: f.ts, Source: f.src, Key: f.key, Role: "primario"}
	mux.Handle("/api/feedback", fb)
	mux.HandleFunc("/monitor/", func(w http.ResponseWriter, r *http.Request) {
		if tag := r.URL.Query().Get("adv"); tag != "" && r.URL.Path == "/monitor/api/treino/export" {
			c := s.doneCh(tag)
			defer close(c)
		}
		f.h.ServeHTTP(w, r)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(func() { f.h.Close(); s.srv.CloseClientConnections(); s.srv.Close() })
	return s
}

// get sends a GET; admin adds the session cookie; ip, when given, is the X-Real-Ip (the gateway trusts it from loopback).
func (s *adv12MSrv) get(ctx context.Context, path string, admin bool, ip string) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", s.srv.URL+path, nil)
	if admin {
		req.Header.Set("Cookie", s.f.cookie)
	}
	if ip != "" {
		req.Header.Set("X-Real-Ip", ip)
	}
	return (&http.Client{Transport: &http.Transport{DisableKeepAlives: true}}).Do(req)
}

func adv12MWait(c chan struct{}, d time.Duration) bool {
	select {
	case <-c:
		return true
	case <-time.After(d):
		return false
	}
}

// adv12MBig creates n confirmed occurrences in the training part, each with an answer of size bytes.
func adv12MBig(t *testing.T, f *tfix, n, size int) {
	t.Helper()
	ans := strings.Repeat("r", size)
	for i := 1; i <= n; i++ {
		id := f.occ(t, 1000+i, "omitiu", ans)
		if err := f.ts.Confirm(context.Background(), id, treino.AdminActor("10.100.2.50")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino'`); err != nil {
		t.Fatal(err)
	}
}

// adv12MLines reads JSONL lines (at most max; -1 for all) and returns the ocorrencia_id of each.
func adv12MLines(r io.Reader, max int) ([]string, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	var ids []string
	for max < 0 || len(ids) < max {
		l, err := br.ReadBytes('\n')
		if len(l) > 0 && l[len(l)-1] == '\n' {
			var x struct {
				Meta struct {
					ID string `json:"ocorrencia_id"`
				} `json:"meta"`
			}
			if json.Unmarshal(l, &x) == nil {
				ids = append(ids, x.Meta.ID)
			}
		}
		if err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func TestAdv12MM4M5ExportOverRealServer(t *testing.T) {
	f := newTreinoFixture(t)
	const N, size = 150, 120 << 10
	adv12MBig(t, f, N, size)
	s := adv12MServer(t, f)
	old := treinoExportMax
	defer func() { treinoExportMax = old }()
	rowsBy := func(ip string) int {
		return f.count(t, `SELECT COUNT(*) FROM historico WHERE acao='exportou' AND campo='treino' AND quem='admin' AND ip=?`, ip)
	}
	const exp = "/monitor/api/treino/export?parte=treino&adv="

	// reference: a full export, read to the end
	ctx0, c0 := context.WithTimeout(context.Background(), 90*time.Second)
	r0, err := s.get(ctx0, exp+"ref", true, "10.9.9.10")
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := adv12MLines(r0.Body, -1)
	r0.Body.Close()
	c0()
	if !adv12MWait(s.doneCh("ref"), 20*time.Second) {
		t.Fatal("o export de referencia nao terminou")
	}
	full := len(ref)
	if full < N-5 || rowsBy("10.9.9.10") != full {
		t.Fatalf("fixture: export completo com %d linhas e %d exportou (esperado ~%d)", full, rowsBy("10.9.9.10"), N)
	}

	// ---- M4: a client that reads nothing ----
	treinoExportMax = 1500 * time.Millisecond
	ctxS, cS := context.WithCancel(context.Background())
	rS, err := s.get(ctxS, exp+"stuck", true, "10.9.9.1") // returns at the headers (first flush)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // the socket buffers fill; the handler blocks in Write
	t0 := time.Now()
	var bad []string
	timed := func(name string, fn func(ctx context.Context) error) {
		ctx, c := context.WithTimeout(context.Background(), 8*time.Second)
		defer c()
		st := time.Now()
		if err := fn(ctx); err != nil {
			bad = append(bad, name+": "+err.Error())
		} else if d := time.Since(st); d > 5*time.Second {
			bad = append(bad, fmt.Sprintf("%s levou %v", name, d))
		}
	}
	get200 := func(path string, admin bool) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			r, err := s.get(ctx, path, admin, "")
			if err != nil {
				return err
			}
			defer r.Body.Close()
			io.Copy(io.Discard, r.Body)
			if r.StatusCode != 200 {
				return fmt.Errorf("%d", r.StatusCode)
			}
			return nil
		}
	}
	timed("state visitante", get200("/monitor/api/state", false))
	timed("state admin", get200("/monitor/api/state", true))
	timed("treino lista admin", get200("/monitor/api/treino?limite=500", true))
	timed("treino resumo visitante", get200("/monitor/api/treino", false))
	timed("SSE", func(ctx context.Context) error {
		r, err := s.get(ctx, "/monitor/api/stream", false, "")
		if err != nil {
			return err
		}
		defer r.Body.Close()
		l, err := bufio.NewReader(r.Body).ReadString('\n')
		if !strings.HasPrefix(l, "data: ") {
			return fmt.Errorf("primeira linha %q %v", l, err)
		}
		return nil
	})
	f.src.add(4000)
	timed("POST /api/feedback", func(ctx context.Context) error {
		b, _ := json.Marshal(treino.Report{Jobs: []treino.JobRef{{ID: tid(4000), Papel: "unico", Backend: "primario"}}, Categoria: "inventou", Descricao: "durante o export", Severidade: "baixa", Origem: "claude"})
		req, _ := http.NewRequestWithContext(ctx, "POST", s.srv.URL+"/api/feedback", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer r.Body.Close()
		rb, _ := io.ReadAll(r.Body)
		if r.StatusCode != 200 {
			return fmt.Errorf("%d %s", r.StatusCode, rb)
		}
		return nil
	})
	timed("acao do admin", func(ctx context.Context) error {
		req, _ := http.NewRequestWithContext(ctx, "POST", s.srv.URL+"/monitor/api/treino/"+tid(1)+"/confirmar", strings.NewReader("{}"))
		req.Header.Set("Cookie", f.cookie)
		req.Header.Set("X-CSRF-Token", f.csrf)
		req.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer r.Body.Close()
		if r.StatusCode != 404 { // unknown id: the route ran through the writers' turn and the read
			return fmt.Errorf("%d", r.StatusCode)
		}
		return nil
	})
	timed("2o export (lido)", func(ctx context.Context) error {
		r, err := s.get(ctx, exp+"second", true, "10.9.9.2")
		if err != nil {
			return err
		}
		defer r.Body.Close()
		ids, _ := adv12MLines(r.Body, 3)
		if len(ids) != 3 {
			return fmt.Errorf("%d linhas", len(ids))
		}
		return nil
	})
	if len(bad) == 0 {
		vec12M(t, "M4.1", "OK", fmt.Sprintf("export de %d linhas x %d KiB (~%d MiB) para um cliente que nao le: state (visitante e admin), lista, resumo, SSE, POST /api/feedback, acao do admin e um 2o export responderam, %v no total (nenhum mutex do monitor nem a vez de escrita do treino ficam presos; C-6.10)", full, size>>10, full*size>>20, time.Since(t0).Round(time.Millisecond)))
	} else {
		vec12M(t, "M4.1", "FALHA", "Importante treino.go:177 export travado prende o monitor: "+strings.Join(bad, "; "))
	}
	if !adv12MWait(s.doneCh("second"), 20*time.Second) {
		t.Fatal("o 2o export (cortado pelo cliente) nao terminou")
	}
	// does the stuck export end by its own deadline (1.5 s, already ~%v ago)?
	endedByDeadline := adv12MWait(s.doneCh("stuck"), 8*time.Second)
	stuckRows := rowsBy("10.9.9.1")
	stuckAge := time.Since(t0) + time.Second
	cS()
	rS.Body.Close()
	if !endedByDeadline && !adv12MWait(s.doneCh("stuck"), 20*time.Second) {
		t.Fatal("o export travado nao terminou nem com a conexao fechada")
	}
	switch {
	case endedByDeadline && stuckRows >= full:
		t.Fatalf("fixture: o export 'travado' foi inteiro (%d linhas): o cliente nao travou o Write", stuckRows)
	case endedByDeadline:
		vec12M(t, "M4.2", "OK", fmt.Sprintf("o export travado terminou pelo prazo treinoExportMax (%d linhas exportou)", stuckRows))
	default:
		vec12M(t, "M4.2", "FALHA", fmt.Sprintf("Menor monitor/treino.go:170-177 + treino/export.go:157-160/179-182: com o cliente sem ler, o export ficou %v preso em Write apesar de treinoExportMax=1.5s (o prazo so e conferido ENTRE linhas; o handler nao faz SetWriteDeadline como o SSE em monitor.go:499; o servidor nao tem WriteTimeout, cmd/mcpollama-gateway/main.go:369). Custo: 1 goroutine + o lote montado (ate 20 linhas / 8 MiB) enquanto a conexao viver; so o admin dispara; o nginx nao amortece (proxy_buffering off no /monitor, deploy/gateway/ollama.conf:91) e so o send_timeout padrao (60 s) dele corta um cliente parado, nao um que leia a conta-gotas. %d de %d linhas `exportou` gravadas para 0 linhas lidas (write-ahead, lado seguro). Correcao: http.NewResponseController(w).SetWriteDeadline(prazo) antes do ExportUntil", stuckAge.Round(time.Second), stuckRows, full))
	}

	// ---- M5: the client cuts the connection in the middle ----
	treinoExportMax = old // no deadline pressure from here on (under -race the export is slow)
	ctx1, c1 := context.WithCancel(context.Background())
	r1, err := s.get(ctx1, exp+"cut", true, "10.9.9.3")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := adv12MLines(r1.Body, 10)
	c1()
	r1.Body.Close()
	if !adv12MWait(s.doneCh("cut"), 20*time.Second) {
		t.Fatal("o export cortado nao terminou")
	}
	rows1 := rowsBy("10.9.9.3")
	missing := 0
	for _, id := range got {
		if f.count(t, `SELECT COUNT(*) FROM historico WHERE acao='exportou' AND ocorrencia_id=? AND campo='treino' AND quem='admin' AND ip='10.9.9.3'`, id) == 0 {
			missing++
		}
	}
	if len(got) == 10 && rows1 >= len(got) && missing == 0 {
		vec12M(t, "M5.1", "OK", fmt.Sprintf("MEDIDA: cliente leu %d linhas e cortou a conexao; %d linhas `exportou` gravadas para ele (write-ahead em lotes de 20: excesso de %d, lado seguro); toda linha recebida tem a sua `exportou` com quem=admin e o ip dele", len(got), rows1, rows1-len(got)))
	} else {
		vec12M(t, "M5.1", "FALHA", fmt.Sprintf("Importante treino/export.go:150 linha entregue sem `exportou`: recebidas %d, gravadas %d, faltando %d", len(got), rows1, missing))
	}

	// two complete exports in parallel
	var wg sync.WaitGroup
	lens := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, c := context.WithTimeout(context.Background(), 120*time.Second)
			defer c()
			r, err := s.get(ctx, exp+"par"+strconv.Itoa(i), true, "10.9.9.2"+strconv.Itoa(i))
			if err != nil {
				return
			}
			ids, _ := adv12MLines(r.Body, -1)
			r.Body.Close()
			lens[i] = len(ids)
		}(i)
	}
	wg.Wait()
	adv12MWait(s.doneCh("par0"), 20*time.Second)
	adv12MWait(s.doneCh("par1"), 20*time.Second)
	p0, p1 := rowsBy("10.9.9.20"), rowsBy("10.9.9.21")
	if lens[0] == full && lens[1] == full && p0 == full && p1 == full {
		vec12M(t, "M5.2", "OK", fmt.Sprintf("2 exports em paralelo: %d e %d linhas (completas), %d e %d linhas `exportou` (uma por ocorrencia por download, sem perda nem duplicata)", lens[0], lens[1], p0, p1))
	} else {
		vec12M(t, "M5.2", "FALHA", fmt.Sprintf("Importante treino/export.go:150 exports paralelos: linhas %v (esperado %d cada), exportou %d e %d", lens, full, p0, p1))
	}

	// an export cut by its deadline, read by a slow client: does the client learn that the file is incomplete?
	treinoExportMax = 1 * time.Second
	ctx3, c3 := context.WithTimeout(context.Background(), 120*time.Second)
	defer c3()
	r3, err := s.get(ctx3, exp+"slow", true, "10.9.9.4")
	if err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReaderSize(r3.Body, 64<<10)
	n3 := 0
	var endErr error
	for {
		l, err := br.ReadBytes('\n')
		if len(l) > 0 && l[len(l)-1] == '\n' {
			n3++
			time.Sleep(60 * time.Millisecond) // slow reader: the deadline passes in the middle
		}
		if err != nil {
			endErr = err
			break
		}
	}
	r3.Body.Close()
	treinoExportMax = old
	adv12MWait(s.doneCh("slow"), 20*time.Second)
	rows3 := rowsBy("10.9.9.4")
	switch {
	case n3 >= full:
		t.Fatalf("fixture: o export lento nao foi cortado pelo prazo (%d linhas)", n3)
	case endErr == io.EOF && r3.StatusCode == 200 && r3.Trailer.Get("X-Export-Incompleto") == "":
		vec12M(t, "M5.3", "FALHA", fmt.Sprintf("Menor monitor/treino.go:177-179: o export cortado pelo prazo (ErrExportDeadline; vale o mesmo para erro de banco no meio) entregou %d de %d linhas e TERMINOU LIMPO para o cliente (HTTP 200, fim de chunked normal, io.EOF, sem trailer nem marca): o comentario \"the cut file is the signal\" nao vale, o navegador mostra o download como completo e o admin pode treinar/avaliar com um arquivo parcial sem saber. O historico bate (%d exportou para %d linhas, lado seguro). Correcao: num erro depois do 1o byte, abortar a conexao (panic(http.ErrAbortHandler)) ou gravar uma ultima linha/trailer de erro", n3, full, rows3, n3))
	default:
		vec12M(t, "M5.3", "OK", fmt.Sprintf("o export cortado (%d de %d linhas) chega ao cliente com erro de transporte (%v)", n3, full, endErr))
	}
}

// ---- M6: ids in the path, paging and the size of the detail -------------------------------------------------

func TestAdv12MM6IdsPagingAndDetailSize(t *testing.T) {
	f := newTreinoFixture(t)
	occ := f.occ(t, 1, "omitiu", "certa")
	before := f.adv12MSnap(t)
	ids := []string{"..%2F..%2Fx", "..%2f" + occ, "%2e%2e", occ[:31], occ + "0", strings.ToUpper(occ), occ[:31] + "%00", occ + "%00", "%0d%0aSet-Cookie:x=1", occ[:30] + "%0d%0a",
		" " + occ, occ + "%20", "g" + occ[1:], occ + "%2Fconfirmar", "*", "%25" + occ[:30]}
	var bad []string
	tried := 0
	for _, id := range ids {
		for _, c := range []struct{ m, p string }{{"GET", "/monitor/api/treino/" + id}, {"POST", "/monitor/api/treino/" + id + "/confirmar"}, {"POST", "/monitor/api/treino/" + id + "/apagar"},
			{"POST", "/monitor/api/treino/" + id + "/liberar-quarentena"}, {"POST", "/monitor/api/jobs/" + id + "/marcar-erro"}} {
			var w *httptest.ResponseRecorder
			func() {
				defer func() {
					if r := recover(); r != nil {
						w = nil // httptest.NewRequest refuses an invalid URL: the client cannot send it either
					}
				}()
				body := "{}"
				if strings.HasSuffix(c.p, "marcar-erro") {
					body = `{"categoria":"omitiu","severidade":"baixa","descricao":"x"}`
				}
				w = f.adm(c.m, c.p, body)
			}()
			if w == nil {
				continue
			}
			tried++
			if w.Code == 200 || w.Code >= 500 || strings.Contains(w.Header().Get("Set-Cookie"), "x=1") {
				bad = append(bad, fmt.Sprintf("%s %s -> %d", c.m, c.p, w.Code))
			}
		}
	}
	if len(bad) == 0 && tried > 40 && f.adv12MSnap(t) == before {
		vec12M(t, "M6.1", "OK", fmt.Sprintf("%d pedidos com id malicioso (..%%2F, %%2e%%2e, %%00, CRLF, 31/33 hex, maiusculas, espaco, %%2F embutido) no detalhe, confirmar, apagar, liberar-quarentena e marcar-erro: 400/404/301/405, nunca 200/5xx, sem cabecalho injetado; estado identico (idRe ^[0-9a-f]{32}$)", tried))
	} else {
		vec12M(t, "M6.1", "FALHA", fmt.Sprintf("Importante treino.go:119 id malicioso aceito: %s (tentados %d)", strings.Join(bad, "; "), tried))
	}

	// paging
	for i := 2; i <= 30; i++ {
		f.occ(t, i, "inventou", "")
	}
	var pbad []string
	rowsOf := func(q string) (int, int) {
		w := f.adm("GET", "/monitor/api/treino?"+q, "")
		var out struct {
			Rows []treino.ListRow `json:"rows"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, len(out.Rows)
	}
	for q, want := range map[string]int{"limite=999999999": 30, "limite=-1": 30, "limite=0": 30, "limite=abc": 30, "limite=5": 5, "offset=-7&limite=5": 5,
		"offset=9223372036854775807": 0, "offset=99999999999999999999999": 30, "limite=5&offset=28": 2, "limite=1e9": 30} {
		code, n := rowsOf(q)
		if code != 200 || n != want {
			pbad = append(pbad, fmt.Sprintf("%s -> %d/%d linhas (esperado %d)", q, code, n, want))
		}
	}
	for _, q := range []string{"desde=99999999999999999999999", "ate=ontem", "desde=2026-13-45T00:00:00Z"} {
		if code, _ := rowsOf(q); code != 400 {
			pbad = append(pbad, fmt.Sprintf("%s -> %d", q, code))
		}
	}
	// a burst of the heaviest page while a visitor reads the state
	var wg sync.WaitGroup
	t0 := time.Now()
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.adm("GET", "/monitor/api/treino?limite=500&estacao=PC01", "") }()
	}
	st := time.Now()
	sw := do(f.h, "GET", "/monitor/api/state", "", nil)
	stateLat := time.Since(st)
	wg.Wait()
	burst := time.Since(t0)
	if len(pbad) == 0 && sw.Code == 200 && stateLat < 3*time.Second {
		vec12M(t, "M6.2", "OK", fmt.Sprintf("paginacao: limite enorme/negativo/0/abc/1e9 cai em 100 (teto 500), offset negativo vira 0, offset int64 max devolve 0 linhas, overflow vira 0; desde/ate invalidos 400. MEDIDA: 40 listas de 500 em paralelo em %v; /state no meio respondeu em %v", burst.Round(time.Millisecond), stateLat.Round(time.Millisecond)))
	} else {
		vec12M(t, "M6.2", "FALHA", fmt.Sprintf("Menor treino.go:82 paginacao: %s; state %d em %v", strings.Join(pbad, "; "), sw.Code, stateLat))
	}

	// a huge field in the detail: multibyte text of 3 MiB in the sample and 1 MiB in the occurrence
	big := strings.Repeat("ção€", (3<<20)/9)
	db := f.openDB(t)
	if _, err := db.Exec(`UPDATE amostras SET prompt_texto=?, resposta=? WHERE job_id=?`, []byte(big), []byte(big), tid(1)); err != nil {
		t.Fatal(err)
	}
	bigDesc := strings.Repeat("€", (1<<20)/3)
	if _, err := db.Exec(`UPDATE ocorrencias SET descricao=?, resposta_correta=?, correcao_aplicada=? WHERE id=?`, bigDesc, bigDesc, bigDesc, occ); err != nil {
		t.Fatal(err)
	}
	w := f.adm("GET", "/monitor/api/treino/"+occ, "")
	var d treino.Detail
	json.Unmarshal(w.Body.Bytes(), &d)
	var dbad []string
	if w.Code != 200 || len(d.Amostras) != 1 {
		dbad = append(dbad, fmt.Sprintf("detalhe %d, %d amostras", w.Code, len(d.Amostras)))
	} else {
		a := d.Amostras[0]
		lim := treino.DetailTextMax + 64
		for name, v := range map[string]string{"prompt": a.Prompt, "resposta": a.Resposta, "descricao": d.Descricao, "resposta_correta": d.RespostaCorreta, "correcao_aplicada": d.CorrecaoAplicada} {
			if len(v) > lim {
				dbad = append(dbad, fmt.Sprintf("%s com %d bytes", name, len(v)))
			}
			if !utf8.ValidString(v) || strings.ContainsRune(v, utf8.RuneError) {
				dbad = append(dbad, name+" cortado no meio de um caractere")
			}
			if !strings.Contains(v, "[cortado (") {
				dbad = append(dbad, name+" sem a marca de corte")
			}
		}
		if !a.PromptCortado || !a.RespCortada || a.PromptBytes != len(big) || a.RespBytes != len(big) {
			dbad = append(dbad, fmt.Sprintf("marcas: %v %v %d %d (esperado %d)", a.PromptCortado, a.RespCortada, a.PromptBytes, a.RespBytes, len(big)))
		}
		if len(w.Body.Bytes()) > 6*treino.DetailTextMax {
			dbad = append(dbad, fmt.Sprintf("JSON com %d bytes", len(w.Body.Bytes())))
		}
	}
	lw := f.adm("GET", "/monitor/api/treino?limite=500", "")
	if len(lw.Body.Bytes()) > 200<<10 {
		dbad = append(dbad, fmt.Sprintf("lista com %d bytes", len(lw.Body.Bytes())))
	}
	if len(dbad) == 0 {
		vec12M(t, "M6.3", "OK", fmt.Sprintf("detalhe com amostra de %d bytes multibyte e campos de 1 MiB: cada campo cortado em <= 256 KiB + marca, em fronteira de caractere, prompt_bytes/resp_bytes com o tamanho real; JSON do detalhe %d bytes; lista com 30 linhas %d bytes (descricao cortada em 200 caracteres)", len(big), len(w.Body.Bytes()), len(lw.Body.Bytes())))
	} else {
		vec12M(t, "M6.3", "FALHA", "Importante treino/query.go:270 corte do detalhe: "+strings.Join(dbad, "; "))
	}
}

// ---- M7: malicious bodies on the actions; every success leaves history with quem=admin and the IP ----------------

func TestAdv12MM7ActionBodies(t *testing.T) {
	f := newTreinoFixture(t)
	occ := f.occ(t, 1, "omitiu", "")
	u := func(id, k string) string { return "/monitor/api/treino/" + id + "/" + k }
	before := f.adv12MSnap(t)
	type c struct {
		name, path, body string
		want             int
	}
	big := func(n int) string { return strings.Repeat("x", n) }
	cases := []c{
		{"campo desconhecido (completar)", u(occ, "completar"), `{"descricao":"a","estado":"confirmada"}`, 400},
		{"campo desconhecido (confirmar)", u(occ, "confirmar"), `{"resposta_correta":"a","quem":"x"}`, 400},
		{"campo desconhecido (exemplo)", u(occ, "exemplo-publico"), `{"marcar":true,"ocorrencia":"x"}`, 400},
		{"campo desconhecido (liberar)", u(occ, "liberar-quarentena"), `{"lado":"treino","particao":"teste"}`, 400},
		{"descricao 4097 bytes", u(occ, "completar"), `{"descricao":"` + big(4097) + `"}`, 400},
		{"resposta_correta 128 KiB + 1", u(occ, "completar"), `{"resposta_correta":"` + big(128<<10+1) + `"}`, 400},
		{"correcao_aplicada 4097 bytes", u(occ, "completar"), `{"correcao_aplicada":"` + big(4097) + `"}`, 400},
		{"corpo > 256 KiB", u(occ, "completar"), `{"resposta_correta":"` + big(260<<10) + `"}`, 413},
		{"JSON truncado", u(occ, "completar"), `{"descricao":"a"`, 400},
		{"tipo errado (marcar string)", u(occ, "exemplo-publico"), `{"marcar":"true"}`, 400},
		{"lado vazio", u(occ, "liberar-quarentena"), `{}`, 400},
		{"lado quarentena", u(occ, "liberar-quarentena"), `{"lado":"quarentena"}`, 400},
		{"lado maiusculo", u(occ, "liberar-quarentena"), `{"lado":"TREINO"}`, 400},
		{"lado com NUL", u(occ, "liberar-quarentena"), `{"lado":"treino\u0000"}`, 400},
		{"confirmar sem resposta (C-1.12a)", u(occ, "confirmar"), `{}`, 422},
		{"exemplo de nova (transicao proibida)", u(occ, "exemplo-publico"), `{"marcar":true}`, 409},
		{"marcar-erro descricao 4097", "/monitor/api/jobs/" + tid(1) + "/marcar-erro", `{"categoria":"omitiu","severidade":"baixa","descricao":"` + big(4097) + `"}`, 400},
		{"marcar-erro projeto 129", "/monitor/api/jobs/" + tid(1) + "/marcar-erro", `{"categoria":"omitiu","severidade":"baixa","descricao":"d","projeto":"` + big(129) + `"}`, 400},
		{"marcar-erro campo desconhecido (jobs)", "/monitor/api/jobs/" + tid(1) + "/marcar-erro", `{"categoria":"omitiu","severidade":"baixa","descricao":"d","jobs":[]}`, 400},
		{"marcar-erro origem forjada", "/monitor/api/jobs/" + tid(1) + "/marcar-erro", `{"categoria":"omitiu","severidade":"baixa","descricao":"d","origem":"claude"}`, 400},
		{"marcar-erro NUL", "/monitor/api/jobs/" + tid(1) + "/marcar-erro", `{"categoria":"omitiu","severidade":"baixa","descricao":"a\u0000b"}`, 400},
		{"marcar-erro severidade fora do enum", "/monitor/api/jobs/" + tid(1) + "/marcar-erro", `{"categoria":"omitiu","severidade":"critica","descricao":"d"}`, 400},
	}
	var bad []string
	for _, k := range cases {
		if w := f.adm("POST", k.path, k.body); w.Code != k.want {
			bad = append(bad, fmt.Sprintf("%s -> %d (esperado %d) %s", k.name, w.Code, k.want, strings.TrimSpace(w.Body.String())))
		}
	}
	if len(bad) == 0 && f.adv12MSnap(t) == before {
		vec12M(t, "M7.1", "OK", fmt.Sprintf("%d corpos maliciosos recusados com o codigo certo (campos desconhecidos, limites C-1.17 +1 byte, corpo > 256 KiB 413, JSON truncado, tipo errado, lado vazio/quarentena/maiusculo/NUL, confirmar sem resposta 422, transicao proibida 409, marcar-erro com jobs/origem forjados/NUL/enum); nada gravado", len(cases)))
	} else {
		vec12M(t, "M7.1", "FALHA", "Importante treino.go:258 corpo malicioso aceito: "+strings.Join(bad, "; "))
	}

	// what encoding/json lets through: duplicated keys, case-folded keys and content after the object
	var notes []string
	var trail []string
	w := f.adm("POST", u(occ, "completar"), `{"descricao":"primeira","descricao":"segunda"}`)
	var desc string
	f.openDB(t).QueryRow(`SELECT descricao FROM ocorrencias WHERE id=?`, occ).Scan(&desc)
	notes = append(notes, fmt.Sprintf("chave duplicada -> %d, vale a ultima (%q)", w.Code, desc))
	w = f.adm("POST", u(occ, "completar"), `{"DESCRICAO":"maiuscula"}`)
	f.openDB(t).QueryRow(`SELECT descricao FROM ocorrencias WHERE id=?`, occ).Scan(&desc)
	notes = append(notes, fmt.Sprintf("chave em maiusculas -> %d (%q)", w.Code, desc))
	w = f.adm("POST", u(occ, "completar"), `{"descricao":"com rabo"}{"estado":"confirmada","quem":"x"}`)
	if w.Code == 200 {
		trail = append(trail, "completar")
	}
	f.src.add(2)
	mk := f.adm("POST", "/monitor/api/jobs/"+tid(2)+"/marcar-erro", `{"categoria":"omitiu","severidade":"baixa","descricao":"d"} {"jobs":[{"id":"`+tid(3)+`"}],"origem":"automatico"}`)
	if mk.Code == 200 {
		var mr treino.Result
		json.Unmarshal(mk.Body.Bytes(), &mr)
		var origem string
		var njobs int
		f.openDB(t).QueryRow(`SELECT origem FROM ocorrencias WHERE id=?`, mr.OcorrenciaID).Scan(&origem)
		njobs = f.count(t, `SELECT COUNT(*) FROM ocorrencia_jobs WHERE ocorrencia_id=?`, mr.OcorrenciaID)
		trail = append(trail, fmt.Sprintf("marcar-erro (o 2o objeto foi ignorado: origem=%s, %d job)", origem, njobs))
	}
	// the same body on the station's route (the SSOT of the report) is refused
	fb := &treino.Handler{Store: f.ts, Source: f.src, Key: f.key, Role: "primario"}
	rq := httptest.NewRequest("POST", "/api/feedback", strings.NewReader(`{"jobs":[{"id":"`+tid(2)+`","papel":"unico","backend":"primario"}],"categoria":"omitiu","severidade":"baixa","descricao":"d","origem":"claude"} {"x":1}`))
	rq.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	fb.ServeHTTP(rw, rq)
	if len(trail) > 0 && rw.Code == 400 {
		vec12M(t, "M7.2", "FALHA", fmt.Sprintf("Menor monitor/treino.go:206-210 (readJSONLimit) aceita conteudo depois do objeto JSON (rotas %s responderam 200 com um 2o objeto com campos desconhecidos, ignorado em silencio), enquanto o SSOT do relato, treino/http.go:75-77, recusa o mesmo corpo com %d (\"ha conteudo depois do objeto\"); a C-1.17 manda recusar campo desconhecido no relato, e o Marcar erro e um relato. Correcao: dec.Token()==io.EOF como em http.go. Medido tambem: %s", strings.Join(trail, ", "), rw.Code, strings.Join(notes, "; ")))
	} else {
		vec12M(t, "M7.2", "OK", "conteudo depois do objeto recusado; "+strings.Join(notes, "; "))
	}

	// the edit lets the occurrence lose its required description and take a NUL (the report refuses both)
	var ebad []string
	if w := f.adm("POST", u(occ, "completar"), `{"descricao":"   "}`); w.Code == 200 {
		ebad = append(ebad, "descricao so de espacos -> 200")
	}
	if w := f.adm("POST", u(occ, "completar"), `{"descricao":""}`); w.Code == 200 {
		ebad = append(ebad, "descricao vazia -> 200")
	}
	if w := f.adm("POST", u(occ, "completar"), `{"resposta_correta":"a\u0000b"}`); w.Code == 200 {
		ebad = append(ebad, "resposta_correta com NUL -> 200")
	}
	if len(ebad) > 0 {
		vec12M(t, "M7.3", "FALHA", "Menor treino/state.go:124-128 (Update) nao aplica as regras do relato (treino/report.go:67-68 descricao obrigatoria, :82-84 UTF-8 sem NUL): "+strings.Join(ebad, "; ")+". A C-1.17 diz descricao obrigatoria; a edicao do admin deixa a ocorrencia sem ela. Correcao: validar no Update como no Validate")
	} else {
		vec12M(t, "M7.3", "OK", "a edicao recusa descricao vazia e NUL como o relato")
	}

	// every successful action leaves history with quem=admin and the IP of the request (X-Real-Ip from loopback)
	a := f.occ(t, 10, "omitiu", "certa")
	b := f.occ(t, 11, "inventou", "certa")
	for _, id := range []string{a, b} {
		if w := f.adm("POST", u(id, "confirmar"), "{}"); w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	f.openDB(t).Exec(`UPDATE ocorrencias SET particao='treino' WHERE id=?`, b)
	var pa string
	f.openDB(t).QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, a).Scan(&pa)
	rep := treino.Report{Jobs: []treino.JobRef{{ID: tid(10), Papel: "map1", Backend: "primario"}, {ID: tid(11), Papel: "final", Backend: "primario"}},
		Categoria: "omitiu", Descricao: "liga os dois", Severidade: "baixa", Origem: "claude"}
	if err := rep.Validate(); err != nil {
		t.Fatal(err)
	}
	link, err := f.ts.Ingest(context.Background(), rep, stOrigin, "", f.src, f.key)
	if err != nil {
		t.Fatal(err)
	}
	var pl string
	f.openDB(t).QueryRow(`SELECT particao FROM ocorrencias WHERE id=?`, link.OcorrenciaID).Scan(&pl)
	if pl != "quarentena" {
		t.Fatalf("fixture: o componente nao entrou em quarentena (%q, a=%q)", pl, pa)
	}
	type step struct {
		path, body, acao, campo string
	}
	f.src.add(20)
	steps := []step{
		{u(a, "exemplo-publico"), `{"marcar":true}`, "alterou", "exemplo_publico"},
		{u(link.OcorrenciaID, "liberar-quarentena"), `{"lado":"teste"}`, "alterou", "particao"},
		{u(a, "completar"), `{"correcao_aplicada":"nova"}`, "desconfirmou", "estado"},
		{u(b, "descartar"), `{}`, "descartou", "estado"},
		{u(a, "apagar"), `{}`, "apagou", ""},
		{"/monitor/api/jobs/" + tid(20) + "/marcar-erro", `{"categoria":"omitiu","severidade":"baixa","descricao":"d"}`, "criou", ""},
	}
	var hbad []string
	for _, s := range steps {
		n0 := f.count(t, `SELECT COUNT(*) FROM historico WHERE acao=? AND quem='admin' AND ip='10.100.2.50' AND (?='' OR campo=?)`, s.acao, s.campo, s.campo)
		w := f.adm("POST", s.path, s.body)
		if w.Code != 200 {
			hbad = append(hbad, fmt.Sprintf("%s -> %d %s", s.path, w.Code, w.Body))
			continue
		}
		if n1 := f.count(t, `SELECT COUNT(*) FROM historico WHERE acao=? AND quem='admin' AND ip='10.100.2.50' AND (?='' OR campo=?)`, s.acao, s.campo, s.campo); n1 <= n0 {
			hbad = append(hbad, fmt.Sprintf("%s sem historico %s/%s por admin+ip", s.path, s.acao, s.campo))
		}
	}
	// and no history row carries text (C-7.4/C-7.6): only hmac/key_id/bytes or a state word
	hrows, err := f.openDB(t).Query(`SELECT acao, COALESCE(campo,''), COALESCE(antes,''), COALESCE(depois,'') FROM historico`)
	if err != nil {
		t.Fatal(err)
	}
	hashRe := regexp.MustCompile(`^(hmac=[0-9a-f]{64} key_id=[0-9a-f]{16} )?bytes=\d+$`)
	texts := []string{"liga os dois", "primeira", "segunda", "maiuscula", "com rabo", "certa"} // distinctive: the apagou row holds "estado=nova descricao=[hmac...]"
	nh := 0
	for hrows.Next() {
		var acao, campo, antes, depois string
		hrows.Scan(&acao, &campo, &antes, &depois)
		nh++
		for _, v := range []string{antes, depois} {
			for _, tx := range texts {
				if strings.Contains(v, tx) {
					hbad = append(hbad, fmt.Sprintf("historico %s/%s com texto %q", acao, campo, tx))
				}
			}
			if (campo == "resposta_correta" || campo == "descricao" || campo == "correcao_aplicada") && v != "" && !hashRe.MatchString(v) {
				hbad = append(hbad, fmt.Sprintf("historico %s/%s fora do formato hmac/key_id/bytes: %q", acao, campo, v))
			}
		}
	}
	hrows.Close()
	if nh < 10 {
		t.Fatalf("fixture: so %d linhas de historico", nh)
	}
	if len(hbad) == 0 {
		vec12M(t, "M7.4", "OK", "exemplo-publico, liberar-quarentena (alterou campo=particao, C-4.11), completar de confirmada (desconfirmou), descartar, apagar e marcar-erro (criou) gravam historico com quem=admin e ip=10.100.2.50; nenhuma linha com texto")
	} else {
		vec12M(t, "M7.4", "FALHA", "Importante treino.go:263 acao sem historico admin+ip: "+strings.Join(hbad, "; "))
	}
}
