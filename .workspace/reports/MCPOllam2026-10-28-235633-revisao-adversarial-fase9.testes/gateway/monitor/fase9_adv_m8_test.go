package monitor

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// M8: /stats aberto e sem cache. Cada GET da janela 7d le ate 200 mil linhas
// (~300 ms, ~49 MiB no banco sintetico de S1); pedidos paralelos multiplicam o
// custo na maquina que tambem roda o Ollama.
func TestAdvStatsNoCache(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"), store.Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := queue.New(queue.Config{OnRecord: st.Record})
	for i := 0; i < 3000; i++ {
		tk, _ := q.Acquire(context.Background(), queue.Request{ClientIP: "10.0.0.1", ClientLabel: "x"})
		tk.Release(queue.Result{})
	}
	st.Flush()
	h, _ := New(Config{Queue: q, Store: st})
	var wg sync.WaitGroup
	t0 := time.Now()
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); do(h, "GET", "/monitor/api/stats?janela=7d", "", nil) }()
	}
	wg.Wait()
	d := time.Since(t0)
	n := h.statsComputed()
	if n > 2 {
		vec(t, "M8-stats-sem-cache", "Baixa", "gateway/monitor/monitor.go:491", fmt.Sprintf("FALHA: 30 GET paralelos -> %d calculos (%v)", n, d))
		t.Errorf("computed %d", n)
	} else {
		vec(t, "M8-stats-sem-cache", "Baixa", "gateway/monitor/monitor.go:491", fmt.Sprintf("OK: 30 GET paralelos -> %d calculo(s) (%v)", n, d))
	}
}
