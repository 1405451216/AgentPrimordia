package llm

import (
	"testing"
)

func TestNewTextContent(t *testing.T) {
	content := NewTextContent("Hello")
	if content.Type != ContentTypeText {
		t.Errorf("Type = %q, want %q", content.Type, ContentTypeText)
	}
	if content.Text != "Hello" {
		t.Errorf("Text = %q, want %q", content.Text, "Hello")
	}
}

func TestNewImageURLContent(t *testing.T) {
	content := NewImageURLContent("https://example.com/image.png")
	if content.Type != ContentTypeImageURL {
		t.Errorf("Type = %q, want %q", content.Type, ContentTypeImageURL)
	}
	if content.URL != "https://example.com/image.png" {
		t.Errorf("URL = %q, want %q", content.URL, "https://example.com/image.png")
	}
	if content.Detail != "auto" {
		t.Errorf("Detail = %q, want %q", content.Detail, "auto")
	}

	contentHigh := NewImageURLContent("https://example.com/image.png", "high")
	if contentHigh.Detail != "high" {
		t.Errorf("Detail = %q, want %q", contentHigh.Detail, "high")
	}
}

func TestNewImageB64Content(t *testing.T) {
	data := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	content := NewImageB64Content(data, "image/png")
	if content.Type != ContentTypeImageB64 {
		t.Errorf("Type = %q, want %q", content.Type, ContentTypeImageB64)
	}
	if content.Data != data {
		t.Error("Data mismatch")
	}
	if content.MIME != "image/png" {
		t.Errorf("MIME = %q, want %q", content.MIME, "image/png")
	}
	if content.Detail != "auto" {
		t.Errorf("Detail = %q, want auto", content.Detail)
	}
}

func TestNewAudioContent(t *testing.T) {
	content := NewAudioContent("base64data", "audio/mp3")
	if content.Type != ContentTypeAudio {
		t.Errorf("Type = %q, want %q", content.Type, ContentTypeAudio)
	}
	if content.MIME != "audio/mp3" {
		t.Errorf("MIME = %q, want %q", content.MIME, "audio/mp3")
	}
}

func TestNewVideoContent(t *testing.T) {
	content := NewVideoContent("base64data", "video/mp4")
	if content.Type != ContentTypeVideo {
		t.Errorf("Type = %q, want %q", content.Type, ContentTypeVideo)
	}
}

func TestChatMessageExt_ToChatMessage(t *testing.T) {
	msg := NewUserTextMessage("Hello World")
	chatMsg := msg.ToChatMessage()

	if chatMsg.Role != "user" {
		t.Errorf("Role = %q, want %q", chatMsg.Role, "user")
	}
	if chatMsg.Content != "Hello World" {
		t.Errorf("Content = %q, want %q", chatMsg.Content, "Hello World")
	}
}

func TestChatMessageExt_ExtractText(t *testing.T) {
	msg := &ChatMessageExt{
		Role: "user",
		Contents: []*MultimodalContent{
			NewTextContent("What's in this image?"),
			NewImageURLContent("https://example.com/img.png"),
			NewTextContent("Please describe it."),
		},
	}

	text := msg.ExtractText()
	expected := "What's in this image? Please describe it."
	if text != expected {
		t.Errorf("ExtractText() = %q, want %q", text, expected)
	}
}

func TestChatMessageExt_HasNonTextContent(t *testing.T) {
	textOnly := NewUserTextMessage("text only")
	if textOnly.HasNonTextContent() {
		t.Error("text-only message should not have non-text content")
	}

	multimodal := NewUserMultimodalMessage(
		NewTextContent("Look at this"),
		NewImageURLContent("https://example.com/img.png"),
	)
	if !multimodal.HasNonTextContent() {
		t.Error("multimodal message should have non-text content")
	}
}

func TestChatMessageExt_ImageCount(t *testing.T) {
	msg := NewUserMultimodalMessage(
		NewTextContent("Describe these"),
		NewImageURLContent("https://example.com/img1.png"),
		NewImageURLContent("https://example.com/img2.jpg"),
		NewAudioContent("audio_data", "audio/wav"),
	)

	count := msg.ImageCount()
	if count != 2 {
		t.Errorf("ImageCount() = %d, want 2", count)
	}
}

func TestNewUserMultimodalMessage(t *testing.T) {
	msg := NewUserMultimodalMessage(
		NewTextContent("What is this?"),
		NewImageURLContent("https://example.com/photo.jpg"),
	)

	if msg.Role != "user" {
		t.Errorf("Role = %q, want user", msg.Role)
	}
	if len(msg.Contents) != 2 {
		t.Errorf("len(Contents) = %d, want 2", len(msg.Contents))
	}
	if !msg.HasNonTextContent() {
		t.Error("should detect non-text content")
	}
}

func TestNewAssistantMessage(t *testing.T) {
	msg := NewAssistantMessage("I see a cat")

	if msg.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", msg.Role)
	}
	if msg.ExtractText() != "I see a cat" {
		t.Errorf("Text mismatch")
	}

	msgWithTools := NewAssistantMessage("Let me check", FunctionCall{
		ID:   "call_123",
		Name: "search",
	})
	if len(msgWithTools.ToolCalls) != 1 {
		t.Error("expected 1 tool call")
	}
}

