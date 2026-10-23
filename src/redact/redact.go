// Package redact holds the single secret-masking function shared by the
// server log, the error paths and the Ollama client (SEC-003).
package redact

import "strings"

// Mask is what every secret is replaced with.
const Mask = "[REDACTED]"

// New returns the function that swaps the exact value of every secret for
// Mask. Empty secrets are ignored (replacing "" would mangle the whole text).
func New(secrets ...string) func(string) string {
	var live []string
	for _, s := range secrets {
		if s != "" {
			live = append(live, s)
		}
	}
	return func(text string) string {
		for _, s := range live {
			text = strings.ReplaceAll(text, s, Mask)
		}
		return text
	}
}
