package treino

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// Job is what the job log (gateway.db, table requests) says about a job.
type Job struct {
	Tool, Endpoint, Model, Think, Status string
	HTTPStatus                           int
	DocHash, DocKeyID                    string // HMAC of X-Mcpollama-Doc and the id of its key ("" when none)
	StationComputer, StationUser         string // declared by the workstation that made the job ("" when none)
	ClientIP                             string // as the job log keeps it (IPv6 grouped by /64)
}

// busy is true while the job has not ended (C-1.4a).
func (j Job) busy() bool { return j.Status == queue.StatusQueued || j.Status == queue.StatusRunning }

// sameOrigin is C-1.11a (1.1.3): the verified IP of the report equals the one
// the gateway recorded for the job (the same calculation: proxy.ClientIP, which
// trusts X-Real-Ip only from a loopback peer, then proxy.FairIP), and, when the
// job has a declared station, the station (machine and user) equals too. The
// station alone is declared and can be forged, so it never decides by itself.
func sameOrigin(j Job, o Origin) bool {
	ipOK := o.ClientIP != "" && j.ClientIP != "" && proxy.FairIP(o.ClientIP) == j.ClientIP
	if j.StationComputer != "" && j.StationUser != "" {
		return o.Computer == j.StationComputer && o.User == j.StationUser && ipOK
	}
	return ipOK
}

// sameOccOrigin is the same rule for an occurrence already stored.
func sameOccOrigin(computer, user, ip string, o Origin) bool {
	ipOK := o.ClientIP != "" && ip != "" && proxy.FairIP(ip) == proxy.FairIP(o.ClientIP)
	if computer != "" && user != "" {
		return o.Computer == computer && o.User == user && ipOK
	}
	return ipOK
}

// Content is what the content database (content.db) holds for a job.
type Content struct {
	Prompt, Response    string
	PromptJSON          []byte // nil: no canonical request was kept (job before 0.13.0, or the scan failed)
	PromptTruncated     bool
	PromptJSONTruncated bool
	HasImage            bool
	RespFinal           bool
	RespTruncated       bool
}

// Probe says what a job holds in the content database without loading it (M-1).
type Probe struct {
	OK      bool  // the row exists
	HasJSON bool  // a canonical request is there
	Bytes   int64 // what the three fields take
}

// Source reads the jobs of this gateway. The real one is NewSource; the tests
// use fakes. The context is the caller's own, never a monitor visitor's.
type Source interface {
	// Job returns the request row of a job; ok is false when this gateway has none.
	Job(ctx context.Context, id string) (j Job, ok bool, err error)
	// Probe sizes the content of a job without reading the text.
	Probe(ctx context.Context, id string) (Probe, error)
	// Content returns the stored text of a job; ok is false when it expired or was never kept.
	Content(ctx context.Context, id string) (c Content, ok bool, err error)
}

type realSource struct {
	st *store.Store
	cs *content.Store // may be nil (--log-content=false): every job has no content
}

// NewSource reads from the job log and the content database. cs may be nil.
func NewSource(st *store.Store, cs *content.Store) Source { return realSource{st: st, cs: cs} }

func (r realSource) Job(ctx context.Context, id string) (Job, bool, error) {
	jr, ok, err := r.st.Job(ctx, id)
	if err != nil || !ok {
		return Job{}, false, err
	}
	dh, kid, err := r.st.JobDoc(ctx, id)
	if err != nil {
		return Job{}, false, err
	}
	return Job{Tool: jr.Tool, Endpoint: jr.Endpoint, Model: jr.Model, Think: jr.Think, Status: jr.Status, HTTPStatus: jr.HTTPStatus,
		DocHash: dh, DocKeyID: kid, StationComputer: jr.StationComputer, StationUser: jr.StationUser, ClientIP: jr.ClientIP}, true, nil
}

