/**
 * governance/quota.ts — 配额管理与令牌桶限流
 *
 * 对齐 Go internal/governance/quota.go 的核心能力。
 * Stability: Stable
 */

import type { TenantQuota } from './types.js';

/**
 * 令牌桶限流器（QPS 控制）。
 *
 * 设计：
 * - 桶容量 = burst（满桶）
 * - 每秒自动补充 rate 个令牌
 * - take(n) 非阻塞尝试消费 n 个令牌；成功返回 true
 */
export class TokenBucket {
  private capacity: number;
  private rate: number;
  private tokens: number;
  private lastRefill: number;

  /**
   * @param rate 每秒允许的请求数（QPS 上限）
   * @param burst 最大突发量（默认等于 rate）
   */
  constructor(rate: number, burst?: number) {
    this.rate = rate <= 0 ? 1e9 : rate;
    this.capacity = burst ?? this.rate;
    this.tokens = this.capacity;
    this.lastRefill = Date.now();
  }

  /** 尝试消费 n 个令牌。成功返回 true。 */
  take(n = 1): boolean {
    this.refill();
    if (this.tokens >= n) {
      this.tokens -= n;
      return true;
    }
    return false;
  }

  /** 当前可用令牌数 */
  available(): number {
    this.refill();
    return Math.floor(this.tokens);
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = (now - this.lastRefill) / 1000;
    if (elapsed <= 0) return;
    this.tokens = Math.min(this.capacity, this.tokens + elapsed * this.rate);
    this.lastRefill = now;
  }
}

/**
 * 单租户配额管理器。
 * 跟踪 QPS、每日 Token 用量、Agent 数和 Session 数。
 */
export class QuotaManager {
  readonly tenantId: string;
  private quotas: TenantQuota;
  private bucket: TokenBucket;
  private dayTokens = 0;
  private dayKey: string;
  private agentCount = 0;
  private sessionCount = 0;

  constructor(tenantId: string, quotas: TenantQuota) {
    this.tenantId = tenantId;
    this.quotas = quotas;
    this.bucket = new TokenBucket(quotas.maxQPS, quotas.maxQPS);
    this.dayKey = new Date().toISOString().slice(0, 10);
  }

  /** 检查 QPS 是否允许一次调用 */
  checkQPS(): boolean {
    return this.bucket.take(1);
  }

  /** 记录 Token 消耗，超过每日上限返回 false */
  recordTokens(count: number): boolean {
    this.resetDayIfNeeded();
    if (this.dayTokens + count > this.quotas.maxTokensPerDay) {
      return false;
    }
    this.dayTokens += count;
    return true;
  }

  /** 尝试注册一个 Agent，超过配额返回 false */
  addAgent(): boolean {
    if (this.agentCount >= this.quotas.maxAgents) return false;
    this.agentCount++;
    return true;
  }

  /** 释放一个 Agent 配额 */
  removeAgent(): void {
    if (this.agentCount > 0) this.agentCount--;
  }

  /** 尝试注册一个 Session，超过配额返回 false */
  addSession(): boolean {
    if (this.sessionCount >= this.quotas.maxSessions) return false;
    this.sessionCount++;
    return true;
  }

  /** 释放一个 Session 配额 */
  removeSession(): void {
    if (this.sessionCount > 0) this.sessionCount--;
  }

  /** 获取当前配额使用快照 */
  snapshot(): { agentCount: number; sessionCount: number; dayTokens: number; qpsAvailable: number } {
    this.resetDayIfNeeded();
    return {
      agentCount: this.agentCount,
      sessionCount: this.sessionCount,
      dayTokens: this.dayTokens,
      qpsAvailable: this.bucket.available(),
    };
  }

  private resetDayIfNeeded(): void {
    const today = new Date().toISOString().slice(0, 10);
    if (today !== this.dayKey) {
      this.dayKey = today;
      this.dayTokens = 0;
    }
  }
}

/**
 * 多租户资源管理器。
 * 统一管理多个租户的 QuotaManager 实例。
 */
export class ResourceManager {
  private managers = new Map<string, QuotaManager>();

  /** 为指定租户注册配额管理器 */
  register(tenantId: string, quotas: TenantQuota): QuotaManager {
    const existing = this.managers.get(tenantId);
    if (existing) return existing;
    const qm = new QuotaManager(tenantId, quotas);
    this.managers.set(tenantId, qm);
    return qm;
  }

  /** 获取指定租户的配额管理器 */
  get(tenantId: string): QuotaManager | undefined {
    return this.managers.get(tenantId);
  }

  /** 移除指定租户 */
  remove(tenantId: string): boolean {
    return this.managers.delete(tenantId);
  }

  /** 列出所有已注册租户 ID */
  listTenants(): string[] {
    return [...this.managers.keys()];
  }
}
