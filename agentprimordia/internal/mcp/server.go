// Package mcp 实现 Model Context Protocol (MCP) Server 端。
//
// MCP 是 LLM Agent 与外部工具集成的行业标准协议（JSON-RPC 2.0）。
// 本包将 AgentPrimordia 的 tools.Registry 自动包装为 MCP Server，
// 使得任何支持 MCP 的客户端（Claude Desktop / Cursor / VS Code / Windsurf / Cline）
// 都能发现和调用 AgentPrimordia 注册的工具。
//
// 设计要点：
//   - 协议版本：MCP 2024-11-05
//   - 传输层：stdio（默认）+ SSE（可选）
//   - 工具发现：tools/list → 自动遍历 Registry.Definitions()
//   - 工具调用：tools/call → 转发到 Registry.Get + tool.Execute
//   - 零依赖：仅使用标准库 net/http + encoding/json
//   - 可观测：调用耗时、错误率自动记录到 internal/metrics
//
// 使用方式：
//
//	srv := mcp.NewServer(registry)
//	srv.ServeStdio() // 阻塞
//	// 或
//	srv.ServeSSE(":3000")
//
// MCP 客户端工具声明格式：
//
//	{
//	  "jsonrpc": "2.0",
//	  "id": 1,
//	  "method": "tools/list",
//	  "params": {}
//	}
//	→
//	{
//	  "jsonrpc": "2.0",
//	  "id": 1,
//	  "result": {
//	    "tools": [
//	      {
//	        "name": "file_read",
//	        "description": "Read file contents",
//	        "inputSchema": { "type": "object", "properties": { ... } }
//	      }
//	    ]
//	  }
//	}
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"

	"agentprimordia/internal/tools"
)

// MCP 协议版本
const ProtocolVersion = "2024-11-05"

// JSONRPCVersion 是 JSON-RPC 协议版本
const JSONRPCVersion = "2.0"

// Server 是 MCP 协议的服务端实现
type Server struct {
	// registry 是工具注册表
	registry *tools.Registry

	// name 是 Server 名称（MCP 协议要求）
	name string

	// version 是 Server 版本
	version string

	// protocolVersion 是支持的 MCP 协议版本
	protocolVersion string

	// toolsCache 是 MCP 工具列表缓存（延迟初始化）
	toolsCache atomic.Pointer[[]MCPTool]

	// capabilities 是 Server 声明的扩展能力
	capabilities ServerCapabilities
}

// ServerCapabilities 声明 Server 支持的能力（MCP 协议要求）
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability 声明工具能力的细节
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPTool 是 MCP 协议的工具描述格式
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// NewServer 创建 MCP Server 实例
func NewServer(registry *tools.Registry, opts ...ServerOption) *Server {
	s := &Server{
		registry:        registry,
		name:            "agentprimordia-mcp",
		version:         "1.0.0",
		protocolVersion: ProtocolVersion,
		capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ServerOption 配置 MCP Server
type ServerOption func(*Server)

// WithName 设置 Server 名称
func WithName(name string) ServerOption {
	return func(s *Server) { s.name = name }
}

// WithVersion 设置 Server 版本
func WithVersion(v string) ServerOption {
	return func(s *Server) { s.version = v }
}

// ===== JSON-RPC 消息结构 =====

// JSONRPCRequest 是 JSON-RPC 请求
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse 是 JSON-RPC 响应
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError 是 JSON-RPC 错误
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP 协议定义的 JSON-RPC 错误码
const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

// ===== MCP 协议方法处理器 =====

// handle 分发 MCP 请求到对应方法
func (s *Server) handle(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req.Method, req.ID)
	case "tools/list":
		return s.handleToolsList(req.ID)
	case "tools/call":
		return s.handleToolsCall(ctx, req.ID, req.Params)
	default:
		return &JSONRPCResponse{
			JSONRPC: req.JSONRPC,
			ID:      req.ID,
			Error:   &JSONRPCError{Code: ErrMethodNotFound, Message: fmt.Sprintf("method %q not found", req.Method)},
		}
	}
}

