package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/ollama"
)

const maxListedModels = 200

func (d Deps) listModelsTool() mcp.ToolEntry {
	return mcp.ToolEntry{
		Def: mcp.Tool{
			Name:  "list_models",
			Title: "List installed and loaded models",
			Description: "Lists the allowed models (--allowed-models) installed and resident in VRAM, with the server defaults. " +
				"A resident model outside the allowlist appears on a separate line (it takes VRAM; it cannot be used). " +
				"It does not load any model and does not wait in the GPU queue.",
			InputSchema: objectSchema(nil, schemaMap{}),
			Annotations: readOnlyClosed("List installed and loaded models", true),
		},
		Handler: d.listModels,
	}
}

func gib(b int64) string { return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30)) }

// listModels asks /api/tags and /api/ps one after the other. It bypasses the
// GPU queue on purpose: neither endpoint loads a model, and /api/ps is exactly
// what one wants to read while a generation is running.
func (d Deps) listModels(ctx context.Context, raw json.RawMessage) (mcp.ToolCallResult, error) {
	if _, err := parseArgs(raw); err != nil {
		return mcp.ToolCallResult{}, err
	}
	be, err := d.backend()
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	tags, err := be.Tags(ctx)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	ps, psErr := be.PS(ctx)

	list := d.allowedModels()
	allowed := func(name string) bool { return len(list) == 0 || ModelAllowed(list, name) }
	var installed []ollama.TagModel
	for _, m := range tags.Models {
		if allowed(m.Name) {
			installed = append(installed, m)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Instalados (%d):\n", len(installed))
	writeTags(&b, installed)
	var outside []string
	if psErr != nil {
		fmt.Fprintf(&b, "Em VRAM: indisponível (%s)\n", psErr.Error())
	} else {
		var running []ollama.RunningModel
		for _, m := range ps.Models {
			if allowed(m.Name) {
				running = append(running, m)
			} else {
				outside = append(outside, fmt.Sprintf("%s (vram %s)", m.Name, gib(m.SizeVRAM)))
			}
		}
		fmt.Fprintf(&b, "Em VRAM (%d):\n", len(running))
		writeRunning(&b, running)
	}
	if len(outside) > 0 {
		fmt.Fprintf(&b, "Fora da allowlist (ocupa VRAM): %s\n", strings.Join(outside, ", "))
	}
	if len(list) > 0 {
		fmt.Fprintf(&b, "Modelos permitidos neste servidor: %s\n", strings.Join(list, ", "))
	}
	fmt.Fprintf(&b, "Padrões do servidor MCP: geração=%s embeddings=%s", d.Model, d.EmbedModel)
	return mcp.TextResult(b.String(), false), nil
}

func writeTags(b *strings.Builder, ms []ollama.TagModel) {
	if len(ms) == 0 {
		b.WriteString("- (nenhum)\n")
	}
	for i, m := range ms {
		if i == maxListedModels {
			fmt.Fprintf(b, "- ... mais %d omitidos\n", len(ms)-maxListedModels)
			break
		}
		fmt.Fprintf(b, "- %s  %s  %s %s\n", m.Name, gib(m.Size), m.Details.ParameterSize, m.Details.QuantizationLevel)
	}
}

func writeRunning(b *strings.Builder, ms []ollama.RunningModel) {
	if len(ms) == 0 {
		b.WriteString("- (nenhum: a próxima chamada paga a carga a frio de ~36 s)\n")
	}
	for i, m := range ms {
		if i == maxListedModels {
			fmt.Fprintf(b, "- ... mais %d omitidos\n", len(ms)-maxListedModels)
			break
		}
		fmt.Fprintf(b, "- %s  vram %s  contexto %d  expira %s\n", m.Name, gib(m.SizeVRAM), m.ContextLength, m.ExpiresAt)
	}
}
