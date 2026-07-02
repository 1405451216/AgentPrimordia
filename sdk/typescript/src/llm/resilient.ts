import type { Provider } from './provider.js';
import type { ToolCallRequest, ToolCallResponse, CompletionRequest, CompletionResponse, Chunk, ModelInfo } from '../types.js';
import { APIError } from './openai.js';

function isRetryable(err: unknown): boolean {
  if (err instanceof APIError) {
    return err.status >= 500 || err.status === 429 || err.status === 408;
  }
  return true; // Network errors are retryable
}

export class ResilientProvider implements Provider {
  private primary: Provider;
  private fallbacks: Provider[] = [];
  private maxRetries: number;
  private retryBackoff: number;
  private maxBackoff: number;
  private circuitThreshold: number;
  private circuitRecoverAfter: number;
  private failures = 0;
  private lastFailTime = 0;
  private circuitOpen = false;

  constructor(primary: Provider, opts?: { maxRetries?: number; retryBackoff?: number; maxBackoff?: number; circuitThreshold?: number; circuitRecoverAfter?: number }) {
    this.primary = primary;
    this.maxRetries = opts?.maxRetries ?? 3;
    this.retryBackoff = opts?.retryBackoff ?? 500;
    this.maxBackoff = opts?.maxBackoff ?? 10000;
    this.circuitThreshold = opts?.circuitThreshold ?? 5;
    this.circuitRecoverAfter = opts?.circuitRecoverAfter ?? 30000;
  }

  addFallback(provider: Provider): void {
    this.fallbacks.push(provider);
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    this.checkCircuit();
    try {
      const result = await this.executeWithRetry(() => this.primary.complete(req), (p) => p.complete(req));
      this.recordSuccess();
      return result;
    } catch (err) {
      this.recordFailure();
      throw err;
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    this.checkCircuit();
    try {
      const result = await this.executeWithRetry(() => this.primary.callTools(req), (p) => p.callTools(req));
      this.recordSuccess();
      return result;
    } catch (err) {
      this.recordFailure();
      throw err;
    }
  }

  info(): ModelInfo {
    return this.primary.info();
  }

  /** 获取熔断器运行时状态（公开 API，替代直接访问私有字段）。
   *
   * 返回值说明：
   * - state: 'closed'（正常）/ 'open'（熔断中）/ 'half_open'（已过恢复时间，下次请求将放行）
   * - failures: 当前连续失败次数
   * - lastFailTime: 上次失败的时间戳（毫秒）
   * - circuitThreshold: 触发熔断的失败次数阈值
   * - circuitRecoverMs: 熔断恢复时间（毫秒）
   */
  getBreakerState(): {
    state: 'closed' | 'open' | 'half_open';
    failures: number;
    lastFailTime: number;
    circuitThreshold: number;
    circuitRecoverMs: number;
  } {
    let state: 'closed' | 'open' | 'half_open' = 'closed';
    if (this.circuitOpen) {
      if (Date.now() - this.lastFailTime > this.circuitRecoverAfter) {
        state = 'half_open';
      } else {
        state = 'open';
      }
    }
    return {
      state,
      failures: this.failures,
      lastFailTime: this.lastFailTime,
      circuitThreshold: this.circuitThreshold,
      circuitRecoverMs: this.circuitRecoverAfter,
    };
  }

  /** 重置熔断器状态（将 failures 清零、circuitOpen 置 false） */
  resetBreaker(): void {
    this.failures = 0;
    this.circuitOpen = false;
    this.lastFailTime = 0;
  }

  private checkCircuit(): void {
    if (this.circuitOpen && Date.now() - this.lastFailTime > this.circuitRecoverAfter) {
      this.circuitOpen = false;
    }
    if (this.circuitOpen) {
      throw new Error('circuit breaker is open');
    }
  }

  private recordSuccess(): void {
    this.failures = 0;
    this.circuitOpen = false;
  }

  private recordFailure(): void {
    this.failures++;
    this.lastFailTime = Date.now();
    if (this.failures >= this.circuitThreshold) {
      this.circuitOpen = true;
    }
  }

  private async executeWithRetry<T>(fn: () => Promise<T>, fallbackFn: (p: Provider) => Promise<T>): Promise<T> {
    let lastErr: Error | null = null;
    for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
      if (attempt > 0) {
        const backoff = Math.min(this.retryBackoff * Math.pow(2, attempt - 1), this.maxBackoff);
        await new Promise((r) => setTimeout(r, backoff));
      }
      try {
        return await fn();
      } catch (err: unknown) {
        if (!isRetryable(err)) throw err;
        lastErr = err instanceof Error ? err : new Error(String(err));
      }
    }
    for (const fallback of this.fallbacks) {
      try {
        return await fallbackFn(fallback);
      } catch (err: unknown) {
        lastErr = err instanceof Error ? err : new Error(String(err));
      }
    }
    throw lastErr ?? new Error('all providers failed');
  }
}

