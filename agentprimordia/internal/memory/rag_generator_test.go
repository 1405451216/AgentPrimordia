package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type mockLLMGenerator struct {
	response string
	err      error
}

func (m *mockLLMGenerator) Generate(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestNewRetrievalAugmentedGenerator_MissingStore(t *testing.T) {
	_, err := NewRetrievalAugmentedGenerator(RAGConfig{
		Generator: &mockLLMGenerator{response: "ok"},
	})
	if err == nil {
		t.Fatal("expected error for missing store")
	}
}

func TestNewRetrievalAugmentedGenerator_MissingGenerator(t *testing.T) {
	mem := NewInMemoryStore()
	store := NewRAGStore(mem, nil)

	_, err := NewRetrievalAugmentedGenerator(RAGConfig{
		Store: store,
	})
	if err == nil {
		t.Fatal("expected error for missing generator")
	}
}

func TestNewRetrievalAugmentedGenerator_Defaults(t *testing.T) {
	mem := NewInMemoryStore()
	store := NewRAGStore(mem, nil)
	gen := &mockLLMGenerator{response: "test answer"}

	rag, err := NewRetrievalAugmentedGenerator(RAGConfig{
		Store:     store,
		Generator: gen,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rag.topK != 5 {
		t.Errorf("topK = %d, want 5", rag.topK)
	}
	if rag.systemPrompt == "" {
		t.Error("systemPrompt should not be empty")
	}
	if !strings.Contains(rag.systemPrompt, "{context}") {
		t.Error("systemPrompt should contain {context} placeholder")
	}
}

func TestRetrievalAugmentedGenerator_Ask(t *testing.T) {
	mem := NewInMemoryStore()
	mockEmbedder := &mockEmbedder{dim: 16}
	store := NewRAGStore(mem, mockEmbedder)
	gen := &mockLLMGenerator{response: "based on doc, answer is 42"}

	ctx := context.Background()

	_ = store.Add(ctx, &Episode{
		ID:      "doc-1",
		Content: "the ultimate answer of universe is 42",
		Role:    "document",
	})

	rag, _ := NewRetrievalAugmentedGenerator(RAGConfig{
		Store:     store,
		Generator: gen,
		TopK:      3,
	})

	result, err := rag.Ask(ctx, "what is the ultimate answer?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if result.Answer != "based on doc, answer is 42" {
		t.Errorf("Answer = %q, want based on doc, answer is 42", result.Answer)
	}
	if result.Query != "what is the ultimate answer?" {
		t.Errorf("Query = %q, want original query", result.Query)
	}
}

func TestRetrievalAugmentedGenerator_Ask_NoResults(t *testing.T) {
	mem := NewInMemoryStore()
	mockEmbedder := &mockEmbedder{dim: 16}
	store := NewRAGStore(mem, mockEmbedder)
	gen := &mockLLMGenerator{response: "no relevant info found"}

	rag, _ := NewRetrievalAugmentedGenerator(RAGConfig{
		Store:     store,
		Generator: gen,
	})

	result, err := rag.Ask(context.Background(), "unknown question")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if result.Answer != "no relevant info found" {
		t.Errorf("Answer = %q", result.Answer)
	}
	if len(result.Sources) != 0 {
		t.Errorf("Sources = %d, want 0", len(result.Sources))
	}
}

func TestRetrievalAugmentedGenerator_RetrieveOnly(t *testing.T) {
	mem := NewInMemoryStore()
	mockEmbedder := &mockEmbedder{dim: 16}
	store := NewRAGStore(mem, mockEmbedder)
	gen := &mockLLMGenerator{response: "test"}

	ctx := context.Background()

	_ = store.Add(ctx, &Episode{
		ID:      "doc-1",
		Content: "test document content here",
		Role:    "document",
	})

	rag, _ := NewRetrievalAugmentedGenerator(RAGConfig{
		Store:     store,
		Generator: gen,
	})

	sources, err := rag.RetrieveOnly(ctx, "test query")
	if err != nil {
		t.Fatalf("RetrieveOnly: %v", err)
	}
	if sources == nil {
		t.Error("sources should not be nil")
	}
}

func TestRetrievalAugmentedGenerator_BuildContext(t *testing.T) {
	mem := NewInMemoryStore()
	store := NewRAGStore(mem, nil)
	gen := &mockLLMGenerator{response: "test"}

	rag, _ := NewRetrievalAugmentedGenerator(RAGConfig{
		Store:        store,
		Generator:    gen,
		SystemPrompt: "custom system prompt\n{context}\nanswer the question",
	})

	sources := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "source doc content", Role: "document"}, Score: 0.9},
	}

	prompt := rag.BuildContext("user question?", sources)
	if !strings.Contains(prompt, "custom system prompt") {
		t.Error("prompt should contain custom system prompt")
	}
	if !strings.Contains(prompt, "source doc content") {
		t.Error("prompt should contain source content")
	}
	if !strings.Contains(prompt, "user question?") {
		t.Error("prompt should contain user query")
	}
}

func TestRetrievalAugmentedGenerator_FilterByMinScore(t *testing.T) {
	mem := NewInMemoryStore()
	mockEmbedder := &mockEmbedder{dim: 16}
	store := NewRAGStore(mem, mockEmbedder)
	gen := &mockLLMGenerator{response: "test"}

	rag, _ := NewRetrievalAugmentedGenerator(RAGConfig{
		Store:     store,
		Generator: gen,
		TopK:      10,
		MinScore:  0.5,
	})

	results := []*RAGResult{
		{Score: 0.8},
		{Score: 0.3},
		{Score: 0.6},
		{Score: 0.1},
	}

	filtered := rag.filterByMinScore(results)
	if len(filtered) != 2 {
		t.Errorf("filtered count = %d, want 2", len(filtered))
	}
	for _, r := range filtered {
		if r.Score < 0.5 {
			t.Errorf("score %.2f below min threshold", r.Score)
		}
	}
}

func TestRetrievalAugmentedGenerator_LLMError(t *testing.T) {
	mem := NewInMemoryStore()
	mockEmbedder := &mockEmbedder{dim: 16}
	store := NewRAGStore(mem, mockEmbedder)
	gen := &mockLLMGenerator{err: fmt.Errorf("llm failed")}

	rag, _ := NewRetrievalAugmentedGenerator(RAGConfig{
		Store:     store,
		Generator: gen,
	})

	_, err := rag.Ask(context.Background(), "test")
	if err == nil {
		t.Fatal("expected LLM error to propagate")
	}
	if !strings.Contains(err.Error(), "LLM") {
		t.Errorf("error = %v, want LLM generation error", err)
	}
}
