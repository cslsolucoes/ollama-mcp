package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/cpu"
	"github.com/cslsoftwares/mcpollama/gateway/gpu"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
)

const usageText = `Usage: mcpollama-gateway [flags]
       mcpollama-gateway hash-password [-user NAME]    (reads the password from stdin)

Queue and monitor in front of a loopback Ollama. Every flag falls back to its
environment variable; the flag wins.

  --listen ADDR           listen address, loopback only      (MCPGW_LISTEN, default 127.0.0.1:11500)
  --ollama HOST:PORT|URL  the Ollama                         (MCPGW_OLLAMA, default 127.0.0.1:11434)
  --db FILE               SQLite file, created 0600          (MCPGW_DB, default /var/lib/mcpollama-gateway/gateway.db)
  --gen-slots N           concurrent generations             (MCPGW_GEN_SLOTS, default 2)
  --embed-slots N         concurrent embeddings              (MCPGW_EMBED_SLOTS, default 2)
  --queue-max N           waiting requests, all clients      (MCPGW_QUEUE_MAX, default 64)
  --queue-per-client N    waiting per IP+label               (MCPGW_QUEUE_PER_CLIENT, default 16)
  --queue-per-ip N        waiting per IP, all labels         (MCPGW_QUEUE_PER_IP, default = --queue-per-client)
  --labels-per-ip N       distinct labels per IP; more are folded into "<ip>/outros"
                                                             (MCPGW_LABELS_PER_IP, default 16)
  --max-wait D            longest wait in the line, then 504 (MCPGW_MAX_WAIT, default 10m)
  --run-timeout D         longest run of a started request; the slot is freed and the
                          client gets 504 or a final NDJSON error line
                                                             (MCPGW_RUN_TIMEOUT, default = --max-wait)
  --age-after D           a baixa request competes as normal after this wait
                                                             (MCPGW_AGE_AFTER, default 5m)
  --start-stagger D       minimum gap between two generation starts
                                                             (MCPGW_START_STAGGER, default 300ms)
  --priority-ips LIST     CIDRs or IPs (separated by , or ;) allowed to ask for priority alta
                                                             (MCPGW_PRIORITY_IPS, default none)
  --allowed-models LIST   models accepted, separated by ; or ,; empty means any
                                                             (MCPGW_ALLOWED_MODELS)
  --default-think BOOL    reasoning when the client does not say: native API gets "think":BOOL,
                          /v1 gets "reasoning_effort":"none" when false (an explicit field wins)
                                                             (MCPGW_DEFAULT_THINK, default false)
  --think-suffix S        a model named "<model>S" (e.g. qwen3-ia-think) is rewritten to <model>
                          with reasoning ON; "none" disables         (MCPGW_THINK_SUFFIX, default -think)
                          The header X-Mcpollama-Think: on|off sets the reasoning when the body does not
                          (precedence: body > header > suffix > --default-think). The --allowed-models
                          check uses the base name.
  --allow-client-keepalive  let the client's top-level keep_alive through; by default it is removed
                          (a 0 would unload the model for everyone)  (MCPGW_ALLOW_CLIENT_KEEPALIVE, default false)
  --allow-client-num-ctx  let options.num_ctx through; by default it is removed (any value other than
                          the resident context reloads the model)    (MCPGW_ALLOW_CLIENT_NUM_CTX, default false)
  --strip-options LIST    options removed from the body, separated by , or ; ("none" = none)
                                                             (MCPGW_STRIP_OPTIONS, default num_gpu,main_gpu,low_vram,use_mmap,use_mlock,numa)
  --max-body-bytes N      request body ceiling               (MCPGW_MAX_BODY_BYTES, default 33554432)
  --admin-file FILE       "user:pbkdf2-sha256$..." line, mode 0600; without it the monitor is
                          read only                          (MCPGW_ADMIN_FILE)
  --gpu-interval D        nvidia-smi and CPU/RAM sampling period (MCPGW_GPU_INTERVAL, default 1s; 0 disables both)
  --nvidia-smi PATH       nvidia-smi binary                  (MCPGW_NVIDIA_SMI, default nvidia-smi)
  --aer-path FILE         correctable AER counter of the bridge 00:1b.4; "-" disables
                                                             (MCPGW_AER_PATH)
  --proc-root DIR         procfs root read by the CPU/RAM sampler (tests)  (MCPGW_PROC_ROOT, default /proc)
  --sys-root DIR          sysfs root read by the CPU/RAM sampler (tests)   (MCPGW_SYS_ROOT, default /sys)
  --sample-store-interval D  how often a GPU/CPU sample goes to SQLite; the live page gets every sample
                                                             (MCPGW_SAMPLE_STORE_INTERVAL, default 5s; >= --gpu-interval)
  --retention-requests D  requests kept for                  (MCPGW_RETENTION_REQUESTS, default 2160h = 90 days)
  --retention-gpu D       GPU and CPU/RAM samples kept for               (MCPGW_RETENTION_GPU, default 168h = 7 days)
  --max-request-rows N    cap of the request history; the oldest rows go first
                                                             (MCPGW_MAX_REQUEST_ROWS, default 3000000)
  --log-content BOOL      job log with the FULL prompt and answer of every job, in a database of its
                          own (--content-db). Owner decision of 2026-10-30; false keeps only the
                          metadata (the job log, the GPU extract and the interrupted-job recovery stay on)
                                                             (MCPGW_LOG_CONTENT, default true)
  --content-db FILE       SQLite file of the job text, created 0600; the text is never in --db
                                                             (MCPGW_CONTENT_DB, default /var/lib/mcpollama-gateway/content.db)
  --content-max-bytes N   total cap of the stored text; above it the oldest jobs are deleted and an event
                          is written                         (MCPGW_CONTENT_MAX_BYTES, default 2147483648 = 2 GiB)
  --content-max-job-bytes N  cap of the prompt and of the answer of one job, each; above it the text is cut
                          and the job is marked truncated    (MCPGW_CONTENT_MAX_JOB_BYTES, default 16777216 = 16 MiB)
  --monitor-keep-done D   how long a finished request stays visible in the monitor ("Concluídos nos últimos
                          5 min", display only; it holds no slot); 0 hides it  (MCPGW_MONITOR_KEEP_DONE, default 5m)
  --retention-content D   job text kept for                  (MCPGW_RETENTION_CONTENT, default 168h = 7 days)
  --retention-job-gpu D   per-second GPU extract of each job kept for
                                                             (MCPGW_RETENTION_JOB_GPU, default 168h = 7 days)
  --non-loopback-listen   allow --listen outside the loopback (MCPGW_NON_LOOPBACK_LISTEN; not recommended)
  --debug                 log one line per request           (MCPGW_DEBUG)
  --version               print the version and exit
`

