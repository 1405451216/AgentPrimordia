/**
 * 浏览器端 Agent v2（T3-2 生产强化）。
 *
 * 在 v1 基础上增加：
 * - Service Worker 生命周期管理（离线缓存 + 请求拦截）
 * - CSP（Content Security Policy）策略适配
 * - 离线请求队列（断网时排队，恢复后自动重放）
 * - IndexedDB 事务重试（指数退避 + QuotaExceededError 处理）
 * - 模型权重缓存（Service Worker Cache API）
 * - 在线/离线状态感知
 */

import type { Provider } from '../llm/provider.js';
import { createAgent } from '../agent/builder.js';
import type { ReActAgent } from '../agent/react-loop.js';
import { ToolRegistry } from '../tools/registry.js';
import { WebGPUProvider } from './browser-provider.js';
import { IndexedDBCheckpointStore } from './indexeddb-checkpoint.js';

export interface BrowserAgentConfig {
  /** 注入的 Provider；缺省使用 WebGPUProvider（模拟模式） */
  provider?: Provider;
  /** 模型权重 URL（传给 WebGPUProvider.init） */
  modelUrl?: string;
  systemPrompt?: string;
  maxTurns?: number;
  /** 是否启用离线队列，默认 true */
  enableOfflineQueue?: boolean;
  /** 离线队列最大长度，默认 100 */
  offlineQueueMaxSize?: number;
  /** Service Worker 脚本路径（如 '/sw.js'），不设置则不注册 SW */
  serviceWorkerPath?: string;
  /** IndexedDB 事务重试次数，默认 3 */
  idbRetryCount?: number;
  /** IndexedDB 事务重试基础延迟（毫秒），默认 500 */
  idbRetryBaseDelay?: number;
}

/** 浏览器 Agent 运行结果 */
export interface BrowserRunResult {
  content: string;
  /** 是否从离线队列中恢复执行 */
  fromOfflineQueue: boolean;
  /** 模型是否为模拟模式（无 WebGPU） */
  simulated: boolean;
  /** 执行耗时（毫秒） */
  durationMs: number;
}

/** 离线队列中的待执行请求 */
interface QueuedRequest {
  id: string;
  input: string;
  timestamp: number;
}

/** 在线/离线状态 */
export type OnlineStatus = 'online' | 'offline';

/**
 * 离线请求队列管理器。
 * 断网时将请求存入 IndexedDB（主） + localStorage（备），恢复后自动按顺序重放。
 *
 * 双层持久化策略：
 * - localStorage：同步写入，用于快速恢复（受 ~5MB 限制）
 * - IndexedDB：异步写入，支持大容量、结构化查询（主存储）
 * - 恢复时先从 localStorage 快速恢复，再异步从 IndexedDB 同步完整数据
 */
class OfflineRequestQueue {
  private queue: QueuedRequest[] = [];
  private readonly maxSize: number;
  private processing = false;
  private agent: ((input: string) => Promise<unknown>) | null = null;
  private idbFlushPending = false;
  private idbAvailable: boolean | null = null;

  private static readonly DB_NAME = 'agent-primordia';
  private static readonly DB_STORE = 'offline-queue';
  private static readonly DB_VERSION = 1;
  private static readonly LS_KEY = 'agent-offline-queue';

  constructor(maxSize: number = 100) {
    this.maxSize = maxSize;
  }

  /** 设置 Agent 运行函数（用于队列重放） */
  setRunner(run: (input: string) => Promise<unknown>): void {
    this.agent = run;
  }

