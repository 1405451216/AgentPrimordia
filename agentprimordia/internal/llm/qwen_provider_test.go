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

func TestNewQwenProvider_Success(t *testing.T) {
	provider, err := NewQwenProvider(Config{
		APIKey: "test-qwen-key",
		Model:  "qwen-vl-max-latest",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewQwenProvider_MissingAPIKey(t *testing.T) {
	_, err := NewQwenProvider(Config{})
	if err != ErrAPIKeyRequired {
		t.Errorf("expected ErrAPIKeyRequired, got: %v", err)
	}
}

func TestQwenProvider_Info(t *testing.T) {
	provider, _ := NewQwenProvider(Config{
		APIKey: "test-key",
		Model:  "qwen-vl-plus",
	})

	info := provider.Info()
	infoExt := provider.InfoExt()

	if info.Name != "qwen-vl-plus" {
		t.Errorf("Model name = %q, want qwen-vl-plus", info.Name)
	}
	if info.Provider != "qwen" {
		t.Errorf("Provider = %q, want qwen", info.Provider)
	}
	if !infoExt.SupportsVision {
		t.Error("Qwen-VL should support vision")
	}
	if infoExt.SupportsAudio {
		t.Error("Qwen-VL should not support audio yet")
	}
	if info.MaxContext != 32768 {
		t.Errorf("MaxContext = %d, want 32768", info.MaxContext)
	}
}

func TestBuildMultimodalMessages_Qwen_TextOnly(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewUserTextMessage("你好，通义千问"),
		NewAssistantMessage("你好！有什么我可以帮助你的？"),
	}

	messages := provider.buildMultimodalMessages(msgs)

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	firstMsg := messages[0]
	if firstMsg["role"] != "user" {
		t.Errorf("first msg role = %v, want user", firstMsg["role"])
	}
	if firstMsg["content"].(string) != "你好，通义千问" {
		t.Errorf("content = %v, want '你好，通义千问'", firstMsg["content"])
	}
}

func TestBuildMultimodalMessages_Qwen_WithImage(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	base64Data := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ"

	msgs := []*ChatMessageExt{
		NewUserMultimodalMessage(
			NewTextContent("请描述这张图片："),
			NewImageB64Content(base64Data, "image/png"),
		),
	}

	messages := provider.buildMultimodalMessages(msgs)

	contentParts := messages[0]["content"].([]map[string]any)

	if len(contentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(contentParts))
	}

	textPart := contentParts[0]
	if textPart["type"] != "text" {
		t.Errorf("text part type = %v, want text", textPart["type"])
	}

	imagePart := contentParts[1]
	if imagePart["type"] != "image_url" {
		t.Errorf("image part type = %v, want image_url", imagePart["type"])
	}

	imageURL := imagePart["image_url"].(map[string]string)["url"]
	expectedURL := "data:image/png;base64," + base64Data
	if imageURL != expectedURL {
		t.Errorf("image URL mismatch, got length %d, expected %d", len(imageURL), len(expectedURL))
	}
}

func TestConvertToQwenFormat_Text(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	part := provider.convertToQwenFormat(NewTextContent("Hello"))
	if part == nil {
		t.Fatal("text part should not be nil")
	}
	if part["type"] != "text" {
		t.Errorf("type = %v, want text", part["type"])
	}
	if part["text"] != "Hello" {
		t.Errorf("text = %v, want Hello", part["text"])
	}
}

func TestConvertToQwenFormat_EmptyText(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	part := provider.convertToQwenFormat(NewTextContent(""))
	if part != nil {
		t.Error("empty text should return nil")
	}
}

func TestConvertToQwenFormat_Base64Image(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	part := provider.convertToQwenFormat(NewImageB64Content("data123", "image/jpeg"))

	imageURL := part["image_url"].(map[string]string)["url"]
	expectedURL := "data:image/jpeg;base64,data123"
	if imageURL != expectedURL {
		t.Errorf("image URL = %v, want %v", imageURL, expectedURL)
	}
}

func TestConvertToQwenFormat_ImageURL(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	part := provider.convertToQwenFormat(&MultimodalContent{
		Type: ContentTypeImageURL,
		URL:  "https://example.com/image.jpg",
	})

	imageURL := part["image_url"].(map[string]string)["url"]
	if imageURL != "https://example.com/image.jpg" {
		t.Errorf("image URL = %v, want https://example.com/image.jpg", imageURL)
	}
}

func TestConvertToQwenFormat_Audio(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	part := provider.convertToQwenFormat(NewAudioContent("audiodata", "audio/mp3"))
	if part != nil {
		t.Error("audio should return nil (not supported by Qwen-VL)")
	}
}

func TestCompleteMultimodal_Qwen_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "chatcmpl-test123",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "qwen-vl-max-latest",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "这是一张展示中国长城的图片，背景是蓝天白云，前景是蜿蜒的城墙。",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     100,
				"completion_tokens": 30,
				"total_tokens":      130,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "qwen-vl-max-latest",
	})
	provider.client = server.Client()

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserMultimodalMessage(
				NewTextContent("请用中文描述这张图片："),
				NewImageB64Content("base64greatwall...", "image/jpeg"),
			),
		},
	}

	resp, err := provider.CompleteMultimodal(context.Background(), req)
	if err != nil {
		t.Fatalf("CompleteMultimodal error: %v", err)
	}

	expectedContent := "这是一张展示中国长城的图片，背景是蓝天白云，前景是蜿蜒的城墙。"
	if resp.Content != expectedContent {
		t.Errorf("Content = %q, want %q", resp.Content, expectedContent)
	}
	if resp.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", resp.Role)
	}
	if resp.Usage.TotalTokens != 130 {
		t.Errorf("TotalTokens = %d, want 130", resp.Usage.TotalTokens)
	}
}

