/**
 * llm-faults.ts — LLM Provider 故障模拟
 *
 * 对齐 Go 端 internal/chaos/llm_faults.go
 * Stability: Experimental
 */

import type { Fault, CleanupFunc } from './faults.js';

/** LLM HTTP 状态码故障（对齐 Go LLMHTTPStatusFault） */
export class LLMHTTPStatusFault implements Fault {
  readonly provider: string;
  readonly statusCode: number;
  readonly body: string;
  readonly durationMs: number;
  private _active = false;

  constructor(provider: string, statusCode: number, body: string, durationMs = 30000) {
    this.provider = provider;
    this.statusCode = statusCode;
    this.body = body;
    this.durationMs = durationMs;
  }

  type(): string { return `llm_http_${this.statusCode}`; }

  description(): string {
    return `模拟 ${this.provider} Provider 返回 HTTP ${this.statusCode}`;
  }

  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; };
  }

  get active(): boolean { return this._active; }
}

/** 创建 503 故障 */
export function llmHTTP503Fault(provider: string): LLMHTTPStatusFault {
  return new LLMHTTPStatusFault(
    provider, 503,
    '{"error": {"message": "Service Unavailable", "type": "server_error"}}',
  );
}

/** 创建 429 限流故障 */
export function llmHTTP429Fault(provider: string): LLMHTTPStatusFault {
  return new LLMHTTPStatusFault(
    provider, 429,
    '{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error"}}',
  );
}

/** 创建 500 服务器错误 */
export function llmHTTP500Fault(provider: string): LLMHTTPStatusFault {
  return new LLMHTTPStatusFault(
    provider, 500,
    '{"error": {"message": "Internal Server Error", "type": "server_error"}}',
  );
}

/** LLM 超时故障（对齐 Go LLMTimeoutFault） */
export class LLMTimeoutFault implements Fault {
  readonly provider: string;
  readonly timeoutMs: number;
  private _active = false;

  constructor(provider: string, timeoutMs: number) {
    this.provider = provider;
    this.timeoutMs = timeoutMs;
  }

  type(): string { return 'llm_timeout'; }

  description(): string {
    return `模拟 ${this.provider} Provider 响应超时 ${this.timeoutMs}ms`;
  }

  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; };
  }

  get active(): boolean { return this._active; }
}

/** LLM 间歇性故障（对齐 Go LLMIntermittentFault） */
export class LLMIntermittentFault implements Fault {
  readonly provider: string;
  readonly failureRate: number;
  readonly failureStatus: number;
  private _active = false;

  constructor(provider: string, failureRate: number, failureStatus = 503) {
    this.provider = provider;
    this.failureRate = failureRate;
    this.failureStatus = failureStatus;
  }

  type(): string { return 'llm_intermittent'; }

  description(): string {
    return `模拟 ${this.provider} Provider 间歇性故障（故障率 ${(this.failureRate * 100).toFixed(0)}%）`;
  }

  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; };
  }

  get active(): boolean { return this._active; }

  /** 判断本次请求是否应触发故障 */
  shouldFail(): boolean {
    return Math.random() < this.failureRate;
  }
}

/** LLM 慢响应故障（对齐 Go LLMSlowResponseFault） */
export class LLMSlowResponseFault implements Fault {
  readonly provider: string;
  readonly minDelayMs: number;
  readonly maxDelayMs: number;
  private _active = false;

  constructor(provider: string, minDelayMs: number, maxDelayMs: number) {
    this.provider = provider;
    this.minDelayMs = minDelayMs;
    this.maxDelayMs = maxDelayMs;
  }

  type(): string { return 'llm_slow_response'; }

  description(): string {
    return `模拟 ${this.provider} Provider 慢响应 ${this.minDelayMs}ms~${this.maxDelayMs}ms`;
  }

  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; };
  }

  get active(): boolean { return this._active; }

  /** 计算本次延迟时间 */
  computeDelay(): number {
    if (this.maxDelayMs <= this.minDelayMs) return this.minDelayMs;
    return this.minDelayMs + Math.random() * (this.maxDelayMs - this.minDelayMs);
  }
}

// ===== 预定义故障场景 =====

/** LLM 故障场景（对齐 Go LLMFaultScenario） */
export interface LLMFaultScenario {
  name: string;
  provider: string;
  faults: Fault[];
}

/** 创建 LLM 故障转移场景：503 → 429 → 超时 → 恢复 */
export function llmFailoverScenario(provider: string): LLMFaultScenario {
  return {
    name: 'llm_failover_sequence',
    provider,
    faults: [
      llmHTTP503Fault(provider),
      llmHTTP429Fault(provider),
      new LLMTimeoutFault(provider, 5000),
    ],
  };
}

/** 创建 LLM 混沌场景：间歇故障 + 慢响应 */
export function llmChaosScenario(provider: string): LLMFaultScenario {
  return {
    name: 'llm_chaos_mixed',
    provider,
    faults: [
      new LLMIntermittentFault(provider, 0.3),
      new LLMSlowResponseFault(provider, 1000, 5000),
    ],
  };
}

// ===== 向后兼容别名 =====

/** @deprecated 使用 LLMHTTPStatusFault */
export class LLMErrorFault implements Fault {
  readonly statusCode: number;
  readonly errorMessage: string;
  readonly modelTarget: string;
  private _active = false;

  constructor(statusCode: number, errorMessage: string, modelTarget: string, _probability = 1.0) {
    this.statusCode = statusCode;
    this.errorMessage = errorMessage;
    this.modelTarget = modelTarget;
  }

  type(): string { return 'llm_error'; }
  description(): string { return `LLM ${this.modelTarget} 返回 ${this.statusCode}: ${this.errorMessage}`; }
  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; };
  }
}

/** @deprecated 使用 LLMTimeoutFault */
export class LLMRateLimitFault implements Fault {
  readonly maxRequests: number;
  readonly windowMs: number;
  readonly modelTarget: string;
  private _active = false;
  private requestCount = 0;
  private windowStart = Date.now();

  constructor(maxRequests: number, windowMs: number, modelTarget: string) {
    this.maxRequests = maxRequests;
    this.windowMs = windowMs;
    this.modelTarget = modelTarget;
  }

  type(): string { return 'llm_rate_limit'; }
  description(): string { return `LLM ${this.modelTarget} 限流 ${this.maxRequests} req/${this.windowMs}ms`; }
  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; this.requestCount = 0; };
  }

  shouldThrottle(): boolean {
    if (!this._active) return false;
    const now = Date.now();
    if (now - this.windowStart > this.windowMs) {
      this.windowStart = now;
      this.requestCount = 0;
    }
    this.requestCount++;
    return this.requestCount > this.maxRequests;
  }
}
