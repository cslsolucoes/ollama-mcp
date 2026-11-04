package treino

// Retrato (snapshot) of the test part (contract C-4.13, C-4.14, C-5.1, C-5.7): the
// frozen list of cases an evaluation uses. It is made from the test partition with
// the single export predicate (Eligible, chapter 3), minus the categories that have
// no criterion (C-5.7), and it never changes afterwards, except valido -> 0.
//
// Nothing here reads the content database: the cases are the samples already
// copied into treino.db (C-2.9). The content of a case (request and correct answer)
// is sensitive (C-5.6), so RetratoCases hands it to the caller in memory only, and
// the bake-off that calls it runs on the server, next to treino.db.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

// Categories without a criterion in the evaluation (C-5.7): they count only in the
// statistics.
const (
	CategoriaOutro  = "outro"
	CategoriaIdioma = "idioma_errado"
)

// NoneValue is what the retrato records for the example index and for the
// instruction version while stage 12.8 has not made them (C-4.13, C-8.5, C-8.7).
const NoneValue = "nenhum"

// Errors of the retrato operations.
var (
	ErrRetratoNotFound = errors.New("treino: retrato não encontrado")
	ErrRetratoInvalido = errors.New("treino: retrato inválido (valido=0)")
	ErrNoCases         = errors.New("treino: nenhum caso elegível na partição de teste")
)

// RetratoItem is one frozen case (C-4.13).
type RetratoItem struct {
	OcorrenciaID string
	JobID        string
	PromptHMAC   string
	Categoria    string
}

// Retrato is the frozen list and the hashes recorded with it (C-4.13).
type Retrato struct {
	ID             string
	CriadoEm       int64 // epoch ms
	IndiceHash     string
	InstrucoesID   string
	InstrucoesHash string
	// ExemplosK and ExemplosLimiar: see RetratoOptions (read back from the table).
	ExemplosK      int
	ExemplosLimiar float64
	Valido         bool
	Itens          []RetratoItem
}

// RetratoOptions are the hashes of the example index and of the instruction
// version that the evaluation will use (C-8.5, C-8.7). Empty means "nenhum".
type RetratoOptions struct {
	IndiceHash     string
	InstrucoesID   string
	InstrucoesHash string
	// ExemplosK and ExemplosLimiar are k and the similarity floor of the index the
	// evaluation uses (C-8.10); zero when there is no index.
	ExemplosK      int
	ExemplosLimiar float64
}

// RetratoCase is a case with its content, for the replay. It lives in memory only.
type RetratoCase struct {
	RetratoItem
	Tool            string
	Endpoint        string
	PromptJSON      []byte
	RespostaCorreta string
}

// Evaluable says whether a category has a criterion (C-5.7): everything but
// `outro` and `idioma_errado`.
func Evaluable(categoria string) bool {
	return categoria != "" && categoria != CategoriaOutro && categoria != CategoriaIdioma
}

func nz(s string) string {
	if s == "" {
		return NoneValue
	}
	return s
}

// CreateRetrato freezes the cases of the test partition that pass the predicate of
// chapter 3 and whose category has a criterion (C-5.7). The reading and the writing
// are one transaction, in the writers' turn, so a case discarded a moment before is
// not listed and one discarded a moment after invalidates the retrato (C-1.12b).
// It fails with ErrNoCases when there is nothing to freeze.
func (s *Store) CreateRetrato(ctx context.Context, o RetratoOptions) (Retrato, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Retrato{}, err
	}
	rt := Retrato{ID: hex.EncodeToString(id[:]), CriadoEm: s.opts.Now().UnixMilli(),
		IndiceHash: nz(o.IndiceHash), InstrucoesID: nz(o.InstrucoesID), InstrucoesHash: nz(o.InstrucoesHash), Valido: true}
	if o.IndiceHash != "" { // k and the floor go with the index, never alone (C-8.10)
		rt.ExemplosK, rt.ExemplosLimiar = o.ExemplosK, o.ExemplosLimiar
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, elSelect+`WHERE o.estado=? AND o.particao=? ORDER BY o.id`, EstadoConfirmada, ParteTeste)
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
			if ok, _ := Eligible(r, false); ok && Evaluable(r.Categoria) { // C-5.1, C-5.7
				els = append(els, r)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, r := range els {
			var ph sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT prompt_hmac FROM amostras WHERE job_id=?`, r.JobDoErro).Scan(&ph); err != nil {
				return err
			}
			if ph.String == "" {
				continue // a case that cannot be conferred later is not frozen
			}
			rt.Itens = append(rt.Itens, RetratoItem{OcorrenciaID: r.ID, JobID: r.JobDoErro, PromptHMAC: ph.String, Categoria: r.Categoria})
		}
		if len(rt.Itens) == 0 {
			return ErrNoCases
		}
		var ek, el any // NULL without an index
		if rt.ExemplosK > 0 {
			ek, el = rt.ExemplosK, rt.ExemplosLimiar
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO retratos(id,criado_em,indice_hash,instrucoes_id,instrucoes_hash,exemplos_k,exemplos_limiar,valido) VALUES(?,?,?,?,?,?,?,1)`,
			rt.ID, rt.CriadoEm, rt.IndiceHash, rt.InstrucoesID, rt.InstrucoesHash, ek, el); err != nil {
			return err
		}
		for _, it := range rt.Itens {
			if _, err := tx.ExecContext(ctx, `INSERT INTO retrato_itens(retrato_id,ocorrencia_id,job_id,prompt_hmac,categoria) VALUES(?,?,?,?,?)`,
				rt.ID, it.OcorrenciaID, it.JobID, it.PromptHMAC, it.Categoria); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Retrato{}, err
	}
	return rt, nil
}

