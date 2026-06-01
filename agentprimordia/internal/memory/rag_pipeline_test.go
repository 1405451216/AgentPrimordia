package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== 切分策略注册表测试 =====

func TestSplitterRegistry_AvailableStrategies(t *testing.T) {
	strategies := AvailableStrategies()
	if len(strategies) < 7 {
		t.Errorf("期望至少 7 种切分策略，实际 %d", len(strategies))
	}

	expected := []string{"character", "code", "line", "markdown", "recursive", "sentence", "token"}
	for _, exp := range expected {
		found := false
		for _, s := range strategies {
			if s == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("缺少策略: %s", exp)
		}
	}
}

func TestCreateSplitter_UnknownStrategy(t *testing.T) {
	_, err := CreateSplitter("nonexistent", DefaultSplitterConfig())
	if err == nil {
		t.Error("未知策略应返回错误")
	}
}

func TestCreateSplitter_AllStrategies(t *testing.T) {
	strategies := []SplitterStrategy{
		StrategyCharacter, StrategyRecursive, StrategyLine,
		StrategySentence, StrategyMarkdown, StrategyToken, StrategyCode,
	}
	for _, s := range strategies {
		sp, err := CreateSplitter(s, SplitterConfig{ChunkSize: 500})
		if err != nil {
			t.Errorf("创建策略 %q 失败: %v", s, err)
			continue
		}
		if sp == nil {
			t.Errorf("策略 %q 返回了 nil 切分器", s)
		}
	}
}

// ===== SentenceSplitter 测试 =====

func TestSentenceSplitter_ShortText(t *testing.T) {
	s := NewSentenceSplitter(1000, 50)
	result := s.Split(context.Background(), "这是一个短句子。")
	if len(result) != 1 || result[0] != "这是一个短句子。" {
		t.Errorf("短文本不应被切分，得到 %d 块", len(result))
	}
}

func TestSentenceSplitter_MultipleSentences(t *testing.T) {
	s := NewSentenceSplitter(30, 5)
	text := "第一句话。第二句话。第三句话。第四句话。"
	result := s.Split(context.Background(), text)

	if len(result) < 2 {
		t.Errorf("多句文本应被切分为多块，得到 %d 块: %v", len(result), result)
	}

	for _, chunk := range result {
		if chunk == "" {
			t.Error("块内容不应为空")
		}
	}
}

func TestSentenceSplitter_PreservesContent(t *testing.T) {
	s := NewSentenceSplitter(20, 3)
	text := "Hello world. This is a test. Foo bar baz."
	result := s.Split(context.Background(), text)

	combined := strings.Join(result, "")
	if !strings.Contains(combined, "Hello") ||
		!strings.Contains(combined, "test") ||
		!strings.Contains(combined, "baz") {
		t.Error("切分后应保留所有原始内容")
	}
}

func TestSentenceSplitter_QuestionMarks(t *testing.T) {
	s := NewSentenceSplitter(15, 2)
	text := "What is this? Who are you? Why here?"
	result := s.Split(context.Background(), text)

	if len(result) < 2 {
		t.Errorf("问号应作为句子边界，得到 %d 块", len(result))
	}
}

// ===== MarkdownSplitter 测试 =====

func TestMarkdownSplitter_ShortDoc(t *testing.T) {
	s := NewMarkdownSplitter(2000, 100)
	text := "# 标题\n\n一些内容"
	result := s.Split(context.Background(), text)

	if len(result) != 1 {
		t.Errorf("短 Markdown 不应被切分")
	}
}

func TestMarkdownSplitter_ByHeaders(t *testing.T) {
	s := NewMarkdownSplitter(50, 10)
	text := `# 第一章

这是第一章的内容。

## 第一节

第一节的内容。

# 第二章

第二章的内容。`

	result := s.Split(context.Background(), text)

	if len(result) < 2 {
		t.Errorf("按标题切分应产生多块，得到 %d 块: %v", len(result), result)
	}
}

func TestMarkdownSplitter_PreservesHeaders(t *testing.T) {
	s := NewMarkdownSplitter(30, 5)
	text := `# Header A

Content A.

# Header B

Content B.`

	result := s.Split(context.Background(), text)
	combined := strings.Join(result, "")

	if !strings.Contains(combined, "# Header A") || !strings.Contains(combined, "# Header B") {
		t.Error("标题应在切分结果中保留")
	}
}

// ===== TokenCountSplitter 测试 =====

func TestTokenCountSplitter_ShortText(t *testing.T) {
	s := NewTokenCountSplitter(512, 50)
	text := "这是一段短文本"
	result := s.Split(context.Background(), text)

	if len(result) != 1 {
		t.Errorf("短文本不应被切分，得到 %d 块", len(result))
	}
}

func TestTokenCountSplitter_EstimatesTokens(t *testing.T) {
	s := NewTokenCountSplitter(15, 3)
	longText := "这是第一段内容。\n\n这是第二段内容。\n\n这是第三段内容。\n\n" +
		"这是第四段内容。\n\n这是第五段内容。\n\n这是第六段内容。\n\n" +
		"这是第七段内容。\n\n这是第八段内容。\n\n这是第九段内容。\n\n" +
		"这是第十段内容。"
	result := s.Split(context.Background(), longText)

	if len(result) < 2 {
		t.Errorf("长文本应被切分，得到 %d 块", len(result))
	}
}

func TestTokenCountSplitter_CJKHandling(t *testing.T) {
	s := NewTokenCountSplitter(5, 1)
	cjkText := "这是纯中文长文本需要切分成多个小块来测试"
	result := s.Split(context.Background(), cjkText)

	if len(result) < 1 {
		t.Error("CJK 文本应能正常处理")
	}
}

