package treino

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// The operations below are the Go API that the admin routes of stage 12.6 call.
// Each one is a single transaction that changes the occurrence and writes its
// history rows (C-7.3). The contract's transitions are the only ones (C-1.12).

type row struct {
	estado string
	resp   string
	desc   string
	corr   string
}

func loadRow(ctx context.Context, tx *sql.Tx, id string) (row, error) {
	var r row
	err := tx.QueryRowContext(ctx, `SELECT estado, COALESCE(resposta_correta,''), COALESCE(descricao,''), COALESCE(correcao_aplicada,'') FROM ocorrencias WHERE id=?`, id).
		Scan(&r.estado, &r.resp, &r.desc, &r.corr)
	if err == sql.ErrNoRows {
		return r, ErrNotFound
	}
	return r, err
}

// dropExample takes the occurrence out of every use as an example and as a test
// case (C-1.12b, C-6.5, C-4.11a): the embedding is deleted, exemplo_publico goes
// to 0 and every snapshot (retrato) that lists it becomes invalid. The vaga of a
// test occurrence in the quota is NOT given back (C-4.11a, C-4.11b): the quota
// lives in cota_grupos, which nothing in this file touches, and the partition and
// grupo_id of the occurrence stay as they are.
func (s *Store) dropExample(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM embeddings WHERE ocorrencia_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET exemplo_publico=0 WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE retratos SET valido=0 WHERE id IN (SELECT retrato_id FROM retrato_itens WHERE ocorrencia_id=?)`, id); err != nil {
		return err
	}
	// the frozen index of examples that lists it goes invalid and loses the vector copy (C-4.10, C-8.9)
	return s.invalidateIndexesOf(ctx, tx, id)
}

// Confirm moves nova to confirmada (C-1.12). It needs a non-empty resposta_correta
// (C-1.12a). Only the admin calls it (C-1.13).
func (s *Store) Confirm(ctx context.Context, id string, a Actor) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		r, err := loadRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if r.estado != EstadoNova {
			return ErrTransition
		}
		if strings.TrimSpace(r.resp) == "" {
			return ErrNeedsAnswer
		}
		now := s.opts.Now()
		if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET estado=?, atualizada_em=?, revisada_por=?, revisada_em=? WHERE id=?`,
			EstadoConfirmada, now.UnixMilli(), a.Who, now.UnixMilli(), id); err != nil {
			return err
		}
		if err := hist(ctx, tx, now, id, AcaoConfirmou, "estado", EstadoNova, EstadoConfirmada, a); err != nil {
			return err
		}
		return s.recompute(ctx, tx, now) // the group may now have its first exportable sample (C-4.6)
	})
}

