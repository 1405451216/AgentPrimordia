package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// mcpServerRequest 辅助函数：构造 JSON-RPC 请求体
func mcpServerRequest(id int, method string, params any) string {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	b, _ := json.Marshal(req)
	return string(b)
}

// mcpDoRequest 辅助函数：向 MCPServer 发送请求并解析响应
func mcpDoRequest(server *MCPServer, method string, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

// mcpDecodeResponse 辅助函数：解码 MCPResponse
func mcpDecodeResponse(rec *httptest.ResponseRecorder) MCPResponse {
	var resp MCPResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return resp
}

// ===== 1. 认证 - API Key 保护 =====

func TestMCPServer_APIKey_无Authorization头(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "auth-server",
		Version: "1.0.0",
		APIKey:  "secret-key",
	}, reg)

	body := mcpServerRequest(1, "ping", nil)
	rec := mcpDoRequest(server, "ping", body, nil)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("缺少 Authorization 头时应返回错误")
	}
	if resp.Error.Code != -32001 {
		t.Errorf("错误码 = %d, 期望 -32001", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "unauthorized") {
		t.Errorf("错误消息应包含 'unauthorized', 实际: %s", resp.Error.Message)
	}
}

func TestMCPServer_APIKey_错误Bearer令牌(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "auth-server",
		Version: "1.0.0",
		APIKey:  "secret-key",
	}, reg)

	body := mcpServerRequest(1, "ping", nil)
	rec := mcpDoRequest(server, "ping", body, map[string]string{
		"Authorization": "Bearer wrong-key",
	})

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("错误的 Bearer 令牌应返回错误")
	}
	if resp.Error.Code != -32001 {
		t.Errorf("错误码 = %d, 期望 -32001", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "unauthorized") {
		t.Errorf("错误消息应包含 'unauthorized', 实际: %s", resp.Error.Message)
	}
}

func TestMCPServer_APIKey_正确Bearer令牌(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "auth-server",
		Version: "1.0.0",
		APIKey:  "secret-key",
	}, reg)

	body := mcpServerRequest(1, "ping", nil)
	rec := mcpDoRequest(server, "ping", body, map[string]string{
		"Authorization": "Bearer secret-key",
	})

	resp := mcpDecodeResponse(rec)
	if resp.Error != nil {
		t.Fatalf("正确的 Bearer 令牌应成功, 错误: %s", resp.Error.Message)
	}
}

func TestMCPServer_APIKey_完整认证流程(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", description: "Echo tool", response: "hello"})
	server := NewMCPServer(MCPServerConfig{
		Name:    "auth-server",
		Version: "1.0.0",
		APIKey:  "secret-key",
	}, reg)

	authHeaders := map[string]string{
		"Authorization": "Bearer secret-key",
	}

	// 无认证请求应被拒绝
	bodyNoAuth := mcpServerRequest(1, "tools/list", nil)
	recNoAuth := mcpDoRequest(server, "tools/list", bodyNoAuth, nil)
	respNoAuth := mcpDecodeResponse(recNoAuth)
	if respNoAuth.Error == nil {
		t.Error("无认证请求应被拒绝")
	}

	// 有认证请求应成功
	bodyAuth := mcpServerRequest(2, "tools/list", nil)
	recAuth := mcpDoRequest(server, "tools/list", bodyAuth, authHeaders)
	respAuth := mcpDecodeResponse(recAuth)
	if respAuth.Error != nil {
		t.Fatalf("有认证请求应成功, 错误: %s", respAuth.Error.Message)
	}
}

// ===== 2. 认证 - 无APIKey配置时无需认证 =====

func TestMCPServer_无APIKey配置_无需认证(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "open-server",
		Version: "1.0.0",
	}, reg)

	body := mcpServerRequest(1, "ping", nil)
	rec := mcpDoRequest(server, "ping", body, nil)

	resp := mcpDecodeResponse(rec)
	if resp.Error != nil {
		t.Fatalf("无 APIKey 配置时请求应成功, 错误: %s", resp.Error.Message)
	}
}