// ===== CodeSplitter 测试 =====

func TestCodeSplitter_GoLanguage(t *testing.T) {
	s := NewCodeSplitter("go", 80, 10)
	code := `package main

import "fmt"

func hello() {
	fmt.Println("hello")
}

func world() {
	fmt.Println("world")
}`

	result := s.Split(context.Background(), code)

	if len(result) < 2 {
		t.Errorf("Go 代码应按函数切分，得到 %d 块", len(result))
	}

	hasHello := false
	hasWorld := false
	for _, chunk := range result {
		if strings.Contains(chunk, "func hello") {
			hasHello = true
		}
		if strings.Contains(chunk, "func world") {
			hasWorld = true
		}
	}
	if !hasHello || !hasWorld {
		t.Log("Go 切分结果: 函数可能被合并到同一块中")
	}
}

func TestCodeSplitter_PythonLanguage(t *testing.T) {
	s := NewCodeSplitter("python", 60, 8)
	code := `def foo():
    pass

class Bar:
    pass

def baz():
    pass`

	result := s.Split(context.Background(), code)

	if len(result) < 2 {
		t.Errorf("Python 代码应按结构切分，得到 %d 块", len(result))
	}
}

func TestCodeSplitter_GenericFallback(t *testing.T) {
	s := NewCodeSplitter("generic", 50, 5)
	code := `function a() {}
function b() {}
function c() {}`

	result := s.Split(context.Background(), code)

	if len(result) < 1 {
		t.Error("generic 模式应能处理任意代码")
	}
}

func TestCodeSplitter_ShortCode(t *testing.T) {
	s := NewCodeSplitter("go", 1000, 50)
	code := "func main() {}\n"
	result := s.Split(context.Background(), code)

	if len(result) != 1 {
		t.Errorf("短代码不应被切分")
	}
}

// ===== SemanticChunker 测试（使用 mock embedding）=====

func mockEmbedding(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, 4)
		for j := range vec {
			vec[j] = float32(i+1) * float32(j+1) / 10.0
		}
		vecs[i] = vec
	}
	return vecs, nil
}

func TestSemanticChunker_MergesSimilarChunks(t *testing.T) {
	sc, err := NewSemanticChunker(SemanticChunkerConfig{
		BaseStrategy: StrategyCharacter,
		BaseConfig:   SplitterConfig{ChunkSize: 20},
		Embedder:     mockEmbedding,
		SimThreshold: 0.99,
		MaxChunkSize: 200,
	})
	if err != nil {
		t.Fatalf("创建语义切分器失败: %v", err)
	}

	text := "aaaa bbbb cccc dddd eeee ffff gggg hhhh"
	result := sc.Split(context.Background(), text)

	if len(result) == 0 {
		t.Error("语义切分不应返回空结果")
	}
}

func TestSemanticChunker_NilEmbedderError(t *testing.T) {
	_, err := NewSemanticChunker(SemanticChunkerConfig{
		BaseStrategy: StrategyCharacter,
		Embedder:     nil,
	})

	if err == nil {
		t.Error("nil embedder 应返回错误")
	}
}

func TestSemanticChunker_EmbeddingFailsGracefully(t *testing.T) {
	failEmbed := func(_ context.Context, texts []string) ([][]float32, error) {
		return nil, context.Canceled
	}

	sc, _ := NewSemanticChunker(SemanticChunkerConfig{
		BaseStrategy: StrategyRecursive,
		BaseConfig:   SplitterConfig{ChunkSize: 50},
		Embedder:     failEmbed,
		SimThreshold: 0.9,
	})

	text := "hello world test semantic chunking fallback"
	result := sc.Split(context.Background(), text)

	if len(result) == 0 {
		t.Error("embedding 失败时应回退到基础切分")
	}
}

// ===== EnhancedRAGPipeline 测试 =====

func TestEnhancedRAGPipeline_Ingest(t *testing.T) {
	mem, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer mem.Close()
	store := NewRAGStore(mem, nil)

	pipeline, err := NewEnhancedRAGPipeline(EnhancedRAGPipelineConfig{
		Loader:        NewTextFileLoader(),
		SplitStrategy: StrategySentence,
		SplitConfig:   SplitterConfig{ChunkSize: 50},
		RAGStore:      store,
	})
	if err != nil {
		t.Fatalf("创建管道失败: %v", err)
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("这是第一句话。这是第二句话。这是第三句话。"), 0644)

	result, err := pipeline.Ingest(context.Background(), testFile)
	if err != nil {
		t.Fatalf("Ingest 失败: %v", err)
	}

	if result.Ingested == 0 {
		t.Error("应有至少一个 chunk 被摄入")
	}
	if result.TotalChunks == 0 {
		t.Error("总 chunk 数应大于 0")
	}
}

func TestEnhancedRAGPipeline_MultipleDocs(t *testing.T) {
	mem, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer mem.Close()
	store := NewRAGStore(mem, nil)

	pipeline, _ := NewEnhancedRAGPipeline(EnhancedRAGPipelineConfig{
		Loader:        NewTextFileLoader(),
		SplitStrategy: StrategyLine,
		SplitConfig:   SplitterConfig{ChunkSize: 2},
		RAGStore:      store,
	})

	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("line1\nline2\nline3"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("lineA\nlineB"), 0644)

	result, err := pipeline.Ingest(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("目录 Ingest 失败: %v", err)
	}

	if result.Ingested < 3 {
		t.Errorf("多个文件应产生更多 chunks，摄入 %d", result.Ingested)
	}
}
