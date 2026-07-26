package governance

import (
	"testing"
	"time"
)

func TestTokenBucket_Basic(t *testing.T) {
	b := NewTokenBucket(2, 2)

	if !b.Take(1) {
		t.Error("first Take should succeed")
	}
	if !b.Take(1) {
		t.Error("second Take should succeed")
	}
	if b.Take(1) {
		t.Error("third Take should fail (bucket empty)")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	b := NewTokenBucket(10, 10)

	for i := 0; i < 10; i++ {
		b.Take(1)
	}
	if b.Take(1) {
		t.Error("should fail after draining")
	}

	time.Sleep(150 * time.Millisecond)
	if !b.Take(1) {
		t.Error("refill should have occurred after 150ms")
	}
}

func TestTokenBucket_NoLimit(t *testing.T) {
	b := NewTokenBucket(0, 0)
	for i := 0; i < 1000; i++ {
		if !b.Take(1) {
			t.Errorf("no-limit bucket: Take %d should succeed", i)
		}
	}
}

func TestTokenBucket_CapacityCapped(t *testing.T) {
	// 使用高速率使得 100ms 内积累足够多的令牌
	b := NewTokenBucket(1000, 5)
	// 清空
	for i := 0; i < 5; i++ {
		b.Take(1)
	}
	// 等待补充；rate=1000 意味着 100ms 补充 100 个令牌，但容量上限为 5
	time.Sleep(100 * time.Millisecond)
	// 补充后最多 capacity=5 个
	for i := 0; i < 5; i++ {
		if !b.Take(1) {
			t.Errorf("Take %d should succeed after refill", i+1)
		}
	}
	if b.Take(1) {
		t.Error("should fail: capacity capped at burst")
	}
}

func TestNewQuotaManager(t *testing.T) {
	qm := NewQuotaManager("t_test", DefaultQuota(PlanFree))
	if qm == nil {
		t.Fatal("NewQuotaManager returned nil")
	}
	if qm.tenantID != "t_test" {
		t.Errorf("tenantID = %q, want t_test", qm.tenantID)
	}
}

func TestQuotaManager_CheckQPS(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxQPS: 3})

	for i := 0; i < 3; i++ {
		if err := qm.CheckQPS(); err != nil {
			t.Errorf("CheckQPS #%d error: %v", i+1, err)
		}
	}
	if err := qm.CheckQPS(); err == nil {
		t.Error("CheckQPS should fail after exceeding QPS limit")
	}
}

func TestQuotaManager_CheckQPS_Unlimited(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxQPS: 0})

	for i := 0; i < 100; i++ {
		if err := qm.CheckQPS(); err != nil {
			t.Errorf("unlimited QPS: CheckQPS #%d error: %v", i, err)
		}
	}
}

func TestQuotaManager_RecordTokens(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxTokensPerDay: 1000})

	if err := qm.RecordTokens(500); err != nil {
		t.Errorf("RecordTokens(500) error: %v", err)
	}
	if qm.DailyTokensUsed() != 500 {
		t.Errorf("DailyTokensUsed = %d, want 500", qm.DailyTokensUsed())
	}

	if err := qm.RecordTokens(400); err != nil {
		t.Errorf("RecordTokens(400) error: %v", err)
	}
	if qm.DailyTokensUsed() != 900 {
		t.Errorf("DailyTokensUsed = %d, want 900", qm.DailyTokensUsed())
	}
}

func TestQuotaManager_RecordTokens_Exceed(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxTokensPerDay: 1000})

	if err := qm.RecordTokens(1001); err == nil {
		t.Error("RecordTokens(1001) should fail (exceeds quota 1000)")
	}
}

func TestQuotaManager_RecordTokens_Boundary(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxTokensPerDay: 1000})

	if err := qm.RecordTokens(1000); err != nil {
		t.Errorf("RecordTokens(1000) error: %v", err)
	}
	if err := qm.RecordTokens(1); err == nil {
		t.Error("RecordTokens(1) should fail after quota exhausted")
	}
}

func TestQuotaManager_RecordTokens_Unlimited(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxTokensPerDay: 0})

	if err := qm.RecordTokens(1000000); err != nil {
		t.Errorf("unlimited tokens: error: %v", err)
	}
}

func TestQuotaManager_CheckAgentCount(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxAgents: 2})

	if err := qm.CheckAgentCount(); err != nil {
		t.Errorf("CheckAgentCount error: %v", err)
	}
	qm.IncrementAgentCount()

	if err := qm.CheckAgentCount(); err != nil {
		t.Errorf("CheckAgentCount error after 1 agent: %v", err)
	}
	qm.IncrementAgentCount()

	if err := qm.CheckAgentCount(); err == nil {
		t.Error("CheckAgentCount should fail at MaxAgents")
	}
}

