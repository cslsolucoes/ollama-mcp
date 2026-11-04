package main

// Replay of the suite `erros` (Fase 12, stage 12.7; C-5.1, C-5.6, C-5.15): the
// canonical request of a case is sent again, through the LOCAL gateway, with the
// same body except `options.seed` (and `model` for the candidate). Nothing leaves
// the machine: the HTTP client refuses any host but the gateway's, ignores the
// proxy variables and follows no redirect, so a case's content cannot reach
// anywhere else, whatever the URL or the answer says (C-5.6).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

const (
	// replayTool is the X-Mcpollama-Tool of every replayed call (C-5.1).
	replayTool = "bakeoff-erros"
	// maxReplayBytes is the ceiling of an answer, as in the gateway (C-2.4).
	maxReplayBytes = 16 << 20
)

// replaySeeds are the seeds of the three paired rounds (C-5.15).
var replaySeeds = []int{1, 2, 3}

// replayEndpoints are the generation endpoints a case may come from (C-3.5): the
// only paths this client will ever call.
var replayEndpoints = map[string]bool{
	"/api/generate": true, "/api/chat": true, "/v1/chat/completions": true, "/v1/completions": true,
}

// guardTransport lets a request through only to the configured host.
type guardTransport struct {
	host string // host:port
	rt   http.RoundTripper
}

func (g guardTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != g.host {
		return nil, fmt.Errorf("host %q recusado: o replay só fala com o gateway %q (C-5.6)", r.URL.Host, g.host)
	}
	return g.rt.RoundTrip(r)
}

// replayClient sends the cases to the gateway.
type replayClient struct {
	base      *url.URL
	hc        *http.Client
	retryWait time.Duration
	dialer    *net.Dialer
	calls     int // counts the calls made (tests and the report)
}

// allowedConnIP says whether the replay may connect to an address: loopback
// always, a private address (RFC 1918 / ULA) only with -gateway-remoto. It is
// checked on the connection itself (net.Dialer.Control), after any name has been
// resolved, so a name cannot lead to a public address.
func allowedConnIP(ip net.IP, allowRemote bool) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || (allowRemote && ip.IsPrivate())
}

// newReplayClient checks the gateway URL: loopback (http or https), or, with
// allowRemote, a LITERAL private IP over https. A hostname or a public IP is
// refused, and the connection is checked again at dial time.
func newReplayClient(raw string, allowRemote bool, timeout time.Duration) (*replayClient, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("-gateway-url inválida")
	}
	h := u.Hostname()
	ip := net.ParseIP(h)
	if !(h == "localhost" || (ip != nil && ip.IsLoopback())) {
		switch {
		case !allowRemote:
			return nil, errors.New("o gateway do replay tem de ser local (loopback): o conteúdo dos casos não sai da máquina (C-5.6); -gateway-remoto só para um IP privado da rede interna, por decisão do owner")
		case ip == nil:
			return nil, errors.New("-gateway-remoto aceita só IP literal privado, não um nome (C-5.6)")
		case !ip.IsPrivate():
			return nil, errors.New("-gateway-remoto recusa IP público (C-5.6)")
		case u.Scheme != "https":
			return nil, errors.New("-gateway-remoto exige https fora de loopback (C-5.6)")
		}
	}
	dialer := newDialer(allowRemote)
	tr := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ResponseHeaderTimeout: timeout, MaxIdleConns: 1}
	hc := &http.Client{
		Transport:     guardTransport{host: u.Host, rt: tr},
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &replayClient{base: u, hc: hc, dialer: dialer, retryWait: 2 * time.Second}, nil
}

// connControl is the Control of the dialer: it runs on the address about to be
// connected to (already resolved), before any packet is sent.
func connControl(allowRemote bool) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil || !allowedConnIP(net.ParseIP(host), allowRemote) {
			return fmt.Errorf("conexão a %q recusada: o replay só fala com loopback ou IP privado autorizado (C-5.6)", host)
		}
		return nil
	}
}

func newDialer(allowRemote bool) *net.Dialer {
	return &net.Dialer{Timeout: 30 * time.Second, Control: connControl(allowRemote)}
}

