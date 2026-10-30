package monitor

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

const (
	adminUser = "admin"
	adminPass = "uma-senha-longa-de-teste"
)

type fixture struct {
	h   *Handler
	q   *queue.Manager
	st  *store.Store
	hub *Hub
}

func newFixture(t *testing.T, withAdmin bool, qc queue.Config) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "gw.db"), store.Options{BootID: "boot-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hub := NewHub()
	qc.OnChange = hub.Notify
	qc.OnRecord = st.Record
	q := queue.New(qc)
	cfg := Config{Queue: q, Store: st, Hub: hub, Version: "test", BootID: "boot-test"}
	if withAdmin {
		line, err := HashPassword(adminPass, minIterations)
		if err != nil {
			t.Fatal(err)
		}
		cr, err := ParseCredentials(adminUser + ":" + line)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Admin = cr
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{h: h, q: q, st: st, hub: hub}
}

func do(h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Real-Ip", "10.100.2.50")
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	if v := hdr["Host"]; v != "" {
		r.Host = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// login returns the cookie header and the CSRF token.
func (f *fixture) login(t *testing.T) (cookie, csrf string) {
	t.Helper()
	w := do(f.h, "POST", "/monitor/api/login", `{"user":"`+adminUser+`","password":"`+adminPass+`"}`, nil)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	var out struct{ CSRF string }
	json.Unmarshal(w.Body.Bytes(), &out)
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			return c.Name + "=" + c.Value, out.CSRF
		}
	}
	t.Fatal("no session cookie")
	return "", ""
}

func hold(t *testing.T, q *queue.Manager, label string) *queue.Ticket {
	t.Helper()
	tk, err := q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "10.0.0.7", ClientLabel: label, Tool: "generate", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

// wait enqueues a request that stays waiting; the result channel gets its error.
func wait(t *testing.T, q *queue.Manager, label string) (id string, errc chan error) {
	t.Helper()
	errc = make(chan error, 1)
	before := len(q.Snapshot().Waiting)
	go func() {
		tk, err := q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "10.0.0.8", ClientLabel: label, Tool: "chat", Model: "m"})
		if err == nil {
			tk.Release(queue.Result{})
		}
		errc <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, it := range q.Snapshot().Waiting {
			if it.ClientLabel == label {
				return it.ID, errc
			}
		}
		time.Sleep(time.Millisecond)
	}
	_ = before
	t.Fatal("request did not enter the line")
	return "", nil
}

func TestPageAndCSP(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	w := do(f.h, "GET", "/monitor", "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	body := w.Body.String()
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "https://", "//cdn", "src=\"http", "href=\"http"} {
		if strings.Contains(body, bad) {
			t.Errorf("page contains %q", bad)
		}
	}
	csp := w.Header().Get("Content-Security-Policy")
	sc := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindStringSubmatch(body)[1]
	sum := sha256.Sum256([]byte(sc))
	if !strings.Contains(csp, "script-src 'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") ||
		!strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Fatalf("csp = %s", csp)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("headers %v", w.Header())
	}
	if do(f.h, "GET", "/monitor/", "", nil).Code != 200 {
		t.Fatal("/monitor/ not served")
	}
}

// XSS: a client-controlled label never reaches the page as markup.
func TestMaliciousLabelIsEscapedInJSONAndSSE(t *testing.T) {
	f := newFixture(t, false, queue.Config{GenSlots: 1})
	evil := `<img src=x onerror=alert(1)></script><script>alert(2)</script>"` + "\n\ndata: forged\n\n"
	tk, err := f.q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "10.0.0.7", ClientLabel: evil, Tool: evil, Model: evil})
	if err != nil {
		t.Fatal(err)
	}
	defer tk.Release(queue.Result{})
	w := do(f.h, "GET", "/monitor/api/state", "", nil)
	raw := w.Body.String()
	if strings.Contains(raw, "<img") || strings.Contains(raw, "</script>") || strings.Contains(raw, "<script>") {
		t.Fatalf("raw markup in the JSON: %s", raw)
	}
	if !strings.Contains(raw, string([]byte{92, 117, 48, 48, 51, 99})+"img") { // backslash + u003c + img
		t.Fatalf("expected escaped markup: %s", raw)
	}
	var st State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || len(st.Queue.Running) != 1 || st.Queue.Running[0].ClientLabel != evil {
		t.Fatalf("round trip: %v %+v", err, st.Queue.Running)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers %v", w.Header())
	}

	// SSE: the forged "data:" line must stay inside one event.
	srv := httptest.NewServer(f.h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/monitor/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if sc.Text() == "" {
			break
		}
	}
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "data: {") || lines[1] != "" {
		t.Fatalf("event framing broken: %q", lines)
	}
	if strings.Contains(lines[0], "<img") {
		t.Fatal("raw markup in the SSE data")
	}
}

