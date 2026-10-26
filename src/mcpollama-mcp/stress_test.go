//go:build stress

package main

// Local stress tests of the MCP server against an httptest mock of Ollama.
// They never touch the real server. Run with:
//
//	go test -tags stress -count=1 -v -run . ./mcpollama-mcp
//
// Metrics are logged as "METRIC name=value".

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// stressMock is a fake Ollama that records how many generate calls ran at the
// same time, and every prompt it received.
type stressMock struct {
	srv     *httptest.Server
	cur     atomic.Int32
	maxCur  atomic.Int32
	hits    atomic.Int32
	minLat  time.Duration
	maxLat  time.Duration
	mu      sync.Mutex
	prompts []string
}

func newStressMock(t testing.TB, minLat, maxLat time.Duration) *stressMock {
	t.Helper()
	m := &stressMock{minLat: minLat, maxLat: maxLat}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Prompt string `json:"prompt"`
		}
		json.Unmarshal(body, &req)
		m.hits.Add(1)
		c := m.cur.Add(1)
		for {
			old := m.maxCur.Load()
			if c <= old || m.maxCur.CompareAndSwap(old, c) {
				break
			}
		}
		defer m.cur.Add(-1)
		m.mu.Lock()
		m.prompts = append(m.prompts, req.Prompt)
		m.mu.Unlock()
		lat := m.minLat
		if m.maxLat > m.minLat {
			lat += time.Duration(rand.Int63n(int64(m.maxLat - m.minLat)))
		}
		select {
		case <-time.After(lat):
		case <-r.Context().Done():
			return
		}
		answerOK(w, r)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *stressMock) snapshotPrompts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.prompts...)
}

// tsBuf is a stdout sink that stamps each response (Run writes one per Write).
type tsBuf struct {
	mu    sync.Mutex
	lines []string
	at    []time.Time
	badJS int
}

func (b *tsBuf) Write(p []byte) (int, error) {
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if !json.Valid([]byte(l)) {
			b.badJS++
		}
		b.lines = append(b.lines, l)
		b.at = append(b.at, now)
	}
	return len(p), nil
}

type respInfo struct {
	at      time.Time
	isError bool // JSON-RPC error or tool isError
	code    int
	line    string
}

// responses indexes by raw id; dup counts repeated ids; null ids are listed apart.
func (b *tsBuf) responses() (m map[string]respInfo, dup int, nullIDs []respInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m = map[string]respInfo{}
	for i, l := range b.lines {
		var r struct {
			ID     json.RawMessage `json:"id"`
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(l), &r) != nil {
			continue
		}
		ri := respInfo{at: b.at[i], isError: r.Result.IsError, line: l}
		if r.Error != nil {
			ri.isError, ri.code = true, r.Error.Code
		}
		id := string(r.ID)
		if id == "null" || id == "" {
			nullIDs = append(nullIDs, ri)
			continue
		}
		if _, ok := m[id]; ok {
			dup++
		}
		m[id] = ri
	}
	return
}

func pct(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[int(float64(len(s)-1)*p)]
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }

func genLine(id int, prompt string) string {
	return toolCall(id, "generate", fmt.Sprintf(`{"prompt":%q}`, prompt))
}

// runLines feeds all lines at once (a reader) and returns stdout, the wall
// time and the instant Run started.
func runLines(t *testing.T, cfg Config, tools []ToolEntry, lines []string) (*tsBuf, time.Duration, time.Time, *syncBuf) {
	t.Helper()
	out, errw := &tsBuf{}, &syncBuf{}
	start := time.Now()
	if err := Run(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), out, errw, cfg, tools); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out, time.Since(start), start, errw
}

