package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"agentprimordia/internal/tools"
)

// mockTool 是测试用的 mock 工具
type mockTool struct {
	name     string
	desc     string
	params   json.RawMessage
	execFunc func(ctx context.Context, args json.RawMessage) (*tools.Result, error)
}

func (t *mockTool) Name() string                { return t.name }
func (t *mockTool) Description() string         { return t.desc }
func (t *mockTool) Parameters() json.RawMessage { return t.params }
func (t *mockTool) Execute(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
	return t.execFunc(ctx, args)
}

func newTestServer() (*Server, *tools.Registry) {
	reg := tools.NewRegistry()
	_ = reg.Register(&mockTool{
		name:   "echo",
		desc:   "echo back input",
		params: json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`),
		execFunc: func(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
			var p struct {
				Msg string `json:"msg"`
			}
			_ = json.Unmarshal(args, &p)
			return tools.NewResult(p.Msg), nil
		},
	})
	srv := NewServer(reg)
	return srv, reg
}

func TestHandle_Initialize(t *testing.T) {
	srv, _ := newTestServer()
	resp := srv.handleInitialize("initialize", []byte("1"))
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected result")
	}
	t.Logf("initialize response: %+v", resp.Result)
}

func TestHandle_ToolsList(t *testing.T) {
	srv, _ := newTestServer()
	resp := srv.handleToolsList([]byte("1"))
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	toolsList, ok := result["tools"].([]MCPTool)
	if !ok {
		t.Fatalf("expected []MCPTool, got %T", result["tools"])
	}
	if len(toolsList) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolsList))
	}
	if toolsList[0].Name != "echo" {
		t.Errorf("expected tool name 'echo', got %q", toolsList[0].Name)
	}
}

func TestHandle_ToolsCall_Success(t *testing.T) {
	srv, _ := newTestServer()
	params, _ := json.Marshal(map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"msg": "hello mcp"},
	})
	resp := srv.handleToolsCall(context.Background(), []byte("2"), params)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	content, ok := result["content"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(content))
	}
}

func TestHandle_ToolsCall_NotFound(t *testing.T) {
	srv, _ := newTestServer()
	params, _ := json.Marshal(map[string]any{
		"name": "nonexistent",
	})
	resp := srv.handleToolsCall(context.Background(), []byte("3"), params)
	if resp.Error == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if resp.Error.Code != ErrMethodNotFound {
		t.Errorf("expected ErrMethodNotFound, got %d", resp.Error.Code)
	}
}

func TestHandle_UnknownMethod(t *testing.T) {
	srv, _ := newTestServer()
	req := &JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      []byte("99"),
		Method:  "unknown/method",
	}
	resp := srv.handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != ErrMethodNotFound {
		t.Errorf("expected ErrMethodNotFound, got %d", resp.Error.Code)
	}
}
