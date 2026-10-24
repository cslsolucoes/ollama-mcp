package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/files"
	"github.com/cslsoftwares/mcpollama/mcp"
)

// toolsListBudget is the byte budget of the full tools/list payload (TOK-006).
// Measured at 4 tools: see TestToolsListBudget for the figure it protects.
const (
	toolsListBudget     = 4400 // the four base tools, no --allow-root
	toolsListBudgetFull = 7600 // with the three file tools (--allow-root)
)

// TEST-008 / TOOL-001 / SEC-012: the exact set, not a "contains".
func TestExactToolSet(t *testing.T) {
	want := []string{"chat", "embed", "generate", "list_models"}
	var got []string
	for _, e := range New(Deps{}) {
		got = append(got, e.Def.Name)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tool set\n got %v\nwant %v", got, want)
	}
}

// TOOL-003: the Cursor prefixes the server id to the tool name; the sum is capped.
func TestQualifiedNameLimit(t *testing.T) {
	longest := 0
	for _, e := range New(CatalogDeps()) {
		if n := len(ServerID) + len(e.Def.Name); n > longest {
			longest = n
		}
		if strings.ToLower(e.Def.Name) != e.Def.Name || strings.ContainsAny(e.Def.Name, " -.") {
			t.Errorf("%q is not snake_case", e.Def.Name)
		}
	}
	if longest > MaxQualifiedName {
		t.Fatalf("longest qualified name = %d > %d", longest, MaxQualifiedName)
	}
	t.Logf("longest qualified name: %d of %d", longest, MaxQualifiedName)
}

// TOOL-005 (P-9): title and explicit closed-world annotations.
func TestAnnotationsAreExplicit(t *testing.T) {
	for _, e := range New(CatalogDeps()) {
		d := e.Def
		if d.Title == "" || d.Annotations == nil {
			t.Fatalf("%s: title/annotations missing", d.Name)
		}
		a := d.Annotations
		if !a.ReadOnlyHint || a.DestructiveHint || a.OpenWorldHint {
			t.Errorf("%s: annotations %+v", d.Name, *a)
		}
		raw, _ := json.Marshal(d)
		for _, key := range []string{`"readOnlyHint":true`, `"destructiveHint":false`, `"openWorldHint":false`, `"idempotentHint":`, `"title":`} {
			if !strings.Contains(string(raw), key) {
				t.Errorf("%s: %s absent from the wire form %s", d.Name, key, raw)
			}
		}
	}
	idem := map[string]bool{"list_models": true, "embed": true, "generate": false, "chat": false,
		"summarize_file": false, "extract": false, "translate": false}
	for _, e := range New(CatalogDeps()) {
		if e.Def.Annotations.IdempotentHint != idem[e.Def.Name] {
			t.Errorf("%s: idempotentHint = %t", e.Def.Name, e.Def.Annotations.IdempotentHint)
		}
	}
}

// TOOL-004: uniform schemas.
func TestSchemaConformance(t *testing.T) {
	var walk func(path string, s map[string]any)
	walk = func(path string, s map[string]any) {
		if s["type"] == "object" {
			props, _ := s["properties"].(schemaMap)
			if props == nil {
				t.Errorf("%s: object without properties", path)
			}
			if s["additionalProperties"] != false {
				t.Errorf("%s: additionalProperties must be false", path)
			}
			req, _ := s["required"].([]string)
			for _, r := range req {
				if _, ok := props[r]; !ok {
					t.Errorf("%s: required %q is not a property", path, r)
				}
			}
			for k, v := range props {
				walk(path+"."+k, v.(schemaMap))
			}
		}
		if items, ok := s["items"].(schemaMap); ok {
			walk(path+"[]", items)
		}
		if alts, ok := s["oneOf"].([]any); ok {
			for i, a := range alts {
				walk(path+"|"+string(rune('0'+i)), a.(schemaMap))
			}
		}
		if s["type"] == nil && s["oneOf"] == nil {
			t.Errorf("%s: neither type nor oneOf", path)
		}
	}
	for _, e := range New(CatalogDeps()) {
		if e.Def.InputSchema["type"] != "object" {
			t.Fatalf("%s: root must be an object", e.Def.Name)
		}
		walk(e.Def.Name, e.Def.InputSchema)
	}
}

