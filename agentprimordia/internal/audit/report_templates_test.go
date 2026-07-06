package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestIsValidReportTemplate 测试模板有效性检查
func TestIsValidReportTemplate(t *testing.T) {
	cases := []struct {
		t    ReportTemplate
		want bool
	}{
		{TemplateSOC2, true},
		{TemplateGDPR, true},
		{TemplateCustom, true},
		{ReportTemplate("unknown"), false},
		{ReportTemplate(""), false},
	}
	for _, c := range cases {
		if got := IsValidReportTemplate(c.t); got != c.want {
			t.Errorf("IsValidReportTemplate(%q) = %v, 期望 %v", c.t, got, c.want)
		}
	}
}

// TestGenerateComplianceReport_SOC2 测试 SOC2 报告
func TestGenerateComplianceReport_SOC2(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	// 写入测试事件
	events := []Event{
		{Actor: "agent-1", Action: "file.read", Resource: "/data/a", Result: "success"},
		{Actor: "agent-1", Action: "file.write", Resource: "/data/b", Result: "success"},
		{Actor: "agent-1", Action: "tool.call", Resource: "search", Result: "success"},
		{Actor: "agent-2", Action: "file.read", Resource: "/data/c", Result: "denied"},
		{Actor: "agent-2", Action: "file.delete", Resource: "/data/d", Result: "blocked"},
		{Actor: "agent-3", Action: "config.change", Resource: "/etc/cfg", Result: "success"},
	}
	for _, e := range events {
		if err := logger.Log(ctx, e); err != nil {
			t.Fatalf("Log 失败: %v", err)
		}
	}

	now := time.Now()
	report, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: TemplateSOC2,
		Start:    now.Add(-1 * time.Hour),
		End:      now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("GenerateComplianceReport 失败: %v", err)
	}
	soc2, ok := report.(*SOC2ReportSummary)
	if !ok {
		t.Fatalf("SOC2 报告类型断言失败: %T", report)
	}
	// 总事件
	if soc2.TotalEvents != 6 {
		t.Errorf("TotalEvents = %d, 期望 6", soc2.TotalEvents)
	}
	// 访问控制事件：file.read、file.write、tool.call、file.read、file.delete = 5
	if soc2.AccessControlEvents != 5 {
		t.Errorf("AccessControlEvents = %d, 期望 5", soc2.AccessControlEvents)
	}
	// 被拒绝：denied(1) + blocked(1) = 2
	if soc2.DeniedActions != 2 {
		t.Errorf("DeniedActions = %d, 期望 2", soc2.DeniedActions)
	}
	// 异常：denied + blocked = 2
	if soc2.ErrorEvents != 2 {
		t.Errorf("ErrorEvents = %d, 期望 2", soc2.ErrorEvents)
	}
	// 独立 Actor
	if soc2.UniqueActors != 3 {
		t.Errorf("UniqueActors = %d, 期望 3", soc2.UniqueActors)
	}
}

// TestGenerateComplianceReport_GDPR 测试 GDPR 报告
func TestGenerateComplianceReport_GDPR(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	events := []Event{
		{Actor: "agent-1", Action: "pii.detect", Resource: "user_email", Result: "success"},
		{Actor: "agent-1", Action: "file.read", Resource: "personal_data.csv", Result: "success"},
		{Actor: "dsar.user-1", Action: "memory.read", Resource: "user-1", Result: "success"},
		{Actor: "agent-2", Action: "guardrail.sanitize", Resource: "pii", Result: "sanitized"},
		{Actor: "system", Action: "memory.cleanup", Resource: "expired", Result: "success"},
		{Actor: "agent-3", Action: "file.read", Resource: "/data/a", Result: "success"},
	}
	for _, e := range events {
		if err := logger.Log(ctx, e); err != nil {
			t.Fatalf("Log 失败: %v", err)
		}
	}

	now := time.Now()
	report, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: TemplateGDPR,
		Start:    now.Add(-1 * time.Hour),
		End:      now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("GenerateComplianceReport 失败: %v", err)
	}
	gdpr, ok := report.(*GDPRReportSummary)
	if !ok {
		t.Fatalf("GDPR 报告类型断言失败: %T", report)
	}
	if gdpr.TotalEvents != 6 {
		t.Errorf("TotalEvents = %d, 期望 6", gdpr.TotalEvents)
	}
	// PII 访问：pii.detect、file.read@personal_data.csv、guardrail.sanitize@pii = 3
	if gdpr.PIIAccessEvents != 3 {
		t.Errorf("PIIAccessEvents = %d, 期望 3", gdpr.PIIAccessEvents)
	}
	// 脱敏：guardrail.sanitize = 1
	if gdpr.SanitizationEvents != 1 {
		t.Errorf("SanitizationEvents = %d, 期望 1", gdpr.SanitizationEvents)
	}
	// DSAR
	if gdpr.DataSubjectAccessEvents != 1 {
		t.Errorf("DataSubjectAccessEvents = %d, 期望 1", gdpr.DataSubjectAccessEvents)
	}
	// 保留策略
	if gdpr.RetentionPolicyApplied != 1 {
		t.Errorf("RetentionPolicyApplied = %d, 期望 1", gdpr.RetentionPolicyApplied)
	}
}

// TestGenerateComplianceReport_Custom 测试自定义模板
func TestGenerateComplianceReport_Custom(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	_ = logger.Log(ctx, Event{Actor: "a1", Action: "x", Result: "ok"})
	_ = logger.Log(ctx, Event{Actor: "a2", Action: "y", Result: "ok"})

	now := time.Now()
	report, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: TemplateCustom,
		Start:    now.Add(-time.Hour),
		End:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GenerateComplianceReport 失败: %v", err)
	}
	base, ok := report.(*ComplianceReport)
	if !ok {
		t.Fatalf("Custom 模板应返回基础报告: %T", report)
	}
	if base.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, 期望 2", base.TotalEvents)
	}
}

