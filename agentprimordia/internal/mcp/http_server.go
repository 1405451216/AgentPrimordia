// Package mcp 提供 AgentCard HTTP-MCP-Server，
// 用于通过 HTTP REST + SSE 暴露 AgentPrimordia 工具给 MCP 客户端。
//
// MCP 客户端（Claude/Cursor）发现 Agent 的标准方法是：
//
//	MCP 客户端 → GET /sse（MCP 协议握手）
//	         → POST /mcp（JSON-RPC）
//
// 本包通过 HTTP Multiplex 在同一端口提供 MCP 协议端点。
package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"agentprimordia/internal/tools"
)

// A2AServerOption 配置 MCP-over-HTTP 的 Agent 端点。
type A2AServerOption func(*AgentCardHTTPServer)

// WithServerVersion 设置服务端版本。
func WithServerVersion(v string) A2AServerOption {
	return func(s *AgentCardHTTPServer) { s.version = v }
}

// WithAgentDescription 设置 Agent 说明。
func WithAgentDescription(d string) A2AServerOption {
	return func(s *AgentCardHTTPServer) { s.description = d }
}

// AgentCardHTTPServer 提供 MCP over HTTP 端点
type AgentCardHTTPServer struct {
	registry    *tools.Registry
	name        string
	version     string
	description string
}

// NewAgentCardHTTPServer 创建 HTTP MCP Server
func NewAgentCardHTTPServer(registry *tools.Registry, opts ...A2AServerOption) *AgentCardHTTPServer {
	s := &AgentCardHTTPServer{
		registry:    registry,
		name:        "agentprimordia-mcp",
		version:     "0.8.0",
		description: "AgentPrimordia Go Agent OS",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ShutdownTimeout 返回优雅关闭超时
func (s *AgentCardHTTPServer) ShutdownTimeout() time.Duration {
	return 5 * time.Second
}

// RegisterRoutes 注册 HTTP 路由到 mux
// 端点:
//
//	POST /mcp          — JSON-RPC 请求
//	POST /mcp/tools    — 列出所有工具（快捷方式）
//	POST /mcp/call      — 调用工具（快捷方式）
//	GET  /health        — 健康检查
func (s *AgentCardHTTPServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", s.handleMCP)
	mux.HandleFunc("/mcp/tools", s.handleTools)
	mux.HandleFunc("/mcp/call", s.handleCall)
	mux.HandleFunc("/health", s.handleHealth)
}

// handleMCP 处理 JSON-RPC 请求
func (s *AgentCardHTTPServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse(nil, ErrParse, err.Error()))
		return
	}

	srv := NewServer(s.registry, WithName(s.name), WithVersion(s.version))
	resp := srv.handle(r.Context(), &req)
	writeJSON(w, http.StatusOK, resp)
}

// handleTools 列出可用工具
func (s *AgentCardHTTPServer) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	tools := s.buildMCPTools()
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

// handleCall 调用指定工具
func (s *AgentCardHTTPServer) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse(nil, ErrInvalidParams, err.Error()))
		return
	}

	tool, ok := s.registry.Get(params.Name)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse(nil, ErrMethodNotFound, fmt.Sprintf("tool %q not found", params.Name)))
		return
	}

	argsBytes, err := json.Marshal(params.Arguments)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse(nil, ErrInvalidParams, err.Error()))
		return
	}

	result, err := tool.Execute(r.Context(), argsBytes)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse(nil, ErrInternal, err.Error()))
		return
	}

	content := map[string]any{
		"type": "text",
		"text": result.Content,
	}
	if result.IsError {
		writeJSON(w, http.StatusOK, map[string]any{"content": []any{content}, "isError": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": []any{content}})
}

// handleHealth 返回 Agent 健康检查+元信息
func (s *AgentCardHTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"name":        s.name,
		"version":     s.version,
		"description": s.description,
		"protocol":    ProtocolVersion,
	})
}

// buildMCPTools 从 Registry 构建 MCP 工具列表
func (s *AgentCardHTTPServer) buildMCPTools() []MCPTool {
	defs := s.registry.Definitions()
	result := make([]MCPTool, 0, len(defs))
	for _, def := range defs {
		tool := MCPTool{
			Name:        getStringFromDef(def, "name"),
			Description: getStringFromDef(def, "description"),
		}
		if f, ok := def["function"].(map[string]any); ok {
			if tool.Name == "" {
				tool.Name = getStringFromAny(f, "name")
			}
			if tool.Description == "" {
				tool.Description = getStringFromAny(f, "description")
			}
			if params, ok := f["parameters"].(map[string]any); ok {
				tool.InputSchema = params
			}
		} else if params, ok := def["parameters"].(map[string]any); ok {
			tool.InputSchema = params
		}
		if tool.InputSchema == nil {
			tool.InputSchema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		result = append(result, tool)
	}
	return result
}

// getStringFromDef 从 function 字段提取字符串
func getStringFromDef(def map[string]any, key string) string {
	if f, ok := def["function"].(map[string]any); ok {
		if v, ok := f[key].(string); ok {
			return v
		}
	}
	if v, ok := def[key].(string); ok {
		return v
	}
	return ""
}

// getStringFromAny 从 map 提取字符串
func getStringFromAny(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
