// rate_limiter.go — 统一令牌桶限流器（v6.x 评估报告 §五.1 "重复实现大赏" 修复）
//
// 背景：仓库存在至少 3 份令牌桶实现——
//   - internal/llm/rate_limiter.go（tokenBucket，float64 + waitToken）
//   - internal/governance/quota.go（TokenBucket，atomic 精度 + Take）
//   - internal/resilience/（此前缺失）
//
// 本文件把 governance/quota.go 中更健壮的实现（固定精度整数 + atomic 计数）
// 提升为 canonical 版本，供 governance 与 llm 共同复用，消除重复。
//
// 迁移计划：
//   - governance.quota.go 改为 `type TokenBucket = resilience.TokenBucket`；
//   - llm/rate_limiter.go 的私有 tokenBucket 保留（API 含阻塞 waitToken，
//     与 canonical 的非阻塞 Take 互补），但在代码注释中指引 canonical 实现。

package resilience

import (
	"sync"
	"sync/atomic"
	"time"
)

// TokenBucket 令牌桶限流器（canonical 实现）。
//
// 设计：
//   - 桶容量 = capacity（满桶）
//   - 每秒自动补充 rate 个令牌
//   - Take(1) 非阻塞尝试消费 1 个令牌；成功返回 true
//
// 内部使用 float64 跟踪令牌数以支持亚秒级精确补充，
// 并以固定精度整数（tokenPrecision）存储避免浮点竞态。
type TokenBucket struct {
	capacity   float64      // 桶容量
	rate       float64      // 每秒补充速率
	tokens     atomic.Int64 // 当前令牌数 * tokenPrecision（固定精度整数，避免浮点竞态）
	lastRefill atomic.Int64 // 上次补充时间的 unix 纳秒
	mu         sync.Mutex
}

// tokenPrecision tokens 值的精度乘数。
const tokenPrecision = 1e9

// NewTokenBucket 创建一个令牌桶。
// rate 是每秒允许的请求数（QPS 上限），burst 是最大突发量。
// 如果 rate <= 0 则不限流。
func NewTokenBucket(rate, burst int) *TokenBucket {
	if rate <= 0 {
		rate = 1 << 30 // 近似无限
	}
	if burst <= 0 {
		burst = rate
	}
	tb := &TokenBucket{
		capacity: float64(burst),
		rate:     float64(rate),
	}
	tb.tokens.Store(int64(float64(burst) * tokenPrecision))
	tb.lastRefill.Store(time.Now().UnixNano())
	return tb
}

// Take 尝试消费 n 个令牌。成功返回 true。
func (b *TokenBucket) Take(n int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refill()

	needed := n * tokenPrecision
	if b.tokens.Load() >= needed {
		b.tokens.Add(-needed)
		return true
	}
	return false
}

// TakeOne 尝试消费 1 个令牌的便捷方法。
func (b *TokenBucket) TakeOne() bool {
	return b.Take(1)
}

// refill 根据经过时间补充令牌。调用者必须持有写锁。
func (b *TokenBucket) refill() {
	now := time.Now().UnixNano()
	last := b.lastRefill.Load()
	elapsed := now - last
	if elapsed <= 0 {
		return
	}
	// 应补充 = 经过秒数 * 速率
	add := float64(elapsed) * b.rate / 1e9
	b.lastRefill.Store(now)
	if add <= 0 {
		return
	}
	// 固定精度整数累加，避免浮点溢出
	cur := b.tokens.Load()
	next := cur + int64(add*tokenPrecision)
	maxTokens := int64(b.capacity * tokenPrecision)
	if next > maxTokens {
		next = maxTokens
	}
	b.tokens.Store(next)
}

// AvailableTokens 返回当前可用令牌数（含小数）。
func (b *TokenBucket) AvailableTokens() float64 {
	return float64(b.tokens.Load()) / tokenPrecision
}
