// quality_baseline_test.go — v5.1 质量度量体系回归门
//
// 加载 bench/results/2026-Q3-v5.1-quality-baseline.json 并校验全部质量门。
// 本测试即「四件套进回归门」的 CI 落点：任一门不达标即失败（AGENTS.md §6.1）。
// requires_key 门在无 secrets 环境自动跳过。
package eval

import (
	"strings"
	"testing"
)

func TestQualityBaselineGate(t *testing.T) {
	qb, err := LoadQualityBaseline(DefaultQualityBaselinePath())
	if err != nil {
		t.Fatalf("加载质量基线失败: %v", err)
	}
	if qb.Version == "" {
		t.Fatal("质量基线缺少 version")
	}
	if len(qb.Gates) < 5 {
		t.Fatalf("质量门数量不足：期望 ≥5，得到 %d", len(qb.Gates))
	}

	// 无 key 环境：requires_key 门跳过，其余必须全过
	violations := qb.Check(false)
	if len(violations) > 0 {
		t.Fatalf("质量门存在违规：\n%s", strings.Join(violations, "\n"))
	}
}

func TestQualityBaselineCheckDetectsViolation(t *testing.T) {
	qb := &QualityBaseline{
		Gates: []QualityGate{
			{Name: "ok_gate", Metric: "recall_at_10", Value: 1.0, Threshold: 0.95, Comparison: AtLeast},
			{Name: "bad_recall", Metric: "recall_at_10", Value: 0.7, Threshold: 0.95, Comparison: AtLeast},
			{Name: "bad_latency", Metric: "p95_ns", Value: 20000, Threshold: 10954, Comparison: AtMost},
			{Name: "skipped_real", Metric: "cost_usd_per_task", Value: 0, Threshold: 0.05, Comparison: AtMost, RequiresKey: true},
		},
	}
	violations := qb.Check(false)
	if len(violations) != 2 {
		t.Fatalf("应检出 2 条违规，得到 %d：%v", len(violations), violations)
	}
	if !strings.Contains(violations[0], "bad_recall") || !strings.Contains(violations[1], "bad_latency") {
		t.Errorf("违规描述应包含门名称：%v", violations)
	}
	// requiresKey=true 时 real 门参与校验（值 0 ≤ 0.05 通过）
	if v := qb.Check(true); len(v) != 2 {
		t.Errorf("requiresKey 模式仍应只有 2 条违规，得到 %d", len(v))
	}
}
