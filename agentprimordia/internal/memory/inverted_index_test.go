// inverted_index_test.go 验证 InMemoryStore 倒排索引的正确性与性能
// perf-v6 round 8 Task 2
package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestInMemoryStore_Search_BasicMatch 验证单 token 搜索
func TestInMemoryStore_Search_BasicMatch(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	_ = store.Add(ctx, MustEpisode("s1", "user", "The quick brown fox jumps over the lazy dog"))
	_ = store.Add(ctx, MustEpisode("s1", "assistant", "I like programming in Go"))

	results, err := store.Search(ctx, "fox", &SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search results length = %d, want 1", len(results))
	}
	if results[0].Content != "The quick brown fox jumps over the lazy dog" {
		t.Errorf("got wrong episode: %q", results[0].Content)
	}
}

// TestInMemoryStore_Search_CaseInsensitive 验证 case-insensitive
func TestInMemoryStore_Search_CaseInsensitive(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	_ = store.Add(ctx, MustEpisode("s1", "user", "Python is great"))

	results, _ := store.Search(ctx, "python", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("lowercase query, lowercase content: got %d, want 1", len(results))
	}
	results, _ = store.Search(ctx, "PYTHON", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("uppercase query: got %d, want 1", len(results))
	}
	results, _ = store.Search(ctx, "Python", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("title-case query: got %d, want 1", len(results))
	}
}

// TestInMemoryStore_Search_MultiToken 验证多 token AND 语义
func TestInMemoryStore_Search_MultiToken(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	_ = store.Add(ctx, MustEpisode("s1", "user", "Go is a programming language"))
	_ = store.Add(ctx, MustEpisode("s1", "user", "Python is also a programming language"))
	_ = store.Add(ctx, MustEpisode("s1", "user", "Just talking about food"))

	// "Go programming" → 期望 1 个 (Go + programming 都在)
	results, _ := store.Search(ctx, "Go programming", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("AND multi-token: got %d, want 1 (Go + programming)", len(results))
	}
	if len(results) > 0 && results[0].Content != "Go is a programming language" {
		t.Errorf("got wrong episode: %q", results[0].Content)
	}
}

// TestInMemoryStore_Search_NoMatch 验证无匹配 token
func TestInMemoryStore_Search_NoMatch(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	_ = store.Add(ctx, MustEpisode("s1", "user", "Hello world"))

	results, err := store.Search(ctx, "nonexistentterm", &SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search results length = %d, want 0", len(results))
	}
}

// TestInMemoryStore_Search_WithSessionFilter 验证 session filter
func TestInMemoryStore_Search_WithSessionFilter(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	_ = store.Add(ctx, MustEpisode("session-a", "user", "Go programming language"))
	_ = store.Add(ctx, MustEpisode("session-b", "user", "Go programming language"))

	results, _ := store.Search(ctx, "programming", &SearchOptions{SessionID: "session-a", Limit: 10})
	if len(results) != 1 {
		t.Errorf("SessionID filter: got %d, want 1", len(results))
	}
	if len(results) > 0 && results[0].SessionID != "session-a" {
		t.Errorf("SessionID = %q, want session-a", results[0].SessionID)
	}
}

// TestInMemoryStore_Search_WithRoleFilter 验证 role filter
func TestInMemoryStore_Search_WithRoleFilter(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	_ = store.Add(ctx, MustEpisode("s1", "user", "What is Go?"))
	_ = store.Add(ctx, MustEpisode("s1", "assistant", "Go is a programming language"))

	results, _ := store.Search(ctx, "Go", &SearchOptions{RoleFilter: "assistant", Limit: 10})
	if len(results) != 1 {
		t.Errorf("RoleFilter: got %d, want 1", len(results))
	}
	if len(results) > 0 && results[0].Role != "assistant" {
		t.Errorf("Role = %q, want assistant", results[0].Role)
	}
}

// TestInMemoryStore_Search_WithLimit 验证 limit
func TestInMemoryStore_Search_WithLimit(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = store.Add(ctx, MustEpisode("s1", "user", fmt.Sprintf("test message %d about search", i)))
	}

	results, _ := store.Search(ctx, "search", &SearchOptions{Limit: 3})
	if len(results) != 3 {
		t.Errorf("Limit: got %d, want 3", len(results))
	}
}

// TestInMemoryStore_Search_SummaryIndex 验证 Summary 也被索引
func TestInMemoryStore_Search_SummaryIndex(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	ep := MustEpisode("s1", "user", "Random content here")
	ep.Summary = "This is about kubernetes deployment"
	_ = store.Add(ctx, ep)

	results, _ := store.Search(ctx, "kubernetes", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("Summary index: got %d, want 1", len(results))
	}
}

// TestInMemoryStore_Search_TopicsIndex 验证 Topics 也被索引
func TestInMemoryStore_Search_TopicsIndex(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	ep := MustEpisode("s1", "user", "Random content here")
	ep.Topics = "kubernetes,devops"
	_ = store.Add(ctx, ep)

	results, _ := store.Search(ctx, "devops", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("Topics index: got %d, want 1", len(results))
	}
}

