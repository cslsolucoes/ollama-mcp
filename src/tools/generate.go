package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/ollama"
)

func (d Deps) generateTool() mcp.ToolEntry {
	return mcp.ToolEntry{
		Def: mcp.Tool{
			Name:  "generate",
			Title: "Generate text with the local model",
			Description: "Runs a single prompt on the self-hosted Ollama model and returns the answer. " +
				"Use it to offload bulk mechanical work (drafts, rewrites, extraction) to the local model; no state is kept between calls. " +
				"Counts and arithmetic are unreliable: ask for steps or a list and check outside. " +
				"The answer ends with a [uso] line with the token counters.",
			InputSchema: objectSchema([]string{"prompt"}, schemaMap{
				"prompt":      nonEmptyStringProp("The prompt text."),
				"system":      stringProp("Optional system instruction."),
				"model":       stringProp("Optional model name; must be one of the allowed models (see list_models); defaults to the server model (--model)."),
				"temperature": numberProp("Sampling temperature, 0 to 2.", 0, 2),
				"num_predict": intProp("Maximum tokens to generate.", 1, maxGeneratePredict),
				"think":       thinkProp(),
			}),
			Annotations: readOnlyClosed("Generate text with the local model", false),
		},
		Handler: d.generate,
	}
}

// samplingOptions reads the optional temperature and num_predict.
func samplingOptions(a args) (*ollama.ModelOptions, error) {
	temp, err := a.optFloat("temperature", 0, 2)
	if err != nil {
		return nil, err
	}
	n, err := a.optInt("num_predict", 1, maxGeneratePredict)
	if err != nil {
		return nil, err
	}
	if temp == nil && n == nil {
		return nil, nil
	}
	return &ollama.ModelOptions{Temperature: temp, NumPredict: n}, nil
}

func (d Deps) generate(ctx context.Context, raw json.RawMessage) (mcp.ToolCallResult, error) {
	a, err := parseArgs(raw, "prompt", "system", "model", "temperature", "num_predict", "think")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	prompt, err := a.reqString("prompt")
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	system, err := a.optString("system")
	if err != nil {
		return mcp.ToolCallResult{}, err
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

	resp, err := be.Generate(ctx, ollama.GenerateRequest{Model: model, Prompt: prompt, System: system, Options: opts, Think: think})
	if err != nil {
		if resp != nil && resp.Response != "" {
			return partialFailure(resp.Response, err, newJobRef(resp.JobID, roleUnico, resp.Backend)), nil
		}
		return mcp.ToolCallResult{}, err
	}
	if err := thinkExhausted(think, resp.Response, resp.DoneReason); err != nil {
		return mcp.ToolCallResult{}, err
	}
	u := usage{
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
		jobs:       []jobRef{newJobRef(resp.JobID, roleUnico, resp.Backend)},
	}
	d.autoCheck(autoInput{Text: resp.Response, DoneReason: resp.DoneReason, NumPredict: numPredictOf(opts), Jobs: u.jobs})
	return textResult(capText(resp.Response), u), nil
}

// numPredictOf is the num_predict of the request, nil when it did not set one.
func numPredictOf(o *ollama.ModelOptions) *int {
	if o == nil {
		return nil
	}
	return o.NumPredict
}

// acquire takes a GPU queue slot and words the cancellation while waiting.
func (d Deps) acquire(ctx context.Context) (func(), time.Duration, error) {
	release, waited, err := d.Queue.Acquire(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, waited, fmt.Errorf("tempo esgotado esperando a fila da GPU (%s)", waited.Round(time.Millisecond))
		}
		return nil, waited, fmt.Errorf("chamada cancelada enquanto esperava a fila da GPU (%s)", waited.Round(time.Millisecond))
	}
	return release, waited, nil
}

// partialFailure keeps the text that arrived before a mid-stream failure: it
// is an error result (the answer is incomplete) that still carries the text.
func partialFailure(partial string, err error, job jobRef) mcp.ToolCallResult {
	body := fmt.Sprintf("%s\n\n[texto parcial: %d caracteres recebidos antes da falha]\n%s",
		err.Error(), len([]rune(partial)), capText(partial))
	if job.ID != "" {
		body += "\n\njob=" + job.ref() // the job exists in the gateway even though the answer is incomplete
	}
	return mcp.TextResult(body, true)
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
