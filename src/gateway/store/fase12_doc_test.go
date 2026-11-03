package store

// Fase 12, etapa 12.3: requests.doc_hash and requests.doc_key_id (contract
// C-4.2, C-4.3a) and their migration (C-2.7).

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func withDoc(r queue.Record) queue.Record {
	r.DocHash, r.DocKeyID = strings.Repeat("ab", 32), "630dcd2966c43366"
	return r
}

func jobDoc(t *testing.T, s *Store, id string) (string, string) {
	t.Helper()
	h, k, err := s.JobDoc(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return h, k
}

// C-2.7: an old database (no doc columns), with data, migrates in place; nothing
// is lost; opening it again changes nothing; old rows read as "no document".
func TestDocColumnsMigrationKeepsDataAndIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(oldSchema + `INSERT INTO requests(id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id)
		VALUES('old1','10.0.0.1','host/1','generate','/api/generate','qwen3-ia','geracao','normal','normal',10,1,1,2,1,2,'ok',200,'',1,2,3,'b0'),
		('old2','10.0.0.2','host/2','extract','/api/chat','qwen3-ia','geracao','normal','normal',20,5,5,9,1,4,'ok',200,'',4,5,6,'b0');`); err != nil {
		t.Fatal(err)
	}
	if c := columns(t, old); c["doc_hash"] != 0 || c["doc_key_id"] != 0 {
		t.Fatalf("the fixture is not an old database: %v", c)
	}
	old.Close()
	for round := 1; round <= 3; round++ {
		s, err := Open(p, Options{BootID: "b"})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		c := columns(t, s.db)
		if c["doc_hash"] != 1 || c["doc_key_id"] != 1 {
			t.Fatalf("round %d: doc columns missing or duplicated: %v", round, c)
		}
		rows, err := s.History(context.Background(), HistoryQuery{})
		// The rows of earlier rounds stay too (the job left in na_fila by the
		// previous round becomes interrupted): 2 old + (round-1) new ones.
		if err != nil || len(rows) != 2+round-1 {
			t.Fatalf("round %d: %v %+v", round, err, rows)
		}
		kept := map[string]int64{}
		for _, r := range rows {
			kept[r.ID] = r.BytesIn
		}
		if kept["old1"] != 10 || kept["old2"] != 20 {
			t.Fatalf("round %d: the old rows changed: %v", round, kept)
		}
		for _, id := range []string{"old1", "old2"} {
			if h, k := jobDoc(t, s, id); h != "" || k != "" {
				t.Fatalf("round %d: %s reads a document: %q %q", round, id, h, k)
			}
		}
		// A new row writes the columns.
		nid := "new" + string(rune('0'+round))
		if err := s.JobEnqueued(withDoc(rec(nid, time.Now(), queue.StatusQueued))); err != nil {
			t.Fatal(err)
		}
		if h, k := jobDoc(t, s, nid); h != strings.Repeat("ab", 32) || k != "630dcd2966c43366" {
			t.Fatalf("round %d: %q %q", round, h, k)
		}
		s.Close()
	}
}

// C-4.2: the HMAC and the key id go from the record to the database through
// every path (journal entry, durable end, batch writer) and survive a reopen.
func TestRecordCarriesDocHashAndKeyID(t *testing.T) {
	s, p := open(t, Options{BootID: "b"})
	now := time.Now()
	r := withDoc(rec("viaJournal", now, queue.StatusQueued))
	if err := s.JobEnqueued(r); err != nil {
		t.Fatal(err)
	}
	r.Status = queue.StatusOK
	if err := s.Finish(r); err != nil {
		t.Fatal(err)
	}
	s.Record(withDoc(rec("viaBatch", now.Add(time.Second), queue.StatusOK)))
	s.Record(rec("noDoc", now.Add(2*time.Second), queue.StatusOK))
	s.Close()

	s2, err := Open(p, Options{BootID: "b2"})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	for _, id := range []string{"viaJournal", "viaBatch"} {
		if h, k := jobDoc(t, s2, id); h != strings.Repeat("ab", 32) || k != "630dcd2966c43366" {
			t.Fatalf("%s: %q %q", id, h, k)
		}
	}
	if h, k := jobDoc(t, s2, "noDoc"); h != "" || k != "" {
		t.Fatalf("no document must stay empty: %q %q", h, k)
	}
}

// An end without a hash never erases the one written at the entry.
func TestFinishWithoutDocKeepsIt(t *testing.T) {
	s, _ := open(t, Options{BootID: "b3"})
	e := withDoc(rec("keep", time.Now(), queue.StatusQueued))
	if err := s.JobEnqueued(e); err != nil {
		t.Fatal(err)
	}
	e.DocHash, e.DocKeyID = "", ""
	e.Status = queue.StatusOK
	if err := s.Finish(e); err != nil {
		t.Fatal(err)
	}
	if h, k := jobDoc(t, s, "keep"); h != strings.Repeat("ab", 32) || k != "630dcd2966c43366" {
		t.Fatalf("COALESCE lost the document: %q %q", h, k)
	}
}

// The document hash never reaches the monitor's JSON of a row.
func TestDocHashIsNotInTheRowJSON(t *testing.T) {
	s, _ := open(t, Options{BootID: "b4"})
	if err := s.JobEnqueued(withDoc(rec("x", time.Now(), queue.StatusQueued))); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Jobs(context.Background(), JobQuery{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %d", err, len(rows))
	}
	b, _ := json.Marshal(rows[0])
	if strings.Contains(string(b), strings.Repeat("ab", 32)) || strings.Contains(string(b), "630dcd2966c43366") || strings.Contains(string(b), "doc_hash") {
		t.Fatalf("the document hash leaked into the row JSON: %s", b)
	}
}

// M4: JobDoc reads through the read pool, never the write pool of the job log
// (pattern of 0.12.6): with every connection of the write pool taken, it still
// answers at once.
func TestJobDocDoesNotUseTheWritePool(t *testing.T) {
	s, _ := open(t, Options{BootID: "b5"})
	if err := s.JobEnqueued(withDoc(rec("rd", time.Now(), queue.StatusQueued))); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var held []*sql.Conn
	for i := 0; i < 2; i++ { // the write pool has 2 connections
		c, err := s.jdb.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	defer func() {
		for _, c := range held {
			c.Close()
		}
	}()
	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	h, k, err := s.JobDoc(tctx, "rd")
	if err != nil || h != strings.Repeat("ab", 32) || k != "630dcd2966c43366" {
		t.Fatalf("JobDoc with the write pool busy: %q %q %v", h, k, err)
	}
}
