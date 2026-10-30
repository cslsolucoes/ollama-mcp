package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrationAddsPowerLimitToAnOldGPUTable(t *testing.T) {
	p := legacyDB(t) // gpu_samples without power_limit_w, one old row
	for round := 1; round <= 2; round++ {
		s, err := Open(p, Options{BootID: "new"})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if !tableCols(t, s, "gpu_samples")["power_limit_w"] {
			t.Fatalf("round %d: column missing", round)
		}
		if round == 1 {
			s.GPU([]GPURow{{TS: time.Now(), GPU: 1, BusID: "y", PowerW: 71, PowerLimitW: 100}})
			s.Flush()
		}
		rows, err := s.GPUSince(context.Background(), time.UnixMilli(0), 10)
		if err != nil || len(rows) != 2 {
			t.Fatalf("round %d: %v %+v", round, err, rows)
		}
		if rows[0].PowerLimitW != -1 { // the old row: NULL read as -1
			t.Fatalf("old row limit = %v", rows[0].PowerLimitW)
		}
		if rows[1].PowerLimitW != 100 || rows[1].PowerW != 71 {
			t.Fatalf("new row %+v", rows[1])
		}
		s.Close()
	}
}

func TestNewDatabaseHasPowerLimit(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "n.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !tableCols(t, s, "gpu_samples")["power_limit_w"] {
		t.Fatal("column missing in a new database")
	}
}
