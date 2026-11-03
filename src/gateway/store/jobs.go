package store

// The durable job log (Fase 10.L).
//
// Why it exists: the record of a request used to be written when it ENDED, so
// the job that was running when the host lost power left no trace. Now each job
// is written three times, and every write is synchronous (synchronous=FULL on
// the jdb pool, never through the batch writer that may lose its last batch):
//
//	JobEnqueued  at the entry of the line   status na_fila
//	JobStarted   when the slot is granted   status em_execucao, started_at, backend
//	Finish       when it ends               ok / erro / cancelado / ...
//
// A job that never reaches Finish belongs to a process that died. On the next
// Open it becomes interrompido_reboot (the boot_id changed: the host rebooted)
// or interrompido_gateway (same boot, the gateway process died), and one event
// reports how many.
//
// The same pool writes the per-job GPU extract (job_gpu_samples), once a second
// while a job runs, so the last second before a power cut is on disk.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// JobEnqueued writes the entry of a job (status na_fila) durably. It never
// overwrites a row that already exists: a fast job, or a removal by the
// administrator, may have written a later state first.
func (s *Store) JobEnqueued(r queue.Record) error {
	args := s.recordArgs(r)
	_, err := s.jdb.Exec(`INSERT INTO requests(id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,
		enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id,think,responded,resp_bytes,backend,failover_rule,station_computer,station_user,station_domain,station_mac,doc_hash,doc_key_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''))
		ON CONFLICT(id) DO NOTHING`, args...)
	return err
}

// JobStarted marks a job as em_execucao, durably. It only moves a job that is
// still na_fila, so it can never undo an end that was written first.
func (s *Store) JobStarted(id string, at time.Time, backend string) error {
	_, err := s.jdb.Exec(`UPDATE requests SET status=?, started_at=?, wait_ms=?-enqueued_at, backend=NULLIF(?,'') WHERE id=? AND status=?`,
		queue.StatusRunning, at.UnixMilli(), at.UnixMilli(), backend, id, queue.StatusQueued)
	return err
}

// Finish writes the end of a job durably (the same UPSERT as Record, on the
// synchronous pool).
func (s *Store) Finish(r queue.Record) error {
	_, err := s.jdb.Exec(upsertRecordSQL, s.recordArgs(r)...)
	return err
}

// SyncModes returns PRAGMA synchronous of the batch pool and of the journal
// pool (0 OFF, 1 NORMAL, 2 FULL). The journal one must be 2.
func (s *Store) SyncModes() (batch, journal int, err error) {
	if err = s.db.QueryRow(`PRAGMA synchronous`).Scan(&batch); err != nil {
		return
	}
	err = s.jdb.QueryRow(`PRAGMA synchronous`).Scan(&journal)
	return
}

// SetResponded records that an interrupted job had already received part of
// its answer (known only from the content database).
func (s *Store) SetResponded(id string, bytes int64) error {
	_, err := s.jdb.Exec(`UPDATE requests SET responded=1, resp_bytes=? WHERE id=?`, bytes, id)
	return err
}

// ---- recovery ----------------------------------------------------------------

// RecoveredJob is a job that was still in flight when the previous process
// died.
type RecoveredJob struct {
	ID     string
	Status string // StatusInterruptedReboot or StatusInterruptedRestart
	// WasRunning is true when the slot had been granted (em_execucao).
	WasRunning bool
}

// Recovered returns the jobs Open closed as interrupted.
func (s *Store) Recovered() []RecoveredJob { return append([]RecoveredJob(nil), s.recovered...) }

