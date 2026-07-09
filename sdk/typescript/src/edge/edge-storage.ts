/**
 * Edge Storage — Edge-Native Agent 的统一存储抽象（T3-1）。
 *
 * 设计目标：让同一套 Agent 逻辑跑在 Cloudflare Workers / Deno Deploy / Bun 上，
 * 底层存储用各平台原生 KV/SQLite，但都实现同一个 EdgeStorage 接口。
 *
 * 平台适配做了「特征检测 + 降级」：在 Node / 测试环境自动降级为内存实现，
 * 因此本文件及其使用者可在 Node 下单测，无需真实平台。
 */

import type { Provider } from '../llm/provider.js';
import { createAgent } from '../agent/builder.js';
import type { ReActAgent } from '../agent/react-loop.js';
import { ToolRegistry } from '../tools/registry.js';

/** 统一存储接口：kv 风格 */
export interface EdgeStorage {
  get(key: string): Promise<unknown>;
  set(key: string, value: unknown): Promise<void>;
  delete(key: string): Promise<void>;
  /** 列出带前缀的 key-value 对 */
  list(prefix: string): Promise<[string, unknown][]>;
}

/** 内存实现（默认 / 测试 / 降级） */
export class MemoryEdgeStorage implements EdgeStorage {
  private map = new Map<string, unknown>();
  private _errorCount = 0;
  private _lastError: number | null = null;

  async get(key: string): Promise<unknown> {
    return this.map.get(key) ?? null;
  }
  async set(key: string, value: unknown): Promise<void> {
    this.map.set(key, value);
  }
  async delete(key: string): Promise<void> {
    this.map.delete(key);
  }
  async list(prefix: string): Promise<[string, unknown][]> {
    const out: [string, unknown][] = [];
    for (const [k, v] of this.map) {
      if (k.startsWith(prefix)) out.push([k, v]);
    }
    return out;
  }
  /** 清空（测试用） */
  clear(): void {
    this.map.clear();
  }
  /** 错误统计 */
  get errorCount(): number { return this._errorCount; }
  get lastError(): number | null { return this._lastError; }
  /** 模拟错误（测试用） */
  injectError(): void { this._errorCount++; this._lastError = Date.now(); }
}

/**
 * Cloudflare KV 适配器。
 * 在 Workers 中，KV 命名空间由调用方通过 env 注入（如 env.MY_KV）。
 * 若未注入或在非 CF 环境，自动降级为内存存储。
 */
export class CloudflareKVStorage implements EdgeStorage {
  private ns: { get(k: string): Promise<unknown>; put(k: string, v: string): Promise<unknown>; delete(k: string): Promise<unknown>; list(opts: { prefix: string }): Promise<{ keys: { name: string }[] }> } | null;
  private fallback = new MemoryEdgeStorage();
  private _errorCount = 0;
  private _lastError: number | null = null;

  constructor(namespace?: unknown) {
    const g = globalThis as Record<string, unknown>;
    this.ns = (namespace ?? g.MY_KV ?? null) as CloudflareKVStorage['ns'];
  }

  private async kv(): Promise<CloudflareKVStorage['ns']> {
    return this.ns;
  }

  /** 带错误恢复的 KV 操作：失败时降级到内存存储 */
  private async withRecovery<T>(
    op: string,
    kvFn: () => Promise<T>,
    fallbackFn: () => Promise<T>,
  ): Promise<T> {
    const ns = await this.kv();
    if (!ns) return fallbackFn();
    try {
      return await kvFn();
    } catch (err) {
      this._errorCount++;
      this._lastError = Date.now();
      // 降级到内存存储
      return fallbackFn();
    }
  }

  async get(key: string): Promise<unknown> {
    return this.withRecovery(
      'get',
      async () => (await this.kv())!.get(key),
      async () => this.fallback.get(key),
    );
  }
  async set(key: string, value: unknown): Promise<void> {
    await this.withRecovery(
      'set',
      async () => { await (await this.kv())!.put(key, JSON.stringify(value)); },
      async () => this.fallback.set(key, value),
    );
  }
  async delete(key: string): Promise<void> {
    await this.withRecovery(
      'delete',
      async () => { await (await this.kv())!.delete(key); },
      async () => this.fallback.delete(key),
    );
  }
  async list(prefix: string): Promise<[string, unknown][]> {
    return this.withRecovery(
      'list',
      async () => {
        const ns = (await this.kv())!;
        const res = await ns.list({ prefix });
        const out: [string, unknown][] = [];
        for (const k of res.keys) {
          out.push([k.name, await ns.get(k.name)]);
        }
        return out;
      },
      async () => this.fallback.list(prefix),
    );
  }

  /** 是否正在使用 KV 命名空间（而非降级内存） */
  isUsingKV(): boolean { return this.ns !== null; }
  /** 错误计数 */
  get errorCount(): number { return this._errorCount; }
  /** 健康检查 */
  isHealthy(): boolean { return this._errorCount < 10; }
}

/**
 * Deno KV 适配器。Deno.openKv() 是异步的，故用静态工厂 create() 打开。
 * 非 Deno 环境：返回内存实现。
 */
export class DenoKVStorage implements EdgeStorage {
  private kv: { get(k: unknown[]): Promise<{ value?: unknown }>; set(k: unknown[], v: unknown): Promise<unknown>; delete(k: unknown[]): Promise<unknown>; list(opts: { prefix: unknown[] }): AsyncIterable<{ key: unknown[]; value?: unknown }> } | null = null;
  private fallback = new MemoryEdgeStorage();
  private _errorCount = 0;
  private _lastError: number | null = null;

