package treino

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// C-4.5a (security review of 481aee0): a reference written before the column
// "valida" existed was never checked against the origin, so the migration must
// leave it as a reference only (valida=0), never as a link of groups.
func TestMigrationLeavesOldJobReferencesUnvalidated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "treino.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE ocorrencia_jobs(ocorrencia_id TEXT NOT NULL, job_id TEXT NOT NULL, papel TEXT NOT NULL,
		  backend TEXT NOT NULL CHECK(backend IN ('primario','fallback')), ordem INTEGER NOT NULL,
		  PRIMARY KEY(ocorrencia_id, job_id))`,
		`INSERT INTO ocorrencia_jobs(ocorrencia_id,job_id,papel,backend,ordem) VALUES('o1','00000000000000000000000000000001','unico','primario',0)`,
		`PRAGMA user_version=3`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()
	s, err := Open(path, Options{Now: openStore(t, Options{}).opts.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var valida int
	if err := s.db.QueryRow(`SELECT valida FROM ocorrencia_jobs WHERE ocorrencia_id='o1'`).Scan(&valida); err != nil {
		t.Fatal(err)
	}
	if valida != 0 {
		t.Fatalf("a reference from before the column came out valida=%d; it must be 0 (fail closed)", valida)
	}
}
