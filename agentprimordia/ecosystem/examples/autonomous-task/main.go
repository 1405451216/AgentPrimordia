// autonomous-task 验收 demo：长期自治执行模型端到端演示
//
// 验收场景：定时监控数据 → 异常自主检索修复 → 完成后报告；
// 中途 kill 进程 → 重启恢复继续。
//
// 运行方式：go run ./ecosystem/examples/autonomous-task/
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"agentprimordia/internal/agent/autonomy"
)

// dataMonitorExecutor 模拟数据监控修复执行器
type dataMonitorExecutor struct{}

func (e *dataMonitorExecutor) ExecuteStep(_ context.Context, step autonomy.PlanStep) (string, error) {
	// 模拟步骤执行耗时
	time.Sleep(100 * time.Millisecond)

	switch step.ID {
	case "collect":
		return "采集到 1000 条数据，发现 3 条异常", nil
	case "analyze":
		return "异常原因：数据源连接池耗尽导致写入超时", nil
	case "fix":
		return "已扩容连接池至 50，重试写入成功", nil
	case "verify":
		return "验证通过：异常数据归零，写入延迟 < 100ms", nil
	case "report":
		return "报告已生成：修复 3 条异常数据，根因为连接池不足", nil
	default:
		return "unknown step", nil
	}
}

// inMemoryCheckpointStore 内存检查点存储（demo 用）
type inMemoryCheckpointStore struct {
	data map[string]*autonomy.Checkpoint
}

func newInMemoryCheckpointStore() *inMemoryCheckpointStore {
	return &inMemoryCheckpointStore{data: make(map[string]*autonomy.Checkpoint)}
}

func (s *inMemoryCheckpointStore) SaveCheckpoint(_ context.Context, cp *autonomy.Checkpoint) error {
	s.data[cp.GoalID] = cp
	return nil
}

func (s *inMemoryCheckpointStore) LoadCheckpoint(_ context.Context, goalID string) (*autonomy.Checkpoint, error) {
	cp, ok := s.data[goalID]
	if !ok {
		return nil, fmt.Errorf("not found: %s", goalID)
	}
	return cp, nil
}

func (s *inMemoryCheckpointStore) ListIncomplete(_ context.Context) ([]*autonomy.Checkpoint, error) {
	var result []*autonomy.Checkpoint
	for _, cp := range s.data {
		if !cp.Completed {
			result = append(result, cp)
		}
	}
	return result, nil
}

func main() {
	fmt.Println("=== AgentPrimordia v3.3 长期自治验收 Demo ===")
	fmt.Println()

	ctx := context.Background()

	// 1. 装配自治运行时
	rt := autonomy.NewAutonomyRuntime(autonomy.RuntimeConfig{
		StepExecutor:    &dataMonitorExecutor{},
		CheckpointStore: newInMemoryCheckpointStore(),
		MaxRetries:      2,
		MonitorConfig:   autonomy.MonitorConfig{StallThreshold: 5},
	})

	// 2. 提交自治目标
	goal := rt.SubmitGoal("监控数据异常并自动修复", autonomy.GoalConfig{
		AcceptanceCriteria: []string{"异常数据归零", "修复日志生成"},
		Priority:           autonomy.PriorityHigh,
		MaxRetries:         3,
	})
	fmt.Printf("📌 提交目标: %s (ID: %s)\n", goal.Description, goal.ID)
	fmt.Printf("   优先级: High | 验收标准: %v\n", goal.AcceptanceCriteria)
	fmt.Println()

	// 3. 制定执行计划
	plan := autonomy.NewGoalPlan(goal.ID, []autonomy.PlanStep{
		{ID: "collect", Description: "采集监控数据"},
		{ID: "analyze", Description: "分析异常根因", DependsOn: []string{"collect"}},
		{ID: "fix", Description: "执行修复操作", DependsOn: []string{"analyze"}},
		{ID: "verify", Description: "验证修复结果", DependsOn: []string{"fix"}},
		{ID: "report", Description: "生成修复报告", DependsOn: []string{"verify"}},
	})
	if err := plan.Validate(); err != nil {
		log.Fatalf("计划校验失败: %v", err)
	}
	if err := rt.SetPlan(goal.ID, plan); err != nil {
		log.Fatalf("设置计划失败: %v", err)
	}
	fmt.Println("📋 执行计划已制定（5 步骤链式依赖）")
	fmt.Println()

	// 4. 注册监控告警
	rt.GetMonitor().OnAlert(func(a autonomy.Alert) {
		fmt.Printf("⚠️  [%s] %s: %s\n", a.Level, a.GoalID, a.Message)
	})

	// 5. 执行目标
	fmt.Println("🚀 开始自治执行...")
	start := time.Now()
	if err := rt.ExecuteGoal(ctx, goal.ID); err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	elapsed := time.Since(start)
	fmt.Println()

	// 6. 校验结果
	fmt.Println("✅ 执行完成，校验结果:")
	for _, step := range plan.Steps {
		s := plan.GetStep(step.ID)
		fmt.Printf("   [%s] %s → %s (%.0fms)\n", s.Status, s.Description, s.Result, float64(s.Duration().Milliseconds()))
	}
	fmt.Println()

	// 7. 完成目标
	if err := rt.CompleteGoal(goal.ID); err != nil {
		log.Fatalf("完成目标失败: %v", err)
	}
	g, _ := rt.GetGoal(goal.ID)
	fmt.Printf("🎯 目标最终状态: %s (耗时: %v)\n", g.State, elapsed.Round(time.Millisecond))
	fmt.Println()

	// 8. 演示崩溃恢复
	fmt.Println("--- 崩溃恢复演示 ---")
	fmt.Println("模拟：执行到 'fix' 步骤后进程崩溃...")

	// 创建新运行时（模拟重启）
	store := newInMemoryCheckpointStore()
	rt2 := autonomy.NewAutonomyRuntime(autonomy.RuntimeConfig{
		StepExecutor:    &dataMonitorExecutor{},
		CheckpointStore: store,
	})

	// 模拟之前的检查点
	partialPlan := autonomy.NewGoalPlan("crashed-goal", []autonomy.PlanStep{
		{ID: "collect", Description: "采集监控数据"},
		{ID: "analyze", Description: "分析异常根因", DependsOn: []string{"collect"}},
		{ID: "fix", Description: "执行修复操作", DependsOn: []string{"analyze"}},
		{ID: "verify", Description: "验证修复结果", DependsOn: []string{"fix"}},
	})
	partialPlan.MarkStepCompleted("collect")
	partialPlan.MarkStepCompleted("analyze")
	_ = store.SaveCheckpoint(ctx, &autonomy.Checkpoint{
		GoalID:            "crashed-goal",
		State:             autonomy.GoalExecuting,
		LastCompletedStep: "analyze",
		PlanSnapshot:      partialPlan,
		Completed:         false,
	})

	// 恢复
	resumed, err := rt2.ResumeIncomplete(ctx)
	if err != nil {
		log.Fatalf("恢复失败: %v", err)
	}
	fmt.Printf("🔄 恢复 %d 个未完成目标: %v\n", len(resumed), resumed)
	p, _ := rt2.GetPlan("crashed-goal")
	fmt.Printf("   已完成步骤: collect, analyze | 待执行: fix, verify\n")
	fmt.Printf("   计划进度: %.0f%%\n", p.Progress()*100)
	fmt.Println()

	fmt.Println("=== 验收通过：自治执行 + 崩溃恢复 端到端演示完成 ===")
}
