// goal_budget_pause_test.go — v5.1 调度质量：预算超限自动暂停/恢复策略回归测试
//
// 验收（V6-ROADMAP §三 任务 4）：预算护栏回归测试全绿。
// 行为定义：
//  1. Charge 触发预算超限 → 返回 ErrGoalBudgetExceeded 且目标自动转入 GoalPaused
//  2. AddBudget 追加预算后 Resume → 目标回到 GoalExecuting，Charge 恢复成功
//  3. 预算未追加时 Resume → 拒绝恢复（ErrGoalBudgetExceeded）
//  4. 非暂停态 Resume → ErrGoalNotPaused
//  5. 自动暂停幂等：重复 Charge 不重复转换、不报状态机错误
package autonomy

import (
	"errors"
	"testing"
	"time"
)

func newTestGoal(t *testing.T, budget float64) *AgentGoal {
	t.Helper()
	g := NewAgentGoal("budget-pause-test", GoalConfig{
		Priority:   PriorityNormal,
		MaxRetries: 2,
	})
	if err := g.TransitionTo(GoalPlanned); err != nil {
		t.Fatalf("转 Planned 失败: %v", err)
	}
	if err := g.TransitionTo(GoalExecuting); err != nil {
		t.Fatalf("转 Executing 失败: %v", err)
	}
	g.SetBudget(budget)
	return g
}

func TestGoalAutoPauseOnBudgetExceeded(t *testing.T) {
	g := newTestGoal(t, 1.0)

	if err := g.Charge(0.6); err != nil {
		t.Fatalf("首次记账应成功: %v", err)
	}
	err := g.Charge(0.6) // 0.6+0.6 > 1.0 → 超限
	if !errors.Is(err, ErrGoalBudgetExceeded) {
		t.Fatalf("期望 ErrGoalBudgetExceeded，得到 %v", err)
	}
	if got := g.Snapshot().State; got != GoalPaused {
		t.Fatalf("预算超限后目标应自动暂停，得到 %s", got)
	}
}

func TestGoalResumeAfterBudgetTopUp(t *testing.T) {
	// 场景 A：额度用满后暂停（spent == budget）——未追加预算时恢复被拒绝
	g := newTestGoal(t, 1.0)
	if err := g.Charge(1.0); err != nil {
		t.Fatalf("恰好用满预算的记账应成功: %v", err)
	}
	if err := g.Charge(0.1); !errors.Is(err, ErrGoalBudgetExceeded) {
		t.Fatalf("期望超限错误，得到 %v", err)
	}
	if g.Snapshot().State != GoalPaused {
		t.Fatal("前置条件：应处于 Paused")
	}
	if err := g.Resume(); !errors.Is(err, ErrGoalBudgetExceeded) {
		t.Fatalf("预算未追加时 Resume 应拒绝，得到 %v", err)
	}
	if err := g.AddBudget(1.0); err != nil {
		t.Fatalf("追加预算失败: %v", err)
	}
	if err := g.Resume(); err != nil {
		t.Fatalf("追加预算后 Resume 应成功: %v", err)
	}
	if got := g.Snapshot().State; got != GoalExecuting {
		t.Fatalf("恢复后应回到 Executing，得到 %s", got)
	}
	if err := g.Charge(0.5); err != nil {
		t.Fatalf("恢复后 Charge 应成功: %v", err)
	}

	// 场景 B：仍有剩余额度时暂停（某次大额记账触发）——Resume 允许恢复
	g2 := newTestGoal(t, 1.0)
	_ = g2.Charge(0.6)
	_ = g2.Charge(0.6) // 0.6+0.6 > 1.0 → 暂停
	if g2.Snapshot().State != GoalPaused {
		t.Fatal("前置条件：g2 应处于 Paused")
	}
	if err := g2.Resume(); err != nil {
		t.Fatalf("有剩余额度时 Resume 应成功: %v", err)
	}
}

