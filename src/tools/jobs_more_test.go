package tools

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cslsoftwares/mcpollama/ollama"
)

// A non-2xx answer of the primary (here with a request id) never names the job:
// the fallback answered without an id, so the result has no job at all.
func TestNon2xxRequestIDIsNeverRecorded(t *testing.T) {
	primary := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", jobID(0x999))
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"error":"fila do servidor cheia"}`, http.StatusServiceUnavailable)
	})
	fb := newGWMock(t, 0, false, nil)
	d := primary.deps(t, func(o *ollama.Options) { o.FallbackURL = fb.srv.URL })
	res, err := call(t, d, "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	line := lastLine(text(t, res))
	if !strings.Contains(line, "backend=fallback") || strings.Contains(line, "job") {
		t.Fatalf("usage line: %q", line)
	}
	if u, _ := res.Meta["mcpollama/usage"].(map[string]any); u["jobs"] != nil {
		t.Fatalf("meta must have no jobs: %+v", u)
	}
}

// 503 with a short Retry-After, then 200 from the same primary: the job is the
// one of the second answer.
func TestRetryOnPrimaryUsesTheSecondRequestID(t *testing.T) {
	var n atomic.Int32
	m := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("X-Request-Id", jobID(0xa1))
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"fila do servidor cheia"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("X-Request-Id", jobID(0xa2))
		generateOK(w, r)
	})
	res, err := call(t, m.deps(t, nil), "generate", `{"prompt":"x"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if n.Load() != 2 {
		t.Fatalf("primary hits %d, want 2", n.Load())
	}
	line := lastLine(text(t, res))
	if !strings.HasSuffix(line, " job="+jobID(0xa2)) || strings.Contains(line, jobID(0xa1)) {
		t.Fatalf("usage line: %q", line)
	}
}

var reduceNonce = regexp.MustCompile(`PARCIAL_INICIO (\w+)=====`)

// Two or more intermediate reduce rounds: the reduceK numbering is continuous
// over the whole request (it never restarts per round), the last call is final
// and its text is the result.
func TestReduceRoundsHaveContinuousRolesAndFinalIsTheResult(t *testing.T) {
	const ceiling = 2600
	e := newFileEnv(t)
	e.put(t, "grande.txt", lines(200, 40))
	long := strings.Repeat("palavra ", 150) // 1,200 runes per map answer
	mid := strings.Repeat("palavra ", 75)   // 600 runes per reduce answer: the rounds shrink slowly
	g := newGWMock(t, 0, true, func(n int32, prompt string) (string, int) {
		if strings.Contains(prompt, "PARCIAL_INICIO") {
			return fmt.Sprintf("R-%d %s", n, mid), 0
		}
		return fmt.Sprintf("M-%d %s", n, long), 0
	})
	res, err := call(t, e.deps(t, g.mock, ceiling, 40), "summarize_file", `{"path":"grande.txt"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	maps, reduces := 0, 0
	nonces := map[string]bool{}
	for _, p := range g.prompts() {
		if m := reduceNonce.FindStringSubmatch(p); m != nil {
			reduces++
			nonces[m[1]] = true
		} else {
			maps++
		}
	}
	if len(nonces) < 3 {
		t.Fatalf("need >= 2 intermediate rounds plus the final (3 nonces), got %d", len(nonces))
	}
	var want []string
	for i := 1; i <= maps; i++ {
		want = append(want, fmt.Sprintf("map%d:%s", i, jobID(i)))
	}
	for i := 1; i < reduces; i++ {
		want = append(want, fmt.Sprintf("reduce%d:%s", i, jobID(maps+i)))
	}
	want = append(want, "final:"+jobID(maps+reduces))
	got := jobsOfLine(t, lastLine(text(t, res)))
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("jobs\n got %v\nwant %v", got, want)
	}
	// the answer is the text of the final call, the very last one
	if out := text(t, res); !strings.HasPrefix(out, fmt.Sprintf("R-%d ", maps+reduces)) {
		t.Fatalf("result is not the final call's text: %.40q", out)
	}
	t.Logf("%d maps, %d intermediate reduces in %d rounds, then final", maps, reduces-1, len(nonces)-1)
}

// A stream cut in the middle still names the job; without an id the text is
// the same as before.
func TestPartialFailureNamesTheJob(t *testing.T) {
	cut := func(path, id string) *mock {
		return newMock(t, func(w http.ResponseWriter, r *http.Request) {
			if id != "" {
				w.Header().Set("X-Request-Id", id)
			}
			if path == "/api/chat" {
				streamLines(w, `{"model":"qwen3-ia","message":{"role":"assistant","content":"parcial"},"done":false}`)
				return
			}
			streamLines(w, `{"model":"qwen3-ia","response":"parcial","done":false}`)
		})
	}
	cases := []struct{ tool, path, args string }{
		{"generate", "/api/generate", `{"prompt":"x"}`},
		{"chat", "/api/chat", `{"messages":[{"role":"user","content":"x"}]}`},
	}
	suffix := "\n\njob=" + jobID(0xb1)
	for _, c := range cases {
		with, err := call(t, cut(c.path, jobID(0xb1)).deps(t, nil), c.tool, c.args)
		if err != nil || !with.IsError {
			t.Fatalf("%s: %v %+v", c.tool, err, with)
		}
		wt := text(t, with)
		if !strings.Contains(wt, "parcial") || !strings.HasSuffix(wt, suffix) {
			t.Fatalf("%s partial text: %q", c.tool, wt)
		}
		without, err := call(t, cut(c.path, "").deps(t, nil), c.tool, c.args)
		if err != nil || !without.IsError {
			t.Fatalf("%s: %v %+v", c.tool, err, without)
		}
		if nt := text(t, without); strings.Contains(nt, "job=") || nt != strings.TrimSuffix(wt, suffix) {
			t.Fatalf("%s without id must equal today's text:\n%q\n%q", c.tool, nt, wt)
		}
	}
}
