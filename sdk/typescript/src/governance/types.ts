/**
 * governance/types.ts — 多租户治理核心类型
 *
 * 对齐 Go internal/governance/ 模块的类型定义。
 * Stability: Stable
 */

/** 租户付费计划 */
export type TenantPlan = 'free' | 'pro' | 'enterprise';

/** 租户状态 */
export type TenantStatus = 'active' | 'disabled' | 'archived';

/** 租户配额 */
export interface TenantQuota {
  maxAgents: number;
  maxSessions: number;
  maxTokensPerDay: number;
  maxStorageGB: number;
  maxQPS: number;
}

/** 租户实体 */
export interface Tenant {
  id: string;
  name: string;
  plan: TenantPlan;
  quotas: TenantQuota;
  createdAt: string;
  status: TenantStatus;
  metadata?: Record<string, string>;
}

/** 策略元信息 */
export interface PolicyMetadata {
  name: string;
}

/** 单工具限制规则 */
export interface ToolRestriction {
  tool: string;
  requireApproval?: boolean;
  maxCallsPerRun?: number;
  blockedArgs?: string[];
  allowedDomains?: string[];
}

/** 成本限制（美元） */
export interface CostLimits {
  perRequest?: number;
  perDay?: number;
  perSession?: number;
}

/** 输出护栏配置 */
export interface OutputGuardrail {
  piiFilter?: 'strict' | 'moderate' | 'off';
  injectionBlock?: boolean;
  maxLength?: number;
}

/** 行为约束配置 */
export interface BehaviorConstraints {
  maxTurns?: number;
  maxToolCalls?: number;
  requireReflection?: boolean;
}

/** 策略规约 */
export interface PolicySpec {
  toolRestrictions?: ToolRestriction[];
  costLimits?: CostLimits;
  outputGuardrail?: OutputGuardrail;
  behaviorConstraints?: BehaviorConstraints;
}

/** 策略定义 */
export interface Policy {
  apiVersion: string;
  kind: string;
  metadata: PolicyMetadata;
  spec: PolicySpec;
}

/** 审计事件类型 */
export type AuditEventType =
  | 'tool_call_blocked'
  | 'cost_exceeded'
  | 'output_blocked'
  | 'pii_detected'
  | 'policy_violation'
  | 'policy_loaded'
  | 'policy_hot_swapped';

/** 审计日志事件 */
export interface AuditEvent {
  id: string;
  timestamp: string;
  type: AuditEventType;
  agentId: string;
  toolName?: string;
  reason: string;
  detail?: unknown;
  severity: 'info' | 'warning' | 'critical';
}

/** 审计日志查询条件 */
export interface AuditQuery {
  startTime?: string;
  endTime?: string;
  agentId?: string;
  type?: AuditEventType;
  severity?: string;
  limit?: number;
}

/** 执行器运行时快照 */
export interface EnforcerSnapshot {
  toolCalls: Record<string, number>;
  totalCost: number;
  totalToolCalls: number;
}

/** 默认配额 */
export function defaultQuota(plan: TenantPlan): TenantQuota {
  switch (plan) {
    case 'free':
      return { maxAgents: 3, maxSessions: 10, maxTokensPerDay: 100_000, maxStorageGB: 1, maxQPS: 5 };
    case 'pro':
      return { maxAgents: 20, maxSessions: 100, maxTokensPerDay: 2_000_000, maxStorageGB: 50, maxQPS: 50 };
    case 'enterprise':
      return { maxAgents: 200, maxSessions: 2000, maxTokensPerDay: 50_000_000, maxStorageGB: 500, maxQPS: 500 };
  }
}
