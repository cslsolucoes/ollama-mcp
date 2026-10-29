package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// oldSchema is the requests table of the gateway before Fase 9.6a (no think).
const oldSchema = `
CREATE TABLE requests(
  id TEXT PRIMARY KEY, client_ip TEXT, client_label TEXT, tool TEXT, endpoint TEXT, model TEXT,
  lane TEXT, priority_in TEXT, priority_final TEXT, bytes_in INTEGER,
  enqueued_at INTEGER, started_at INTEGER, finished_at INTEGER,
  wait_ms INTEGER, run_ms INTEGER,
  status TEXT, http_status INTEGER, error_class TEXT,
  prompt_eval_count INTEGER, eval_count INTEGER, load_ms INTEGER, boot_id TEXT);
`

func columns(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(requests)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		out[name]++
	}
	return out
}

// Fase 9.6a: a database created before the think column is migrated in place,
// the old rows stay readable (think empty), new ones carry the mode, and
// opening it again does not fail or duplicate anything.
func TestThinkColumnMigrationOnAnOldDatabase(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := old.Exec(`INSERT INTO requests(id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id)
		VALUES('old1','10.0.0.1','host/1','generate','/api/generate','qwen3-ia','geracao','normal','normal',10,?,?,?,1,2,'ok',200,'',1,2,3,'b0')`,
		now.UnixMilli(), now.UnixMilli(), now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if c := columns(t, old); c["think"] != 0 {
		t.Fatalf("the fixture is not an old database: %v", c)
	}
	old.Close()

	for round := 1; round <= 2; round++ { // the 2nd open proves the migration is idempotent
		s, err := Open(p, Options{BootID: "boot-A"})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if c := columns(t, s.db); c["think"] != 1 {
			t.Fatalf("round %d: think column: %v", round, c)
		}
		r := rec("new"+string(rune('0'+round)), now.Add(time.Duration(round)*time.Second), queue.StatusOK)
		r.Think = "on"
		s.Record(r)
		s.Flush()
		rows, err := s.History(context.Background(), HistoryQuery{})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, x := range rows {
			got[x.ID] = x.Think
		}
		if len(rows) != 1+round || got["old1"] != "" || got["new"+string(rune('0'+round))] != "on" {
			t.Fatalf("round %d: rows %+v", round, got)
		}
		s.Close()
	}
}

func TestThinkRoundTripAllModes(t *testing.T) {
	s, _ := open(t, Options{BootID: "b"})
	now := time.Now()
	for i, mode := range []string{"on", "off", "modelo", ""} {
		r := rec("r"+mode+string(rune('0'+i)), now.Add(time.Duration(i)*time.Second), queue.StatusOK)
		r.Think = mode
		s.Record(r)
	}
	s.Flush()
	rows, _ := s.History(context.Background(), HistoryQuery{})
	if len(rows) != 4 {
		t.Fatalf("%d rows", len(rows))
	}
	want := map[string]string{"ron0": "on", "roff1": "off", "rmodelo2": "modelo", "r3": ""}
	for _, x := range rows {
		if want[x.ID] != x.Think {
			t.Fatalf("%s: think %q, want %q", x.ID, x.Think, want[x.ID])
		}
	}
}
