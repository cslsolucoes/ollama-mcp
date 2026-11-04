package treino

// Frozen index of examples and the embeddings of the public examples (Fase 12,
// stage 12.8; contract C-6.6, C-8.1 to C-8.5, C-8.9, C-8.10, C-4.10).
//
// The index is a snapshot, made on purpose (FreezeIndex), of the occurrences that
// may be shown to a model as an example: confirmed, marked public by the admin,
// in the TRAINING part, with an embedding, and passing the export predicate. The
// snapshot holds ids, the group and document hashes (to exclude them in an
// evaluation, C-8.4), the tool and the vector; the text of an example is read from
// the occurrence when the index is loaded. It has an id and a hash over what makes
// its answers (model, k, similarity floor, members and vectors), and a retrato
// records that hash (C-8.5). It goes invalid (valido=0) the moment any member stops
// being an example: unconfirmed, discarded, erased, no longer public, or in
// quarantine (C-4.10, C-8.9). The check runs in every write of this database, so
// no path is forgotten, and the process is told through Options.OnIndexInvalidated.
//
// Nothing here logs or returns a text through an error.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
)

// Limits of the retrieval parameters (C-8.10).
const (
	MaxExamplesK = 10
)

// Errors of the index and embedding operations.
var (
	ErrNoExamples    = errors.New("treino: nenhum exemplo público com embedding para congelar")
	ErrIndexNotFound = errors.New("treino: índice de exemplos não encontrado")
	ErrIndexInvalid  = errors.New("treino: índice de exemplos inválido (valido=0)")
	ErrIndexCorrupt  = errors.New("treino: índice de exemplos não confere com o hash registrado")
	ErrStaleEmbed    = errors.New("treino: a ocorrência mudou durante o cálculo do embedding")
	ErrNoSample      = errors.New("treino: ocorrência sem pedido para o embedding")
)

// FreezeOptions are the parameters frozen with an index (C-8.10).
type FreezeOptions struct {
	K      int     // pairs shown at most (1 to MaxExamplesK)
	Limiar float64 // cosine floor, in (0, 1]
	Modelo string  // embedding model the vectors must come from
	// Formato is the version of the format of the examples (melhora.FormatVersion): it is part of the
	// hash, so an index made with another format is a different index (C-8.10a).
	Formato string
}

// IndexMeta describes a frozen index.
type IndexMeta struct {
	ID       string
	CriadoEm int64 // epoch ms
	Hash     string
	Modelo   string
	Formato  string
	K        int
	Limiar   float64
	Valido   bool
	Itens    int
}

// IndexItem is a member with its vector and, when loaded, its content.
type IndexItem struct {
	OcorrenciaID, Tool, Categoria, GrupoID, DocHash, DocKeyID string
	Vec                                                       []float32
	// Content, read when the index is loaded (never stored in the snapshot).
	Endpoint        string
	PromptJSON      []byte
	Resposta        []byte
	RespostaCorreta string
}

// IndexData is a loaded index.
type IndexData struct {
	IndexMeta
	Items []IndexItem
}

// HashItem is what enters the hash of an index for each member.
type HashItem struct {
	ID  string
	Vec []byte // the stored blob (little-endian float32)
}

// IndexHash is the hash of an index: sha256 over the model, k, the floor and the
// members in id order, each with the bytes of its vector. Two indexes with the
// same hash answer every search the same way.
func IndexHash(modelo, formato string, k int, limiar float64, items []HashItem) string {
	its := append([]HashItem(nil), items...)
	sort.Slice(its, func(i, j int) bool { return its[i].ID < its[j].ID })
	h := sha256.New()
	fmt.Fprintf(h, "mcpollama-indice/2\n%s\n%s\n%d\n%.6f\n%d\n", modelo, formato, k, limiar, len(its))
	for _, it := range its {
		fmt.Fprintf(h, "%s\x00%d\x00", it.ID, len(it.Vec))
		h.Write(it.Vec)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// EncodeVec and DecodeVec are the storage form of a vector: little-endian float32.
func EncodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

// DecodeVec reads EncodeVec's form; nil when the blob has a bad size or a non-finite number.
func DecodeVec(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		f := math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return nil
		}
		v[i] = f
	}
	return v
}

