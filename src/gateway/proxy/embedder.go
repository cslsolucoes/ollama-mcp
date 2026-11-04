package proxy

// The gateway's own embedding call (Fase 12, stage 12.8): the query embedding of a
// request that gets examples, and the embedding of the public examples made in the
// background. It goes through the queue like any other embedding, in the EMBED lane
// and with its slots, so it can never take a slot of generation and the queue's
// limits, fairness and monitor see it (tool "melhora-embed"). It talks to the primary
// Ollama only (the queue starts embeddings on the primary: a model's embedding space
// must not change with the backend).
//
// RISK TO MEASURE BEFORE SWITCHING THE IMPROVEMENT ON (M-1 of the review): every embedding of the
// gateway itself (the query of each request and the background ones) shares ONE quota of the queue,
// the pair 127.0.0.1 + label "melhora" (at most --queue-per-client waiting, 16 by default), and the
// embed slots (2 by default) with the stations' own embeddings. Under a burst the excess searches are
// rejected and their requests go on WITHOUT examples (the counter embed_falhas shows it). Measure
// how often that happens with real traffic before relying on the examples.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cslsoftwares/mcpollama/gateway/melhora"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// EmbedTool and EmbedLabel name the gateway's own embeddings in the queue.
const (
	EmbedTool  = "melhora-embed"
	EmbedLabel = "melhora"
)

const maxEmbedAnswer = 4 << 20

// EmbedderConfig configures NewEmbedder.
type EmbedderConfig struct {
	Upstream *url.URL       // the primary Ollama
	Queue    *queue.Manager // the line of the gateway
	Model    string         // default melhora.DefaultEmbedModel
	Client   *http.Client   // optional (tests)
}

// NewEmbedder returns the EmbedFunc of the gateway.
func NewEmbedder(c EmbedderConfig) melhora.EmbedFunc {
	if c.Model == "" {
		c.Model = melhora.DefaultEmbedModel
	}
	if c.Client == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.Proxy = nil // the Ollama is on the loopback
		c.Client = &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return func(ctx context.Context, text string) ([]float32, error) {
		return embedThroughQueue(ctx, c, text)
	}
}

func embedThroughQueue(ctx context.Context, c EmbedderConfig, text string) (vec []float32, err error) {
	tk, err := c.Queue.Acquire(ctx, queue.Request{
		Lane: queue.LaneEmbed, ClientIP: "127.0.0.1", ClientLabel: EmbedLabel, Tool: EmbedTool,
		Endpoint: "/api/embed", Model: c.Model, PriorityIn: queue.Normal, PriorityFinal: queue.Normal, BytesIn: int64(len(text)),
	})
	if err != nil {
		return nil, err
	}
	res := queue.Result{Status: queue.StatusError, ErrorClass: "melhora_embed"}
	defer func() { tk.Release(res) }()
	body, _ := json.Marshal(map[string]any{"model": c.Model, "input": text})
	req, err := http.NewRequestWithContext(tk.Ctx, http.MethodPost, c.Upstream.JoinPath("/api/embed").String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			res.Status, res.ErrorClass, res.HTTPStatus = queue.StatusCanceled, "client_gone", 499
		} else {
			res.ErrorClass, res.HTTPStatus = "upstream_unreachable", 502
		}
		return nil, errors.New("embedding: o Ollama não respondeu")
	}
	defer resp.Body.Close()
	res.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		res.ErrorClass = "upstream_" + strconv.Itoa(resp.StatusCode)
		return nil, fmt.Errorf("embedding: o Ollama respondeu %d", resp.StatusCode)
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxEmbedAnswer)).Decode(&out); err != nil || len(out.Embeddings) != 1 || len(out.Embeddings[0]) == 0 {
		res.ErrorClass = "bad_answer"
		return nil, errors.New("embedding: resposta inválida")
	}
	for _, f := range out.Embeddings[0] {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			res.ErrorClass = "bad_answer"
			return nil, errors.New("embedding: resposta inválida")
		}
	}
	res.Status, res.ErrorClass, res.Responded = queue.StatusOK, "", true
	return out.Embeddings[0], nil
}
