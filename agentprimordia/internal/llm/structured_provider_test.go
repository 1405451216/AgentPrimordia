package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupTestServer 创建测试 HTTP 服务器，返回服务器和捕获的请求体
func setupTestServer(responseBody string) (*httptest.Server, *map[string]any) {
	captured := &map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseBody))
	}))
	return srv, captured
}

func TestOpenAIProvider_ResponseFormat(t *testing.T) {
	resp := `{"id":"test","object":"chat.completion","choices":[{"message":{"content":"{\"name\":\"test\"}","role":"assistant"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4"})

	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
		Strict: true,
	}

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, JSONSchema: schema},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	rf, ok := (*captured)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not found in request body")
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want json_schema", rf["type"])
	}
	js, ok := rf["json_schema"].(map[string]any)
	if !ok {
		t.Fatal("json_schema not found in response_format")
	}
	if js["name"] != "test" {
		t.Errorf("json_schema.name = %v, want test", js["name"])
	}
	if js["strict"] != true {
		t.Errorf("json_schema.strict = %v, want true", js["strict"])
	}
}

func TestGeminiProvider_ResponseFormat(t *testing.T) {
	resp := `{"candidates":[{"content":{"parts":[{"text":"{\"name\":\"test\"}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewGeminiProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gemini-pro"})

	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
	}

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, JSONSchema: schema},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	gc, ok := (*captured)["generationConfig"].(map[string]any)
	if !ok {
		t.Fatal("generationConfig not found in request body")
	}
	if gc["responseMimeType"] != "application/json" {
		t.Errorf("responseMimeType = %v, want application/json", gc["responseMimeType"])
	}
	if gc["responseSchema"] == nil {
		t.Error("responseSchema should not be nil")
	}
}

func TestGeminiProvider_ResponseFormat_JSONObject(t *testing.T) {
	resp := `{"candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewGeminiProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gemini-pro"})

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	gc, ok := (*captured)["generationConfig"].(map[string]any)
	if !ok {
		t.Fatal("generationConfig not found in request body")
	}
	if gc["responseMimeType"] != "application/json" {
		t.Errorf("responseMimeType = %v, want application/json", gc["responseMimeType"])
	}
	if _, hasSchema := gc["responseSchema"]; hasSchema {
		t.Error("responseSchema should not be set for json_object type")
	}
}

func TestOllamaProvider_ResponseFormat(t *testing.T) {
	resp := `{"model":"test","message":{"content":"{\"name\":\"test\"}","role":"assistant"},"done":true}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "llama3"})

	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
	}

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, JSONSchema: schema},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	format, ok := (*captured)["format"]
	if !ok {
		t.Fatal("format not found in request body")
	}
	formatMap, isMap := format.(map[string]any)
	if !isMap {
		t.Fatalf("format should be a map for json_schema, got %T", format)
	}
	if formatMap["type"] != "object" {
		t.Errorf("format.type = %v, want object", formatMap["type"])
	}
}

func TestOllamaProvider_ResponseFormat_JSONObject(t *testing.T) {
	resp := `{"model":"test","message":{"content":"{}","role":"assistant"},"done":true}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "llama3"})

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	format, ok := (*captured)["format"]
	if !ok {
		t.Fatal("format not found in request body")
	}
	if format != "json" {
		t.Errorf("format = %v, want json", format)
	}
}

func TestMistralProvider_ResponseFormat(t *testing.T) {
	resp := `{"id":"test","object":"chat.completion","choices":[{"message":{"content":"{\"name\":\"test\"}","role":"assistant"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewMistralProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "mistral-large"})

	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object"},
	}

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, JSONSchema: schema},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	rf, ok := (*captured)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not found in request body")
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want json_schema", rf["type"])
	}
}

