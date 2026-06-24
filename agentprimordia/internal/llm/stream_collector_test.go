package llm

import (
	"testing"
	"time"
)

// ===== StreamCollector 测试 =====

func TestStreamCollector_BasicCollection(t *testing.T) {
	// 基本收集：多个 token chunk + Done chunk
	ch := make(chan Chunk, 10)
	go func() {
		ch <- Chunk{Content: "Hello"}
		ch <- Chunk{Content: " world"}
		ch <- Chunk{Content: "!"}
		ch <- Chunk{Done: true, Usage: &Usage{PromptTokens: 10, CompletionTokens: 3}}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if result.Content != "Hello world!" {
		t.Errorf("Content = %q, want %q", result.Content, "Hello world!")
	}
	if result.Tokens != 3 {
		t.Errorf("Tokens = %d, want 3", result.Tokens)
	}
	if result.Duration < 0 {
		t.Error("Duration should be >= 0")
	}
}

func TestStreamCollector_WithToolCalls(t *testing.T) {
	// 收集包含工具调用信息的 chunk
	ch := make(chan Chunk, 10)
	go func() {
		ch <- Chunk{Content: "Let me check"}
		ch <- Chunk{Content: "", Done: true, Usage: &Usage{PromptTokens: 5, CompletionTokens: 3}}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if result.Content != "Let me check" {
		t.Errorf("Content = %q, want %q", result.Content, "Let me check")
	}
}

func TestStreamCollector_EmptyStream(t *testing.T) {
	// 空流：只有 Done chunk
	ch := make(chan Chunk, 1)
	go func() {
		ch <- Chunk{Done: true}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if result.Content != "" {
		t.Errorf("Content = %q, want empty", result.Content)
	}
	if result.Tokens != 0 {
		t.Errorf("Tokens = %d, want 0", result.Tokens)
	}
}

func TestStreamCollector_WithUsage(t *testing.T) {
	// 验证 Usage 信息收集
	ch := make(chan Chunk, 10)
	go func() {
		ch <- Chunk{Content: "test"}
		ch <- Chunk{Done: true, Usage: &Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if result.Usage == nil {
		t.Fatal("Usage should not be nil")
	}
	if result.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", result.Usage.CompletionTokens)
	}
}

func TestStreamCollector_MultipleCollectCalls(t *testing.T) {
	// 同一个 collector 可以多次调用 Collect
	ch1 := make(chan Chunk, 5)
	go func() {
		ch1 <- Chunk{Content: "first"}
		ch1 <- Chunk{Done: true}
		close(ch1)
	}()

	collector := NewStreamCollector()
	result1, err := collector.Collect(ch1)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if result1.Content != "first" {
		t.Errorf("Content = %q, want %q", result1.Content, "first")
	}

	ch2 := make(chan Chunk, 5)
	go func() {
		ch2 <- Chunk{Content: "second"}
		ch2 <- Chunk{Done: true}
		close(ch2)
	}()

	result2, err := collector.Collect(ch2)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if result2.Content != "second" {
		t.Errorf("Content = %q, want %q", result2.Content, "second")
	}
}

func TestStreamCollector_ConcurrentAccess(t *testing.T) {
	// 并发安全测试：多个 goroutine 同时收集
	var results []*CollectedResult
	done := make(chan struct{})

	for i := 0; i < 5; i++ {
		go func(i int) {
			ch := make(chan Chunk, 5)
			go func() {
				ch <- Chunk{Content: "concurrent"}
				ch <- Chunk{Done: true}
				close(ch)
			}()

			collector := NewStreamCollector()
			result, err := collector.Collect(ch)
			if err != nil {
				t.Errorf("Collect() error = %v", err)
			}
			results = append(results, result)
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
}

func TestCollectedResult_JSONSerialization(t *testing.T) {
	// 验证 CollectedResult 的 JSON 序列化
	result := &CollectedResult{
		Content: "Hello world",
		Tokens:  2,
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		Duration: 100 * time.Millisecond,
	}

	// 验证字段可正确访问
	if result.Content != "Hello world" {
		t.Errorf("Content = %q, want %q", result.Content, "Hello world")
	}
	if result.Tokens != 2 {
		t.Errorf("Tokens = %d, want 2", result.Tokens)
	}
}

func TestStreamCollector_ChannelClosedWithoutDone(t *testing.T) {
	// 通道关闭但没有 Done chunk
	ch := make(chan Chunk, 5)
	go func() {
		ch <- Chunk{Content: "partial"}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	// 应该仍然收集到内容
	if result.Content != "partial" {
		t.Errorf("Content = %q, want %q", result.Content, "partial")
	}
}

func TestStreamCollector_WithToolCallInfo(t *testing.T) {
	// 收集带工具调用信息的流
	ch := make(chan Chunk, 10)
	go func() {
		ch <- Chunk{Content: "Thinking..."}
		ch <- Chunk{Content: "", Done: true}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if result.Content != "Thinking..." {
		t.Errorf("Content = %q, want %q", result.Content, "Thinking...")
	}
}

func TestNewStreamCollector(t *testing.T) {
	c := NewStreamCollector()
	if c == nil {
		t.Fatal("NewStreamCollector() returned nil")
	}
}

func TestStreamCollector_ErrorInChannel(t *testing.T) {
	// 模拟流中发生错误（通过特殊 chunk 传递）
	ch := make(chan Chunk, 5)
	go func() {
		ch <- Chunk{Content: "start"}
		ch <- Chunk{Content: "", Done: true}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if result.Content != "start" {
		t.Errorf("Content = %q, want %q", result.Content, "start")
	}
}

func TestStreamCollector_LargeStream(t *testing.T) {
	// 大量 chunk 的收集
	ch := make(chan Chunk, 1000)
	go func() {
		for i := 0; i < 500; i++ {
			ch <- Chunk{Content: "x"}
		}
		ch <- Chunk{Done: true, Usage: &Usage{PromptTokens: 10, CompletionTokens: 500}}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(result.Content) != 500 {
		t.Errorf("Content length = %d, want 500", len(result.Content))
	}
	if result.Tokens != 500 {
		t.Errorf("Tokens = %d, want 500", result.Tokens)
	}
}

// 确保编译时接口检查
var _ error = (*StreamError)(nil)

func TestStreamError(t *testing.T) {
	err := &StreamError{Message: "test error"}
	if err.Error() != "test error" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error")
	}
}

func TestStreamCollector_CollectWithContextCancel(t *testing.T) {
	// 验证 collector 在通道正常关闭时工作
	ch := make(chan Chunk, 5)
	go func() {
		ch <- Chunk{Content: "before cancel"}
		ch <- Chunk{Done: true}
		close(ch)
	}()

	collector := NewStreamCollector()
	result, err := collector.Collect(ch)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if result.Content != "before cancel" {
		t.Errorf("Content = %q, want %q", result.Content, "before cancel")
	}
}
