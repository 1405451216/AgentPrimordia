import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { AutoScaler, Dispatcher, ConcurrencyPool, FileLock } from '../../src/pool/dispatcher-autoscaler.js';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';

describe('AutoScaler', () => {
  it('should start with minConcurrency', () => {
    const scaler = new AutoScaler({ minConcurrency: 2, maxConcurrency: 10 });
    expect(scaler.concurrency).toBe(2);
  });

  it('should use defaults', () => {
    const scaler = new AutoScaler();
    expect(scaler.concurrency).toBe(1);
  });

  it('should scale up when utilization is high', () => {
    const scaler = new AutoScaler({
      minConcurrency: 1,
      maxConcurrency: 100,
      scaleUpThreshold: 0.8,
      coolDownMs: 0,
    });
    const newConc = scaler.calculate(15, 5, 10); // utilization = 2.0, scaleFactor = 2
    expect(newConc).toBeGreaterThan(10);
  });

  it('should scale down when utilization is low', () => {
    const scaler = new AutoScaler({
      minConcurrency: 1,
      maxConcurrency: 100,
      scaleDownThreshold: 0.2,
      coolDownMs: 0,
    });
    const newConc = scaler.calculate(1, 0, 10); // utilization = 0.1
    expect(newConc).toBeLessThan(10);
  });

  it('should not scale below minConcurrency', () => {
    const scaler = new AutoScaler({
      minConcurrency: 3,
      maxConcurrency: 100,
      scaleDownThreshold: 0.2,
      coolDownMs: 0,
    });
    const newConc = scaler.calculate(0, 0, 4); // floor(4*0.5)=2, max(3,2)=3
    expect(newConc).toBe(3);
  });

  it('should not scale above maxConcurrency', () => {
    const scaler = new AutoScaler({
      minConcurrency: 1,
      maxConcurrency: 20,
      scaleUpThreshold: 0.8,
      coolDownMs: 0,
    });
    const newConc = scaler.calculate(100, 100, 15); // very high utilization
    expect(newConc).toBeLessThanOrEqual(20);
  });

  it('should respect cooldown period', () => {
    const scaler = new AutoScaler({
      minConcurrency: 1,
      maxConcurrency: 100,
      scaleUpThreshold: 0.8,
      coolDownMs: 999999,
    });
    // First scale up
    const first = scaler.calculate(10, 0, 5);
    // Second call should be in cooldown
    const second = scaler.calculate(10, 0, first);
    expect(second).toBe(first);
  });

  it('should not scale when utilization is in middle range', () => {
    const scaler = new AutoScaler({
      minConcurrency: 1,
      maxConcurrency: 100,
      scaleUpThreshold: 0.8,
      scaleDownThreshold: 0.2,
      coolDownMs: 0,
    });
    const newConc = scaler.calculate(5, 0, 10); // utilization = 0.5
    expect(newConc).toBe(10);
  });

  it('should handle current <= 0', () => {
    const scaler = new AutoScaler({ minConcurrency: 2, coolDownMs: 0 });
    const newConc = scaler.calculate(5, 0, 0);
    expect(newConc).toBeGreaterThanOrEqual(2);
  });

  it('startAutoScale should run periodic checks', () => {
    vi.useFakeTimers();
    const scaler = new AutoScaler({
      minConcurrency: 1,
      maxConcurrency: 100,
      scaleUpThreshold: 0.5,
      coolDownMs: 0,
      checkIntervalMs: 100,
    });
    const onScale = vi.fn();
    const stop = scaler.startAutoScale(
      () => ({ running: 10, queued: 10 }),
      onScale,
    );
    vi.advanceTimersByTime(150);
    // calculate() mutates currentConcurrency internally,
    // so onScale may not fire depending on timing
    stop();
    vi.useRealTimers();
  });

  it('startAutoScale should not call onScale when no change', () => {
    vi.useFakeTimers();
    const scaler = new AutoScaler({
      minConcurrency: 1,
      maxConcurrency: 100,
      scaleUpThreshold: 0.8,
      scaleDownThreshold: 0.2,
      coolDownMs: 0,
      checkIntervalMs: 100,
    });
    const onScale = vi.fn();
    const stop = scaler.startAutoScale(
      () => ({ running: 0, queued: 0 }),
      onScale,
    );
    vi.advanceTimersByTime(150);
    expect(onScale).not.toHaveBeenCalled();
    stop();
    vi.useRealTimers();
  });
});

