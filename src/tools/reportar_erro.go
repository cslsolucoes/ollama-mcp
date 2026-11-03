package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/ollama"
)

// reportar_erro (Fase 12.5): the Claude that checked a result of the local model
// and found it wrong records ONE occurrence on the server (POST /api/feedback of
// the gateway). The server keeps the exact request and the wrong answer next to
// the correction; this tool neither sends nor returns any of that content, only
// the job ids of the [uso] line and the Claude's own fields. The rules are the
// contract's (C-1.2, C-1.3, C-1.16, C-1.17) and docs/RELATAR-ERROS-IA.md.

// Limits of a report (C-1.17), checked here BEFORE anything is sent.
const (
	maxReportJobs     = 64
	maxReportDesc     = 4 << 10
	maxReportAnswer   = 128 << 10
	maxReportFix      = 4 << 10
	maxReportProject  = 128
	maxReportBody     = 256 << 10
	maxFeedbackWait   = 10 * time.Second // ceiling of the wait before the single retry after a 409
	defaultFeedbackRA = 5 * time.Second  // used when the 409 carries no Retry-After
)

// Enums (C-1.16). origemTool has no "automatico": that value is only set by the
// server itself for the automatic candidates.
var (
	categoriaEnum  = []string{"numeracao_errada", "contagem_errada", "calculo_errado", "inventou", "omitiu", "formato_invalido", "resumo_infiel", "traducao_errada", "idioma_errado", "truncado", "ignorou_instrucao", "outro"}
	severidadeEnum = []string{"baixa", "media", "alta"}
	origemToolEnum = []string{"claude", "usuario"}
)

var (
	reJobID = regexp.MustCompile(`^[0-9a-f]{32}$`)
	rePapel = regexp.MustCompile(`^(map|reduce)[1-9][0-9]{0,3}$`)
)

func validPapel(p string) bool { return p == "final" || p == "unico" || rePapel.MatchString(p) }

func enumSchema(desc string, values []string) schemaMap {
	vs := make([]any, len(values))
	for i, v := range values {
		vs[i] = v
	}
	m := schemaMap{"type": "string", "enum": vs}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func (d Deps) reportarErroTool() mcp.ToolEntry {
	return mcp.ToolEntry{
		Def: mcp.Tool{
			Name:        "reportar_erro",
			Title:       "Report model error",
			Description: "Records ONE wrong local-model result. Paste its [uso] job=/jobs=, never the request or answers. Rules: docs/RELATAR-ERROS-IA.md.",
			InputSchema: objectSchema([]string{"projeto", "jobs", "categoria", "erro", "severidade", "origem"}, schemaMap{
				"projeto":           schemaMap{"type": "string"},
				"jobs":              schemaMap{"type": "string"},
				"job_do_erro":       stringProp("Role/id; default final."),
				"categoria":         enumSchema("", categoriaEnum),
				"erro":              schemaMap{"type": "string"},
				"resposta_correta":  schemaMap{"type": "string"},
				"correcao_aplicada": schemaMap{"type": "string"},
				"severidade":        enumSchema("", severidadeEnum),
				"origem":            enumSchema("", origemToolEnum),
			}),
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    false,
				DestructiveHint: false,
				IdempotentHint:  false,
				OpenWorldHint:   false,
			},
		},
		Handler: d.reportarErro,
	}
}

// jobEntry is one parsed job of the [uso] line.
type jobEntry = ollama.FeedbackJob

