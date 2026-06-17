package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===== Mock Worker =====

// MockWorker 模拟 Worker 实现
type MockWorker struct {
	id           string
	executeFunc  func(ctx context.Context, task *Task) (*TaskResult, error)
	executeCount int64
}

func NewMockWorker(id string, executeFunc func(ctx context.Context, task *Task) (*TaskResult, error)) *MockWorker {
	if executeFunc == nil {
		executeFunc = func(ctx context.Context, task *Task) (*TaskResult, error) {
			atomic.AddInt64(executeCount(id), 1)
			return &TaskResult{
				WorkerID: id,
				TaskID:   task.ID,
				Status:   TaskStatusCompleted,
				Output:   map[string]any{"result": fmt.Sprintf("worker-%s completed task %s", id, task.ID)},
			}, nil
		}
	}
	return &MockWorker{
		id:          id,
		executeFunc: executeFunc,
	}
}

func (m *MockWorker) ID() string { return m.id }

func (m *MockWorker) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	atomic.AddInt64(&m.executeCount, 1)
	return m.executeFunc(ctx, task)
}

func (m *MockWorker) ExecuteCount() int {
	return int(atomic.LoadInt64(&m.executeCount))
}

// 全局计数器（用于默认执行函数）
var (
	globalCounters   = make(map[string]*int64)
	globalCountersMu sync.Mutex
)

func executeCount(id string) *int64 {
	globalCountersMu.Lock()
	defer globalCountersMu.Unlock()
	if _, ok := globalCounters[id]; !ok {
		var c int64
		globalCounters[id] = &c
	}
	return globalCounters[id]
}

// ===== Strategy Tests =====

func TestRoundRobinStrategy(t *testing.T) {
	strategy := NewRoundRobinStrategy()
	if strategy.Name() != "round_robin" {
		t.Errorf("expected name 'round_robin', got '%s'", strategy.Name())
	}

	workers := []*WorkerState{
		{ID: "w1", MaxConcurrency: 10, available: true},
		{ID: "w2", MaxConcurrency: 10, available: true},
		{ID: "w3", MaxConcurrency: 10, available: true},
	}

	task := &Task{ID: "task-1", Name: "test"}

	// 测试轮询分配
	selected := make(map[string]int)
	for i := 0; i < 9; i++ {
		w, err := strategy.Select(task, workers)
		if err != nil {
			t.Fatalf("Select error: %v", err)
		}
		selected[w.ID]++
	}

	// 每个 worker 应该被选中 3 次
	for _, id := range []string{"w1", "w2", "w3"} {
		if selected[id] != 3 {
			t.Errorf("worker %s selected %d times, expected 3", id, selected[id])
		}
	}
}

func TestRoundRobinStrategy_NoAvailableWorkers(t *testing.T) {
	strategy := NewRoundRobinStrategy()
	workers := []*WorkerState{
		{ID: "w1", MaxConcurrency: 1, available: true},
	}
	// 模拟 worker 已满
	atomic.AddInt64(&workers[0].activeTasks, 1)

	task := &Task{ID: "task-1"}
	_, err := strategy.Select(task, workers)
	if err == nil {
		t.Error("expected error when no workers available")
	}
}

func TestLoadBalancedStrategy(t *testing.T) {
	strategy := NewLoadBalancedStrategy()
	if strategy.Name() != "load_balanced" {
		t.Errorf("expected name 'load_balanced', got '%s'", strategy.Name())
	}

	workers := []*WorkerState{
		{ID: "w1", MaxConcurrency: 10, available: true},
		{ID: "w2", MaxConcurrency: 10, available: true},
		{ID: "w3", MaxConcurrency: 10, available: true},
	}

	// 设置不同的负载
	atomic.AddInt64(&workers[0].activeTasks, 5) // w1: 5 tasks
	atomic.AddInt64(&workers[1].activeTasks, 2) // w2: 2 tasks
	atomic.AddInt64(&workers[2].activeTasks, 8) // w3: 8 tasks

	task := &Task{ID: "task-1"}
	w, err := strategy.Select(task, workers)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}

	// 应该选择负载最低的 w2
	if w.ID != "w2" {
		t.Errorf("expected w2 (lowest load), got %s", w.ID)
	}
}

