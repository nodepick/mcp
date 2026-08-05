package mcp

import (
	"context"
	"encoding/json"
)

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request represents a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  any              `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
}

// Error represents a JSON-RPC 2.0 error structure.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// NewError creates a JSON-RPC response error.
func NewError(code int, message string, data any) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// InitializeResult is the response payload for the "initialize" method.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
}

// ServerCapabilities defines what MCP capabilities this server supports.
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability indicates whether and how tools are supported.
type ToolsCapability struct {
	ListChanged *bool `json:"listChanged,omitempty"`
}

// Implementation defines the server name and version metadata.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ListToolsResult is the response payload for "tools/list".
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// Tool describes an executable tool provided by the server.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the properties and JSON Schema for a tool's arguments.
type InputSchema struct {
	Type       string         `json:"type"` // should be "object"
	Properties map[string]any `json:"properties"`
	Required   []string       `json:"required,omitempty"`
}

// CallToolParams represents the input parameters for "tools/call".
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallToolResult is the response payload for "tools/call".
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents a single block of output (text, image, etc.) from a tool.
type Content struct {
	Type string `json:"type"` // e.g., "text"
	Text string `json:"text"`
}

// ToolHandler represents the execution logic for a specific tool.
type ToolHandler func(ctx context.Context, arguments json.RawMessage) (CallToolResult, error)
