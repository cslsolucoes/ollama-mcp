package treino

// Partition, groups, quota and quarantine (contract chapter 4, version 1.1.x) and
// the single export predicate (chapter 3).
//
// recompute runs inside the transaction of every change that can alter a group
// (a report, a confirmation, a discard, an erasure, a release): it rebuilds the
// groups of all the occurrences that are not erased and applies the partition
// rules. Groups and sides are stored in ocorrencias (grupo_id, particao); the
// quota is stored in cota_grupos (C-4.11b), which nothing else writes or deletes.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CotaPorCategoria is the number of groups of each category that go to the test
// part first (C-4.6).
const CotaPorCategoria = 30

// Sides of the partition (C-1.8). The empty string is "pending".
const (
	ParteTreino     = "treino"
	ParteTeste      = "teste"
	ParteQuarentena = "quarentena"
)

// systemActor is who the history names for a change the gateway makes by itself.
var systemActor = Actor{Who: "sistema"}

// generationEndpoints are the jobs of generation (C-3.5): never embeddings.
var generationEndpoints = map[string]bool{
	"/api/generate": true, "/api/chat": true, "/v1/chat/completions": true, "/v1/completions": true,
}

// eligibleTools are the tools whose jobs may be exported (C-3.7).
var eligibleTools = map[string]bool{"generate": true, "chat": true, "summarize_file": true, "extract": true, "translate": true}

// ElRow is what the export predicate looks at: the occurrence and the sample of
// its job_do_erro (C-3.2 to C-3.10). The predicate reads only the sample, never
// the content database (C-2.9).
type ElRow struct {
	ID, Estado, Particao, Tool, Categoria, RespostaCorreta, JobDoErro, Papel string
	HasSample                                                                bool
	Endpoint, Status                                                         string
	HTTPStatus                                                               int
	PromptTruncado, PromptJSONTruncado, TemImagem, RespFinal, RespTruncada   bool
	HasPromptJSON                                                            bool
	RevisadaEm                                                               int64
}

// Eligible is the ONE export predicate (C-3.1). It returns false with the name of
// the first condition that fails. ignorePartition drops only the part of C-3.9
// that depends on the partition itself (used to decide the partition of a group,
// C-4.6); the rest of C-3.9 (the state) stays.
func Eligible(r ElRow, ignorePartition bool) (bool, string) {
	switch {
	case r.Estado != EstadoConfirmada: // C-3.2 and, with it, C-3.9 (distribuida, fallback)
		return false, "estado"
	case !r.HasSample: // C-3.3
		return false, "sem_amostra"
	case strings.TrimSpace(r.RespostaCorreta) == "": // C-3.4
		return false, "resposta_vazia"
	case !generationEndpoints[r.Endpoint] || r.Status != "ok" || r.HTTPStatus != 200: // C-3.5
		return false, "job"
	case r.PromptTruncado || r.PromptJSONTruncado || !r.RespFinal || r.RespTruncada || !r.HasPromptJSON: // C-3.6
		return false, "truncado"
	case !eligibleTools[r.Tool]: // C-3.7
		return false, "tool"
	case r.TemImagem: // C-3.8
		return false, "imagem"
	case !ignorePartition && r.Particao == ParteQuarentena: // C-3.9
		return false, "quarentena"
	case r.Tool == "translate" && !translatePartOK(r.Papel): // C-3.10
		return false, "translate_montado"
	}
	return true, ""
}

// translatePartOK: in translate only a part (mapN) or the single call (unico) can
// be exported; the assembled text and anything else cannot (C-3.10).
func translatePartOK(papel string) bool {
	if papel == "unico" {
		return true
	}
	return strings.HasPrefix(papel, "map") && papelRe.MatchString(papel)
}

const elSelect = `SELECT o.id, o.estado, o.particao, COALESCE(o.tool,''), COALESCE(o.categoria,''), COALESCE(o.resposta_correta,''), COALESCE(o.job_do_erro,''),
	COALESCE(oj.papel,''), a.job_id IS NOT NULL, COALESCE(a.endpoint,''), COALESCE(a.status,''), COALESCE(a.http_status,0),
	COALESCE(a.prompt_truncado,1), COALESCE(a.prompt_json_truncado,1), COALESCE(a.tem_imagem,1), COALESCE(a.resp_final,0), COALESCE(a.resp_truncada,1),
	COALESCE(length(a.prompt_json),0) > 0, COALESCE(o.revisada_em,0)
	FROM ocorrencias o
	LEFT JOIN ocorrencia_jobs oj ON oj.ocorrencia_id=o.id AND oj.job_id=o.job_do_erro
	LEFT JOIN amostras a INDEXED BY idx_amostras_pred ON a.job_id=o.job_do_erro `

