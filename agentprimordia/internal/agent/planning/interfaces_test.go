package planning

import "testing"

func TestPlanStateConstants(t *testing.T) {
	states := []PlanState{PlanStatePending, PlanStateActive, PlanStateBlocked, PlanStateCompleted, PlanStateFailed}
	seen := make(map[PlanState]bool)
	for _, s := range states {
		if s == "" {
			t.Error("plan state should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate state: %s", s)
		}
		seen[s] = true
	}
	if len(states) != 5 {
		t.Errorf("expected 5 states, got %d", len(states))
	}
}

func TestManagedPlanCreation(t *testing.T) {
	mp := &ManagedPlan{
		Plan:  &Plan{Goal: "test"},
		State: PlanStatePending,
	}
	if mp.State != PlanStatePending {
		t.Errorf("expected pending, got %s", mp.State)
	}
	if mp.Plan.Goal != "test" {
		t.Errorf("expected test goal, got %s", mp.Plan.Goal)
	}
}