func (r realSource) Probe(ctx context.Context, id string) (Probe, error) {
	if r.cs == nil {
		return Probe{}, nil
	}
	p, err := r.cs.Probe(ctx, id)
	return Probe{OK: p.OK, HasJSON: p.HasJSON, Bytes: p.Bytes}, err
}

func (r realSource) Content(ctx context.Context, id string) (Content, bool, error) {
	if r.cs == nil {
		return Content{}, false, nil
	}
	c, ok, err := r.cs.Get(ctx, id)
	if err != nil || !ok {
		return Content{}, false, err
	}
	pj, _, err := r.cs.GetPromptJSON(ctx, id)
	if err != nil {
		return Content{}, false, err
	}
	return Content{Prompt: c.Prompt, Response: c.Response, PromptJSON: pj.JSON, PromptTruncated: c.PromptTruncated,
		PromptJSONTruncated: pj.Truncated, HasImage: pj.HasImage, RespFinal: c.RespFinal, RespTruncated: c.RespTruncated}, true, nil
}

// Origin is what the gateway knows about who reports: the declared station
// (X-Mcpollama-Station, not authenticated, C-1.18), the IP and the client label.
type Origin struct {
	Computer, User, Domain, MAC string
	ClientIP, ClientLabel       string
}

// Who is the actor name of the history for a report (C-7.5): computer/user, as
// the default client label of the workstation is built.
func (o Origin) Who() string {
	switch {
	case o.Computer != "" && o.User != "":
		return o.Computer + "/" + o.User
	case o.Computer != "":
		return o.Computer
	case o.User != "":
		return o.User
	case o.ClientLabel != "":
		return o.ClientLabel
	}
	return "desconhecido"
}

