package content

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func open(t *testing.T, o Options) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "content.db")
	s, err := Open(p, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

// The text comes back byte for byte: quotes, NUL, invalid UTF-8, emoji, CRLF,
// markup. It is stored as a BLOB, so nothing is interpreted.
func TestPromptAndAnswerRoundTripByteForByte(t *testing.T) {
	s, _ := open(t, Options{})
	prompt := "linha1\r\nlinha2 \"aspas\" 'simples' \x00 nulo \xff\xfe inválido 🙂 ç ã </script><img src=x onerror=alert(1)> -- ; DROP TABLE job_content;"
	if _, tr, err := s.SavePrompt("job1", prompt, time.Now()); err != nil || tr {
		t.Fatal(err, tr)
	}
	k := s.NewSink("job1")
	parts := []string{"Resposta ", "em partes ", "\x00\xff", " fim 🙂"}
	for _, p := range parts {
		k.Write(p)
	}
	resp, n := k.Finish("")
	if !resp || n != int64(len(strings.Join(parts, ""))) {
		t.Fatalf("finish: %v %d", resp, n)
	}
	c, ok, err := s.Get(context.Background(), "job1")
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if c.Prompt != prompt || c.Response != strings.Join(parts, "") {
		t.Fatalf("not byte for byte:\n%q\n%q", c.Prompt, c.Response)
	}
	sum := sha256.Sum256([]byte(prompt))
	if c.PromptSHA256 != hex.EncodeToString(sum[:]) || c.PromptBytes != int64(len(prompt)) || c.Truncated || !c.RespFinal {
		t.Fatalf("%+v", c)
	}
}

func TestPerJobCapTruncatesAndMarks(t *testing.T) {
	s, _ := open(t, Options{MaxJobBytes: 1000, MaxBytes: 1 << 20})
	big := strings.Repeat("ç", 800) // 1600 bytes
	stored, tr, err := s.SavePrompt("j", big, time.Now())
	if err != nil || !tr || stored > 1000 || stored%2 != 0 {
		t.Fatalf("stored %d truncated %v err %v (must cut on a rune boundary)", stored, tr, err)
	}
	k := s.NewSink("j")
	k.Write(strings.Repeat("a", 700))
	k.Write(strings.Repeat("b", 700)) // 1400 received, 1000 kept
	k.Finish("")
	c, _, _ := s.Get(context.Background(), "j")
	if !c.PromptTruncated || !c.RespTruncated || !c.Truncated || len(c.Response) != 1000 || c.RespBytes != 1400 || c.PromptBytes != 1600 {
		t.Fatalf("prompt %d/%d resp %d/%d %+v", len(c.Prompt), c.PromptBytes, len(c.Response), c.RespBytes, c)
	}
	if !utf8Valid(c.Prompt) {
		t.Fatal("prompt cut inside a rune")
	}
	// sha256 is of the FULL prompt, not of the cut one
	sum := sha256.Sum256([]byte(big))
	if c.PromptSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("hash must cover the whole prompt")
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "") == s }

