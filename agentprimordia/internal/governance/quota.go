package governance

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// TokenBucket 是一个简单的令牌桶限流器，用于 QPS 控制。
//
// 设计：
//   - 桶容量 = capacity (满桶)
//   - 每秒自动补充 rate 个令牌
//   - Take(1) 非阻塞尝试消费 1 个令牌；成功返回 true
//
// 内部使用 float64 跟踪令牌数以支持亚秒级精确补充。
type TokenBucket struct {
	capacity   float64      // 桶容量
	rate       float64      // 每秒补充速率
	tokens     atomic.Int64 // 当前令牌数 * 1e9（固定精度整数，避免浮点竞态）
	lastRefill atomic.Int64 // 上次补充时间的 unix 纳秒
	mu         sync.Mutex
}

const tokenPrecision = 1e9 // tokens 值的精度乘数

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

// refill 根据经过时间补充令牌。调用者必须持有写锁。
func (b *TokenBucket) refill() {
	now := time.Now().UnixNano()
	last := b.lastRefill.Load()
	elapsed := now - last
	if elapsed <= 0 {
		return
	}
	// 应补充 = 经过秒数 * 速率
	elapsedSeconds := float64(elapsed) / 1e9
	refillCount := elapsedSeconds * b.rate
	if refillCount <= 0 {
		return
	}
	refillUnits := int64(refillCount * tokenPrecision)
	if refillUnits <= 0 {
		return
	}
	newTokens := b.tokens.Load() + refillUnits
	maxTokens := int64(b.capacity * tokenPrecision)
	if newTokens > maxTokens {
		newTokens = maxTokens
	}
	b.tokens.Store(newTokens)
	b.lastRefill.Store(now)
}

// --- QuotaManager ---

// QuotaManager 跟踪单个租户的配额使用情况。
//
// 线程安全：所有字段通过内部锁或原子操作保护。
type QuotaManager struct {
	tenantID string
	quotas   TenantQuota

	// QPS 令牌桶
	bucket *TokenBucket

	// 每日 Token 用量
	dayTokens atomic.Int64
	// 当前计数器的日期键
	dayKey atomic.Value // string

	// 当前已用资源计数
	agentCount   atomic.Int64
	sessionCount atomic.Int64

	mu sync.RWMutex
}

// dailyKey 返回 UTC 当天的日期字符串。
func dailyKey() string {
	return time.Now().UTC().Format("2006-01-02")
}

// NewQuotaManager 为指定租户创建 QuotaManager。
func NewQuotaManager(tenantID string, quotas TenantQuota) *QuotaManager {
	qm := &QuotaManager{
		tenantID: tenantID,
		quotas:   quotas,
	}
	qm.bucket = NewTokenBucket(quotas.MaxQPS, quotas.MaxQPS)
	qm.dayKey.Store(dailyKey())
	return qm
}

// CheckQPS 检查是否允许一次调用（QPS 限流）。
func (q *QuotaManager) CheckQPS() error {
	if q.quotas.MaxQPS <= 0 {
		return nil // 无限制
	}
	if !q.bucket.Take(1) {
		return fmt.Errorf("%w: QPS limit %d exceeded", ErrQuotaExceeded, q.quotas.MaxQPS)
	}
	return nil
}

// RecordTokens 记录 Token 消耗。如果超出日配额返回 ErrQuotaExceeded。
func (q *QuotaManager) RecordTokens(n int64) error {
	if n <= 0 {
		return nil
	}
	if q.quotas.MaxTokensPerDay <= 0 {
		return nil // 无限制
	}

	// 检查是否需要重置日计数器（跨天）
	key := dailyKey()
	storedKey, _ := q.dayKey.Load().(string)
	if storedKey != key {
		q.mu.Lock()
		// 双重检查
		storedKey2, _ := q.dayKey.Load().(string)
		if storedKey2 != key {
			q.dayTokens.Store(0)
			q.dayKey.Store(key)
		}
		q.mu.Unlock()
	}

	// 先检查再增加（避免超额）
	current := q.dayTokens.Load()
	if current+n > q.quotas.MaxTokensPerDay {
		return fmt.Errorf("%w: daily token quota %d exceeded (used=%d, add=%d)",
			ErrQuotaExceeded, q.quotas.MaxTokensPerDay, current, n)
	}

	newVal := q.dayTokens.Add(n)
	if newVal > q.quotas.MaxTokensPerDay {
		// 超额了，回滚并返回错误
		q.dayTokens.Add(-n)
		return fmt.Errorf("%w: daily token quota %d exceeded", ErrQuotaExceeded, q.quotas.MaxTokensPerDay)
	}

	return nil
}

// DailyTokensUsed 返回当日已用 Token 数。
func (q *QuotaManager) DailyTokensUsed() int64 {
	// 检查是否跨天
	key := dailyKey()
	storedKey, _ := q.dayKey.Load().(string)
	if storedKey != key {
		return 0
	}
	return q.dayTokens.Load()
}

// CheckAgentCount 检查是否还能创建新 Agent。
func (q *QuotaManager) CheckAgentCount() error {
	if q.quotas.MaxAgents <= 0 {
		return nil
	}
	if int(q.agentCount.Load()) >= q.quotas.MaxAgents {
		return fmt.Errorf("%w: agent limit %d exceeded", ErrQuotaExceeded, q.quotas.MaxAgents)
	}
	return nil
}

// IncrementAgentCount 增加 Agent 计数。
func (q *QuotaManager) IncrementAgentCount() {
	q.agentCount.Add(1)
}

// DecrementAgentCount 减少 Agent 计数。
func (q *QuotaManager) DecrementAgentCount() {
	q.agentCount.Add(-1)
}

// CheckSessionCount 检查是否还能创建新 Session。
func (q *QuotaManager) CheckSessionCount() error {
	if q.quotas.MaxSessions <= 0 {
		return nil
	}
	if int(q.sessionCount.Load()) >= q.quotas.MaxSessions {
		return fmt.Errorf("%w: session limit %d exceeded", ErrQuotaExceeded, q.quotas.MaxSessions)
	}
	return nil
}

// IncrementSessionCount 增加 Session 计数。
func (q *QuotaManager) IncrementSessionCount() {
	q.sessionCount.Add(1)
}

// DecrementSessionCount 减少 Session 计数。
func (q *QuotaManager) DecrementSessionCount() {
	q.sessionCount.Add(-1)
}

// QuotaStatus 返回当前配额使用快照。
type QuotaStatus struct {
	TenantID        string
	Plan            TenantPlan
	DailyTokensUsed int64
	MaxTokensPerDay int64
	AgentCount      int
	MaxAgents       int
	SessionCount    int
	MaxSessions     int
	MaxQPS          int
}

// Status 返回配额状态快照。
func (q *QuotaManager) Status() QuotaStatus {
	return QuotaStatus{
		TenantID:        q.tenantID,
		Plan:            PlanFree,
		DailyTokensUsed: q.DailyTokensUsed(),
		MaxTokensPerDay: q.quotas.MaxTokensPerDay,
		AgentCount:      int(q.agentCount.Load()),
		MaxAgents:       q.quotas.MaxAgents,
		SessionCount:    int(q.sessionCount.Load()),
		MaxSessions:     q.quotas.MaxSessions,
		MaxQPS:          q.quotas.MaxQPS,
	}
}
