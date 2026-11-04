package treino

// Read side for the monitor (D12-6): the counts that anybody may see, and the
// list and detail that only the administrator sees. Every read opens no
// transaction and takes no writers' turn; the caller gives a context of its own
// (C-6.10), never a visitor's.

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Summary is the open part: counts and the use of the ceiling, no text at all.
type Summary struct {
	Enabled      bool           `json:"enabled"`
	SizeBytes    int64          `json:"size_bytes"`
	MaxBytes     int64          `json:"max_bytes"`
	Total        int            `json:"total"` // occurrences that are not erased
	PorEstado    map[string]int `json:"por_estado"`
	PorParticao  map[string]int `json:"por_particao"` // "pendente" is the empty partition
	PorCategoria map[string]int `json:"por_categoria"`
	ExportTreino int            `json:"exportaveis_treino"`
	ExportTeste  int            `json:"exportaveis_teste"`
	Quarentena   int            `json:"quarentena"`
	EmbPendentes int            `json:"embeddings_pendentes"`
	Cota         map[string]int `json:"cota_teste_por_categoria"`
}

// statsStates are the states that count in the error statistics (C-1.14): not
// descartada and not apagada.
var statsStates = map[string]bool{EstadoNova: true, EstadoConfirmada: true, EstadoSemPedido: true, EstadoDistrib: true, EstadoFallback: true}

