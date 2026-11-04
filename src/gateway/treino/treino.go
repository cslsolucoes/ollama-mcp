// Package treino is the training database of Fase 12 (treino.db): the
// occurrences of wrong answers of the local model reported by the workstations,
// the jobs each one involves, the copied samples (canonical request and answer)
// and the history of every change. The rules are in docs/CONTRATO-DADOS-TREINO.md
// (version 1.1.1); the code names them as C-<chapter>.<n>.
//
// What lives here (stage 12.4): the schema, the ingestion of a report in one
// transaction, the state machine and the history. The monitor tab, the admin
// routes, the export and the partition are stage 12.6.
//
// Concurrency (C-6.10, pattern of 0.12.6): nothing here takes a context from a
// monitor visitor, and no lock of the proxy hot path is used. The writers of
// this database take turns through a semaphore of its own, and a writer that does
// not get its turn before the deadline gets ErrBusy (HTTP 503 with Retry-After).
package treino

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
)

// Limits of a report (contract C-1.17) and the ceiling of the file (C-6.2).
const (
	DefaultMaxBytes = 2 << 30   // 2 GiB, the file and its -wal
	MaxBodyBytes    = 256 << 10 // the whole request body
	MaxDescricao    = 4 << 10
	MaxResposta     = 128 << 10
	MaxCorrecao     = 4 << 10 // C-1.17 (1.1.2)
	MaxProjeto      = 128
	MaxJobs         = 64
)

// States (C-1.8) and the roles of a gateway (C-1.21).
const (
	EstadoNova       = "nova"
	EstadoConfirmada = "confirmada"
	EstadoDescartada = "descartada"
	EstadoSemPedido  = "sem_pedido"
	EstadoDistrib    = "distribuida"
	EstadoFallback   = "fallback"
	EstadoApagada    = "apagada"

	RolePrimario = "primario"
	RoleFallback = "fallback"
)

// Actions of the history (C-7.2).
const (
	AcaoCriou        = "criou"
	AcaoAlterou      = "alterou"
	AcaoConfirmou    = "confirmou"
	AcaoDesconfirmou = "desconfirmou"
	AcaoDescartou    = "descartou"
	AcaoApagou       = "apagou"
	AcaoExportou     = "exportou"
)

// Errors of the operations. The HTTP layer maps them to status codes.
var (
	ErrNotFound      = errors.New("treino: ocorrência não encontrada")
	ErrTransition    = errors.New("treino: transição de estado não permitida")
	ErrNeedsAnswer   = errors.New("treino: confirmar exige resposta_correta não vazia")
	ErrInvalid       = errors.New("treino: valor inválido")
	ErrJobNotFound   = errors.New("treino: job_do_erro não existe neste gateway")
	ErrCapReached    = errors.New("treino: teto do treino.db atingido")
	ErrNoKey         = errors.New("treino: gateway sem chave HMAC (--doc-key-file)")
	ErrClosed        = errors.New("treino: base fechada")
	ErrJobBusy       = errors.New("treino: job ainda na fila ou em execução")
	ErrBusy          = errors.New("treino: base ocupada, tente de novo")
	errInvalidSource = errors.New("treino: fonte de conteúdo ausente")
)