// TestInMemoryStore_Delete_RemovesFromIndex 验证 Delete 时索引同步
func TestInMemoryStore_Delete_RemovesFromIndex(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	ep := MustEpisode("s1", "user", "unique_token_xyz")
	_ = store.Add(ctx, ep)

	// 确认能找到
	results, _ := store.Search(ctx, "unique_token_xyz", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Fatalf("before delete: got %d, want 1", len(results))
	}

	// 删除
	if err := store.Delete(ctx, ep.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 应该找不到了
	results, _ = store.Search(ctx, "unique_token_xyz", &SearchOptions{Limit: 10})
	if len(results) != 0 {
		t.Errorf("after delete: got %d, want 0", len(results))
	}
}

// TestInMemoryStore_UpdateSummary_RefreshesIndex 验证 UpdateSummary 时索引同步
func TestInMemoryStore_UpdateSummary_RefreshesIndex(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	ep := MustEpisode("s1", "user", "Content here")
	ep.Summary = "Old summary"
	_ = store.Add(ctx, ep)

	// 用旧 summary 搜索能找到
	results, _ := store.Search(ctx, "old", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("before update: got %d, want 1", len(results))
	}

	// 更新 summary
	if err := store.UpdateSummary(ctx, ep.ID, "New summary", ""); err != nil {
		t.Fatalf("UpdateSummary: %v", err)
	}

	// 旧 token 不应再匹配
	results, _ = store.Search(ctx, "old", &SearchOptions{Limit: 10})
	if len(results) != 0 {
		t.Errorf("after update (old token): got %d, want 0", len(results))
	}
	// 新 token 应能匹配
	results, _ = store.Search(ctx, "new", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("after update (new token): got %d, want 1", len(results))
	}
}

// TestInMemoryStore_AddBatch_IndexesAll 验证批量 add 全部进索引
func TestInMemoryStore_AddBatch_IndexesAll(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	episodes := []*Episode{
		MustEpisode("s1", "user", "first message about golang"),
		MustEpisode("s1", "user", "second message about golang"),
		MustEpisode("s1", "user", "third message about python"),
	}
	if err := store.AddBatch(ctx, episodes); err != nil {
		t.Fatalf("AddBatch: %v", err)
	}

	results, _ := store.Search(ctx, "golang", &SearchOptions{Limit: 10})
	if len(results) != 2 {
		t.Errorf("AddBatch index: got %d, want 2", len(results))
	}
	results, _ = store.Search(ctx, "python", &SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("AddBatch index (python): got %d, want 1", len(results))
	}
}

// TestInMemoryStore_ClearAll_ClearsIndex 验证 ClearAll 时索引同步
func TestInMemoryStore_ClearAll_ClearsIndex(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	_ = store.Add(ctx, MustEpisode("s1", "user", "content with token_xyz"))
	_ = store.Add(ctx, MustEpisode("s2", "user", "another token_xyz content"))

	if err := store.ClearAll(ctx, ""); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}

	results, _ := store.Search(ctx, "token_xyz", &SearchOptions{Limit: 10})
	if len(results) != 0 {
		t.Errorf("after ClearAll: got %d, want 0", len(results))
	}
}

// TestInMemoryStore_Concurrent_AddSearch 验证并发安全
func TestInMemoryStore_Concurrent_AddSearch(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	const writers = 5
	const perWriter = 50
	done := make(chan struct{}, writers)

	for w := 0; w < writers; w++ {
		w := w
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < perWriter; i++ {
				_ = store.Add(ctx, MustEpisode(fmt.Sprintf("s%d", w), "user", fmt.Sprintf("concurrent_token_%d_%d", w, i)))
			}
		}()
	}
	for w := 0; w < writers; w++ {
		<-done
	}

	// 并发搜索
	const readers = 10
	rdone := make(chan struct{}, readers)
	for r := 0; r < readers; r++ {
		go func() {
			defer func() { rdone <- struct{}{} }()
			results, err := store.Search(ctx, "concurrent_token", &SearchOptions{Limit: 100})
			if err != nil {
				t.Errorf("concurrent search: %v", err)
				return
			}
			if len(results) == 0 {
				t.Errorf("concurrent search returned 0 results")
			}
		}()
	}
	for r := 0; r < readers; r++ {
		<-rdone
	}
}

// BenchmarkInMemoryStore_Search_Indexed 验证索引搜索性能
func BenchmarkInMemoryStore_Search_Indexed(b *testing.B) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	// 预填充 1000 条数据
	for i := 0; i < 1000; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		sessionID := fmt.Sprintf("session-%d", i%10)
		ep := MustEpisode(sessionID, role, fmt.Sprintf("Agent 框架中的 ReAct 循环和工具调用机制，第 %d 次迭代", i))
		ep.Summary = fmt.Sprintf("关于 ReAct 循环的摘要 %d", i)
		ep.Topics = "react,tools,agent"
		_ = store.Add(ctx, ep)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = store.Search(ctx, "ReAct 工具", &SearchOptions{
			SessionID:  "session-0",
			RoleFilter: "assistant",
			Limit:      20,
		})
	}
}

