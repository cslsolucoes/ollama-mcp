package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cslsoftwares/mcpollama/mcp"
	"github.com/cslsoftwares/mcpollama/redact"
)

const serverName = "mcpollama"

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ToolHandler runs one tool. A returned error becomes a result with
// isError:true; protocol errors are reserved for the dispatcher.
type ToolHandler func(ctx context.Context, args json.RawMessage) (string, error)

// ToolEntry pairs a tool definition with its handler.
type ToolEntry struct {
	Def     mcp.Tool
	Handler ToolHandler
}

type server struct {
	cfg    Config
	log    *logger
	redact func(string) string
	tools  map[string]ToolEntry
	list   []mcp.Tool // sorted by name

	mu  sync.Mutex // serialises writes to out
	out io.Writer
}

// Run serves JSON-RPC over in/out until EOF (nil error) or a read failure.
// Everything that is not a JSON-RPC message goes to errw.
func Run(ctx context.Context, in io.Reader, out, errw io.Writer, cfg Config, tools []ToolEntry) error {
	red := redact.New(cfg.secrets()...)
	s := &server{
		cfg:    cfg,
		log:    &logger{w: errw, redact: red, debug: cfg.Debug},
		redact: red,
		tools:  make(map[string]ToolEntry, len(tools)),
		out:    out,
	}
	for _, t := range tools {
		if _, dup := s.tools[t.Def.Name]; dup {
			return fmt.Errorf("duplicate tool %q", t.Def.Name)
		}
		s.tools[t.Def.Name] = t
	}
	names := make([]string, 0, len(s.tools))
	for n := range s.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	s.list = make([]mcp.Tool, 0, len(names))
	for _, n := range names {
		d := s.tools[n].Def
		if cfg.ToolsVerbosity == "compact" {
			d = compactTool(d)
		}
		s.list = append(s.list, d)
	}
	s.logStartup()

	r := bufio.NewReaderSize(in, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, tooLong, err := readLine(r, cfg.MaxLineBytes)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		start := time.Now()
		if tooLong {
			s.write(errorResponse(nil, mcp.CodeInvalidRequest,
				fmt.Sprintf("request line exceeds --max-line-bytes (%d)", cfg.MaxLineBytes)))
			s.log.logf("method=- status=error:%d reason=line_too_long dur=%s", mcp.CodeInvalidRequest, time.Since(start))
			continue
		}
		line = bytes.TrimSpace(bytes.TrimPrefix(line, utf8BOM)) // some Windows pipes prepend a BOM
		if len(line) == 0 {
			continue
		}
		resp, info := s.handleLine(ctx, line)
		if resp != nil {
			s.write(*resp)
		}
		s.log.logf("method=%s%s status=%s dur=%s", info.method, info.toolAttr(), info.status, time.Since(start))
		s.log.debugf("method=%s req_bytes=%d resp=%t", info.method, len(line), resp != nil)
	}
}

func (s *server) logStartup() {
	s.log.logf("%s %s ready (stdio)", serverName, s.cfg.Version)
	if s.cfg.Insecure {
		s.log.logf("WARNING: --insecure is set: TLS certificates are NOT verified")
		if s.cfg.TLSCA != "" {
			s.log.logf("WARNING: --tls-ca is ignored because --insecure is set")
		}
	}
	if s.cfg.TraceFile != "" || s.cfg.AuditLogPath != "" {
		s.log.logf("WARNING: --trace-file and --audit-log-path take effect in a later phase; nothing is written")
	}
	s.log.debugf("config url=%s model=%s embed_model=%s timeout=%s api_key_set=%t max_prompt_chars=%d max_line_bytes=%d verbosity=%s",
		s.cfg.safeURL(), s.cfg.Model, s.cfg.EmbedModel, s.cfg.Timeout, s.cfg.APIKey != "",
		s.cfg.MaxPromptChars, s.cfg.MaxLineBytes, s.cfg.ToolsVerbosity)
}

// readLine reads one line without the 64 KB ceiling of bufio.Scanner. A line
// longer than max bytes (newline excluded) is drained and reported as tooLong,
// never as a fatal error. A final line without newline is returned as a line.
func readLine(r *bufio.Reader, max int) (line []byte, tooLong bool, err error) {
	var buf []byte
	for {
		frag, e := r.ReadSlice('\n')
		if !tooLong {
			buf = append(buf, frag...)
			if len(buf) > max+2 { // +2: room for \r\n
				tooLong, buf = true, nil
			}
		}
		switch {
		case e == nil:
		case errors.Is(e, bufio.ErrBufferFull):
			continue
		case errors.Is(e, io.EOF) && (len(buf) > 0 || tooLong):
		default:
			return nil, false, e
		}
		if tooLong {
			return nil, true, nil
		}
		buf = bytes.TrimRight(buf, "\r\n")
		if len(buf) > max {
			return nil, true, nil
		}
		return buf, false, nil
	}
}

