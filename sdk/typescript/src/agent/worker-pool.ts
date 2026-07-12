/**
 * Worker Threads 并行执行池 — 利用 Node.js worker_threads 实现 CPU 真并行。
 *
 * 与 Go 的 goroutine（协作式调度）不同，Node.js 的 worker_threads 是真正的
 * 操作系统线程，适合 CPU 密集型任务（如向量化计算、加密、压缩）。
 *
 * 这是 TS SDK 相对 Go SDK 的性能优势场景：
 * - Go 的 goroutine 在 CPU 密集型任务上受 GOMAXPROCS 限制
 * - Node.js worker_threads 可以利用所有 CPU 核心
 *
 * 使用方式：
 *   const pool = new WorkerPool({ maxWorkers: 4 });
 *   const result = await pool.run HeavyTask(data);
 *   pool.terminate();
 */

// ===== 类型定义 =====

export interface WorkerPoolConfig {
  /** 最大 worker 数量，默认 CPU 核心数 */
  maxWorkers?: number;
  /** Worker 脚本路径（内联代码用 data: URL） */
  workerScript?: string;
  /** 任务超时（毫秒），0 表示不超时 */
  taskTimeout?: number;
}

export interface WorkerTask<I, _O = unknown> {
  id: string;
  input: I;
  transfer?: Transferable[];
  timeout?: number;
}

export interface WorkerResult<O> {
  id: string;
  output?: O;
  error?: Error;
  duration: number;
}

// ===== Worker Pool 实现 =====

/**
 * Worker 并行池。
 *
 * 注意：仅在 Node.js 18+ 环境下可用。
 * 在浏览器中使用 Web Workers API（不在本类范围内）。
 * 在 Edge Runtime 中不可用（Cloudflare Workers 不支持 worker_threads）。
 */
export class ComputeWorkerPool implements Disposable {
  private config: Required<Omit<WorkerPoolConfig, 'workerScript'>> & Pick<WorkerPoolConfig, 'workerScript'>;
  private workers: { worker: { terminate(): void }; busy: boolean; id: number }[] = [];
  private taskQueue: Array<{ task: WorkerTask<unknown, unknown>; resolve: (v: unknown) => void; reject: (e: Error) => void }> = [];
  private nextId = 0;
  private workerThreadModule: { Worker: new (filename: string, options?: Record<string, unknown>) => unknown } | null = null;
  private initialized = false;

  constructor(config?: WorkerPoolConfig) {
    this.config = {
      maxWorkers: config?.maxWorkers ?? 0, // 0 = auto-detect
      workerScript: config?.workerScript,
      taskTimeout: config?.taskTimeout ?? 0,
    };
  }

  /** 初始化 worker 池（惰性初始化，首次 run 时自动调用） */
  async init(): Promise<void> {
    if (this.initialized) return;

    // 动态加载 worker_threads 模块
    try {
      this.workerThreadModule = await import('node:worker_threads');
    } catch {
      throw new Error('worker_threads is not available in this runtime');
    }

    const { Worker } = this.workerThreadModule;
    // 使用 os.cpus() 获取 CPU 核心数（worker_threads 模块不导出 cpus）
    let cpuCount = 4;
    try {
      const os = await import('node:os');
      cpuCount = os.cpus().length;
    } catch {
      // fallback to default
    }
    const maxWorkers = this.config.maxWorkers > 0
      ? this.config.maxWorkers
      : cpuCount;

    for (let i = 0; i < maxWorkers; i++) {
      // 创建 worker（内联脚本需要 eval: true）
      const script = this.config.workerScript ?? this.getDefaultWorkerScript();
      const worker = new Worker(script, { eval: true });
      this.workers.push({ worker: worker as unknown as { terminate(): void }, busy: false, id: i });
    }

    this.initialized = true;
  }

  /** 执行单个任务 */
  async run<I, O>(input: I, opts?: { transfer?: Transferable[]; timeout?: number }): Promise<O> {
    await this.init();

    const task: WorkerTask<I, O> = {
      id: `task-${this.nextId++}`,
      input,
      transfer: opts?.transfer,
      timeout: opts?.timeout ?? this.config.taskTimeout,
    };

    return this.enqueue<I, O>(task);
  }

