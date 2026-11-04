package treino

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
)

// Handler serves POST /api/feedback (D12-5, §8.3). The Bearer is checked by the
// nginx, as for every route of the gateway; the handler does not authenticate
// again, and accepts only POST with JSON.
//
// Store nil means the feature is off (no --treino-db): 503. A Key that is not
// configured also gives 503 (C-4.3a). The database work uses a context of its
// own with a timeout, never the request's (C-6.10).
type Handler struct {
	Store   *Store
	Source  Source
	Key     *hmackey.Key
	Role    string        // --role: primario (default) or fallback
	Timeout time.Duration // database work, default 30 s
	Logf    func(format string, args ...any)
}

func writeErr(w http.ResponseWriter, status int, msg string, extra map[string]any) {
	m := map[string]any{"error": msg}
	for k, v := range extra {
		m[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(m)
	_, _ = w.Write(append(b, '\n'))
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, http.StatusMethodNotAllowed, "use POST", nil)
		return
	}
	if h.Store == nil {
		writeErr(w, http.StatusServiceUnavailable, "registro de erros desligado neste gateway: falta --treino-db", nil)
		return
	}
	if !h.Key.Configured() {
		writeErr(w, http.StatusServiceUnavailable, "este gateway não aceita relatos: falta --doc-key-file (a chave HMAC dos dados de treino, C-4.3a)", nil)
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "use Content-Type: application/json", nil)
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, http.StatusRequestEntityTooLarge, "corpo maior que 256 KB", nil)
		} else { // m-19: a body that broke off is not a body that was too big
			writeErr(w, http.StatusBadRequest, "corpo incompleto ou ilegível", nil)
		}
		return
	}
	if err := checkFieldNames(b); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido ou com campo desconhecido: "+err.Error(), nil)
		return
	}
	var rep Report
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido ou com campo desconhecido: "+err.Error(), nil)
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		writeErr(w, http.StatusBadRequest, "JSON inválido: há conteúdo depois do objeto", nil)
		return
	}
	if err := rep.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	st := proxy.ParseStation(r.Header.Values(proxy.HeaderStation))
	o := Origin{Computer: st.Computer, User: st.User, Domain: st.Domain, MAC: st.MAC,
		ClientIP: proxy.ClientIP(r), ClientLabel: proxy.ClientLabel(r)}

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := h.Store.Ingest(ctx, rep, o, h.Role, h.Source, h.Key)
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		out, _ := json.Marshal(res)
		_, _ = w.Write(append(out, '\n'))
	case errors.Is(err, ErrJobNotFound):
		writeErr(w, http.StatusNotFound, "job_do_erro não existe neste gateway: o relato vai ao gateway que criou o job; nada foi gravado", nil)
	case errors.Is(err, ErrCapReached):
		writeErr(w, http.StatusInsufficientStorage, "treino.db no teto: novos relatos recusados até o admin apagar ocorrências; nada foi apagado",
			map[string]any{"teto_bytes": h.Store.MaxBytes(), "tamanho_bytes": h.Store.Size()})
	case errors.Is(err, ErrJobBusy):
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusConflict, "o job ainda está na fila ou em execução: espere terminar e relate de novo; nada foi gravado", nil)
	case errors.Is(err, ErrBusy), errors.Is(err, context.DeadlineExceeded):
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusServiceUnavailable, "treino.db ocupado: tente de novo em instantes; nada foi gravado", nil)
	case errors.Is(err, ErrNoKey):
		writeErr(w, http.StatusServiceUnavailable, "este gateway não aceita relatos: falta --doc-key-file", nil)
	default:
		if h.Logf != nil {
			h.Logf("treino: relato falhou: %v", err)
		}
		writeErr(w, http.StatusInternalServerError, "erro interno ao gravar o relato", nil)
	}
}
