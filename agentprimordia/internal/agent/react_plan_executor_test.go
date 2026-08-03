// react_plan_executor_test.go — v3.4-1 executePlan 重试与 plan 级 checkpoint 测试
package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentprimordia/internal/agent/planning"
	"agentprimordia/internal/persist"
)

// ===== runSubtaskWithRetry 纯函数测试 =====

func TestRunSubtaskWithRetry_SuccessFirst(t *testing.T) {
	attempts := 0
	run := func() (*Response, error) {
		attempts++
		return &Response{Content: "ok"}, nil
	}
	resp, err := runSubtaskWithRetry(context.Background(), run, 1)
	if err != nil {
		t.Fatalf("不应失败: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("Content = %q, want ok", resp.Content)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRunSubtaskWithRetry_RetryThenSuccess(t *testing.T) {
	attempts := 0
	run := func() (*Response, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("boom")
		}
		return &Response{Content: "recovered"}, nil
	}
	resp, err := runSubtaskWithRetry(context.Background(), run, 1)
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if resp.Content != "recovered" {
		t.Fatalf("Content = %q, want recovered", resp.Content)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRunSubtaskWithRetry_AllFail(t *testing.T) {
	attempts := 0
	run := func() (*Response, error) {
		attempts++
		return nil, errors.New("always fails")
	}
	if _, err := runSubtaskWithRetry(context.Background(), run, 2); err == nil {
		t.Fatal("应返回错误")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3（maxRetries=2 + 首次）", attempts)
	}
}

// ===== executePlan 重试 + plan 级 checkpoint 集成测试 =====

// mockCheckpointCapable 让 ReActAgent.getCheckpointStore 可解析（通过 self 接口发现）
// 同时实现 core.Agent 接口（self 字段的类型约束）
type mockCheckpointCapable struct {
	store persist.CheckpointStore
}

func (m *mockCheckpointCapable) GetCheckpointStore() persist.CheckpointStore { return m.store }
func (m *mockCheckpointCapable) Run(_ context.Context, _ Message) (*Response, error) {
	return nil, errors.New("not used in test")
}
func (m *mockCheckpointCapable) StreamRun(_ context.Context, _ Message) (<-chan StreamEvent, error) {
	return nil, errors.New("not used in test")
}
func (m *mockCheckpointCapable) Stop()                     {}
func (m *mockCheckpointCapable) Stats() AgentStats         { return AgentStats{} }
func (m *mockCheckpointCapable) Name() string              { return "mock" }

// TestExecutePlan_SubtaskRetryAndCheckpoint 验证：
//  1. 子任务失败自动重试（maxRetries=1）
//  2. 每个子任务完成后保存带 Plan 进度的 checkpoint
//  3. checkpoint 记录 completed 与 results
func TestExecutePlan_SubtaskRetryAndCheckpoint(t *testing.T) {
	store, err := persist.InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore 失败: %v", err)
	}

	a := newReActAgent(ReActConfig{Name: "plan-ckpt", MaxTurns: 10})
	a.self = &mockCheckpointCapable{store: store}

	// 子任务 2 第一次执行失败，第二次成功（记录重试）
	retried := false
	a.subtaskExecutor = func(_ context.Context, task planning.SubTask, _ []Message, _ loopConfig) (*Response, error) {
		if task.ID == "2" && !retried {
			retried = true
			return nil, errors.New("transient failure")
		}
		return &Response{Content: "subtask " + task.ID + " done"}, nil
	}

	plan := &planning.Plan{
		Goal: "test",
		SubTasks: []planning.SubTask{
			{ID: "1", Description: "编写", DependsOn: []string{}},
			{ID: "2", Description: "测试", DependsOn: []string{"1"}},
			{ID: "3", Description: "发布", DependsOn: []string{"2"}},
		},
	}

	resp, err := a.executePlan(context.Background(), []Message{}, plan, loopConfig{requestID: "req-1"}, time.Now(), 0, 0, 0)
	if err != nil {
		t.Fatalf("executePlan 失败: %v", err)
	}
	if resp.Content != "subtask 3 done" {
		t.Fatalf("最终 Content = %q, want 'subtask 3 done'", resp.Content)
	}
	if !retried {
		t.Fatal("子任务 2 应发生一次重试")
	}

	// checkpoint 应保存了 plan 进度：3 个子任务全部完成
	saved, err := store.Load(context.Background(), "plan-ckpt")
	if err != nil {
		t.Fatalf("Load checkpoint 失败: %v", err)
	}
	if saved.Plan == nil {
		t.Fatal("checkpoint 应包含 Plan 进度")
	}
	if len(saved.Plan.Subtasks) != 3 {
		t.Fatalf("Subtasks = %d, want 3", len(saved.Plan.Subtasks))
	}
	if len(saved.Plan.Completed) != 3 {
		t.Fatalf("Completed = %v, want 全部 3 个", saved.Plan.Completed)
	}
	if saved.Plan.Results["2"] != "subtask 2 done" {
		t.Fatalf("Results['2'] = %q", saved.Plan.Results["2"])
	}
}

// TestExecutePlan_FailedSubtask_SavesFailedCheckpoint 验证子任务重试耗尽后：
//  1. executePlan 返回错误
//  2. checkpoint 保存 Status=failed 且含已完成进度
func TestExecutePlan_FailedSubtask_SavesFailedCheckpoint(t *testing.T) {
	store, err := persist.InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore 失败: %v", err)
	}

	a := newReActAgent(ReActConfig{Name: "plan-fail", MaxTurns: 10})
	a.self = &mockCheckpointCapable{store: store}

	// 子任务 2 永远失败
	a.subtaskExecutor = func(_ context.Context, task planning.SubTask, _ []Message, _ loopConfig) (*Response, error) {
		if task.ID == "2" {
			return nil, errors.New("permanent failure")
		}
		return &Response{Content: "ok"}, nil
	}

	plan := &planning.Plan{
		SubTasks: []planning.SubTask{
			{ID: "1", Description: "a", DependsOn: []string{}},
			{ID: "2", Description: "b", DependsOn: []string{"1"}},
			{ID: "3", Description: "c", DependsOn: []string{"2"}},
		},
	}

	_, err = a.executePlan(context.Background(), []Message{}, plan, loopConfig{requestID: "req-2"}, time.Now(), 0, 0, 0)
	if err == nil {
		t.Fatal("子任务失败应返回错误")
	}

	saved, loadErr := store.Load(context.Background(), "plan-fail")
	if loadErr != nil {
		t.Fatalf("Load checkpoint 失败: %v", loadErr)
	}
	if saved.Plan == nil {
		t.Fatal("checkpoint 应包含 Plan 进度")
	}
	if saved.Status != "failed" {
		t.Fatalf("Status = %q, want failed", saved.Status)
	}
	// 已完成的子任务 1 记录在案
	if len(saved.Plan.Completed) != 1 || saved.Plan.Completed[0] != "1" {
		t.Fatalf("Completed = %v, want [1]", saved.Plan.Completed)
	}
}
