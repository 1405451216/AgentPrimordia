package observability

import "testing"

func TestAlertRuleInterface(t *testing.T) {
	var _ AlertRule = &thresholdRule{}
}

func TestAlertSeverity(t *testing.T) {
	severities := []AlertSeverity{SeverityInfo, SeverityWarning, SeverityCritical}
	for _, s := range severities {
		if s == "" {
			t.Error("severity should not be empty")
		}
	}
}

func TestAlertEvent(t *testing.T) {
	ae := AlertEvent{
		Rule:     "high_error_rate",
		Severity: SeverityCritical,
		Message:  "error rate > 50%",
	}
	if ae.Rule == "" || ae.Severity == "" || ae.Message == "" {
		t.Error("alert event fields should be populated")
	}
}
