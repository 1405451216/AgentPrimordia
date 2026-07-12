/**
 * EnhancedWorkerPool unit tests
 */
import { describe, it, expect, afterEach } from 'vitest';
import { EnhancedWorkerPool, BackPressureError } from '../../src/agent/worker-pool-enhanced.js';

describe('EnhancedWorkerPool', () => {
  let pool: EnhancedWorkerPool;

  afterEach(async () => {
    if (pool) await pool.stop();
  });

  it('should execute submitted tasks', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 2, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 10, priorityLevels: 1 });
    const result = await pool.submit(async () => 'hello');
    expect(result).toBe('hello');
  });

  it('should execute multiple tasks', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 2, maxWorkers: 4, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 1 });
    const results = await Promise.all([
      pool.submit(async () => 1),
      pool.submit(async () => 2),
      pool.submit(async () => 3),
    ]);
    expect(results).toEqual([1, 2, 3]);
  });

  it('should respect priority ordering', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 1, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 3 });
    const executionOrder: number[] = [];
    const p1 = pool.submit(async () => { executionOrder.push(1); return 1; }, 0);
    const p3 = pool.submit(async () => { executionOrder.push(3); return 3; }, 2);
    const p2 = pool.submit(async () => { executionOrder.push(2); return 2; }, 1);
    await Promise.all([p1, p2, p3]);
    expect(executionOrder).toEqual([3, 2, 1]);
  });

  it('should throw BackPressureError when queue is full', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 1, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 2, priorityLevels: 1 });
    await pool.start();

    // Submit long task that occupies the only worker
    const longTask = pool.submit(async () => {
      await new Promise((r) => setTimeout(r, 200));
      return 'done';
    });

    // Worker busy -> queue fills (2 slots)
    const promiseA = pool.submit(async () => 'a');
    const promiseB = pool.submit(async () => 'b');

    // Queue full -> reject
    await expect(pool.submit(async () => 'overflow')).rejects.toThrow(BackPressureError);

    await Promise.allSettled([longTask, promiseA, promiseB]);
    expect(pool.stats().backpressuredCount).toBe(1);
  });

  it('should auto-expand when queue utilization > 80%', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 4, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 10, priorityLevels: 1 });
    await pool.start();
    expect(pool.stats().totalWorkers).toBe(1);

    // Keep worker busy with a long task
    const longTask = pool.submit(async () => {
      await new Promise((r) => setTimeout(r, 500));
      return 'done';
    });

    // Submit slow tasks that actually queue up (not instant)
    const futures: Promise<unknown>[] = [];
    for (let i = 0; i < 9; i++) {
      futures.push(pool.submit(async () => {
        await new Promise((r) => setTimeout(r, 50));
        return i;
      }));
    }

    // Expansion happens synchronously within dispatch after each submit
    expect(pool.stats().totalWorkers).toBeGreaterThan(1);

    await longTask;
    await Promise.allSettled(futures);
  });

  it('should report accurate stats', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 2, maxWorkers: 4, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 1 });
    await pool.start();
    const stats1 = pool.stats();
    expect(stats1.totalWorkers).toBe(2);
    expect(stats1.activeWorkers).toBe(0);
    await pool.submit(async () => 'test');
    const stats2 = pool.stats();
    expect(stats2.completedTasks).toBe(1);
  });

  it('should cull idle workers after idleTimeoutMs', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 4, idleTimeoutMs: 100, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 1 });
    await pool.start();
    expect(pool.stats().totalWorkers).toBe(1);
    pool.resize(3);
    expect(pool.stats().totalWorkers).toBe(3);
    await new Promise((r) => setTimeout(r, 200));
    const culled = pool.cullIdleWorkers();
    expect(culled).toBeGreaterThanOrEqual(1);
    expect(pool.stats().totalWorkers).toBeGreaterThanOrEqual(1);
  });

  it('should resize to specific count', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 10, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 1 });
    await pool.start();
    expect(pool.stats().totalWorkers).toBe(1);
    pool.resize(5);
    expect(pool.stats().totalWorkers).toBe(5);
    pool.resize(2);
    expect(pool.stats().totalWorkers).toBe(2);
  });

  it('should clamp resize to min/max bounds', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 2, maxWorkers: 5, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 1 });
    await pool.start();
    pool.resize(100);
    expect(pool.stats().totalWorkers).toBe(5);
    pool.resize(0);
    expect(pool.stats().totalWorkers).toBe(2);
  });

  it('should wait for all tasks to complete on drain', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 2, maxWorkers: 4, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 1 });
    const results = await Promise.all([
      pool.submit(async () => { await new Promise((r) => setTimeout(r, 50)); return 'a'; }),
      pool.submit(async () => { await new Promise((r) => setTimeout(r, 30)); return 'b'; }),
    ]);
    expect(results).toEqual(['a', 'b']);
    await pool.drain();
    const stats = pool.stats();
    expect(stats.queueDepth).toBe(0);
    expect(stats.activeWorkers).toBe(0);
  });

  it('should count failed tasks', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 2, idleTimeoutMs: 5000, taskTimeoutMs: 0, queueLimit: 20, priorityLevels: 1 });
    await expect(pool.submit(async () => {
      throw new Error('task error');
    })).rejects.toThrow('task error');
    const stats = pool.stats();
    expect(stats.failedTasks).toBe(1);
    expect(stats.completedTasks).toBe(0);
  });

  it('should handle task timeout', async () => {
    pool = new EnhancedWorkerPool({ minWorkers: 1, maxWorkers: 2, idleTimeoutMs: 5000, taskTimeoutMs: 50, queueLimit: 20, priorityLevels: 1 });
    await expect(pool.submit(async () => {
      await new Promise((r) => setTimeout(r, 200));
      return 'too-slow';
    })).rejects.toThrow(/timed out/);
    const stats = pool.stats();
    expect(stats.timeoutTasks).toBe(1);
  });

  it('should validate config', () => {
    expect(() => new EnhancedWorkerPool({ minWorkers: -1 })).toThrow();
    expect(() => new EnhancedWorkerPool({ minWorkers: 5, maxWorkers: 2 })).toThrow();
    expect(() => new EnhancedWorkerPool({ queueLimit: 0 })).toThrow();
    expect(() => new EnhancedWorkerPool({ priorityLevels: 0 })).toThrow();
  });
});