// BenchmarkInMemoryStore_Add_IndexingCost 验证 Add 时索引构建开销
func BenchmarkInMemoryStore_Add_IndexingCost(b *testing.B) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.Add(ctx, MustEpisode("s1", "user", fmt.Sprintf("benchmark message %d with some content to index", i)))
	}
}

// BenchmarkInMemoryStore_Search_LargeDataset 在 5000 episode 的大数据集上验证索引收益
func BenchmarkInMemoryStore_Search_LargeDataset(b *testing.B) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	// 预填充 5000 条数据
	for i := 0; i < 5000; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		sessionID := fmt.Sprintf("session-%d", i%50)
		ep := MustEpisode(sessionID, role, fmt.Sprintf("Agent 框架中的 ReAct 循环和工具调用机制，第 %d 次迭代", i))
		ep.Summary = fmt.Sprintf("关于 ReAct 循环的摘要 %d", i)
		ep.Topics = "react,tools,agent"
		_ = store.Add(ctx, ep)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = store.Search(ctx, "ReAct 工具", &SearchOptions{
			SessionID:  "session-0",
			RoleFilter: "assistant",
			Limit:      20,
		})
	}
}

// scanBasedSearch 复刻原 InMemoryStore.Search 的全表扫描实现
// 用于 A/B 对比基准，证明倒排索引的收益
func scanBasedSearch(episodes map[string]*Episode, query string, opts *SearchOptions) []*Episode {
	if opts == nil {
		opts = &SearchOptions{Limit: 10}
	}
	lowerQuery := strings.ToLower(query)
	var candidates []*Episode
	for _, e := range episodes {
		if opts.SessionID != "" && e.SessionID != opts.SessionID {
			continue
		}
		if opts.RoleFilter != "" && e.Role != opts.RoleFilter {
			continue
		}
		if query != "" {
			if !strings.Contains(e.Content, query) && !strings.Contains(e.Summary, query) {
				continue
			}
		}
		candidates = append(candidates, e)
	}
	var results []*Episode
	if query != "" {
		for _, ep := range candidates {
			if strings.Contains(strings.ToLower(ep.Content), lowerQuery) ||
				strings.Contains(strings.ToLower(ep.Summary), lowerQuery) {
				results = append(results, ep)
			}
		}
	} else {
		results = candidates
	}
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results
}

// BenchmarkScanBasedSearch 对照基线：原全表扫描版（perf-v6 round 8 Task 2 对比）
func BenchmarkScanBasedSearch(b *testing.B) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	for i := 0; i < 1000; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		sessionID := fmt.Sprintf("session-%d", i%10)
		ep := MustEpisode(sessionID, role, fmt.Sprintf("Agent 框架中的 ReAct 循环和工具调用机制，第 %d 次迭代", i))
		ep.Summary = fmt.Sprintf("关于 ReAct 循环的摘要 %d", i)
		ep.Topics = "react,tools,agent"
		_ = store.Add(ctx, ep)
	}

	// 拿到底层 episodes（仅用于对比测试）
	type snapshot struct {
		mu       sync.RWMutex
		episodes map[string]*Episode
	}
	// 通过 reflection-free 接口拿不到 episodes map；
	// 改为：在测试里用 Search 路径对比，直接调用 indexed 路径。
	_ = store

	// 直接走全表扫描
	episodes := make(map[string]*Episode, 1000)
	for i := 0; i < 1000; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		sessionID := fmt.Sprintf("session-%d", i%10)
		ep := MustEpisode(sessionID, role, fmt.Sprintf("Agent 框架中的 ReAct 循环和工具调用机制，第 %d 次迭代", i))
		ep.Summary = fmt.Sprintf("关于 ReAct 循环的摘要 %d", i)
		ep.Topics = "react,tools,agent"
		episodes[ep.ID] = ep
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scanBasedSearch(episodes, "ReAct 工具", &SearchOptions{
			SessionID:  "session-0",
			RoleFilter: "assistant",
			Limit:      20,
		})
	}
}

// BenchmarkIndexedSearch 对比：新的索引版
func BenchmarkIndexedSearch(b *testing.B) {
	store := NewInMemoryStore()
	defer store.Close()
	ctx := context.Background()

	for i := 0; i < 1000; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		sessionID := fmt.Sprintf("session-%d", i%10)
		ep := MustEpisode(sessionID, role, fmt.Sprintf("Agent 框架中的 ReAct 循环和工具调用机制，第 %d 次迭代", i))
		ep.Summary = fmt.Sprintf("关于 ReAct 循环的摘要 %d", i)
		ep.Topics = "react,tools,agent"
		_ = store.Add(ctx, ep)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = store.Search(ctx, "ReAct 工具", &SearchOptions{
			SessionID:  "session-0",
			RoleFilter: "assistant",
			Limit:      20,
		})
	}
}
