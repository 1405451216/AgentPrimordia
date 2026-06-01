package llm

import (
	"context"
	"testing"
	"time"
)

func TestMockLLM_Complete(t *testing.T) {
	mock := NewMockLLM(t).WithResponse("Hello, world!")

	resp, err := mock.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got '%s'", resp.Content)
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.CallCount())
	}
}

func TestMockLLM_Stream(t *testing.T) {
	mock := NewMockLLM(t).WithResponse("Streamed content")

	ch, err := mock.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Stream"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	chunks := []Chunk{}
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0].Content != "Streamed content" {
		t.Errorf("unexpected content: %s", chunks[0].Content)
	}
}

func TestMockLLM_CallTools(t *testing.T) {
	mock := NewMockLLM(t).WithToolResponse([]FunctionCall{
		{ID: "call_1", Name: "get_weather", Arguments: `{"city": "Beijing"}`},
	})

	resp, err := mock.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Weather?"}},
		Tools:    []ToolDefinition{},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
}

func TestMockLLM_Error(t *testing.T) {
	mock := NewMockLLM(t).WithError(context.Canceled)

	_, err := mock.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestMockLLM_Delay(t *testing.T) {
	start := time.Now()
	mock := NewMockLLM(t).WithResponse("delayed").WithDelay(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := mock.Complete(ctx, &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 9*time.Millisecond {
		t.Errorf("expected ~10ms delay, got %v", elapsed)
	}
}

func TestMockLLM_Info(t *testing.T) {
	mock := NewMockLLM(t)
	info := mock.Info()

	if info.Name != "mock-model" {
		t.Errorf("expected 'mock-model', got '%s'", info.Name)
	}
	if !info.SupportsTools {
		t.Error("SupportsTools should be true")
	}
}

func TestMockLLM_Embeddings(t *testing.T) {
	mock := NewMockLLM(t)

	embeddings, err := mock.Embeddings(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embeddings) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(embeddings))
	}
	if len(embeddings[0]) != 16 {
		t.Errorf("expected 16-dim vector, got %d", len(embeddings[0]))
	}
}