func (s *server) write(resp mcp.Response) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil { // Encode appends the newline
		buf.Reset()
		enc.Encode(errorResponse(resp.ID, mcp.CodeInternalError, "cannot encode response"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.out.Write(buf.Bytes()); err != nil {
		s.log.logf("write stdout: %v", err)
	}
}

type callInfo struct {
	method string
	tool   string
	status string
}

func (c callInfo) toolAttr() string {
	if c.tool == "" {
		return ""
	}
	return " tool=" + c.tool
}

func errorResponse(id json.RawMessage, code int, msg string) mcp.Response {
	return mcp.Response{JSONRPC: "2.0", ID: id, Error: &mcp.Error{Code: code, Message: msg}}
}

func resultResponse(id json.RawMessage, result any) mcp.Response {
	return mcp.Response{JSONRPC: "2.0", ID: id, Result: result}
}

// handleLine answers one input line. It returns nil for notifications. A panic
// in dispatch becomes -32603 and never ends the loop.
func (s *server) handleLine(ctx context.Context, line []byte) (resp *mcp.Response, info callInfo) {
	info = callInfo{method: "-", status: "ok"}
	var id json.RawMessage
	defer func() {
		if r := recover(); r != nil {
			s.log.logf("panic recovered in dispatch: %v", r)
			s.log.debugf("%s", debug.Stack())
			e := errorResponse(id, mcp.CodeInternalError, "internal error")
			resp, info.status = &e, "error:-32603"
		}
	}()

	var req mcp.Request
	if err := unmarshalRequest(line, &req); err != nil {
		code, msg := mcp.CodeParseError, "parse error"
		if json.Valid(line) {
			code, msg = mcp.CodeInvalidRequest, "invalid request: expected a JSON object"
		}
		e := errorResponse(nil, code, msg)
		info.status = fmt.Sprintf("error:%d", code)
		return &e, info
	}
	id = req.ID
	info.method = sanitizeLabel(req.Method)
	notification := len(req.ID) == 0

	if notification {
		info.status = "notification"
		return nil, info
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		e := errorResponse(id, mcp.CodeInvalidRequest, `invalid request: need "jsonrpc":"2.0" and a method`)
		info.status = fmt.Sprintf("error:%d", mcp.CodeInvalidRequest)
		return &e, info
	}

	var out mcp.Response
	switch req.Method {
	case "initialize":
		out = s.initialize(id, req.Params)
	case "ping":
		out = resultResponse(id, map[string]any{})
	case "tools/list":
		out = resultResponse(id, mcp.ToolsListResult{Tools: s.list})
	case "tools/call":
		out = s.toolsCall(ctx, id, req.Params, &info)
	default:
		out = errorResponse(id, mcp.CodeMethodNotFound, "method not found: "+req.Method)
	}
	if out.Error != nil {
		info.status = fmt.Sprintf("error:%d", out.Error.Code)
	}
	return &out, info
}

func sanitizeLabel(s string) string {
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == ' ' {
			return '_'
		}
		return r
	}, s)
}

// negotiateVersion returns the requested version when supported, otherwise the
// newest supported one.
func negotiateVersion(requested string) string {
	for _, v := range mcp.SupportedProtocolVersions {
		if v == requested {
			return v
		}
	}
	return mcp.LatestProtocolVersion
}

func (s *server) initialize(id json.RawMessage, params json.RawMessage) mcp.Response {
	var p mcp.InitializeParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return errorResponse(id, mcp.CodeInvalidParams, "invalid initialize params")
		}
	}
	return resultResponse(id, mcp.InitializeResult{
		ProtocolVersion: negotiateVersion(p.ProtocolVersion),
		Capabilities:    mcp.Capabilities{Tools: &mcp.ToolsCapability{ListChanged: false}},
		ServerInfo:      mcp.ServerInfo{Name: serverName, Version: s.cfg.Version},
	})
}

func (s *server) toolsCall(ctx context.Context, id json.RawMessage, params json.RawMessage, info *callInfo) mcp.Response {
	var p mcp.ToolCallParams
	if len(params) == 0 || json.Unmarshal(params, &p) != nil || p.Name == "" {
		return errorResponse(id, mcp.CodeInvalidParams, "invalid tools/call params: object with a tool name required")
	}
	info.tool = sanitizeLabel(p.Name)
	entry, ok := s.tools[p.Name]
	if !ok {
		return errorResponse(id, mcp.CodeInvalidParams, "Unknown tool: "+p.Name)
	}
	args := bytes.TrimSpace(p.Arguments)
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	} else if args[0] != '{' {
		return errorResponse(id, mcp.CodeInvalidParams, "invalid tools/call params: arguments must be an object")
	}
	res := s.callTool(ctx, entry, args)
	if res.IsError {
		info.status = "tool_error"
	}
	return resultResponse(id, res)
}

// callTool runs a handler, turning errors and panics into isError results.
func (s *server) callTool(ctx context.Context, e ToolEntry, args json.RawMessage) (res mcp.ToolCallResult) {
	defer func() {
		if r := recover(); r != nil {
			s.log.logf("panic recovered in tool=%s: %v", e.Def.Name, r)
			s.log.debugf("%s", debug.Stack())
			res = mcp.TextResult(s.redact(fmt.Sprintf("tool %s failed with an internal error", e.Def.Name)), true)
		}
	}()
	text, err := e.Handler(ctx, args)
	if err != nil {
		return mcp.TextResult(s.redact(err.Error()), true)
	}
	return mcp.TextResult(text, false)
}

// compactTool shortens a definition for --tools-verbosity compact: first
// sentence of the description, no per-property descriptions.
func compactTool(t mcp.Tool) mcp.Tool {
	d := t.Description
	if i := strings.Index(d, ". "); i >= 0 {
		d = d[:i+1]
	}
	t.Description = d
	t.InputSchema = stripDescriptions(t.InputSchema)
	return t
}

func stripDescriptions(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == "description" {
			if _, isStr := v.(string); isStr {
				continue
			}
		}
		if sub, ok := v.(map[string]any); ok {
			v = stripDescriptions(sub)
		}
		out[k] = v
	}
	return out
}

// unmarshalRequest decodes one request object. json.Unmarshal accepts the
// literal null into a struct without error, so anything that is not an
// object is rejected here.
func unmarshalRequest(line []byte, req *mcp.Request) error {
	if len(line) == 0 || line[0] != '{' {
		return errors.New("not an object")
	}
	return json.Unmarshal(line, req)
}
