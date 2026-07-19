// Package audit 提供审计日志与合规报告能力。
//
// 本文件实现合规报告生成，支持 GDPR、SOC2、等保（MLPS）等框架。
// 通过 ComplianceCheck 接口扩展自定义检查项。
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ComplianceFramework 合规框架类型。
type ComplianceFramework string

const (
	GDPR ComplianceFramework = "GDPR"
	SOC2 ComplianceFramework = "SOC2"
	MLPS ComplianceFramework = "MLPS"
)

// TimeRange 时间范围。
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ComplianceFinding 单个合规检查发现。
type ComplianceFinding struct {
	Check    string `json:"check"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

// DataAccessRecord 数据访问记录。
type DataAccessRecord struct {
	Actor     string    `json:"actor"`
	Resource  string    `json:"resource"`
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

// AssessmentReport 合规评估报告。
type AssessmentReport struct {
	Framework    ComplianceFramework `json:"framework"`
	Period       TimeRange           `json:"period"`
	Findings     []ComplianceFinding `json:"findings"`
	DataAccesses []DataAccessRecord  `json:"data_accesses"`
	Score        float64             `json:"score"`
	GeneratedAt  time.Time           `json:"generated_at"`
}

// ComplianceCheck 合规检查接口。
type ComplianceCheck interface {
	Name() string
	Check(ctx context.Context) (*ComplianceFinding, error)
}

// === 内置检查项 ===

// PIICheck 检查 PII 是否被正确检测和脱敏。
type PIICheck struct {
	PIIDetectedCount int
	PIIRedactedCount int
	TotalRequests    int
}

func (c *PIICheck) Name() string { return "PII Detection & Redaction" }

func (c *PIICheck) Check(ctx context.Context) (*ComplianceFinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.TotalRequests == 0 {
		return &ComplianceFinding{
			Check:    c.Name(),
			Passed:   true,
			Detail:   "No requests to evaluate",
			Severity: "info",
		}, nil
	}
	redactionRate := float64(c.PIIRedactedCount) / float64(max(c.PIIDetectedCount, 1))
	passed := redactionRate >= 0.95
	detail := "PII redaction rate: " + formatPercent(redactionRate) +
		" (" + itoa(c.PIIRedactedCount) + "/" + itoa(c.PIIDetectedCount) + ")"
	severity := "info"
	if !passed {
		severity = "high"
	}
	return &ComplianceFinding{
		Check:    c.Name(),
		Passed:   passed,
		Detail:   detail,
		Severity: severity,
	}, nil
}

// EncryptionCheck 检查静态加密是否启用。
type EncryptionCheck struct {
	Enabled         bool
	Algorithm       string
	KeyRotationDays int
}

func (c *EncryptionCheck) Name() string { return "Encryption at Rest" }

func (c *EncryptionCheck) Check(ctx context.Context) (*ComplianceFinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !c.Enabled {
		return &ComplianceFinding{
			Check:    c.Name(),
			Passed:   false,
			Detail:   "Encryption at rest is not enabled",
			Severity: "critical",
		}, nil
	}
	passed := true
	detail := "Encryption enabled: " + c.Algorithm
	severity := "info"
	if c.KeyRotationDays > 90 {
		passed = false
		detail += ", key rotation period exceeds 90 days (" + itoa(c.KeyRotationDays) + " days)"
		severity = "medium"
	}
	return &ComplianceFinding{
		Check:    c.Name(),
		Passed:   passed,
		Detail:   detail,
		Severity: severity,
	}, nil
}

// RetentionCheck 检查数据保留策略是否执行。
type RetentionCheck struct {
	PolicyDefined  bool
	PolicyDays     int
	LastCleanupAt  time.Time
	OverdueRecords int
}

func (c *RetentionCheck) Name() string { return "Data Retention Policy" }

func (c *RetentionCheck) Check(ctx context.Context) (*ComplianceFinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !c.PolicyDefined {
		return &ComplianceFinding{
			Check:    c.Name(),
			Passed:   false,
			Detail:   "No data retention policy defined",
			Severity: "high",
		}, nil
	}
	daysSinceCleanup := 9999
	if !c.LastCleanupAt.IsZero() {
		daysSinceCleanup = int(time.Since(c.LastCleanupAt).Hours() / 24)
	}
	passed := c.OverdueRecords == 0 && daysSinceCleanup < c.PolicyDays
	detail := "Policy: " + itoa(c.PolicyDays) + " days, last cleanup: " + itoa(daysSinceCleanup) + " days ago"
	severity := "info"
	if !passed {
		severity = "medium"
		if c.OverdueRecords > 0 {
			detail += ", " + itoa(c.OverdueRecords) + " overdue records"
		}
	}
	return &ComplianceFinding{
		Check:    c.Name(),
		Passed:   passed,
		Detail:   detail,
		Severity: severity,
	}, nil
}

// AccessControlCheck 检查 ACL 访问控制。
type AccessControlCheck struct {
	TotalAccessEvents int
	DeniedEvents      int
	UniqueActors      int
	PrivilegedEvents  int
}

func (c *AccessControlCheck) Name() string { return "Access Control (ACL)" }

func (c *AccessControlCheck) Check(ctx context.Context) (*ComplianceFinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.TotalAccessEvents == 0 {
		return &ComplianceFinding{
			Check:    c.Name(),
			Passed:   true,
			Detail:   "No access events to evaluate",
			Severity: "info",
		}, nil
	}
	denyRate := float64(c.DeniedEvents) / float64(c.TotalAccessEvents)
	passed := denyRate < 0.5 && c.UniqueActors > 0
	detail := "Actors: " + itoa(c.UniqueActors) +
		", deny rate: " + formatPercent(denyRate) +
		", privileged ops: " + itoa(c.PrivilegedEvents)
	severity := "info"
	if !passed {
		severity = "high"
	}
	return &ComplianceFinding{
		Check:    c.Name(),
		Passed:   passed,
		Detail:   detail,
		Severity: severity,
	}, nil
}

// === 报告生成 ===

// GenerateReport 根据指定的合规框架和检查项生成报告。
func GenerateReport(ctx context.Context, framework ComplianceFramework, checks []ComplianceCheck) (*AssessmentReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report := &AssessmentReport{
		Framework:    framework,
		Findings:     make([]ComplianceFinding, 0, len(checks)),
		DataAccesses: make([]DataAccessRecord, 0),
		GeneratedAt:  time.Now(),
	}
	if len(checks) == 0 {
		return report, nil
	}
	passed := 0
	for _, c := range checks {
		finding, err := c.Check(ctx)
		if err != nil {
			return nil, errors.New("compliance check " + c.Name() + " failed: " + err.Error())
		}
		report.Findings = append(report.Findings, *finding)
		if finding.Passed {
			passed++
		}
	}
	report.Score = float64(passed) / float64(len(checks)) * 100.0
	return report, nil
}

// SetPeriod 设置报告的时间范围。
func (r *AssessmentReport) SetPeriod(start, end time.Time) {
	r.Period = TimeRange{Start: start, End: end}
}

// AddDataAccess 添加数据访问记录。
func (r *AssessmentReport) AddDataAccess(record DataAccessRecord) {
	r.DataAccesses = append(r.DataAccesses, record)
}

// ToJSON 将报告序列化为 JSON 字符串。
func (r *AssessmentReport) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// HasCriticalFindings 判断是否有 critical 级别的发现。
func (r *AssessmentReport) HasCriticalFindings() bool {
	for _, f := range r.Findings {
		if f.Severity == "critical" {
			return true
		}
	}
	return false
}

// PassedChecks 返回通过的检查项数量。
func (r *AssessmentReport) PassedChecks() int {
	count := 0
	for _, f := range r.Findings {
		if f.Passed {
			count++
		}
	}
	return count
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func formatPercent(v float64) string {
	return itoa(int(v*100+0.5)) + "%"
}