// Result is the answer of a report (D12-5, §8.3). SemPedido counts the local jobs
// that got no sample because their content expired or holds no canonical request;
// JobsOutraOrigem counts the local jobs of another origin than the report
// (C-1.11a), kept as references only. AmostrasCopiadas counts the sample rows
// written by this report; a sample that another occurrence already holds (C-1.6)
// is shared, not copied again.
type Result struct {
	OcorrenciaID     string `json:"ocorrencia_id"`
	Estado           string `json:"estado"`
	AmostrasCopiadas int    `json:"amostras_copiadas"`
	SemPedido        int    `json:"sem_pedido"`
	JobsExternos     int    `json:"jobs_externos"`
	JobsOutraOrigem  int    `json:"jobs_outra_origem"`
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Ingest records a validated report in one transaction (C-1.5): the occurrence,
// its jobs, the samples of the local jobs and the history row. role is this
// gateway's --role. It fails with ErrNoKey (C-4.3a), ErrCapReached (C-6.2),
// ErrJobNotFound (C-1.4) or ErrJobBusy (C-1.4a) before writing anything, and with
// ErrBusy when the writers' turn does not come before the deadline (M-2).
//
// State (C-1.9, precedence distribuida > fallback > sem_pedido > nova):
// distribuida when the jobs have different backends, fallback on a gateway with
// --role fallback, sem_pedido when the job_do_erro is of another origin (C-1.11a),
// its content expired or it has no canonical request, nova otherwise. The first
// two copy no sample at all.
//
// A human report of a job whose occurrence is a nova automatic candidate of the
// same origin replaces it (C-1.19): the state is recomputed by the same
// precedence, the job list is swapped, and every changed field gets an `alterou`
// row (C-1.12d). An automatic report of a job that already has an occurrence of
// the same origin returns that one and writes nothing. An occurrence of another
// origin is never returned or touched: the report makes a new one.
func (s *Store) Ingest(ctx context.Context, rep Report, o Origin, role string, src Source, key *hmackey.Key) (Result, error) {
	return s.IngestWith(ctx, rep, o, role, src, key, IngestOptions{})
}

// IngestOptions changes how a report is taken.
type IngestOptions struct {
	// Admin is the "Marcar erro" action of the monitor (D12-6): the report comes
	// from the logged-in administrator, so it has no same-origin restriction
	// (C-1.11a: the samples of every local job are copied, and the occurrence of
	// the job is found whatever its origin) and the history names "admin".
	Admin bool
}

// IngestWith is Ingest with options.
func (s *Store) IngestWith(ctx context.Context, rep Report, o Origin, role string, src Source, key *hmackey.Key, opts IngestOptions) (Result, error) {
	same := func(j Job) bool { return opts.Admin || sameOrigin(j, o) }
	if !key.Configured() {
		return Result{}, ErrNoKey
	}
	if src == nil {
		return Result{}, errInvalidSource
	}
	if role != RoleFallback {
		role = RolePrimario
	}
	maxB := s.opts.MaxBytes
	// A first look, to refuse early; the binding measure is taken in the writers'
	// turn, just before the transaction (C-6.2, 1.1.3).
	if sz := s.Size(); sz >= maxB {
		s.capRefused(sz)
		return Result{}, ErrCapReached
	}
	jobErr, ok, err := src.Job(ctx, rep.JobDoErro)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, ErrJobNotFound
	}
	errBackend := ""
	for _, j := range rep.Jobs {
		if j.ID == rep.JobDoErro {
			errBackend = j.Backend
		}
	}
	externos := 0
	for _, j := range rep.Jobs {
		if j.Backend != errBackend {
			externos++
		}
	}
	// The local jobs (same backend as the job_do_erro): none may still be in the
	// line or running (C-1.4a). A local job this gateway no longer knows stays a
	// reference only.
	jobs := map[string]Job{rep.JobDoErro: jobErr}
	for _, j := range rep.Jobs {
		if j.Backend != errBackend || j.ID == rep.JobDoErro {
			continue
		}
		jb, ok, err := src.Job(ctx, j.ID)
		if err != nil {
			return Result{}, err
		}
		if ok {
			jobs[j.ID] = jb
		}
	}
	for _, jb := range jobs {
		if jb.busy() {
			return Result{}, ErrJobBusy
		}
	}
	who := Actor{Who: o.Who(), IP: o.ClientIP}
	if opts.Admin {
		who = AdminActor(o.ClientIP)
	}

	var res Result
	err = s.withTxSized(ctx, func(tx *sql.Tx, size int64) error {
		if size >= maxB { // measured in this writer's turn: no window between measuring and writing
			s.capRefused(size)
			return ErrCapReached
		}
		now := s.opts.Now()
		// An occurrence of this job and of the same origin already there (not erased)?
		var exID, exOrigem, exEstado string
		have, err := s.findSameOrigin(ctx, tx, rep.JobDoErro, o, opts.Admin, &exID, &exOrigem, &exEstado)
		if err != nil {
			return err
		}
		if have && rep.Origem == "automatico" {
			res = Result{OcorrenciaID: exID, Estado: exEstado}
			return nil // the same candidate again: nothing to write
		}
		replace := have && exOrigem == "automatico" && exEstado == EstadoNova

		// The state (C-1.9 precedence).
		estado := EstadoNova
		switch {
		case externos > 0:
			estado = EstadoDistrib
		case role == RoleFallback:
			estado = EstadoFallback
		case !same(jobErr):
			estado = EstadoSemPedido
		default:
			has, err := s.hasSampleOrContent(ctx, tx, src, rep.JobDoErro)
			if err != nil {
				return err
			}
			if !has {
				estado = EstadoSemPedido
			}
		}

		id := exID
		if replace {
			if err := s.replaceOccurrence(ctx, tx, now, exID, rep, o, who, key, estado); err != nil {
				return err
			}
		} else {
			if id, err = newID(); err != nil {
				return err
			}
			dh, kid := "", ""
			if k := key.ID(); k != "" && jobErr.DocHash != "" {
				dh, kid = jobErr.DocHash, jobErr.DocKeyID
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado,categoria,severidade,descricao,resposta_correta,correcao_aplicada,
				origem,projeto,tool,estacao_computer,estacao_user,estacao_domain,estacao_mac,client_ip,client_label,job_do_erro,doc_hash,doc_key_id)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, now.UnixMilli(), now.UnixMilli(), estado, rep.Categoria, rep.Severidade, rep.Descricao, rep.RespostaCorreta, rep.CorrecaoAplicada,
				rep.Origem, rep.Projeto, jobErr.Tool, o.Computer, o.User, o.Domain, o.MAC, o.ClientIP, o.ClientLabel, rep.JobDoErro, dh, kid); err != nil {
				return err
			}
			if err := hist(ctx, tx, now, id, AcaoCriou, "", "", fmt.Sprintf("estado=%s origem=%s categoria=%s severidade=%s descricao=[%s] resposta_correta=[%s] correcao_aplicada=[%s]",
				estado, rep.Origem, rep.Categoria, rep.Severidade, hashSize(key, rep.Descricao), hashSize(key, rep.RespostaCorreta), hashSize(key, rep.CorrecaoAplicada)), who); err != nil {
				return err
			}
			if err := insertJobs(ctx, tx, id, rep.Jobs); err != nil {
				return err
			}
		}

		res = Result{OcorrenciaID: id, Estado: estado, JobsExternos: externos}
		if estado == EstadoDistrib || estado == EstadoFallback {
			return s.recompute(ctx, tx, now) // C-1.10, C-1.21: references only, no sample at all
		}
		budget := maxB - size
		var written int64
		for _, j := range rep.Jobs {
			jb, known := jobs[j.ID]
			if !known {
				res.SemPedido++ // this gateway no longer has the job: only the reference stays
				continue
			}
			if !same(jb) {
				res.JobsOutraOrigem++ // C-1.11a: reference only
				continue
			}
			// M-1: a job that already has a sample is not read again (C-1.6).
			var one int
			switch err := tx.QueryRowContext(ctx, `SELECT 1 FROM amostras WHERE job_id=?`, j.ID).Scan(&one); err {
			case nil:
				continue
			case sql.ErrNoRows:
			default:
				return err
			}
			pr, err := src.Probe(ctx, j.ID)
			if err != nil {
				return err
			}
			if !pr.OK || !pr.HasJSON {
				res.SemPedido++ // expired or no canonical request: only the reference stays (C-1.9a)
				continue
			}
			written += pr.Bytes // sized before the three fields are loaded
			if written > budget {
				s.capRefused(size + written)
				return ErrCapReached
			}
			c, ok, err := src.Content(ctx, j.ID)
			if err != nil {
				return err
			}
			if !ok || c.PromptJSON == nil {
				res.SemPedido++
				continue
			}
			n, err := s.insertSample(ctx, tx, now, j.ID, jb, c, key)
			if err != nil {
				return err
			}
			res.AmostrasCopiadas += n
		}
		return s.recompute(ctx, tx, now)
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// findSameOrigin looks for an occurrence of the job, not erased, made by the same
// origin as the report (C-1.11a); an occurrence of another origin is not seen.
func (s *Store) findSameOrigin(ctx context.Context, tx *sql.Tx, job string, o Origin, anyOrigin bool, id, origem, estado *string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, COALESCE(origem,''), estado, COALESCE(estacao_computer,''), COALESCE(estacao_user,''), COALESCE(client_ip,'')
		FROM ocorrencias WHERE job_do_erro=? AND estado!=? ORDER BY criada_em, id`, job, EstadoApagada)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var i, og, es, c, u, ip string
		if err := rows.Scan(&i, &og, &es, &c, &u, &ip); err != nil {
			return false, err
		}
		if anyOrigin || sameOccOrigin(c, u, ip, o) {
			*id, *origem, *estado = i, og, es
			return true, nil
		}
	}
	return false, rows.Err()
}

// hasSampleOrContent says whether the job has what a sample needs: a sample
// already copied, or a canonical request in the content database (not loaded).
func (s *Store) hasSampleOrContent(ctx context.Context, tx *sql.Tx, src Source, job string) (bool, error) {
	var one int
	switch err := tx.QueryRowContext(ctx, `SELECT 1 FROM amostras WHERE job_id=?`, job).Scan(&one); err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
	default:
		return false, err
	}
	p, err := src.Probe(ctx, job)
	return p.OK && p.HasJSON, err
}

func insertJobs(ctx context.Context, tx *sql.Tx, id string, jobs []JobRef) error {
	for i, j := range jobs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ocorrencia_jobs(ocorrencia_id,job_id,papel,backend,ordem) VALUES(?,?,?,?,?)`,
			id, j.ID, j.Papel, j.Backend, i); err != nil {
			return err
		}
	}
	return nil
}

