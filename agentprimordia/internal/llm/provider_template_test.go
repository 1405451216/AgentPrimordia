//go:build ignore_template

package llm

// ===== 复制此文件并重命名为 {provider}_provider_test.go =====
// 然后全局替换 "Template" 为你的 Provider 名称

// 本文件是 TemplateProvider 误用防护的测试。
// 由于 NewTemplateProvider 现在直接返回 ErrTemplateNotImplemented，
// 模板行为测试改为：断言调用即失败、错误信息含指引。
// 真实 Provider 的测试在 copy 此文件后重写。
//
// 实现新 Provider 时，删除以下"误用防护"测试 + 添加真实测试。

import (
	"strings"
	"testing"
)

// TestNewTemplateProvider_Refused 验证 NewTemplateProvider 启动期拒绝。
// 防止任何代码误把 TemplateProvider 当真 Provider 使用。
func TestNewTemplateProvider_Refused(t *testing.T) {
	provider, err := NewTemplateProvider(Config{
		APIKey: "test-key",
		Model:  "test-model",
	})

	if err == nil {
		t.Fatal("NewTemplateProvider 应返回错误而非成功")
	}
	if provider != nil {
		t.Errorf("期望 nil provider, got %v", provider)
	}
	if err != ErrTemplateNotImplemented {
		t.Errorf("期望 ErrTemplateNotImplemented, got: %v", err)
	}
}

// TestNewTemplateProvider_RefusedNoAPIKey 即便无 API Key 也应拒绝
// (拒绝优先于 API Key 校验，避免"先校验后拒绝"反模式)。
func TestNewTemplateProvider_RefusedNoAPIKey(t *testing.T) {
	provider, err := NewTemplateProvider(Config{})

	if err != ErrTemplateNotImplemented {
		t.Errorf("无 API Key 也应返回 ErrTemplateNotImplemented, got: %v", err)
	}
	if provider != nil {
		t.Errorf("期望 nil provider, got %v", provider)
	}
}

// TestTemplateNotImplemented_ErrorMessage 验证错误信息包含可操作指引。
// 防止"错误信息被简化、丢失指引"导致用户误用。
func TestTemplateNotImplemented_ErrorMessage(t *testing.T) {
	_, err := NewTemplateProvider(Config{APIKey: "test"})
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	wantParts := []string{
		"code template",
		"Copy internal/llm/provider_template.go",
		"ecosystem/contributing/PROVIDER.md",
	}
	for _, want := range wantParts {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息应包含 %q, 实际: %q", want, msg)
		}
	}
}

// TestTemplateProvider_DefaultConstants 验证模板默认常量已设置。
// 贡献者复制模板时应使用这些值（可改）作为占位符。
func TestTemplateProvider_DefaultConstants(t *testing.T) {
	if templateDefaultBaseURL == "" {
		t.Error("templateDefaultBaseURL 应有默认值")
	}
	if defaultTemplateMaxContext <= 0 {
		t.Errorf("defaultTemplateMaxContext 应 > 0, got %d", defaultTemplateMaxContext)
	}
	if defaultTemplateMaxTokens <= 0 {
		t.Errorf("defaultTemplateMaxTokens 应 > 0, got %d", defaultTemplateMaxTokens)
	}
}

// ===== 以下为实现后应添加的测试示例 =====
// 取消注释并替换为实际的 Mock Server 测试

/*
func TestTemplateProvider_Complete_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// 验证认证头
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected Bearer auth, got %s", auth)
		}

		// 返回模拟响应
		resp := map[string]any{
			"id":      "chatcmpl-test123",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "你好！我是模板 Provider 的回复。",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 15,
				"total_tokens":      25,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewTemplateProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})
	provider.client = server.Client()

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "你好"}},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Content != "你好！我是模板 Provider 的回复。" {
		t.Errorf("Content = %q, want '你好！我是模板 Provider 的回复。'", resp.Content)
	}
	if resp.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", resp.Role)
	}
	if resp.Usage.TotalTokens != 25 {
		t.Errorf("TotalTokens = %d, want 25", resp.Usage.TotalTokens)
	}
}

func TestTemplateProvider_Complete_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := map[string]any{
			"error": map[string]string{
				"message": "Invalid API Key",
				"type":    "invalid_request_error",
				"code":    "invalid_api_key",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewTemplateProvider(Config{
		APIKey:  "invalid-key",
		BaseURL: server.URL,
	})

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
}

func TestTemplateProvider_CallTools_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "chatcmpl-tools123",
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_abc123",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city": "北京"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     50,
				"completion_tokens": 20,
				"total_tokens":      70,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewTemplateProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})
	provider.client = server.Client()

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "北京天气如何？"}},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "获取天气信息",
					Parameters:  map[string]any{"city": map[string]any{"type": "string"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTools error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments != `{"city": "北京"}` {
		t.Errorf("arguments = %q, want '{\"city\": \"北京\"}'", resp.ToolCalls[0].Arguments)
	}
}
*/
