// Package models lists what the Ollama behind the gateway has loaded
// (/api/ps) and installed (/api/tags) for the monitor "Modelos" board. The
// reads are two GETs to the Ollama itself, on a timer and outside the request
// path: the monitor only ever reads the cached Snapshot, so any number of
// viewers costs no query. Nothing here changes the Ollama.
package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// maxBody bounds one answer of the Ollama.
const maxBody = 4 << 20

// Loaded is a model resident in memory (/api/ps).
type Loaded struct {
	Name          string `json:"name"`
	SizeVRAMMiB   int64  `json:"size_vram_mib"`
	SizeMiB       int64  `json:"size_mib"`
	Processor     string `json:"processor"` // "100% GPU", "100% CPU" or "30%/70% CPU/GPU"
	ContextLength int    `json:"context_length"`
	ExpiresAt     string `json:"expires_at"`
	Allowed       bool   `json:"allowed"`
}

// Installed is a model on the Ollama disk (/api/tags).
type Installed struct {
	Name          string `json:"name"`
	SizeMiB       int64  `json:"size_mib"`
	ParameterSize string `json:"parameter_size"`
	Quantization  string `json:"quantization"`
	Family        string `json:"family"`
	ModifiedAt    string `json:"modified_at"`
	Allowed       bool   `json:"allowed"`
}

// Snapshot is the latest reading. A list whose query failed is empty and the
// reason is in Err.
type Snapshot struct {
	At          time.Time   `json:"at"`
	Loaded      []Loaded    `json:"loaded"`
	Installed   []Installed `json:"installed"`
	Allowed     []string    `json:"allowed"`      // --allowed-models; empty = any model
	AllowAll    bool        `json:"allow_all"`    // no --allowed-models
	ThinkSuffix string      `json:"think_suffix"` // "<model><suffix>" is a virtual name that turns reasoning on
	Err         string      `json:"error,omitempty"`
}

// Collector polls the Ollama on a fixed interval.
type Collector struct {
	Upstream    *url.URL // the Ollama (scheme and host are used)
	Allowed     []string
	ThinkSuffix string
	Interval    time.Duration // default 5 s
	Timeout     time.Duration // per query, default 3 s
	Client      *http.Client  // default: a client with Timeout
	// OnChange receives the models that appeared in and left the loaded set
	// since the previous successful /api/ps (never for the first reading).
	OnChange func(loaded, unloaded []string)
	Logf     func(format string, args ...any)

	mu     sync.Mutex
	latest Snapshot

	runMu    sync.Mutex // one round at a time
	prev     map[string]bool
	havePrev bool
	lastErr  string
}

func (c *Collector) defaults() {
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.Timeout <= 0 {
		c.Timeout = 3 * time.Second
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: c.Timeout}
	}
}

// Latest returns the cached reading (a copy). It never queries the Ollama.
func (c *Collector) Latest() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.latest
	// non-nil slices: the JSON must say [] before the first reading, not null
	s.Loaded = append([]Loaded{}, s.Loaded...)
	s.Installed = append([]Installed{}, s.Installed...)
	s.Allowed = append([]string{}, s.Allowed...)
	return s
}

// IsAllowed applies the rule of the proxy: no list = any model; otherwise the
// name (":latest" ignored) must be in the list.
func IsAllowed(allowed []string, name string) bool {
	if len(allowed) == 0 {
		return true
	}
	n := strings.TrimSuffix(name, ":latest")
	for _, m := range allowed {
		if strings.TrimSuffix(m, ":latest") == n {
			return true
		}
	}
	return false
}

type details struct {
	Family        string `json:"family"`
	ParameterSize string `json:"parameter_size"`
	Quantization  string `json:"quantization_level"`
}

type psAnswer struct {
	Models []struct {
		Name          string  `json:"name"`
		Model         string  `json:"model"`
		Size          int64   `json:"size"`
		SizeVRAM      int64   `json:"size_vram"`
		ContextLength int     `json:"context_length"`
		ExpiresAt     string  `json:"expires_at"`
		Details       details `json:"details"`
	} `json:"models"`
}