func TestGETsHaveNoSideEffects(t *testing.T) {
	f := newFixture(t, true, queue.Config{GenSlots: 1})
	run := hold(t, f.q, "run")
	defer run.Release(queue.Result{})
	id, _ := wait(t, f.q, "w1")
	before := f.q.Snapshot()
	for _, p := range []string{"/monitor", "/monitor/api/state", "/monitor/api/series", "/monitor/api/history", "/monitor/api/stats", "/monitor/api/audit", "/monitor/api/session",
		"/monitor/api/queue/" + id + "/remove", "/monitor/api/queue/" + id + "/promote", "/monitor/api/running/" + run.ID + "/cancel", "/monitor/api/login", "/monitor/api/logout"} {
		w := do(f.h, "GET", p, "", nil)
		if strings.Contains(p, "/queue/") || strings.Contains(p, "/running/") || strings.HasSuffix(p, "login") || strings.HasSuffix(p, "logout") {
			if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
				t.Errorf("GET %s answered %d", p, w.Code)
			}
		}
	}
	after := f.q.Snapshot()
	if len(after.Waiting) != len(before.Waiting) || len(after.Running) != len(before.Running) || after.Waiting[0].Priority != before.Waiting[0].Priority {
		t.Fatalf("a GET changed the queue: %+v -> %+v", before, after)
	}
	if a, _ := f.st.Audits(context.Background(), 10); len(a) != 0 {
		t.Fatalf("a GET wrote an audit entry: %+v", a)
	}
	f.h.sess.mu.Lock()
	n := len(f.h.sess.m)
	f.h.sess.mu.Unlock()
	if n != 0 {
		t.Fatal("a GET created a session")
	}
}

func TestActionsNeedSessionAndCSRF(t *testing.T) {
	f := newFixture(t, true, queue.Config{GenSlots: 1})
	run := hold(t, f.q, "run")
	defer run.Release(queue.Result{})
	id, _ := wait(t, f.q, "w1")
	path := "/monitor/api/queue/" + id + "/remove"
	if w := do(f.h, "POST", path, "", nil); w.Code != 401 {
		t.Fatalf("no session: %d", w.Code)
	}
	cookie, csrf := f.login(t)
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie}); w.Code != 403 {
		t.Fatalf("no CSRF: %d", w.Code)
	}
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf + "x"}); w.Code != 403 {
		t.Fatalf("wrong CSRF: %d", w.Code)
	}
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Origin": "https://evil.example"}); w.Code != 403 {
		t.Fatalf("cross-site Origin: %d", w.Code)
	}
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Sec-Fetch-Site": "cross-site"}); w.Code != 403 {
		t.Fatalf("Sec-Fetch-Site cross-site: %d", w.Code)
	}
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": "mcpgw_session=forged", "X-CSRF-Token": csrf}); w.Code != 401 {
		t.Fatalf("forged cookie: %d", w.Code)
	}
	if len(f.q.Snapshot().Waiting) != 1 {
		t.Fatal("a refused action changed the queue")
	}
	// same-origin Origin is fine
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Origin": "https://example.com", "Host": "example.com"}); w.Code != 200 {
		t.Fatalf("same-origin: %d %s", w.Code, w.Body)
	}
}

