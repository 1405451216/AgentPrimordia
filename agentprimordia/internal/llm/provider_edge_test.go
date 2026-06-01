package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ===== OpenAI 边界情况 =====

func TestOpenAI_Embeddings_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"object": "list",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": []float32{0.1, 0.2, 0.3},
				},
				{
					"object":    "embedding",
					"index":     1,
					"embedding": []float32{0.4, 0.5, 0.6},
				},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]any{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o-mini",
	})

	embeddings, err := provider.Embeddings(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	if len(embeddings[0]) != 3 {
		t.Errorf("expected 3 dimensions, got %d", len(embeddings[0]))
	}
}

func TestOpenAI_Stream_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "Service Unavailable")
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o-mini",
	})

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOpenAI_Stream_APIErrorStructured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"message": "Model not found",
			"type":    "invalid_request_error",
			"code":    "model_not_found",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o-mini",
	})

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOpenAI_BuildMessages_ToolMessages(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]any{
			"id": "test",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o-mini",
	})

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "What's the weather?"},
			{Role: "assistant", Content: "", ToolCalls: []FunctionCall{
				{ID: "call_1", Name: "get_weather", Arguments: `{"city":"Beijing"}`},
			}},
			{Role: "tool", Content: `{"temp": 25}`, ToolCallID: "call_1"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs, ok := capturedBody["messages"].([]any)
	if !ok {
		t.Fatal("expected messages array")
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	assistantMsg, _ := msgs[1].(map[string]any)
	toolCalls, _ := assistantMsg["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Errorf("expected 1 tool_call, got %d", len(toolCalls))
	}

	toolMsg, _ := msgs[2].(map[string]any)
	if toolMsg["tool_call_id"] != "call_1" {
		t.Errorf("expected tool_call_id 'call_1', got %v", toolMsg["tool_call_id"])
	}
}

func TestOpenAI_ResolveModel(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]any{
			"id": "test",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o-mini",
	})

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Model:    "gpt-4o",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBody["model"] != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %v", capturedBody["model"])
	}
}

func TestOpenAI_Complete_ConfigMaxTokens(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]any{
			"id": "test",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		Model:     "gpt-4o-mini",
		MaxTokens: 200,
	})

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBody["max_tokens"] != float64(200) {
		t.Errorf("expected max_tokens 200, got %v", capturedBody["max_tokens"])
	}
}

func TestOpenAI_CallTools_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "test",
			"choices": []map[string]any{},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 0, "total_tokens": 1},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o-mini",
	})

	_, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
		Tools:    []ToolDefinition{{Type: "function", Function: FunctionDefinition{Name: "test"}}},
	})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "empty choices") {
		t.Errorf("expected 'empty choices' error, got '%s'", err.Error())
	}
}

// ===== Anthropic 测试 =====

func newAnthropicTestServer(handler http.HandlerFunc) (*httptest.Server, *AnthropicProvider) {
	server := httptest.NewServer(handler)
	provider, _ := NewAnthropicProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "claude-sonnet-4-20250514",
	})
	return server, provider
}

func TestAnthropicProvider_Complete_Success(t *testing.T) {
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("unexpected x-api-key header: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Errorf("unexpected anthropic-version: %s", r.Header.Get("anthropic-version"))
		}

		resp := map[string]any{
			"id":   "msg-123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "Hello from Claude!"},
			},
			"model": "claude-sonnet-4-20250514",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
			"stop_reason": "end_turn",
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Claude!" {
		t.Errorf("expected 'Hello from Claude!', got '%s'", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestAnthropicProvider_Complete_WithSystem(t *testing.T) {
	var capturedBody map[string]any
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]any{
			"id":   "msg-sys",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "ok"},
			},
			"usage": map[string]any{"input_tokens": 5, "output_tokens": 3},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBody["system"] != "You are helpful" {
		t.Errorf("expected system message, got %v", capturedBody["system"])
	}
}

func TestAnthropicProvider_Complete_EmptyResponse(t *testing.T) {
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "msg-empty",
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]any{},
			"usage":   map[string]any{"input_tokens": 5, "output_tokens": 0},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content, got '%s'", resp.Content)
	}
}