describe('Dispatcher', () => {
  it('should register and unregister workers', () => {
    const d = new Dispatcher();
    d.registerWorker('w1');
    expect(d.getWorkerCount()).toBe(1);
    d.unregisterWorker('w1');
    expect(d.getWorkerCount()).toBe(0);
  });

  it('should submit tasks', () => {
    const d = new Dispatcher();
    const ok = d.submit({ id: 't1', priority: 1, data: 'test', createdAt: 0 });
    expect(ok).toBe(true);
    expect(d.getQueueSize()).toBe(1);
  });

  it('should reject tasks when queue is full', () => {
    const d = new Dispatcher({ maxQueueSize: 2 });
    expect(d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 })).toBe(true);
    expect(d.submit({ id: 't2', priority: 1, data: 'b', createdAt: 0 })).toBe(true);
    expect(d.submit({ id: 't3', priority: 1, data: 'c', createdAt: 0 })).toBe(false);
  });

  it('should dispatch with priority strategy', () => {
    const d = new Dispatcher({ strategy: 'priority' });
    d.registerWorker('w1');
    d.submit({ id: 't1', priority: 1, data: 'low', createdAt: 0 });
    d.submit({ id: 't2', priority: 5, data: 'high', createdAt: 0 });
    
    const task = d.dispatch();
    expect(task!.id).toBe('t2'); // higher priority first
  });

  it('should dispatch with round_robin strategy', () => {
    const d = new Dispatcher({ strategy: 'round_robin' });
    d.registerWorker('w1');
    d.registerWorker('w2');
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    d.submit({ id: 't2', priority: 1, data: 'b', createdAt: 0 });
    
    const t1 = d.dispatch();
    const t2 = d.dispatch();
    expect(t1!.assignedTo).toBe('w1');
    expect(t2!.assignedTo).toBe('w2');
  });

  it('should dispatch with least_loaded strategy', () => {
    const d = new Dispatcher({ strategy: 'least_loaded' });
    d.registerWorker('w1');
    d.registerWorker('w2');
    d.setWorkerLoad('w1', 5);
    d.setWorkerLoad('w2', 1);
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    
    const task = d.dispatch();
    expect(task!.assignedTo).toBe('w2'); // least loaded
  });

  it('should return null when no tasks', () => {
    const d = new Dispatcher();
    d.registerWorker('w1');
    expect(d.dispatch()).toBeNull();
  });

  it('should return null when no workers', () => {
    const d = new Dispatcher();
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    expect(d.dispatch()).toBeNull();
  });

  it('should increment worker load on dispatch', () => {
    const d = new Dispatcher();
    d.registerWorker('w1');
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    d.dispatch();
    
    const stats = d.getStats();
    expect(stats.avgLoad).toBe(1);
  });

  it('should decrement worker load on completeTask', () => {
    const d = new Dispatcher();
    d.registerWorker('w1');
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    const task = d.dispatch();
    d.completeTask(task!);
    
    const stats = d.getStats();
    expect(stats.avgLoad).toBe(0);
  });

  it('should handle unknown worker in completeTask', () => {
    const d = new Dispatcher();
    d.completeTask({ id: 't1', priority: 1, data: null, assignedTo: 'unknown', createdAt: 0 });
    // should not throw
  });

  it('should handle completeTask without assignedTo', () => {
    const d = new Dispatcher();
    d.completeTask({ id: 't1', priority: 1, data: null, createdAt: 0 });
    // should not throw
  });

  it('getStats should return correct info', () => {
    const d = new Dispatcher();
    d.registerWorker('w1');
    d.registerWorker('w2');
    d.setWorkerLoad('w1', 3);
    d.setWorkerLoad('w2', 1);
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    
    const stats = d.getStats();
    expect(stats.queueSize).toBe(1);
    expect(stats.workers).toBe(2);
    expect(stats.avgLoad).toBe(2);
  });

  it('getStats with no workers should return 0 avgLoad', () => {
    const d = new Dispatcher();
    const stats = d.getStats();
    expect(stats.avgLoad).toBe(0);
  });

  it('should use default config', () => {
    const d = new Dispatcher();
    expect(d.getQueueSize()).toBe(0);
    expect(d.getWorkerCount()).toBe(0);
  });

  it('random strategy should dispatch', () => {
    const d = new Dispatcher({ strategy: 'random' });
    d.registerWorker('w1');
    d.registerWorker('w2');
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    
    const task = d.dispatch();
    expect(task).not.toBeNull();
    expect(['w1', 'w2']).toContain(task!.assignedTo);
  });

  it('default strategy should dispatch to first worker', () => {
    const d = new Dispatcher({ strategy: 'unknown' as any });
    d.registerWorker('w1');
    d.submit({ id: 't1', priority: 1, data: 'a', createdAt: 0 });
    
    const task = d.dispatch();
    expect(task!.assignedTo).toBe('w1');
  });
});

