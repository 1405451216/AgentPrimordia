package autonomy

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- mock 实现 ---

type mockRAGRetriever struct{}

func (m *mockRAGRetriever) Retrieve(_ context.Context, query string, topK int) ([]RAGDocument, error) {
	return []RAGDocument{
		{Content: "知识: " + query, Score: 0.95, Source: "kb"},
	}, nil
}

type mockPoolDispatcher struct {
	dispatched []string
}

func (m *mockPoolDispatcher) Dispatch(_ context.Context, goalID string, fn func() error) error {
	m.dispatched = append(m.dispatched, goalID)
	return fn()
}

func (m *mockPoolDispatcher) ActiveCount() int { return len(m.dispatched) }

type mockClusterSync struct {
	states map[string]GoalState
	owners map[string]string
}

func newMockClusterSync() *mockClusterSync {
	return &mockClusterSync{states: make(map[string]GoalState), owners: make(map[string]string)}
}

func (m *mockClusterSync) SyncState(_ context.Context, goalID string, state GoalState, _ map[string]string) error {
	m.states[goalID] = state
	return nil
}

func (m *mockClusterSync) AcquireOwnership(_ context.Context, goalID string, nodeID string) (bool, error) {
	if owner, ok := m.owners[goalID]; ok && owner != nodeID {
		return false, nil
	}
	m.owners[goalID] = nodeID
	return true, nil
}

func (m *mockClusterSync) ReleaseOwnership(_ context.Context, goalID string, _ string) error {
	delete(m.owners, goalID)
	return nil
}

type mockGoalMetrics struct {
	lifecycleCount int
	stepCount      int
	replanCount    int
}

func (m *mockGoalMetrics) RecordGoalLifecycle(_ string, _ GoalState, _ time.Duration) { m.lifecycleCount++ }
func (m *mockGoalMetrics) RecordStepExecution(_ string, _ string, _ time.Duration, _ error) { m.stepCount++ }
func (m *mockGoalMetrics) RecordReplan(_ string, _ string) { m.replanCount++ }

type mockStepGuardrail struct {
	blockSteps   map[string]bool
	blockOutputs map[string]bool
}

func (m *mockStepGuardrail) CheckStep(_ context.Context, _ string, step PlanStep) (bool, string, error) {
	if m.blockSteps[step.ID] {
		return false, "步骤被禁止", nil
	}
	return true, "", nil
}

func (m *mockStepGuardrail) CheckOutput(_ context.Context, _ string, output string) (string, bool, error) {
	if m.blockOutputs[output] {
		return "", true, nil
	}
	return "", false, nil
}

// --- 测试 ---

func TestRAGIntegration(t *testing.T) {
	rag := NewRAGIntegration(&mockRAGRetriever{}, 3)
	docs, err := rag.EnrichStepContext(context.Background(), PlanStep{ID: "s1", Description: "数据修复"})
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if len(docs) != 1 || docs[0].Score != 0.95 {
		t.Errorf("docs = %v", docs)
	}
}

func TestPoolIntegration(t *testing.T) {
	pool := NewPoolIntegration(&mockPoolDispatcher{})
	executed := false
	err := pool.DispatchGoal(context.Background(), "goal-1", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !executed {
		t.Error("goal function should have been executed")
	}
}

func TestClusterIntegration(t *testing.T) {
	sync := newMockClusterSync()
	cluster := NewClusterIntegration(sync, "node-1")
	ctx := context.Background()

	// 获取执行权
	acquired, err := cluster.AcquireGoal(ctx, "goal-1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !acquired {
		t.Error("should acquire ownership")
	}

	// 同步状态
	err = cluster.SyncGoalState(ctx, "goal-1", GoalExecuting)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if sync.states["goal-1"] != GoalExecuting {
		t.Errorf("state = %s, want executing", sync.states["goal-1"])
	}
}

func TestObservabilityIntegration(t *testing.T) {
	metrics := &mockGoalMetrics{}
	obs := NewObservabilityIntegration(metrics)

	obs.RecordLifecycle("goal-1", GoalExecuting, time.Second)
	obs.RecordStep("goal-1", "s1", 500*time.Millisecond, nil)

	if metrics.lifecycleCount != 1 {
		t.Errorf("lifecycle count = %d, want 1", metrics.lifecycleCount)
	}
	if metrics.stepCount != 1 {
		t.Errorf("step count = %d, want 1", metrics.stepCount)
	}
}

func TestGuardrailIntegration(t *testing.T) {
	guard := &mockStepGuardrail{
		blockSteps:   map[string]bool{"dangerous": true},
		blockOutputs: map[string]bool{"secret-data": true},
	}
	gi := NewGuardrailIntegration(guard)
	ctx := context.Background()

	// 正常步骤通过
	err := gi.ValidateStep(ctx, "goal-1", PlanStep{ID: "safe", Description: "安全步骤"})
	if err != nil {
		t.Fatalf("safe step should pass: %v", err)
	}

	// 危险步骤被拦截
	err = gi.ValidateStep(ctx, "goal-1", PlanStep{ID: "dangerous", Description: "危险步骤"})
	if err == nil {
		t.Fatal("dangerous step should be blocked")
	}
	var violation *GuardrailViolation
	if !asGuardrailViolation(err, &violation) {
		t.Errorf("error should be GuardrailViolation, got %T", err)
	}

	// 输出校验
	sanitized, err := gi.SanitizeOutput(ctx, "goal-1", "normal output")
	if err != nil {
		t.Fatalf("normal output: %v", err)
	}
	if sanitized != "normal output" {
		t.Errorf("sanitized = %q", sanitized)
	}

	// 被拦截的输出
	_, err = gi.SanitizeOutput(ctx, "goal-1", "secret-data")
	if err == nil {
		t.Fatal("blocked output should return error")
	}
}

func asGuardrailViolation(err error, target **GuardrailViolation) bool {
	if e, ok := err.(*GuardrailViolation); ok {
		*target = e
		return true
	}
	return false
}

// TestGuardrailViolationError 验证错误信息格式
func TestGuardrailViolationError(t *testing.T) {
	e := &GuardrailViolation{GoalID: "g1", StepID: "s1", Reason: "禁止"}
	want := "autonomy: 护栏拦截 [goal=g1, step=s1]: 禁止"
	if e.Error() != want {
		t.Errorf("error = %q, want %q", e.Error(), want)
	}

	e2 := &GuardrailViolation{GoalID: "g1", Reason: "输出违规"}
	want2 := "autonomy: 护栏拦截 [goal=g1]: 输出违规"
	if e2.Error() != want2 {
		t.Errorf("error = %q, want %q", e2.Error(), want2)
	}
}

// 确保 fmt 被使用
var _ = fmt.Sprintf
