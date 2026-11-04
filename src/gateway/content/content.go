// Package content keeps the full text of each job (Fase 10.L): the prompt that
// was sent and the answer that came back, in a SQLite file of its own.
//
// This is an owner decision (2026-10-30) that replaces the old rule "no text in
// the database": after the host rebooted twice under load, the job that was
// running had left no trace. The text lives ONLY here, never in the metadata
// database (gateway.db), so that file can still be shared and inspected freely.
//
// Protections:
//   - the file is created 0600 (the -wal and -shm inherit it);
//   - a total cap (MaxBytes): above it the oldest jobs are deleted and an event
//     is reported;
//   - a cap per part (MaxJobBytes, the flat prompt, the prompt JSON and the answer each): above it the
//     text is cut and the row is marked truncated;
//   - a retention (Keep, 7 days by default);
//   - the monitor serves the text only to the logged-in administrator.
//
// Durability: every commit is synchronous=FULL, so the prompt and the answer
// received so far (flushed every FlushEvery while it streams) survive a power
// cut.
package content

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Defaults of Options.
const (
	DefaultMaxBytes    = 4 << 30  // 4 GiB, all jobs together (contract C-2.8; was 2 GiB before Fase 12)
	DefaultMaxJobBytes = 16 << 20 // 16 MiB, the flat prompt, the prompt JSON and the answer each (C-2.3, C-2.4)
	DefaultKeep        = 7 * 24 * time.Hour
	DefaultFlushEvery  = 2 * time.Second
)

const schema = `
CREATE TABLE IF NOT EXISTS job_content(
  job_id TEXT PRIMARY KEY, created_at INTEGER, updated_at INTEGER,
  prompt BLOB, prompt_bytes INTEGER, prompt_sha256 TEXT, prompt_truncated INTEGER,
  response BLOB, resp_bytes INTEGER, resp_truncated INTEGER, resp_final INTEGER,
  prompt_json BLOB, prompt_json_truncated INTEGER NOT NULL DEFAULT 0, tem_imagem INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS idx_content_created ON job_content(created_at);
`

// migrate adds the columns of Fase 12 (contract C-2.7) to a database made by an
// older version: prompt_json (the request body as the Ollama received it, image
// markers in place), its own truncation mark and tem_imagem. Each step checks
// PRAGMA table_info first, so a second start changes nothing and no row is lost.
// Old rows keep prompt_json NULL: "no JSON was kept for this job".
func migrate(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(job_content)`)
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
	for _, c := range []struct{ name, ddl string }{
		{"prompt_json", `ALTER TABLE job_content ADD COLUMN prompt_json BLOB`},
		{"prompt_json_truncated", `ALTER TABLE job_content ADD COLUMN prompt_json_truncated INTEGER NOT NULL DEFAULT 0`},
		{"tem_imagem", `ALTER TABLE job_content ADD COLUMN tem_imagem INTEGER NOT NULL DEFAULT 0`},
		// stage 12.8 (C-2.10a): the request before the improvement and the trace of the improvement
		{"prompt_json_original", `ALTER TABLE job_content ADD COLUMN prompt_json_original BLOB`},
		{"prompt_json_original_truncated", `ALTER TABLE job_content ADD COLUMN prompt_json_original_truncated INTEGER NOT NULL DEFAULT 0`},
		{"melhora_instrucoes_id", `ALTER TABLE job_content ADD COLUMN melhora_instrucoes_id TEXT`},
		{"melhora_indice_id", `ALTER TABLE job_content ADD COLUMN melhora_indice_id TEXT`},
		{"melhora_exemplos", `ALTER TABLE job_content ADD COLUMN melhora_exemplos TEXT`},
	} {
		if !have[c.name] {
			if _, err := db.Exec(c.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// Options configures Open. Zero values take the defaults.
type Options struct {
	MaxBytes    int64         // total stored bytes (prompts + answers), default DefaultMaxBytes
	MaxJobBytes int64         // per part, default DefaultMaxJobBytes
	Keep        time.Duration // retention, default DefaultKeep
	FlushEvery  time.Duration // partial answer flush period, default DefaultFlushEvery
	Logf        func(format string, args ...any)
	// OnEvent reports the rare events (a cap that deleted jobs). May be nil.
	OnEvent func(kind, detail string)
}

// Store is the content database. Safe for concurrent use.
type Store struct {
	db   *sql.DB
	opts Options

	total  atomic.Int64 // stored bytes, kept in step with the writes
	capMu  sync.Mutex   // one cap enforcement at a time
	closed atomic.Bool

	// The cap runs in the background (revisão 10.L, D4): bgMu guards capBusy
	// and the closing, capWG lets Close wait for a run in progress.
	bgMu       sync.Mutex
	capBusy    bool
	capRetryAt time.Time     // after a failed trigger, no new run before this (D6)
	capEventAt time.Time     // last teto_conteudo_falhou event
	stop       chan struct{} // closed by Close: cuts the backoff between runs
	capWG      sync.WaitGroup
}

// Open creates (0600) or opens the content database.
func Open(path string, o Options) (*Store, error) {
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.MaxJobBytes <= 0 {
		o.MaxJobBytes = DefaultMaxJobBytes
	}
	if o.Keep <= 0 {
		o.Keep = DefaultKeep
	}
	if o.FlushEvery <= 0 {
		o.FlushEvery = DefaultFlushEvery
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("content: cannot create %s: %w", path, err)
	}
	f.Close()
	// A file that already existed keeps its mode on O_CREATE: force 0600 (the
	// -wal and -shm copy the mode of the main file; revisão 10.L, P1).
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("content: cannot chmod %s: %w", path, err)
	}
	// auto_vacuum first: it only takes effect before the first table exists.
	db, err := sql.Open("sqlite", path+"?_pragma=auto_vacuum(2)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("content: schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("content: migração: %w", err)
	}
	s := &Store{db: db, opts: o, stop: make(chan struct{})}
	var sum int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(COALESCE(length(prompt),0)+COALESCE(length(prompt_json),0)+COALESCE(length(prompt_json_original),0)+COALESCE(length(response),0)),0) FROM job_content`).Scan(&sum); err != nil {
		db.Close()
		return nil, fmt.Errorf("content: %w", err)
	}
	s.total.Store(sum)
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	s.bgMu.Lock()
	already := s.closed.Swap(true)
	if !already {
		close(s.stop)
	}
	s.bgMu.Unlock()
	if already {
		return nil
	}
	s.capWG.Wait() // a cap run in progress ends before the database closes
	return s.db.Close()
}

