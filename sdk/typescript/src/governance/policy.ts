/**
 * governance/policy.ts — 策略执行器
 *
 * 对齐 Go internal/governance/policy_enforcer.go 的核心能力。
 * Stability: Stable
 */

import type { Policy, EnforcerSnapshot, AuditEvent, AuditEventType } from './types.js';

/** 策略执行错误 */
export class PolicyViolationError extends Error {
  readonly code: string;
  constructor(code: string, message: string) {
    super(message);
    this.name = 'PolicyViolationError';
    this.code = code;
  }
}

/** 审计日志接口 */
export interface AuditLogger {
  log(event: AuditEvent): void;
  close(): void;
}

/** 内存审计日志（测试/轻量场景） */
export class InMemoryAuditLogger implements AuditLogger {
  events: AuditEvent[] = [];

  log(event: AuditEvent): void {
    this.events.push(event);
  }

  close(): void {
    // no-op
  }
}

let auditIdCounter = 0;

/**
 * 策略执行器（运行时强制）。
 *
 * 检查工具调用、成本和输出是否符合策略定义。
 * 线程安全：JS 单线程模型下天然安全。
 */
export class PolicyEnforcer {
  private policy: Policy;
  private toolCallCount = new Map<string, number>();
  private totalCost = 0;
  private totalToolCalls = 0;
  private auditLog?: AuditLogger;
  private agentId: string;

  constructor(policy: Policy, opts?: { auditLog?: AuditLogger; agentId?: string }) {
    this.policy = policy;
    this.auditLog = opts?.auditLog;
    this.agentId = opts?.agentId ?? 'unknown';
  }

  /** 检查工具调用是否允许 */
  checkToolCall(toolName: string, args: string): void {
    const restrictions = this.policy.spec.toolRestrictions ?? [];
    const restriction = restrictions.find((r) => r.tool === toolName);

    if (restriction) {
      // 检查调用次数上限
      if (restriction.maxCallsPerRun) {
        const count = this.toolCallCount.get(toolName) ?? 0;
        if (count >= restriction.maxCallsPerRun) {
          this.emitAudit('tool_call_blocked', toolName, `tool ${toolName} exceeded max calls per run`);
          throw new PolicyViolationError('TOOL_CALL_LIMIT', `Tool ${toolName} call limit exceeded`);
        }
      }

      // 检查禁止参数模式
      if (restriction.blockedArgs) {
        for (const pattern of restriction.blockedArgs) {
          if (args.includes(pattern)) {
            this.emitAudit('tool_call_blocked', toolName, `blocked argument pattern: ${pattern}`);
            throw new PolicyViolationError('BLOCKED_ARGUMENT', `Blocked argument pattern: ${pattern}`);
          }
        }
      }
    }

    // 全局工具调用上限
    const maxToolCalls = this.policy.spec.behaviorConstraints?.maxToolCalls;
    if (maxToolCalls && this.totalToolCalls >= maxToolCalls) {
      this.emitAudit('tool_call_blocked', toolName, 'global tool call limit exceeded');
      throw new PolicyViolationError('TOOL_CALL_LIMIT', 'Global tool call limit exceeded');
    }
  }

  /** 检查成本是否允许 */
  checkCost(cost: number): void {
    const limits = this.policy.spec.costLimits;
    if (!limits) return;

    if (limits.perRequest && cost > limits.perRequest) {
      this.emitAudit('cost_exceeded', undefined, `per-request cost ${cost} exceeds ${limits.perRequest}`);
      throw new PolicyViolationError('COST_LIMIT', `Per-request cost limit exceeded`);
    }

    if (limits.perDay && this.totalCost + cost > limits.perDay) {
      this.emitAudit('cost_exceeded', undefined, `daily cost would exceed ${limits.perDay}`);
      throw new PolicyViolationError('COST_LIMIT', `Daily cost limit exceeded`);
    }
  }

  /** 检查输出是否合规 */
  checkOutput(output: string): void {
    const guardrail = this.policy.spec.outputGuardrail;
    if (!guardrail) return;

    // 输出长度检查
    if (guardrail.maxLength && output.length > guardrail.maxLength) {
      this.emitAudit('output_blocked', undefined, `output length ${output.length} exceeds ${guardrail.maxLength}`);
      throw new PolicyViolationError('OUTPUT_TOO_LONG', `Output exceeds max length`);
    }

    // PII 检测（strict 模式）
    if (guardrail.piiFilter === 'strict') {
      const phoneRe = /1[3-9]\d{9}/;
      const idRe = /\d{17}[\dXx]/;
      if (phoneRe.test(output) || idRe.test(output)) {
        this.emitAudit('pii_detected', undefined, 'strict PII filter triggered');
        throw new PolicyViolationError('PII_DETECTED', `Output triggered strict PII filter`);
      }
    }
  }

  /** 记录工具调用（计数） */
  recordToolCall(toolName: string): void {
    this.toolCallCount.set(toolName, (this.toolCallCount.get(toolName) ?? 0) + 1);
    this.totalToolCalls++;
  }

  /** 记录成本 */
  recordCost(cost: number): void {
    this.totalCost += cost;
  }

  /** 获取运行时快照 */
  snapshot(): EnforcerSnapshot {
    const toolCalls: Record<string, number> = {};
    for (const [k, v] of this.toolCallCount) toolCalls[k] = v;
    return { toolCalls, totalCost: this.totalCost, totalToolCalls: this.totalToolCalls };
  }

  /** 热替换策略 */
  hotSwap(newPolicy: Policy): void {
    this.policy = newPolicy;
    this.emitAudit('policy_hot_swapped', undefined, `policy swapped to ${newPolicy.metadata.name}`);
  }

  private emitAudit(type: AuditEventType, toolName: string | undefined, reason: string): void {
    if (!this.auditLog) return;
    this.auditLog.log({
      id: `audit-${++auditIdCounter}-${Date.now()}`,
      timestamp: new Date().toISOString(),
      type,
      agentId: this.agentId,
      toolName,
      reason,
      severity: type === 'pii_detected' ? 'critical' : 'warning',
    });
  }
}