func TestTotalCapDeletesOldestAndReportsEvent(t *testing.T) {
	var mu sync.Mutex
	var events []string
	s, _ := open(t, Options{MaxBytes: 10000, MaxJobBytes: 4000, OnEvent: func(k, d string) { mu.Lock(); events = append(events, k+" "+d); mu.Unlock() }})
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 6; i++ { // 6 x 2000 = 12000 > 10000
		if _, _, err := s.SavePrompt(fmt.Sprintf("job%d", i), strings.Repeat("x", 2000), base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if s.Total() > 10000 {
		t.Fatalf("total %d still above the cap", s.Total())
	}
	if _, ok, _ := s.Get(context.Background(), "job0"); ok {
		t.Fatal("the OLDEST job must go first")
	}
	if _, ok, _ := s.Get(context.Background(), "job5"); !ok {
		t.Fatal("the newest job must stay")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 || !strings.HasPrefix(events[0], "teto_conteudo ") {
		t.Fatalf("events %v", events)
	}
	// the counter agrees with the table
	var real int64
	s.db.QueryRow(`SELECT SUM(length(prompt)+length(response)) FROM job_content`).Scan(&real)
	if real != s.Total() {
		t.Fatalf("counter %d != table %d", s.Total(), real)
	}
}

func TestRetentionSevenDays(t *testing.T) {
	s, _ := open(t, Options{})
	now := time.Now()
	s.SavePrompt("old", "velho", now.Add(-8*24*time.Hour))
	s.SavePrompt("edge", "quase", now.Add(-6*24*time.Hour-23*time.Hour))
	s.SavePrompt("new", "novo", now)
	if err := s.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"old": false, "edge": true, "new": true} {
		if _, ok, _ := s.Get(context.Background(), id); ok != want {
			t.Errorf("%s present=%v want %v", id, ok, want)
		}
	}
	if s.Total() != int64(len("quase")+len("novo")) {
		t.Fatalf("total %d", s.Total())
	}
}

// Every commit is synchronous=FULL (it must survive a power cut) and the file
// is private.
func TestSynchronousFullAndPrivateFile(t *testing.T) {
	s, p := open(t, Options{})
	if v, err := s.SyncMode(); err != nil || v != 2 {
		t.Fatalf("synchronous = %d (%v), want 2 (FULL)", v, err)
	}
	var av int
	s.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&av)
	if av != 2 {
		t.Fatalf("auto_vacuum = %d, want 2 (incremental)", av)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(p)
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", fi.Mode().Perm())
		}
	}
}

// The partial answer is on disk every FlushEvery, before Finish: this is what a
// reboot leaves behind.
func TestPartialAnswerIsFlushedWhileStreaming(t *testing.T) {
	s, _ := open(t, Options{FlushEvery: 20 * time.Millisecond})
	s.SavePrompt("p", "pergunta", time.Now())
	k := s.NewSink("p")
	k.Write("primeiro ")
	time.Sleep(40 * time.Millisecond)
	k.Write("segundo ") // this write finds the flush due
	c, _, _ := s.Get(context.Background(), "p")
	if c.Response != "primeiro segundo " || c.RespFinal {
		t.Fatalf("partial not on disk: %q final=%v", c.Response, c.RespFinal)
	}
	k.Write("terceiro") // pending, not yet flushed
	c, _, _ = s.Get(context.Background(), "p")
	if c.Response != "primeiro segundo " {
		t.Fatalf("unexpected early flush: %q", c.Response)
	}
	k.Finish("")
	c, _, _ = s.Get(context.Background(), "p")
	if c.Response != "primeiro segundo terceiro" || !c.RespFinal {
		t.Fatalf("%q %v", c.Response, c.RespFinal)
	}
}

// A separate connection (the "second route") sees the committed prompt even
// though the first one never closed: nothing waits for a checkpoint.
func TestOtherConnectionSeesCommittedData(t *testing.T) {
	s, p := open(t, Options{})
	s.SavePrompt("x", "visivel", time.Now())
	s2, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	c, ok, _ := s2.Get(context.Background(), "x")
	if !ok || c.Prompt != "visivel" {
		t.Fatal("second handle does not see the prompt")
	}
	if s2.Total() != int64(len("visivel")) {
		t.Fatalf("total on reopen %d", s2.Total())
	}
}

func TestEmptyFinishDoesNotWipeTheAnswer(t *testing.T) {
	s, _ := open(t, Options{FlushEvery: time.Millisecond})
	s.SavePrompt("e", "p", time.Now())
	k := s.NewSink("e")
	k.Write("abc")
	time.Sleep(5 * time.Millisecond)
	k.Write("d")
	k.Finish("") // nothing pending: must keep what is stored
	c, _, _ := s.Get(context.Background(), "e")
	if c.Response != "abcd" {
		t.Fatalf("answer wiped: %q", c.Response)
	}
	k2 := s.NewSink("none")
	if resp, n := k2.Finish(""); resp || n != 0 {
		t.Fatal("no text must mean no response")
	}
	k3 := s.NewSink("emb")
	if resp, _ := k3.Finish("embeddings: 2 vetor(es) de 768 dimensões"); !resp {
		t.Fatal("a note counts as a response")
	}
	c, ok, _ := s.Get(context.Background(), "emb")
	if !ok || !strings.HasPrefix(c.Response, "embeddings: 2") {
		t.Fatalf("%v %+v", ok, c)
	}
	_ = bytes.MinRead
}
