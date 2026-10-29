package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fillSynthetic inserts n rows spread evenly over the last `days` days with
// one recursive CTE (fast: no Go round trip per row).
func fillSynthetic(t *testing.T, db *sql.DB, n int, days int) {
	t.Helper()
	now := time.Now().UnixMilli()
	span := int64(days) * 24 * 3600 * 1000
	q := `INSERT INTO requests(id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,
	  enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id)
	WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < ?)
	SELECT lower(hex(randomblob(16))), '10.100.2.' || (x % 50), 'agente-' || (x % 20), 'generate', '/api/generate', 'qwen3-ia',
	  'geracao', 'normal', 'normal', 1000 + (x % 5000),
	  ? - ? + (x * ? / ?), ? - ? + (x * ? / ?) + 50, ? - ? + (x * ? / ?) + 4000, 50 + (x % 900), 3000 + (x % 7000),
	  CASE WHEN x % 50 = 0 THEN 'erro' ELSE 'ok' END, 200, CASE WHEN x % 50 = 0 THEN 'upstream_500' ELSE '' END,
	  100 + (x % 3000), 50 + (x % 800), 0, 'boot-sint'
	FROM c`
	if _, err := db.Exec(q, n, now, span, span, n, now, span, span, n, now, span, span, n); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Fase 9.5, S1 (Baixa): the Stats window uses an index on finished_at, and a
// window above maxStatRows says so.
func TestRegressStatsUsesIndex(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "s.db"), Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var plan strings.Builder
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN SELECT status,error_class,started_at,wait_ms,run_ms,prompt_eval_count,eval_count FROM requests WHERE finished_at >= ? ORDER BY finished_at DESC LIMIT ?`, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, c int
		var d string
		rows.Scan(&a, &b, &c, &d)
		plan.WriteString(d + ";")
	}
	rows.Close()
	if !strings.Contains(plan.String(), "idx_requests_fin") {
		t.Fatalf("plan %q", plan.String())
	}
	old := maxStatRows
	maxStatRows = 100
	defer func() { maxStatRows = old }()
	fillSynthetic(t, s.db, 110, 1)
	st, err := s.Stats(context.Background(), 48*time.Hour, "x")
	if err != nil || !st.Truncated || st.Total != 100 {
		t.Fatalf("truncated=%v total=%d err=%v", st.Truncated, st.Total, err)
	}
}

// Fase 9.5, S2 (Media): the requests table has a row cap enforced by every
// Maintain call, not only the age retention.
func TestRegressRequestsCap(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "c.db"), Options{BootID: "b", MaxRequests: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fillSynthetic(t, s.db, 1500, 1)
	var newest string
	s.db.QueryRow(`SELECT id FROM requests ORDER BY rowid DESC LIMIT 1`).Scan(&newest)
	ctx := context.Background()
	if err := s.Maintain(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, s.db, "requests"); n != 1000 {
		t.Fatalf("rows %d", n)
	}
	var still int
	s.db.QueryRow(`SELECT COUNT(*) FROM requests WHERE id=?`, newest).Scan(&still)
	if still != 1 {
		t.Fatal("the newest row was deleted")
	}
	// hourly calls keep enforcing it (the daily prune is not due again)
	fillSynthetic(t, s.db, 300, 1)
	if err := s.Maintain(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, s.db, "requests"); n != 1000 {
		t.Fatalf("rows %d after the 2nd call", n)
	}
	var ev int
	s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE kind='teto_pedidos'`).Scan(&ev)
	if ev != 2 {
		t.Fatalf("teto_pedidos events %d", ev)
	}
	if DefaultMaxRequests != 3_000_000 {
		t.Fatalf("default cap changed: %d", DefaultMaxRequests)
	}
}

// Fase 9.5, S3 (Baixa): the "registros_descartados" event is written at most
// once a minute, and the count is not lost (it accumulates and is written at
// the next report or at Close).
func TestRegressDropEventsCoalesced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.db")
	s, err := Open(path, Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		s.dropped.Add(3)
		s.Event("x", "")
		s.Flush()
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE kind='registros_descartados'`).Scan(&n)
	if n != 1 {
		t.Fatalf("events %d", n)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path, Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var total, k int64
	r, _ := s2.db.Query(`SELECT detail FROM events WHERE kind='registros_descartados'`)
	for r.Next() {
		var d string
		r.Scan(&d)
		var v int64
		for _, c := range d {
			v = v*10 + int64(c-'0')
		}
		total += v
		k++
	}
	r.Close()
	if total != 60 || k != 2 {
		t.Fatalf("dropped total %d in %d events, want 60 in 2 (one at the first batch, the rest at Close)", total, k)
	}
}
