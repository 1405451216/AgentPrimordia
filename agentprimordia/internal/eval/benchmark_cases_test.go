package eval

import "testing"

// TestHarnessBenchmarkCases 验证基准集完整性（v3.5-1 验收：≥50 条真实任务）。
func TestHarnessBenchmarkCases(t *testing.T) {
	cases, err := HarnessBenchmarkCases()
	if err != nil {
		t.Fatalf("HarnessBenchmarkCases failed: %v", err)
	}
	if len(cases) < 50 {
		t.Fatalf("基准集用例数 = %d, 要求 ≥50", len(cases))
	}

	ids := make(map[string]bool)
	for _, c := range cases {
		if c.ID == "" {
			t.Errorf("存在空 ID")
		}
		if ids[c.ID] {
			t.Errorf("重复用例 ID: %s", c.ID)
		}
		ids[c.ID] = true
		if c.Name == "" {
			t.Errorf("case %s 缺 Name", c.ID)
		}
		if c.Input == "" {
			t.Errorf("case %s 缺 Input", c.ID)
		}
		if c.Expected == "" {
			t.Errorf("case %s 缺 Expected", c.ID)
		}
		if c.Threshold <= 0 || c.Threshold > 1 {
			t.Errorf("case %s Threshold = %f, 期望 (0,1]", c.ID, c.Threshold)
		}
		// 真实任务必须可被评估：Requires 或 Expected 至少其一
		if len(c.Requires) == 0 && c.Expected == "" {
			t.Errorf("case %s 既无 Requires 也无 Expected", c.ID)
		}
	}
}

// TestHarnessBenchmarkCases_PhaseCoverage 验证全 harness 阶段覆盖。
func TestHarnessBenchmarkCases_PhaseCoverage(t *testing.T) {
	cases := MustBenchmarkCases()

	phases := make(map[string]int)
	langs := make(map[string]int)
	for _, c := range cases {
		phases[c.HarnessPhase]++
		langs[c.Lang]++
	}

	// 必须覆盖计划-编写-实施-测试-审查-发布 + 护栏/记忆/工具
	requiredPhases := []string{
		PhasePlan, PhaseImplement, PhaseTest, PhaseReview, PhaseRelease, PhaseGuard, PhaseMemory, PhaseTool,
	}
	for _, p := range requiredPhases {
		if phases[p] == 0 {
			t.Errorf("基准集缺少阶段覆盖: %s", p)
		}
	}

	// 双线语言覆盖：go 与 ts 各 ≥15 条真实编码/测试/审查任务
	if langs[LangGo] < 15 {
		t.Errorf("Go 侧用例 = %d, 要求 ≥15", langs[LangGo])
	}
	if langs[LangTS] < 15 {
		t.Errorf("TS 侧用例 = %d, 要求 ≥15", langs[LangTS])
	}
}

// TestHarnessBenchmarkCases_JSONRoundTrip 验证 JSON 往返兼容（字段映射）。
func TestHarnessBenchmarkCases_JSONRoundTrip(t *testing.T) {
	cases := MustBenchmarkCases()
	data, err := CompileCases(cases)
	if err != nil {
		t.Fatalf("CompileCases failed: %v", err)
	}
	var restored []EvalCase
	if err := jsonUnmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if len(restored) != len(cases) {
		t.Fatalf("restored = %d, want %d", len(restored), len(cases))
	}
	// 扩展字段必须映射正确
	if restored[0].HarnessPhase == "" && restored[0].Lang == "" {
		t.Error("扩展字段 Lang/HarnessPhase 未正确序列化")
	}
}

// TestCodeConstructRequired 验证编码任务必须声明代码构造片段。
func TestCodeConstructRequired(t *testing.T) {
	for _, c := range MustBenchmarkCases() {
		if c.HarnessPhase == PhaseImplement && len(c.Requires) == 0 {
			t.Errorf("implement 用例 %s 必须声明 Requires 代码构造片段", c.ID)
		}
	}
}
