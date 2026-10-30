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

var chatRoles = []string{"system", "user", "assistant"}

func (d Deps) chatTool() mcp.ToolEntry {
	return mcp.ToolEntry{
		Def: mcp.Tool{
			Name:  "chat",
			Title: "Chat with the local model",
			Description: "Sends a conversation to the self-hosted Ollama model and returns the next assistant message. " +
				"The server keeps no history: send the whole conversation in messages on every call. " +
				"The answer ends with a [uso] line with the token counters.",
			InputSchema: objectSchema([]string{"messages"}, schemaMap{
				"messages": arrayProp("Conversation so far, oldest first.", objectSchema([]string{"role", "content"}, schemaMap{
					"role":    schemaMap{"type": "string", "enum": chatRoles, "description": "Message author."},
					"content": nonEmptyStringProp("Message text."),
				})),
				"system":      stringProp("Optional system instruction, placed before messages."),
				"model":       stringProp("Optional model name; must be one of the allowed models (see list_models); defaults to the server model (--model)."),
				"temperature": numberProp("Sampling temperature, 0 to 2.", 0, 2),
				"num_predict": intProp("Maximum tokens to generate.", 1, maxGeneratePredict),
				"think":       thinkProp(),
			}),
			Annotations: readOnlyClosed("Chat with the local model", false),
		},
		Handler: d.chat,
	}
}

// parseMessages validates the history sent by the client (OLL-013): known
// roles, non-empty text, bounded count. The total size is checked against the
// prompt ceiling by the client before anything is sent.
func parseMessages(list []any) ([]ollama.ChatMessage, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf("`messages` não pode ser vazio: envie ao menos uma mensagem")
	}
	if len(list) > maxChatMessages {
		return nil, fmt.Errorf("`messages` tem %d itens; o máximo é %d", len(list), maxChatMessages)
	}
	out := make([]ollama.ChatMessage, 0, len(list))
	for i, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("`messages[%d]` must be an object (recebido: %s)", i, jsonType(it))
		}
		item, err := parseArgs(mustJSON(m), "role", "content")
		if err != nil {
			return nil, fmt.Errorf("`messages[%d]`: %w", i, err)
		}
		role, err := item.reqString("role")
		if err != nil {
			return nil, fmt.Errorf("`messages[%d]`: %w", i, err)
		}
		valid := false
		for _, r := range chatRoles {
			valid = valid || role == r
		}
		if !valid {
			return nil, fmt.Errorf("`messages[%d].role` inválido %q: use %s", i, role, strings.Join(chatRoles, ", "))
		}
		content, err := item.reqString("content")
		if err != nil {
			return nil, fmt.Errorf("`messages[%d]`: %w", i, err)
		}
		out = append(out, ollama.ChatMessage{Role: role, Content: content})
	}
	return out, nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (d Deps) chat(ctx context.Context, raw json.RawMessage) (mcp.ToolCallResult, error) {
	a, err := parseArgs(raw, "messages", "system", "model", "temperature", "num_predict", "think")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	list, present, err := a.optList("messages")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	if !present {
		return mcp.ToolCallResult{}, requiredErr("messages")
	}
	msgs, err := parseMessages(list)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	system, err := a.optString("system")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	if strings.TrimSpace(system) != "" {
		msgs = append([]ollama.ChatMessage{{Role: "system", Content: system}}, msgs...)
	}
	model, err := d.modelArg(a)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	opts, err := samplingOptions(a)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	think, err := a.optThink()
	if err != nil {
		return mcp.ToolCallResult{}, err
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

	resp, err := be.Chat(ctx, ollama.ChatRequest{Model: model, Messages: msgs, Options: opts, Think: think})
	if err != nil {
		if resp != nil && resp.Message.Content != "" {
			return partialFailure(resp.Message.Content, err), nil
		}
		return mcp.ToolCallResult{}, err
	}
	if err := thinkExhausted(think, resp.Message.Content, resp.DoneReason); err != nil {
		return mcp.ToolCallResult{}, err
	}
	return textResult(capText(resp.Message.Content), usage{
		model:      orStr(resp.Model, model),
		promptEval: resp.PromptEvalCount,
		eval:       resp.EvalCount,
		hasEval:    true,
		total:      time.Duration(resp.TotalDuration),
		load:       time.Duration(resp.LoadDuration),
		queue:      waited + time.Duration(resp.QueueMS)*time.Millisecond,
		doneReason: resp.DoneReason,
		think:      think,
		backend:    resp.Backend,
	}), nil
}
