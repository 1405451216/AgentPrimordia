//go:build integration
// +build integration

package llm

import (
	"context"
	"os"
	"testing"
	"time"
)

func getAPIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}
	return key
}

func getBaseURL() string {
	if url := os.Getenv("OPENAI_BASE_URL"); url != "" {
		return url
	}
	return ""
}

func TestIntegration_OpenAI_Complete(t *testing.T) {
	apiKey := getAPIKey(t)

	provider, err := NewOpenAIProvider(Config{
		APIKey:  apiKey,
		BaseURL: getBaseURL(),
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Say 'hello world' and nothing else."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   20,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if resp.Content == "" {
		t.Error("expected non-empty content")
	}
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.Usage.TotalTokens == 0 {
		t.Error("expected non-zero token usage")
	}

	t.Logf("Response: %s", resp.Content)
	t.Logf("Usage: prompt=%d completion=%d total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
}

func TestIntegration_OpenAI_CallTools(t *testing.T) {
	apiKey := getAPIKey(t)

	provider, err := NewOpenAIProvider(Config{
		APIKey:  apiKey,
		BaseURL: getBaseURL(),
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.CallTools(ctx, &ToolCallRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "What is the weather in Beijing?"},
		},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather for a city",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]any{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []string{"city"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTools() error = %v", err)
	}

	if len(resp.ToolCalls) == 0 {
		t.Fatal("expected at least 1 tool call")
	}

	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got '%s'", resp.ToolCalls[0].Name)
	}

	t.Logf("Tool call: name=%s args=%s", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
}

func TestIntegration_OpenAI_Stream(t *testing.T) {
	apiKey := getAPIKey(t)

	provider, err := NewOpenAIProvider(Config{
		APIKey:  apiKey,
		BaseURL: getBaseURL(),
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Count from 1 to 5."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   50,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var fullContent string
	chunkCount := 0
	for chunk := range ch {
		fullContent += chunk.Content
		chunkCount++
		if chunk.Done {
			break
		}
	}

	if fullContent == "" {
		t.Error("expected non-empty streamed content")
	}
	if chunkCount == 0 {
		t.Error("expected at least 1 chunk")
	}

	t.Logf("Streamed (%d chunks): %s", chunkCount, fullContent)
}

func TestIntegration_OpenAI_APIError(t *testing.T) {
	provider, err := NewOpenAIProvider(Config{
		APIKey: "sk-invalid-key-12345",
		Model:  "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = provider.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "test"},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}

	apiErr, ok := err.(*APIError)
	if ok {
		t.Logf("Got APIError: message=%s type=%s", apiErr.Message, apiErr.Type)
	} else {
		t.Logf("Got error: %v", err)
	}
}

func TestIntegration_OpenAI_DeepSeek(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set, skipping DeepSeek integration test")
	}

	provider, err := NewOpenAIProvider(Config{
		APIKey:  apiKey,
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Say 'hello from deepseek' and nothing else."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   20,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if resp.Content == "" {
		t.Error("expected non-empty content from DeepSeek")
	}

	t.Logf("DeepSeek Response: %s", resp.Content)
}

func TestIntegration_Gemini_Complete(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping Gemini integration test")
	}

	provider, err := NewGeminiProvider(Config{
		APIKey: apiKey,
		Model:  "gemini-2.0-flash",
	})
	if err != nil {
		t.Fatalf("NewGeminiProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Say 'hello from gemini' and nothing else."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   20,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if resp.Content == "" {
		t.Error("expected non-empty content from Gemini")
	}

	t.Logf("Gemini Response: %s", resp.Content)
	t.Logf("Usage: prompt=%d completion=%d total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
}

func TestIntegration_Gemini_CallTools(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping Gemini integration test")
	}

	provider, err := NewGeminiProvider(Config{
		APIKey: apiKey,
		Model:  "gemini-2.0-flash",
	})
	if err != nil {
		t.Fatalf("NewGeminiProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.CallTools(ctx, &ToolCallRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "What is the weather in Shanghai?"},
		},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather for a city",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]any{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []string{"city"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTools() error = %v", err)
	}

	t.Logf("Response: %s", resp.Content)
	if len(resp.ToolCalls) > 0 {
		t.Logf("Tool call: name=%s args=%s", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
	}
}

// ===== Phase 17-A: Anthropic 集成测试 =====

func TestIntegration_Anthropic_Complete(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Anthropic integration test")
	}

	provider, err := NewAnthropicProvider(Config{
		APIKey: apiKey,
		Model:  "claude-haiku-4-5-20251001",
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Say 'hello from anthropic' and nothing else."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   20,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if resp.Content == "" {
		t.Error("expected non-empty content from Anthropic")
	}

	t.Logf("Anthropic Response: %s", resp.Content)
	t.Logf("Usage: prompt=%d completion=%d total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
}

func TestIntegration_Anthropic_Stream(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Anthropic stream integration test")
	}

	provider, err := NewAnthropicProvider(Config{
		APIKey: apiKey,
		Model:  "claude-haiku-4-5-20251001",
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Count from 1 to 3."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   50,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var fullContent string
	chunkCount := 0
	for chunk := range ch {
		fullContent += chunk.Content
		chunkCount++
		if chunk.Done {
			break
		}
	}

	if fullContent == "" {
		t.Error("expected non-empty streamed content from Anthropic")
	}
	if chunkCount == 0 {
		t.Error("expected at least 1 chunk from Anthropic stream")
	}

	t.Logf("Anthropic Streamed (%d chunks): %s", chunkCount, fullContent)
}

func TestIntegration_Anthropic_CallTools(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Anthropic tool call integration test")
	}

	provider, err := NewAnthropicProvider(Config{
		APIKey: apiKey,
		Model:  "claude-haiku-4-5-20251001",
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.CallTools(ctx, &ToolCallRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "What is the weather in Tokyo?"},
		},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather for a city",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]any{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []string{"city"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTools() error = %v", err)
	}

	t.Logf("Response: %s", resp.Content)
	if len(resp.ToolCalls) > 0 {
		t.Logf("Tool call: name=%s args=%s", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
	}
}

// ===== Phase 17-B: GLM 集成测试 =====
// 注意：GLM CallTools 跳过，Phase 16-B 已用 mock 测试锁定 ErrNotSupported 行为。

func TestIntegration_GLM_Complete(t *testing.T) {
	apiKey := os.Getenv("GLM_API_KEY")
	if apiKey == "" {
		t.Skip("GLM_API_KEY not set, skipping GLM integration test")
	}

	provider, err := NewGLMProvider(Config{
		APIKey: apiKey,
		Model:  "glm-4-flash",
	})
	if err != nil {
		t.Fatalf("NewGLMProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Say 'hello from glm' and nothing else."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   20,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if resp.Content == "" {
		t.Error("expected non-empty content from GLM")
	}

	t.Logf("GLM Response: %s", resp.Content)
	t.Logf("Usage: prompt=%d completion=%d total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
}

func TestIntegration_GLM_Stream(t *testing.T) {
	apiKey := os.Getenv("GLM_API_KEY")
	if apiKey == "" {
		t.Skip("GLM_API_KEY not set, skipping GLM stream integration test")
	}

	provider, err := NewGLMProvider(Config{
		APIKey: apiKey,
		Model:  "glm-4-flash",
	})
	if err != nil {
		t.Fatalf("NewGLMProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Count from 1 to 3."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   50,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var fullContent string
	chunkCount := 0
	for chunk := range ch {
		fullContent += chunk.Content
		chunkCount++
		if chunk.Done {
			break
		}
	}

	if fullContent == "" {
		t.Error("expected non-empty streamed content from GLM")
	}
	if chunkCount == 0 {
		t.Error("expected at least 1 chunk from GLM stream")
	}

	t.Logf("GLM Streamed (%d chunks): %s", chunkCount, fullContent)
}

// ===== Phase 17-C: Qwen / DeepSeek Stream 集成测试 =====

func TestIntegration_Qwen_Stream(t *testing.T) {
	apiKey := os.Getenv("QWEN_API_KEY")
	if apiKey == "" {
		t.Skip("QWEN_API_KEY not set, skipping Qwen stream integration test")
	}

	provider, err := NewQwenProvider(Config{
		APIKey: apiKey,
		Model:  "qwen-turbo",
	})
	if err != nil {
		t.Fatalf("NewQwenProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Count from 1 to 3."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   50,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var fullContent string
	chunkCount := 0
	for chunk := range ch {
		fullContent += chunk.Content
		chunkCount++
		if chunk.Done {
			break
		}
	}

	if fullContent == "" {
		t.Error("expected non-empty streamed content from Qwen")
	}
	if chunkCount == 0 {
		t.Error("expected at least 1 chunk from Qwen stream")
	}

	t.Logf("Qwen Streamed (%d chunks): %s", chunkCount, fullContent)
}

func TestIntegration_DeepSeek_Stream(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set, skipping DeepSeek stream integration test")
	}

	provider, err := NewOpenAIProvider(Config{
		APIKey:  apiKey,
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Count from 1 to 3."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   50,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var fullContent string
	chunkCount := 0
	for chunk := range ch {
		fullContent += chunk.Content
		chunkCount++
		if chunk.Done {
			break
		}
	}

	if fullContent == "" {
		t.Error("expected non-empty streamed content from DeepSeek")
	}
	if chunkCount == 0 {
		t.Error("expected at least 1 chunk from DeepSeek stream")
	}

	t.Logf("DeepSeek Streamed (%d chunks): %s", chunkCount, fullContent)
}

func TestIntegration_Qwen_Complete(t *testing.T) {
	apiKey := os.Getenv("QWEN_API_KEY")
	if apiKey == "" {
		t.Skip("QWEN_API_KEY not set, skipping Qwen integration test")
	}

	provider, err := NewQwenProvider(Config{
		APIKey: apiKey,
		Model:  "qwen-plus",
	})
	if err != nil {
		t.Fatalf("NewQwenProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Say 'hello from qwen' and nothing else."},
		},
		Temperature: Float64Ptr(0),
		MaxTokens:   20,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if resp.Content == "" {
		t.Error("expected non-empty content from Qwen")
	}

	t.Logf("Qwen Response: %s", resp.Content)
	t.Logf("Usage: prompt=%d completion=%d total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
}

func TestIntegration_Qwen_CallTools(t *testing.T) {
	apiKey := os.Getenv("QWEN_API_KEY")
	if apiKey == "" {
		t.Skip("QWEN_API_KEY not set, skipping Qwen integration test")
	}

	provider, err := NewQwenProvider(Config{
		APIKey: apiKey,
		Model:  "qwen-plus",
	})
	if err != nil {
		t.Fatalf("NewQwenProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.CallTools(ctx, &ToolCallRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "What is the weather in Shenzhen?"},
		},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather for a city",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]any{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []string{"city"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTools() error = %v", err)
	}

	t.Logf("Response: %s", resp.Content)
	if len(resp.ToolCalls) > 0 {
		t.Logf("Tool call: name=%s args=%s", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
	}
}