func TestSkillBasedStrategy(t *testing.T) {
	strategy := NewSkillBasedStrategy()
	if strategy.Name() != "skill_based" {
		t.Errorf("expected name 'skill_based', got '%s'", strategy.Name())
	}

	workers := []*WorkerState{
		{ID: "w1", Skills: []string{"python", "ml"}, MaxConcurrency: 10, available: true},
		{ID: "w2", Skills: []string{"go", "web"}, MaxConcurrency: 10, available: true},
		{ID: "w3", Skills: []string{"python", "go", "ml"}, MaxConcurrency: 10, available: true},
	}

	// 测试技能匹配：需要 python + ml
	task := &Task{
		ID:             "task-1",
		RequiredSkills: []string{"python", "ml"},
	}

	w, err := strategy.Select(task, workers)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}

	// w1 和 w3 都匹配，但 w3 多一个 go 技能（不影响匹配数），应该选负载低的
	// 这里 w1 和 w3 都匹配 2 个技能，w1 负载更低（都是 0）
	// 实际实现中，得分相同时选负载低的，所以 w1 或 w3 都可能（取决于遍历顺序）
	if w.ID != "w1" && w.ID != "w3" {
		t.Errorf("expected w1 or w3 (best skill match), got %s", w.ID)
	}
}

func TestSkillBasedStrategy_NoSkillsRequired(t *testing.T) {
	strategy := NewSkillBasedStrategy()
	workers := []*WorkerState{
		{ID: "w1", MaxConcurrency: 10, available: true},
		{ID: "w2", MaxConcurrency: 10, available: true},
	}

	task := &Task{ID: "task-1"} // 无技能要求
	w, err := strategy.Select(task, workers)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}

	// 无技能要求时返回第一个可用的
	if w.ID != "w1" {
		t.Errorf("expected w1 (first available), got %s", w.ID)
	}
}

func TestSkillBasedStrategy_NoMatch(t *testing.T) {
	strategy := NewSkillBasedStrategy()
	workers := []*WorkerState{
		{ID: "w1", Skills: []string{"python"}, MaxConcurrency: 10, available: true},
	}

	task := &Task{
		ID:             "task-1",
		RequiredSkills: []string{"rust"},
	}

	_, err := strategy.Select(task, workers)
	if err == nil {
		t.Error("expected error when no worker matches skills")
	}
}

func TestSkillBasedStrategy_WithFallback(t *testing.T) {
	fallback := NewRoundRobinStrategy()
	strategy := &SkillBasedStrategy{Fallback: fallback}

	workers := []*WorkerState{
		{ID: "w1", Skills: []string{"python"}, MaxConcurrency: 10, available: true},
	}

	task := &Task{
		ID:             "task-1",
		RequiredSkills: []string{"rust"}, // 无匹配
	}

	w, err := strategy.Select(task, workers)
	if err != nil {
		t.Fatalf("Select error: %v", err)
	}

	// 应该回退到 round_robin
	if w.ID != "w1" {
		t.Errorf("expected fallback to w1, got %s", w.ID)
	}
}

// ===== Supervisor Tests =====

func TestSupervisor_NewSupervisor(t *testing.T) {
	config := SupervisorConfig{
		Name:        "test-supervisor",
		Description: "测试监督者",
		Timeout:     10 * time.Second,
		MaxRetries:  2,
	}

	strategy := NewRoundRobinStrategy()
	sup, err := NewSupervisor(config, strategy)
	if err != nil {
		t.Fatalf("NewSupervisor error: %v", err)
	}

	if sup.Strategy() != "round_robin" {
		t.Errorf("expected strategy 'round_robin', got '%s'", sup.Strategy())
	}

	if sup.WorkerCount() != 0 {
		t.Errorf("expected 0 workers, got %d", sup.WorkerCount())
	}
}

func TestSupervisor_NewSupervisor_NilStrategy(t *testing.T) {
	config := SupervisorConfig{Name: "test"}
	_, err := NewSupervisor(config, nil)
	if err == nil {
		t.Error("expected error when strategy is nil")
	}
}

func TestSupervisor_AddWorker(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	worker := NewMockWorker("w1", nil)
	err := sup.AddWorker(worker, []string{"python", "ml"}, 5)
	if err != nil {
		t.Fatalf("AddWorker error: %v", err)
	}

	if sup.WorkerCount() != 1 {
		t.Errorf("expected 1 worker, got %d", sup.WorkerCount())
	}

	workers := sup.Workers()
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker in list, got %d", len(workers))
	}

	w := workers[0]
	if w.ID != "w1" {
		t.Errorf("expected worker ID 'w1', got '%s'", w.ID)
	}
	if len(w.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(w.Skills))
	}
	if w.MaxConcurrency != 5 {
		t.Errorf("expected max concurrency 5, got %d", w.MaxConcurrency)
	}
}

