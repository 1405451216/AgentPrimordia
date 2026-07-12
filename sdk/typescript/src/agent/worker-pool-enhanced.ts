/**
 * Enhanced Worker Pool - dynamic scaling + priority queue + backpressure control.
 */
export interface WorkerPoolConfig {
  minWorkers: number;
  maxWorkers: number;
  idleTimeoutMs: number;
  taskTimeoutMs: number;
  queueLimit: number;
  priorityLevels: number;
}

export interface PoolStats {
  totalWorkers: number;
  activeWorkers: number;
  idleWorkers: number;
  queueDepth: number;
  completedTasks: number;
  failedTasks: number;
  timeoutTasks: number;
  backpressuredCount: number;
}

export class BackPressureError extends Error {
  constructor(public readonly queueDepth: number, public readonly queueLimit: number) {
    super(`Worker pool queue is full: ${queueDepth}/${queueLimit}`);
    this.name = 'BackPressureError';
  }
}

interface QueueItem<T> {
  task: () => Promise<T>;
  priority: number;
  resolve: (value: T) => void;
  reject: (error: Error) => void;
}

interface Worker {
  id: number;
  busy: boolean;
  lastActiveTime: number;
}

const DEFAULT_CONFIG: WorkerPoolConfig = {
  minWorkers: 2,
  maxWorkers: 8,
  idleTimeoutMs: 30000,
  taskTimeoutMs: 60000,
  queueLimit: 100,
  priorityLevels: 3,
};

export class EnhancedWorkerPool {
  private config: WorkerPoolConfig;
  private workers: Worker[] = [];
  private queue: QueueItem<unknown>[] = [];
  private nextWorkerId = 0;
  private completedTasks = 0;
  private failedTasks = 0;
  private timeoutTasks = 0;
  private backpressuredCount = 0;
  private idleTimer: ReturnType<typeof setInterval> | null = null;
  private running = false;

  constructor(config?: Partial<WorkerPoolConfig>) {
    this.config = { ...DEFAULT_CONFIG, ...config };
    this.validateConfig();
  }

  async start(): Promise<void> {
    if (this.running) return;
    this.running = true;
    for (let i = 0; i < this.config.minWorkers; i++) {
      this.addWorker();
    }
    this.idleTimer = setInterval(() => {
      this.cullIdleWorkers();
    }, Math.max(this.config.idleTimeoutMs / 2, 1000));
  }

  async submit<T>(task: () => Promise<T>, priority?: number): Promise<T> {
    if (!this.running) await this.start();
    const effectivePriority = this.normalizePriority(priority);
    if (this.queue.length >= this.config.queueLimit) {
      this.backpressuredCount++;
      throw new BackPressureError(this.queue.length, this.config.queueLimit);
    }
    return new Promise<T>((resolve, reject) => {
      this.enqueue({ task, priority: effectivePriority, resolve, reject } as QueueItem<unknown>);
      this.dispatch();
    });
  }

  resize(count: number): void {
    const target = Math.max(this.config.minWorkers, Math.min(this.config.maxWorkers, count));
    const current = this.workers.length;
    if (target > current) {
      for (let i = current; i < target; i++) this.addWorker();
    } else if (target < current) {
      const toRemove = current - target;
      const idleWorkers = this.workers.filter((w) => !w.busy);
      for (let i = 0; i < Math.min(toRemove, idleWorkers.length); i++) {
        this.removeWorker(idleWorkers[i]!.id);
      }
    }
  }

  stats(): PoolStats {
    const active = this.workers.filter((w) => w.busy).length;
    return {
      totalWorkers: this.workers.length,
      activeWorkers: active,
      idleWorkers: this.workers.length - active,
      queueDepth: this.queue.length,
      completedTasks: this.completedTasks,
      failedTasks: this.failedTasks,
      timeoutTasks: this.timeoutTasks,
      backpressuredCount: this.backpressuredCount,
    };
  }

