package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGeminiMultimodalProvider_Success(t *testing.T) {
	provider, err := NewGeminiMultimodalProvider(Config{
		APIKey: "test-gemini-key",
		Model:  "gemini-2.0-flash",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewGeminiMultimodalProvider_MissingAPIKey(t *testing.T) {
	_, err := NewGeminiMultimodalProvider(Config{})
	if err != ErrAPIKeyRequired {
		t.Errorf("expected ErrAPIKeyRequired, got: %v", err)
	}
}

func TestGeminiMultimodalProvider_Info(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{
		APIKey: "test-key",
		Model:  "gemini-2.0-flash",
	})

	info := provider.Info()
	infoExt := provider.InfoExt()

	if info.Name != "gemini-2.0-flash" {
		t.Errorf("Model name = %q, want gemini-2.0-flash", info.Name)
	}
	if info.Provider != "google" {
		t.Errorf("Provider = %q, want google", info.Provider)
	}
	if !infoExt.SupportsVision {
		t.Error("Gemini should support vision")
	}
	if !infoExt.SupportsAudio {
		t.Error("Gemini should support audio")
	}
	if !infoExt.SupportsVideo {
		t.Error("Gemini should support video")
	}
	if info.MaxContext != 1000000 {
		t.Errorf("MaxContext = %d, want 1000000", info.MaxContext)
	}
}

func TestBuildMultimodalContents_TextOnly(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewSystemMessage("You are a helpful assistant"),
		NewUserTextMessage("Hello Gemini"),
		NewAssistantMessage("Hello! How can I help?"),
	}

	contents, systemParts := provider.buildMultimodalContents(msgs)

	if len(systemParts) != 1 || systemParts[0]["text"] != "You are a helpful assistant" {
		t.Error("system instruction not properly set")
	}

	if len(contents) != 2 {
		t.Fatalf("expected 2 messages (excluding system), got %d", len(contents))
	}

	firstMsg := contents[0]
	if firstMsg["role"] != "user" {
		t.Errorf("first msg role = %v, want user", firstMsg["role"])
	}

	parts := firstMsg["parts"].([]map[string]any)
	if parts[0]["text"] != "Hello Gemini" {
		t.Errorf("text = %v, want 'Hello Gemini'", parts[0]["text"])
	}
}

func TestBuildMultimodalContents_WithImage(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewUserMultimodalMessage(
			NewTextContent("What's in this image?"),
			NewImageB64Content("base64imagedata...", "image/png"),
		),
	}

	contents, _ := provider.buildMultimodalContents(msgs)

	parts := contents[0]["parts"].([]map[string]any)

	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}

	textPart := parts[0]
	if textPart["text"] != "What's in this image?" {
		t.Errorf("text part = %v", textPart["text"])
	}

	imagePart := parts[1]
	inlineData := imagePart["inlineData"].(map[string]any)
	if inlineData["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v, want image/png", inlineData["mimeType"])
	}
	if inlineData["data"] != "base64imagedata..." {
		t.Error("base64 data mismatch")
	}
}

func TestBuildMultimodalContents_AssistantRole(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewUserTextMessage("Question"),
		NewAssistantMessage("Answer"),
	}

	contents, _ := provider.buildMultimodalContents(msgs)

	if contents[1]["role"] != "model" {
		t.Errorf("assistant should map to 'model' in Gemini API, got: %v", contents[1]["role"])
	}
}

func TestConvertToGeminiFormat_Text(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertToGeminiFormat(NewTextContent("Hello"))
	if part == nil {
		t.Fatal("text part should not be nil")
	}
	if part["text"] != "Hello" {
		t.Errorf("text = %v, want Hello", part["text"])
	}
}

func TestConvertToGeminiFormat_EmptyText(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertToGeminiFormat(NewTextContent(""))
	if part != nil {
		t.Error("empty text should return nil")
	}
}

func TestConvertToGeminiFormat_Base64Image(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertToGeminiFormat(NewImageB64Content("data123", "image/jpeg"))

	inlineData := part["inlineData"].(map[string]any)
	if inlineData["mimeType"] != "image/jpeg" {
		t.Errorf("mimeType = %v, want image/jpeg", inlineData["mimeType"])
	}
	if inlineData["data"] != "data123" {
		t.Error("data mismatch")
	}
}

func TestConvertToGeminiFormat_Audio(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	part := provider.convertToGeminiFormat(NewAudioContent("audiodata", "audio/mp3"))

	inlineData := part["inlineData"].(map[string]any)
	if inlineData["mimeType"] != "audio/mp3" {
		t.Errorf("mimeType = %v, want audio/mp3", inlineData["mimeType"])
	}
}

