package debugger

import (
	"testing"
)

// === Breakpoint 测试 ===

func TestBreakpoint_MatchCondition(t *testing.T) {
	bp := &Breakpoint{
		StepName: "test-step",
		Condition: func(state *AgentState) bool {
			return state.Turn == 3
		},
		Action: ActionPause,
	}

	// Turn 不满足条件
	state := &AgentState{Turn: 1}
	if bp.Match(state) {
		t.Error("expected breakpoint not to match at turn 1")
	}

	// Turn 满足条件
	state.Turn = 3
	if !bp.Match(state) {
		t.Error("expected breakpoint to match at turn 3")
	}
}

func TestBreakpoint_NilConditionAlwaysMatches(t *testing.T) {
	bp := &Breakpoint{
		StepName:  "always-match",
		Condition: nil, // nil condition = 总是匹配
		Action:    ActionPause,
	}

	state := &AgentState{Turn: 0}
	if !bp.Match(state) {
		t.Error("expected breakpoint with nil condition to always match")
	}
}

func TestBreakpoint_LogAction(t *testing.T) {
	bp := &Breakpoint{
		StepName:  "log-step",
		Condition: func(state *AgentState) bool { return true },
		Action:    ActionLog,
	}

	state := &AgentState{Turn: 1}
	if !bp.Match(state) {
		t.Error("expected breakpoint to match")
	}

	if bp.Action != ActionLog {
		t.Errorf("expected ActionLog, got %v", bp.Action)
	}
}

func TestBreakpointManager_AddAndCheck(t *testing.T) {
	mgr := NewBreakpointManager()

	bp := &Breakpoint{
		StepName: "check-turn-5",
		Condition: func(state *AgentState) bool {
			return state.Turn >= 5
		},
		Action: ActionPause,
	}

	mgr.Add(bp)

	// Turn 3 不应该触发
	state := &AgentState{Turn: 3}
	if mgr.Check(state) {
		t.Error("expected no breakpoint triggered at turn 3")
	}

	// Turn 5 应该触发
	state.Turn = 5
	if !mgr.Check(state) {
		t.Error("expected breakpoint triggered at turn 5")
	}
}

func TestBreakpointManager_Remove(t *testing.T) {
	mgr := NewBreakpointManager()

	bp := &Breakpoint{
		StepName:  "temp-bp",
		Condition: func(state *AgentState) bool { return true },
		Action:    ActionPause,
	}
	mgr.Add(bp)

	state := &AgentState{Turn: 1}
	if !mgr.Check(state) {
		t.Error("expected breakpoint to match before removal")
	}

	mgr.Remove("temp-bp")
	if mgr.Check(state) {
		t.Error("expected no breakpoint after removal")
	}
}

func TestBreakpointManager_Clear(t *testing.T) {
	mgr := NewBreakpointManager()

	mgr.Add(&Breakpoint{
		StepName:  "bp1",
		Condition: func(state *AgentState) bool { return true },
		Action:    ActionPause,
	})
	mgr.Add(&Breakpoint{
		StepName:  "bp2",
		Condition: func(state *AgentState) bool { return true },
		Action:    ActionLog,
	})

	state := &AgentState{Turn: 1}
	if !mgr.Check(state) {
		t.Error("expected breakpoint to match before clear")
	}

	mgr.Clear()
	if mgr.Check(state) {
		t.Error("expected no breakpoint after clear")
	}
}

// === WatchVar 测试 ===

func TestWatchVar_DetectChange(t *testing.T) {
	changed := false
	var oldVal, newVal any

	wv := &WatchVar{
		Name:      "turn-counter",
		Path:      "state.Turn",
		LastValue: 0,
		OnChange: func(old, new any) {
			changed = true
			oldVal = old
			newVal = new
		},
	}

	// 模拟值变化
	wv.Update(1)
	if !changed {
		t.Error("expected OnChange to be called")
	}
	if oldVal != 0 {
		t.Errorf("expected old value 0, got %v", oldVal)
	}
	if newVal != 1 {
		t.Errorf("expected new value 1, got %v", newVal)
	}
}

func TestWatchVar_NoChange(t *testing.T) {
	called := false
	wv := &WatchVar{
		Name:      "stable-var",
		Path:      "state.Name",
		LastValue: "hello",
		OnChange: func(old, new any) {
			called = true
		},
	}

	// 值未变化
	wv.Update("hello")
	if called {
		t.Error("expected OnChange not to be called when value unchanged")
	}
}

func TestWatchManager_AddAndPoll(t *testing.T) {
	mgr := NewWatchManager()

	turnVal := 0
	mgr.Add(&WatchVar{
		Name:      "turn",
		Path:      "state.Turn",
		LastValue: &turnVal,
		OnChange: func(old, new any) {
			turnVal = new.(int)
		},
	})

	// Poll 时传入新值
	mgr.Poll(&AgentState{Turn: 5})
	if turnVal != 5 {
		t.Errorf("expected turn to be 5, got %d", turnVal)
	}
}

