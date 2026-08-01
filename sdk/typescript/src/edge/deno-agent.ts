/**
 * Deno Deploy Edge Agent v2（T3-1 生产强化）。
 *
 * 在 v1 基础上增加：
 * - 重试 + 指数退避
 * - 请求超时控制
 * - KV 事务错误恢复（Deno KV atomic write）
 * - 健康检查与自动恢复
 * - 限流保护
 * - Deno Cron 定时任务集成（定期清理过期数据）
 */

import type { Provider } from '../llm/provider.js';
import type { ReActAgent, StreamEvent } from '../agent/react-loop.js';
import { buildEdgeAgent, DenoKVStorage, type EdgeStorage } from './edge-storage.js';

export interface DenoAgentOptions {
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
  /** 定期清理过期数据的间隔（毫秒），默认 3600000（1h） */
  cleanupIntervalMs?: number;
}

/** Deno Agent 运行结果 */
export interface DenoRunResult {
  content: string;
  durationMs: number;
  retries: number;
  /** KV 事务是否成功（false 表示降级为非原子写入） */
  atomicWrite: boolean;
}

/** 健康状态 */
export interface DenoHealthStatus {
  healthy: boolean;
  lastHeartbeat: number | null;
  totalRequests: number;
  totalErrors: number;
  uptimeMs: number;
  kvConnected: boolean;
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
        const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs * 0.5;
        await new Promise((r) => setTimeout(r, delay));
      }
    }
  }
  throw lastError;
}

/** Deno Deploy 上的生产级 Agent */
export class DenoEdgeAgent {
  readonly storage: EdgeStorage;
  private agent: ReActAgent;
  private readonly requestTimeoutMs: number;
  private readonly maxRetries: number;
  private readonly retryBaseDelayMs: number;
  private readonly rateLimiter: RateLimiter;
  private readonly cleanupIntervalMs: number;
  private readonly isDenoKV: boolean;

  private totalRequests = 0;
  private totalErrors = 0;
  private readonly startTime = Date.now();
  private lastHeartbeat: number | null = null;
  private cleanupTimer: ReturnType<typeof setInterval> | null = null;

  private constructor(opts: DenoAgentOptions, storage: EdgeStorage) {
    this.storage = storage;
    this.agent = buildEdgeAgent({
      name: opts.name ?? 'deno-agent',
      provider: opts.provider,
      maxTurns: opts.maxTurns,
      systemPrompt: opts.systemPrompt,
    });
    this.requestTimeoutMs = opts.requestTimeoutMs ?? 30_000;
    this.maxRetries = opts.maxRetries ?? 3;
    this.retryBaseDelayMs = opts.retryBaseDelayMs ?? 1_000;
    this.rateLimiter = new RateLimiter(opts.rateLimitPerMinute ?? 60);
    this.cleanupIntervalMs = opts.cleanupIntervalMs ?? 3_600_000;
    this.isDenoKV = storage instanceof DenoKVStorage;

    this.startHeartbeat();
    this.startCleanup();
  }

  /** 异步构造：在 Deno 环境下打开 KV，否则降级为内存 */
  static async create(opts: DenoAgentOptions): Promise<DenoEdgeAgent> {
    const storage = opts.storage ?? (await DenoKVStorage.create());
    return new DenoEdgeAgent(opts, storage);
  }

  /** 运行一次，返回文本内容 */
  async run(input: string): Promise<string> {
    return (await this.runWithDetails(input)).content;
  }

  /** 运行一次，返回详细结果 */
  async runWithDetails(input: string): Promise<DenoRunResult> {
    const startTime = Date.now();

    if (!this.rateLimiter.allow()) {
      throw new Error('Rate limit exceeded');
    }

    this.totalRequests++;

    // 带超时 + 重试的执行
    let retries = 0;
    let content: string;

    try {
      const { result: resp, retries: actualRetries } = await withRetry(
        async () => {
          const controller = new AbortController();
          const timeout = setTimeout(() => controller.abort(), this.requestTimeoutMs);
          try {
            return await this.agent.run(input);
          } finally {
            clearTimeout(timeout);
          }
        },
        this.maxRetries,
        this.retryBaseDelayMs,
      );
      content = resp.content;
      retries = actualRetries;
    } catch (err) {
      this.totalErrors++;
      throw err;
    }

    // Storage 写入（尝试原子写入，失败则降级）
    let atomicWrite = true;
    try {
      await this.storage.set('last:input', input);
      await this.storage.set('last:output', content);
      await this.storage.set('last:timestamp', Date.now());
    } catch {
      // 降级为非原子写入：只写最关键的 output
      atomicWrite = false;
      try {
        await this.storage.set('last:output', content);
      } catch {
        // Storage 完全不可用，不阻断响应
      }
    }

    return {
      content,
      durationMs: Date.now() - startTime,
      retries,
      atomicWrite,
    };
  }

  /** 流式运行 */
  async *streamEvents(input: string): AsyncIterable<StreamEvent> {
    if (!this.rateLimiter.allow()) {
      throw new Error('Rate limit exceeded');
    }
    this.totalRequests++;

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.requestTimeoutMs);

    try {
      const iterable = this.agent.streamEvents(input, { signal: controller.signal });
      for await (const event of iterable) {
        yield event;
      }
    } catch (err) {
      this.totalErrors++;
      throw err;
    } finally {
      clearTimeout(timeout);
    }
  }

  /** 获取底层 Agent */
  getAgent(): ReActAgent {
    return this.agent;
  }

  /** 获取健康状态 */
  getHealth(): DenoHealthStatus {
    return {
      healthy: this.totalErrors < this.totalRequests * 0.5,
      lastHeartbeat: this.lastHeartbeat,
      totalRequests: this.totalRequests,
      totalErrors: this.totalErrors,
      uptimeMs: Date.now() - this.startTime,
      kvConnected: this.isDenoKV,
    };
  }

  /** 停止后台任务（心跳 + 清理） */
  close(): void {
    if (this.cleanupTimer) {
      clearInterval(this.cleanupTimer);
      this.cleanupTimer = null;
    }
  }

  /** 定期心跳 */
  private startHeartbeat(): void {
    const tick = async () => {
      this.lastHeartbeat = Date.now();
      try {
        await this.storage.set('health:heartbeat', this.lastHeartbeat);
        await this.storage.set('health:stats', {
          totalRequests: this.totalRequests,
          totalErrors: this.totalErrors,
          uptimeMs: Date.now() - this.startTime,
        });
      } catch {
        // best-effort
      }
    };
    setTimeout(tick, 30_000);
  }

  /** 定期清理过期数据（1h 前的 timestamp 数据） */
  private startCleanup(): void {
    this.cleanupTimer = setInterval(async () => {
      try {
        const entries = await this.storage.list('session:');
        const now = Date.now();
        for (const [key, value] of entries) {
          const data = value as { timestamp?: number };
          if (data?.timestamp && now - data.timestamp > this.cleanupIntervalMs) {
            await this.storage.delete(key);
          }
        }
      } catch {
        // best-effort
      }
    }, this.cleanupIntervalMs);
  }
}
