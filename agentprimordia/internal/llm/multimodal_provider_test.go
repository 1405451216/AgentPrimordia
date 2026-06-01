package llm

import (
	"context"
	"testing"
)

func TestMultimodalCapability_HasCapability(t *testing.T) {
	caps := CapText | CapVision

	if !caps.HasCapability(CapText) {
		t.Error("should have text capability")
	}
	if !caps.HasCapability(CapVision) {
		t.Error("should have vision capability")
	}
	if caps.HasCapability(CapAudio) {
		t.Error("should not have audio capability")
	}
	if caps.HasCapability(CapVideo) {
		t.Error("should not have video capability")
	}
}

func TestNewMultimodalAdapter_OpenAI(t *testing.T) {
	p, err := NewOpenAIMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	adapter, err := NewMultimodalAdapter(p)
	if err != nil {
		t.Fatalf("NewMultimodalAdapter: %v", err)
	}

	if !adapter.SupportsVision() {
		t.Error("OpenAI adapter should support vision")
	}
	if !adapter.SupportsAudio() {
		t.Error("OpenAI adapter should support audio")
	}

	caps := adapter.Capabilities()
	if !caps.HasCapability(CapText) || !caps.HasCapability(CapVision) || !caps.HasCapability(CapAudio) {
		t.Errorf("caps = %b, want CapText|CapVision|CapAudio", caps)
	}
}

func TestNewMultimodalAdapter_Anthropic(t *testing.T) {
	p, err := NewAnthropicVisionProvider(Config{APIKey: "test", Model: "claude-sonnet-4-20250514"})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	adapter, err := NewMultimodalAdapter(p)
	if err != nil {
		t.Fatalf("NewMultimodalAdapter: %v", err)
	}

	if !adapter.SupportsVision() {
		t.Error("Anthropic adapter should support vision")
	}
	if adapter.SupportsAudio() {
		t.Error("Anthropic adapter should not support audio")
	}
}

func TestNewMultimodalAdapter_Gemini(t *testing.T) {
	p, err := NewGeminiMultimodalProvider(Config{APIKey: "test", Model: "gemini-2.0-flash"})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	adapter, err := NewMultimodalAdapter(p)
	if err != nil {
		t.Fatalf("NewMultimodalAdapter: %v", err)
	}

	if !adapter.SupportsVision() {
		t.Error("Gemini adapter should support vision")
	}
	if !adapter.SupportsAudio() {
		t.Error("Gemini adapter should support audio")
	}
}

func TestNewMultimodalAdapter_Unsupported(t *testing.T) {
	_, err := NewMultimodalAdapter("not a provider")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if err != ErrUnsupportedMultimodalProvider {
		t.Errorf("err = %v, want ErrUnsupportedMultimodalProvider", err)
	}
}

func TestMultimodalAdapter_Info(t *testing.T) {
	p, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	adapter, _ := NewMultimodalAdapter(p)

	info := adapter.Info()
	if info.Name == "" {
		t.Error("info.Name should not be empty")
	}
	if info.Provider != "openai" {
		t.Errorf("info.Provider = %q, want openai", info.Provider)
	}
}

func TestMultimodalAdapter_ModelInfoExt(t *testing.T) {
	p, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	adapter, _ := NewMultimodalAdapter(p)

	ext := adapter.ModelInfoExt()
	if !ext.SupportsVision {
		t.Error("ModelInfoExt.SupportsVision should be true")
	}
}

func TestMultimodalAdapter_As(t *testing.T) {
	p, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	adapter, _ := NewMultimodalAdapter(p)

	var openai *OpenAIMultimodalProvider
	if !adapter.As(&openai) {
		t.Fatal("As should succeed for OpenAIMultimodalProvider")
	}
	if openai == nil {
		t.Error("converted pointer should not be nil")
	}

	var gemini *GeminiMultimodalProvider
	if adapter.As(&gemini) {
		t.Error("As should fail for wrong type")
	}
}

