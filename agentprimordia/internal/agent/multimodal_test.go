package agent

import (
	"encoding/json"
	"testing"

	"agentprimordia/internal/llm"
)

func TestContentPart_Marshal(t *testing.T) {
	part := ContentPart{
		Type: "text",
		Text: "hello",
	}
	data, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ContentPart
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if parsed.Type != "text" {
		t.Errorf("Type = %q, want %q", parsed.Type, "text")
	}
	if parsed.Text != "hello" {
		t.Errorf("Text = %q, want %q", parsed.Text, "hello")
	}
}

func TestContentPart_ImageURL(t *testing.T) {
	part := ContentPart{
		Type:   "image_url",
		URL:    "https://example.com/img.png",
		Detail: "high",
	}
	data, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ContentPart
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if parsed.Type != "image_url" {
		t.Errorf("Type = %q, want %q", parsed.Type, "image_url")
	}
	if parsed.URL != "https://example.com/img.png" {
		t.Errorf("URL = %q, want %q", parsed.URL, "https://example.com/img.png")
	}
	if parsed.Detail != "high" {
		t.Errorf("Detail = %q, want %q", parsed.Detail, "high")
	}
}

func TestContentPart_ImageB64(t *testing.T) {
	part := ContentPart{
		Type: "image_b64",
		Data: "iVBORw0KGgo=",
		MIME: "image/png",
	}
	data, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ContentPart
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if parsed.Data != "iVBORw0KGgo=" {
		t.Errorf("Data mismatch")
	}
	if parsed.MIME != "image/png" {
		t.Errorf("MIME = %q, want %q", parsed.MIME, "image/png")
	}
}

func TestMessage_HasMultimodal(t *testing.T) {
	msg := Message{
		Role:    RoleUser,
		Content: "hello",
	}
	if msg.HasMultimodal() {
		t.Error("text-only message should not be multimodal")
	}

	msg.ContentParts = []ContentPart{
		{Type: "text", Text: "hello"},
		{Type: "image_url", URL: "https://example.com/img.png"},
	}
	if !msg.HasMultimodal() {
		t.Error("message with image_url should be multimodal")
	}
}

func TestMessage_HasMultimodal_TextOnlyParts(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		ContentParts: []ContentPart{
			{Type: "text", Text: "hello"},
			{Type: "text", Text: "world"},
		},
	}
	if msg.HasMultimodal() {
		t.Error("text-only ContentParts should not be multimodal")
	}
}

func TestMessage_TextContent(t *testing.T) {
	msg := Message{
		Role:    RoleUser,
		Content: "plain text",
	}
	if msg.TextContent() != "plain text" {
		t.Errorf("TextContent() = %q, want %q", msg.TextContent(), "plain text")
	}

	msg2 := Message{
		Role: RoleUser,
		ContentParts: []ContentPart{
			{Type: "text", Text: "part1"},
			{Type: "text", Text: "part2"},
		},
	}
	if msg2.TextContent() != "part1 part2" {
		t.Errorf("TextContent() = %q, want %q", msg2.TextContent(), "part1 part2")
	}
}

func TestMessage_TextContent_FallbackToContent(t *testing.T) {
	msg := Message{
		Role:    RoleUser,
		Content: "fallback",
	}
	if msg.TextContent() != "fallback" {
		t.Errorf("TextContent() should fall back to Content when no ContentParts")
	}
}

func TestMultimodalAdapter_ToLLMContents(t *testing.T) {
	adapter := &MultimodalAdapter{}

	parts := []ContentPart{
		{Type: "text", Text: "describe this"},
		{Type: "image_url", URL: "https://example.com/img.png", Detail: "auto"},
		{Type: "image_b64", Data: "base64data", MIME: "image/jpeg"},
	}

	contents := adapter.ToLLMContents(parts)
	if len(contents) != 3 {
		t.Fatalf("len = %d, want 3", len(contents))
	}

	if contents[0].Type != llm.ContentTypeText || contents[0].Text != "describe this" {
		t.Errorf("first content mismatch: %+v", contents[0])
	}
	if contents[1].Type != llm.ContentTypeImageURL || contents[1].URL != "https://example.com/img.png" {
		t.Errorf("second content mismatch: %+v", contents[1])
	}
	if contents[2].Type != llm.ContentTypeImageB64 || contents[2].Data != "base64data" {
		t.Errorf("third content mismatch: %+v", contents[2])
	}
}

