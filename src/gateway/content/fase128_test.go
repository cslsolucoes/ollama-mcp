package content

// Fase 12.8 (review): prompt_json_original and the trace of the improvement (C-2.10a).

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openC(t *testing.T, o Options) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.db")
	s, err := Open(p, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func TestMelhoraTraceIsStoredBesideTheBodyTheOllamaReceived(t *testing.T) { // C-2.10a
	s, _ := openC(t, Options{})
	now := time.Now()
	sent, orig := []byte(`{"system":"a\n\nINSTR","prompt":"p"}`), []byte(`{"system":"a","prompt":"p"}`)
	if _, err := s.SavePromptJSONMelhora("job-m", "texto", sent, false, MelhoraTrace{Original: orig, InstrucoesID: "iabc", IndiceID: "idx1", Exemplos: []string{"o1", "o2"}}, now); err != nil {
		t.Fatal(err)
	}
	p, ok, err := s.GetPromptJSON(context.Background(), "job-m")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if string(p.JSON) != string(sent) || string(p.Original) != string(orig) || p.OriginalTruncated || p.InstrucoesID != "iabc" || p.IndiceID != "idx1" || p.Exemplos != "o1,o2" {
		t.Errorf("%+v", p)
	}
	// a job the improvement did not change: all of it empty
	if _, err := s.SavePromptJSON("job-p", "texto", sent, false, now); err != nil {
		t.Fatal(err)
	}
	q, _, _ := s.GetPromptJSON(context.Background(), "job-p")
	if q.Original != nil || q.InstrucoesID != "" || q.IndiceID != "" || q.Exemplos != "" || q.OriginalTruncated {
		t.Errorf("a plain job has a trace: %+v", q)
	}
	// instructions only: no index, no examples
	if _, err := s.SavePromptJSONMelhora("job-i", "t", sent, false, MelhoraTrace{Original: orig, InstrucoesID: "iabc"}, now); err != nil {
		t.Fatal(err)
	}
	r, _, _ := s.GetPromptJSON(context.Background(), "job-i")
	if r.InstrucoesID != "iabc" || r.IndiceID != "" || r.Exemplos != "" {
		t.Errorf("%+v", r)
	}
}

func TestOriginalHasItsOwnCapAndTruncationMark(t *testing.T) { // C-2.10a: same cap, own mark
	s, _ := openC(t, Options{MaxJobBytes: 64})
	sent := []byte(`{"p":"` + strings.Repeat("a", 20) + `"}`)
	orig := []byte(`{"p":"` + strings.Repeat("b", 200) + `"}`)
	res, err := s.SavePromptJSONMelhora("j", "t", sent, false, MelhoraTrace{Original: orig, InstrucoesID: "i"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.JSONTruncated {
		t.Error("the cut of the original marked prompt_json")
	}
	p, _, _ := s.GetPromptJSON(context.Background(), "j")
	if !p.OriginalTruncated || len(p.Original) > 64 || p.Truncated || string(p.JSON) != string(sent) {
		t.Errorf("original truncated=%v len=%d, prompt_json truncated=%v", p.OriginalTruncated, len(p.Original), p.Truncated)
	}
}

func TestMigrationAddsTheMelhoraColumnsToAnOldFileAndIsIdempotent(t *testing.T) { // C-2.7
	p := filepath.Join(t.TempDir(), "c.db")
	old, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	// a file of Fase 12.3: prompt_json exists, the 12.8 columns do not
	for _, q := range []string{
		`CREATE TABLE job_content(job_id TEXT PRIMARY KEY, created_at INTEGER, updated_at INTEGER, prompt BLOB, prompt_bytes INTEGER, prompt_sha256 TEXT, prompt_truncated INTEGER,
			response BLOB, resp_bytes INTEGER, resp_truncated INTEGER, resp_final INTEGER, prompt_json BLOB, prompt_json_truncated INTEGER NOT NULL DEFAULT 0, tem_imagem INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO job_content(job_id,created_at,updated_at,prompt,prompt_bytes,prompt_sha256,prompt_truncated,response,resp_bytes,resp_truncated,resp_final,prompt_json) VALUES('velho',1,1,x'61',1,'h',0,x'',0,0,0,x'7b7d')`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()
	for i := 0; i < 3; i++ {
		s, err := Open(p, Options{})
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		g, ok, err := s.GetPromptJSON(context.Background(), "velho")
		if err != nil || !ok || string(g.JSON) != "{}" || g.Original != nil || g.InstrucoesID != "" {
			t.Fatalf("open %d: %+v %v", i, g, err)
		}
		if _, err := s.SavePromptJSONMelhora("novo"+string(rune('a'+i)), "t", []byte(`{}`), false, MelhoraTrace{Original: []byte(`{"o":1}`), IndiceID: "x"}, time.Now()); err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}

func TestTheOriginalCountsInTheTotalCap(t *testing.T) { // the 4 GiB cap sees both requests
	s, _ := openC(t, Options{})
	if _, err := s.SavePromptJSONMelhora("j", "t", []byte(`{}`), false, MelhoraTrace{Original: []byte(strings.Repeat("o", 1000)), InstrucoesID: "i"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := s.total.Load(); got < 1000 {
		t.Errorf("the total is %d, the original was not counted", got)
	}
}
