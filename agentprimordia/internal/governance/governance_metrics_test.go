package governance

import (
	"testing"
)

func TestNewGovernanceMetrics(t *testing.T) {
	m := NewGovernanceMetrics()
	if m == nil {
		t.Fatal("NewGovernanceMetrics returned nil")
	}
}

func TestGovernanceMetrics_RecordToolCall(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordToolCall("agent1", "tool1")
	m.RecordToolCall("agent1", "tool1")

	snap := m.Snapshot()
	if snap.ToolCallsTotal != 2 {
		t.Errorf("ToolCallsTotal = %d, want 2", snap.ToolCallsTotal)
	}
	if snap.ToolCallsByLabel["agent1|tool1"] != 2 {
		t.Errorf("ToolCallsByLabel[agent1|tool1] = %d, want 2", snap.ToolCallsByLabel["agent1|tool1"])
	}
}

func TestGovernanceMetrics_RecordToolBlocked(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordToolBlocked("agent1", "tool1", "rate_limit")

	snap := m.Snapshot()
	if snap.ToolBlockedTotal != 1 {
		t.Errorf("ToolBlockedTotal = %d, want 1", snap.ToolBlockedTotal)
	}
}

func TestGovernanceMetrics_RecordCost(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordCost(0.05)
	m.RecordCost(0.03)

	snap := m.Snapshot()
	if snap.CostUSDTotal < 0.079 || snap.CostUSDTotal > 0.081 {
		t.Errorf("CostUSDTotal = %f, want ~0.08", snap.CostUSDTotal)
	}
}

func TestGovernanceMetrics_RecordCostExceeded(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordCostExceeded()

	snap := m.Snapshot()
	if snap.CostExceededTotal != 1 {
		t.Errorf("CostExceededTotal = %d, want 1", snap.CostExceededTotal)
	}
}

func TestGovernanceMetrics_RecordOutputBlocked(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordOutputBlocked()

	snap := m.Snapshot()
	if snap.OutputBlockedTotal != 1 {
		t.Errorf("OutputBlockedTotal = %d, want 1", snap.OutputBlockedTotal)
	}
}

func TestGovernanceMetrics_RecordPIIDetected(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordPIIDetected()

	snap := m.Snapshot()
	if snap.PIIDetectedTotal != 1 {
		t.Errorf("PIIDetectedTotal = %d, want 1", snap.PIIDetectedTotal)
	}
}

func TestGovernanceMetrics_RecordPolicyHotSwap(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordPolicyHotSwap()

	snap := m.Snapshot()
	if snap.PolicyHotSwapsTotal != 1 {
		t.Errorf("PolicyHotSwapsTotal = %d, want 1", snap.PolicyHotSwapsTotal)
	}
}

func TestGovernanceMetrics_SetPolicyVersion(t *testing.T) {
	m := NewGovernanceMetrics()
	m.SetPolicyVersion(42)

	snap := m.Snapshot()
	if snap.PolicyVersion != 42 {
		t.Errorf("PolicyVersion = %d, want 42", snap.PolicyVersion)
	}
}

func TestGovernanceMetrics_SetActivePolicyRules(t *testing.T) {
	m := NewGovernanceMetrics()
	m.SetActivePolicyRules(10)

	snap := m.Snapshot()
	if snap.ActivePolicyRules != 10 {
		t.Errorf("ActivePolicyRules = %d, want 10", snap.ActivePolicyRules)
	}
}

func TestGovernanceMetrics_RecordAuditLogWrite(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordAuditLogWrite()

	snap := m.Snapshot()
	if snap.AuditLogWritesTotal != 1 {
		t.Errorf("AuditLogWritesTotal = %d, want 1", snap.AuditLogWritesTotal)
	}
}

func TestGovernanceMetrics_RecordAuditLogError(t *testing.T) {
	m := NewGovernanceMetrics()
	m.RecordAuditLogError()

	snap := m.Snapshot()
	if snap.AuditLogErrorsTotal != 1 {
		t.Errorf("AuditLogErrorsTotal = %d, want 1", snap.AuditLogErrorsTotal)
	}
}

func TestGovernanceMetrics_NilReceiver(t *testing.T) {
	var m *GovernanceMetrics
	// 所有方法在 nil receiver 下不应 panic
	m.RecordToolCall("a", "t")
	m.RecordToolBlocked("a", "t", "r")
	m.RecordCost(1.0)
	m.RecordCostExceeded()
	m.RecordOutputBlocked()
	m.RecordPIIDetected()
	m.RecordPolicyHotSwap()
	m.SetPolicyVersion(1)
	m.SetActivePolicyRules(1)
	m.RecordAuditLogWrite()
	m.RecordAuditLogError()

	snap := m.Snapshot()
	if snap.ToolCallsTotal != 0 {
		t.Error("nil receiver Snapshot should return zero values")
	}
}