// WaitCap waits for a background cap run in progress (tests and diagnostics).
func (s *Store) WaitCap() { s.capWG.Wait() }

func (s *Store) logf(format string, a ...any) {
	if s.opts.Logf != nil {
		s.opts.Logf(format, a...)
	}
}

// Total is the number of stored bytes (prompts + answers) the cap looks at.
func (s *Store) Total() int64 { return s.total.Load() }

// Limits returns the effective caps (tests and diagnostics).
func (s *Store) Limits() (maxBytes, maxJobBytes int64) { return s.opts.MaxBytes, s.opts.MaxJobBytes }

// SyncMode returns PRAGMA synchronous of a connection of the pool (2 = FULL).
func (s *Store) SyncMode() (int, error) {
	var v int
	err := s.db.QueryRow(`PRAGMA synchronous`).Scan(&v)
	return v, err
}

// cutUTF8 cuts b to at most n bytes without splitting a rune.
func cutUTF8(b []byte, n int) []byte {
	if n >= len(b) {
		return b
	}
	if n <= 0 {
		return nil
	}
	for n > 0 && !utf8.RuneStart(b[n]) {
		n--
	}
	return b[:n]
}

// cutUTF8String is cutUTF8 for a string (no copy).
func cutUTF8String(s string, n int) string {
	if n >= len(s) {
		return s
	}
	if n <= 0 {
		return ""
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// SavePrompt stores the flat text of the prompt of a job and nothing else (no
// prompt_json). It is idempotent (the first one wins), synchronous and durable.
// It returns the stored size and whether it was cut. The proxy uses
// SavePromptJSON; this form stays for callers that only have the text.
func (s *Store) SavePrompt(id, prompt string, now time.Time) (stored int, truncated bool, err error) {
	r, err := s.SavePromptJSON(id, prompt, nil, false, now)
	return r.Stored, r.Truncated, err
}

// PromptSaved is what SavePromptJSON reports.
type PromptSaved struct {
	Stored        int  // bytes of the flat text kept
	Truncated     bool // the flat text was cut at the per-job cap (prompt_truncated)
	JSONStored    int  // bytes of prompt_json kept
	JSONTruncated bool // prompt_json was cut at its own per-job cap (prompt_json_truncated)
}

// SavePromptJSON stores, in ONE statement, the flat text of the prompt and the
// request body (pj) as the Ollama received it, with the images already replaced
// by their markers (contract C-2.1, C-2.2, C-2.5). hasImage becomes tem_imagem.
// The two parts have separate caps and separate truncation marks (C-2.3): cutting
// one never marks the other. pj nil keeps the column NULL. It is idempotent
// (the first one wins), synchronous and durable.
func (s *Store) SavePromptJSON(id, prompt string, pj []byte, hasImage bool, now time.Time) (PromptSaved, error) {
	return s.savePrompt(id, prompt, pj, hasImage, false, nil, now)
}

// MelhoraTrace is what the improvement without training leaves in a job (C-2.10a): the request
// BEFORE the improvement (images already replaced by markers; its own cap and truncation mark)
// and the ids of what was added. prompt_json stays the body the Ollama received (C-2.10).
type MelhoraTrace struct {
	Original     []byte
	InstrucoesID string
	IndiceID     string
	Exemplos     []string // occurrence ids
}

// SavePromptJSONMelhora is SavePromptJSON for a request the improvement changed.
func (s *Store) SavePromptJSONMelhora(id, prompt string, pj []byte, hasImage bool, m MelhoraTrace, now time.Time) (PromptSaved, error) {
	return s.savePrompt(id, prompt, pj, hasImage, false, &m, now)
}

// SavePromptJSONFailed stores the flat text of a job whose body could not be
// scanned for images (fail closed): prompt_json stays NULL, tem_imagem=1 and
// prompt_json_truncated=1, so the job can never be exported.
func (s *Store) SavePromptJSONFailed(id, prompt string, now time.Time) (PromptSaved, error) {
	return s.savePrompt(id, prompt, nil, true, true, nil, now)
}

func (s *Store) savePrompt(id, prompt string, pj []byte, hasImage, failed bool, m *MelhoraTrace, now time.Time) (PromptSaved, error) {
	var res PromptSaved
	// Hash the string as it is and copy only what is stored: a 32 MiB prompt
	// must not be copied whole once more (revisão 10.L, M1).
	hs := sha256.New()
	chunk := make([]byte, 64<<10) // io.WriteString would copy the whole string
	for i := 0; i < len(prompt); i += len(chunk) {
		n := copy(chunk, prompt[i:])
		hs.Write(chunk[:n])
	}
	var sum [sha256.Size]byte
	hs.Sum(sum[:0])
	keep := prompt
	if int64(len(keep)) > s.opts.MaxJobBytes {
		keep, res.Truncated = cutUTF8String(keep, int(s.opts.MaxJobBytes)), true
	}
	b := []byte(keep)
	trunc := 0
	if res.Truncated {
		trunc = 1
	}
	var jb any // NULL when there is no JSON
	jtrunc, img := 0, 0
	if pj != nil {
		kj := pj
		if int64(len(kj)) > s.opts.MaxJobBytes {
			kj, res.JSONTruncated, jtrunc = cutUTF8(kj, int(s.opts.MaxJobBytes)), true, 1
		}
		jb, res.JSONStored = kj, len(kj)
	}
	if hasImage {
		img = 1
	}
	if failed {
		jtrunc, res.JSONTruncated = 1, true
	}
	var ob any // the request before the improvement: NULL when the improvement changed nothing
	otrunc, origStored := 0, 0
	var instrID, idxID, exs any
	if m != nil {
		if m.Original != nil {
			ko := m.Original
			if int64(len(ko)) > s.opts.MaxJobBytes {
				ko, otrunc = cutUTF8(ko, int(s.opts.MaxJobBytes)), 1
			}
			ob, origStored = ko, len(ko)
		}
		instrID, idxID, exs = m.InstrucoesID, m.IndiceID, strings.Join(m.Exemplos, ",")
	}
	r, err := s.db.Exec(`INSERT OR IGNORE INTO job_content(job_id,created_at,updated_at,prompt,prompt_bytes,prompt_sha256,prompt_truncated,response,resp_bytes,resp_truncated,resp_final,prompt_json,prompt_json_truncated,tem_imagem,prompt_json_original,prompt_json_original_truncated,melhora_instrucoes_id,melhora_indice_id,melhora_exemplos)
		VALUES(?,?,?,?,?,?,?,x'',0,0,0,?,?,?,?,?,?,?,?)`,
		id, now.UnixMilli(), now.UnixMilli(), b, len(prompt), hex.EncodeToString(sum[:]), trunc, jb, jtrunc, img, ob, otrunc, instrID, idxID, exs)
	if err != nil {
		return PromptSaved{Truncated: res.Truncated, JSONTruncated: res.JSONTruncated}, err
	}
	res.Stored = len(b)
	if n, _ := r.RowsAffected(); n > 0 {
		s.added(int64(len(b) + res.JSONStored + origStored))
	}
	return res, nil
}

// added accounts for stored bytes and starts the total cap when it is passed.
func (s *Store) added(n int64) {
	if s.total.Add(n) > s.opts.MaxBytes {
		s.triggerCap()
	}
}

// triggerCap runs enforceCap in the background, one run at a time. Whoever
// passed the cap is a prompt being accepted or an answer streaming with the GPU
// slot held: it must not pay for deleting thousands of old jobs (revisão 10.L,
// D4: 614 deletes took 829 ms inside the write that passed the cap).
func (s *Store) triggerCap() {
	s.bgMu.Lock()
	if s.closed.Load() || s.capBusy || time.Now().Before(s.capRetryAt) {
		s.bgMu.Unlock()
		return
	}
	s.capBusy = true
	s.capWG.Add(1)
	s.bgMu.Unlock()
	go func() {
		defer s.capWG.Done()
		for attempt := 1; ; attempt++ {
			removed, err := s.enforceCap()
			s.bgMu.Lock()
			over := s.total.Load() > s.opts.MaxBytes
			// A write that came in meanwhile may have passed the cap again: run
			// once more, but only while the runs make progress. An error, a run
			// that removed nothing, or too many runs end the goroutine (revisão
			// 10.L, D6: a failing enforceCap used to make this loop spin at 100%
			// of a CPU forever).
			if s.closed.Load() || !over {
				s.capBusy = false
				s.bgMu.Unlock()
				return
			}
			if err != nil || removed == 0 {
				// no progress: stop, cool down, re-read the truth, report
				s.capBusy = false
				s.capRetryAt = time.Now().Add(capCooldown)
				s.bgMu.Unlock()
				s.resyncTotal()
				s.capFailed(err, removed, attempt)
				return
			}
			if attempt >= capMaxAttempts {
				// progress, but writes keep coming: end this goroutine; the
				// next write past the cap starts a new one
				s.capBusy = false
				s.bgMu.Unlock()
				return
			}
			s.bgMu.Unlock()
			select { // a short backoff between runs; Close cuts it
			case <-time.After(time.Duration(attempt) * capBackoff):
			case <-s.stop:
			}
		}
	}()
}

// Limits of the background cap (revisão 10.L, D6).
const (
	capBatch       = 500 // jobs one transaction of the cap deletes (one fsync)
	capMaxAttempts = 5   // runs per trigger
	capBackoff     = 20 * time.Millisecond
	capCooldown    = time.Minute // after a failed trigger, no new one before this
	capEventEvery  = time.Minute // at most one teto_conteudo_falhou per minute
)

// resyncTotal re-reads the real total, so the next write decides again on the
// truth and not on a counter that a failed run left behind.
func (s *Store) resyncTotal() {
	var sum int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(COALESCE(length(prompt),0)+COALESCE(length(prompt_json),0)+COALESCE(length(prompt_json_original),0)+COALESCE(length(response),0)),0) FROM job_content`).Scan(&sum); err == nil {
		s.total.Store(sum)
	}
}

// capFailed logs a cap run that could not bring the total under the cap and
// reports teto_conteudo_falhou, at most once per capEventEvery.
func (s *Store) capFailed(err error, removed int64, attempt int) {
	why := fmt.Sprintf("tentativas=%d removidos_na_ultima=%d total=%d teto=%d", attempt, removed, s.total.Load(), s.opts.MaxBytes)
	if err != nil {
		why += " erro=" + err.Error()
	}
	s.logf("content: o teto não conseguiu baixar o total (%s); nova tentativa em %s", why, capCooldown)
	s.bgMu.Lock()
	send := time.Since(s.capEventAt) >= capEventEvery
	if send {
		s.capEventAt = time.Now()
	}
	s.bgMu.Unlock()
	if send && s.opts.OnEvent != nil {
		s.opts.OnEvent("teto_conteudo_falhou", why)
	}
}

// enforceCap deletes the oldest jobs until the total is under 90% of the cap
// (a margin, so the next write does not trigger it again) and reports an event.
// The deletes go in transactions of capBatch: one synchronous commit each,
// not one per job.
func (s *Store) enforceCap() (removed int64, err error) {
	s.capMu.Lock()
	defer s.capMu.Unlock()
	if s.total.Load() <= s.opts.MaxBytes {
		return 0, nil
	}
	target := s.opts.MaxBytes / 10 * 9
	var freed int64
	for s.total.Load() > target && !s.closed.Load() {
		rows, err := s.db.Query(`SELECT job_id, COALESCE(length(prompt),0)+COALESCE(length(prompt_json),0)+COALESCE(length(prompt_json_original),0)+COALESCE(length(response),0) FROM job_content ORDER BY created_at, rowid LIMIT ?`, capBatch)
		if err != nil {
			s.logf("content: teto: %v", err)
			return removed, err
		}
		type victim struct {
			id string
			n  int64
		}
		var vs []victim
		for rows.Next() {
			var v victim
			if err := rows.Scan(&v.id, &v.n); err == nil {
				vs = append(vs, v)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return removed, err
		}
		if len(vs) == 0 {
			break
		}
		tx, err := s.db.Begin()
		if err != nil {
			s.logf("content: teto: %v", err)
			return removed, err
		}
		var bn, bf, need int64 = 0, 0, s.total.Load() - target
		for _, v := range vs {
			if bf >= need {
				break
			}
			if _, err := tx.Exec(`DELETE FROM job_content WHERE job_id=?`, v.id); err != nil {
				_ = tx.Rollback()
				s.logf("content: teto: %v", err)
				return removed, err
			}
			bn++
			bf += v.n
		}
		if err := tx.Commit(); err != nil {
			s.logf("content: teto: %v", err)
			return removed, err
		}
		s.total.Add(-bf)
		removed += bn
		freed += bf
	}
	// Re-read the real total: it corrects any drift of the running counter.
	var sum int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(COALESCE(length(prompt),0)+COALESCE(length(prompt_json),0)+COALESCE(length(prompt_json_original),0)+COALESCE(length(response),0)),0) FROM job_content`).Scan(&sum); err == nil {
		s.total.Store(sum)
	}
	s.logf("content: teto de %d bytes removeu os %d jobs mais antigos (%d bytes)", s.opts.MaxBytes, removed, freed)
	if s.opts.OnEvent != nil {
		s.opts.OnEvent("teto_conteudo", fmt.Sprintf("removidos=%d bytes_liberados=%d total_agora=%d teto=%d", removed, freed, s.total.Load(), s.opts.MaxBytes))
	}
	return removed, nil
}

