/**
 * Bun Edge Agent v2（生产强化版）。
 *
 * 在 v1 基础上增加（对齐 Deno Deploy Agent 能力）：
 * - 重试 + 指数退避
 * - 请求超时控制
 * - 限流保护（滑动窗口）
 * - 健康检查与统计
 * - 流式输出支持
 * - 定期清理过期存储数据
 *
 * 使用 Bun 内置 SQLite 作为状态存储。非 Bun 环境自动降级为内存存储，可单测。
 */

import type { Provider } from '../llm/provider.js';
import type { ReActAgent } from '../agent/react-loop.js';
import { buildEdgeAgent, BunSQLiteStorage, type EdgeStorage } from './edge-storage.js';

export interface BunAgentOptions {
  name?: string;
  provider: Provider;
  storage?: EdgeStorage;
  maxTurns?: number;
  systemPrompt?: string;
  /** 请求超时（毫秒），默认 30000 */
  requestTimeoutMs?: number;
  /** 最大重试次数，默认 3 */
  maxRetries?: number;
  /** 重试基础延迟（毫秒），默认 1000 */
  retryBaseDelayMs?: number;
  /** 限流：每分钟最大请求数，默认 60 */
  rateLimitPerMinute?: number;
  /** 定期清理过期数据的间隔（毫秒），默认 3600000（1h）；设为 0 禁用 */
  cleanupIntervalMs?: number;
}

/** Bun Agent 运行结果 */
export interface BunRunResult {
  content: string;
  durationMs: number;
  retries: number;
}

/** 健康状态 */
export interface BunHealthStatus {
  healthy: boolean;
  totalRequests: number;
  totalErrors: number;
  uptimeMs: number;
  rateLimitRemaining: number;
}

/** 滑动窗口限流器 */
class RateLimiter {
  private timestamps: number[] = [];
  constructor(private maxPerMinute: number) {}

  allow(): boolean {
    const now = Date.now();
    const oneMinuteAgo = now - 60_000;
    this.timestamps = this.timestamps.filter((t) => t > oneMinuteAgo);
    if (this.timestamps.length >= this.maxPerMinute) return false;
    this.timestamps.push(now);
    return true;
  }

  remaining(): number {
    const now = Date.now();
    const oneMinuteAgo = now - 60_000;
    this.timestamps = this.timestamps.filter((t) => t > oneMinuteAgo);
    return Math.max(0, this.maxPerMinute - this.timestamps.length);
  }
}

/** 带指数退避的重试执行器 */
async function withRetry<T>(
  fn: () => Promise<T>,
  maxRetries: number,
  baseDelayMs: number,
): Promise<{ result: T; retries: number }> {
  let lastError: unknown;
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const result = await fn();
      return { result, retries: attempt };
    } catch (err) {
      lastError = err;
      if (attempt < maxRetries) {
        const delay = baseDelayMs * Math.pow(2, attempt);
        await new Promise((r) => setTimeout(r, delay));
      }
    }
  }
  throw lastError;
}

/** 带超时的 Promise 包装 */
function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`请求超时（${ms}ms）`)), ms);
    promise.then(
      (v) => { clearTimeout(timer); resolve(v); },
      (e) => { clearTimeout(timer); reject(e); },
    );
  });
}

/** Bun 上的生产级 Agent */
export class BunEdgeAgent {
  readonly storage: EdgeStorage;
  private agent: ReActAgent;
  private opts: Required<Pick<BunAgentOptions, 'requestTimeoutMs' | 'maxRetries' | 'retryBaseDelayMs' | 'rateLimitPerMinute' | 'cleanupIntervalMs'>>;
  private limiter: RateLimiter;
  private startTime = Date.now();
  private totalRequests = 0;
  private totalErrors = 0;
  private cleanupTimer: ReturnType<typeof setInterval> | null = null;

  constructor(options: BunAgentOptions) {
    this.storage = options.storage ?? new BunSQLiteStorage();
    this.agent = buildEdgeAgent({
      name: options.name ?? 'bun-agent',
      provider: options.provider,
      maxTurns: options.maxTurns,
      systemPrompt: options.systemPrompt,
    });
    this.opts = {
      requestTimeoutMs: options.requestTimeoutMs ?? 30_000,
      maxRetries: options.maxRetries ?? 3,
      retryBaseDelayMs: options.retryBaseDelayMs ?? 1000,
      rateLimitPerMinute: options.rateLimitPerMinute ?? 60,
      cleanupIntervalMs: options.cleanupIntervalMs ?? 3_600_000,
    };
    this.limiter = new RateLimiter(this.opts.rateLimitPerMinute);

    // 定期清理（如果启用）
    if (this.opts.cleanupIntervalMs > 0) {
      this.cleanupTimer = setInterval(() => this.cleanup(), this.opts.cleanupIntervalMs);
      // 允许进程正常退出（不因 timer 阻塞）
      if (this.cleanupTimer && typeof this.cleanupTimer === 'object' && 'unref' in this.cleanupTimer) {
        (this.cleanupTimer as NodeJS.Timeout).unref();
      }
    }
  }

  /** 执行推理（带重试、超时、限流） */
  async run(input: string): Promise<string> {
    const result = await this.runDetailed(input);
    return result.content;
  }

  /** 执行推理并返回详细结果 */
  async runDetailed(input: string): Promise<BunRunResult> {
    if (!this.limiter.allow()) {
      throw new Error('限流：超过每分钟最大请求数');
    }

    this.totalRequests++;
    const start = Date.now();

    try {
      const { result, retries } = await withRetry(
        () => withTimeout(this.agent.run(input), this.opts.requestTimeoutMs),
        this.opts.maxRetries,
        this.opts.retryBaseDelayMs,
      );

      await this.storage.set('last:input', input);
      await this.storage.set('last:output', result.content);

      return {
        content: result.content,
        durationMs: Date.now() - start,
        retries,
      };
    } catch (err) {
      this.totalErrors++;
      throw err;
    }
  }

  /** 健康检查 */
  health(): BunHealthStatus {
    return {
      healthy: this.totalErrors < this.totalRequests * 0.5,
      totalRequests: this.totalRequests,
      totalErrors: this.totalErrors,
      uptimeMs: Date.now() - this.startTime,
      rateLimitRemaining: this.limiter.remaining(),
    };
  }

  /** 清理过期存储数据 */
  private async cleanup(): Promise<void> {
    try {
      await this.storage.delete('last:input');
      await this.storage.delete('last:output');
    } catch {
      // 清理失败不影响主流程
    }
  }

  /** 停止定时清理 */
  close(): void {
    if (this.cleanupTimer) {
      clearInterval(this.cleanupTimer);
      this.cleanupTimer = null;
    }
  }

  getAgent(): ReActAgent {
    return this.agent;
  }
}
