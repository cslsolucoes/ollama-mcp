package monitor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/cpu"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func writeFake(t *testing.T, root, rel, txt string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(txt), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cpuHandler(t *testing.T, col *cpu.Collector) *Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "gw.db"), store.Options{BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hub := NewHub()
	q := queue.New(queue.Config{OnChange: hub.Notify, OnRecord: st.Record})
	h, err := New(Config{Queue: q, Store: st, Hub: hub, CPU: col, Version: "test", BootID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestStateCarriesCPU(t *testing.T) {
	proc, sys := t.TempDir(), t.TempDir()
	writeFake(t, proc, "stat", "cpu  10 0 10 80 0 0 0 0 0 0\ncpu0 5 0 5 40 0 0 0 0 0 0\ncpu1 5 0 5 40 0 0 0 0 0 0\n")
	writeFake(t, proc, "loadavg", "0.10 0.20 0.30 1/2 3\n")
	writeFake(t, proc, "meminfo", "MemTotal: 2097152 kB\nMemAvailable: 1048576 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n")
	writeFake(t, sys, "devices/system/cpu/cpu0/topology/core_id", "0\n")
	writeFake(t, sys, "devices/system/cpu/cpu1/topology/core_id", "0\n")
	writeFake(t, sys, "class/hwmon/hwmon1/name", "coretemp\n")
	writeFake(t, sys, "class/hwmon/hwmon1/temp1_label", "Package id 0\n")
	writeFake(t, sys, "class/hwmon/hwmon1/temp1_input", "55000\n")
	writeFake(t, sys, "class/hwmon/hwmon1/temp2_label", "Core 0\n")
	writeFake(t, sys, "class/hwmon/hwmon1/temp2_input", "52000\n")
	col := &cpu.Collector{ProcRoot: proc, SysRoot: sys, BootID: "b"}
	col.Once(context.Background())

	h := cpuHandler(t, col)
	w := do(h, "GET", "/monitor/api/state", "", nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var st struct {
		CPU *struct {
			UtilPct     float64 `json:"util_pct"`
			RAMTotalMiB int64   `json:"ram_total_mib"`
			RAMUsedMiB  int64   `json:"ram_used_mib"`
			TempC       float64 `json:"temp_c"`
			PowerW      float64 `json:"power_w"`
			Threads     []struct {
				ID     int `json:"id"`
				CoreID int `json:"core_id"`
			} `json:"threads"`
			Cores []struct {
				CoreID int     `json:"core_id"`
				TempC  float64 `json:"temp_c"`
			} `json:"cores"`
		} `json:"cpu"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	c := st.CPU
	if c == nil {
		t.Fatalf("state without cpu: %s", w.Body)
	}
	if c.RAMTotalMiB != 2048 || c.RAMUsedMiB != 1024 || c.TempC != 55 || c.PowerW != -1 || c.UtilPct != -1 {
		t.Fatalf("%+v", c)
	}
	if len(c.Threads) != 2 || c.Threads[1].ID != 1 || c.Threads[1].CoreID != 0 || len(c.Cores) != 1 || c.Cores[0].TempC != 52 {
		t.Fatalf("%+v", c)
	}
}

func TestStateWithoutCPUCollector(t *testing.T) {
	h := cpuHandler(t, nil)
	w := do(h, "GET", "/monitor/api/state", "", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"cpu":`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestPageHasCPUBoardAndNoInlineHandlers(t *testing.T) {
	page := string(indexHTML)
	for _, want := range []string{`<h2>CPU / RAM</h2>`, `id="cpucards"`, `id="cputhreads"`, `potência da CPU: indisponível (só root)`, `fill.style.width`, `fillP.style.width`, `power_limit_w`, `"pbar hot"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, bad := range []string{" onclick=", " onload=", " onerror=", "innerHTML", `style="`} {
		if strings.Contains(page, bad) {
			t.Errorf("page contains %q", bad)
		}
	}
	if !strings.Contains(page, "setInterval(poll, 1000)") {
		t.Error("fallback polling is not 1 s")
	}
}

func TestStreamEveryDefaultsToOneSecond(t *testing.T) {
	h := cpuHandler(t, nil)
	if h.cfg.StreamEvery != time.Second {
		t.Fatalf("%v", h.cfg.StreamEvery)
	}
}
