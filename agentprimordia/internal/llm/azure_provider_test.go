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

// ===== 辅助函数 =====

func newAzureTestServer(handler http.HandlerFunc) (*httptest.Server, *AzureOpenAIProvider) {
	server := httptest.NewServer(handler)
	provider, _ := NewAzureOpenAIProvider(AzureConfig{
		APIKey:         "test-azure-key",
		BaseURL:        server.URL,
		DeploymentName: "gpt-4o-deployment",
		APIVersion:     "2024-02-15-preview",
	})
	return server, provider
}

// ===== 创建与验证测试 =====

func TestAzureOpenAIProvider_New_Success(t *testing.T) {
	provider, err := NewAzureOpenAIProvider(AzureConfig{
		ResourceName:   "my-resource",
		DeploymentName: "gpt-4o",
		APIKey:         "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.config.BaseURL != "https://my-resource.openai.azure.com" {
		t.Errorf("expected baseURL from resource name, got '%s'", provider.config.BaseURL)
	}
	if provider.config.DeploymentName != "gpt-4o" {
		t.Errorf("expected deployment 'gpt-4o', got '%s'", provider.config.DeploymentName)
	}
	if provider.config.APIVersion != azureDefaultAPIVersion {
		t.Errorf("expected default API version '%s', got '%s'", azureDefaultAPIVersion, provider.config.APIVersion)
	}
}

func TestAzureOpenAIProvider_New_WithCustomBaseURL(t *testing.T) {
	provider, err := NewAzureOpenAIProvider(AzureConfig{
		BaseURL:        "https://custom.azure.com",
		DeploymentName: "gpt-4o",
		APIKey:         "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.config.BaseURL != "https://custom.azure.com" {
		t.Errorf("expected custom baseURL, got '%s'", provider.config.BaseURL)
	}
}

func TestAzureOpenAIProvider_New_NoAPIKey(t *testing.T) {
	_, err := NewAzureOpenAIProvider(AzureConfig{
		DeploymentName: "gpt-4o",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if err != ErrAPIKeyRequired {
		t.Errorf("expected ErrAPIKeyRequired, got %v", err)
	}
}

func TestAzureOpenAIProvider_New_NoDeployment(t *testing.T) {
	_, err := NewAzureOpenAIProvider(AzureConfig{
		APIKey: "test-key",
	})
	if err == nil {
		t.Fatal("expected error for missing deployment name")
	}
	if err != ErrAzureDeploymentRequired {
		t.Errorf("expected ErrAzureDeploymentRequired, got %v", err)
	}
}

func TestAzureOpenAIProvider_New_NoResource(t *testing.T) {
	_, err := NewAzureOpenAIProvider(AzureConfig{
		APIKey:         "test-key",
		DeploymentName: "gpt-4o",
		// 没有 ResourceName 也没有 BaseURL
	})
	if err == nil {
		t.Fatal("expected error for missing resource name")
	}
	if err != ErrAzureResourceRequired {
		t.Errorf("expected ErrAzureResourceRequired, got %v", err)
	}
}

func TestAzureOpenAIProvider_New_DefaultAPIVersion(t *testing.T) {
	provider, err := NewAzureOpenAIProvider(AzureConfig{
		ResourceName:   "my-resource",
		DeploymentName: "gpt-4o",
		APIKey:         "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.config.APIVersion != azureDefaultAPIVersion {
		t.Errorf("expected default API version, got '%s'", provider.config.APIVersion)
	}
}

// ===== Complete 测试 =====

func TestAzureOpenAIProvider_Complete_Success(t *testing.T) {
	server, provider := newAzureTestServer(func(w http.ResponseWriter, r *http.Request) {
		// 验证 URL 格式
		if !strings.Contains(r.URL.Path, "/openai/deployments/gpt-4o-deployment/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// 验证 API 版本参数
		if r.URL.Query().Get("api-version") != "2024-02-15-preview" {
			t.Errorf("unexpected api-version: %s", r.URL.Query().Get("api-version"))
		}
		// 验证认证头（Azure 使用 api-key 而非 Bearer）
		if r.Header.Get("api-key") != "test-azure-key" {
			t.Errorf("unexpected api-key header: %s", r.Header.Get("api-key"))
		}
		// 验证不使用 Bearer
		if r.Header.Get("Authorization") != "" {
			t.Error("Azure should not use Authorization Bearer header")
		}

		resp := map[string]any{
			"id": "chatcmpl-azure-123",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello from Azure!",
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
	if resp.Content != "Hello from Azure!" {
		t.Errorf("expected 'Hello from Azure!', got '%s'", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestAzureOpenAIProvider_Complete_APIError(t *testing.T) {
	server, provider := newAzureTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"error": map[string]any{
				"message": "Deployment not found",
				"type":    "invalid_request_error",
				"code":    "deployment_not_found",
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
	if apiErr.Message != "Deployment not found" {
		t.Errorf("unexpected error message: %s", apiErr.Message)
	}
}

func TestAzureOpenAIProvider_Complete_HTTPError(t *testing.T) {
	server, provider := newAzureTestServer(func(w http.ResponseWriter, r *http.Request) {
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

// ===== CallTools 测试 =====

func TestAzureOpenAIProvider_CallTools_Success(t *testing.T) {
	server, provider := newAzureTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)

		tools, ok := reqBody["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Error("expected tools in request")
		}

		resp := map[string]any{
			"id": "chatcmpl-azure-456",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_azure_1",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"Beijing"}`,
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
		Messages: []ChatMessage{{Role: "user", Content: "Weather?"}},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "Get weather",
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
}

// ===== Stream 测试 =====

func TestAzureOpenAIProvider_Stream_Basic(t *testing.T) {
	server, provider := newAzureTestServer(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		if !reqBody["stream"].(bool) {
			t.Error("expected stream=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-azure-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-azure-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" from Azure\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-azure-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"!\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\n")
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

	fullContent := ""
	for _, c := range chunks {
		fullContent += c.Content
	}
	if fullContent != "Hello from Azure!" {
		t.Errorf("expected 'Hello from Azure!', got '%s'", fullContent)
	}

	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should have Done=true")
	}
}

// ===== Embeddings 测试 =====

func TestAzureOpenAIProvider_Embeddings_EmptyDeployment(t *testing.T) {
	server, provider := newAzureTestServer(func(w http.ResponseWriter, r *http.Request) {
		// 验证 Embedding 请求路径
		if !strings.Contains(r.URL.Path, "/openai/deployments/gpt-4o-deployment/embeddings") {
			t.Errorf("unexpected embedding path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"object": "list",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": []float32{0.1, 0.2, 0.3},
				},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]any{
				"prompt_tokens": 5,
				"total_tokens":  5,
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
	if len(embeddings[0]) != 3 {
		t.Errorf("expected 3 dimensions, got %d", len(embeddings[0]))
	}
}

func TestAzureOpenAIProvider_Embeddings_CustomDeployment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证使用自定义 embedding 部署名
		if !strings.Contains(r.URL.Path, "/openai/deployments/text-embedding-deployment/embeddings") {
			t.Errorf("expected custom embedding deployment path, got: %s", r.URL.Path)
		}

		resp := map[string]any{
			"object": "list",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": []float32{0.1, 0.2},
				},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]any{
				"prompt_tokens": 3,
				"total_tokens":  3,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewAzureOpenAIProvider(AzureConfig{
		APIKey:                  "test-key",
		BaseURL:                 server.URL,
		DeploymentName:          "gpt-4o-deployment",
		EmbeddingDeploymentName: "text-embedding-deployment",
	})

	_, err := provider.Embeddings(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ===== Info 测试 =====

func TestAzureOpenAIProvider_Info(t *testing.T) {
	tests := []struct {
		deployment  string
		maxContext  int
	}{
		{"gpt-4o", 128000},
		{"gpt-4-32k", 32768},
		{"gpt-4", 8192},
		{"gpt-35-turbo-16k", 16384},
		{"gpt-35-turbo", 4096},
		{"unknown-model", 128000},
	}

	for _, tt := range tests {
		t.Run(tt.deployment, func(t *testing.T) {
			provider, _ := NewAzureOpenAIProvider(AzureConfig{
				ResourceName:   "test",
				DeploymentName: tt.deployment,
				APIKey:         "test-key",
			})
			info := provider.Info()
			if info.Provider != "azure-openai" {
				t.Errorf("expected 'azure-openai', got '%s'", info.Provider)
			}
			if info.MaxContext != tt.maxContext {
				t.Errorf("expected max context %d for %s, got %d", tt.maxContext, tt.deployment, info.MaxContext)
			}
			if !info.SupportsTools {
				t.Error("SupportsTools should be true")
			}
			if !info.SupportsStreaming {
				t.Error("SupportsStreaming should be true")
			}
		})
	}
}

// ===== URL 构建测试 =====

func TestAzureOpenAIProvider_BuildURL(t *testing.T) {
	provider, _ := NewAzureOpenAIProvider(AzureConfig{
		ResourceName:   "my-resource",
		DeploymentName: "gpt-4o",
		APIKey:         "test-key",
		APIVersion:     "2024-06-01",
	})

	url := provider.buildURL("/openai/deployments/gpt-4o/chat/completions")
	expected := "https://my-resource.openai.azure.com/openai/deployments/gpt-4o/chat/completions?api-version=2024-06-01"
	if url != expected {
		t.Errorf("expected '%s', got '%s'", expected, url)
	}
}

// ===== 认证头测试 =====

func TestAzureOpenAIProvider_Headers(t *testing.T) {
	var capturedHeaders http.Header
	server, provider := newAzureTestServer(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()

		resp := map[string]any{
			"id": "test",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "ok",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	_, _ = provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})

	// Azure 使用 api-key header 而非 Authorization Bearer
	if capturedHeaders.Get("api-key") != "test-azure-key" {
		t.Errorf("expected api-key header, got '%s'", capturedHeaders.Get("api-key"))
	}
	if capturedHeaders.Get("Authorization") != "" {
		t.Error("Azure should not set Authorization header")
	}
	if capturedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", capturedHeaders.Get("Content-Type"))
	}
}
