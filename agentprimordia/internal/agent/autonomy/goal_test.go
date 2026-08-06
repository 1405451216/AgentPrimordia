package autonomy

import (
	"testing"
	"time"
)

// TestNewAgentGoal 验证目标创建
func TestNewAgentGoal(t *testing.T) {
	goal := NewAgentGoal("监控数据异常并修复", GoalConfig{
		AcceptanceCriteria: []string{"异常数据归零", "修复日志生成"},
		Priority:           PriorityHigh,
		MaxRetries:         3,
	})

	if goal.ID == "" {
		t.Error("goal ID should not be empty")
	}
	if goal.Description != "监控数据异常并修复" {
		t.Errorf("description = %q, want %q", goal.Description, "监控数据异常并修复")
	}
	if goal.State != GoalCreated {
		t.Errorf("initial state = %s, want created", goal.State)
	}
	if goal.Priority != PriorityHigh {
		t.Errorf("priority = %d, want %d", goal.Priority, PriorityHigh)
	}
	if len(goal.AcceptanceCriteria) != 2 {
		t.Errorf("acceptance criteria len = %d, want 2", len(goal.AcceptanceCriteria))
	}
	if goal.MaxRetries != 3 {
		t.Errorf("max retries = %d, want 3", goal.MaxRetries)
	}
	if goal.CreatedAt.IsZero() {
		t.Error("created at should not be zero")
	}
}

// TestGoalTransition 验证目标状态转换
func TestGoalTransition(t *testing.T) {
	goal := NewAgentGoal("测试目标", GoalConfig{})

	err := goal.TransitionTo(GoalPlanned)
	if err != nil {
		t.Fatalf("transition to planned: %v", err)
	}
	if goal.State != GoalPlanned {
		t.Errorf("state = %s, want planned", goal.State)
	}

	// 非法转换
	err = goal.TransitionTo(GoalDone)
	if err == nil {
		t.Fatal("expected error for illegal transition planned→done")
	}
	if goal.State != GoalPlanned {
		t.Errorf("state should remain planned after failed transition, got %s", goal.State)
	}
}

// TestGoalRetryCount 验证重试计数
func TestGoalRetryCount(t *testing.T) {
	goal := NewAgentGoal("重试目标", GoalConfig{MaxRetries: 2})

	if !goal.CanRetry() {
		t.Error("should be able to retry at count=0, max=2")
	}

	goal.IncrementRetry()
	if goal.RetryCount != 1 {
		t.Errorf("retry count = %d, want 1", goal.RetryCount)
	}
	if !goal.CanRetry() {
		t.Error("should be able to retry at count=1, max=2")
	}

	goal.IncrementRetry()
	if goal.RetryCount != 2 {
		t.Errorf("retry count = %d, want 2", goal.RetryCount)
	}
	if goal.CanRetry() {
		t.Error("should not be able to retry at count=2, max=2")
	}
}

// TestGoalMetadata 验证元数据存取
func TestGoalMetadata(t *testing.T) {
	goal := NewAgentGoal("元数据目标", GoalConfig{})

	goal.SetMetadata("source", "scheduler")
	goal.SetMetadata("node", "worker-1")

	if v := goal.GetMetadata("source"); v != "scheduler" {
		t.Errorf("metadata[source] = %q, want %q", v, "scheduler")
	}
	if v := goal.GetMetadata("node"); v != "worker-1" {
		t.Errorf("metadata[node] = %q, want %q", v, "worker-1")
	}
	if v := goal.GetMetadata("nonexist"); v != "" {
		t.Errorf("metadata[nonexist] = %q, want empty", v)
	}
}

// TestGoalHistory 验证状态历史记录
func TestGoalHistory(t *testing.T) {
	goal := NewAgentGoal("历史目标", GoalConfig{})

	_ = goal.TransitionTo(GoalPlanned)
	_ = goal.TransitionTo(GoalExecuting)

	history := goal.History()
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].From != GoalCreated || history[0].To != GoalPlanned {
		t.Errorf("history[0] = %+v, want created→planned", history[0])
	}
	if history[1].From != GoalPlanned || history[1].To != GoalExecuting {
		t.Errorf("history[1] = %+v, want planned→executing", history[1])
	}
}

// TestGoalDeadline 验证截止时间
func TestGoalDeadline(t *testing.T) {
	goal := NewAgentGoal("限时目标", GoalConfig{
		Deadline: time.Now().Add(-time.Hour), // 已过期
	})
	if !goal.IsExpired() {
		t.Error("goal with past deadline should be expired")
	}

	goal2 := NewAgentGoal("未来目标", GoalConfig{
		Deadline: time.Now().Add(time.Hour),
	})
	if goal2.IsExpired() {
		t.Error("goal with future deadline should not be expired")
	}

	goal3 := NewAgentGoal("无截止目标", GoalConfig{})
	if goal3.IsExpired() {
		t.Error("goal without deadline should not be expired")
	}
}

// TestDefaultPriority 验证默认优先级
func TestDefaultPriority(t *testing.T) {
	goal := NewAgentGoal("默认优先级", GoalConfig{})
	if goal.Priority != PriorityNormal {
		t.Errorf("default priority = %d, want %d (normal)", goal.Priority, PriorityNormal)
	}
}
