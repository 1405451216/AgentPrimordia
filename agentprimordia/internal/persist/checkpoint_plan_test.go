package persist

import (
	"testing"
)

// TestAgentState_PlanRoundTrip 验证 Plan 字段的 JSON 序列化往返。
// Plan 为 nil（非 plan 执行）时旧版本 checkpoint 兼容，omitempty 不落盘。
func TestAgentState_PlanRoundTrip(t *testing.T) {
	state := &AgentState{
		AgentID:   "agent-a",
		SessionID: "sess-1",
		Status:    "running",
		Messages:  []CheckpointMessage{{Role: "user", Content: "hi"}},
		TurnCount: 2,
		Plan: &CheckpointPlan{
			Subtasks: []CheckpointSubTask{
				{ID: "1", Description: "编写", DependsOn: []string{}},
				{ID: "2", Description: "测试", DependsOn: []string{"1"}},
			},
			Completed: []string{"1"},
			Results:   map[string]string{"1": "ok"},
			TotalTools: 3,
		},
	}

	data, err := state.Marshal()
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	got, err := UnmarshalAgentState(data)
	if err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if got.Plan == nil {
		t.Fatal("Plan 应为非 nil")
	}
	if len(got.Plan.Subtasks) != 2 || got.Plan.Subtasks[1].ID != "2" {
		t.Errorf("Subtask 恢复错误: %+v", got.Plan.Subtasks)
	}
	if len(got.Plan.Completed) != 1 || got.Plan.Completed[0] != "1" {
		t.Errorf("Completed 恢复错误: %v", got.Plan.Completed)
	}
	if got.Plan.Results["1"] != "ok" {
		t.Errorf("Results 恢复错误: %v", got.Plan.Results)
	}
	if got.Plan.TotalTools != 3 {
		t.Errorf("TotalTools = %d, want 3", got.Plan.TotalTools)
	}
}

// TestAgentState_PlanNil 验证无 Plan 时序列化不携带 plan 字段（向后兼容）。
func TestAgentState_PlanNil(t *testing.T) {
	state := &AgentState{
		AgentID:   "agent-b",
		Status:    "completed",
		Messages:  []CheckpointMessage{},
		TurnCount: 1,
	}
	data, err := state.Marshal()
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	got, err := UnmarshalAgentState(data)
	if err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	if got.Plan != nil {
		t.Fatal("Plan 应为 nil")
	}
}

// TestSQLiteCheckpointStore_PlanRoundTrip 验证 Plan 进度经 SQLite 持久化往返。
func TestSQLiteCheckpointStore_PlanRoundTrip(t *testing.T) {
	store, err := InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore 失败: %v", err)
	}
	defer store.Close()

	ctx := t.Context()
	state := &AgentState{
		AgentID:   "agent-p",
		SessionID: "sess-p",
		Status:    "running",
		Messages:  []CheckpointMessage{{Role: "user", Content: "goal"}},
		TurnCount: 1,
		Plan: &CheckpointPlan{
			Subtasks: []CheckpointSubTask{
				{ID: "1", Description: "编写", DependsOn: []string{}},
				{ID: "2", Description: "发布", DependsOn: []string{"1"}},
			},
			Completed:  []string{"1"},
			Results:    map[string]string{"1": "done"},
			TotalTools: 4,
		},
	}

	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	got, err := store.Load(ctx, "agent-p")
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if got.Plan == nil {
		t.Fatal("SQLite 往返后 Plan 应为非 nil")
	}
	if len(got.Plan.Completed) != 1 || got.Plan.Completed[0] != "1" {
		t.Errorf("Completed 恢复错误: %v", got.Plan.Completed)
	}
	if got.Plan.TotalTools != 4 {
		t.Errorf("TotalTools = %d, want 4", got.Plan.TotalTools)
	}
}

// TestSQLiteCheckpointStore_PlanNilCompat 验证旧 checkpoint（无 plan 列数据）加载后 Plan 为 nil。
func TestSQLiteCheckpointStore_PlanNilCompat(t *testing.T) {
	store, err := InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore 失败: %v", err)
	}
	defer store.Close()

	ctx := t.Context()
	state := &AgentState{
		AgentID:   "agent-np",
		SessionID: "sess-np",
		Status:    "completed",
		Messages:  []CheckpointMessage{},
		TurnCount: 2,
	}
	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	got, err := store.Load(ctx, "agent-np")
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if got.Plan != nil {
		t.Fatal("无 Plan 时加载后应为 nil")
	}
}