// sweepIndexes invalidates every valid index that has a member that is no longer an
// example (C-4.10, C-8.9). It runs inside every write transaction, before the
// commit; the ids go to s.invalidated and are announced after the commit.
func (s *Store) sweepIndexes(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT i.id FROM indices i WHERE i.valido=1 AND EXISTS (
		SELECT 1 FROM indice_itens ii LEFT JOIN ocorrencias o ON o.id=ii.ocorrencia_id
		WHERE ii.indice_id=i.id AND (o.id IS NULL OR o.estado!=? OR o.exemplo_publico=0 OR o.particao!=?))`, EstadoConfirmada, ParteTreino)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE indices SET valido=0 WHERE id=?`, id); err != nil {
			return err
		}
		s.invalidated = append(s.invalidated, id)
	}
	return nil
}

// invalidateIndexesOf marks invalid the indexes that list an occurrence and drops
// the vector copies it has in them (erasing an occurrence leaves nothing of its
// content behind, C-6.3).
func (s *Store) invalidateIndexesOf(ctx context.Context, tx *sql.Tx, ocorrenciaID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT i.id FROM indices i JOIN indice_itens ii ON ii.indice_id=i.id WHERE ii.ocorrencia_id=? AND i.valido=1`, ocorrenciaID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE indices SET valido=0 WHERE id=?`, id); err != nil {
			return err
		}
		s.invalidated = append(s.invalidated, id)
	}
	_, err = tx.ExecContext(ctx, `UPDATE indice_itens SET vetor=x'' WHERE ocorrencia_id=?`, ocorrenciaID)
	return err
}

// flushInvalidated tells the process which indexes went invalid; called after the
// commit, still in the writers' turn.
func (s *Store) flushInvalidated() {
	ids := s.invalidated
	s.invalidated = nil
	if s.opts.OnIndexInvalidated == nil {
		return
	}
	for _, id := range ids {
		s.opts.OnIndexInvalidated(id)
	}
}

