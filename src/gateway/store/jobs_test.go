package store_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func openJ(t *testing.T, path, boot string) *store.Store {
	t.Helper()
	st, err := store.Open(path, store.Options{BootID: boot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	paths[st] = path
	return st
}

func rec(id string) queue.Record {
	return queue.Record{ID: id, ClientIP: "10.0.0.5", ClientLabel: "cli/1", Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia",
		Lane: queue.LaneGen, PriorityIn: queue.Normal, PriorityFinal: queue.Normal, BytesIn: 1234, EnqueuedAt: time.Now(), Status: queue.StatusQueued}
}

func jobByID(t *testing.T, st *store.Store, id string) store.JobRow {
	t.Helper()
	j, ok, err := st.Job(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("job %s: %v %v", id, ok, err)
	}
	return j
}

func TestJobLifecycleNaFilaEmExecucaoEnd(t *testing.T) {
	st := openJ(t, filepath.Join(t.TempDir(), "gw.db"), "boot-A")
	r := rec("aaaa")
	if err := st.JobEnqueued(r); err != nil {
		t.Fatal(err)
	}
	if j := jobByID(t, st, "aaaa"); j.Status != "na_fila" || j.StartedAt != 0 || j.FinishedAt != 0 || j.BootID != "boot-A" {
		t.Fatalf("%+v", j.Row)
	}
	at := r.EnqueuedAt.Add(1500 * time.Millisecond)
	if err := st.JobStarted("aaaa", at, "gpu-local"); err != nil {
		t.Fatal(err)
	}
	if j := jobByID(t, st, "aaaa"); j.Status != "em_execucao" || j.StartedAt != at.UnixMilli() || j.Backend != "gpu-local" || j.WaitMS < 1400 {
		t.Fatalf("%+v", j.Row)
	}
	// still in flight: History (the finished list) does not show it, Jobs does
	if h, _ := st.History(context.Background(), store.HistoryQuery{}); len(h) != 0 {
		t.Fatalf("in-flight job leaked into History: %d", len(h))
	}
	if js, _ := st.Jobs(context.Background(), store.JobQuery{Status: []string{"em_execucao"}}); len(js) != 1 {
		t.Fatalf("Jobs by status: %d", len(js))
	}
	var rowid1 int64
	st2 := openRaw(t, st)
	st2.QueryRow(`SELECT rowid FROM requests WHERE id='aaaa'`).Scan(&rowid1)

	end := r
	end.Status, end.HTTPStatus, end.StartedAt, end.FinishedAt = queue.StatusOK, 200, at, at.Add(3*time.Second)
	end.WaitMS, end.RunMS, end.PromptEval, end.Eval, end.Responded, end.RespBytes = 1500, 3000, 50, 20, true, 99
	if err := st.Finish(end); err != nil {
		t.Fatal(err)
	}
	j := jobByID(t, st, "aaaa")
	if j.Status != "ok" || !j.Responded || j.RespBytes != 99 || j.Eval != 20 || j.Backend != "gpu-local" || j.FinishedAt != end.FinishedAt.UnixMilli() {
		t.Fatalf("end lost data (the backend of the start must survive the UPSERT): %+v", j.Row)
	}
	var rowid2 int64
	st2.QueryRow(`SELECT rowid FROM requests WHERE id='aaaa'`).Scan(&rowid2)
	if rowid1 != rowid2 {
		t.Fatalf("rowid changed %d -> %d: the row cap depends on it", rowid1, rowid2)
	}
	if h, _ := st.History(context.Background(), store.HistoryQuery{}); len(h) != 1 {
		t.Fatalf("finished job missing in History: %d", len(h))
	}
}

// openRaw is a second, independent handle on the same file (the "2nd route").
func openRaw(t *testing.T, st *store.Store) *sql.DB {
	t.Helper()
	return rawDB(t, dbPathOf(st))
}

var paths = map[*store.Store]string{}

func dbPathOf(st *store.Store) string { return paths[st] }

func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// A later state written first is never undone by an earlier one.
func TestLateEntryAndStartNeverUndoTheEnd(t *testing.T) {
	st := openJ(t, filepath.Join(t.TempDir(), "gw.db"), "b")
	r := rec("bbbb")
	end := r
	end.Status, end.FinishedAt, end.HTTPStatus = queue.StatusRemoved, time.Now(), 409
	if err := st.Finish(end); err != nil {
		t.Fatal(err)
	}
	if err := st.JobEnqueued(r); err != nil { // arrives late
		t.Fatal(err)
	}
	if err := st.JobStarted("bbbb", time.Now(), "x"); err != nil {
		t.Fatal(err)
	}
	if j := jobByID(t, st, "bbbb"); j.Status != "removido" {
		t.Fatalf("the end was undone: %s", j.Status)
	}
}

func TestJournalPoolIsSynchronousFull(t *testing.T) {
	st := openJ(t, filepath.Join(t.TempDir(), "gw.db"), "b")
	batch, journal, err := st.SyncModes()
	if err != nil || journal != 2 {
		t.Fatalf("journal pool synchronous=%d (%v), want 2 (FULL)", journal, err)
	}
	if batch != 1 {
		t.Fatalf("batch pool synchronous=%d, want 1 (NORMAL, as before)", batch)
	}
}

func TestRecoveryAfterRebootAndAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gw.db")
	a := openJ(t, path, "boot-1")
	t0 := time.Now().Add(-time.Minute)
	// one queued, one running (with 3 s of extract), one finished
	q := rec("queued00")
	q.EnqueuedAt = t0
	run := rec("running0")
	run.EnqueuedAt = t0
	done := rec("done0000")
	done.EnqueuedAt = t0
	for _, r := range []queue.Record{q, run, done} {
		if err := a.JobEnqueued(r); err != nil {
			t.Fatal(err)
		}
	}
	a.JobStarted("running0", t0.Add(2*time.Second), "gpu-local")
	for i := 0; i < 3; i++ {
		a.AddJobSamples([]store.JobSample{{TS: t0.Add(time.Duration(3+i) * time.Second), JobID: "running0", GPU: 0, PowerW: 90}})
	}
	d := done
	d.Status, d.FinishedAt, d.StartedAt = "ok", t0.Add(time.Second), t0.Add(time.Second)
	a.Finish(d)
	// the process dies here: no Close, no Flush

	b := openJ(t, path, "boot-2") // another boot: the host rebooted
	rc := b.Recovered()
	if len(rc) != 2 {
		t.Fatalf("recovered %v", rc)
	}
	for _, id := range []string{"queued00", "running0"} {
		if j := jobByID(t, b, id); j.Status != "interrompido_reboot" || j.ErrorClass != "interrompido_reboot" || j.FinishedAt == 0 {
			t.Fatalf("%s: %+v", id, j.Row)
		}
	}
	j := jobByID(t, b, "running0")
	if j.FinishedAt != t0.Add(5*time.Second).UnixMilli() || j.RunMS != 3000 {
		t.Fatalf("end must be the last second of the extract: finished=%d run=%d", j.FinishedAt-t0.UnixMilli(), j.RunMS)
	}
	if j := jobByID(t, b, "done0000"); j.Status != "ok" {
		t.Fatal("a finished job must not be touched")
	}
	ev, _ := b.Events(context.Background(), 50)
	found := false
	for _, e := range ev {
		if e.Kind == "jobs_interrompidos" && strings.Contains(e.Detail, "reboot=2") && strings.Contains(e.Detail, "total=2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("event missing: %v", ev)
	}
	// they show in History now (they ended)
	if h, _ := b.History(context.Background(), store.HistoryQuery{}); len(h) != 3 {
		t.Fatalf("history %d", len(h))
	}
	b.Close()

	// same boot id: the gateway process died, the host did not
	c := openJ(t, path, "boot-2")
	if len(c.Recovered()) != 0 {
		t.Fatal("nothing left to recover")
	}
	c.JobEnqueued(rec("again000"))
	c.Close()
	d2 := openJ(t, path, "boot-2")
	if j := jobByID(t, d2, "again000"); j.Status != "interrompido_gateway" {
		t.Fatalf("same boot must be interrompido_gateway, got %s", j.Status)
	}
	// recovery is idempotent
	d2.Close()
	e := openJ(t, path, "boot-3")
	if len(e.Recovered()) != 0 {
		t.Fatal("second recovery found work")
	}
}

// A real killed process: the child writes the log and exits with no Close, no
// Flush and no deferred function; the parent reopens the file with another boot.
func TestHelperProcessKilled(t *testing.T) {
	dir := os.Getenv("STORE_KILL_DIR")
	if dir == "" {
		t.Skip("helper for TestJobSurvivesAKilledProcess")
	}
	st, err := store.Open(filepath.Join(dir, "gw.db"), store.Options{BootID: "boot-old"})
	if err != nil {
		os.Exit(2)
	}
	r := rec("killed00")
	st.JobEnqueued(r)
	st.JobStarted("killed00", time.Now(), "gpu-local")
	st.AddJobSamples([]store.JobSample{{TS: time.Now(), JobID: "killed00", GPU: 0, PowerW: 99, ClockMHz: 1400}})
	os.Exit(3) // the hard stop
}

func TestJobSurvivesAKilledProcess(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessKilled$")
	cmd.Env = append(os.Environ(), "STORE_KILL_DIR="+dir)
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
		t.Fatalf("helper did not stop hard: %v", err)
	}
	st := openJ(t, filepath.Join(dir, "gw.db"), "boot-new")
	j := jobByID(t, st, "killed00")
	if j.Status != "interrompido_reboot" || j.Backend != "gpu-local" || j.StartedAt == 0 || j.Samples != 1 || j.PeakClockMHz != 1400 {
		t.Fatalf("the job of the killed process is not in the log: %+v", j)
	}
}

func TestJobSamplesPeaksAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gw.db")
	st, err := store.Open(path, store.Options{BootID: "b", JobGPUKeep: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now()
	st.JobEnqueued(rec("jjjj"))
	var rows []store.JobSample
	for s := 0; s < 5; s++ {
		for g := 0; g < 4; g++ {
			rows = append(rows, store.JobSample{TS: now.Add(time.Duration(s) * time.Second), JobID: "jjjj", GPU: g, PowerW: float64(80 + g*5 + s), PowerLimitW: 100, ClockMHz: float64(1300 + g), UtilPct: 90, MemUsedMiB: 1000, TempC: 60})
		}
		rows = append(rows, store.JobSample{TS: now.Add(time.Duration(s) * time.Second), JobID: "jjjj", GPU: store.CPUGPUIndex, PowerW: 500, UtilPct: 10, TempC: 50})
	}
	if err := st.AddJobSamples(rows); err != nil {
		t.Fatal(err)
	}
	got, _ := st.JobSamplesOf(context.Background(), "jjjj", 0)
	if len(got) != 25 {
		t.Fatalf("%d samples", len(got))
	}
	j := jobByID(t, st, "jjjj")
	if j.Samples != 5 || j.PeakPowerW != 99 || j.PeakClockMHz != 1303 { // the CPU line (500 W) must not count as a GPU peak
		t.Fatalf("peaks %+v", j)
	}
	js, _ := st.Jobs(context.Background(), store.JobQuery{})
	if len(js) != 1 || js[0].PeakPowerW != 99 {
		t.Fatalf("%+v", js)
	}
	// retention: 7 days
	st.AddJobSamples([]store.JobSample{{TS: now.Add(-8 * 24 * time.Hour), JobID: "old", GPU: 0}, {TS: now.Add(-6 * 24 * time.Hour), JobID: "edge", GPU: 0}})
	if _, _, err := st.Prune(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if o, _ := st.JobSamplesOf(context.Background(), "old", 0); len(o) != 0 {
		t.Fatal("8-day-old extract kept")
	}
	if o, _ := st.JobSamplesOf(context.Background(), "edge", 0); len(o) != 1 {
		t.Fatal("6-day-old extract dropped")
	}
}

// An old database (before 10.L) opens, gets the new columns once, and keeps
// its rows.
func TestMigrationFromTheOldSchemaIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE requests(id TEXT PRIMARY KEY, client_ip TEXT, client_label TEXT, tool TEXT, endpoint TEXT, model TEXT,
		lane TEXT, priority_in TEXT, priority_final TEXT, bytes_in INTEGER, enqueued_at INTEGER, started_at INTEGER, finished_at INTEGER,
		wait_ms INTEGER, run_ms INTEGER, status TEXT, http_status INTEGER, error_class TEXT, prompt_eval_count INTEGER, eval_count INTEGER, load_ms INTEGER, boot_id TEXT, think TEXT);
		CREATE TABLE gpu_samples(ts INTEGER, gpu INTEGER, bus_id TEXT, power_w REAL, mem_used_mib INTEGER, util_pct INTEGER, temp_c INTEGER, aer_corr_1b4 INTEGER, boot_id TEXT, power_limit_w REAL);
		INSERT INTO requests VALUES('old1','1.1.1.1','x','t','/e','m','geracao','normal','normal',1,1000,1100,2000,100,900,'ok',200,'',5,7,0,'b','off');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ {
		st, err := store.Open(path, store.Options{BootID: "b"})
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		j := jobByID(t, st, "old1")
		if j.Eval != 7 || j.Responded || j.Backend != "" {
			t.Fatalf("%+v", j.Row)
		}
		st.GPU([]store.GPURow{{TS: time.Now(), GPU: 0, ClockMHz: 1395, PowerLimitW: 100}})
		st.Flush()
		g, err := st.GPUSince(context.Background(), time.Now().Add(-time.Minute), 10)
		if err != nil || len(g) != i+1 || g[i].ClockMHz != 1395 {
			t.Fatalf("gpu clock column: %v %+v", err, g)
		}
		st.Close()
	}
}

// The metadata file carries no text column at all (the sentinel's schema half).
func TestMetadataSchemaHasNoTextColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gw.db")
	st := openJ(t, path, "b")
	db := rawDB(t, path)
	_ = st
	rows, err := db.Query(`SELECT m.name, p.name FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tbl, col string
		rows.Scan(&tbl, &col)
		for _, bad := range []string{"prompt", "response", "message", "content", "system", "input", "answer"} {
			if col == bad || strings.HasPrefix(col, bad+"_text") {
				t.Errorf("text-like column %s.%s in the metadata database", tbl, col)
			}
		}
	}
}
