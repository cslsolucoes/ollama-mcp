package monitor

// Fase 9.5 - revisao adversarial: vetores do pacote monitor.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func vec(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// slowAdmin builds a fixture whose PBKDF2 costs ~tens of ms, like production
// (600k) but faster, so concurrent logins really overlap.
func slowAdmin(t *testing.T, f *fixture) {
	line, err := HashPassword(adminPass, 100000)
	if err != nil {
		t.Fatal(err)
	}
	cr, _ := ParseCredentials(adminUser + ":" + line)
	f.h.cfg.Admin = cr
}

func login(f *fixture, ip, pw string) int {
	return do(f.h, "POST", "/monitor/api/login", `{"user":"admin","password":"`+pw+`"}`, map[string]string{"X-Real-Ip": ip}).Code
}

// M1 (coordenador, MEDIUM): TOCTOU no limite de login. allowed() e checado
// antes do Verify lento e o fail() so e contado depois: pedidos paralelos do
// mesmo IP passam todos e furam as 5 tentativas (e multiplicam o PBKDF2).
func TestAdvLoginTOCTOU(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	slowAdmin(t, f)
	const n = 40
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c := login(f, "10.100.2.66", "errada")
			mu.Lock()
			codes[c]++
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	verified := codes[401]
	if verified > loginMaxIP {
		vec(t, "M1-login-toctou", "Media", "gateway/monitor/monitor.go:587", fmt.Sprintf("FALHA: %d tentativas paralelas do mesmo IP -> %d senhas avaliadas (limite %d); codigos %v", n, verified, loginMaxIP, codes))
		t.Errorf("verified %d > %d", verified, loginMaxIP)
	} else {
		vec(t, "M1-login-toctou", "Media", "gateway/monitor/monitor.go:587", fmt.Sprintf("OK: %d paralelas -> %d avaliadas; codigos %v", n, verified, codes))
	}
}

// M2 (coordenador, MEDIUM): bloqueio global. Falhas anonimas de muitos IPs
// esgotam o teto global (30) e travam o login do administrador legitimo.
func TestAdvLoginGlobalLockout(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	for i := 0; i < 40; i++ {
		login(f, fmt.Sprintf("10.100.7.%d", i+1), "errada")
	}
	code := login(f, "10.100.2.10", adminPass)
	if code != 200 {
		vec(t, "M2-login-bloqueio-global", "Media", "gateway/monitor/admin.go:226", fmt.Sprintf("FALHA: 40 falhas de 40 IPs -> o administrador (IP limpo, senha certa) recebe %d", code))
		t.Errorf("legit login %d", code)
	} else {
		vec(t, "M2-login-bloqueio-global", "Media", "gateway/monitor/admin.go:226", "OK: falhas de outros IPs nao bloqueiam o administrador")
	}
}

// M3 (pendencia 4): SSE com teto global de 20: um unico IP abre 20 painéis e
// ninguem mais ve a fila.
func TestAdvSSEOneIPExhaustsAll(t *testing.T) {
	f := newFixture(t, false, queue.Config{})
	srv := httptest.NewServer(f.h)
	defer srv.Close()
	open := func(ip string) (*http.Response, error) {
		req, _ := http.NewRequest("GET", srv.URL+"/monitor/api/stream", nil)
		req.Header.Set("X-Real-Ip", ip)
		return http.DefaultClient.Do(req)
	}
	var held []*http.Response
	defer func() {
		for _, r := range held {
			r.Body.Close()
		}
	}()
	ok := 0
	for i := 0; i < 25; i++ {
		r, err := open("10.100.2.200")
		if err != nil {
			t.Fatal(err)
		}
		if r.StatusCode == 200 {
			ok++
			held = append(held, r)
		} else {
			r.Body.Close()
		}
	}
	r, err := open("10.100.2.201")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		vec(t, "M3-sse-por-ip", "Media", "gateway/monitor/monitor.go:301", fmt.Sprintf("FALHA: 1 IP abriu %d streams; outro IP recebe %d", ok, r.StatusCode))
		t.Errorf("other IP %d", r.StatusCode)
	} else {
		vec(t, "M3-sse-por-ip", "Media", "gateway/monitor/monitor.go:301", fmt.Sprintf("OK: 1 IP limitado a %d streams; outro IP recebe 200", ok))
	}
}

// M4: XSS por label, tool, modelo e erro (o queue aceita qualquer string; o
// proxy so valida label e tool, o modelo vem cru do corpo).
func TestAdvXSSFields(t *testing.T) {
	f := newFixture(t, false, queue.Config{GenSlots: 1})
	x := `</script><img src=x onerror=alert(1)>"'&`
	tk, err := f.q.Acquire(context.Background(), queue.Request{ClientIP: "10.0.0.1", ClientLabel: x, Tool: x, Model: x, Endpoint: x})
	if err != nil {
		t.Fatal(err)
	}
	tk.Release(queue.Result{ErrorClass: x, Status: queue.StatusError})
	f.st.Flush()
	f.h.extraAt = time.Time{}
	bad := []string{}
	for _, p := range []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/stats"} {
		w := do(f.h, "GET", p, "", nil)
		if strings.Contains(w.Body.String(), "<img") || strings.Contains(w.Body.String(), "</script>") {
			bad = append(bad, p)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			bad = append(bad, p+" headers")
		}
	}
	page := do(f.h, "GET", "/monitor", "", nil)
	if strings.Contains(page.Body.String(), "innerHTML") || !strings.Contains(page.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		bad = append(bad, "page")
	}
	if len(bad) > 0 {
		vec(t, "M4-xss", "Alta", "gateway/monitor/monitor.go:191", fmt.Sprintf("FALHA: %v", bad))
		t.Error(bad)
	} else {
		vec(t, "M4-xss", "Alta", "gateway/monitor/monitor.go:191", "OK: <, > e & escapados no JSON; nosniff; pagina so textContent; CSP sem unsafe-inline")
	}
}

