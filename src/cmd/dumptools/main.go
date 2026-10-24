// Command dumptools writes TOOLS.md, the tool catalog, from the same registry
// the server hands to tools/list. The registry is built with no Ollama backend
// (nothing is contacted, nothing is read), so the dump is fully offline and
// the file is never written by hand (BUILD-006).
//
//	go run ./cmd/dumptools            # writes TOOLS.md at the repository root
//	go run ./cmd/dumptools -o FILE    # writes elsewhere
//	go run ./cmd/dumptools -check     # exits 1 when the file is out of date
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cslsoftwares/mcpollama/tools"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dumptools", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output file (default: TOOLS.md at the repository root)")
	check := fs.Bool("check", false, "compare instead of writing; exit 1 when different")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	path := *out
	if path == "" {
		root, err := repoRoot()
		if err != nil {
			fmt.Fprintln(stderr, "dumptools:", err)
			return 2
		}
		path = filepath.Join(root, "TOOLS.md")
	}
	want := tools.RenderCatalog(tools.Defs(tools.New(tools.Deps{})))
	if *check {
		have, err := os.ReadFile(path)
		if err != nil || strings.ReplaceAll(string(have), "\r\n", "\n") != want {
			fmt.Fprintf(stderr, "dumptools: %s is out of date; run go run ./cmd/dumptools\n", path)
			return 1
		}
		fmt.Fprintf(stdout, "%s is in sync\n", path)
		return 0
	}
	if err := os.WriteFile(path, bytes.NewBufferString(want).Bytes(), 0o644); err != nil {
		fmt.Fprintln(stderr, "dumptools:", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", path, len(want))
	return 0
}

// repoRoot is the parent of the directory holding go.mod (src/), found by
// walking up from the working directory.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Dir(dir), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above the working directory; use -o")
		}
		dir = parent
	}
}