func TestAnthropicProvider_Complete_APIError(t *testing.T) {
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"error": map[string]any{
				"message": "Invalid request",
				"type":    "invalid_request_error",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAnthropicProvider_Complete_HTTPError(t *testing.T) {
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "Rate limited")
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected error to contain '429', got '%s'", err.Error())
	}
}

func TestAnthropicProvider_Stream_Basic(t *testing.T) {
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\" Claude\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	})
	defer server.Close()

	ch, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []Chunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}

	fullContent := ""
	for _, c := range chunks {
		fullContent += c.Content
	}
	if fullContent != "Hello Claude" {
		t.Errorf("expected 'Hello Claude', got '%s'", fullContent)
	}

	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should have Done=true")
	}
}

func TestAnthropicProvider_Stream_Error(t *testing.T) {
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	})
	defer server.Close()

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAnthropicProvider_CallTools_Success(t *testing.T) {
	server, provider := newAnthropicTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		if _, ok := reqBody["tools"]; !ok {
			t.Error("expected tools in request")
		}

		resp := map[string]any{
			"id":   "msg-tool",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "Let me check."},
				{"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": `{"city":"Paris"}`},
			},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 10},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Weather in Paris?"}},
		Tools: []ToolDefinition{
			{Type: "function", Function: FunctionDefinition{
				Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Let me check." {
		t.Errorf("expected 'Let me check.', got '%s'", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected 'get_weather', got '%s'", resp.ToolCalls[0].Name)
	}
}

func TestAnthropicProvider_Embeddings_NotSupported(t *testing.T) {
	provider, _ := NewAnthropicProvider(Config{
		APIKey: "test-key",
		Model:  "claude-sonnet-4-20250514",
	})

	var p Provider = provider
	_, ok := p.(Embedder)
	if ok {
		t.Error("AnthropicProvider should not implement Embedder")
	}
}

func TestAnthropicProvider_Info(t *testing.T) {
	provider, _ := NewAnthropicProvider(Config{
		APIKey: "test-key",
		Model:  "claude-sonnet-4-20250514",
	})

	info := provider.Info()
	if info.Provider != "anthropic" {
		t.Errorf("expected 'anthropic', got '%s'", info.Provider)
	}
	if info.MaxContext != 200000 {
		t.Errorf("expected 200000, got %d", info.MaxContext)
	}
}

func TestAnthropicProvider_Info_UnknownModel(t *testing.T) {
	provider, _ := NewAnthropicProvider(Config{
		APIKey: "test-key",
		Model:  "claude-unknown",
	})

	info := provider.Info()
	if info.MaxContext != 200000 {
		t.Errorf("expected default 200000, got %d", info.MaxContext)
	}
}

func TestAnthropicProvider_New_NoAPIKey(t *testing.T) {
	_, err := NewAnthropicProvider(Config{Model: "claude-sonnet-4-20250514"})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestAnthropicProvider_New_Defaults(t *testing.T) {
	provider, err := NewAnthropicProvider(Config{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.config.BaseURL != anthropicDefaultBaseURL {
		t.Errorf("expected default baseURL, got '%s'", provider.config.BaseURL)
	}
	if provider.config.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected default model, got '%s'", provider.config.Model)
	}
}

// ===== Gemini 测试 =====

func newGeminiTestServer(handler http.HandlerFunc) (*httptest.Server, *GeminiProvider) {
	server := httptest.NewServer(handler)
	provider, _ := NewGeminiProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gemini-2.0-flash",
	})
	return server, provider
}

func TestGeminiProvider_Complete_Success(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "Hello from Gemini!"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     10,
				"candidatesTokenCount": 5,
				"totalTokenCount":      15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Gemini!" {
		t.Errorf("expected 'Hello from Gemini!', got '%s'", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestGeminiProvider_Complete_WithSystem(t *testing.T) {
	var capturedBody map[string]any
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{{"text": "ok"}},
						"role":   "model",
					},
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount": 5, "candidatesTokenCount": 3, "totalTokenCount": 8,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sysInstr, ok := capturedBody["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("expected systemInstruction in request body")
	}
	parts, _ := sysInstr["parts"].([]any)
	if len(parts) == 0 {
		t.Error("expected system instruction parts")
	}
}

func TestGeminiProvider_Complete_APIError(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"error": map[string]any{
				"message": "API key not valid",
				"type":    "INVALID_ARGUMENT",
				"code":    400,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGeminiProvider_Complete_HTTPError(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "Rate limited")
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGeminiProvider_Complete_EmptyResponse(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{},
			"usageMetadata": map[string]any{
				"promptTokenCount": 5, "candidatesTokenCount": 0, "totalTokenCount": 5,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content, got '%s'", resp.Content)
	}
}

func TestGeminiProvider_Stream_Basic(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}],\"role\":\"model\"},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":3,\"totalTokenCount\":8}}\n\n")
	})
	defer server.Close()

	ch, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []Chunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0].Content != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", chunks[0].Content)
	}
}