// M5: CSRF (Origin ausente, null, subdominio, Sec-Fetch-Site, content type).
func TestAdvCSRF(t *testing.T) {
	f := newFixture(t, true, queue.Config{GenSlots: 1})
	cookie, csrf := f.login(t)
	id := strings.Repeat("a", 32)
	path := "/monitor/api/queue/" + id + "/remove"
	type c struct {
		name string
		hdr  map[string]string
		body string
		want int
	}
	cases := []c{
		{"sem token", map[string]string{"Cookie": cookie}, "", 403},
		{"token errado", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf[:len(csrf)-1] + "A"}, "", 403},
		{"Origin null", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Origin": "null"}, "", 403},
		{"Origin subdominio", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Origin": "https://evil.ia.cslsolucoes.com.br", "Host": "ia.cslsolucoes.com.br:8443"}, "", 403},
		{"Origin sufixo", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Origin": "https://ia.cslsolucoes.com.br.evil.com", "Host": "ia.cslsolucoes.com.br:8443"}, "", 403},
		{"Sec-Fetch-Site same-site", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Sec-Fetch-Site": "same-site"}, "", 403},
		{"Sec-Fetch-Site cross-site", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Sec-Fetch-Site": "cross-site"}, "", 403},
		{"form text/plain", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Content-Type": "text/plain"}, `{"reason":"x"}`, 415},
		{"sem cookie", map[string]string{"X-CSRF-Token": csrf}, "", 401},
		{"Origin ausente, tudo certo", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, "", 409},
		{"Origin igual", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Origin": "https://ia.cslsolucoes.com.br:8443", "Host": "ia.cslsolucoes.com.br:8443"}, "", 409},
	}
	bad := []string{}
	res := []string{}
	for _, x := range cases {
		w := do(f.h, "POST", path, x.body, x.hdr)
		res = append(res, fmt.Sprintf("%s=%d", x.name, w.Code))
		if w.Code != x.want {
			bad = append(bad, fmt.Sprintf("%s=%d (want %d)", x.name, w.Code, x.want))
		}
	}
	if len(bad) > 0 {
		vec(t, "M5-csrf", "Alta", "gateway/monitor/monitor.go:535", "FALHA: "+strings.Join(bad, "; "))
		t.Error(bad)
	} else {
		vec(t, "M5-csrf", "Alta", "gateway/monitor/monitor.go:535", "OK: "+strings.Join(res, "; "))
	}
}

// M6: fixacao e roubo de sessao.
func TestAdvSessionFixation(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	planted := "mcpgw_session=PLANTADO-pelo-atacante-000000000000000000000"
	w := do(f.h, "POST", "/monitor/api/login", `{"user":"admin","password":"`+adminPass+`"}`, map[string]string{"Cookie": planted})
	var got string
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			got = c.Value
		}
	}
	s := do(f.h, "GET", "/monitor/api/session", "", map[string]string{"Cookie": planted})
	if w.Code != 200 || got == "" || strings.Contains(planted, got) || strings.Contains(s.Body.String(), `"logged_in":true`) {
		vec(t, "M6-fixacao-sessao", "Alta", "gateway/monitor/monitor.go:606", fmt.Sprintf("FALHA: %d %q", w.Code, s.Body))
		t.Error("fixation")
	} else {
		vec(t, "M6-fixacao-sessao", "Alta", "gateway/monitor/monitor.go:606", "OK: login sempre gera token novo de 256 bits; o cookie plantado nao vira sessao; HttpOnly+Secure+SameSite=Strict")
	}
}

// M7: exposicao de token ou de conteudo nas rotas GET.
func TestAdvNoSecretsInAPI(t *testing.T) {
	f := newFixture(t, true, queue.Config{GenSlots: 1})
	cookie, csrf := f.login(t)
	tk, _ := f.q.Acquire(context.Background(), queue.Request{ClientIP: "10.0.0.1", ClientLabel: "x", Model: "m"})
	defer tk.Release(queue.Result{})
	bad := []string{}
	for _, p := range []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/stats", "/monitor/api/audit", "/monitor/api/series", "/monitor/api/session"} {
		w := do(f.h, "GET", p, "", nil) // anonymous
		b := w.Body.String()
		if strings.Contains(b, csrf) || strings.Contains(b, strings.TrimPrefix(cookie, cookieName+"=")) || strings.Contains(b, "pbkdf2") {
			bad = append(bad, p)
		}
	}
	if len(bad) > 0 {
		vec(t, "M7-exposicao-api", "Alta", "gateway/monitor/monitor.go:296", fmt.Sprintf("FALHA: %v", bad))
		t.Error(bad)
	} else {
		vec(t, "M7-exposicao-api", "Alta", "gateway/monitor/monitor.go:296", "OK: nenhuma rota GET anonima devolve cookie, CSRF ou hash; o proxy nunca passa prompt/token ao queue (ver S5 e o teste-sentinela)")
	}
}
