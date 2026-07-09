// hot-update.ts — Phase 2 T2-2 Prompt 热更新
// 不重启服务的前提下，监听 prompt 文件变化或 HTTP 端点，自动更新活跃版本。
//
// 支持三种更新源：
//   1. FileWatcherSource：监听本地文件（使用 fs.watch）
//   2. PollingSource：定时 HTTP 拉取（适用于远程配置中心）
//   3. ManualSource：手动调用 update() 触发（适用于测试/手动管理）
//
// 线程安全：所有状态变更通过 callback 通知订阅者；订阅者不应阻塞事件循环。

import * as fs from 'node:fs';
import * as path from 'node:path';
import type { VersionedPromptRegistry, PromptVersion } from './versioned-registry.js';

/** 热更新事件 */
export type HotUpdateEvent =
  | { type: 'version_added'; name: string; version: PromptVersion }
  | { type: 'version_activated'; name: string; version: PromptVersion }
  | { type: 'error'; error: Error }
  | { type: 'source_stopped'; reason?: string };

export type HotUpdateListener = (event: HotUpdateEvent) => void;

/** 更新源接口 */
export interface HotUpdateSource {
  /** 启动源（开始监听） */
  start(): Promise<void>;
  /** 停止源 */
  stop(): Promise<void>;
  /** 是否正在运行 */
  isRunning(): boolean;
}

// ===== 文件监听源 =====

export interface FileWatcherOptions {
  /** 监听目录 */
  dir: string;
  /** 文件名 → prompt 名 的映射（不指定则使用文件名去掉 .md/.txt 等后缀） */
  filenameToName?: (filename: string) => string;
  /** 文件名 → prompt 名 的映射（可选，覆盖 filenameToName 的行为） */
  filenameToPath?: (filename: string) => string;
  /** 文件稳定性检查（避免半写入读取），默认 100ms */
  stabilityMs?: number;
}

/**
 * FileWatcherSource — 监听 prompt 文件目录变化
 * 文件命名约定：{promptName}.md 或 .txt
 * 文件内容直接作为 prompt 内容
 */
export class FileWatcherSource implements HotUpdateSource {
  private watcher: fs.FSWatcher | null = null;
  private timers = new Map<string, NodeJS.Timeout>();
  private running = false;
  private options: Required<FileWatcherOptions>;

  constructor(options: FileWatcherOptions) {
    this.options = {
      dir: options.dir,
      filenameToName: options.filenameToName ?? ((f) => f.replace(/\.[^.]+$/, '')),
      filenameToPath: options.filenameToPath ?? ((f: string) => f),
      stabilityMs: options.stabilityMs ?? 100,
    };
  }

  start(): Promise<void> {
    if (this.running) return Promise.resolve();
    return new Promise<void>((resolve, reject) => {
      this.watcher = fs.watch(this.options.dir, (eventType, filename) => {
        if (!filename) return;
        const fullPath = path.join(this.options.dir, filename);
        // 防抖：相同文件短时间内多次变化只读取一次
        const existing = this.timers.get(fullPath);
        if (existing) clearTimeout(existing);
        this.timers.set(
          fullPath,
          setTimeout(() => {
            this.timers.delete(fullPath);
            this.readAndUpdate(fullPath, filename);
          }, this.options.stabilityMs),
        );
      });
      this.watcher.on('error', (err) => reject(err));
      this.watcher.on('ready', () => {
        this.running = true;
        resolve();
      });
    });
  }

  stop(): Promise<void> {
    if (!this.running) return Promise.resolve();
    return new Promise<void>((resolve) => {
      for (const t of this.timers.values()) clearTimeout(t);
      this.timers.clear();
      this.watcher?.close();
      this.watcher = null;
      this.running = false;
      resolve();
    });
  }

  isRunning(): boolean {
    return this.running;
  }

  /** 同步读取并触发 onUpdate（公开用于测试或初次全量加载） */
  async loadAll(onUpdate: (name: string, content: string) => void): Promise<void> {
    const files = await fs.promises.readdir(this.options.dir);
    for (const f of files) {
      const fullPath = path.join(this.options.dir, f);
      const stat = await fs.promises.stat(fullPath);
      if (!stat.isFile()) continue;
      const content = await fs.promises.readFile(fullPath, 'utf-8');
      const name = this.options.filenameToName(f);
      onUpdate(name, content);
    }
  }

  private async readAndUpdate(fullPath: string, filename: string): Promise<void> {
    try {
      const content = await fs.promises.readFile(fullPath, 'utf-8');
      const name = this.options.filenameToPath ? this.options.filenameToPath(filename) : this.options.filenameToName(filename);
      if (this.onUpdate) {
        this.onUpdate(name, content);
      }
    } catch (err) {
      if (this.onError) this.onError(err as Error);
    }
  }

  /** 设置回调（必须在 start 之前） */
  onUpdate: ((name: string, content: string) => void) | null = null;
  onError: ((err: Error) => void) | null = null;

  // Allow filenameToPath for tests if needed
  filenameToPath?: (filename: string) => string;
}

// ===== HTTP 轮询源 =====

export interface PollingSourceOptions {
  /** 拉取 URL（应返回 JSON：[{name, content}, ...] 或 {entries: [...]}) */
  url: string;
  /** 轮询间隔（毫秒） */
  intervalMs: number;
  /** 自定义 fetch（用于注入测试或自定义 header） */
  fetchImpl?: typeof fetch;
}