const schema = `
CREATE TABLE IF NOT EXISTS ocorrencias(
  id TEXT PRIMARY KEY, criada_em INTEGER NOT NULL, atualizada_em INTEGER NOT NULL,
  estado TEXT NOT NULL CHECK(estado IN ('nova','confirmada','descartada','sem_pedido','distribuida','fallback','apagada')),
  categoria TEXT, severidade TEXT, descricao TEXT, resposta_correta TEXT, correcao_aplicada TEXT,
  origem TEXT, projeto TEXT, tool TEXT,
  estacao_computer TEXT, estacao_user TEXT, estacao_domain TEXT, estacao_mac TEXT,
  client_ip TEXT, client_label TEXT,
  job_do_erro TEXT, doc_hash TEXT, doc_key_id TEXT, grupo_id TEXT,
  particao TEXT NOT NULL DEFAULT '' CHECK(particao IN ('','treino','teste','quarentena')),
  exemplo_publico INTEGER NOT NULL DEFAULT 0,
  revisada_por TEXT, revisada_em INTEGER);
CREATE INDEX IF NOT EXISTS idx_oc_estado ON ocorrencias(estado);
CREATE INDEX IF NOT EXISTS idx_oc_job ON ocorrencias(job_do_erro);
CREATE TABLE IF NOT EXISTS ocorrencia_jobs(
  ocorrencia_id TEXT NOT NULL, job_id TEXT NOT NULL, papel TEXT NOT NULL,
  backend TEXT NOT NULL CHECK(backend IN ('primario','fallback')), ordem INTEGER NOT NULL,
  PRIMARY KEY(ocorrencia_id, job_id));
CREATE INDEX IF NOT EXISTS idx_ocjobs_job ON ocorrencia_jobs(job_id);
CREATE TABLE IF NOT EXISTS amostras(
  job_id TEXT PRIMARY KEY, copiada_em INTEGER NOT NULL, endpoint TEXT, modelo TEXT, think TEXT,
  prompt_json BLOB, prompt_texto BLOB, resposta BLOB,
  prompt_hmac TEXT, conteudo_hmac TEXT, key_id TEXT,
  prompt_truncado INTEGER NOT NULL, prompt_json_truncado INTEGER NOT NULL, tem_imagem INTEGER NOT NULL,
  doc_hash TEXT, resp_final INTEGER NOT NULL, resp_truncada INTEGER NOT NULL,
  status TEXT, http_status INTEGER);
CREATE TABLE IF NOT EXISTS historico(
  id INTEGER PRIMARY KEY AUTOINCREMENT, quando INTEGER NOT NULL, ocorrencia_id TEXT NOT NULL,
  acao TEXT NOT NULL CHECK(acao IN ('criou','alterou','confirmou','desconfirmou','descartou','apagou','exportou')),
  campo TEXT, antes TEXT, depois TEXT, quem TEXT, ip TEXT);
CREATE INDEX IF NOT EXISTS idx_hist_oc ON historico(ocorrencia_id, id);
CREATE TABLE IF NOT EXISTS retratos(
  id TEXT PRIMARY KEY, criado_em INTEGER NOT NULL, indice_hash TEXT, instrucoes_id TEXT, instrucoes_hash TEXT,
  valido INTEGER NOT NULL DEFAULT 1);
CREATE TABLE IF NOT EXISTS retrato_itens(
  retrato_id TEXT NOT NULL, ocorrencia_id TEXT NOT NULL, job_id TEXT NOT NULL, prompt_hmac TEXT, categoria TEXT);
CREATE INDEX IF NOT EXISTS idx_retitens_oc ON retrato_itens(ocorrencia_id);
CREATE TABLE IF NOT EXISTS embeddings(
  ocorrencia_id TEXT PRIMARY KEY, vetor BLOB, modelo TEXT);
-- covering indexes of the recompute and of the summary (review of 12.6, I-2): they answer from
-- the index alone, without reading the rows that carry the blobs of the samples
CREATE INDEX IF NOT EXISTS idx_amostras_grupo ON amostras(job_id, key_id, doc_hash, conteudo_hmac);
CREATE INDEX IF NOT EXISTS idx_oc_estado_part ON ocorrencias(estado, particao, categoria);
CREATE TABLE IF NOT EXISTS cota_grupos(
  grupo_id TEXT PRIMARY KEY, categoria TEXT NOT NULL,
  particao TEXT NOT NULL CHECK(particao IN ('treino','teste')), atribuida_em INTEGER NOT NULL);
-- frozen index of examples (stage 12.8; C-8.5): the header and the members with their vectors, no text
CREATE TABLE IF NOT EXISTS indices(
  id TEXT PRIMARY KEY, criado_em INTEGER NOT NULL, hash TEXT NOT NULL, modelo TEXT NOT NULL,
  k INTEGER NOT NULL, limiar REAL NOT NULL, valido INTEGER NOT NULL DEFAULT 1);
CREATE TABLE IF NOT EXISTS indice_itens(
  indice_id TEXT NOT NULL, ocorrencia_id TEXT NOT NULL, tool TEXT NOT NULL, grupo_id TEXT, doc_hash TEXT, doc_key_id TEXT,
  vetor BLOB NOT NULL, PRIMARY KEY(indice_id, ocorrencia_id));
CREATE INDEX IF NOT EXISTS idx_indice_itens_oc ON indice_itens(ocorrencia_id);
`