// ===== P4-A3: RateLimitedProvider — 令牌桶限流 Provider 包装器 =====
// 与 Go 端 RateLimitedProvider 对齐，使用令牌桶算法平滑 LLM API 调用速率。
// 防止突发流量触发 429 Too Many Requests，适用于多 Agent 共享 API 配额场景。
//
// 使用方式：
//   const provider = new RateLimitedProvider(openaiProvider, { maxRPM: 60, burst: 10 });
//   const agent = new ReActAgent({ model: provider, ... });

export interface RateLimitConfig {
  /** 每分钟最大请求数（RPM），默认 60 */
  maxRPM?: number;
  /** 突发容量（允许短时间内超出 RPM 的最大请求数），默认为 maxRPM */
  burst?: number;
  /** 最大等待时间（毫秒），超时则抛错，默认 30000 */
  maxWaitMs?: number;
}

export class RateLimitedProvider implements Provider {
  private inner: Provider;
  private tokens: number;
  private maxTokens: number;
  private refillRate: number; // tokens per millisecond
  private lastRefill: number;
  private maxWaitMs: number;

  constructor(inner: Provider, config?: RateLimitConfig) {
    this.inner = inner;
    const rpm = config?.maxRPM ?? 60;
    this.maxTokens = config?.burst ?? rpm;
    this.tokens = this.maxTokens;
    this.refillRate = rpm / 60000; // tokens per millisecond
    this.lastRefill = Date.now();
    this.maxWaitMs = config?.maxWaitMs ?? 30000;
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    await this.acquire();
    return this.inner.complete(req);
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    await this.acquire();
    return this.inner.callTools(req);
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    await this.acquire();
    if (this.inner.stream) {
      yield* this.inner.stream(req);
    } else {
      const resp = await this.inner.complete(req);
      yield { content: resp.content, done: true, usage: resp.usage };
    }
  }

  info(): ModelInfo {
    return this.inner.info();
  }

  /** 获取当前可用令牌数 */
  availableTokens(): number {
    this.refill();
    return Math.floor(this.tokens);
  }

  private async acquire(): Promise<void> {
    const waitStart = Date.now();
    while (true) {
      this.refill();
      if (this.tokens >= 1) {
        this.tokens -= 1;
        return;
      }
      // 计算等待时间
      const needed = 1 - this.tokens;
      const waitMs = Math.ceil(needed / this.refillRate);
      if (Date.now() - waitStart + waitMs > this.maxWaitMs) {
        throw new Error(`Rate limit exceeded: would need to wait more than ${this.maxWaitMs}ms`);
      }
      await new Promise((r) => setTimeout(r, Math.min(waitMs, 100)));
    }
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = now - this.lastRefill;
    this.tokens = Math.min(this.maxTokens, this.tokens + elapsed * this.refillRate);
    this.lastRefill = now;
  }
}

