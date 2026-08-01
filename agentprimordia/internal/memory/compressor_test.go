package memory

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCompressor_CompressOldEpisodes(t *testing.T) {
	mem := NewInMemoryStore()
	ctx := context.Background()

	// 添加 20 条记忆
	for i := 0; i < 20; i++ {
		_ = mem.Add(ctx, &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: "test-session",
			Content:   fmt.Sprintf("第 %d 条对话内容，包含一些常见的关键词", i),
			Role:      "user",
			CreatedAt: time.Now().Add(-time.Duration(20-i) * time.Hour).Format(time.RFC3339),
		})
	}

	comp := NewCompressor(CompressorConfig{
		WindowSize:  10,
		MinEpisodes: 5,
		Summarizer:  &mockCompressorSummarizer{},
	})

	// 压缩前：20 条
	episodes, _ := mem.List(ctx, nil)
	if len(episodes) != 20 {
		t.Fatalf("压缩前 = %d 条, 期望 20", len(episodes))
	}

	// 执行压缩
	err := comp.Compress(ctx, mem)
	if err != nil {
		t.Fatalf("压缩失败: %v", err)
	}

	// 压缩后：旧条目被替换为摘要，总数应减少
	episodes, _ = mem.List(ctx, nil)
	if len(episodes) >= 20 {
		t.Errorf("压缩后 = %d 条, 期望少于 20", len(episodes))
	}
}

func TestCompressor_SkipRecentEpisodes(t *testing.T) {
	mem := NewInMemoryStore()
	ctx := context.Background()

	// 只添加 3 条（少于窗口大小）
	for i := 0; i < 3; i++ {
		_ = mem.Add(ctx, &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: "test-session",
			Content:   fmt.Sprintf("最近的对话 %d", i),
			Role:      "user",
			CreatedAt: time.Now().Format(time.RFC3339),
		})
	}

	comp := NewCompressor(CompressorConfig{
		WindowSize:  10,
		MinEpisodes: 5,
		Summarizer:  &mockCompressorSummarizer{},
	})

	err := comp.Compress(ctx, mem)
	if err != nil {
		t.Fatalf("压缩失败: %v", err)
	}

	// 条目少不应触发压缩
	episodes, _ := mem.List(ctx, nil)
	if len(episodes) != 3 {
		t.Errorf("不应压缩: %d 条, 期望 3", len(episodes))
	}
}

// mockCompressorSummarizer 模拟摘要器
type mockCompressorSummarizer struct{}

func (m *mockCompressorSummarizer) Summarize(ctx context.Context, episodes []*Episode) (*CompressorSummary, error) {
	return &CompressorSummary{
		Text: "对话摘要：用户进行了多次交流",
		Tags: []string{"summary"},
	}, nil
}
