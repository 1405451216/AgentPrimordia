/**
 * governance/tenant.ts — 租户生命周期管理
 *
 * 对齐 Go internal/governance/tenant_manager.go 的核心能力。
 * Stability: Stable
 */

import type { Tenant, TenantPlan, TenantQuota, TenantStatus } from './types.js';
import { defaultQuota } from './types.js';

let tenantIdCounter = 0;

function generateTenantId(): string {
  const bytes = new Uint8Array(8);
  if (typeof globalThis.crypto !== 'undefined' && globalThis.crypto.getRandomValues) {
    globalThis.crypto.getRandomValues(bytes);
  } else {
    for (let i = 0; i < 8; i++) bytes[i] = Math.floor(Math.random() * 256);
  }
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  return `tenant-${hex}-${++tenantIdCounter}`;
}

/**
 * 租户生命周期管理器。
 * 创建、查询、更新、删除租户，以及 API Key 绑定。
 */
export class TenantManager {
  private tenants = new Map<string, Tenant>();
  private apiKeys = new Map<string, string>(); // hashedKey -> tenantId

  /** 创建新租户 */
  createTenant(name: string, plan?: TenantPlan, quotas?: TenantQuota): { tenant: Tenant; apiKey: string } {
    if (!name) throw new Error('governance: tenant name cannot be empty');

    const resolvedPlan = plan ?? 'free';
    const resolvedQuotas = quotas ?? defaultQuota(resolvedPlan);
    const id = generateTenantId();

    const tenant: Tenant = {
      id,
      name,
      plan: resolvedPlan,
      quotas: resolvedQuotas,
      createdAt: new Date().toISOString(),
      status: 'active',
      metadata: {},
    };

    this.tenants.set(id, tenant);

    // 生成 API Key
    const apiKey = `ap-${id}-${Date.now().toString(36)}`;
    const hashed = this.hashKey(apiKey);
    this.apiKeys.set(hashed, id);

    return { tenant, apiKey };
  }

  /** 根据 ID 获取租户 */
  getTenant(tenantId: string): Tenant | undefined {
    return this.tenants.get(tenantId);
  }

  /** 列出所有租户 */
  listTenants(): Tenant[] {
    return [...this.tenants.values()];
  }

  /** 更新租户状态 */
  updateStatus(tenantId: string, status: TenantStatus): boolean {
    const tenant = this.tenants.get(tenantId);
    if (!tenant) return false;
    tenant.status = status;
    return true;
  }

  /** 更新租户配额 */
  updateQuotas(tenantId: string, quotas: TenantQuota): boolean {
    const tenant = this.tenants.get(tenantId);
    if (!tenant) return false;
    tenant.quotas = quotas;
    return true;
  }

  /** 删除租户 */
  deleteTenant(tenantId: string): boolean {
    return this.tenants.delete(tenantId);
  }

  /** 通过 API Key 认证租户 */
  authenticate(apiKey: string): Tenant | undefined {
    const hashed = this.hashKey(apiKey);
    const tenantId = this.apiKeys.get(hashed);
    if (!tenantId) return undefined;
    const tenant = this.tenants.get(tenantId);
    if (tenant && tenant.status === 'active') return tenant;
    return undefined;
  }

  /** 获取租户数量 */
  get size(): number {
    return this.tenants.size;
  }

  private hashKey(key: string): string {
    // 简化哈希（生产环境应使用 SHA-256）
    let hash = 0;
    for (let i = 0; i < key.length; i++) {
      const char = key.charCodeAt(i);
      hash = ((hash << 5) - hash + char) | 0;
    }
    return `h-${hash.toString(36)}`;
  }
}
