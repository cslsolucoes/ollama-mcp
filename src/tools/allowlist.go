package tools

import (
	"fmt"
	"strings"
)

// Model allowlist. The owner decided the server only works with the models
// that fit the GPUs (qwen3-ia and nomic-embed-text), so every tool that takes
// an optional `model` checks it here BEFORE any request to Ollama.

// SplitModels splits a list separated by ';' or ',' (blanks dropped, spaces
// trimmed, duplicates removed, order kept).
func SplitModels(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' })
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// normModel makes the implicit ":latest" tag explicit-free, so "qwen3-ia" and
// "qwen3-ia:latest" compare equal. Everything else is exact and case-sensitive,
// like Ollama.
func normModel(name string) string { return strings.TrimSuffix(name, ":latest") }

// ModelAllowed reports whether name is in the list (":latest" aware).
func ModelAllowed(list []string, name string) bool {
	n := normModel(name)
	for _, m := range list {
		if normModel(m) == n {
			return true
		}
	}
	return false
}

// allowedModels is the effective allowlist of the registry: AllowedModels, or
// (when unset) the two server defaults. Empty means "no restriction" and only
// happens for the offline catalog Deps, where no tool runs.
func (d Deps) allowedModels() []string {
	if len(d.AllowedModels) > 0 {
		return d.AllowedModels
	}
	var out []string
	for _, m := range []string{d.Model, d.EmbedModel} {
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

// checkModel validates the optional `model` argument. An empty model means the
// server default and is accepted (the default is validated at start).
func (d Deps) checkModel(model string) error {
	if model == "" {
		return nil
	}
	list := d.allowedModels()
	if len(list) == 0 || ModelAllowed(list, model) {
		return nil
	}
	return fmt.Errorf("modelo %s não permitido neste servidor; permitidos: %s", model, strings.Join(list, ", "))
}

// modelArg reads the optional `model` argument and checks it against the
// allowlist.
func (d Deps) modelArg(a args) (string, error) {
	model, err := a.optString("model")
	if err != nil {
		return "", err
	}
	if err := d.checkModel(model); err != nil {
		return "", err
	}
	return model, nil
}