// schemaVersion is PRAGMA user_version after migrate. Version 2 (stage 12.8): the
// retrato records k and the similarity floor of the index it used (C-8.10).
const schemaVersion = 3

// Options configures Open. Zero values take the defaults.
type Options struct {
	MaxBytes int64 // ceiling of the live data (C-6.2), default DefaultMaxBytes
	// Key is the HMAC key of C-4.3: the history holds content fields only as
	// hmac=<hex> key_id=<id> bytes=<n> (C-7.4). Without it only bytes=<n> is kept,
	// never a plain hash.
	Key  *hmackey.Key
	Logf func(format string, args ...any) // optional
	// OnEvent reports the rare events (the ceiling reached). May be nil; at most
	// one event a minute for the same kind.
	OnEvent func(kind, detail string)
	Now     func() time.Time // tests
	// TxLockImmediate: see dsn. Default false: the gateway behaves as before.
	TxLockImmediate bool
	// OnIndexInvalidated is told, after the commit, that a frozen index of examples
	// went invalid (C-4.10, C-8.9): the process that serves examples drops it at once.
	// Runs in the writers' turn; it must not call the Store.
	OnIndexInvalidated func(id string)
}

// dsn is the connection string. TxLockImmediate (off for the gateway, which has
// one process and its own writers' turn) makes every transaction take the write
// lock when it begins: a second process that writes to the same file (the bake-off
// creating a retrato while the gateway runs) waits with busy_timeout instead of
// failing later, when a read transaction tries to become a write one.
func dsn(path string, o Options) string {
	d := path + "?_pragma=auto_vacuum(2)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=journal_size_limit(67108864)"
	if o.TxLockImmediate {
		d += "&_txlock=immediate"
	}
	return d
}

// Store is the training database. Safe for concurrent use.
type Store struct {
	db   *sql.DB
	path string
	opts Options

	wsem   chan struct{} // the writers of this database, one at a time; waiting honours the context (M-2)
	evMu   sync.Mutex
	evAt   map[string]time.Time
	closed bool
	// invalidated collects, during a write transaction, the indexes that went invalid; flushed after the commit.
	invalidated []string
}

// Open creates (0600) or opens treino.db, applies the schema and migrates it.
func Open(path string, o Options) (*Store, error) {
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	// The file is created 0600 first: SQLite gives the -wal and -shm the same bits.
	// auto_vacuum=incremental (2) only takes effect on a file that has no table yet
	// (C-6.2a): a treino.db created before this setting stays as it was, with
	// auto_vacuum off, and is not migrated (no production file exists yet). The
	// ceiling still counts live pages only, so it works on such a file too; the
	// space is just not returned to the disk.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("treino: cannot create %s: %w", path, err)
	}
	f.Close()
	db, err := sql.Open("sqlite", dsn(path, o))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("treino: schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("treino: migrate: %w", err)
	}
	// Open every pooled connection now. A connection opened later applies the
	// journal_mode pragma, which fails with SQLITE_BUSY while another connection is
	// writing; with all four idle in the pool none is opened during a write.
	var warm []*sql.Conn
	for i := 0; i < 4; i++ {
		c, err := db.Conn(context.Background())
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("treino: pool: %w", err)
		}
		warm = append(warm, c)
	}
	for _, c := range warm {
		c.Close()
	}
	st := &Store{db: db, path: path, opts: o, evAt: map[string]time.Time{}, wsem: make(chan struct{}, 1)}
	return st, nil
}

