package cpu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, root, rel, txt string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(txt), 0o644); err != nil {
		t.Fatal(err)
	}
}

// statText builds /proc/stat with 12 logical CPUs. busy[i] and idle[i] are the
// jiffies of cpuI; the aggregate line is the sum.
func statText(busy, idle [12]uint64) string {
	var b strings.Builder
	var tb, ti uint64
	for i := range busy {
		tb += busy[i]
		ti += idle[i]
	}
	fmt.Fprintf(&b, "cpu  %d 0 0 %d 0 0 0 0 0 0\n", tb, ti)
	for i := range busy {
		fmt.Fprintf(&b, "cpu%d %d 0 0 %d 0 0 0 0 0 0\n", i, busy[i], idle[i])
	}
	b.WriteString("intr 1 2 3\nctxt 5\n")
	return b.String()
}

const meminfo = `MemTotal:       98304000 kB
MemFree:         1000000 kB
MemAvailable:   49152000 kB
SwapTotal:      15728640 kB
SwapFree:       14680064 kB
`

type tree struct{ proc, sys string }

func newTree(t *testing.T) tree {
	tr := tree{t.TempDir(), t.TempDir()}
	put(t, tr.proc, "stat", statText([12]uint64{}, [12]uint64{}))
	put(t, tr.proc, "loadavg", "1.50 2.25 3.00 2/900 1234\n")
	put(t, tr.proc, "meminfo", meminfo)
	put(t, tr.proc, "cpuinfo", "processor : 0\ncpu MHz\t\t: 3000.000\nprocessor : 1\ncpu MHz\t\t: 4000.000\n")
	// 12 threads on 6 cores: cpuN and cpuN+6 share core N.
	for i := 0; i < 12; i++ {
		put(t, tr.sys, fmt.Sprintf("devices/system/cpu/cpu%d/topology/core_id", i), fmt.Sprintf("%d\n", i%6))
	}
	put(t, tr.sys, "class/hwmon/hwmon3/name", "coretemp\n")
	put(t, tr.sys, "class/hwmon/hwmon3/temp1_label", "Package id 0\n")
	put(t, tr.sys, "class/hwmon/hwmon3/temp1_input", "61000\n")
	for c := 0; c < 6; c++ {
		put(t, tr.sys, fmt.Sprintf("class/hwmon/hwmon3/temp%d_label", c+2), fmt.Sprintf("Core %d\n", c))
		put(t, tr.sys, fmt.Sprintf("class/hwmon/hwmon3/temp%d_input", c+2), fmt.Sprintf("%d\n", 50000+c*1000))
	}
	put(t, tr.sys, "class/hwmon/hwmon0/name", "acpitz\n")
	put(t, tr.sys, "class/hwmon/hwmon0/temp1_input", "99000\n")
	// ollama: two processes (server and runner) and one stranger.
	put(t, tr.proc, "100/comm", "ollama\n")
	put(t, tr.proc, "100/status", "Name:\tollama\nVmRSS:\t  2097152 kB\n")
	put(t, tr.proc, "101/comm", "ollama\n")
	put(t, tr.proc, "101/status", "Name:\tollama\nVmRSS:\t  1048576 kB\n")
	put(t, tr.proc, "102/comm", "bash\n")
	put(t, tr.proc, "102/status", "VmRSS:\t 9999999 kB\n")
	return tr
}

