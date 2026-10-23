package main

import (
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/ollama"
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
	defaultVerbosity      = "full"
)

// Config is the resolved server configuration (flag > env > default).
type Config struct {
	OllamaURL      string
	APIKey         string
	TLSCA          string
	Insecure       bool
	Model          string
	EmbedModel     string
	Timeout        time.Duration
	MaxPromptChars int
	MaxLineBytes   int
	MaxFileBytes   int
	Debug          bool
	ToolsVerbosity string
	TraceFile      string
	AuditLogPath   string
	ShowVersion    bool

	// Version is filled by the caller (main.Version); it feeds serverInfo.
	Version string
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
  --model NAME            generation model, default qwen3-ia       (OLLAMA_MODEL)
  --embed-model NAME      embeddings model, default nomic-embed-text (OLLAMA_EMBED_MODEL)
  --timeout DURATION      per-call deadline, default 2m0s          (OLLAMA_TIMEOUT)
  --max-prompt-chars N    prompt ceiling in characters, default 128000 (MCPOLLAMA_MAX_PROMPT_CHARS)
  --max-file-bytes N      file ceiling in bytes, default 4194304   (MCPOLLAMA_MAX_FILE_BYTES)
  --max-line-bytes N      stdin line ceiling, default 8388608      (MCPOLLAMA_MAX_LINE_BYTES)
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
		fModel    = fs.String("model", "", "")
		fEmbed    = fs.String("embed-model", "", "")
		fTimeout  = fs.Duration("timeout", 0, "")
		fPrompt   = fs.Int("max-prompt-chars", 0, "")
		fFile     = fs.Int("max-file-bytes", 0, "")
		fLine     = fs.Int("max-line-bytes", 0, "")
		fVerb     = fs.String("tools-verbosity", "", "")
		fTrace    = fs.String("trace-file", "", "")
		fAudit    = fs.String("audit-log-path", "", "")
		fDebug    = fs.Bool("debug", false, "")
		fVersion  = fs.Bool("version", false, "")
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
		Model:          str("model", "OLLAMA_MODEL", defaultModel, fModel),
		EmbedModel:     str("embed-model", "OLLAMA_EMBED_MODEL", defaultEmbedModel, fEmbed),
		MaxPromptChars: integer("max-prompt-chars", "MCPOLLAMA_MAX_PROMPT_CHARS", defaultMaxPromptChars, fPrompt),
		MaxFileBytes:   integer("max-file-bytes", "MCPOLLAMA_MAX_FILE_BYTES", defaultMaxFileBytes, fFile),
		MaxLineBytes:   integer("max-line-bytes", "MCPOLLAMA_MAX_LINE_BYTES", defaultMaxLineBytes, fLine),
		ToolsVerbosity: str("tools-verbosity", "MCPOLLAMA_TOOLS_VERBOSITY", defaultVerbosity, fVerb),
		TraceFile:      str("trace-file", "MCPOLLAMA_TRACE", "", fTrace),
		AuditLogPath:   str("audit-log-path", "MCPOLLAMA_AUDIT_LOG", "", fAudit),
		Debug:          boolean("debug", "MCPOLLAMA_DEBUG", fDebug),
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
	return cfg, cfgErr
}

// validate runs the checks that make a start fatal. It does no network I/O.
func (c Config) validate() error {
	if c.ToolsVerbosity != "full" && c.ToolsVerbosity != "compact" {
		return fmt.Errorf("--tools-verbosity must be full or compact, got %q", c.ToolsVerbosity)
	}
	if c.Timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	if c.MaxPromptChars <= 0 || c.MaxFileBytes <= 0 || c.MaxLineBytes <= 0 {
		return errors.New("--max-prompt-chars, --max-file-bytes and --max-line-bytes must be positive")
	}
	if c.OllamaURL != "" {
		u, err := url.Parse(c.OllamaURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("--ollama-url must be an http(s) URL with a host")
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

// secrets returns every value that must never reach a log or an error:
// the token and the credentials embedded in the URL.
func (c Config) secrets() []string {
	s := []string{c.APIKey}
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
		Model:          c.Model,
		EmbedModel:     c.EmbedModel,
		Timeout:        c.Timeout,
		MaxPromptChars: c.MaxPromptChars,
		MaxLineBytes:   c.MaxLineBytes,
	}
}
