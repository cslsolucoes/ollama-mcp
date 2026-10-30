package monitor

// The job log API (Fase 10.L). Metadata and the per-second GPU extract are open
// like every GET of the monitor (the LAN restriction is the nginx's job). The
// TEXT of the jobs (prompt and answer) is served only to the logged-in
// administrator: a GET needs the session cookie (no CSRF token, it changes
// nothing); without it the answer is 401.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// liveJobsLimit is the number of jobs the live stream carries.
const liveJobsLimit = 200

const (
	liveJobsTTL   = 500 * time.Millisecond
	jobsCacheTTL  = time.Second
	jobsCacheMax  = 32
	exportPage    = 500
	exportMaxRows = 100000
	sampleLimit   = 20000
)

type jobsCacheEntry struct {
	at   time.Time
	rows []store.JobRow
}

type jobsCache struct {
	mu   sync.Mutex
	live struct {
		at   time.Time
		rows []store.JobRow
	}
	done struct {
		at   time.Time
		rows []store.JobRow
	}
	byQuery map[string]jobsCacheEntry
}

// liveJobs is the list the event stream carries, recomputed at most every
// liveJobsTTL however many panels are open.
func (h *Handler) liveJobs(ctx context.Context) []store.JobRow {
	if h.cfg.Store == nil {
		return []store.JobRow{}
	}
	c := &h.jobs
	c.mu.Lock()
	defer c.mu.Unlock()
	if h.cfg.Now().Sub(c.live.at) < liveJobsTTL && c.live.rows != nil {
		return c.live.rows
	}
	rows, err := h.cfg.Store.Jobs(ctx, store.JobQuery{Limit: liveJobsLimit})
	if err != nil {
		h.logf("monitor: jobs: %v", err)
		if c.live.rows != nil {
			return c.live.rows
		}
		return []store.JobRow{}
	}
	c.live.at, c.live.rows = h.cfg.Now(), rows
	return rows
}

// recentDone lists the requests that ended within KeepDone (cached like the live
// list). It is a view of the job log: the queue does not know about it.
func (h *Handler) recentDone(ctx context.Context) []store.JobRow {
	if h.cfg.Store == nil || h.cfg.KeepDone <= 0 {
		return []store.JobRow{}
	}
	c := &h.jobs
	c.mu.Lock()
	defer c.mu.Unlock()
	now := h.cfg.Now()
	if now.Sub(c.done.at) < liveJobsTTL && c.done.rows != nil {
		return c.done.rows
	}
	rows, err := h.cfg.Store.Jobs(ctx, store.JobQuery{FinishedSince: now.Add(-h.cfg.KeepDone), Limit: liveJobsLimit})
	if err != nil {
		h.logf("monitor: concluídos recentes: %v", err)
		if c.done.rows != nil {
			return c.done.rows
		}
		return []store.JobRow{}
	}
	// ordered by when they ended, newest first
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].FinishedAt > rows[j].FinishedAt })
	c.done.at, c.done.rows = now, rows
	return rows
}

func jobQueryFrom(r *http.Request) (store.JobQuery, bool) {
	q := r.URL.Query()
	since, err1 := parseTime(q.Get("desde"))
	until, err2 := parseTime(q.Get("ate"))
	if err1 != nil || err2 != nil {
		return store.JobQuery{}, false
	}
	jq := store.JobQuery{Since: since, Until: until, Client: q.Get("cliente"),
		Limit: atoiDef(q.Get("limite"), 200), Offset: atoiDef(q.Get("offset"), 0)}
	for _, s := range strings.Split(q.Get("status"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			jq.Status = append(jq.Status, s)
		}
	}
	return jq, true
}

func (h *Handler) apiJobs(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Store == nil {
		jsonErr(w, http.StatusServiceUnavailable, "histórico desabilitado")
		return
	}
	jq, ok := jobQueryFrom(r)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "desde/ate: use RFC 3339 ou epoch em ms")
		return
	}
	key := r.URL.RawQuery
	c := &h.jobs
	c.mu.Lock()
	if e, hit := c.byQuery[key]; hit && time.Since(e.at) < jobsCacheTTL {
		c.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"rows": e.rows})
		return
	}
	c.mu.Unlock()
	rows, err := h.cfg.Store.Jobs(r.Context(), jq)
	if err != nil {
		h.logf("monitor: jobs: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar o log de jobs")
		return
	}
	c.mu.Lock()
	if c.byQuery == nil || len(c.byQuery) >= jobsCacheMax {
		c.byQuery = map[string]jobsCacheEntry{}
	}
	c.byQuery[key] = jobsCacheEntry{at: time.Now(), rows: rows}
	c.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