// TestGenerateComplianceReport_ActorFilter 测试 Actor 筛选
func TestGenerateComplianceReport_ActorFilter(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	_ = logger.Log(ctx, Event{Actor: "a1", Action: "x", Result: "ok"})
	_ = logger.Log(ctx, Event{Actor: "a2", Action: "x", Result: "ok"})
	_ = logger.Log(ctx, Event{Actor: "a3", Action: "x", Result: "ok"})

	now := time.Now()
	report, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: TemplateCustom,
		Start:    now.Add(-time.Hour),
		End:      now.Add(time.Hour),
		Actors:   []string{"a1", "a3"},
	})
	if err != nil {
		t.Fatalf("GenerateComplianceReport 失败: %v", err)
	}
	base := report.(*ComplianceReport)
	if base.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, 期望 2 (a1+a3)", base.TotalEvents)
	}
	if _, ok := base.ActorStats["a2"]; ok {
		t.Error("a2 不应在筛选后的报告里")
	}
}

// TestGenerateComplianceReport_InvalidTemplate 测试非法模板
func TestGenerateComplianceReport_InvalidTemplate(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	_, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: ReportTemplate("nope"),
		Start:    time.Now(),
		End:      time.Now(),
	})
	if !errors.Is(err, ErrInvalidTemplate) {
		t.Errorf("错误类型 = %v, 期望 ErrInvalidTemplate", err)
	}
}

// TestGenerateComplianceReport_MissingPeriod 测试缺少时间范围
func TestGenerateComplianceReport_MissingPeriod(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	_, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: TemplateSOC2,
	})
	if !errors.Is(err, ErrMissingPeriod) {
		t.Errorf("错误类型 = %v, 期望 ErrMissingPeriod", err)
	}
}

// TestGenerateComplianceReport_EmptyEvents 测试空事件
func TestGenerateComplianceReport_EmptyEvents(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	now := time.Now()
	report, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: TemplateSOC2,
		Start:    now.Add(-time.Hour),
		End:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GenerateComplianceReport 失败: %v", err)
	}
	soc2 := report.(*SOC2ReportSummary)
	if soc2.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, 期望 0", soc2.TotalEvents)
	}
	if soc2.UniqueActors != 0 {
		t.Errorf("UniqueActors = %d, 期望 0", soc2.UniqueActors)
	}
}

// TestSOC2Report_ExportJSON 测试 SOC2 报告 JSON 序列化
func TestSOC2Report_ExportJSON(t *testing.T) {
	out := newMemoryOutput()
	logger, _ := NewLogger(LoggerConfig{Output: out})
	ctx := context.Background()

	_ = logger.Log(ctx, Event{Actor: "a1", Action: "file.read", Result: "success"})

	now := time.Now()
	report, err := logger.GenerateComplianceReport(ctx, ReportConfig{
		Template: TemplateSOC2,
		Start:    now.Add(-time.Hour),
		End:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GenerateComplianceReport 失败: %v", err)
	}
	soc2 := report.(*SOC2ReportSummary)
	jsonStr, err := soc2.ComplianceReport.ExportJSON()
	if err != nil {
		t.Fatalf("ExportJSON 失败: %v", err)
	}
	if !contains(jsonStr, "total_events") {
		t.Error("SOC2 JSON 缺少 total_events")
	}
	if !contains(jsonStr, "access_control_events") && !contains(jsonStr, "actor_stats") {
		t.Error("SOC2 JSON 缺少关键字段")
	}
}

// TestHelperFunctions 测试辅助函数
func TestHelperFunctions(t *testing.T) {
	if !isAccessControlAction("file.read") {
		t.Error("file.read 应为访问控制类")
	}
	if isAccessControlAction("config.change") {
		t.Error("config.change 不应为访问控制类")
	}
	if !isDeniedOrBlocked("denied") {
		t.Error("denied 应被识别")
	}
	if !isErrorOrBlocked("error") {
		t.Error("error 应被识别")
	}
	if !isPIIAccessAction("pii.detect") {
		t.Error("pii.detect 应被识别为 PII")
	}
	if isPIIAccessAction("file.read") {
		t.Error("file.read 不应为 PII")
	}
	if !isPIIResource("user_pii_data") {
		t.Error("user_pii_data 应被识别")
	}
	if isPIIResource("/data/file") {
		t.Error("/data/file 不应匹配 PII")
	}
	if !isDSAREvent("dsar.user-1") {
		t.Error("dsar.user-1 应被识别")
	}
	if isDSAREvent("agent-1") {
		t.Error("agent-1 不应为 DSAR")
	}
	if !isRetentionEvent("memory.cleanup") {
		t.Error("memory.cleanup 应被识别")
	}
	if !isRetentionEvent("memory.retention.apply") {
		t.Error("memory.retention.apply 应被识别")
	}
	if isRetentionEvent("file.read") {
		t.Error("file.read 不应为保留策略")
	}
}

// TestContainsKeyword 测试关键字匹配
func TestContainsKeyword(t *testing.T) {
	if !containsKeyword("hello world", "world") {
		t.Error("应匹配 world")
	}
	if containsKeyword("hello", "world") {
		t.Error("不应匹配 world")
	}
	if !containsKeyword("anything", "") {
		t.Error("空关键字应始终匹配")
	}
	if containsKeyword("", "x") {
		t.Error("空字符串不应包含 x")
	}
}