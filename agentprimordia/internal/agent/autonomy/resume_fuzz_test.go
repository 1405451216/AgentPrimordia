package autonomy

import (
	"context"
	"math/rand"
	"testing"
)

// FuzzResumeConsistency 随机中断点 fuzz：验证任意中断后恢复的状态一致性
func FuzzResumeConsistency(f *testing.F) {
	// 种子：不同步骤数和中断点
	f.Add(3, 1)
	f.Add(5, 2)
	f.Add(10, 5)
	f.Add(1, 0)
	f.Add(7, 6)

	f.Fuzz(func(t *testing.T, numSteps int, interruptAfter int) {
		// 约束参数范围
		if numSteps < 1 {
			numSteps = 1
		}
		if numSteps > 20 {
			numSteps = 20
		}
		if interruptAfter < 0 {
			interruptAfter = 0
		}
		if interruptAfter >= numSteps {
			interruptAfter = numSteps - 1
		}

		ctx := context.Background()
		store := newMockCheckpointStore()
		rm := NewResumeManager(store)

		// 构建链式依赖计划
		steps := make([]PlanStep, numSteps)
		for i := 0; i < numSteps; i++ {
			step := PlanStep{
				ID:          stepID(i),
				Description: "步骤",
			}
			if i > 0 {
				step.DependsOn = []string{stepID(i - 1)}
			}
			steps[i] = step
		}
		plan := NewGoalPlan("fuzz-goal", steps)

		// 模拟执行到中断点
		for i := 0; i <= interruptAfter; i++ {
			plan.MarkStepCompleted(stepID(i))
		}

		// 保存检查点（模拟崩溃前最后写入）
		err := rm.SaveCheckpoint(ctx, "fuzz-goal", plan, GoalExecuting)
		if err != nil {
			t.Fatalf("save checkpoint: %v", err)
		}

		// 恢复检查点
		cp, err := rm.LoadCheckpoint(ctx, "fuzz-goal")
		if err != nil {
			t.Fatalf("load checkpoint: %v", err)
		}

		// 验证一致性
		if err := rm.ValidateConsistency(cp); err != nil {
			t.Fatalf("consistency validation failed: %v", err)
		}

		// 验证已完成步骤数正确
		completedCount := 0
		for _, s := range cp.PlanSnapshot.Steps {
			if s.Status == StepCompleted {
				completedCount++
			}
		}
		if completedCount != interruptAfter+1 {
			t.Errorf("completed = %d, want %d", completedCount, interruptAfter+1)
		}

		// 验证 lastCompletedStep 正确
		if cp.LastCompletedStep != stepID(interruptAfter) {
			t.Errorf("lastCompleted = %q, want %q", cp.LastCompletedStep, stepID(interruptAfter))
		}

		// 验证剩余步骤可继续执行
		remaining := cp.PlanSnapshot.RemainingSteps()
		expectedRemaining := numSteps - interruptAfter - 1
		if len(remaining) != expectedRemaining {
			t.Errorf("remaining = %d, want %d", len(remaining), expectedRemaining)
		}
	})
}

// FuzzStateMachineRandomTransitions 随机状态转换序列 fuzz
func FuzzStateMachineRandomTransitions(f *testing.F) {
	f.Add(int64(42))
	f.Add(int64(123))
	f.Add(int64(999))

	f.Fuzz(func(t *testing.T, seed int64) {
		rng := rand.New(rand.NewSource(seed))
		sm := NewStateMachine()
		state := GoalCreated

		allStates := []GoalState{GoalCreated, GoalPlanned, GoalExecuting, GoalValidated, GoalDone, GoalFailed}

		// 随机尝试 100 次转换
		for i := 0; i < 100; i++ {
			next := allStates[rng.Intn(len(allStates))]
			newState, err := sm.Apply(state, next)
			if err == nil {
				// 合法转换：状态应更新
				state = newState
			}
			// 非法转换：状态不变（Apply 返回原状态）

			// 不变量：终态不可再转换（除 failed→planned）
			if state == GoalDone {
				for _, s := range allStates {
					if _, err := sm.Apply(state, s); err == nil {
						t.Errorf("GoalDone should not transition to %s", s)
					}
				}
				break
			}
		}
	})
}

func stepID(i int) string {
	return "s" + string(rune('0'+i%10)) + string(rune('a'+i/10))
}
