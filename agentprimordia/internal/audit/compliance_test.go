package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPIICheck(t *testing.T) {
	t.Run("all-redacted", func(t *testing.T) {
		c := &PIICheck{PIIDetectedCount: 100, PIIRedactedCount: 100, TotalRequests: 1000}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !f.Passed {
			t.Error("expected pass")
		}
		if f.Severity != "info" {
			t.Errorf("severity = %v, want info", f.Severity)
		}
	})

	t.Run("partial-redacted", func(t *testing.T) {
		c := &PIICheck{PIIDetectedCount: 100, PIIRedactedCount: 50, TotalRequests: 1000}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if f.Passed {
			t.Error("expected fail")
		}
		if f.Severity != "high" {
			t.Errorf("severity = %v, want high", f.Severity)
		}
	})

	t.Run("no-requests", func(t *testing.T) {
		c := &PIICheck{TotalRequests: 0}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !f.Passed {
			t.Error("expected pass with no requests")
		}
	})

	t.Run("context-canceled", func(t *testing.T) {
		c := &PIICheck{TotalRequests: 10}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.Check(ctx)
		if err == nil {
			t.Error("expected error for canceled context")
		}
	})
}

func TestEncryptionCheck(t *testing.T) {
	t.Run("enabled-good-rotation", func(t *testing.T) {
		c := &EncryptionCheck{Enabled: true, Algorithm: "AES-256-GCM", KeyRotationDays: 30}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !f.Passed {
			t.Error("expected pass")
		}
	})

	t.Run("enabled-bad-rotation", func(t *testing.T) {
		c := &EncryptionCheck{Enabled: true, Algorithm: "AES-256-GCM", KeyRotationDays: 120}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if f.Passed {
			t.Error("expected fail for long rotation")
		}
		if f.Severity != "medium" {
			t.Errorf("severity = %v, want medium", f.Severity)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		c := &EncryptionCheck{Enabled: false}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if f.Passed {
			t.Error("expected fail")
		}
		if f.Severity != "critical" {
			t.Errorf("severity = %v, want critical", f.Severity)
		}
	})
}

func TestRetentionCheck(t *testing.T) {
	t.Run("policy-ok", func(t *testing.T) {
		c := &RetentionCheck{
			PolicyDefined:  true,
			PolicyDays:     90,
			LastCleanupAt:  time.Now().Add(-24 * time.Hour),
			OverdueRecords: 0,
		}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !f.Passed {
			t.Error("expected pass")
		}
	})

	t.Run("no-policy", func(t *testing.T) {
		c := &RetentionCheck{PolicyDefined: false}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if f.Passed {
			t.Error("expected fail")
		}
		if f.Severity != "high" {
			t.Errorf("severity = %v, want high", f.Severity)
		}
	})

	t.Run("overdue", func(t *testing.T) {
		c := &RetentionCheck{
			PolicyDefined:  true,
			PolicyDays:     90,
			LastCleanupAt:  time.Now().Add(-24 * time.Hour),
			OverdueRecords: 100,
		}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if f.Passed {
			t.Error("expected fail for overdue records")
		}
	})
}

func TestAccessControlCheck(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		c := &AccessControlCheck{TotalAccessEvents: 1000, DeniedEvents: 50, UniqueActors: 20, PrivilegedEvents: 10}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !f.Passed {
			t.Error("expected pass")
		}
	})

	t.Run("high-deny-rate", func(t *testing.T) {
		c := &AccessControlCheck{TotalAccessEvents: 100, DeniedEvents: 60, UniqueActors: 5}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if f.Passed {
			t.Error("expected fail for high deny rate")
		}
	})

	t.Run("no-events", func(t *testing.T) {
		c := &AccessControlCheck{TotalAccessEvents: 0}
		f, err := c.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !f.Passed {
			t.Error("expected pass with no events")
		}
	})
}

