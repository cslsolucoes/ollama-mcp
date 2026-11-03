package content

// Fase 12, etapa 12.3: prompt_json in the content database. One test (or more)
// per rule of docs/CONTRATO-DADOS-TREINO.md, chapter 2.

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func getPJ(t *testing.T, s *Store, id string) PromptJSON {
	t.Helper()
	p, ok, err := s.GetPromptJSON(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("GetPromptJSON(%s): ok=%v err=%v", id, ok, err)
	}
	return p
}

// C-2.1, C-2.2: the JSON is stored with the flat text, in the same insert, and
// comes back byte for byte (quotes, NUL, invalid UTF-8 included: it is a BLOB).
func TestSavePromptJSONRoundTripByteForByte(t *testing.T) {
	s, _ := open(t, Options{})
	pj := []byte("{\"model\":\"qwen3-ia\",\"messages\":[{\"role\":\"user\",\"content\":\"a\\u0000 \xff 🙂\"}],\"options\":{\"seed\":7}}")
	res, err := s.SavePromptJSON("j1", "[user]\nflat", pj, false, time.Now())
	if err != nil || res.Truncated || res.JSONTruncated || res.JSONStored != len(pj) {
		t.Fatalf("%+v %v", res, err)
	}
	got := getPJ(t, s, "j1")
	if !bytes.Equal(got.JSON, pj) || got.Truncated || got.HasImage {
		t.Fatalf("not byte for byte: %q %+v", got.JSON, got)
	}
	c, _, _ := s.Get(context.Background(), "j1")
	if c.Prompt != "[user]\nflat" {
		t.Fatalf("the flat text changed: %q", c.Prompt)
	}
}

// C-2.1: it is idempotent like the flat text (the first one wins), and a job
// saved without JSON keeps the column NULL (nothing to export, C-1.9).
func TestSavePromptJSONFirstWinsAndNullWithoutJSON(t *testing.T) {
	s, _ := open(t, Options{})
	s.SavePromptJSON("j", "t", []byte(`{"a":1}`), false, time.Now())
	s.SavePromptJSON("j", "t2", []byte(`{"a":2}`), true, time.Now())
	if got := getPJ(t, s, "j"); string(got.JSON) != `{"a":1}` || got.HasImage {
		t.Fatalf("the second save overwrote the first: %+v", got)
	}
	s.SavePrompt("old", "só texto", time.Now())
	if got := getPJ(t, s, "old"); got.JSON != nil || got.Truncated || got.HasImage {
		t.Fatalf("SavePrompt must leave prompt_json NULL: %+v", got)
	}
}

