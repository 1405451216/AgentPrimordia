package agent

import (
	"agentprimordia/internal/llm"
	"context"
	"testing"
	"time"
)

func TestMultimodalMessage_HasNonTextContent(t *testing.T) {
	msg := &MultimodalMessage{
		Role: RoleUser,
		Contents: []*llm.MultimodalContent{
			{Type: llm.ContentTypeText, Text: "Hello"},
		},
	}
	if msg.HasNonTextContent() {
		t.Error("text-only message should return false")
	}

	msgWithImage := &MultimodalMessage{
		Role: RoleUser,
		Contents: []*llm.MultimodalContent{
			{Type: llm.ContentTypeText, Text: "Describe this"},
			{Type: llm.ContentTypeImageB64, Data: "base64data", MIME: "image/png"},
		},
	}
	if !msgWithImage.HasNonTextContent() {
		t.Error("message with image should return true")
	}
}

func TestMultimodalMessage_ExtractText(t *testing.T) {
	msg1 := &MultimodalMessage{
		Role:    RoleUser,
		Content: "Simple text",
	}
	if msg1.ExtractText() != "Simple text" {
		t.Errorf("ExtractText = %q, want 'Simple text'", msg1.ExtractText())
	}

	msg2 := &MultimodalMessage{
		Role: RoleUser,
		Contents: []*llm.MultimodalContent{
			{Type: llm.ContentTypeText, Text: "Part 1"},
			{Type: llm.ContentTypeImageB64, Data: "image", MIME: "image/png"},
			{Type: llm.ContentTypeText, Text: " Part 2"},
		},
	}
	extracted := msg2.ExtractText()
	if extracted != "Part 1 Part 2" {
		t.Errorf("ExtractText = %q, want 'Part 1 Part 2'", extracted)
	}
}

func TestMultimodalMessage_ToChatMessageExt(t *testing.T) {
	msg := &MultimodalMessage{
		Role: RoleUser,
		Contents: []*llm.MultimodalContent{
			{Type: llm.ContentTypeText, Text: "Hello"},
			{Type: llm.ContentTypeImageB64, Data: "imgdata", MIME: "image/jpeg"},
		},
		ToolCalls: []ToolCall{
			{ID: "call_123", Name: "search", Args: "{}"},
		},
	}

	ext := msg.ToChatMessageExt()

	if ext.Role != "user" {
		t.Errorf("Role = %q, want user", ext.Role)
	}
	if len(ext.Contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(ext.Contents))
	}
	if ext.Contents[0].Text != "Hello" {
		t.Errorf("first content text = %q, want Hello", ext.Contents[0].Text)
	}
	if len(ext.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(ext.ToolCalls))
	}
	if ext.ToolCalls[0].Name != "search" {
		t.Errorf("tool call name = %q, want search", ext.ToolCalls[0].Name)
	}
}

func TestNewUserMultimodalMessage(t *testing.T) {
	msg := NewUserMultimodalMessage(
		&llm.MultimodalContent{Type: llm.ContentTypeText, Text: "Question"},
		&llm.MultimodalContent{Type: llm.ContentTypeImageB64, Data: "img", MIME: "image/png"},
	)

	if msg.Role != RoleUser {
		t.Errorf("Role = %q, want user", msg.Role)
	}
	if !msg.HasNonTextContent() {
		t.Error("should have non-text content")
	}
	if msg.Metadata.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestUserImageB64Message(t *testing.T) {
	msg := UserImageB64Message("What's this?", "base64image", "image/jpeg")

	if msg.Role != RoleUser {
		t.Error("should be user role")
	}
	if len(msg.Contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(msg.Contents))
	}
	if msg.Contents[0].Text != "What's this?" {
		t.Errorf("text = %q, want 'What's this?'", msg.Contents[0].Text)
	}
	if msg.Contents[1].Type != llm.ContentTypeImageB64 {
		t.Error("second content should be image")
	}
}

func TestUserImageURLMessage(t *testing.T) {
	msg := UserImageURLMessage("Analyze:", "https://example.com/image.jpg")

	if msg.Contents[1].URL != "https://example.com/image.jpg" {
		t.Errorf("URL = %q, want https://example.com/image.jpg", msg.Contents[1].URL)
	}
}

func TestToMessage_Degradation(t *testing.T) {
	multiMsg := &MultimodalMessage{
		Role:      RoleUser,
		Contents:  []*llm.MultimodalContent{{Type: llm.ContentTypeText, Text: "Multimodal"}},
		ToolCalls: []ToolCall{{ID: "tc1", Name: "tool1", Args: "{}"}},
		Metadata:  Metadata{SessionID: "sess123"},
	}

	stdMsg := multiMsg.ToMessage()

	if stdMsg.Content != "Multimodal" {
		t.Errorf("Content = %q, want Multimodal", stdMsg.Content)
	}
	if stdMsg.Role != RoleUser {
		t.Error("role should be preserved")
	}
	if len(stdMsg.ToolCalls) != 1 {
		t.Error("tool calls should be preserved")
	}
}

func TestConvertToLLMMessagesExt(t *testing.T) {
	history := []*MultimodalMessage{
		NewSystemMultimodalMessage(
			&llm.MultimodalContent{Type: llm.ContentTypeText, Text: "You are a vision assistant"},
		),
		NewUserMultimodalMessage(
			&llm.MultimodalContent{Type: llm.ContentTypeText, Text: "Hello"},
			&llm.MultimodalContent{Type: llm.ContentTypeImageB64, Data: "img", MIME: "image/png"},
		),
	}

	extMsgs := convertToLLMMessagesExt(history)

	if len(extMsgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(extMsgs))
	}

	if extMsgs[0].Role != "system" {
		t.Errorf("first msg role = %q, want system", extMsgs[0].Role)
	}

	userMsg := extMsgs[1]
	if userMsg.Role != "user" {
		t.Errorf("second msg role = %q, want user", userMsg.Role)
	}
	if !userMsg.HasNonTextContent() {
		t.Error("user message should have non-text content")
	}
}

func TestIsMultimodalProvider(t *testing.T) {
	textOnlyProvider := &mockTextOnlyProvider{}
	if IsMultimodalProvider(textOnlyProvider) {
		t.Error("text-only provider should not be multimodal")
	}
}

type mockTextOnlyProvider struct{}

func (p *mockTextOnlyProvider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return nil, nil
}
func (p *mockTextOnlyProvider) Stream(ctx context.Context, req *llm.CompletionRequest) (<-chan llm.Chunk, error) {
	return nil, nil
}
func (p *mockTextOnlyProvider) CallTools(ctx context.Context, req *llm.ToolCallRequest) (*llm.ToolCallResponse, error) {
	return nil, nil
}
func (p *mockTextOnlyProvider) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, nil
}
func (p *mockTextOnlyProvider) Info() llm.ModelInfo {
	return llm.ModelInfo{}
}

func Now() time.Time {
	return time.Now()
}
