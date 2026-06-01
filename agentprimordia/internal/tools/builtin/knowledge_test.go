package builtin

import (
	"context"
	"encoding/json"
	"testing"
)

// ===== Mock KnowledgeSearcher =====

type mockKSearcher struct {
	docs   []*KnowledgeDoc
	err    error
	called bool
}

func (m *mockKSearcher) SearchKnowledge(ctx context.Context, query string, topK int) ([]*KnowledgeDoc, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	if topK > len(m.docs) {
		topK = len(m.docs)
	}
	return m.docs[:topK], nil
}

// ===== KnowledgeSearch 工具测试 =====

func TestKnowledgeSearch_Name(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	if ks.Name() != "knowledge_search" {
		t.Errorf("expected name 'knowledge_search', got '%s'", ks.Name())
	}
}

func TestKnowledgeSearch_Description(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	if ks.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestKnowledgeSearch_Parameters(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	params := ks.Parameters()
	if params == nil {
		t.Error("expected non-nil parameters")
	}
	var parsed map[string]any
	if err := json.Unmarshal(params, &parsed); err != nil {
		t.Errorf("parameters should be valid JSON: %v", err)
	}
}

func TestKnowledgeSearch_Execute_Success(t *testing.T) {
	searcher := &mockKSearcher{
		docs: []*KnowledgeDoc{
			{ID: "1", Content: "Go is fast", Score: 0.95, Source: "vector"},
			{ID: "2", Content: "Go has goroutines", Score: 0.85, Source: "fts+vector"},
		},
	}

	ks := NewKnowledgeSearch(searcher)
	args := json.RawMessage(`{"query":"golang","top_k":2}`)

	result, err := ks.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}
	if !searcher.called {
		t.Error("expected searcher to be called")
	}
}

func TestKnowledgeSearch_Execute_DefaultTopK(t *testing.T) {
	searcher := &mockKSearcher{
		docs: []*KnowledgeDoc{
			{ID: "1", Content: "Result", Score: 0.9},
		},
	}

	ks := NewKnowledgeSearch(searcher)
	args := json.RawMessage(`{"query":"test"}`)

	result, err := ks.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}
}

func TestKnowledgeSearch_Execute_NoProvider(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	args := json.RawMessage(`{"query":"test"}`)

	result, err := ks.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error when no provider")
	}
}

func TestKnowledgeSearch_Execute_InvalidArgs(t *testing.T) {
	searcher := &mockKSearcher{}
	ks := NewKnowledgeSearch(searcher)
	args := json.RawMessage(`invalid json`)

	result, err := ks.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for invalid arguments")
	}
}

func TestKnowledgeSearch_Execute_EmptyQuery(t *testing.T) {
	searcher := &mockKSearcher{}
	ks := NewKnowledgeSearch(searcher)
	args := json.RawMessage(`{"query":""}`)

	result, err := ks.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for empty query")
	}
}

func TestKnowledgeSearch_Execute_SearchError(t *testing.T) {
	searcher := &mockKSearcher{err: context.DeadlineExceeded}
	ks := NewKnowledgeSearch(searcher)
	args := json.RawMessage(`{"query":"test"}`)

	result, err := ks.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when search fails")
	}
}

func TestKnowledgeSearch_Execute_NoResults(t *testing.T) {
	searcher := &mockKSearcher{docs: []*KnowledgeDoc{}}
	ks := NewKnowledgeSearch(searcher)
	args := json.RawMessage(`{"query":"nonexistent"}`)

	result, err := ks.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected non-error result for no matches")
	}
}

func TestKnowledgeSearch_WithTopK(t *testing.T) {
	ks := NewKnowledgeSearch(nil).WithTopK(10)
	if ks.topK != 10 {
		t.Errorf("expected topK 10, got %d", ks.topK)
	}
}
