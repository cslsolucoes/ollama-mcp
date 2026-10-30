package pool_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
)

// ---- registry file ----------------------------------------------------------------

func writeFile(t *testing.T, name, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

const goodRegistry = `{"backends":[{"id":"sec-ws01","url":"https://10.100.2.50:8443","tipo":"remoto","slots":2,"modelos":["qwen3-ia"],"max_prompt_tokens":16000,"ordem":1}]}`

func TestLoadFileOK(t *testing.T) {
	specs, err := pool.LoadFile(writeFile(t, "backends.json", goodRegistry, 0o600))
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].ID != "sec-ws01" || specs[0].Slots != 2 || specs[0].Tipo != "remoto" {
		t.Fatalf("specs %+v", specs)
	}
}

// The registry must be 0600, like --admin-file. The mode bits exist only on
// Unix, so the rule is checked through CheckMode on every host and through a
// real file where the host has modes.
func TestRegistryWrongModeIsRefused(t *testing.T) {
	for _, mode := range []os.FileMode{0o644, 0o640, 0o604, 0o666, 0o660} {
		if err := pool.CheckMode("backends.json", "--backends", mode, "linux"); err == nil || !strings.Contains(err.Error(), "0600") {
			t.Fatalf("mode %v accepted: %v", mode, err)
		}
	}
	for _, mode := range []os.FileMode{0o600, 0o400} {
		if err := pool.CheckMode("backends.json", "--backends", mode, "linux"); err != nil {
			t.Fatalf("mode %v refused: %v", mode, err)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("no Unix mode bits on this host; the rule above is what runs on the server")
	}
	p := writeFile(t, "backends.json", goodRegistry, 0o644)
	if _, err := pool.LoadFile(p); err == nil || !strings.Contains(err.Error(), "0600") {
		t.Fatalf("a 0644 registry was accepted: %v", err)
	}
}

func TestRegistryValidation(t *testing.T) {
	bad := map[string]string{
		"unknown field":     `{"backends":[{"id":"a","url":"http://10.0.0.1:11434","tipo":"gpu","extra":1}]}`,
		"bad kind":          `{"backends":[{"id":"a","url":"http://10.0.0.1:11434","tipo":"nuvem"}]}`,
		"cloud-looking url": `{"backends":[{"id":"a","url":"ftp://10.0.0.1","tipo":"gpu"}]}`,
		"url with creds":    `{"backends":[{"id":"a","url":"http://u:p@10.0.0.1:11434","tipo":"gpu"}]}`,
		"url with path":     `{"backends":[{"id":"a","url":"http://10.0.0.1:11434/x","tipo":"gpu"}]}`,
		"metadata address":  `{"backends":[{"id":"a","url":"http://169.254.169.254","tipo":"gpu"}]}`,
		"unspecified":       `{"backends":[{"id":"a","url":"http://0.0.0.0:11434","tipo":"gpu"}]}`,
		"reserved id":       `{"backends":[{"id":"gpu-local","url":"http://10.0.0.1:11434","tipo":"gpu"}]}`,
		"duplicate id":      `{"backends":[{"id":"a","url":"http://10.0.0.1:11434","tipo":"gpu"},{"id":"a","url":"http://10.0.0.2:11434","tipo":"gpu"}]}`,
		"bad id":            `{"backends":[{"id":"A b","url":"http://10.0.0.1:11434","tipo":"gpu"}]}`,
		"bad class":         `{"backends":[{"id":"a","url":"http://10.0.0.1:11434","tipo":"gpu","classes":["urgente"]}]}`,
		"token over http":   `{"backends":[{"id":"a","url":"http://10.0.0.1:11434","tipo":"gpu","token_file":"/x"}]}`,
		"ca over http":      `{"backends":[{"id":"a","url":"http://10.0.0.1:11434","tipo":"gpu","tls_ca":"/x"}]}`,
		"trailing text":     goodRegistry + ` {}`,
		"not json":          `backends`,
	}
	for name, body := range bad {
		if _, err := pool.LoadFile(writeFile(t, "b.json", body, 0o600)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestBuildReadsTheTokenFileAndNeverPrintsIt(t *testing.T) {
	dir := t.TempDir()
	tok := filepath.Join(dir, "tok")
	if err := os.WriteFile(tok, []byte("sec-token-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bs, err := pool.Build([]pool.Spec{{ID: "s", URL: "http://127.0.0.1:1", Tipo: "gpu", TokenFile: tok}}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := bs[0]
	if b.Token() != "sec-token-123" {
		t.Fatalf("token %q", b.Token())
	}
	for _, s := range []string{b.String(), b.Host(), mustJSON(t, b.Spec)} {
		if strings.Contains(s, "sec-token-123") {
			t.Fatalf("token leaked in %q", s)
		}
	}
	// a token with a newline inside would split the header
	bad := filepath.Join(dir, "bad")
	_ = os.WriteFile(bad, []byte("abc\r\nX-Evil: 1"), 0o600)
	if _, err := pool.Build([]pool.Spec{{ID: "s", URL: "http://127.0.0.1:1", Tipo: "gpu", TokenFile: bad}}, nil, "-think", time.Second, nil); err == nil {
		t.Fatal("a token with CR/LF was accepted")
	}
	// a missing token file fails the start without echoing a secret
	if _, err := pool.Build([]pool.Spec{{ID: "s", URL: "http://127.0.0.1:1", Tipo: "gpu", TokenFile: filepath.Join(dir, "nope")}}, nil, "-think", time.Second, nil); err == nil {
		t.Fatal("a missing token file was accepted")
	}
}

// ---- circuit breaker --------------------------------------------------------------

func newBare(t *testing.T, ev *events) (*pool.Pool, *pool.Backend) {
	t.Helper()
	bs, err := pool.Build([]pool.Spec{{ID: "sec", URL: "http://127.0.0.1:1", Tipo: "gpu"}}, nil, "-think", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	pl := pool.New(pool.Config{PrimarySlots: 1, Secondaries: bs, BreakerFails: 3, BreakerOpen: 30 * time.Second, OnEvent: ev.add})
	return pl, bs[0]
}

func TestBreakerOpensHalfOpensAndCloses(t *testing.T) {
	ev := &events{}
	pl, _ := newBare(t, ev)
	t0 := time.Now()
	for i := 0; i < 2; i++ {
		pl.NoteEnd("sec", pool.OutcomeFail, false, true, 10, t0)
	}
	if !pl.CanStart("sec", t0) {
		t.Fatal("two failures must not open it")
	}
	pl.NoteEnd("sec", pool.OutcomeFail, false, true, 10, t0) // third
	if pl.CanStart("sec", t0.Add(time.Second)) {
		t.Fatal("the breaker must be open after 3 failures")
	}
	if !ev.has("breaker_open") {
		t.Fatalf("events %v", ev.kinds())
	}
	// still open just before 30 s
	if pl.CanStart("sec", t0.Add(29*time.Second)) {
		t.Fatal("open for 30 s")
	}
	// half-open: exactly one probe
	at := t0.Add(31 * time.Second)
	if !pl.CanStart("sec", at) {
		t.Fatal("half-open must allow the probe")
	}
	probe := pl.NoteStart("sec", at)
	if !probe {
		t.Fatal("the first start in half-open is the probe")
	}
	if pl.CanStart("sec", at) {
		t.Fatal("only ONE probe in half-open")
	}
	// the probe fails: open again for another 30 s
	pl.NoteEnd("sec", pool.OutcomeFail, true, true, 10, at)
	if pl.CanStart("sec", at.Add(time.Second)) {
		t.Fatal("a failed probe reopens the breaker")
	}
	// next half-open, the probe succeeds: closed
	at2 := at.Add(31 * time.Second)
	if !pl.CanStart("sec", at2) || !pl.NoteStart("sec", at2) {
		t.Fatal("second half-open")
	}
	pl.NoteEnd("sec", pool.OutcomeOK, true, true, 10, at2)
	if !pl.CanStart("sec", at2) || !pl.CanStart("sec", at2) {
		t.Fatal("closed again")
	}
	if !ev.has("breaker_close") {
		t.Fatalf("events %v", ev.kinds())
	}
}

// A success resets the count: failures must be consecutive.
func TestBreakerNeedsConsecutiveFailures(t *testing.T) {
	ev := &events{}
	pl, _ := newBare(t, ev)
	now := time.Now()
	for i := 0; i < 10; i++ {
		pl.NoteEnd("sec", pool.OutcomeFail, false, true, 1, now)
		pl.NoteEnd("sec", pool.OutcomeFail, false, true, 1, now)
		pl.NoteEnd("sec", pool.OutcomeOK, false, true, 1, now)
	}
	if !pl.CanStart("sec", now) || ev.has("breaker_open") {
		t.Fatal("alternating failures must not open the breaker")
	}
}

// A neutral end (client gone, admin cancel) frees the probe slot without
// judging the backend.
func TestNeutralEndFreesTheProbe(t *testing.T) {
	pl, _ := newBare(t, &events{})
	now := time.Now()
	for i := 0; i < 3; i++ {
		pl.NoteEnd("sec", pool.OutcomeFail, false, true, 1, now)
	}
	at := now.Add(31 * time.Second)
	if !pl.CanStart("sec", at) || !pl.NoteStart("sec", at) {
		t.Fatal("probe")
	}
	pl.NoteEnd("sec", pool.OutcomeNeutral, true, true, 1, at)
	if !pl.CanStart("sec", at) {
		t.Fatal("a neutral end must give the probe slot back")
	}
}

// Without secondaries the breaker is tracked but never gates: the gateway keeps
// dispatching to its only Ollama as it always did.
func TestLoneBackendIsNeverGated(t *testing.T) {
	pl := pool.New(pool.Config{PrimarySlots: 1})
	now := time.Now()
	for i := 0; i < 10; i++ {
		pl.NoteEnd(pool.DefaultPrimaryID, pool.OutcomeFail, false, true, 1, now)
	}
	if !pl.CanStart(pool.DefaultPrimaryID, now) {
		t.Fatal("a lone backend must always be startable")
	}
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 1000}); o.On {
		t.Fatal("nothing to evaluate without a secondary")
	}
}

// ---- overload + hysteresis ----------------------------------------------------------

func TestOverloadHysteresis(t *testing.T) {
	ev := &events{}
	bs, _ := pool.Build([]pool.Spec{{ID: "sec", URL: "http://127.0.0.1:1", Tipo: "gpu"}}, nil, "-think", time.Second, nil)
	pl := pool.New(pool.Config{PrimarySlots: 1, Secondaries: bs, QueueMax: 8, OverloadQueue: 4, Hold: 30 * time.Second, OnEvent: ev.add})
	t0 := time.Now()
	if o := pl.Evaluate(t0, pool.LoadInput{WaitingGen: 4, PrimaryUsed: 1}); o.On {
		t.Fatal("4 waiting is not above 4")
	}
	if o := pl.Evaluate(t0, pool.LoadInput{WaitingGen: 5, PrimaryUsed: 1}); !o.On || !o.S2 {
		t.Fatalf("5 waiting must trigger S2: %+v", o)
	}
	// load drops below half of the limit (2): still on until 30 s of calm
	low := pool.LoadInput{WaitingGen: 1, PrimaryUsed: 1}
	if o := pl.Evaluate(t0.Add(time.Second), low); !o.On {
		t.Fatal("must stay on: the hold has not passed")
	}
	if o := pl.Evaluate(t0.Add(20*time.Second), low); !o.On {
		t.Fatal("must stay on at 19 s of calm")
	}
	// a bump in the middle restarts the hold
	if o := pl.Evaluate(t0.Add(25*time.Second), pool.LoadInput{WaitingGen: 3, PrimaryUsed: 1}); !o.On {
		t.Fatal("above half the limit: still on")
	}
	if o := pl.Evaluate(t0.Add(40*time.Second), low); !o.On {
		t.Fatal("the hold restarted at the bump")
	}
	if o := pl.Evaluate(t0.Add(71*time.Second), low); o.On {
		t.Fatal("30 s of calm: the primary takes work back")
	}
	got := strings.Join(ev.kinds(), ",")
	if got != "overload_on,overload_off" {
		t.Fatalf("events %s", got)
	}
}

func TestEstimatedWaitUsesTheEWMA(t *testing.T) {
	bs, _ := pool.Build([]pool.Spec{{ID: "sec", URL: "http://127.0.0.1:1", Tipo: "gpu"}}, nil, "-think", time.Second, nil)
	pl := pool.New(pool.Config{PrimarySlots: 2, Secondaries: bs, OverloadWait: time.Minute})
	now := time.Now()
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 50, PrimaryUsed: 2}); o.S1 {
		t.Fatal("no samples yet: no estimate")
	}
	for i := 0; i < 20; i++ {
		pl.NoteEnd(pool.DefaultPrimaryID, pool.OutcomeOK, false, true, 20_000, now)
	}
	// 20 s each, 2 slots, a newcomer behind 5 waiting: 20 s * 6 / 2 = 60 s: not above
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 5, PrimaryUsed: 2}); o.S1 || o.EstWait != 60*time.Second {
		t.Fatalf("estimate %v S1=%v", o.EstWait, o.S1)
	}
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 6, PrimaryUsed: 2}); !o.S1 {
		t.Fatalf("70 s must be S1: %+v", o)
	}
	// a free slot and nobody waiting: the newcomer starts at once
	if o := pl.Evaluate(now, pool.LoadInput{WaitingGen: 0, PrimaryUsed: 1}); o.EstWait != 0 {
		t.Fatalf("estimate with a free slot = %v", o.EstWait)
	}
}