// === TimeTravelDebugger 测试 ===

func TestTimeTravelDebugger_RecordAndRestore(t *testing.T) {
	tt := NewTimeTravelDebugger(100)

	// 记录3个 turn
	tt.Record(0, &AgentState{Turn: 0, Memory: &DebugMemorySnapshot{Latest: "state-0"}})
	tt.Record(1, &AgentState{Turn: 1, Memory: &DebugMemorySnapshot{Latest: "state-1"}})
	tt.Record(2, &AgentState{Turn: 2, Memory: &DebugMemorySnapshot{Latest: "state-2"}})

	// 恢复到 turn 1
	state, err := tt.Restore(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Turn != 1 {
		t.Errorf("expected turn 1, got %d", state.Turn)
	}
	if state.Memory.Latest != "state-1" {
		t.Errorf("expected memory 'state-1', got '%s'", state.Memory.Latest)
	}
}

func TestTimeTravelDebugger_StepForward(t *testing.T) {
	tt := NewTimeTravelDebugger(100)

	tt.Record(0, &AgentState{Turn: 0, Memory: &DebugMemorySnapshot{Latest: "s0"}})
	tt.Record(1, &AgentState{Turn: 1, Memory: &DebugMemorySnapshot{Latest: "s1"}})
	tt.Record(2, &AgentState{Turn: 2, Memory: &DebugMemorySnapshot{Latest: "s2"}})

	// 从 turn 0 开始
	_, _ = tt.Restore(0)

	// 前进一步
	state, err := tt.StepForward()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Turn != 1 {
		t.Errorf("expected turn 1, got %d", state.Turn)
	}

	// 再前进一步
	state, err = tt.StepForward()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Turn != 2 {
		t.Errorf("expected turn 2, got %d", state.Turn)
	}

	// 已到末尾
	_, err = tt.StepForward()
	if err == nil {
		t.Error("expected error when stepping past last snapshot")
	}
}

func TestTimeTravelDebugger_StepBackward(t *testing.T) {
	tt := NewTimeTravelDebugger(100)

	tt.Record(0, &AgentState{Turn: 0, Memory: &DebugMemorySnapshot{Latest: "s0"}})
	tt.Record(1, &AgentState{Turn: 1, Memory: &DebugMemorySnapshot{Latest: "s1"}})
	tt.Record(2, &AgentState{Turn: 2, Memory: &DebugMemorySnapshot{Latest: "s2"}})

	// 从 turn 2 开始
	_, _ = tt.Restore(2)

	// 后退一步
	state, err := tt.StepBackward()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Turn != 1 {
		t.Errorf("expected turn 1, got %d", state.Turn)
	}

	// 再退一步
	state, err = tt.StepBackward()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Turn != 0 {
		t.Errorf("expected turn 0, got %d", state.Turn)
	}

	// 已到开头
	_, err = tt.StepBackward()
	if err == nil {
		t.Error("expected error when stepping before first snapshot")
	}
}

func TestTimeTravelDebugger_RestoreNotFound(t *testing.T) {
	tt := NewTimeTravelDebugger(100)

	tt.Record(0, &AgentState{Turn: 0})

	_, err := tt.Restore(5)
	if err == nil {
		t.Error("expected error for non-existent turn")
	}
}

func TestTimeTravelDebugger_RecordLimit(t *testing.T) {
	tt := NewTimeTravelDebugger(3) // 最多3个快照

	tt.Record(0, &AgentState{Turn: 0})
	tt.Record(1, &AgentState{Turn: 1})
	tt.Record(2, &AgentState{Turn: 2})
	tt.Record(3, &AgentState{Turn: 3}) // 超出限制，应自动淘汰最旧

	snapshots := tt.GetSnapshots()
	if len(snapshots) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(snapshots))
	}

	// 应该是 turn 1, 2, 3
	if snapshots[0].Turn != 1 {
		t.Errorf("expected first snapshot turn 1, got %d", snapshots[0].Turn)
	}
}

func TestTimeTravelDebugger_GetCurrent(t *testing.T) {
	tt := NewTimeTravelDebugger(100)

	tt.Record(0, &AgentState{Turn: 0, Memory: &DebugMemorySnapshot{Latest: "s0"}})
	tt.Record(1, &AgentState{Turn: 1, Memory: &DebugMemorySnapshot{Latest: "s1"}})

	_, _ = tt.Restore(0)
	state := tt.GetCurrent()
	if state == nil {
		t.Fatal("expected current state")
	}
	if state.Turn != 0 {
		t.Errorf("expected turn 0, got %d", state.Turn)
	}
}
