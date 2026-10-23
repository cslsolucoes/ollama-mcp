package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Endpoints (native Ollama API). The list is closed on purpose: no admin
// endpoint exists in this package.
const (
	pathTags     = "/api/tags"
	pathPS       = "/api/ps"
	pathShow     = "/api/show"
	pathGenerate = "/api/generate"
	pathChat     = "/api/chat"
	pathEmbed    = "/api/embed"
)

// Tags lists the installed models (GET /api/tags).
func (c *Client) Tags(ctx context.Context) (*TagsResponse, error) {
	var out TagsResponse
	if err := c.callJSON(ctx, http.MethodGet, pathTags, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PS lists the models resident in VRAM (GET /api/ps).
func (c *Client) PS(ctx context.Context) (*PSResponse, error) {
	var out PSResponse
	if err := c.callJSON(ctx, http.MethodGet, pathPS, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Show returns details of one model (POST /api/show). model empty means the
// client default.
func (c *Client) Show(ctx context.Context, model string) (*ShowResponse, error) {
	m, err := c.pick(model, c.model, "--model")
	if err != nil {
		return nil, err
	}
	var out ShowResponse
	if err := c.callJSON(ctx, http.MethodPost, pathShow, map[string]string{"model": m}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type generateWire struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	System  string        `json:"system,omitempty"`
	Stream  bool          `json:"stream"`
	Options *ModelOptions `json:"options,omitempty"`
}

type chatWire struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  *ModelOptions `json:"options,omitempty"`
}

type embedWire struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// Generate runs POST /api/generate and returns the whole answer.
//
// Stream x non-stream decision (PB5-8): the default is the internal stream
// ("stream":true), aggregated here; the MCP client still gets one result.
// Evidence and reasoning:
//   - The measured failure (VAL §7) happens before the response headers, so
//     neither mode helps with it; that is handled as ErrConnectionDropped.
//   - With "stream":false nothing is sent until the model finishes, so a long
//     generation sits idle behind nginx (proxy_read_timeout) and a failure at
//     the end loses everything. With the stream, bytes flow after the first
//     token, a server error or a cut connection in the middle is detected
//     (a {"error":...} chunk, EOF without done:true) and the text received so
//     far is returned together with the error.
//   - Options.DisableStream selects "stream":false; both paths are tested and
//     decode the same structs.
//
// On a mid-stream failure the returned *GenerateResponse is non-nil and holds
// the partial text; always check err first.
func (c *Client) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	model, err := c.pick(req.Model, c.model, "--model")
	if err != nil {
		return nil, err
	}
	if err := c.checkPrompt(utf8.RuneCountInString(req.Prompt) + utf8.RuneCountInString(req.System)); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	wire := generateWire{Model: model, Prompt: req.Prompt, System: req.System, Stream: !c.noStream, Options: req.Options}
	var out GenerateResponse
	if c.noStream {
		if err := c.callJSON(ctx, http.MethodPost, pathGenerate, wire, &out); err != nil {
			return nil, err
		}
		if out.Error != "" {
			return nil, c.fail(ErrStream, 0, "o servidor reportou erro na geração: %s", c.detail([]byte(out.Error)))
		}
		return &out, nil
	}

	var text strings.Builder
	err = c.streamNDJSON(ctx, pathGenerate, wire, func(line []byte) (bool, error) {
		var ch GenerateResponse
		if err := json.Unmarshal(line, &ch); err != nil {
			return false, c.fail(ErrBadResponse, 0, "linha NDJSON inválida na resposta do servidor")
		}
		if ch.Error != "" {
			return false, c.fail(ErrStream, 0, "o servidor reportou erro durante a geração: %s", c.detail([]byte(ch.Error)))
		}
		text.WriteString(ch.Response)
		if ch.Done {
			out = ch
			return true, nil
		}
		return false, nil
	})
	out.Response = text.String()
	if err != nil {
		if text.Len() == 0 {
			return nil, err
		}
		return &out, err
	}
	return &out, nil
}

// Chat runs POST /api/chat; same stream decision and partial-result rule as
// Generate. The history is whatever the caller sends: no server-side state.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model, err := c.pick(req.Model, c.model, "--model")
	if err != nil {
		return nil, err
	}
	if len(req.Messages) == 0 {
		return nil, c.fail(ErrBadRequest, 0, "chat sem mensagens")
	}
	n := 0
	for _, m := range req.Messages {
		n += utf8.RuneCountInString(m.Content)
	}
	if err := c.checkPrompt(n); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	wire := chatWire{Model: model, Messages: req.Messages, Stream: !c.noStream, Options: req.Options}
	var out ChatResponse
	if c.noStream {
		if err := c.callJSON(ctx, http.MethodPost, pathChat, wire, &out); err != nil {
			return nil, err
		}
		if out.Error != "" {
			return nil, c.fail(ErrStream, 0, "o servidor reportou erro na geração: %s", c.detail([]byte(out.Error)))
		}
		return &out, nil
	}

	var text strings.Builder
	err = c.streamNDJSON(ctx, pathChat, wire, func(line []byte) (bool, error) {
		var ch ChatResponse
		if err := json.Unmarshal(line, &ch); err != nil {
			return false, c.fail(ErrBadResponse, 0, "linha NDJSON inválida na resposta do servidor")
		}
		if ch.Error != "" {
			return false, c.fail(ErrStream, 0, "o servidor reportou erro durante a geração: %s", c.detail([]byte(ch.Error)))
		}
		text.WriteString(ch.Message.Content)
		if ch.Done {
			out = ch
			return true, nil
		}
		return false, nil
	})
	out.Message = ChatMessage{Role: "assistant", Content: text.String()}
	if err != nil {
		if text.Len() == 0 {
			return nil, err
		}
		return &out, err
	}
	return &out, nil
}

// Embed runs POST /api/embed for a batch of inputs. An empty batch, or a
// response without embeddings, is an error ("no embeddings returned").
func (c *Client) Embed(ctx context.Context, req EmbedRequest) (*EmbedResponse, error) {
	model, err := c.pick(req.Model, c.embedModel, "--embed-model")
	if err != nil {
		return nil, err
	}
	if len(req.Input) == 0 {
		return nil, c.fail(ErrBadRequest, 0, "no embeddings returned: lista de entrada vazia")
	}
	n := 0
	for _, s := range req.Input {
		n += utf8.RuneCountInString(s)
	}
	if err := c.checkPrompt(n); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var out EmbedResponse
	if err := c.callJSON(ctx, http.MethodPost, pathEmbed, embedWire{Model: model, Input: req.Input}, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, c.fail(ErrServer, 0, "o servidor reportou erro no embed: %s", c.detail([]byte(out.Error)))
	}
	if len(out.Embeddings) == 0 {
		return nil, c.fail(ErrBadResponse, 0, "no embeddings returned")
	}
	return &out, nil
}

func (c *Client) pick(requested, def, flagName string) (string, error) {
	if m := strings.TrimSpace(requested); m != "" {
		return m, nil
	}
	if def != "" {
		return def, nil
	}
	return "", c.fail(ErrBadRequest, 0, "modelo não informado (configure %s)", flagName)
}

// checkPrompt is the ceiling of PERF-004: an oversized prompt never leaves
// the client.
func (c *Client) checkPrompt(chars int) error {
	if chars > c.maxPrompt {
		return c.fail(ErrPromptTooLong, 0,
			"prompt com %d caracteres excede o teto de %d (--max-prompt-chars); nada foi enviado ao servidor", chars, c.maxPrompt)
	}
	return nil
}

// send performs one exchange. A 2xx response is returned open (the caller
// closes it); any other status becomes a mapped *Error.
func (c *Client) send(ctx context.Context, method, path string, in any, start time.Time) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, c.fail(ErrBadRequest, 0, "não foi possível montar a requisição: %v", err)
		}
		body = bytes.NewReader(b)
	}
	u := *c.base
	u.Path += path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, c.fail(ErrBadRequest, 0, "requisição inválida: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key) // the only place the token is used (SEC-002)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.debugf("%s %s failed after %dms", method, path, time.Since(start).Milliseconds())
		return nil, c.transportErr(ctx, err, start, false, 0)
	}
	c.debugf("%s %s status=%d headers_after=%dms", method, path, resp.StatusCode, time.Since(start).Milliseconds())
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
		return nil, c.statusError(method, path, resp.StatusCode, b)
	}
	return resp, nil
}