func TestGeminiProvider_Stream_Error(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal error")
	})
	defer server.Close()

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGeminiProvider_CallTools_Success(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "Checking weather"},
							{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Tokyo"}}},
						},
						"role": "model",
					},
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount": 20, "candidatesTokenCount": 10, "totalTokenCount": 30,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Weather in Tokyo?"}},
		Tools: []ToolDefinition{
			{Type: "function", Function: FunctionDefinition{
				Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Checking weather" {
		t.Errorf("expected 'Checking weather', got '%s'", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected 'get_weather', got '%s'", resp.ToolCalls[0].Name)
	}
}

func TestGeminiProvider_Embeddings_Success(t *testing.T) {
	server, provider := newGeminiTestServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":batchEmbedContents") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"embeddings": []map[string]any{
				{
					"values": []float32{0.1, 0.2, 0.3, 0.4},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	embeddings, err := provider.Embeddings(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embeddings) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(embeddings))
	}
	if len(embeddings[0]) != 4 {
		t.Errorf("expected 4 dimensions, got %d", len(embeddings[0]))
	}
}

func TestGeminiProvider_Info(t *testing.T) {
	provider, _ := NewGeminiProvider(Config{APIKey: "test-key", Model: "gemini-2.0-flash"})

	info := provider.Info()
	if info.Provider != "google" {
		t.Errorf("expected 'google', got '%s'", info.Provider)
	}
	if info.MaxContext != 1048576 {
		t.Errorf("expected 1048576, got %d", info.MaxContext)
	}
}

func TestGeminiProvider_Info_UnknownModel(t *testing.T) {
	provider, _ := NewGeminiProvider(Config{APIKey: "test-key", Model: "gemini-unknown"})

	info := provider.Info()
	if info.MaxContext != 1048576 {
		t.Errorf("expected default 1048576, got %d", info.MaxContext)
	}
}

func TestGeminiProvider_New_NoAPIKey(t *testing.T) {
	_, err := NewGeminiProvider(Config{Model: "gemini-2.0-flash"})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestGeminiProvider_New_Defaults(t *testing.T) {
	provider, err := NewGeminiProvider(Config{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.config.BaseURL != geminiDefaultBaseURL {
		t.Errorf("expected default baseURL, got '%s'", provider.config.BaseURL)
	}
	if provider.config.Model != "gemini-2.0-flash" {
		t.Errorf("expected default model, got '%s'", provider.config.Model)
	}
}

// ===== Ollama 测试 =====

func newOllamaTestServer(handler http.HandlerFunc) (*httptest.Server, *OllamaProvider) {
	server := httptest.NewServer(handler)
	provider, _ := NewOllamaProvider(Config{
		BaseURL: server.URL,
		Model:   "llama3",
	})
	return server, provider
}

func TestOllamaProvider_Complete_Success(t *testing.T) {
	server, provider := newOllamaTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		if stream, ok := reqBody["stream"].(bool); ok && stream {
			t.Error("expected stream=false for Complete")
		}

		resp := map[string]any{
			"model":     "llama3",
			"message":   map[string]any{"role": "assistant", "content": "Hello from Ollama!"},
			"done":      true,
			"prompt_eval_count": 10,
			"eval_count":        5,
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Ollama!" {
		t.Errorf("expected 'Hello from Ollama!', got '%s'", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestOllamaProvider_Complete_APIError(t *testing.T) {
	server, provider := newOllamaTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "model not found")
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOllamaProvider_Stream_Basic(t *testing.T) {
	server, provider := newOllamaTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		if stream, ok := reqBody["stream"].(bool); !ok || !stream {
			t.Error("expected stream=true for Stream")
		}

		fmt.Fprintf(w, `{"model":"llama3","message":{"role":"assistant","content":"Hello"},"done":false}` + "\n")
		fmt.Fprintf(w, `{"model":"llama3","message":{"role":"assistant","content":" Ollama"},"done":false}` + "\n")
		fmt.Fprintf(w, `{"model":"llama3","message":{"role":"assistant","content":"!"},"done":true}` + "\n")
	})
	defer server.Close()

	ch, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []Chunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}

	fullContent := ""
	for _, c := range chunks {
		fullContent += c.Content
	}
	if fullContent != "Hello Ollama!" {
		t.Errorf("expected 'Hello Ollama!', got '%s'", fullContent)
	}

	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should have Done=true")
	}
}

func TestOllamaProvider_Stream_Error(t *testing.T) {
	server, provider := newOllamaTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "model not found")
	})
	defer server.Close()

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOllamaProvider_CallTools_Success(t *testing.T) {
	server, provider := newOllamaTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"model":   "llama3",
			"message": map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []map[string]any{
					{
						"id": 0,
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": map[string]any{"city": "Berlin"},
						},
					},
				},
			},
			"done":             true,
			"prompt_eval_count": 20,
			"eval_count":        10,
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Weather?"}},
		Tools: []ToolDefinition{
			{Type: "function", Function: FunctionDefinition{
				Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected 'get_weather', got '%s'", resp.ToolCalls[0].Name)
	}
}

func TestOllamaProvider_Embeddings_Success(t *testing.T) {
	server, provider := newOllamaTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"embedding": []float32{0.1, 0.2, 0.3},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	embeddings, err := provider.Embeddings(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embeddings) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(embeddings))
	}
	if len(embeddings[0]) != 3 {
		t.Errorf("expected 3 dimensions, got %d", len(embeddings[0]))
	}
}

