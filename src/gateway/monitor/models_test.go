package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func TestStateCarriesModelsFromTheCacheOnly(t *testing.T) {
	var queries atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		switch r.URL.Path {
		case "/api/ps":
			w.Write([]byte(`{"models":[{"name":"qwen3-ia:latest","size":2097152,"size_vram":2097152,"context_length":65536,"expires_at":"2026-10-30T15:00:00Z"}]}`))
		case "/api/tags":
			w.Write([]byte(`{"models":[{"name":"qwen3-ia:latest","size":4194304,"details":{"parameter_size":"9.7B","quantization_level":"Q4_K_M","family":"qwen35"}},{"name":"outro:1b","size":1048576}]}`))
		}
	}))
	defer up.Close()
	u, _ := url.Parse(up.URL)
	col := &models.Collector{Upstream: u, Allowed: []string{"qwen3-ia"}, ThinkSuffix: "-think"}
	col.Once(context.Background()) // the collector's own round: 2 queries
	base := queries.Load()
	if base != 2 {
		t.Fatalf("%d queries in one round", base)
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "gw.db"), store.Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hub := NewHub()
	q := queue.New(queue.Config{OnChange: hub.Notify, OnRecord: st.Record})
	h, err := New(Config{Queue: q, Store: st, Hub: hub, Models: col, Version: "t", BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for i := 0; i < 25; i++ { // many viewers
		w := do(h, "GET", "/monitor/api/state", "", nil)
		if w.Code != 200 {
			t.Fatalf("%d", w.Code)
		}
		body = w.Body.String()
	}
	if queries.Load() != base {
		t.Fatalf("the state queried the Ollama: %d -> %d", base, queries.Load())
	}
	var st2 struct {
		Models *models.Snapshot `json:"models"`
	}
	if err := json.Unmarshal([]byte(body), &st2); err != nil || st2.Models == nil {
		t.Fatalf("%v %s", err, body)
	}
	m := st2.Models
	if len(m.Loaded) != 1 || m.Loaded[0].Name != "qwen3-ia:latest" || m.Loaded[0].Processor != "100% GPU" || m.Loaded[0].ContextLength != 65536 || !m.Loaded[0].Allowed {
		t.Fatalf("%+v", m.Loaded)
	}
	if len(m.Installed) != 2 || !m.Installed[0].Allowed || m.Installed[1].Allowed || m.Installed[0].Quantization != "Q4_K_M" {
		t.Fatalf("%+v", m.Installed)
	}
	if len(m.Allowed) != 1 || m.Allowed[0] != "qwen3-ia" || m.ThinkSuffix != "-think" || m.AllowAll {
		t.Fatalf("%+v", m)
	}
}

func TestStateWithoutModelsCollector(t *testing.T) {
	h := cpuHandler(t, nil)
	if w := do(h, "GET", "/monitor/api/state", "", nil); strings.Contains(w.Body.String(), `"models":`) {
		t.Fatal("models without a collector")
	}
}

func TestPageHasModelsBoard(t *testing.T) {
	page := string(indexHTML)
	for _, want := range []string{`<h2>Modelos</h2>`, `id="mloaded"`, `id="minstalled"`, `Carregados (VRAM)`, `Disponíveis (instalados)`, `liga o raciocínio de`, `renderModels(s.models)`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}