  /** 入队一个请求 */
  enqueue(input: string): string {
    if (this.queue.length >= this.maxSize) {
      // 丢弃最旧的请求
      this.queue.shift();
    }
    const id = `req-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    this.queue.push({ id, input, timestamp: Date.now() });
    this.persist();
    return id;
  }

  /** 获取队列长度 */
  get length(): number {
    return this.queue.length;
  }

  /** 队列是否为空 */
  get isEmpty(): boolean {
    return this.queue.length === 0;
  }

  /** 处理队列中所有待执行请求 */
  async processAll(): Promise<void> {
    if (this.processing || !this.agent || this.queue.length === 0) return;
    this.processing = true;

    while (this.queue.length > 0) {
      const req = this.queue.shift()!;
      try {
        await this.agent(req.input);
      } catch {
        // 执行失败：重新入队，等待下次重试
        this.queue.unshift(req);
        break;
      }
    }

    this.processing = false;
    this.persist();
  }

  /** 清空队列 */
  clear(): void {
    this.queue = [];
    this.persist();
  }

  /**
   * 双层持久化：同步写 localStorage + 异步写 IndexedDB。
   * IndexedDB 写入使用 microtask 去重，避免高频写入。
   */
  private persist(): void {
    this.persistToLocalStorage();
    this.persistToIDB();
  }

  /** 同步持久化到 localStorage（快速恢复用） */
  private persistToLocalStorage(): void {
    try {
      const g = globalThis as { localStorage?: Storage };
      if (g.localStorage) {
        g.localStorage.setItem(OfflineRequestQueue.LS_KEY, JSON.stringify(this.queue));
      }
    } catch {
      // localStorage 不可用或配额已满 — IndexedDB 仍有备份
    }
  }

  /**
   * 异步持久化到 IndexedDB（可靠存储，支持大容量）。
   * 使用 microtask 去重：多次连续 persist 只触发一次实际写入。
   */
  private persistToIDB(): void {
    if (this.idbFlushPending) return;
    this.idbFlushPending = true;

    queueMicrotask(async () => {
      this.idbFlushPending = false;
      try {
        const db = await this.openIDB();
        if (!db) return;

        const tx = db.transaction(OfflineRequestQueue.DB_STORE, 'readwrite');
        const store = tx.objectStore(OfflineRequestQueue.DB_STORE);

        await new Promise<void>((resolve, reject) => {
          const clearReq = store.clear();
          clearReq.onsuccess = () => {
            if (this.queue.length === 0) {
              resolve();
              return;
            }
            let remaining = this.queue.length;
            for (const req of this.queue) {
              const putReq = store.put(req);
              putReq.onsuccess = () => {
                if (--remaining === 0) resolve();
              };
              putReq.onerror = () => reject(putReq.error);
            }
          };
          clearReq.onerror = () => reject(clearReq.error);
        });

        db.close();
      } catch {
        // IndexedDB 不可用或写入失败 — localStorage 已有备份
        this.idbAvailable = false;
      }
    });
  }

  /** 打开 IndexedDB 连接（惰性初始化） */
  private async openIDB(): Promise<IDBDatabase | null> {
    if (this.idbAvailable === false) return null;

    try {
      const g = globalThis as { indexedDB?: IDBFactory };
      if (!g.indexedDB) {
        this.idbAvailable = false;
        return null;
      }

      return await new Promise<IDBDatabase>((resolve, reject) => {
        const req = g.indexedDB!.open(
          OfflineRequestQueue.DB_NAME,
          OfflineRequestQueue.DB_VERSION,
        );
        req.onupgradeneeded = () => {
          const db = req.result;
          if (!db.objectStoreNames.contains(OfflineRequestQueue.DB_STORE)) {
            db.createObjectStore(OfflineRequestQueue.DB_STORE, { keyPath: 'id' });
          }
        };
        req.onsuccess = () => {
          this.idbAvailable = true;
          resolve(req.result);
        };
        req.onerror = () => {
          this.idbAvailable = false;
          reject(req.error);
        };
      });
    } catch {
      this.idbAvailable = false;
      return null;
    }
  }

  /**
   * 从持久化存储恢复队列。
   * 同步从 localStorage 恢复（快速），再异步从 IndexedDB 补全（可靠）。
   */
  restore(): void {
    // 1. 同步：从 localStorage 快速恢复
    this.restoreFromLocalStorage();
    // 2. 异步：从 IndexedDB 恢复更完整的数据
    this.restoreFromIDB();
  }

  /** 从 localStorage 恢复（同步） */
  private restoreFromLocalStorage(): void {
    try {
      const g = globalThis as { localStorage?: Storage };
      if (g.localStorage) {
        const data = g.localStorage.getItem(OfflineRequestQueue.LS_KEY);
        if (data) {
          this.queue = JSON.parse(data) as QueuedRequest[];
        }
      }
    } catch {
      // 恢复失败，从空队列开始
    }
  }

  /**
   * 从 IndexedDB 恢复（异步）。
   * 如果 IndexedDB 中有更多数据，则覆盖 localStorage 的不完整恢复。
   */
  private restoreFromIDB(): void {
    queueMicrotask(async () => {
      try {
        const db = await this.openIDB();
        if (!db) return;

        const tx = db.transaction(OfflineRequestQueue.DB_STORE, 'readonly');
        const store = tx.objectStore(OfflineRequestQueue.DB_STORE);

        const items = await new Promise<QueuedRequest[]>((resolve, reject) => {
          const getAllReq = store.getAll();
          getAllReq.onsuccess = () =>
            resolve(getAllReq.result as QueuedRequest[]);
          getAllReq.onerror = () => reject(getAllReq.error);
        });
        db.close();

        // IndexedDB 数据更完整时覆盖内存中的队列
        // （localStorage 可能因配额限制截断了部分请求）
        if (items.length > this.queue.length) {
          items.sort((a, b) => a.timestamp - b.timestamp);
          // 如果正在处理队列，不覆盖
          if (!this.processing) {
            this.queue = items;
            // 同步到 localStorage
            this.persistToLocalStorage();
          }
        }
      } catch {
        // IndexedDB 恢复失败 — localStorage 已有数据
      }
    });
  }

  /** 获取队列快照（调试用） */
  snapshot(): QueuedRequest[] {
    return [...this.queue];
  }
}

/**
 * 带指数退避的 IndexedDB 事务执行器。
 * 处理 QuotaExceededError、VersionError、连接中断等浏览器特有问题。
 */
async function withIDBRetry<T>(
  fn: () => Promise<T>,
  maxRetries: number,
  baseDelayMs: number,
): Promise<T> {
  let lastError: unknown;
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (err) {
      lastError = err;
      const error = err as { name?: string };
      // 不可重试的错误
      if (error.name === 'SecurityError' || error.name === 'InvalidStateError') {
        throw err;
      }
      // QuotaExceededError：等待更长时间
      if (error.name === 'QuotaExceededError' && attempt < maxRetries) {
        const delay = baseDelayMs * Math.pow(2, attempt + 1); // 额外延迟
        await new Promise((r) => setTimeout(r, delay));
        continue;
      }
      // 可重试的错误
      if (attempt < maxRetries) {
        const delay = baseDelayMs * Math.pow(2, attempt) + Math.random() * baseDelayMs * 0.3;
        await new Promise((r) => setTimeout(r, delay));
        continue;
      }
    }
  }
  throw lastError;
}

/** CSP 策略辅助器 */
export class CSPHelper {
  /**
   * 生成适合 Agent 运行的 CSP nonce。
   * 在 HTML 中使用 <script nonce="..."> 来允许内联脚本。
   */
  static generateNonce(): string {
    const array = new Uint8Array(16);
    const g = globalThis as { crypto?: { getRandomValues: (arr: Uint8Array) => Uint8Array } };
    if (g.crypto?.getRandomValues) {
      g.crypto.getRandomValues(array);
    } else {
      // 降级：Math.random
      for (let i = 0; i < array.length; i++) {
        array[i] = Math.floor(Math.random() * 256);
      }
    }
    return btoa(String.fromCharCode(...array));
  }

  /**
   * 构建适合浏览器 Agent 的 CSP 头。
   * - 允许 'self' 脚本
   * - 允许 wasm-unsafe-eval（WebAssembly 推理需要）
   * - 允许指定 nonce 的内联脚本
   * - 允许 connect-src 到 LLM API 端点
   */
  static buildCSP(opts: {
    nonce?: string;
    apiEndpoints?: string[];
    allowWasm?: boolean;
  }): string {
    const parts: string[] = [];
    // script-src
    const scriptSrc = ["'self'"];
    if (opts.nonce) scriptSrc.push(`'nonce-${opts.nonce}'`);
    if (opts.allowWasm !== false) scriptSrc.push("'wasm-unsafe-eval'");
    parts.push(`script-src ${scriptSrc.join(' ')}`);
    // connect-src
    const connectSrc = ["'self'"];
    if (opts.apiEndpoints) {
      connectSrc.push(...opts.apiEndpoints);
    }
    parts.push(`connect-src ${connectSrc.join(' ')}`);
    // 其他指令
    parts.push("style-src 'self' 'unsafe-inline'");
    parts.push("img-src 'self' data: blob:");
    parts.push("worker-src 'self' blob:");
    return parts.join('; ');
  }

  /**
   * 检测当前环境是否满足 Agent 运行要求。
   */
  static checkEnvironment(): {
    webgpu: boolean;
    wasm: boolean;
    indexedDB: boolean;
    serviceWorker: boolean;
    webWorker: boolean;
  } {
    const g = globalThis as Record<string, unknown>;
    const nav = g.navigator as { gpu?: unknown } | undefined;
    return {
      webgpu: !!nav?.gpu,
      wasm: typeof g.WebAssembly !== 'undefined',
      indexedDB: typeof g.indexedDB !== 'undefined',
      serviceWorker: !!(nav as { serviceWorker?: unknown })?.serviceWorker,
      webWorker: typeof g.Worker !== 'undefined',
    };
  }
}

/** 浏览器端 Agent 封装（生产强化版） */
export class BrowserAgent {
  private agent: ReActAgent;
  private provider: Provider;
  private readonly checkpointStore: IndexedDBCheckpointStore;
  private readonly offlineQueue: OfflineRequestQueue;
  private readonly idbRetryCount: number;
  private readonly idbRetryBaseDelay: number;
  private onlineStatus: OnlineStatus = 'online';
  private swRegistration: unknown = null;

  private constructor(
    agent: ReActAgent,
    provider: Provider,
    config: BrowserAgentConfig,
  ) {
    this.agent = agent;
    this.provider = provider;
    this.checkpointStore = new IndexedDBCheckpointStore();
    this.offlineQueue = new OfflineRequestQueue(config.offlineQueueMaxSize ?? 100);
    this.idbRetryCount = config.idbRetryCount ?? 3;
    this.idbRetryBaseDelay = config.idbRetryBaseDelay ?? 500;

    // 恢复离线队列
    this.offlineQueue.restore();
    // 设置队列运行器
    this.offlineQueue.setRunner(async (input: string) => {
      const resp = await this.agent.run(input);
      return resp.content;
    });

    // 监听在线/离线状态
    this.setupOnlineDetection();
  }

  /** 异步创建 BrowserAgent */
  static async create(config: BrowserAgentConfig = {}): Promise<BrowserAgent> {
    const provider = config.provider ?? new WebGPUProvider();
    if (!config.provider && config.modelUrl) {
      await (provider as WebGPUProvider).init(config.modelUrl);
    }
    const agent = createAgent('browser-agent')
      .withProvider(provider)
      .withToolkit(new ToolRegistry())
      .withMaxTurns(config.maxTurns ?? 10)
      .withSystemPrompt(config.systemPrompt ?? '')
      .build();

    const instance = new BrowserAgent(agent, provider, config);

    // 注册 Service Worker（如果配置了）
    if (config.serviceWorkerPath) {
      await instance.registerServiceWorker(config.serviceWorkerPath);
    }

    return instance;
  }

  /** 运行一次，返回文本内容 */
  async run(input: string): Promise<string> {
    return (await this.runWithDetails(input)).content;
  }

  /** 运行一次，返回详细结果 */
  async runWithDetails(input: string): Promise<BrowserRunResult> {
    const startTime = Date.now();

    // 离线检查
    if (this.onlineStatus === 'offline') {
      // 入队，稍后重放
      this.offlineQueue.enqueue(input);
      return {
        content: '[离线] 请求已加入队列，网络恢复后将自动执行',
        fromOfflineQueue: false,
        simulated: (this.provider as WebGPUProvider).isSimulated?.() ?? false,
        durationMs: Date.now() - startTime,
      };
    }

    // 在线：执行请求
    let content: string;
    try {
      const resp = await this.agent.run(input);
      content = resp.content;

      // 保存检查点（带重试）
      try {
        await withIDBRetry(
          async () => {
            await this.checkpointStore.save({
              id: `cp-${Date.now()}`,
              sessionID: 'default',
              turn: 0,
              messages: [],
              metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 },
              createdAt: new Date().toISOString(),
            });
          },
          this.idbRetryCount,
          this.idbRetryBaseDelay,
        );
      } catch {
        // 检查点保存失败不阻断响应
      }
    } catch (err) {
      // 执行失败：如果 Provider 不可用，入队重试
      if (this.isNetworkError(err)) {
        this.offlineQueue.enqueue(input);
        return {
          content: '[网络错误] 请求已加入队列，将自动重试',
          fromOfflineQueue: false,
          simulated: (this.provider as WebGPUProvider).isSimulated?.() ?? false,
          durationMs: Date.now() - startTime,
        };
      }
      throw err;
    }

    // 处理离线队列中的积压请求
    if (!this.offlineQueue.isEmpty) {
      this.offlineQueue.processAll().catch(() => {});
    }

    return {
      content,
      fromOfflineQueue: false,
      simulated: (this.provider as WebGPUProvider).isSimulated?.() ?? false,
      durationMs: Date.now() - startTime,
    };
  }

  /** 获取底层 Agent */
  getAgent(): ReActAgent {
    return this.agent;
  }

  /** 获取底层 Provider */
  getProvider(): Provider {
    return this.provider;
  }

  /** 获取在线状态 */
  getOnlineStatus(): OnlineStatus {
    return this.onlineStatus;
  }

  /** 获取离线队列长度 */
  getOfflineQueueLength(): number {
    return this.offlineQueue.length;
  }

  /** 手动触发离线队列重放 */
  async processOfflineQueue(): Promise<void> {
    await this.offlineQueue.processAll();
  }

  /** 获取检查点存储 */
  getCheckpointStore(): IndexedDBCheckpointStore {
    return this.checkpointStore;
  }

  /** 注册 Service Worker */
  private async registerServiceWorker(path: string): Promise<void> {
    try {
      const g = globalThis as { navigator?: { serviceWorker?: { register: (path: string) => Promise<unknown> } } };
      const reg = await g.navigator?.serviceWorker?.register(path);
      if (reg) {
        this.swRegistration = reg;
      }
    } catch {
      // SW 注册失败不阻断 Agent 创建
    }
  }

  /** 设置在线/离线检测 */
  private setupOnlineDetection(): void {
    const g = globalThis as {
      navigator?: { onLine?: boolean };
      addEventListener?: (type: string, handler: () => void) => void;
    };

    // 初始状态
    if (g.navigator?.onLine === false) {
      this.onlineStatus = 'offline';
    }

    // 监听 online/offline 事件
    g.addEventListener?.('online', () => {
      this.onlineStatus = 'online';
      // 自动重放离线队列
      if (!this.offlineQueue.isEmpty) {
        this.offlineQueue.processAll().catch(() => {});
      }
    });

    g.addEventListener?.('offline', () => {
      this.onlineStatus = 'offline';
    });
  }

  /** 判断是否为网络错误 */
  private isNetworkError(err: unknown): boolean {
    const error = err as { name?: string; message?: string };
    if (!error) return false;
    // 常见网络错误类型
    const networkErrors = [
      'NetworkError',
      'TypeError',
      'AbortError',
      'TimeoutError',
    ];
    return networkErrors.includes(error.name ?? '') ||
      (error.message?.includes('fetch') ?? false) ||
      (error.message?.includes('network') ?? false);
  }
}