// ---- answer sink -------------------------------------------------------------

// Sink receives the answer of one job while it streams and writes it to the
// database: appended every FlushEvery and at the end. Safe for concurrent use,
// although the proxy feeds it from one goroutine.
type Sink struct {
	s  *Store
	id string

	mu        sync.Mutex
	pend      []byte
	stored    int64 // bytes already in the database
	recv      int64 // bytes received, before the cap
	truncated bool
	last      time.Time
	done      bool
}

// NewSink returns the sink of job id.
func (s *Store) NewSink(id string) *Sink { return &Sink{s: s, id: id, last: time.Now()} }

// Write adds generated text. Once the per-job cap is reached the rest is
// dropped and the row is marked truncated. Every FlushEvery the pending text
// goes to disk (a synchronous commit), so a power cut loses at most that much.
func (k *Sink) Write(text string) {
	if text == "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.done {
		return
	}
	k.recv += int64(len(text))
	k.appendLocked([]byte(text))
	if len(k.pend) > 0 && time.Since(k.last) >= k.s.opts.FlushEvery {
		k.flushLocked(false)
	}
}

func (k *Sink) appendLocked(b []byte) {
	room := k.s.opts.MaxJobBytes - k.stored - int64(len(k.pend))
	if room <= 0 {
		k.truncated = true
		return
	}
	if int64(len(b)) > room {
		b, k.truncated = cutUTF8(b, int(room)), true
	}
	k.pend = append(k.pend, b...)
}