// TOK-001 / TOOL-009: nothing in the base tools asks for file text; the
// descriptions are short and static.
func TestDescriptionsShortAndStatic(t *testing.T) {
	for _, e := range New(Deps{Model: "dynamic-model-name", Files: &files.Guard{}}) {
		d := e.Def.Description
		if len(d) > 480 {
			t.Errorf("%s: description has %d bytes", e.Def.Name, len(d))
		}
		first := d
		if i := strings.Index(d, ". "); i >= 0 {
			first = d[:i+1]
		}
		if len(first) > 140 || strings.Contains(d, "dynamic-model-name") {
			t.Errorf("%s: first sentence too long or description not static: %q", e.Def.Name, first)
		}
	}
}

// TOK-006: the size of the full tools/list is bounded.
func TestToolsListBudget(t *testing.T) {
	b, err := json.Marshal(mcp.ToolsListResult{Tools: Defs(New(Deps{}))})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("full tools/list payload: %d bytes (budget %d)", len(b), toolsListBudget)
	if len(b) > toolsListBudget {
		t.Fatalf("tools/list = %d bytes > budget %d", len(b), toolsListBudget)
	}
}

// TOK-006 with the file tools registered.
func TestToolsListBudgetWithFileTools(t *testing.T) {
	b, err := json.Marshal(mcp.ToolsListResult{Tools: Defs(New(CatalogDeps()))})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("full tools/list payload with file tools: %d bytes (budget %d)", len(b), toolsListBudgetFull)
	if len(b) > toolsListBudgetFull {
		t.Fatalf("tools/list = %d bytes > budget %d", len(b), toolsListBudgetFull)
	}
}

// BUILD-006 / TEST-010: the catalog file is exactly what the registry renders.
func TestCatalogInSync(t *testing.T) {
	want := RenderCatalog(Defs(New(CatalogDeps())))
	raw, err := os.ReadFile(filepath.Join("..", "..", "TOOLS.md"))
	if err != nil {
		t.Fatalf("TOOLS.md missing (run: go run ./cmd/dumptools): %v", err)
	}
	have := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if have != want {
		t.Fatalf("TOOLS.md differs from the registry (run: go run ./cmd/dumptools)\nfile bytes=%d, registry bytes=%d", len(have), len(want))
	}
}

func TestCatalogIsDeterministicAndSorted(t *testing.T) {
	defs := Defs(New(CatalogDeps()))
	rev := append([]mcp.Tool(nil), defs...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	if RenderCatalog(defs) != RenderCatalog(rev) {
		t.Fatal("catalog depends on registry order")
	}
	if RenderCatalog(defs) != RenderCatalog(defs) {
		t.Fatal("catalog is not deterministic")
	}
	c := RenderCatalog(defs)
	last := -1
	for _, n := range []string{"## `chat`", "## `embed`", "## `extract`", "## `generate`", "## `list_models`", "## `summarize_file`", "## `translate`"} {
		i := strings.Index(c, n)
		if i <= last {
			t.Fatalf("%s out of order", n)
		}
		last = i
	}
}

// SEC-012: no code path to the administrative endpoints, in this package or
// in the client. Test files are excluded (they name the paths on purpose).
func TestNoAdminEndpointsInSource(t *testing.T) {
	banned := []string{"/api/pull", "/api/push", "/api/create", "/api/delete", "/api/copy", "/api/blobs"}
	for _, dir := range []string{".", filepath.Join("..", "ollama"), filepath.Join("..", "mcpollama-mcp"), filepath.Join("..", "cmd")} {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, _ := os.ReadFile(p)
			for _, s := range banned {
				if strings.Contains(string(b), s) {
					t.Errorf("%s mentions %s", p, s)
				}
			}
			return nil
		})
	}
}

// TOOL-006: argument helpers live in one file.
func TestArgumentHelpersLiveInOneFile(t *testing.T) {
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || n == "args.go" {
			continue
		}
		b, _ := os.ReadFile(n)
		for _, pat := range []string{"func getString(", "func getInt(", "func getBool(", "func stringArg(", "func intArg(", "func (a args) "} {
			if strings.Contains(string(b), pat) {
				t.Errorf("%s defines an argument helper (%s); keep them in args.go", n, pat)
			}
		}
	}
}
