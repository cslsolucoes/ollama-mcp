package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// Fase 9.5, P2 (Baixa): IPv6 clients are grouped by /64 for the per-IP limits
// and the fairness; IPv4 and IPv4-mapped stay as they are.
func TestRegressFairIP(t *testing.T) {
	for in, want := range map[string]string{
		"10.0.0.1":                  "10.0.0.1",
		"::ffff:10.0.0.1":           "10.0.0.1",
		"2001:db8:1:2::5":           "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::9":      "2001:db8:1:2::/64",
		"2001:db8:1:3::5":           "2001:db8:1:3::/64",
		"::1":                       "::/64",
		"not-an-ip":                 "not-an-ip",
		"fe80::1%eth0":              "fe80::/64",
		"2001:db8:aaaa:bbbb:cccc::": "2001:db8:aaaa:bbbb::/64",
	} {
		if got := fairIP(in); got != want {
			t.Errorf("fairIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegressIPv6RotationCountsAsOneClient(t *testing.T) {
	up := &blockingUp{release: make(chan struct{})}
	e := setup(t, up, queue.Config{GenSlots: 1, PerClient: 2, PerIP: 2, QueueMax: 64}, nil)
	defer close(up.release)
	go postBg(e, "/api/generate", `{"model":"m"}`, map[string]string{"X-Real-Ip": "10.0.0.1"})
	waitFor(t, func() bool { return up.seen.Load() == 1 })
	codes := make(chan int, 10)
	for i := 1; i <= 10; i++ {
		ip := fmt.Sprintf("2001:db8:1:2::%x", i)
		go func() {
			resp, _ := postBg(e, "/api/generate", `{"model":"m"}`, map[string]string{"X-Real-Ip": ip})
			codes <- resp.StatusCode
		}()
	}
	n503 := 0
	for i := 0; i < 8; i++ {
		select {
		case c := <-codes:
			if c == 503 {
				n503++
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d answers", i)
		}
	}
	if n503 != 8 || len(e.q.Snapshot().Waiting) != 2 {
		t.Fatalf("503=%d waiting=%d: the /64 must count as one IP (PerIP 2)", n503, len(e.q.Snapshot().Waiting))
	}
	if w := e.q.Snapshot().Waiting; w[0].ClientIP != "2001:db8:1:2::/64" {
		t.Fatalf("client ip %q", w[0].ClientIP)
	}
}

// Fase 9.5, P6 (Media): a started request cannot hold its slot past the run
// timeout, before the headers (504) or mid-stream (final NDJSON error line).
func TestRegressRunTimeout(t *testing.T) {
	up := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/api/chat" {
			<-r.Context().Done() // never answers
			return
		}
		for {
			if _, err := io.WriteString(w, `{"response":"x","done":false}`+"\n"); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	})
	e := setup(t, up, queue.Config{GenSlots: 1, MaxWait: time.Minute}, func(c *Config) { c.RunTimeout = 250 * time.Millisecond })

	start := time.Now()
	resp, body := post(t, e, "/api/generate", `{"model":"m"}`, nil)
	if resp.StatusCode != 200 || !strings.HasSuffix(strings.TrimSpace(body), `{"error":"`+ErrRunTimeout.Error()+`"}`) || time.Since(start) > 2*time.Second {
		t.Fatalf("mid-stream: %d after %v, tail %q", resp.StatusCode, time.Since(start), body[max(0, len(body)-120):])
	}
	r1 := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if r1.Status != queue.StatusError || r1.ErrorClass != "run_timeout" {
		t.Fatalf("record %+v", r1)
	}
	e.waitIdle(t)

	resp, body = post(t, e, "/api/chat", `{"model":"m"}`, nil)
	if resp.StatusCode != http.StatusGatewayTimeout || !strings.Contains(body, ErrRunTimeout.Error()) {
		t.Fatalf("before headers: %d %s", resp.StatusCode, body)
	}
	r2 := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if r2.ErrorClass != "run_timeout" || r2.HTTPStatus != 504 {
		t.Fatalf("record %+v", r2)
	}
	e.waitIdle(t)
}

// The default run timeout is the queue MaxWait.
func TestRegressRunTimeoutDefaultsToMaxWait(t *testing.T) {
	e := setup(t, http.NotFoundHandler(), queue.Config{MaxWait: 7 * time.Minute}, nil)
	h := e.gw.Config.Handler.(*Handler)
	if h.cfg.RunTimeout != 7*time.Minute {
		t.Fatalf("run timeout %v", h.cfg.RunTimeout)
	}
}

// blockingUp holds every generation until release is closed.
type blockingUp struct {
	release chan struct{}
	seen    atomic.Int64
}

func (b *blockingUp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	b.seen.Add(1)
	select {
	case <-b.release:
	case <-r.Context().Done():
		return
	}
	io.WriteString(w, finalChunk+"\n")
}
