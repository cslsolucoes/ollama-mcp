package monitor

import (
	"net/http"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// backendAction drains or resumes a backend (Fase 10.4): a drained backend takes
// no new start, what is running there finishes. Same gate as the queue actions:
// the administrator session, the CSRF token and the same-site check (authorize),
// and one audit row. The id comes from the path and must name a backend of the
// registry; nothing the client sends can name a new one.
func (h *Handler) backendAction(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, s, ok := h.authorize(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !pool.IDValid(id) {
			jsonErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !readSmallJSON(w, r, &in) {
			return
		}
		reason := in.Reason
		if len([]rune(reason)) > 200 {
			reason = string([]rune(reason)[:200])
		}
		pl := h.cfg.Queue.Pool()
		if h.cfg.Pool != nil {
			pl = h.cfg.Pool
		}
		before, after := "ativo", "drenado"
		if kind == "undrain" {
			before, after = after, before
		}
		changed, found := pl.SetDrained(id, kind == "drain")
		if !found {
			jsonErr(w, http.StatusNotFound, "backend desconhecido")
			return
		}
		if !changed {
			jsonErr(w, http.StatusConflict, "o backend já está "+after)
			return
		}
		out := map[string]any{"ok": true, "acao": kind, "id": id, "antes": before, "depois": after}
		if h.cfg.Store != nil {
			if aerr := h.cfg.Store.AddAudit(store.AuditEntry{
				User: s.user, IP: proxy.ClientIP(r), Action: kind, RequestID: id, Before: before, After: after, Reason: reason,
			}); aerr != nil {
				h.logf("monitor: auditoria falhou: %v", aerr)
				out["audit_error"] = "a ação foi feita, mas a auditoria não foi gravada"
			}
		}
		h.cfg.Queue.Poke() // a drained primary hands its waiting work to a secondary; a resumed backend starts taking work
		h.cfg.Hub.Notify()
		writeJSON(w, http.StatusOK, out)
	}
}
