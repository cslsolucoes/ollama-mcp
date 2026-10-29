package store

// Fase 9.5 - revisao adversarial: vetores do pacote store.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado.

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func vec(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

func measure(f func()) (time.Duration, uint64) {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	t0 := time.Now()
	f()
	d := time.Since(t0)
	runtime.ReadMemStats(&b)
	return d, b.TotalAlloc - a.TotalAlloc
}

// S1 (pendencia 5): Stats sobre um banco sintetico de 90 dias. Mede tempo e
// alocacao e verifica o plano de consulta: a janela de Stats (finished_at)
// nao tem indice, entao cada chamada varre a tabela inteira (e o monitor chama
// Stats duas vezes a cada 2 s enquanto houver painel aberto).
func TestAdvStatsSynthetic90Days(t *testing.T) {
	n := 207000 // ~2,3 mil pedidos/dia em 90 dias
	if v := os.Getenv("MCPGW_ADV_ROWS"); v != "" {
		fmt.Sscan(v, &n)
	}
	s, err := Open(filepath.Join(t.TempDir(), "s.db"), Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t0 := time.Now()
	fillSynthetic(t, s.db, n, 90)
	t.Logf("preenchido: %d linhas em %v", countRows(t, s.db, "requests"), time.Since(t0))
	ctx := context.Background()
	for _, w := range []struct {
		label string
		d     time.Duration
	}{{"15m", 15 * time.Minute}, {"hoje~12h", 12 * time.Hour}, {"24h", 24 * time.Hour}, {"7d", 7 * 24 * time.Hour}, {"90d", 90 * 24 * time.Hour}} {
		var st Stats
		d, alloc := measure(func() { st, err = s.Stats(ctx, w.d, w.label) })
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("MEDIDA|stats|%s|linhas=%d|tempo=%v|alocado=%.1fMiB", w.label, st.Total, d, float64(alloc)/(1<<20))
	}
	d, alloc := measure(func() { _, err = s.History(ctx, HistoryQuery{Limit: 50}) })
	t.Logf("MEDIDA|history50|tempo=%v|alocado=%.1fMiB", d, float64(alloc)/(1<<20))
	d, _ = measure(func() { _, err = s.History(ctx, HistoryQuery{Limit: 100, Offset: n - 10}) })
	t.Logf("MEDIDA|history-offset-fim|tempo=%v", d)

	var plan strings.Builder
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN SELECT status,error_class,started_at,wait_ms,run_ms,prompt_eval_count,eval_count FROM requests WHERE finished_at >= ? LIMIT ?`, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, c int
		var detail string
		rows.Scan(&a, &b, &c, &detail)
		plan.WriteString(detail + ";")
	}
	rows.Close()
	t.Logf("plano: %s", plan.String())
	if strings.Contains(plan.String(), "SCAN requests") && !strings.Contains(plan.String(), "INDEX") {
		vec(t, "S1-stats-scan", "Baixa", "gateway/store/store.go:511", "FALHA: Stats varre a tabela inteira (sem indice em finished_at)")
		t.Errorf("Stats sem indice: %s", plan.String())
	} else {
		vec(t, "S1-stats-scan", "Baixa", "gateway/store/store.go:511", "OK: Stats usa indice")
	}
}

// S2: crescimento sem limite. Com a taxa permitida pelo nginx (600/min por
// IP), um cliente em laco grava ~864 mil linhas/dia que so expiram em 90 dias.
// O vetor grava (teto padrao + 5%) linhas recentes e chama Maintain: tem de
// sobrar no maximo o teto.
func TestAdvRequestsGrowthWithoutCap(t *testing.T) {
	if testing.Short() {
		t.Skip("grava milhoes de linhas")
	}
	s, err := Open(filepath.Join(t.TempDir(), "g.db"), Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	limit := int64(3_000_000) // teto proposto (DefaultMaxRequests da correcao)
	fillSynthetic(t, s.db, int(limit+limit/20), 1)
	if err := s.Maintain(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	got := countRows(t, s.db, "requests")
	if got > limit {
		vec(t, "S2-requests-sem-teto", "Media", "gateway/store/store.go:621", fmt.Sprintf("FALHA: %d linhas apos Maintain (teto %d)", got, limit))
		t.Errorf("rows %d > cap %d", got, limit)
	} else {
		vec(t, "S2-requests-sem-teto", "Media", "gateway/store/store.go:621", fmt.Sprintf("OK: %d linhas apos Maintain (teto %d)", got, limit))
	}
}

// S3: sob sobrecarga cada lote grava um evento registros_descartados, e events
// nunca expira: 4 eventos/s para sempre.
func TestAdvDroppedEventsFlood(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "d.db"), Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 20; i++ {
		s.dropped.Add(3)
		s.Event("x", "")
		s.Flush()
	}
	var n int64
	s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE kind='registros_descartados'`).Scan(&n)
	if n > 1 {
		vec(t, "S3-eventos-descarte", "Baixa", "gateway/store/store.go:321", fmt.Sprintf("FALHA: %d eventos registros_descartados em 20 lotes seguidos", n))
		t.Errorf("events %d", n)
	} else {
		vec(t, "S3-eventos-descarte", "Baixa", "gateway/store/store.go:321", fmt.Sprintf("OK: %d evento(s) em 20 lotes (agregado)", n))
	}
	if s.Dropped() == 0 && n == 0 {
		t.Error("the drop count vanished without an event")
	}
}

// S4: injecao SQL pelos filtros do /history.
func TestAdvHistorySQLInjection(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "i.db"), Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fillSynthetic(t, s.db, 100, 1)
	for _, p := range []string{`' OR 1=1 --`, `x' UNION SELECT * FROM audit --`, `"; DROP TABLE requests; --`, "agente-1'"} {
		rows, err := s.History(context.Background(), HistoryQuery{Client: p, Model: p})
		if err != nil || len(rows) != 0 {
			vec(t, "S4-sqli-history", "Alta", "gateway/store/store.go:462", fmt.Sprintf("FALHA: %q -> %d linhas, err=%v", p, len(rows), err))
			t.Errorf("sqli %q", p)
		}
	}
	if countRows(t, s.db, "requests") != 100 {
		t.Error("table changed")
	}
	vec(t, "S4-sqli-history", "Alta", "gateway/store/store.go:462", "OK: filtros parametrizados; 0 linhas e tabela intacta")
}

