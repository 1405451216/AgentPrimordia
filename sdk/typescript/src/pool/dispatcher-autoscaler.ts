// ===== AutoScaler — Automatic concurrency scaling =====

export interface AutoScalerConfig {
  minConcurrency: number;
  maxConcurrency: number;
  scaleUpThreshold: number;   // utilization > this → scale up
  scaleDownThreshold: number;  // utilization < this → scale down
  coolDownMs: number;          // prevent frequent scaling
  checkIntervalMs: number;
}

export class AutoScaler {
  private config: AutoScalerConfig;
  private lastScaleTime: number = 0;
  private lastScaleUp: boolean = false;
  private currentConcurrency: number;

  constructor(config?: Partial<AutoScalerConfig>) {
    this.config = {
      minConcurrency: config?.minConcurrency ?? 1,
      maxConcurrency: config?.maxConcurrency ?? 100,
      scaleUpThreshold: config?.scaleUpThreshold ?? 0.8,
      scaleDownThreshold: config?.scaleDownThreshold ?? 0.2,
      coolDownMs: config?.coolDownMs ?? 5000,
      checkIntervalMs: config?.checkIntervalMs ?? 10000,
    };
    this.currentConcurrency = this.config.minConcurrency;
  }

  /** Calculate new concurrency based on current load. */
  calculate(running: number, queued: number, current: number): number {
    if (current <= 0) current = this.config.minConcurrency;

    const totalDemand = running + queued;
    const utilization = totalDemand / current;

    const now = Date.now();
    if (now - this.lastScaleTime < this.config.coolDownMs) {
      return current; // In cooldown
    }

    let newConcurrency = current;

    if (utilization >= this.config.scaleUpThreshold) {
      // Scale up
      const scaleFactor = Math.min(2, utilization);
      newConcurrency = Math.min(
        this.config.maxConcurrency,
        Math.ceil(current * scaleFactor)
      );
      this.lastScaleUp = true;
    } else if (utilization <= this.config.scaleDownThreshold && current > this.config.minConcurrency) {
      // Scale down
      newConcurrency = Math.max(
        this.config.minConcurrency,
        Math.floor(current * 0.5)
      );
      this.lastScaleUp = false;
    }

    if (newConcurrency !== current) {
      this.lastScaleTime = now;
      this.currentConcurrency = newConcurrency;
    }

    return newConcurrency;
  }

  get concurrency(): number { return this.currentConcurrency; }

  /** Start periodic auto-scaling. */
  startAutoScale(
    getLoad: () => { running: number; queued: number },
    onScale: (newConcurrency: number) => void
  ): () => void {
    const timer = setInterval(() => {
      const { running, queued } = getLoad();
      const newConc = this.calculate(running, queued, this.currentConcurrency);
      if (newConc !== this.currentConcurrency) {
        onScale(newConc);
      }
    }, this.config.checkIntervalMs);

    return () => clearInterval(timer);
  }
}

// ===== Dispatcher — Task dispatch with priority and routing =====

export interface DispatchTask {
  id: string;
  priority: number;
  data: unknown;
  assignedTo?: string;
  createdAt: number;
}

export interface DispatcherConfig {
  maxQueueSize: number;
  strategy: 'priority' | 'round_robin' | 'least_loaded' | 'random';
}

export class Dispatcher {
  private config: DispatcherConfig;
  private queue: DispatchTask[] = [];
  private workers: Map<string, { load: number; active: boolean }> = new Map();
  private rrIndex = 0;

  constructor(config?: Partial<DispatcherConfig>) {
    this.config = {
      maxQueueSize: config?.maxQueueSize ?? 1000,
      strategy: config?.strategy ?? 'priority',
    };
  }

  registerWorker(id: string): void {
    this.workers.set(id, { load: 0, active: true });
  }

  unregisterWorker(id: string): void {
    this.workers.delete(id);
  }

  setWorkerLoad(id: string, load: number): void {
    const worker = this.workers.get(id);
    if (worker) worker.load = load;
  }

  submit(task: DispatchTask): boolean {
    if (this.queue.length >= this.config.maxQueueSize) return false;
    task.createdAt = Date.now();
    this.queue.push(task);
    // Sort by priority (higher first)
    if (this.config.strategy === 'priority') {
      this.queue.sort((a, b) => b.priority - a.priority);
    }
    return true;
  }