// callJSON is the non-stream path: one bounded body, one JSON document. The
// per-call deadline is applied here (a shorter caller deadline still wins).
func (c *Client) callJSON(ctx context.Context, method, path string, in, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	start := time.Now()
	resp, err := c.send(ctx, method, path, in, start)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBody)+1))
	if err != nil {
		return c.transportErr(ctx, err, start, true, len(b))
	}
	if len(b) > c.maxBody {
		return c.fail(ErrBadResponse, 0, "resposta acima do limite de %d bytes", c.maxBody)
	}
	if err := json.Unmarshal(b, out); err != nil {
		return c.fail(ErrBadResponse, 0, "resposta inválida do servidor (JSON malformado em %s)", path)
	}
	c.debugf("%s %s done bytes=%d total=%dms", method, path, len(b), time.Since(start).Milliseconds())
	return nil
}

// streamNDJSON sends the request and feeds each NDJSON line to handle until it
// reports done. It owns the failure modes of a stream: a cut connection
// (ErrConnectionDropped with the bytes received so far), a line above the
// buffer (ErrBadResponse) and EOF before done:true (ErrStream).
func (c *Client) streamNDJSON(ctx context.Context, path string, in any, handle func(line []byte) (bool, error)) error {
	start := time.Now()
	resp, err := c.send(ctx, http.MethodPost, path, in, start)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, min(64*1024, c.maxLine)), c.maxLine) // cap above max would raise the limit
	received := 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		received += len(line) + 1
		if len(line) == 0 {
			continue
		}
		done, err := handle(line)
		if err != nil {
			return err
		}
		if done {
			c.debugf("POST %s stream done bytes=%d total=%dms", path, received, time.Since(start).Milliseconds())
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return c.fail(ErrBadResponse, 0, "linha NDJSON acima do limite de %d bytes (--max-line-bytes)", c.maxLine)
		}
		return c.transportErr(ctx, err, start, true, received)
	}
	if ctx.Err() != nil {
		return c.transportErr(ctx, ctx.Err(), start, true, received)
	}
	return c.fail(ErrStream, 0, "o stream terminou sem done:true após %d bytes: resposta truncada", received)
}
