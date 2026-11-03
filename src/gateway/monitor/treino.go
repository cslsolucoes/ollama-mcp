package monitor

// The "Erros (treino)" board and the administrator routes of Fase 12.6.
//
// Open to every visitor (like the rest of the monitor): only COUNTS and the use
// of the ceiling, never a text. Administrator only (the session, plus the CSRF
// token and the same-site check for every POST): the list, the detail, with the
// text of the request and of the answer, the export and the actions. Every read
// uses a context of its own with a timeout, never the visitor's, and the open
// summary comes from a background cache (swr.go), so no visitor waits for the
// database holding a lock (C-6.10, pattern of 0.12.6).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

const (
	treinoSumTTL  = 2 * time.Second
	treinoWriteTO = 30 * time.Second
)

// treinoExportMax is how long an export may run; a var so a test can shorten it.
var treinoExportMax = 30 * time.Minute

type treinoSum struct {
	sum treino.Summary
	ok  bool
}

// treinoSummary is the open summary, from the cache; nil when the feature is off.
func (h *Handler) treinoSummary() *treino.Summary {
	if h.cfg.Treino == nil {
		return nil
	}
	v := h.treinoSum.get(h.cfg.Now, treinoSumTTL, extrasFirstWait, func() (treinoSum, bool) {
		sm, err := h.cfg.Treino.Summary(context.Background()) // no Done channel (pattern of 0.12.6)
		if err != nil {
			h.logf("monitor: treino: %v", err)
			return treinoSum{}, false
		}
		return treinoSum{sum: sm, ok: true}, true
	})
	if !v.ok {
		return nil
	}
	sm := v.sum
	return &sm
}

func (h *Handler) treinoOff(w http.ResponseWriter) bool {
	if h.cfg.Treino == nil {
		jsonErr(w, http.StatusServiceUnavailable, "registro de erros desligado neste gateway (--treino-db)")
		return true
	}
	return false
}