// recoverInterrupted turns every job left in na_fila / em_execucao into an
// interrupted one. At Open this process has an empty queue, so nothing in
// flight can be alive. finished_at is the last second the per-job extract has
// for the job (the best evidence of when it died), or its start / entry.
func (s *Store) recoverInterrupted(now time.Time) {
	rows, err := s.jdb.Query(`SELECT id, status, COALESCE(boot_id,''), enqueued_at, started_at,
		COALESCE((SELECT MAX(ts) FROM job_gpu_samples WHERE job_id=requests.id),0) FROM requests WHERE status IN (?,?)`,
		queue.StatusQueued, queue.StatusRunning)
	if err != nil {
		s.logf("store: recuperar jobs interrompidos: %v", err)
		return
	}
	type row struct {
		id, status, boot         string
		enq, started, lastSample int64
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.status, &r.boot, &r.enq, &r.started, &r.lastSample); err != nil {
			s.logf("store: recuperar jobs interrompidos: %v", err)
			rows.Close()
			return
		}
		all = append(all, r)
	}
	rows.Close()
	if len(all) == 0 {
		return
	}
	var reboot, restart int
	var ids []string
	for _, r := range all {
		status := queue.StatusInterruptedRestart
		if r.boot != s.opts.BootID {
			status = queue.StatusInterruptedReboot
			reboot++
		} else {
			restart++
		}
		end := r.lastSample
		if end == 0 {
			end = r.started
		}
		if end == 0 {
			end = r.enq
		}
		if end < r.enq {
			end = r.enq
		}
		var wait, run int64
		if r.started > 0 {
			wait, run = r.started-r.enq, end-r.started
			if run < 0 {
				run = 0
			}
		} else {
			wait = end - r.enq
		}
		_, err := s.jdb.Exec(`UPDATE requests SET status=?, error_class=?, finished_at=?, wait_ms=?, run_ms=?, http_status=0 WHERE id=? AND status IN (?,?)`,
			status, status, end, wait, run, r.id, queue.StatusQueued, queue.StatusRunning)
		if err != nil {
			s.logf("store: recuperar job %s: %v", r.id, err)
			continue
		}
		s.recovered = append(s.recovered, RecoveredJob{ID: r.id, Status: status, WasRunning: r.status == queue.StatusRunning})
		if len(ids) < 20 {
			ids = append(ids, r.id[:min(8, len(r.id))])
		}
	}
	detail := fmt.Sprintf("reboot=%d reinicio_gateway=%d total=%d ids=%s", reboot, restart, len(s.recovered), strings.Join(ids, ","))
	_, _ = s.jdb.Exec(`INSERT INTO events(ts,kind,detail) VALUES(?,?,?)`, now.UnixMilli(), "jobs_interrompidos", detail)
	s.logf("store: %s", detail)
}

// ---- per-job GPU extract -----------------------------------------------------

// JobSample is one GPU (or the CPU, GPU == -1) at one second of one job.
type JobSample struct {
	TS          time.Time `json:"ts"`
	JobID       string    `json:"job_id"`
	GPU         int       `json:"gpu"` // -1 is the CPU line
	PowerW      float64   `json:"power_w"`
	PowerLimitW float64   `json:"power_limit_w"`
	ClockMHz    float64   `json:"clock_mhz"`
	UtilPct     int       `json:"util_pct"`
	MemUsedMiB  int       `json:"mem_used_mib"`
	TempC       int       `json:"temp_c"`
}

// CPUGPUIndex is the gpu value of the CPU line of the extract.
const CPUGPUIndex = -1

// AddJobSamples writes a round of the extract in one durable transaction.
func (s *Store) AddJobSamples(rows []JobSample) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.jdb.Begin()
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.Exec(`INSERT INTO job_gpu_samples(ts,job_id,gpu,power_w,power_limit_w,clock_mhz,util_pct,mem_used_mib,temp_c) VALUES(?,?,?,?,?,?,?,?,?)`,
			r.TS.UnixMilli(), r.JobID, r.GPU, r.PowerW, r.PowerLimitW, r.ClockMHz, r.UtilPct, r.MemUsedMiB, r.TempC); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// JobSamplesOf returns the extract of one job, oldest first.
