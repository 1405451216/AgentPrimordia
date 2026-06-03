package memory

import (
	"context"
	"testing"
)

// ===== RAG 测试 =====

func TestRAGStore_Add_NoEmbedder(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory failed: %v", err)
	}
	defer store.Close()

	rag := NewRAGStore(store, nil)
	ep := MustEpisode("s1", "user", "Hello RAG")
	if err := rag.Add(context.Background(), ep); err != nil {
		t.Fatalf("RAGStore.Add() error = %v", err)
	}

	count, _ := store.Count(context.Background(), "s1")
	if count != 1 {
		t.Errorf("expected 1 episode, got %d", count)
	}
}

func TestRAGStore_Add_WithEmbedder(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory failed: %v", err)
	}
	defer store.Close()

	embedder := &mockEmbedder{dim: 8}
	rag := NewRAGStore(store, embedder)

	ep := MustEpisode("s1", "user", "Hello RAG with embeddings")
	if err := rag.Add(context.Background(), ep); err != nil {
		t.Fatalf("RAGStore.Add() error = %v", err)
	}

	// 等待异步 embedding 完成
	count, _ := store.Count(context.Background(), "s1")
	if count != 1 {
		t.Errorf("expected 1 episode, got %d", count)
	}
}

func TestRAGStore_Query_NeedsEmbedder(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	rag := NewRAGStore(store, nil)

	_, err := rag.Query(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error when no embedder configured")
	}
}

func TestRAGStore_Query_WithEmbedder(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	embedder := &mockEmbedder{dim: 8}
	rag := NewRAGStore(store, embedder)

	// 添加一些文档
	for i := 0; i < 5; i++ {
		ep := MustEpisode("s1", "user", "document content")
		_ = rag.Add(context.Background(), ep)
	}

	results, err := rag.Query(context.Background(), "document", 3)
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	// mock embedder 返回一致向量，应该有结果
	if len(results) == 0 {
		t.Error("expected at least 1 result")
	}
}

func TestRAGStore_HybridSearch(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	embedder := &mockEmbedder{dim: 8}
	rag := NewRAGStore(store, embedder)

	ep := MustEpisode("s1", "user", "golang programming language")
	_ = rag.Add(context.Background(), ep)

	results, err := rag.HybridSearch(context.Background(), "golang", 5)
	if err != nil {
		t.Fatalf("HybridSearch error = %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result from hybrid search")
	}
}

func TestFormatRAGContext(t *testing.T) {
	results := []*RAGResult{
		{Episode: &Episode{Role: "user", Content: "Hello"}, Score: 0.95, Sources: []string{"fts", "vector"}},
		{Episode: &Episode{Role: "assistant", Content: "Hi there"}, Score: 0.80, Sources: []string{"vector"}},
	}
	context := FormatRAGContext(results)
	if context == "" {
		t.Error("expected non-empty context")
	}
	if !contains(context, "相关记忆") {
		t.Error("expected '相关记忆' in context")
	}
}

// ===== 文档切分器测试 =====

func TestCharacterSplitter_ShortText(t *testing.T) {
	splitter := NewCharacterSplitter(1000, 200)
	chunks := splitter.Split(context.Background(), "Hello world")
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for short text, got %d", len(chunks))
	}
}

func TestCharacterSplitter_LongText(t *testing.T) {
	splitter := NewCharacterSplitter(50, 10)
	// 使用 \n\n 分隔符构造超长文本
	text := "This is a test sentence that is definitely longer than fifty characters.\n\nThis is another sentence that also exceeds the chunk size limit by a lot.\n\nAnd a third paragraph here with more content to ensure multiple chunks."
	chunks := splitter.Split(context.Background(), text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestRecursiveSplitter(t *testing.T) {
	splitter := NewRecursiveSplitter(50, 10)
	text := "First paragraph with some text.\n\nSecond paragraph with more content.\n\nThird paragraph here."
	chunks := splitter.Split(context.Background(), text)
	if len(chunks) == 0 {
		t.Error("expected at least 1 chunk")
	}
}

func TestLineSplitter(t *testing.T) {
	splitter := NewLineSplitter(3)
	text := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	chunks := splitter.Split(context.Background(), text)
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestTextFileLoader_UnsupportedType(t *testing.T) {
	loader := NewTextFileLoader()
	_, err := loader.Load(context.Background(), "test.bin")
	if err == nil {
		t.Error("expected error for unsupported file type")
	}
}

// ===== 辅助 =====

type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dim)
		for j := range vec {
			vec[j] = 0.1 // 固定向量
		}
		results[i] = vec
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int {
	return m.dim
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstring(s, sub)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
