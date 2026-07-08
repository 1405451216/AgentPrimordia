package tokencache

import (
	"fmt"
	"sync"
	"testing"
)

// TestEstimateTokensCached_LargeText 验证长文本 token 估算。
func TestEstimateTokensCached_LargeText(t *testing.T) {
	ClearTokenCache()
	// 1000 个字符的文本
	text := ""
	for i := 0; i < 250; i++ {
		text += "abcd"
	}
	expected := len(text) / 4
	tokens := EstimateTokensCached(text)
	if tokens != expected {
		t.Errorf("期望 %d tokens，实际 %d", expected, tokens)
	}
}

// TestEstimateTokensCached_Eviction 验证缓存淘汰逻辑。
// 当条目超过 tokenCacheSize 时应触发 evictHalfTokenCache。
func TestEstimateTokensCached_Eviction(t *testing.T) {
	ClearTokenCache()

	// 填充超过上限的缓存条目
	for i := 0; i < tokenCacheSize+100; i++ {
		text := fmt.Sprintf("unique-text-for-eviction-test-%d", i)
		_ = EstimateTokensCached(text)
	}

	// 验证缓存仍然可用
	tokens := EstimateTokensCached("post-eviction-test")
	if tokens <= 0 {
		t.Errorf("淘汰后缓存应仍然可用，got=%d", tokens)
	}
}

// TestHashText_Empty 验证空字符串哈希不 panic。
func TestHashText_Empty(t *testing.T) {
	h := HashText("")
	if h == 0 {
		// FNV-1a 对空字符串返回非零值，但这里只验证不 panic
		t.Log("empty string hash:", h)
	}
}

// TestHashText_Distribution 验证不同输入产生不同哈希（无碰撞）。
func TestHashText_Distribution(t *testing.T) {
	t.Parallel()
	seen := make(map[uint32]string)
	collisions := 0
	for i := 0; i < 10000; i++ {
		text := fmt.Sprintf("text-%d", i)
		h := HashText(text)
		if existing, ok := seen[h]; ok && existing != text {
			collisions++
		}
		seen[h] = text
	}
	// 10000 个短字符串不应有碰撞
	if collisions > 0 {
		t.Errorf("期望 0 碰撞，实际 %d", collisions)
	}
}

// TestEstimateTokensCached_Consistency 验证同一文本多次调用结果一致。
func TestEstimateTokensCached_Consistency(t *testing.T) {
	ClearTokenCache()
	text := "consistency-test-text-1234567890"

	first := EstimateTokensCached(text)
	for i := 0; i < 10; i++ {
		result := EstimateTokensCached(text)
		if result != first {
			t.Errorf("第 %d 次调用结果不一致: first=%d, got=%d", i+1, first, result)
		}
	}
}

// TestClearTokenCache_Empty 验证对空缓存执行 Clear 不 panic。
func TestClearTokenCache_Empty(t *testing.T) {
	ClearTokenCache()
	ClearTokenCache() // 再次清理应无副作用
}

// TestEstimateTokensCached_ConcurrentEviction 验证并发场景下的淘汰安全性。
func TestEstimateTokensCached_ConcurrentEviction(t *testing.T) {
	ClearTokenCache()

	const goroutines = 20
	const perGoroutine = 300

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				text := fmt.Sprintf("concurrent-eviction-%d-%d", idx, j)
				_ = EstimateTokensCached(text)
			}
		}(i)
	}
	wg.Wait()

	// 验证缓存仍可用
	tokens := EstimateTokensCached("after-concurrent-eviction")
	if tokens <= 0 {
		t.Error("并发淘汰后缓存应仍然可用")
	}
}
