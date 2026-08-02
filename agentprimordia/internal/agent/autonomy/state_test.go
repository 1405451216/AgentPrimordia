package autonomy

import (
	"testing"
)

// TestGoalStateTransitions 验证合法状态转换
func TestGoalStateTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    GoalState
		to      GoalState
		wantErr bool
	}{
		{"created→planned", GoalCreated, GoalPlanned, false},
		{"planned→executing", GoalPlanned, GoalExecuting, false},
		{"executing→validated", GoalExecuting, GoalValidated, false},
		{"validated→done", GoalValidated, GoalDone, false},
		{"executing→failed", GoalExecuting, GoalFailed, false},
		{"planned→failed", GoalPlanned, GoalFailed, false},
		{"validated→executing (重规划)", GoalValidated, GoalExecuting, false},
		{"failed→planned (重试)", GoalFailed, GoalPlanned, false},
		// 非法转换
		{"created→executing (跳过planned)", GoalCreated, GoalExecuting, true},
		{"created→done", GoalCreated, GoalDone, true},
		{"done→executing", GoalDone, GoalExecuting, true},
		{"executing→planned (回退)", GoalExecuting, GoalPlanned, true},
		{"done→failed", GoalDone, GoalFailed, true},
	}

	sm := NewStateMachine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.ValidateTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition(%s, %s) error = %v, wantErr %v",
					tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

// TestStateMachineApply 验证状态应用与非法转换防护
func TestStateMachineApply(t *testing.T) {
	sm := NewStateMachine()

	state := GoalCreated
	newState, err := sm.Apply(state, GoalPlanned)
	if err != nil {
		t.Fatalf("Apply(created, planned) unexpected error: %v", err)
	}
	if newState != GoalPlanned {
		t.Errorf("Apply(created, planned) = %s, want %s", newState, GoalPlanned)
	}

	// 非法转换不改变状态
	_, err = sm.Apply(newState, GoalDone)
	if err == nil {
		t.Fatal("Apply(planned, done) expected error, got nil")
	}
}

// TestStateMachineEvents 验证状态变更事件发布
func TestStateMachineEvents(t *testing.T) {
	sm := NewStateMachine()
	var events []StateChangeEvent
	sm.OnTransition(func(e StateChangeEvent) {
		events = append(events, e)
	})

	_, _ = sm.Apply(GoalCreated, GoalPlanned)
	_, _ = sm.Apply(GoalPlanned, GoalExecuting)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].From != GoalCreated || events[0].To != GoalPlanned {
		t.Errorf("event[0] = %v, want created→planned", events[0])
	}
	if events[1].From != GoalPlanned || events[1].To != GoalExecuting {
		t.Errorf("event[1] = %v, want planned→executing", events[1])
	}
}

// TestGoalStateString 验证状态字符串表示
func TestGoalStateString(t *testing.T) {
	states := map[GoalState]string{
		GoalCreated:   "created",
		GoalPlanned:   "planned",
		GoalExecuting: "executing",
		GoalValidated: "validated",
		GoalDone:      "done",
		GoalFailed:    "failed",
	}
	for state, want := range states {
		if got := state.String(); got != want {
			t.Errorf("GoalState(%d).String() = %q, want %q", state, got, want)
		}
	}
}

// TestIsTerminal 验证终态判断
func TestIsTerminal(t *testing.T) {
	if !GoalDone.IsTerminal() {
		t.Error("GoalDone should be terminal")
	}
	if !GoalFailed.IsTerminal() {
		t.Error("GoalFailed should be terminal")
	}
	if GoalExecuting.IsTerminal() {
		t.Error("GoalExecuting should not be terminal")
	}
}
