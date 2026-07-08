package audit

import (
	"context"
	"errors"
	"time"
)

// ReportTemplate 合规报告模板类型。
type ReportTemplate string

const (
	// TemplateSOC2 SOC2（Service Organization Control 2）合规模板
	TemplateSOC2 ReportTemplate = "soc2"
	// TemplateGDPR GDPR（通用数据保护条例）合规模板
	TemplateGDPR ReportTemplate = "gdpr"
	// TemplateCustom 自定义模板（等同于基础统计报告）
	TemplateCustom ReportTemplate = "custom"
)

// IsValidReportTemplate 检查是否为已知的报告模板。
func IsValidReportTemplate(t ReportTemplate) bool {
	switch t {
	case TemplateSOC2, TemplateGDPR, TemplateCustom:
		return true
	default:
		return false
	}
}

// ReportConfig 报告生成配置。
type ReportConfig struct {
	// Template 报告模板类型
	Template ReportTemplate
	// Start 报告起始时间
	Start time.Time
	// End 报告结束时间
	End time.Time
	// Actors 限定报告的 Actor 列表；空表示全部
	Actors []string
	// IncludeDetails 是否在报告中包含每个事件的明细（可能很大）
	IncludeDetails bool
}

// 通用错误
var (
	ErrInvalidTemplate = errors.New("audit: invalid report template")
	ErrMissingPeriod   = errors.New("audit: report period (start, end) is required")
)

// SOC2ReportSummary SOC2 报告摘要，包含访问控制关键指标。
type SOC2ReportSummary struct {
	ComplianceReport
	// AccessControlEvents 访问控制相关事件数（file.read、file.write 等）
	AccessControlEvents int `json:"access_control_events"`
	// DeniedActions 被拒绝的操作次数
	DeniedActions int `json:"denied_actions"`
	// ErrorEvents 异常事件数（error/blocked/denied）
	ErrorEvents int `json:"error_events"`
	// UniqueActors 独立 Actor 数
	UniqueActors int `json:"unique_actors"`
}

// GDPRReportSummary GDPR 报告摘要，包含数据保护相关指标。
type GDPRReportSummary struct {
	ComplianceReport
	// PIIAccessEvents 涉及 PII 资源访问的事件数（resource 匹配 pii.* 或 action 是 pii.*）
	PIIAccessEvents int `json:"pii_access_events"`
	// SanitizationEvents 脱敏事件数
	SanitizationEvents int `json:"sanitization_events"`
	// DataSubjectAccessEvents 数据主体访问请求事件数（actor 含 dsar.* 前缀）
	DataSubjectAccessEvents int `json:"data_subject_access_events"`
	// RetentionPolicyApplied 数据保留策略执行次数
	RetentionPolicyApplied int `json:"retention_policy_applied"`
}

// GenerateComplianceReport 根据模板生成合规报告。
func (l *Logger) GenerateComplianceReport(ctx context.Context, cfg ReportConfig) (any, error) {
	if !IsValidReportTemplate(cfg.Template) {
		return nil, ErrInvalidTemplate
	}
	if cfg.Start.IsZero() || cfg.End.IsZero() {
		return nil, ErrMissingPeriod
	}

	// 基础过滤
	filter := QueryFilter{Start: cfg.Start, End: cfg.End}
	events, err := l.config.Output.Query(filter)
	if err != nil {
		return nil, err
	}
	// 应用 Actor 筛选
	if len(cfg.Actors) > 0 {
		events = filterByActors(events, cfg.Actors)
	}

	switch cfg.Template {
	case TemplateSOC2:
		return buildSOC2Report(cfg, events), nil
	case TemplateGDPR:
		return buildGDPRReport(cfg, events), nil
	default:
		// 自定义：返回基础报告
		return buildBaseReport(cfg, events), nil
	}
}

// filterByActors 按 Actor 列表过滤事件。
func filterByActors(events []Event, actors []string) []Event {
	set := make(map[string]struct{}, len(actors))
	for _, a := range actors {
		set[a] = struct{}{}
	}
	out := make([]Event, 0, len(events))
	for _, e := range events {
		if _, ok := set[e.Actor]; ok {
			out = append(out, e)
		}
	}
	return out
}