func TestGoalPausedGateBlocksCharge(t *testing.T) {
	// 暂停闸门：Paused 态即使有剩余额度也拒绝记账
	g := newTestGoal(t, 10.0)
	_ = g.Charge(9.5)
	if err := g.Charge(1.0); !errors.Is(err, ErrGoalBudgetExceeded) {
		t.Fatalf("期望超限暂停，得到 %v", err)
	}
	if err := g.Charge(0.1); !errors.Is(err, ErrGoalBudgetExceeded) {
		t.Fatalf("暂停态下有额度也应拒绝记账，得到 %v", err)
	}
	if spent := g.CostSpent(); spent != 9.5 {
		t.Errorf("暂停态不应产生新花费：%.2f", spent)
	}
}

func TestGoalResumeNotPaused(t *testing.T) {
	g := newTestGoal(t, 10.0)
	if err := g.Resume(); !errors.Is(err, ErrGoalNotPaused) {
		t.Fatalf("非暂停态 Resume 应返回 ErrGoalNotPaused，得到 %v", err)
	}
}

func TestGoalAutoPauseIdempotent(t *testing.T) {
	g := newTestGoal(t, 1.0)
	_ = g.Charge(2.0) // 超限 → 暂停
	first := g.Snapshot()

	// 重复 Charge：仍返回超限错误，但不重复转换/不报错
	for i := 0; i < 3; i++ {
		if err := g.Charge(0.1); !errors.Is(err, ErrGoalBudgetExceeded) {
			t.Fatalf("重复 Charge 应持续返回 ErrGoalBudgetExceeded，得到 %v", err)
		}
	}
	snap := g.Snapshot()
	if snap.State != GoalPaused {
		t.Fatalf("重复 Charge 后应保持 Paused，得到 %s", snap.State)
	}
	if !snap.UpdatedAt.Equal(first.UpdatedAt) {
		t.Error("幂等暂停不应刷新 UpdatedAt")
	}
}

func TestGoalPausedStateMachineTransitions(t *testing.T) {
	sm := NewStateMachine()

	// 合法：Executing→Paused、Planned→Paused、Paused→Executing、Paused→Failed
	for _, tr := range [][2]GoalState{
		{GoalExecuting, GoalPaused},
		{GoalPlanned, GoalPaused},
		{GoalPaused, GoalExecuting},
		{GoalPaused, GoalFailed},
	} {
		if err := sm.ValidateTransition(tr[0], tr[1]); err != nil {
			t.Errorf("转换 %s→%s 应合法: %v", tr[0], tr[1], err)
		}
	}
	// 非法：Created→Paused（未开始无预算可超）、Paused→Done（必须先恢复执行）
	for _, tr := range [][2]GoalState{
		{GoalCreated, GoalPaused},
		{GoalPaused, GoalDone},
	} {
		if err := sm.ValidateTransition(tr[0], tr[1]); err == nil {
			t.Errorf("转换 %s→%s 应非法", tr[0], tr[1])
		}
	}
}

func TestGoalAddBudgetValidation(t *testing.T) {
	g := newTestGoal(t, 1.0)
	before := g.Budget()
	if err := g.AddBudget(-0.5); err == nil {
		t.Error("负数追加应报错")
	}
	if err := g.AddBudget(0); err == nil {
		t.Error("零追加应报错")
	}
	if g.Budget() != before {
		t.Errorf("失败追加不应改变预算：%f → %f", before, g.Budget())
	}
}

// 并发安全冒烟：多 goroutine 同时 Charge，最终花费不超预算且无 panic
func TestGoalConcurrentChargeRespectsBudget(t *testing.T) {
	g := newTestGoal(t, 1.0)
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = g.Charge(0.05)
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	if g.CostSpent() > 1.0+1e-9 {
		t.Errorf("并发记账超出预算：%.4f", g.CostSpent())
	}
	_ = time.Now
}