func TestQuotaManager_AgentCountDecrement(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxAgents: 1})
	qm.IncrementAgentCount()

	if err := qm.CheckAgentCount(); err == nil {
		t.Error("should be at limit")
	}
	qm.DecrementAgentCount()
	if err := qm.CheckAgentCount(); err != nil {
		t.Errorf("after decrement: %v", err)
	}
}

func TestQuotaManager_CheckSessionCount(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxSessions: 1})

	if err := qm.CheckSessionCount(); err != nil {
		t.Errorf("CheckSessionCount error: %v", err)
	}
	qm.IncrementSessionCount()

	if err := qm.CheckSessionCount(); err == nil {
		t.Error("CheckSessionCount should fail at MaxSessions")
	}
}

func TestQuotaManager_Status(t *testing.T) {
	quotas := TenantQuota{
		MaxAgents:       5,
		MaxSessions:     10,
		MaxTokensPerDay: 10000,
		MaxStorageGB:    5,
		MaxQPS:          20,
	}
	qm := NewQuotaManager("t_test", quotas)
	qm.IncrementAgentCount()
	qm.IncrementSessionCount()
	qm.RecordTokens(500)

	status := qm.Status()
	if status.TenantID != "t_test" {
		t.Errorf("Status.TenantID = %q, want t_test", status.TenantID)
	}
	if status.AgentCount != 1 {
		t.Errorf("Status.AgentCount = %d, want 1", status.AgentCount)
	}
	if status.SessionCount != 1 {
		t.Errorf("Status.SessionCount = %d, want 1", status.SessionCount)
	}
	if status.DailyTokensUsed != 500 {
		t.Errorf("Status.DailyTokensUsed = %d, want 500", status.DailyTokensUsed)
	}
}

func TestQuotaManager_ConcurrentAccess(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxTokensPerDay: 1000000})

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				qm.RecordTokens(10)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	used := qm.DailyTokensUsed()
	if used != 100000 {
		t.Errorf("concurrent DailyTokensUsed = %d, want 100000", used)
	}
}

func TestDailyKey(t *testing.T) {
	k1 := dailyKey()
	time.Sleep(1 * time.Millisecond)
	k2 := dailyKey()
	if k1 != k2 {
		t.Errorf("dailyKey should be same within same day: %q vs %q", k1, k2)
	}
	if len(k1) != 10 {
		t.Errorf("dailyKey length = %d, want 10", len(k1))
	}
}

// === 补充：DecrementSessionCount 边界条件测试 ===

func TestQuotaManager_DecrementSessionCount(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxSessions: 5})
	qm.IncrementSessionCount()
	qm.IncrementSessionCount()

	if qm.Status().SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", qm.Status().SessionCount)
	}

	qm.DecrementSessionCount()
	if qm.Status().SessionCount != 1 {
		t.Errorf("after decrement: SessionCount = %d, want 1", qm.Status().SessionCount)
	}

	qm.DecrementSessionCount()
	if qm.Status().SessionCount != 0 {
		t.Errorf("after second decrement: SessionCount = %d, want 0", qm.Status().SessionCount)
	}
}

func TestQuotaManager_DecrementSessionCount_GoesNegative(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxSessions: 5})
	// 未 increment 就 decrement，atomic 允许负数（这是设计上的 trade-off）
	qm.DecrementSessionCount()
	status := qm.Status()
	// 验证 decrement 确实减少了计数
	if status.SessionCount != -1 {
		t.Errorf("SessionCount = %d, want -1 (atomic decrement)", status.SessionCount)
	}
}

func TestQuotaManager_SessionCountAtLimit(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxSessions: 2})
	qm.IncrementSessionCount()
	qm.IncrementSessionCount()

	if err := qm.CheckSessionCount(); err == nil {
		t.Error("CheckSessionCount should fail at MaxSessions")
	}

	// Decrement 后应恢复
	qm.DecrementSessionCount()
	if err := qm.CheckSessionCount(); err != nil {
		t.Errorf("after decrement, CheckSessionCount error: %v", err)
	}
}

func TestQuotaManager_CheckSessionCount_Unlimited(t *testing.T) {
	qm := NewQuotaManager("t_test", TenantQuota{MaxSessions: 0})
	for i := 0; i < 100; i++ {
		qm.IncrementSessionCount()
	}
	if err := qm.CheckSessionCount(); err != nil {
		t.Errorf("unlimited sessions: CheckSessionCount error: %v", err)
	}
}
