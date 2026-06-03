package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mcpHandler 模拟 MCP 服务端
func mcpHandler(responses map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		resp, ok := responses[req.Method]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resp,
		})
	}
}

func TestMCPClient_Initialize_Success(t *testing.T) {
	responses := map[string]any{
		"initialize": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo": map[string]any{
				"name":    "test-server",
				"version": "1.0.0",
			},
		},
		"tools/list": map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "test_tool",
					"description": "A test tool",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
			},
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools := client.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got '%s'", tools[0].Name)
	}
	if tools[0].Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got '%s'", tools[0].Description)
	}
}

func TestMCPClient_Initialize_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	err := client.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestMCPClient_CallTool_Success(t *testing.T) {
	responses := map[string]any{
		"tools/call": map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "hello from MCP",
				},
			},
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	result, err := client.CallTool(context.Background(), "test_tool", map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text != "hello from MCP" {
		t.Errorf("expected 'hello from MCP', got '%s'", result.Content[0].Text)
	}
	if result.IsError {
		t.Error("result should not be error")
	}
}

func TestMCPClient_CallTool_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32600,
				Message: "tool not found",
			},
		})
	}))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	result, err := client.CallTool(context.Background(), "bad_tool", nil)
	if err == nil {
		t.Fatal("expected error for MCP tool error")
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestMCPClient_CallTool_IsError(t *testing.T) {
	responses := map[string]any{
		"tools/call": map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "something went wrong",
				},
			},
			"isError": true,
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	result, err := client.CallTool(context.Background(), "failing_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("result should be error when isError is true")
	}
}

func TestMCPClient_RegisterIntoRegistry(t *testing.T) {
	responses := map[string]any{
		"initialize": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo": map[string]any{
				"name":    "test-server",
				"version": "1.0.0",
			},
		},
		"tools/list": map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "mcp_tool_a",
					"description": "MCP Tool A",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
				map[string]any{
					"name":        "mcp_tool_b",
					"description": "MCP Tool B",
					"inputSchema": nil,
				},
			},
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reg := NewRegistry()
	if err := client.RegisterIntoRegistry(reg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reg.Count() != 2 {
		t.Errorf("expected 2 tools, got %d", reg.Count())
	}

	toolA, ok := reg.Get("mcp_tool_a")
	if !ok {
		t.Fatal("mcp_tool_a should be registered")
	}
	if toolA.Description() != "MCP Tool A" {
		t.Errorf("expected 'MCP Tool A', got '%s'", toolA.Description())
	}

	toolB, ok := reg.Get("mcp_tool_b")
	if !ok {
		t.Fatal("mcp_tool_b should be registered")
	}
	params := toolB.Parameters()
	if params == nil {
		t.Error("parameters should not be nil even with nil InputSchema")
	}
}

func TestMCPToolAdapter_Execute_Basic(t *testing.T) {
	responses := map[string]any{
		"tools/call": map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "result text",
				},
			},
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)

	adapter := &mcpToolAdapter{
		client: client,
		def: MCPToolDefinition{
			Name:        "test_tool",
			Description: "Test tool",
			InputSchema: map[string]any{"type": "object"},
		},
	}

	result, err := adapter.Execute(context.Background(), json.RawMessage(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should not be error: %s", result.Content)
	}
	if result.Content != "result text" {
		t.Errorf("expected 'result text', got '%s'", result.Content)
	}
}

func TestMCPToolAdapter_Execute_InvalidArgs(t *testing.T) {
	responses := map[string]any{
		"tools/call": map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "ok",
				},
			},
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)

	adapter := &mcpToolAdapter{
		client: client,
		def: MCPToolDefinition{
			Name:        "test_tool",
			Description: "Test tool",
		},
	}

	// 无效 JSON 参数应被解析为空 map
	result, err := adapter.Execute(context.Background(), json.RawMessage(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should handle invalid args gracefully: %s", result.Content)
	}
}

func TestMCPToolAdapter_Execute_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32600,
				Message: "execution failed",
			},
		})
	}))
	defer ts.Close()

	client := NewMCPClient(ts.URL)

	adapter := &mcpToolAdapter{
		client: client,
		def: MCPToolDefinition{
			Name:        "failing_tool",
			Description: "Always fails",
		},
	}

	result, err := adapter.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if result == nil || !result.IsError {
		t.Error("result should be error")
	}
}