// Config is the resolved configuration.
type Config struct {
	Listen         string
	Ollama         *url.URL
	DB             string
	GenSlots       int
	EmbedSlots     int
	QueueMax       int
	PerClient      int
	PerIP          int
	LabelsPerIP    int
	MaxWait        time.Duration
	RunTimeout     time.Duration
	MaxReqRows     int
	AgeAfter       time.Duration
	StartStagger   time.Duration
	PriorityNets   []*net.IPNet
	AllowedModels  []string
	DefaultThink   bool
	ThinkSuffix    string
	AllowKeepAlive bool
	AllowNumCtx    bool
	StripOptions   []string
	MaxBodyBytes   int64
	AdminFile      string
	GPUInterval    time.Duration
	NvidiaSMI      string
	AERPath        string
	SampleStore    time.Duration
	ProcRoot       string
	SysRoot        string
	RetentionReq   time.Duration
	RetentionGPU   time.Duration
	KeepDone       time.Duration
	LogContent     bool
	ContentDB      string
	ContentMax     int64
	ContentMaxJob  int64
	RetentionCont  time.Duration
	RetentionJobGP time.Duration
	NonLoopback    bool
	Debug          bool
	ShowVersion    bool
}

func parseConfig(args []string, getenv func(string) string, errw io.Writer) (Config, error) {
	fs := flag.NewFlagSet("mcpollama-gateway", flag.ContinueOnError)
	fs.SetOutput(errw)
	fs.Usage = func() { fmt.Fprint(errw, usageText) }
	var (
		fListen  = fs.String("listen", "", "")
		fOllama  = fs.String("ollama", "", "")
		fDB      = fs.String("db", "", "")
		fGen     = fs.Int("gen-slots", 0, "")
		fEmbed   = fs.Int("embed-slots", 0, "")
		fQMax    = fs.Int("queue-max", 0, "")
		fPerCl   = fs.Int("queue-per-client", 0, "")
		fPerIP   = fs.Int("queue-per-ip", 0, "")
		fLabels  = fs.Int("labels-per-ip", 0, "")
		fMaxWait = fs.Duration("max-wait", 0, "")
		fRunTO   = fs.Duration("run-timeout", 0, "")
		fMaxRows = fs.Int("max-request-rows", 0, "")
		fAge     = fs.Duration("age-after", 0, "")
		fStagger = fs.Duration("start-stagger", 0, "")
		fPrio    = fs.String("priority-ips", "", "")
		fModels  = fs.String("allowed-models", "", "")
		fThink   = fs.Bool("default-think", false, "")
		fSuffix  = fs.String("think-suffix", "", "")
		fKeepAl  = fs.Bool("allow-client-keepalive", false, "")
		fNumCtx  = fs.Bool("allow-client-num-ctx", false, "")
		fStrip   = fs.String("strip-options", "", "")
		fBody    = fs.Int64("max-body-bytes", 0, "")
		fAdmin   = fs.String("admin-file", "", "")
		fGPUInt  = fs.Duration("gpu-interval", 0, "")
		fSMI     = fs.String("nvidia-smi", "", "")
		fAER     = fs.String("aer-path", "", "")
		fSStore  = fs.Duration("sample-store-interval", 0, "")
		fProc    = fs.String("proc-root", "", "")
		fSys     = fs.String("sys-root", "", "")
		fRetReq  = fs.Duration("retention-requests", 0, "")
		fRetGPU  = fs.Duration("retention-gpu", 0, "")
		fKeepDn  = fs.Duration("monitor-keep-done", 0, "")
		fLogCont = fs.Bool("log-content", true, "")
		fContDB  = fs.String("content-db", "", "")
		fContMax = fs.Int64("content-max-bytes", 0, "")
		fContJob = fs.Int64("content-max-job-bytes", 0, "")
		fRetCont = fs.Duration("retention-content", 0, "")
		fRetJob  = fs.Duration("retention-job-gpu", 0, "")
		fNonLoop = fs.Bool("non-loopback-listen", false, "")
		fDebug   = fs.Bool("debug", false, "")
		fVersion = fs.Bool("version", false, "")
	)
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	var cfgErr error
	fail := func(format string, a ...any) {
		if cfgErr == nil {
			cfgErr = fmt.Errorf(format, a...)
		}
	}
	str := func(name, env, def string, v *string) string {
		if set[name] {
			return *v
		}
		if e := getenv(env); e != "" {
			return e
		}
		return def
	}
	integer := func(name, env string, def int, v *int) int {
		if set[name] {
			return *v
		}
		if e := getenv(env); e != "" {
			n, err := strconv.Atoi(strings.TrimSpace(e))
			if err != nil {
				fail("%s: invalid integer", env)
			}
			return n
		}
		return def
	}
	dur := func(name, env string, def time.Duration, v *time.Duration) time.Duration {
		if set[name] {
			return *v
		}
		if e := getenv(env); e != "" {
			d, err := time.ParseDuration(strings.TrimSpace(e))
			if err != nil {
				fail("%s: invalid duration", env)
			}
			return d
		}
		return def
	}
	boolTrue := func(name, env string, v *bool) bool { // default true: only an explicit "no" turns it off
		if set[name] {
			return *v
		}
		switch strings.ToLower(strings.TrimSpace(getenv(env))) {
		case "0", "false", "no", "off":
			return false
		}
		return true
	}
	int64v := func(name, env string, def int64, v *int64) int64 {
		if set[name] {
			return *v
		}
		if e := getenv(env); e != "" {
			n, err := strconv.ParseInt(strings.TrimSpace(e), 10, 64)
			if err != nil {
				fail("%s: invalid integer", env)
			}
			return n
		}
		return def
	}
	boolean := func(name, env string, v *bool) bool {
		if set[name] {
			return *v
		}
		switch strings.ToLower(strings.TrimSpace(getenv(env))) {
		case "", "0", "false", "no", "off":
			return false
		}
		return true
	}

	c := Config{
		Listen:         str("listen", "MCPGW_LISTEN", "127.0.0.1:11500", fListen),
		DB:             str("db", "MCPGW_DB", "/var/lib/mcpollama-gateway/gateway.db", fDB),
		GenSlots:       integer("gen-slots", "MCPGW_GEN_SLOTS", 2, fGen),
		EmbedSlots:     integer("embed-slots", "MCPGW_EMBED_SLOTS", 2, fEmbed),
		QueueMax:       integer("queue-max", "MCPGW_QUEUE_MAX", 64, fQMax),
		PerClient:      integer("queue-per-client", "MCPGW_QUEUE_PER_CLIENT", 16, fPerCl),
		LabelsPerIP:    integer("labels-per-ip", "MCPGW_LABELS_PER_IP", 16, fLabels),
		MaxWait:        dur("max-wait", "MCPGW_MAX_WAIT", 10*time.Minute, fMaxWait),
		AgeAfter:       dur("age-after", "MCPGW_AGE_AFTER", 5*time.Minute, fAge),
		StartStagger:   dur("start-stagger", "MCPGW_START_STAGGER", 300*time.Millisecond, fStagger),
		AdminFile:      str("admin-file", "MCPGW_ADMIN_FILE", "", fAdmin),
		GPUInterval:    dur("gpu-interval", "MCPGW_GPU_INTERVAL", time.Second, fGPUInt),
		SampleStore:    dur("sample-store-interval", "MCPGW_SAMPLE_STORE_INTERVAL", 5*time.Second, fSStore),
		NvidiaSMI:      str("nvidia-smi", "MCPGW_NVIDIA_SMI", "nvidia-smi", fSMI),
		AERPath:        str("aer-path", "MCPGW_AER_PATH", gpu.DefaultAERPath, fAER),
		ProcRoot:       str("proc-root", "MCPGW_PROC_ROOT", cpu.DefaultProcRoot, fProc),
		SysRoot:        str("sys-root", "MCPGW_SYS_ROOT", cpu.DefaultSysRoot, fSys),
		RetentionReq:   dur("retention-requests", "MCPGW_RETENTION_REQUESTS", 90*24*time.Hour, fRetReq),
		RetentionGPU:   dur("retention-gpu", "MCPGW_RETENTION_GPU", 7*24*time.Hour, fRetGPU),
		KeepDone:       dur("monitor-keep-done", "MCPGW_MONITOR_KEEP_DONE", 5*time.Minute, fKeepDn),
		LogContent:     boolTrue("log-content", "MCPGW_LOG_CONTENT", fLogCont),
		ContentMax:     int64v("content-max-bytes", "MCPGW_CONTENT_MAX_BYTES", 2<<30, fContMax),
		ContentMaxJob:  int64v("content-max-job-bytes", "MCPGW_CONTENT_MAX_JOB_BYTES", 16<<20, fContJob),
		RetentionCont:  dur("retention-content", "MCPGW_RETENTION_CONTENT", 7*24*time.Hour, fRetCont),
		RetentionJobGP: dur("retention-job-gpu", "MCPGW_RETENTION_JOB_GPU", 7*24*time.Hour, fRetJob),
		NonLoopback:    boolean("non-loopback-listen", "MCPGW_NON_LOOPBACK_LISTEN", fNonLoop),
		Debug:          boolean("debug", "MCPGW_DEBUG", fDebug),
		ShowVersion:    *fVersion,

		DefaultThink:   boolean("default-think", "MCPGW_DEFAULT_THINK", fThink),
		AllowKeepAlive: boolean("allow-client-keepalive", "MCPGW_ALLOW_CLIENT_KEEPALIVE", fKeepAl),
		AllowNumCtx:    boolean("allow-client-num-ctx", "MCPGW_ALLOW_CLIENT_NUM_CTX", fNumCtx),
	}
	c.ThinkSuffix = str("think-suffix", "MCPGW_THINK_SUFFIX", "-think", fSuffix)
	if strings.EqualFold(c.ThinkSuffix, "none") {
		c.ThinkSuffix = ""
	}
	if strings.ContainsAny(c.ThinkSuffix, " \t\r\n\"") {
		fail("--think-suffix: must not contain spaces or quotes")
	}
	if strip := strings.TrimSpace(str("strip-options", "MCPGW_STRIP_OPTIONS", "", fStrip)); strip == "" {
		c.StripOptions = append([]string(nil), proxy.DefaultStripOptions...)
	} else if strings.EqualFold(strip, "none") {
		c.StripOptions = []string{}
	} else {
		c.StripOptions = splitList(strip)
	}
	// --queue-per-ip defaults to --queue-per-client.
	c.PerIP = integer("queue-per-ip", "MCPGW_QUEUE_PER_IP", c.PerClient, fPerIP)
	// --run-timeout defaults to --max-wait: a request should not hold a slot
	// longer than anyone is allowed to wait for one (Fase 9.5).
	c.RunTimeout = dur("run-timeout", "MCPGW_RUN_TIMEOUT", c.MaxWait, fRunTO)
	c.MaxReqRows = integer("max-request-rows", "MCPGW_MAX_REQUEST_ROWS", 3_000_000, fMaxRows)
	switch {
	case set["max-body-bytes"]:
		c.MaxBodyBytes = *fBody
	case getenv("MCPGW_MAX_BODY_BYTES") != "":
		n, err := strconv.ParseInt(strings.TrimSpace(getenv("MCPGW_MAX_BODY_BYTES")), 10, 64)
		if err != nil {
			fail("MCPGW_MAX_BODY_BYTES: invalid integer")
		}
		c.MaxBodyBytes = n
	default:
		c.MaxBodyBytes = 32 << 20
	}

	// --content-db defaults to content.db next to --db (/var/lib/mcpollama-gateway by default).
	c.ContentDB = str("content-db", "MCPGW_CONTENT_DB", filepath.Join(filepath.Dir(c.DB), "content.db"), fContDB)
	c.AllowedModels = splitList(str("allowed-models", "MCPGW_ALLOWED_MODELS", "", fModels))
	nets, err := parseNets(str("priority-ips", "MCPGW_PRIORITY_IPS", "", fPrio))
	if err != nil {
		fail("--priority-ips: %v", err)
	}
	c.PriorityNets = nets

	u, err := parseOllama(str("ollama", "MCPGW_OLLAMA", "127.0.0.1:11434", fOllama))
	if err != nil {
		fail("--ollama: %v", err)
	}
	c.Ollama = u
	return c, cfgErr
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseNets reads CIDRs or bare IPs (a bare IP is a /32 or /128). IPv4-mapped
// IPv6 forms are turned into the IPv4 prefix they mean ("::ffff:10.0.0.5" is
// 10.0.0.5/32, "::ffff:10.0.0.0/104" is 10.0.0.0/8): net.ParseCIDR read
// "::ffff:10.0.0.5/32" as ::/32, which matched nothing (Fase 9.5, C1). A
// mapped prefix shorter than /96 and zones are refused.
func parseNets(s string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, p := range splitList(s) {
		var pfx netip.Prefix
		if !strings.Contains(p, "/") {
			a, err := netip.ParseAddr(p)
			if err != nil || a.Zone() != "" {
				return nil, fmt.Errorf("%q is not an IP or CIDR", p)
			}
			a = a.Unmap()
			pfx = netip.PrefixFrom(a, a.BitLen())
		} else {
			x, err := netip.ParsePrefix(p)
			if err != nil {
				return nil, fmt.Errorf("%q is not an IP or CIDR", p)
			}
			if x.Addr().Is4In6() {
				if x.Bits() < 96 {
					return nil, fmt.Errorf("%q: an IPv4-mapped prefix must be /96 or longer", p)
				}
				x = netip.PrefixFrom(x.Addr().Unmap(), x.Bits()-96)
			}
			pfx = x
		}
		pfx = pfx.Masked()
		out = append(out, &net.IPNet{IP: net.IP(pfx.Addr().AsSlice()), Mask: net.CIDRMask(pfx.Bits(), pfx.Addr().BitLen())})
	}
	return out, nil
}

// parseOllama accepts host:port or an http URL.
func parseOllama(s string) (*url.URL, error) {
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.Scheme != "http" {
		return nil, errors.New("expected host:port or an http:// URL")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validate runs the checks that make a start fatal.
func (c Config) validate() error {
	if c.GenSlots < 1 || c.EmbedSlots < 1 || c.QueueMax < 1 || c.PerClient < 1 || c.PerIP < 1 || c.LabelsPerIP < 1 {
		return errors.New("slots, --queue-max, --queue-per-client, --queue-per-ip and --labels-per-ip must be positive")
	}
	if c.MaxWait <= 0 || c.RunTimeout <= 0 || c.AgeAfter <= 0 || c.StartStagger < 0 || c.GPUInterval < 0 {
		return errors.New("durations must be positive (--start-stagger and --gpu-interval may be 0)")
	}
	if c.SampleStore <= 0 || (c.GPUInterval > 0 && c.SampleStore < c.GPUInterval) {
		return errors.New("--sample-store-interval must be positive and not shorter than --gpu-interval")
	}
	if c.MaxBodyBytes <= 0 {
		return errors.New("--max-body-bytes must be positive")
	}
	if c.RetentionReq <= 0 || c.RetentionGPU <= 0 || c.MaxReqRows < 1000 {
		return errors.New("retention must be positive and --max-request-rows at least 1000")
	}
	if !c.NonLoopback && !isLoopbackAddr(c.Listen) {
		return fmt.Errorf("--listen %q is not on the loopback: the gateway trusts the nginx (X-Real-IP); use --non-loopback-listen only if you know why", c.Listen)
	}
	if c.DB == "" {
		return errors.New("--db is required")
	}
	if c.KeepDone < 0 {
		return errors.New("--monitor-keep-done must not be negative")
	}
	if c.RetentionCont <= 0 || c.RetentionJobGP <= 0 {
		return errors.New("--retention-content and --retention-job-gpu must be positive")
	}
	if c.LogContent {
		if c.ContentDB == "" {
			return errors.New("--content-db is required with --log-content")
		}
		if c.ContentDB == c.DB {
			return errors.New("--content-db must be a different file from --db: the text never goes in the metadata database")
		}
		if c.ContentMax < 1<<20 || c.ContentMaxJob < 1024 || c.ContentMaxJob > c.ContentMax {
			return errors.New("--content-max-bytes must be at least 1 MiB, --content-max-job-bytes at least 1024 and not above the total cap")
		}
	}
	return nil
}
