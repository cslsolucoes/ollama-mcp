package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// An old database (no station columns) is migrated on open, idempotently, and
// its rows stay readable with empty station fields.
func TestStationColumnsMigrationIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(oldSchema + `INSERT INTO requests(id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id)
		VALUES('old1','10.0.0.1','host/1','generate','/api/generate','qwen3-ia','geracao','normal','normal',10,1,1,2,1,2,'ok',200,'',1,2,3,'b0');`); err != nil {
		t.Fatal(err)
	}
	if c := columns(t, old); c["station_mac"] != 0 {
		t.Fatalf("the fixture is not an old database: %v", c)
	}
	old.Close()
	for round := 1; round <= 3; round++ {
		s, err := Open(p, Options{BootID: "b"})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		c := columns(t, s.db)
		for _, n := range []string{"station_computer", "station_user", "station_domain", "station_mac"} {
			if c[n] != 1 {
				t.Fatalf("round %d: column %s missing: %v", round, n, c)
			}
		}
		rows, err := s.History(context.Background(), HistoryQuery{})
		if err != nil || len(rows) != 1 || rows[0].ID != "old1" || rows[0].StationMAC != "" || rows[0].StationComputer != "" {
			t.Fatalf("round %d: %v %+v", round, err, rows)
		}
		s.Close()
	}
}

func withStation(r queue.Record) queue.Record {
	r.StationComputer, r.StationUser, r.StationDomain, r.StationMAC = "002-1985", "claiton.linhares", "CSLSOLUCOES", "5CCD5BF40B2D"
	return r
}

// The station goes from the record to the database through the journal pool
// (na_fila), the durable end and the batch writer, and survives a reopen.
func TestRecordCarriesStation(t *testing.T) {
	s, p := open(t, Options{BootID: "b"})
	now := time.Now()
	r := withStation(rec("viaJournal", now, queue.StatusQueued))
	if err := s.JobEnqueued(r); err != nil {
		t.Fatal(err)
	}
	r.Status = queue.StatusOK
	if err := s.Finish(r); err != nil { // UPSERT on the same row
		t.Fatal(err)
	}
	s.Record(withStation(rec("viaBatch", now.Add(time.Second), queue.StatusOK)))
	s.Record(rec("noStation", now.Add(2*time.Second), queue.StatusOK))
	s.Close()

	s2, err := Open(p, Options{BootID: "b2"})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	for _, id := range []string{"viaJournal", "viaBatch"} {
		j, ok, err := s2.Job(context.Background(), id)
		if err != nil || !ok || j.StationComputer != "002-1985" || j.StationUser != "claiton.linhares" || j.StationDomain != "CSLSOLUCOES" || j.StationMAC != "5CCD5BF40B2D" {
			t.Fatalf("%s: %v %v %+v", id, ok, err, j.Row)
		}
	}
	j, _, _ := s2.Job(context.Background(), "noStation")
	if j.StationMAC != "" || j.StationUser != "" {
		t.Fatalf("no station must stay empty: %+v", j.Row)
	}
}

// An end without station never erases the one written at enqueue.
func TestFinishWithoutStationKeepsIt(t *testing.T) {
	s, _ := open(t, Options{BootID: "b3"})
	e := withStation(rec("keep", time.Now(), queue.StatusQueued))
	if err := s.JobEnqueued(e); err != nil {
		t.Fatal(err)
	}
	e.StationComputer, e.StationUser, e.StationDomain, e.StationMAC = "", "", "", ""
	e.Status = queue.StatusOK
	if err := s.Finish(e); err != nil {
		t.Fatal(err)
	}
	if j, _, _ := s.Job(context.Background(), "keep"); j.StationMAC != "5CCD5BF40B2D" {
		t.Fatalf("COALESCE lost the station: %+v", j.Row)
	}
}