// parseJobsArg accepts the three shapes of the job field: `<id>[@fallback]`
// (single job, role unico), `<role>:<id>[@fallback],...`, each with or without
// the `job=` / `jobs=` prefix, and a whole [uso] line (or any text that contains
// the field), from which the field is taken.
func parseJobsArg(s string) ([]jobEntry, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("`jobs` não pode ser vazio: copie o job= ou o jobs= da linha [uso] da resposta errada")
	}
	field := s
	if fs := strings.Fields(s); len(fs) > 1 || strings.HasPrefix(s, "[uso]") {
		// A text with several fields (a whole [uso] line): take its one job=/jobs=
		// field. Two of them (two lines, or job= and jobs= together) are ambiguous.
		var found []string
		for _, f := range fs {
			if strings.HasPrefix(f, "jobs=") || strings.HasPrefix(f, "job=") {
				found = append(found, f)
			}
		}
		switch len(found) {
		case 0:
			return nil, errors.New("`jobs`: não achei job= nem jobs= no texto; a linha [uso] sem esse campo é de uma resposta sem gateway ou de um servidor antigo, e não pode ser relatada")
		case 1:
			field = found[0]
		default:
			return nil, errors.New("`jobs`: o texto tem mais de um campo job=/jobs= (mais de uma linha [uso]?); cole só o campo da resposta errada")
		}
	}
	field = strings.TrimPrefix(strings.TrimPrefix(field, "jobs="), "job=")
	items := strings.Split(field, ",")
	if len(items) > maxReportJobs {
		return nil, fmt.Errorf("`jobs` tem %d chamadas; o máximo é %d: um pedido com mais chamadas ao modelo não pode ser relatado (avise o usuário e siga o trabalho)", len(items), maxReportJobs)
	}
	out := make([]jobEntry, 0, len(items))
	for i, it := range items {
		it = strings.TrimSpace(it)
		papel, rest := "", it
		if k := strings.Index(it, ":"); k >= 0 {
			papel, rest = it[:k], it[k+1:]
		}
		backend := ollama.BackendPrimary
		if strings.HasSuffix(rest, "@fallback") {
			backend, rest = ollama.BackendFallback, strings.TrimSuffix(rest, "@fallback")
		}
		if !reJobID.MatchString(rest) {
			return nil, fmt.Errorf("`jobs`: o item %d (%q) não tem um id de 32 hexadecimais minúsculos; copie o job=/jobs= da linha [uso] sem editar", i+1, clipText(it, 48))
		}
		if papel == "" {
			if len(items) > 1 {
				return nil, fmt.Errorf("`jobs`: o item %d não traz o papel; cole o campo jobs= da linha [uso] como foi impresso (<papel>:<id>,...)", i+1)
			}
			papel = roleUnico
		}
		if !validPapel(papel) {
			return nil, fmt.Errorf("`jobs`: papel %q inválido no item %d: use map<N>, reduce<N>, final ou unico", clipText(papel, 24), i+1)
		}
		out = append(out, jobEntry{ID: rest, Papel: papel, Backend: backend})
	}
	return out, nil
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

// checkJobList applies the structural rules of the list that the server would
// refuse anyway (C-1.2): no repeated id or role, unico only alone.
func checkJobList(jobs []jobEntry) error {
	ids, roles := map[string]bool{}, map[string]bool{}
	for i, j := range jobs {
		if ids[j.ID] {
			return fmt.Errorf("`jobs`: o id do item %d está repetido", i+1)
		}
		if roles[j.Papel] {
			return fmt.Errorf("`jobs`: o papel %q está repetido", j.Papel)
		}
		ids[j.ID], roles[j.Papel] = true, true
		if j.Papel == roleUnico && len(jobs) > 1 {
			return errors.New("`jobs`: o papel unico só vale para uma lista com um único job")
		}
	}
	return nil
}

// resolveJobDoErro implements C-1.3: the given role or id (which must belong to
// the list), else the final, else the only job; otherwise it is required.
func resolveJobDoErro(jobs []jobEntry, given string) (jobEntry, error) {
	given = strings.TrimSpace(given)
	if given != "" {
		id := strings.TrimSuffix(given, "@fallback")
		for _, j := range jobs {
			if j.Papel == given || j.ID == id {
				return j, nil
			}
		}
		return jobEntry{}, fmt.Errorf("`job_do_erro` %q não pertence à lista `jobs`: use um dos papéis (%s) ou um dos ids", clipText(given, 40), rolesOf(jobs))
	}
	for _, j := range jobs {
		if j.Papel == "final" {
			return j, nil
		}
	}
	if len(jobs) == 1 {
		return jobs[0], nil
	}
	return jobEntry{}, fmt.Errorf("`job_do_erro` é obrigatório quando a lista tem mais de um job e nenhum é o final (por exemplo no translate): indique a parte que errou (%s) e dê a resposta certa daquela chamada", rolesOf(jobs))
}

func rolesOf(jobs []jobEntry) string {
	rs := make([]string, len(jobs))
	for i, j := range jobs {
		rs[i] = j.Papel
	}
	if len(rs) > 8 {
		rs = append(rs[:8], "...")
	}
	return strings.Join(rs, ", ")
}