func TestConvertToGeminiFormat_VideoWithURL(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	partURL := provider.convertToGeminiFormat(&MultimodalContent{
		Type: ContentTypeVideo,
		URL:  "https://example.com/video.mp4",
		MIME: "video/mp4",
	})

	if partURL == nil {
		t.Fatal("video with URL should not be nil")
	}

	fileData := partURL["fileData"].(map[string]any)
	if fileData["fileUri"] != "https://example.com/video.mp4" {
		t.Errorf("fileUri = %v, want https://example.com/video.mp4", fileData["fileUri"])
	}
}

func TestGeminiCompleteMultimodal_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]string{
							{"text": "I can see a beautiful mountain landscape with snow-capped peaks."},
						},
					},
				},
			},
			"usageMetadata": map[string]int{
				"promptTokenCount":     150,
				"candidatesTokenCount": 20,
				"totalTokenCount":      170,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewGeminiMultimodalProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gemini-2.0-flash",
	})
	provider.client = server.Client()

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserMultimodalMessage(
				NewTextContent("Describe this landscape:"),
				NewImageB64Content("base64landscape...", "image/jpeg"),
			),
		},
	}

	resp, err := provider.CompleteMultimodal(context.Background(), req)
	if err != nil {
		t.Fatalf("CompleteMultimodal error: %v", err)
	}

	expectedContent := "I can see a beautiful mountain landscape with snow-capped peaks."
	if resp.Content != expectedContent {
		t.Errorf("Content = %q, want %q", resp.Content, expectedContent)
	}
	if resp.Usage.TotalTokens != 170 {
		t.Errorf("TotalTokens = %d, want 170", resp.Usage.TotalTokens)
	}
}

func TestGeminiCompleteMultimodal_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"error": map[string]any{
				"code":    400,
				"message": "Invalid request",
				"status":  "INVALID_ARGUMENT",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewGeminiMultimodalProvider(Config{
		APIKey:  "invalid-key",
		BaseURL: server.URL,
	})

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{NewUserTextMessage("test")},
	}

	_, err := provider.CompleteMultimodal(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid request")
	}
}

func TestGeminiResolveMaxTokens_Default(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 8192 {
		t.Errorf("default max_tokens = %d, want 8192", maxTokens)
	}
}

func TestGeminiResolveMaxTokens_FromRequest(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{MaxTokens: 2048}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 2048 {
		t.Errorf("max_tokens from request = %d, want 2048", maxTokens)
	}
}

func TestGeminiComplete_BackwardCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		contents := body["contents"].([]any)
		firstContent := contents[0].(map[string]any)
		parts := firstContent["parts"].([]any)
		textPart := parts[0].(map[string]any)
		content := textPart["text"].(string)

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]string{{"text": "Response: " + content}},
					},
				},
			},
			"usageMetadata": map[string]int{"totalTokenCount": 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewGeminiMultimodalProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello Gemini"},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Content != "Response: Hello Gemini" {
		t.Errorf("Content = %q, want 'Response: Hello Gemini'", resp.Content)
	}
}

func TestComplexMultimodalScenario_Gemini(t *testing.T) {
	provider, _ := NewGeminiMultimodalProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewSystemMessage("你是一个多模态助手"),
			NewUserMultimodalMessage(
				NewTextContent("分析这张图片和这段音频：\n"),
				NewImageB64Content("imagedata", "image/png"),
				NewTextContent("\n以及这个音频："),
				NewAudioContent("audiodata", "audio/wav"),
			),
		},
	}

	contents, systemParts := provider.buildMultimodalContents(req.Messages)

	if len(systemParts) == 0 || systemParts[0]["text"] != "你是一个多模态助手" {
		t.Error("system instruction missing or incorrect")
	}

	userContent := contents[0]
	parts := userContent["parts"].([]map[string]any)

	imageCount := 0
	audioCount := 0
	textCount := 0
	for _, part := range parts {
		if _, ok := part["inlineData"]; ok {
			mimeType := part["inlineData"].(map[string]any)["mimeType"].(string)
			if strings.HasPrefix(mimeType, "image/") {
				imageCount++
			} else if strings.HasPrefix(mimeType, "audio/") {
				audioCount++
			}
		}
		if _, ok := part["text"]; ok {
			textCount++
		}
	}

	if imageCount != 1 {
		t.Errorf("expected 1 image, got %d", imageCount)
	}
	if audioCount != 1 {
		t.Errorf("expected 1 audio, got %d", audioCount)
	}
	if textCount != 2 {
		t.Errorf("expected 2 text parts, got %d", textCount)
	}
}
