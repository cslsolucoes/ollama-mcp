// Package gpu samples the server GPUs (Fase 9.3): nvidia-smi every few seconds
// plus the correctable-AER counter of the PCIe bridge that had the incidents
// (00:1b.4, plan sections 4.1 and 5). Read only; nothing here changes the host.
package gpu

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAERPath is the correctable-error counter of the bridge 00:1b.4.
const DefaultAERPath = "/sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable"

const baseFields = "index,pci.bus_id,power.draw,memory.used,utilization.gpu,temperature.gpu"

// DefaultArgs is the fixed nvidia-smi query: the base fields plus the power
// cap in force (enforced.power.limit), the card ceiling (power.max_limit) and
// the current graphics clock (clocks.gr, Fase 10.L: it proves the clock lock).
var DefaultArgs = []string{
	"--query-gpu=" + baseFields + ",enforced.power.limit,power.max_limit,clocks.gr",
	"--format=csv,noheader,nounits",
}

// fallbackArgs are tried, in order, when a driver rejects the query above
// (an unknown field makes nvidia-smi fail as a whole): power.limit instead of
// enforced.power.limit; then the same two without the clock; then no limit
// columns at all.
var fallbackArgs = [][]string{
	{"--query-gpu=" + baseFields + ",power.limit,power.max_limit,clocks.gr", "--format=csv,noheader,nounits"},
	{"--query-gpu=" + baseFields + ",enforced.power.limit,power.max_limit", "--format=csv,noheader,nounits"},
	{"--query-gpu=" + baseFields + ",power.limit,power.max_limit", "--format=csv,noheader,nounits"},
	{"--query-gpu=" + baseFields, "--format=csv,noheader,nounits"},
}

// Sample is one GPU at one instant.
type Sample struct {
	Index  int     `json:"index"`
	BusID  string  `json:"bus_id"`
	PowerW float64 `json:"power_w"`
	// PowerLimitW is the power cap in force and PowerMaxW the card ceiling;
	// -1 when the driver does not report them.
	PowerLimitW float64 `json:"power_limit_w"`
	PowerMaxW   float64 `json:"power_max_w"`
	// ClockMHz is the current graphics clock (clocks.gr); -1 when unreported.
	ClockMHz   float64   `json:"clock_mhz"`
	MemUsedMiB int       `json:"mem_used_mib"`
	UtilPct    int       `json:"util_pct"`
	TempC      int       `json:"temp_c"`
	At         time.Time `json:"at"`
}

// Snapshot is the latest reading.
type Snapshot struct {
	At      time.Time `json:"at"`
	Samples []Sample  `json:"samples"`
	AER     int64     `json:"aer_corr_1b4"` // -1 when unreadable
	Err     string    `json:"error,omitempty"`
	BootID  string    `json:"boot_id"`
}

// Runner executes a command and returns its stdout. Injected in tests.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Collector samples on a fixed interval.
type Collector struct {
	Cmd      string        // nvidia-smi path; default "nvidia-smi"
	Args     []string      // default DefaultArgs
	AERPath  string        // default DefaultAERPath; "-" disables
	Interval time.Duration // default 5 s
	Timeout  time.Duration // per nvidia-smi call, default 4 s
	BootID   string
	Run      Runner // default: os/exec
	// OnSamples receives every successful round (outside any lock).
	OnSamples func(ts time.Time, samples []Sample, aer int64)
	Logf      func(format string, args ...any)

	mu     sync.Mutex
	latest Snapshot
	tier   int // 0 = DefaultArgs, 1.. = fallbackArgs[tier-1]; sticky once a query works
}

func (c *Collector) defaults() {
	if c.Cmd == "" {
		c.Cmd = "nvidia-smi"
	}
	if c.Args == nil {
		c.Args = DefaultArgs
	}
	if c.AERPath == "" {
		c.AERPath = DefaultAERPath
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.Timeout <= 0 {
		c.Timeout = 4 * time.Second
	}
	if c.Run == nil {
		c.Run = execRunner
	}
}

// Latest returns the last reading (a copy).
func (c *Collector) Latest() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.latest
	s.Samples = append([]Sample(nil), s.Samples...)
	return s
}

