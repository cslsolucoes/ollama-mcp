package main

import (
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/files"
	"github.com/cslsoftwares/mcpollama/ocr"
	"github.com/cslsoftwares/mcpollama/ollama"
	"github.com/cslsoftwares/mcpollama/tools"
)

// Defaults. The prompt ceiling is a character budget decided by the owner
// (P-6), not a model context size; nothing here assumes a num_ctx.
const (
	defaultModel          = "qwen3-ia"
	defaultEmbedModel     = "nomic-embed-text"
	defaultTimeout        = 120 * time.Second // above the 32-36 s cold load
	defaultMaxPromptChars = 128000
	defaultMaxLineBytes   = 8 << 20
	defaultMaxFileBytes   = 4 << 20
	defaultMaxDocBytes    = 32 << 20 // .docx, .pptx and .pdf (D-3)
	defaultMaxChunks      = 40       // model calls per file request (FILE-011)
	defaultVerbosity      = "full"
)

// Config is the resolved server configuration (flag > env > default).
type Config struct {
	OllamaURL string
	APIKey    string
	TLSCA     string
	Insecure  bool
	// Fallback (Fase 10.4), off by default: tried once when the primary cannot
	// be reached or answers 503 "queue full". Its token and CA are its own.
	FallbackURL    string
	FallbackAPIKey string
	FallbackTLSCA  string
	Model          string
	EmbedModel     string
	AllowedModels  string // raw list separated by ; or ,; empty means Model and EmbedModel only
	Timeout        time.Duration
	MaxPromptChars int
	MaxLineBytes   int
	MaxFileBytes   int
	MaxDocBytes    int    // ceiling for .docx/.pptx/.pdf; text keeps MaxFileBytes
	PDFCacheDir    string // absolute directory of the WASM compilation cache; empty means no cache and no disk writes
	MaxChunks      int
	AllowRoot      string // directories separated by ; (os.PathListSeparator); empty disables the file tools
	Debug          bool
	ToolsVerbosity string
	TraceFile      string
	AuditLogPath   string
	ClientLabel    string // --client-label: name of this agent for the server queue; empty means <hostname>/<user>
	StationInfo    bool   // --station-info: send X-Mcpollama-Station (default true)
	stationHeader  string // header text collected once at start; empty when off or unknown
	AutoReport     bool   // --auto-report: send the automatic error candidates in the background (default false, C-1.20a)
	ShowVersion    bool
	OCR            OCRFlags // --ocr-*: off unless OCR.Lib is set

	// Version is filled by the caller (main.Version); it feeds serverInfo.
	Version string

	// ocrRT is the running OCR (set by realMain after setupOCR); nil means no
	// OCR. Not a flag: the document extractor reads it.
	ocrRT *ocrRuntime
}

// OCRFlags are the --ocr-* settings (step 7.9). OCR is opt-in: without Lib
// nothing changes. A problem with them (a relative path, a hash mismatch, a
// DLL that does not load) never stops the start: OCR stays off with a WARNING.
type OCRFlags struct {
	Lib           string        // absolute path of libtesseract64.dll
	LibSHA256     string        // required with Lib
	Tessdata      string        // absolute model directory; empty means <dir of Lib>\tessdata\best (D-11)
	Workers       int           // OCR child processes
	PageTimeout   time.Duration // per page
	MaxPages      int           // PDF pages sent to the OCR per call
	MaxMem        int64         // bytes per OCR process (Windows Job Object)
	MaxMegapixels int           // per page image; the dpi is lowered to fit
	DPI           int           // render resolution
}

// defaultOCRFlags are the OCR defaults (brief 11.6).
func defaultOCRFlags() OCRFlags {
	return OCRFlags{
		Workers:       ocr.DefaultWorkers(),
		PageTimeout:   ocr.DefaultPageTimeout,
		MaxPages:      doctext.DefaultOCRMaxPages,
		MaxMem:        ocr.DefaultMaxMem,
		MaxMegapixels: ocr.DefaultMaxMegapixels,
		DPI:           ocr.DefaultDPI,
	}
}

// parseBool is the single boolean rule for environment variables: "", "0",
// "false", "no" and "off" are false; anything else is true.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