func TestGenerateReport(t *testing.T) {
	t.Run("all-pass", func(t *testing.T) {
		checks := []ComplianceCheck{
			&PIICheck{PIIDetectedCount: 100, PIIRedactedCount: 100, TotalRequests: 1000},
			&EncryptionCheck{Enabled: true, Algorithm: "AES-256-GCM", KeyRotationDays: 30},
			&AccessControlCheck{TotalAccessEvents: 100, DeniedEvents: 5, UniqueActors: 10},
		}
		report, err := GenerateReport(context.Background(), SOC2, checks)
		if err != nil {
			t.Fatal(err)
		}
		if report.Score != 100.0 {
			t.Errorf("score = %v, want 100", report.Score)
		}
		if len(report.Findings) != 3 {
			t.Errorf("findings = %d, want 3", len(report.Findings))
		}
		if report.Framework != SOC2 {
			t.Errorf("framework = %v, want SOC2", report.Framework)
		}
		if report.HasCriticalFindings() {
			t.Error("should not have critical findings")
		}
		if report.PassedChecks() != 3 {
			t.Errorf("passed = %d, want 3", report.PassedChecks())
		}
	})

	t.Run("partial-pass", func(t *testing.T) {
		checks := []ComplianceCheck{
			&PIICheck{PIIDetectedCount: 100, PIIRedactedCount: 50, TotalRequests: 1000},
			&EncryptionCheck{Enabled: true, Algorithm: "AES-256-GCM", KeyRotationDays: 30},
			&RetentionCheck{PolicyDefined: false},
			&AccessControlCheck{TotalAccessEvents: 100, DeniedEvents: 5, UniqueActors: 10},
		}
		report, err := GenerateReport(context.Background(), GDPR, checks)
		if err != nil {
			t.Fatal(err)
		}
		if report.Score != 50.0 {
			t.Errorf("score = %v, want 50", report.Score)
		}
		if report.PassedChecks() != 2 {
			t.Errorf("passed = %d, want 2", report.PassedChecks())
		}
	})

	t.Run("all-fail", func(t *testing.T) {
		checks := []ComplianceCheck{
			&EncryptionCheck{Enabled: false},
		}
		report, err := GenerateReport(context.Background(), MLPS, checks)
		if err != nil {
			t.Fatal(err)
		}
		if report.Score != 0.0 {
			t.Errorf("score = %v, want 0", report.Score)
		}
		if !report.HasCriticalFindings() {
			t.Error("should have critical findings")
		}
	})

	t.Run("empty-checks", func(t *testing.T) {
		report, err := GenerateReport(context.Background(), GDPR, nil)
		if err != nil {
			t.Fatal(err)
		}
		if report.Score != 0 {
			t.Errorf("score = %v, want 0", report.Score)
		}
		if len(report.Findings) != 0 {
			t.Errorf("findings = %d, want 0", len(report.Findings))
		}
	})

	t.Run("context-canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := GenerateReport(ctx, GDPR, []ComplianceCheck{&PIICheck{}})
		if err == nil {
			t.Error("expected error for canceled context")
		}
	})
}

func TestAssessmentReport_JSON(t *testing.T) {
	report := &AssessmentReport{
		Framework:   GDPR,
		Score:       85.5,
		GeneratedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Findings: []ComplianceFinding{
			{Check: "PII", Passed: true, Severity: "info"},
		},
		DataAccesses: []DataAccessRecord{
			{Actor: "user1", Resource: "/data", Action: "read", Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	report.SetPeriod(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))

	jsonStr, err := report.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if m["framework"] != "GDPR" {
		t.Errorf("framework = %v, want GDPR", m["framework"])
	}
	_, ok := m["score"].(float64)
	if !ok {
		t.Error("score should be float64")
	}

	// 验证 period 字段
	period, ok := m["period"].(map[string]any)
	if !ok {
		t.Error("period should be object")
	}
	if _, ok := period["start"]; !ok {
		t.Error("period.start missing")
	}

	// 验证 findings 字段
	findings, ok := m["findings"].([]any)
	if !ok || len(findings) != 1 {
		t.Error("findings should have 1 element")
	}

	// 验证 data_accesses 字段
	accesses, ok := m["data_accesses"].([]any)
	if !ok || len(accesses) != 1 {
		t.Error("data_accesses should have 1 element")
	}
}

func TestComplianceFramework_Constants(t *testing.T) {
	if GDPR != "GDPR" {
		t.Errorf("GDPR = %v", GDPR)
	}
	if SOC2 != "SOC2" {
		t.Errorf("SOC2 = %v", SOC2)
	}
	if MLPS != "MLPS" {
		t.Errorf("MLPS = %v", MLPS)
	}
}

func TestCustomCheck(t *testing.T) {
	// 测试自定义检查项
	customCheck := &mockComplianceCheck{
		name:   "Custom Security Check",
		passed: true,
		detail: "Everything is fine",
	}
	checks := []ComplianceCheck{customCheck}
	report, err := GenerateReport(context.Background(), SOC2, checks)
	// Actually it is fine:
	if err != nil {
		t.Fatal(err)
	}
	if report.Score != 100.0 {
		t.Errorf("score = %v, want 100", report.Score)
	}
}

type mockComplianceCheck struct {
	name   string
	passed bool
	detail string
}

func (m *mockComplianceCheck) Name() string { return m.name }
func (m *mockComplianceCheck) Check(ctx context.Context) (*ComplianceFinding, error) {
	return &ComplianceFinding{
		Check:    m.name,
		Passed:   m.passed,
		Detail:   m.detail,
		Severity: "info",
	}, nil
}
