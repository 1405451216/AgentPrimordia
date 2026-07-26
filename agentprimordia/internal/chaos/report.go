package chaos

import (
	"fmt"
	"strings"
	"time"
)

// ===== 实验报告生成器 =====

// FormatReport 将实验结果格式化为 Markdown 报告
func FormatReport(result *ExperimentResult) string {
	var sb strings.Builder

	sb.WriteString("# 混沌实验报告\n\n")
	sb.WriteString(fmt.Sprintf("**实验名称**: %s\n\n", result.Experiment.Name))

	if result.Experiment.Description != "" {
		sb.WriteString(fmt.Sprintf("**描述**: %s\n\n", result.Experiment.Description))
	}

	sb.WriteString(fmt.Sprintf("**假设**: %s\n\n", result.Experiment.Hypothesis))

	statusEmoji := "✅"
	if !result.HypothesisValidated {
		statusEmoji = "❌"
	}

	sb.WriteString(fmt.Sprintf("**假设验证**: %s %s\n\n",
		statusEmoji,
		map[bool]string{true: "已验证", false: "未验证"}[result.HypothesisValidated],
	))

	sb.WriteString(fmt.Sprintf("**状态**: %s\n\n", result.Status))
	sb.WriteString(fmt.Sprintf("**持续时间**: %v\n\n", result.Duration))
	sb.WriteString(fmt.Sprintf("**开始时间**: %s\n\n", result.StartTime.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**结束时间**: %s\n\n", result.EndTime.Format(time.RFC3339)))

	// 故障注入结果
	if len(result.FaultResults) > 0 {
		sb.WriteString("## 注入的故障\n\n")
		sb.WriteString("| # | 类型 | 描述 | 注入状态 | 注入时间 | 清理时间 | 错误 |\n")
		sb.WriteString("|---|------|------|----------|----------|----------|------|\n")
		for i, fr := range result.FaultResults {
			injected := "✅ 成功"
			if !fr.Injected {
				injected = "❌ 失败"
			}
			errMsg := ""
			if fr.Error != nil {
				errMsg = fr.Error.Error()
			}
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s |\n",
				i+1,
				fr.FaultType,
				fr.Description,
				injected,
				fr.InjectTime.Format("15:04:05"),
				func() string {
					if fr.CleanupTime.IsZero() {
						return "-"
					}
					return fr.CleanupTime.Format("15:04:05")
				}(),
				errMsg,
			))
		}
		sb.WriteString("\n")
	}

	// 稳态检查
	if result.Experiment.SteadyState != nil {
		sb.WriteString("## 稳态检查\n\n")
		sb.WriteString(fmt.Sprintf("**稳态条件**: %s\n\n", result.Experiment.SteadyState.Name()))

		sb.WriteString("### 实验前\n\n")
		formatSteadyStateResult(&sb, &result.PreSteadyState)

		sb.WriteString("### 实验后\n\n")
		formatSteadyStateResult(&sb, &result.PostSteadyState)
	}

	// 标签
	if len(result.Experiment.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("\n**标签**: %s\n", strings.Join(result.Experiment.Tags, ", ")))
	}

	return sb.String()
}

func formatSteadyStateResult(sb *strings.Builder, r *SteadyStateResult) {
	metEmoji := "✅"
	if !r.Met {
		metEmoji = "❌"
	}
	sb.WriteString(fmt.Sprintf("- **状态**: %s %s\n", metEmoji,
		map[bool]string{true: "满足", false: "不满足"}[r.Met]))
	sb.WriteString(fmt.Sprintf("- **消息**: %s\n", r.Message))
	if len(r.Details) > 0 {
		sb.WriteString("- **详情**:\n")
		for k, v := range r.Details {
			sb.WriteString(fmt.Sprintf("  - %s: %v\n", k, v))
		}
	}
	sb.WriteString("\n")
}

// ExperimentSummary 实验摘要（用于批量报告）
type ExperimentSummary struct {
	Name                string
	Status              ExperimentStatus
	HypothesisValidated bool
	Duration            time.Duration
	FaultCount          int
	SteadyStateMet      bool
}

// Summarize 生成实验摘要
func Summarize(result *ExperimentResult) ExperimentSummary {
	return ExperimentSummary{
		Name:                result.Experiment.Name,
		Status:              result.Status,
		HypothesisValidated: result.HypothesisValidated,
		Duration:            result.Duration,
		FaultCount:          len(result.FaultResults),
		SteadyStateMet:      result.PostSteadyState.Met,
	}
}

// FormatSummaryTable 将多个实验摘要格式化为表格
func FormatSummaryTable(summaries []ExperimentSummary) string {
	var sb strings.Builder
	sb.WriteString("| 实验 | 状态 | 假设 | 持续时间 | 故障数 | 稳态 |\n")
	sb.WriteString("|------|------|------|----------|--------|------|\n")
	for _, s := range summaries {
		validated := "✅"
		if !s.HypothesisValidated {
			validated = "❌"
		}
		met := "✅"
		if !s.SteadyStateMet {
			met = "❌"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %v | %d | %s |\n",
			s.Name, s.Status, validated, s.Duration, s.FaultCount, met))
	}
	return sb.String()
}
