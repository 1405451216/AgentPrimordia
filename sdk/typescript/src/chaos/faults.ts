/**
 * faults.ts — 故障注入器接口与实现
 *
 * 对齐 Go 端 internal/chaos/injector.go + llm_faults.go
 * Stability: Experimental
 */

/** 故障注入结果 */
export interface FaultResult {
  faultType: string;
  description: string;
  injected: boolean;
  injectTime: Date;
  cleanupTime?: Date;
  error?: Error;
}

/** 故障注入器接口（对齐 Go FaultInjector） */
export interface FaultInjector {
  /** 注入故障 */
  inject(): Promise<void>;
  /** 清理故障 */
  cleanup(): Promise<void>;
  /** 故障类型标识 */
  type(): string;
  /** 故障描述 */
  description(): string;
}

/** 延迟注入故障 */
export class LatencyFault implements FaultInjector {
  readonly delayMs: number;
  readonly jitterMs: number;
  readonly target: string;
  readonly probability: number;
  private active = false;

  constructor(delayMs: number, jitterMs: number, target: string, probability = 1.0) {
    this.delayMs = delayMs;
    this.jitterMs = jitterMs;
    this.target = target;
    this.probability = probability;
  }

  async inject(): Promise<void> {
    if (Math.random() > this.probability) return;
    this.active = true;
  }

  async cleanup(): Promise<void> {
    this.active = false;
  }

  type(): string { return 'latency'; }

  description(): string {
    return `对 ${this.target} 注入 ${this.delayMs}ms 延迟 (jitter=${this.jitterMs}ms)`;
  }

  get isActive(): boolean { return this.active; }
}

/** 错误注入故障 */
export class ErrorFault implements FaultInjector {
  readonly errorCode: number;
  readonly errorMessage: string;
  readonly target: string;
  readonly probability: number;
  private active = false;

  constructor(errorCode: number, errorMessage: string, target: string, probability = 1.0) {
    this.errorCode = errorCode;
    this.errorMessage = errorMessage;
    this.target = target;
    this.probability = probability;
  }

  async inject(): Promise<void> {
    if (Math.random() > this.probability) return;
    this.active = true;
  }

  async cleanup(): Promise<void> {
    this.active = false;
  }

  type(): string { return 'error'; }

  description(): string {
    return `对 ${this.target} 注入错误 ${this.errorCode}: ${this.errorMessage}`;
  }

  get isActive(): boolean { return this.active; }
}

/** 资源耗尽故障 */
export class ResourceFault implements FaultInjector {
  readonly resourceType: string;
  readonly limit: number;
  readonly target: string;
  readonly durationMs: number;
  private active = false;

  constructor(resourceType: string, limit: number, target: string, durationMs: number) {
    this.resourceType = resourceType;
    this.limit = limit;
    this.target = target;
    this.durationMs = durationMs;
  }

  async inject(): Promise<void> {
    this.active = true;
  }

  async cleanup(): Promise<void> {
    this.active = false;
  }

  type(): string { return 'resource'; }

  description(): string {
    return `对 ${this.target} 注入 ${this.resourceType} 资源限制 (${this.limit})`;
  }

  get isActive(): boolean { return this.active; }
}

// ===== LLM 专用故障 =====

/** LLM 故障接口（扩展 FaultInjector，增加模型过滤） */
export interface LLMFault extends FaultInjector {
  /** 是否影响指定模型 */
  affectsModel(model: string): boolean;
}

/** LLM 超时故障 */
export class LLMTimeoutFault implements LLMFault {
  readonly delayMs: number;
  readonly modelTarget: string;
  readonly probability: number;
  private active = false;

  constructor(delayMs: number, modelTarget: string, probability = 1.0) {
    this.delayMs = delayMs;
    this.modelTarget = modelTarget;
    this.probability = probability;
  }

  async inject(): Promise<void> {
    if (Math.random() > this.probability) return;
    this.active = true;
  }

  async cleanup(): Promise<void> { this.active = false; }
  type(): string { return 'llm_timeout'; }
  description(): string { return `LLM ${this.modelTarget} 超时 ${this.delayMs}ms`; }
  affectsModel(model: string): boolean {
    return this.modelTarget === '*' || model.includes(this.modelTarget);
  }
  get isActive(): boolean { return this.active; }
}

/** LLM 错误响应故障 */
export class LLMErrorFault implements LLMFault {
  readonly statusCode: number;
  readonly errorMessage: string;
  readonly modelTarget: string;
  readonly probability: number;
  private active = false;

  constructor(statusCode: number, errorMessage: string, modelTarget: string, probability = 1.0) {
    this.statusCode = statusCode;
    this.errorMessage = errorMessage;
    this.modelTarget = modelTarget;
    this.probability = probability;
  }

  async inject(): Promise<void> {
    if (Math.random() > this.probability) return;
    this.active = true;
  }

  async cleanup(): Promise<void> { this.active = false; }
  type(): string { return 'llm_error'; }
  description(): string { return `LLM ${this.modelTarget} 返回 ${this.statusCode}: ${this.errorMessage}`; }
  affectsModel(model: string): boolean {
    return this.modelTarget === '*' || model.includes(this.modelTarget);
  }
  get isActive(): boolean { return this.active; }
}

/** LLM 限流故障 */
export class LLMRateLimitFault implements LLMFault {
  readonly maxRequests: number;
  readonly windowMs: number;
  readonly modelTarget: string;
  private requestCount = 0;
  private windowStart = Date.now();
  private active = false;

  constructor(maxRequests: number, windowMs: number, modelTarget: string) {
    this.maxRequests = maxRequests;
    this.windowMs = windowMs;
    this.modelTarget = modelTarget;
  }

  async inject(): Promise<void> { this.active = true; }
  async cleanup(): Promise<void> { this.active = false; this.requestCount = 0; }
  type(): string { return 'llm_rate_limit'; }
  description(): string { return `LLM ${this.modelTarget} 限流 ${this.maxRequests} req/${this.windowMs}ms`; }
  affectsModel(model: string): boolean {
    return this.modelTarget === '*' || model.includes(this.modelTarget);
  }

  /** 检查是否应限流 */
  shouldThrottle(): boolean {
    if (!this.active) return false;
    const now = Date.now();
    if (now - this.windowStart > this.windowMs) {
      this.windowStart = now;
      this.requestCount = 0;
    }
    this.requestCount++;
    return this.requestCount > this.maxRequests;
  }
}

/** NoopFault 空操作故障（用于测试） */
export class NoopFault implements FaultInjector {
  async inject(): Promise<void> {}
  async cleanup(): Promise<void> {}
  type(): string { return 'noop'; }
  description(): string { return 'no-op fault'; }
}