func TestCompleteMultimodal_Qwen_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := map[string]any{
			"error": map[string]string{
				"message": "Invalid API Key",
				"type":    "InvalidApiKey",
				"code":    "InvalidApiKey",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "invalid-key",
		BaseURL: server.URL,
	})

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{NewUserTextMessage("test")},
	}

	_, err := provider.CompleteMultimodal(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
}

func TestResolveMaxTokens_Qwen_Default(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 2000 {
		t.Errorf("default max_tokens = %d, want 2000", maxTokens)
	}
}

func TestResolveMaxTokens_Qwen_FromRequest(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{MaxTokens: 1500}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 1500 {
		t.Errorf("max_tokens from request = %d, want 1500", maxTokens)
	}
}

func TestComplete_Qwen_BackwardCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		messages := body["messages"].([]any)
		firstMsg := messages[0].(map[string]any)
		content := firstMsg["content"].(string)

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "回复: " + content,
					},
				},
			},
			"usage": map[string]int{"total_tokens": 20},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "你好，通义千问"},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Content != "回复: 你好，通义千问" {
		t.Errorf("Content = %q, want '回复: 你好，通义千问'", resp.Content)
	}
}

func TestComplexScenario_QwenVL_MultiImage(t *testing.T) {
	provider, _ := NewQwenProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserMultimodalMessage(
				NewTextContent("对比这两张图片的差异：\n图1："),
				NewImageB64Content("image1data", "image/png"),
				NewTextContent("\n\n图2："),
				NewImageB64Content("image2data", "image/jpeg"),
			),
		},
	}

	messages := provider.buildMultimodalMessages(req.Messages)

	userContent := messages[0]["content"].([]map[string]any)

	textCount := 0
	imageCount := 0
	for _, part := range userContent {
		switch part["type"] {
		case "text":
			textCount++
		case "image_url":
			imageCount++
		}
	}

	if textCount != 2 {
		t.Errorf("expected 2 text parts, got %d", textCount)
	}
	if imageCount != 2 {
		t.Errorf("expected 2 image parts, got %d", imageCount)
	}
}

// TestQwenProvider_CallTools_Success 验证 Qwen 工具调用响应解析
// 覆盖：tool_calls 数组、单工具调用、参数解析
func TestQwenProvider_CallTools_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		tools, ok := reqBody["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Error("expected tools in request body")
		}

		resp := map[string]any{
			"id":      "chatcmpl-qwen-tool-1",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "qwen-plus",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_qwen_abc",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"Hangzhou"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     25,
				"completion_tokens": 12,
				"total_tokens":      37,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "qwen-plus",
	})

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "杭州天气怎么样？"}},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "获取指定城市的天气",
					Parameters:  map[string]any{"type": "object"},
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
	if resp.ToolCalls[0].ID != "call_qwen_abc" {
		t.Errorf("tool ID = %q, want call_qwen_abc", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Arguments != `{"city":"Hangzhou"}` {
		t.Errorf("tool arguments = %q", resp.ToolCalls[0].Arguments)
	}
	if resp.Usage.TotalTokens != 37 {
		t.Errorf("total tokens = %d, want 37", resp.Usage.TotalTokens)
	}
}

// TestQwenProvider_CallTools_MultipleTools 验证 Qwen 多个工具调用并行触发
func TestQwenProvider_CallTools_MultipleTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id": "chatcmpl-multi",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"Beijing"}`,
								},
							},
							{
								"id":   "call_2",
								"type": "function",
								"function": map[string]any{
									"name":      "get_time",
									"arguments": `{"timezone":"Asia/Shanghai"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]any{"total_tokens": 50},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "查询"}},
		Tools: []ToolDefinition{
			{Type: "function", Function: FunctionDefinition{Name: "get_weather"}},
			{Type: "function", Function: FunctionDefinition{Name: "get_time"}},
		},
	})
	if err != nil {
		t.Fatalf("CallTools error: %v", err)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}
	names := []string{resp.ToolCalls[0].Name, resp.ToolCalls[1].Name}
	if names[0] != "get_weather" || names[1] != "get_time" {
		t.Errorf("tool names = %v, want [get_weather get_time]", names)
	}
}

// TestQwenProvider_CallTools_NoToolCall 验证模型未发起工具调用时返回空列表
func TestQwenProvider_CallTools_NoToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "你好，我可以直接回答你",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{"total_tokens": 15},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "你好"}},
		Tools:    []ToolDefinition{{Type: "function", Function: FunctionDefinition{Name: "x"}}},
	})
	if err != nil {
		t.Fatalf("CallTools error: %v", err)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.Content != "你好，我可以直接回答你" {
		t.Errorf("content = %q", resp.Content)
	}
}

// TestQwenProvider_Stream_Basic 验证 SSE 流式输出拼接与 Done 信号
func TestQwenProvider_Stream_Basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if !reqBody["stream"].(bool) {
			t.Error("expected stream=true in request body")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"你好\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"，\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"通义千问\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":5,\"total_tokens\":13}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "qwen-turbo",
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

	got := content.String()
	if got != "你好，通义千问" {
		t.Errorf("streamed content = %q, want %q", got, "你好，通义千问")
	}
	if !lastChunk.Done {
		t.Error("last chunk should have Done=true")
	}
}

// TestQwenProvider_Stream_ContextCancel 验证流式 context 取消时提前终止
func TestQwenProvider_Stream_ContextCancel(t *testing.T) {
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

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
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

// TestQwenProvider_Stream_APIError 验证流式请求错误返回
func TestQwenProvider_Stream_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit","code":"rate_limit","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	provider, _ := NewQwenProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected error to contain '429', got %q", err.Error())
	}
}
