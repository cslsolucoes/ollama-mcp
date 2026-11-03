package main

import (
	"sync"

	"github.com/cslsoftwares/mcpollama/ollama"
	"github.com/cslsoftwares/mcpollama/tools"
)

// lazyBackend builds the Ollama client on the first tool call, never at start
// or on initialize (PROTO-011, PERF-012): the handshake stays instant with the
// Ollama down, and a missing --ollama-url only fails the call that needs it.
// A failed build is not cached, so fixing the environment needs no restart of
// the call path (the process still reads its flags once).
type lazyBackend struct {
	opts ollama.Options

	mu sync.Mutex
	c  *ollama.Client
}

func (l *lazyBackend) get() (tools.Backend, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.c == nil {
		c, err := ollama.New(l.opts)
		if err != nil {
			return nil, err
		}
		l.c = c
	}
	return l.c, nil
}

// buildTools wires the registry: lazy client, a one-slot GPU queue (PERF-002)
// and the informational defaults for list_models.
func buildTools(cfg Config, lg *logger) []ToolEntry {
	entries, _ := buildToolsWithClose(cfg, lg)
	return entries
}

// newAutoReporter is tools.NewAutoReporter; a variable so a test can reach the
// sender it starts.
var newAutoReporter = tools.NewAutoReporter

// buildToolsWithClose is buildTools plus the function that stops the background
// sender of the automatic error candidates (--auto-report); realMain defers it
// so the goroutine ends with the server.
func buildToolsWithClose(cfg Config, lg *logger) ([]ToolEntry, func()) {
	opts := cfg.ollamaOptions()
	if cfg.Debug {
		opts.Logf = lg.debugf
	}
	lb := &lazyBackend{opts: opts}
	// validate() already refused an unusable root, so an error here cannot happen
	// in the binary; if it did, the file tools stay unregistered (fail closed).
	var docLog func(string, ...any)
	if cfg.Debug {
		docLog = lg.debugf
	}
	guard, _ := cfg.guardWith(docLog)
	var auto *tools.AutoReporter
	if cfg.AutoReport {
		auto = newAutoReporter(lb.get, lg.debugf)
	}
	entries := tools.New(tools.Deps{
		Auto:           auto,
		Backend:        lb.get,
		Queue:          tools.NewQueue(1),
		Model:          cfg.Model,
		EmbedModel:     cfg.EmbedModel,
		AllowedModels:  cfg.allowedModelList(),
		Files:          guard,
		MaxPromptChars: cfg.MaxPromptChars,
		MaxChunks:      cfg.MaxChunks,
	})
	return entries, auto.Close // Close on a nil *AutoReporter is a no-op
}