type tagsAnswer struct {
	Models []struct {
		Name       string  `json:"name"`
		Model      string  `json:"model"`
		Size       int64   `json:"size"`
		ModifiedAt string  `json:"modified_at"`
		Details    details `json:"details"`
	} `json:"models"`
}

// Processor describes where a loaded model sits, from size_vram and size, as
// "ollama ps" does.
func Processor(size, vram int64) string {
	switch {
	case size <= 0:
		return "-"
	case vram <= 0:
		return "100% CPU"
	case vram >= size:
		return "100% GPU"
	}
	cpu := int((size - vram) * 100 / size)
	return fmt.Sprintf("%d%%/%d%% CPU/GPU", cpu, 100-cpu)
}

func mib(b int64) int64 {
	if b < 0 {
		return 0
	}
	return b >> 20
}

func (c *Collector) get(ctx context.Context, path string, into any) error {
	u := *c.Upstream
	u.Path, u.RawPath, u.RawQuery, u.Fragment = path, "", "", ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(b) > maxBody {
		return errors.New("resposta grande demais")
	}
	return json.Unmarshal(b, into)
}

func short(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// Once takes one reading. Not reentrant: a second call waits for the first.
func (c *Collector) Once(ctx context.Context) Snapshot {
	c.defaults()
	c.runMu.Lock()
	defer c.runMu.Unlock()
	snap := Snapshot{
		At: time.Now(), Loaded: []Loaded{}, Installed: []Installed{},
		Allowed: append([]string{}, c.Allowed...), AllowAll: len(c.Allowed) == 0, ThinkSuffix: c.ThinkSuffix,
	}
	var errs []string

	var ps psAnswer
	psOK := true
	if err := c.get(ctx, "/api/ps", &ps); err != nil {
		psOK = false
		errs = append(errs, "ps: "+short(err))
	} else {
		for _, m := range ps.Models {
			name := m.Name
			if name == "" {
				name = m.Model
			}
			snap.Loaded = append(snap.Loaded, Loaded{
				Name: name, SizeVRAMMiB: mib(m.SizeVRAM), SizeMiB: mib(m.Size),
				Processor: Processor(m.Size, m.SizeVRAM), ContextLength: m.ContextLength,
				ExpiresAt: m.ExpiresAt, Allowed: IsAllowed(c.Allowed, name),
			})
		}
	}
	var tags tagsAnswer
	if err := c.get(ctx, "/api/tags", &tags); err != nil {
		errs = append(errs, "tags: "+short(err))
	} else {
		for _, m := range tags.Models {
			name := m.Name
			if name == "" {
				name = m.Model
			}
			snap.Installed = append(snap.Installed, Installed{
				Name: name, SizeMiB: mib(m.Size), ParameterSize: m.Details.ParameterSize,
				Quantization: m.Details.Quantization, Family: m.Details.Family,
				ModifiedAt: m.ModifiedAt, Allowed: IsAllowed(c.Allowed, name),
			})
		}
	}
	snap.Err = strings.Join(errs, "; ")

	var added, removed []string
	if psOK {
		cur := map[string]bool{}
		for _, l := range snap.Loaded {
			cur[l.Name] = true
		}
		if c.havePrev {
			for n := range cur {
				if !c.prev[n] {
					added = append(added, n)
				}
			}
			for n := range c.prev {
				if !cur[n] {
					removed = append(removed, n)
				}
			}
			sort.Strings(added)
			sort.Strings(removed)
		}
		c.prev, c.havePrev = cur, true
	}

	c.mu.Lock()
	c.latest = snap
	c.mu.Unlock()
	if (len(added) > 0 || len(removed) > 0) && c.OnChange != nil {
		c.OnChange(added, removed)
	}
	// Log only when the error text changes: a down Ollama would fill the journal.
	if snap.Err != c.lastErr && snap.Err != "" && c.Logf != nil {
		c.Logf("models: %s", snap.Err)
	}
	c.lastErr = snap.Err
	return snap
}

// RunLoop polls until ctx ends, first reading at once. Rounds never overlap,
// and a tick that came due during a slow round is dropped.
func (c *Collector) RunLoop(ctx context.Context) {
	c.defaults()
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for {
		start := time.Now()
		c.Once(ctx)
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