// 1. Concurrent burst: 500 generate calls at once, GPU queue N=1.
func TestStress1Burst(t *testing.T) {
	const n = 500
	m := newStressMock(t, 5*time.Millisecond, 20*time.Millisecond)
	cfg := testConfigWith(m.srv.URL)
	lines := make([]string, n)
	for i := range lines {
		lines[i] = genLine(i+1, fmt.Sprintf("p%d", i+1))
	}
	out, total, t0, _ := runLines(t, cfg, buildToolsFor(t, cfg), lines)
	resp, dup, nulls := out.responses()
	var lat []time.Duration // all calls are sent in the first instants: latency = completion offset
	errs := 0
	for i := 1; i <= n; i++ {
		r, ok := resp[fmt.Sprint(i)]
		if !ok {
			t.Errorf("lost response id=%d", i)
			continue
		}
		if r.isError {
			errs++
		}
		lat = append(lat, r.at.Sub(t0))
	}
	t.Logf("METRIC burst_total=%s", total.Round(time.Millisecond))
	t.Logf("METRIC burst_throughput=%.1f/s", float64(n)/total.Seconds())
	t.Logf("METRIC burst_lat_p50=%s p95=%s p99=%s", ms(pct(lat, .5)), ms(pct(lat, .95)), ms(pct(lat, .99)))
	t.Logf("METRIC burst_responses=%d dup=%d lost=%d errors=%d nullid=%d bad_json=%d mock_hits=%d mock_max_concurrency=%d",
		len(resp), dup, n-len(resp), errs, len(nulls), out.badJS, m.hits.Load(), m.maxCur.Load())
	if len(resp) != n || dup != 0 || errs != 0 || len(nulls) != 0 || out.badJS != 0 {
		t.Fatalf("burst: responses=%d dup=%d errs=%d nulls=%d badjson=%d", len(resp), dup, errs, len(nulls), out.badJS)
	}
	if m.maxCur.Load() != 1 {
		t.Fatalf("max concurrency at the mock = %d, want 1", m.maxCur.Load())
	}
}

// pipeSender feeds Run through a pipe and stamps when each line was written.
type pipeSender struct {
	pw   *io.PipeWriter
	sent map[string]time.Time
}

func startPipe(t *testing.T, cfg Config, tools []ToolEntry) (*pipeSender, *tsBuf, chan error) {
	t.Helper()
	pr, pw := io.Pipe()
	out := &tsBuf{}
	res := make(chan error, 1)
	go func() { res <- Run(context.Background(), pr, out, &syncBuf{}, cfg, tools) }()
	t.Cleanup(func() { pw.Close() })
	return &pipeSender{pw: pw, sent: map[string]time.Time{}}, out, res
}

// send is called from the test goroutine only.
func (p *pipeSender) send(t *testing.T, id, line string) {
	t.Helper()
	if _, err := io.WriteString(p.pw, line+"\n"); err != nil {
		t.Fatal(err)
	}
	p.sent[id] = time.Now()
}

func waitRun(t *testing.T, p *pipeSender, res chan error, d time.Duration) {
	t.Helper()
	p.pw.Close()
	select {
	case err := <-res:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(d):
		t.Fatalf("Run did not return %s after EOF", d)
	}
}

