package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// Fase 10.4: requests.failover_rule is added to an old database in place, the
// old rows stay readable, and opening it again changes nothing.
func TestFailoverRuleMigrationIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(oldSchema + `INSERT INTO requests(id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id)
		VALUES('old1','10.0.0.1','host/1','generate','/api/generate','qwen3-ia','geracao','normal','normal',10,1,1,2,1,2,'ok',200,'',1,2,3,'b0');`); err != nil {
		t.Fatal(err)
	}
	if c := columns(t, old); c["failover_rule"] != 0 {
		t.Fatalf("the fixture is not an old database: %v", c)
	}
	old.Close()
	for round := 1; round <= 3; round++ {
		s, err := Open(p, Options{BootID: "b"})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		c := columns(t, s.db)
		if c["failover_rule"] != 1 || c["backend"] != 1 {
			t.Fatalf("round %d: columns %v", round, c)
		}
		if cols, _ := tableColumns(s.db, "backend_samples"); !cols["backend_id"] || !cols["estado"] || !cols["slots_usados"] || !cols["fila"] || !cols["p50_ms"] {
			t.Fatalf("round %d: backend_samples %v", round, cols)
		}
		rows, err := s.History(context.Background(), HistoryQuery{})
		if err != nil || len(rows) != 1 || rows[0].ID != "old1" || rows[0].FailoverRule != "" || rows[0].Backend != "" {
			t.Fatalf("round %d: %v %+v", round, err, rows)
		}
		s.Close()
	}
}

// The record carries the backend and the rule from the line to the database,
// through both the batch writer and the durable journal pool.
func TestRecordCarriesBackendAndFailoverRule(t *testing.T) {
	s, _ := open(t, Options{BootID: "b"})
	now := time.Now()
	r := rec("viaBatch", now, queue.StatusOK)
	r.Backend, r.FailoverRule = "sec-ws01", "F2:S1+S2"
	s.Record(r)
	r2 := rec("viaJournal", now.Add(time.Second), queue.StatusOK)
	r2.Backend, r2.FailoverRule = "gpu-local", ""
	if err := s.JobEnqueued(r2); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(r2); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	rows, err := s.History(context.Background(), HistoryQuery{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %d", err, len(rows))
	}
	got := map[string]Row{}
	for _, x := range rows {
		got[x.ID] = x
	}
	if x := got["viaBatch"]; x.Backend != "sec-ws01" || x.FailoverRule != "F2:S1+S2" {
		t.Fatalf("%+v", x)
	}
	if x := got["viaJournal"]; x.Backend != "gpu-local" || x.FailoverRule != "" {
		t.Fatalf("%+v", x)
	}
	// the UPSERT at the end keeps what the start wrote when the end has nothing
	r3 := rec("started", now.Add(2*time.Second), queue.StatusQueued)
	_ = s.JobEnqueued(r3)
	_ = s.JobStarted("started", now.Add(2*time.Second), "sec-ws01")
	r3.Status = queue.StatusOK
	r3.FinishedAt = now.Add(3 * time.Second)
	_ = s.Finish(r3)
	if j, ok, _ := s.Job(context.Background(), "started"); !ok || j.Backend != "sec-ws01" {
		t.Fatalf("the backend written at the start was lost: %+v", j.Row)
	}
}

func TestBackendSamplesRoundTripAndRetention(t *testing.T) {
	s, _ := open(t, Options{BootID: "b", GPUKeep: time.Hour})
	now := time.Now()
	s.Backends([]BackendRow{
		{TS: now.Add(-2 * time.Hour), BackendID: "gpu-local", State: "ok", SlotsUsed: 1, Queue: 0, P50MS: 900},
		{TS: now, BackendID: "gpu-local", State: "sobrecarregado", SlotsUsed: 2, Queue: 40, P50MS: 1200},
		{TS: now, BackendID: "sec-ws01", State: "ok", SlotsUsed: 1, Queue: 3, P50MS: 3000},
	})
	s.Flush()
	got, err := s.BackendsSince(context.Background(), now.Add(-3*time.Hour), 0)
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %+v", err, got)
	}
	if got[2].BackendID != "sec-ws01" && got[1].BackendID != "sec-ws01" {
		t.Fatalf("%+v", got)
	}
	if _, _, err := s.Prune(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.BackendsSince(context.Background(), now.Add(-3*time.Hour), 0)
	if len(got) != 2 {
		t.Fatalf("retention: %d rows left, want 2 (same keep as the GPU samples)", len(got))
	}
	s.Backends(nil) // nothing to write is fine
}
