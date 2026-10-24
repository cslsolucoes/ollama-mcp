package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/tools"
)

func TestWriteThenCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TOOLS.md")
	var out, errw bytes.Buffer
	if code := run([]string{"-o", path}, &out, &errw); code != 0 {
		t.Fatalf("write: exit %d: %s", code, errw.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != tools.RenderCatalog(tools.Defs(tools.New(tools.Deps{}))) || !strings.Contains(string(got), "## `generate`") {
		t.Fatal("the file is not the rendered registry")
	}
	out.Reset()
	if code := run([]string{"-o", path, "-check"}, &out, &errw); code != 0 || !strings.Contains(out.String(), "in sync") {
		t.Fatalf("check on a fresh file: exit %d %s", code, out.String())
	}
	// CRLF checkouts (core.autocrlf) still count as in sync.
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(got), "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-o", path, "-check"}, &out, &errw); code != 0 {
		t.Fatalf("CRLF copy must be accepted: exit %d", code)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	errw.Reset()
	if code := run([]string{"-o", path, "-check"}, &out, &errw); code != 1 || !strings.Contains(errw.String(), "out of date") {
		t.Fatalf("stale file: exit %d %s", code, errw.String())
	}
	if code := run([]string{"-bogus"}, &out, &errw); code != 2 {
		t.Fatalf("bad flag: exit %d", code)
	}
}

func TestRepoRootFindsTheParentOfGoMod(t *testing.T) {
	root, err := repoRoot() // the test runs in src/cmd/dumptools
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "go.mod")); err != nil {
		t.Fatalf("root %s does not contain src/go.mod: %v", root, err)
	}
}
