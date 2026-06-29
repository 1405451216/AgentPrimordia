package agent

import (
	"sync"
	"testing"
)

// TestEstimateTokensCached_Basic 验证基本缓存命中。
func TestEstimateTokensCached_Basic(t *testing.T) {
	ClearTokenCache()
	// 40 个字符的 ASCII 文本：40/4 = 10 tokens
	text := "abcdefghijklmnopqrstuvwxyz1234567890abcd"
	expected := len(text) / 4
	tokens := estimateTokensCached(text)
	if tokens != expected {
		t.Errorf("期望 %d tokens，实际 %d", expected, tokens)
	}
	// 第二次调用应命中缓存
	tokens2 := estimateTokensCached(text)
	if tokens2 != tokens {
		t.Errorf("缓存未命中：%d != %d", tokens2, tokens)
	}
}

// TestEstimateTokensCached_Empty 验证空字符串返回 0。
func TestEstimateTokensCached_Empty(t *testing.T) {
	if got := estimateTokensCached(""); got != 0 {
		t.Errorf("空字符串应返回 0，实际 %d", got)
	}
}

// TestEstimateTokensCached_Different 验证不同文本产生不同结果。
func TestEstimateTokensCached_Different(t *testing.T) {
	ClearTokenCache()
	a := estimateTokensCached("short")
	b := estimateTokensCached("this is a much longer text than the previous one")
	if a >= b {
		t.Errorf("长文本 tokens 应大于短文本：a=%d b=%d", a, b)
	}
}

// TestEstimateTokensCached_Concurrent 验证并发安全。
func TestEstimateTokensCached_Concurrent(t *testing.T) {
	ClearTokenCache()
	const goroutines = 50
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// 50% 重复 + 50% 唯一
				text := "common"
				if j%2 == 0 {
					text = "common"
				} else {
					text = string(rune('a'+idx)) + string(rune('b'+j%26))
				}
				_ = estimateTokensCached(text)
			}
		}(i)
	}
	wg.Wait()
}

// TestClearTokenCache 验证缓存清理。
func TestClearTokenCache(t *testing.T) {
	estimateTokensCached("test1")
	estimateTokensCached("test2")
	ClearTokenCache()
	// 清理后重新调用应正常返回
	tokens := estimateTokensCached("test1")
	if tokens != 1 {
		t.Errorf("清理后调用失败：%d", tokens)
	}
}

// TestHashText_Stability 验证 hash 函数稳定性。
func TestHashText_Stability(t *testing.T) {
	a := hashText("hello")
	b := hashText("hello")
	if a != b {
		t.Errorf("相同输入应产生相同 hash：%d != %d", a, b)
	}
	c := hashText("world")
	if a == c {
		t.Errorf("不同输入不应产生相同 hash")
	}
}

// BenchmarkEstimateTokensCached 基准：缓存版 token 估算（混合短/长文本）
func BenchmarkEstimateTokensCached(b *testing.B) {
	// 模拟真实场景：50% 重复 + 50% 唯一
	texts := make([]string, 20)
	for i := range texts {
		if i%2 == 0 {
			texts[i] = "common user query that repeats across turns"
		} else {
			texts[i] = string(rune('a'+i%26)) + "unique content for this message that should not be cached after first lookup"
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = estimateTokensCached(texts[i%len(texts)])
	}
}

// BenchmarkEstimateTokens_Direct 对比：未缓存版本
func BenchmarkEstimateTokens_Direct(b *testing.B) {
	texts := make([]string, 20)
	for i := range texts {
		if i%2 == 0 {
			texts[i] = "common user query that repeats across turns"
		} else {
			texts[i] = string(rune('a'+i%26)) + "unique content for this message that should not be cached after first lookup"
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total := 0
		for _, t := range texts {
			total += len(t) / 4
		}
		_ = total
	}
}
