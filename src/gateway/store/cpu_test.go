package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// legacyDB makes a database as the gateway left it before the CPU board: the
// requests table without think and no cpu_samples.
func legacyDB(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(oldSchema + `
CREATE TABLE gpu_samples(ts INTEGER, gpu INTEGER, bus_id TEXT, power_w REAL, mem_used_mib INTEGER, util_pct INTEGER, temp_c INTEGER, aer_corr_1b4 INTEGER, boot_id TEXT);
INSERT INTO gpu_samples VALUES(1,0,'x',1.0,1,1,1,0,'old');
INSERT INTO requests(id,enqueued_at,status) VALUES('keep',1,'ok');`); err != nil {
		t.Fatal(err)
	}
	return p
}

func tableCols(t *testing.T, s *Store, table string) map[string]bool {
	t.Helper()
	m, err := tableColumns(s.db, table)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMigrationAddsCPUTableToAnOldDatabase(t *testing.T) {
	p := legacyDB(t)
	for round := 1; round <= 2; round++ { // the second open proves idempotence
		s, err := Open(p, Options{BootID: "new"})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		cols := tableCols(t, s, "cpu_samples")
		for _, c := range []string{"ts", "util_pct", "load1", "load5", "load15", "ram_total_mib", "ram_used_mib", "ram_avail_mib",
			"swap_used_mib", "swap_total_mib", "temp_c", "freq_mhz", "ollama_rss_mib", "power_w", "boot_id", "threads_json"} {
			if !cols[c] {
				t.Fatalf("round %d: cpu_samples lacks %s", round, c)
			}
		}
		s.CPU(CPURow{TS: time.Now(), UtilPct: 12.5, Load1: 1, RAMTotalMiB: 96000, RAMUsedMiB: 48000, PowerW: -1,
			Threads: EncodeThreads([][3]float64{{0, 10, 0}, {6, 15.5, 0}}, [][2]float64{{0, 50}})})
		s.Flush()
		rows, err := s.CPUSince(context.Background(), time.Now().Add(-time.Minute), 10)
		if err != nil || len(rows) != round || rows[round-1].UtilPct != 12.5 || rows[round-1].PowerW != -1 { // rows persist across the reopen
			t.Fatalf("round %d: %v %+v", round, err, rows)
		}
		th, co, ok := DecodeThreads(rows[round-1].Threads)
		if !ok || len(th) != 2 || th[1] != [3]float64{6, 15.5, 0} || len(co) != 1 || co[0] != [2]float64{0, 50} {
			t.Fatalf("threads %v %v %v", th, co, ok)
		}
		// the old data survived
		var kept int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM requests WHERE id='keep'`).Scan(&kept); err != nil || kept != 1 {
			t.Fatalf("old request lost: %d %v", kept, err)
		}
		if g, _ := s.GPUSince(context.Background(), time.UnixMilli(0), 10); len(g) != 1 {
			t.Fatalf("gpu rows %+v", g)
		}
		s.Close()
	}
}

func TestMigrationAddsThreadsColumnToAnEarlyCPUTable(t *testing.T) {
	p := legacyDB(t)
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cpu_samples(ts INTEGER, util_pct REAL, load1 REAL, load5 REAL, load15 REAL,
  ram_total_mib INTEGER, ram_used_mib INTEGER, ram_avail_mib INTEGER, swap_used_mib INTEGER, swap_total_mib INTEGER,
  temp_c REAL, freq_mhz REAL, ollama_rss_mib INTEGER, power_w REAL, boot_id TEXT);
  INSERT INTO cpu_samples VALUES(5,50.0,0,0,0,1,1,1,0,0,40.0,1000.0,0,-1.0,'old');`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !tableCols(t, s, "cpu_samples")["threads_json"] {
		t.Fatal("threads_json not added")
	}
	rows, err := s.CPUSince(context.Background(), time.UnixMilli(0), 10)
	if err != nil || len(rows) != 1 || rows[0].Threads != "" {
		t.Fatalf("%v %+v", err, rows)
	}
}

func TestCPURetentionFollowsGPURetention(t *testing.T) {
	s, _ := open(t, Options{GPUKeep: 7 * 24 * time.Hour})
	now := time.Now()
	s.CPU(CPURow{TS: now.Add(-8 * 24 * time.Hour), UtilPct: 1})
	s.CPU(CPURow{TS: now.Add(-6 * 24 * time.Hour), UtilPct: 2})
	s.CPU(CPURow{TS: now.Add(-time.Hour), UtilPct: 3})
	s.Flush()
	if _, _, err := s.Prune(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	rows, err := s.CPUSince(context.Background(), time.UnixMilli(0), 100)
	if err != nil || len(rows) != 2 || rows[0].UtilPct != 2 || rows[1].UtilPct != 3 {
		t.Fatalf("%v %+v", err, rows)
	}
}

func TestDecodeThreadsRejectsGarbage(t *testing.T) {
	if _, _, ok := DecodeThreads("{not json"); ok {
		t.Fatal("accepted")
	}
	if th, co, ok := DecodeThreads(""); !ok || th != nil || co != nil {
		t.Fatal("empty is valid")
	}
	if EncodeThreads(nil, nil) != "" {
		t.Fatal("nothing to encode")
	}
}