func TestCollectorFakeTree(t *testing.T) {
	tr := newTree(t)
	c := &Collector{ProcRoot: tr.proc, SysRoot: tr.sys, BootID: "b1"}
	first := c.Once(context.Background())
	if first.UtilPct != -1 {
		t.Fatalf("first round has no delta, util = %v", first.UtilPct)
	}
	for _, th := range first.Threads {
		if th.UtilPct != -1 {
			t.Fatalf("first round thread %+v", th)
		}
	}
	// Second round: every thread did 25 busy + 75 idle jiffies, except cpu3 = 100% busy.
	var busy, idle [12]uint64
	for i := range busy {
		busy[i], idle[i] = 25, 75
	}
	busy[3], idle[3] = 100, 0
	put(t, tr.proc, "stat", statText(busy, idle))
	s := c.Once(context.Background())

	wantTotal := 100 * float64(11*25+100) / float64(12*100)
	if d := s.UtilPct - wantTotal; d > 0.01 || d < -0.01 {
		t.Fatalf("util = %v want %v", s.UtilPct, wantTotal)
	}
	if len(s.Threads) != 12 {
		t.Fatalf("threads = %d", len(s.Threads))
	}
	for i, th := range s.Threads {
		want := 25.0
		if i == 3 {
			want = 100
		}
		if th.ID != i || th.UtilPct != want || th.CoreID != i%6 {
			t.Fatalf("thread %d = %+v", i, th)
		}
	}
	if s.Load1 != 1.5 || s.Load5 != 2.25 || s.Load15 != 3 {
		t.Fatalf("load %+v", s)
	}
	if s.RAMTotalMiB != 96000 || s.RAMAvailMiB != 48000 || s.RAMUsedMiB != 48000 {
		t.Fatalf("ram %d/%d/%d", s.RAMUsedMiB, s.RAMAvailMiB, s.RAMTotalMiB)
	}
	if s.SwapTotalMiB != 15360 || s.SwapUsedMiB != 1024 {
		t.Fatalf("swap %d/%d", s.SwapUsedMiB, s.SwapTotalMiB)
	}
	if s.TempC != 61 {
		t.Fatalf("temp %v", s.TempC)
	}
	if len(s.Cores) != 6 || s.Cores[0] != (Core{0, 50}) || s.Cores[5] != (Core{5, 55}) {
		t.Fatalf("cores %+v", s.Cores)
	}
	if s.FreqMHz != 3500 {
		t.Fatalf("freq %v", s.FreqMHz)
	}
	if s.OllamaRSSMiB != 3072 {
		t.Fatalf("ollama rss %d", s.OllamaRSSMiB)
	}
	if s.PowerW != -1 {
		t.Fatalf("no rapl must be -1, got %v", s.PowerW)
	}
	if s.Err != "" || s.BootID != "b1" {
		t.Fatalf("err %q boot %q", s.Err, s.BootID)
	}
	if got := c.Latest(); got.RAMTotalMiB != 96000 {
		t.Fatalf("latest %+v", got)
	}
}

func TestTopologyUnreadableIsMinusOne(t *testing.T) {
	tr := newTree(t)
	if err := os.RemoveAll(filepath.Join(tr.sys, "devices")); err != nil {
		t.Fatal(err)
	}
	c := &Collector{ProcRoot: tr.proc, SysRoot: tr.sys}
	s := c.Once(context.Background())
	if len(s.Threads) != 12 || s.Threads[0].CoreID != -1 {
		t.Fatalf("%+v", s.Threads)
	}
}

func TestTempFallbackThermalZone(t *testing.T) {
	sys := t.TempDir()
	put(t, sys, "class/thermal/thermal_zone0/type", "acpitz\n")
	put(t, sys, "class/thermal/thermal_zone0/temp", "99000\n")
	put(t, sys, "class/thermal/thermal_zone1/type", "x86_pkg_temp\n")
	put(t, sys, "class/thermal/thermal_zone1/temp", "47500\n")
	if v, err := ReadTemp(sys); err != nil || v != 47.5 {
		t.Fatalf("%v %v", v, err)
	}
	if cs := ReadCoreTemps(sys); len(cs) != 0 {
		t.Fatalf("no coretemp, no cores: %+v", cs)
	}
}

func TestTempCoretempWithoutLabelUsesTemp1(t *testing.T) {
	sys := t.TempDir()
	put(t, sys, "class/hwmon/hwmon1/name", "coretemp\n")
	put(t, sys, "class/hwmon/hwmon1/temp1_input", "70000\n")
	if v, err := ReadTemp(sys); err != nil || v != 70 {
		t.Fatalf("%v %v", v, err)
	}
}

func TestTempMissingAndInsane(t *testing.T) {
	sys := t.TempDir()
	if v, err := ReadTemp(sys); err == nil || v != -1 {
		t.Fatalf("%v %v", v, err)
	}
	put(t, sys, "class/hwmon/hwmon1/name", "coretemp\n")
	put(t, sys, "class/hwmon/hwmon1/temp1_input", "999999999\n")
	if _, err := ReadTemp(sys); err == nil {
		t.Fatal("absurd temperature accepted")
	}
}

