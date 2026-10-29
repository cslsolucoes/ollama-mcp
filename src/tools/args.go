package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// args is the decoded arguments object of one tools/call. This file is the
// single home of argument handling (TOOL-006): JSON numbers arrive as float64,
// and a wrong type gets its own message, never the "is required" one
// (TOOL-007).
type args map[string]any

// parseArgs decodes the arguments object and rejects names outside allowed, so
// a misspelled option is reported instead of silently ignored.
func parseArgs(raw json.RawMessage, allowed ...string) (args, error) {
	a := args{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, fmt.Errorf("os argumentos devem ser um objeto JSON")
		}
	}
	var unknown []string
	for k := range a {
		ok := false
		for _, n := range allowed {
			if k == n {
				ok = true
				break
			}
		}
		if !ok {
			unknown = append(unknown, "`"+k+"`")
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("argumento desconhecido: %s (aceitos: `%s`)", strings.Join(unknown, ", "), strings.Join(allowed, "`, `"))
	}
	return a, nil
}

// get returns the value and whether it is present; JSON null counts as absent.
func (a args) get(key string) (any, bool) {
	v, ok := a[key]
	if !ok || v == nil {
		return nil, false
	}
	return v, true
}

func jsonType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "null"
}

func typeErr(key, want string, got any) error {
	return fmt.Errorf("`%s` must be %s (recebido: %s)", key, want, jsonType(got))
}

func requiredErr(key string) error { return fmt.Errorf("`%s` is required (obrigatório)", key) }

// optString returns the string argument, "" when absent.
func (a args) optString(key string) (string, error) {
	v, ok := a.get(key)
	if !ok {
		return "", nil
	}
	s, isStr := v.(string)
	if !isStr {
		return "", typeErr(key, "a string", v)
	}
	return s, nil
}

// reqString returns a non-blank string argument.
func (a args) reqString(key string) (string, error) {
	v, ok := a.get(key)
	if !ok {
		return "", requiredErr(key)
	}
	s, isStr := v.(string)
	if !isStr {
		return "", typeErr(key, "a string", v)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("`%s` não pode ser vazio", key)
	}
	return s, nil
}

// optFloat returns a pointer to the number, nil when absent.
func (a args) optFloat(key string, min, max float64) (*float64, error) {
	v, ok := a.get(key)
	if !ok {
		return nil, nil
	}
	f, isNum := v.(float64)
	if !isNum {
		return nil, typeErr(key, "a number", v)
	}
	if f < min || f > max || math.IsNaN(f) {
		return nil, fmt.Errorf("`%s` deve estar entre %g e %g", key, min, max)
	}
	return &f, nil
}

// optInt returns a pointer to the integer, nil when absent.
func (a args) optInt(key string, min, max int) (*int, error) {
	v, ok := a.get(key)
	if !ok {
		return nil, nil
	}
	f, isNum := v.(float64)
	if !isNum || f != math.Trunc(f) || math.IsInf(f, 0) {
		return nil, typeErr(key, "an integer", v)
	}
	if f < float64(min) || f > float64(max) {
		return nil, fmt.Errorf("`%s` deve estar entre %d e %d", key, min, max)
	}
	n := int(f)
	return &n, nil
}

// optThink reads the optional `think` flag (default false).
func (a args) optThink() (bool, error) { return a.optBool("think") }

func (a args) optBool(key string) (bool, error) {
	v, ok := a.get(key)
	if !ok {
		return false, nil
	}
	b, isBool := v.(bool)
	if !isBool {
		return false, typeErr(key, "a boolean", v)
	}
	return b, nil
}

// optList returns the array argument and whether it was present.
func (a args) optList(key string) ([]any, bool, error) {
	v, ok := a.get(key)
	if !ok {
		return nil, false, nil
	}
	l, isList := v.([]any)
	if !isList {
		return nil, false, typeErr(key, "an array", v)
	}
	return l, true, nil
}

// limitedString reads a string argument (required or optional) and enforces a
// rune ceiling, so a prompt fragment taken from the arguments cannot be
// arbitrarily large.
func (a args) limitedString(key string, max int, required bool) (string, error) {
	var s string
	var err error
	if required {
		s, err = a.reqString(key)
	} else {
		s, err = a.optString(key)
	}
	if err != nil {
		return "", err
	}
	if n := utf8.RuneCountInString(s); n > max {
		return "", fmt.Errorf("`%s` é longo demais (%d caracteres; máximo %d)", key, n, max)
	}
	return s, nil
}
