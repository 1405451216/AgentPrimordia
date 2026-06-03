package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextFileLoader_LoadFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(f, []byte("hello world"), 0644)

	loader := NewTextFileLoader()
	docs, err := loader.Load(context.Background(), f)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Content != "hello world" {
		t.Errorf("content = %q, want 'hello world'", docs[0].Content)
	}
	if docs[0].Metadata["ext"] != ".txt" {
		t.Errorf("ext = %q, want '.txt'", docs[0].Metadata["ext"])
	}
}

func TestTextFileLoader_LoadDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.md"), []byte("bbb"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "c.bin"), []byte("ccc"), 0644)

	loader := NewTextFileLoader()
	docs, err := loader.Load(context.Background(), dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 docs (skip .bin), got %d", len(docs))
	}
}

func TestTextFileLoader_UnsupportedExt(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.exe")
	_ = os.WriteFile(f, []byte("binary"), 0644)

	loader := NewTextFileLoader()
	_, err := loader.Load(context.Background(), f)
	if err == nil {
		t.Error("expected error for unsupported file type")
	}
}

func TestTextFileLoader_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.txt")
	_ = os.WriteFile(f, []byte(strings.Repeat("x", 200)), 0644)

	loader := &TextFileLoader{MaxFileSize: 100}
	_, err := loader.Load(context.Background(), f)
	if err == nil {
		t.Error("expected error for file too large")
	}
}

func TestTextFileLoader_NotFound(t *testing.T) {
	loader := NewTextFileLoader()
	_, err := loader.Load(context.Background(), "/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReaderLoader_LoadFromReader(t *testing.T) {
	loader := NewReaderLoader()
	doc, err := loader.LoadFromReader(context.Background(), strings.NewReader("reader content"), "test-source")
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	if doc.Content != "reader content" {
		t.Errorf("content = %q, want 'reader content'", doc.Content)
	}
	if doc.Source != "test-source" {
		t.Errorf("source = %q, want 'test-source'", doc.Source)
	}
}

func TestCharacterSplitter_MultipleChunks(t *testing.T) {
	splitter := NewCharacterSplitter(50, 10)
	text := strings.Repeat("hello world\n\n", 20)
	chunks := splitter.Split(context.Background(), text)
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestCharacterSplitter_DefaultValues(t *testing.T) {
	splitter := NewCharacterSplitter(0, -1)
	if splitter.ChunkSize != 1000 {
		t.Errorf("ChunkSize = %d, want 1000", splitter.ChunkSize)
	}
	if splitter.ChunkOverlap != 200 {
		t.Errorf("ChunkOverlap = %d, want 200", splitter.ChunkOverlap)
	}
}

func TestCharacterSplitter_OverlapLargerThanSize(t *testing.T) {
	splitter := NewCharacterSplitter(100, 200)
	if splitter.ChunkOverlap >= splitter.ChunkSize {
		t.Errorf("overlap should be < size: overlap=%d, size=%d", splitter.ChunkOverlap, splitter.ChunkSize)
	}
}

func TestRecursiveSplitter_MultipleChunks(t *testing.T) {
	splitter := NewRecursiveSplitter(50, 10)
	text := strings.Repeat("This is a sentence. ", 50)
	chunks := splitter.Split(context.Background(), text)
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestRecursiveSplitter_SmallInput(t *testing.T) {
	splitter := NewRecursiveSplitter(1000, 100)
	chunks := splitter.Split(context.Background(), "short")
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestRecursiveSplitter_ForceSplit(t *testing.T) {
	splitter := NewRecursiveSplitter(10, 2)
	text := strings.Repeat("abcdefghij", 20)
	chunks := splitter.Split(context.Background(), text)
	if len(chunks) < 2 {
		t.Errorf("expected force split to produce multiple chunks, got %d", len(chunks))
	}
}

func TestLineSplitter_Basic(t *testing.T) {
	splitter := NewLineSplitter(3)
	text := "line1\nline2\nline3\nline4\nline5"
	chunks := splitter.Split(context.Background(), text)
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestLineSplitter_DefaultValue(t *testing.T) {
	splitter := NewLineSplitter(0)
	if splitter.LinesPerChunk != 100 {
		t.Errorf("LinesPerChunk = %d, want 100", splitter.LinesPerChunk)
	}
}

func TestDocumentPipeline_Process(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "doc.txt")
	_ = os.WriteFile(f, []byte("hello world from pipeline"), 0644)

	loader := NewTextFileLoader()
	splitter := NewCharacterSplitter(1000, 100)
	pipeline := NewDocumentPipeline(loader, splitter)

	chunks, err := pipeline.Process(context.Background(), f)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "hello world from pipeline" {
		t.Errorf("content = %q", chunks[0].Content)
	}
	if chunks[0].Metadata["source"] == "" {
		t.Error("expected source metadata")
	}
}

func TestConvMem_AddAndGet(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{MaxMessages: 10})
	ctx := context.Background()

	_ = mem.AddMessage(ctx, "user", "Hello", nil)
	_ = mem.AddMessage(ctx, "assistant", "Hi there!", nil)

	msgs := mem.GetMessages()
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestConvMem_TriggerCompression(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages:    10,
		SummaryTrigger: 4,
	})
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		_ = mem.AddMessage(ctx, "user", "message "+string(rune('A'+i)), nil)
		_ = mem.AddMessage(ctx, "assistant", "response "+string(rune('A'+i)), nil)
	}

	summary := mem.GetSummary()
	if summary == "" {
		t.Error("expected summary to be generated after exceeding trigger")
	}
}

func TestConvMem_RecentMessages(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{MaxMessages: 100})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = mem.AddMessage(ctx, "user", "msg", nil)
	}

	recent := mem.GetRecentMessages(3)
	if len(recent) != 3 {
		t.Errorf("expected 3 recent messages, got %d", len(recent))
	}
}

