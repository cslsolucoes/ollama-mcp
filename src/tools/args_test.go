package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustArgs(t *testing.T, js string, allowed ...string) args {
	t.Helper()
	a, err := parseArgs(json.RawMessage(js), allowed...)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestArgsStrings(t *testing.T) {
	a := mustArgs(t, `{"s":"x","n":5,"blank":"  ","nul":null}`, "s", "n", "blank", "nul", "missing")
	if v, err := a.reqString("s"); err != nil || v != "x" {
		t.Fatalf("%q %v", v, err)
	}
	// TOOL-007: a wrong type has its own message, not "required".
	_, err := a.reqString("n")
	if err == nil || !strings.Contains(err.Error(), "`n` must be a string") || strings.Contains(err.Error(), "required") {
		t.Fatalf("wrong-type error: %v", err)
	}
	if _, err := a.reqString("missing"); err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("missing: %v", err)
	}
	if _, err := a.reqString("nul"); err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("null counts as absent: %v", err)
	}
	if _, err := a.reqString("blank"); err == nil {
		t.Fatal("blank string must be refused")
	}
	if v, err := a.optString("missing"); err != nil || v != "" {
		t.Fatalf("%q %v", v, err)
	}
	if _, err := a.optString("n"); err == nil {
		t.Fatal("optString must type-check")
	}
}

func TestArgsNumbers(t *testing.T) {
	a := mustArgs(t, `{"i":7,"f":0.5,"frac":1.5,"big":99999,"s":"7","neg":-1}`, "i", "f", "frac", "big", "s", "neg", "none")
	if p, err := a.optInt("i", 1, 10); err != nil || *p != 7 {
		t.Fatal(p, err)
	}
	if p, err := a.optInt("none", 1, 10); err != nil || p != nil {
		t.Fatal(p, err)
	}
	for _, k := range []string{"frac", "s"} {
		if _, err := a.optInt(k, 1, 10); err == nil || !strings.Contains(err.Error(), "must be an integer") {
			t.Errorf("%s: %v", k, err)
		}
	}
	for _, k := range []string{"big", "neg"} {
		if _, err := a.optInt(k, 1, 10); err == nil || !strings.Contains(err.Error(), "deve estar entre") {
			t.Errorf("%s: %v", k, err)
		}
	}
	if p, err := a.optFloat("f", 0, 2); err != nil || *p != 0.5 {
		t.Fatal(p, err)
	}
	if _, err := a.optFloat("s", 0, 2); err == nil || !strings.Contains(err.Error(), "must be a number") {
		t.Fatal(err)
	}
	if _, err := a.optFloat("big", 0, 2); err == nil {
		t.Fatal("range")
	}
}

func TestArgsBoolListAndUnknown(t *testing.T) {
	a := mustArgs(t, `{"b":true,"l":[1],"s":"x"}`, "b", "l", "s", "z")
	if v, err := a.optBool("b"); err != nil || !v {
		t.Fatal(v, err)
	}
	if v, err := a.optBool("z"); err != nil || v {
		t.Fatal(v, err)
	}
	if _, err := a.optBool("s"); err == nil || !strings.Contains(err.Error(), "must be a boolean") {
		t.Fatal(err)
	}
	if l, ok, err := a.optList("l"); err != nil || !ok || len(l) != 1 {
		t.Fatal(l, ok, err)
	}
	if _, _, err := a.optList("s"); err == nil || !strings.Contains(err.Error(), "must be an array") {
		t.Fatal(err)
	}
	if _, err := parseArgs(json.RawMessage(`{"extra":1}`), "a", "b"); err == nil ||
		!strings.Contains(err.Error(), "`extra`") || !strings.Contains(err.Error(), "aceitos") {
		t.Fatalf("unknown argument: %v", err)
	}
	if _, err := parseArgs(json.RawMessage(`[1]`), "a"); err == nil {
		t.Fatal("non-object must fail")
	}
	if a, err := parseArgs(nil, "a"); err != nil || len(a) != 0 {
		t.Fatal("absent arguments are empty")
	}
}
