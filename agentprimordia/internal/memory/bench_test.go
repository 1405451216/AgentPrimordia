package memory

import (
	"context"
	"fmt"
	"testing"
)

// BenchmarkMemory_Add 测试单条 Episode 写入性能
func BenchmarkMemory_Add(b *testing.B) {
	b.ReportAllocs()

	store, err := WithInMemory()
	if err != nil {
		b.Fatalf("WithInMemory() error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ep := MustEpisode("bench-session", "user", fmt.Sprintf("benchmark message %d", i))
		if err := store.Add(ctx, ep); err != nil {
			b.Fatalf("Add() error: %v", err)
		}
	}
}

// BenchmarkMemory_Search 测试 FTS 全文搜索性能
func BenchmarkMemory_Search(b *testing.B) {
	b.ReportAllocs()

	store, err := WithInMemory()
	if err != nil {
		b.Fatalf("WithInMemory() error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// 预填充数据
	for i := 0; i < 1000; i++ {
		ep := MustEpisode("bench-session", "user", fmt.Sprintf("这是第 %d 条关于 Go 语言和并发编程的测试消息", i))
		ep.Topics = "go,concurrency,benchmark"
		if err := store.Add(ctx, ep); err != nil {
			b.Fatalf("Add() error: %v", err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := store.Search(ctx, "Go 语言", &SearchOptions{Limit: 10})
		if err != nil {
			b.Fatalf("Search() error: %v", err)
		}
	}
}

// BenchmarkMemory_FTS5Search 测试 FTS5 复杂查询性能（多关键词 + 过滤）
func BenchmarkMemory_FTS5Search(b *testing.B) {
	b.ReportAllocs()

	store, err := WithInMemory()
	if err != nil {
		b.Fatalf("WithInMemory() error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// 预填充数据，包含不同 session 和 role
	for i := 0; i < 1000; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		sessionID := fmt.Sprintf("session-%d", i%10)
		ep := MustEpisode(sessionID, role, fmt.Sprintf("Agent 框架中的 ReAct 循环和工具调用机制，第 %d 次迭代", i))
		ep.Summary = fmt.Sprintf("关于 ReAct 循环的摘要 %d", i)
		ep.Topics = "react,tools,agent"
		if err := store.Add(ctx, ep); err != nil {
			b.Fatalf("Add() error: %v", err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := store.Search(ctx, "ReAct 工具", &SearchOptions{
			SessionID:  "session-0",
			RoleFilter: "assistant",
			Limit:      20,
		})
		if err != nil {
			b.Fatalf("Search() error: %v", err)
		}
	}
}
