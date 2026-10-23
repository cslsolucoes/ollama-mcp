package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

// maxLogLine is the ceiling of one log line, newline included, applied after
// redaction.
const maxLogLine = 4000

const redacted = "[REDACTED]"

// newRedactor returns the single function that swaps the exact value of every
// secret for [REDACTED]. Empty secrets are ignored (replacing "" would
// mangle the whole text).
func newRedactor(secrets ...string) func(string) string {
	var live []string
	for _, s := range secrets {
		if s != "" {
			live = append(live, s)
		}
	}
	return func(text string) string {
		for _, s := range live {
			text = strings.ReplaceAll(text, s, redacted)
		}
		return text
	}
}

// logger writes one redacted, capped, single-line record per call (stderr).
type logger struct {
	mu     sync.Mutex
	w      io.Writer
	redact func(string) string
	debug  bool
}

func (l *logger) logf(format string, a ...any) {
	line := l.redact(fmt.Sprintf(format, a...))
	line = strings.NewReplacer("\r", " ", "\n", " ").Replace(line)
	if len(line) > maxLogLine-1 {
		line = line[:maxLogLine-1]
		for len(line) > 0 && !utf8.ValidString(line) {
			line = line[:len(line)-1]
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	io.WriteString(l.w, line+"\n")
}

func (l *logger) debugf(format string, a ...any) {
	if l.debug {
		l.logf("debug: "+format, a...)
	}
}
