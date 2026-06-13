package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDeepSeekProvider_Configuration 验证 DeepSeek 通过 OpenAI 兼容模式配置
// DeepSeek 没有独立实现，依赖 OpenAIProvider 的 OpenAI 兼容接口。
// 关键配置：BaseURL=https://api.deepseek.com/v1, Model=deepseek-chat 或 deepseek-reasoner
func TestDeepSeekProvider_Configuration(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		wantModel   string
		wantBaseURL string
	}{
		{
			name:        "deepseek-chat 通用对话模型",
			model:       "deepseek-chat",
			wantModel:   "deepseek-chat",
			wantBaseURL: "https://api.deepseek.com/v1",
		},
		{
			name:        "deepseek-reasoner 推理模型",
			model:       "deepseek-reasoner",
			wantModel:   "deepseek-reasoner",
			wantBaseURL: "https://api.deepseek.com/v1",
		},
		{
			name:        "deepseek-coder 代码模型",
			model:       "deepseek-coder",
			wantModel:   "deepseek-coder",
			wantBaseURL: "https://api.deepseek.com/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewOpenAIProvider(Config{
				APIKey:  "test-deepseek-key",
				BaseURL: tt.wantBaseURL,
				Model:   tt.model,
			})
			if err != nil {
				t.Fatalf("NewOpenAIProvider error: %v", err)
			}
			if provider == nil {
				t.Fatal("provider should not be nil")
			}
			if info := provider.Info(); info.Name != tt.wantModel {
				t.Errorf("Info().Name = %q, want %q", info.Name, tt.wantModel)
			}
		})
	}
}

// TestDeepSeekProvider_Complete 验证 DeepSeek 文本对话路径
func TestDeepSeekProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-deepseek-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["model"] != "deepseek-chat" {
			t.Errorf("unexpected model: %v", reqBody["model"])
		}

		resp := map[string]any{
			"id":      "chatcmpl-deepseek-1",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "deepseek-chat",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "你好！我是 DeepSeek。",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     11,
				"completion_tokens": 8,
				"total_tokens":      19,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-deepseek-key",
		BaseURL: server.URL,
		Model:   "deepseek-chat",
	})

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "你好"}},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp.Content != "你好！我是 DeepSeek。" {
		t.Errorf("content = %q, want %q", resp.Content, "你好！我是 DeepSeek。")
	}
	if resp.Usage.TotalTokens != 19 {
		t.Errorf("total tokens = %d, want 19", resp.Usage.TotalTokens)
	}
}

// TestDeepSeekProvider_CallTools 验证 DeepSeek 工具调用支持
// deepseek-chat 和 deepseek-reasoner 都支持 OpenAI 兼容的 tool_calls 协议
func TestDeepSeekProvider_CallTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		tools, ok := reqBody["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Error("expected tools in request body")
		}

		resp := map[string]any{
			"id": "chatcmpl-deepseek-tool",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_deepseek_1",
								"type": "function",
								"function": map[string]any{
									"name":      "search_docs",
									"arguments": `{"query":"AgentPrimordia"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]any{"total_tokens": 30},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-deepseek-key",
		BaseURL: server.URL,
		Model:   "deepseek-chat",
	})

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "查询 AgentPrimordia 文档"}},
		Tools: []ToolDefinition{
			{Type: "function", Function: FunctionDefinition{
				Name:        "search_docs",
				Description: "搜索文档",
				Parameters:  map[string]any{"type": "object"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("CallTools error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "search_docs" {
		t.Errorf("tool name = %q, want search_docs", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments != `{"query":"AgentPrimordia"}` {
		t.Errorf("tool arguments = %q", resp.ToolCalls[0].Arguments)
	}
}

// TestDeepSeekProvider_Stream 验证 DeepSeek SSE 流式输出
func TestDeepSeekProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if !reqBody["stream"].(bool) {
			t.Error("expected stream=true in request body")
		}
		if reqBody["model"] != "deepseek-chat" {
			t.Errorf("unexpected model: %v", reqBody["model"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-ds\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"你好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-ds\",\"choices\":[{\"delta\":{\"content\":\"，\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-ds\",\"choices\":[{\"delta\":{\"content\":\"DeepSeek\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-ds\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"total_tokens\":15}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-deepseek-key",
		BaseURL: server.URL,
		Model:   "deepseek-chat",
	})

	ch, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "你好"}},
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var content strings.Builder
	var lastChunk Chunk
	for chunk := range ch {
		content.WriteString(chunk.Content)
		lastChunk = chunk
	}

	if got := content.String(); got != "你好，DeepSeek" {
		t.Errorf("streamed content = %q, want %q", got, "你好，DeepSeek")
	}
	if !lastChunk.Done {
		t.Error("last chunk should have Done=true")
	}
}

// TestDeepSeekProvider_Stream_ContextCancel 验证流式 context 取消时提前终止
func TestDeepSeekProvider_Stream_ContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-deepseek-key",
		BaseURL: server.URL,
		Model:   "deepseek-chat",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ch, err := provider.Stream(ctx, &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	count := 0
	for range ch {
		count++
	}
	if count >= 100 {
		t.Errorf("expected stream to be canceled early, got %d chunks", count)
	}
}

// TestDeepSeekProvider_Stream_APIError 验证流式请求错误返回
func TestDeepSeekProvider_Stream_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","code":"invalid_api_key","type":"authentication_error"}}`))
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "bad-key",
		BaseURL: server.URL,
		Model:   "deepseek-chat",
	})

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to contain '401', got %q", err.Error())
	}
}

// TestDeepSeekProvider_New_NoAPIKey 验证缺失 API Key 时返回明确错误
func TestDeepSeekProvider_New_NoAPIKey(t *testing.T) {
	_, err := NewOpenAIProvider(Config{
		APIKey:  "",
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-chat",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("expected error to mention API key, got %q", err.Error())
	}
}