// GET /monitor/api/treino: the counts for everybody; the list, for the administrator.
func (h *Handler) apiTreino(w http.ResponseWriter, r *http.Request) {
	if h.treinoOff(w) {
		return
	}
	out := map[string]any{"admin": false}
	if !h.adminRead(r) {
		out["summary"] = h.treinoSummary()
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx := context.Background() // reads never carry a Done channel (0.12.6); the page is bounded by LIMIT
	q := r.URL.Query()
	f := treino.Filter{Categoria: q.Get("categoria"), Tool: q.Get("tool"), Estado: q.Get("estado"), Particao: q.Get("particao"), Estacao: q.Get("estacao"),
		Limit: atoiDef(q.Get("limite"), 100), Offset: atoiDef(q.Get("offset"), 0)}
	desde, e1 := parseTime(q.Get("desde"))
	ate, e2 := parseTime(q.Get("ate"))
	if e1 != nil || e2 != nil {
		jsonErr(w, http.StatusBadRequest, "desde/ate: use RFC 3339 ou epoch em ms")
		return
	}
	if !desde.IsZero() {
		f.Desde = desde.UnixMilli()
	}
	if !ate.IsZero() {
		f.Ate = ate.UnixMilli()
	}
	rows, err := h.cfg.Treino.List(ctx, f)
	if err != nil {
		h.logf("monitor: treino list: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar as ocorrências")
		return
	}
	sm, err := h.cfg.Treino.Summary(ctx)
	if err != nil {
		h.logf("monitor: treino summary: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar as contagens")
		return
	}
	out["admin"], out["rows"], out["summary"] = true, rows, sm
	writeJSON(w, http.StatusOK, out)
}

// GET /monitor/api/treino/{id}: the whole occurrence, text included; administrator only.
func (h *Handler) apiTreinoDetail(w http.ResponseWriter, r *http.Request) {
	if h.treinoOff(w) {
		return
	}
	if !h.requireSessionMsg(w, r, "o detalhe das ocorrências só é mostrado ao administrador logado") {
		return
	}
	id := r.PathValue("id")
	if !idRe.MatchString(id) {
		jsonErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	d, err := h.cfg.Treino.Detail(context.Background(), id) // the text of each field is cut by Detail
	if err != nil {
		if errors.Is(err, treino.ErrNotFound) {
			jsonErr(w, http.StatusNotFound, "ocorrência não encontrada")
			return
		}
		h.logf("monitor: treino detail: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar a ocorrência")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// requireSessionMsg is requireSession with its own 401 message.
func (h *Handler) requireSessionMsg(w http.ResponseWriter, r *http.Request, msg string) bool {
	if !sameSite(r) {
		jsonErr(w, http.StatusForbidden, "origem não permitida")
		return false
	}
	if _, s := h.current(r); s != nil {
		return true
	}
	jsonErr(w, http.StatusUnauthorized, msg)
	return false
}

// GET /monitor/api/treino/export?parte=treino|teste: JSONL in streaming (C-3.12).
// Session plus same-site, checked before the first byte. Nothing is kept: lines go
// to the client as they are made, and the history gets one `exportou` row per
// exported occurrence.
func (h *Handler) apiTreinoExport(w http.ResponseWriter, r *http.Request) {
	if h.treinoOff(w) {
		return
	}
	if !h.requireSessionMsg(w, r, "a exportação só é permitida ao administrador logado") {
		return
	}
	parte := r.URL.Query().Get("parte")
	if parte != treino.ParteTreino && parte != treino.ParteTeste {
		jsonErr(w, http.StatusBadRequest, "parte: treino ou teste")
		return
	}
	hd := w.Header()
	hd.Set("Cache-Control", "no-store")
	hd.Set("Content-Type", "application/x-ndjson; charset=utf-8")
	hd.Set("Content-Disposition", `attachment; filename="erros-`+parte+"-"+time.Now().Format("20060102-150405")+`.jsonl"`)
	fl, _ := w.(http.Flusher)
	flush := func() {
		if fl != nil {
			fl.Flush()
		}
	}
	// A context without Done (0.12.6); the deadline is checked between the lines instead.
	if _, err := h.cfg.Treino.ExportUntil(context.Background(), parte, treino.AdminActor(proxy.ClientIP(r)), w, flush, time.Now().Add(treinoExportMax)); err != nil {
		h.logf("monitor: treino export: %v", err) // the headers are gone: the cut file is the signal
	}
}

// treinoBody is what the action routes accept.
type treinoBody struct {
	RespostaCorreta  *string `json:"resposta_correta"`
	Descricao        *string `json:"descricao"`
	CorrecaoAplicada *string `json:"correcao_aplicada"`
	Marcar           *bool   `json:"marcar"`
	Lado             string  `json:"lado"`
}

// readJSONLimit reads a JSON body of at most max bytes (the answer of a report
// is up to 128 KB, so the 4 KB of readSmallJSON does not do).
func readJSONLimit(w http.ResponseWriter, r *http.Request, max int64, v any) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		jsonErr(w, http.StatusRequestEntityTooLarge, "corpo grande demais")
		return false
	}
	if len(b) == 0 {
		return true
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		jsonErr(w, http.StatusUnsupportedMediaType, "use Content-Type: application/json")
		return false
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		jsonErr(w, http.StatusBadRequest, "JSON inválido ou com campo desconhecido")
		return false
	}
	return true
}

func (h *Handler) treinoError(w http.ResponseWriter, err error) {
	var ve *treino.ValidationError
	switch {
	case errors.Is(err, treino.ErrNotFound), errors.Is(err, treino.ErrJobNotFound):
		jsonErr(w, http.StatusNotFound, "não encontrado")
	case errors.Is(err, treino.ErrTransition):
		jsonErr(w, http.StatusConflict, "ação não permitida neste estado")
	case errors.Is(err, treino.ErrNeedsAnswer):
		jsonErr(w, http.StatusUnprocessableEntity, "confirmar exige a resposta correta (resposta_correta não vazia)")
	case errors.Is(err, treino.ErrInvalid), errors.As(err, &ve):
		jsonErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, treino.ErrJobBusy):
		w.Header().Set("Retry-After", "5")
		jsonErr(w, http.StatusConflict, "o job ainda está na fila ou em execução")
	case errors.Is(err, treino.ErrCapReached):
		jsonErr(w, http.StatusInsufficientStorage, "treino.db no teto: nada foi gravado")
	case errors.Is(err, treino.ErrBusy), errors.Is(err, context.DeadlineExceeded):
		w.Header().Set("Retry-After", "5")
		jsonErr(w, http.StatusServiceUnavailable, "treino.db ocupado: tente de novo")
	default:
		h.logf("monitor: treino: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro")
	}
}

// treinoAction builds the handler of one administrator action on an occurrence
// (C-1.13): session, CSRF token and same-site (authorize); the change and its
// history row go in one transaction (treino.Store).
func (h *Handler) treinoAction(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.treinoOff(w) {
			return
		}
		_, _, ok := h.authorize(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !idRe.MatchString(id) {
			jsonErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		var in treinoBody
		if !readJSONLimit(w, r, treino.MaxBodyBytes, &in) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), treinoWriteTO)
		defer cancel()
		a := treino.AdminActor(proxy.ClientIP(r))
		st := h.cfg.Treino
		out := map[string]any{"ok": true, "acao": kind, "id": id}
		var err error
		switch kind {
		case "confirmar":
			err = st.Confirm(ctx, id, a)
		case "descartar":
			err = st.Discard(ctx, id, a)
		case "completar":
			if in.RespostaCorreta == nil && in.Descricao == nil && in.CorrecaoAplicada == nil {
				jsonErr(w, http.StatusBadRequest, "informe resposta_correta, descricao ou correcao_aplicada")
				return
			}
			var des bool
			des, err = st.Update(ctx, id, treino.Edit{RespostaCorreta: in.RespostaCorreta, Descricao: in.Descricao, CorrecaoAplicada: in.CorrecaoAplicada}, a)
			out["desconfirmou"] = des
		case "apagar":
			err = st.Delete(ctx, id, a)
		case "exemplo-publico":
			if in.Marcar == nil {
				jsonErr(w, http.StatusBadRequest, "informe marcar: true ou false")
				return
			}
			err = st.SetExemploPublico(ctx, id, *in.Marcar, a)
		case "liberar-quarentena":
			var info treino.ReleaseInfo
			info, err = st.ReleaseQuarantineInfo(ctx, id, in.Lado, a)
			out["membros"], out["exportados_na_outra_parte"] = info.Membros, info.ExportadosNaOutraParte
			if info.ExportadosNaOutraParte > 0 {
				out["aviso"] = "atenção: " + strconv.Itoa(info.ExportadosNaOutraParte) + " ocorrência(s) deste componente já foi(ram) exportada(s) para a OUTRA parte; o mesmo caso agora está nos dois arquivos baixados"
			}
		}
		if err != nil {
			h.treinoError(w, err)
			return
		}
		h.cfg.Hub.Notify()
		writeJSON(w, http.StatusOK, out)
	}
}

