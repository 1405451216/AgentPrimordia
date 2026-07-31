/**
 * governance/index.ts — 多租户治理模块统一导出
 *
 * 对齐 Go internal/governance/ 模块，提供：
 * - 租户生命周期管理（创建/查询/更新/删除/API Key 认证）
 * - 配额限流（令牌桶 QPS 控制 + 每日 Token 上限）
 * - 策略执行（工具限制/成本限制/输出护栏/行为约束）
 * - 审计日志（违规事件记录与查询）
 *
 * Stability: Stable
 */

export type {
  TenantPlan,
  TenantStatus,
  TenantQuota,
  Tenant,
  PolicyMetadata,
  ToolRestriction,
  CostLimits,
  OutputGuardrail,
  BehaviorConstraints,
  PolicySpec,
  Policy,
  AuditEventType,
  AuditEvent,
  AuditQuery,
  EnforcerSnapshot,
} from './types.js';

export { defaultQuota } from './types.js';
export { TokenBucket, QuotaManager, ResourceManager } from './quota.js';
export { PolicyEnforcer, PolicyViolationError, InMemoryAuditLogger } from './policy.js';
export type { AuditLogger } from './policy.js';
export { TenantManager } from './tenant.js';
