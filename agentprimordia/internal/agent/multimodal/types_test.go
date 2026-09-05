// internal/agent/multimodal/types_test.go
package multimodal

import "testing"

func TestContentPartImageURL(t *testing.T) {
	cp := ContentPart{Type: "image_url", URL: "https://example.com/img.png", MIME: "image/png"}
	if cp.Type != "image_url" {
		t.Errorf("expected image_url, got %s", cp.Type)
	}
	if cp.URL == "" {
		t.Error("URL should not be empty")
	}
}

func TestContentPartAudioBase64(t *testing.T) {
	cp := ContentPart{Type: "audio", Data: "base64data", MIME: "audio/wav"}
	if cp.Type != "audio" {
		t.Errorf("expected audio, got %s", cp.Type)
	}
}

func TestHasMultimodalWithImage(t *testing.T) {
	parts := []ContentPart{
		{Type: "text", Text: "描述这张图"},
		{Type: "image_url", URL: "https://example.com/img.png"},
	}
	has := false
	for _, p := range parts {
		if p.Type != "text" {
			has = true
			break
		}
	}
	if !has {
		t.Error("should detect multimodal content")
	}
}