func TestConvMem_Clear(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{})
	ctx := context.Background()
	_ = mem.AddMessage(ctx, "user", "test", nil)

	mem.Clear()
	if mem.GetMessageCount() != 0 {
		t.Errorf("expected 0 messages after clear, got %d", mem.GetMessageCount())
	}
}

func TestConvMem_Stats(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{})
	ctx := context.Background()
	_ = mem.AddMessage(ctx, "user", "hello", nil)
	_ = mem.AddMessage(ctx, "assistant", "hi", nil)

	stats := mem.GetStats()
	if stats["current_messages"] != 2 {
		t.Errorf("current_messages = %v, want 2", stats["current_messages"])
	}
	if stats["user_messages"] != 1 {
		t.Errorf("user_messages = %v, want 1", stats["user_messages"])
	}
}

func TestConvMem_ExportImportRoundTrip(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{})
	ctx := context.Background()
	_ = mem.AddMessage(ctx, "user", "export test", nil)

	data, err := mem.Export()
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	mem2 := NewConversationalMemory(ConversationalMemoryConfig{})
	err = mem2.Import(data)
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if mem2.GetMessageCount() != 1 {
		t.Errorf("expected 1 message after import, got %d", mem2.GetMessageCount())
	}
}

func TestConvMem_ImportBadJSON(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{})
	err := mem.Import([]byte("invalid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestConvMem_WindowLimit(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{MaxMessages: 3})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = mem.AddMessage(ctx, "user", "msg", nil)
	}

	if mem.GetMessageCount() > 3 {
		t.Errorf("expected max 3 messages, got %d", mem.GetMessageCount())
	}
}

func TestConvMem_TotalCount(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{MaxMessages: 3})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = mem.AddMessage(ctx, "user", "msg", nil)
	}

	if mem.GetTotalMessageCount() != 5 {
		t.Errorf("expected 5 total messages, got %d", mem.GetTotalMessageCount())
	}
}

func TestConvMem_InitialSummary(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		InitialSummary: "Previous context",
	})
	if mem.GetSummary() != "Previous context" {
		t.Errorf("summary = %q, want 'Previous context'", mem.GetSummary())
	}
}

func TestConvMem_MetaData(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		Metadata: map[string]string{"session": "test"},
	})
	stats := mem.GetStats()
	_ = stats
}

func TestDefaultCompressor_Direct(t *testing.T) {
	compressor := &DefaultCompressor{}
	messages := []*Message{
		{Role: "user", Content: "What is Go?"},
		{Role: "assistant", Content: "Go is a programming language."},
	}
	summary, err := compressor.Compress(context.Background(), messages, "")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestDefaultCompressor_WithExisting(t *testing.T) {
	compressor := &DefaultCompressor{}
	messages := []*Message{
		{Role: "user", Content: "Tell me more"},
	}
	summary, err := compressor.Compress(context.Background(), messages, "Existing summary")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}
	if !strings.Contains(summary, "Existing summary") {
		t.Error("expected existing summary to be preserved")
	}
}

func TestLLMCompressor_Direct(t *testing.T) {
	compressor := &LLMCompressor{}
	messages := []*Message{
		{Role: "user", Content: "Hello"},
	}
	summary, err := compressor.Compress(context.Background(), messages, "")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestEstimateTokens_Direct(t *testing.T) {
	if estimateTokens("") != 0 {
		t.Error("empty string should have 0 tokens")
	}
	if estimateTokens("hello") < 1 {
		t.Error("should have at least 1 token")
	}
}
