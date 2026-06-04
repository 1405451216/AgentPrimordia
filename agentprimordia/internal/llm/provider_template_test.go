package llm

// ===== 复制此文件并重命名为 {provider}_provider_test.go =====
// 然后全局替换 "Template" 为你的 Provider 名称

import (
	"context"
	"testing"
)

func TestNewTemplateProvider_Success(t *testing.T) {
	provider, err := NewTemplateProvider(Config{
		APIKey: "test-key",
		Model:  "test-model",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewTemplateProvider_MissingAPIKey(t *testing.T) {
	_, err := NewTemplateProvider(Config{})
	if err != ErrAPIKeyRequired {
		t.Errorf("expected ErrAPIKeyRequired, got: %v", err)
	}
}

func TestTemplateProvider_Info(t *testing.T) {
	provider, _ := NewTemplateProvider(Config{
		APIKey: "test-key",
		Model:  "test-model",
	})

	info := provider.Info()
	if info.Provider != "template" {
		t.Errorf("Provider 应为 template，实际为 %s", info.Provider)
	}
	if info.Name != "test-model" {
		t.Errorf("Model 应为 test-model，实际为 %s", info.Name)
	}
	if info.MaxContext != defaultTemplateMaxContext {
		t.Errorf("MaxContext 应为 %d，实际为 %d", defaultTemplateMaxContext, info.MaxContext)
	}
}

func TestTemplateProvider_Complete_NotImplemented(t *testing.T) {
	provider, _ := NewTemplateProvider(Config{
		APIKey: "test-key",
		Model:  "test-model",
	})

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Error("未实现的 Complete 应返回错误")
	}
}

func TestTemplateProvider_Stream_NotImplemented(t *testing.T) {
	provider, _ := NewTemplateProvider(Config{
		APIKey: "test-key",
		Model:  "test-model",
	})

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Error("未实现的 Stream 应返回错误")
	}
}

func TestTemplateProvider_CallTools_NotImplemented(t *testing.T) {
	provider, _ := NewTemplateProvider(Config{
		APIKey: "test-key",
		Model:  "test-model",
	})

	_, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		Tools:    []ToolDefinition{},
	})
	if err == nil {
		t.Error("未实现的 CallTools 应返回错误")
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
