// metrics.go — Governance 可观测性指标接入（生产集成深度）
//
// 为 PolicyEnforcer / AuditLogger 接入 Prometheus 指标体系：
//   - ap_governance_tool_calls_total（counter, 按 agent/tool 维度）
//   - ap_governance_tool_blocked_total（counter, 按 agent/tool/reason 维度）
//   - ap_governance_cost_usd_total（counter, 按 agent 维度）
//   - ap_governance_cost_exceeded_total（counter, 按 agent 维度）
//   - ap_governance_output_blocked_total（counter, 按 agent/reason 维度）
//   - ap_governance_pii_detected_total（counter, 按 agent 维度）
//   - ap_governance_policy_version（gauge, 当前策略版本号）
//   - ap_governance_policy_hot_swaps_total（counter）
//   - ap_governance_audit_log_writes_total（counter）
//   - ap_governance_audit_log_write_errors_total（counter）
//
// 使用 sync/atomic 实现无锁热路径，与 internal/metrics 包风格一致。
package governance

import (
	"math"
	"sync"
	"sync/atomic"
)

// GovernanceMetrics Governance 可观测性指标（线程安全）。
type GovernanceMetrics struct {
	// 计数器
	toolCallsTotal      atomic.Int64
	toolBlockedTotal    atomic.Int64
	costUSDTotal        atomic.Uint64 // math.Float64bits
	costExceededTotal   atomic.Int64
	outputBlockedTotal  atomic.Int64
	piiDetectedTotal    atomic.Int64
	policyHotSwapsTotal atomic.Int64
	auditLogWritesTotal atomic.Int64
	auditLogErrorsTotal atomic.Int64

	// 仪表盘
	policyVersion     atomic.Int64
	activePolicyRules atomic.Int64

	// 按标签维度
	mu               sync.RWMutex
	toolCallsByLabel map[string]*labeledGovCounter // key: agent|tool
	blockedByLabel   map[string]*labeledGovCounter // key: agent|tool|reason
}

type labeledGovCounter struct {
	count atomic.Int64
}

// NewGovernanceMetrics 创建 Governance 指标实例。
func NewGovernanceMetrics() *GovernanceMetrics {
	return &GovernanceMetrics{
		toolCallsByLabel: make(map[string]*labeledGovCounter),
		blockedByLabel:   make(map[string]*labeledGovCounter),
	}
}

// RecordToolCall 记录一次tool调用检查（通过或拒绝均计）。
func (m *GovernanceMetrics) RecordToolCall(agentID, toolName string) {
	if m == nil {
		return
	}
	m.toolCallsTotal.Add(1)
	key := agentID + "|" + toolName
	m.mu.RLock()
	c, ok := m.toolCallsByLabel[key]
	m.mu.RUnlock()
	if !ok {
		m.mu.Lock()
		if c, ok = m.toolCallsByLabel[key]; !ok {
			c = &labeledGovCounter{}
			m.toolCallsByLabel[key] = c
		}
		m.mu.Unlock()
	}
	c.count.Add(1)
}

// RecordToolBlocked 记录一次tool调用被策略拒绝。
func (m *GovernanceMetrics) RecordToolBlocked(agentID, toolName, reason string) {
	if m == nil {
		return
	}
	m.toolBlockedTotal.Add(1)
	key := agentID + "|" + toolName + "|" + reason
	m.mu.RLock()
	c, ok := m.blockedByLabel[key]
	m.mu.RUnlock()
	if !ok {
		m.mu.Lock()
		if c, ok = m.blockedByLabel[key]; !ok {
			c = &labeledGovCounter{}
			m.blockedByLabel[key] = c
		}
		m.mu.Unlock()
	}
	c.count.Add(1)
}

// RecordCost 记录累计成本（USD）。
func (m *GovernanceMetrics) RecordCost(cost float64) {
	if m == nil {
		return
	}
	for {
		old := m.costUSDTotal.Load()
		newVal := float64FromBits(old) + cost
		if m.costUSDTotal.CompareAndSwap(old, bitsFromFloat64(newVal)) {
			break
		}
	}
}