// migrate is idempotent: the schema above is version 1 and creates every table
// with IF NOT EXISTS, so a second start changes nothing and loses no row. A
// later version adds its steps here, each guarded by user_version.
func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > schemaVersion {
		return fmt.Errorf("treino.db is version %d, this gateway knows %d", v, schemaVersion)
	}
	// Version 2: the retrato columns for k and the floor (C-8.10). Guarded by the
	// table itself, not only by user_version, so a file half-migrated by a crash, or
	// opened by two processes at once, ends the same way.
	// Version 3 (stage 12.8, review): the trace of the improvement in the sample (C-2.10a, C-3.10a) and the
	// version of the format of the examples in the index (C-8.10a).
	for _, col := range []struct{ table, name, typ string }{
		{"retratos", "exemplos_k", "INTEGER"}, {"retratos", "exemplos_limiar", "REAL"},
		{"amostras", "prompt_json_original", "BLOB"}, {"amostras", "prompt_json_original_truncado", "INTEGER NOT NULL DEFAULT 0"},
		{"amostras", "melhora", "INTEGER NOT NULL DEFAULT 0"}, {"amostras", "melhora_instrucoes_id", "TEXT"},
		{"amostras", "melhora_indice_id", "TEXT"}, {"amostras", "melhora_exemplos", "TEXT"},
		{"indices", "formato", "TEXT NOT NULL DEFAULT ''"},
	} {
		has, err := hasColumn(db, col.table, col.name)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE ` + col.table + ` ADD COLUMN ` + col.name + ` ` + col.typ); err != nil {
			if again, e2 := hasColumn(db, col.table, col.name); e2 != nil || !again { // lost a race with another opener: fine
				return err
			}
		}
	}
	// the covering index of the export predicate, with the column of the improvement (C-3.10a)
	for _, q := range []string{
		`DROP INDEX IF EXISTS idx_amostras_pred`,
		`CREATE INDEX IF NOT EXISTS idx_amostras_pred2 ON amostras(job_id, endpoint, status, http_status, prompt_truncado, prompt_json_truncado, tem_imagem, resp_final, resp_truncada, melhora, (COALESCE(length(prompt_json),0) > 0))`,
	} {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	if v < schemaVersion {
		if _, err := db.Exec(`PRAGMA user_version=` + strconv.Itoa(schemaVersion)); err != nil {
			return err
		}
	}
	return nil
}

func hasColumn(db *sql.DB, table, col string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`) // table is a constant of this package
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close closes the database.
func (s *Store) Close() error {
	s.wsem <- struct{}{}
	defer func() { <-s.wsem }()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

func (s *Store) logf(format string, a ...any) {
	if s.opts.Logf != nil {
		s.opts.Logf(format, a...)
	}
}

// Size is the live data of the database, the number the ceiling of C-6.2 is
// compared with: (page_count - freelist_count) * page_size. Free pages and the
// -wal are not counted. When the pragmas cannot be read it falls back to the
// size of the file on disk.
func (s *Store) Size() int64 {
	var pc, fc, ps int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pc); err == nil {
		if err = s.db.QueryRow(`PRAGMA freelist_count`).Scan(&fc); err == nil {
			if err = s.db.QueryRow(`PRAGMA page_size`).Scan(&ps); err == nil {
				return (pc - fc) * ps
			}
		}
	}
	if fi, err := os.Stat(s.path); err == nil {
		return fi.Size()
	}
	return 0
}

// MaxBytes is the ceiling in force.
func (s *Store) MaxBytes() int64 { return s.opts.MaxBytes }

