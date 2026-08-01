package governance

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Enforcer 策略执行接口。
// 由 agent 包依赖此接口（而非具体 *PolicyEnforcer），
// 避免 governance 反向引用 agent，保持模块依赖方向正确。
type Enforcer interface {
	CheckToolCall(ctx context.Context, toolName, args string) error
	CheckCost(ctx context.Context, cost float64) error
	CheckOutput(ctx context.Context, output string) error
	RecordToolCall(ctx context.Context, toolName string)
	Snapshot() EnforcerSnapshot
}

// EnforcerSnapshot 执行器运行时快照（监控/调试）。
type EnforcerSnapshot struct {
	ToolCalls      map[string]int
	TotalCost      float64
	TotalToolCalls int
}

// PolicyEnforcer 策略执行器（运行时强制）。
type PolicyEnforcer struct {
	policy         *Policy
	mu             sync.Mutex
	toolCallCount  map[string]int
	totalCost      float64
	totalToolCalls int
	auditLog       AuditLogger        // 可选的审计日志
	agentID        string             // 当前 Agent ID（用于审计）
	metrics        *GovernanceMetrics // 可选的可观测性指标
}

// NewPolicyEnforcer 创建执行器。
func NewPolicyEnforcer(p *Policy) *PolicyEnforcer {
	return &PolicyEnforcer{
		policy:        p,
		toolCallCount: make(map[string]int),
	}
}

// NewPolicyEnforcerWithMetrics 创建带可观测性指标的执行器。
func NewPolicyEnforcerWithMetrics(p *Policy, metrics *GovernanceMetrics) *PolicyEnforcer {
	return &PolicyEnforcer{
		policy:        p,
		toolCallCount: make(map[string]int),
		metrics:       metrics,
	}
}

// NewPolicyEnforcerWithAudit 创建带审计日志的执行器。
func NewPolicyEnforcerWithAudit(p *Policy, auditLog AuditLogger, agentID string) *PolicyEnforcer {
	return &PolicyEnforcer{
		policy:        p,
		toolCallCount: make(map[string]int),
		auditLog:      auditLog,
		agentID:       agentID,
	}
}

// UpdatePolicy 原子更新策略（用于热加载集成）。
func (e *PolicyEnforcer) UpdatePolicy(p *Policy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policy = p
	if e.metrics != nil {
		e.metrics.RecordPolicyHotSwap()
		if p != nil {
			e.metrics.SetActivePolicyRules(int64(len(p.Spec.ToolRestrictions)))
		}
	}
}

// logAudit 记录审计事件（如果配置了审计日志）。
func (e *PolicyEnforcer) logAudit(eventType AuditEventType, toolName, reason, severity string) {
	if e.auditLog == nil {
		return
	}
	e.auditLog.Log(AuditEvent{
		Type:     eventType,
		AgentID:  e.agentID,
		ToolName: toolName,
		Reason:   reason,
		Severity: severity,
	})
}

// findToolRestriction 在策略中查找指定tool的限制。
func (e *PolicyEnforcer) findToolRestriction(toolName string) *ToolRestriction {
	for i := range e.policy.Spec.ToolRestrictions {
		if e.policy.Spec.ToolRestrictions[i].Tool == toolName {
			return &e.policy.Spec.ToolRestrictions[i]
		}
	}
	return nil
}