  /** 批量并行执行 */
  async runAll<I, O>(inputs: I[]): Promise<O[]> {
    await this.init();

    const tasks = inputs.map((input, i) => ({
      id: `batch-${i}`,
      input,
    } as WorkerTask<I, O>));

    const results = await Promise.all(tasks.map((t) => this.enqueue<I, O>(t)));
    return results;
  }

  /** map-reduce 模式 */
  async mapReduce<I, M, R>(
    inputs: I[],
    reducer: (results: M[]) => R,
  ): Promise<R> {
    const mapped = await this.runAll<I, M>(inputs);
    return reducer(mapped);
  }

  /** 终止所有 worker */
  terminate(): void {
    for (const { worker } of this.workers) {
      worker.terminate();
    }
    this.workers = [];
    this.initialized = false;
  }

  /** 显式资源清理（TS 5.2+ Explicit Resource Management） */
  [Symbol.dispose](): void {
    this.terminate();
  }

  /** 获取统计信息 */
  get stats(): { total: number; busy: number; idle: number; queued: number } {
    const busy = this.workers.filter((w) => w.busy).length;
    return {
      total: this.workers.length,
      busy,
      idle: this.workers.length - busy,
      queued: this.taskQueue.length,
    };
  }

  // ===== 内部方法 =====

  private enqueue<I, O>(task: WorkerTask<I, O>): Promise<O> {
    return new Promise((resolve, reject) => {
      this.taskQueue.push({
        task: task as unknown as WorkerTask<unknown, unknown>,
        resolve: resolve as (v: unknown) => void,
        reject,
      });
      this.dispatch();
    });
  }

  private dispatch(): void {
    const idleWorker = this.workers.find((w) => !w.busy);
    if (!idleWorker) return;

    const item = this.taskQueue.shift();
    if (!item) return;

    idleWorker.busy = true;

    const worker = idleWorker.worker as unknown as {
      on(event: string, cb: (data: unknown) => void): void;
      off(event: string, cb: (data: unknown) => void): void;
      postMessage(data: unknown, transfer?: Transferable[]): void;
    };

    // 超时处理
    let timeoutHandle: ReturnType<typeof setTimeout> | null = null;
    if (item.task.timeout && item.task.timeout > 0) {
      timeoutHandle = setTimeout(() => {
        idleWorker.busy = false;
        item.reject(new Error(`Task ${item.task.id} timed out after ${item.task.timeout}ms`));
        this.dispatch();
      }, item.task.timeout);
    }

    // 监听 worker 消息（一次性）
    const handler = (data: unknown) => {
      const result = data as WorkerResult<unknown>;
      worker.off('message', handler);
      if (timeoutHandle) clearTimeout(timeoutHandle);
      idleWorker.busy = false;

      if (result.error) {
        item.reject(result.error);
      } else {
        item.resolve(result.output);
      }
      this.dispatch();
    };

    worker.on('message', handler as (data: unknown) => void);
    worker.postMessage(item.task.input, item.task.transfer ?? []);
  }

  private getDefaultWorkerScript(): string {
    // 内联 worker 脚本 — 通用任务执行器
    return `
      const { parentPort } = require('node:worker_threads');
      parentPort.on('message', async (data) => {
        try {
          // 通用执行：直接返回数据（用户应提供自定义 worker 脚本）
          parentPort.postMessage({ output: data, error: null, duration: 0 });
        } catch (err) {
          parentPort.postMessage({ output: null, error: err.message, duration: 0 });
        }
      });
    `;
  }
}

// ===== 检查运行时是否支持 Worker Threads =====

export function isWorkerThreadsAvailable(): boolean {
  try {
    // 检查是否在 Node.js 环境且有 worker_threads
    if (typeof process !== 'undefined' && process.versions?.node) {
      const major = parseInt(process.versions.node.split('.')[0]!, 10);
      return major >= 18;
    }
  } catch {
    // ignore
  }
  return false;
}