// Finish appends note (the embeddings summary, or "") and writes everything
// durably. responded is true when some text or a note was produced; bytes is
// the generated text received.
func (k *Sink) Finish(note string) (responded bool, bytes int64) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.done {
		return k.recv > 0, k.recv
	}
	k.done = true
	if note != "" {
		k.appendLocked([]byte(note))
	}
	k.flushLocked(true)
	return k.recv > 0 || note != "", k.recv
}

func (k *Sink) flushLocked(final bool) {
	k.last = time.Now()
	if len(k.pend) == 0 && !final {
		return
	}
	trunc, fin := 0, 0
	if k.truncated {
		trunc = 1
	}
	if final {
		fin = 1
	}
	res, err := k.s.db.Exec(`UPDATE job_content SET response=COALESCE(response,x'')||COALESCE(?,x''), resp_bytes=?, resp_truncated=?, resp_final=?, updated_at=? WHERE job_id=?`,
		k.pend, k.recv, trunc, fin, time.Now().UnixMilli(), k.id)
	if err == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			// No prompt row (it failed, or the cap removed it): keep the answer anyway.
			now := time.Now().UnixMilli()
			_, err = k.s.db.Exec(`INSERT OR IGNORE INTO job_content(job_id,created_at,updated_at,prompt,prompt_bytes,prompt_sha256,prompt_truncated,response,resp_bytes,resp_truncated,resp_final)
				VALUES(?,?,?,x'',0,'',0,?,?,?,?)`, k.id, now, now, k.pend, k.recv, trunc, fin)
		}
	}
	if err != nil {
		k.s.logf("content: gravar resposta de %s: %v", k.id, err)
		return // keep pend: the next flush retries
	}
	n := int64(len(k.pend))
	k.stored += n
	k.pend = k.pend[:0]
	if n > 0 {
		k.s.added(n)
	}
}

