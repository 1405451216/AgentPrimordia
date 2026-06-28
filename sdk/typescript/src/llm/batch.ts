// batch.ts 实现 LLM 请求批量处理器
// 将多个并发请求收集到批次中，达到 MaxBatchSize 或 FlushTimeout 后统一执行
// 与 Go 端 batch.go 对齐

import type { Provider } from './provider.js';
import type { CompletionRequest, CompletionResponse, Chunk, ModelInfo, ToolCallRequest, ToolCallResponse } from '../types.js';

// ===== 批量处理器配置 =====

/** 批量处理器配置，与 Go 端 BatchConfig 对齐 */
export interface BatchConfig {
  /** 单个批次最大请求数，达到此数量立即刷新 */
  maxBatchSize: number;
  /** 刷新超时（毫秒），即使批次未满也会在超时后执行 */
  flushTimeout: number;
}

/** 默认批量配置，与 Go 端 DefaultBatchConfig 对齐 */
export function defaultBatchConfig(): BatchConfig {
  return {
    maxBatchSize: 10,
    flushTimeout: 100,
  };
}

// ===== 批量条目 =====

/** 批次中的单个请求条目，与 Go 端 batchEntry 对齐 */
interface BatchEntry {
  req: CompletionRequest;
  resolve: (resp: CompletionResponse) => void;
  reject: (err: Error) => void;
}

// ===== 批量处理器 =====

/** 批量处理器，包装 Provider 实现请求批量执行
 *
 * 收集并发请求到批次中，达到 MaxBatchSize 或 FlushTimeout 后统一执行。
 * 与 Go 端 BatchProcessor 对齐。
 *
 * 使用方式：
 *   const bp = new BatchProcessor(provider);
 *   const resp = await bp.complete({ messages: [...] });
 */
export class BatchRequestProcessor implements Provider {
  private provider: Provider;
  private config: BatchConfig;
  private entries: BatchEntry[] = [];
  private closed: boolean = false;
  private flushTimer: ReturnType<typeof setTimeout> | null = null;
  private donePromise: Promise<void>;
  private resolveDone!: () => void;

  stream?: (req: CompletionRequest) => AsyncIterable<Chunk>;

  constructor(provider: Provider, config?: Partial<BatchConfig>) {
    this.provider = provider;
    this.config = { ...defaultBatchConfig(), ...config };
    this.stream = provider.stream?.bind(provider);

    this.donePromise = new Promise<void>((resolve) => {
      this.resolveDone = resolve;
    });

    // 启动后台刷新定时器
    this.startTimer();
  }

  /** 提交请求到批量处理器，等待执行结果 */
  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    if (this.closed) {
      throw new Error('batch processor is closed');
    }

    return new Promise<CompletionResponse>((resolve, reject) => {
      const entry: BatchEntry = { req, resolve, reject };
      this.entries.push(entry);

      if (this.entries.length >= this.config.maxBatchSize) {
        this.flush();
      }
    });
  }

  /** 批量处理器不支持工具调用的批量，直接委托给底层 Provider */
  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    return this.provider.callTools(req);
  }

  /** 返回底层 Provider 的模型信息 */
  info(): ModelInfo {
    return this.provider.info();
  }

  /** 关闭批量处理器，等待所有待处理请求完成 */
  async close(): Promise<void> {
    if (this.closed) return;
    this.closed = true;

    if (this.flushTimer) {
      clearTimeout(this.flushTimer);
      this.flushTimer = null;
    }

    // 刷新剩余请求
    this.flush();
    this.resolveDone();
  }

  /** 启动后台刷新定时器 */
  private startTimer(): void {
    this.flushTimer = setTimeout(() => {
      this.flush();
      if (!this.closed) {
        this.startTimer();
      }
    }, this.config.flushTimeout);
  }

  /** 执行一次批量刷新，将当前收集到的所有请求并发执行 */
  private flush(): void {
    if (this.entries.length === 0) return;

    const entries = this.entries;
    this.entries = [];

    // 并发执行批次中的所有请求
    for (const entry of entries) {
      this.provider
        .complete(entry.req)
        .then(entry.resolve)
        .catch(entry.reject);
    }
  }
}