const usageText = `Usage: mcpollama-mcp [flags]

MCP server (stdio, JSON-RPC 2.0) in front of a self-hosted Ollama.
Every flag falls back to its environment variable; the flag wins.

  --ollama-url URL        Ollama base URL                          (OLLAMA_HOST)
  --api-key TOKEN         Bearer token; the command line is visible in the
                          process list, prefer the env var         (OLLAMA_API_KEY)
  --tls-ca FILE           extra CA bundle (PEM)                    (OLLAMA_TLS_CA)
  --insecure              skip TLS verification (not recommended)  (OLLAMA_INSECURE)
  --ollama-url-fallback URL  second server tried ONCE when the primary cannot be
                          reached or answers 503 "queue full"; the [uso] line then
                          carries backend=fallback. Off unless set (OLLAMA_HOST_FALLBACK)
  --fallback-api-key TOKEN   Bearer token of the fallback (never the primary's)
                                                                   (OLLAMA_FALLBACK_API_KEY)
  --fallback-tls-ca FILE  extra CA bundle (PEM) of the fallback    (OLLAMA_FALLBACK_TLS_CA)
  --model NAME            generation model, default qwen3-ia       (OLLAMA_MODEL)
  --embed-model NAME      embeddings model, default nomic-embed-text (OLLAMA_EMBED_MODEL)
  --allowed-models LIST   models the tools may use, separated by ; or ,; default is
                          qwen3-ia;nomic-embed-text (tuned for the server GPUs);
                          --model and --embed-model must be in it (MCPOLLAMA_ALLOWED_MODELS)
  --timeout DURATION      per-call deadline, default 2m0s          (OLLAMA_TIMEOUT)
  --max-prompt-chars N    prompt ceiling in characters, default 128000 (MCPOLLAMA_MAX_PROMPT_CHARS)
  --max-file-bytes N      file ceiling in bytes, default 4194304   (MCPOLLAMA_MAX_FILE_BYTES)
  --max-doc-bytes N       .docx/.pptx/.pdf ceiling in bytes, default 33554432 (MCPOLLAMA_MAX_DOC_BYTES)
  --pdf-cache-dir DIR     absolute directory for the PDF engine compilation cache
                          (created if missing); without it nothing is written to
                          disk and the first PDF takes a few seconds longer (MCPOLLAMA_PDF_CACHE_DIR)
  --allow-root DIR[;DIR]  absolute directories the file tools may read, separated by ;
                          (: on Linux), not nested; without it
                          summarize_file, extract and translate do not exist (MCPOLLAMA_ALLOW_ROOT)
  --max-chunks N          model calls per file request, default 40 (MCPOLLAMA_MAX_CHUNKS)
  --max-line-bytes N      stdin line ceiling, default 8388608      (MCPOLLAMA_MAX_LINE_BYTES)
  --ocr-lib FILE          absolute path of libtesseract64.dll: OCR of PDF pages without
                          text and of .png/.jpg images (Windows); without it no OCR (MCPOLLAMA_OCR_LIB)
  --ocr-lib-sha256 HEX    required with --ocr-lib: SHA-256 of the DLL; on any OCR problem
                          the server starts without OCR and logs a WARNING (MCPOLLAMA_OCR_LIB_SHA256)
  --ocr-tessdata DIR      absolute directory of the models, default <DLL dir>\tessdata\best
                                                                   (MCPOLLAMA_OCR_TESSDATA)
  --ocr-workers N         OCR processes, default min(CPUs/2, 4)    (MCPOLLAMA_OCR_WORKERS)
  --ocr-page-timeout D    per page, default 1m0s                   (MCPOLLAMA_OCR_PAGE_TIMEOUT)
  --ocr-max-pages N       pages sent to OCR per call, default 30   (MCPOLLAMA_OCR_MAX_PAGES)
  --ocr-max-mem N         bytes per OCR process, default 1073741824 (MCPOLLAMA_OCR_MAX_MEM)
  --ocr-max-megapixels N  pixels per page, default 40 (dpi lowered to fit) (MCPOLLAMA_OCR_MAX_MEGAPIXELS)
  --ocr-dpi N             page render resolution, default 300      (MCPOLLAMA_OCR_DPI)
  --client-label NAME     name of this agent for the server queue and its monitor
                          (metadata only); default <hostname>/<user> (MCPOLLAMA_CLIENT_LABEL)
  --station-info          send X-Mcpollama-Station (computer, user, domain, MAC of this
                          workstation; declared, not authenticated) for the server job
                          log; on by default, --station-info=false turns it off (MCPOLLAMA_STATION_INFO)
  --auto-report           turns ON the automatic error candidates: an empty or token-limit-cut
                          answer is reported in the background to the server error base, which
                          copies the request and the answer (even of sensitive documents) to its
                          training base, with no expiry. Never delays or fails a tool. OFF by
                          default (MCPOLLAMA_AUTO_REPORT). The invalid-JSON rule is reserved: no
                          tool exposes the Ollama "format" option yet, so it does not fire.
  --tools-verbosity MODE  full|compact tools/list size             (MCPOLLAMA_TOOLS_VERBOSITY)
  --trace-file FILE       opt-in trace file (later phase)          (MCPOLLAMA_TRACE)
  --audit-log-path FILE   opt-in audit log (later phase)           (MCPOLLAMA_AUDIT_LOG)
  --debug                 log names, sizes and counters only       (MCPOLLAMA_DEBUG)
  --version               print the version and exit
`

