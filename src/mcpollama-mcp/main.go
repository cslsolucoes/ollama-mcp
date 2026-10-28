// Command mcpollama-mcp is an MCP server (stdio, JSON-RPC 2.0) that fronts a
// self-hosted Ollama. stdout carries JSON-RPC only; logs go to stderr.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/ocr"
	"github.com/cslsoftwares/mcpollama/redact"
)

// Version is overridden at build time:
// go build -ldflags "-X main.Version=1.2.3"
var Version = "dev"

func main() {
	log.SetOutput(os.Stderr)
	// The internal OCR worker mode is decided before any flag or environment
	// variable of the server is read: a worker never sees the API key.
	if len(os.Args) > 1 && os.Args[1] == ocr.WorkerFlag {
		os.Exit(ocrWorkerMain(os.Args[1:], os.Stderr))
	}
	os.Exit(realMain(os.Args[1:], os.Getenv, os.Stdin, os.Stdout, os.Stderr))
}

// realMain parses flags and runs the server; it returns the process exit code
// (0 on EOF or --version, 1 on a runtime failure, 2 on bad configuration).
func realMain(args []string, getenv func(string) string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, getenv, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err == nil && !cfg.ShowVersion {
		err = cfg.validate()
	}
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-mcp: "+redact.New(cfg.secrets()...)(err.Error()))
		return 2
	}
	if cfg.ShowVersion {
		fmt.Fprintf(stdout, "mcpollama-mcp version %s\n", Version)
		return 0
	}
	cfg.Version = Version
	// Only records the PDF engine options (and creates the cache directory when
	// asked): the WASM engine itself starts on the first PDF, so the start stays
	// instant. ClosePDF releases it on the way out.
	if err := setupDocs(cfg); err != nil {
		fmt.Fprintln(stderr, "mcpollama-mcp: "+redact.New(cfg.secrets()...)(err.Error()))
		return 2
	}
	defer doctext.ClosePDF()
	// Both the server log and the Ollama client log reach stderr from several
	// goroutines; one locked writer keeps the lines whole.
	errw := &syncWriter{w: stderr}
	lg := &logger{w: errw, redact: redact.New(cfg.secrets()...), debug: cfg.Debug}
	// OCR (opt-in): never fatal; the workers start on demand and are stopped
	// together with the PDF engine on the way out.
	if rt := setupOCR(cfg, lg); rt != nil {
		cfg.ocrRT = rt
		defer rt.Close()
	}
	if err := Run(context.Background(), stdin, stdout, errw, cfg, buildTools(cfg, lg)); err != nil {
		fmt.Fprintln(stderr, "mcpollama-mcp: "+redact.New(cfg.secrets()...)(err.Error()))
		return 1
	}
	return 0
}

// syncWriter serialises writes to one underlying writer.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
