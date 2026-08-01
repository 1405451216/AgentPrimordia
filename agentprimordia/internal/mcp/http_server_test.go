// P0-1: mcp client tests + mcp http server tests
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentprimordia/internal/tools"
)

type mockToolMCPHTTP struct {
	name     string
	desc     string
	params   json.RawMessage
	execFunc func(ctx context.Context, args json.RawMessage) (*tools.Result, error)
}

func (t *mockToolMCPHTTP) Name() string                { return t.name }
func (t *mockToolMCPHTTP) Description() string         { return t.desc }
func (t *mockToolMCPHTTP) Parameters() json.RawMessage { return t.params }
func (t *mockToolMCPHTTP) Execute(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
	if t.execFunc != nil {
		return t.execFunc(ctx, args)
	}
	return tools.NewResult("ok"), nil
}

func newHTTPTestServer() (*AgentCardHTTPServer, *tools.Registry) {
	reg := tools.NewRegistry()
	_ = reg.Register(&mockToolMCPHTTP{
		name:   "echo",
		desc:   "echo input",
		params: json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`),
		execFunc: func(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
			var p struct {
				Msg string `json:"msg"`
			}
			_ = json.Unmarshal(args, &p)
			return tools.NewResult("echo: " + p.Msg), nil
		},
	})
	return NewAgentCardHTTPServer(reg), reg
}

func TestHTTP_MCP_Tools(t *testing.T) {
	srv, _ := newHTTPTestServer()
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/mcp/tools", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(resp.Tools))
	}
	if resp.Tools[0].Name != "echo" {
		t.Errorf("expected tool name 'echo', got %q", resp.Tools[0].Name)
	}
}

func TestHTTP_MCP_Call(t *testing.T) {
	srv, _ := newHTTPTestServer()
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"msg": "hello"},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp/call", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Content []map[string]any `json:"content"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(resp.Content))
	}
	text, _ := resp.Content[0]["text"].(string)
	if !strings.Contains(text, "hello") {
		t.Errorf("expected response to contain 'hello', got %q", text)
	}
}

func TestHTTP_MCP_JSONRPC(t *testing.T) {
	srv, _ := newHTTPTestServer()
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %v", resp.Error)
	}
}

func TestHTTP_Health(t *testing.T) {
	srv, _ := newHTTPTestServer()
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	if resp["name"] == nil {
		t.Errorf("expected name field")
	}
}
