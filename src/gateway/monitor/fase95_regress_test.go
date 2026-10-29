package monitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func loginFrom(f *fixture, ip, pw string) int {
	return do(f.h, "POST", "/monitor/api/login", `{"user":"admin","password":"`+pw+`"}`, map[string]string{"X-Real-Ip": ip}).Code
}

// Fase 9.5, M1 (Media): the attempt is reserved before the slow PBKDF2, so N
// parallel wrong passwords from one IP evaluate at most loginMaxIP of them.
func TestRegressLoginReservationIsAtomic(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	line, _ := HashPassword(adminPass, 100000) // slow enough for the requests to overlap
	f.h.cfg.Admin, _ = ParseCredentials(adminUser + ":" + line)
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	start := make(chan struct{})
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c := loginFrom(f, "10.100.2.66", "errada")
			mu.Lock()
			codes[c]++
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	if codes[401] > loginMaxIP || codes[401]+codes[429] != 40 {
		t.Fatalf("codes %v: at most %d evaluations", codes, loginMaxIP)
	}
	// the right password is refused while the IP is locked out
	if c := loginFrom(f, "10.100.2.66", adminPass); c != 429 {
		t.Fatalf("locked IP got %d", c)
	}
}

// Fase 9.5, M2 (Media): failures from many IPs do not lock the administrator
// out (there is no global ceiling any more).
func TestRegressNoGlobalLockout(t *testing.T) {
	f := newFixture(t, true, queue.Config{})
	for i := 0; i < 60; i++ {
		loginFrom(f, fmt.Sprintf("10.100.7.%d", i+1), "errada")
	}
	if c := loginFrom(f, "10.100.2.10", adminPass); c != 200 {
		t.Fatalf("legit login %d", c)
	}
}

// The lockout doubles at each new round of failures and is capped; a success
// clears the IP; an aborted reservation does not count.
func TestRegressLoginBackoff(t *testing.T) {
	var l limiter
	now := time.Unix(1_700_000_000, 0)
	fail := func() {
		ok, _ := l.tryBegin("ip", now)
		if !ok {
			t.Fatal("refused while it should be allowed")
		}
		l.end("ip", now, false)
	}
	for round, want := range []time.Duration{loginWindow, 2 * loginWindow, 4 * loginWindow} {
		for i := 0; i < loginMaxIP; i++ {
			fail()
		}
		ok, retry := l.tryBegin("ip", now)
		if ok || retry != want {
			t.Fatalf("round %d: ok=%v retry=%v want %v", round, ok, retry, want)
		}
		now = now.Add(want)
	}
	ok, _ := l.tryBegin("ip", now)
	if !ok {
		t.Fatal("after the lockout the IP gets attempts again")
	}
	l.abort("ip")
	for i := 0; i < loginMaxIP-1; i++ {
		fail()
	}
	ok, _ = l.tryBegin("ip", now)
	l.end("ip", now, true)
	if !ok || l.tracked() != 0 {
		t.Fatalf("success must clear the IP (ok=%v tracked=%d)", ok, l.tracked())
	}
	// a long series is capped
	for r := 0; r < 20; r++ {
		for i := 0; i < loginMaxIP; i++ {
			if ok, _ := l.tryBegin("x", now); ok {
				l.end("x", now, false)
			}
		}
		_, retry := l.tryBegin("x", now)
		if retry > loginMaxLock {
			t.Fatalf("lock %v above the cap", retry)
		}
		now = now.Add(retry)
	}
}

// The limiter does not grow without bound with many idle IPs.
func TestRegressLimiterSweeps(t *testing.T) {
	var l limiter
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < loginMaxTracked+100; i++ {
		ip := fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255)
		l.tryBegin(ip, now)
		l.abort(ip)
	}
	l.tryBegin("10.200.0.1", now.Add(time.Second))
	if n := l.tracked(); n > 101 { // swept when it passed loginMaxTracked; only the later IPs remain
		t.Fatalf("tracked %d after the sweep", n)
	}
}

// Fase 9.5, M3 (Media): one IP cannot take every event stream.
func TestRegressSSEPerIP(t *testing.T) {
	f := newFixture(t, false, queue.Config{})
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close) // registered first, runs last: the streams are closed before
	open := func(ip string) int {
		req, _ := http.NewRequest("GET", srv.URL+"/monitor/api/stream", nil)
		req.Header.Set("X-Real-Ip", ip)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp.StatusCode
	}
	for i := 0; i < 4; i++ {
		if c := open("10.100.2.200"); c != 200 {
			t.Fatalf("stream %d: %d", i, c)
		}
	}
	if c := open("10.100.2.200"); c != 503 {
		t.Fatalf("5th stream of the same IP: %d", c)
	}
	if c := open("10.100.2.201"); c != 200 {
		t.Fatalf("other IP: %d", c)
	}
}

// Fase 9.5, M8 (Baixa): parallel GET /stats cost one computation per window
// and TTL.
func TestRegressStatsCached(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"), store.Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := queue.New(queue.Config{OnRecord: st.Record})
	tk, _ := q.Acquire(context.Background(), queue.Request{ClientIP: "10.0.0.1", ClientLabel: "x"})
	tk.Release(queue.Result{})
	st.Flush()
	h, _ := New(Config{Queue: q, Store: st})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := do(h, "GET", "/monitor/api/stats?janela=1h", "", nil); w.Code != 200 {
				t.Errorf("stats %d", w.Code)
			}
		}()
	}
	wg.Wait()
	do(h, "GET", "/monitor/api/stats?janela=24h", "", nil)
	if n := h.statsComputed(); n != 2 {
		t.Fatalf("computations %d, want 2 (one per window)", n)
	}
}