// markBody is the "Marcar erro" form of the job detail.
type markBody struct {
	Categoria        string `json:"categoria"`
	Severidade       string `json:"severidade"`
	Descricao        string `json:"descricao"`
	RespostaCorreta  string `json:"resposta_correta"`
	CorrecaoAplicada string `json:"correcao_aplicada"`
	Projeto          string `json:"projeto"`
}

// POST /monitor/api/jobs/{id}/marcar-erro: the administrator reports a job as an
// error of the model (D12-6). origem=usuario, quem=admin, and no same-origin
// restriction (C-1.11a): the administrator may report any job of this gateway.
func (h *Handler) apiMarkError(w http.ResponseWriter, r *http.Request) {
	if h.treinoOff(w) {
		return
	}
	_, _, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if h.cfg.TreinoSource == nil || !h.cfg.TreinoKey.Configured() {
		jsonErr(w, http.StatusServiceUnavailable, "este gateway não aceita relatos: falta --doc-key-file")
		return
	}
	id := r.PathValue("id")
	if !idRe.MatchString(id) {
		jsonErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	var in markBody
	if !readJSONLimit(w, r, treino.MaxBodyBytes, &in) {
		return
	}
	backend := treino.RolePrimario
	if h.cfg.Role == treino.RoleFallback {
		backend = treino.RoleFallback
	}
	// The papel is "unico" because the gateway does not know the role the client gave this job in a
	// larger request (map, reduce, final): only the station that made the call does (M-4 of the
	// review of 12.6). A report with the whole list of jobs goes through POST /api/feedback.
	rep := treino.Report{Projeto: in.Projeto, Jobs: []treino.JobRef{{ID: id, Papel: "unico", Backend: backend}}, Categoria: in.Categoria, Descricao: in.Descricao,
		RespostaCorreta: in.RespostaCorreta, CorrecaoAplicada: in.CorrecaoAplicada, Severidade: in.Severidade, Origem: "usuario"}
	if err := rep.Validate(); err != nil {
		h.treinoError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), treinoWriteTO)
	defer cancel()
	res, err := h.cfg.Treino.IngestWith(ctx, rep, treino.Origin{ClientIP: proxy.ClientIP(r), ClientLabel: "admin"}, h.cfg.Role, h.cfg.TreinoSource, h.cfg.TreinoKey, treino.IngestOptions{Admin: true})
	if err != nil {
		h.treinoError(w, err)
		return
	}
	h.cfg.Hub.Notify()
	writeJSON(w, http.StatusOK, res)
}