// Once takes one reading.
func (c *Collector) Once(ctx context.Context) Snapshot {
	c.defaults()
	now := time.Now()
	snap := Snapshot{At: now, AER: -1, BootID: c.BootID}
	cctx, cancel := context.WithTimeout(ctx, c.Timeout)
	out, err := c.query(cctx)
	cancel()
	if err != nil {
		snap.Err = "nvidia-smi: " + oneLine(err.Error())
	} else if samples, perr := ParseCSV(string(out), now); perr != nil {
		snap.Err = "nvidia-smi: " + oneLine(perr.Error())
	} else {
		snap.Samples = samples
	}
	if c.AERPath != "-" {
		if v, aerr := ReadAER(c.AERPath); aerr == nil {
			snap.AER = v
		} else if snap.Err == "" {
			snap.Err = "aer: " + oneLine(aerr.Error())
		}
	}
	c.mu.Lock()
	c.latest = snap
	c.mu.Unlock()
	if len(snap.Samples) > 0 && c.OnSamples != nil {
		c.OnSamples(now, snap.Samples, snap.AER)
	}
	if snap.Err != "" && c.Logf != nil {
		c.Logf("gpu: %s", snap.Err)
	}
	return snap
}

// Run samples until ctx ends, first reading at once.
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

// query runs the configured query. With the default query, a failure walks
// down the fallbacks and remembers the first tier that works, so a driver
// without the limit fields costs one extra call, not one per round.
func (c *Collector) query(ctx context.Context) ([]byte, error) {
	if !sameArgs(c.Args, DefaultArgs) {
		return c.Run(ctx, c.Cmd, c.Args...)
	}
	c.mu.Lock()
	tier := c.tier
	c.mu.Unlock()
	try := func(t int) ([]byte, error) {
		if t == 0 {
			return c.Run(ctx, c.Cmd, DefaultArgs...)
		}
		return c.Run(ctx, c.Cmd, fallbackArgs[t-1]...)
	}
	out, err := try(tier)
	if err == nil || ctx.Err() != nil {
		return out, err
	}
	for t := tier + 1; t <= len(fallbackArgs); t++ {
		if o2, e2 := try(t); e2 == nil {
			c.mu.Lock()
			c.tier = t
			c.mu.Unlock()
			return o2, nil
		}
	}
	return out, err
}

func sameArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// ParseCSV parses the nvidia-smi query output (csv, noheader, nounits): 6
// columns, 8 with the power limit and ceiling, or 9 with the graphics clock
// too. A base field the driver
// cannot report ("[N/A]", "N/A") becomes 0; a missing or N/A limit is -1.
func ParseCSV(out string, at time.Time) ([]Sample, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(out)))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("saída inesperada: %w", err)
	}
	if len(recs) == 0 {
		return nil, errors.New("saída vazia")
	}
	var res []Sample
	for _, f := range recs {
		if len(f) != 6 && len(f) != 8 && len(f) != 9 {
			return nil, fmt.Errorf("saída inesperada: %d colunas", len(f))
		}
		idx, err := strconv.Atoi(strings.TrimSpace(f[0]))
		if err != nil {
			return nil, fmt.Errorf("índice de GPU inválido %q", f[0])
		}
		res = append(res, Sample{
			Index: idx, BusID: strings.TrimSpace(f[1]), At: at,
			PowerW:      num(f[2]),
			MemUsedMiB:  int(num(f[3])),
			UtilPct:     int(num(f[4])),
			TempC:       int(num(f[5])),
			PowerLimitW: -1, PowerMaxW: -1, ClockMHz: -1,
		})
		if len(f) >= 8 {
			last := &res[len(res)-1]
			last.PowerLimitW, last.PowerMaxW = numOr(f[6], -1), numOr(f[7], -1)
			if len(f) == 9 {
				last.ClockMHz = numOr(f[8], -1)
			}
		}
	}
	return res, nil
}

// numOr parses a number; "[N/A]", "N/A" and garbage give def.
func numOr(s string, def float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func num(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// ReadAER reads aer_dev_correctable. The file has "NAME count" lines ending
// with "TOTAL_ERR_COR n"; without the total line the counts are summed.
func ReadAER(path string) (int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return -1, err
	}
	var sum int64
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		n, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		if f[0] == "TOTAL_ERR_COR" {
			return n, nil
		}
		sum += n
	}
	return sum, nil
}
