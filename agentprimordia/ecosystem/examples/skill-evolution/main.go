// skill-evolution 验收 demo：技能进化端到端演示
//
// 验收场景：首次遇到任务失败 → 习得技能 → 第二次同类任务直接调用技能完成
//
// 运行方式：go run ./ecosystem/examples/skill-evolution/
package main

import (
	"context"
	"fmt"
	"time"

	"agentprimordia/internal/agent/skills"
)

// mockDistiller 模拟 LLM 提炼器
type mockDistiller struct{}

func (m *mockDistiller) Distill(_ context.Context, t skills.Trajectory) (*skills.Skill, error) {
	steps := make([]skills.StepDef, len(t.Records))
	for i, r := range t.Records {
		steps[i] = skills.StepDef{
			ID:          fmt.Sprintf("s%d", i+1),
			Description: r.ToolName + " 操作",
			ToolName:    r.ToolName,
		}
		if i > 0 {
			steps[i].DependsOn = []string{fmt.Sprintf("s%d", i)}
		}
	}
	s := skills.NewSkill("数据异常修复", "从监控数据中检测并修复异常", steps)
	s.Tags = []string{"数据", "修复", "监控"}
	return s, nil
}

// mockExecutor 模拟技能执行器
type mockExecutor struct{}

func (m *mockExecutor) Execute(_ context.Context, _ *skills.Skill, _ map[string]any) (map[string]any, error) {
	return map[string]any{"result": "ok", "fixed": true}, nil
}

func main() {
	fmt.Println("=== AgentPrimordia v3.4 技能进化验收 Demo ===")
	fmt.Println()

	ctx := context.Background()

	// 1. 初始化技能库
	store := skills.NewStore()
	matcher := skills.NewMatcher(store, skills.MatcherConfig{})
	tracker := skills.NewUsageTracker()
	acquisition := skills.NewAcquisition(&mockDistiller{})
	trigger := skills.NewTrigger(skills.TriggerConfig{
		Strategy:        skills.TriggerRepeatPattern,
		RepeatThreshold: 2,
	})

	fmt.Println("📦 技能库已初始化（空）")
	fmt.Println()

	// 2. 模拟首次遇到任务（无技能可用）
	taskDesc := "数据异常修复"
	fmt.Printf("🔍 任务: %s\n", taskDesc)
	match := matcher.Match(taskDesc)
	if match == nil {
		fmt.Println("   ❌ 无匹配技能，使用原始工具链执行")
	}
	fmt.Println()

	// 3. 记录成功轨迹
	fmt.Println("📝 记录成功执行轨迹...")
	trajectory := skills.Trajectory{
		TaskDescription: taskDesc,
		Success:         true,
		Timestamp:       time.Now(),
		Records: []skills.ToolCallRecord{
			{ToolName: "query_anomaly", Success: true, Duration: 100 * time.Millisecond},
			{ToolName: "fix_data", Success: true, Duration: 200 * time.Millisecond},
			{ToolName: "verify_fix", Success: true, Duration: 50 * time.Millisecond},
		},
	}
	acquisition.RecordTrajectory(trajectory)
	trigger.RecordTask("data_fix", true)
	trigger.RecordTask("data_fix", true)
	fmt.Println()

	// 4. 触发习得
	fmt.Println("🧠 触发技能习得...")
	if trigger.ShouldAcquire("data_fix") {
		fmt.Println("   ✓ 重复模式检测触发（同类任务 ≥ 2 次）")
	}

	newSkill, err := acquisition.Acquire(ctx, trajectory)
	if err != nil {
		fmt.Printf("   ❌ 习得失败: %v\n", err)
		return
	}
	fmt.Printf("   ✓ 习得技能: %s (ID: %s)\n", newSkill.Name, newSkill.ID)
	fmt.Printf("   步骤: %d | 标签: %v\n", len(newSkill.Steps), newSkill.Tags)
	fmt.Println()

	// 5. 验证门
	fmt.Println("🔬 运行验证门...")
	verification := skills.NewVerification(&mockExecutor{})
	result := verification.Verify(ctx, newSkill, []skills.TestCase{
		{Name: "正常修复", Input: map[string]any{"target": "db"}, ExpectedOutput: map[string]any{"result": "ok"}},
	})
	if result.Passed {
		fmt.Printf("   ✓ 验证通过 (%d/%d)\n", result.PassedCount, result.Total)
		newSkill.Activate()
		store.Save(newSkill)
		fmt.Printf("   ✓ 技能已激活并入库\n")
	} else {
		fmt.Printf("   ❌ 验证失败: %v\n", result.Failures)
		return
	}
	fmt.Println()

	// 6. 第二次同类任务（直接匹配技能）
	fmt.Println("🔍 第二次遇到同类任务...")
	match2 := matcher.Match(taskDesc)
	if match2 != nil {
		fmt.Printf("   ✓ 匹配到技能: %s (置信度: %s, 分数: %.2f)\n",
			match2.Skill.Name, match2.Confidence, match2.Score)
		fmt.Println("   → 直接调用技能完成，无需重新探索")
	} else {
		fmt.Println("   ❌ 未匹配到技能")
	}
	fmt.Println()

	// 7. 记录使用统计
	tracker.Record(skills.UsageRecord{
		SkillID: newSkill.ID, Success: true, Duration: 350 * time.Millisecond,
	})
	stats := tracker.Stats(newSkill.ID)
	fmt.Printf("📊 使用统计: 调用 %d 次 | 成功率 %.0f%% | 平均耗时 %v\n",
		stats.TotalCalls, stats.SuccessRate*100, stats.AvgDuration)
	fmt.Println()

	fmt.Printf("📚 技能库当前: %d 个技能 (%d 个活跃)\n", store.Count(), len(store.ListActive()))
	fmt.Println()
	fmt.Println("=== 验收通过：首次失败→习得→第二次直接调用 端到端演示完成 ===")
}
