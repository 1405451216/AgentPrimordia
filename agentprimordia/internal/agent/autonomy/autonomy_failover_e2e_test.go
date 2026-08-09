//go:build e2e

// autonomy_failover_e2e_test.go — v4.5-1/3 分布式自治：目标跨节点迁移/续跑 + 故障域隔离
//
// 验收标准：
//   - v4.5-1 目标跨节点迁移/续跑：kill 执行节点后目标在另一节点自动续跑
//   - v4.5-3 故障域隔离：3 节点 kill 1，无人工干预自动续跑
//
// 场景：3 个 AutonomyRuntime 共享同一 CheckpointStore（模拟集群共享后端），
// 节点 A 执行目标至中途（已完成 collect/analyze 并落 checkpoint）→ 模拟崩溃
// （不再继续）→ 节点 B/C 启动时 ResumeIncomplete 自动接管续跑 → GoalDone。
//
// 运行方式：
//
//	go test -tags=e2e -run TestE2E_Autonomy_CrossNodeResume -v ./internal/agent/autonomy/
package autonomy

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// sharedCheckpointStore 跨节点共享的 CheckpointStore（模拟集群共享后端）。
type sharedCheckpointStore struct {
	mu          sync.Mutex
	checkpoints map[string]*Checkpoint
}

func newSharedCheckpointStore() *sharedCheckpointStore {
	return &sharedCheckpointStore{checkpoints: make(map[string]*Checkpoint)}
}

func (s *sharedCheckpointStore) SaveCheckpoint(_ context.Context, cp *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpoints[cp.GoalID] = cp
	return nil
}

func (s *sharedCheckpointStore) LoadCheckpoint(_ context.Context, goalID string) (*Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp, ok := s.checkpoints[goalID]
	if !ok {
		return nil, fmt.Errorf("checkpoint not found: %s", goalID)
	}
	return cp, nil
}

func (s *sharedCheckpointStore) ListIncomplete(_ context.Context) ([]*Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Checkpoint
	for _, cp := range s.checkpoints {
		if !cp.Completed {
			result = append(result, cp)
		}
	}
	return result, nil
}

// failoverExecutor 确定性步骤执行器（跨节点共享实例，模拟同一业务）。
type failoverExecutor struct{}

func (e *failoverExecutor) ExecuteStep(_ context.Context, step PlanStep) (string, error) {
	switch step.ID {
	case "collect":
		return "采集完成", nil
	case "analyze":
		return "根因定位", nil
	case "fix":
		return "修复完成", nil
	case "verify":
		return "验证通过", nil
	default:
		return "ok", nil
	}
}

// runGoalToStep 在节点上启动目标并执行到指定步骤（模拟执行中崩溃）。
func runGoalToStep(t *testing.T, store *sharedCheckpointStore, nodeName string, stopAt string) *AgentGoal {
	t.Helper()
	rt := NewAutonomyRuntime(RuntimeConfig{
		StepExecutor:    &failoverExecutor{},
		CheckpointStore: store,
	})
	ctx := context.Background()
	goal := rt.SubmitGoal("跨节点续跑目标", GoalConfig{MaxRetries: 2})
	plan := NewGoalPlan(goal.ID, []PlanStep{
		{ID: "collect", Description: "采集", Strategy: StepStrategySequential},
		{ID: "analyze", Description: "分析", DependsOn: []string{"collect"}, Strategy: StepStrategySequential},
		{ID: "fix", Description: "修复", DependsOn: []string{"analyze"}, Strategy: StepStrategySequential},
		{ID: "verify", Description: "验证", DependsOn: []string{"fix"}, Strategy: StepStrategySequential},
	})
	if err := rt.SetPlan(goal.ID, plan); err != nil {
		t.Fatalf("SetPlan: %v", err)
	}

	// 手动执行到 stopAt（含）为止，模拟节点执行至该步骤后崩溃
	for _, step := range plan.Steps {
		if _, err := (&failoverExecutor{}).ExecuteStep(ctx, step); err != nil {
			t.Fatalf("step %s: %v", step.ID, err)
		}
		plan.MarkStepCompleted(step.ID)
		if step.ID == stopAt {
			break
		}
	}
	// 崩溃前落 checkpoint（含目标描述，供接管节点重建目标）
	cp := &Checkpoint{
		GoalID:            goal.ID,
		GoalDescription:   goal.Description,
		State:             GoalExecuting,
		LastCompletedStep: stopAt,
		PlanSnapshot:      plan,
		Completed:         false,
	}
	if err := store.SaveCheckpoint(ctx, cp); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	t.Logf("节点 %s 执行至 %q 后崩溃（checkpoint 已落共享后端）", nodeName, stopAt)
	return goal
}