func TestSupervisor_AddWorker_Duplicate(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	worker1 := NewMockWorker("w1", nil)
	worker2 := NewMockWorker("w1", nil) // 重复 ID

	_ = sup.AddWorker(worker1, nil, 10)
	err := sup.AddWorker(worker2, nil, 10)
	if err == nil {
		t.Error("expected error when adding duplicate worker")
	}
}

func TestSupervisor_AddWorker_InvalidWorker(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	// nil worker
	err := sup.AddWorker(nil, nil, 10)
	if err == nil {
		t.Error("expected error when worker is nil")
	}

	// empty ID
	emptyWorker := NewMockWorker("", nil)
	err = sup.AddWorker(emptyWorker, nil, 10)
	if err == nil {
		t.Error("expected error when worker ID is empty")
	}
}

func TestSupervisor_RemoveWorker(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	worker := NewMockWorker("w1", nil)
	_ = sup.AddWorker(worker, nil, 10)

	err := sup.RemoveWorker("w1")
	if err != nil {
		t.Fatalf("RemoveWorker error: %v", err)
	}

	if sup.WorkerCount() != 0 {
		t.Errorf("expected 0 workers after removal, got %d", sup.WorkerCount())
	}
}

func TestSupervisor_RemoveWorker_NotFound(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	err := sup.RemoveWorker("nonexistent")
	if err == nil {
		t.Error("expected error when removing nonexistent worker")
	}
}

func TestSupervisor_RemoveWorker_WithActiveTasks(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	worker := NewMockWorker("w1", nil)
	_ = sup.AddWorker(worker, nil, 10)

	// 模拟活跃任务
	workers := sup.Workers()
	atomic.AddInt64(&workers[0].activeTasks, 1)

	err := sup.RemoveWorker("w1")
	if err == nil {
		t.Error("expected error when worker has active tasks")
	}

	// worker 应该被标记为不可用但仍在列表中
	if sup.WorkerCount() != 1 {
		t.Errorf("expected 1 worker (marked unavailable), got %d", sup.WorkerCount())
	}

	workers = sup.Workers()
	if workers[0].Available() {
		t.Error("worker should be marked unavailable")
	}
}

func TestSupervisor_Execute_RoundRobin(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{
		Name:    "test",
		Timeout: 5 * time.Second,
	}, NewRoundRobinStrategy())

	// 添加 3 个 worker
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("w%d", i)
		worker := NewMockWorker(id, nil)
		_ = sup.AddWorker(worker, nil, 10)
	}

	// 执行 6 个任务
	for i := 0; i < 6; i++ {
		task := &Task{
			ID:   fmt.Sprintf("task-%d", i),
			Name: fmt.Sprintf("Task %d", i),
		}

		result, err := sup.Execute(context.Background(), task)
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		if result.Status != TaskStatusCompleted {
			t.Errorf("task %d: expected completed, got %s", i, result.Status)
		}
	}

	// 检查每个 worker 执行了 2 次任务
	workers := sup.Workers()
	for _, w := range workers {
		mock := w.Worker.(*MockWorker)
		if mock.ExecuteCount() != 2 {
			t.Errorf("worker %s executed %d times, expected 2", w.ID, mock.ExecuteCount())
		}
	}

	// 检查统计
	stats := sup.Stats()
	if stats["total_completed"].(int64) != 6 {
		t.Errorf("expected 6 completed tasks, got %v", stats["total_completed"])
	}
}

func TestSupervisor_Execute_LoadBalanced(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{
		Name:    "test",
		Timeout: 5 * time.Second,
	}, NewLoadBalancedStrategy())

	// 添加 2 个 worker
	w1 := NewMockWorker("w1", nil)
	w2 := NewMockWorker("w2", nil)
	_ = sup.AddWorker(w1, nil, 10)
	_ = sup.AddWorker(w2, nil, 10)

	// 手动设置 w1 负载更高
	workers := sup.Workers()
	for _, w := range workers {
		if w.ID == "w1" {
			atomic.AddInt64(&w.activeTasks, 5)
		}
	}

	// 执行 1 个任务，应该分配给 w2（负载低）
	task := &Task{ID: "task-1", Name: "test"}
	result, err := sup.Execute(context.Background(), task)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.WorkerID != "w2" {
		t.Errorf("expected task assigned to w2 (lower load), got %s", result.WorkerID)
	}
}