// FreezeIndex makes the snapshot of the examples (C-8.5). It fails with
// ErrNoExamples when there is none. Reading and writing are one transaction in the
// writers' turn, so a member that stops being an example a moment later invalidates it.
func (s *Store) FreezeIndex(ctx context.Context, o FreezeOptions) (IndexMeta, error) {
	if o.K < 1 || o.K > MaxExamplesK || !(o.Limiar > 0 && o.Limiar <= 1) || o.Modelo == "" || o.Formato == "" {
		return IndexMeta{}, ErrInvalid
	}
	var idb [16]byte
	if _, err := rand.Read(idb[:]); err != nil {
		return IndexMeta{}, err
	}
	m := IndexMeta{ID: hex.EncodeToString(idb[:]), CriadoEm: s.opts.Now().UnixMilli(), Modelo: o.Modelo, Formato: o.Formato, K: o.K, Limiar: o.Limiar, Valido: true}
	type member struct {
		id, tool, grupo, doc, docKey string
		blob                         []byte
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		// an index that went invalid is of no use to anybody (M-4 of the review): it goes with the freeze
		if _, err := deleteInvalidIndexes(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, elSelect+`WHERE o.estado=? AND o.exemplo_publico=1 AND o.particao=? ORDER BY o.id`, EstadoConfirmada, ParteTreino)
		if err != nil {
			return err
		}
		var els []ElRow
		for rows.Next() {
			r, err := scanEl(rows)
			if err != nil {
				rows.Close()
				return err
			}
			if ok, _ := Eligible(r, false); ok { // C-8.1, C-8.3, C-8.9: the single predicate, in the training part
				els = append(els, r)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		var ms []member
		dim := 0
		for _, r := range els {
			var blob []byte
			var modelo sql.NullString
			var grupo, doc, docKey sql.NullString
			err := tx.QueryRowContext(ctx, `SELECT e.vetor, e.modelo, o.grupo_id, o.doc_hash, o.doc_key_id FROM embeddings e JOIN ocorrencias o ON o.id=e.ocorrencia_id WHERE e.ocorrencia_id=?`, r.ID).
				Scan(&blob, &modelo, &grupo, &doc, &docKey)
			if err == sql.ErrNoRows || (err == nil && (len(blob) == 0 || modelo.String != o.Modelo)) {
				continue // embedding still pending (or from another model): not in this snapshot
			}
			if err != nil {
				return err
			}
			v := DecodeVec(blob)
			if v == nil {
				continue
			}
			if dim == 0 {
				dim = len(v)
			}
			if len(v) != dim {
				return fmt.Errorf("treino: vetores de tamanhos diferentes no índice (%d e %d)", dim, len(v))
			}
			ms = append(ms, member{id: r.ID, tool: r.Tool, grupo: grupo.String, doc: doc.String, docKey: docKey.String, blob: blob})
		}
		if len(ms) == 0 {
			return ErrNoExamples
		}
		hi := make([]HashItem, len(ms))
		for i, x := range ms {
			hi[i] = HashItem{ID: x.id, Vec: x.blob}
		}
		m.Hash, m.Itens = IndexHash(o.Modelo, o.Formato, o.K, o.Limiar, hi), len(ms)
		if _, err := tx.ExecContext(ctx, `INSERT INTO indices(id,criado_em,hash,modelo,formato,k,limiar,valido) VALUES(?,?,?,?,?,?,?,1)`,
			m.ID, m.CriadoEm, m.Hash, m.Modelo, m.Formato, m.K, m.Limiar); err != nil {
			return err
		}
		for _, x := range ms {
			if _, err := tx.ExecContext(ctx, `INSERT INTO indice_itens(indice_id,ocorrencia_id,tool,grupo_id,doc_hash,doc_key_id,vetor) VALUES(?,?,?,?,?,?,?)`,
				m.ID, x.id, x.tool, x.grupo, x.doc, x.docKey, x.blob); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return IndexMeta{}, err
	}
	return m, nil
}

// GetIndexMeta reads an index's header.
func (s *Store) GetIndexMeta(ctx context.Context, id string) (IndexMeta, error) {
	var m IndexMeta
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT i.id,i.criado_em,i.hash,i.modelo,i.formato,i.k,i.limiar,i.valido,(SELECT COUNT(*) FROM indice_itens WHERE indice_id=i.id) FROM indices i WHERE i.id=?`, id).
		Scan(&m.ID, &m.CriadoEm, &m.Hash, &m.Modelo, &m.Formato, &m.K, &m.Limiar, &v, &m.Itens)
	if err == sql.ErrNoRows {
		return m, ErrIndexNotFound
	}
	m.Valido = v != 0
	return m, err
}

// LoadIndex returns a valid index with the content of its members. It refuses an
// invalid one (ErrIndexInvalid), one whose members no longer match their hash
// (ErrIndexCorrupt), and invalidates the index when a member is no longer an
// example (defence in depth: the sweep of every write should have done it).
func (s *Store) LoadIndex(ctx context.Context, id string) (IndexData, error) {
	m, err := s.GetIndexMeta(ctx, id)
	if err != nil {
		return IndexData{}, err
	}
	if !m.Valido {
		return IndexData{IndexMeta: m}, ErrIndexInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ii.ocorrencia_id, ii.tool, COALESCE(o.categoria,''), COALESCE(ii.grupo_id,''), COALESCE(ii.doc_hash,''), COALESCE(ii.doc_key_id,''), ii.vetor,
		COALESCE(o.estado,''), COALESCE(o.exemplo_publico,0), COALESCE(o.particao,''),
		COALESCE(a.endpoint,''), a.prompt_json, a.resposta, COALESCE(o.resposta_correta,'')
		FROM indice_itens ii LEFT JOIN ocorrencias o ON o.id=ii.ocorrencia_id LEFT JOIN amostras a ON a.job_id=o.job_do_erro
		WHERE ii.indice_id=? ORDER BY ii.ocorrencia_id`, id)
	if err != nil {
		return IndexData{}, err
	}
	defer rows.Close()
	d := IndexData{IndexMeta: m}
	var hi []HashItem
	stale := false
	for rows.Next() {
		var it IndexItem
		var blob []byte
		var estado, particao string
		var pub int
		if err := rows.Scan(&it.OcorrenciaID, &it.Tool, &it.Categoria, &it.GrupoID, &it.DocHash, &it.DocKeyID, &blob, &estado, &pub, &particao,
			&it.Endpoint, &it.PromptJSON, &it.Resposta, &it.RespostaCorreta); err != nil {
			return IndexData{}, err
		}
		if estado != EstadoConfirmada || pub == 0 || particao != ParteTreino {
			stale = true
			continue
		}
		it.Vec = DecodeVec(blob)
		hi = append(hi, HashItem{ID: it.OcorrenciaID, Vec: blob})
		d.Items = append(d.Items, it)
	}
	if err := rows.Err(); err != nil {
		return IndexData{}, err
	}
	rows.Close()
	if stale {
		_ = s.withTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE indices SET valido=0 WHERE id=?`, id)
			s.invalidated = append(s.invalidated, id)
			return err
		})
		m.Valido = false
		return IndexData{IndexMeta: m}, ErrIndexInvalid
	}
	if len(d.Items) != m.Itens || IndexHash(m.Modelo, m.Formato, m.K, m.Limiar, hi) != m.Hash {
		return IndexData{IndexMeta: m}, ErrIndexCorrupt
	}
	for _, it := range d.Items {
		if it.Vec == nil {
			return IndexData{IndexMeta: m}, ErrIndexCorrupt
		}
	}
	return d, nil
}

// CaseMeta is what an evaluation needs to keep a case out of the examples (C-8.4):
// the group and the document (hash and key id) of an occurrence.
type CaseMeta struct {
	GrupoID, DocHash, DocKeyID string
}

// CaseMeta reads it.
func (s *Store) CaseMeta(ctx context.Context, ocorrenciaID string) (CaseMeta, error) {
	var c CaseMeta
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(grupo_id,''), COALESCE(doc_hash,''), COALESCE(doc_key_id,'') FROM ocorrencias WHERE id=?`, ocorrenciaID).
		Scan(&c.GrupoID, &c.DocHash, &c.DocKeyID)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	return c, err
}

// ---- embeddings (C-6.6) ---------------------------------------------------------------------

// PendingEmb is an occurrence that is a public example and has no vector yet.
// Version is atualizada_em when it was read: SetEmbedding only writes if it has
// not changed since.
type PendingEmb struct {
	OcorrenciaID string
	Version      int64
	Rowid        int64 // the cursor of PendingEmbeddingsAfter
}

// PendingEmbeddings lists the rows of embeddings with no vector (marked public by the
// admin and still confirmed), oldest first. No content is read.
func (s *Store) PendingEmbeddings(ctx context.Context, limit int) ([]PendingEmb, error) {
	return s.PendingEmbeddingsAfter(ctx, 0, limit)
}

// PendingEmbeddingsAfter is PendingEmbeddings from a cursor: the rows after rowid `after`. A row that
// failed for good (MarkEmbeddingFailed) is not pending any more, so it never holds the head of the
// queue (M-2 of the review).
func (s *Store) PendingEmbeddingsAfter(ctx context.Context, after int64, limit int) ([]PendingEmb, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.ocorrencia_id, o.atualizada_em, e.rowid FROM embeddings e JOIN ocorrencias o ON o.id=e.ocorrencia_id
		WHERE e.vetor IS NULL AND e.modelo IS NULL AND e.rowid>? AND o.estado=? AND o.exemplo_publico=1 ORDER BY e.rowid LIMIT ?`, after, EstadoConfirmada, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingEmb
	for rows.Next() {
		var p PendingEmb
		if err := rows.Scan(&p.OcorrenciaID, &p.Version, &p.Rowid); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EmbSource is the request of an occurrence, the text the embedding is made from.
type EmbSource struct {
	OcorrenciaID string
	Version      int64
	Tool         string
	Endpoint     string
	PromptJSON   []byte
}

// EmbeddingSource reads the request of a pending occurrence. ErrNoSample when it
// has none (the row stays pending and is not retried in a tight loop by the caller).
func (s *Store) EmbeddingSource(ctx context.Context, id string) (EmbSource, error) {
	var e EmbSource
	var pj []byte
	err := s.db.QueryRowContext(ctx, `SELECT o.id, o.atualizada_em, COALESCE(o.tool,''), COALESCE(a.endpoint,''), a.prompt_json
		FROM ocorrencias o LEFT JOIN amostras a ON a.job_id=o.job_do_erro WHERE o.id=?`, id).Scan(&e.OcorrenciaID, &e.Version, &e.Tool, &e.Endpoint, &pj)
	if err == sql.ErrNoRows {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	if len(pj) == 0 {
		return e, ErrNoSample
	}
	e.PromptJSON = pj
	return e, nil
}

// SetEmbedding stores the vector of a pending row. It writes only if the row is
// still pending and the occurrence is still a confirmed public example at the
// version that was read; otherwise ErrStaleEmbed (the caller just tries again on
// its next pass).
func (s *Store) SetEmbedding(ctx context.Context, id string, version int64, vec []float32, modelo string) error {
	if len(vec) == 0 || modelo == "" {
		return ErrInvalid
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE embeddings SET vetor=?, modelo=? WHERE ocorrencia_id=? AND vetor IS NULL
			AND EXISTS (SELECT 1 FROM ocorrencias o WHERE o.id=? AND o.atualizada_em=? AND o.exemplo_publico=1 AND o.estado=?)`,
			EncodeVec(vec), modelo, id, id, version, EstadoConfirmada)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrStaleEmbed
		}
		return nil
	})
}