// TestE2E_Autonomy_CrossNodeResume 3 节点 kill 1：剩余节点自动续跑至完成。
func TestE2E_Autonomy_CrossNodeResume(t *testing.T) {
	store := newSharedCheckpointStore()

	// 节点 A（执行节点）崩溃：已完成 collect+analyze
	goal := runGoalToStep(t, store, "node-a", "analyze")
	goalID := goal.ID

	// 故障域隔离：节点 B/C 启动，自动扫描未完成目标并续跑（无人工干预）
	for _, nodeName := range []string{"node-b", "node-c"} {
		rt := NewAutonomyRuntime(RuntimeConfig{
			StepExecutor:    &failoverExecutor{},
			CheckpointStore: store,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		resumed, err := rt.ResumeIncomplete(ctx)
		if err != nil {
			cancel()
			t.Fatalf("节点 %s 恢复: %v", nodeName, err)
		}
		t.Logf("节点 %s 自动接管 %d 个未完成目标（无人工干预）", nodeName, len(resumed))
		if len(resumed) == 0 {
			cancel()
			continue // 已被前序节点接管
		}
		// 续跑：ExecuteGoal 从 checkpoint 计划继续（已完成步骤自动跳过）
		if err := rt.ExecuteGoal(ctx, goalID); err != nil {
			cancel()
			t.Fatalf("节点 %s 续跑执行: %v", nodeName, err)
		}
		if err := rt.CompleteGoal(goalID); err != nil {
			cancel()
			t.Fatalf("节点 %s 完成目标: %v", nodeName, err)
		}
		g, _ := rt.GetGoal(goalID)
		if g.State != GoalDone {
			cancel()
			t.Fatalf("节点 %s 目标终态 = %s, want done", nodeName, g.State)
		}
		cancel()
		t.Logf("✅ 节点 %s 续跑完成目标 %s（从 fix 继续，非重头）", nodeName, goalID)
		return
	}
	t.Fatal("无节点成功接管并续跑目标（故障域隔离失败）")
}

// TestE2E_Autonomy_ResumeContinuesNotRestarts 续跑从断点继续（不重头执行）。
func TestE2E_Autonomy_ResumeContinuesNotRestarts(t *testing.T) {
	store := newSharedCheckpointStore()
	goal := runGoalToStep(t, store, "node-a", "analyze")

	rt := NewAutonomyRuntime(RuntimeConfig{
		StepExecutor:    &failoverExecutor{},
		CheckpointStore: store,
	})
	ctx := context.Background()
	resumed, err := rt.ResumeIncomplete(ctx)
	if err != nil {
		t.Fatalf("ResumeIncomplete: %v", err)
	}
	if len(resumed) != 1 {
		t.Fatalf("resumed = %d, want 1", len(resumed))
	}
	plan, _ := rt.GetPlan(goal.ID)
	completed := 0
	for _, s := range plan.Steps {
		if s.Status == StepCompleted {
			completed++
		}
	}
	// 续跑必须从 checkpoint 继续：collect+analyze 已完成，待执行 fix/verify
	if completed != 2 {
		t.Errorf("恢复后已完成步骤 = %d, want 2（从断点继续而非重头）", completed)
	}
}
