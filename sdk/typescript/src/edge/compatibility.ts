/**
 * Edge Runtime 兼容层 — 检测运行时环境并提供统一 API。
 *
 * 支持：Cloudflare Workers / Vercel Edge / Deno / Bun / Node.js / Browser
 *
 * 与 runtime.ts 的区别：
 * - runtime.ts 提供底层运行时检测和平台 API 适配
 * - compatibility.ts 提供高层业务兼容：Agent 构建、KV 存储、fetch 封装
 *
 * 使用方式：
 *   import { createAgent, createKVStore } from './compatibility.js';
 *   const agent = await createAgent({ name: 'edge-agent' });
 *   const kv = createKVStore();
 */

import { detectRuntime, type RuntimeInfo } from './runtime.js';

// ===== 类型导出 =====

export type { RuntimeInfo };
export { detectRuntime, supports, getWebSocketConstructor, createTimer } from './runtime.js';

// ===== KV 存储接口 =====

export interface KVStore {
  get(key: string): Promise<string | null>;
  put(key: string, value: string, options?: { expirationTtl?: number }): Promise<void>;
  delete(key: string): Promise<void>;
  list(options?: { prefix?: string; limit?: number }): Promise<string[]>;
}

// ===== 运行时环境 =====

export type RuntimeEnv = 'cloudflare-workers' | 'vercel-edge' | 'deno' | 'bun' | 'node' | 'browser' | 'unknown';

/** 检测当前运行时环境（简化版，用于业务层） */
export function detectEnv(): RuntimeEnv {
  const rt = detectRuntime();
  switch (rt.name) {
    case 'cloudflare':
      return 'cloudflare-workers';
    case 'deno':
      return 'deno';
    case 'bun':
      return 'bun';
    case 'node':
      return 'node';
    case 'browser':
      return 'browser';
    default:
      return 'unknown';
  }
}

/** 是否为 Edge Runtime（Cloudflare Workers / Vercel Edge） */
export function isEdgeRuntime(): boolean {
  return detectRuntime().isEdge;
}

/** 是否为 Node.js 环境 */
export function isNodeRuntime(): boolean {
  return detectRuntime().isNode;
}

// ===== KV 存储实现 =====

/** 内存 KV 存储（fallback） */
class InMemoryKVStore implements KVStore {
  private store = new Map<string, { value: string; expires?: number }>();

  async get(key: string): Promise<string | null> {
    const entry = this.store.get(key);
    if (!entry) return null;
    if (entry.expires && Date.now() > entry.expires) {
      this.store.delete(key);
      return null;
    }
    return entry.value;
  }

  async put(key: string, value: string, options?: { expirationTtl?: number }): Promise<void> {
    const expires = options?.expirationTtl ? Date.now() + options.expirationTtl * 1000 : undefined;
    this.store.set(key, { value, expires });
  }

  async delete(key: string): Promise<void> {
    this.store.delete(key);
  }

  async list(options?: { prefix?: string; limit?: number }): Promise<string[]> {
    const keys: string[] = [];
    for (const [key] of this.store) {
      if (!options?.prefix || key.startsWith(options.prefix)) {
        keys.push(key);
      }
    }
    return keys.slice(0, options?.limit ?? 1000);
  }
}

/** Cloudflare Workers KV 存储 */
class CloudflareKVStore implements KVStore {
  constructor(private kv: unknown) {}

  async get(key: string): Promise<string | null> {
    const kv = this.kv as { get: (key: string) => Promise<string | null> };
    return kv.get(key);
  }

  async put(key: string, value: string, options?: { expirationTtl?: number }): Promise<void> {
    const kv = this.kv as { put: (key: string, value: string, options?: { expirationTtl?: number }) => Promise<void> };
    await kv.put(key, value, options);
  }

  async delete(key: string): Promise<void> {
    const kv = this.kv as { delete: (key: string) => Promise<void> };
    await kv.delete(key);
  }

  async list(options?: { prefix?: string; limit?: number }): Promise<string[]> {
    const kv = this.kv as { list: (options?: { prefix?: string; limit?: number }) => Promise<{ keys: { name: string }[] }> };
    const result = await kv.list(options);
    return result.keys.map((k) => k.name);
  }
}

/** Vercel Edge KV 存储（基于 Upstash Redis） */
class VercelEdgeKVStore implements KVStore {
  constructor(private kv: unknown) {}

  async get(key: string): Promise<string | null> {
    const kv = this.kv as { get: (key: string) => Promise<string | null> };
    return kv.get(key);
  }

  async put(key: string, value: string, options?: { expirationTtl?: number }): Promise<void> {
    const kv = this.kv as { set: (key: string, value: string, options?: { ex?: number }) => Promise<void> };
    await kv.set(key, value, options?.expirationTtl ? { ex: options.expirationTtl } : undefined);
  }

  async delete(key: string): Promise<void> {
    const kv = this.kv as { del: (key: string) => Promise<void> };
    await kv.del(key);
  }

  async list(options?: { prefix?: string; limit?: number }): Promise<string[]> {
    const kv = this.kv as { keys: (pattern: string) => Promise<string[]> };
    const pattern = options?.prefix ? `${options.prefix}*` : '*';
    const keys = await kv.keys(pattern);
    return keys.slice(0, options?.limit ?? 1000);
  }
}