// RecordCostExceeded 记录一次成本超限。
func (m *GovernanceMetrics) RecordCostExceeded() {
	if m == nil {
		return
	}
	m.costExceededTotal.Add(1)
}

// RecordOutputBlocked 记录一次输出被拦截。
func (m *GovernanceMetrics) RecordOutputBlocked() {
	if m == nil {
		return
	}
	m.outputBlockedTotal.Add(1)
}

// RecordPIIDetected 记录一次 PII 检测命中。
func (m *GovernanceMetrics) RecordPIIDetected() {
	if m == nil {
		return
	}
	m.piiDetectedTotal.Add(1)
}

// RecordPolicyHotSwap 记录一次策略热加载。
func (m *GovernanceMetrics) RecordPolicyHotSwap() {
	if m == nil {
		return
	}
	m.policyHotSwapsTotal.Add(1)
}

// SetPolicyVersion 设置当前策略版本号。
func (m *GovernanceMetrics) SetPolicyVersion(version int64) {
	if m == nil {
		return
	}
	m.policyVersion.Store(version)
}

// SetActivePolicyRules 设置当前策略规则数。
func (m *GovernanceMetrics) SetActivePolicyRules(count int64) {
	if m == nil {
		return
	}
	m.activePolicyRules.Store(count)
}

// RecordAuditLogWrite 记录一次审计日志写入。
func (m *GovernanceMetrics) RecordAuditLogWrite() {
	if m == nil {
		return
	}
	m.auditLogWritesTotal.Add(1)
}

// RecordAuditLogError 记录一次failed to write audit log。
func (m *GovernanceMetrics) RecordAuditLogError() {
	if m == nil {
		return
	}
	m.auditLogErrorsTotal.Add(1)
}

// GovernanceMetricsSnapshot 指标快照（用于 Prometheus 导出）。
type GovernanceMetricsSnapshot struct {
	ToolCallsTotal      int64
	ToolBlockedTotal    int64
	CostUSDTotal        float64
	CostExceededTotal   int64
	OutputBlockedTotal  int64
	PIIDetectedTotal    int64
	PolicyHotSwapsTotal int64
	AuditLogWritesTotal int64
	AuditLogErrorsTotal int64
	PolicyVersion       int64
	ActivePolicyRules   int64
	ToolCallsByLabel    map[string]int64
	BlockedByLabel      map[string]int64
}

// Snapshot 返回指标快照。
func (m *GovernanceMetrics) Snapshot() GovernanceMetricsSnapshot {
	if m == nil {
		return GovernanceMetricsSnapshot{}
	}
	m.mu.RLock()
	tcl := make(map[string]int64, len(m.toolCallsByLabel))
	for k, v := range m.toolCallsByLabel {
		tcl[k] = v.count.Load()
	}
	bbl := make(map[string]int64, len(m.blockedByLabel))
	for k, v := range m.blockedByLabel {
		bbl[k] = v.count.Load()
	}
	m.mu.RUnlock()

	return GovernanceMetricsSnapshot{
		ToolCallsTotal:      m.toolCallsTotal.Load(),
		ToolBlockedTotal:    m.toolBlockedTotal.Load(),
		CostUSDTotal:        float64FromBits(m.costUSDTotal.Load()),
		CostExceededTotal:   m.costExceededTotal.Load(),
		OutputBlockedTotal:  m.outputBlockedTotal.Load(),
		PIIDetectedTotal:    m.piiDetectedTotal.Load(),
		PolicyHotSwapsTotal: m.policyHotSwapsTotal.Load(),
		AuditLogWritesTotal: m.auditLogWritesTotal.Load(),
		AuditLogErrorsTotal: m.auditLogErrorsTotal.Load(),
		PolicyVersion:       m.policyVersion.Load(),
		ActivePolicyRules:   m.activePolicyRules.Load(),
		ToolCallsByLabel:    tcl,
		BlockedByLabel:      bbl,
	}
}

// float64FromBits / bitsFromFloat64 — 使用 math 包的标准位模式转换。
func float64FromBits(b uint64) float64 {
	return math.Float64frombits(b)
}

func bitsFromFloat64(f float64) uint64 {
	return math.Float64bits(f)
}
