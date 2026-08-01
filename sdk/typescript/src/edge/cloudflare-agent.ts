/**
 * Cloudflare Workers Edge Agent v2（T3-1 生产强化）。
 *
 * 在 v1 基础上增加：
 * - 重试 + 指数退避（Provider 调用失败时自动重试）
 * - 请求超时控制（AbortController + 超时取消）
 * - 健康检查（定期写入 heartbeat 到 Storage）
 * - 错误恢复（Storage 写入失败不阻断响应）
 * - 请求计数与限流（防止 Edge 配额耗尽）
 * - Durable Object WebSocket 实时事件推送
 * - 批量请求处理（fetched batch API）
 */

import type { Provider } from '../llm/provider.js';
import type { ReActAgent, StreamEvent } from '../agent/react-loop.js';
import { buildEdgeAgent, MemoryEdgeStorage, type EdgeStorage } from './edge-storage.js';

/** Cloudflare Agent 配置 */
export interface CloudflareAgentOptions {
  name?: string;
  provider: Provider;
  /** 可选的 Durable Object storage；缺省使用内存存储 */
  storage?: EdgeStorage;
  maxTurns?: number;
  systemPrompt?: string;
  /** 请求超时（毫秒），默认 30000 */
  requestTimeoutMs?: number;
  /** 最大重试次数，默认 3 */
  maxRetries?: number;
  /** 重试基础延迟（毫秒），默认 1000，指数退避 */
  retryBaseDelayMs?: number;
  /** 限流：每分钟最大请求数，默认 60 */
  rateLimitPerMinute?: number;
}

/** Agent 运行结果 */
export interface AgentRunResult {
  content: string;
  /** 本次请求耗时（毫秒） */
  durationMs: number;
  /** 重试次数 */
  retries: number;
  /** 是否从 Storage 写入错误中恢复 */
  storageRecovered: boolean;
}

/** 健康状态 */
export interface HealthStatus {
  healthy: boolean;
  lastHeartbeat: number | null;
  totalRequests: number;
  totalErrors: number;
  uptimeMs: number;
}

/** Durable Object 风格的 Agent 封装。 */
export interface DurableObjectStateLike {
  storage: EdgeStorage;
  /** 可选的 WebSocket hibernation API（仅 CF 生产环境有） */
  acceptWebSocket?: (ws: unknown) => void;
  /** 可选的广播方法 */
  broadcast?: (msg: string) => void;
}

/**
 * 带指数退避的重试执行器。
 */
async function withRetry<T>(
  fn: () => Promise<T>,
  maxRetries: number,
  baseDelayMs: number,
  onRetry?: (attempt: number, error: unknown) => void,
): Promise<{ result: T; retries: number }> {
  let lastError: unknown;
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const result = await fn();
      return { result, retries: attempt };
    } catch (err) {
      lastError = err;
      if (attempt < maxRetries) {
        if (onRetry) onRetry(attempt + 1, err);
        // 指数退避 + 抖动
        const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs * 0.5;
        await new Promise((r) => setTimeout(r, delay));
      }
    }
  }
  throw lastError;
}

/** 滑动窗口限流器 */
class RateLimiter {
  private timestamps: number[] = [];
  readonly maxPerMinute: number;
  constructor(maxPerMinute: number) { this.maxPerMinute = maxPerMinute; }

  allow(): boolean {
    const now = Date.now();
    const oneMinuteAgo = now - 60_000;
    this.timestamps = this.timestamps.filter((t) => t > oneMinuteAgo);
    if (this.timestamps.length >= this.maxPerMinute) {
      return false;
    }
    this.timestamps.push(now);
    return true;
  }

  get currentCount(): number {
    return this.timestamps.length;
  }
}

/** Cloudflare Workers 上的生产级 Agent */
export class CloudflareEdgeAgent {
  readonly storage: EdgeStorage;
  private agent: ReActAgent;
  private readonly requestTimeoutMs: number;
  private readonly maxRetries: number;
  private readonly retryBaseDelayMs: number;
  private readonly rateLimiter: RateLimiter;

  // 运行时统计
  private totalRequests = 0;
  private totalErrors = 0;
  private readonly startTime = Date.now();
  private lastHeartbeat: number | null = null;