// 2. Ping during load (PB5-9): 200 slow calls queued, ping/tools/list interleaved.
func TestStress2PingUnderLoad(t *testing.T) {
	const calls = 200
	m := newStressMock(t, 50*time.Millisecond, 50*time.Millisecond)
	cfg := testConfigWith(m.srv.URL)
	p, out, res := startPipe(t, cfg, buildToolsFor(t, cfg))
	start := time.Now()
	var pings []string
	for i := 1; i <= calls; i++ {
		p.send(t, fmt.Sprint(i), genLine(i, fmt.Sprintf("p%d", i)))
		if i%10 == 0 {
			a, b := fmt.Sprint(10000+i), fmt.Sprint(20000+i)
			p.send(t, a, rpc(10000+i, "ping", ""))
			p.send(t, b, rpc(20000+i, "tools/list", ""))
			pings = append(pings, a, b)
		}
	}
	sendDone := time.Since(start)
	waitRun(t, p, res, 60*time.Second)
	total := time.Since(start)
	resp, dup, _ := out.responses()
	var lat []time.Duration
	for _, id := range pings {
		r, ok := resp[id]
		if !ok {
			t.Errorf("ping/list %s lost", id)
			continue
		}
		lat = append(lat, r.at.Sub(p.sent[id]))
	}
	for i := 1; i <= calls; i++ {
		if _, ok := resp[fmt.Sprint(i)]; !ok {
			t.Errorf("call %d lost", i)
		}
	}
	p50, p99 := pct(lat, .5), pct(lat, .99)
	t.Logf("METRIC ping_queue_total=%s send_done=%s", total.Round(time.Millisecond), sendDone.Round(time.Millisecond))
	t.Logf("METRIC ping_lat_p50=%s p99=%s max=%s n=%d dup=%d bad_json=%d mock_max_concurrency=%d", ms(p50), ms(p99), ms(pct(lat, 1)), len(lat), dup, out.badJS, m.maxCur.Load())
	if p99 > total/10 || p99 > 500*time.Millisecond {
		t.Fatalf("ping p99 %s too high vs queue %s", p99, total)
	}
	if dup != 0 || out.badJS != 0 || m.maxCur.Load() > 1 {
		t.Fatalf("dup=%d badjson=%d maxcur=%d", dup, out.badJS, m.maxCur.Load())
	}
}

// 3. Mass cancellation: 300 calls, cancel every even id.
func TestStress3MassCancel(t *testing.T) {
	const n = 300
	m := newStressMock(t, 10*time.Millisecond, 10*time.Millisecond)
	cfg := testConfigWith(m.srv.URL)
	p, out, res := startPipe(t, cfg, buildToolsFor(t, cfg))
	start := time.Now()
	for i := 1; i <= n; i++ {
		p.send(t, fmt.Sprint(i), genLine(i, fmt.Sprintf("p%d|", i)))
	}
	for i := 2; i <= n; i += 2 {
		p.send(t, "c"+fmt.Sprint(i), fmt.Sprintf(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":%d}}`, i))
	}
	waitRun(t, p, res, 60*time.Second)
	total := time.Since(start)
	resp, dup, nulls := out.responses()
	odd, evenAnswered, evenLate := 0, 0, 0
	for i := 1; i <= n; i++ {
		r, ok := resp[fmt.Sprint(i)]
		if i%2 == 1 {
			if !ok || r.isError {
				t.Errorf("non-cancelled %d: answered=%t err=%t", i, ok, r.isError)
			} else {
				odd++
			}
			continue
		}
		if ok {
			evenAnswered++
			// a cancelled id may only have answered before its cancel arrived
			if r.at.After(p.sent["c"+fmt.Sprint(i)].Add(50 * time.Millisecond)) {
				evenLate++
				t.Errorf("cancelled id %d answered after its cancel", i)
			}
		}
	}
	reached := 0
	for _, pr := range m.snapshotPrompts() {
		var id int
		fmt.Sscanf(pr, "p%d|", &id)
		if id%2 == 0 {
			reached++
		}
	}
	t.Logf("METRIC cancel_total=%s answered_odd=%d/150 even_answered=%d even_late=%d even_reached_mock=%d mock_hits=%d dup=%d nullid=%d bad_json=%d mock_max_concurrency=%d",
		total.Round(time.Millisecond), odd, evenAnswered, evenLate, reached, m.hits.Load(), dup, len(nulls), out.badJS, m.maxCur.Load())
	if odd != n/2 || dup != 0 || out.badJS != 0 {
		t.Fatalf("odd=%d dup=%d badjson=%d", odd, dup, out.badJS)
	}
	if reached > evenAnswered+2 {
		t.Fatalf("%d cancelled calls reached the mock but only %d were answered", reached, evenAnswered)
	}
}

// 4. No leaks: goroutines and heap over 3 cycles of 500 calls.
func TestStress4NoLeak(t *testing.T) {
	const n, cycles = 500, 3
	m := newStressMock(t, 0, time.Millisecond)
	cfg := testConfigWith(m.srv.URL)
	tools := buildToolsFor(t, cfg) // one client for the whole process, as in production
	cycle := func() {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = genLine(i+1, "x")
		}
		out, _, _, _ := runLines(t, cfg, tools, lines)
		if resp, dup, _ := out.responses(); len(resp) != n || dup != 0 {
			t.Fatalf("cycle: %d responses, %d dup", len(resp), dup)
		}
	}
	settle := func() (int, uint64) {
		g := -1
		for i := 0; i < 30; i++ { // idle HTTP conns and finished goroutines wind down
			time.Sleep(100 * time.Millisecond)
			runtime.GC()
			g2 := runtime.NumGoroutine()
			if g2 == g {
				break
			}
			g = g2
		}
		var st runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&st)
		return runtime.NumGoroutine(), st.HeapAlloc
	}
	cycle() // warm-up: first connections, pools, lazy init
	g0, h0 := settle()
	for c := 1; c <= cycles; c++ {
		cycle()
		g, h := settle()
		t.Logf("METRIC leak_cycle%d goroutines=%d delta=%+d heap_alloc=%dKiB delta=%+dKiB", c, g, g-g0, h/1024, (int64(h)-int64(h0))/1024)
	}
	g1, h1 := settle()
	t.Logf("METRIC leak_goroutines_before=%d after=%d delta=%+d", g0, g1, g1-g0)
	t.Logf("METRIC leak_heap_before=%dKiB after=%dKiB delta=%+dKiB", h0/1024, h1/1024, (int64(h1)-int64(h0))/1024)
	if g1 > g0+3 {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines grew %d -> %d\n%s", g0, g1, buf[:runtime.Stack(buf, true)])
	}
}