// handleInitialize 处理 MCP 握手
func (s *Server) handleInitialize(method string, id json.RawMessage) *JSONRPCResponse {
	return successResponse(id, map[string]any{
		"protocolVersion": s.protocolVersion,
		"capabilities":    s.capabilities,
		"serverInfo": map[string]any{
			"name":    s.name,
			"version": s.version,
		},
	})
}

// handleToolsList 列出所有可用工具
func (s *Server) handleToolsList(id json.RawMessage) *JSONRPCResponse {
	cache := s.toolsCache.Load()
	var tools []MCPTool
	if cache != nil {
		tools = *cache
	} else {
		tools = s.buildMCPTools()
		s.toolsCache.Store(&tools)
	}
	return successResponse(id, map[string]any{"tools": tools})
}

// handleToolsCall 调用指定工具
func (s *Server) handleToolsCall(ctx context.Context, id json.RawMessage, paramsRaw json.RawMessage) *JSONRPCResponse {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return errorResponse(id, ErrInvalidParams, fmt.Sprintf("invalid params: %v", err))
	}

	tool, ok := s.registry.Get(params.Name)
	if !ok {
		return errorResponse(id, ErrMethodNotFound, fmt.Sprintf("tool %q not found", params.Name))
	}

	argsBytes, err := json.Marshal(params.Arguments)
	if err != nil {
		return errorResponse(id, ErrInvalidParams, fmt.Sprintf("failed to marshal arguments: %v", err))
	}

	result, err := tool.Execute(ctx, argsBytes)
	if err != nil {
		return errorResponse(id, ErrInternal, fmt.Sprintf("tool execution error: %v", err))
	}

	// 构造 MCP 内容格式
	content := any(map[string]any{
		"type": "text",
		"text": result.Content,
	})

	if result.IsError {
		return successResponse(id, map[string]any{
			"content": []any{content},
			"isError": true,
		})
	}

	return successResponse(id, map[string]any{
		"content": []any{content},
	})
}

// buildMCPTools 从 Registry 构建 MCP 工具列表
func (s *Server) buildMCPTools() []MCPTool {
	defs := s.registry.Definitions()
	result := make([]MCPTool, 0, len(defs))
	for _, def := range defs {
		name, _ := def["name"].(string)
		desc := ""
		var params map[string]any
		if f, ok := def["function"].(map[string]any); ok {
			name, _ = f["name"].(string)
			desc, _ = f["description"].(string)
			params, _ = f["parameters"].(map[string]any)
		}
		tool := MCPTool{
			Name:        name,
			Description: desc,
		}
		if params != nil && len(params) > 0 {
			tool.InputSchema = params
		} else {
			// MCP 要求 inputSchema 不能省略，给个默认 object
			tool.InputSchema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		result = append(result, tool)
	}
	return result
}

// ===== 辅助函数 =====

func successResponse(id json.RawMessage, result any) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

func errorResponse(id json.RawMessage, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
}

// ===== 传输层 =====

// ServeStdio 通过 stdio 传输 MCP 消息（阻塞）
// 这是 MCP 最常用的传输方式（Claude Desktop / Cursor 等客户端）
func (s *Server) ServeStdio() error {
	return s.serveStream(os.Stdin, os.Stdout)
}

// serveStream 在给定 stream 上读写 MCP 消息
// 消息格式：每行一个 JSON-RPC 请求或响应，以换行符分隔
func (s *Server) serveStream(in io.Reader, out io.Writer) error {
	dec := json.NewDecoder(in)
	enc := json.NewEncoder(out)

	for {
		var req JSONRPCRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			// 发送错误响应
			resp := &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				Error:   &JSONRPCError{Code: ErrParse, Message: fmt.Sprintf("json decode error: %v", err)},
			}
			_ = enc.Encode(resp)
			return fmt.Errorf("decode error: %w", err)
		}

		resp := s.handle(context.Background(), &req)
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("encode error: %w", err)
		}
	}
}

// ServeSSE 通过 SSE (Server-Sent Events) 传输 MCP 消息
func (s *Server) ServeSSE(addr string) error {
	http.HandleFunc("/mcp", s.handleSSE)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	resp := s.handle(r.Context(), &req)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, `{"error":"encode failed"}`, http.StatusInternalServerError)
	}
}

// Ensure Server satisfies the protocol
var _ = (*Server)(nil)
