// report_test.go — 实验报告生成器详细测试
package chaos

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ===== FormatReport 测试 =====

func TestFormatReport_Basic(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:       "basic-report",
			Hypothesis: "测试报告生成",
		},
		Status:              StatusCompleted,
		StartTime:           time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		EndTime:             time.Date(2025, 1, 1, 10, 1, 0, 0, time.UTC),
		Duration:            time.Minute,
		HypothesisValidated: true,
		PreSteadyState:      SteadyStateResult{Met: true, Message: "通过"},
		PostSteadyState:     SteadyStateResult{Met: true, Message: "通过"},
	}

	report := FormatReport(result)
	if !strings.Contains(report, "混沌实验报告") {
		t.Error("报告应包含标题")
	}
	if !strings.Contains(report, "basic-report") {
		t.Error("报告应包含实验名称")
	}
	if !strings.Contains(report, "测试报告生成") {
		t.Error("报告应包含假设")
	}
	if !strings.Contains(report, "已验证") {
		t.Error("报告应包含验证状态")
	}
	if !strings.Contains(report, "completed") {
		t.Error("报告应包含实验状态")
	}
}

func TestFormatReport_WithDescription(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:        "desc-report",
			Description: "这是一个详细的描述",
			Hypothesis:  "假设内容",
		},
		Status:              StatusCompleted,
		StartTime:           time.Now(),
		EndTime:             time.Now(),
		HypothesisValidated: true,
	}

	report := FormatReport(result)
	if !strings.Contains(report, "这是一个详细的描述") {
		t.Error("报告应包含描述")
	}
}

func TestFormatReport_HypothesisNotValidated(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:       "failed-report",
			Hypothesis: "假设被推翻",
		},
		Status:              StatusFailed,
		StartTime:           time.Now(),
		EndTime:             time.Now(),
		HypothesisValidated: false,
	}

	report := FormatReport(result)
	if !strings.Contains(report, "未验证") {
		t.Error("报告应显示'未验证'")
	}
}

func TestFormatReport_WithFaultResults(t *testing.T) {
	now := time.Now()
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:       "fault-report",
			Hypothesis: "故障注入测试",
		},
		Status:    StatusCompleted,
		StartTime: now,
		EndTime:   now,
		FaultResults: []FaultResult{
			{
				FaultType:   "network_delay",
				Description: "延迟注入",
				Injected:    true,
				InjectTime:  now,
				CleanupTime: now.Add(time.Second),
			},
			{
				FaultType:   "cpu_stress",
				Description: "CPU 压力",
				Injected:    false,
				InjectTime:  now,
				Error:       fmt.Errorf("注入失败原因"),
			},
		},
		HypothesisValidated: true,
	}

	report := FormatReport(result)
	if !strings.Contains(report, "注入的故障") {
		t.Error("报告应包含故障注入部分")
	}
	if !strings.Contains(report, "network_delay") {
		t.Error("报告应包含故障类型")
	}
	if !strings.Contains(report, "成功") {
		t.Error("报告应显示成功状态")
	}
	if !strings.Contains(report, "失败") {
		t.Error("报告应显示失败状态")
	}
	if !strings.Contains(report, "注入失败原因") {
		t.Error("报告应包含错误信息")
	}
}

func TestFormatReport_WithSteadyState(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:        "steady-report",
			Hypothesis:  "稳态测试",
			SteadyState: NewAlwaysMetSteadyState(),
		},
		Status:    StatusCompleted,
		StartTime: time.Now(),
		EndTime:   time.Now(),
		PreSteadyState: SteadyStateResult{
			Met:     true,
			Message: "实验前满足",
			Details: map[string]any{"key": "value"},
		},
		PostSteadyState: SteadyStateResult{
			Met:     false,
			Message: "实验后不满足",
		},
		HypothesisValidated: false,
	}

	report := FormatReport(result)
	if !strings.Contains(report, "稳态检查") {
		t.Error("报告应包含稳态检查部分")
	}
	if !strings.Contains(report, "实验前") {
		t.Error("报告应包含实验前稳态")
	}
	if !strings.Contains(report, "实验后") {
		t.Error("报告应包含实验后稳态")
	}
	if !strings.Contains(report, "满足") {
		t.Error("报告应包含稳态状态")
	}
	if !strings.Contains(report, "key") {
		t.Error("报告应包含详情")
	}
}

func TestFormatReport_WithTags(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:       "tagged-report",
			Hypothesis: "标签测试",
			Tags:       []string{"production", "critical", "v2"},
		},
		Status:              StatusCompleted,
		StartTime:           time.Now(),
		EndTime:             time.Now(),
		HypothesisValidated: true,
	}

	report := FormatReport(result)
	if !strings.Contains(report, "标签") {
		t.Error("报告应包含标签部分")
	}
	if !strings.Contains(report, "production") {
		t.Error("报告应包含具体标签")
	}
	if !strings.Contains(report, "critical") {
		t.Error("报告应包含所有标签")
	}
}

