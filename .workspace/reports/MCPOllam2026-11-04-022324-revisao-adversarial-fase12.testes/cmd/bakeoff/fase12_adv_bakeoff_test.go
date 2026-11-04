package main

// Revisão adversarial da etapa 12.9 (Fase 12), revisor B: o bakeoff `-suites erros`
// (C-5.1 a C-5.16, C-4.13, C-4.14, C-3.10a). Cada vetor imprime uma linha VEC.
// Depende dos helpers de erros_test.go, erros_fix_test.go, erros_eval_test.go e
// erros_melhora_test.go (newFixture, synthCase, omitiu, formato, blindCase, fill,
// answer, scenarioPolicy, syntheticSet, writeScore, sidecarFiles, readFile,
// secretMarkers, caseKeyRe, cat, cmp, goodS23, instrJSON, instrText, sysText).

import (
	"bytes"
	"database/sql"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func vec12B(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res != "OK" {
		t.Errorf("%s: %s", id, desc)
	}
}

// adv12BCounter is a fake "external host": it only counts what reaches it.
func adv12BCounter(t *testing.T) (*httptest.Server, *atomic.Int64) {
	var n atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		io.Copy(io.Discard, r.Body)
	}))
	t.Cleanup(s.Close)
	return s, &n
}

// ---- B1: o conteúdo não sai da rede ------------------------------------------------------------------

func TestAdv12BReplayURLRefusals(t *testing.T) {
	bad := []string{
		"https://gateway.interno", "https://localhost.", "https://LOCALHOST:1", "https://10.0.0.1.nip.io",
		"https://8.8.8.8", "https://[2001:4860:4860::8888]", "https://[fe80::1]", "https://[fe80::1%25eth0]",
		"https://[fd00::1%25eth0]", "https://[::ffff:8.8.8.8]", "https://0.0.0.0", "https://[::]",
		"https://2130706433", "https://0177.0.0.1", "https://0x7f.1", "https://0x0a.0.0.1", "https://010.0.0.1",
		"https://100.64.0.1", "https://169.254.169.254", "https://[64:ff9b::808:808]", "https://[2002:a00:1::]",
		"http://10.0.0.1:11500", "http://[fd00::1]:8443", "https://user:pw@8.8.8.8", "http://[::1%25lo]:1",
	}
	var leaked []string
	for _, u := range bad {
		for _, remote := range []bool{false, true} {
			if _, err := newReplayClient(u, remote, time.Second); err == nil {
				leaked = append(leaked, fmt.Sprintf("%s(remoto=%v)", u, remote))
			}
		}
	}
	good := []string{"https://10.0.0.1", "https://172.16.0.1:8443", "https://[fd12::1]:8443", "https://[::ffff:10.0.0.1]"}
	var refusedGood []string
	for _, u := range good {
		if _, err := newReplayClient(u, true, time.Second); err != nil {
			refusedGood = append(refusedGood, u)
		}
	}
	switch {
	case len(leaked) > 0:
		vec12B(t, "B1-01", "FALHA", "Bloqueador erros_replay.go:79-97: URL aceita para o replay: "+strings.Join(leaked, ", "))
	case len(refusedGood) > 0:
		vec12B(t, "B1-01", "FALHA", "Menor erros_replay.go:79-97: IP privado literal em https recusado: "+strings.Join(refusedGood, ", "))
	default:
		vec12B(t, "B1-01", "OK", fmt.Sprintf("%d URLs recusadas com e sem -gateway-remoto (nome, IP público v4/v6, link-local, zona %%25, ::ffff:8.8.8.8, 0.0.0.0, [::], decimal/octal/hex, CGNAT, metadata, NAT64, 6to4, http privado, userinfo); %d IPs privados em https aceitos", len(bad), len(good)))
	}
}

func TestAdv12BDialControlOnResolvedAddress(t *testing.T) {
	refuse := []string{"8.8.8.8:443", "[::ffff:8.8.8.8]:443", "0.0.0.0:80", "[::]:80", "[fe80::1%eth0]:80", "169.254.169.254:80",
		"100.64.0.1:1", "[64:ff9b::808:808]:1", "[2002:a00:1::]:1", "[2001:4860:4860::8888]:443", "localhost:80", "garbage"}
	var through []string
	for _, a := range refuse {
		if connControl(true)("tcp", a, nil) == nil {
			through = append(through, a)
		}
	}
	if connControl(false)("tcp", "10.0.0.1:1", nil) == nil {
		through = append(through, "10.0.0.1 sem -gateway-remoto")
	}
	for _, a := range []string{"10.0.0.1:1", "[fd00::1]:1", "127.0.0.1:1", "[::1]:1"} {
		if connControl(true)("tcp", a, nil) != nil {
			through = append(through, "recusou "+a)
		}
	}
	// the name "localhost" reaches the Control already resolved: a successful call proves it
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":"x","done":true,"done_reason":"stop"}`+"\n")
	}))
	defer gw.Close()
	_, port, _ := net.SplitHostPort(gw.Listener.Addr().String())
	rc, err := newReplayClient("http://localhost:"+port, false, 5*time.Second)
	if err == nil {
		_, err = rc.do(t.Context(), "/api/generate", []byte(`{"prompt":"a"}`))
	}
	if len(through) > 0 || err != nil {
		vec12B(t, "B1-02", "FALHA", fmt.Sprintf("Bloqueador erros_replay.go:110-118: Control deixou passar %v (localhost err=%v)", through, err))
		return
	}
	vec12B(t, "B1-02", "OK", fmt.Sprintf("Dialer.Control recusa %d endereços já resolvidos (público, ::ffff:público, 0.0.0.0, link-local com zona, metadata, CGNAT, NAT64, 6to4, nome cru); 'localhost' chega resolvido ao Control e a chamada passa (DNS rebinding cai no Control)", len(refuse)))
}