// ===== 3. notifications/initialized =====

func TestMCPServer_NotificationsInitialized(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("notifications/initialized 应无响应体, 实际: %s", rec.Body.String())
	}
}

// ===== 4. tools/call 无效参数 =====

func TestMCPServer_ToolsCall_无效参数(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	// params 为字符串而非对象
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"invalid"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无效参数应返回错误")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("错误码 = %d, 期望 -32602", resp.Error.Code)
	}
}

// ===== 5. tools/call 无Registry =====

func TestMCPServer_ToolsCall_无Registry(t *testing.T) {
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无 Registry 时应返回错误")
	}
	if !strings.Contains(resp.Error.Message, "no tool registry") {
		t.Errorf("错误消息应包含 'no tool registry', 实际: %s", resp.Error.Message)
	}
}

// ===== 6. resources/read 处理器错误 =====

func TestMCPServer_ResourcesRead_处理器错误(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)
	server.SetResourceHandler(func(ctx context.Context, uri string) (*MCPResourceContent, error) {
		return nil, fmt.Errorf("资源读取失败: %s", uri)
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"file:///data/secret.txt"}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("处理器返回错误时应体现在响应中")
	}
	if !strings.Contains(resp.Error.Message, "资源读取失败") {
		t.Errorf("错误消息应包含 '资源读取失败', 实际: %s", resp.Error.Message)
	}
	if resp.Error.Code != -32603 {
		t.Errorf("内部错误码 = %d, 期望 -32603", resp.Error.Code)
	}
}

// ===== 7. prompts/get 无处理器 =====

func TestMCPServer_PromptsGet_无处理器(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	body := `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"greet","arguments":{}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无处理器时应返回错误")
	}
	if !strings.Contains(resp.Error.Message, "prompts not supported") {
		t.Errorf("错误消息应包含 'prompts not supported', 实际: %s", resp.Error.Message)
	}
}

// ===== 8. prompts/get 处理器错误 =====

func TestMCPServer_PromptsGet_处理器错误(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)
	server.SetPromptHandler(func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error) {
		return nil, fmt.Errorf("提示词生成失败: %s", name)
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"broken","arguments":{}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("处理器返回错误时应体现在响应中")
	}
	if !strings.Contains(resp.Error.Message, "提示词生成失败") {
		t.Errorf("错误消息应包含 '提示词生成失败', 实际: %s", resp.Error.Message)
	}
	if resp.Error.Code != -32603 {
		t.Errorf("内部错误码 = %d, 期望 -32603", resp.Error.Code)
	}
}

// ===== 9. prompts/get 非字符串参数过滤 =====

func TestMCPServer_PromptsGet_非字符串参数过滤(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	var capturedArgs map[string]string
	server.SetPromptHandler(func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error) {
		capturedArgs = args
		return []MCPPromptMessage{
			{
				Role: "user",
				Content: struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: "ok"},
			},
		}, nil
	})

	// arguments 包含字符串和非字符串值
	body := `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"test","arguments":{"name":"Alice","count":42,"active":true,"note":null}}}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error != nil {
		t.Fatalf("非字符串参数应被过滤而非报错, 错误: %s", resp.Error.Message)
	}

	// 只有字符串值 "Alice" 应被保留
	if len(capturedArgs) != 1 {
		t.Errorf("期望 1 个字符串参数, 实际 %d 个: %v", len(capturedArgs), capturedArgs)
	}
	if capturedArgs["name"] != "Alice" {
		t.Errorf("name 参数 = %q, 期望 'Alice'", capturedArgs["name"])
	}
	if _, exists := capturedArgs["count"]; exists {
		t.Error("count (int) 应被过滤掉")
	}
	if _, exists := capturedArgs["active"]; exists {
		t.Error("active (bool) 应被过滤掉")
	}
	if _, exists := capturedArgs["note"]; exists {
		t.Error("note (null) 应被过滤掉")
	}
}

// ===== 10. 并发安全 - 并发请求 =====