  async drain(): Promise<void> {
    while (this.queue.length > 0 || this.workers.some((w) => w.busy)) {
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
  }

  cullIdleWorkers(): number {
    const now = Date.now();
    let culled = 0;
    const minToKeep = this.config.minWorkers;
    const candidates = this.workers.filter(
      (w) => !w.busy && (now - w.lastActiveTime) > this.config.idleTimeoutMs,
    );
    for (const worker of candidates) {
      if (this.workers.length - culled <= minToKeep) break;
      this.removeWorker(worker.id);
      culled++;
    }
    return culled;
  }

  async stop(): Promise<void> {
    this.running = false;
    if (this.idleTimer) { clearInterval(this.idleTimer); this.idleTimer = null; }
    await this.drain();
    this.workers = [];
  }

  private validateConfig(): void {
    if (this.config.minWorkers < 0) throw new Error('minWorkers must be >= 0');
    if (this.config.maxWorkers < this.config.minWorkers) throw new Error('maxWorkers must be >= minWorkers');
    if (this.config.queueLimit < 1) throw new Error('queueLimit must be >= 1');
    if (this.config.priorityLevels < 1) throw new Error('priorityLevels must be >= 1');
  }

  private normalizePriority(priority?: number): number {
    if (priority === undefined) return 0;
    return Math.max(0, Math.min(this.config.priorityLevels - 1, priority));
  }

  private addWorker(): Worker {
    const worker: Worker = { id: this.nextWorkerId++, busy: false, lastActiveTime: Date.now() };
    this.workers.push(worker);
    return worker;
  }

  private removeWorker(id: number): void {
    const idx = this.workers.findIndex((w) => w.id === id);
    if (idx !== -1) this.workers.splice(idx, 1);
  }

  private enqueue(item: QueueItem<unknown>): void {
    let insertIdx = this.queue.length;
    for (let i = 0; i < this.queue.length; i++) {
      if (this.queue[i]!.priority < item.priority) { insertIdx = i; break; }
    }
    this.queue.splice(insertIdx, 0, item);
  }

  private dispatch(): void {
    while (this.queue.length > 0) {
      const idleWorker = this.workers.find((w) => !w.busy);
      if (!idleWorker) {
        if (this.workers.length < this.config.maxWorkers) { this.addWorker(); continue; }
        break;
      }
      const item = this.queue.shift()!;
      this.executeOnWorker(idleWorker, item);
    }
    const utilization = this.queue.length / this.config.queueLimit;
    if (utilization > 0.8 && this.workers.length < this.config.maxWorkers) {
      const expandBy = Math.min(Math.ceil((this.config.maxWorkers - this.workers.length) / 2), this.config.maxWorkers - this.workers.length);
      for (let i = 0; i < expandBy; i++) this.addWorker();
      this.dispatch();
    }
    if (utilization < 0.2 && this.workers.length > this.config.minWorkers) {
      const idleWorkers = this.workers.filter((w) => !w.busy);
      const excess = this.workers.length - this.config.minWorkers;
      const toRemove = Math.min(excess, idleWorkers.length);
      for (let i = 0; i < toRemove; i++) this.removeWorker(idleWorkers[i]!.id);
    }
  }

  private async executeOnWorker(worker: Worker, item: QueueItem<unknown>): Promise<void> {
    worker.busy = true;
    worker.lastActiveTime = Date.now();
    try {
      let result: unknown;
      if (this.config.taskTimeoutMs > 0) {
        result = await this.executeWithTimeout(item.task, this.config.taskTimeoutMs);
      } else {
        result = await item.task();
      }
      this.completedTasks++;
      item.resolve(result);
    } catch (err) {
      if (err instanceof Error && err.message.includes('timed out')) {
        this.timeoutTasks++;
      } else {
        this.failedTasks++;
      }
      item.reject(err instanceof Error ? err : new Error(String(err)));
    } finally {
      worker.busy = false;
      worker.lastActiveTime = Date.now();
      this.dispatch();
    }
  }

  private executeWithTimeout<T>(task: () => Promise<T>, timeoutMs: number): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`Task timed out after ${timeoutMs}ms`)), timeoutMs);
      task().then(
        (result) => { clearTimeout(timer); resolve(result); },
        (err) => { clearTimeout(timer); reject(err); },
      );
    });
  }
}
