package planning

import (
	"testing"
	"time"
)

// TestTransition_ValidChain 验证合法转换链：pending→active→blocked→active→completed
func TestTransition_ValidChain(t *testing.T) {
	plan := &Plan{Goal: "测试目标", SubTasks: nil, CreatedAt: time.Now()}
	mp := NewManagedPlan(plan)

	if mp.State != PlanStatePending {
		t.Fatalf("初始状态应为 pending，实际 %s", mp.State)
	}

	// pending → active
	if err := mp.Transition(PlanStateActive, "开始执行"); err != nil {
		t.Fatalf("pending→active 应成功: %v", err)
	}
	if mp.State != PlanStateActive {
		t.Fatalf("状态应为 active，实际 %s", mp.State)
	}

	// active → blocked
	if err := mp.Transition(PlanStateBlocked, "等待外部依赖"); err != nil {
		t.Fatalf("active→blocked 应成功: %v", err)
	}
	if mp.State != PlanStateBlocked {
		t.Fatalf("状态应为 blocked，实际 %s", mp.State)
	}

	// blocked → active
	if err := mp.Transition(PlanStateActive, "依赖已满足"); err != nil {
		t.Fatalf("blocked→active 应成功: %v", err)
	}
	if mp.State != PlanStateActive {
		t.Fatalf("状态应为 active，实际 %s", mp.State)
	}

	// active → completed
	if err := mp.Transition(PlanStateCompleted, "所有子任务完成"); err != nil {
		t.Fatalf("active→completed 应成功: %v", err)
	}
	if mp.State != PlanStateCompleted {
		t.Fatalf("状态应为 completed，实际 %s", mp.State)
	}

	// 验证历史记录
	if len(mp.History) != 4 {
		t.Fatalf("历史记录应为 4 条，实际 %d", len(mp.History))
	}
	expected := []struct {
		from, to PlanState
	}{
		{PlanStatePending, PlanStateActive},
		{PlanStateActive, PlanStateBlocked},
		{PlanStateBlocked, PlanStateActive},
		{PlanStateActive, PlanStateCompleted},
	}
	for i, e := range expected {
		if mp.History[i].From != e.from || mp.History[i].To != e.to {
			t.Errorf("历史[%d] 应为 %s→%s，实际 %s→%s", i, e.from, e.to, mp.History[i].From, mp.History[i].To)
		}
	}
}

// TestTransition_TerminalStateRejects 验证终态拒绝后续转换
func TestTransition_TerminalStateRejects(t *testing.T) {
	tests := []struct {
		name     string
		to       PlanState
		rejectTo PlanState
	}{
		{"completed 拒绝转换", PlanStateCompleted, PlanStateActive},
		{"failed 拒绝转换", PlanStateFailed, PlanStateActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := NewManagedPlan(&Plan{Goal: "test"})
			_ = mp.Transition(PlanStateActive, "start")
			_ = mp.Transition(tt.to, "finish")

			err := mp.Transition(tt.rejectTo, "retry")
			if err == nil {
				t.Fatalf("终态 %s 应拒绝转换到 %s", tt.to, tt.rejectTo)
			}
		})
	}
}

// TestTransition_InvalidTransition 验证非法转换返回错误
func TestTransition_InvalidTransition(t *testing.T) {
	mp := NewManagedPlan(&Plan{Goal: "test"})

	// pending → blocked 非法（必须先到 active）
	if err := mp.Transition(PlanStateBlocked, "skip"); err == nil {
		t.Fatal("pending→blocked 应返回错误")
	}

	// pending → completed 非法
	if err := mp.Transition(PlanStateCompleted, "skip"); err == nil {
		t.Fatal("pending→completed 应返回错误")
	}

	// active → pending 非法（不能回退到 pending）
	_ = mp.Transition(PlanStateActive, "start")
	if err := mp.Transition(PlanStatePending, "back"); err == nil {
		t.Fatal("active→pending 应返回错误")
	}
}

// TestTransition_FailedFromPending 验证 pending→failed 合法
func TestTransition_FailedFromPending(t *testing.T) {
	mp := NewManagedPlan(&Plan{Goal: "test"})
	if err := mp.Transition(PlanStateFailed, "初始化失败"); err != nil {
		t.Fatalf("pending→failed 应成功: %v", err)
	}
	if !mp.IsTerminal() {
		t.Fatal("failed 应为终态")
	}
}

// TestIsTerminal 验证终态判定
func TestIsTerminal(t *testing.T) {
	mp := NewManagedPlan(&Plan{Goal: "test"})
	if mp.IsTerminal() {
		t.Fatal("pending 不应为终态")
	}
	_ = mp.Transition(PlanStateActive, "start")
	if mp.IsTerminal() {
		t.Fatal("active 不应为终态")
	}
	_ = mp.Transition(PlanStateCompleted, "done")
	if !mp.IsTerminal() {
		t.Fatal("completed 应为终态")
	}
}