func TestSupervisor_Execute_SkillBased(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{
		Name:    "test",
		Timeout: 5 * time.Second,
	}, NewSkillBasedStrategy())

	// 添加不同技能的 worker
	w1 := NewMockWorker("w1", nil)
	w2 := NewMockWorker("w2", nil)
	_ = sup.AddWorker(w1, []string{"python", "ml"}, 10)
	_ = sup.AddWorker(w2, []string{"go", "web"}, 10)

	// 执行需要 python 技能的任务
	task := &Task{
		ID:             "task-1",
		Name:           "ML Task",
		RequiredSkills: []string{"python"},
	}

	result, err := sup.Execute(context.Background(), task)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.WorkerID != "w1" {
		t.Errorf("expected task assigned to w1 (python skill), got %s", result.WorkerID)
	}
}

func TestSupervisor_Execute_NoWorkers(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	task := &Task{ID: "task-1"}
	_, err := sup.Execute(context.Background(), task)
	if err == nil {
		t.Error("expected error when no workers registered")
	}
}

func TestSupervisor_Execute_WorkerFailure(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{
		Name:       "test",
		MaxRetries: 2,
		Timeout:    5 * time.Second,
	}, NewRoundRobinStrategy())

	// 创建总是失败的 worker
	failWorker := NewMockWorker("w1", func(ctx context.Context, task *Task) (*TaskResult, error) {
		return &TaskResult{
			WorkerID: "w1",
			TaskID:   task.ID,
			Status:   TaskStatusFailed,
			Error:    fmt.Errorf("simulated failure"),
		}, nil
	})
	_ = sup.AddWorker(failWorker, nil, 10)

	task := &Task{ID: "task-1"}
	result, err := sup.Execute(context.Background(), task)

	if err == nil {
		t.Error("expected error when worker fails")
	}

	if result.Status != TaskStatusFailed {
		t.Errorf("expected failed status, got %s", result.Status)
	}

	// 检查重试次数（1 次初始 + 2 次重试 = 3 次执行）
	if failWorker.ExecuteCount() != 3 {
		t.Errorf("expected 3 execution attempts, got %d", failWorker.ExecuteCount())
	}

	// 检查统计
	stats := sup.Stats()
	if stats["total_failed"].(int64) != 1 {
		t.Errorf("expected 1 failed task, got %v", stats["total_failed"])
	}
}

func TestSupervisor_Execute_ContextCancellation(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{
		Name:    "test",
		Timeout: 10 * time.Second,
	}, NewRoundRobinStrategy())

	// 创建慢速 worker
	slowWorker := NewMockWorker("w1", func(ctx context.Context, task *Task) (*TaskResult, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &TaskResult{Status: TaskStatusCompleted}, nil
		}
	})
	_ = sup.AddWorker(slowWorker, nil, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	task := &Task{ID: "task-1"}
	_, err := sup.Execute(ctx, task)

	if err == nil {
		t.Error("expected error when context is cancelled")
	}
}

func TestSupervisor_SetStrategy(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	if sup.Strategy() != "round_robin" {
		t.Errorf("expected round_robin, got %s", sup.Strategy())
	}

	err := sup.SetStrategy(NewLoadBalancedStrategy())
	if err != nil {
		t.Fatalf("SetStrategy error: %v", err)
	}

	if sup.Strategy() != "load_balanced" {
		t.Errorf("expected load_balanced, got %s", sup.Strategy())
	}
}

func TestSupervisor_SetStrategy_Nil(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	err := sup.SetStrategy(nil)
	if err == nil {
		t.Error("expected error when setting nil strategy")
	}
}

func TestSupervisor_Events(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{Name: "test"}, NewRoundRobinStrategy())

	worker := NewMockWorker("w1", nil)
	_ = sup.AddWorker(worker, nil, 10)

	// 消费事件
	events := sup.Events()
	eventCount := 0
	done := make(chan bool)

	go func() {
		for {
			select {
			case ev := <-events:
				eventCount++
				if ev.Type == "task_completed" {
					done <- true
					return
				}
			case <-time.After(2 * time.Second):
				done <- true
				return
			}
		}
	}()

	task := &Task{ID: "task-1"}
	_, _ = sup.Execute(context.Background(), task)

	<-done

	// 至少应该有 worker_added 和 task_assigned 和 task_completed 事件
	if eventCount < 3 {
		t.Errorf("expected at least 3 events, got %d", eventCount)
	}
}