func inEnum(v string, enum []string) bool {
	for _, e := range enum {
		if v == e {
			return true
		}
	}
	return false
}

// buildReport validates every argument (nothing is sent when it fails) and
// returns the body of the gateway plus the backend of the job in error, which
// decides the server (C-1.11).
func buildReport(a args) (ollama.FeedbackReport, string, error) {
	var rep ollama.FeedbackReport
	projeto, err := a.reqString("projeto")
	if err != nil {
		return rep, "", err
	}
	jobsRaw, err := a.reqString("jobs")
	if err != nil {
		return rep, "", err
	}
	givenErr, err := a.optString("job_do_erro")
	if err != nil {
		return rep, "", err
	}
	categoria, err := a.reqString("categoria")
	if err != nil {
		return rep, "", err
	}
	desc, err := a.reqString("erro")
	if err != nil {
		return rep, "", err
	}
	resposta, err := a.optString("resposta_correta")
	if err != nil {
		return rep, "", err
	}
	fix, err := a.reqString("correcao_aplicada")
	if err != nil {
		return rep, "", err
	}
	sev, err := a.reqString("severidade")
	if err != nil {
		return rep, "", err
	}
	origem, err := a.reqString("origem")
	if err != nil {
		return rep, "", err
	}
	if !inEnum(categoria, categoriaEnum) {
		return rep, "", fmt.Errorf("`categoria` %q inválida: use uma de %s", clipText(categoria, 40), strings.Join(categoriaEnum, ", "))
	}
	if !inEnum(sev, severidadeEnum) {
		return rep, "", fmt.Errorf("`severidade` %q inválida: use %s", clipText(sev, 40), strings.Join(severidadeEnum, ", "))
	}
	if !inEnum(origem, origemToolEnum) {
		return rep, "", fmt.Errorf("`origem` %q inválida: use claude ou usuario (automatico é só do servidor)", clipText(origem, 40))
	}
	for name, v := range map[string]struct {
		s   string
		max int
	}{
		"projeto":           {projeto, maxReportProject},
		"erro":              {desc, maxReportDesc},
		"resposta_correta":  {resposta, maxReportAnswer},
		"correcao_aplicada": {fix, maxReportFix},
	} {
		if len(v.s) > v.max {
			return rep, "", fmt.Errorf("`%s` tem %d bytes; o máximo é %d", name, len(v.s), v.max)
		}
		if strings.ContainsRune(v.s, 0) {
			return rep, "", fmt.Errorf("`%s` contém um caractere nulo", name)
		}
	}
	jobs, err := parseJobsArg(jobsRaw)
	if err != nil {
		return rep, "", err
	}
	if err := checkJobList(jobs); err != nil {
		return rep, "", err
	}
	errJob, err := resolveJobDoErro(jobs, givenErr)
	if err != nil {
		return rep, "", err
	}
	rep = ollama.FeedbackReport{
		Projeto: projeto, Jobs: jobs, JobDoErro: errJob.ID, Categoria: categoria, Descricao: desc,
		RespostaCorreta: resposta, CorrecaoAplicada: fix, Severidade: sev, Origem: origem,
	}
	if b, err := json.Marshal(rep); err == nil && len(b) > maxReportBody {
		return ollama.FeedbackReport{}, "", fmt.Errorf("o relato ocupa %d bytes depois de codificado; o máximo é %d (256 KB): reduza `resposta_correta`", len(b), maxReportBody)
	}
	return rep, errJob.Backend, nil
}

// waitFeedback is the wait before the single retry after a 409: the
// Retry-After of the server, 5 s without it, never above 10 s.
func (d Deps) waitFeedback(ra time.Duration) time.Duration {
	if d.feedbackWait != nil {
		return d.feedbackWait(ra)
	}
	if ra <= 0 {
		ra = defaultFeedbackRA
	}
	return min(ra, maxFeedbackWait)
}

