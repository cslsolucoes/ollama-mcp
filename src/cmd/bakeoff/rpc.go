package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// rpcResp is one JSON-RPC response line of the server.
type rpcResp struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// client is a minimal MCP stdio client: one request at a time is the normal
// use (the bake-off is sequential), but the reader is concurrent-safe.
type client struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	wmu      sync.Mutex
	mu       sync.Mutex
	pending  map[int64]chan rpcResp
	nextID   atomic.Int64
	dead     chan struct{}
	deadErr  atomic.Value // string
	badLines atomic.Int64 // stdout lines that are not JSON-RPC (protocol errors)
	logf     func(format string, a ...any)
}

func startClient(exe string, args []string, logf func(string, ...any)) (*client, error) {
	cmd := exec.Command(exe, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	so, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	se, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &client{cmd: cmd, stdin: stdin, pending: map[int64]chan rpcResp{}, dead: make(chan struct{}), logf: logf}
	go func() {
		sc := bufio.NewScanner(se)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			logf("SRV-STDERR %s", sc.Text())
		}
	}()
	go func() {
		rd := bufio.NewReaderSize(so, 1<<22)
		for {
			line, err := rd.ReadBytes('\n')
			if len(strings.TrimSpace(string(line))) > 0 {
				var r rpcResp
				if jerr := json.Unmarshal(line, &r); jerr != nil {
					c.badLines.Add(1)
					logf("PROTO linha_stdout_invalida chars=%d", len(line))
				} else if len(r.ID) > 0 && string(r.ID) != "null" {
					if id, perr := strconv.ParseInt(string(r.ID), 10, 64); perr == nil {
						c.mu.Lock()
						ch := c.pending[id]
						delete(c.pending, id)
						c.mu.Unlock()
						if ch != nil {
							ch <- r
						}
					}
				}
			}
			if err != nil {
				c.deadErr.Store("stdout do servidor fechou: " + err.Error())
				close(c.dead)
				return
			}
		}
	}()
	return c, nil
}

func (c *client) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.stdin.Write(append(b, '\n'))
	return err
}

func (c *client) notify(method string, params any) error {
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		m["params"] = params
	}
	return c.send(m)
}

func (c *client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan rpcResp, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if err := c.send(req); err != nil {
		return nil, fmt.Errorf("escrita no servidor falhou: %w", err)
	}
	select {
	case r := <-ch:
		if r.Error != nil {
			return nil, fmt.Errorf("rpc_error %d: %s", r.Error.Code, r.Error.Message)
		}
		return r.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		_ = c.notify("notifications/cancelled", map[string]any{"requestId": id, "reason": "bakeoff timeout"})
		return nil, fmt.Errorf("timeout_driver: %v", ctx.Err())
	case <-c.dead:
		e, _ := c.deadErr.Load().(string)
		return nil, errors.New("servidor_morreu: " + e)
	}
}

func (c *client) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "bakeoff", "version": "1"},
	})
	if err != nil {
		return err
	}
	return c.notify("notifications/initialized", nil)
}

func (c *client) close() {
	_ = c.stdin.Close()
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
	}
}

// callRes is the outcome of one tools/call.
type callRes struct {
	Suite    string `json:"suite"`
	Label    string `json:"label"`
	Tool     string `json:"tool"`
	Start    string `json:"start"`
	WallMS   int64  `json:"wall_ms"`
	IsError  bool   `json:"is_error"`
	Proto    bool   `json:"proto_error"`
	Err      string `json:"err,omitempty"`
	Uso      string `json:"uso,omitempty"`
	PromptE  int64  `json:"prompt_eval_count"`
	EvalE    int64  `json:"eval_count"`
	TotalMS  int64  `json:"total_ms"`
	LoadMS   int64  `json:"load_ms"`
	QueueMS  int64  `json:"queue_ms"`
	Parts    int64  `json:"partes"`
	Fim      string `json:"fim,omitempty"`
	Model    string `json:"modelo,omitempty"`
	BodyFile string `json:"body_file,omitempty"`
	text     string
}

var kvRe = regexp.MustCompile(`(\w+)=(\S+)`)

// parseUso reads the "[uso] k=v ..." footer of a tool answer.
func parseUso(text string) (string, map[string]string) {
	line := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "[uso]") {
			line = strings.TrimSpace(l)
		}
	}
	kv := map[string]string{}
	for _, m := range kvRe.FindAllStringSubmatch(line, -1) {
		kv[m[1]] = m[2]
	}
	return line, kv
}

// bodyOf strips the technical footer ([arquivo], [aviso], [uso]) of an answer.
func bodyOf(text string) string {
	if i := strings.LastIndex(text, "[uso]"); i >= 0 {
		text = text[:i]
	}
	if i := strings.Index(text, "\n[arquivo]"); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "..."
	}
	return s
}

// toolCall runs one tools/call and decodes the answer and its counters.
func (c *client) toolCall(timeout time.Duration, tool string, args map[string]any) callRes {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	raw, err := c.call(ctx, "tools/call", map[string]any{"name": tool, "arguments": args})
	r := callRes{Tool: tool, Start: start.Format("15:04:05.000"), WallMS: time.Since(start).Milliseconds()}
	if err != nil {
		r.Proto, r.IsError, r.Err = true, true, oneLine(err.Error(), 200)
		return r
	}
	var res struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if jerr := json.Unmarshal(raw, &res); jerr != nil {
		r.Proto, r.IsError, r.Err = true, true, "resultado_invalido: "+oneLine(jerr.Error(), 100)
		return r
	}
	var sb strings.Builder
	for _, ct := range res.Content {
		sb.WriteString(ct.Text)
		sb.WriteString("\n")
	}
	r.text = sb.String()
	r.IsError = res.IsError
	if res.IsError {
		r.Err = oneLine(r.text, 200)
	}
	line, kv := parseUso(r.text)
	r.Uso = line
	r.PromptE, _ = strconv.ParseInt(kv["prompt_eval_count"], 10, 64)
	r.EvalE, _ = strconv.ParseInt(kv["eval_count"], 10, 64)
	r.TotalMS, _ = strconv.ParseInt(kv["total_ms"], 10, 64)
	r.LoadMS, _ = strconv.ParseInt(kv["load_ms"], 10, 64)
	r.QueueMS, _ = strconv.ParseInt(kv["queue_ms"], 10, 64)
	r.Parts, _ = strconv.ParseInt(kv["partes"], 10, 64)
	r.Fim = kv["fim"]
	r.Model = kv["modelo"]
	return r
}
