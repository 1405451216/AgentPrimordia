package autonomy

import (
	"context"
	"sync"
	"testing"
)

// TestIdempotencyKeyGeneration 验证幂等键生成
func TestIdempotencyKeyGeneration(t *testing.T) {
	ig := NewIdempotencyGuard()

	key1 := ig.GenerateKey("goal-1", "s1", 1)
	key2 := ig.GenerateKey("goal-1", "s1", 1)
	key3 := ig.GenerateKey("goal-1", "s1", 2) // 不同 attempt

	if key1 != key2 {
		t.Errorf("same inputs should produce same key: %q != %q", key1, key2)
	}
	if key1 == key3 {
		t.Error("different attempt should produce different key")
	}
}

// TestIdempotencyCheckAndMark 验证幂等检查与标记
func TestIdempotencyCheckAndMark(t *testing.T) {
	ig := NewIdempotencyGuard()
	ctx := context.Background()

	key := ig.GenerateKey("goal-1", "s1", 1)

	// 首次执行：未标记
	if ig.IsExecuted(ctx, key) {
		t.Error("key should not be marked initially")
	}

	// 标记已执行
	ig.MarkExecuted(ctx, key, "result-1")

	// 再次检查：已标记
	if !ig.IsExecuted(ctx, key) {
		t.Error("key should be marked after MarkExecuted")
	}

	// 获取缓存结果
	result, ok := ig.GetCachedResult(ctx, key)
	if !ok {
		t.Fatal("cached result should exist")
	}
	if result != "result-1" {
		t.Errorf("cached result = %q, want %q", result, "result-1")
	}
}

// TestIdempotencyPreventsDoubleExecution 验证防重复执行
func TestIdempotencyPreventsDoubleExecution(t *testing.T) {
	ig := NewIdempotencyGuard()
	ctx := context.Background()
	execCount := 0

	execute := func() string {
		key := ig.GenerateKey("goal-1", "s1", 1)
		if ig.IsExecuted(ctx, key) {
			r, _ := ig.GetCachedResult(ctx, key)
			return r
		}
		execCount++
		result := "executed"
		ig.MarkExecuted(ctx, key, result)
		return result
	}

	// 执行两次
	r1 := execute()
	r2 := execute()

	if execCount != 1 {
		t.Errorf("exec count = %d, want 1 (idempotent)", execCount)
	}
	if r1 != r2 {
		t.Errorf("results differ: %q != %q", r1, r2)
	}
}

// TestIdempotencyDifferentGoals 验证不同目标隔离
func TestIdempotencyDifferentGoals(t *testing.T) {
	ig := NewIdempotencyGuard()
	ctx := context.Background()

	key1 := ig.GenerateKey("goal-1", "s1", 1)
	key2 := ig.GenerateKey("goal-2", "s1", 1)

	ig.MarkExecuted(ctx, key1, "r1")

	if !ig.IsExecuted(ctx, key1) {
		t.Error("goal-1 key should be marked")
	}
	if ig.IsExecuted(ctx, key2) {
		t.Error("goal-2 key should not be marked")
	}
}

// TestIdempotencyReset 验证重置（目标重试时清除旧标记）
func TestIdempotencyReset(t *testing.T) {
	ig := NewIdempotencyGuard()
	ctx := context.Background()

	key := ig.GenerateKey("goal-1", "s1", 1)
	ig.MarkExecuted(ctx, key, "old")

	ig.Reset("goal-1")

	if ig.IsExecuted(ctx, key) {
		t.Error("key should be cleared after reset")
	}
}

// TestIdempotencyConcurrency 验证并发安全
func TestIdempotencyConcurrency(t *testing.T) {
	ig := NewIdempotencyGuard()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := ig.GenerateKey("goal-1", "s1", n)
			ig.MarkExecuted(ctx, key, "result")
			_ = ig.IsExecuted(ctx, key)
		}(i)
	}
	wg.Wait()
}
