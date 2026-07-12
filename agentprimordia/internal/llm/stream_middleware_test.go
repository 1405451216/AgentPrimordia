package llm

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== StreamPipeline 测试 =====

func TestStreamPipeline_Process_NoMiddleware(t *testing.T) {
	// 无中间件时，chunk 应直接传递给最终处理器
	var collected []Chunk
	handler := func(c Chunk) error {
		collected = append(collected, c)
		return nil
	}

	p := NewStreamPipeline(handler)

	c1 := Chunk{Content: "hello"}
	c2 := Chunk{Content: " world", Done: true}

	if err := p.Process(c1); err != nil {
		t.Fatalf("Process(c1) error = %v", err)
	}
	if err := p.Process(c2); err != nil {
		t.Fatalf("Process(c2) error = %v", err)
	}

	if len(collected) != 2 {
		t.Fatalf("collected %d chunks, want 2", len(collected))
	}
	if collected[0].Content != "hello" {
		t.Errorf("collected[0].Content = %q, want %q", collected[0].Content, "hello")
	}
	if !collected[1].Done {
		t.Error("collected[1].Done = false, want true")
	}
}

func TestStreamPipeline_Use_ChainOrder(t *testing.T) {
	// 中间件应按添加顺序执行
	var order []string

	mw1 := func(c Chunk, next StreamHandler) error {
		order = append(order, "mw1-before")
		err := next(c)
		order = append(order, "mw1-after")
		return err
	}
	mw2 := func(c Chunk, next StreamHandler) error {
		order = append(order, "mw2-before")
		err := next(c)
		order = append(order, "mw2-after")
		return err
	}

	handler := func(c Chunk) error {
		order = append(order, "handler")
		return nil
	}

	p := NewStreamPipeline(handler)
	p.Use(mw1).Use(mw2)

	if err := p.Process(Chunk{Content: "test"}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("execution order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestStreamPipeline_MiddlewareCanModify(t *testing.T) {
	// 中间件可以修改 chunk 内容
	upperMW := func(c Chunk, next StreamHandler) error {
		c.Content = strings.ToUpper(c.Content)
		return next(c)
	}

	var collected []Chunk
	handler := func(c Chunk) error {
		collected = append(collected, c)
		return nil
	}

	p := NewStreamPipeline(handler)
	p.Use(upperMW)

	if err := p.Process(Chunk{Content: "hello"}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(collected) != 1 {
		t.Fatalf("collected %d chunks, want 1", len(collected))
	}
	if collected[0].Content != "HELLO" {
		t.Errorf("content = %q, want %q", collected[0].Content, "HELLO")
	}
}

func TestStreamPipeline_MiddlewareCanShortCircuit(t *testing.T) {
	// 中间件可以短路，不调用 next
	blockMW := func(c Chunk, next StreamHandler) error {
		// 不调用 next，直接返回
		return nil
	}

	var handlerCalled bool
	handler := func(c Chunk) error {
		handlerCalled = true
		return nil
	}

	p := NewStreamPipeline(handler)
	p.Use(blockMW)

	if err := p.Process(Chunk{Content: "test"}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if handlerCalled {
		t.Error("handler should not have been called when middleware short-circuits")
	}
}

// ===== 内置中间件测试 =====

func TestFilterMiddleware_RemovesEmptyChunks(t *testing.T) {
	var collected []Chunk
	handler := func(c Chunk) error {
		collected = append(collected, c)
		return nil
	}

	p := NewStreamPipeline(handler)
	p.Use(FilterMiddleware())

	chunks := []Chunk{
		{Content: ""},
		{Content: "hello"},
		{Content: ""},
		{Content: " world"},
		{Content: "", Done: true}, // Done chunk 即使内容为空也应保留
	}

	for _, c := range chunks {
		if err := p.Process(c); err != nil {
			t.Fatalf("Process() error = %v", err)
		}
	}

	// 应该保留 "hello", " world", 和 Done chunk
	if len(collected) != 3 {
		t.Fatalf("collected %d chunks, want 3", len(collected))
	}
	if collected[0].Content != "hello" {
		t.Errorf("collected[0].Content = %q, want %q", collected[0].Content, "hello")
	}
	if collected[1].Content != " world" {
		t.Errorf("collected[1].Content = %q, want %q", collected[1].Content, " world")
	}
	if !collected[2].Done {
		t.Error("collected[2].Done = false, want true")
	}
}

func TestTransformMiddleware(t *testing.T) {
	var collected []Chunk
	handler := func(c Chunk) error {
		collected = append(collected, c)
		return nil
	}

	// 转大写变换
	upperTransform := func(content string) string {
		return strings.ToUpper(content)
	}

	p := NewStreamPipeline(handler)
	p.Use(TransformMiddleware(upperTransform))

	chunks := []Chunk{
		{Content: "hello"},
		{Content: " world"},
	}

	for _, c := range chunks {
		if err := p.Process(c); err != nil {
			t.Fatalf("Process() error = %v", err)
		}
	}

	if len(collected) != 2 {
		t.Fatalf("collected %d chunks, want 2", len(collected))
	}
	if collected[0].Content != "HELLO" {
		t.Errorf("collected[0].Content = %q, want %q", collected[0].Content, "HELLO")
	}
	if collected[1].Content != " WORLD" {
		t.Errorf("collected[1].Content = %q, want %q", collected[1].Content, " WORLD")
	}
}

func TestLoggingMiddleware(t *testing.T) {
	// LoggingMiddleware 应记录每个 chunk 的内容长度和时间戳
	var collected []Chunk
	handler := func(c Chunk) error {
		collected = append(collected, c)
		return nil
	}

	logEntries := LoggingMiddleware()

	p := NewStreamPipeline(handler)
	p.Use(logEntries)

	if err := p.Process(Chunk{Content: "hello"}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// LoggingMiddleware 不应修改 chunk，只记录日志
	if len(collected) != 1 {
		t.Fatalf("collected %d chunks, want 1", len(collected))
	}
	if collected[0].Content != "hello" {
		t.Errorf("content = %q, want %q", collected[0].Content, "hello")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	// RateLimitMiddleware 应限制每秒处理的 chunk 数量
	var collected []Chunk
	var mu sync.Mutex
	handler := func(c Chunk) error {
		mu.Lock()
		collected = append(collected, c)
		mu.Unlock()
		return nil
	}

	// 每秒 100 个 chunk
	p := NewStreamPipeline(handler)
	p.Use(RateLimitMiddleware(100))

	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := p.Process(Chunk{Content: "x"}); err != nil {
			t.Fatalf("Process() error = %v", err)
		}
	}
	elapsed := time.Since(start)

	// 5 个 chunk 在 100/s 限制下应该很快完成（突发允许）
	// 不应超过 1 秒
	if elapsed > time.Second {
		t.Errorf("rate limited too aggressively: %v", elapsed)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(collected) != 5 {
		t.Errorf("collected %d chunks, want 5", len(collected))
	}
}

func TestBufferMiddleware(t *testing.T) {
	// BufferMiddleware 应在句子边界处刷新
	var collected []string
	var mu sync.Mutex
	handler := func(c Chunk) error {
		mu.Lock()
		collected = append(collected, c.Content)
		mu.Unlock()
		return nil
	}

	p := NewStreamPipeline(handler)
	p.Use(BufferMiddleware())

	// 逐字发送 "Hello. World!"
	chunks := []Chunk{
		{Content: "Hel"},
		{Content: "lo. "},
		{Content: "Wor"},
		{Content: "ld!"},
		{Content: "", Done: true},
	}

	for _, c := range chunks {
		if err := p.Process(c); err != nil {
			t.Fatalf("Process() error = %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	// 至少应有 1 个收集到的内容（Done 时强制刷新）
	if len(collected) == 0 {
		t.Error("expected at least 1 buffered output")
	}

	// 完整内容应包含 "Hello. World!"
	full := strings.Join(collected, "")
	if !strings.Contains(full, "Hello.") || !strings.Contains(full, "World!") {
		t.Errorf("full content = %q, want to contain 'Hello.' and 'World!'", full)
	}
}

func TestBufferMiddleware_FlushOnSentenceBoundary(t *testing.T) {
	// 测试在句子边界处刷新
	var flushCount int
	handler := func(c Chunk) error {
		if c.Content != "" {
			flushCount++
		}
		return nil
	}

	p := NewStreamPipeline(handler)
	p.Use(BufferMiddleware())

	// 句子边界字符：. ! ? \n
	if err := p.Process(Chunk{Content: "Hello."}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if err := p.Process(Chunk{Content: " World!"}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if err := p.Process(Chunk{Content: "", Done: true}); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// "Hello." 和 " World!" 各触发一次刷新
	if flushCount != 2 {
		t.Errorf("flushCount = %d, want 2", flushCount)
	}
}

func TestStreamPipeline_ConcurrentProcess(t *testing.T) {
	// 并发安全测试
	var collected []Chunk
	var mu sync.Mutex
	handler := func(c Chunk) error {
		mu.Lock()
		collected = append(collected, c)
		mu.Unlock()
		return nil
	}

	p := NewStreamPipeline(handler)
	p.Use(FilterMiddleware())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			content := ""
			if i%2 == 0 {
				content = "chunk"
			}
			if err := p.Process(Chunk{Content: content}); err != nil {
				t.Errorf("Process() error = %v", err)
			}
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	// 只有偶数 i 的 chunk 有内容（50 个）
	if len(collected) != 50 {
		t.Errorf("collected %d chunks, want 50", len(collected))
	}
}

func TestNewStreamPipeline(t *testing.T) {
	handler := func(c Chunk) error { return nil }
	p := NewStreamPipeline(handler)
	if p == nil {
		t.Fatal("NewStreamPipeline() returned nil")
	}
	if len(p.middlewares) != 0 {
		t.Errorf("new pipeline should have no middlewares, got %d", len(p.middlewares))
	}
}