func TestMultimodalAdapter_AutoFallback_PureText(t *testing.T) {
	p, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	adapter, _ := NewMultimodalAdapter(p)

	req := &CompletionRequestExt{
		Messages: []*ChatMessageExt{NewUserTextMessage("hello")},
	}

	resp, err := adapter.AutoFallback(context.Background(), req)
	if err != nil {
		t.Logf("AutoFallback: %v (network may be unavailable)", err)
		return
	}
	if resp == nil {
		t.Fatal("response should not be nil")
	}
}

func TestMultimodalAdapter_ImplementsInterface(t *testing.T) {
	p, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	adapter, _ := NewMultimodalAdapter(p)

	var _ Provider = adapter
	var _ MultimodalProvider = adapter
}

func TestMultimodalAdapter_CompleteDelegation(t *testing.T) {
	p, _ := NewOpenAIMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	adapter, _ := NewMultimodalAdapter(p)

	req := &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}
	resp, err := adapter.Complete(context.Background(), req)
	if err != nil {
		t.Logf("Complete delegation: %v (network may be unavailable)", err)
		return
	}
	if resp == nil {
		t.Fatal("response should not be nil")
	}
}

func TestMultimodalAdapter_EmbeddingsDelegation(t *testing.T) {
	p, _ := NewGeminiMultimodalProvider(Config{APIKey: "test", Model: "gemini-2.0-flash"})
	adapter, _ := NewMultimodalAdapter(p)

	vecs, err := adapter.Embeddings(context.Background(), []string{"hello"})
	if err != nil {
		t.Logf("Embeddings delegation: %v (may not be supported)", err)
		return
	}
	if len(vecs) != 1 {
		t.Fatalf("len(vecs) = %d, want 1", len(vecs))
	}
	if len(vecs[0]) == 0 {
		t.Error("embedding vector should not be empty")
	}
}

func TestNewMultimodalProvider_OpenAI(t *testing.T) {
	mp, err := NewMultimodalProvider(Config{APIKey: "test", Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("NewMultimodalProvider: %v", err)
	}
	if !mp.Capabilities().HasCapability(CapVision) {
		t.Error("should support vision")
	}
	if !mp.Capabilities().HasCapability(CapAudio) {
		t.Error("should support audio")
	}
}

func TestNewMultimodalProvider_Anthropic(t *testing.T) {
	mp, err := NewMultimodalProvider(Config{APIKey: "test", Model: "claude-sonnet-4-20250514"})
	if err != nil {
		t.Fatalf("NewMultimodalProvider: %v", err)
	}
	if !mp.Capabilities().HasCapability(CapVision) {
		t.Error("should support vision")
	}
}

func TestNewMultimodalProvider_Gemini(t *testing.T) {
	mp, err := NewMultimodalProvider(Config{APIKey: "test", Model: "gemini-2.0-flash"})
	if err != nil {
		t.Fatalf("NewMultimodalProvider: %v", err)
	}
	if !mp.Capabilities().HasCapability(CapVision) {
		t.Error("should support vision")
	}
	if !mp.Capabilities().HasCapability(CapAudio) {
		t.Error("should support audio")
	}
}

func TestNewMultimodalProvider_MissingAPIKey(t *testing.T) {
	_, err := NewMultimodalProvider(Config{Model: "gpt-4o"})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestDetectMultimodalProviderType(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected string
	}{
		{"openai model", Config{Model: "gpt-4o"}, "openai"},
		{"anthropic model", Config{Model: "claude-sonnet-4"}, "anthropic"},
		{"gemini model", Config{Model: "gemini-2.0-flash"}, "gemini"},
		{"openai baseurl", Config{BaseURL: "https://api.openai.com/v1", Model: "test"}, "openai"},
		{"anthropic baseurl", Config{BaseURL: "https://api.anthropic.com/v1", Model: "test"}, "anthropic"},
		{"gemini baseurl", Config{BaseURL: "https://generativelanguage.googleapis.com/v1beta", Model: "test"}, "gemini"},
		{"default fallback", Config{Model: "unknown-model"}, "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMultimodalProviderType(tt.cfg)
			if got != tt.expected {
				t.Errorf("detectMultimodalProviderType() = %q, want %q", got, tt.expected)
			}
		})
	}
}
