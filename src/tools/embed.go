package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/ollama"
)

func (d Deps) embedTool() mcp.ToolEntry {
	return mcp.ToolEntry{
		Def: mcp.Tool{
			Name:  "embed",
			Title: "Compute embeddings with the local embeddings model",
			Description: "Computes embeddings for one text or a batch with the dedicated embeddings model (--embed-model, never the generation model). " +
				"By default it returns only a summary (count, dimensions, counters); set include_vectors for the numbers, limited to a few items. " +
				"Batch texts: if the server keeps one model loaded, embed evicts the generation model (~25 s reload). " +
				"The answer ends with a [uso] line with the counters.",
			InputSchema: objectSchema([]string{"input"}, schemaMap{
				"input": schemaMap{
					"description": "One text, or a list of texts (batch).",
					"oneOf": []any{
						nonEmptyStringProp("One text."),
						arrayProp("A batch of texts.", nonEmptyStringProp("One text.")),
					},
				},
				"model":           stringProp("Optional embeddings model; any model of the allowlist (--allowed-models); defaults to --embed-model."),
				"include_vectors": boolProp(fmt.Sprintf("Return the vectors too (at most %d texts). Default false: summary only.", maxVectorItems)),
			}),
			Annotations: readOnlyClosed("Compute embeddings with the local embeddings model", true),
		},
		Handler: d.embed,
	}
}

// parseEmbedInput accepts a string or a list of strings (OLL-011). An empty
// list is passed on so that the client reports "no embeddings returned".
func parseEmbedInput(a args) ([]string, error) {
	v, ok := a.get("input")
	if !ok {
		return nil, requiredErr("input")
	}
	switch in := v.(type) {
	case string:
		if strings.TrimSpace(in) == "" {
			return nil, fmt.Errorf("`input` não pode ser vazio")
		}
		return []string{in}, nil
	case []any:
		if len(in) > maxEmbedInputs {
			return nil, fmt.Errorf("`input` tem %d textos; o máximo por chamada é %d", len(in), maxEmbedInputs)
		}
		out := make([]string, 0, len(in))
		for i, it := range in {
			s, isStr := it.(string)
			if !isStr {
				return nil, fmt.Errorf("`input[%d]` must be a string (recebido: %s)", i, jsonType(it))
			}
			if strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("`input[%d]` não pode ser vazio", i)
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, typeErr("input", "a string or an array of strings", v)
}

func (d Deps) embed(ctx context.Context, raw json.RawMessage) (mcp.ToolCallResult, error) {
	a, err := parseArgs(raw, "input", "model", "include_vectors")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	input, err := parseEmbedInput(a)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	model, err := d.modelArg(a)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	withVectors, err := a.optBool("include_vectors")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	if withVectors && len(input) > maxVectorItems {
		return mcp.ToolCallResult{}, fmt.Errorf(
			"include_vectors aceita no máximo %d textos (recebidos %d): envie menos textos ou peça só o resumo", maxVectorItems, len(input))
	}

	be, err := d.backend()
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	release, waited, err := d.acquire(ctx)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	defer release()

	resp, err := be.Embed(ctx, ollama.EmbedRequest{Model: model, Input: input})
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	dims := 0
	if len(resp.Embeddings) > 0 {
		dims = len(resp.Embeddings[0])
	}
	u := usage{
		model:      orStr(resp.Model, orStr(model, d.EmbedModel)),
		promptEval: resp.PromptEvalCount,
		total:      time.Duration(resp.TotalDuration),
		load:       time.Duration(resp.LoadDuration),
		queue:      waited + time.Duration(resp.QueueMS)*time.Millisecond,
		backend:    resp.Backend,
	}
	body := fmt.Sprintf("embeddings: %d vetor(es) de %d dimensões, modelo %s", len(resp.Embeddings), dims, u.model)
	if withVectors {
		b, err := json.Marshal(resp.Embeddings) // compact, [] never null
		if err != nil {
			return mcp.ToolCallResult{}, fmt.Errorf("não foi possível serializar os vetores")
		}
		body += "\n" + string(b)
	} else {
		body += " (use include_vectors:true para receber os números)"
	}
	return textResult(body, u), nil
}