  dispatch(): DispatchTask | null {
    if (this.queue.length === 0) return null;

    const availableWorkers = Array.from(this.workers.entries())
      .filter(([_, w]) => w.active)
      .map(([id, w]) => ({ id, load: w.load }));

    if (availableWorkers.length === 0) return null;

    let workerId: string;

    switch (this.config.strategy) {
      case 'priority':
        // Highest priority task → least loaded worker
        availableWorkers.sort((a, b) => a.load - b.load);
        workerId = availableWorkers[0]!.id;
        break;

      case 'round_robin':
        workerId = availableWorkers[this.rrIndex % availableWorkers.length]!.id;
        this.rrIndex++;
        break;

      case 'least_loaded':
        availableWorkers.sort((a, b) => a.load - b.load);
        workerId = availableWorkers[0]!.id;
        break;

      case 'random':
        workerId = availableWorkers[Math.floor(Math.random() * availableWorkers.length)]!.id;
        break;

      default:
        workerId = availableWorkers[0]!.id;
    }

    const task = this.queue.shift()!;
    task.assignedTo = workerId;

    const worker = this.workers.get(workerId);
    if (worker) worker.load++;

    return task;
  }

  completeTask(task: DispatchTask): void {
    if (task.assignedTo) {
      const worker = this.workers.get(task.assignedTo);
      if (worker) worker.load = Math.max(0, worker.load - 1);
    }
  }

  getQueueSize(): number { return this.queue.length; }
  getWorkerCount(): number { return this.workers.size; }

  getStats(): { queueSize: number; workers: number; avgLoad: number } {
    const loads = Array.from(this.workers.values()).map(w => w.load);
    const avgLoad = loads.length > 0 ? loads.reduce((a, b) => a + b, 0) / loads.length : 0;
    return { queueSize: this.queue.length, workers: this.workers.size, avgLoad };
  }
}

// ===== File Lock =====

import * as fs from 'node:fs';
import * as path from 'node:path';

export class FileLock {
  private lockDir: string;

  constructor(lockDir?: string) {
    this.lockDir = lockDir ?? path.join(process.cwd(), '.locks');
  }

  async acquire(key: string, timeoutMs: number = 30000): Promise<boolean> {
    fs.mkdirSync(this.lockDir, { recursive: true });
    const lockFile = path.join(this.lockDir, `${key}.lock`);
    const deadline = Date.now() + timeoutMs;

    while (Date.now() < deadline) {
      try {
        // Try to create lock file exclusively
        const fd = fs.openSync(lockFile, 'wx');
        fs.writeSync(fd, JSON.stringify({ pid: process.pid, time: Date.now() }));
        fs.closeSync(fd);
        return true;
      } catch (err) {
        // Check if lock is stale
        try {
          const stat = fs.statSync(lockFile);
          if (Date.now() - stat.mtimeMs > 60000) {
            // Stale lock, remove and retry
            fs.unlinkSync(lockFile);
            continue;
          }
        } catch {
          // File was removed by someone else
        }
        await new Promise(r => setTimeout(r, 100));
      }
    }
    return false;
  }

  release(key: string): void {
    const lockFile = path.join(this.lockDir, `${key}.lock`);
    try { fs.unlinkSync(lockFile); } catch {}
  }

  async withLock<T>(key: string, fn: () => Promise<T>, timeoutMs?: number): Promise<T | null> {
    const acquired = await this.acquire(key, timeoutMs);
    if (!acquired) return null;
    try {
      return await fn();
    } finally {
      this.release(key);
    }
  }
}

// ===== Concurrency Pool =====

export class ConcurrencyPool<T> {
  private maxConcurrent: number;
  private active: number = 0;
  private waiting: (() => void)[] = [];

  constructor(maxConcurrent: number) {
    this.maxConcurrent = maxConcurrent;
  }

  async acquire(): Promise<void> {
    if (this.active < this.maxConcurrent) {
      this.active++;
      return;
    }
    // Wait until a slot is handed off by release()
    await new Promise<void>(resolve => this.waiting.push(resolve));
    // Slot is already reserved by release(); no increment needed
  }

  release(): void {
    const next = this.waiting.shift();
    if (next) {
      // Hand off the slot directly to the next waiter
      next();
    } else {
      // No waiter — actually free the slot
      this.active--;
    }
  }

  async run<R>(fn: () => Promise<R>): Promise<R> {
    await this.acquire();
    try {
      return await fn();
    } finally {
      this.release();
    }
  }

  async map<R>(items: T[], fn: (item: T) => Promise<R>): Promise<R[]> {
    return Promise.all(items.map(item => this.run(() => fn(item))));
  }

  get activeCount(): number { return this.active; }
  get waitingCount(): number { return this.waiting.length; }
}