func TestActionsEndToEndAreAudited(t *testing.T) {
	f := newFixture(t, true, queue.Config{GenSlots: 1})
	run := hold(t, f.q, "run")
	id1, err1 := wait(t, f.q, "w1")
	id2, _ := wait(t, f.q, "w2")
	cookie, csrf := f.login(t)
	h := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	post := func(p, body string) *httptest.ResponseRecorder { return do(f.h, "POST", p, body, h) }

	if w := post("/monitor/api/queue/"+id2+"/promote", `{"reason":"urgente"}`); w.Code != 200 {
		t.Fatalf("promote: %d %s", w.Code, w.Body)
	}
	if s := f.q.Snapshot(); s.Waiting[0].ID != id2 || s.Waiting[0].Priority != "alta" {
		t.Fatalf("promote had no effect: %+v", s.Waiting)
	}
	if w := post("/monitor/api/queue/"+id2+"/demote", ``); w.Code != 200 {
		t.Fatalf("demote: %d", w.Code)
	}
	if w := post("/monitor/api/queue/"+id1+"/remove", `{"reason":"`+strings.Repeat("é", 500)+`"}`); w.Code != 200 {
		t.Fatalf("remove: %d %s", w.Code, w.Body)
	}
	select {
	case err := <-err1:
		if err == nil || err.Error() != queue.ErrRemoved.Error() {
			t.Fatalf("the waiting client got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the waiting client was not released")
	}
	if w := post("/monitor/api/queue/"+id1+"/remove", ``); w.Code != 409 {
		t.Fatalf("second remove: %d", w.Code)
	}
	if w := post("/monitor/api/queue/not-an-id/remove", ``); w.Code != 400 {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := post("/monitor/api/running/"+run.ID+"/cancel", ``); w.Code != 200 {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	if !queue.IsAdminCancel(run.Ctx) {
		t.Fatal("the running context was not cancelled")
	}
	run.Release(queue.Result{})
	if w := post("/monitor/api/running/"+run.ID+"/cancel", ``); w.Code != 409 {
		t.Fatalf("second cancel: %d", w.Code)
	}
	a, err := f.st.Audits(context.Background(), 20)
	if err != nil || len(a) != 4 {
		t.Fatalf("audit entries: %v %+v", err, a)
	}
	// newest first: cancel, remove, demote, promote
	want := []string{"cancel", "remove", "demote", "promote"}
	for i, e := range a {
		if e.Action != want[i] || e.User != adminUser || e.IP != "10.100.2.50" || e.Before == "" || e.After == "" {
			t.Fatalf("audit[%d] = %+v", i, e)
		}
	}
	if len([]rune(a[1].Reason)) != 200 || a[3].Reason != "urgente" {
		t.Fatalf("reasons: %d %q", len([]rune(a[1].Reason)), a[3].Reason)
	}
	// the page can read the trail
	w := do(f.h, "GET", "/monitor/api/audit", "", nil)
	if !strings.Contains(w.Body.String(), `"acao":"promote"`) {
		t.Fatalf("audit endpoint: %s", w.Body)
	}
}

func TestLoginCookieAndRateLimit(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	bad := func(pw string) *httptest.ResponseRecorder {
		return do(f.h, "POST", "/monitor/api/login", `{"user":"admin","password":"`+pw+`"}`, nil)
	}
	for i := 0; i < loginMaxIP; i++ {
		if w := bad("errada"); w.Code != 401 {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	if w := bad(adminPass); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("after %d failures even the right password is refused: %d", loginMaxIP, w.Code)
	}
	// another IP is not locked out; check cookie attributes
	w := do(f.h, "POST", "/monitor/api/login", `{"user":"admin","password":"`+adminPass+`"}`, map[string]string{"X-Real-Ip": "10.100.2.99"})
	if w.Code != 200 {
		t.Fatalf("other IP: %d", w.Code)
	}
	var c *http.Cookie
	for _, x := range w.Result().Cookies() {
		if x.Name == cookieName {
			c = x
		}
	}
	if c == nil || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 8*3600 || c.Path != "/monitor" || len(c.Value) < 40 {
		t.Fatalf("cookie %+v", c)
	}
	// unknown user and wrong password look the same
	if w := do(f.h, "POST", "/monitor/api/login", `{"user":"nobody","password":"x"}`, map[string]string{"X-Real-Ip": "10.100.2.98"}); w.Code != 401 || !strings.Contains(w.Body.String(), "usuário ou senha") {
		t.Fatalf("unknown user: %d %s", w.Code, w.Body)
	}
	// JSON only
	r := httptest.NewRequest("POST", "/monitor/api/login", strings.NewReader("user=admin&password=x"))
	r.RemoteAddr = "127.0.0.1:1"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	if rec.Code != 415 {
		t.Fatalf("form login: %d", rec.Code)
	}
	// session endpoint
	cookie := c.Name + "=" + c.Value
	s := do(f.h, "GET", "/monitor/api/session", "", map[string]string{"Cookie": cookie})
	if !strings.Contains(s.Body.String(), `"logged_in":true`) || !strings.Contains(s.Body.String(), `"csrf"`) {
		t.Fatalf("session: %s", s.Body)
	}
	if s := do(f.h, "GET", "/monitor/api/session", "", nil); strings.Contains(s.Body.String(), "csrf") {
		t.Fatal("anonymous session leaks a CSRF token")
	}
}

func TestLogoutEndsSession(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	cookie, csrf := f.login(t)
	h := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	if w := do(f.h, "POST", "/monitor/api/logout", "", h); w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := do(f.h, "POST", "/monitor/api/queue/"+strings.Repeat("a", 32)+"/remove", "", h); w.Code != 401 {
		t.Fatalf("session survived the logout: %d", w.Code)
	}
}

func TestSessionExpires(t *testing.T) {
	var s sessions
	tok, _ := s.create("admin", time.Now().Add(-sessionTTL-time.Minute))
	if s.get(tok, time.Now()) != nil {
		t.Fatal("expired session accepted")
	}
}

func TestNoAdminFileDisablesActions(t *testing.T) {
	f := newFixture(t, false, queue.Config{})
	if w := do(f.h, "POST", "/monitor/api/login", `{"user":"a","password":"b"}`, nil); w.Code != 503 {
		t.Fatalf("login: %d", w.Code)
	}
	if w := do(f.h, "POST", "/monitor/api/queue/"+strings.Repeat("a", 32)+"/remove", "", nil); w.Code != 503 {
		t.Fatalf("action: %d", w.Code)
	}
	if w := do(f.h, "GET", "/monitor/api/state", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"admin_enabled":false`) {
		t.Fatalf("state: %d %s", w.Code, w.Body)
	}
}

func TestSSEPushesChangesAndCapsConnections(t *testing.T) {
	f := newFixture(t, false, queue.Config{GenSlots: 1})
	f.h.cfg.MaxSSE = 1
	srv := httptest.NewServer(f.h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/monitor/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	br := bufio.NewReader(resp.Body)
	readEvent := func() State {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		br.ReadString('\n')
		var st State
		if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(line), "data: ")), &st); err != nil {
			t.Fatalf("bad event %q: %v", line, err)
		}
		return st
	}
	if st := readEvent(); len(st.Queue.Running) != 0 {
		t.Fatalf("first event %+v", st)
	}
	// a second viewer is refused (cap 1)
	r2, err := http.Get(srv.URL + "/monitor/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != 503 {
		t.Fatalf("second stream: %d", r2.StatusCode)
	}
	tk := hold(t, f.q, "ao-vivo")
	defer tk.Release(queue.Result{})
	got := make(chan State, 1)
	go func() { got <- readEvent() }()
	select {
	case st := <-got:
		if len(st.Queue.Running) != 1 || st.Queue.Running[0].ClientLabel != "ao-vivo" {
			t.Fatalf("event after the change: %+v", st.Queue.Running)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event after a queue change")
	}
}

func TestHistoryStatsAudit(t *testing.T) {
	f := newFixture(t, false, queue.Config{GenSlots: 1})
	tk := hold(t, f.q, "hist")
	tk.Release(queue.Result{HTTPStatus: 200, Eval: 9, PromptEval: 3})
	f.st.Flush()
	w := do(f.h, "GET", "/monitor/api/history?cliente=hist&limite=5", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"client_label":"hist"`) {
		t.Fatalf("history: %d %s", w.Code, w.Body)
	}
	if w := do(f.h, "GET", "/monitor/api/history?desde=nao-e-data", "", nil); w.Code != 400 {
		t.Fatalf("bad date: %d", w.Code)
	}
	if w := do(f.h, "GET", "/monitor/api/history?cliente="+"x'%20OR%20'1'='1", "", nil); w.Code != 200 || strings.Contains(w.Body.String(), "hist") {
		t.Fatalf("injection: %d %s", w.Code, w.Body)
	}
	if w := do(f.h, "GET", "/monitor/api/stats?janela=1h", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatalf("stats: %d %s", w.Code, w.Body)
	}
	if w := do(f.h, "GET", "/monitor/api/stats?janela=99y", "", nil); w.Code != 400 {
		t.Fatalf("bad window: %d", w.Code)
	}
}

func TestSeriesSampler(t *testing.T) {
	f := newFixture(t, false, queue.Config{GenSlots: 1})
	f.h.Observe(queue.Record{StartedAt: time.Now(), WaitMS: 100})
	f.h.Observe(queue.Record{StartedAt: time.Now(), WaitMS: 300})
	f.h.Observe(queue.Record{Status: queue.StatusRejected})
	f.h.samplePoint(time.Now())
	w := do(f.h, "GET", "/monitor/api/series", "", nil)
	var out struct {
		Points []Point `json:"points"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Points) != 1 || out.Points[0].Finished != 2 || out.Points[0].WaitAvg != 200 {
		t.Fatalf("%+v", out)
	}
}

func TestCredentials(t *testing.T) {
	line, err := HashPassword("senha-bem-longa", minIterations)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseCredentials("admin:" + line)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Verify("admin", "senha-bem-longa") || c.Verify("admin", "outra-senha") || c.Verify("root", "senha-bem-longa") {
		t.Fatal("Verify")
	}
	for _, bad := range []string{"", "admin", "admin:bcrypt$x", "admin:pbkdf2-sha256$10$AAAA$BBBB", "admin:pbkdf2-sha256$1000$!!!$!!!", ":pbkdf2-sha256$1000$AAAAAAAAAAAA$AAAA"} {
		if _, err := ParseCredentials(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := HashPassword("curta", minIterations); err == nil {
		t.Error("short password accepted")
	}
	// two hashes of the same password differ (random salt)
	l2, _ := HashPassword("senha-bem-longa", minIterations)
	if l2 == line {
		t.Error("salt is not random")
	}
}

func TestAdminFilePermissions(t *testing.T) {
	line, _ := HashPassword("senha-bem-longa", minIterations)
	p := filepath.Join(t.TempDir(), "admin")
	os.WriteFile(p, []byte("admin:"+line+"\n"), 0o644)
	if runtime.GOOS != "windows" {
		if _, err := LoadAdminFile(p); err == nil {
			t.Fatal("a group/world readable credentials file must be refused")
		}
		os.Chmod(p, 0o600)
	}
	c, err := LoadAdminFile(p)
	if err != nil || c.User != "admin" {
		t.Fatalf("%v %+v", err, c)
	}
	if _, err := LoadAdminFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file accepted")
	}
}

// Fase 9.6a: the page has the "Raciocínio" column in the three tables and the
// state carries the reasoning mode of running requests and of the history.
func TestThinkColumnInPageAndState(t *testing.T) {
	f := newFixture(t, false, queue.Config{GenSlots: 1})
	page := do(f.h, "GET", "/monitor", "", nil).Body.String()
	if n := strings.Count(page, `"Raciocínio"`); n != 4 { // running, waiting, recent + the Fase 10.L job log table
		t.Fatalf("the Raciocínio column must be in the 4 tables, found %d", n)
	}
	tk, err := f.q.Acquire(context.Background(), queue.Request{Lane: queue.LaneGen, ClientIP: "10.0.0.7", ClientLabel: "h/1", Model: "qwen3-ia", Think: "off"})
	if err != nil {
		t.Fatal(err)
	}
	var st State
	if err := json.Unmarshal(do(f.h, "GET", "/monitor/api/state", "", nil).Body.Bytes(), &st); err != nil || len(st.Queue.Running) != 1 || st.Queue.Running[0].Think != "off" {
		t.Fatalf("running think: %v %+v", err, st.Queue.Running)
	}
	tk.Release(queue.Result{})
	deadline := time.Now().Add(3 * time.Second)
	for {
		f.st.Flush()
		h := do(f.h, "GET", "/monitor/api/history", "", nil).Body.String()
		if strings.Contains(h, `"think":"off"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("history without think: %s", h)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
