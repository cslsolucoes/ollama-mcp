// Package cpu samples the server CPU and RAM (monitor "CPU / RAM" board): the
// delta of /proc/stat (total and per logical CPU), loadavg, meminfo, the
// package and per-core temperature (hwmon coretemp, then thermal
// x86_pkg_temp), the mean frequency, the RSS of the ollama processes and, when
// readable, the RAPL package power. Read only; nothing here changes the host,
// and a field that cannot be read becomes -1 (never an error that stops the
// gateway).
//
// It is a package of its own, and not part of gpu, because the source is
// different (procfs/sysfs, no external command) and the sample is one row per
// instant, not one per device. The loop shape mirrors gpu.Collector.
package cpu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default roots of the pseudo filesystems (overridden in tests).
const (
	DefaultProcRoot = "/proc"
	DefaultSysRoot  = "/sys"
)

// maxFile bounds one pseudo file read, so a hostile or fake root cannot make
// the sampler allocate without limit.
const maxFile = 1 << 20

// maxProcs bounds the /proc entries inspected for the ollama RSS.
const maxProcs = 20000

// maxThreads bounds the cpuN lines kept.
const maxThreads = 1024

// Thread is one logical CPU. UtilPct and CoreID are -1 when unknown.
type Thread struct {
	ID      int     `json:"id"`
	UtilPct float64 `json:"util_pct"`
	CoreID  int     `json:"core_id"`
}

// Core is one physical core temperature (coretemp "Core N"); TempC is -1 when
// unknown.
type Core struct {
	CoreID int     `json:"core_id"`
	TempC  float64 `json:"temp_c"`
}

// Sample is the machine at one instant. Every unreadable number is -1.
type Sample struct {
	At           time.Time `json:"at"`
	UtilPct      float64   `json:"util_pct"` // total CPU use since the previous sample; -1 on the first round
	Load1        float64   `json:"load1"`
	Load5        float64   `json:"load5"`
	Load15       float64   `json:"load15"`
	RAMTotalMiB  int64     `json:"ram_total_mib"`
	RAMUsedMiB   int64     `json:"ram_used_mib"` // total - MemAvailable
	RAMAvailMiB  int64     `json:"ram_avail_mib"`
	SwapUsedMiB  int64     `json:"swap_used_mib"`
	SwapTotalMiB int64     `json:"swap_total_mib"`
	TempC        float64   `json:"temp_c"`
	FreqMHz      float64   `json:"freq_mhz"`
	OllamaRSSMiB int64     `json:"ollama_rss_mib"`
	PowerW       float64   `json:"power_w"` // RAPL is root-only on many hosts: -1 then
	Threads      []Thread  `json:"threads"`
	Cores        []Core    `json:"cores"`
	Err          string    `json:"error,omitempty"`
	BootID       string    `json:"boot_id"`
}

// Collector samples on a fixed interval.
type Collector struct {
	ProcRoot string        // default DefaultProcRoot
	SysRoot  string        // default DefaultSysRoot
	Interval time.Duration // default 5 s
	BootID   string
	// OnSample receives every round (outside any lock).
	OnSample func(Sample)
	Logf     func(format string, args ...any)

	mu     sync.Mutex
	latest Sample

	// state of the previous round (Once is serialized by runMu)
	runMu    sync.Mutex
	havePrev bool
	prevAgg  jiff
	prevPer  map[int]jiff
	prevE    int64 // RAPL energy_uj
	prevEAt  time.Time
	haveE    bool
	lastErr  string
}

func (c *Collector) defaults() {
	if c.ProcRoot == "" {
		c.ProcRoot = DefaultProcRoot
	}
	if c.SysRoot == "" {
		c.SysRoot = DefaultSysRoot
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
}

// Latest returns the last reading (a copy).
func (c *Collector) Latest() Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.latest
	s.Threads = append([]Thread(nil), s.Threads...)
	s.Cores = append([]Core(nil), s.Cores...)
	return s
}

