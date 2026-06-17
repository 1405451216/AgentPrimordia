package agent

import (
	"context"
	"testing"

	"agentprimordia/internal/llm"
	"agentprimordia/internal/tools"
)

// mockMemoryStore 简单的内存 MemoryStore 实现
type mockMemoryStore struct {
	episodes []*MemoryEpisode
}

func (m *mockMemoryStore) Add(_ context.Context, episode *MemoryEpisode) error {
	m.episodes = append(m.episodes, episode)
	return nil
}

func (m *mockMemoryStore) UpdateSummary(_ context.Context, _, _, _ string) error {
	return nil // stub: 测试不验证摘要存储
}

func TestReActAgent_RAG_AutoMode_WithMemory(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "Go supports concurrency", Score: 0.9, Role: "knowledge"},
		},
	}

	memStore := &mockMemoryStore{}

	mockLLM := llm.NewMockLLM(t).WithResponse("Go supports concurrency natively.")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-mem-agent",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithMemory(memStore).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeAuto,
		TopK:     5,
		MinScore: 0.3,
	})

	resp, err := agent.Run(context.Background(), UserMessage("Tell me about Go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Go supports concurrency natively." {
		t.Errorf("unexpected response: %s", resp.Content)
	}

	if len(memStore.episodes) == 0 {
		t.Error("expected memory episodes to be saved")
	}

	if len(ragProvider.queries) != 1 {
		t.Errorf("expected 1 RAG query, got %d", len(ragProvider.queries))
	}
}

func TestReActAgent_RAG_OnDemandMode_NoAutoQuery(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "On demand result", Score: 0.9},
		},
	}

	mockLLM := llm.NewMockLLM(t).WithResponse("No auto RAG here")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-ondemand",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeOnDemand,
	})

	resp, err := agent.Run(context.Background(), UserMessage("Test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ragProvider.queries) != 0 {
		t.Errorf("OnDemand mode should not auto-query, got %d queries", len(ragProvider.queries))
	}

	_ = resp
}

func TestReActAgent_RAG_NoMemoryStore_NoCrash(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "Some knowledge", Score: 0.8},
		},
	}

	mockLLM := llm.NewMockLLM(t).WithResponse("OK")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-no-mem",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeAuto,
	})

	resp, err := agent.Run(context.Background(), UserMessage("Test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("unexpected response: %s", resp.Content)
	}
}

func TestReActAgent_RAG_AllResultsBelowMinScore(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "Low relevance", Score: 0.1, Role: "knowledge"},
			{ID: "2", Content: "Also low", Score: 0.05, Role: "knowledge"},
		},
	}

	mockLLM := llm.NewMockLLM(t).WithResponse("No relevant context")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-low-score",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeAuto,
		MinScore: 0.5,
	})

	resp, err := agent.Run(context.Background(), UserMessage("Test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "No relevant context" {
		t.Errorf("unexpected response: %s", resp.Content)
	}
}

func TestReActAgent_RAG_ContextReplacement(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "First context", Score: 0.9, Role: "knowledge"},
		},
	}

	mockLLM := llm.NewMockLLM(t).
		WithToolResponse([]llm.FunctionCall{
			{ID: "call_1", Name: "echo_tool", Arguments: `{"msg":"test"}`},
		}).
		WithResponse("Final answer")

	registry := tools.NewRegistry()
	_ = registry.Register(&mockEchoTool{name: "echo_tool"})

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-replace",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(registry).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeAuto,
	})

	resp, err := agent.Run(context.Background(), UserMessage("Test RAG context replacement"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Final answer" {
		t.Errorf("unexpected response: %s", resp.Content)
	}
}

func TestReActAgent_RAG_WithInMemoryStore(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "Memory knowledge", Score: 0.85, Role: "knowledge"},
		},
	}

	memStore := &mockMemoryStore{}

	mockLLM := llm.NewMockLLM(t).WithResponse("Memory works with RAG")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-mem",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithMemory(memStore).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeAuto,
	})

	resp, err := agent.Run(context.Background(), UserMessage("Test Memory + RAG"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Memory works with RAG" {
		t.Errorf("unexpected response: %s", resp.Content)
	}

	if len(memStore.episodes) == 0 {
		t.Error("expected memory episodes to be saved")
	}
}

func TestReActAgent_RAG_DefaultTopK(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "Result 1", Score: 0.9},
			{ID: "2", Content: "Result 2", Score: 0.8},
			{ID: "3", Content: "Result 3", Score: 0.7},
			{ID: "4", Content: "Result 4", Score: 0.6},
			{ID: "5", Content: "Result 5", Score: 0.5},
			{ID: "6", Content: "Result 6", Score: 0.4},
		},
	}

	mockLLM := llm.NewMockLLM(t).WithResponse("Default TopK test")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-default-topk",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeAuto,
	})

	_, err := agent.Run(context.Background(), UserMessage("Test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ragProvider.queries) != 1 {
		t.Errorf("expected 1 RAG query, got %d", len(ragProvider.queries))
	}
}

func TestReActAgent_RAG_CustomTopK(t *testing.T) {
	ragProvider := &mockRAGProvider{
		docs: []*RAGDocument{
			{ID: "1", Content: "Result 1", Score: 0.9},
			{ID: "2", Content: "Result 2", Score: 0.8},
		},
	}

	mockLLM := llm.NewMockLLM(t).WithResponse("Custom TopK test")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-custom-topk",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithRAG(RAGConfig{
		Provider: ragProvider,
		Mode:     RAGModeAuto,
		TopK:     2,
	})

	_, err := agent.Run(context.Background(), UserMessage("Test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReActAgent_RAG_NilProvider_NoPanic(t *testing.T) {
	mockLLM := llm.NewMockLLM(t).WithResponse("No provider")

	agent := NewReActAgent(ReActConfig{
		Name:     "rag-nil-provider",
		Model:    mockLLM,
		MaxTurns: 10,
	}).AsCapability().WithToolkit(tools.NewRegistry()).WithRAG(RAGConfig{
		Provider: nil,
		Mode:     RAGModeAuto,
	})

	resp, err := agent.Run(context.Background(), UserMessage("Test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "No provider" {
		t.Errorf("unexpected response: %s", resp.Content)
	}
}