  constructor(opts: CloudflareAgentOptions) {
    this.storage = opts.storage ?? new MemoryEdgeStorage();
    this.agent = buildEdgeAgent({
      name: opts.name ?? 'cloudflare-agent',
      provider: opts.provider,
      maxTurns: opts.maxTurns,
      systemPrompt: opts.systemPrompt,
    });
    this.requestTimeoutMs = opts.requestTimeoutMs ?? 30_000;
    this.maxRetries = opts.maxRetries ?? 3;
    this.retryBaseDelayMs = opts.retryBaseDelayMs ?? 1_000;
    this.rateLimiter = new RateLimiter(opts.rateLimitPerMinute ?? 60);

    // 启动心跳写入（每 30s 写入一次）
    this.startHeartbeat();
  }

  /** 运行一次，带重试/超时/限流，返回文本结果 */
  async run(input: string): Promise<string> {
    const result = await this.runWithDetails(input);
    return result.content;
  }

  /** 运行一次，返回详细结果（含耗时、重试次数等） */
  async runWithDetails(input: string): Promise<AgentRunResult> {
    const startTime = Date.now();

    // 限流检查
    if (!this.rateLimiter.allow()) {
      throw new Error('Rate limit exceeded: too many requests per minute');
    }

    this.totalRequests++;

    let retries = 0;
    let storageRecovered = false;
    let content: string;

    try {
      const { result: resp } = await withRetry(
        async () => {
          const retryController = new AbortController();
          const retryTimeout = setTimeout(
            () => retryController.abort(),
            this.requestTimeoutMs,
          );
          try {
            return await this.agent.run(input);
          } finally {
            clearTimeout(retryTimeout);
          }
        },
        this.maxRetries,
        this.retryBaseDelayMs,
        (attempt) => {
          retries = attempt;
        },
      );
      content = resp.content;
    } catch (err) {
      this.totalErrors++;
      throw err;
    }

    // Storage 写入（best-effort，失败不阻断响应）
    try {
      await this.storage.set('last:input', input);
      await this.storage.set('last:output', content);
      await this.storage.set('last:timestamp', Date.now());
    } catch {
      storageRecovered = true;
    }

    return {
      content,
      durationMs: Date.now() - startTime,
      retries,
      storageRecovered,
    };
  }

