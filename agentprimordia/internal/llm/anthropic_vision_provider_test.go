package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAnthropicVisionProvider_Success(t *testing.T) {
	provider, err := NewAnthropicVisionProvider(Config{
		APIKey: "test-ant-key",
		Model:  "claude-sonnet-4-20250514",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewAnthropicVisionProvider_MissingAPIKey(t *testing.T) {
	_, err := NewAnthropicVisionProvider(Config{})
	if err != ErrAPIKeyRequired {
		t.Errorf("expected ErrAPIKeyRequired, got: %v", err)
	}
}

func TestAnthropicVisionProvider_Info(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{
		APIKey: "test-key",
		Model:  "claude-sonnet-4-20250514",
	})

	info := provider.Info()
	infoExt := provider.InfoExt()

	if info.Name != "claude-sonnet-4-20250514" {
		t.Errorf("Model name = %q, want claude-sonnet-4-20250514", info.Name)
	}
	if info.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", info.Provider)
	}
	if !infoExt.SupportsVision {
		t.Error("Claude should support vision")
	}
	if infoExt.SupportsAudio {
		t.Error("Claude should not support audio yet")
	}
	if infoExt.MaxImagesPerMsg != 20 {
		t.Errorf("MaxImagesPerMsg = %d, want 20", infoExt.MaxImagesPerMsg)
	}
	if info.MaxContext != 200000 {
		t.Errorf("MaxContext = %d, want 200000", info.MaxContext)
	}
}

func TestBuildVisionMessages_TextOnly(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewSystemMessage("You are a vision assistant"),
		NewUserTextMessage("Hello Claude"),
		NewAssistantMessage("Hello! How can I help?"),
	}

	messages, systemMsg, err := provider.buildVisionMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if systemMsg != "You are a vision assistant" {
		t.Errorf("systemMsg = %q, want 'You are a vision assistant'", systemMsg)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (excluding system), got %d", len(messages))
	}

	firstMsg := messages[0]
	if firstMsg["role"] != "user" {
		t.Errorf("first msg role = %v, want user", firstMsg["role"])
	}

	contentParts := firstMsg["content"].([]map[string]any)
	if contentParts[0]["type"] != "text" {
		t.Errorf("content part type = %v, want text", contentParts[0]["type"])
	}
}

func TestBuildVisionMessages_WithImage(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	base64Data := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ"

	msgs := []*ChatMessageExt{
		NewUserMultimodalMessage(
			NewTextContent("Describe this image:"),
			NewImageB64Content(base64Data, "image/jpeg"),
		),
	}

	messages, _, err := provider.buildVisionMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	contentParts := messages[0]["content"].([]map[string]any)

	if len(contentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(contentParts))
	}

	textPart := contentParts[0]
	if textPart["type"] != "text" {
		t.Errorf("text part type = %v, want text", textPart["type"])
	}

	imagePart := contentParts[1]
	if imagePart["type"] != "image" {
		t.Errorf("image part type = %v, want image", imagePart["type"])
	}

	source := imagePart["source"].(map[string]any)
	if source["type"] != "base64" {
		t.Errorf("source type = %v, want base64", source["type"])
	}
	if source["media_type"] != "image/jpeg" {
		t.Errorf("media_type = %v, want image/jpeg", source["media_type"])
	}
	if source["data"] != base64Data {
		t.Error("base64 data mismatch")
	}
}

func TestConvertToAnthropicFormat_Text(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	part, err := provider.convertToAnthropicFormat(NewTextContent("Hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if part == nil {
		t.Fatal("text part should not be nil")
	}
	if part["type"] != "text" {
		t.Errorf("type = %v, want text", part["type"])
	}
}

func TestConvertToAnthropicFormat_EmptyText(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	part, err := provider.convertToAnthropicFormat(NewTextContent(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if part != nil {
		t.Error("empty text should return nil")
	}
}

func TestConvertToAnthropicFormat_Base64Image(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	part, err := provider.convertToAnthropicFormat(NewImageB64Content("data123", "image/png"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	source := part["source"].(map[string]any)
	if source["media_type"] != "image/png" {
		t.Errorf("media_type = %v, want image/png", source["media_type"])
	}
}

func TestConvertToAnthropicFormat_Audio(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	part, err := provider.convertToAnthropicFormat(NewAudioContent("audiodata", "audio/mp3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if part != nil {
		t.Error("audio should return nil (not supported by Anthropic)")
	}
}

func TestConvertToAnthropicFormat_Video(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	part, err := provider.convertToAnthropicFormat(NewVideoContent("videodata", "video/mp4"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if part != nil {
		t.Error("video should return nil (not supported)")
	}
}

func TestConvertToAnthropicFormat_ImageURL(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	_, err := provider.convertToAnthropicFormat(NewImageURLContent("https://example.com/image.jpg"))
	if err == nil {
		t.Error("image URL should return error (not supported by Anthropic)")
	}
}

func TestAnthropicCompleteMultimodal_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":   "msg_test123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]string{
				{"type": "text", "text": "I can see a beautiful sunset with orange and pink colors."},
			},
			"stop_reason": "end_turn",
			"usage": map[string]int{
				"input_tokens":  100,
				"output_tokens": 25,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewAnthropicVisionProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "claude-sonnet-4-20250514",
	})
	provider.client = server.Client()

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserMultimodalMessage(
				NewTextContent("What do you see?"),
				NewImageB64Content("base64imagedata...", "image/jpeg"),
			),
		},
	}

	resp, err := provider.CompleteMultimodal(context.Background(), req)
	if err != nil {
		t.Fatalf("CompleteMultimodal error: %v", err)
	}

	expectedContent := "I can see a beautiful sunset with orange and pink colors."
	if resp.Content != expectedContent {
		t.Errorf("Content = %q, want %q", resp.Content, expectedContent)
	}
	if resp.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", resp.Role)
	}
	if resp.Usage.TotalTokens != 125 {
		t.Errorf("TotalTokens = %d, want 125", resp.Usage.TotalTokens)
	}
}

func TestAnthropicCompleteMultimodal_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := map[string]any{
			"error": map[string]string{
				"type":    "authentication_error",
				"message": "Invalid API key",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewAnthropicVisionProvider(Config{
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

func TestResolveMaxTokens_Default(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 4096 {
		t.Errorf("default max_tokens = %d, want 4096", maxTokens)
	}
}

func TestResolveMaxTokens_FromRequest(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{MaxTokens: 1000}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 1000 {
		t.Errorf("max_tokens from request = %d, want 1000", maxTokens)
	}
}

func TestResolveMaxTokens_FromConfig(t *testing.T) {
	provider, _ := NewAnthropicVisionProvider(Config{
		APIKey:    "test",
		MaxTokens: 8192,
	})

	req := &CompletionRequestExt{}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 8192 {
		t.Errorf("max_tokens from config = %d, want 8192", maxTokens)
	}
}

func TestAnthropicComplete_BackwardCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		messages := body["messages"].([]any)
		firstMsg := messages[0].(map[string]any)
		contentArr := firstMsg["content"].([]any)
		textPart := contentArr[0].(map[string]any)
		content := textPart["text"].(string)

		resp := map[string]any{
			"id":      "msg_compat",
			"content": []map[string]string{{"type": "text", "text": "Response: " + content}},
			"usage":   map[string]int{"input_tokens": 10, "output_tokens": 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewAnthropicVisionProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello Claude"},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Content != "Response: Hello Claude" {
		t.Errorf("Content = %q, want 'Response: Hello Claude'", resp.Content)
	}
}