func TestSupervisor_Stats(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{
		Name:       "test-supervisor",
		MaxRetries: 1,
	}, NewRoundRobinStrategy())

	w1 := NewMockWorker("w1", nil)
	w2 := NewMockWorker("w2", nil)
	_ = sup.AddWorker(w1, []string{"python"}, 10)
	_ = sup.AddWorker(w2, []string{"go"}, 10)

	// 执行成功任务
	task1 := &Task{ID: "task-1"}
	_, _ = sup.Execute(context.Background(), task1)

	// 执行失败任务
	failWorker := NewMockWorker("fail", func(ctx context.Context, task *Task) (*TaskResult, error) {
		return &TaskResult{Status: TaskStatusFailed, Error: fmt.Errorf("fail")}, nil
	})
	_ = sup.AddWorker(failWorker, nil, 10)
	_ = sup.SetStrategy(NewRoundRobinStrategy()) // 重置策略确保轮询

	task2 := &Task{ID: "task-2"}
	_, _ = sup.Execute(context.Background(), task2)

	stats := sup.Stats()

	if stats["name"] != "test-supervisor" {
		t.Errorf("expected name 'test-supervisor', got %v", stats["name"])
	}
	if stats["worker_count"].(int) != 3 {
		t.Errorf("expected 3 workers, got %v", stats["worker_count"])
	}
	if stats["total_assigned"].(int64) != 2 {
		t.Errorf("expected 2 assigned, got %v", stats["total_assigned"])
	}
}

func TestSupervisor_Export(t *testing.T) {
	sup, _ := NewSupervisor(SupervisorConfig{
		Name:        "test",
		Description: "test supervisor",
	}, NewRoundRobinStrategy())

	worker := NewMockWorker("w1", nil)
	_ = sup.AddWorker(worker, []string{"python"}, 10)

	data, err := sup.Export()
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty export data")
	}

	// 验证是有效的 JSON
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["config"] == nil {
		t.Error("expected config in export")
	}
	if result["strategy"] != "round_robin" {
		t.Errorf("expected strategy 'round_robin', got %v", result["strategy"])
	}
}

func TestWorkerState_Available(t *testing.T) {
	state := &WorkerState{
		ID:             "w1",
		MaxConcurrency: 2,
		available:      true,
	}

	if !state.Available() {
		t.Error("worker should be available")
	}

	// 增加到最大并发
	atomic.AddInt64(&state.activeTasks, 2)
	if state.Available() {
		t.Error("worker should not be available when at max concurrency")
	}

	// 标记为不可用
	state.available = false
	atomic.AddInt64(&state.activeTasks, -1)
	if state.Available() {
		t.Error("worker should not be available when marked unavailable")
	}
}

func TestFilterAvailable(t *testing.T) {
	workers := []*WorkerState{
		{ID: "w1", MaxConcurrency: 10, available: true},
		{ID: "w2", MaxConcurrency: 1, available: true},
		{ID: "w3", MaxConcurrency: 10, available: false},
	}

	// w2 已满
	atomic.AddInt64(&workers[1].activeTasks, 1)

	available := filterAvailable(workers)
	if len(available) != 1 {
		t.Errorf("expected 1 available worker, got %d", len(available))
	}
	if available[0].ID != "w1" {
		t.Errorf("expected w1 available, got %s", available[0].ID)
	}
}

func TestSkillMatchScore(t *testing.T) {
	tests := []struct {
		workerSkills  []string
		requiredSkills []string
		expected      int
	}{
		{[]string{"python", "ml"}, []string{"python"}, 1},
		{[]string{"python", "ml"}, []string{"python", "ml"}, 2},
		{[]string{"python"}, []string{"go", "rust"}, 0},
		{[]string{"Python", "ML"}, []string{"python", "ml"}, 2}, // 大小写不敏感
		{[]string{}, []string{"python"}, 0},
		{[]string{"python"}, []string{}, 0},
	}

	for i, tt := range tests {
		score := skillMatchScore(tt.workerSkills, tt.requiredSkills)
		if score != tt.expected {
			t.Errorf("test %d: expected score %d, got %d", i, tt.expected, score)
		}
	}
}

// 需要导入 json 包
func init() {
	// 确保 json 包被使用（在 Export 测试中）
	_ = json.Marshal
}
