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
)

// Version is overridden at build time:
// go build -ldflags "-X main.Version=1.2.3"
var Version = "dev"

func main() {
	log.SetOutput(os.Stderr)
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
		fmt.Fprintln(stderr, "mcpollama-mcp: "+newRedactor(cfg.secrets()...)(err.Error()))
		return 2
	}
	if cfg.ShowVersion {
		fmt.Fprintf(stdout, "mcpollama-mcp version %s\n", Version)
		return 0
	}
	cfg.Version = Version
	if err := Run(context.Background(), stdin, stdout, stderr, cfg, nil); err != nil {
		fmt.Fprintln(stderr, "mcpollama-mcp: "+newRedactor(cfg.secrets()...)(err.Error()))
		return 1
	}
	return 0
}
