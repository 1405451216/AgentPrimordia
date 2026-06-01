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

func newCohereTestServer(handler http.HandlerFunc) (*httptest.Server, *CohereProvider) {
	server := httptest.NewServer(handler)
	provider, _ := NewCohereProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "command-r-plus",
	})
	return server, provider
}

func TestCohereProvider_Complete_Success(t *testing.T) {
	server, provider := newCohereTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		resp := map[string]any{
			"id": "cohere-123",
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{
					{"type": "text", "text": "Hello from Cohere!"},
				},
			},
			"usage": map[string]any{
				"billed_units": map[string]any{
					"input_tokens":  10,
					"output_tokens": 5,
				},
				"tokens": map[string]any{
					"input_tokens":  15,
					"output_tokens": 8,
				},
			},
			"finish_reason": "COMPLETE",
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
	if resp.Content != "Hello from Cohere!" {
		t.Errorf("expected 'Hello from Cohere!', got '%s'", resp.Content)
	}
	if resp.ID != "cohere-123" {
		t.Errorf("expected ID 'cohere-123', got '%s'", resp.ID)
	}
	if resp.Usage.TotalTokens != 23 {
		t.Errorf("expected 23 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestCohereProvider_Complete_Error(t *testing.T) {
	server, provider := newCohereTestServer(func(w http.ResponseWriter, r *http.Request) {
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

func TestCohereProvider_Stream_Basic(t *testing.T) {
	server, provider := newCohereTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		if stream, ok := reqBody["stream"].(bool); !ok || !stream {
			t.Error("expected stream=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"content-delta\",\"delta\":{\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"Hello\"}]}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content-delta\",\"delta\":{\"message\":{\"content\":[{\"type\":\"text\",\"text\":\" Cohere\"}]}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content-delta\",\"delta\":{\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"!\"}]}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_end\",\"usage\":{\"billed_units\":{\"input_tokens\":10,\"output_tokens\":5},\"tokens\":{\"input_tokens\":15,\"output_tokens\":8}}}\n\n")
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
	if fullContent != "Hello Cohere!" {
		t.Errorf("expected 'Hello Cohere!', got '%s'", fullContent)
	}

	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should have Done=true")
	}
	if lastChunk.Usage == nil {
		t.Error("last chunk should have Usage")
	} else if lastChunk.Usage.TotalTokens != 23 {
		t.Errorf("expected 23 total tokens, got %d", lastChunk.Usage.TotalTokens)
	}
}

func TestCohereProvider_CallTools_Success(t *testing.T) {
	server, provider := newCohereTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		tools, ok := reqBody["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Error("expected tools in request")
		}

		resp := map[string]any{
			"id": "cohere-tool-456",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{},
				"tool_calls": []map[string]any{
					{
						"id":   "call_cohere_1",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"city":"Paris"}`,
						},
					},
				},
			},
			"usage": map[string]any{
				"billed_units": map[string]any{
					"input_tokens":  20,
					"output_tokens": 10,
				},
				"tokens": map[string]any{
					"input_tokens":  30,
					"output_tokens": 15,
				},
			},
			"finish_reason": "TOOL_CALL",
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Weather in Paris?"}},
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
	if resp.ToolCalls[0].ID != "call_cohere_1" {
		t.Errorf("expected tool ID 'call_cohere_1', got '%s'", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Arguments != `{"city":"Paris"}` {
		t.Errorf("expected arguments '{\"city\":\"Paris\"}', got '%s'", resp.ToolCalls[0].Arguments)
	}
	if resp.Usage.TotalTokens != 45 {
		t.Errorf("expected 45 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestCohereProvider_Info(t *testing.T) {
	provider, _ := NewCohereProvider(Config{
		APIKey: "test-key",
		Model:  "command-r-plus",
	})

	info := provider.Info()
	if info.Name != "command-r-plus" {
		t.Errorf("expected 'command-r-plus', got '%s'", info.Name)
	}
	if info.Provider != "cohere" {
		t.Errorf("expected 'cohere', got '%s'", info.Provider)
	}
	if !info.SupportsTools {
		t.Error("SupportsTools should be true")
	}
	if !info.SupportsStreaming {
		t.Error("SupportsStreaming should be true")
	}
}

func TestCohereProvider_New_NoAPIKey(t *testing.T) {
	_, err := NewCohereProvider(Config{
		Model: "command-r-plus",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestCohereProvider_Complete_HTTPError(t *testing.T) {
	server, provider := newCohereTestServer(func(w http.ResponseWriter, r *http.Request) {
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

func TestCohereProvider_Embeddings_NotSupported(t *testing.T) {
	provider, _ := NewCohereProvider(Config{
		APIKey: "test-key",
		Model:  "command-r-plus",
	})

	var p Provider = provider
	_, ok := p.(Embedder)
	if ok {
		t.Error("CohereProvider should not implement Embedder")
	}
}