// sendFeedback sends the report and, after a 409 (the job is still queued or
// running), waits Retry-After and tries ONE more time.
func (d Deps) sendFeedback(ctx context.Context, be Backend, rep ollama.FeedbackReport, backend string) (*ollama.FeedbackResult, error) {
	res, err := be.Feedback(ctx, rep, backend)
	var fe *ollama.FeedbackError
	if err == nil || !errors.As(err, &fe) || fe.Status() != 409 {
		return res, err
	}
	t := time.NewTimer(d.waitFeedback(fe.RetryAfter))
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
		return nil, errors.New("chamada cancelada enquanto esperava o job terminar; nada foi gravado")
	}
	return be.Feedback(ctx, rep, backend)
}

// feedbackFailure words a refusal of the server for the Claude (pt-BR). Nothing
// of the request content is echoed.
func feedbackFailure(err error) error {
	var fe *ollama.FeedbackError
	if !errors.As(err, &fe) {
		return err
	}
	detail := ""
	if fe.Detail != "" {
		detail = ": " + fe.Detail
	}
	switch fe.Status() {
	case 400:
		return fmt.Errorf("o servidor recusou o relato (HTTP 400)%s; nada foi gravado", detail)
	case 404:
		return fmt.Errorf("o job do erro não existe no servidor que recebeu o relato (HTTP 404)%s; o job expirou ou é de outro gateway; nada foi gravado", detail)
	case 409:
		return errors.New("o job ainda está na fila ou em execução no servidor (HTTP 409), mesmo depois de esperar; tente relatar de novo mais tarde; nada foi gravado")
	case 503:
		return fmt.Errorf("o registro de erros do servidor está desligado ou ocupado (HTTP 503)%s; tente mais tarde ou avise o administrador; nada foi gravado", detail)
	case 507:
		return fmt.Errorf("o banco de erros do servidor está cheio (HTTP 507)%s; avise o administrador (nada foi apagado nem gravado)", detail)
	}
	return err
}

func (d Deps) reportarErro(ctx context.Context, raw json.RawMessage) (mcp.ToolCallResult, error) {
	a, err := parseArgs(raw, "projeto", "jobs", "job_do_erro", "categoria", "erro", "resposta_correta", "correcao_aplicada", "severidade", "origem")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	rep, backend, err := buildReport(a)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	be, err := d.backend()
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	res, err := d.sendFeedback(ctx, be, rep, backend)
	if err != nil {
		return mcp.ToolCallResult{}, feedbackFailure(err)
	}
	out := mcp.TextResult(feedbackSummary(res, len(rep.Jobs)), false)
	out.Meta = map[string]any{"mcpollama/reportar_erro": map[string]any{"ocorrencia_id": res.OcorrenciaID, "estado": res.Estado}}
	return out, nil
}

// feedbackSummary is the answer of the tool: the occurrence number, the state
// and what the state means, plus how many jobs stayed as references only.
func feedbackSummary(r *ollama.FeedbackResult, njobs int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Erro registrado: ocorrência %s, estado %s.", r.OcorrenciaID, r.Estado)
	switch r.Estado {
	case "nova":
		b.WriteString(" É um candidato: só serve para exemplos e treino depois que o administrador conferir a correção e confirmar.")
	case "sem_pedido":
		b.WriteString(" O conteúdo do pedido desse job expirou do servidor ou não é desta estação: o registro conta só na estatística e não entra no treino.")
	case "distribuida":
		b.WriteString(" Os jobs rodaram em servidores diferentes: o registro conta só na estatística e não entra no treino.")
	case "fallback":
		b.WriteString(" O job foi atendido pelo servidor de fallback: o registro conta só na estatística e não entra no treino.")
	}
	fmt.Fprintf(&b, "\nJobs relatados: %d; amostras copiadas: %d.", njobs, r.AmostrasCopiadas)
	if r.JobsExternos > 0 {
		fmt.Fprintf(&b, " %d job(s) de outro servidor ficaram só como referência.", r.JobsExternos)
	}
	if r.JobsOutraOrigem > 0 {
		fmt.Fprintf(&b, " %d job(s) de outra estação ficaram só como referência.", r.JobsOutraOrigem)
	}
	if r.SemPedido > 0 && r.Estado != "sem_pedido" {
		fmt.Fprintf(&b, " %d job(s) sem conteúdo guardado (expirado) ficaram só como referência.", r.SemPedido)
	}
	b.WriteString("\nCite o número da ocorrência no seu relato ao usuário.")
	return b.String()
}