// buildBaseReport 构造基础合规报告。
func buildBaseReport(cfg ReportConfig, events []Event) *ComplianceReport {
	report := &ComplianceReport{
		Period:      PeriodStats{Start: cfg.Start, End: cfg.End},
		TotalEvents: len(events),
		ActorStats:  make(map[string]ActorStats),
		ActionStats: make(map[string]int),
	}
	fillReportDetails(report, events)
	return report
}

// fillReportDetails 填充 ActorStats、ActionStats 以及可选的事件明细。
func fillReportDetails(report *ComplianceReport, events []Event) {
	for _, e := range events {
		as, ok := report.ActorStats[e.Actor]
		if !ok {
			as = ActorStats{Actions: make(map[string]int)}
		}
		as.TotalActions++
		as.Actions[e.Action]++
		report.ActorStats[e.Actor] = as
		report.ActionStats[e.Action]++
	}
}

// buildSOC2Report 构造 SOC2 报告。
//
// SOC2 关注：访问控制、操作审计、异常检测。
// 关键指标：
//   - 访问控制事件（file.* 资源访问类）
//   - 被拒绝的操作（result=denied/blocked）
//   - 异常事件（error/blocked/denied）
//   - 独立 Actor 数
func buildSOC2Report(cfg ReportConfig, events []Event) *SOC2ReportSummary {
	base := buildBaseReport(cfg, events)
	summary := &SOC2ReportSummary{ComplianceReport: *base}

	seenActors := make(map[string]struct{})
	for _, e := range events {
		// 访问控制事件：资源以 file.、http. 或包含权限字样
		if isAccessControlAction(e.Action) {
			summary.AccessControlEvents++
		}
		// 拒绝/拦截事件
		if isDeniedOrBlocked(e.Result) {
			summary.DeniedActions++
		}
		// 异常事件
		if isErrorOrBlocked(e.Result) {
			summary.ErrorEvents++
		}
		seenActors[e.Actor] = struct{}{}
	}
	summary.UniqueActors = len(seenActors)
	return summary
}

// buildGDPRReport 构造 GDPR 报告。
//
// GDPR 关注：个人数据处理、PII 脱敏记录、数据主体权利。
// 关键指标：
//   - 涉及 PII 的访问事件
//   - 脱敏事件
//   - 数据主体访问请求（DSAR）
//   - 数据保留策略执行
func buildGDPRReport(cfg ReportConfig, events []Event) *GDPRReportSummary {
	base := buildBaseReport(cfg, events)
	summary := &GDPRReportSummary{ComplianceReport: *base}

	for _, e := range events {
		if isPIIAccessAction(e.Action) || isPIIResource(e.Resource) {
			summary.PIIAccessEvents++
		}
		if e.Result == "sanitized" || e.Action == "guardrail.sanitize" {
			summary.SanitizationEvents++
		}
		if isDSAREvent(e.Actor) {
			summary.DataSubjectAccessEvents++
		}
		if isRetentionEvent(e.Action) {
			summary.RetentionPolicyApplied++
		}
	}
	return summary
}

// isAccessControlAction 判断 action 是否为访问控制类。
func isAccessControlAction(action string) bool {
	switch action {
	case "file.read", "file.write", "file.delete",
		"tool.call", "tool.result", "http.request":
		return true
	default:
		return false
	}
}

// isDeniedOrBlocked 判断结果是否为 denied/blocked。
func isDeniedOrBlocked(result string) bool {
	return result == "denied" || result == "blocked"
}

// isErrorOrBlocked 判断结果是否为异常。
func isErrorOrBlocked(result string) bool {
	return result == "error" || result == "denied" || result == "blocked"
}

// isPIIAccessAction 判断 action 是否涉及 PII。
func isPIIAccessAction(action string) bool {
	return len(action) >= 4 && action[:4] == "pii."
}

// isPIIResource 判断 resource 是否包含 PII 关键字。
func isPIIResource(resource string) bool {
	if len(resource) == 0 {
		return false
	}
	// 简单包含匹配
	if containsKeyword(resource, "pii") || containsKeyword(resource, "personal") {
		return true
	}
	return false
}

// isDSAREvent 判断 actor 是否为 DSAR（数据主体访问请求）类。
func isDSAREvent(actor string) bool {
	return len(actor) >= 5 && actor[:5] == "dsar."
}

// isRetentionEvent 判断 action 是否为数据保留策略类。
func isRetentionEvent(action string) bool {
	return action == "memory.cleanup" || action == "memory.retention.apply"
}

// containsKeyword 简单包含匹配。
func containsKeyword(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
