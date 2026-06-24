// Package mcp 实现 Model Context Protocol (MCP) 客户端，
// 允许 AgentPrimordia Agent 连接外部 MCP 服务器并调用其工具。
//
// MCP 协议规范: https://spec.modelcontextprotocol.io/
//
// 本包仅实现客户端功能，通过 JSON-RPC 2.0 over stdio 与 MCP 服务器通信。
package mcp

import "encoding/json"

// ===== MCP 协议核心类型 =====

// ToolDefinition 描述 MCP 服务器暴露的工具
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"` // JSON Schema
}

// ToolCallResult MCP 工具调用结果
type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock 内容块
type ContentBlock struct {
	Type     string `json:"type"`               // "text" 或 "image"
	Text     string `json:"text,omitempty"`      // 文本内容
	Data     string `json:"data,omitempty"`      // base64 编码的图片数据
	MimeType string `json:"mimeType,omitempty"`  // 媒体类型
}

// Resource MCP 资源
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent MCP 资源内容
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // base64 编码
}

// PromptDefinition MCP 提示词模板
type PromptDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Arguments   []PromptArgument    `json:"arguments,omitempty"`
}

// PromptArgument 提示词模板参数
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptMessage 提示词消息
type PromptMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
}

// ServerInfo MCP 服务器信息
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ===== JSON-RPC 2.0 类型 =====

// jsonRPCRequest JSON-RPC 2.0 请求
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCNotification JSON-RPC 2.0 通知（无 ID，无需响应）
type jsonRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse JSON-RPC 2.0 响应
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError JSON-RPC 2.0 错误
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ===== MCP 方法参数/结果类型 =====

// initializeParams initialize 方法的参数
type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// initializeResult initialize 方法的结果
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      ServerInfo     `json:"serverInfo"`
}

// listToolsResult tools/list 方法的结果
type listToolsResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// callToolParams tools/call 方法的参数
type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// listResourcesResult resources/list 方法的结果
type listResourcesResult struct {
	Resources []Resource `json:"resources"`
}

// readResourceParams resources/read 方法的参数
type readResourceParams struct {
	URI string `json:"uri"`
}

// readResourceResult resources/read 方法的结果
type readResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

// listPromptsResult prompts/list 方法的结果
type listPromptsResult struct {
	Prompts []PromptDefinition `json:"prompts"`
}

// getPromptParams prompts/get 方法的参数
type getPromptParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// getPromptResult prompts/get 方法的结果
type getPromptResult struct {
	Messages []PromptMessage `json:"messages"`
}