func TestFormatReport_CleanupTimeZero(t *testing.T) {
	now := time.Now()
	result := &ExperimentResult{
		Experiment: Experiment{
			Name:       "cleanup-zero",
			Hypothesis: "清理时间为零",
		},
		Status:    StatusCompleted,
		StartTime: now,
		EndTime:   now,
		FaultResults: []FaultResult{
			{
				FaultType:   "test",
				Injected:    true,
				InjectTime:  now,
				CleanupTime: time.Time{}, // 零值
			},
		},
		HypothesisValidated: true,
	}

	report := FormatReport(result)
	// 零值清理时间应显示为 "-"
	if !strings.Contains(report, "-") {
		t.Error("零值清理时间应显示为 -")
	}
}

// ===== Summarize 测试 =====

func TestSummarize_Full(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name: "full-summary",
		},
		Status:              StatusCompleted,
		HypothesisValidated: true,
		Duration:            10 * time.Second,
		FaultResults: []FaultResult{
			{FaultType: "a", Injected: true},
			{FaultType: "b", Injected: true},
			{FaultType: "c", Injected: true},
		},
		PostSteadyState: SteadyStateResult{Met: true, Message: "ok"},
	}

	summary := Summarize(result)
	if summary.Name != "full-summary" {
		t.Errorf("Name = %s", summary.Name)
	}
	if summary.Status != StatusCompleted {
		t.Errorf("Status = %s", summary.Status)
	}
	if !summary.HypothesisValidated {
		t.Error("HypothesisValidated 应为 true")
	}
	if summary.Duration != 10*time.Second {
		t.Errorf("Duration = %v", summary.Duration)
	}
	if summary.FaultCount != 3 {
		t.Errorf("FaultCount = %d, 期望 3", summary.FaultCount)
	}
	if !summary.SteadyStateMet {
		t.Error("SteadyStateMet 应为 true")
	}
}

func TestSummarize_Failed(t *testing.T) {
	result := &ExperimentResult{
		Experiment: Experiment{
			Name: "failed-summary",
		},
		Status:              StatusFailed,
		HypothesisValidated: false,
		Duration:            2 * time.Second,
		FaultResults:        []FaultResult{},
		PostSteadyState:     SteadyStateResult{Met: false},
	}

	summary := Summarize(result)
	if summary.Status != StatusFailed {
		t.Errorf("Status = %s", summary.Status)
	}
	if summary.HypothesisValidated {
		t.Error("HypothesisValidated 应为 false")
	}
	if summary.FaultCount != 0 {
		t.Errorf("FaultCount = %d", summary.FaultCount)
	}
	if summary.SteadyStateMet {
		t.Error("SteadyStateMet 应为 false")
	}
}

// ===== FormatSummaryTable 测试 =====

func TestFormatSummaryTable_Basic(t *testing.T) {
	summaries := []ExperimentSummary{
		{
			Name:                "exp-1",
			Status:              StatusCompleted,
			HypothesisValidated: true,
			Duration:            5 * time.Second,
			FaultCount:          2,
			SteadyStateMet:      true,
		},
		{
			Name:                "exp-2",
			Status:              StatusFailed,
			HypothesisValidated: false,
			Duration:            3 * time.Second,
			FaultCount:          1,
			SteadyStateMet:      false,
		},
	}

	table := FormatSummaryTable(summaries)
	if !strings.Contains(table, "实验") {
		t.Error("表格应包含表头")
	}
	if !strings.Contains(table, "exp-1") {
		t.Error("表格应包含实验名称")
	}
	if !strings.Contains(table, "exp-2") {
		t.Error("表格应包含所有实验")
	}
	if !strings.Contains(table, "completed") {
		t.Error("表格应包含状态")
	}
}

func TestFormatSummaryTable_Empty(t *testing.T) {
	table := FormatSummaryTable(nil)
	// 即使为空也应包含表头
	if !strings.Contains(table, "实验") {
		t.Error("空表格也应包含表头")
	}
}

func TestFormatSummaryTable_AllValidated(t *testing.T) {
	summaries := []ExperimentSummary{
		{
			Name:                "all-pass",
			Status:              StatusCompleted,
			HypothesisValidated: true,
			SteadyStateMet:      true,
		},
	}
	table := FormatSummaryTable(summaries)
	lines := strings.Split(strings.TrimSpace(table), "\n")
	if len(lines) < 3 {
		t.Errorf("表格至少应有 3 行（表头+分隔+数据）, 得到 %d", len(lines))
	}
}

func TestFormatSummaryTable_MixedResults(t *testing.T) {
	summaries := []ExperimentSummary{
		{Name: "pass", HypothesisValidated: true, SteadyStateMet: true},
		{Name: "fail-hyp", HypothesisValidated: false, SteadyStateMet: true},
		{Name: "fail-steady", HypothesisValidated: true, SteadyStateMet: false},
		{Name: "fail-all", HypothesisValidated: false, SteadyStateMet: false},
	}
	table := FormatSummaryTable(summaries)
	if len(strings.Split(table, "\n")) < 6 {
		t.Error("表格行数不足")
	}
}