func (s *Store) JobSamplesOf(ctx context.Context, id string, limit int) ([]JobSample, error) {
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ts,job_id,gpu,power_w,COALESCE(power_limit_w,-1),COALESCE(clock_mhz,-1),util_pct,mem_used_mib,temp_c FROM job_gpu_samples WHERE job_id=? ORDER BY ts, gpu LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobSample{}
	for rows.Next() {
		var r JobSample
		var ts int64
		if err := rows.Scan(&ts, &r.JobID, &r.GPU, &r.PowerW, &r.PowerLimitW, &r.ClockMHz, &r.UtilPct, &r.MemUsedMiB, &r.TempC); err != nil {
			return nil, err
		}
		r.TS = time.UnixMilli(ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- reads for the monitor ---------------------------------------------------

// JobRow is a job as the log shows it: the request row plus what its GPU
// extract says.
type JobRow struct {
	Row
	PeakPowerW   float64 `json:"peak_power_w"`   // highest single-GPU draw during the job; 0 without samples
	PeakClockMHz float64 `json:"peak_clock_mhz"` // highest graphics clock; 0 without samples
	Samples      int     `json:"samples"`        // seconds of extract kept
}

// JobQuery filters the job log. Zero fields do not filter.
type JobQuery struct {
	Since, Until time.Time
	Status       []string // any of
	// FinishedSince, when set, keeps only the jobs that ENDED at or after it
	// (the in-flight ones are left out).
	FinishedSince time.Time
	Client        string
	Limit         int // default 200, max 1000
	Offset        int
}

// Jobs returns the jobs newest first, all states included (na_fila and
// em_execucao too: the log is live).
func (s *Store) Jobs(ctx context.Context, q JobQuery) ([]JobRow, error) {
	if q.Limit <= 0 {
		q.Limit = 200
	}
	if q.Limit > 1000 {
		q.Limit = 1000
	}
	where, args := "1=1", []any{}
	if !q.Since.IsZero() {
		where += " AND enqueued_at >= ?"
		args = append(args, q.Since.UnixMilli())
	}
	if !q.Until.IsZero() {
		where += " AND enqueued_at <= ?"
		args = append(args, q.Until.UnixMilli())
	}
	if !q.FinishedSince.IsZero() {
		where += " AND finished_at >= ? AND " + inFlightSQL
		args = append(args, q.FinishedSince.UnixMilli())
	}
	if len(q.Status) > 0 {
		where += " AND status IN (?" + strings.Repeat(",?", len(q.Status)-1) + ")"
		for _, st := range q.Status {
			args = append(args, st)
		}
	}
	if q.Client != "" {
		where += " AND client_label = ?"
		args = append(args, q.Client)
	}
	args = append(args, q.Limit, q.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT `+rowCols+` FROM requests WHERE `+where+` ORDER BY enqueued_at DESC, id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	out := []JobRow{}
	idx := map[string]int{}
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		idx[r.ID] = len(out)
		out = append(out, JobRow{Row: r})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(out) == 0 {
		return out, nil
	}
	ph := "?" + strings.Repeat(",?", len(out)-1)
	ids := make([]any, 0, len(out))
	for _, j := range out {
		ids = append(ids, j.ID)
	}
	pr, err := s.db.QueryContext(ctx, `SELECT job_id, MAX(power_w), MAX(clock_mhz), COUNT(DISTINCT ts) FROM job_gpu_samples WHERE gpu >= 0 AND job_id IN (`+ph+`) GROUP BY job_id`, ids...)
	if err != nil {
		return nil, err
	}
	defer pr.Close()
	for pr.Next() {
		var id string
		var pw, ck sql.NullFloat64
		var n int
		if err := pr.Scan(&id, &pw, &ck, &n); err != nil {
			return nil, err
		}
		if i, ok := idx[id]; ok {
			out[i].PeakPowerW, out[i].PeakClockMHz, out[i].Samples = pw.Float64, ck.Float64, n
		}
	}
	return out, pr.Err()
}

// Job returns one job (any state) with its peaks; ok is false when it does not
// exist.
func (s *Store) Job(ctx context.Context, id string) (JobRow, bool, error) {
	var found []JobRow
	rows, err := s.db.QueryContext(ctx, `SELECT `+rowCols+` FROM requests WHERE id = ?`, id)
	if err != nil {
		return JobRow{}, false, err
	}
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			rows.Close()
			return JobRow{}, false, err
		}
		found = append(found, JobRow{Row: r})
	}
	rows.Close()
	if len(found) == 0 {
		return JobRow{}, false, nil
	}
	j := found[0]
	var pw, ck sql.NullFloat64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(power_w), MAX(clock_mhz), COUNT(DISTINCT ts) FROM job_gpu_samples WHERE gpu >= 0 AND job_id = ?`, id).Scan(&pw, &ck, &j.Samples); err != nil {
		return JobRow{}, false, err
	}
	j.PeakPowerW, j.PeakClockMHz = pw.Float64, ck.Float64
	return j, true, nil
}

// JobDoc returns the document HMAC and the id of the key that made it, as
// stored for a job (Fase 12, C-4.2, C-4.3a); both "" when none was recorded.
// The value is never put in the monitor's JSON. Use the caller's own context,
// not a monitor request context held under a shared mutex.
func (s *Store) JobDoc(ctx context.Context, id string) (docHash, keyID string, err error) {
	var h, k sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT doc_hash, doc_key_id FROM requests WHERE id=?`, id).Scan(&h, &k)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return h.String, k.String, err
}