// insertSample copies one job (C-1.7, C-2.9). The prompt_hmac and conteudo_hmac
// come from hmackey (C-2.12). doc_hash is kept only when it was made with the
// current key, because the sample has a single key_id.
func (s *Store) insertSample(ctx context.Context, tx *sql.Tx, now time.Time, jobID string, j Job, c Content, key *hmackey.Key) (int, error) {
	ph := key.PromptHMAC(c.PromptJSON)
	ch, err := key.ContentHMAC(c.PromptJSON)
	if err != nil {
		ch = "" // a JSON that is not an object (truncated): no content hash
	}
	dh := ""
	if j.DocHash != "" && j.DocKeyID == key.ID() {
		dh = j.DocHash
	}
	r, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO amostras(job_id,copiada_em,endpoint,modelo,think,prompt_json,prompt_texto,resposta,
		prompt_hmac,conteudo_hmac,key_id,prompt_truncado,prompt_json_truncado,tem_imagem,doc_hash,resp_final,resp_truncada,status,http_status)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		jobID, now.UnixMilli(), j.Endpoint, j.Model, j.Think, c.PromptJSON, []byte(c.Prompt), []byte(c.Response),
		ph, ch, key.ID(), b2i(c.PromptTruncated), b2i(c.PromptJSONTruncated), b2i(c.HasImage), dh, b2i(c.RespFinal), b2i(c.RespTruncated),
		j.Status, j.HTTPStatus)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// jobList is the list of an occurrence as the history shows it: papel:id@backend, in order.