func TestMCPServer_并发安全(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", description: "Echo tool", response: "pong"})
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)
	server.AddResource(MCPResourceDefinition{
		URI:      "file:///data/test.txt",
		Name:     "Test",
		MimeType: "text/plain",
	})
	server.AddPrompt(MCPPromptDefinition{
		Name:        "greet",
		Description: "Greeting",
	})
	server.SetResourceHandler(func(ctx context.Context, uri string) (*MCPResourceContent, error) {
		return &MCPResourceContent{URI: uri, MimeType: "text/plain", Text: "resource-data"}, nil
	})
	server.SetPromptHandler(func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error) {
		return []MCPPromptMessage{
			{
				Role: "user",
				Content: struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: "prompt-data"},
			},
		}, nil
	})

	methods := []struct {
		method string
		params any
	}{
		{"ping", nil},
		{"tools/list", nil},
		{"initialize", map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "client", "version": "1.0"},
		}},
		{"resources/list", nil},
		{"prompts/list", nil},
		{"tools/call", map[string]any{"name": "echo", "arguments": map[string]any{}}},
		{"resources/read", map[string]any{"uri": "file:///data/test.txt"}},
		{"prompts/get", map[string]any{"name": "greet", "arguments": map[string]any{}}},
	}

	var wg sync.WaitGroup
	var errorCount int64

	for i := 0; i < 50; i++ {
		for _, m := range methods {
			wg.Add(1)
			go func(idx int, method string, params any) {
				defer wg.Done()
				body := mcpServerRequest(idx, method, params)
				req := httptest.NewRequest("POST", "/", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					atomic.AddInt64(&errorCount, 1)
				}

				var resp MCPResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					// notifications/initialized 无响应体，可忽略
					if method != "notifications/initialized" {
						atomic.AddInt64(&errorCount, 1)
					}
				}
			}(i*10+len(methods), m.method, m.params)
		}
	}

	wg.Wait()
	if errorCount > 0 {
		t.Errorf("并发请求中出现 %d 个错误", errorCount)
	}
}

// ===== 11. 安全性 - 请求体大小限制 =====

func TestMCPServer_请求体大小限制(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	// 构造超过 1MB 的请求体
	largeBody := make([]byte, 1<<20+100)
	for i := range largeBody {
		largeBody[i] = 'a'
	}
	largeBodyStr := string(largeBody)

	req := httptest.NewRequest("POST", "/", strings.NewReader(largeBodyStr))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	// 超过大小限制应返回解析错误
	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Error("超过 1MB 的请求体应被拒绝")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("错误码 = %d, 期望 -32700 (解析错误)", resp.Error.Code)
	}
}

// ===== 12. 兼容性 - 完整MCP流程 =====