/**
 * PollingSource — 定时从远程端点拉取 prompt
 * 端点应返回 JSON，格式支持：
 *   - 顶层数组：[{ name, content }]
 *   - 顶层对象：{ entries: [{ name, content }] }
 */
export class PollingSource implements HotUpdateSource {
  private timer: NodeJS.Timeout | null = null;
  private running = false;
  private options: Required<PollingSourceOptions>;

  constructor(options: PollingSourceOptions) {
    this.options = {
      url: options.url,
      intervalMs: options.intervalMs,
      fetchImpl: options.fetchImpl ?? globalThis.fetch,
    };
  }

  async start(): Promise<void> {
    if (this.running) return;
    this.running = true;
    await this.poll(); // 立即拉取一次
    this.timer = setInterval(() => this.poll(), this.options.intervalMs);
  }

  async stop(): Promise<void> {
    if (!this.running) return;
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
    this.running = false;
  }

  isRunning(): boolean {
    return this.running;
  }

  private async poll(): Promise<void> {
    try {
      const resp = await this.options.fetchImpl(this.options.url);
      if (!resp.ok) {
        if (this.onError) this.onError(new Error(`HTTP ${resp.status} from ${this.options.url}`));
        return;
      }
      const data = await resp.json();
      const entries: Array<{ name: string; content: string }> = Array.isArray(data)
        ? data
        : Array.isArray(data?.entries) ? data.entries : [];
      if (this.onUpdate) this.onUpdate(entries);
    } catch (err) {
      if (this.onError) this.onError(err as Error);
    }
  }

  onUpdate: ((entries: Array<{ name: string; content: string }>) => void) | null = null;
  onError: ((err: Error) => void) | null = null;
}

// ===== HotUpdateManager — 整合 Source + Registry =====

export class HotUpdateManager {
  private registry: VersionedPromptRegistry;
  private listeners = new Set<HotUpdateListener>();
  private sources = new Map<string, HotUpdateSource>();

  constructor(registry: VersionedPromptRegistry) {
    this.registry = registry;
  }

  /** 订阅事件 */
  subscribe(listener: HotUpdateListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  /**
   * 手动更新（无 source 时使用，或用于立即推送）。
   * 如果 prompt 不存在则 register；存在则 addVersion。
   */
  update(name: string, content: string, meta?: { author?: string; tags?: string[] }): PromptVersion {
    let v: PromptVersion;
    if (!this.registry.listNames().includes(name)) {
      v = this.registry.register(name, content, meta);
      this.emit({ type: 'version_added', name, version: v });
    } else {
      v = this.registry.addVersion(name, content, meta);
      this.emit({ type: 'version_added', name, version: v });
      this.emit({ type: 'version_activated', name, version: v });
    }
    return v;
  }

  /**
   * 激活指定版本（不影响其他版本）。
   */
  activate(name: string, version: number): PromptVersion {
    const v = this.registry.activate(name, version);
    this.emit({ type: 'version_activated', name, version: v });
    return v;
  }

  /** 添加并启动一个 source */
  async attachSource(id: string, source: HotUpdateSource): Promise<void> {
    if (this.sources.has(id)) {
      throw new Error(`HotUpdateManager: source "${id}" already attached`);
    }
    this.sources.set(id, source);
    if ('onUpdate' in source && source.onUpdate !== null) {
      const prev = source.onUpdate;
      source.onUpdate = (...args: unknown[]) => {
        (prev as (...a: unknown[]) => void)(...args);
        this.handleSourceUpdate(source, args);
      };
    }
    if ('onError' in source && source.onError !== null) {
      const prevErr = source.onError;
      source.onError = (err: Error) => {
        (prevErr as (e: Error) => void)(err);
        this.emit({ type: 'error', error: err });
      };
    }
    await source.start();
  }

  /** 停止并移除一个 source */
  async detachSource(id: string): Promise<void> {
    const s = this.sources.get(id);
    if (!s) return;
    await s.stop();
    this.sources.delete(id);
    this.emit({ type: 'source_stopped', reason: id });
  }

  /** 停止所有 source */
  async stopAll(): Promise<void> {
    for (const [id, s] of this.sources) {
      await s.stop();
      this.emit({ type: 'source_stopped', reason: id });
    }
    this.sources.clear();
  }

  /** 获取注册的 registry */
  getRegistry(): VersionedPromptRegistry {
    return this.registry;
  }

  private handleSourceUpdate(source: HotUpdateSource, args: unknown[]): void {
    if (source instanceof FileWatcherSource && typeof args[0] === 'string' && typeof args[1] === 'string') {
      const name = args[0];
      const content = args[1];
      this.update(name, content, { tags: ['hot-update'] });
    } else if (source instanceof PollingSource && Array.isArray(args[0])) {
      const entries = args[0] as Array<{ name: string; content: string }>;
      for (const e of entries) {
        this.update(e.name, e.content, { tags: ['polling'] });
      }
    }
  }

  private emit(event: HotUpdateEvent): void {
    for (const l of this.listeners) {
      try {
        l(event);
      } catch (err) {
        // listener 抛错不阻断其他 listener
        // eslint-disable-next-line no-console
        console.error('HotUpdateManager listener error:', err);
      }
    }
  }
}