func TestAdv12BRedirectsNeverFollowed(t *testing.T) {
	ext, hits := adv12BCounter(t)
	var fails []string
	for _, code := range []int{301, 302, 303, 307, 308} {
		gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, ext.URL+"/api/generate?d=SEGREDO", code)
		}))
		rc, err := newReplayClient(gw.URL, false, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rc.do(t.Context(), "/api/generate", []byte(`{"prompt":"SEGREDO-CLIENTE"}`)); err == nil {
			fails = append(fails, fmt.Sprintf("%d aceito", code))
		}
		if _, err := rc.embed(t.Context(), "nomic-embed-text", "SEGREDO-CLIENTE"); err == nil {
			fails = append(fails, fmt.Sprintf("embed %d aceito", code))
		}
		gw.Close()
	}
	if hits.Load() != 0 || len(fails) > 0 {
		vec12B(t, "B1-03", "FALHA", fmt.Sprintf("Bloqueador erros_replay.go:100-104: redirect seguido (host externo recebeu %d pedidos; %v)", hits.Load(), fails))
		return
	}
	vec12B(t, "B1-03", "OK", "301/302/303/307/308 para host externo, no replay e no embed: erro e 0 pedidos no host externo")
}

func adv12BPrivateIPv4() net.IP {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if in, ok := a.(*net.IPNet); ok && in.IP.To4() != nil && in.IP.IsPrivate() {
			return in.IP.To4()
		}
	}
	return nil
}

func TestAdv12BProxyEnvironmentIgnored(t *testing.T) {
	ip := adv12BPrivateIPv4()
	if ip == nil {
		t.Skip("sem IPv4 privado na máquina: o vetor B1-04 não pode ser exercitado")
	}
	var proxyHits atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1) // CONNECT (https) or absolute-URI (http)
		w.WriteHeader(502)
	}))
	defer proxy.Close()
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(k, proxy.URL)
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	// a closed port on the machine's own LAN address: the direct dial is refused locally, no packet leaves
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	l.Close()
	target := "https://" + net.JoinHostPort(ip.String(), port)
	rc, err := newReplayClient(target, true, 3*time.Second)
	if err != nil {
		t.Fatalf("%s: %v", target, err)
	}
	_, derr := rc.do(t.Context(), "/api/generate", []byte(`{"prompt":"SEGREDO-CLIENTE"}`))
	// control: the same target through an explicit proxy reaches the counter (the counter works)
	pu, _ := url.Parse(proxy.URL)
	ctl := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}, Timeout: 3 * time.Second}
	req, _ := http.NewRequest("POST", target+"/api/generate", strings.NewReader("x"))
	if resp, err := ctl.Do(req); err == nil {
		resp.Body.Close()
	}
	ctlHits := proxyHits.Load()
	if derr == nil || ctlHits != 1 {
		vec12B(t, "B1-04", "FALHA", fmt.Sprintf("Bloqueador erros_replay.go:99: replay err=%v proxyHits=%d (esperado 1, só o controle)", derr, ctlHits))
		return
	}
	vec12B(t, "B1-04", "OK", fmt.Sprintf("HTTP(S)_PROXY/ALL_PROXY no ambiente e NO_PROXY vazio: o replay para %s discou direto (erro %q) e o proxy recebeu só o pedido do controle (1)", ip, oneLine(derr.Error(), 60)))
}

func TestAdv12BBaseURLPathIsNotAnEndpointBypass(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.RequestURI())
		mu.Unlock()
		fmt.Fprint(w, `{"response":"x","done":true,"done_reason":"stop"}`+"\n")
	}))
	defer gw.Close()
	rc, err := newReplayClient(gw.URL+"/api/pull?nome=", false, 5*time.Second)
	if err != nil {
		vec12B(t, "B1-05", "OK", "-gateway-url com caminho/consulta recusada: "+err.Error())
		return
	}
	_, _ = rc.do(t.Context(), "/api/generate", []byte(`{"prompt":"SEGREDO"}`))
	mu.Lock()
	defer mu.Unlock()
	if len(paths) == 1 && paths[0] != "/api/generate" {
		vec12B(t, "B1-05", "FALHA", fmt.Sprintf("Menor erros_replay.go:81 e :169: o caminho/consulta da -gateway-url não é recusado e o pedido do caso foi a %q (mesmo host, mas fora da lista replayEndpoints da linha 36)", paths[0]))
		return
	}
	vec12B(t, "B1-05", "OK", fmt.Sprintf("caminhos vistos: %v", paths))
}

// ---- B2: gate aprovando melhora falsa ----------------------------------------------------------------

// adv12BWilsonLo is an independent form of the Wilson lower bound.
func adv12BWilsonLo(k, n int) float64 {
	z := 1.959963984540054
	nf, kf := float64(n), float64(k)
	p := kf / nf
	return (2*nf*p + z*z - z*math.Sqrt(z*z+4*nf*p*(1-p))) / (2 * (nf + z*z))
}

func TestAdv12BWilsonAndTargetDecisionSweep(t *testing.T) {
	maxd := 0.0
	for n := 1; n <= 200; n++ {
		for k := 0; k <= n; k++ {
			lo, _ := wilson(k, n)
			want := adv12BWilsonLo(k, n)
			if k == 0 {
				want = 0
			}
			if d := math.Abs(lo - want); d > maxd {
				maxd = d
			}
		}
	}
	if maxd > 1e-9 {
		vec12B(t, "B2-01", "FALHA", fmt.Sprintf("Importante erros_score.go:446-463: Wilson difere da fórmula independente em %.3g", maxd))
	} else {
		vec12B(t, "B2-01", "OK", fmt.Sprintf("MEDIDA Wilson (limite inferior) = fórmula independente em n=1..200, todos os k (desvio máx %.2g)", maxd))
	}
	var wrong []string
	checked := 0
	for _, n := range []int{30, 31, 47, 100} {
		for a := 0; a <= n; a++ {
			for c := 0; c <= n; c++ {
				lo := adv12BWilsonLo(c, n)
				point := float64(a) / float64(n)
				if math.Abs(lo-point) < 1e-9 {
					continue
				}
				want := (c-a)*100 >= 10*n && lo > point
				o := evaluate(cmp("omitiu", cat("omitiu", n, a, c)))
				checked++
				if o.Accepted != want && len(wrong) < 5 {
					wrong = append(wrong, fmt.Sprintf("n=%d %d->%d aceito=%v", n, a, c, o.Accepted))
				}
			}
		}
	}
	if len(wrong) > 0 {
		vec12B(t, "B2-02", "FALHA", "Importante erros_report.go:165-173: decisão C-5.10 difere do oráculo: "+strings.Join(wrong, "; "))
	} else {
		vec12B(t, "B2-02", "OK", fmt.Sprintf("MEDIDA C-5.10 (≥10 pontos e limite inferior > ponto, estrito) igual ao oráculo independente em %d combinações (n=30,31,47,100)", checked))
	}
}