func jobList(ctx context.Context, tx *sql.Tx, id string) (string, []string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT papel, job_id, backend FROM ocorrencia_jobs WHERE ocorrencia_id=? ORDER BY ordem`, id)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var parts, ids []string
	for rows.Next() {
		var p, j, b string
		if err := rows.Scan(&p, &j, &b); err != nil {
			return "", nil, err
		}
		parts = append(parts, p+":"+j+"@"+b)
		ids = append(ids, j)
	}
	return strings.Join(parts, ","), ids, rows.Err()
}

// replaceOccurrence turns a nova automatic candidate into the human report
// (C-1.19, C-1.12d): the state is the one recomputed for the new list, the job
// list is swapped (not summed), and every changed field, the station and the
// origin included, gets an `alterou` row. Content fields enter the history as
// hmac and size only (C-7.4). When the new state is distribuida or fallback, or a
// job left the list, the samples nobody else uses leave too (C-6.4).
func (s *Store) replaceOccurrence(ctx context.Context, tx *sql.Tx, now time.Time, id string, rep Report, o Origin, who Actor, key *hmackey.Key, estado string) error {
	var old Report
	var oldOrigem string
	var oc, ou, od, om, oip, olb string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(categoria,''),COALESCE(severidade,''),COALESCE(descricao,''),COALESCE(resposta_correta,''),
		COALESCE(correcao_aplicada,''),COALESCE(origem,''),COALESCE(projeto,''),
		COALESCE(estacao_computer,''),COALESCE(estacao_user,''),COALESCE(estacao_domain,''),COALESCE(estacao_mac,''),COALESCE(client_ip,''),COALESCE(client_label,'')
		FROM ocorrencias WHERE id=?`, id).
		Scan(&old.Categoria, &old.Severidade, &old.Descricao, &old.RespostaCorreta, &old.CorrecaoAplicada, &oldOrigem, &old.Projeto, &oc, &ou, &od, &om, &oip, &olb)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ocorrencias SET atualizada_em=?, estado=?, categoria=?, severidade=?, descricao=?, resposta_correta=?, correcao_aplicada=?,
		origem=?, projeto=?, estacao_computer=?, estacao_user=?, estacao_domain=?, estacao_mac=?, client_ip=?, client_label=? WHERE id=?`,
		now.UnixMilli(), estado, rep.Categoria, rep.Severidade, rep.Descricao, rep.RespostaCorreta, rep.CorrecaoAplicada, rep.Origem, rep.Projeto,
		o.Computer, o.User, o.Domain, o.MAC, o.ClientIP, o.ClientLabel, id); err != nil {
		return err
	}
	// The job list is swapped.
	oldList, oldIDs, err := jobList(ctx, tx, id)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ocorrencia_jobs WHERE ocorrencia_id=?`, id); err != nil {
		return err
	}
	if err := insertJobs(ctx, tx, id, rep.Jobs); err != nil {
		return err
	}
	newList, _, err := jobList(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, j := range oldIDs {
		// a sample that no occurrence references any more goes (C-6.4)
		if _, err := tx.ExecContext(ctx, `DELETE FROM amostras WHERE job_id=? AND NOT EXISTS (SELECT 1 FROM ocorrencia_jobs WHERE job_id=amostras.job_id)`, j); err != nil {
			return err
		}
		if estado == EstadoDistrib || estado == EstadoFallback {
			// no sample at all for these states (C-1.10, C-1.21): what only this occurrence used goes
			if _, err := tx.ExecContext(ctx, `DELETE FROM amostras WHERE job_id=? AND NOT EXISTS (SELECT 1 FROM ocorrencia_jobs WHERE job_id=amostras.job_id AND ocorrencia_id!=?)`, j, id); err != nil {
				return err
			}
		}
	}
	type ch struct {
		campo, antes, depois string
		content              bool
	}
	for _, c := range []ch{
		{"origem", oldOrigem, rep.Origem, false},
		{"estado", EstadoNova, estado, false},
		{"categoria", old.Categoria, rep.Categoria, false},
		{"severidade", old.Severidade, rep.Severidade, false},
		{"projeto", old.Projeto, rep.Projeto, false},
		{"estacao_computer", oc, o.Computer, false},
		{"estacao_user", ou, o.User, false},
		{"estacao_domain", od, o.Domain, false},
		{"estacao_mac", om, o.MAC, false},
		{"client_ip", oip, o.ClientIP, false},
		{"client_label", olb, o.ClientLabel, false},
		{"jobs", oldList, newList, false},
		{"descricao", old.Descricao, rep.Descricao, true},
		{"resposta_correta", old.RespostaCorreta, rep.RespostaCorreta, true},
		{"correcao_aplicada", old.CorrecaoAplicada, rep.CorrecaoAplicada, true},
	} {
		if c.antes == c.depois {
			continue
		}
		a, d := c.antes, c.depois
		if c.content {
			a, d = hashSize(key, a), hashSize(key, d)
		}
		if err := hist(ctx, tx, now, id, AcaoAlterou, c.campo, a, d, who); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) capRefused(size int64) {
	s.event("treino_teto_atingido", fmt.Sprintf("treino.db com %d bytes de dados vivos (teto %d): novos relatos recusados com 507; nada foi apagado", size, s.opts.MaxBytes))
	s.logf("treino: teto atingido (%d de %d bytes): relato recusado", size, s.opts.MaxBytes)
}
