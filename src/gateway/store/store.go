// Package store keeps the gateway history in SQLite (Fase 9.3): metadata only,
// never a prompt, a document, an answer or a token (plan section 5). Since
// Fase 10.L the text of the jobs lives in a SEPARATE database (package content,
// owner decision of 2026-10-30); this file stays free of any text.
//
// Fase 10.L also makes the job log durable (jobs.go): the entry, the start and
// the end of a job are written synchronously (synchronous=FULL) on a second
// connection pool, outside the batch writer below, so a power cut cannot take
// the last transitions with it.
//
// The database is never on the hot path: the queue lives in memory and the
// producers (Record, GPU, Event) only push into a buffered channel. One
// goroutine writes the items in batches inside a transaction. When the buffer
// is full the item is dropped and counted; the count is later written as an
// event. Audit entries are rare and written synchronously.
//
// The driver is modernc.org/sqlite (pure Go, no cgo), decision D9-5.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

const schema = `
CREATE TABLE IF NOT EXISTS requests(
  id TEXT PRIMARY KEY, client_ip TEXT, client_label TEXT, tool TEXT, endpoint TEXT, model TEXT,
  lane TEXT, priority_in TEXT, priority_final TEXT, bytes_in INTEGER,
  enqueued_at INTEGER, started_at INTEGER, finished_at INTEGER,
  wait_ms INTEGER, run_ms INTEGER,
  status TEXT, http_status INTEGER, error_class TEXT,
  prompt_eval_count INTEGER, eval_count INTEGER, load_ms INTEGER, boot_id TEXT, think TEXT,
  responded INTEGER, resp_bytes INTEGER, backend TEXT, failover_rule TEXT,
  station_computer TEXT, station_user TEXT, station_domain TEXT, station_mac TEXT,
  doc_hash TEXT, doc_key_id TEXT);
CREATE TABLE IF NOT EXISTS audit(
  id INTEGER PRIMARY KEY, ts INTEGER, usuario TEXT, ip TEXT, acao TEXT, request_id TEXT,
  antes TEXT, depois TEXT, motivo TEXT);
CREATE TABLE IF NOT EXISTS gpu_samples(
  ts INTEGER, gpu INTEGER, bus_id TEXT, power_w REAL, mem_used_mib INTEGER,
  util_pct INTEGER, temp_c INTEGER, aer_corr_1b4 INTEGER, boot_id TEXT, power_limit_w REAL, clock_mhz REAL);
CREATE TABLE IF NOT EXISTS job_gpu_samples(
  ts INTEGER, job_id TEXT, gpu INTEGER, power_w REAL, power_limit_w REAL, clock_mhz REAL,
  util_pct INTEGER, mem_used_mib INTEGER, temp_c INTEGER);
CREATE TABLE IF NOT EXISTS cpu_samples(
  ts INTEGER, util_pct REAL, load1 REAL, load5 REAL, load15 REAL,
  ram_total_mib INTEGER, ram_used_mib INTEGER, ram_avail_mib INTEGER,
  swap_used_mib INTEGER, swap_total_mib INTEGER, temp_c REAL, freq_mhz REAL,
  ollama_rss_mib INTEGER, power_w REAL, boot_id TEXT, threads_json TEXT);
CREATE TABLE IF NOT EXISTS backend_samples(
  ts INTEGER, backend_id TEXT, estado TEXT, slots_usados INTEGER, fila INTEGER, p50_ms INTEGER);
CREATE TABLE IF NOT EXISTS events(ts INTEGER, kind TEXT, detail TEXT);
CREATE TABLE IF NOT EXISTS meta(k TEXT PRIMARY KEY, v TEXT);
CREATE INDEX IF NOT EXISTS idx_requests_enq ON requests(enqueued_at);
CREATE INDEX IF NOT EXISTS idx_requests_client ON requests(client_label, enqueued_at);
CREATE INDEX IF NOT EXISTS idx_requests_fin ON requests(finished_at);
CREATE INDEX IF NOT EXISTS idx_gpu_ts ON gpu_samples(ts);
CREATE INDEX IF NOT EXISTS idx_jgs_job ON job_gpu_samples(job_id, ts);
CREATE INDEX IF NOT EXISTS idx_jgs_ts ON job_gpu_samples(ts);
CREATE INDEX IF NOT EXISTS idx_requests_status ON requests(status);
CREATE INDEX IF NOT EXISTS idx_cpu_ts ON cpu_samples(ts);
CREATE INDEX IF NOT EXISTS idx_backend_ts ON backend_samples(ts);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit(ts);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
`