func TestRAPLUnreadableIsMinusOneWithoutError(t *testing.T) {
	skipNoColons(t)
	tr := newTree(t)
	rapl := filepath.Join(tr.sys, "class", "powercap", "intel-rapl:0")
	// energy_uj exists but cannot be read: a directory makes ReadFile fail on
	// every OS and user (chmod 0400 stops neither root nor Windows).
	if err := os.MkdirAll(filepath.Join(rapl, "energy_uj"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Collector{ProcRoot: tr.proc, SysRoot: tr.sys}
	c.Once(context.Background())
	s := c.Once(context.Background())
	if s.PowerW != -1 || strings.Contains(s.Err, "energy") || strings.Contains(s.Err, "RAPL") {
		t.Fatalf("power %v err %q", s.PowerW, s.Err)
	}
}

func TestRAPLReadable(t *testing.T) {
	skipNoColons(t)
	tr := newTree(t)
	put(t, tr.sys, "class/powercap/intel-rapl:0/energy_uj", "1000000\n")
	put(t, tr.sys, "class/powercap/intel-rapl:0:0/energy_uj", "5\n") // sub-domain, ignored
	c := &Collector{ProcRoot: tr.proc, SysRoot: tr.sys}
	c.Once(context.Background())
	time.Sleep(50 * time.Millisecond)
	put(t, tr.sys, "class/powercap/intel-rapl:0/energy_uj", "3000000\n") // +2 J
	s := c.Once(context.Background())
	if s.PowerW <= 0 || s.PowerW > 2/0.05+1 {
		t.Fatalf("power %v", s.PowerW)
	}
}

func TestUnreadableFilesGiveMinusOneAndShortError(t *testing.T) {
	c := &Collector{ProcRoot: t.TempDir(), SysRoot: t.TempDir()}
	s := c.Once(context.Background())
	if s.UtilPct != -1 || s.Load1 != -1 || s.RAMTotalMiB != -1 || s.SwapUsedMiB != -1 || s.TempC != -1 ||
		s.FreqMHz != -1 || s.OllamaRSSMiB != -1 || s.PowerW != -1 {
		t.Fatalf("%+v", s)
	}
	if s.Err == "" || len(s.Err) > 200 || strings.Contains(s.Err, "\n") {
		t.Fatalf("err %q", s.Err)
	}
	if s.Threads == nil || s.Cores == nil {
		t.Fatal("slices must be non-nil for the JSON")
	}
}

func TestCounterResetAndGarbage(t *testing.T) {
	tr := newTree(t)
	c := &Collector{ProcRoot: tr.proc, SysRoot: tr.sys}
	var busy, idle [12]uint64
	for i := range busy {
		busy[i], idle[i] = 500, 500
	}
	put(t, tr.proc, "stat", statText(busy, idle))
	c.Once(context.Background())
	// counters went backwards (reboot inside the interval): no delta, no panic
	put(t, tr.proc, "stat", statText([12]uint64{}, [12]uint64{}))
	if s := c.Once(context.Background()); s.UtilPct != -1 {
		t.Fatalf("util %v", s.UtilPct)
	}
	put(t, tr.proc, "stat", "cpu  a b c\ncpu0 x\n")
	put(t, tr.proc, "meminfo", "MemTotal: lots kB\n")
	put(t, tr.proc, "loadavg", "x y z\n")
	s := c.Once(context.Background())
	if s.UtilPct != -1 || s.RAMTotalMiB != -1 || s.Load1 != -1 || s.Err == "" {
		t.Fatalf("%+v", s)
	}
}

func TestRunLoopCallsOnSampleAndStops(t *testing.T) {
	tr := newTree(t)
	got := make(chan Sample, 8)
	c := &Collector{ProcRoot: tr.proc, SysRoot: tr.sys, Interval: 20 * time.Millisecond, OnSample: func(s Sample) { got <- s }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.RunLoop(ctx); close(done) }()
	<-got
	<-got
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunLoop did not stop")
	}
}

// CollectionCost documents the time of one round on a fake tree (no I/O wait
// beyond the page cache): the changelog quotes it.
func TestCollectionCost(t *testing.T) {
	tr := newTree(t)
	c := &Collector{ProcRoot: tr.proc, SysRoot: tr.sys}
	c.Once(context.Background())
	const n = 200
	start := time.Now()
	for i := 0; i < n; i++ {
		c.Once(context.Background())
	}
	per := time.Since(start) / n
	t.Logf("cpu round on the fake tree: %v", per)
	if per > 100*time.Millisecond {
		t.Fatalf("a round takes %v", per)
	}
}

// skipNoColons: the sysfs names intel-rapl:0 cannot exist on Windows.
func skipNoColons(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory names with ':' are invalid on Windows; runs on Linux")
	}
}