// ---- reads -------------------------------------------------------------------

// Content is the stored text of a job.
type Content struct {
	JobID           string `json:"job_id"`
	Prompt          string `json:"prompt"`
	Response        string `json:"response"`
	PromptBytes     int64  `json:"prompt_bytes"` // size sent, before the cap
	RespBytes       int64  `json:"resp_bytes"`   // size received, before the cap
	PromptSHA256    string `json:"prompt_sha256"`
	PromptTruncated bool   `json:"prompt_truncated"`
	RespTruncated   bool   `json:"resp_truncated"`
	Truncated       bool   `json:"truncado"`
	// RespFinal is false while the answer is still streaming or when the job was
	// interrupted: what is stored is then the partial answer.
	RespFinal bool  `json:"resp_final"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// Info is Content without the text.
type Info struct {
	JobID        string `json:"job_id"`
	PromptBytes  int64  `json:"prompt_bytes"`
	RespBytes    int64  `json:"resp_bytes"`
	PromptSHA256 string `json:"prompt_sha256"`
	Truncated    bool   `json:"truncado"`
	RespFinal    bool   `json:"resp_final"`
	Stored       int64  `json:"stored_bytes"` // bytes held for this job
}

// Get returns the text of a job; ok is false when there is none.
func (s *Store) Get(ctx context.Context, id string) (c Content, ok bool, err error) {
	var prompt, resp []byte
	var pt, rt, rf int
	err = s.db.QueryRowContext(ctx, `SELECT job_id,prompt,response,prompt_bytes,resp_bytes,prompt_sha256,prompt_truncated,resp_truncated,resp_final,created_at,updated_at FROM job_content WHERE job_id=?`, id).
		Scan(&c.JobID, &prompt, &resp, &c.PromptBytes, &c.RespBytes, &c.PromptSHA256, &pt, &rt, &rf, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return Content{}, false, nil
	}
	if err != nil {
		return Content{}, false, err
	}
	c.Prompt, c.Response = string(prompt), string(resp)
	c.PromptTruncated, c.RespTruncated, c.RespFinal = pt != 0, rt != 0, rf != 0
	c.Truncated = c.PromptTruncated || c.RespTruncated
	return c, true, nil
}

// Info returns the sizes of a job without reading the text.
func (s *Store) Info(ctx context.Context, id string) (in Info, ok bool, err error) {
	var pt, rt, rf int
	err = s.db.QueryRowContext(ctx, `SELECT job_id,prompt_bytes,resp_bytes,prompt_sha256,prompt_truncated,resp_truncated,resp_final,COALESCE(length(prompt),0)+COALESCE(length(prompt_json),0)+COALESCE(length(prompt_json_original),0)+COALESCE(length(response),0) FROM job_content WHERE job_id=?`, id).
		Scan(&in.JobID, &in.PromptBytes, &in.RespBytes, &in.PromptSHA256, &pt, &rt, &rf, &in.Stored)
	if err == sql.ErrNoRows {
		return Info{}, false, nil
	}
	if err != nil {
		return Info{}, false, err
	}
	in.Truncated, in.RespFinal = pt != 0 || rt != 0, rf != 0
	return in, true, nil
}

// Count is the number of jobs held.
func (s *Store) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_content`).Scan(&n)
	return n, err
}