// Options configures Open.
type Options struct {
	BootID      string                           // stamped on requests and GPU rows
	BufferSize  int                              // channel capacity, default 4096
	BatchSize   int                              // items per transaction, default 200
	FlushEvery  time.Duration                    // default 250 ms
	Logf        func(format string, args ...any) // optional
	ReqKeep     time.Duration                    // requests retention, default 90 days
	GPUKeep     time.Duration                    // GPU samples retention, default 7 days
	JobGPUKeep  time.Duration                    // per-job GPU extract retention, default 7 days (Fase 10.L)
	VacuumEvery time.Duration                    // default 7 days
	// MaxRequests caps the requests table (Fase 9.5, S2): the retention is by
	// age only, and a client in a loop at the rate the nginx allows writes
	// ~864 thousand rows a day. Maintain deletes the oldest rows above the cap
	// at every call (hourly). Default DefaultMaxRequests.
	MaxRequests int64
}

// DefaultMaxRequests is the default cap of the requests table (~1 GB with the
// indexes; ~33 thousand requests a day for the 90 days of retention).
const DefaultMaxRequests = 3_000_000

// dropEventEvery is the shortest gap between two "registros_descartados"
// events: under a sustained overload every batch would write one, and the
// events table never expires (Fase 9.5, S3).
const dropEventEvery = time.Minute

// Store is the SQLite history. Safe for concurrent use.
type Store struct {
	db   *sql.DB
	opts Options

	// jdb is a second pool on the same file with synchronous=FULL: the job log
	// (jobs.go) and the per-job GPU extract never go through the batch writer.
	jdb       *sql.DB
	recovered []RecoveredJob

	ch      chan item
	dropped atomic.Int64
	written atomic.Int64
	wg      sync.WaitGroup
	closed  atomic.Bool
	closeMu sync.RWMutex

	lastDropEvent time.Time // writer goroutine only
}

type itemKind int

const (
	kRecord itemKind = iota
	kGPU
	kCPU
	kBackend
	kEvent
	kBarrier
)

type item struct {
	kind     itemKind
	rec      queue.Record
	gpu      []GPURow
	cpu      CPURow
	backends []BackendRow
	ev       Event
	done     chan struct{}
}

// GPURow is one GPU sample as stored.
type GPURow struct {
	TS          time.Time
	GPU         int
	BusID       string
	PowerW      float64
	MemUsedMiB  int
	UtilPct     int
	TempC       int
	AER         int64   // correctable AER counter of the bridge; -1 when unknown
	PowerLimitW float64 // power cap in force; -1 when unknown
	ClockMHz    float64 // current graphics clock (clocks.gr); -1 when unknown
}

// CPURow is one CPU/RAM sample as stored. Unknown numbers are -1. Threads is
// the compact per-thread detail (see EncodeThreads), "" when there is none.
type CPURow struct {
	TS           time.Time
	UtilPct      float64
	Load1        float64
	Load5        float64
	Load15       float64
	RAMTotalMiB  int64
	RAMUsedMiB   int64
	RAMAvailMiB  int64
	SwapUsedMiB  int64
	SwapTotalMiB int64
	TempC        float64
	FreqMHz      float64
	OllamaRSSMiB int64
	PowerW       float64
	Threads      string // threads_json
}

// Event is a row of the events table.
type Event struct {
	TS     time.Time
	Kind   string
	Detail string
}

// Open creates (0600) or opens the database, applies the schema and starts
// the writer. A new boot id (compared with the last one seen) is recorded as
// an event.
func Open(path string, o Options) (*Store, error) {
	if o.BufferSize <= 0 {
		o.BufferSize = 4096
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 200
	}
	if o.FlushEvery <= 0 {
		o.FlushEvery = 250 * time.Millisecond
	}
	if o.ReqKeep <= 0 {
		o.ReqKeep = 90 * 24 * time.Hour
	}
	if o.GPUKeep <= 0 {
		o.GPUKeep = 7 * 24 * time.Hour
	}
	if o.JobGPUKeep <= 0 {
		o.JobGPUKeep = 7 * 24 * time.Hour
	}
	if o.VacuumEvery <= 0 {
		o.VacuumEvery = 7 * 24 * time.Hour
	}
	if o.MaxRequests <= 0 {
		o.MaxRequests = DefaultMaxRequests
	}
	// Create the file with 0600 first: SQLite gives the -wal and -shm files the
	// same permission bits as the main file.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: cannot create %s: %w", path, err)
	}
	f.Close()
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	jdb, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		db.Close()
		return nil, err
	}
	jdb.SetMaxOpenConns(2)
	s := &Store{db: db, jdb: jdb, opts: o, ch: make(chan item, o.BufferSize)}
	s.noteBoot()
	s.recoverInterrupted(time.Now())
	s.wg.Add(1)
	go s.writer()
	return s, nil
}