func TestMultimodalAdapter_ToLLMContents_Empty(t *testing.T) {
	adapter := &MultimodalAdapter{}
	contents := adapter.ToLLMContents(nil)
	if len(contents) != 0 {
		t.Errorf("nil parts should return empty slice, got %d", len(contents))
	}
}

func TestMultimodalAdapter_ToLLMContents_Audio(t *testing.T) {
	adapter := &MultimodalAdapter{}

	parts := []ContentPart{
		{Type: "audio", Data: "audiodata", MIME: "audio/mp3"},
	}

	contents := adapter.ToLLMContents(parts)
	if len(contents) != 1 {
		t.Fatalf("len = %d, want 1", len(contents))
	}
	if contents[0].Type != llm.ContentTypeAudio {
		t.Errorf("Type = %q, want %q", contents[0].Type, llm.ContentTypeAudio)
	}
	if contents[0].MIME != "audio/mp3" {
		t.Errorf("MIME = %q, want %q", contents[0].MIME, "audio/mp3")
	}
}

func TestUserMultimodalMessage(t *testing.T) {
	msg := UserMultimodalMessage(
		ContentPart{Type: "text", Text: "what is this?"},
		ContentPart{Type: "image_url", URL: "https://example.com/img.png"},
	)

	if msg.Role != RoleUser {
		t.Errorf("Role = %q, want %q", msg.Role, RoleUser)
	}
	if !msg.HasMultimodal() {
		t.Error("should be multimodal")
	}
	if len(msg.ContentParts) != 2 {
		t.Errorf("len(ContentParts) = %d, want 2", len(msg.ContentParts))
	}
}

func TestUserImageMessage(t *testing.T) {
	msg := UserImageMessage("describe this image", "https://example.com/photo.jpg")

	if msg.Role != RoleUser {
		t.Errorf("Role = %q, want %q", msg.Role, RoleUser)
	}
	if !msg.HasMultimodal() {
		t.Error("should be multimodal")
	}
	if msg.TextContent() != "describe this image" {
		t.Errorf("TextContent() = %q, want %q", msg.TextContent(), "describe this image")
	}
}

func TestMessage_BackwardCompatibility(t *testing.T) {
	msg := Message{
		Role:    RoleUser,
		Content: "just text",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	if parsed["content"] != "just text" {
		t.Errorf("content field = %v, want 'just text'", parsed["content"])
	}

	if _, hasParts := parsed["content_parts"]; hasParts {
		t.Error("content_parts should not be present when empty (omitempty)")
	}
}

func TestMessage_WithContentPartsSerialization(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		ContentParts: []ContentPart{
			{Type: "text", Text: "hello"},
			{Type: "image_url", URL: "https://example.com/img.png"},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Message
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(parsed.ContentParts) != 2 {
		t.Errorf("len(ContentParts) = %d, want 2", len(parsed.ContentParts))
	}
	if parsed.ContentParts[0].Type != "text" {
		t.Errorf("first part type = %q, want 'text'", parsed.ContentParts[0].Type)
	}
	if parsed.ContentParts[1].URL != "https://example.com/img.png" {
		t.Errorf("second part URL = %q, want 'https://example.com/img.png'", parsed.ContentParts[1].URL)
	}
}

func TestMultimodalAdapter_ConvertHistory(t *testing.T) {
	adapter := &MultimodalAdapter{}

	history := []Message{
		{Role: RoleSystem, Content: "you are helpful"},
		{Role: RoleUser, Content: "hello"},
		{
			Role: RoleUser,
			ContentParts: []ContentPart{
				{Type: "text", Text: "what is this?"},
				{Type: "image_url", URL: "https://example.com/img.png"},
			},
		},
	}

	hasMulti := adapter.HistoryHasMultimodal(history)
	if !hasMulti {
		t.Error("history should contain multimodal content")
	}

	extMsgs := adapter.ConvertHistoryToExt(history)
	if len(extMsgs) != 3 {
		t.Fatalf("len = %d, want 3", len(extMsgs))
	}

	if extMsgs[0].Role != "system" {
		t.Errorf("first msg role = %q, want 'system'", extMsgs[0].Role)
	}

	if extMsgs[2].HasNonTextContent() != true {
		t.Error("third message should have non-text content")
	}
}

func TestMultimodalAdapter_ConvertHistory_TextOnly(t *testing.T) {
	adapter := &MultimodalAdapter{}

	history := []Message{
		{Role: RoleSystem, Content: "you are helpful"},
		{Role: RoleUser, Content: "hello"},
	}

	hasMulti := adapter.HistoryHasMultimodal(history)
	if hasMulti {
		t.Error("text-only history should not be multimodal")
	}
}