func TestNewSystemMessage(t *testing.T) {
	msg := NewSystemMessage("You are a helpful assistant")
	if msg.Role != "system" {
		t.Errorf("Role = %q, want system", msg.Role)
	}
}

func TestNewToolMessage(t *testing.T) {
	msg := NewToolMessage("call_456", "result data", false)
	if msg.Role != "tool" {
		t.Errorf("Role = %q, want tool", msg.Role)
	}
	if msg.ToolCallID != "call_456" {
		t.Errorf("ToolCallID = %q, want call_456", msg.ToolCallID)
	}
	if msg.IsToolError {
		t.Error("IsToolError should be false")
	}

	errMsg := NewToolMessage("call_789", "error occurred", true)
	if !errMsg.IsToolError {
		t.Error("IsToolError should be true for error message")
	}
}

func TestCompletionRequestExt_ToCompletionRequest(t *testing.T) {
	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewSystemMessage("Be helpful"),
			NewUserTextMessage("Hello"),
			NewAssistantMessage("Hi there!"),
		},
		Model:       "gpt-4o",
		Temperature: Float64Ptr(0.7),
		MaxTokens:   1000,
	}

	standardReq := req.ToCompletionRequest()

	if len(standardReq.Messages) != 3 {
		t.Errorf("len(Messages) = %d, want 3", len(standardReq.Messages))
	}
	if standardReq.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", standardReq.Model)
	}
	if *standardReq.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", *standardReq.Temperature)
	}
	if standardReq.MaxTokens != 1000 {
		t.Errorf("MaxTokens = %d, want 1000", standardReq.MaxTokens)
	}
}

func TestCompletionRequestExt_HasMultimodalContent(t *testing.T) {
	textOnlyReq := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserTextMessage("Just text"),
		},
	}
	if textOnlyReq.HasMultimodalContent() {
		t.Error("text-only request should not have multimodal content")
	}

	multimodalReq := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewUserMultimodalMessage(
				NewTextContent("Look"),
				NewImageURLContent("https://img.png"),
			),
		},
	}
	if !multimodalReq.HasMultimodalContent() {
		t.Error("request with image should have multimodal content")
	}
}

func TestModelInfoExt_Fields(t *testing.T) {
	info := ModelInfoExt{
		ModelInfo: ModelInfo{
			Name:              "gpt-4o",
			Provider:          "openai",
			MaxContext:        128000,
			SupportsTools:     true,
			SupportsStreaming: true,
		},
		SupportsVision:    true,
		SupportsAudio:     false,
		SupportsVideo:     false,
		MaxImageSize:      20,
		MaxImagesPerMsg:   10,
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/gif", "image/webp"},
	}

	if !info.SupportsVision {
		t.Error("gpt-4o should support vision")
	}
	if info.MaxImagesPerMsg != 10 {
		t.Errorf("MaxImagesPerMsg = %d, want 10", info.MaxImagesPerMsg)
	}
	if len(info.AcceptedMIMETypes) != 4 {
		t.Errorf("len(AcceptedMIMETypes) = %d, want 4", len(info.AcceptedMIMETypes))
	}
}

func TestContentType_Constants(t *testing.T) {
	tests := []struct {
		ct       ContentType
		expected string
	}{
		{ContentTypeText, "text"},
		{ContentTypeImageURL, "image_url"},
		{ContentTypeImageB64, "image_b64"},
		{ContentTypeAudio, "audio"},
		{ContentTypeVideo, "video"},
	}

	for _, tt := range tests {
		if string(tt.ct) != tt.expected {
			t.Errorf("ContentType %s = %q, want %q", tt.ct, string(tt.ct), tt.expected)
		}
	}
}

func TestComplexMultimodalScenario(t *testing.T) {
	// 模拟一个复杂的多模态对话场景
	conversation := &CompletionRequestExt{
		Messages: []*ChatMessageExt{
			NewSystemMessage("你是一个视觉助手，擅长分析图片和视频"),
			NewUserMultimodalMessage(
				NewTextContent("请分析这张图片：\n"),
				NewImageURLContent("https://example.com/photo.jpg", "high"),
				NewTextContent("\n并描述其中的主要对象"),
			),
			NewAssistantMessage("这张图片显示了一只猫坐在窗台上..."),
			NewUserMultimodalMessage(
				NewTextContent("这是另一张对比图："),
				NewImageB64Content("base64encodeddata...", "image/png", "low"),
			),
		},
		Model: "gpt-4o",
	}

	if !conversation.HasMultimodalContent() {
		t.Error("conversation should contain multimodal content")
	}

	userMsgs := 0
	imageCount := 0
	for _, msg := range conversation.Messages {
		if msg.Role == "user" {
			userMsgs++
			imageCount += msg.ImageCount()
		}
	}

	if userMsgs != 2 {
		t.Errorf("expected 2 user messages, got %d", userMsgs)
	}
	if imageCount != 2 {
		t.Errorf("expected 2 images total, got %d", imageCount)
	}
}
