package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewOpenAIMultimodalProvider_Success(t *testing.T) {
	provider, err := NewOpenAIMultimodalProvider(Config{
		APIKey: "test-api-key",
		Model:  "gpt-4o",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewOpenAIMultimodalProvider_MissingAPIKey(t *testing.T) {
	_, err := NewOpenAIMultimodalProvider(Config{})
	if err != ErrAPIKeyRequired {
		t.Errorf("expected ErrAPIKeyRequired, got: %v", err)
	}
}

func TestOpenAIMultimodalProvider_Info(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{
		APIKey: "test-key",
		Model:  "gpt-4o",
	})

	info := provider.Info()
	infoExt := provider.InfoExt()

	if info.Name != "gpt-4o" {
		t.Errorf("Model name = %q, want gpt-4o", info.Name)
	}
	if info.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", info.Provider)
	}
	if !infoExt.SupportsVision {
		t.Error("gpt-4o should support vision")
	}
	if !infoExt.SupportsAudio {
		t.Error("gpt-4o should support audio")
	}
	if infoExt.SupportsVideo {
		t.Error("gpt-4o should not support video yet")
	}
	if infoExt.MaxImagesPerMsg != 10 {
		t.Errorf("MaxImagesPerMsg = %d, want 10", infoExt.MaxImagesPerMsg)
	}
}

func TestBuildMultimodalMessages_TextOnly(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewSystemMessage("You are helpful"),
		NewUserTextMessage("Hello"),
		NewAssistantMessage("Hi there!"),
	}

	result := provider.buildMultimodalMessages(msgs)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// 验证第一条消息
	if result[0]["role"] != "system" {
		t.Errorf("msg[0] role = %v, want system", result[0]["role"])
	}
	if result[0]["content"].(string) != "You are helpful" {
		t.Errorf("msg[0] content = %v, want 'You are helpful'", result[0]["content"])
	}
}

func TestBuildMultimodalMessages_WithImage(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewUserMultimodalMessage(
			NewTextContent("What's in this image?"),
			NewImageURLContent("https://example.com/cat.jpg"),
		),
	}

	result := provider.buildMultimodalMessages(msgs)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	contentParts, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatal("content should be an array of parts")
	}

	if len(contentParts) != 2 {
		t.Errorf("expected 2 content parts, got %d", len(contentParts))
	}

	// 验证文本部分
	if contentParts[0]["type"] != "text" {
		t.Errorf("part[0] type = %v, want text", contentParts[0]["type"])
	}

	// 验证图片部分
	if contentParts[1]["type"] != "image_url" {
		t.Errorf("part[1] type = %v, want image_url", contentParts[1]["type"])
	}

	imageURL := contentParts[1]["image_url"].(map[string]any)
	if imageURL["url"] != "https://example.com/cat.jpg" {
		t.Errorf("image URL = %v, want https://example.com/cat.jpg", imageURL["url"])
	}
}

func TestBuildMultimodalMessages_WithBase64Image(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	base64Data := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	msgs := []*ChatMessageExt{
		NewUserMultimodalMessage(
			NewTextContent("Analyze this:"),
			NewImageB64Content(base64Data, "image/png", "high"),
		),
	}

	result := provider.buildMultimodalMessages(msgs)

	contentParts := result[0]["content"].([]map[string]any)
	imagePart := contentParts[1]

	imageURL := imagePart["image_url"].(map[string]any)
	urlStr := imageURL["url"].(string)

	if urlStr[:10] != "data:image" {
		t.Errorf("base64 image URL should start with 'data:image', got: %.20s...", urlStr)
	}

	detail := imageURL["detail"].(string)
	if detail != "high" {
		t.Errorf("detail = %q, want high", detail)
	}
}

func TestConvertContentPart_Text(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertContentPart(NewTextContent("Hello"))
	if part == nil {
		t.Fatal("text part should not be nil")
	}
	if part["type"] != "text" {
		t.Errorf("type = %v, want text", part["type"])
	}
}

func TestConvertContentPart_EmptyText(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertContentPart(NewTextContent(""))
	if part != nil {
		t.Error("empty text should return nil")
	}
}

func TestConvertContentPart_ImageURL(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertContentPart(NewImageURLContent("https://img.png", "low"))

	imageURL := part["image_url"].(map[string]any)
	if imageURL["url"] != "https://img.png" {
		t.Error("URL mismatch")
	}
	if imageURL["detail"] != "low" {
		t.Errorf("detail = %v, want low", imageURL["detail"])
	}
}

func TestConvertContentPart_Audio(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertContentPart(NewAudioContent("base64audiodata", "audio/mp3"))

	if part["type"] != "input_audio" {
		t.Errorf("type = %v, want input_audio", part["type"])
	}

	inputAudio := part["input_audio"].(map[string]any)
	if inputAudio["format"] != "mp3" {
		t.Errorf("format = %v, want mp3", inputAudio["format"])
	}
}

func TestConvertContentPart_Video(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertContentPart(NewVideoContent("videodata", "video/mp4"))

	if part != nil {
		t.Error("video should return nil (not supported)")
	}
}

func TestCompleteMultimodal_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4o",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "I can see a cat sitting on a windowsill.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     50,
				"completion_tokens": 10,
				"total_tokens":      60,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIMultimodalProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4o",
	})
	provider.client = server.Client()

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserMultimodalMessage(
				NewTextContent("What's this?"),
				NewImageURLContent("https://example.com/image.png"),
			),
		},
	}

	resp, err := provider.CompleteMultimodal(context.Background(), req)
	if err != nil {
		t.Fatalf("CompleteMultimodal error: %v", err)
	}

	if resp.Content != "I can see a cat sitting on a windowsill." {
		t.Errorf("Content = %q, want expected response", resp.Content)
	}
	if resp.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", resp.Role)
	}
	if resp.Usage.TotalTokens != 60 {
		t.Errorf("TotalTokens = %d, want 60", resp.Usage.TotalTokens)
	}
}

func TestCompleteMultimodal_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := map[string]string{
			"error": "Invalid API key",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIMultimodalProvider(Config{
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

func TestComplete_BackwardCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		messages := body["messages"].([]any)
		firstMsg := messages[0].(map[string]any)

		content, isString := firstMsg["content"].(string)
		if !isString {
			t.Errorf("backward compatible mode should send string content, got: %T", firstMsg["content"])
		}

		resp := map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "Response to: " + content,
					},
				},
			},
			"usage": map[string]int{"total_tokens": 10},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIMultimodalProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Content != "Response to: Hello" {
		t.Errorf("Content = %q, want 'Response to: Hello'", resp.Content)
	}
}

func TestBuildToolCalls(t *testing.T) {
	provider, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test"})

	toolCalls := []FunctionCall{
		{ID: "call_1", Name: "search", Arguments: `{"query":"test"}`},
		{ID: "call_2", Name: "calculator", Arguments: `{"expr":"1+1"}`},
	}

	result := provider.buildToolCalls(toolCalls)

	if len(result) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(result))
	}

	if result[0]["id"] != "call_1" {
		t.Errorf("toolCall[0].id = %v, want call_1", result[0]["id"])
	}
	fn := result[0]["function"].(map[string]any)
	if fn["name"] != "search" {
		t.Errorf("function.name = %v, want search", fn["name"])
	}
}