  private constructor() {}

  static async create(): Promise<DenoKVStorage> {
    const inst = new DenoKVStorage();
    const g = globalThis as Record<string, unknown>;
    if (typeof g.Deno !== 'undefined' && (g.Deno as { openKv?: () => Promise<unknown> }).openKv) {
      try {
        inst.kv = (await (g.Deno as { openKv: () => Promise<DenoKVStorage['kv']> }).openKv()) as DenoKVStorage['kv'];
      } catch {
        // KV 打开失败，降级为内存
        inst.kv = null;
      }
    }
    return inst;
  }

  /** 带错误恢复的 KV 操作 */
  private async withRecovery<T>(
    kvFn: () => Promise<T>,
    fallbackFn: () => Promise<T>,
  ): Promise<T> {
    if (!this.kv) return fallbackFn();
    try {
      return await kvFn();
    } catch {
      this._errorCount++;
      this._lastError = Date.now();
      return fallbackFn();
    }
  }

  async get(key: string): Promise<unknown> {
    return this.withRecovery(
      async () => (await this.kv!.get([key])).value ?? null,
      async () => this.fallback.get(key),
    );
  }
  async set(key: string, value: unknown): Promise<void> {
    await this.withRecovery(
      async () => { await this.kv!.set([key], value); },
      async () => this.fallback.set(key, value),
    );
  }
  async delete(key: string): Promise<void> {
    await this.withRecovery(
      async () => { await this.kv!.delete([key]); },
      async () => this.fallback.delete(key),
    );
  }
  async list(prefix: string): Promise<[string, unknown][]> {
    return this.withRecovery(
      async () => {
        const out: [string, unknown][] = [];
        for await (const entry of this.kv!.list({ prefix: [prefix] })) {
          out.push([(entry.key as unknown[])[0] as string, entry.value ?? null]);
        }
        return out;
      },
      async () => this.fallback.list(prefix),
    );
  }

  /** 是否连接了 Deno KV */
  isKVConnected(): boolean { return this.kv !== null; }
  /** 错误计数 */
  get errorCount(): number { return this._errorCount; }
  /** 健康检查 */
  isHealthy(): boolean { return this._errorCount < 10; }
}

/**
 * Bun SQLite 适配器。Bun.sqlite 是同步 API。
 * 非 Bun 环境：降级为内存实现。
 */
export class BunSQLiteStorage implements EdgeStorage {
  private db: { query(s: string, ...args: unknown[]): { all(): unknown[] } } | null = null;
  private fallback = new MemoryEdgeStorage();

  constructor() {
    const g = globalThis as Record<string, unknown>;
    const bun = g.Bun as { sqlite?: (path: string) => BunSQLiteStorage['db'] } | undefined;
    if (bun?.sqlite) {
      try {
        const db2 = bun.sqlite(':memory:');
        if (db2) {
          this.db = db2;
          db2.query('CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT)');
        }
      } catch {
        this.db = null;
      }
    }
  }

  async get(key: string): Promise<unknown> {
    if (!this.db) return this.fallback.get(key);
    const rows = this.db.query('SELECT v FROM kv WHERE k = ?', key).all() as Array<{ v: string }>;
    return rows.length ? JSON.parse(rows[0]!.v) : null;
  }
  async set(key: string, value: unknown): Promise<void> {
    if (!this.db) return this.fallback.set(key, value);
    this.db.query('INSERT OR REPLACE INTO kv (k, v) VALUES (?, ?)', key, JSON.stringify(value)).all();
  }
  async delete(key: string): Promise<void> {
    if (!this.db) return this.fallback.delete(key);
    this.db.query('DELETE FROM kv WHERE k = ?', key).all();
  }
  async list(prefix: string): Promise<[string, unknown][]> {
    if (!this.db) return this.fallback.list(prefix);
    const rows = this.db.query('SELECT k, v FROM kv WHERE k LIKE ?', `${prefix}%`).all() as Array<{ k: string; v: string }>;
    return rows.map((r) => [r.k, JSON.parse(r.v)] as [string, unknown]);
  }
}

/** 检测当前 Edge 平台 */
export function detectEdgePlatform(): 'cloudflare' | 'deno' | 'bun' | 'node' | 'browser' | 'unknown' {
  const g = globalThis as Record<string, unknown>;
  if (typeof g.Deno !== 'undefined') return 'deno';
  if (typeof g.Bun !== 'undefined') return 'bun';
  if (typeof g.document !== 'undefined') return 'browser';
  if (typeof g.MY_KV !== 'undefined' || typeof (g as Record<string, unknown>).WorkerGlobalScope !== 'undefined') return 'cloudflare';
  if (typeof g.process !== 'undefined' && (g.process as { versions?: { node?: string } }).versions?.node) return 'node';
  return 'unknown';
}

/**
 * 共享的 Edge Agent 构造助手：使用空工具集构建 ReActAgent。
 * 各平台 agent 文件复用此函数，避免重复样板。
 */
export function buildEdgeAgent(opts: {
  name?: string;
  provider: Provider;
  maxTurns?: number;
  systemPrompt?: string;
}): ReActAgent {
  return createAgent(opts.name ?? 'edge-agent')
    .withProvider(opts.provider)
    .withToolkit(new ToolRegistry())
    .withMaxTurns(opts.maxTurns ?? 10)
    .withSystemPrompt(opts.systemPrompt ?? '')
    .build();
}