func TestMCPToolAdapter_Execute_MultipleContent(t *testing.T) {
	responses := map[string]any{
		"tools/call": map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "part1",
				},
				map[string]any{
					"type": "text",
					"text": "part2",
				},
			},
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)

	adapter := &mcpToolAdapter{
		client: client,
		def: MCPToolDefinition{
			Name:        "multi_tool",
			Description: "Returns multiple content",
		},
	}

	result, err := adapter.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should not be error: %s", result.Content)
	}
	if result.Content != "part1\npart2" {
		t.Errorf("expected 'part1\\npart2', got '%s'", result.Content)
	}
}

func TestMCPToolAdapter_Execute_IsError(t *testing.T) {
	responses := map[string]any{
		"tools/call": map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "error content",
				},
			},
			"isError": true,
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)

	adapter := &mcpToolAdapter{
		client: client,
		def: MCPToolDefinition{
			Name:        "error_tool",
			Description: "Returns error",
		},
	}

	result, err := adapter.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when isError is true")
	}
	if result == nil || !result.IsError {
		t.Error("result should be error")
	}
}

func TestMCPToolAdapter_Definitions(t *testing.T) {
	adapter := &mcpToolAdapter{
		def: MCPToolDefinition{
			Name:        "def_tool",
			Description: "Definition test",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
			},
		},
	}

	if adapter.Name() != "def_tool" {
		t.Errorf("expected 'def_tool', got '%s'", adapter.Name())
	}
	if adapter.Description() != "Definition test" {
		t.Errorf("expected 'Definition test', got '%s'", adapter.Description())
	}

	params := adapter.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("parameters should be valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got '%v'", schema["type"])
	}
}

func TestMCPToolAdapter_Definitions_NilSchema(t *testing.T) {
	adapter := &mcpToolAdapter{
		def: MCPToolDefinition{
			Name:        "nil_schema_tool",
			Description: "No schema",
			InputSchema: nil,
		},
	}

	params := adapter.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("parameters should be valid JSON: %v", err)
	}
}

func TestMCPClient_Close(t *testing.T) {
	client := NewMCPClient("http://localhost:9999")
	if err := client.Close(); err != nil {
		t.Errorf("close should not return error: %v", err)
	}
}

func TestMCPClient_Initialize_InvalidURL(t *testing.T) {
	client := NewMCPClient("http://localhost:1")
	err := client.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestMCPClient_RegisterIntoRegistry_NoTools(t *testing.T) {
	client := NewMCPClient("http://localhost:9999")
	// 不初始化，tools 为空

	reg := NewRegistry()
	err := client.RegisterIntoRegistry(reg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if reg.Count() != 0 {
		t.Errorf("expected 0 tools, got %d", reg.Count())
	}
}

func TestMCPClient_Initialize_ToolsListEmpty(t *testing.T) {
	responses := map[string]any{
		"initialize": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo": map[string]any{
				"name":    "empty-server",
				"version": "1.0.0",
			},
		},
		"tools/list": map[string]any{
			"tools": []any{},
		},
	}

	ts := httptest.NewServer(mcpHandler(responses))
	defer ts.Close()

	client := NewMCPClient(ts.URL)
	err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.Tools()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(client.Tools()))
	}
}

// ===== MCP Server 测试 =====

func TestMCPServer_Initialize(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, want 2024-11-05", result["protocolVersion"])
	}
	serverInfo, _ := result["serverInfo"].(map[string]any)
	if serverInfo["name"] != "test-server" {
		t.Errorf("server name = %v, want test-server", serverInfo["name"])
	}
}

func TestMCPServer_ToolsList(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "tool_a", description: "Tool A"})
	_ = reg.Register(&mockTool{name: "tool_b", description: "Tool B"})

	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	toolsRaw, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("tools should be an array")
	}
	if len(toolsRaw) != 2 {
		t.Errorf("tools count = %d, want 2", len(toolsRaw))
	}
}

func TestMCPServer_ToolsCall(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", description: "Echo tool"})

	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"msg":"hello"}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
}