// C-2.3: the flat text and the JSON have separate 16 MiB (here 1000 B) caps and
// separate marks; cutting one never marks the other.
func TestSeparateCapsAndMarks(t *testing.T) {
	s, _ := open(t, Options{MaxJobBytes: 1000, MaxBytes: 1 << 20})
	small, big := "p", strings.Repeat("é", 800) // 1600 bytes: over the cap
	smallJ, bigJ := []byte(`{"p":"x"}`), []byte(`{"p":"`+strings.Repeat("é", 800)+`"}`)
	cases := []struct {
		id                 string
		flat               string
		pj                 []byte
		wantFlat, wantJSON bool
	}{
		{"neither", small, smallJ, false, false},
		{"flat only", big, smallJ, true, false},
		{"json only", small, bigJ, false, true},
		{"both", big, bigJ, true, true},
	}
	for _, c := range cases {
		res, err := s.SavePromptJSON(c.id, c.flat, c.pj, false, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if res.Truncated != c.wantFlat || res.JSONTruncated != c.wantJSON {
			t.Errorf("%s: result flat=%v json=%v, want %v %v", c.id, res.Truncated, res.JSONTruncated, c.wantFlat, c.wantJSON)
		}
		in, _, _ := s.Get(context.Background(), c.id)
		pj := getPJ(t, s, c.id)
		if in.PromptTruncated != c.wantFlat || pj.Truncated != c.wantJSON {
			t.Errorf("%s: stored marks flat=%v json=%v, want %v %v", c.id, in.PromptTruncated, pj.Truncated, c.wantFlat, c.wantJSON)
		}
		if len(pj.JSON) > 1000 || len(in.Prompt) > 1000 {
			t.Errorf("%s: a part passed the cap (%d, %d)", c.id, len(pj.JSON), len(in.Prompt))
		}
		if c.wantJSON && !utf8Valid(string(pj.JSON)) {
			t.Errorf("%s: the cut JSON split a rune", c.id)
		}
	}
}

// C-2.4: the answer keeps its own cap, resp_truncated and resp_final (unchanged).
func TestAnswerCapAndFinalMarksUnchanged(t *testing.T) {
	s, _ := open(t, Options{MaxJobBytes: 500, MaxBytes: 1 << 20})
	s.SavePromptJSON("a", "p", []byte(`{}`), false, time.Now())
	k := s.NewSink("a")
	k.Write(strings.Repeat("r", 800))
	// Not finished yet: resp_final stays 0.
	k.mu.Lock()
	k.flushLocked(false)
	k.mu.Unlock()
	c, _, _ := s.Get(context.Background(), "a")
	if c.RespFinal || !c.RespTruncated || len(c.Response) != 500 {
		t.Fatalf("mid-stream: %+v", c)
	}
	k.Finish("")
	c, _, _ = s.Get(context.Background(), "a")
	if !c.RespFinal || !c.RespTruncated || c.RespBytes != 800 || len(c.Response) != 500 {
		t.Fatalf("after finish: final=%v trunc=%v bytes=%d stored=%d", c.RespFinal, c.RespTruncated, c.RespBytes, len(c.Response))
	}
	if c.PromptTruncated || getPJ(t, s, "a").Truncated {
		t.Fatal("an answer cut marked the prompt")
	}
	// An untouched answer under the cap is final and not truncated.
	s.SavePromptJSON("b", "p", []byte(`{}`), false, time.Now())
	kb := s.NewSink("b")
	kb.Write("curta")
	kb.Finish("")
	cb, _, _ := s.Get(context.Background(), "b")
	if !cb.RespFinal || cb.RespTruncated {
		t.Fatalf("short answer: %+v", cb)
	}
}

// C-2.5: tem_imagem is stored as given and does not touch the other marks.
func TestTemImagemStored(t *testing.T) {
	s, _ := open(t, Options{})
	s.SavePromptJSON("img", "t", []byte(`{"images":[{"imagem_hmac":"ab"}]}`), true, time.Now())
	s.SavePromptJSON("noimg", "t", []byte(`{"prompt":"x"}`), false, time.Now())
	if !getPJ(t, s, "img").HasImage || getPJ(t, s, "noimg").HasImage {
		t.Fatal("tem_imagem not stored as given")
	}
}

// C-2.8: the prompt_json counts in the total and the oldest jobs go first.
func TestTotalCapCountsPromptJSONAndDeletesOldest(t *testing.T) {
	s, _ := open(t, Options{MaxBytes: 10000, MaxJobBytes: 4000})
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 6; i++ { // only the JSON is big: 6 x 2000 = 12000 > 10000
		if _, err := s.SavePromptJSON(fmt.Sprintf("job%d", i), "p", bytes.Repeat([]byte("j"), 2000), false, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	s.WaitCap()
	if s.Total() > 10000 {
		t.Fatalf("total %d still above the cap: the JSON is not counted", s.Total())
	}
	if _, ok, _ := s.GetPromptJSON(context.Background(), "job0"); ok {
		t.Fatal("the OLDEST job must go first")
	}
	if _, ok, _ := s.GetPromptJSON(context.Background(), "job5"); !ok {
		t.Fatal("the newest job must stay")
	}
	var real int64
	s.db.QueryRow(`SELECT SUM(length(prompt)+COALESCE(length(prompt_json),0)+length(response)) FROM job_content`).Scan(&real)
	if real != s.Total() {
		t.Fatalf("counter %d != table %d", s.Total(), real)
	}
}

// C-2.8: the default total cap is 4 GiB and the retention stays 7 days.
func TestDefaultsFourGiBAndSevenDays(t *testing.T) {
	s, _ := open(t, Options{})
	mb, mj := s.Limits()
	if mb != 4<<30 || mj != 16<<20 || DefaultKeep != 7*24*time.Hour {
		t.Fatalf("defaults: total=%d job=%d keep=%v", mb, mj, DefaultKeep)
	}
}

// The total counter at start-up already includes the JSON of an existing file.
func TestOpenCountsPromptJSONInTheTotal(t *testing.T) {
	s, p := open(t, Options{})
	s.SavePromptJSON("a", "xx", bytes.Repeat([]byte("j"), 500), false, time.Now())
	want := s.Total()
	s.Close()
	s2, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.Total() != want || want < 500 {
		t.Fatalf("total after reopen %d, want %d", s2.Total(), want)
	}
}

// The old schema (before Fase 12), with rows that have an answer.
const oldSchema = `
CREATE TABLE job_content(
  job_id TEXT PRIMARY KEY, created_at INTEGER, updated_at INTEGER,
  prompt BLOB, prompt_bytes INTEGER, prompt_sha256 TEXT, prompt_truncated INTEGER,
  response BLOB, resp_bytes INTEGER, resp_truncated INTEGER, resp_final INTEGER);
CREATE INDEX idx_content_created ON job_content(created_at);
`

func columns(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(job_content)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var cid, nn, pk int
		var name, typ string
		var d sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &nn, &d, &pk); err != nil {
			t.Fatal(err)
		}
		have[name] = true
	}
	return have
}

// C-2.7: a database in the old schema, with data, opens; nothing is lost; the
// new columns exist; opening it twice more does not fail; the old rows read as
// "no JSON kept"; and new rows work beside them.
func TestMigrationOldDatabaseKeepsDataAndIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "content.db")
	old, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := old.Exec(`INSERT INTO job_content VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("old%d", i), 1000+i, 2000+i, []byte(fmt.Sprintf("prompt antigo %d", i)), 15, "sha", 0, []byte("resposta antiga"), 15, 0, 1); err != nil {
			t.Fatal(err)
		}
	}
	if columns(t, old)["prompt_json"] {
		t.Fatal("the fixture is not the old schema")
	}
	old.Close()

	for round := 1; round <= 3; round++ {
		s, err := Open(p, Options{})
		if err != nil {
			t.Fatalf("open #%d: %v", round, err)
		}
		cols := columns(t, s.db)
		for _, c := range []string{"prompt_json", "prompt_json_truncated", "tem_imagem"} {
			if !cols[c] {
				t.Fatalf("open #%d: column %s missing", round, c)
			}
		}
		n, _ := s.Count(context.Background())
		if n != int64(3+round-1) {
			t.Fatalf("open #%d: %d rows", round, n)
		}
		for i := 0; i < 3; i++ {
			c, ok, err := s.Get(context.Background(), fmt.Sprintf("old%d", i))
			if err != nil || !ok || c.Prompt != fmt.Sprintf("prompt antigo %d", i) || c.Response != "resposta antiga" || !c.RespFinal || c.CreatedAt != int64(1000+i) {
				t.Fatalf("open #%d: old row %d changed: %+v %v %v", round, i, c, ok, err)
			}
			pj := getPJ(t, s, fmt.Sprintf("old%d", i))
			if pj.JSON != nil || pj.Truncated || pj.HasImage {
				t.Fatalf("open #%d: old row %d reads as having JSON: %+v", round, i, pj)
			}
		}
		if _, err := s.SavePromptJSON(fmt.Sprintf("new%d", round), "t", []byte(`{"a":1}`), true, time.Now()); err != nil {
			t.Fatal(err)
		}
		if got := getPJ(t, s, fmt.Sprintf("new%d", round)); string(got.JSON) != `{"a":1}` || !got.HasImage {
			t.Fatalf("open #%d: new row: %+v", round, got)
		}
		s.Close()
	}
}

// A row for an answer whose prompt row is missing (the sink's fallback insert)
// still works with the new columns.
func TestSinkFallbackInsertWorksWithNewColumns(t *testing.T) {
	s, _ := open(t, Options{})
	k := s.NewSink("orphan")
	k.Write("resp")
	k.Finish("")
	c, ok, _ := s.Get(context.Background(), "orphan")
	if !ok || c.Response != "resp" {
		t.Fatalf("%+v %v", c, ok)
	}
	if pj := getPJ(t, s, "orphan"); pj.JSON != nil || pj.Truncated || pj.HasImage {
		t.Fatalf("%+v", pj)
	}
}

// B1: a job whose body could not be scanned keeps no JSON and is marked as
// having an image and as truncated, so it can never be exported.
func TestSavePromptJSONFailedFailsClosed(t *testing.T) {
	s, _ := open(t, Options{})
	res, err := s.SavePromptJSONFailed("fc", "texto", time.Now())
	if err != nil || !res.JSONTruncated || res.JSONStored != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	p := getPJ(t, s, "fc")
	if p.JSON != nil || !p.HasImage || !p.Truncated {
		t.Fatalf("%+v", p)
	}
	if c, _, _ := s.Get(context.Background(), "fc"); c.Prompt != "texto" {
		t.Fatal("the flat text must still be kept")
	}
}