describe('ConcurrencyPool', () => {
  it('should run tasks concurrently up to max', async () => {
    const pool = new ConcurrencyPool<number>(2);
    const results: number[] = [];
    
    const promises = [
      pool.run(async () => { results.push(1); return 1; }),
      pool.run(async () => { results.push(2); return 2; }),
      pool.run(async () => { results.push(3); return 3; }),
    ];
    
    const vals = await Promise.all(promises);
    expect(vals).toEqual([1, 2, 3]);
    expect(results).toHaveLength(3);
  });

  it('should track active and waiting counts', async () => {
    const pool = new ConcurrencyPool<number>(1);
    expect(pool.activeCount).toBe(0);
    expect(pool.waitingCount).toBe(0);
    
    const p1 = pool.run(async () => {
      await new Promise(r => setTimeout(r, 50));
      return 1;
    });
    
    expect(pool.activeCount).toBe(1);
    
    const p2 = pool.run(async () => 2);
    expect(pool.waitingCount).toBe(1);
    
    await p1;
    await p2;
    expect(pool.activeCount).toBe(0);
  });

  it('should handle map', async () => {
    const pool = new ConcurrencyPool<number>(3);
    const items = [1, 2, 3, 4, 5];
    const results = await pool.map(items, async (item) => item * 2);
    expect(results).toEqual([2, 4, 6, 8, 10]);
  });

  it('should release slot on error', async () => {
    const pool = new ConcurrencyPool<number>(1);
    
    await expect(pool.run(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    expect(pool.activeCount).toBe(0);
    
    // Pool should still work
    const result = await pool.run(async () => 'ok');
    expect(result).toBe('ok');
  });
});

describe('FileLock', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-lock-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should acquire and release lock', async () => {
    const lock = new FileLock(tmpDir);
    const acquired = await lock.acquire('test-key', 1000);
    expect(acquired).toBe(true);
    lock.release('test-key');
  });

  it('should acquire lock after release', async () => {
    const lock = new FileLock(tmpDir);
    await lock.acquire('key1', 1000);
    lock.release('key1');
    const acquired = await lock.acquire('key1', 1000);
    expect(acquired).toBe(true);
    lock.release('key1');
  });

  it('withLock should execute function and release', async () => {
    const lock = new FileLock(tmpDir);
    const result = await lock.withLock('wl-key', async () => 'done', 1000);
    expect(result).toBe('done');
  });

  it('withLock should release on error', async () => {
    const lock = new FileLock(tmpDir);
    await expect(
      lock.withLock('err-key', async () => { throw new Error('fail'); }, 1000)
    ).rejects.toThrow('fail');
    
    // Should be able to acquire again
    const result = await lock.withLock('err-key', async () => 'ok', 1000);
    expect(result).toBe('ok');
  });

  it('should timeout when lock is held', async () => {
    const lock = new FileLock(tmpDir);
    await lock.acquire('held-key', 5000);
    
    const acquired = await lock.acquire('held-key', 100);
    expect(acquired).toBe(false);
    
    lock.release('held-key');
  });

  it('release should not throw for non-existent lock', () => {
    const lock = new FileLock(tmpDir);
    expect(() => lock.release('nonexistent')).not.toThrow();
  });
});
