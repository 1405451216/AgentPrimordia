package eval

import (
	"context"
	"testing"
)

// TestAutonomyGoalBench_Pass 计划覆盖全部必达阶段 → 通过。
func TestAutonomyGoalBench_Pass(t *testing.T) {
	provider := &mockBenchProvider{responses: []string{
		`[{"id":"1","description":"采集监控数据","depends_on":[]},{"id":"2","description":"修复异常","depends_on":["1"]}]`,
	}}
	agent := NewLLMBenchAgent(provider, "gpt-4o-mini")

	res, err := RunAutonomyGoalBench(context.Background(), LLMBenchConfig{
		Model: "gpt-4o-mini", ProviderName: "mock", Threshold: 0,
	}, agent, []AutonomyGoalCase{
		{ID: "g1", Goal: "监控数据异常并修复", Required: []string{"采集", "修复"}},
	})
	if err != nil {
		t.Fatalf("RunAutonomyGoalBench: %v", err)
	}
	if res.Total != 1 || res.Passed != 1 || res.PassRate != 1.0 {
		t.Errorf("res = total=%d passed=%d rate=%.2f, want 1/1/1.0", res.Total, res.Passed, res.PassRate)
	}
	if res.CostUSD == 0 {
		t.Error("cost 应为 0 以上（用量已累计）")
	}
}

// TestAutonomyGoalBench_FailZero 计划缺必达阶段 → 0 分失败（不门禁）。
func TestAutonomyGoalBench_FailZero(t *testing.T) {
	provider := &mockBenchProvider{responses: []string{`[{"id":"1","description":"直接完成","depends_on":[]}]`}}
	agent := NewLLMBenchAgent(provider, "mock-bench")

	res, err := RunAutonomyGoalBench(context.Background(), LLMBenchConfig{
		Model: "mock-bench", ProviderName: "mock", Threshold: 0,
	}, agent, []AutonomyGoalCase{
		{ID: "g1", Goal: "监控数据异常并修复", Required: []string{"采集", "修复"}},
	})
	if err != nil {
		t.Fatalf("RunAutonomyGoalBench: %v", err)
	}
	if res.Passed != 0 || res.Cases[0].Score != 0 {
		t.Errorf("缺阶段应记 0 分失败，got passed=%d score=%.2f", res.Passed, res.Cases[0].Score)
	}
}

// TestAutonomyGoalBench_BadJSON 计划 JSON 解析失败 → 0 分失败。
func TestAutonomyGoalBench_BadJSON(t *testing.T) {
	provider := &mockBenchProvider{responses: []string{"不是 JSON"}}
	agent := NewLLMBenchAgent(provider, "mock-bench")

	res, err := RunAutonomyGoalBench(context.Background(), LLMBenchConfig{
		Model: "mock-bench", ProviderName: "mock", Threshold: 0,
	}, agent, []AutonomyGoalCase{
		{ID: "g1", Goal: "目标", Required: []string{"采集"}},
	})
	if err != nil {
		t.Fatalf("RunAutonomyGoalBench: %v", err)
	}
	if res.Passed != 0 {
		t.Errorf("坏 JSON 应失败，got passed=%d", res.Passed)
	}
}

// TestSkillAcquisitionBench_Pass 合法技能 JSON → 通过。
func TestSkillAcquisitionBench_Pass(t *testing.T) {
	provider := &mockBenchProvider{responses: []string{
		`{"name":"数据修复","description":"检测并修复异常","steps":[{"id":"s1","tool_name":"query_anomaly","description":"查异常","depends_on":[]},{"id":"s2","tool_name":"fix_data","description":"修复","depends_on":["s1"]}],"tags":["数据"]}`,
	}}
	agent := NewLLMBenchAgent(provider, "mock-bench")

	res, err := RunSkillAcquisitionBench(context.Background(), LLMBenchConfig{
		Model: "mock-bench", ProviderName: "mock", Threshold: 0,
	}, agent, []SkillAcquisitionCase{
		{ID: "s1", Task: "数据修复", ToolCalls: []string{"query_anomaly", "fix_data"}, MinSteps: 2},
	})
	if err != nil {
		t.Fatalf("RunSkillAcquisitionBench: %v", err)
	}
	if res.Total != 1 || res.Passed != 1 || res.PassRate != 1.0 {
		t.Errorf("res = total=%d passed=%d rate=%.2f, want 1/1/1.0", res.Total, res.Passed, res.PassRate)
	}
}

// TestSkillAcquisitionBench_TooFewSteps 步骤数不达标 → 0 分失败。
func TestSkillAcquisitionBench_TooFewSteps(t *testing.T) {
	provider := &mockBenchProvider{responses: []string{
		`{"name":"数据修复","description":"d","steps":[{"id":"s1","tool_name":"fix","description":"修复","depends_on":[]}]}`,
	}}
	agent := NewLLMBenchAgent(provider, "mock-bench")

	res, err := RunSkillAcquisitionBench(context.Background(), LLMBenchConfig{
		Model: "mock-bench", ProviderName: "mock", Threshold: 0,
	}, agent, []SkillAcquisitionCase{
		{ID: "s1", Task: "数据修复", ToolCalls: []string{"a", "b", "c"}, MinSteps: 3},
	})
	if err != nil {
		t.Fatalf("RunSkillAcquisitionBench: %v", err)
	}
	if res.Passed != 0 {
		t.Errorf("步骤不足应失败，got passed=%d", res.Passed)
	}
}

// TestSkillAcquisitionBench_EmptyName 名称缺失 → 0 分失败。
func TestSkillAcquisitionBench_EmptyName(t *testing.T) {
	provider := &mockBenchProvider{responses: []string{
		`{"name":"","description":"d","steps":[{"id":"s1","tool_name":"fix","description":"修复","depends_on":[]}]}`,
	}}
	agent := NewLLMBenchAgent(provider, "mock-bench")

	res, err := RunSkillAcquisitionBench(context.Background(), LLMBenchConfig{
		Model: "mock-bench", ProviderName: "mock", Threshold: 0,
	}, agent, []SkillAcquisitionCase{
		{ID: "s1", Task: "数据修复", ToolCalls: []string{"fix"}, MinSteps: 1},
	})
	if err != nil {
		t.Fatalf("RunSkillAcquisitionBench: %v", err)
	}
	if res.Passed != 0 {
		t.Errorf("空名称应失败，got passed=%d", res.Passed)
	}
}