// migrate brings a database created by an older version up to the current
// schema. Each step checks before it changes, so running it twice is harmless.
//   - think (Fase 9.6a): reasoning mode of the request.
//   - gpu_samples.power_limit_w (power cap in the GPU cards).
//   - cpu_samples (CPU/RAM board): new table, created by the schema; threads_json column.
func migrate(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(requests)`)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !have["think"] {
		if _, err := db.Exec(`ALTER TABLE requests ADD COLUMN think TEXT`); err != nil {
			return err
		}
	}
	// gpu_samples.power_limit_w (power cap board): old rows keep NULL, read as -1.
	gpuCols, err := tableColumns(db, "gpu_samples")
	if err != nil {
		return err
	}
	if len(gpuCols) > 0 && !gpuCols["power_limit_w"] {
		if _, err := db.Exec(`ALTER TABLE gpu_samples ADD COLUMN power_limit_w REAL`); err != nil {
			return err
		}
	}
	// Fase 10.L: the job-log columns of requests, and the clock of gpu_samples.
	// Old rows keep NULL (read as 0 / "").
	for _, c := range []struct{ name, ddl string }{
		{"responded", `ALTER TABLE requests ADD COLUMN responded INTEGER`},
		{"resp_bytes", `ALTER TABLE requests ADD COLUMN resp_bytes INTEGER`},
		{"backend", `ALTER TABLE requests ADD COLUMN backend TEXT`},
		// Fase 10.4: the rule that sent a job away from the primary. Idempotent: the
		// column is added only when missing, so a second start changes nothing.
		{"failover_rule", `ALTER TABLE requests ADD COLUMN failover_rule TEXT`},
		// Station declared by the workstation (X-Mcpollama-Station), same idempotent rule.
		{"station_computer", `ALTER TABLE requests ADD COLUMN station_computer TEXT`},
		{"station_user", `ALTER TABLE requests ADD COLUMN station_user TEXT`},
		{"station_domain", `ALTER TABLE requests ADD COLUMN station_domain TEXT`},
		{"station_mac", `ALTER TABLE requests ADD COLUMN station_mac TEXT`},
		// Fase 12 (contract C-4.2): HMAC of X-Mcpollama-Doc and the id of the key; same idempotent rule.
		{"doc_hash", `ALTER TABLE requests ADD COLUMN doc_hash TEXT`},
		{"doc_key_id", `ALTER TABLE requests ADD COLUMN doc_key_id TEXT`},
	} {
		if !have[c.name] {
			if _, err := db.Exec(c.ddl); err != nil {
				return err
			}
		}
	}
	if len(gpuCols) > 0 && !gpuCols["clock_mhz"] {
		if _, err := db.Exec(`ALTER TABLE gpu_samples ADD COLUMN clock_mhz REAL`); err != nil {
			return err
		}
	}
	// cpu_samples itself comes from the schema (CREATE TABLE IF NOT EXISTS);
	// only a table made by a build without the per-thread detail lacks the
	// column.
	cpuCols, err := tableColumns(db, "cpu_samples")
	if err != nil {
		return err
	}
	if len(cpuCols) > 0 && !cpuCols["threads_json"] {
		if _, err := db.Exec(`ALTER TABLE cpu_samples ADD COLUMN threads_json TEXT`); err != nil {
			return err
		}
	}
	return nil
}

// tableColumns returns the column names of a table (empty when it does not exist).
func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		have[name] = true
	}
	return have, rows.Err()
}

func (s *Store) logf(format string, a ...any) {
	if s.opts.Logf != nil {
		s.opts.Logf(format, a...)
	}
}

// noteBoot records the start of the gateway and, when the boot id changed
// since the last run, a "boot_novo" event (the machine rebooted).
func (s *Store) noteBoot() {
	now := time.Now()
	var last string
	_ = s.db.QueryRow(`SELECT v FROM meta WHERE k='boot_id'`).Scan(&last)
	if s.opts.BootID != "" && last != s.opts.BootID {
		detail := "boot_id=" + s.opts.BootID
		if last != "" {
			detail += " anterior=" + last
		}
		s.exec(`INSERT INTO events(ts,kind,detail) VALUES(?,?,?)`, now.UnixMilli(), "boot_novo", detail)
		s.exec(`INSERT INTO meta(k,v) VALUES('boot_id',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, s.opts.BootID)
	}
	s.exec(`INSERT INTO events(ts,kind,detail) VALUES(?,?,?)`, now.UnixMilli(), "gateway_inicio", "boot_id="+s.opts.BootID)
}

func (s *Store) exec(q string, args ...any) {
	if _, err := s.db.Exec(q, args...); err != nil {
		s.logf("store: %v", err)
	}
}

// Dropped is the number of items lost because the buffer was full or a batch
// failed.
func (s *Store) Dropped() int64 { return s.dropped.Load() }

// Written is the number of items written.
func (s *Store) Written() int64 { return s.written.Load() }

func (s *Store) push(it item) {
	s.closeMu.RLock()
	defer s.closeMu.RUnlock()
	if s.closed.Load() {
		return
	}
	select {
	case s.ch <- it:
	default:
		s.dropped.Add(1)
	}
}

// Record queues the metadata of a finished request. Never blocks.
func (s *Store) Record(r queue.Record) { s.push(item{kind: kRecord, rec: r}) }

// GPU queues one round of GPU samples. Never blocks.
func (s *Store) GPU(rows []GPURow) {
	if len(rows) > 0 {
		s.push(item{kind: kGPU, gpu: rows})
	}
}

