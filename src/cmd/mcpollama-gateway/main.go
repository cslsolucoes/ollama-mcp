// Command mcpollama-gateway is the queue and monitor in front of the CSL
// Ollama (Fase 9). It listens on the loopback behind the nginx, puts
// generation and embedding calls in a fair line, keeps a metadata-only history
// in SQLite and serves /monitor.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/cpu"
	"github.com/cslsoftwares/mcpollama/gateway/gpu"
	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/gateway/monitor"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// Version is overridden at build time:
// go build -ldflags "-X main.Version=1.2.3"
var Version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdin, os.Stdout, os.Stderr, nil))
}

// run is main with injectable I/O. ready, when set, receives the bound address.
func run(ctx context.Context, args []string, getenv func(string) string, stdin io.Reader, stdout, stderr io.Writer, ready func(addr string)) int {
	if len(args) > 0 && args[0] == "hash-password" {
		return hashPassword(args[1:], stdin, stdout, stderr)
	}
	cfg, err := parseConfig(args, getenv, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err == nil && !cfg.ShowVersion {
		err = cfg.validate()
	}
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 2
	}
	if cfg.ShowVersion {
		fmt.Fprintf(stdout, "mcpollama-gateway version %s\n", Version)
		return 0
	}
	lg := log.New(&syncWriter{w: stderr}, "", log.LstdFlags)
	logf := func(format string, a ...any) { lg.Printf(format, a...) }
	var dbg func(string, ...any)
	if cfg.Debug {
		dbg = logf
	}

	var admin *monitor.Credentials
	if cfg.AdminFile != "" {
		admin, err = monitor.LoadAdminFile(cfg.AdminFile)
		if err != nil {
			fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
			return 2
		}
	} else {
		logf("sem --admin-file: o monitor é somente leitura")
	}

	bootID := store.BootID("")
	st, err := store.Open(cfg.DB, store.Options{
		BootID: bootID, Logf: logf, ReqKeep: cfg.RetentionReq, GPUKeep: cfg.RetentionGPU, MaxRequests: int64(cfg.MaxReqRows),
	})
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 1
	}
	defer st.Close()

	hub := monitor.NewHub()
	var mon *monitor.Handler
	q := queue.New(queue.Config{
		GenSlots: cfg.GenSlots, EmbedSlots: cfg.EmbedSlots, QueueMax: cfg.QueueMax,
		PerClient: cfg.PerClient, PerIP: cfg.PerIP, LabelsPerIP: cfg.LabelsPerIP,
		MaxWait: cfg.MaxWait, AgeAfter: cfg.AgeAfter, StartStagger: cfg.StartStagger,
		OnChange: hub.Notify,
		OnRecord: func(r queue.Record) {
			st.Record(r)
			if mon != nil {
				mon.Observe(r)
			}
		},
	})

	// The live state gets every sample; SQLite only one per SampleStore, so a
	// 1 s sampling does not write 4 GPU rows and 1 CPU row a second. The two
	// callbacks run on their collector goroutine, so each has its own clock.
	var lastGPU, lastCPU time.Time
	due := func(last *time.Time, ts time.Time) bool {
		if !last.IsZero() && ts.Sub(*last) < cfg.SampleStore-cfg.GPUInterval/2 { // half a tick of tolerance for jitter
			return false
		}
		*last = ts
		return true
	}
	var col *gpu.Collector
	if cfg.GPUInterval > 0 {
		col = &gpu.Collector{
			Cmd: cfg.NvidiaSMI, AERPath: cfg.AERPath, Interval: cfg.GPUInterval, BootID: bootID, Logf: logf,
			OnSamples: func(ts time.Time, ss []gpu.Sample, aer int64) {
				if !due(&lastGPU, ts) {
					return
				}
				rows := make([]store.GPURow, 0, len(ss))
				for _, s := range ss {
					rows = append(rows, store.GPURow{TS: ts, GPU: s.Index, BusID: s.BusID, PowerW: s.PowerW, MemUsedMiB: s.MemUsedMiB, UtilPct: s.UtilPct, TempC: s.TempC, AER: aer, PowerLimitW: s.PowerLimitW})
				}
				st.GPU(rows)
			},
		}
	}

	var ccol *cpu.Collector
	if cfg.GPUInterval > 0 {
		ccol = &cpu.Collector{
			ProcRoot: cfg.ProcRoot, SysRoot: cfg.SysRoot, Interval: cfg.GPUInterval, BootID: bootID, Logf: logf,
			OnSample: func(s cpu.Sample) {
				if !due(&lastCPU, s.At) {
					return
				}
				th := make([][3]float64, 0, len(s.Threads))
				for _, t := range s.Threads {
					th = append(th, [3]float64{float64(t.ID), round1(t.UtilPct), float64(t.CoreID)})
				}
				cs := make([][2]float64, 0, len(s.Cores))
				for _, c := range s.Cores {
					cs = append(cs, [2]float64{float64(c.CoreID), round1(c.TempC)})
				}
				st.CPU(store.CPURow{
					TS: s.At, UtilPct: s.UtilPct, Load1: s.Load1, Load5: s.Load5, Load15: s.Load15,
					RAMTotalMiB: s.RAMTotalMiB, RAMUsedMiB: s.RAMUsedMiB, RAMAvailMiB: s.RAMAvailMiB,
					SwapUsedMiB: s.SwapUsedMiB, SwapTotalMiB: s.SwapTotalMiB, TempC: s.TempC, FreqMHz: s.FreqMHz,
					OllamaRSSMiB: s.OllamaRSSMiB, PowerW: s.PowerW, Threads: store.EncodeThreads(th, cs),
				})
			},
		}
	}

	// The Ollama is asked directly (loopback), outside the queue and the proxy.
	mcol := &models.Collector{
		Upstream: cfg.Ollama, Allowed: cfg.AllowedModels, ThinkSuffix: cfg.ThinkSuffix, Logf: logf,
		OnChange: func(loaded, unloaded []string) {
			for _, n := range loaded {
				st.Event("modelo_carregado", n)
			}
			for _, n := range unloaded {
				st.Event("modelo_descarregado", n)
			}
		},
	}

	mon, err = monitor.New(monitor.Config{Models: mcol, Queue: q, Store: st, GPU: col, CPU: ccol, StreamEvery: streamEvery(cfg), Hub: hub, Admin: admin, Version: Version, BootID: bootID, Logf: logf})
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 1
	}
	px, err := proxy.New(proxy.Config{
		Upstream: cfg.Ollama, Queue: q, PriorityNets: cfg.PriorityNets, AllowedModels: cfg.AllowedModels,
		MaxBodyBytes: cfg.MaxBodyBytes, RunTimeout: cfg.RunTimeout, Logf: dbg,
		DefaultThink: cfg.DefaultThink, ThinkSuffix: cfg.ThinkSuffix,
		AllowClientKeepAlive: cfg.AllowKeepAlive, AllowClientNumCtx: cfg.AllowNumCtx, StripOptions: cfg.StripOptions,
	})
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 1
	}

	mux := http.NewServeMux()
	mux.Handle("/monitor", mon)
	mux.Handle("/monitor/", mon)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok\n") })
	mux.Handle("/", px)

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 1
	}
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No Read/WriteTimeout: generations and event streams are long-lived.
		ErrorLog: lg,
	}
	srv.RegisterOnShutdown(mon.Close) // open /monitor panels must not hold the shutdown

	bg, cancelBG := context.WithCancel(ctx)
	var wg sync.WaitGroup
	start := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	start(func() { mon.Start(bg) })
	start(func() { st.RunMaintenance(bg, time.Hour) })
	start(func() { sweepLoop(bg, q) })
	if col != nil {
		start(func() { col.RunLoop(bg) })
	}
	if ccol != nil {
		start(func() { ccol.RunLoop(bg) })
	}
	start(func() { mcol.RunLoop(bg) })

	logf("mcpollama-gateway %s em %s -> Ollama %s (geracao=%d embed=%d fila=%d por-ip=%d) boot=%s",
		Version, ln.Addr(), cfg.Ollama.Host, cfg.GenSlots, cfg.EmbedSlots, cfg.QueueMax, cfg.PerIP, bootID)
	st.Event("gateway_config", fmt.Sprintf("gen=%d embed=%d queue=%d per_client=%d per_ip=%d labels=%d stagger=%s max_wait=%s run_timeout=%s max_rows=%d default_think=%v think_suffix=%q allow_keepalive=%v allow_num_ctx=%v strip=%s",
		cfg.GenSlots, cfg.EmbedSlots, cfg.QueueMax, cfg.PerClient, cfg.PerIP, cfg.LabelsPerIP, cfg.StartStagger, cfg.MaxWait, cfg.RunTimeout, cfg.MaxReqRows,
		cfg.DefaultThink, cfg.ThinkSuffix, cfg.AllowKeepAlive, cfg.AllowNumCtx, strings.Join(cfg.StripOptions, ",")))
	if ready != nil {
		ready(ln.Addr().String())
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	code := 0
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logf("servidor: %v", err)
			code = 1
		}
	}
	logf("encerrando")
	sctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close() // open generations are cut after the grace period
	}
	cancelBG()
	wg.Wait()
	st.Event("gateway_parada", "")
	st.Flush()
	return code
}