func (h *Handler) apiJob(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Store == nil {
		jsonErr(w, http.StatusServiceUnavailable, "histórico desabilitado")
		return
	}
	id := r.PathValue("id")
	if !idRe.MatchString(id) {
		jsonErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	job, ok, err := h.cfg.Store.Job(r.Context(), id)
	if err != nil {
		h.logf("monitor: job: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar o job")
		return
	}
	if !ok {
		jsonErr(w, http.StatusNotFound, "job não encontrado")
		return
	}
	samples, err := h.cfg.Store.JobSamplesOf(r.Context(), id, sampleLimit)
	if err != nil {
		h.logf("monitor: job samples: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar o extrato")
		return
	}
	out := map[string]any{"job": job, "samples": samples, "content_enabled": h.cfg.Content != nil}
	if h.cfg.Content != nil {
		if in, found, cerr := h.cfg.Content.Info(r.Context(), id); cerr == nil && found {
			out["content"] = in // sizes only; the text has its own endpoint
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// requireSession answers 401 unless the request carries the administrator's
// session.
func (h *Handler) requireSession(w http.ResponseWriter, r *http.Request) bool {
	if _, s := h.current(r); s != nil {
		return true
	}
	jsonErr(w, http.StatusUnauthorized, "o conteúdo dos jobs só é mostrado ao administrador logado")
	return false
}

func (h *Handler) apiJobContent(w http.ResponseWriter, r *http.Request) {
	if !h.requireSession(w, r) {
		return
	}
	if h.cfg.Content == nil {
		jsonErr(w, http.StatusNotFound, "o conteúdo não é gravado (--log-content=false)")
		return
	}
	id := r.PathValue("id")
	if !idRe.MatchString(id) {
		jsonErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	c, ok, err := h.cfg.Content.Get(r.Context(), id)
	if err != nil {
		h.logf("monitor: content: %v", err)
		jsonErr(w, http.StatusInternalServerError, "erro ao consultar o conteúdo")
		return
	}
	if !ok {
		jsonErr(w, http.StatusNotFound, "sem conteúdo para este job (apagado pela retenção ou pelo teto)")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// csvSafe defuses spreadsheet formulas: a cell starting with = + - @ or a
// control character is read by Excel as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func isoMs(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format(time.RFC3339)
}

var exportHeader = []string{"id", "entrada", "inicio", "fim", "status", "http", "classe_erro", "cliente", "ip", "tool", "endpoint", "modelo",
	"raciocinio", "faixa", "prioridade", "backend", "bytes_enviados", "espera_ms", "execucao_ms", "tokens_prompt", "tokens_gerados",
	"houve_resposta", "bytes_resposta", "pico_w", "pico_clock_mhz", "segundos_de_extrato", "boot_id"}

func exportCells(j store.JobRow) []string {
	i := func(n int64) string { return strconv.FormatInt(n, 10) }
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
	yes := "nao"
	if j.Responded {
		yes = "sim"
	}
	return []string{j.ID, isoMs(j.EnqueuedAt), isoMs(j.StartedAt), isoMs(j.FinishedAt), csvSafe(j.Status), strconv.Itoa(j.HTTPStatus), csvSafe(j.ErrorClass),
		csvSafe(j.ClientLabel), j.ClientIP, csvSafe(j.Tool), j.Endpoint, csvSafe(j.Model), j.Think, j.Lane, j.PriorityFinal, csvSafe(j.Backend),
		i(j.BytesIn), i(j.WaitMS), i(j.RunMS), i(j.PromptEval), i(j.Eval), yes, i(j.RespBytes), f(j.PeakPowerW), f(j.PeakClockMHz), strconv.Itoa(j.Samples), j.BootID}
}

// apiJobsExport streams the job log as JSONL or CSV. Metadata only, unless
// conteudo=1: then it needs the administrator session, checked before the first
// byte is written.
func (h *Handler) apiJobsExport(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Store == nil {
		jsonErr(w, http.StatusServiceUnavailable, "histórico desabilitado")
		return
	}
	q := r.URL.Query()
	withContent := q.Get("conteudo") == "1"
	if withContent {
		if !h.requireSession(w, r) {
			return
		}
		if h.cfg.Content == nil {
			jsonErr(w, http.StatusNotFound, "o conteúdo não é gravado (--log-content=false)")
			return
		}
	}
	format := q.Get("formato")
	if format == "" {
		format = "jsonl"
	}
	if format != "jsonl" && format != "csv" {
		jsonErr(w, http.StatusBadRequest, "formato: jsonl ou csv")
		return
	}
	jq, ok := jobQueryFrom(r)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "desde/ate: use RFC 3339 ou epoch em ms")
		return
	}
	jq.Limit, jq.Offset = exportPage, 0
	name := "log-jobs-" + time.Now().Format("20060102-150405")
	hd := w.Header()
	hd.Set("Cache-Control", "no-store")
	if format == "csv" {
		hd.Set("Content-Type", "text/csv; charset=utf-8")
		hd.Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
	} else {
		hd.Set("Content-Type", "application/x-ndjson; charset=utf-8")
		hd.Set("Content-Disposition", `attachment; filename="`+name+`.jsonl"`)
	}
	var cw *csv.Writer
	var enc *json.Encoder
	if format == "csv" {
		cw = csv.NewWriter(w)
		head := exportHeader
		if withContent {
			head = append(append([]string{}, head...), "prompt", "resposta", "truncado")
		}
		_ = cw.Write(head)
	} else {
		enc = json.NewEncoder(w) // escapes < > & like every JSON of the monitor
	}
	written := 0
	for written < exportMaxRows {
		rows, err := h.cfg.Store.Jobs(r.Context(), jq)
		if err != nil {
			h.logf("monitor: export: %v", err)
			return // headers are gone; the truncated file is the signal
		}
		for _, j := range rows {
			var c content.Content
			var have bool
			if withContent {
				c, have, _ = h.cfg.Content.Get(r.Context(), j.ID)
			}
			if cw != nil {
				cells := exportCells(j)
				if withContent {
					cells = append(cells, csvSafe(c.Prompt), csvSafe(c.Response), strconv.FormatBool(have && c.Truncated))
				}
				_ = cw.Write(cells)
			} else {
				rec := map[string]any{"job": j}
				if withContent && have {
					rec["prompt"], rec["response"], rec["truncado"], rec["resp_final"] = c.Prompt, c.Response, c.Truncated, c.RespFinal
				}
				_ = enc.Encode(rec)
			}
			written++
		}
		if len(rows) < exportPage {
			break
		}
		jq.Offset += exportPage
	}
	if cw != nil {
		cw.Flush()
	}
}