// Summary computes the open counts.
func (s *Store) Summary(ctx context.Context) (Summary, error) {
	sm := Summary{Enabled: true, SizeBytes: s.Size(), MaxBytes: s.opts.MaxBytes,
		PorEstado: map[string]int{}, PorParticao: map[string]int{}, PorCategoria: map[string]int{}, Cota: map[string]int{}}
	rows, err := s.db.QueryContext(ctx, `SELECT estado, particao, COALESCE(categoria,''), COUNT(*) FROM ocorrencias WHERE estado!=? GROUP BY 1,2,3`, EstadoApagada)
	if err != nil {
		return sm, err
	}
	for rows.Next() {
		var e, p, c string
		var n int
		if err := rows.Scan(&e, &p, &c, &n); err != nil {
			rows.Close()
			return sm, err
		}
		sm.Total += n
		sm.PorEstado[e] += n
		if p == "" {
			p = "pendente"
		}
		sm.PorParticao[p] += n
		if statsStates[e] {
			sm.PorCategoria[c] += n
		}
		if p == ParteQuarentena {
			sm.Quarentena += n
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return sm, err
	}
	rows.Close()
	// exportable cases (the predicate, part by part) and the pending embeddings
	rows, err = s.db.QueryContext(ctx, elSelect+`WHERE o.estado=? AND o.particao IN (?,?)`, EstadoConfirmada, ParteTreino, ParteTeste)
	if err != nil {
		return sm, err
	}
	for rows.Next() {
		r, err := scanEl(rows)
		if err != nil {
			rows.Close()
			return sm, err
		}
		if ok, _ := Eligible(r, false); ok {
			if r.Particao == ParteTreino {
				sm.ExportTreino++
			} else {
				sm.ExportTeste++
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return sm, err
	}
	rows.Close()
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM embeddings WHERE vetor IS NULL`).Scan(&sm.EmbPendentes); err != nil {
		return sm, err
	}
	crow, err := s.db.QueryContext(ctx, `SELECT categoria, COUNT(*) FROM cota_grupos WHERE particao=? GROUP BY 1`, ParteTeste)
	if err != nil {
		return sm, err
	}
	defer crow.Close()
	for crow.Next() {
		var c string
		var n int
		if err := crow.Scan(&c, &n); err != nil {
			return sm, err
		}
		if n > CotaPorCategoria { // m-7: the groups the 15% rule sent to the test are not the quota
			n = CotaPorCategoria
		}
		sm.Cota[c] = n
	}
	return sm, crow.Err()
}

// Filter narrows the list. Zero fields do not filter.
type Filter struct {
	Categoria, Tool, Estado, Particao, Estacao string // Particao "pendente" selects the empty one
	Desde, Ate                                 int64  // criada_em, epoch ms
	Limit, Offset                              int
}

// ListRow is one line of the list (administrator only).
type ListRow struct {
	ID             string `json:"id"`
	CriadaEm       int64  `json:"criada_em"`
	Estado         string `json:"estado"`
	Categoria      string `json:"categoria"`
	Severidade     string `json:"severidade"`
	Origem         string `json:"origem"`
	Projeto        string `json:"projeto"`
	Tool           string `json:"tool"`
	Estacao        string `json:"estacao"`
	Particao       string `json:"particao"`
	Grupo          string `json:"grupo"`
	ExemploPublico bool   `json:"exemplo_publico"`
	EmbPendente    bool   `json:"embedding_pendente"`
	Descricao      string `json:"descricao"` // clipped
}

// List returns one page of the occurrences that are not erased, newest first.
func (s *Store) List(ctx context.Context, f Filter) ([]ListRow, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	var where []string
	var args []any
	where = append(where, `o.estado!=?`)
	args = append(args, EstadoApagada)
	add := func(cond string, a ...any) { where = append(where, cond); args = append(args, a...) }
	if f.Categoria != "" {
		add(`o.categoria=?`, f.Categoria)
	}
	if f.Tool != "" {
		add(`o.tool=?`, f.Tool)
	}
	if f.Estado != "" {
		add(`o.estado=?`, f.Estado)
	}
	switch f.Particao {
	case "":
	case "pendente":
		add(`o.particao=''`)
	default:
		add(`o.particao=?`, f.Particao)
	}
	if f.Estacao != "" {
		add(`(o.estacao_computer=? OR o.estacao_user=?)`, f.Estacao, f.Estacao)
	}
	if f.Desde > 0 {
		add(`o.criada_em>=?`, f.Desde)
	}
	if f.Ate > 0 {
		add(`o.criada_em<=?`, f.Ate)
	}
	q := `SELECT o.id, o.criada_em, o.estado, COALESCE(o.categoria,''), COALESCE(o.severidade,''), COALESCE(o.origem,''), COALESCE(o.projeto,''), COALESCE(o.tool,''),
		COALESCE(o.estacao_computer,''), COALESCE(o.estacao_user,''), o.particao, COALESCE(o.grupo_id,''), o.exemplo_publico,
		EXISTS(SELECT 1 FROM embeddings e WHERE e.ocorrencia_id=o.id AND e.vetor IS NULL), COALESCE(substr(o.descricao,1,200),'')
		FROM ocorrencias o WHERE ` + strings.Join(where, " AND ") + ` ORDER BY o.criada_em DESC, o.id LIMIT ? OFFSET ?`
	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ListRow{}
	for rows.Next() {
		var r ListRow
		var comp, user string
		var ex int
		if err := rows.Scan(&r.ID, &r.CriadaEm, &r.Estado, &r.Categoria, &r.Severidade, &r.Origem, &r.Projeto, &r.Tool, &comp, &user, &r.Particao, &r.Grupo, &ex, &r.EmbPendente, &r.Descricao); err != nil {
			return nil, err
		}
		r.ExemploPublico = ex != 0
		switch {
		case comp != "" && user != "":
			r.Estacao = comp + "/" + user
		default:
			r.Estacao = comp + user
		}
		if len(r.Grupo) > 8 {
			r.Grupo = r.Grupo[:8]
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DetailJob is one job of an occurrence.
type DetailJob struct {
	ID      string `json:"id"`
	Papel   string `json:"papel"`
	Backend string `json:"backend"`
	Ordem   int    `json:"ordem"`
}

// DetailSample is the sample of a job: the marks always, the text and the
// answer because the detail is for the administrator only.
type DetailSample struct {
	JobID           string `json:"job_id"`
	Endpoint        string `json:"endpoint"`
	Modelo          string `json:"modelo"`
	Think           string `json:"think"`
	Status          string `json:"status"`
	HTTPStatus      int    `json:"http_status"`
	PromptTruncado  bool   `json:"prompt_truncado"`
	PromptJSONTrunc bool   `json:"prompt_json_truncado"`
	TemImagem       bool   `json:"tem_imagem"`
	RespFinal       bool   `json:"resp_final"`
	RespTruncada    bool   `json:"resp_truncada"`
	PromptBytes     int    `json:"prompt_bytes"` // the whole size, even when the text shown is cut
	RespBytes       int    `json:"resp_bytes"`
	Prompt          string `json:"prompt"`
	Resposta        string `json:"resposta"`
	PromptCortado   bool   `json:"prompt_cortado"`
	RespCortada     bool   `json:"resposta_cortada"`
}

// DetailTextMax is how much of each text field the detail shows (M-2 of the review of 12.6):
// a field can hold 16 MiB, and a page of the monitor must not.
const DetailTextMax = 256 << 10

// cutText keeps the first DetailTextMax bytes of s (at a character boundary) and marks the cut
// with the size of the whole text.
func cutText(s string) (string, bool) {
	if len(s) <= DetailTextMax {
		return s, false
	}
	end := DetailTextMax
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "\n[cortado (" + strconv.Itoa(len(s)) + " bytes)]", true
}

// DetailHist is a row of the history (hash and size only, never text).
type DetailHist struct {
	Quando int64  `json:"quando"`
	Acao   string `json:"acao"`
	Campo  string `json:"campo"`
	Antes  string `json:"antes"`
	Depois string `json:"depois"`
	Quem   string `json:"quem"`
	IP     string `json:"ip"`
}

// Detail is the whole occurrence (administrator only).
type Detail struct {
	ListRow
	Descricao        string         `json:"descricao"`
	RespostaCorreta  string         `json:"resposta_correta"`
	CorrecaoAplicada string         `json:"correcao_aplicada"`
	JobDoErro        string         `json:"job_do_erro"`
	Computer         string         `json:"estacao_computer"`
	User             string         `json:"estacao_user"`
	Domain           string         `json:"estacao_domain"`
	ClientIP         string         `json:"client_ip"`
	RevisadaPor      string         `json:"revisada_por"`
	RevisadaEm       int64          `json:"revisada_em"`
	Exportavel       bool           `json:"exportavel"`
	MotivoNaoExport  string         `json:"motivo_nao_exportavel"`
	Jobs             []DetailJob    `json:"jobs"`
	Amostras         []DetailSample `json:"amostras"`
	Historico        []DetailHist   `json:"historico"`
}

// Detail reads one occurrence; ErrNotFound when it does not exist or was erased.
func (s *Store) Detail(ctx context.Context, id string) (Detail, error) {
	var d Detail
	var ex int
	var comp, user string
	err := s.db.QueryRowContext(ctx, `SELECT o.id, o.criada_em, o.estado, COALESCE(o.categoria,''), COALESCE(o.severidade,''), COALESCE(o.origem,''), COALESCE(o.projeto,''), COALESCE(o.tool,''),
		COALESCE(o.estacao_computer,''), COALESCE(o.estacao_user,''), COALESCE(o.estacao_domain,''), COALESCE(o.client_ip,''), o.particao, COALESCE(o.grupo_id,''), o.exemplo_publico,
		EXISTS(SELECT 1 FROM embeddings e WHERE e.ocorrencia_id=o.id AND e.vetor IS NULL),
		COALESCE(o.descricao,''), COALESCE(o.resposta_correta,''), COALESCE(o.correcao_aplicada,''), COALESCE(o.job_do_erro,''), COALESCE(o.revisada_por,''), COALESCE(o.revisada_em,0)
		FROM ocorrencias o WHERE o.id=? AND o.estado!=?`, id, EstadoApagada).
		Scan(&d.ID, &d.CriadaEm, &d.Estado, &d.Categoria, &d.Severidade, &d.Origem, &d.Projeto, &d.Tool, &comp, &user, &d.Domain, &d.ClientIP, &d.Particao, &d.Grupo, &ex, &d.EmbPendente,
			&d.ListRow.Descricao, &d.RespostaCorreta, &d.CorrecaoAplicada, &d.JobDoErro, &d.RevisadaPor, &d.RevisadaEm)
	if err == sql.ErrNoRows {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.Descricao, _ = cutText(d.ListRow.Descricao)
	d.RespostaCorreta, _ = cutText(d.RespostaCorreta)
	d.CorrecaoAplicada, _ = cutText(d.CorrecaoAplicada)
	d.Computer, d.User, d.ExemploPublico = comp, user, ex != 0
	d.Estacao = comp + "/" + user
	if r, err := scanEl(s.db.QueryRowContext(ctx, elSelect+`WHERE o.id=?`, id)); err == nil {
		d.Exportavel, d.MotivoNaoExport = Eligible(r, false)
	}
	d.Jobs, d.Amostras, d.Historico = []DetailJob{}, []DetailSample{}, []DetailHist{}
	jr, err := s.db.QueryContext(ctx, `SELECT job_id, papel, backend, ordem FROM ocorrencia_jobs WHERE ocorrencia_id=? ORDER BY ordem`, id)
	if err != nil {
		return d, err
	}
	for jr.Next() {
		var j DetailJob
		if err := jr.Scan(&j.ID, &j.Papel, &j.Backend, &j.Ordem); err != nil {
			jr.Close()
			return d, err
		}
		d.Jobs = append(d.Jobs, j)
	}
	jr.Close()
	for _, j := range d.Jobs {
		var a DetailSample
		var pt, pjt, img, rf, rt int
		var prompt, resp []byte
		var plen, rlen int
		// the text is cut in SQL (substr on the blob): the 16 MiB of a field are not read to show 256 KiB
		err := s.db.QueryRowContext(ctx, `SELECT job_id, COALESCE(endpoint,''), COALESCE(modelo,''), COALESCE(think,''), COALESCE(status,''), COALESCE(http_status,0),
			prompt_truncado, prompt_json_truncado, tem_imagem, resp_final, resp_truncada,
			COALESCE(substr(prompt_texto,1,?),''), COALESCE(length(prompt_texto),0), COALESCE(substr(resposta,1,?),''), COALESCE(length(resposta),0) FROM amostras WHERE job_id=?`,
			DetailTextMax+4, DetailTextMax+4, j.ID).
			Scan(&a.JobID, &a.Endpoint, &a.Modelo, &a.Think, &a.Status, &a.HTTPStatus, &pt, &pjt, &img, &rf, &rt, &prompt, &plen, &resp, &rlen)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return d, err
		}
		a.PromptTruncado, a.PromptJSONTrunc, a.TemImagem, a.RespFinal, a.RespTruncada = pt != 0, pjt != 0, img != 0, rf != 0, rt != 0
		a.PromptBytes, a.RespBytes = plen, rlen
		a.Prompt, a.PromptCortado = cutShown(prompt, plen)
		a.Resposta, a.RespCortada = cutShown(resp, rlen)
		d.Amostras = append(d.Amostras, a)
	}
	hr, err := s.db.QueryContext(ctx, `SELECT quando, acao, COALESCE(campo,''), COALESCE(antes,''), COALESCE(depois,''), COALESCE(quem,''), COALESCE(ip,'') FROM historico WHERE ocorrencia_id=? ORDER BY id`, id)
	if err != nil {
		return d, err
	}
	defer hr.Close()
	for hr.Next() {
		var h DetailHist
		if err := hr.Scan(&h.Quando, &h.Acao, &h.Campo, &h.Antes, &h.Depois, &h.Quem, &h.IP); err != nil {
			return d, err
		}
		d.Historico = append(d.Historico, h)
	}
	return d, hr.Err()
}

// cutShown is cutText for a prefix read from the database: b holds at most DetailTextMax+4
// bytes of a text whose whole size is total.
func cutShown(b []byte, total int) (string, bool) {
	if total <= DetailTextMax {
		return string(b), false
	}
	end := DetailTextMax
	if end > len(b) {
		end = len(b)
	}
	for end > 0 && end < len(b) && !utf8.RuneStart(b[end]) {
		end--
	}
	return string(b[:end]) + "\n[cortado (" + strconv.Itoa(total) + " bytes)]", true
}