func scanEl(sc interface{ Scan(...any) error }) (ElRow, error) {
	var r ElRow
	var pt, pjt, img, rf, rt int
	err := sc.Scan(&r.ID, &r.Estado, &r.Particao, &r.Tool, &r.Categoria, &r.RespostaCorreta, &r.JobDoErro, &r.Papel, &r.HasSample, &r.Endpoint, &r.Status,
		&r.HTTPStatus, &pt, &pjt, &img, &rf, &rt, &r.HasPromptJSON, &r.RevisadaEm)
	r.PromptTruncado, r.PromptJSONTruncado, r.TemImagem, r.RespFinal, r.RespTruncada = pt != 0, pjt != 0, img != 0, rf != 0, rt != 0
	return r, err
}

// ---- groups -----------------------------------------------------------------------------------

type occ struct {
	id, estado, categoria, grupo, particao string
	revisadaEm                             int64
}

type component struct {
	members []*occ
}

// buildComponents groups every occurrence that is not erased: two occurrences are
// in the same group when they share a job, a document hash or a content hash
// (C-4.5, C-4.15), comparing hashes only under the same key id (C-4.3a).
func buildComponents(ctx context.Context, x execer) ([]component, error) {
	rows, err := x.QueryContext(ctx, `SELECT id, estado, COALESCE(categoria,''), COALESCE(grupo_id,''), particao, COALESCE(revisada_em,0),
		COALESCE(doc_hash,''), COALESCE(doc_key_id,'') FROM ocorrencias WHERE estado!=? ORDER BY id`, EstadoApagada)
	if err != nil {
		return nil, err
	}
	var occs []*occ
	idx := map[string]int{}
	var keysOf [][]string
	for rows.Next() {
		o := &occ{}
		var dh, dk string
		if err := rows.Scan(&o.id, &o.estado, &o.categoria, &o.grupo, &o.particao, &o.revisadaEm, &dh, &dk); err != nil {
			rows.Close()
			return nil, err
		}
		idx[o.id] = len(occs)
		occs = append(occs, o)
		var k []string
		if dh != "" {
			k = append(k, "d:"+dk+":"+dh)
		}
		keysOf = append(keysOf, k)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = x.QueryContext(ctx, `SELECT oj.ocorrencia_id, oj.job_id, COALESCE(a.key_id,''), COALESCE(a.doc_hash,''), COALESCE(a.conteudo_hmac,'')
		FROM ocorrencia_jobs oj LEFT JOIN amostras a INDEXED BY idx_amostras_grupo ON a.job_id=oj.job_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var oid, job, kid, dh, ch string
		if err := rows.Scan(&oid, &job, &kid, &dh, &ch); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := idx[oid]
		if !ok {
			continue
		}
		keysOf[i] = append(keysOf[i], "j:"+job)
		if dh != "" {
			keysOf[i] = append(keysOf[i], "d:"+kid+":"+dh)
		}
		if ch != "" {
			keysOf[i] = append(keysOf[i], "c:"+kid+":"+ch)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	parent := make([]int, len(occs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	first := map[string]int{}
	for i, ks := range keysOf {
		for _, k := range ks {
			if j, ok := first[k]; ok {
				if a, b := find(i), find(j); a != b {
					parent[b] = a
				}
			} else {
				first[k] = i
			}
		}
	}
	byRoot := map[int]*component{}
	var order []int
	for i, o := range occs {
		r := find(i)
		c := byRoot[r]
		if c == nil {
			c = &component{}
			byRoot[r] = c
			order = append(order, r)
		}
		c.members = append(c.members, o)
	}
	out := make([]component, 0, len(order))
	for _, r := range order {
		out = append(out, *byRoot[r])
	}
	return out, nil
}

// groupID is the id a component keeps or takes (C-4.8): the smallest id any member
// already carries; a brand new group takes the smallest occurrence id.
//
// Note (M-3 of the review of 12.6): the id of a group that already has a side can change
// when it absorbs a pending group (no side yet) that carries a smaller id. The side, which
// is what C-4.12 protects, never changes, and the quota rows keep the id they were made under.
func (c component) groupID(taken map[string]bool) string {
	best := ""
	for _, m := range c.members {
		if m.grupo != "" && !taken[m.grupo] && (best == "" || m.grupo < best) {
			best = m.grupo
		}
	}
	if best != "" {
		return best
	}
	for _, m := range c.members {
		if !taken[m.id] && (best == "" || m.id < best) {
			best = m.id
		}
	}
	// Every id the members could use is taken by another component (a split part whose id was
	// the id of a member of the other part): derive a new one, deterministic and unused.
	for k := 0; best == ""; k++ {
		sum := sha256.Sum256([]byte(c.members[0].id + "/" + strconv.Itoa(k)))
		if cand := hex.EncodeToString(sum[:16]); !taken[cand] {
			best = cand
		}
	}
	return best
}

// hashSide is C-4.7: sha256(group id), read as a 256-bit number, mod 100 < 15 goes to the test.
func hashSide(grupo string) string {
	sum := sha256.Sum256([]byte(grupo))
	n := new(big.Int).SetBytes(sum[:])
	if new(big.Int).Mod(n, big.NewInt(100)).Int64() < 15 {
		return ParteTeste
	}
	return ParteTreino
}

// recompute rebuilds the groups and applies the partition rules (C-4.5 to C-4.9),
// in the transaction of the change that called it. It records an `alterou` row,
// by "sistema", for every occurrence whose partition changes.
func (s *Store) recompute(ctx context.Context, tx *sql.Tx, now time.Time) error {
	comps, err := buildComponents(ctx, tx)
	if err != nil {
		return err
	}
	type pend struct {
		c      component
		first  *occ
		catego string
	}
	var pending []pend
	type newState struct{ grupo, particao string }
	target := map[string]newState{}
	var quarantined [][]string // components that just entered quarantine (C-4.10)
	taken := map[string]bool{} // a group id belongs to one component: a split gives the new part an id of its own
	for _, c := range comps {
		hasT, hasE, anyQ := false, false, false
		for _, m := range c.members {
			switch m.particao {
			case ParteTreino:
				hasT = true
			case ParteTeste:
				hasE = true
			case ParteQuarentena:
				anyQ = true
			}
		}
		g := c.groupID(taken)
		taken[g] = true
		side := ""
		switch {
		case anyQ || (hasT && hasE): // C-4.9a: opposite sides meet, or a quarantined group is joined
			side = ParteQuarentena
			var entering []string
			for _, m := range c.members {
				if m.particao != ParteQuarentena {
					entering = append(entering, m.id)
				}
			}
			if len(entering) > 0 {
				var all []string
				for _, m := range c.members {
					all = append(all, m.id)
				}
				quarantined = append(quarantined, all)
			}
		case hasT:
			side = ParteTreino // C-4.9b: one side only: the component inherits it
		case hasE:
			side = ParteTeste
		default: // C-4.9c: no side yet: pending until a member is exportable (C-4.6)
			var first *occ
			for _, m := range c.members {
				if m.estado != EstadoConfirmada {
					continue
				}
				ok, err := s.eligibleOnce(ctx, tx, m.id)
				if err != nil {
					return err
				}
				if ok && (first == nil || m.revisadaEm < first.revisadaEm || (m.revisadaEm == first.revisadaEm && m.id < first.id)) {
					first = m
				}
			}
			if first != nil {
				pending = append(pending, pend{c: c, first: first, catego: first.categoria})
			}
		}
		for _, m := range c.members {
			target[m.id] = newState{grupo: g, particao: side}
		}
	}
	// C-4.6: in the order in which each group gets its first exportable sample.
	sort.SliceStable(pending, func(i, j int) bool {
		a, b := pending[i].first, pending[j].first
		if a.revisadaEm != b.revisadaEm {
			return a.revisadaEm < b.revisadaEm
		}
		return a.id < b.id
	})
	for _, p := range pending {
		g := target[p.c.members[0].id].grupo
		var used int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cota_grupos WHERE categoria=? AND particao=?`, p.catego, ParteTeste).Scan(&used); err != nil {
			return err
		}
		side := ParteTeste // the first 30 groups of the category are the test
		if used >= CotaPorCategoria {
			side = hashSide(g) // C-4.7
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cota_grupos(grupo_id,categoria,particao,atribuida_em) VALUES(?,?,?,?)`, g, p.catego, side, now.UnixMilli()); err != nil {
			return err
		}
		for _, m := range p.c.members {
			target[m.id] = newState{grupo: g, particao: side}
		}
	}
	for _, c := range comps {
		for _, m := range c.members {
			t := target[m.id]
			if t.grupo == m.grupo && t.particao == m.particao {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET grupo_id=?, particao=? WHERE id=?`, t.grupo, t.particao, m.id); err != nil {
				return err
			}
			if t.particao != m.particao {
				if err := hist(ctx, tx, now, m.id, AcaoAlterou, "particao", m.particao, t.particao, systemActor); err != nil {
					return err
				}
			}
			if m.grupo != "" && t.grupo != m.grupo {
				if err := hist(ctx, tx, now, m.id, AcaoAlterou, "grupo_id", m.grupo, t.grupo, systemActor); err != nil {
					return err
				}
			}
		}
	}
	// C-4.10: the snapshots that had cases of a component that just entered quarantine are invalid.
	// The frozen example index is referenced by the snapshot (indice_hash), so it goes with it.
	for _, ids := range quarantined {
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE retratos SET valido=0 WHERE id IN (SELECT retrato_id FROM retrato_itens WHERE ocorrencia_id=?)`, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// eligibleOnce says whether an occurrence passes the export predicate apart from
// the partition (C-4.6).
func (s *Store) eligibleOnce(ctx context.Context, x execer, id string) (bool, error) {
	r, err := scanEl(x.QueryRowContext(ctx, elSelect+`WHERE o.id=?`, id))
	if err != nil {
		return false, err
	}
	ok, _ := Eligible(r, true)
	return ok, nil
}

// ReleaseQuarantine lets the admin choose a side for a whole quarantined
// component (C-4.11): every member gets that side and an `alterou`/`particao` row.
func (s *Store) ReleaseQuarantine(ctx context.Context, id, lado string, a Actor) error {
	_, err := s.ReleaseQuarantineInfo(ctx, id, lado, a)
	return err
}

// ReleaseInfo tells the administrator what a release touched (M-1 of the review of 12.6).
type ReleaseInfo struct {
	Membros int `json:"membros"`
	// ExportadosNaOutraParte counts the members that were already exported to the OTHER part
	// before the quarantine: releasing them to this side leaves the same case in a file that
	// went to the other side. The release is not blocked (C-4.11 allows it); the admin is told.
	ExportadosNaOutraParte int `json:"exportados_na_outra_parte"`
}

// ReleaseQuarantineInfo is ReleaseQuarantine that also reports ReleaseInfo.
func (s *Store) ReleaseQuarantineInfo(ctx context.Context, id, lado string, a Actor) (ReleaseInfo, error) {
	var info ReleaseInfo
	if lado != ParteTreino && lado != ParteTeste {
		return info, ErrInvalid
	}
	other := ParteTeste
	if lado == ParteTeste {
		other = ParteTreino
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		r, err := loadRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if r.estado == EstadoApagada {
			return ErrTransition
		}
		var particao string
		if err := tx.QueryRowContext(ctx, `SELECT particao FROM ocorrencias WHERE id=?`, id).Scan(&particao); err != nil {
			return err
		}
		if particao != ParteQuarentena {
			return ErrTransition
		}
		comps, err := buildComponents(ctx, tx)
		if err != nil {
			return err
		}
		now := s.opts.Now()
		for _, c := range comps {
			found := false
			for _, m := range c.members {
				if m.id == id {
					found = true
				}
			}
			if !found {
				continue
			}
			info.Membros = len(c.members)
			for _, m := range c.members {
				var ex int
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM historico WHERE ocorrencia_id=? AND acao=? AND campo=?)`, m.id, AcaoExportou, other).Scan(&ex); err != nil {
					return err
				}
				info.ExportadosNaOutraParte += ex
				if m.particao == lado {
					continue
				}
				if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET particao=?, atualizada_em=? WHERE id=?`, lado, now.UnixMilli(), m.id); err != nil {
					return err
				}
				if err := hist(ctx, tx, now, m.id, AcaoAlterou, "particao", m.particao, lado, a); err != nil {
					return err
				}
			}
		}
		return s.recompute(ctx, tx, now)
	})
	return info, err
}

// SetExemploPublico marks or unmarks a confirmed occurrence as a public example
// (C-6.6). Marking leaves the embedding PENDING: a row of embeddings with no
// vector and no model, which stage 12.8 fills in; unmarking deletes it. The
// embedding itself is not computed here.
func (s *Store) SetExemploPublico(ctx context.Context, id string, on bool, a Actor) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		r, err := loadRow(ctx, tx, id)
		if err != nil {
			return err
		}
		var cur int
		if err := tx.QueryRowContext(ctx, `SELECT exemplo_publico FROM ocorrencias WHERE id=?`, id).Scan(&cur); err != nil {
			return err
		}
		if on && r.estado != EstadoConfirmada {
			return ErrTransition // only a confirmed occurrence can be an example
		}
		if (cur != 0) == on {
			return nil
		}
		now := s.opts.Now()
		if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET exemplo_publico=?, atualizada_em=? WHERE id=?`, b2i(on), now.UnixMilli(), id); err != nil {
			return err
		}
		if on {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO embeddings(ocorrencia_id, vetor, modelo) VALUES(?, NULL, NULL)`, id); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `DELETE FROM embeddings WHERE ocorrencia_id=?`, id); err != nil {
			return err
		}
		return hist(ctx, tx, now, id, AcaoAlterou, "exemplo_publico", itoa(b2i(cur != 0)), itoa(b2i(on)), a)
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	return "1"
}
