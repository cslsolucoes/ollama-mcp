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

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/cpu"
	"github.com/cslsoftwares/mcpollama/gateway/gpu"
	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/joblog"
	"github.com/cslsoftwares/mcpollama/gateway/models"
	"github.com/cslsoftwares/mcpollama/gateway/monitor"
	"github.com/cslsoftwares/mcpollama/gateway/pool"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
	"github.com/cslsoftwares/mcpollama/gateway/treino"
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

	// HMAC key of the training data (Fase 12, C-4.3). Optional: without it the
	// gateway works as before, keeping no doc_hash and no hash in image markers.
	// A key that is named but cannot be used (bad mode, too short, not hex) is a
	// refusal to start, never a silent fallback to "no key". The key is not logged.
	var docKey *hmackey.Key
	if cfg.DocKeyFile != "" {
		docKey, err = hmackey.Load(cfg.DocKeyFile)
		if err != nil {
			fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
			return 2
		}
		logf("chave HMAC dos dados de treino carregada (key_id %s)", docKey.ID())
	} else {
		logf("sem --doc-key-file: doc_hash não é gravado e o marcador de imagem não leva hash")
	}

	bootID := store.BootID("")
	st, err := store.Open(cfg.DB, store.Options{
		BootID: bootID, Logf: logf, ReqKeep: cfg.RetentionReq, GPUKeep: cfg.RetentionGPU, JobGPUKeep: cfg.RetentionJobGP, MaxRequests: int64(cfg.MaxReqRows),
	})
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 1
	}
	defer st.Close()

	// Job log (Fase 10.L): the text goes to a database of its own, never to --db.
	var cs *content.Store
	if cfg.LogContent {
		cs, err = content.Open(cfg.ContentDB, content.Options{
			MaxBytes: cfg.ContentMax, MaxJobBytes: cfg.ContentMaxJob, Keep: cfg.RetentionCont, Logf: logf,
			OnEvent: st.Event,
		})
		if err != nil {
			// A corrupt or unreadable content.db must not take the gateway down
			// (revisão 10.L, R2): it goes on without the text and says so in the
			// log and in an event. The file is left as it is, for inspection.
			cs = nil
			logf("conteúdo dos jobs DESLIGADO: %v (o gateway segue sem gravar prompt/resposta)", err)
			st.Event("conteudo_indisponivel", "content.db não abriu; o gateway segue sem gravar o texto: "+err.Error())
		} else {
			defer cs.Close()
		}
	} else {
		logf("--log-content=false: o texto dos jobs não é gravado")
	}

	// Training database (Fase 12.4). Optional: without --treino-db the route
	// POST /api/feedback answers 503. Like the job text, a database that does not
	// open must not take the gateway down: it goes on without the feature.
	var tr *treino.Store
	if cfg.TreinoDB != "" {
		tr, err = treino.Open(cfg.TreinoDB, treino.Options{Logf: logf, OnEvent: st.Event, Key: docKey})
		if err != nil {
			tr = nil
			logf("registro de erros DESLIGADO: %v (POST /api/feedback responde 503)", err)
			st.Event("treino_indisponivel", "treino.db não abriu; o gateway segue sem o registro de erros: "+err.Error())
		} else {
			defer tr.Close()
			logf("registro de erros ligado (treino.db, role %s)", cfg.Role)
		}
	} else {
		logf("sem --treino-db: o registro de erros (POST /api/feedback) está desligado")
	}
	jr := &joblog.Journal{Store: st, Content: cs, Backend: pool.DefaultPrimaryID, Logf: logf}
	jr.RecoverResponses(context.Background())
	if rec := st.Recovered(); len(rec) > 0 {
		logf("%d job(s) do processo anterior ficaram em andamento e foram marcados como interrompidos", len(rec))
	}

	hub := monitor.NewHub()
	var mon *monitor.Handler
	var q *queue.Manager

	// The Ollama is asked directly (loopback), outside the queue and the proxy.
	// The pool drives this collector (its first reading is also the health check
	// of the primary), so the "Modelos" board and the health share one reading.
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

	// Backend pool (Fase 10.4). Without --backends it is the single gpu-local.
	var specs []pool.Spec
	if cfg.Backends != "" {
		specs, err = pool.LoadFile(cfg.Backends)
		if err != nil {
			fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
			return 2
		}
	}
	secs, err := pool.Build(specs, cfg.AllowedModels, cfg.ThinkSuffix, 3*time.Second, logf)
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 2
	}
	pl := pool.New(pool.Config{
		PrimarySlots: cfg.GenSlots, PrimaryURL: cfg.Ollama, PrimaryCollector: mcol, Secondaries: secs,
		QueueMax: cfg.QueueMax, OverloadWait: cfg.OverloadWait, OverloadQueue: cfg.OverloadQueue, Hold: cfg.OverloadHold,
		CheckEvery: cfg.HealthEvery, BreakerFails: cfg.BreakerFails, BreakerOpen: cfg.BreakerOpen, Logf: dbg,
		OnEvent: func(kind, detail string) { st.Event(kind, detail) },
		OnChange: func() {
			hub.Notify()
			if q != nil {
				q.Poke()
			}
		},
	})
	for _, b := range secs {
		logf("backend secundário %s (%s) em %s, %d slot(s)%s", b.ID, b.Tipo, b.Host(), b.Slots, map[bool]string{true: ", com token próprio", false: ""}[b.Token() != ""])
	}

	q = queue.New(queue.Config{
		Pool:     pl,
		GenSlots: cfg.GenSlots, EmbedSlots: cfg.EmbedSlots, QueueMax: cfg.QueueMax,
		PerClient: cfg.PerClient, PerIP: cfg.PerIP, LabelsPerIP: cfg.LabelsPerIP,
		MaxWait: cfg.MaxWait, AgeAfter: cfg.AgeAfter, StartStagger: cfg.StartStagger,
		OnChange: hub.Notify,
		OnRecord: func(r queue.Record) {
			jr.Finish(r) // synchronous and durable (the async writer stays as the fallback)
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
	jr.Running = q.RunningIDs
	var ccol *cpu.Collector
	var col *gpu.Collector
	if cfg.GPUInterval > 0 {
		col = &gpu.Collector{
			Cmd: cfg.NvidiaSMI, AERPath: cfg.AERPath, Interval: cfg.GPUInterval, BootID: bootID, Logf: logf,
			OnSamples: func(ts time.Time, ss []gpu.Sample, aer int64) {
				// Every second, while a job runs, one durable row per GPU and job.
				var cpuNow *cpu.Sample
				if ccol != nil {
					l := ccol.Latest()
					cpuNow = &l
				}
				jr.Sample(ts, ss, cpuNow)
				if !due(&lastGPU, ts) {
					return
				}
				rows := make([]store.GPURow, 0, len(ss))
				for _, s := range ss {
					rows = append(rows, store.GPURow{TS: ts, GPU: s.Index, BusID: s.BusID, PowerW: s.PowerW, MemUsedMiB: s.MemUsedMiB, UtilPct: s.UtilPct, TempC: s.TempC, AER: aer, PowerLimitW: s.PowerLimitW, ClockMHz: s.ClockMHz})
				}
				st.GPU(rows)
			},
		}
	}

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

	mon, err = monitor.New(monitor.Config{Pool: pl, Models: mcol, Queue: q, Store: st, Content: cs, Treino: tr, TreinoSource: treino.NewSource(st, cs), TreinoKey: docKey, Role: cfg.Role, KeepDone: cfg.KeepDone, GPU: col, CPU: ccol, StreamEvery: streamEvery(cfg), Hub: hub, Admin: admin, Version: Version, BootID: bootID, Logf: logf})
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-gateway: "+err.Error())
		return 1
	}
	px, err := proxy.New(proxy.Config{
		Journal: jr, Backend: pool.DefaultPrimaryID, Pool: pl, DocKey: docKey,
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
	// The feedback route is registered before the catch-all "/" (which is the proxy).
	mux.Handle("/api/feedback", &treino.Handler{Store: tr, Source: treino.NewSource(st, cs), Key: docKey, Role: cfg.Role, Logf: logf})
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
	if cs != nil {
		start(func() { cs.RunMaintenance(bg, time.Hour) })
	}
	start(func() { sweepLoop(bg, q) })
	if col != nil {
		start(func() { col.RunLoop(bg) })
	}
	if ccol != nil {
		start(func() { ccol.RunLoop(bg) })
	}
	start(func() { pl.Run(bg) })
	start(func() { pokeLoop(bg, q) })
	start(func() { backendSamples(bg, pl, q, st, cfg.SampleStore) })

	logf("mcpollama-gateway %s em %s -> Ollama %s (geracao=%d embed=%d fila=%d por-ip=%d) boot=%s",
		Version, ln.Addr(), cfg.Ollama.Host, cfg.GenSlots, cfg.EmbedSlots, cfg.QueueMax, cfg.PerIP, bootID)
	ids := []string{pool.DefaultPrimaryID}
	for _, b := range secs {
		ids = append(ids, b.ID)
	}
	st.Event("gateway_pool", fmt.Sprintf("backends=%s overload_wait=%s overload_queue=%d hold=%s health=%s breaker=%d/%s",
		strings.Join(ids, ","), cfg.OverloadWait, cfg.OverloadQueue, cfg.OverloadHold, cfg.HealthEvery, cfg.BreakerFails, cfg.BreakerOpen))
	st.Event("gateway_config", fmt.Sprintf("gen=%d embed=%d queue=%d per_client=%d per_ip=%d labels=%d stagger=%s max_wait=%s run_timeout=%s max_rows=%d log_content=%v content_max=%d content_max_job=%d ret_content=%s ret_job_gpu=%s default_think=%v think_suffix=%q allow_keepalive=%v allow_num_ctx=%v strip=%s",
		cfg.GenSlots, cfg.EmbedSlots, cfg.QueueMax, cfg.PerClient, cfg.PerIP, cfg.LabelsPerIP, cfg.StartStagger, cfg.MaxWait, cfg.RunTimeout, cfg.MaxReqRows,
		cfg.LogContent, cfg.ContentMax, cfg.ContentMaxJob, cfg.RetentionCont, cfg.RetentionJobGP,
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

// pokeLoop wakes the dispatcher every second: the hysteresis of the overload and
// the open breaker depend on time, not only on queue events.
func pokeLoop(ctx context.Context, q *queue.Manager) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			q.Poke()
		}
	}
}

// backendSamples writes one row per backend every period into SQLite.
func backendSamples(ctx context.Context, pl *pool.Pool, q *queue.Manager, st *store.Store, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			sts := q.PoolStatus()
			rows := make([]store.BackendRow, 0, len(sts))
			for _, s := range sts {
				rows = append(rows, store.BackendRow{TS: now, BackendID: s.ID, State: s.State, SlotsUsed: s.Used, Queue: s.Queue, P50MS: s.P50MS})
			}
			st.Backends(rows)
		}
	}
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