func TestAdv12BBoundariesOfNAndDrop(t *testing.T) {
	var bad []string
	// C-5.8 report/decide boundaries in the report text
	for n, want := range map[int]string{19: "N < 20: não reportada", 20: "reportada; N < 30, não decide", 29: "reportada; N < 30, não decide", 30: "| decide |"} {
		in := reportInput{Mode: "base", RetratoID: strings.Repeat("a", 32), Cats: []catRes{cat("omitiu", n, n/2, 0)}}
		if !strings.Contains(renderReport(in, evaluate(in)), want) {
			bad = append(bad, fmt.Sprintf("N=%d sem %q", n, want))
		}
	}
	// target at 29 never decides; at 30 it does
	if o := evaluate(cmp("omitiu", cat("omitiu", 29, 0, 29))); o.Code != exitGate || o.Accepted {
		bad = append(bad, "alvo N=29 decidiu")
	}
	if o := evaluate(cmp("omitiu", cat("omitiu", 30, 0, 30))); !o.Accepted {
		bad = append(bad, "alvo N=30 0->30 não aceito")
	}
	// C-5.11: 3.01 points (9 of 299) rejects, 3.00 (9 of 300) does not
	if o := evaluate(cmp("omitiu", cat("omitiu", 30, 0, 30), cat("formato_invalido", 299, 200, 191))); o.Accepted {
		bad = append(bad, "queda de 3,01 pontos aceita")
	}
	if o := evaluate(cmp("omitiu", cat("omitiu", 30, 0, 30), cat("formato_invalido", 300, 200, 191))); !o.Accepted {
		bad = append(bad, "queda de exatamente 3,00 pontos recusada")
	}
	// C-5.12: S2 drop of 2.01 points rejects, 2.00 does not
	in := cmp("omitiu", cat("omitiu", 30, 0, 30))
	in.CandS23 = &s23{S2OK: 8799, S2Tot: 10000, S3OK: 28, S3Tot: 30}
	in.AtualS23 = &s23{S2OK: 9000, S2Tot: 10000, S3OK: 28, S3Tot: 30}
	if o := evaluate(in); o.Accepted {
		bad = append(bad, "S2 caiu 2,01 e foi aceito")
	}
	in.CandS23 = &s23{S2OK: 8800, S2Tot: 10000, S3OK: 28, S3Tot: 30}
	if o := evaluate(in); !o.Accepted {
		bad = append(bad, "S2 caiu 2,00 e foi recusado")
	}
	if len(bad) > 0 {
		vec12B(t, "B2-03", "FALHA", "Importante erros_report.go:87-195: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "B2-03", "OK", "N 19/20/29/30 (reporta/decide), queda de 3,01 vs 3,00 pontos (C-5.11) e S2 2,01 vs 2,00 (C-5.12) nas fronteiras certas, com inteiros")
}

func TestAdv12BUnequalNIsSkippedNotRejected(t *testing.T) {
	// a target whose candidate has fewer cases than the current model (a resultado-*.json that
	// is corrupt or edited by hand, read by mode apurar): the comparison is not made at all
	tgt := cat("omitiu", 30, 15, 15) // no gain
	tgt.Cand.N = 29
	o1 := evaluate(cmp("omitiu", tgt))
	drop := cat("formato_invalido", 30, 30, 0) // a 100-point drop
	drop.Cand.N = 31
	o2 := evaluate(cmp("omitiu", cat("omitiu", 30, 0, 30), drop))
	if o1.Accepted || o2.Accepted {
		vec12B(t, "B2-04", "FALHA", fmt.Sprintf("Menor erros_report.go:161: categoria com Cand.N != Atual.N é pulada (continue) em vez de falhar o gate: alvo sem ganho aceito=%v (%q); outra caindo 100 pontos aceita=%v. Só alcançável por resultado-*.json corrompido/editado lido pelo modo apurar (erros_apurar.go:132), mas é fail-open", o1.Accepted, strings.Join(o1.Rejects, "|"), o2.Accepted))
		return
	}
	vec12B(t, "B2-04", "OK", "N diferente entre atual e candidato não é aceito")
}

// adv12BGW answers each case by whether the request carries a system text (the candidate of
// stage 12.8, which is the SAME model name with the frozen instructions).
func adv12BGW(t *testing.T, cs []synthCase, right func(hasSystem bool, c synthCase) bool) *httptest.Server {
	byID := map[string]synthCase{}
	for _, c := range cs {
		byID[synthID(c.N)] = fill(c)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]json.RawMessage
		_ = json.Unmarshal(raw, &body)
		m := caseKeyRe.FindSubmatch(raw)
		if m == nil {
			w.WriteHeader(404)
			return
		}
		c := byID[string(m[1])]
		_, hasSys := body["system"]
		text := c.Wrong
		if right(hasSys, c) {
			text = c.Right
		}
		tb, _ := json.Marshal(text)
		fmt.Fprintf(w, `{"response":%s,"done":true,"done_reason":"stop"}`+"\n", tb)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestAdv12BS2S3OfACandidateWithInstructionsIsNeverMeasured(t *testing.T) {
	var cs []synthCase
	for i := 1; i <= 30; i++ {
		cs = append(cs, omitiu(i))
	}
	for i := 31; i <= 60; i++ {
		cs = append(cs, formato(i))
	}
	f := newFixture(t, cs, scenarioPolicy(false))
	gw := adv12BGW(t, cs, func(sys bool, c synthCase) bool {
		if c.Cat == "omitiu" {
			return sys || c.N <= 15
		}
		return c.N-30 <= 20
	})
	instr := f.instrFile(instrJSON)
	rid, _ := f.retratoWith("", instr)
	// S2/S3 measured only for the plain current model; nobody measured them WITH the instructions
	writeScore(t, f.out, "qwen3-ia", 90, 100, 28, 30)
	ce := f.opts("comparar")
	// the gateway accepts only qwen3-ia / nomic-embed-text (allowlist), so the candidate of a
	// melhora (12.8) is necessarily the current model name with the transform
	ce.RetratoID, ce.Candidato, ce.Alvo, ce.InstrFile, ce.GatewayURL = rid, "qwen3-ia", "omitiu", instr, gw.URL
	code, out := f.run(ce)
	if !strings.Contains(out, "| omitiu | 30 | 50,0%") {
		t.Fatalf("fixture: the comparison did not run as planned (code %d)\n%s", code, out)
	}
	if code == exitOK && strings.Contains(out, "MELHORA ACEITA") {
		vec12B(t, "B2-05", "FALHA", "Importante erros_report.go:378-379 (e erros.go:271, sem recusa de -candidato = -atual): no comparar da melhora 12.8 o candidato é o mesmo nome de modelo (o gateway só aceita qwen3-ia), então o S2/S3 do candidato é lido do MESMO <out>/qwen3-ia/score.json do atual; o C-5.12 compara o modelo sem instruções com ele mesmo e a melhora foi ACEITA sem o S2/S3 com as instruções jamais ter sido medido")
		return
	}
	vec12B(t, "B2-05", "OK", fmt.Sprintf("sem S2/S3 do candidato com instruções o gate não aceita (code %d)", code))
}

func TestAdv12BStaleOrForeignScoreFailsTheGate(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	writeScore(t, f.out, "qwen3-ia", 9, 10, 9, 10)
	// candidate: a score.json of 3 days ago, and then one written for another model
	dir := filepath.Join(f.out, safeName("qwen3-ia-v2"))
	os.MkdirAll(dir, 0o700)
	var bad []string
	for name, body := range map[string]string{
		"antigo":       `{"modelo":"qwen3-ia-v2","gerado_em":"2026-10-31T14:00:00Z","s2":[{"numero_e_titulo_ok":9,"total_gabarito":10}],"s3":{"a":{"ok":true}}}`,
		"outro modelo": `{"modelo":"qwen3-ia","gerado_em":"2026-11-03T14:00:00Z","s2":[{"numero_e_titulo_ok":9,"total_gabarito":10}],"s3":{"a":{"ok":true}}}`,
		"sem S3":       `{"modelo":"qwen3-ia-v2","gerado_em":"2026-11-03T14:00:00Z","s2":[{"numero_e_titulo_ok":9,"total_gabarito":10}],"s3":{}}`,
	} {
		os.WriteFile(filepath.Join(dir, "score.json"), []byte(body), 0o600)
		ce := f.opts("comparar")
		ce.RetratoID, ce.Candidato, ce.Alvo = rid, "qwen3-ia-v2", "omitiu"
		code, out := f.run(ce)
		if code != exitGate || !strings.Contains(out, "S2 e o S3 da Fase 8 não foram medidos") {
			bad = append(bad, fmt.Sprintf("%s: code %d", name, code))
		}
	}
	if len(bad) > 0 {
		vec12B(t, "B2-06", "FALHA", "Importante erros_report.go:378 / erros.go:151-195: S2/S3 inválido tratado como medido: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "B2-06", "OK", "score.json do candidato antigo (>48 h), de outro modelo ou sem S3: o C-5.12 vira \"não medido\" e o gate falha (nunca \"sem queda\")")
}

func TestAdv12BMissingRoundIsNotTwoOfThree(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.makeRetrato()
	// case 1 is right in rounds 1 and 2 (scenario) and its round 3 fails at the gateway
	f.gw.onReq = nil
	base := f.gw.srv.Config.Handler
	f.gw.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if bytes.Contains(raw, []byte("PEDIDO-"+synthID(1))) && bytes.Contains(raw, []byte(`"seed":3`)) {
			w.WriteHeader(500)
			return
		}
		base.ServeHTTP(w, r)
	})
	eo := f.opts("base")
	eo.RetratoID = rid
	code, out := f.run(eo)
	if code != exitGate || !strings.Contains(out, "1 caso(s) não puderam ser medidos") {
		vec12B(t, "B2-07", "FALHA", fmt.Sprintf("Importante erros.go:401-452: uma rodada faltando foi contada (code %d)", code))
		return
	}
	vec12B(t, "B2-07", "OK", "a rodada 3 de um caso falhou (500): o caso não é medido e o gate falha; 2 rodadas nunca valem como \"2 de 3\"")
}

func TestAdv12BDuplicateOccurrenceOfTheSameJob(t *testing.T) {
	cs := syntheticSet()
	f := newFixture(t, cs, scenarioPolicy(false))
	// a second confirmed test occurrence of the very same job (C-1.6: the sample is shared)
	dup := strings.Repeat("d", 32)
	f.dbExec(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado,categoria,resposta_correta,tool,job_do_erro,particao,grupo_id) SELECT ?,criada_em,atualizada_em,estado,categoria,resposta_correta,tool,job_do_erro,particao,grupo_id FROM ocorrencias WHERE id=?`, dup, synthID(1))
	f.dbExec(`INSERT INTO ocorrencia_jobs(ocorrencia_id,job_id,papel,backend,ordem) VALUES(?,?,?,?,0)`, dup, synthJob(1), "unico", "primario")
	_, out := f.run(f.opts("retrato"))
	if !strings.Contains(out, "RETRATO categoria=omitiu casos=31") {
		vec12B(t, "B2-08", "OK", "MEDIDA a segunda ocorrência do mesmo job não entrou duas vezes: "+oneLine(out, 200))
		return
	}
	vec12B(t, "B2-08", "OK", "MEDIDA duas ocorrências confirmadas do MESMO job_do_erro entram no retrato como 2 casos (omitiu=31): o mesmo pedido é medido 2x. Coerente com C-1.1 (a unidade é a ocorrência) e C-4.5 (mesmo grupo, mesmo lado), mas infla o N rumo ao mínimo de 30 (C-5.8); ver \"só leitura\"")
}

func TestAdv12BNumberScoring(t *testing.T) {
	var issues []string
	// unicode minus (U+2212) in the correct answer is read as +5; a model saying +5 passes
	if v := scoreNumber("RESULTADO: 5", "RESULTADO: \u22125"); v.Applicable && v.Pass {
		issues = append(issues, "want \"RESULTADO: −5\" (U+2212) lido como 5 e a resposta 5 passou")
	}
	if v := scoreNumber("5", "\u22125"); v.Applicable && v.Pass {
		issues = append(issues, "want \"−5\" lido como 5")
	}
	if len(issues) > 0 {
		vec12B(t, "B2-09", "FALHA", "Menor erros_score.go:290-291 e :296-307: o sinal de menos Unicode (U+2212, comum em texto colado) não é reconhecido na resposta certa, que vira positiva; a resposta do modelo com o sinal trocado passa: "+strings.Join(issues, "; "))
	} else {
		vec12B(t, "B2-09", "OK", "sinal de menos Unicode tratado")
	}
	var bad []string
	for _, c := range []struct {
		got, want string
		applicable, pass bool
	}{
		{"RESULTADO: 1500", "RESULTADO: 1,500", false, false},           // ambiguous correct answer
		{"RESULTADO: 7 ... na verdade RESULTADO: 8", "RESULTADO: 7", true, false}, // the last one counts
		{"RESULTADO: ５", "RESULTADO: 5", true, false},                    // fullwidth digit is not a number
		{"RESULTADO: 12,0", "12", true, true},
		{"são 12 itens (seção 3)", "12", true, false},
	} {
		v := scoreNumber(c.got, c.want)
		if v.Applicable != c.applicable || v.Pass != c.pass {
			bad = append(bad, fmt.Sprintf("%q vs %q: %+v", c.got, c.want, v))
		}
	}
	if len(bad) > 0 {
		vec12B(t, "B2-10", "FALHA", "Menor erros_score.go:310-416: "+strings.Join(bad, "; "))
	} else {
		vec12B(t, "B2-10", "OK", "RESULTADO ambíguo (1,500) inaplicável; o último RESULTADO vale; dígito largo não conta; 12,0 = 12; prosa termina no último número")
	}
}

func TestAdv12BJSONAndItems(t *testing.T) {
	var bad []string
	if v := scoreJSON(`{"a":1,"\u0061":2}`, `{"a":2}`); v.Pass {
		bad = append(bad, "chave duplicada por escape passou")
	}
	if v := scoreJSON(`{"x":1}{"x":1}`, `{"x":1}`); v.Pass {
		bad = append(bad, "dois objetos passaram")
	}
	if v := scoreJSON("```json\n{\"x\":1}\n```", `{"x":1}`); v.Pass {
		bad = append(bad, "cerca de código passou (C-5.3 exige json.Valid no texto)")
	}
	if v := scoreJSON(`{"x":["a","b"]}`, `{"x":["b","a"]}`); v.Pass {
		bad = append(bad, "ordem de array ignorada")
	}
	start := time.Now()
	v := scoreJSON(`{"x":1e999999999,"y":1e999999}`, `{"x":1,"y":1}`)
	if d := time.Since(start); d > 2*time.Second || v.Pass {
		bad = append(bad, fmt.Sprintf("expoente gigante: %v pass=%v", d, v.Pass))
	}
	deep := strings.Repeat("[", 200000) + strings.Repeat("]", 200000)
	start = time.Now()
	if v := scoreJSON(deep, `[]`); v.Pass || time.Since(start) > 2*time.Second {
		bad = append(bad, "aninhamento profundo")
	}
	// items (C-5.2): repeated items, missing items and look-alikes never pass
	want := "item-1\nitem-2\nitem-3"
	for _, got := range []string{"item-1\nitem-1\nitem-2\nitem-3", "item-1\nitem-2", "item-1\nitem-2\nitem-3\nitem-4", "item-1\nitem-2\nitem-3\u200b", "1. item-1\n2. item-2\n3. item-3", ""} {
		if scoreItems(got, want).Pass {
			bad = append(bad, fmt.Sprintf("itens %q passaram", got))
		}
	}
	if !scoreItems("- item-3\n* item-1\n+ item-2\r\n\n", want).Pass || !scoreItems(`["item-1","item-2","item-3"]`, want).Pass {
		bad = append(bad, "lista certa (marcadores, CRLF, JSON) reprovada")
	}
	if len(bad) > 0 {
		vec12B(t, "B2-11", "FALHA", "Importante erros_score.go:124-285: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "B2-11", "OK", "JSON: chave duplicada por escape, 2 objetos, cerca, ordem de array, 1e999999999 (<2 s) e 200 mil níveis reprovam; itens repetidos/faltando/extras/zero-width/numerados reprovam e a lista certa passa")
}

func TestAdv12BBaselineCannotBeReplacedOrBorrowed(t *testing.T) {
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid := f.runBase()
	bp := filepath.Join(f.out, "erros", rid, "linha-de-base.json")
	before := sha256.Sum256([]byte(readFile(t, bp)))
	eo := f.opts("base")
	eo.RetratoID = rid
	code, _ := f.run(eo)
	after := sha256.Sum256([]byte(readFile(t, bp)))
	var bad []string
	if code != exitUsage || before != after {
		bad = append(bad, fmt.Sprintf("2ª base: code %d, arquivo mudou=%v", code, before != after))
	}
	// a baseline of another retrato, of another model, or unreadable is refused by comparar
	other := f.makeRetrato()
	od := filepath.Join(f.out, "erros", other)
	os.MkdirAll(od, 0o700)
	for name, content := range map[string][]byte{
		"de outro retrato": []byte(readFile(t, bp)),
		"de outro modelo":  mustJSON(baselineFile{RetratoID: other, Modelo: "qwen3-ia-think"}),
		"ilegível":         []byte("{"),
	} {
		os.WriteFile(filepath.Join(od, "linha-de-base.json"), content, 0o600)
		f.noBaseline = true
		ce := f.compareOpts(other, "omitiu")
		f.gw.reset()
		c, _ := f.run(ce)
		if c != exitUsage || len(f.gw.requests()) != 0 {
			bad = append(bad, fmt.Sprintf("linha de base %s: code %d, %d chamadas", name, c, len(f.gw.requests())))
		}
	}
	if len(bad) > 0 {
		vec12B(t, "B2-12", "FALHA", "Importante erros.go:285-302: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "B2-12", "OK", "linha de base não é sobrescrita (2ª base = FATAL, arquivo idêntico); comparar recusa, sem chamar o gateway, a base de outro retrato, de outro modelo ou ilegível")
}

// ---- B3: retrato invalidado, quarentena, melhora_* -------------------------------------------------------

func TestAdv12BRetratoQuarantineAndMelhora(t *testing.T) {
	var bad []string
	// (a) melhora=1 before the retrato: the case is not frozen (C-3.10a)
	f := newFixture(t, syntheticSet(), scenarioPolicy(false))
	f.dbExec(`UPDATE amostras SET melhora=1, melhora_instrucoes_id='i1' WHERE job_id=?`, synthJob(4))
	rid := f.makeRetrato()
	cnt := 0
	func() {
		db := adv12BOpen(t, f.db)
		defer db.Close()
		db.QueryRow(`SELECT COUNT(*) FROM retrato_itens WHERE retrato_id=? AND ocorrencia_id=?`, rid, synthID(4)).Scan(&cnt)
	}()
	if cnt != 0 {
		bad = append(bad, "caso com melhora_* entrou no retrato")
	}
	// (b) quarantine / melhora after the retrato: base fails the gate without calling the gateway
	for name, q := range map[string]string{
		"quarentena": `UPDATE ocorrencias SET particao='quarentena' WHERE id='` + synthID(5) + `'`,
		"melhora":    `UPDATE amostras SET melhora=1, melhora_indice_id='x' WHERE job_id='` + synthJob(5) + `'`,
	} {
		f := newFixture(t, syntheticSet(), scenarioPolicy(false))
		rid := f.makeRetrato()
		f.dbExec(q)
		eo := f.opts("base")
		eo.RetratoID = rid
		code, out := f.run(eo)
		if code != exitGate || len(f.gw.requests()) != 0 || strings.Contains(out, "LINHA DE BASE REGISTRADA") {
			bad = append(bad, fmt.Sprintf("%s depois do retrato: code %d, %d chamadas", name, code, len(f.gw.requests())))
		}
	}
	// (c) quarantine in the middle of the round, and melhora between base and comparar
	f2 := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid2 := f2.makeRetrato()
	var once sync.Once
	f2.gw.onReq = func(n int) {
		if n == 30 {
			once.Do(func() { f2.dbExec(`UPDATE ocorrencias SET particao='quarentena' WHERE id=?`, synthID(50)) })
		}
	}
	eo := f2.opts("base")
	eo.RetratoID = rid2
	code, out := f2.run(eo)
	if code != exitGate || !strings.Contains(out, "retrato invalidado durante a rodada") {
		bad = append(bad, fmt.Sprintf("quarentena no meio da rodada: code %d", code))
	}
	if _, err := os.Stat(filepath.Join(f2.out, "erros", rid2, "linha-de-base.json")); err == nil {
		bad = append(bad, "linha de base registrada de retrato que entrou em quarentena")
	}
	f3 := newFixture(t, syntheticSet(), scenarioPolicy(false))
	rid3 := f3.runBase()
	f3.dbExec(`UPDATE amostras SET melhora=1 WHERE job_id=?`, synthJob(40))
	f3.gw.reset()
	code, _ = f3.run(f3.compareOpts(rid3, "omitiu"))
	if code != exitGate || len(f3.gw.requests()) != 0 {
		bad = append(bad, fmt.Sprintf("melhora entre base e comparar: code %d, %d chamadas", code, len(f3.gw.requests())))
	}
	// (d) a retrato whose cases cannot be scored (correct answer not JSON in formato_invalido)
	var cs []synthCase
	for i := 1; i <= 30; i++ {
		c := formato(i)
		c.Correct = "não é json SEGREDO"
		cs = append(cs, c)
	}
	f4 := newFixture(t, cs, scenarioPolicy(false))
	rid4 := f4.makeRetrato()
	eo = f4.opts("base")
	eo.RetratoID = rid4
	code, out = f4.run(eo)
	if code != exitGate || !strings.Contains(out, "(0 casos)") || len(f4.gw.requests()) != 0 {
		bad = append(bad, fmt.Sprintf("retrato só de inaplicáveis: code %d", code))
	}
	if len(bad) > 0 {
		vec12B(t, "B3-01", "FALHA", "Importante erros.go:303-316/462-465: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "B3-01", "OK", "melhora_* fora do retrato (C-3.10a); quarentena ou melhora depois do retrato, quarentena no meio da rodada e melhora entre base e comparar: gate falha, sem linha de base e (fora do meio da rodada) sem chamar o gateway; retrato só de casos inaplicáveis = \"0 casos\"")
}

// ---- B4: stream cortado ----------------------------------------------------------------------------------

func TestAdv12BStreamShapes(t *testing.T) {
	// (1) an invalid JSON chunk in the middle, then done: the chunk is silently dropped
	r, err := parseReply("/api/generate", strings.NewReader(`{"response":"item-1\n","done":false}`+"\n"+`{"response":"item-EXTRA-ERRADO\n","done":fa`+"\n"+`{"response":"item-2\nitem-3","done":true,"done_reason":"stop"}`+"\n"))
	if err == nil {
		v := scoreItems(r.Text, "item-1\nitem-2\nitem-3")
		vec12B(t, "B4-01", "FALHA", fmt.Sprintf("Menor erros_replay.go:255-256 e :276-277: uma linha JSON inválida NO MEIO do stream (não truncado) é ignorada e a resposta vale como medida (texto %q, omitiu pass=%v): o pedaço perdido continha o item errado e o caso passou", r.Text, v.Pass))
	} else {
		vec12B(t, "B4-01", "OK", "linha JSON inválida no meio do stream: erro")
	}
	// (2) a line that is not JSON at all in the middle
	r, err = parseReply("/api/generate", strings.NewReader(`{"response":"a","done":false}`+"\n<html>502 Bad Gateway</html>\n"+`{"response":"b","done":true,"done_reason":"stop"}`+"\n"))
	if err == nil {
		vec12B(t, "B4-02", "FALHA", fmt.Sprintf("Menor erros_replay.go:218: uma linha que não começa com '{' no meio do stream nativo é pulada sem erro e a resposta (%q) vale como medida", r.Text))
	} else {
		vec12B(t, "B4-02", "OK", "linha não-JSON no meio do stream: erro")
	}
	// (3) text after done
	r, err = parseReply("/api/generate", strings.NewReader(`{"response":"a","done":true,"done_reason":"stop"}`+"\n"+`{"response":"b","done":false}`+"\n"))
	vec12B(t, "B4-03", "OK", fmt.Sprintf("MEDIDA texto depois do done é somado (texto=%q err=%v, Final=%v); o Ollama não manda nada depois do done, por isso não conto como defeito", r.Text, err, r.Final))
	// (4) done:false only, EOF; and the connection breaking
	_, e1 := parseReply("/api/chat", strings.NewReader(`{"message":{"content":"x"},"done":false}`+"\n"))
	_, e2 := parseReply("/api/generate", io.MultiReader(strings.NewReader(`{"response":"x","done":false}`+"\n"), errReader{}))
	_, e3 := parseReply("/v1/chat/completions", strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"a\"},\"finish_reason\":null}]}\n"))
	if e1 == nil || e2 == nil || e3 == nil {
		vec12B(t, "B4-04", "FALHA", fmt.Sprintf("Importante erros_replay.go:238: stream sem fim aceito (%v %v %v)", e1, e2, e3))
	} else {
		vec12B(t, "B4-04", "OK", "done:false até o EOF, conexão quebrada e SSE com finish_reason null sem [DONE]: erro (caso não medido)")
	}
	// (5) done_reason length in a category other than truncado
	r, err = parseReply("/api/generate", strings.NewReader(`{"response":"item-1\nitem-2\nitem-3","done":true,"done_reason":"length"}`+"\n"))
	vec12B(t, "B4-05", "OK", fmt.Sprintf("MEDIDA done_reason=length é um fim normal: pontuado (omitiu pass=%v err=%v) e, em truncado, reprova (pass=%v); é comportamento do modelo com as opções gravadas", scoreItems(r.Text, "item-1\nitem-2\nitem-3").Pass, err, scoreTruncado(r).Pass))
	// (6) an answer above 16 MiB that never ends
	var big bytes.Buffer
	big.WriteString(`{"response":"item-1\nitem-2\nitem-3\n","done":false}` + "\n")
	filler := `{"response":"` + strings.Repeat(`\n`, 32<<10) + `","done":false}` + "\n"
	for big.Len() <= maxReplayBytes+len(filler) {
		big.WriteString(filler)
	}
	big.WriteString(`{"response":"item-ERRADO-4","done":true,"done_reason":"stop"}` + "\n")
	r, err = parseReply("/api/generate", &big)
	if err == nil && r.Truncated {
		v := scoreReply("omitiu", r, "item-1\nitem-2\nitem-3")
		vec12B(t, "B4-06", "FALHA", fmt.Sprintf("Menor erros_replay.go:231-241 e erros.go:422: resposta acima de 16 MiB, cortada no teto e sem done, é PONTUADA como medida em categoria que não é truncado (omitiu pass=%v; o item errado veio depois do corte). Na exportação a amostra com resp_truncada=1 é inelegível (C-3.6), mas no replay o corte vale como resposta", v.Pass))
	} else {
		vec12B(t, "B4-06", "OK", fmt.Sprintf("resposta gigante: err=%v truncated=%v", err, r.Truncated))
	}
}

// ---- B5: blind e apuração -----------------------------------------------------------------------------------

func TestAdv12BAnswersFileParsing(t *testing.T) {
	known := map[string]bool{"C01": true, "C02": true}
	dir := t.TempDir()
	w := func(name, s string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(s), 0o600)
		return p
	}
	var bad []string
	for name, c := range map[string]struct {
		body string
		ok   bool
	}{
		"CRLF, branco, comentário, maiúsculas no veredito, espaços": {"# r\r\n\r\nC01\tCERTA\terrada\r\nC02  errada   Certa\r\n", true},
		"NBSP como separador":     {"C01\u00a0certa\u00a0errada\nC02\terrada\terrada\n", true},
		"BOM":                     {"\ufeffC01\tcerta\terrada\nC02\terrada\terrada\n", false},
		"rótulo minúsculo":        {"c01\tcerta\terrada\n", false},
		"rótulo repetido":         {"C01\tcerta\terrada\nC01\terrada\terrada\n", false},
		"rótulo extra":            {"C01\tcerta\terrada\nC09\tcerta\tcerta\n", false},
		"4 campos":                {"C01\tcerta\terrada\tcerta\n", false},
		"veredito inválido":       {"C01\tcerto\terrada\n", false},
		"veredito vazio (2 campos)": {"C01\tcerta\n", false},
	} {
		_, err := readAnswers(w(strings.ReplaceAll(name, " ", "_")+".txt", c.body), known)
		if (err == nil) != c.ok {
			bad = append(bad, fmt.Sprintf("%s: err=%v", name, err))
		}
	}
	if len(bad) > 0 {
		vec12B(t, "B5-01", "FALHA", "Importante erros_apurar.go:60-96: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "B5-01", "OK", "respostas do owner: CRLF/branco/comentário/maiúsculas/espaços/NBSP aceitos; BOM, rótulo minúsculo, repetido, extra, 4 campos, veredito inválido ou faltando recusados (fail-closed; o BOM recusa o arquivo inteiro com o rótulo \"\\ufeffC01\")")
}

func TestAdv12BApurarAndFilesHoldNoText(t *testing.T) {
	var cs []synthCase
	for i := 1; i <= 30; i++ {
		cs = append(cs, omitiu(i))
	}
	for i := 31; i <= 40; i++ {
		cs = append(cs, blindCase(i))
	}
	f := newFixture(t, cs, func(model string, seed int, c synthCase) (string, string) {
		if c.Cat == "inventou" {
			return "OUT-" + model + "-SEGREDO", "stop"
		}
		return answer(model != "qwen3-ia" || c.N <= 15, c)
	})
	rid := f.runBase()
	ce := f.compareOpts(rid, "omitiu")
	code, out := f.run(ce)
	var bad []string
	if code != exitPending || strings.Contains(out, "MELHORA ACEITA") {
		bad = append(bad, fmt.Sprintf("comparar com blind pendente: code %d", code))
	}
	keyPath := f.findOne(".chave.tsv")
	// one label left unanswered: the gate fails, nothing is accepted
	partial := answersFile(t, keyPath, func(string, int, string) bool { return true })
	lines := strings.Split(strings.TrimSpace(readFile(t, partial)), "\n")
	os.WriteFile(partial, []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"), 0o600)
	if c, o := f.run(f.apurarOpts(partial)); c != exitGate || strings.Contains(o, "MELHORA ACEITA") {
		bad = append(bad, fmt.Sprintf("apurar com 1 rótulo sem resposta: code %d", c))
	}
	full := answersFile(t, keyPath, func(_ string, _ int, model string) bool { return true })
	if c, _ := f.run(f.apurarOpts(full)); c != exitOK {
		bad = append(bad, fmt.Sprintf("apurar completo: code %d", c))
	}
	// failing runs whose gateway ECHOES the case content: a 500 body, an error chunk, a redirect
	for _, h := range []http.HandlerFunc{
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			io.Copy(w, r.Body)
		},
		func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			eb, _ := json.Marshal("eco: " + string(b))
			fmt.Fprintf(w, `{"error":%s}`+"\n", eb)
		},
		func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			http.Redirect(w, r, "http://127.0.0.1:1/x?"+url.QueryEscape(string(b)), 307)
		},
	} {
		s := httptest.NewServer(h)
		e := f.compareOpts(rid, "omitiu")
		e.GatewayURL = s.URL
		f.run(e)
		s.Close()
	}
	scanned := 0
	for _, p := range sidecarFiles(t, f.out) {
		base := filepath.Base(p)
		if (strings.HasPrefix(base, "blind-") && strings.HasSuffix(base, ".txt")) || base == "score.json" {
			continue
		}
		scanned++
		body := readFile(t, p)
		for _, m := range append(secretMarkers, "PEDIDO-", "item-SEGREDO", "eco:") {
			if strings.Contains(body, m) {
				bad = append(bad, fmt.Sprintf("%s contém %q", base, m))
			}
		}
	}
	for _, l := range f.logs {
		for _, m := range append(secretMarkers, "PEDIDO-", "eco:") {
			if strings.Contains(l, m) {
				bad = append(bad, fmt.Sprintf("log contém %q", m))
				break
			}
		}
	}
	if scanned < 6 {
		t.Fatalf("fixture: only %d files scanned", scanned)
	}
	if len(bad) > 0 {
		vec12B(t, "B5-02", "FALHA", "Bloqueador erros_report.go/erros.go: texto de caso fora do arquivo blind: "+strings.Join(bad, "; "))
		return
	}
	vec12B(t, "B5-02", "OK", fmt.Sprintf("comparar com blind = PENDENTE (exit 4, nada aceito); apurar com 1 rótulo sem resposta = gate falha; %d arquivos gerados (relatórios, resultado-*.json, chave.tsv, linha de base) e todos os logs, inclusive de rodadas cujo gateway ecoa o pedido no corpo 500, num chunk de erro e no Location de um 307, sem nenhum texto de pedido/resposta", scanned))
}

func TestAdv12BPrivateModeConstant(t *testing.T) {
	// the mode cannot be observed on Windows; what is checked is the value every writer uses
	src, err := os.ReadFile("erros_report.go")
	if err != nil {
		t.Fatal(err)
	}
	opens := strings.Count(string(src), "os.OpenFile(")
	excl := strings.Count(string(src), "os.O_CREATE|os.O_EXCL|os.O_WRONLY, privateFileMode")
	if privateFileMode != 0o600 || opens != excl || opens == 0 {
		vec12B(t, "B5-03", "FALHA", fmt.Sprintf("Importante erros_report.go:450-463: modo %v, %d OpenFile e %d com O_EXCL+0600", privateFileMode, opens, excl))
		return
	}
	vec12B(t, "B5-03", "OK", "MEDIDA todo arquivo da suíte passa por writeNew (O_EXCL, 0600) e a pasta é 0700 (erros.go:280); o modo 0600 em si não é observável no Windows (prova no Linux, pendência da 12.10)")
}

func adv12BOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
