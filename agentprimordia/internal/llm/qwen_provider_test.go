package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
		json.NewEncoder(w).Encode(resp)
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
		json.NewEncoder(w).Encode(resp)
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
		json.NewDecoder(r.Body).Decode(&body)

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
		json.NewEncoder(w).Encode(resp)
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
