// Package tools is the single tool registry of mcpollama-mcp (TOOL-006): each
// tool is a definition plus a handler, built by New. The package talks to
// Ollama only through the Backend interface, so tests use httptest and the
// offline catalog dump (cmd/dumptools) runs with no backend at all.
//
// Closed-world declaration (P-9): every tool carries openWorldHint:false. They
// talk only to the private CSL Ollama and (from phase 4) to files inside the
// --allow-root allowlist; none reaches an open set of external entities.
package tools

import (
	"context"
	"errors"

	"github.com/cslsoftwares/mcpollama/files"
	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/ollama"
)

// ServerID is the short server id used in client configs; TOOL-003 bounds
// len(ServerID)+len(tool name) by MaxQualifiedName (the Cursor limit).
const (
	ServerID           = "mcpollama"
	MaxQualifiedName   = 60
	maxOutputChars     = 32000 // ceiling of the text returned to the MCP client (TOK-007)
	maxChatMessages    = 200
	maxEmbedInputs     = 128
	maxVectorItems     = 8 // include_vectors ceiling (TOOL-012)
	maxGeneratePredict = 32768
)

// Backend is the part of *ollama.Client the tools use.
type Backend interface {
	Tags(ctx context.Context) (*ollama.TagsResponse, error)
	PS(ctx context.Context) (*ollama.PSResponse, error)
	Generate(ctx context.Context, req ollama.GenerateRequest) (*ollama.GenerateResponse, error)
	Chat(ctx context.Context, req ollama.ChatRequest) (*ollama.ChatResponse, error)
	Embed(ctx context.Context, req ollama.EmbedRequest) (*ollama.EmbedResponse, error)
}

// Deps are the collaborators of the tools. The zero value is valid for the
// offline catalog dump: no tool ever runs there.
type Deps struct {
	// Backend builds (or returns) the Ollama client on the first call. It is
	// lazy on purpose: nothing talks to Ollama at start or on initialize
	// (PROTO-011, PERF-012). An error becomes an isError result.
	Backend func() (Backend, error)
	// Queue serialises the model calls (PERF-002); nil does not block.
	Queue *Queue
	// Model and EmbedModel are informational (list_models); the client applies
	// the real defaults.
	Model      string
	EmbedModel string
	// AllowedModels is the model allowlist (":latest" implicit). Empty means
	// Model and EmbedModel only; see allowlist.go.
	AllowedModels []string
	// Files is the read gate of the economy tools. nil (no --allow-root) means
	// summarize_file, extract and translate are not registered at all
	// (FILE-001): they are absent from tools/list and tools/call answers
	// "unknown tool" (-32602).
	Files *files.Guard
	// MaxPromptChars is the prompt ceiling the map-reduce splits against; it
	// must equal the ollama client ceiling. 0 means the config default.
	MaxPromptChars int
	// MaxChunks caps the model calls of one file request (FILE-011); 0 means 40.
	MaxChunks int
}

func (d Deps) backend() (Backend, error) {
	if d.Backend == nil {
		return nil, errors.New("nenhum backend Ollama configurado")
	}
	return d.Backend()
}

func readOnlyClosed(title string, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  idempotent,
		OpenWorldHint:   false, // closed world, see the package comment
	}
}

// New returns the registry: the base tools (phase 3) and, only when d.Files is
// set, the file tools (phase 4).
func New(d Deps) []mcp.ToolEntry {
	out := []mcp.ToolEntry{
		d.listModelsTool(),
		d.generateTool(),
		d.chatTool(),
		d.embedTool(),
	}
	if d.Files != nil {
		out = append(out, d.summarizeFileTool(), d.extractTool(), d.translateTool())
	}
	return out
}

// CatalogDeps is the Deps of the offline catalog dump: no backend and a Guard
// with no roots, so every tool is registered and none can read anything. The
// catalog documents the file tools although a running server has them only
// with --allow-root.
func CatalogDeps() Deps { return Deps{Files: &files.Guard{}} }

// Defs returns just the definitions of a registry, in registry order.
func Defs(entries []mcp.ToolEntry) []mcp.Tool {
	out := make([]mcp.Tool, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Def)
	}
	return out
}
