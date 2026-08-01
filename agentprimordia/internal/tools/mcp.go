package tools

import (
	"sync/atomic"
)

const (
	mcpProtocolVersion  = "2024-11-05"
	mcpMaxResponseBody  = 10 * 1024 * 1024
	mcpMaxRequestBody   = 1 << 20
	mcpMaxToolResultLen = 100 * 1024 // 单个 MCP tool结果文本最大长度
)

// ===== MCP 协议类型 =====

// MCPToolDefinition 描述 MCP 服务端提供的tool
type MCPToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// MCPToolCallRequest MCP 调用tool的请求
type MCPToolCallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// MCPToolCallResult MCP 调用tool的结果
type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent MCP 返回的内容块
type MCPContent struct {
	Type string `json:"type"` // "text" | "image" | "resource"
	Text string `json:"text,omitempty"`
}

// MCPInitializeRequest MCP 初始化请求
type MCPInitializeRequest struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      MCPClientInfo  `json:"clientInfo"`
}

// MCPInitializeResponse MCP 初始化响应
type MCPInitializeResponse struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      MCPServerInfo  `json:"serverInfo"`
}

// MCPClientInfo MCP 客户端信息
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerInfo MCP 服务端信息
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPListToolsResponse MCP 列出tool的响应
type MCPListToolsResponse struct {
	Tools []MCPToolDefinition `json:"tools"`
}

// ===== MCP JSON-RPC 类型 =====

// MCPRequest MCP JSON-RPC 请求
type MCPRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// MCPResponse MCP JSON-RPC 响应
type MCPResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *MCPError `json:"error,omitempty"`
}

// MCPError MCP JSON-RPC 错误
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// mcpIDCounter provides auto-incrementing IDs for JSON-RPC requests
var mcpIDCounter int64

// nextMCPID returns the next unique JSON-RPC request ID
func nextMCPID() int {
	return int(atomic.AddInt64(&mcpIDCounter, 1))
}

// MCPClient 连接外部 MCP 服务器，发现并调用其tool
