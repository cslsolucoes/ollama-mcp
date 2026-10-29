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

// DefaultArgs is the fixed nvidia-smi query.
var DefaultArgs = []string{
	"--query-gpu=index,pci.bus_id,power.draw,memory.used,utilization.gpu,temperature.gpu",
	"--format=csv,noheader,nounits",
}

// Sample is one GPU at one instant.
type Sample struct {
	Index      int       `json:"index"`
	BusID      string    `json:"bus_id"`
	PowerW     float64   `json:"power_w"`
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
	out, err := c.Run(cctx, c.Cmd, c.Args...)
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
		c.Once(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// ParseCSV parses the nvidia-smi query output (csv, noheader, nounits). A
// field the driver cannot report ("[N/A]", "N/A") becomes 0.
func ParseCSV(out string, at time.Time) ([]Sample, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(out)))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = 6
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("saída inesperada: %w", err)
	}
	if len(recs) == 0 {
		return nil, errors.New("saída vazia")
	}
	var res []Sample
	for _, f := range recs {
		idx, err := strconv.Atoi(strings.TrimSpace(f[0]))
		if err != nil {
			return nil, fmt.Errorf("índice de GPU inválido %q", f[0])
		}
		res = append(res, Sample{
			Index: idx, BusID: strings.TrimSpace(f[1]), At: at,
			PowerW:     num(f[2]),
			MemUsedMiB: int(num(f[3])),
			UtilPct:    int(num(f[4])),
			TempC:      int(num(f[5])),
		})
	}
	return res, nil
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
