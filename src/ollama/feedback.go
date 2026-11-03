package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// POST /api/feedback is the route of the gateway that records the errors of the
// model (Fase 12, contract C-1.x). The body is the gateway's `treino.Report`;
// the field names below are the wire format, kept in step by a test that
// decodes what is sent with the gateway's own strict decoder.
const pathFeedback = "/api/feedback"

// Backends a job can have run on (contract C-1.2).
const (
	BackendPrimary  = "primario"
	BackendFallback = "fallback"
)

// FeedbackJob is one job of a report: the gateway job id, its role in the
// request (unico, mapN, reduceN, final) and the backend that ran it.
type FeedbackJob struct {
	ID      string `json:"id"`
	Papel   string `json:"papel"`
	Backend string `json:"backend"`
}

// FeedbackReport is the body of POST /api/feedback.
type FeedbackReport struct {
	Projeto          string        `json:"projeto"`
	Jobs             []FeedbackJob `json:"jobs"`
	JobDoErro        string        `json:"job_do_erro"`
	Categoria        string        `json:"categoria"`
	Descricao        string        `json:"descricao"`
	RespostaCorreta  string        `json:"resposta_correta"`
	CorrecaoAplicada string        `json:"correcao_aplicada"`
	Severidade       string        `json:"severidade"`
	Origem           string        `json:"origem"`
}

// FeedbackResult is the answer of a recorded report.
type FeedbackResult struct {
	OcorrenciaID     string `json:"ocorrencia_id"`
	Estado           string `json:"estado"`
	AmostrasCopiadas int    `json:"amostras_copiadas"`
	SemPedido        int    `json:"sem_pedido"`
	JobsExternos     int    `json:"jobs_externos"`
	JobsOutraOrigem  int    `json:"jobs_outra_origem"`
}

// FeedbackError is a refusal of the server (any non-200 status) or a failure of
// the exchange. Status is 0 for a failure before an HTTP answer. Detail is the
// server's own message, scrubbed and short; RetryAfter is the Retry-After
// header of a 409 or 503 (0 when absent).
type FeedbackError struct {
	Err        *Error
	Detail     string
	RetryAfter time.Duration
}

func (e *FeedbackError) Error() string { return e.Err.Msg }
func (e *FeedbackError) Unwrap() error { return e.Err }

// Status is the HTTP status of the refusal, 0 when there was none.
func (e *FeedbackError) Status() int { return e.Err.Status }

// Feedback sends one report to the server that ran the job in error. backend is
// "primario" (this client's server) or "fallback" (the fallback server, with its
// own token and CA). It never retries and never falls back to the other
// server: a report for a job of one gateway would be refused by the other, and
// its token must not travel there. Without a configured fallback a report for a
// fallback job fails before any request.
func (c *Client) Feedback(ctx context.Context, rep FeedbackReport, backend string) (*FeedbackResult, error) {
	target := c
	if backend == BackendFallback {
		if c.fb == nil {
			return nil, &FeedbackError{Err: c.fail(ErrBadRequest, 0,
				"o job do erro foi atendido pelo servidor de fallback, mas nenhum fallback está configurado (--ollama-url-fallback); nada foi enviado")}
		}
		target = c.fb
	} else if backend != BackendPrimary {
		return nil, &FeedbackError{Err: c.fail(ErrBadRequest, 0, "backend %q inválido: use primario ou fallback", backend)}
	}
	return target.postFeedback(ctx, rep)
}

// feedbackStates are the states of an occurrence (contract C-1.8).
var feedbackStates = map[string]bool{"nova": true, "confirmada": true, "descartada": true, "sem_pedido": true, "distribuida": true, "fallback": true, "apagada": true}

// valid tells whether the answer has the shape the gateway promises: a 32
// lowercase hex id, a known state and non-negative counters. Anything else is
// refused without echoing it.
func (r FeedbackResult) valid() bool {
	return validJobID(r.OcorrenciaID) && feedbackStates[r.Estado] &&
		r.AmostrasCopiadas >= 0 && r.SemPedido >= 0 && r.JobsExternos >= 0 && r.JobsOutraOrigem >= 0
}

func (c *Client) postFeedback(ctx context.Context, rep FeedbackReport) (*FeedbackResult, error) {
	b, err := json.Marshal(rep)
	if err != nil {
		return nil, &FeedbackError{Err: c.fail(ErrBadRequest, 0, "não foi possível montar o relato: %v", err)}
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u := *c.base
	u.Path += pathFeedback
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(b))
	if err != nil {
		return nil, &FeedbackError{Err: c.fail(ErrBadRequest, 0, "requisição inválida: %v", err)}
	}
	c.setHeaders(ctx, req, true)
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.debugf("POST %s failed after %dms", pathFeedback, time.Since(start).Milliseconds())
		return nil, &FeedbackError{Err: c.transportErr(ctx, err, start, false, 0)}
	}
	defer resp.Body.Close()
	c.debugf("POST %s status=%d after=%dms", pathFeedback, resp.StatusCode, time.Since(start).Milliseconds())
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBody)+1))
	if err != nil {
		return nil, &FeedbackError{Err: c.transportErr(ctx, err, start, true, len(body))}
	}
	if resp.StatusCode != http.StatusOK {
		fe := &FeedbackError{Detail: c.detail(body)}
		if n, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && n > 0 {
			fe.RetryAfter = time.Duration(n) * time.Second
		}
		fe.Err = c.statusError(http.MethodPost, pathFeedback, resp.StatusCode, body)
		return nil, fe
	}
	if len(body) > c.maxBody {
		return nil, &FeedbackError{Err: c.fail(ErrBadResponse, 0, "resposta acima do limite de %d bytes", c.maxBody)}
	}
	var out FeedbackResult
	if err := json.Unmarshal(body, &out); err != nil || !out.valid() {
		return nil, &FeedbackError{Err: c.fail(ErrBadResponse, 0, "resposta inválida do servidor em %s", pathFeedback)}
	}
	return &out, nil
}