func sweepLoop(ctx context.Context, q *queue.Manager) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			q.Sweep(10 * time.Minute)
		}
	}
}

// hashPassword implements `mcpollama-gateway hash-password`: it prints the
// credentials line for --admin-file. The password comes from stdin, never
// from the command line.
func hashPassword(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hash-password", flag.ContinueOnError)
	fs.SetOutput(stderr)
	user := fs.String("user", "admin", "user name")
	iter := fs.Int("iterations", monitor.DefaultIterations, "PBKDF2 iterations")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	pw, err := bufio.NewReader(io.LimitReader(stdin, 1024)).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintln(stderr, "hash-password: "+err.Error())
		return 1
	}
	pw = strings.TrimRight(pw, "\r\n")
	if strings.ContainsAny(*user, ":\r\n") || *user == "" {
		fmt.Fprintln(stderr, "hash-password: invalid user name")
		return 2
	}
	h, err := monitor.HashPassword(pw, *iter)
	if err != nil {
		fmt.Fprintln(stderr, "hash-password: "+err.Error())
		return 2
	}
	fmt.Fprintf(stdout, "%s:%s\n", *user, h)
	return 0
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// round1 keeps one decimal in the stored per-thread detail.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// streamEvery is the periodic push of the monitor stream: the sampling
// interval, or 2 s when sampling is off.
func streamEvery(cfg Config) time.Duration {
	if cfg.GPUInterval > 0 {
		return cfg.GPUInterval
	}
	return 2 * time.Second
}