func TestMCPServer_完整MCP流程(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "calculator", description: "Calculator tool", response: "42"})
	server := NewMCPServer(MCPServerConfig{
		Name:    "flow-server",
		Version: "2.0.0",
	}, reg)
	server.AddResource(MCPResourceDefinition{
		URI:         "file:///data/config.json",
		Name:        "Config",
		Description: "Application config",
		MimeType:    "application/json",
	})
	server.AddPrompt(MCPPromptDefinition{
		Name:        "summarize",
		Description: "Summarize text",
		Arguments: []MCPPromptArgument{
			{Name: "text", Description: "Text to summarize", Required: true},
		},
	})
	server.SetResourceHandler(func(ctx context.Context, uri string) (*MCPResourceContent, error) {
		return &MCPResourceContent{
			URI:      uri,
			MimeType: "application/json",
			Text:     `{"key": "value"}`,
		}, nil
	})
	server.SetPromptHandler(func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error) {
		return []MCPPromptMessage{
			{
				Role: "user",
				Content: struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: "Summarize: " + args["text"]},
			},
		}, nil
	})

	// 步骤1: initialize
	initBody := mcpServerRequest(1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
	})
	rec1 := mcpDoRequest(server, "initialize", initBody, nil)
	resp1 := mcpDecodeResponse(rec1)
	if resp1.Error != nil {
		t.Fatalf("initialize 失败: %s", resp1.Error.Message)
	}
	result1, ok := resp1.Result.(map[string]any)
	if !ok {
		t.Fatal("initialize 结果应为 map")
	}
	if result1["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, 期望 2024-11-05", result1["protocolVersion"])
	}

	// 步骤2: tools/list
	toolsListBody := mcpServerRequest(2, "tools/list", nil)
	rec2 := mcpDoRequest(server, "tools/list", toolsListBody, nil)
	resp2 := mcpDecodeResponse(rec2)
	if resp2.Error != nil {
		t.Fatalf("tools/list 失败: %s", resp2.Error.Message)
	}
	result2, ok := resp2.Result.(map[string]any)
	if !ok {
		t.Fatal("tools/list 结果应为 map")
	}
	tools, ok := result2["tools"].([]any)
	if !ok {
		t.Fatal("tools 应为数组")
	}
	if len(tools) != 1 {
		t.Errorf("tools 数量 = %d, 期望 1", len(tools))
	}

	// 步骤3: tools/call
	toolsCallBody := mcpServerRequest(3, "tools/call", map[string]any{
		"name":      "calculator",
		"arguments": map[string]any{},
	})
	rec3 := mcpDoRequest(server, "tools/call", toolsCallBody, nil)
	resp3 := mcpDecodeResponse(rec3)
	if resp3.Error != nil {
		t.Fatalf("tools/call 失败: %s", resp3.Error.Message)
	}
	result3, ok := resp3.Result.(map[string]any)
	if !ok {
		t.Fatal("tools/call 结果应为 map")
	}
	content, ok := result3["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("content 应为非空数组")
	}

	// 步骤4: resources/list
	resListBody := mcpServerRequest(4, "resources/list", nil)
	rec4 := mcpDoRequest(server, "resources/list", resListBody, nil)
	resp4 := mcpDecodeResponse(rec4)
	if resp4.Error != nil {
		t.Fatalf("resources/list 失败: %s", resp4.Error.Message)
	}
	result4, ok := resp4.Result.(map[string]any)
	if !ok {
		t.Fatal("resources/list 结果应为 map")
	}
	resources, ok := result4["resources"].([]any)
	if !ok {
		t.Fatal("resources 应为数组")
	}
	if len(resources) != 1 {
		t.Errorf("resources 数量 = %d, 期望 1", len(resources))
	}

	// 步骤5: resources/read
	resReadBody := mcpServerRequest(5, "resources/read", map[string]any{
		"uri": "file:///data/config.json",
	})
	rec5 := mcpDoRequest(server, "resources/read", resReadBody, nil)
	resp5 := mcpDecodeResponse(rec5)
	if resp5.Error != nil {
		t.Fatalf("resources/read 失败: %s", resp5.Error.Message)
	}

	// 步骤6: prompts/list
	promptsListBody := mcpServerRequest(6, "prompts/list", nil)
	rec6 := mcpDoRequest(server, "prompts/list", promptsListBody, nil)
	resp6 := mcpDecodeResponse(rec6)
	if resp6.Error != nil {
		t.Fatalf("prompts/list 失败: %s", resp6.Error.Message)
	}
	result6, ok := resp6.Result.(map[string]any)
	if !ok {
		t.Fatal("prompts/list 结果应为 map")
	}
	prompts, ok := result6["prompts"].([]any)
	if !ok {
		t.Fatal("prompts 应为数组")
	}
	if len(prompts) != 1 {
		t.Errorf("prompts 数量 = %d, 期望 1", len(prompts))
	}

	// 步骤7: prompts/get
	promptsGetBody := mcpServerRequest(7, "prompts/get", map[string]any{
		"name":      "summarize",
		"arguments": map[string]any{"text": "hello world"},
	})
	rec7 := mcpDoRequest(server, "prompts/get", promptsGetBody, nil)
	resp7 := mcpDecodeResponse(rec7)
	if resp7.Error != nil {
		t.Fatalf("prompts/get 失败: %s", resp7.Error.Message)
	}

	// 步骤8: ping
	pingBody := mcpServerRequest(8, "ping", nil)
	rec8 := mcpDoRequest(server, "ping", pingBody, nil)
	resp8 := mcpDecodeResponse(rec8)
	if resp8.Error != nil {
		t.Fatalf("ping 失败: %s", resp8.Error.Message)
	}
}