// unavailable is the sample with every field unreadable.
func unavailable(at time.Time, boot string) Sample {
	return Sample{At: at, UtilPct: -1, Load1: -1, Load5: -1, Load15: -1,
		RAMTotalMiB: -1, RAMUsedMiB: -1, RAMAvailMiB: -1, SwapUsedMiB: -1, SwapTotalMiB: -1,
		TempC: -1, FreqMHz: -1, OllamaRSSMiB: -1, PowerW: -1, BootID: boot,
		Threads: []Thread{}, Cores: []Core{}}
}

// jiff is the total and idle (idle + iowait) jiffies of one /proc/stat line.
type jiff struct{ total, idle uint64 }

// util is the use between two readings, false when there is no valid delta.
func util(prev, cur jiff) (float64, bool) {
	if cur.total <= prev.total {
		return -1, false
	}
	dt := cur.total - prev.total
	var di uint64
	if cur.idle > prev.idle {
		di = cur.idle - prev.idle
	}
	if di > dt {
		di = dt
	}
	return clamp(100*float64(dt-di)/float64(dt), 0, 100), true
}

// Once takes one reading.
func (c *Collector) Once(_ context.Context) Sample {
	c.defaults()
	c.runMu.Lock()
	defer c.runMu.Unlock()
	now := time.Now()
	s := unavailable(now, c.BootID)
	var errs []string
	add := func(what string, err error) {
		if err != nil {
			errs = append(errs, what+": "+oneLine(err.Error()))
		}
	}

	// CPU % (total and per logical CPU) by the delta of /proc/stat.
	agg, per, err := readStat(c.ProcRoot)
	if err != nil {
		add("stat", err)
		c.havePrev = false
	} else {
		ids := make([]int, 0, len(per))
		for id := range per {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		topo := readTopology(c.SysRoot, ids)
		for _, id := range ids {
			t := Thread{ID: id, UtilPct: -1, CoreID: topo[id]}
			if c.havePrev {
				if p, ok := c.prevPer[id]; ok {
					if u, ok := util(p, per[id]); ok {
						t.UtilPct = u
					}
				}
			}
			s.Threads = append(s.Threads, t)
		}
		if c.havePrev {
			if u, ok := util(c.prevAgg, agg); ok {
				s.UtilPct = u
			}
		}
		c.prevAgg, c.prevPer, c.havePrev = agg, per, true
	}

	if l1, l5, l15, err := readLoad(c.ProcRoot); err != nil {
		add("loadavg", err)
	} else {
		s.Load1, s.Load5, s.Load15 = l1, l5, l15
	}

	if m, err := readMem(c.ProcRoot); err != nil {
		add("meminfo", err)
	} else {
		s.RAMTotalMiB, s.RAMAvailMiB = m.total, m.avail
		s.RAMUsedMiB = max(m.total-m.avail, 0)
		s.SwapTotalMiB = m.swapTotal
		s.SwapUsedMiB = max(m.swapTotal-m.swapFree, 0)
	}

	if t, err := ReadTemp(c.SysRoot); err != nil {
		add("temp", err)
	} else {
		s.TempC = t
	}
	s.Cores = ReadCoreTemps(c.SysRoot)

	if f, err := readFreq(c.ProcRoot, c.SysRoot); err != nil {
		add("freq", err)
	} else {
		s.FreqMHz = f
	}

	if r, err := ollamaRSS(c.ProcRoot); err != nil {
		add("ollama", err)
	} else {
		s.OllamaRSSMiB = r
	}

	// RAPL: unreadable (root 0400) is the normal case here, not an error.
	if e, maxE, err := readRAPL(c.SysRoot); err != nil {
		c.haveE = false
	} else {
		if c.haveE {
			dE := e - c.prevE
			if dE < 0 && maxE > 0 {
				dE += maxE // counter wrapped
			}
			if dt := now.Sub(c.prevEAt).Seconds(); dE >= 0 && dt > 0 {
				s.PowerW = float64(dE) / 1e6 / dt
			}
		}
		c.prevE, c.prevEAt, c.haveE = e, now, true
	}

	s.Err = oneLine(strings.Join(errs, "; "))
	c.mu.Lock()
	c.latest = s
	c.mu.Unlock()
	if c.OnSample != nil {
		c.OnSample(s)
	}
	// Log only when the error text changes: at 5 s a permanent failure would
	// fill the journal.
	if s.Err != c.lastErr && s.Err != "" && c.Logf != nil {
		c.Logf("cpu: %s", s.Err)
	}
	c.lastErr = s.Err
	return s
}

// RunLoop samples until ctx ends, first reading at once.
func (c *Collector) RunLoop(ctx context.Context) {
	c.defaults()
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for {
		start := time.Now()
		c.Once(ctx)
		// One round at a time by construction. A round longer than the interval
		// leaves a tick waiting in the channel: drop it, so the next round is
		// one interval after this one ended instead of back to back.
		if time.Since(start) >= c.Interval {
			select {
			case <-t.C:
			default:
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// readSmall reads a pseudo file, at most maxFile bytes.
func readSmall(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFile))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseJiff sums the first 8 fields after the label (guest and guest_nice are
// already inside user and nice) and takes idle + iowait.
func parseJiff(f []string) (jiff, error) {
	var j jiff
	for i := 0; i < 8 && i+1 < len(f); i++ {
		n, err := strconv.ParseUint(f[i+1], 10, 64)
		if err != nil {
			return jiff{}, fmt.Errorf("campo %d inválido", i+1)
		}
		j.total += n
		if i == 3 || i == 4 {
			j.idle += n
		}
	}
	return j, nil
}

// readStat returns the aggregate "cpu" line and every "cpuN" line of
// /proc/stat.
func readStat(proc string) (agg jiff, per map[int]jiff, err error) {
	txt, err := readSmall(filepath.Join(proc, "stat"))
	if err != nil {
		return jiff{}, nil, err
	}
	per = map[int]jiff{}
	have := false
	for _, line := range strings.Split(txt, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		j, perr := parseJiff(f)
		if perr != nil {
			return jiff{}, nil, perr
		}
		if f[0] == "cpu" {
			agg, have = j, true
			continue
		}
		id, perr := strconv.Atoi(f[0][3:])
		if perr != nil || id < 0 || id >= maxThreads {
			continue
		}
		per[id] = j
	}
	if !have {
		return jiff{}, nil, errors.New("linha cpu ausente")
	}
	return agg, per, nil
}

// readTopology maps each logical CPU to its physical core (topology/core_id);
// -1 when unreadable.
func readTopology(sys string, ids []int) map[int]int {
	m := make(map[int]int, len(ids))
	for _, id := range ids {
		m[id] = -1
		txt, err := readSmall(filepath.Join(sys, "devices", "system", "cpu", "cpu"+strconv.Itoa(id), "topology", "core_id"))
		if err != nil {
			continue
		}
		if n, perr := strconv.Atoi(strings.TrimSpace(txt)); perr == nil && n >= 0 {
			m[id] = n
		}
	}
	return m
}

func readLoad(proc string) (l1, l5, l15 float64, err error) {
	txt, err := readSmall(filepath.Join(proc, "loadavg"))
	if err != nil {
		return 0, 0, 0, err
	}
	f := strings.Fields(txt)
	if len(f) < 3 {
		return 0, 0, 0, errors.New("formato inesperado")
	}
	var v [3]float64
	for i := range v {
		v[i], err = strconv.ParseFloat(f[i], 64)
		if err != nil || v[i] < 0 {
			return 0, 0, 0, errors.New("valor inválido")
		}
	}
	return v[0], v[1], v[2], nil
}

type mem struct{ total, avail, swapTotal, swapFree int64 } // MiB

func readMem(proc string) (mem, error) {
	txt, err := readSmall(filepath.Join(proc, "meminfo"))
	if err != nil {
		return mem{}, err
	}
	got := map[string]int64{}
	for _, line := range strings.Split(txt, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		k := strings.TrimSuffix(f[0], ":")
		switch k {
		case "MemTotal", "MemAvailable", "SwapTotal", "SwapFree":
			n, perr := strconv.ParseInt(f[1], 10, 64)
			if perr != nil || n < 0 {
				return mem{}, fmt.Errorf("%s inválido", k)
			}
			got[k] = n / 1024 // kB -> MiB
		}
	}
	for _, k := range []string{"MemTotal", "MemAvailable", "SwapTotal", "SwapFree"} {
		if _, ok := got[k]; !ok {
			return mem{}, fmt.Errorf("%s ausente", k)
		}
	}
	return mem{got["MemTotal"], got["MemAvailable"], got["SwapTotal"], got["SwapFree"]}, nil
}

// coretempDirs lists the hwmon directories whose name is "coretemp".
func coretempDirs(sys string) []string {
	var out []string
	names, _ := filepath.Glob(filepath.Join(sys, "class", "hwmon", "hwmon*", "name"))
	for _, np := range names {
		if n, err := readSmall(np); err == nil && strings.TrimSpace(n) == "coretemp" {
			out = append(out, filepath.Dir(np))
		}
	}
	return out
}

// ReadTemp returns the CPU package temperature in °C: hwmon "coretemp" (the
// "Package id" label, else temp1), then the thermal zone "x86_pkg_temp".
func ReadTemp(sys string) (float64, error) {
	var firstErr error
	for _, dir := range coretempDirs(sys) {
		input := filepath.Join(dir, "temp1_input")
		labels, _ := filepath.Glob(filepath.Join(dir, "temp*_label"))
		for _, lp := range labels {
			if l, err := readSmall(lp); err == nil && strings.HasPrefix(strings.TrimSpace(l), "Package id") {
				input = strings.TrimSuffix(lp, "_label") + "_input"
				break
			}
		}
		if t, err := readMilliC(input); err == nil {
			return t, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	types, _ := filepath.Glob(filepath.Join(sys, "class", "thermal", "thermal_zone*", "type"))
	for _, tp := range types {
		if ty, err := readSmall(tp); err != nil || strings.TrimSpace(ty) != "x86_pkg_temp" {
			continue
		}
		if t, err := readMilliC(filepath.Join(filepath.Dir(tp), "temp")); err == nil {
			return t, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return -1, firstErr
	}
	return -1, errors.New("sensor do pacote não encontrado")
}

// ReadCoreTemps returns the per-core temperatures of coretemp (labels "Core
// N"), sorted by core id; empty when there are none.
func ReadCoreTemps(sys string) []Core {
	out := []Core{}
	for _, dir := range coretempDirs(sys) {
		labels, _ := filepath.Glob(filepath.Join(dir, "temp*_label"))
		for _, lp := range labels {
			l, err := readSmall(lp)
			rest, ok := strings.CutPrefix(strings.TrimSpace(l), "Core ")
			if err != nil || !ok {
				continue
			}
			id, perr := strconv.Atoi(rest)
			if perr != nil || id < 0 {
				continue
			}
			t, err := readMilliC(strings.TrimSuffix(lp, "_label") + "_input")
			if err != nil {
				t = -1
			}
			out = append(out, Core{CoreID: id, TempC: t})
		}
		if len(out) > 0 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CoreID < out[j].CoreID })
	return out
}

// readMilliC reads a sysfs temperature in millidegrees and rejects values
// outside a plausible range.
func readMilliC(path string) (float64, error) {
	txt, err := readSmall(path)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(txt), 10, 64)
	if err != nil {
		return 0, errors.New("temperatura inválida")
	}
	c := float64(n) / 1000
	if c <= 0 || c > 150 {
		return 0, errors.New("temperatura fora da faixa")
	}
	return c, nil
}

// readFreq is the mean "cpu MHz" of /proc/cpuinfo; without it, the mean of
// cpufreq scaling_cur_freq (kHz).
func readFreq(proc, sys string) (float64, error) {
	if txt, err := readSmall(filepath.Join(proc, "cpuinfo")); err == nil {
		var sum float64
		var n int
		for _, line := range strings.Split(txt, "\n") {
			k, v, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(k) != "cpu MHz" {
				continue
			}
			if f, perr := strconv.ParseFloat(strings.TrimSpace(v), 64); perr == nil && f > 0 {
				sum += f
				n++
			}
		}
		if n > 0 {
			return sum / float64(n), nil
		}
	}
	files, _ := filepath.Glob(filepath.Join(sys, "devices", "system", "cpu", "cpu[0-9]*", "cpufreq", "scaling_cur_freq"))
	var sum float64
	var n int
	for _, p := range files {
		txt, err := readSmall(p)
		if err != nil {
			continue
		}
		if k, perr := strconv.ParseFloat(strings.TrimSpace(txt), 64); perr == nil && k > 0 {
			sum += k / 1000
			n++
		}
	}
	if n == 0 {
		return -1, errors.New("frequência indisponível")
	}
	return sum / float64(n), nil
}

// ollamaRSS sums VmRSS (MiB) of the processes whose comm is "ollama". No such
// process gives -1 without an error; an unreadable /proc is an error.
func ollamaRSS(proc string) (int64, error) {
	ents, err := os.ReadDir(proc)
	if err != nil {
		return -1, err
	}
	var sumKB int64
	found := false
	seen := 0
	for _, e := range ents {
		name := e.Name()
		if !isPID(name) {
			continue
		}
		if seen++; seen > maxProcs {
			break
		}
		comm, err := readSmall(filepath.Join(proc, name, "comm"))
		if err != nil || strings.TrimSpace(comm) != "ollama" {
			continue // exited meanwhile or another process
		}
		st, err := readSmall(filepath.Join(proc, name, "status"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(st, "\n") {
			if !strings.HasPrefix(line, "VmRSS:") {
				continue
			}
			f := strings.Fields(line)
			if len(f) >= 2 {
				if kb, perr := strconv.ParseInt(f[1], 10, 64); perr == nil && kb >= 0 {
					sumKB += kb
					found = true
				}
			}
			break
		}
	}
	if !found {
		return -1, nil
	}
	return sumKB / 1024, nil
}

func isPID(s string) bool {
	if s == "" || len(s) > 10 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readRAPL reads the energy counter (µJ) of the package domain
// (intel-rapl:N, not the sub-domains intel-rapl:N:M), and the range where it
// wraps (0 when unknown).
func readRAPL(sys string) (energy, maxRange int64, err error) {
	dirs, _ := filepath.Glob(filepath.Join(sys, "class", "powercap", "intel-rapl:[0-9]*"))
	var firstErr error
	for _, d := range dirs {
		if strings.Count(filepath.Base(d), ":") > 1 {
			continue
		}
		txt, rerr := readSmall(filepath.Join(d, "energy_uj"))
		if rerr != nil {
			if firstErr == nil {
				firstErr = rerr
			}
			continue
		}
		e, perr := strconv.ParseInt(strings.TrimSpace(txt), 10, 64)
		if perr != nil || e < 0 {
			continue
		}
		var m int64
		if mt, merr := readSmall(filepath.Join(d, "max_energy_range_uj")); merr == nil {
			m, _ = strconv.ParseInt(strings.TrimSpace(mt), 10, 64)
		}
		return e, m, nil
	}
	if firstErr != nil {
		return 0, 0, firstErr
	}
	return 0, 0, errors.New("RAPL ausente")
}