// Discard moves nova, confirmada, sem_pedido, distribuida or fallback to
// descartada (C-1.12); descartada never returns. Discarding a confirmada also
// drops the example and invalidates the snapshots that list it (C-1.12b).
func (s *Store) Discard(ctx context.Context, id string, a Actor) error {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		r, err := loadRow(ctx, tx, id)
		if err != nil {
			return err
		}
		switch r.estado {
		case EstadoNova, EstadoConfirmada, EstadoSemPedido, EstadoDistrib, EstadoFallback:
		default:
			return ErrTransition
		}
		now := s.opts.Now()
		if r.estado == EstadoConfirmada {
			if err := s.dropExample(ctx, tx, id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET estado=?, atualizada_em=?, revisada_por=?, revisada_em=? WHERE id=?`,
			EstadoDescartada, now.UnixMilli(), a.Who, now.UnixMilli(), id); err != nil {
			return err
		}
		if err := hist(ctx, tx, now, id, AcaoDescartou, "estado", r.estado, EstadoDescartada, a); err != nil {
			return err
		}
		return s.recompute(ctx, tx, now)
	})
	if err == nil {
		s.vacuum(ctx)
	}
	return err
}

// Edit holds the fields an admin may change; nil leaves a field alone.
type Edit struct {
	RespostaCorreta  *string
	Descricao        *string
	CorrecaoAplicada *string
}

// Update changes resposta_correta, descricao and correcao_aplicada of a nova or
// confirmada occurrence; every changed field gets an `alterou` row with hash and
// size only (C-7.4). Changing any of them in a confirmada undoes the
// confirmation (C-6.5): it goes back to nova, the embedding is deleted and a
// `desconfirmou` row is written. It reports whether that happened.
func (s *Store) Update(ctx context.Context, id string, e Edit, a Actor) (desconfirmou bool, err error) {
	if e.Descricao != nil && len(*e.Descricao) > MaxDescricao || e.RespostaCorreta != nil && len(*e.RespostaCorreta) > MaxResposta ||
		e.CorrecaoAplicada != nil && len(*e.CorrecaoAplicada) > MaxCorrecao {
		return false, ErrInvalid
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		r, err := loadRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if r.estado != EstadoNova && r.estado != EstadoConfirmada {
			return ErrTransition
		}
		now := s.opts.Now()
		type f struct {
			campo, col, old string
			neu             *string
		}
		changed := false
		for _, c := range []f{
			{"resposta_correta", "resposta_correta", r.resp, e.RespostaCorreta},
			{"descricao", "descricao", r.desc, e.Descricao},
			{"correcao_aplicada", "correcao_aplicada", r.corr, e.CorrecaoAplicada},
		} {
			if c.neu == nil || *c.neu == c.old {
				continue
			}
			changed = true
			// col is a constant of this function, never a caller's text.
			if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET `+c.col+`=?, atualizada_em=? WHERE id=?`, *c.neu, now.UnixMilli(), id); err != nil {
				return err
			}
			if err := hist(ctx, tx, now, id, AcaoAlterou, c.campo, hashSize(s.opts.Key, c.old), hashSize(s.opts.Key, *c.neu), a); err != nil {
				return err
			}
		}
		if changed && r.estado == EstadoConfirmada {
			if err := s.dropExample(ctx, tx, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET estado=?, atualizada_em=?, revisada_por=NULL, revisada_em=NULL WHERE id=?`,
				EstadoNova, now.UnixMilli(), id); err != nil {
				return err
			}
			if err := hist(ctx, tx, now, id, AcaoDesconfirmou, "estado", EstadoConfirmada, EstadoNova, a); err != nil {
				return err
			}
			desconfirmou = true
		}
		if changed {
			return s.recompute(ctx, tx, now)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return desconfirmou, nil
}

// Delete moves any state to apagada (C-1.12) and leaves only the minimal row
// (id, estado=apagada, dates; C-6.3). Its jobs are removed, and a sample goes
// only when no other occurrence references it (C-6.4); the embedding goes and the
// snapshots that list it become invalid. The `apagou` row carries hash and size
// of the content fields (C-7.8); the history stays (C-7.9).
func (s *Store) Delete(ctx context.Context, id string, a Actor) error {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		r, err := loadRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if r.estado == EstadoApagada {
			return ErrTransition
		}
		now := s.opts.Now()
		jobs, err := jobIDs(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.dropExample(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM ocorrencia_jobs WHERE ocorrencia_id=?`, id); err != nil {
			return err
		}
		for _, j := range jobs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM amostras WHERE job_id=? AND NOT EXISTS (SELECT 1 FROM ocorrencia_jobs WHERE job_id=?)`, j, j); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET estado=?, atualizada_em=?,
			categoria=NULL, severidade=NULL, descricao=NULL, resposta_correta=NULL, correcao_aplicada=NULL,
			origem=NULL, projeto=NULL, tool=NULL, estacao_computer=NULL, estacao_user=NULL, estacao_domain=NULL, estacao_mac=NULL,
			client_ip=NULL, client_label=NULL, job_do_erro=NULL, doc_hash=NULL, doc_key_id=NULL, grupo_id=NULL,
			particao='', exemplo_publico=0, revisada_por=NULL, revisada_em=NULL WHERE id=?`,
			EstadoApagada, now.UnixMilli(), id); err != nil {
			return err
		}
		antes := "estado=" + r.estado + " descricao=[" + hashSize(s.opts.Key, r.desc) + "] resposta_correta=[" + hashSize(s.opts.Key, r.resp) + "] correcao_aplicada=[" + hashSize(s.opts.Key, r.corr) + "]"
		if err := hist(ctx, tx, now, id, AcaoApagou, "conteudo", antes, "estado="+EstadoApagada, a); err != nil {
			return err
		}
		return s.recompute(ctx, tx, now) // an erased occurrence leaves the groups; the quota rows stay (C-4.11b)
	})
	if err == nil {
		s.vacuum(ctx) // the space comes back to the budget (C-6.2a)
	}
	return err
}

func jobIDs(ctx context.Context, tx *sql.Tx, id string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT job_id FROM ocorrencia_jobs WHERE ocorrencia_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var j string
		if err := rows.Scan(&j); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Now exposes the clock of the store (tests and the admin routes).
func (s *Store) Now() time.Time { return s.opts.Now() }