// event reports a rare event, at most once a minute per kind.
func (s *Store) event(kind, detail string) {
	if s.opts.OnEvent == nil {
		return
	}
	now := s.opts.Now()
	s.evMu.Lock()
	last := s.evAt[kind]
	ok := now.Sub(last) >= time.Minute
	if ok {
		s.evAt[kind] = now
	}
	s.evMu.Unlock()
	if ok {
		s.opts.OnEvent(kind, detail)
	}
}

// Actor is who does a change and from where (C-7.5): the declared station of
// a report, or "admin" for an action in the monitor.
type Actor struct {
	Who string
	IP  string
}

// AdminActor is the actor of an action in the monitor.
func AdminActor(ip string) Actor { return Actor{Who: "admin", IP: ip} }

// execer is the part of *sql.Tx and *sql.DB the helpers need.
type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// hashSize is the only form in which a content field enters the history
// (C-7.4): "hmac=<hex> key_id=<id> bytes=<n>" (HMAC-SHA256 of the text with the
// key of C-4.3), "" for an empty text. Without a key only "bytes=<n>" is kept:
// never the text and never a plain sha256.
func hashSize(key *hmackey.Key, text string) string {
	if text == "" {
		return ""
	}
	n := strconv.Itoa(len(text))
	if !key.Configured() {
		return "bytes=" + n
	}
	return "hmac=" + key.MACString(text) + " key_id=" + key.ID() + " bytes=" + n
}

// hist writes one history row (C-7.3: always inside the transaction of the change).
func hist(ctx context.Context, x execer, now time.Time, oc, acao, campo, antes, depois string, a Actor) error {
	_, err := x.ExecContext(ctx, `INSERT INTO historico(quando,ocorrencia_id,acao,campo,antes,depois,quem,ip) VALUES(?,?,?,?,?,?,?,?)`,
		now.UnixMilli(), oc, acao, campo, antes, depois, a.Who, a.IP)
	return err
}

// acquire takes the writer turn or gives up with ErrBusy when ctx ends first (M-2).
func (s *Store) acquire(ctx context.Context) error {
	select {
	case s.wsem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ErrBusy
	}
}

func (s *Store) release() { <-s.wsem }

// withTx runs f in a transaction, in the writers' turn.
func (s *Store) withTx(ctx context.Context, f func(tx *sql.Tx) error) error {
	return s.withTxSized(ctx, func(tx *sql.Tx, _ int64) error { return f(tx) })
}

// withTxSized is withTx that also hands f the live size of the database, measured
// after the writers' turn was obtained and before the transaction opens: nobody
// else writes in between, and the measure opens no pool connection that would
// wait for a write lock held by this very transaction (C-6.2, 1.1.3).
func (s *Store) withTxSized(ctx context.Context, f func(tx *sql.Tx, size int64) error) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.closed {
		return ErrClosed
	}
	size := s.Size()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	s.invalidated = nil
	if err := f(tx, size); err != nil {
		_ = tx.Rollback()
		s.invalidated = nil
		return err
	}
	// Every write checks the frozen indexes of examples (C-4.10, C-8.9): one that lists
	// something that stopped being an example goes invalid in the same transaction.
	if err := s.sweepIndexes(ctx, tx); err != nil {
		_ = tx.Rollback()
		s.invalidated = nil
		return err
	}
	if err := tx.Commit(); err != nil {
		s.invalidated = nil
		return err
	}
	s.flushInvalidated()
	return nil
}

// vacuum returns the free pages to the file system (C-6.2a); a failure only
// costs disk space, so it is logged and not returned.
func (s *Store) vacuum(ctx context.Context) {
	if err := s.acquire(ctx); err != nil {
		return
	}
	defer s.release()
	if _, err := s.db.ExecContext(ctx, `PRAGMA incremental_vacuum`); err != nil {
		s.logf("treino: incremental_vacuum: %v", err)
	}
}