// CPU queues one CPU/RAM sample. Never blocks.
func (s *Store) CPU(r CPURow) { s.push(item{kind: kCPU, cpu: r}) }

// Event queues an event row. Never blocks.
func (s *Store) Event(kind, detail string) {
	s.push(item{kind: kEvent, ev: Event{TS: time.Now(), Kind: kind, Detail: detail}})
}

// Flush blocks until everything queued before the call is on disk (tests and
// shutdown).
func (s *Store) Flush() {
	done := make(chan struct{})
	s.closeMu.RLock()
	if s.closed.Load() {
		s.closeMu.RUnlock()
		return
	}
	s.ch <- item{kind: kBarrier, done: done} // may block: a barrier must not be dropped
	s.closeMu.RUnlock()
	<-done
}

// Close flushes, stops the writer and closes the database.
func (s *Store) Close() error {
	s.closeMu.Lock()
	if s.closed.Swap(true) {
		s.closeMu.Unlock()
		return nil
	}
	close(s.ch)
	s.closeMu.Unlock()
	s.wg.Wait()
	_ = s.jdb.Close()
	return s.db.Close()
}

func (s *Store) writer() {
	defer s.wg.Done()
	tick := time.NewTicker(s.opts.FlushEvery)
	defer tick.Stop()
	batch := make([]item, 0, s.opts.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		n := s.writeBatch(batch)
		if n < 0 {
			s.dropped.Add(int64(len(batch)))
		}
		for _, it := range batch {
			if it.kind == kBarrier {
				close(it.done)
			}
		}
		batch = batch[:0]
	}
	for {
		select {
		case it, ok := <-s.ch:
			if !ok {
				flush()
				if s.dropped.Load() > 0 { // the last count must not be lost at shutdown
					s.lastDropEvent = time.Time{}
					s.writeBatch(nil)
				}
				return
			}
			batch = append(batch, it)
			if it.kind == kBarrier || len(batch) >= s.opts.BatchSize {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

// writeBatch writes the items in one transaction; -1 means the batch failed.
func (s *Store) writeBatch(batch []item) int {
	tx, err := s.db.Begin()
	if err != nil {
		s.logf("store: begin: %v", err)
		return -1
	}
	n := 0
	for _, it := range batch {
		var err error
		switch it.kind {
		case kRecord:
			err = s.insertRecord(tx, it.rec)
		case kGPU:
			for _, g := range it.gpu {
				_, err = tx.Exec(`INSERT INTO gpu_samples(ts,gpu,bus_id,power_w,mem_used_mib,util_pct,temp_c,aer_corr_1b4,boot_id,power_limit_w,clock_mhz) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
					g.TS.UnixMilli(), g.GPU, g.BusID, g.PowerW, g.MemUsedMiB, g.UtilPct, g.TempC, g.AER, s.opts.BootID, g.PowerLimitW, g.ClockMHz)
				if err != nil {
					break
				}
			}
		case kCPU:
			c := it.cpu
			_, err = tx.Exec(`INSERT INTO cpu_samples(ts,util_pct,load1,load5,load15,ram_total_mib,ram_used_mib,ram_avail_mib,swap_used_mib,swap_total_mib,temp_c,freq_mhz,ollama_rss_mib,power_w,boot_id,threads_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				c.TS.UnixMilli(), c.UtilPct, c.Load1, c.Load5, c.Load15, c.RAMTotalMiB, c.RAMUsedMiB, c.RAMAvailMiB,
				c.SwapUsedMiB, c.SwapTotalMiB, c.TempC, c.FreqMHz, c.OllamaRSSMiB, c.PowerW, s.opts.BootID, c.Threads)
		case kBackend:
			for _, b := range it.backends {
				_, err = tx.Exec(`INSERT INTO backend_samples(ts,backend_id,estado,slots_usados,fila,p50_ms) VALUES(?,?,?,?,?,?)`,
					b.TS.UnixMilli(), b.BackendID, b.State, b.SlotsUsed, b.Queue, b.P50MS)
				if err != nil {
					break
				}
			}
		case kEvent:
			_, err = tx.Exec(`INSERT INTO events(ts,kind,detail) VALUES(?,?,?)`, it.ev.TS.UnixMilli(), it.ev.Kind, it.ev.Detail)
		}
		if err != nil {
			s.logf("store: insert: %v", err)
			_ = tx.Rollback()
			return -1
		}
		if it.kind != kBarrier {
			n++
		}
	}
	// Report items lost earlier in the same transaction, at most once a
	// minute (the count accumulates in between).
	if s.dropped.Load() > 0 && time.Since(s.lastDropEvent) >= dropEventEvery {
		if d := s.dropped.Swap(0); d > 0 {
			_, _ = tx.Exec(`INSERT INTO events(ts,kind,detail) VALUES(?,?,?)`, time.Now().UnixMilli(), "registros_descartados", strconv.FormatInt(d, 10))
			s.lastDropEvent = time.Now()
		}
	}
	if err := tx.Commit(); err != nil {
		s.logf("store: commit: %v", err)
		return -1
	}
	s.written.Add(int64(n))
	return n
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// upsertRecordSQL writes the end of a request. The row may already exist: the
// job log (jobs.go) wrote it as na_fila / em_execucao, so this is an UPSERT that
// keeps the rowid (the row cap relies on it) and the backend written at the
// start. INSERT OR REPLACE would drop both.
const upsertRecordSQL = `INSERT INTO requests(id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,
		enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id,think,responded,resp_bytes,backend,failover_rule,station_computer,station_user,station_domain,station_mac,doc_hash,doc_key_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''))
		ON CONFLICT(id) DO UPDATE SET station_computer=COALESCE(excluded.station_computer, requests.station_computer),
		station_user=COALESCE(excluded.station_user, requests.station_user),
		station_domain=COALESCE(excluded.station_domain, requests.station_domain),
		station_mac=COALESCE(excluded.station_mac, requests.station_mac),
		doc_hash=COALESCE(excluded.doc_hash, requests.doc_hash), doc_key_id=COALESCE(excluded.doc_key_id, requests.doc_key_id), client_ip=excluded.client_ip, client_label=excluded.client_label, tool=excluded.tool,
		endpoint=excluded.endpoint, model=excluded.model, lane=excluded.lane, priority_in=excluded.priority_in,
		priority_final=excluded.priority_final, bytes_in=excluded.bytes_in, enqueued_at=excluded.enqueued_at,
		started_at=excluded.started_at, finished_at=excluded.finished_at, wait_ms=excluded.wait_ms, run_ms=excluded.run_ms,
		status=excluded.status, http_status=excluded.http_status, error_class=excluded.error_class,
		prompt_eval_count=excluded.prompt_eval_count, eval_count=excluded.eval_count, load_ms=excluded.load_ms,
		boot_id=excluded.boot_id, think=excluded.think, responded=excluded.responded, resp_bytes=excluded.resp_bytes,
		backend=COALESCE(excluded.backend, requests.backend),
		failover_rule=COALESCE(excluded.failover_rule, requests.failover_rule)`

func (s *Store) recordArgs(r queue.Record) []any {
	return []any{r.ID, r.ClientIP, r.ClientLabel, r.Tool, r.Endpoint, r.Model, string(r.Lane), r.PriorityIn.String(), r.PriorityFinal.String(), r.BytesIn,
		ms(r.EnqueuedAt), ms(r.StartedAt), ms(r.FinishedAt), r.WaitMS, r.RunMS, r.Status, r.HTTPStatus, r.ErrorClass,
		r.PromptEval, r.Eval, r.LoadMS, s.opts.BootID, r.Think, boolInt(r.Responded), r.RespBytes, r.Backend, r.FailoverRule,
		r.StationComputer, r.StationUser, r.StationDomain, r.StationMAC, r.DocHash, r.DocKeyID}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) insertRecord(tx *sql.Tx, r queue.Record) error {
	_, err := tx.Exec(upsertRecordSQL, s.recordArgs(r)...)
	return err
}

// ---- audit -------------------------------------------------------------------

// AuditEntry is one administrator action.
type AuditEntry struct {
	TS        time.Time `json:"ts"`
	User      string    `json:"usuario"`
	IP        string    `json:"ip"`
	Action    string    `json:"acao"`
	RequestID string    `json:"request_id"`
	Before    string    `json:"antes"`
	After     string    `json:"depois"`
	Reason    string    `json:"motivo"`
}

// AddAudit writes an audit entry synchronously (actions are rare, and the
// trail must not be lost to a full buffer).
func (s *Store) AddAudit(e AuditEntry) error {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	_, err := s.db.Exec(`INSERT INTO audit(ts,usuario,ip,acao,request_id,antes,depois,motivo) VALUES(?,?,?,?,?,?,?,?)`,
		e.TS.UnixMilli(), e.User, e.IP, e.Action, e.RequestID, e.Before, e.After, e.Reason)
	return err
}

// Audits returns the latest entries, newest first.
func (s *Store) Audits(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ts,usuario,ip,acao,request_id,antes,depois,motivo FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var ts int64
		if err := rows.Scan(&ts, &e.User, &e.IP, &e.Action, &e.RequestID, &e.Before, &e.After, &e.Reason); err != nil {
			return nil, err
		}
		e.TS = time.UnixMilli(ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- reads ---------------------------------------------------------------------

// Row is a request as the history shows it.
type Row struct {
	ID            string `json:"id"`
	ClientIP      string `json:"client_ip"`
	ClientLabel   string `json:"client_label"`
	Tool          string `json:"tool"`
	Endpoint      string `json:"endpoint"`
	Model         string `json:"model"`
	Lane          string `json:"lane"`
	PriorityIn    string `json:"priority_in"`
	PriorityFinal string `json:"priority_final"`
	BytesIn       int64  `json:"bytes_in"`
	EnqueuedAt    int64  `json:"enqueued_at"`
	StartedAt     int64  `json:"started_at"`
	FinishedAt    int64  `json:"finished_at"`
	WaitMS        int64  `json:"wait_ms"`
	RunMS         int64  `json:"run_ms"`
	Status        string `json:"status"`
	HTTPStatus    int    `json:"http_status"`
	ErrorClass    string `json:"error_class"`
	PromptEval    int64  `json:"prompt_eval_count"`
	Eval          int64  `json:"eval_count"`
	LoadMS        int64  `json:"load_ms"`
	BootID        string `json:"boot_id"`
	Think         string `json:"think"` // on, off, modelo or "" (rows before 9.6a)
	// Fase 10.L (job log). Responded: the model returned an answer; RespBytes:
	// bytes of generated text received; Backend: where it ran ("" if it never
	// started).
	Responded bool   `json:"responded"`
	RespBytes int64  `json:"resp_bytes"`
	Backend   string `json:"backend"`
	// Fase 10.4: the rule that gave the job to a secondary ("" when it ran on the primary).
	FailoverRule string `json:"failover_rule"`
	// Declared by the workstation (X-Mcpollama-Station); not authenticated.
	StationComputer string `json:"station_computer"`
	StationUser     string `json:"station_user"`
	StationDomain   string `json:"station_domain"`
	StationMAC      string `json:"station_mac"`
}

// HistoryQuery filters the history. Zero fields do not filter.
type HistoryQuery struct {
	Since, Until time.Time
	Client       string // client_label
	Model        string
	Limit        int // default 100, max 1000
	Offset       int
}

const rowCols = `id,client_ip,client_label,tool,endpoint,model,lane,priority_in,priority_final,bytes_in,enqueued_at,started_at,finished_at,wait_ms,run_ms,status,http_status,error_class,prompt_eval_count,eval_count,load_ms,boot_id,think,COALESCE(responded,0),COALESCE(resp_bytes,0),COALESCE(backend,''),COALESCE(failover_rule,''),COALESCE(station_computer,''),COALESCE(station_user,''),COALESCE(station_domain,''),COALESCE(station_mac,'')`

// inFlightSQL leaves out the jobs that have not ended (Fase 10.L): History is
// the list of what finished; Jobs (jobs.go) lists everything.
const inFlightSQL = `status NOT IN ('na_fila','em_execucao')`

func scanRow(sc interface{ Scan(...any) error }) (Row, error) {
	var r Row
	var bootID, ec, think sql.NullString
	var responded int
	err := sc.Scan(&r.ID, &r.ClientIP, &r.ClientLabel, &r.Tool, &r.Endpoint, &r.Model, &r.Lane, &r.PriorityIn, &r.PriorityFinal, &r.BytesIn,
		&r.EnqueuedAt, &r.StartedAt, &r.FinishedAt, &r.WaitMS, &r.RunMS, &r.Status, &r.HTTPStatus, &ec, &r.PromptEval, &r.Eval, &r.LoadMS, &bootID, &think, &responded, &r.RespBytes, &r.Backend, &r.FailoverRule,
		&r.StationComputer, &r.StationUser, &r.StationDomain, &r.StationMAC)
	r.ErrorClass, r.BootID, r.Think, r.Responded = ec.String, bootID.String, think.String, responded != 0
	return r, err
}

// History returns requests newest first.
func (s *Store) History(ctx context.Context, q HistoryQuery) ([]Row, error) {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 1000 {
		q.Limit = 1000
	}
	where, args := inFlightSQL, []any{}
	if !q.Since.IsZero() {
		where += " AND enqueued_at >= ?"
		args = append(args, q.Since.UnixMilli())
	}
	if !q.Until.IsZero() {
		where += " AND enqueued_at <= ?"
		args = append(args, q.Until.UnixMilli())
	}
	if q.Client != "" {
		where += " AND client_label = ?"
		args = append(args, q.Client)
	}
	if q.Model != "" {
		where += " AND model = ?"
		args = append(args, q.Model)
	}
	args = append(args, q.Limit, q.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT `+rowCols+` FROM requests WHERE `+where+` ORDER BY enqueued_at DESC, id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Stats summarises a window.
type Stats struct {
	Window       string         `json:"window"`
	Total        int            `json:"total"`
	OK           int            `json:"ok"`
	Errors       int            `json:"errors"`
	ByStatus     map[string]int `json:"by_status"`
	ByClass      map[string]int `json:"by_class"`
	WaitP50      int64          `json:"wait_p50_ms"`
	WaitP95      int64          `json:"wait_p95_ms"`
	RunP50       int64          `json:"run_p50_ms"`
	RunP95       int64          `json:"run_p95_ms"`
	PerMinute    float64        `json:"per_minute"`
	PromptTokens int64          `json:"prompt_tokens"`
	EvalTokens   int64          `json:"eval_tokens"`
	TokensPerSec float64        `json:"tokens_per_sec"`
	// Truncated is true when the window had more than maxStatRows requests:
	// the numbers then cover only the newest maxStatRows (Fase 9.5, S1).
	Truncated bool `json:"truncated"`
}

// maxStatRows bounds the rows one Stats call reads (a var for the tests).
var maxStatRows = 200000

// Stats computes the statistics of the requests that finished in the window.
func (s *Store) Stats(ctx context.Context, window time.Duration, label string) (Stats, error) {
	st := Stats{Window: label, ByStatus: map[string]int{}, ByClass: map[string]int{}}
	since := time.Now().Add(-window).UnixMilli()
	rows, err := s.db.QueryContext(ctx, `SELECT status,error_class,started_at,wait_ms,run_ms,prompt_eval_count,eval_count FROM requests WHERE finished_at >= ? ORDER BY finished_at DESC LIMIT ?`, since, maxStatRows)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	var waits, runs []int64
	for rows.Next() {
		var status string
		var class sql.NullString
		var started, w, r, pe, e int64
		if err := rows.Scan(&status, &class, &started, &w, &r, &pe, &e); err != nil {
			return st, err
		}
		st.Total++
		st.ByStatus[status]++
		if status == queue.StatusOK {
			st.OK++
			runs = append(runs, r)
		} else if status == queue.StatusError {
			st.Errors++
		}
		if class.String != "" && status != queue.StatusOK {
			st.ByClass[class.String]++
		}
		if started > 0 { // only requests that reached a slot have a meaningful queue wait
			waits = append(waits, w)
		}
		st.PromptTokens += pe
		st.EvalTokens += e
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	st.Truncated = st.Total >= maxStatRows
	st.WaitP50, st.WaitP95 = Percentile(waits, 50), Percentile(waits, 95)
	st.RunP50, st.RunP95 = Percentile(runs, 50), Percentile(runs, 95)
	secs := window.Seconds()
	if secs > 0 {
		st.PerMinute = float64(st.Total) / (secs / 60)
		st.TokensPerSec = float64(st.EvalTokens) / secs
	}
	// rows.Err() was already checked right after the loop. A second call here
	// raced the context's awaitDone goroutine (incident of 2026-11-01).
	return st, nil
}

// Percentile returns the p-th percentile (nearest rank) of v; 0 for empty.
func Percentile(v []int64, p int) int64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]int64(nil), v...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	idx := (p*len(c) + 99) / 100
	if idx < 1 {
		idx = 1
	}
	if idx > len(c) {
		idx = len(c)
	}
	return c[idx-1]
}

// GPUSince returns GPU samples newer than since, oldest first.
func (s *Store) GPUSince(ctx context.Context, since time.Time, limit int) ([]GPURow, error) {
	if limit <= 0 || limit > 100000 {
		limit = 20000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ts,gpu,bus_id,power_w,mem_used_mib,util_pct,temp_c,aer_corr_1b4,COALESCE(power_limit_w,-1),COALESCE(clock_mhz,-1) FROM gpu_samples WHERE ts >= ? ORDER BY ts LIMIT ?`, since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GPURow{}
	for rows.Next() {
		var g GPURow
		var ts int64
		if err := rows.Scan(&ts, &g.GPU, &g.BusID, &g.PowerW, &g.MemUsedMiB, &g.UtilPct, &g.TempC, &g.AER, &g.PowerLimitW, &g.ClockMHz); err != nil {
			return nil, err
		}
		g.TS = time.UnixMilli(ts)
		out = append(out, g)
	}
	return out, rows.Err()
}

// CPUSince returns CPU/RAM samples newer than since, oldest first.
func (s *Store) CPUSince(ctx context.Context, since time.Time, limit int) ([]CPURow, error) {
	if limit <= 0 || limit > 100000 {
		limit = 20000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ts,util_pct,load1,load5,load15,ram_total_mib,ram_used_mib,ram_avail_mib,swap_used_mib,swap_total_mib,temp_c,freq_mhz,ollama_rss_mib,power_w,COALESCE(threads_json,'') FROM cpu_samples WHERE ts >= ? ORDER BY ts LIMIT ?`, since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CPURow{}
	for rows.Next() {
		var c CPURow
		var ts int64
		if err := rows.Scan(&ts, &c.UtilPct, &c.Load1, &c.Load5, &c.Load15, &c.RAMTotalMiB, &c.RAMUsedMiB, &c.RAMAvailMiB,
			&c.SwapUsedMiB, &c.SwapTotalMiB, &c.TempC, &c.FreqMHz, &c.OllamaRSSMiB, &c.PowerW, &c.Threads); err != nil {
			return nil, err
		}
		c.TS = time.UnixMilli(ts)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Events returns the latest events, newest first.
func (s *Store) Events(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ts,kind,detail FROM events ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&ts, &e.Kind, &e.Detail); err != nil {
			return nil, err
		}
		e.TS = time.UnixMilli(ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- maintenance -------------------------------------------------------------

// Prune deletes requests, GPU samples and CPU samples (same retention as the
// GPU) older than the retention; gpus counts the GPU rows. audit and events
// never expire.
func (s *Store) Prune(ctx context.Context, now time.Time) (reqs, gpus int64, err error) {
	r, err := s.db.ExecContext(ctx, `DELETE FROM requests WHERE enqueued_at < ?`, now.Add(-s.opts.ReqKeep).UnixMilli())
	if err != nil {
		return 0, 0, err
	}
	reqs, _ = r.RowsAffected()
	g, err := s.db.ExecContext(ctx, `DELETE FROM gpu_samples WHERE ts < ?`, now.Add(-s.opts.GPUKeep).UnixMilli())
	if err != nil {
		return reqs, 0, err
	}
	gpus, _ = g.RowsAffected()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM cpu_samples WHERE ts < ?`, now.Add(-s.opts.GPUKeep).UnixMilli()); err != nil {
		return reqs, gpus, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM backend_samples WHERE ts < ?`, now.Add(-s.opts.GPUKeep).UnixMilli()); err != nil {
		return reqs, gpus, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM job_gpu_samples WHERE ts < ?`, now.Add(-s.opts.JobGPUKeep).UnixMilli()); err != nil {
		return reqs, gpus, err
	}
	return reqs, gpus, nil
}

// Cap deletes the oldest requests above opts.MaxRequests. The rowid grows with
// every insert (INSERT OR REPLACE included), so the range delete is cheap and
// keeps at most MaxRequests rows.
func (s *Store) Cap(ctx context.Context) (int64, error) {
	r, err := s.db.ExecContext(ctx, `DELETE FROM requests WHERE rowid <= (SELECT MAX(rowid) FROM requests) - ?`, s.opts.MaxRequests)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	return n, nil
}

// Vacuum compacts the database.
func (s *Store) Vacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `VACUUM`)
	return err
}

func (s *Store) metaTime(k string) time.Time {
	var v string
	if s.db.QueryRow(`SELECT v FROM meta WHERE k=?`, k).Scan(&v) != nil {
		return time.Time{}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(n)
}

func (s *Store) setMetaTime(k string, t time.Time) {
	s.exec(`INSERT INTO meta(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, strconv.FormatInt(t.UnixMilli(), 10))
}

// Maintain runs the daily cleanup and the weekly VACUUM when they are due.
// Call it from a ticker (hourly is enough); the due dates survive restarts.
func (s *Store) Maintain(ctx context.Context, now time.Time) error {
	var errs []error
	if n, err := s.Cap(ctx); err != nil {
		errs = append(errs, err)
	} else if n > 0 {
		s.logf("store: teto de %d pedidos removeu os %d mais antigos", s.opts.MaxRequests, n)
		s.exec(`INSERT INTO events(ts,kind,detail) VALUES(?,?,?)`, now.UnixMilli(), "teto_pedidos", strconv.FormatInt(n, 10))
	}
	if now.Sub(s.metaTime("last_prune")) >= 24*time.Hour {
		reqs, gpus, err := s.Prune(ctx, now)
		if err != nil {
			errs = append(errs, err)
		} else {
			s.setMetaTime("last_prune", now)
			s.logf("store: retenção removeu %d pedidos e %d amostras de GPU", reqs, gpus)
		}
	}
	if now.Sub(s.metaTime("last_vacuum")) >= s.opts.VacuumEvery {
		if err := s.Vacuum(ctx); err != nil {
			errs = append(errs, err)
		} else {
			s.setMetaTime("last_vacuum", now)
		}
	}
	return errors.Join(errs...)
}

// RunMaintenance calls Maintain every interval until ctx ends.
func (s *Store) RunMaintenance(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := s.Maintain(ctx, time.Now()); err != nil {
			s.logf("store: manutenção: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// BootID reads the kernel boot id; "unknown" when it cannot be read (not Linux).
func BootID(path string) string {
	if path == "" {
		path = "/proc/sys/kernel/random/boot_id"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// threadsDetail is the shape of CPURow.Threads: short keys because there is
// one row every few seconds. t = [id, util_pct, core_id] per logical CPU and
// c = [core_id, temp_c] per physical core; -1 is "unknown".
type threadsDetail struct {
	T [][3]float64 `json:"t"`
	C [][2]float64 `json:"c"`
}

// EncodeThreads builds the threads_json text from the per-thread values.
func EncodeThreads(threads [][3]float64, cores [][2]float64) string {
	if len(threads) == 0 && len(cores) == 0 {
		return ""
	}
	b, err := json.Marshal(threadsDetail{T: threads, C: cores})
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeThreads is the inverse of EncodeThreads; ok is false on malformed text.
func DecodeThreads(txt string) (threads [][3]float64, cores [][2]float64, ok bool) {
	if txt == "" {
		return nil, nil, true
	}
	var d threadsDetail
	if err := json.Unmarshal([]byte(txt), &d); err != nil {
		return nil, nil, false
	}
	return d.T, d.C, true
}