// withSeedAndModel returns the body of the case with options.seed set (top-level
// `seed` on the /v1 endpoints) and, when model != "", the model replaced. Every
// other value is kept exactly as it was recorded (C-5.1).
func withSeedAndModel(body []byte, endpoint string, seed int, model string) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil || top == nil {
		return nil, errors.New("prompt_json não é um objeto JSON")
	}
	sb, _ := json.Marshal(seed)
	if strings.HasPrefix(endpoint, "/v1/") {
		top["seed"] = sb
	} else {
		opts := map[string]json.RawMessage{}
		if raw, ok := top["options"]; ok && string(raw) != "null" {
			if err := json.Unmarshal(raw, &opts); err != nil {
				return nil, errors.New("options do prompt_json não é um objeto")
			}
		}
		opts["seed"] = sb
		ob, _ := json.Marshal(opts)
		top["options"] = ob
	}
	if model != "" {
		mb, _ := json.Marshal(model)
		top["model"] = mb
	}
	return json.Marshal(top)
}

// do sends one request and returns the parsed answer. 429 and 503 (queue full)
// are retried a few times; any other failure is returned (the case is then not
// measured). The error never carries the body of the answer.
func (c *replayClient) do(ctx context.Context, endpoint string, body []byte) (reply, error) {
	if !replayEndpoints[endpoint] {
		return reply{}, fmt.Errorf("endpoint %q fora da lista de geração", endpoint)
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(c.retryWait):
			case <-ctx.Done():
				return reply{}, ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base.String()+endpoint, bytes.NewReader(body))
		if err != nil {
			return reply{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mcpollama-Tool", replayTool)
		c.calls++
		resp, err := c.hc.Do(req)
		if err != nil {
			return reply{}, fmt.Errorf("chamada ao gateway falhou: %s", oneLine(err.Error(), 120))
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			lastErr = fmt.Errorf("gateway respondeu %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			return reply{}, fmt.Errorf("gateway respondeu %d", resp.StatusCode)
		}
		r, perr := parseReply(endpoint, resp.Body)
		resp.Body.Close()
		return r, perr
	}
	return reply{}, lastErr
}

// parseReply reads the answer of a call: an NDJSON stream (native API), a single
// JSON object or an SSE stream (/v1). Text of the thinking is not part of the
// answer. Truncated is set when the answer hit the ceiling.
func parseReply(endpoint string, body io.Reader) (reply, error) {
	lr := &io.LimitedReader{R: body, N: maxReplayBytes + 1}
	br := bufio.NewReaderSize(lr, 1<<16)
	var out reply
	var text strings.Builder
	v1 := strings.HasPrefix(endpoint, "/v1/")
	for {
		line, err := br.ReadBytes('\n')
		l := bytes.TrimSpace(line)
		if len(l) > 0 {
			if v1 {
				l = bytes.TrimSpace(bytes.TrimPrefix(l, []byte("data:")))
				if string(l) == "[DONE]" {
					out.Final = true
					l = nil
				}
			}
			if len(l) > 0 && l[0] == '{' {
				if perr := parseChunk(l, v1, &text, &out); perr != nil {
					return reply{}, perr
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				return reply{}, errors.New("leitura da resposta interrompida")
			}
			break
		}
	}
	if lr.N <= 0 {
		out.Truncated = true
	}
	out.Text = text.String()
	// An answer that did not end (no done / finish_reason / [DONE]) is a cut stream:
	// the case is NOT measured, in any category (only the 16 MiB ceiling is allowed
	// to end an answer without it).
	if !out.Final && !out.Truncated {
		return reply{}, errors.New("resposta cortada: o stream terminou sem done")
	}
	return out, nil
}

func parseChunk(l []byte, v1 bool, text *strings.Builder, out *reply) error {
	if v1 {
		var c struct {
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Message      struct{ Content string } `json:"message"`
				Delta        struct{ Content string } `json:"delta"`
				Text         string                   `json:"text"`
				FinishReason *string                  `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal(l, &c) != nil {
			return nil // a partial last line (truncated answer): ignored
		}
		if len(c.Error) > 0 && string(c.Error) != "null" {
			return errors.New("o gateway devolveu erro no corpo")
		}
		for _, ch := range c.Choices {
			text.WriteString(ch.Message.Content + ch.Delta.Content + ch.Text)
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				out.Fim, out.Final = *ch.FinishReason, true
			}
		}
		return nil
	}
	var c struct {
		Error      string                   `json:"error"`
		Response   string                   `json:"response"`
		Message    struct{ Content string } `json:"message"`
		Done       bool                     `json:"done"`
		DoneReason string                   `json:"done_reason"`
	}
	if json.Unmarshal(l, &c) != nil {
		return nil
	}
	if c.Error != "" {
		return errors.New("o gateway devolveu erro no corpo")
	}
	text.WriteString(c.Response + c.Message.Content)
	if c.Done {
		out.Final, out.Fim = true, c.DoneReason
	}
	return nil
}