// ---- the pure eligibility rule ------------------------------------------------------

func TestEligibleRules(t *testing.T) {
	base := pool.Cand{Kind: pool.KindGPU, Digest: "d1"}
	req := pool.Req{Model: "qwen3-ia", Class: pool.ClassNormal, Tokens: 100, PrimaryDigest: "d1"}
	if !pool.Eligible(base, req) {
		t.Fatal("same model, same digest, normal: eligible")
	}
	cases := []struct {
		name string
		c    func(c *pool.Cand)
		r    func(r *pool.Req)
	}{
		{"digest differs", func(c *pool.Cand) { c.Digest = "d2" }, nil},
		{"primary digest unknown", nil, func(r *pool.Req) { r.PrimaryDigest = "" }},
		{"secondary digest unknown", func(c *pool.Cand) { c.Digest = "" }, nil},
		{"drained", func(c *pool.Cand) { c.Drained = true }, nil},
		{"unhealthy", func(c *pool.Cand) { c.Unhealthy = true }, nil},
		{"model not served", func(c *pool.Cand) { c.Models = []string{"outro"} }, nil},
		{"class not accepted", func(c *pool.Cand) { c.Classes = []string{pool.ClassLow} }, nil},
		{"prompt too big", func(c *pool.Cand) { c.MaxTokens = 50 }, nil},
		{"alta to cpu", func(c *pool.Cand) { c.Kind = pool.KindCPU; c.Classes = []string{"alta", "normal"} }, func(r *pool.Req) { r.Class = pool.ClassHigh }},
	}
	for _, tc := range cases {
		c, r := base, req
		if tc.c != nil {
			tc.c(&c)
		}
		if tc.r != nil {
			tc.r(&r)
		}
		if pool.Eligible(c, r) {
			t.Errorf("%s: eligible", tc.name)
		}
	}
	// alta to a gpu or remote secondary is allowed (F3)
	for _, k := range []pool.Kind{pool.KindGPU, pool.KindRemote} {
		c, r := base, req
		c.Kind, r.Class = k, pool.ClassHigh
		if !pool.Eligible(c, r) {
			t.Errorf("alta must be allowed to %s", k)
		}
	}
	// ":latest" is the same model
	c, r := base, req
	c.Models, r.Model = []string{"qwen3-ia:latest"}, "qwen3-ia"
	if !pool.Eligible(c, r) {
		t.Error(":latest must not matter")
	}
	// a cpu backend defaults to normal and baixa only
	c, r = base, req
	c.Kind, r.Class = pool.KindCPU, pool.ClassHigh
	if pool.Eligible(c, r) {
		t.Error("cpu default classes exclude alta")
	}
}

