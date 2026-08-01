package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"context"
	"testing"

	"agentprimordia/internal/tools"
)

// === AgentCardHTTPServer 测试 ===

func TestNewAgentCardHTTPServer_Defaults(t *testing.T) {
	reg := tools.NewRegistry()
	s := NewAgentCardHTTPServer(reg)
	if s.name != "agentprimordia-mcp" {
		t.Errorf("name = %q, want %q", s.name, "agentprimordia-mcp")
	}
	if s.version != "0.8.0" {
		t.Errorf("version = %q, want %q", s.version, "0.8.0")
	}
}

func TestNewAgentCardHTTPServer_WithOptions(t *testing.T) {
	reg := tools.NewRegistry()
	s := NewAgentCardHTTPServer(reg,
		WithServerVersion("1.0.0"),
		WithAgentDescription("test agent"),
	)
	if s.version != "1.0.0" {
		t.Errorf("version = %q, want %q", s.version, "1.0.0")
	}
	if s.description != "test agent" {
		t.Errorf("description = %q, want %q", s.description, "test agent")
	}
}

func TestAgentCardHTTPServer_HandleHealth(t *testing.T) {
	reg := tools.NewRegistry()
	s := NewAgentCardHTTPServer(reg)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health response not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("health status = %v, want %q", body["status"], "ok")
	}
}

func TestAgentCardHTTPServer_HandleTools(t *testing.T) {
	reg := tools.NewRegistry()
	s := NewAgentCardHTTPServer(reg)

	// handleTools 仅接受 POST
	req := httptest.NewRequest("POST", "/tools", nil)
	rec := httptest.NewRecorder()
	s.handleTools(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("tools status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAgentCardHTTPServer_HandleMCP(t *testing.T) {
	reg := tools.NewRegistry()
	s := NewAgentCardHTTPServer(reg)

	// 测试 initialize 请求
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	}
	body, _ := json.Marshal(rpcReq)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleMCP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("mcp status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAgentCardHTTPServer_RegisterRoutes(t *testing.T) {
	reg := tools.NewRegistry()
	s := NewAgentCardHTTPServer(reg)

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	// 验证路由注册成功
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("routed health status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAgentCardHTTPServer_ShutdownTimeout(t *testing.T) {
	reg := tools.NewRegistry()
	s := NewAgentCardHTTPServer(reg)
	d := s.ShutdownTimeout()
	if d <= 0 {
		t.Errorf("ShutdownTimeout() = %v, want > 0", d)
	}
}

// === Server 测试 ===

func TestNewServer_Defaults(t *testing.T) {
	reg := tools.NewRegistry()
	srv := NewServer(reg)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestNewServer_WithOptions(t *testing.T) {
	reg := tools.NewRegistry()
	srv := NewServer(reg, WithName("test-srv"), WithVersion("2.0"))
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestServer_Handle_Initialize(t *testing.T) {
	reg := tools.NewRegistry()
	srv := NewServer(reg)

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
	}
	resp := srv.handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("initialize result is nil")
	}
}

func TestServer_Handle_ToolsList(t *testing.T) {
	reg := tools.NewRegistry()
	srv := NewServer(reg)

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	}
	resp := srv.handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}
}

func TestServer_Handle_UnknownMethod(t *testing.T) {
	reg := tools.NewRegistry()
	srv := NewServer(reg)

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`3`),
		Method:  "unknown/method",
	}
	resp := srv.handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
}

// === 辅助函数测试 ===

func TestGetStringFromAny(t *testing.T) {
	m := map[string]any{"name": "hello", "count": 42}
	if got := getStringFromAny(m, "name"); got != "hello" {
		t.Errorf("getStringFromAny(name) = %q, want %q", got, "hello")
	}
	if got := getStringFromAny(m, "count"); got != "" {
		t.Errorf("getStringFromAny(count) = %q, want empty", got)
	}
	if got := getStringFromAny(m, "missing"); got != "" {
		t.Errorf("getStringFromAny(missing) = %q, want empty", got)
	}
}

func TestGetStringFromDef(t *testing.T) {
	def := map[string]any{"description": "a tool"}
	if got := getStringFromDef(def, "description"); got != "a tool" {
		t.Errorf("getStringFromDef(description) = %q, want %q", got, "a tool")
	}
	if got := getStringFromDef(def, "missing"); got != "" {
		t.Errorf("getStringFromDef(missing) = %q, want empty", got)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"key": "value"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}
