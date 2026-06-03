package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGLMProvider_Success(t *testing.T) {
	provider, err := NewGLMProvider(Config{
		APIKey: "test-glm-key",
		Model:  "glm-4v-flash",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewGLMProvider_MissingAPIKey(t *testing.T) {
	_, err := NewGLMProvider(Config{})
	if err != ErrAPIKeyRequired {
		t.Errorf("expected ErrAPIKeyRequired, got: %v", err)
	}
}

func TestGLMProvider_Info(t *testing.T) {
	provider, _ := NewGLMProvider(Config{
		APIKey: "test-key",
		Model:  "glm-4-plus",
	})

	info := provider.Info()
	infoExt := provider.InfoExt()

	if info.Name != "glm-4-plus" {
		t.Errorf("Model name = %q, want glm-4-plus", info.Name)
	}
	if info.Provider != "zhipu" {
		t.Errorf("Provider = %q, want zhipu", info.Provider)
	}
	if !infoExt.SupportsVision {
		t.Error("GLM-4V should support vision")
	}
	if infoExt.SupportsAudio {
		t.Error("GLM-4V should not support audio yet")
	}
	if info.MaxContext != 128000 {
		t.Errorf("MaxContext = %d, want 128000", info.MaxContext)
	}
}

func TestBuildMultimodalMessages_GLM_TextOnly(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	msgs := []*ChatMessageExt{
		NewUserTextMessage("你好，智谱"),
		NewAssistantMessage("你好！我是智谱AI的助手。"),
	}

	messages := provider.buildMultimodalMessages(msgs)

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	firstMsg := messages[0]
	if firstMsg["role"] != "user" {
		t.Errorf("first msg role = %v, want user", firstMsg["role"])
	}
	if firstMsg["content"].(string) != "你好，智谱" {
		t.Errorf("content = %v, want '你好，智谱'", firstMsg["content"])
	}
}

func TestBuildMultimodalMessages_GLM_WithImage(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	base64Data := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ"

	msgs := []*ChatMessageExt{
		NewUserMultimodalMessage(
			NewTextContent("请识别这张图片中的文字："),
			NewImageB64Content(base64Data, "image/jpeg"),
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
	expectedPrefix := "data:image/jpeg;base64,"
	if !strings.HasPrefix(imageURL, expectedPrefix) {
		t.Errorf("image URL should start with data URL format")
	}
}

func TestConvertToGLMFormat_Text(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	part := provider.convertToGLMFormat(NewTextContent("Hello"))
	if part == nil {
		t.Fatal("text part should not be nil")
	}
	if part["type"] != "text" {
		t.Errorf("type = %v, want text", part["type"])
	}
}

func TestConvertToGLMFormat_EmptyText(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	part := provider.convertToGLMFormat(NewTextContent(""))
	if part != nil {
		t.Error("empty text should return nil")
	}
}

func TestConvertToGLMFormat_Base64Image(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	part := provider.convertToGLMFormat(NewImageB64Content("data123", "image/png"))

	imageURL := part["image_url"].(map[string]string)["url"]
	expectedURL := "data:image/png;base64,data123"
	if imageURL != expectedURL {
		t.Errorf("image URL = %v, want %v", imageURL, expectedURL)
	}
}

func TestConvertToGLMFormat_Audio(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	part := provider.convertToGLMFormat(NewAudioContent("audiodata", "audio/mp3"))
	if part != nil {
		t.Error("audio should return nil (not supported by GLM-4V)")
	}
}

func TestCompleteMultimodal_GLM_WithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "chatcmpl-glm123",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "glm-4v-flash",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "这张图片显示的是一段中文文字：'人工智能改变世界'。字体清晰，背景为白色。",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     80,
				"completion_tokens": 25,
				"total_tokens":      105,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewGLMProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "glm-4v-flash",
	})
	provider.client = server.Client()

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserMultimodalMessage(
				NewTextContent("请识别这张图片中的文字内容："),
				NewImageB64Content("base64ocr...", "image/png"),
			),
		},
	}

	resp, err := provider.CompleteMultimodal(context.Background(), req)
	if err != nil {
		t.Fatalf("CompleteMultimodal error: %v", err)
	}

	expectedContent := "这张图片显示的是一段中文文字：'人工智能改变世界'。字体清晰，背景为白色。"
	if resp.Content != expectedContent {
		t.Errorf("Content = %q, want %q", resp.Content, expectedContent)
	}
	if resp.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", resp.Role)
	}
	if resp.Usage.TotalTokens != 105 {
		t.Errorf("TotalTokens = %d, want 105", resp.Usage.TotalTokens)
	}
}

func TestCompleteMultimodal_GLM_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := map[string]any{
			"error": map[string]string{
				"message": "无效的 API Key",
				"type":    "invalid_request_error",
				"code":    "invalid_api_key",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewGLMProvider(Config{
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

func TestResolveMaxTokens_GLM_Default(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 4096 {
		t.Errorf("default max_tokens = %d, want 4096", maxTokens)
	}
}

func TestResolveMaxTokens_GLM_FromRequest(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{MaxTokens: 2048}
	maxTokens := provider.resolveMaxTokens(req)
	if maxTokens != 2048 {
		t.Errorf("max_tokens from request = %d, want 2048", maxTokens)
	}
}

func TestComplete_GLM_BackwardCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

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
			"usage": map[string]int{"total_tokens": 18},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewGLMProvider(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := &CompletionRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "你好，智谱AI"},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Content != "回复: 你好，智谱AI" {
		t.Errorf("Content = %q, want '回复: 你好，智谱AI'", resp.Content)
	}
}

func TestComplexScenario_GLM4V_MultiModal(t *testing.T) {
	provider, _ := NewGLMProvider(Config{APIKey: "test"})

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewSystemMessage("你是一个专业的图像分析助手"),
			NewUserMultimodalMessage(
				NewTextContent("请分析以下图片并回答问题：\n1. 这是什么场景？\n2. 图片中有哪些物体？\n\n图片如下："),
				NewImageB64Content("scenedata", "image/jpeg"),
			),
		},
	}

	messages := provider.buildMultimodalMessages(req.Messages)

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(messages))
	}

	systemMsg := messages[0]
	if systemMsg["role"] != "system" {
		t.Errorf("first msg should be system, got: %v", systemMsg["role"])
	}

	userMsg := messages[1]
	parts := userMsg["content"].([]map[string]any)

	hasText := false
	hasImage := false
	for _, part := range parts {
		switch part["type"] {
		case "text":
			hasText = true
		case "image_url":
			hasImage = true
		}
	}

	if !hasText {
		t.Error("user message should contain text parts")
	}
	if !hasImage {
		t.Error("user message should contain image parts")
	}
}
