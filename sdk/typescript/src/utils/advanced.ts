import * as fs from 'node:fs';
import type { RateLimiter, BatchProcessor } from '../llm/cache-structured.js';
import type { BatchRequest, BatchResult } from '../llm/cache-structured.js';

// ===== Config Hot Reload =====

export interface ConfigWatcherOptions {
  intervalMs?: number;
  onUpdate?: (config: Record<string, unknown>) => void;
}

export class ConfigWatcher {
  private filePath: string;
  private intervalMs: number;
  private interval?: NodeJS.Timeout;
  private lastModified: number = 0;
  private lastContent: string = '';
  private onUpdate?: (config: Record<string, unknown>) => void;
  private currentConfig: Record<string, unknown> = {};

  constructor(filePath: string, opts?: ConfigWatcherOptions) {
    this.filePath = filePath;
    this.intervalMs = opts?.intervalMs ?? 5000;
    this.onUpdate = opts?.onUpdate;
  }

  async start(): Promise<void> {
    // Initial load
    await this.load();

    // Start watching
    this.interval = setInterval(() => {
      this.checkAndReload().catch((err) => {
        console.error('Config checkAndReload failed:', err);
      });
    }, this.intervalMs);
  }

  stop(): void {
    if (this.interval) {
      clearInterval(this.interval);
      this.interval = undefined;
    }
  }

  getConfig(): Record<string, unknown> {
    return { ...this.currentConfig };
  }

  private async load(): Promise<void> {
    try {
      const stat = fs.statSync(this.filePath);
      this.lastModified = stat.mtimeMs;
      const content = fs.readFileSync(this.filePath, 'utf-8');
      this.lastContent = content;
      this.currentConfig = JSON.parse(content);
    } catch (err) {
      // If file doesn't exist or is invalid, start with empty config
      this.currentConfig = {};
    }
  }

  private async checkAndReload(): Promise<void> {
    try {
      const stat = fs.statSync(this.filePath);
      if (stat.mtimeMs <= this.lastModified) return;

      const content = fs.readFileSync(this.filePath, 'utf-8');
      if (content === this.lastContent) return;

      this.lastModified = stat.mtimeMs;
      this.lastContent = content;

      try {
        const newConfig = JSON.parse(content);
        const _oldConfig = this.currentConfig;
        this.currentConfig = newConfig;

        if (this.onUpdate) {
          this.onUpdate(newConfig);
        }
      } catch (err) {
        // Invalid JSON, keep old config
        console.error('Config reload failed: invalid JSON');
      }
    } catch {
      // File may have been deleted
    }
  }
}

// ===== Zero-Copy Optimization (Buffer Pool) =====

export class BufferPool {
  private pool: Map<number, Buffer[]> = new Map();
  private maxPoolSize: number;

  constructor(maxPoolSize: number = 100) {
    this.maxPoolSize = maxPoolSize;
  }

  acquire(size: number): Buffer {
    const key = this.roundUp(size);
    const pool = this.pool.get(key);
    if (pool && pool.length > 0) {
      return pool.pop()!.subarray(0, size);
    }
    return Buffer.allocUnsafe(key);
  }

  release(buf: Buffer): void {
    const key = buf.length;
    let pool = this.pool.get(key);
    if (!pool) {
      pool = [];
      this.pool.set(key, pool);
    }
    if (pool.length < this.maxPoolSize) {
      pool.push(buf);
    }
  }

  private roundUp(size: number): number {
    const power = Math.ceil(Math.log2(Math.max(1, size)));
    return Math.pow(2, power);
  }
}

// ===== Structured Logger =====

export type LogLevel = 'debug' | 'info' | 'warn' | 'error' | 'fatal';

export interface LogEntry {
  level: LogLevel;
  message: string;
  timestamp: string;
  fields?: Record<string, unknown>;
  traceID?: string;
  requestID?: string;
}

export class StructuredLogger {
  private level: LogLevel;
  private entries: LogEntry[] = [];
  private maxEntries: number;
  private outputs: ((entry: LogEntry) => void)[] = [];

  constructor(level: LogLevel = 'info', maxEntries: number = 10_000) {
    this.level = level;
    this.maxEntries = maxEntries;
  }

  addOutput(output: (entry: LogEntry) => void): void {
    this.outputs.push(output);
  }

  log(level: LogLevel, message: string, fields?: Record<string, unknown>): void {
    if (this.levelRank(level) < this.levelRank(this.level)) return;

    const entry: LogEntry = {
      level,
      message,
      timestamp: new Date().toISOString(),
      fields,
    };

    this.entries.push(entry);
    if (this.entries.length > this.maxEntries) {
      this.entries.shift();
    }

    for (const output of this.outputs) {
      output(entry);
    }
  }

