package monitor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func poolFixture(t *testing.T, withAdmin bool) *fixture {
	t.Helper()
	bs, err := pool.Build([]pool.Spec{{ID: "sec-ws01", URL: "http://10.100.2.50:11434", Tipo: "remoto", Slots: 2}}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	pl := pool.New(pool.Config{PrimarySlots: 2, Secondaries: bs})
	return newFixture(t, withAdmin, queue.Config{GenSlots: 2, Pool: pl})
}

type bstate struct {
	Backends []pool.Status `json:"backends"`
}

func backendsOf(t *testing.T, f *fixture) []pool.Status {
	t.Helper()
	w := do(f.h, "GET", "/monitor/api/state", "", nil)
	if w.Code != 200 {
		t.Fatalf("state %d", w.Code)
	}
	var s bstate
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	return s.Backends
}

// The board: the primary and the secondary, with the fields the page shows.
func TestStateHasTheBackendsBoard(t *testing.T) {
	f := poolFixture(t, false)
	bs := backendsOf(t, f)
	if len(bs) != 2 || bs[0].ID != "gpu-local" || !bs[0].Primary || bs[1].ID != "sec-ws01" || bs[1].Type != "remoto" {
		t.Fatalf("backends %+v", bs)
	}
	if bs[0].Slots != 2 || bs[0].Free != 2 || bs[1].Slots != 2 || bs[1].State != "ok" || bs[1].Breaker != "fechado" {
		t.Fatalf("board %+v", bs)
	}
	// the host is shown, a URL path or a token never is
	if raw := do(f.h, "GET", "/monitor/api/state", "", nil).Body.String(); strings.Contains(raw, "http://10.100.2.50") {
		t.Fatal("the full URL is in the open state")
	}
}

// Without --backends the board has the single gpu-local.
func TestStateWithoutPoolHasOneBackend(t *testing.T) {
	f := newFixture(t, false, queue.Config{GenSlots: 2})
	bs := backendsOf(t, f)
	if len(bs) != 1 || bs[0].ID != "gpu-local" || bs[0].Slots != 2 || bs[0].State != "ok" {
		t.Fatalf("backends %+v", bs)
	}
}

func TestDrainNeedsAdminCSRFAndSameSite(t *testing.T) {
	f := poolFixture(t, true)
	path := "/monitor/api/backends/sec-ws01/drain"
	if w := do(f.h, "POST", path, "", nil); w.Code != 401 {
		t.Fatalf("no session: %d", w.Code)
	}
	cookie, csrf := f.login(t)
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf + "x"}); w.Code != 403 {
		t.Fatalf("bad csrf: %d", w.Code)
	}
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Origin": "https://evil.example"}); w.Code != 403 {
		t.Fatalf("cross-origin: %d", w.Code)
	}
	if w := do(f.h, "POST", path, "", map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf, "Sec-Fetch-Site": "cross-site"}); w.Code != 403 {
		t.Fatalf("cross-site: %d", w.Code)
	}
	if bs := backendsOf(t, f); bs[1].Drained {
		t.Fatal("a refused request drained the backend")
	}
	// a GET has no side effect and no route
	if w := do(f.h, "GET", path, "", map[string]string{"Cookie": cookie}); w.Code == 200 {
		t.Fatal("GET must not drain")
	}
}

func TestDrainEndToEndIsAudited(t *testing.T) {
	f := poolFixture(t, true)
	cookie, csrf := f.login(t)
	h := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	w := do(f.h, "POST", "/monitor/api/backends/sec-ws01/drain", `{"reason":"troca de disco"}`, h)
	if w.Code != 200 {
		t.Fatalf("drain: %d %s", w.Code, w.Body)
	}
	if bs := backendsOf(t, f); !bs[1].Drained || bs[1].State != "drenado" || bs[0].Drained {
		t.Fatalf("board after drain: %+v", bs)
	}
	if w := do(f.h, "POST", "/monitor/api/backends/sec-ws01/drain", "", h); w.Code != 409 {
		t.Fatalf("draining twice: %d", w.Code)
	}
	if w := do(f.h, "POST", "/monitor/api/backends/ghost/drain", "", h); w.Code != 404 {
		t.Fatalf("unknown backend: %d", w.Code)
	}
	if w := do(f.h, "POST", "/monitor/api/backends/BAD%20ID/drain", "", h); w.Code != 400 && w.Code != 404 {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := do(f.h, "POST", "/monitor/api/backends/sec-ws01/undrain", `{"reason":"voltou"}`, h); w.Code != 200 {
		t.Fatalf("undrain: %d %s", w.Code, w.Body)
	}
	if bs := backendsOf(t, f); bs[1].Drained || bs[1].State != "ok" {
		t.Fatalf("board after undrain: %+v", bs)
	}
	f.st.Flush()
	au, err := f.st.Audits(t.Context(), 10)
	if err != nil || len(au) != 2 {
		t.Fatalf("audits %v %v", au, err)
	}
	// newest first
	if au[1].Action != "drain" || au[1].RequestID != "sec-ws01" || au[1].User != adminUser || au[1].Reason != "troca de disco" || au[1].After != "drenado" {
		t.Fatalf("audit %+v", au[1])
	}
	if au[0].Action != "undrain" || au[0].After != "ativo" {
		t.Fatalf("audit %+v", au[0])
	}
}

// Draining the primary hands its waiting work to the secondary at once.
func TestDrainingThePrimaryWakesTheDispatcher(t *testing.T) {
	f := poolFixture(t, true)
	cookie, csrf := f.login(t)
	h := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	if w := do(f.h, "POST", "/monitor/api/backends/gpu-local/drain", "", h); w.Code != 200 {
		t.Fatalf("drain primary: %d", w.Code)
	}
	if bs := backendsOf(t, f); !bs[0].Drained || bs[0].State != "drenado" {
		t.Fatalf("%+v", bs[0])
	}
}

func TestNoAdminFileNoDrain(t *testing.T) {
	f := poolFixture(t, false)
	if w := do(f.h, "POST", "/monitor/api/backends/sec-ws01/drain", "", nil); w.Code == 200 {
		t.Fatalf("drain without an administrator: %d", w.Code)
	}
	if bs := backendsOf(t, f); bs[1].Drained {
		t.Fatal("drained")
	}
}

// The page: the Backends board, the drain buttons and the Backend columns are
// built with textContent only, and the file keeps LF (the CSP hashes need it).
func TestIndexHasTheBackendsBoard(t *testing.T) {
	page := string(rawIndexHTML)
	for _, want := range []string{`<h2>Backends</h2>`, `id="backends"`, `"/monitor/api/backends/"`, `function renderBackends`, `"Backend"`} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	if strings.Contains(page, "\r") {
		t.Fatal("index.html has CR: keep it LF")
	}
	if strings.Contains(page, "innerHTML") {
		t.Fatal("innerHTML in the page")
	}
}