// parseConfig resolves flags and environment. Flags are registered with zero
// defaults and the real defaults are applied here, so no default (and no
// token read from the environment) ever shows up in a usage or error message.
func parseConfig(args []string, getenv func(string) string, errw io.Writer) (Config, error) {
	fs := flag.NewFlagSet("mcpollama-mcp", flag.ContinueOnError)
	fs.SetOutput(errw)
	fs.Usage = func() { fmt.Fprint(errw, usageText) }

	var (
		fURL      = fs.String("ollama-url", "", "")
		fKey      = fs.String("api-key", "", "")
		fCA       = fs.String("tls-ca", "", "")
		fInsecure = fs.Bool("insecure", false, "")
		fFbURL    = fs.String("ollama-url-fallback", "", "")
		fFbKey    = fs.String("fallback-api-key", "", "")
		fFbCA     = fs.String("fallback-tls-ca", "", "")
		fModel    = fs.String("model", "", "")
		fEmbed    = fs.String("embed-model", "", "")
		fAllowed  = fs.String("allowed-models", "", "")
		fTimeout  = fs.Duration("timeout", 0, "")
		fPrompt   = fs.Int("max-prompt-chars", 0, "")
		fFile     = fs.Int("max-file-bytes", 0, "")
		fDoc      = fs.Int("max-doc-bytes", 0, "")
		fPDFCache = fs.String("pdf-cache-dir", "", "")
		fChunks   = fs.Int("max-chunks", 0, "")
		fRoot     = fs.String("allow-root", "", "")
		fLine     = fs.Int("max-line-bytes", 0, "")
		fVerb     = fs.String("tools-verbosity", "", "")
		fTrace    = fs.String("trace-file", "", "")
		fAudit    = fs.String("audit-log-path", "", "")
		fLabel    = fs.String("client-label", "", "")
		fStation  = fs.Bool("station-info", true, "")
		fAuto     = fs.Bool("auto-report", false, "")
		fDebug    = fs.Bool("debug", false, "")
		fVersion  = fs.Bool("version", false, "")

		fOCRLib      = fs.String("ocr-lib", "", "")
		fOCRSHA      = fs.String("ocr-lib-sha256", "", "")
		fOCRTessdata = fs.String("ocr-tessdata", "", "")
		fOCRWorkers  = fs.Int("ocr-workers", 0, "")
		fOCRTimeout  = fs.Duration("ocr-page-timeout", 0, "")
		fOCRPages    = fs.Int("ocr-max-pages", 0, "")
		fOCRMem      = fs.Int64("ocr-max-mem", 0, "")
		fOCRMP       = fs.Int("ocr-max-megapixels", 0, "")
		fOCRDPI      = fs.Int("ocr-dpi", 0, "")
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
	str := func(name, env, def string, flagVal *string) string {
		if set[name] {
			return *flagVal
		}
		if v := getenv(env); v != "" {
			return v
		}
		return def
	}
	boolean := func(name, env string, flagVal *bool) bool {
		if set[name] {
			return *flagVal
		}
		return parseBool(getenv(env))
	}
	integer := func(name, env string, def int, flagVal *int) int {
		if set[name] {
			return *flagVal
		}
		if v := getenv(env); v != "" {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil && cfgErr == nil {
				cfgErr = fmt.Errorf("%s: invalid integer", env)
			}
			return n
		}
		return def
	}

	cfg := Config{
		OllamaURL:      str("ollama-url", "OLLAMA_HOST", "", fURL),
		APIKey:         str("api-key", "OLLAMA_API_KEY", "", fKey),
		TLSCA:          str("tls-ca", "OLLAMA_TLS_CA", "", fCA),
		Insecure:       boolean("insecure", "OLLAMA_INSECURE", fInsecure),
		FallbackURL:    str("ollama-url-fallback", "OLLAMA_HOST_FALLBACK", "", fFbURL),
		FallbackAPIKey: str("fallback-api-key", "OLLAMA_FALLBACK_API_KEY", "", fFbKey),
		FallbackTLSCA:  str("fallback-tls-ca", "OLLAMA_FALLBACK_TLS_CA", "", fFbCA),
		Model:          str("model", "OLLAMA_MODEL", defaultModel, fModel),
		EmbedModel:     str("embed-model", "OLLAMA_EMBED_MODEL", defaultEmbedModel, fEmbed),
		AllowedModels:  str("allowed-models", "MCPOLLAMA_ALLOWED_MODELS", "", fAllowed),
		MaxPromptChars: integer("max-prompt-chars", "MCPOLLAMA_MAX_PROMPT_CHARS", defaultMaxPromptChars, fPrompt),
		MaxFileBytes:   integer("max-file-bytes", "MCPOLLAMA_MAX_FILE_BYTES", defaultMaxFileBytes, fFile),
		MaxDocBytes:    integer("max-doc-bytes", "MCPOLLAMA_MAX_DOC_BYTES", defaultMaxDocBytes, fDoc),
		PDFCacheDir:    str("pdf-cache-dir", "MCPOLLAMA_PDF_CACHE_DIR", "", fPDFCache),
		MaxChunks:      integer("max-chunks", "MCPOLLAMA_MAX_CHUNKS", defaultMaxChunks, fChunks),
		AllowRoot:      str("allow-root", "MCPOLLAMA_ALLOW_ROOT", "", fRoot),
		MaxLineBytes:   integer("max-line-bytes", "MCPOLLAMA_MAX_LINE_BYTES", defaultMaxLineBytes, fLine),
		ToolsVerbosity: str("tools-verbosity", "MCPOLLAMA_TOOLS_VERBOSITY", defaultVerbosity, fVerb),
		TraceFile:      str("trace-file", "MCPOLLAMA_TRACE", "", fTrace),
		AuditLogPath:   str("audit-log-path", "MCPOLLAMA_AUDIT_LOG", "", fAudit),
		ClientLabel:    str("client-label", "MCPOLLAMA_CLIENT_LABEL", "", fLabel),
		Debug:          boolean("debug", "MCPOLLAMA_DEBUG", fDebug),
		StationInfo:    true,
		AutoReport:     boolean("auto-report", "MCPOLLAMA_AUTO_REPORT", fAuto),
		ShowVersion:    *fVersion,
		Timeout:        defaultTimeout,
	}
	switch {
	case set["timeout"]:
		cfg.Timeout = *fTimeout
	case getenv("OLLAMA_TIMEOUT") != "":
		d, err := time.ParseDuration(strings.TrimSpace(getenv("OLLAMA_TIMEOUT")))
		if err != nil && cfgErr == nil {
			cfgErr = errors.New("OLLAMA_TIMEOUT: invalid duration")
		}
		cfg.Timeout = d
	}

	switch {
	case set["station-info"]:
		cfg.StationInfo = *fStation
	case strings.TrimSpace(getenv("MCPOLLAMA_STATION_INFO")) != "":
		cfg.StationInfo = parseBool(getenv("MCPOLLAMA_STATION_INFO"))
	}
	if cfg.StationInfo && !cfg.ShowVersion {
		cfg.stationHeader = collectStationHeader(getenv)
	}

	od := defaultOCRFlags()
	cfg.OCR = OCRFlags{
		Lib:           str("ocr-lib", "MCPOLLAMA_OCR_LIB", "", fOCRLib),
		LibSHA256:     str("ocr-lib-sha256", "MCPOLLAMA_OCR_LIB_SHA256", "", fOCRSHA),
		Tessdata:      str("ocr-tessdata", "MCPOLLAMA_OCR_TESSDATA", "", fOCRTessdata),
		Workers:       integer("ocr-workers", "MCPOLLAMA_OCR_WORKERS", od.Workers, fOCRWorkers),
		PageTimeout:   od.PageTimeout,
		MaxPages:      integer("ocr-max-pages", "MCPOLLAMA_OCR_MAX_PAGES", od.MaxPages, fOCRPages),
		MaxMem:        od.MaxMem,
		MaxMegapixels: integer("ocr-max-megapixels", "MCPOLLAMA_OCR_MAX_MEGAPIXELS", od.MaxMegapixels, fOCRMP),
		DPI:           integer("ocr-dpi", "MCPOLLAMA_OCR_DPI", od.DPI, fOCRDPI),
	}
	switch {
	case set["ocr-page-timeout"]:
		cfg.OCR.PageTimeout = *fOCRTimeout
	case getenv("MCPOLLAMA_OCR_PAGE_TIMEOUT") != "":
		d, err := time.ParseDuration(strings.TrimSpace(getenv("MCPOLLAMA_OCR_PAGE_TIMEOUT")))
		if err != nil && cfgErr == nil {
			cfgErr = errors.New("MCPOLLAMA_OCR_PAGE_TIMEOUT: invalid duration")
		}
		cfg.OCR.PageTimeout = d
	}
	switch {
	case set["ocr-max-mem"]:
		cfg.OCR.MaxMem = *fOCRMem
	case getenv("MCPOLLAMA_OCR_MAX_MEM") != "":
		n, err := strconv.ParseInt(strings.TrimSpace(getenv("MCPOLLAMA_OCR_MAX_MEM")), 10, 64)
		if err != nil && cfgErr == nil {
			cfgErr = errors.New("MCPOLLAMA_OCR_MAX_MEM: invalid integer")
		}
		cfg.OCR.MaxMem = n
	}
	return cfg, cfgErr
}

// validate runs the checks that make a start fatal. It does no network I/O.
func (c Config) validate() error {
	if c.ToolsVerbosity != "full" && c.ToolsVerbosity != "compact" {
		return fmt.Errorf("--tools-verbosity must be full or compact, got %q", c.ToolsVerbosity)
	}
	if err := c.checkModelsAllowed(); err != nil {
		return err
	}
	if c.Timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	if c.MaxPromptChars <= 0 || c.MaxFileBytes <= 0 || c.MaxLineBytes <= 0 || c.MaxChunks <= 0 {
		return errors.New("--max-prompt-chars, --max-file-bytes, --max-line-bytes and --max-chunks must be positive")
	}
	if c.MaxDocBytes <= 0 {
		return errors.New("--max-doc-bytes must be positive")
	}
	if c.PDFCacheDir != "" && !filepath.IsAbs(c.PDFCacheDir) {
		return errors.New("--pdf-cache-dir must be an absolute path")
	}
	if _, err := c.guard(); err != nil {
		return err
	}
	if c.OllamaURL != "" {
		u, err := url.Parse(c.OllamaURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("--ollama-url must be an http(s) URL with a host")
		}
	}
	if c.FallbackURL != "" {
		u, err := url.Parse(c.FallbackURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("--ollama-url-fallback must be an http(s) URL with a host")
		}
	} else if c.FallbackAPIKey != "" || c.FallbackTLSCA != "" {
		return errors.New("--fallback-api-key and --fallback-tls-ca need --ollama-url-fallback")
	}
	if c.FallbackTLSCA != "" {
		pem, err := os.ReadFile(c.FallbackTLSCA)
		if err != nil {
			return fmt.Errorf("--fallback-tls-ca: cannot read %s", c.FallbackTLSCA)
		}
		if !x509.NewCertPool().AppendCertsFromPEM(pem) {
			return fmt.Errorf("--fallback-tls-ca: no PEM certificate found in %s", c.FallbackTLSCA)
		}
	}
	if c.TLSCA != "" {
		pem, err := os.ReadFile(c.TLSCA)
		if err != nil {
			return fmt.Errorf("--tls-ca: cannot read %s", c.TLSCA)
		}
		if !x509.NewCertPool().AppendCertsFromPEM(pem) {
			return fmt.Errorf("--tls-ca: no PEM certificate found in %s", c.TLSCA)
		}
	}
	return nil
}

// allowedModelList is the effective allowlist: --allowed-models when it names
// at least one model, otherwise the fixed set of models tuned for the CSL
// server hardware (owner decision 2026-10-26). It is not derived from --model:
// that would let any --model through and the start check would never fire.
func (c Config) allowedModelList() []string {
	if l := tools.SplitModels(c.AllowedModels); len(l) > 0 {
		return l
	}
	return []string{defaultModel, defaultEmbedModel}
}

// checkModelsAllowed makes a start fatal when a default model is outside the
// allowlist: every call without an explicit model would be refused.
func (c Config) checkModelsAllowed() error {
	list := c.allowedModelList()
	for _, m := range []struct{ flag, name string }{{"--model", c.Model}, {"--embed-model", c.EmbedModel}} {
		if !tools.ModelAllowed(list, m.name) {
			return fmt.Errorf("%s %q não está na lista de modelos permitidos (--allowed-models: %s)", m.flag, m.name, strings.Join(list, ", "))
		}
	}
	return nil
}

// broadDirs supplies the home and system directories that make a root "broad";
// a variable so tests inject them instead of depending on the machine.
var broadDirs = files.DefaultBroadDirs

// guard builds the file gate from --allow-root. It returns nil (no file tools)
// when no root is configured, and an error when a root is unusable: a typo in
// a root must stop the start, not silently leave the tools off.
func (c Config) guard() (*files.Guard, error) { return c.guardWith(nil) }

// guardWith is guard with a debug sink for the document extractor (metadata
// only: format, pages, bytes, ms). Documents are always enabled with the file
// tools: there is no switch to turn the extraction off.
func (c Config) guardWith(logf func(string, ...any)) (*files.Guard, error) {
	roots := files.SplitRoots(c.AllowRoot)
	if len(roots) == 0 {
		return nil, nil
	}
	g, err := files.NewGuard(roots, int64(c.MaxFileBytes))
	if err != nil {
		return nil, err
	}
	if err := g.SetExtractor(newDocExtractor(c, logf), int64(c.MaxDocBytes)); err != nil {
		return nil, err
	}
	return g, nil
}

// secrets returns every value that must never reach a log or an error:
// the token and the credentials embedded in the URL.
func (c Config) secrets() []string {
	s := []string{c.APIKey, c.FallbackAPIKey}
	if u, err := url.Parse(c.FallbackURL); err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok {
			s = append(s, pw)
		}
		s = append(s, u.User.Username())
	}
	if u, err := url.Parse(c.OllamaURL); err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok {
			s = append(s, pw)
		}
		s = append(s, u.User.Username())
	}
	return s
}