// 5. Extreme inputs: 1 MiB and 7.9 MiB lines, one above the 8 MiB ceiling, 50 bad JSON lines.
func TestStress5ExtremeInputs(t *testing.T) {
	m := newStressMock(t, 0, 0)
	cfg := testConfigWith(m.srv.URL)
	pad := func(n int) string { return strings.Repeat("a", n) }
	pingPad := func(id, n int) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping","params":{"pad":"%s"}}`, id, pad(n))
	}
	var lines []string
	lines = append(lines, pingPad(1, 1<<20))                 // 1 MiB ping
	lines = append(lines, pingPad(2, 79*(1<<20)/10))         // 7.9 MiB ping
	lines = append(lines, genLine(3, pad(1<<20)))            // 1 MiB prompt > max prompt chars: tool error, never reaches Ollama
	lines = append(lines, pingPad(4, cfg.MaxLineBytes+1024)) // above the ceiling
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"pi`, 100+i)) // truncated JSON
		lines = append(lines, rpc(1000+i, "ping", ""))
	}
	lines = append(lines, genLine(5, "final"))
	out, total, _, errw := runLines(t, cfg, buildToolsFor(t, cfg), lines)
	resp, dup, nulls := out.responses()
	for _, id := range []string{"1", "2", "5"} {
		r, ok := resp[id]
		if !ok || r.isError {
			t.Errorf("id %s: answered=%t err=%t %.200s", id, ok, r.isError, r.line)
		}
	}
	if r, ok := resp["3"]; !ok || !r.isError {
		t.Errorf("1 MiB prompt must be a tool error: %t %.200s", ok, r.line)
	}
	for i := 0; i < 50; i++ {
		if _, ok := resp[fmt.Sprint(1000+i)]; !ok {
			t.Errorf("ping %d after bad json lost", 1000+i)
		}
	}
	tooLong, parseErr := 0, 0
	for _, r := range nulls {
		switch r.code {
		case -32600:
			tooLong++
		case -32700:
			parseErr++
		}
	}
	t.Logf("METRIC extreme_total=%s responses=%d null_id=%d too_long=%d parse_errors=%d dup=%d bad_json=%d mock_hits=%d stderr_bytes=%d",
		total.Round(time.Millisecond), len(resp), len(nulls), tooLong, parseErr, dup, out.badJS, m.hits.Load(), len(errw.String()))
	if tooLong != 1 || parseErr != 50 || dup != 0 || out.badJS != 0 {
		t.Fatalf("too_long=%d parse_errors=%d dup=%d badjson=%d", tooLong, parseErr, dup, out.badJS)
	}
	if m.hits.Load() != 1 {
		t.Fatalf("only the final generate may reach the mock, got %d", m.hits.Load())
	}
}