// ===== 13. 错误码验证 =====

func TestMCPServer_错误码_ParseError(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	req := httptest.NewRequest("POST", "/", strings.NewReader("{{{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无效 JSON 应返回错误")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("解析错误码 = %d, 期望 -32700", resp.Error.Code)
	}
}

func TestMCPServer_错误码_MethodNotFound(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	body := mcpServerRequest(1, "nonexistent/method", nil)
	rec := mcpDoRequest(server, "nonexistent/method", body, nil)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("未知方法应返回错误")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("方法未找到错误码 = %d, 期望 -32601", resp.Error.Code)
	}
}

func TestMCPServer_错误码_InvalidParams(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	// tools/call params 为数字
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":12345}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无效参数应返回错误")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("无效参数错误码 = %d, 期望 -32602", resp.Error.Code)
	}
}

func TestMCPServer_错误码_InvalidParams_resources_read(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)
	server.SetResourceHandler(func(ctx context.Context, uri string) (*MCPResourceContent, error) {
		return &MCPResourceContent{URI: uri, Text: "ok"}, nil
	})

	// resources/read params 为数组
	body := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":[1,2,3]}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无效参数应返回错误")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("无效参数错误码 = %d, 期望 -32602", resp.Error.Code)
	}
}

func TestMCPServer_错误码_InvalidParams_prompts_get(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)
	server.SetPromptHandler(func(ctx context.Context, name string, args map[string]string) ([]MCPPromptMessage, error) {
		return []MCPPromptMessage{}, nil
	})

	// prompts/get params 为布尔值
	body := `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":true}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无效参数应返回错误")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("无效参数错误码 = %d, 期望 -32602", resp.Error.Code)
	}
}

// ===== 14. resources/list 空列表 =====

func TestMCPServer_ResourcesList_空列表(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	body := mcpServerRequest(1, "resources/list", nil)
	rec := mcpDoRequest(server, "resources/list", body, nil)

	resp := mcpDecodeResponse(rec)
	if resp.Error != nil {
		t.Fatalf("空资源列表不应返回错误: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("结果应为 map")
	}
	resources, ok := result["resources"].([]any)
	if !ok {
		t.Fatal("resources 应为数组")
	}
	if len(resources) != 0 {
		t.Errorf("空资源列表应返回空数组, 实际长度: %d", len(resources))
	}
}

// ===== 15. prompts/list 空列表 =====

func TestMCPServer_PromptsList_空列表(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	body := mcpServerRequest(1, "prompts/list", nil)
	rec := mcpDoRequest(server, "prompts/list", body, nil)

	resp := mcpDecodeResponse(rec)
	if resp.Error != nil {
		t.Fatalf("空提示词列表不应返回错误: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("结果应为 map")
	}
	prompts, ok := result["prompts"].([]any)
	if !ok {
		t.Fatal("prompts 应为数组")
	}
	if len(prompts) != 0 {
		t.Errorf("空提示词列表应返回空数组, 实际长度: %d", len(prompts))
	}
}

// ===== 16. JSON-RPC 版本验证 =====

func TestMCPServer_InvalidJSONRPCVersion(t *testing.T) {
	reg := NewRegistry()
	server := NewMCPServer(MCPServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}, reg)

	req := MCPRequest{
		JSONRPC: "1.0",
		ID:      1,
		Method:  "initialize",
		Params:  nil,
	}
	b, _ := json.Marshal(req)

	rec := mcpDoRequest(server, "initialize", string(b), nil)

	resp := mcpDecodeResponse(rec)
	if resp.Error == nil {
		t.Fatal("无效 JSON-RPC 版本应返回错误")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("错误码 = %d, 期望 -32600 (Invalid Request)", resp.Error.Code)
	}
}
