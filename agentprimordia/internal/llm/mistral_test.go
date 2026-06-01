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

func newMistralTestServer(handler http.HandlerFunc) (*httptest.Server, *MistralProvider) {
	server := httptest.NewServer(handler)
	provider, _ := NewMistralProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "mistral-large-latest",
	})
	return server, provider
}

func TestMistralProvider_Complete_Success(t *testing.T) {
	server, provider := newMistralTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		resp := map[string]any{
			"id": "mistral-123",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello from Mistral!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
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
	if resp.Content != "Hello from Mistral!" {
		t.Errorf("expected 'Hello from Mistral!', got '%s'", resp.Content)
	}
	if resp.ID != "mistral-123" {
		t.Errorf("expected ID 'mistral-123', got '%s'", resp.ID)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestMistralProvider_Complete_Error(t *testing.T) {
	server, provider := newMistralTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"error": map[string]any{
				"message": "Invalid request",
				"type":    "invalid_request_error",
				"code":    "invalid_request",
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
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Type != "invalid_request_error" {
		t.Errorf("expected type 'invalid_request_error', got '%s'", apiErr.Type)
	}
}

func TestMistralProvider_Stream_Basic(t *testing.T) {
	server, provider := newMistralTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		if !reqBody["stream"].(bool) {
			t.Error("expected stream=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"mistral-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"mistral-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" Mistral\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"mistral-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"!\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
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

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}

	fullContent := ""
	for _, c := range chunks {
		fullContent += c.Content
	}
	if fullContent != "Hello Mistral!" {
		t.Errorf("expected 'Hello Mistral!', got '%s'", fullContent)
	}

	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should have Done=true")
	}
}

func TestMistralProvider_CallTools_Success(t *testing.T) {
	server, provider := newMistralTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		tools, ok := reqBody["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Error("expected tools in request")
		}

		resp := map[string]any{
			"id": "mistral-tool-789",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_mistral_1",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"Berlin"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     30,
				"completion_tokens": 15,
				"total_tokens":      45,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Weather in Berlin?"}},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "Get weather for a city",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got '%s'", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].ID != "call_mistral_1" {
		t.Errorf("expected tool ID 'call_mistral_1', got '%s'", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Arguments != `{"city":"Berlin"}` {
		t.Errorf("expected arguments '{\"city\":\"Berlin\"}', got '%s'", resp.ToolCalls[0].Arguments)
	}
	if resp.Usage.TotalTokens != 45 {
		t.Errorf("expected 45 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestMistralProvider_Info(t *testing.T) {
	provider, _ := NewMistralProvider(Config{
		APIKey: "test-key",
		Model:  "mistral-large-latest",
	})

	info := provider.Info()
	if info.Name != "mistral-large-latest" {
		t.Errorf("expected 'mistral-large-latest', got '%s'", info.Name)
	}
	if info.Provider != "mistral" {
		t.Errorf("expected 'mistral', got '%s'", info.Provider)
	}
	if !info.SupportsTools {
		t.Error("SupportsTools should be true")
	}
	if !info.SupportsStreaming {
		t.Error("SupportsStreaming should be true")
	}
}

func TestMistralProvider_New_NoAPIKey(t *testing.T) {
	_, err := NewMistralProvider(Config{
		Model: "mistral-large-latest",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestMistralProvider_Complete_HTTPError(t *testing.T) {
	server, provider := newMistralTestServer(func(w http.ResponseWriter, r *http.Request) {
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

func TestMistralProvider_Complete_EmptyChoices(t *testing.T) {
	server, provider := newMistralTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "mistral-empty",
			"choices": []map[string]any{},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 0,
				"total_tokens":      5,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "empty choices") {
		t.Errorf("expected 'empty choices' error, got '%s'", err.Error())
	}
}
