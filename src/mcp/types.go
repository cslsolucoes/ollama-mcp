// Package mcp defines the JSON-RPC 2.0 and MCP (Model Context Protocol)
// message shapes used by the stdio transport in mcpollama-mcp. Hand-written on
// purpose: the core has no external dependency and no MCP SDK.
package mcp

import "encoding/json"

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// LatestProtocolVersion is the newest revision this server speaks; it is the
// fallback of the initialize negotiation.
const LatestProtocolVersion = "2025-11-25"

// SupportedProtocolVersions lists every protocolVersion the server accepts,
// newest first.
var SupportedProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Request is a JSON-RPC request or notification. ID stays raw so that numbers
// (including 0) and strings come back byte-identical; an empty ID means
// notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response must always carry "id" (null when it is unknown), so ID has no
// omitempty: a nil RawMessage marshals as null.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// MCP-specific types

type InitializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// TextResult builds a single-text-block tool result.
func TextResult(text string, isError bool) ToolCallResult {
	return ToolCallResult{Content: []ContentBlock{{Type: "text", Text: text}}, IsError: isError}
}