// S5: sentinela nos caminhos de erro: erro HTTP do Ollama com o prompt ecoado,
// linha de erro no stream, rejeicao por modelo e por fila cheia. error_class
// so pode carregar texto fixo do gateway.
func TestAdvSentinelErrorPaths(t *testing.T) {
	const sent = "SENTINELA-ERRO-9b1f-prompt-ecoado"
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "e.db")
	s, err := Open(dbPath, Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mode := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		switch mode {
		case 0:
			w.WriteHeader(500)
			io.WriteString(w, `{"error":"falhou ao processar: `+sent+`"}`)
		case 1:
			io.WriteString(w, `{"response":"a","done":false}`+"\n"+`{"error":"`+sent+`"}`+"\n")
		case 2:
			w.WriteHeader(400)
			io.WriteString(w, sent)
		}
	}))
	defer up.Close()
	u, _ := url.Parse(up.URL)
	q := queue.New(queue.Config{OnRecord: s.Record})
	h, _ := proxy.New(proxy.Config{Upstream: u, Queue: q, AllowedModels: []string{"qwen3-ia"}})
	gw := httptest.NewServer(h)
	defer gw.Close()
	send := func(body string) int {
		resp, err := http.Post(gw.URL+"/api/generate", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	for mode = 0; mode < 3; mode++ {
		send(`{"model":"qwen3-ia","prompt":"` + sent + `"}`)
	}
	send(`{"model":"` + sent + `","prompt":"x"}`) // 403: nao entra na fila
	time.Sleep(50 * time.Millisecond)
	s.Flush()
	var classes []string
	r, _ := s.db.Query(`SELECT error_class FROM requests`)
	for r.Next() {
		var c sql.NullString
		r.Scan(&c)
		classes = append(classes, c.String)
	}
	r.Close()
	t.Logf("error_class gravados: %v", classes)
	leak := false
	for _, f := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		b, _ := os.ReadFile(f)
		if bytes.Contains(b, []byte(sent)) {
			leak = true
			t.Errorf("sentinel found in %s", filepath.Base(f))
		}
	}
	if leak {
		vec(t, "S5-sentinela-erros", "Alta", "gateway/proxy/proxy.go:387", "FALHA: texto do upstream chegou ao banco")
	} else {
		vec(t, "S5-sentinela-erros", "Alta", "gateway/proxy/proxy.go:387", fmt.Sprintf("OK: 0 ocorrencias em .db/-wal/-shm; error_class=%v", classes))
	}
}