  debug(message: string, fields?: Record<string, unknown>): void { this.log('debug', message, fields); }
  info(message: string, fields?: Record<string, unknown>): void { this.log('info', message, fields); }
  warn(message: string, fields?: Record<string, unknown>): void { this.log('warn', message, fields); }
  error(message: string, fields?: Record<string, unknown>): void { this.log('error', message, fields); }
  fatal(message: string, fields?: Record<string, unknown>): void { this.log('fatal', message, fields); }

  getEntries(filter?: { level?: LogLevel; since?: Date; contains?: string }): LogEntry[] {
    let result = [...this.entries];
    if (filter?.level) result = result.filter((e) => this.levelRank(e.level) >= this.levelRank(filter.level!));
    if (filter?.since) result = result.filter((e) => e.timestamp >= filter.since!.toISOString());
    if (filter?.contains) result = result.filter((e) => e.message.includes(filter.contains!));
    return result;
  }

  setLevel(level: LogLevel): void {
    this.level = level;
  }

  clear(): void {
    this.entries = [];
  }

  private levelRank(level: LogLevel): number {
    const ranks: Record<LogLevel, number> = { debug: 0, info: 1, warn: 2, error: 3, fatal: 4 };
    return ranks[level];
  }
}

// ===== Default logger instance =====
export const defaultLogger = new StructuredLogger('info');

// ===== Convenience exports for already-defined types =====
export type { RateLimiter, BatchProcessor, BatchRequest, BatchResult };

// ===== Async Memory Writer (non-blocking) =====

export interface AsyncMemoryWriterConfig {
  maxQueueSize?: number;
  flushIntervalMs?: number;
  maxRetries?: number;
}

export class AsyncMemoryWriter {
  private queue: { id: string; data: unknown; retries: number }[] = [];
  private processing = false;
  private config: Required<AsyncMemoryWriterConfig>;
  private writeFn: (id: string, data: unknown) => Promise<void>;
  private flushTimer?: NodeJS.Timeout;

  constructor(writeFn: (id: string, data: unknown) => Promise<void>, config?: AsyncMemoryWriterConfig) {
    this.writeFn = writeFn;
    this.config = {
      maxQueueSize: config?.maxQueueSize ?? 1000,
      flushIntervalMs: config?.flushIntervalMs ?? 1000,
      maxRetries: config?.maxRetries ?? 3,
    };
  }

  /** Queue a write operation (non-blocking). */
  enqueue(id: string, data: unknown): boolean {
    if (this.queue.length >= this.config.maxQueueSize) {
      return false; // Queue full
    }
    this.queue.push({ id, data, retries: 0 });
    this.scheduleFlush();
    return true;
  }

  /** Flush all pending writes. */
  async flush(): Promise<void> {
    if (this.processing) return;
    this.processing = true;

    while (this.queue.length > 0) {
      const item = this.queue.shift()!;
      try {
        await this.writeFn(item.id, item.data);
      } catch (err) {
        item.retries++;
        if (item.retries < this.config.maxRetries) {
          this.queue.push(item); // Re-queue for retry
        }
      }
    }

    this.processing = false;
  }

  /** Get queue size. */
  get queueSize(): number {
    return this.queue.length;
  }

  stop(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
      this.flushTimer = undefined;
    }
  }

  private scheduleFlush(): void {
    if (this.flushTimer) return;
    this.flushTimer = setInterval(() => {
      this.flush().catch((err) => {
        console.error('AsyncMemoryWriter flush failed:', err);
      });
    }, this.config.flushIntervalMs);
  }
}

// ===== Event Bus (internal pub/sub) =====

export type EventHandler<T = unknown> = (data: T) => void | Promise<void>;

export interface EventBusSubscription {
  unsubscribe: () => void;
}

export class EventBus {
  private handlers: Map<string, Set<EventHandler>> = new Map();

  on<T = unknown>(event: string, handler: EventHandler<T>): EventBusSubscription {
    if (!this.handlers.has(event)) {
      this.handlers.set(event, new Set());
    }
    this.handlers.get(event)!.add(handler as EventHandler);

    return {
      unsubscribe: () => {
        this.handlers.get(event)?.delete(handler as EventHandler);
      },
    };
  }

  once<T = unknown>(event: string, handler: EventHandler<T>): EventBusSubscription {
    const sub = this.on<T>(event, (data) => {
      sub.unsubscribe();
      handler(data);
    });
    return sub;
  }

  async emit<T = unknown>(event: string, data?: T): Promise<void> {
    const handlers = this.handlers.get(event);
    if (!handlers || handlers.size === 0) return;

    const promises: Promise<void>[] = [];
    for (const handler of handlers) {
      try {
        const result = handler(data);
        if (result instanceof Promise) {
          promises.push(result);
        }
      } catch {}
    }

    await Promise.allSettled(promises);
  }

  off(event: string): void {
    this.handlers.delete(event);
  }

  clear(): void {
    this.handlers.clear();
  }

  listenerCount(event: string): number {
    return this.handlers.get(event)?.size ?? 0;
  }
}