/**
 * 创建适合当前运行时的 KV 存储实例。
 *
 * 优先级：
 * 1. Cloudflare Workers: 使用 globalThis.env.KV
 * 2. Vercel Edge: 使用 process.env.KV（Upstash Redis）
 * 3. Deno: 使用 Deno.openKv()
 * 4. 其他: 使用内存 KV（fallback）
 */
export function createKVStore(): KVStore {
  const rt = detectRuntime();

  // Cloudflare Workers
  if (rt.name === 'cloudflare') {
    const env = (globalThis as Record<string, unknown>).env as Record<string, unknown> | undefined;
    if (env?.KV) {
      return new CloudflareKVStore(env.KV);
    }
  }

  // Vercel Edge
  if (rt.name === 'node') {
    const env = process.env;
    if (env.KV_REST_API_URL && env.KV_REST_API_TOKEN) {
      // Vercel KV (Upstash Redis) - 动态导入避免 Node.js 依赖
      try {
        // @ts-ignore - 运行时动态加载
        const { kv: vercelKV } = await import('@vercel/kv');
        return new VercelEdgeKVStore(vercelKV);
      } catch {
        // 不可用，降级到内存
      }
    }
  }

  // Deno
  if (rt.name === 'deno') {
    try {
      // @ts-ignore - Deno 全局
      const kv = await Deno.openKv();
      return {
        get: (key: string) => kv.get([key]).then((r: unknown) => (r as { value?: string })?.value ?? null),
        put: (key: string, value: string) => kv.set([key], value).then(() => {}),
        delete: (key: string) => kv.delete([key]).then(() => {}),
        list: async (options?: { prefix?: string }) => {
          const entries = kv.list({ prefix: options?.prefix ? [options.prefix] : [] });
          const keys: string[] = [];
          for await (const entry of entries) {
            keys.push((entry.key as string[])[0]);
          }
          return keys;
        },
      };
    } catch {
      // 不可用，降级到内存
    }
  }

  // Fallback: 内存 KV
  return new InMemoryKVStore();
}

// ===== Edge-compatible fetch =====

/**
 * Edge Runtime 兼容的 fetch 封装。
 *
 * 与原生 fetch 的区别：
 * - Edge Runtime 不支持 keep-alive，自动关闭连接
 * - Edge Runtime 不支持 streaming response 的某些特性
 * - 自动添加 User-Agent 标识
 */
export async function edgeFetch(
  url: string,
  init: RequestInit = {},
): Promise<Response> {
  const rt = detectRuntime();

  // Edge Runtime 特殊处理
  if (rt.isEdge) {
    const headers = new Headers(init.headers);
    headers.set('User-Agent', 'AgentPrimordia-Edge/1.0');
    // Edge Runtime 不支持 keep-alive
    if (!init.headers || !new Headers(init.headers).has('Connection')) {
      headers.set('Connection', 'close');
    }

    return fetch(url, {
      ...init,
      headers,
    });
  }

  // Node.js / Deno / Bun: 直接 fetch
  return fetch(url, init);
}

// ===== Agent 构建 =====

/**
 * 在 Edge Runtime 中创建 Agent 实例。
 *
 * 与 Node.js 的区别：
 * - Edge Runtime 不支持长连接，Agent 需要配置更短的超时
 * - Edge Runtime 不支持文件系统，Agent 不能使用 filesystem 工具
 * - Edge Runtime 的 LLM 调用需要走 fetch（不支持 Node.js 的 http 模块）
 */
export async function createAgent(options: {
  name: string;
  systemPrompt?: string;
  maxTurns?: number;
  timeout?: number;
  llmEndpoint?: string;
  llmApiKey?: string;
}): Promise<{
  name: string;
  run: (prompt: string) => Promise<string>;
  streamRun: (prompt: string) => AsyncGenerator<string>;
}> {
  const rt = detectRuntime();

  // Edge Runtime 默认配置
  const timeout = rt.isEdge ? (options.timeout ?? 25000) : (options.timeout ?? 60000);
  const maxTurns = rt.isEdge ? (options.maxTurns ?? 5) : (options.maxTurns ?? 10);

  return {
    name: options.name,
    run: async (prompt: string): Promise<string> => {
      // 简化实现：实际应调用 Agent 构建器
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), timeout);

      try {
        // 这里应该调用实际的 Agent 构建和运行逻辑
        // 简化版本：直接返回模拟响应
        await new Promise((resolve) => setTimeout(resolve, 100));
        return `[${rt.name}] Agent ${options.name} 响应: ${prompt}`;
      } finally {
        clearTimeout(timeoutId);
      }
    },
    streamRun: async function* (prompt: string): AsyncGenerator<string> {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), timeout);

      try {
        // 简化实现：逐字输出
        const response = `[${rt.name}] Agent ${options.name} 流式响应: ${prompt}`;
        for (const char of response) {
          if (controller.signal.aborted) return;
          yield char;
          await new Promise((resolve) => setTimeout(resolve, 10));
        }
      } finally {
        clearTimeout(timeoutId);
      }
    },
  };
}

// ===== 导出 =====

export default {
  detectEnv,
  isEdgeRuntime,
  isNodeRuntime,
  createKVStore,
  edgeFetch,
  createAgent,
};
