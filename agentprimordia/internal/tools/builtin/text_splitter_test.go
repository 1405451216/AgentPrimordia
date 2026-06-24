package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// --- TextSplitter 测试 ---

func TestTextSplitter_SimpleSplit(t *testing.T) {
	text := "第一段内容\n\n第二段内容\n\n第三段内容"
	splitter := NewTextSplitter(20, 0, "\n\n")
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Index != 0 {
		t.Errorf("expected first chunk index 0, got %d", chunks[0].Index)
	}
}

func TestTextSplitter_EmptyText(t *testing.T) {
	splitter := NewTextSplitter(100, 0, "\n\n")
	chunks := splitter.Split("")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestTextSplitter_ShortText(t *testing.T) {
	text := "短文本"
	splitter := NewTextSplitter(100, 0, "\n\n")
	chunks := splitter.Split(text)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "短文本" {
		t.Errorf("expected '短文本', got '%s'", chunks[0].Content)
	}
}

func TestTextSplitter_WithOverlap(t *testing.T) {
	text := "第一段内容比较长需要切分。第二段内容也比较长需要切分。第三段内容。"
	splitter := NewTextSplitter(20, 5, "")
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestTextSplitter_DefaultSeparator(t *testing.T) {
	splitter := NewTextSplitter(100, 0, "")
	if splitter.Separator != "\n\n" {
		t.Errorf("expected default separator '\\n\\n', got '%s'", splitter.Separator)
	}
}

func TestTextSplitter_DefaultChunkSize(t *testing.T) {
	splitter := NewTextSplitter(0, 0, "\n\n")
	if splitter.ChunkSize != 1000 {
		t.Errorf("expected default chunk size 1000, got %d", splitter.ChunkSize)
	}
}

// --- RecursiveTextSplitter 测试 ---

func TestRecursiveTextSplitter_BasicSplit(t *testing.T) {
	text := "第一段\n\n第二段\n\n第三段"
	splitter := NewRecursiveTextSplitter(20, 0, []string{"\n\n", "\n", " ", ""})
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestRecursiveTextSplitter_EmptyText(t *testing.T) {
	splitter := NewRecursiveTextSplitter(100, 0, nil)
	chunks := splitter.Split("")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestRecursiveTextSplitter_ShortText(t *testing.T) {
	text := "短文本"
	splitter := NewRecursiveTextSplitter(100, 0, nil)
	chunks := splitter.Split(text)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestRecursiveTextSplitter_FallbackToLowerSeparator(t *testing.T) {
	// 长段落，需要从 \n\n 降级到 \n 再到空格
	text := "这是一个很长的段落，包含很多内容需要被切分。这是同一行中的更多内容。还有更多文字来增加长度。"
	splitter := NewRecursiveTextSplitter(30, 0, []string{"\n\n", "\n", " ", ""})
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
	// 验证每个块不超过 chunkSize（允许一定误差）
	for _, chunk := range chunks {
		if len(chunk.Content) > 60 { // 给一些余量
			t.Errorf("chunk too large: %d chars", len(chunk.Content))
		}
	}
}

func TestRecursiveTextSplitter_DefaultSeparators(t *testing.T) {
	splitter := NewRecursiveTextSplitter(100, 0, nil)
	if len(splitter.Separators) != 4 {
		t.Errorf("expected 4 default separators, got %d", len(splitter.Separators))
	}
}

func TestRecursiveTextSplitter_WithOverlap(t *testing.T) {
	text := "段落一内容比较长需要切分处理。段落二内容也比较长需要切分处理。段落三内容。"
	splitter := NewRecursiveTextSplitter(20, 5, []string{"\n\n", "\n", " ", ""})
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

// --- FixedLengthSplitter 测试 ---

func TestFixedLengthSplitter_BasicSplit(t *testing.T) {
	text := "0123456789ABCDEFGHIJ" // 20 字符
	splitter := NewFixedLengthSplitter(10, 0)
	chunks := splitter.Split(text)
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Content != "0123456789" {
		t.Errorf("expected '0123456789', got '%s'", chunks[0].Content)
	}
	if chunks[1].Content != "ABCDEFGHIJ" {
		t.Errorf("expected 'ABCDEFGHIJ', got '%s'", chunks[1].Content)
	}
}

func TestFixedLengthSplitter_WithOverlap(t *testing.T) {
	text := "0123456789ABCDEFGHIJ" // 20 字符
	splitter := NewFixedLengthSplitter(10, 3)
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
	// step = 10 - 3 = 7, 所以第二个块从索引 7 开始
	if len(chunks) > 1 {
		if chunks[1].Content != "789ABCDEFG" {
			t.Errorf("expected '789ABCDEFG', got '%s'", chunks[1].Content)
		}
	}
}

func TestFixedLengthSplitter_EmptyText(t *testing.T) {
	splitter := NewFixedLengthSplitter(10, 0)
	chunks := splitter.Split("")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestFixedLengthSplitter_ExactFit(t *testing.T) {
	text := "0123456789" // 10 字符
	splitter := NewFixedLengthSplitter(10, 0)
	chunks := splitter.Split(text)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

// --- SentenceSplitter 测试 ---

func TestSentenceSplitter_BasicSplit(t *testing.T) {
	text := "这是第一句话。这是第二句话。这是第三句话。"
	splitter := NewSentenceSplitter(15, 0)
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestSentenceSplitter_EnglishSentences(t *testing.T) {
	text := "This is sentence one. This is sentence two. This is sentence three."
	splitter := NewSentenceSplitter(30, 0)
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestSentenceSplitter_QuestionAndExclamation(t *testing.T) {
	text := "What is this? This is great! Really? Yes."
	splitter := NewSentenceSplitter(20, 0)
	chunks := splitter.Split(text)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestSentenceSplitter_EmptyText(t *testing.T) {
	splitter := NewSentenceSplitter(100, 0)
	chunks := splitter.Split("")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestSentenceSplitter_ShortText(t *testing.T) {
	text := "短文本"
	splitter := NewSentenceSplitter(100, 0)
	chunks := splitter.Split(text)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

// --- TextSplitterTool 工具接口测试 ---

func TestTextSplitterTool_Name(t *testing.T) {
	tst := NewTextSplitterTool()
	if tst.Name() != "text_splitter" {
		t.Errorf("expected 'text_splitter', got '%s'", tst.Name())
	}
}

func TestTextSplitterTool_Description(t *testing.T) {
	tst := NewTextSplitterTool()
	desc := tst.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
}

func TestTextSplitterTool_Parameters(t *testing.T) {
	tst := NewTextSplitterTool()
	params := tst.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v", err)
	}
}

func TestTextSplitterTool_SimpleStrategy(t *testing.T) {
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action":     "split",
		"text":       "第一段\n\n第二段\n\n第三段",
		"strategy":   "simple",
		"chunk_size": 20,
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	var chunks []Chunk
	if err := json.Unmarshal([]byte(result.Content), &chunks); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestTextSplitterTool_RecursiveStrategy(t *testing.T) {
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action":     "split",
		"text":       "这是一个很长的段落需要被递归切分处理。这是更多内容。",
		"strategy":   "recursive",
		"chunk_size": 20,
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	var chunks []Chunk
	if err := json.Unmarshal([]byte(result.Content), &chunks); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestTextSplitterTool_FixedLengthStrategy(t *testing.T) {
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action":     "split",
		"text":       "0123456789ABCDEFGHIJ",
		"strategy":   "fixed_length",
		"chunk_size": 10,
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	var chunks []Chunk
	if err := json.Unmarshal([]byte(result.Content), &chunks); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestTextSplitterTool_SentenceStrategy(t *testing.T) {
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action":     "split",
		"text":       "这是第一句话。这是第二句话。这是第三句话。",
		"strategy":   "sentence",
		"chunk_size": 15,
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	var chunks []Chunk
	if err := json.Unmarshal([]byte(result.Content), &chunks); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestTextSplitterTool_InvalidAction(t *testing.T) {
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action": "invalid",
		"text":   "hello",
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestTextSplitterTool_EmptyText(t *testing.T) {
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action": "split",
		"text":   "",
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for empty text")
	}
}

func TestTextSplitterTool_InvalidStrategy(t *testing.T) {
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action":   "split",
		"text":     "hello",
		"strategy": "nonexistent",
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid strategy")
	}
}

// --- 内部函数测试 ---

func TestSplitSentences(t *testing.T) {
	text := "Hello world. How are you? I am fine!"
	sentences := splitSentences(text)
	if len(sentences) < 3 {
		t.Errorf("expected at least 3 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestSplitSentences_Chinese(t *testing.T) {
	text := "你好世界。你好吗？我很好！"
	sentences := splitSentences(text)
	if len(sentences) < 3 {
		t.Errorf("expected at least 3 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestIsSentenceEndRune(t *testing.T) {
	tests := []struct {
		ch       rune
		expected bool
	}{
		{'.', true},
		{'?', true},
		{'!', true},
		{'。', true},
		{'？', true},
		{'！', true},
		{',', false},
		{'a', false},
	}
	for _, tt := range tests {
		result := isSentenceEndRune(tt.ch)
		if result != tt.expected {
			t.Errorf("isSentenceEndRune(%q): expected %v, got %v", tt.ch, tt.expected, result)
		}
	}
}

func TestChunkIndexSequence(t *testing.T) {
	text := "段落一\n\n段落二\n\n段落三\n\n段落四"
	splitter := NewTextSplitter(20, 0, "\n\n")
	chunks := splitter.Split(text)
	for i, chunk := range chunks {
		if chunk.Index != i {
			t.Errorf("chunk %d has Index %d, expected %d", i, chunk.Index, i)
		}
	}
}

func TestTextSplitterTool_DefaultStrategy(t *testing.T) {
	// 不指定 strategy 时默认使用 simple
	tst := NewTextSplitterTool()
	args, _ := json.Marshal(map[string]any{
		"action":     "split",
		"text":       "第一段\n\n第二段",
		"chunk_size": 100,
	})
	result, err := tst.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	var chunks []Chunk
	if err := json.Unmarshal([]byte(result.Content), &chunks); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if !strings.Contains(chunks[0].Content, "第一段") {
		t.Errorf("first chunk should contain '第一段', got '%s'", chunks[0].Content)
	}
}