// Fuzz target kept as a seed-corpus regression test (the long run is
// FuzzEligible below): the invariants of the routing decision.
func TestEligibleInvariantsSeeds(t *testing.T) {
	for _, kind := range []string{"gpu", "cpu", "remoto"} {
		for _, class := range []string{"alta", "normal", "baixa"} {
			checkEligibleInvariants(t, kind, class, "d1", "d1", 10, 100, false, false)
			checkEligibleInvariants(t, kind, class, "d1", "d2", 10, 100, false, false)
			checkEligibleInvariants(t, kind, class, "", "", 10, 0, true, true)
		}
	}
}

func checkEligibleInvariants(t *testing.T, kind, class, primDigest, secDigest string, tokens int64, maxTokens int, drained, unhealthy bool) {
	t.Helper()
	c := pool.Cand{Kind: pool.Kind(kind), Digest: secDigest, MaxTokens: maxTokens, Drained: drained, Unhealthy: unhealthy}
	r := pool.Req{Model: "m", Class: class, Tokens: tokens, PrimaryDigest: primDigest}
	got := pool.Eligible(c, r)
	if !got {
		return
	}
	switch {
	case c.Drained || c.Unhealthy:
		t.Fatalf("a drained/unhealthy backend was eligible: %+v %+v", c, r)
	case c.Kind == pool.KindCPU && r.Class == pool.ClassHigh:
		t.Fatalf("alta went to cpu: %+v %+v", c, r)
	case r.PrimaryDigest == "" || c.Digest == "" || r.PrimaryDigest != c.Digest:
		t.Fatalf("a digest mismatch was eligible: %+v %+v", c, r)
	case c.MaxTokens > 0 && r.Tokens > int64(c.MaxTokens):
		t.Fatalf("a prompt over the limit was eligible: %+v %+v", c, r)
	}
}

func FuzzEligible(f *testing.F) {
	f.Add("gpu", "normal", "d1", "d1", int64(10), 100, false, false)
	f.Add("cpu", "alta", "d1", "d1", int64(10), 0, false, false)
	f.Add("remoto", "baixa", "", "d1", int64(1<<40), 5, true, false)
	f.Fuzz(func(t *testing.T, kind, class, pd, sd string, tokens int64, maxTokens int, drained, unhealthy bool) {
		checkEligibleInvariants(t, kind, class, pd, sd, tokens, maxTokens, drained, unhealthy)
	})
}

func TestEstimateTokens(t *testing.T) {
	if pool.EstimateTokens(0) != 0 || pool.EstimateTokens(-5) != 0 || pool.EstimateTokens(3) != 1 || pool.EstimateTokens(4) != 2 || pool.EstimateTokens(30000) != 10000 {
		t.Fatal("estimate")
	}
}