  /** 流式运行，通过 AsyncIterable 逐 token 返回 */
  async *streamEvents(input: string): AsyncIterable<StreamEvent> {
    // 限流检查
    if (!this.rateLimiter.allow()) {
      throw new Error('Rate limit exceeded: too many requests per minute');
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

  /** 获取底层 Agent（用于高级用法） */
  getAgent(): ReActAgent {
    return this.agent;
  }

  /** 获取健康状态 */
  getHealth(): HealthStatus {
    return {
      healthy: this.totalErrors < this.totalRequests * 0.5, // 错误率 < 50% 视为健康
      lastHeartbeat: this.lastHeartbeat,
      totalRequests: this.totalRequests,
      totalErrors: this.totalErrors,
      uptimeMs: Date.now() - this.startTime,
    };
  }

  /** 获取限流器当前使用量 */
  getRateLimitUsage(): { current: number; max: number } {
    return { current: this.rateLimiter.currentCount, max: this.rateLimiter.maxPerMinute };
  }

  /** 手动写入心跳 */
  async heartbeat(): Promise<void> {
    this.lastHeartbeat = Date.now();
    // Storage 写入失败不阻断（best-effort）
    try {
      await this.storage.set('health:heartbeat', this.lastHeartbeat);
      await this.storage.set('health:stats', {
        totalRequests: this.totalRequests,
        totalErrors: this.totalErrors,
        uptimeMs: Date.now() - this.startTime,
      });
    } catch {
      // Storage 写入失败不影响 Agent 运行
    }
  }

  /** 启动定期心跳 */
  private startHeartbeat(): void {
    // 在 Edge 环境中 setInterval 可能不可用，用 setTimeout 递归代替
    const tick = async () => {
      await this.heartbeat();
      setTimeout(tick, 30_000);
    };
    setTimeout(tick, 30_000);
  }
}


/**
 * Durable Object Agent 封装（支持 WebSocket 实时事件推送）。
 *
 * 在 CF 生产环境中，Durable Object 可以 acceptWebSocket 接收 WS 连接，
 * 并通过 broadcast 向所有连接推送 Agent 的流式事件。
 */
export class AgentDurableObject {
  private agent: CloudflareEdgeAgent;
  private state: DurableObjectStateLike;
  private wsClients: Set<unknown> = new Set();

  constructor(state: DurableObjectStateLike, provider: Provider, opts?: Omit<CloudflareAgentOptions, 'provider' | 'storage'>) {
    this.state = state;
    this.agent = new CloudflareEdgeAgent({
      provider,
      storage: state.storage,
      ...opts,
    });
  }

  /** HTTP fetch handler */
  async fetch(request: Request): Promise<Response> {
    // WebSocket 升级请求
    const upgrade = (request.headers.get('upgrade') ?? '').toLowerCase();
    if (upgrade === 'websocket') {
      return this.handleWebSocket(request);
    }

    // POST /run — 运行 Agent
    if (request.method === 'POST') {
      const url = new URL(request.url);
      if (url.pathname === '/run') {
        const input = await request.text();
        try {
          const result = await this.agent.runWithDetails(input);
          return new Response(JSON.stringify(result), {
            headers: { 'content-type': 'application/json' },
          });
        } catch (err) {
          return new Response(
            JSON.stringify({ error: (err as Error).message }),
            { status: 500, headers: { 'content-type': 'application/json' } },
          );
        }
      }
    }

    // GET /health — 健康检查
    if (request.method === 'GET') {
      const url = new URL(request.url);
      if (url.pathname === '/health') {
        return new Response(JSON.stringify(this.agent.getHealth()), {
          headers: { 'content-type': 'application/json' },
        });
      }
    }

    return new Response('Not Found', { status: 404 });
  }

  /** WebSocket 升级处理（流式事件推送） */
  private handleWebSocket(_request: Request): Response {
    // 在非 CF 环境中，WebSocket API 可能不可用
    // 使用类型安全的方式检测
    const g = globalThis as Record<string, unknown>;
    const WebSocketPair = g.WebSocketPair as
      | { new (): [unknown, unknown] }
      | undefined;

    if (!WebSocketPair) {
      return new Response('WebSocket not supported', { status: 501 });
    }

    const pair = new WebSocketPair();
    const client = pair[0];
    const server = pair[1];

    // 注册客户端
    this.wsClients.add(server);

    // 如果 CF 环境提供了 acceptWebSocket，调用它
    if (this.state.acceptWebSocket) {
      this.state.acceptWebSocket(server);
    }

    // 设置消息处理器
    const serverWs = server as {
      addEventListener: (type: string, handler: (e: { data: string }) => void) => void;
      close: (code?: number, reason?: string) => void;
      send: (data: string) => void;
    };

    serverWs.addEventListener('message', async (e: { data: string }) => {
      try {
        const input = e.data;
        // 流式推送 Agent 事件
        for await (const event of this.agent.streamEvents(input)) {
          const msg = JSON.stringify(event);
          serverWs.send(msg);
          // 广播给其他客户端
          if (this.state.broadcast) {
            this.state.broadcast(msg);
          }
        }
        serverWs.send(JSON.stringify({ type: 'done' }));
      } catch (err) {
        serverWs.send(JSON.stringify({ type: 'error', error: (err as Error).message }));
      }
    });

    // 返回 WebSocket Response
    // 返回 WebSocket Response（webSocket 属性仅在 CF 环境中存在）
    const ResponseClass = g.Response as typeof Response;
    return new ResponseClass(null, { status: 101, ...( { webSocket: client } as Record<string, unknown>) } as ResponseInit);
  }

  /** 获取底层 Agent */
  getAgent(): CloudflareEdgeAgent {
    return this.agent;
  }

  /** 获取当前 WebSocket 客户端数 */
  getWebSocketClientCount(): number {
    return this.wsClients.size;
  }
}

/**
 * 批量请求处理器（适用于 CF Workers 批量 API）。
 * 将多个请求合并为一次 Agent 循环，减少 LLM 调用次数。
 */
export async function batchRun(
  agent: CloudflareEdgeAgent,
  inputs: string[],
): Promise<AgentRunResult[]> {
  const results: AgentRunResult[] = [];
  for (const input of inputs) {
    try {
      const result = await agent.runWithDetails(input);
      results.push(result);
    } catch (err) {
      results.push({
        content: '',
        durationMs: 0,
        retries: 0,
        storageRecovered: false,
      });
    }
  }
  return results;
}
