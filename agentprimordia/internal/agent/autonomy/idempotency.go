package autonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// idempotencyRecord 幂等执行记录
type idempotencyRecord struct {
	Result string
}

// IdempotencyGuard 幂等保护：防止崩溃恢复后重复执行产生副作用
type IdempotencyGuard struct {
	mu        sync.RWMutex
	records   map[string]idempotencyRecord
	goalIndex map[string][]string // goalID → 该目标生成的幂等键，支持精确 Reset
}

// NewIdempotencyGuard 创建幂等保护器
func NewIdempotencyGuard() *IdempotencyGuard {
	return &IdempotencyGuard{
		records:   make(map[string]idempotencyRecord),
		goalIndex: make(map[string][]string),
	}
}

// GenerateKey 生成幂等键（基于目标ID + 步骤ID + 尝试次数），并登记到目标索引
func (ig *IdempotencyGuard) GenerateKey(goalID string, stepID string, attempt int) string {
	raw := fmt.Sprintf("%s:%s:%d", goalID, stepID, attempt)
	h := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(h[:16])
	ig.mu.Lock()
	ig.goalIndex[goalID] = append(ig.goalIndex[goalID], key)
	ig.mu.Unlock()
	return key
}

// IsExecuted 检查指定键是否已执行
func (ig *IdempotencyGuard) IsExecuted(_ context.Context, key string) bool {
	ig.mu.RLock()
	defer ig.mu.RUnlock()
	_, ok := ig.records[key]
	return ok
}

// MarkExecuted 标记键已执行并缓存结果
func (ig *IdempotencyGuard) MarkExecuted(_ context.Context, key string, result string) {
	ig.mu.Lock()
	defer ig.mu.Unlock()
	ig.records[key] = idempotencyRecord{Result: result}
}

// GetCachedResult 获取已执行操作的缓存结果
func (ig *IdempotencyGuard) GetCachedResult(_ context.Context, key string) (string, bool) {
	ig.mu.RLock()
	defer ig.mu.RUnlock()
	rec, ok := ig.records[key]
	if !ok {
		return "", false
	}
	return rec.Result, true
}

// Reset 重置指定目标的所有幂等标记（目标重试时调用）。
// 通过 goalIndex 精确删除该目标生成的键，不影响其它目标。
func (ig *IdempotencyGuard) Reset(goalID string) {
	ig.mu.Lock()
	defer ig.mu.Unlock()
	for _, k := range ig.goalIndex[goalID] {
		delete(ig.records, k)
	}
	delete(ig.goalIndex, goalID)
}