// 6. Map-reduce under load: 5 files of ~600k chars, small prompt ceiling.
func TestStress6MapReduce(t *testing.T) {
	const files, size, maxPrompt = 5, 600000, 20000
	root, _ := filepath.EvalSymlinks(t.TempDir())
	for f := 0; f < files; f++ {
		var sb strings.Builder
		for i := 0; sb.Len() < size; i++ {
			fmt.Fprintf(&sb, "FILE%d linha %06d: texto de teste para o map-reduce sob carga, sem segredos.\n", f, i)
		}
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("big%d.txt", f)), []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name      string
		maxChunks int
		wantOK    bool
	}{{"max_chunks_200", 200, true}, {"max_chunks_5", 5, false}} {
		t.Run(tc.name, func(t *testing.T) {
			m := newStressMock(t, time.Millisecond, 3*time.Millisecond)
			cfg := testConfigWith(m.srv.URL)
			cfg.AllowRoot, cfg.MaxPromptChars, cfg.MaxChunks = root, maxPrompt, tc.maxChunks
			var lines []string
			for f := 0; f < files; f++ {
				lines = append(lines, toolCall(f+1, "summarize_file", fmt.Sprintf(`{"path":"big%d.txt"}`, f)))
			}
			out, total, _, _ := runLines(t, cfg, buildToolsFor(t, cfg), lines)
			resp, dup, _ := out.responses()
			perFile := make([]int, files)
			maxLen := 0
			for _, pr := range m.snapshotPrompts() {
				if l := utf8.RuneCountInString(pr); l > maxLen {
					maxLen = l
				}
				for f := 0; f < files; f++ {
					if strings.Contains(pr, fmt.Sprintf("FILE%d ", f)) {
						perFile[f]++
						break
					}
				}
			}
			okc := 0
			for i := 1; i <= files; i++ {
				r, ok := resp[fmt.Sprint(i)]
				if !ok {
					t.Errorf("job %d never ended", i)
					continue
				}
				if !r.isError {
					okc++
				}
			}
			mx := 0
			for _, c := range perFile {
				if c > mx {
					mx = c
				}
			}
			t.Logf("METRIC mapreduce_%s total=%s jobs_ok=%d/%d mock_hits=%d per_file=%v max_per_file=%d max_prompt_runes=%d ceiling=%d mock_max_concurrency=%d dup=%d bad_json=%d",
				tc.name, total.Round(time.Millisecond), okc, files, m.hits.Load(), perFile, mx, maxLen, maxPrompt, m.maxCur.Load(), dup, out.badJS)
			if maxLen > maxPrompt {
				t.Errorf("a request had %d chars > ceiling %d", maxLen, maxPrompt)
			}
			if mx > tc.maxChunks {
				t.Errorf("a job made %d calls > --max-chunks %d", mx, tc.maxChunks)
			}
			if tc.wantOK && okc != files {
				t.Errorf("jobs_ok=%d, want %d: %.300s", okc, files, resp["1"].line)
			}
			if !tc.wantOK && okc != 0 {
				t.Errorf("with a tiny --max-chunks no job may succeed: ok=%d", okc)
			}
			if dup != 0 || out.badJS != 0 || m.maxCur.Load() > 1 {
				t.Errorf("dup=%d badjson=%d maxcur=%d", dup, out.badJS, m.maxCur.Load())
			}
		})
	}
}