func TestCohereProvider_ResponseFormat(t *testing.T) {
	resp := `{"id":"test","message":{"role":"assistant","content":[{"type":"text","text":"{\"name\":\"test\"}"}]},"usage":{"billed_units":{"input_tokens":1,"output_tokens":1}},"finish_reason":"COMPLETE"}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewCohereProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "command-r"})

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	rf, ok := (*captured)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not found in request body")
	}
	if rf["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want json_object", rf["type"])
	}
}

func TestBuildOpenAIResponseFormat(t *testing.T) {
	t.Run("json_object", func(t *testing.T) {
		result := buildOpenAIResponseFormat(&ResponseFormat{Type: ResponseFormatJSONObject})
		if result["type"] != "json_object" {
			t.Errorf("type = %v, want json_object", result["type"])
		}
		if _, has := result["json_schema"]; has {
			t.Error("json_schema should not be set for json_object type")
		}
	})

	t.Run("json_schema with all fields", func(t *testing.T) {
		schema := &SchemaDef{
			Name:        "test_schema",
			Description: "A test schema",
			Schema:      map[string]any{"type": "object"},
			Strict:      true,
		}
		result := buildOpenAIResponseFormat(&ResponseFormat{Type: ResponseFormatJSONSchema, JSONSchema: schema})

		if result["type"] != "json_schema" {
			t.Errorf("type = %v, want json_schema", result["type"])
		}

		js, ok := result["json_schema"].(map[string]any)
		if !ok {
			t.Fatal("json_schema should be a map")
		}
		if js["name"] != "test_schema" {
			t.Errorf("name = %v, want test_schema", js["name"])
		}
		if js["description"] != "A test schema" {
			t.Errorf("description = %v, want 'A test schema'", js["description"])
		}
		if js["strict"] != true {
			t.Errorf("strict = %v, want true", js["strict"])
		}
		if js["schema"] == nil {
			t.Error("schema should not be nil")
		}
	})

	t.Run("text type", func(t *testing.T) {
		result := buildOpenAIResponseFormat(&ResponseFormat{Type: ResponseFormatText})
		if result["type"] != "text" {
			t.Errorf("type = %v, want text", result["type"])
		}
	})
}

func TestOpenAIProvider_ResponseFormat_JSONObject(t *testing.T) {
	resp := `{"id":"test","object":"chat.completion","choices":[{"message":{"content":"{}","role":"assistant"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4"})

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	rf, ok := (*captured)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not found in request body")
	}
	if rf["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want json_object", rf["type"])
	}
	if _, has := rf["json_schema"]; has {
		t.Error("json_schema should not be set for json_object type")
	}
}

func TestOpenAIProvider_ResponseFormat_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"{}\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4"})

	ch, err := p.Stream(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	for chunk := range ch {
		if chunk.Done {
			break
		}
	}
}

func TestOllamaProvider_ResponseFormat_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		if req["stream"] != true {
			t.Errorf("stream should be true")
		}

		format, ok := req["format"]
		if !ok {
			t.Error("format not found in stream request")
		} else if format != "json" {
			t.Errorf("format = %v, want json", format)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"model\":\"test\",\"message\":{\"content\":\"{}\",\"role\":\"assistant\"},\"done\":true}\n"))
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "llama3"})

	ch, err := p.Stream(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	for chunk := range ch {
		if chunk.Done {
			break
		}
	}
}

func TestQwenProvider_ResponseFormat(t *testing.T) {
	resp := `{"id":"test","object":"chat.completion","choices":[{"message":{"content":"{\"name\":\"test\"}","role":"assistant"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewQwenProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "qwen-max"})

	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object"},
	}

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, JSONSchema: schema},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	rf, ok := (*captured)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not found in request body")
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want json_schema", rf["type"])
	}
}

func TestGLMProvider_ResponseFormat(t *testing.T) {
	resp := `{"id":"test","object":"chat.completion","choices":[{"message":{"content":"{\"name\":\"test\"}","role":"assistant"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewGLMProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "glm-4"})

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	rf, ok := (*captured)["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not found in request body")
	}
	if rf["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want json_object", rf["type"])
	}
}

func TestNoResponseFormat_Omitted(t *testing.T) {
	resp := `{"id":"test","object":"chat.completion","choices":[{"message":{"content":"hello","role":"assistant"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4"})

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if _, has := (*captured)["response_format"]; has {
		t.Error("response_format should be omitted when not set")
	}
}

func TestAnthropicProvider_ResponseFormat(t *testing.T) {
	resp := `{"id":"test","type":"message","role":"assistant","content":[{"type":"text","text":"{\"name\":\"test\"}"}],"model":"claude-3","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	srv, captured := setupTestServer(resp)
	defer srv.Close()

	p, _ := NewAnthropicProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "claude-3"})

	schema := &SchemaDef{
		Name:   "test",
		Schema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
	}

	_, err := p.Complete(t.Context(), &CompletionRequest{
		Messages:       []ChatMessage{{Role: "user", Content: "test"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, JSONSchema: schema},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	bodyStr, _ := json.Marshal(*captured)
	bodyStrLower := strings.ToLower(string(bodyStr))

	if !strings.Contains(bodyStrLower, "tool") {
		t.Error("Anthropic should use tool_choice approach for structured output")
	}
}