// CheckToolCall 在tool执行前检查策略（线程安全）。
func (e *PolicyEnforcer) CheckToolCall(ctx context.Context, toolName, args string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.policy == nil {
		return ErrPolicyNotFound
	}

	// 输入安全：防止 prompt injection（检查tool名和参数中的可疑模式）
	if sanitized := detectPromptInjection(args); sanitized != "" {
		e.logAudit(AuditToolCallBlocked, toolName, "检测到 prompt injection 尝试: "+sanitized, "critical")
		if e.metrics != nil {
			e.metrics.RecordToolBlocked(e.agentID, toolName, "prompt_injection")
		}
		return fmt.Errorf("%w: prompt injection detected in tool %s arguments", ErrBlockedArgument, toolName)
	}

	// 全局行为约束：会话tool总调用数
	if e.policy.Spec.BehaviorConstraints.MaxToolCalls > 0 &&
		e.totalToolCalls >= e.policy.Spec.BehaviorConstraints.MaxToolCalls {
		if e.metrics != nil {
			e.metrics.RecordToolBlocked(e.agentID, toolName, "max_calls_exceeded")
		}
		return fmt.Errorf("%w: session tool call limit %d exceeded",
			ErrToolCallLimitExceeded, e.policy.Spec.BehaviorConstraints.MaxToolCalls)
	}

	restriction := e.findToolRestriction(toolName)
	if restriction == nil {
		return nil // 无限制
	}

	if restriction.MaxCallsPerRun > 0 && e.toolCallCount[toolName] >= restriction.MaxCallsPerRun {
		if e.metrics != nil {
			e.metrics.RecordToolBlocked(e.agentID, toolName, "per_run_limit")
		}
		return fmt.Errorf("%w: tool %s per-run call limit %d exceeded",
			ErrToolCallLimitExceeded, toolName, restriction.MaxCallsPerRun)
	}

	for _, blocked := range restriction.BlockedArgs {
		if strings.Contains(args, blocked) {
			e.logAudit(AuditToolCallBlocked, toolName, fmt.Sprintf("命中禁止参数 '%s'", maskSecret(blocked)), "warning")
			if e.metrics != nil {
				e.metrics.RecordToolBlocked(e.agentID, toolName, "blocked_arg")
			}
			return fmt.Errorf("%w: tool %s hit forbidden argument '%s'",
				ErrBlockedArgument, toolName, blocked)
		}
	}

	return nil
}

// RecordToolCall 在 CheckToolCall 通过后记录一次tool调用。
func (e *PolicyEnforcer) RecordToolCall(ctx context.Context, toolName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.toolCallCount[toolName]++
	e.totalToolCalls++
	if e.metrics != nil {
		e.metrics.RecordToolCall(e.agentID, toolName)
	}
}

// CheckCost 在 LLM 调用后检查成本（线程安全）。
func (e *PolicyEnforcer) CheckCost(ctx context.Context, cost float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.policy == nil {
		return ErrPolicyNotFound
	}
	e.totalCost += cost
	if e.metrics != nil {
		e.metrics.RecordCost(cost)
	}
	if e.policy.Spec.CostLimits.PerRequest > 0 && e.totalCost > e.policy.Spec.CostLimits.PerRequest {
		e.logAudit(AuditCostExceeded, "", fmt.Sprintf("成本 %.4f 超过上限 %.4f", e.totalCost, e.policy.Spec.CostLimits.PerRequest), "critical")
		if e.metrics != nil {
			e.metrics.RecordCostExceeded()
		}
		return fmt.Errorf("%w: request cost %.4f exceeds limit %.4f",
			ErrCostLimitExceeded, e.totalCost, e.policy.Spec.CostLimits.PerRequest)
	}
	return nil
}

// CheckOutput 输出护栏检查（长度 / strict PII）。
func (e *PolicyEnforcer) CheckOutput(ctx context.Context, output string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.policy == nil {
		return ErrPolicyNotFound
	}
	g := e.policy.Spec.OutputGuardrail
	if g.MaxLength > 0 && len(output) > g.MaxLength {
		e.logAudit(AuditOutputBlocked, "", fmt.Sprintf("output length %d exceeds limit %d", len(output), g.MaxLength), "warning")
		if e.metrics != nil {
			e.metrics.RecordOutputBlocked()
		}
		return fmt.Errorf("%w: output length %d exceeds limit %d", ErrOutputTooLong, len(output), g.MaxLength)
	}
	if g.PIIFilter == "strict" && containsPII(output) {
		e.logAudit(AuditPIIDetected, "", "strict PII 检测命中", "critical")
		if e.metrics != nil {
			e.metrics.RecordPIIDetected()
		}
		return ErrPIIDetected
	}
	return nil
}

// Snapshot 返回运行时快照。
func (e *PolicyEnforcer) Snapshot() EnforcerSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	toolCalls := make(map[string]int, len(e.toolCallCount))
	for k, v := range e.toolCallCount {
		toolCalls[k] = v
	}
	return EnforcerSnapshot{
		ToolCalls:      toolCalls,
		TotalCost:      e.totalCost,
		TotalToolCalls: e.totalToolCalls,
	}
}

// containsPII 简化 PII 检测（手机号 / 18 位身份证）。
func containsPII(s string) bool {
	return rePhone.MatchString(s) || reID.MatchString(s)
}