func TestOllamaProvider_Info(t *testing.T) {
	provider, _ := NewOllamaProvider(Config{Model: "llama3"})

	info := provider.Info()
	if info.Provider != "ollama" {
		t.Errorf("expected 'ollama', got '%s'", info.Provider)
	}
	if info.MaxContext != 8192 {
		t.Errorf("expected 8192, got %d", info.MaxContext)
	}
}

func TestOllamaProvider_Info_KnownModel(t *testing.T) {
	provider, _ := NewOllamaProvider(Config{Model: "llama3.1"})

	info := provider.Info()
	if info.MaxContext != 131072 {
		t.Errorf("expected 131072 for llama3.1, got %d", info.MaxContext)
	}
}

func TestOllamaProvider_New_Defaults(t *testing.T) {
	provider, err := NewOllamaProvider(Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.config.BaseURL != ollamaDefaultBaseURL {
		t.Errorf("expected default baseURL, got '%s'", provider.config.BaseURL)
	}
	if provider.config.Model != "llama3" {
		t.Errorf("expected default model 'llama3', got '%s'", provider.config.Model)
	}
}

func TestOllamaProvider_Complete_WithTemperature(t *testing.T) {
	var capturedBody map[string]any
	server, provider := newOllamaTestServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]any{
			"model": "llama3", "message": map[string]any{"role": "assistant", "content": "ok"},
			"done": true, "prompt_eval_count": 5, "eval_count": 3,
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages:    []ChatMessage{{Role: "user", Content: "Hi"}},
		Temperature: Float64Ptr(0.5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	opts, ok := capturedBody["options"].(map[string]any)
	if !ok {
		t.Fatal("expected options in request body")
	}
	if opts["temperature"] == nil {
		t.Error("expected temperature in options")
	}
}