// MarkEmbeddingFailed records that the embedding of a pending row cannot be made, for a reason that
// will not go away (no request to embed): the row keeps no vector and gets modelo "falha:<why>", so
// it is not pending any more and never holds the queue (M-2). Marking the occurrence public again
// (unmark, mark) makes a fresh row. why is a short constant, never text.
func (s *Store) MarkEmbeddingFailed(ctx context.Context, id string, version int64, why string) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE embeddings SET modelo=? WHERE ocorrencia_id=? AND vetor IS NULL AND modelo IS NULL
			AND EXISTS (SELECT 1 FROM ocorrencias o WHERE o.id=? AND o.atualizada_em=?)`, "falha:"+why, id, id, version)
		return err
	})
}

// deleteInvalidIndexes removes the indexes that went invalid, with their members (M-4). The
// retratos keep the hash they recorded: it is only a record.
func deleteInvalidIndexes(ctx context.Context, tx *sql.Tx) (int64, error) {
	if _, err := tx.ExecContext(ctx, `DELETE FROM indice_itens WHERE indice_id IN (SELECT id FROM indices WHERE valido=0)`); err != nil {
		return 0, err
	}
	r, err := tx.ExecContext(ctx, `DELETE FROM indices WHERE valido=0`)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

// DeleteInvalidIndexes is the cleanup command: it removes every invalid index and returns how many.
func (s *Store) DeleteInvalidIndexes(ctx context.Context) (n int64, err error) {
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		n, err = deleteInvalidIndexes(ctx, tx)
		return err
	})
	if err == nil && n > 0 {
		s.vacuum(ctx)
	}
	return n, err
}