// ---- maintenance ---------------------------------------------------------------

// Maintain deletes the jobs older than the retention, enforces the total cap
// and returns the freed pages to the file system.
func (s *Store) Maintain(ctx context.Context, now time.Time) error {
	r, err := s.db.ExecContext(ctx, `DELETE FROM job_content WHERE created_at < ?`, now.Add(-s.opts.Keep).UnixMilli())
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n > 0 {
		var sum int64
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(COALESCE(length(prompt),0)+COALESCE(length(prompt_json),0)+COALESCE(length(prompt_json_original),0)+COALESCE(length(response),0)),0) FROM job_content`).Scan(&sum); err == nil {
			s.total.Store(sum)
		}
		s.logf("content: retenção de %s removeu %d jobs", s.opts.Keep, n)
		if _, err := s.db.ExecContext(ctx, `PRAGMA incremental_vacuum`); err != nil {
			s.logf("content: incremental_vacuum: %v", err)
		}
	}
	if s.total.Load() > s.opts.MaxBytes {
		if _, err := s.enforceCap(); err != nil {
			s.logf("content: manutenção: teto: %v", err)
		}
	}
	return nil
}

// RunMaintenance calls Maintain every interval until ctx ends.
func (s *Store) RunMaintenance(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := s.Maintain(ctx, time.Now()); err != nil {
			s.logf("content: manutenção: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PromptJSON is the stored request body of a job (Fase 12).
type PromptJSON struct {
	JSON      []byte // nil when no JSON was kept (rows written before Fase 12, or SavePrompt)
	Truncated bool   // prompt_json_truncated
	HasImage  bool   // tem_imagem
	// The trace of the improvement without training (C-2.10a); all empty for a job it did not change.
	Original          []byte // prompt_json_original
	OriginalTruncated bool
	InstrucoesID      string
	IndiceID          string
	Exemplos          string // occurrence ids, comma separated
}

// GetPromptJSON returns the prompt_json of a job; ok is false when the job has
// no row. The context is the caller's: never pass a monitor request context
// under a shared mutex (pattern of 0.12.6).
func (s *Store) GetPromptJSON(ctx context.Context, id string) (p PromptJSON, ok bool, err error) {
	var pj, orig []byte
	var tr, img, otr int
	var ii, xi, ex sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT prompt_json,prompt_json_truncated,tem_imagem,prompt_json_original,prompt_json_original_truncated,melhora_instrucoes_id,melhora_indice_id,melhora_exemplos FROM job_content WHERE job_id=?`, id).
		Scan(&pj, &tr, &img, &orig, &otr, &ii, &xi, &ex)
	if err == sql.ErrNoRows {
		return PromptJSON{}, false, nil
	}
	if err != nil {
		return PromptJSON{}, false, err
	}
	return PromptJSON{JSON: pj, Truncated: tr != 0, HasImage: img != 0, Original: orig, OriginalTruncated: otr != 0,
		InstrucoesID: ii.String, IndiceID: xi.String, Exemplos: ex.String}, true, nil
}

// Probe says what a job holds without reading the text (Fase 12.4, M-1): the
// row exists, a canonical request (prompt_json) is there, and the bytes the
// three fields take. A caller that only needs to decide whether to copy a job
// asks this first and loads the text afterwards.
type Probe struct {
	OK      bool
	HasJSON bool
	Bytes   int64
}

// Probe returns the Probe of a job; OK is false when the job has no row.
func (s *Store) Probe(ctx context.Context, id string) (Probe, error) {
	var jsonLen, total int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(length(prompt_json),0), COALESCE(length(prompt),0)+COALESCE(length(prompt_json),0)+COALESCE(length(prompt_json_original),0)+COALESCE(length(response),0) FROM job_content WHERE job_id=?`, id).
		Scan(&jsonLen, &total)
	if err == sql.ErrNoRows {
		return Probe{}, nil
	}
	if err != nil {
		return Probe{}, err
	}
	return Probe{OK: true, HasJSON: jsonLen > 0, Bytes: total}, nil
}