// GetRetrato reads a retrato and its items (no content).
func (s *Store) GetRetrato(ctx context.Context, id string) (Retrato, error) {
	var rt Retrato
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT id,criado_em,COALESCE(indice_hash,''),COALESCE(instrucoes_id,''),COALESCE(instrucoes_hash,''),COALESCE(exemplos_k,0),COALESCE(exemplos_limiar,0),valido FROM retratos WHERE id=?`, id).
		Scan(&rt.ID, &rt.CriadoEm, &rt.IndiceHash, &rt.InstrucoesID, &rt.InstrucoesHash, &rt.ExemplosK, &rt.ExemplosLimiar, &v)
	if err == sql.ErrNoRows {
		return rt, ErrRetratoNotFound
	}
	if err != nil {
		return rt, err
	}
	rt.Valido = v != 0
	rows, err := s.db.QueryContext(ctx, `SELECT ocorrencia_id,job_id,COALESCE(prompt_hmac,''),COALESCE(categoria,'') FROM retrato_itens WHERE retrato_id=? ORDER BY ocorrencia_id`, id)
	if err != nil {
		return rt, err
	}
	defer rows.Close()
	for rows.Next() {
		var it RetratoItem
		if err := rows.Scan(&it.OcorrenciaID, &it.JobID, &it.PromptHMAC, &it.Categoria); err != nil {
			return rt, err
		}
		rt.Itens = append(rt.Itens, it)
	}
	return rt, rows.Err()
}

// InvalidateRetrato sets valido=0, the only change a retrato admits (C-4.13).
func (s *Store) InvalidateRetrato(ctx context.Context, id string) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE retratos SET valido=0 WHERE id=?`, id)
		return err
	})
}

// RetratoCases loads the content of every case of a valid retrato. Before handing
// anything it checks each item against the database as it is NOW: the occurrence
// must still be confirmed, in the test part, with the same category and job, still
// pass the predicate of chapter 3 and still carry the request that was frozen
// (prompt_hmac equal; and, when the store has the key, recomputed from the stored
// request). Any difference makes the retrato invalid (valido=0, C-4.13) and the
// call returns ErrRetratoInvalido: an invalid retrato is never used (C-5.13). The
// error names only the occurrence id, never content.
func (s *Store) RetratoCases(ctx context.Context, id string) (Retrato, []RetratoCase, error) {
	rt, err := s.GetRetrato(ctx, id)
	if err != nil {
		return rt, nil, err
	}
	if !rt.Valido {
		return rt, nil, ErrRetratoInvalido
	}
	bad := func(oc string) error {
		if err := s.InvalidateRetrato(ctx, id); err != nil {
			return err
		}
		rt.Valido = false
		return fmt.Errorf("%w: caso %s mudou desde o congelamento", ErrRetratoInvalido, oc)
	}
	out := make([]RetratoCase, 0, len(rt.Itens))
	for _, it := range rt.Itens {
		r, err := scanEl(s.db.QueryRowContext(ctx, elSelect+`WHERE o.id=?`, it.OcorrenciaID))
		if err == sql.ErrNoRows {
			return rt, nil, bad(it.OcorrenciaID)
		}
		if err != nil {
			return rt, nil, err
		}
		if ok, _ := Eligible(r, false); !ok || r.Particao != ParteTeste || r.Categoria != it.Categoria || r.JobDoErro != it.JobID || !Evaluable(r.Categoria) {
			return rt, nil, bad(it.OcorrenciaID)
		}
		var pj []byte
		var ph sql.NullString
		if err := s.db.QueryRowContext(ctx, `SELECT prompt_json, prompt_hmac FROM amostras WHERE job_id=?`, it.JobID).Scan(&pj, &ph); err != nil {
			if err == sql.ErrNoRows {
				return rt, nil, bad(it.OcorrenciaID)
			}
			return rt, nil, err
		}
		if ph.String != it.PromptHMAC {
			return rt, nil, bad(it.OcorrenciaID)
		}
		if s.opts.Key.Configured() && s.opts.Key.PromptHMAC(pj) != it.PromptHMAC {
			return rt, nil, bad(it.OcorrenciaID)
		}
		out = append(out, RetratoCase{RetratoItem: it, Tool: r.Tool, Endpoint: r.Endpoint, PromptJSON: pj, RespostaCorreta: r.RespostaCorreta})
	}
	return rt, out, nil
}
