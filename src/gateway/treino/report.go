package treino

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// JobRef is one job of a report: the ID of the job (the X-Request-Id), its role
// in the occurrence and the backend that ran it (C-1.2).
type JobRef struct {
	ID      string `json:"id"`
	Papel   string `json:"papel"`
	Backend string `json:"backend"`
}

// Report is the body of POST /api/feedback. Unknown fields are refused.
type Report struct {
	Projeto          string   `json:"projeto"`
	Jobs             []JobRef `json:"jobs"`
	JobDoErro        string   `json:"job_do_erro"`
	Categoria        string   `json:"categoria"`
	Descricao        string   `json:"descricao"`
	RespostaCorreta  string   `json:"resposta_correta"`
	CorrecaoAplicada string   `json:"correcao_aplicada"`
	Severidade       string   `json:"severidade"`
	Origem           string   `json:"origem"`
}

var (
	jobIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)
	papelRe = regexp.MustCompile(`^(map|reduce)[1-9][0-9]{0,3}$`)
)

var (
	categorias = map[string]bool{"numeracao_errada": true, "contagem_errada": true, "calculo_errado": true, "inventou": true,
		"omitiu": true, "formato_invalido": true, "resumo_infiel": true, "traducao_errada": true, "idioma_errado": true,
		"truncado": true, "ignorou_instrucao": true, "outro": true}
	origens     = map[string]bool{"claude": true, "usuario": true, "automatico": true}
	severidades = map[string]bool{"baixa": true, "media": true, "alta": true}
)

// ValidationError is a refusal of the content of a report (HTTP 400): the
// message names the field and the rule.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func bad(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

// ValidPapel reports whether p is a role of C-1.2: map<N>, reduce<N>, final or unico.
func ValidPapel(p string) bool { return p == "final" || p == "unico" || papelRe.MatchString(p) }

// Validate applies C-1.3, C-1.16 and C-1.17 and fills JobDoErro with its default
// when the report left it empty. It changes nothing when it fails.
func (r *Report) Validate() error {
	if !categorias[r.Categoria] {
		return bad("categoria %q inválida: use uma de numeracao_errada, contagem_errada, calculo_errado, inventou, omitiu, formato_invalido, resumo_infiel, traducao_errada, idioma_errado, truncado, ignorou_instrucao, outro", clip(r.Categoria))
	}
	if !origens[r.Origem] {
		return bad("origem %q inválida: use claude, usuario ou automatico", clip(r.Origem))
	}
	if !severidades[r.Severidade] {
		return bad("severidade %q inválida: use baixa, media ou alta", clip(r.Severidade))
	}
	if strings.TrimSpace(r.Descricao) == "" {
		return bad("descricao é obrigatória")
	}
	if len(r.Descricao) > MaxDescricao {
		return bad("descricao tem %d bytes; o máximo é %d (4 KB)", len(r.Descricao), MaxDescricao)
	}
	if len(r.RespostaCorreta) > MaxResposta {
		return bad("resposta_correta tem %d bytes; o máximo é %d (128 KB)", len(r.RespostaCorreta), MaxResposta)
	}
	if len(r.CorrecaoAplicada) > MaxCorrecao {
		return bad("correcao_aplicada tem %d bytes; o máximo é %d (4 KB)", len(r.CorrecaoAplicada), MaxCorrecao)
	}
	if len(r.Projeto) > MaxProjeto {
		return bad("projeto tem %d bytes; o máximo é %d", len(r.Projeto), MaxProjeto)
	}
	for name, v := range map[string]string{"projeto": r.Projeto, "descricao": r.Descricao, "resposta_correta": r.RespostaCorreta, "correcao_aplicada": r.CorrecaoAplicada} {
		if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
			return bad("%s não é texto UTF-8 válido", name)
		}
	}
	if len(r.Jobs) == 0 {
		return bad("jobs é obrigatório: informe de 1 a %d jobs (o job= ou a lista jobs= da linha [uso])", MaxJobs)
	}
	if len(r.Jobs) > MaxJobs {
		return bad("jobs tem %d itens; o máximo é %d: um pedido com mais chamadas não pode ser relatado", len(r.Jobs), MaxJobs)
	}
	seenID, seenPapel := map[string]bool{}, map[string]bool{}
	finals, unicos := 0, 0
	for i, j := range r.Jobs {
		if !jobIDRe.MatchString(j.ID) {
			return bad("jobs[%d].id deve ter 32 hexadecimais minúsculos", i)
		}
		if !ValidPapel(j.Papel) {
			return bad("jobs[%d].papel %q inválido: use map<N>, reduce<N>, final ou unico", i, clip(j.Papel))
		}
		if j.Backend != RolePrimario && j.Backend != RoleFallback {
			return bad("jobs[%d].backend %q inválido: use primario ou fallback", i, clip(j.Backend))
		}
		if seenID[j.ID] {
			return bad("jobs[%d].id repetido", i)
		}
		if seenPapel[j.Papel] {
			return bad("jobs[%d].papel %q repetido", i, j.Papel)
		}
		seenID[j.ID], seenPapel[j.Papel] = true, true
		switch j.Papel {
		case "final":
			finals++
		case "unico":
			unicos++
		}
	}
	if unicos > 0 && len(r.Jobs) > 1 {
		return bad("o papel unico só vale para uma lista com um único job")
	}
	if r.JobDoErro != "" {
		if !jobIDRe.MatchString(r.JobDoErro) {
			return bad("job_do_erro deve ter 32 hexadecimais minúsculos")
		}
		if !seenID[r.JobDoErro] {
			return bad("job_do_erro não pertence à lista de jobs")
		}
		return nil
	}
	// Default (C-1.3): the final; with a single job, that job. A list of several
	// jobs without a final (translate, for one) must say which job made the error.
	for _, j := range r.Jobs {
		if j.Papel == "final" {
			r.JobDoErro = j.ID
			return nil
		}
	}
	if len(r.Jobs) == 1 {
		r.JobDoErro = r.Jobs[0].ID
		return nil
	}
	return bad("job_do_erro é obrigatório quando a lista tem mais de um job e nenhum é o final (por exemplo no translate): indique a parte (mapN) que errou")
}

// clip keeps an echoed value short in a message.
func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}