func TestMCPServer_ToolsCall_NotFound(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	body := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nonexistent","arguments":{}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestMCPServer_ResourcesList(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)
	server.AddResource(MCPResourceDefinition{
		URI:         "file:///data/config.json",
		Name:        "Config",
		Description: "Application config",
		MimeType:    "application/json",
	})

	body := `{"jsonrpc":"2.0","id":5,"method":"resources/list"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	resources, ok := result["resources"].([]any)
	if !ok {
		t.Fatal("resources should be an array")
	}
	if len(resources) != 1 {
		t.Errorf("resources count = %d, want 1", len(resources))
	}
}

func TestMCPServer_ResourcesRead(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)
	server.SetResourceHandler(func(ctx context.Context, uri string) (*MCPResourceContent, error) {
		return &MCPResourceContent{
			URI:      uri,
			MimeType: "text/plain",
			Text:     "hello resource",
		}, nil
	})

	body := `{"jsonrpc":"2.0","id":6,"method":"resources/read","params":{"uri":"file:///data/test.txt"}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	contents, ok := result["contents"].([]any)
	if !ok {
		t.Fatal("contents should be an array")
	}
	if len(contents) != 1 {
		t.Errorf("contents count = %d, want 1", len(contents))
	}
}

func TestMCPServer_ResourcesRead_NoHandler(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	body := `{"jsonrpc":"2.0","id":7,"method":"resources/read","params":{"uri":"file:///test"}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Error("expected error when no resource handler")
	}
}

func TestMCPServer_PromptsList(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)
	server.AddPrompt(MCPPromptDefinition{
		Name:        "greet",
		Description: "Greeting prompt",
		Arguments: []MCPPromptArgument{
			{Name: "name", Description: "Person name", Required: true},
		},
	})

	body := `{"jsonrpc":"2.0","id":8,"method":"prompts/list"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	prompts, ok := result["prompts"].([]any)
	if !ok {
		t.Fatal("prompts should be an array")
	}
	if len(prompts) != 1 {
		t.Errorf("prompts count = %d, want 1", len(prompts))
	}
}

func TestMCPServer_PromptsGet(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)
	server.SetPromptHandler(func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error) {
		userMsg := "Hello, " + args["name"]
		return []MCPPromptMessage{
			{
				Role: "user",
				Content: struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: userMsg},
			},
		}, nil
	})

	body := `{"jsonrpc":"2.0","id":9,"method":"prompts/get","params":{"name":"greet","arguments":{"name":"World"}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	messages, ok := result["messages"].([]any)
	if !ok {
		t.Fatal("messages should be an array")
	}
	if len(messages) != 1 {
		t.Errorf("messages count = %d, want 1", len(messages))
	}
}

func TestMCPServer_Ping(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	body := `{"jsonrpc":"2.0","id":10,"method":"ping"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
}

func TestMCPServer_MethodNotFound(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	body := `{"jsonrpc":"2.0","id":11,"method":"unknown/method"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Error("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
}

func TestMCPServer_InvalidJSON(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	req := httptest.NewRequest("POST", "/", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMCPServer_GetMethod(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestMCPToolAdapter_Execute_LargeResultTruncated(t *testing.T) {
	// 创建一个返回超大文本的 MCP 服务端
	largeText := strings.Repeat("x", 200*1024) // 200KB，超过 mcpMaxToolResultLen (100KB)
	handler := mcpHandler(map[string]any{
		"initialize": MCPInitializeResponse{
			ProtocolVersion: mcpProtocolVersion,
			ServerInfo:      MCPServerInfo{Name: "test", Version: "1.0"},
		},
		"tools/list": MCPListToolsResponse{
			Tools: []MCPToolDefinition{
				{Name: "big_tool", Description: "returns big data", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
			},
		},
		"tools/call": MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: largeText}},
		},
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewMCPClient(server.URL)
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	reg := NewRegistry()
	if err := client.RegisterIntoRegistry(reg); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	tool, ok := reg.Get("big_tool")
	if !ok {
		t.Fatal("tool not found")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(result.Content) > mcpMaxToolResultLen+50 { // 50 容差给截断提示
		t.Errorf("result should be truncated near %d bytes, got %d", mcpMaxToolResultLen, len(result.Content))
	}
}

func TestMCPServer_SetExecutor(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{Name: "test-server", Version: "1.0.0"}, reg)

	exec := NewExecutor(reg)
	server.SetExecutor(exec)
}