// safeURL is the URL reduced to scheme and host, fit for logs.
func (c Config) safeURL() string {
	u, err := url.Parse(c.OllamaURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// ollamaOptions maps the resolved configuration to the Ollama client options.
// Building the client is lazy: nothing calls this at start or on initialize.
func (c Config) ollamaOptions() ollama.Options {
	return ollama.Options{
		BaseURL:        c.OllamaURL,
		APIKey:         c.APIKey,
		TLSCA:          c.TLSCA,
		Insecure:       c.Insecure,
		FallbackURL:    c.FallbackURL,
		FallbackAPIKey: c.FallbackAPIKey,
		FallbackTLSCA:  c.FallbackTLSCA,
		Model:          c.Model,
		EmbedModel:     c.EmbedModel,
		Timeout:        c.Timeout,
		MaxPromptChars: c.MaxPromptChars,
		MaxLineBytes:   c.MaxLineBytes,
		ClientLabel:    c.clientLabel(),
		StationInfo:    c.stationHeader,
	}
}

// clientLabel is --client-label, or <hostname>/<user> by default, so the server
// monitor shows which machine and user sent each request (owner's decision,
// 2026-10-30). All sessions of the same user share one client in the queue.
func (c Config) clientLabel() string {
	if c.ClientLabel != "" {
		return c.ClientLabel
	}
	host, _ := os.Hostname()
	return defaultLabel(host, currentUser(), os.Getpid())
}

// defaultLabel builds <host>/<user>, dropping a Windows "DOMAIN\" prefix; with
// no user it falls back to <host>/<pid>.
func defaultLabel(host, usr string, pid int) string {
	if host == "" {
		host = "mcpollama"
	}
	if i := strings.LastIndexAny(usr, `\/`); i >= 0 {
		usr = usr[i+1:]
	}
	if usr = strings.TrimSpace(usr); usr == "" {
		return fmt.Sprintf("%s/%d", host, pid)
	}
	return host + "/" + usr
}

// currentUser is the login name of this process, or "" if it is unknown.
func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	for _, k := range []string{"USERNAME", "USER"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
