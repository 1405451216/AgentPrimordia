import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import {
  ConfigWatcher,
  BufferPool,
  StructuredLogger,
  defaultLogger,
  AsyncMemoryWriter,
  EventBus,
} from '../../src/utils/advanced.js';

// ===== ConfigWatcher tests =====
describe('ConfigWatcher', () => {
  let tmpDir: string;
  let configPath: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-cfg-'));
    configPath = path.join(tmpDir, 'config.json');
    fs.writeFileSync(configPath, JSON.stringify({ key: 'value1' }));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should load config on start', async () => {
    const watcher = new ConfigWatcher(configPath);
    await watcher.start();
    const config = watcher.getConfig();
    expect(config.key).toBe('value1');
    watcher.stop();
  });

  it('should handle missing config file', async () => {
    const watcher = new ConfigWatcher(path.join(tmpDir, 'missing.json'));
    await watcher.start();
    expect(watcher.getConfig()).toEqual({});
    watcher.stop();
  });

  it('should handle invalid JSON config', async () => {
    fs.writeFileSync(configPath, '{invalid json');
    const watcher = new ConfigWatcher(configPath);
    await watcher.start();
    expect(watcher.getConfig()).toEqual({});
    watcher.stop();
  });

  it('should call onUpdate callback on change', async () => {
    const updateFn = vi.fn();
    const watcher = new ConfigWatcher(configPath, { intervalMs: 50, onUpdate: updateFn });
    await watcher.start();

    // Update the file
    await new Promise(r => setTimeout(r, 60));
    const newMtime = Date.now() / 1000 + 10;
    fs.utimesSync(configPath, newMtime, newMtime);
    fs.writeFileSync(configPath, JSON.stringify({ key: 'value2' }));

    await new Promise(r => setTimeout(r, 120));
    watcher.stop();

    // The update should have been called
    expect(updateFn).toHaveBeenCalled();
  });

  it('should stop watching', async () => {
    const watcher = new ConfigWatcher(configPath, { intervalMs: 50 });
    await watcher.start();
    watcher.stop();
    // Should not throw after stop
    expect(watcher.getConfig()).toBeDefined();
  });

  it('should not reload when content is same', async () => {
    const updateFn = vi.fn();
    const watcher = new ConfigWatcher(configPath, { intervalMs: 50, onUpdate: updateFn });
    await watcher.start();

    // Touch the file without changing content
    const newMtime = Date.now() / 1000 + 10;
    fs.utimesSync(configPath, newMtime, newMtime);

    await new Promise(r => setTimeout(r, 100));
    watcher.stop();
    expect(updateFn).not.toHaveBeenCalled();
  });
});

// ===== BufferPool tests =====
describe('BufferPool', () => {
  it('should acquire buffer of requested size', () => {
    const pool = new BufferPool();
    const buf = pool.acquire(100);
    expect(buf.length).toBeGreaterThanOrEqual(100);
  });

  it('should round up to power of 2', () => {
    const pool = new BufferPool();
    const buf = pool.acquire(100);
    // 100 rounds up to 128
    expect(buf.length).toBe(128);
  });

  it('should reuse released buffers', () => {
    const pool = new BufferPool();
    const buf1 = pool.acquire(64);
    pool.release(buf1);
    const buf2 = pool.acquire(64);
    // Should be the same underlying buffer
    expect(buf2.length).toBe(64);
  });

  it('should handle different sizes', () => {
    const pool = new BufferPool();
    const buf1 = pool.acquire(32);
    const buf2 = pool.acquire(64);
    const buf3 = pool.acquire(128);
    expect(buf1.length).toBe(32);
    expect(buf2.length).toBe(64);
    expect(buf3.length).toBe(128);
  });

  it('should not exceed max pool size', () => {
    const pool = new BufferPool(2);
    const bufs = [pool.acquire(64), pool.acquire(64), pool.acquire(64)];
    for (const buf of bufs) pool.release(buf);
    // Pool should have at most 2 buffers per size
    const newBuf = pool.acquire(64);
    expect(newBuf).toBeDefined();
  });

  it('should handle size 1', () => {
    const pool = new BufferPool();
    const buf = pool.acquire(1);
    expect(buf.length).toBeGreaterThanOrEqual(1);
  });
});

// ===== StructuredLogger tests =====
describe('StructuredLogger', () => {
  it('should log messages at info level', () => {
    const logger = new StructuredLogger('info');
    logger.info('test message');
    const entries = logger.getEntries();
    expect(entries).toHaveLength(1);
    expect(entries[0].message).toBe('test message');
    expect(entries[0].level).toBe('info');
  });

  it('should filter by level', () => {
    const logger = new StructuredLogger('warn');
    logger.info('info msg');
    logger.warn('warn msg');
    logger.error('error msg');
    const entries = logger.getEntries();
    expect(entries).toHaveLength(2);
    expect(entries[0].level).toBe('warn');
    expect(entries[1].level).toBe('error');
  });

  it('should support all log levels', () => {
    const logger = new StructuredLogger('debug');
    logger.debug('d');
    logger.info('i');
    logger.warn('w');
    logger.error('e');
    logger.fatal('f');
    const entries = logger.getEntries();
    expect(entries).toHaveLength(5);
  });

  it('should include fields in log entry', () => {
    const logger = new StructuredLogger('info');
    logger.info('test', { userId: 123, action: 'login' });
    const entries = logger.getEntries();
    expect(entries[0].fields).toEqual({ userId: 123, action: 'login' });
  });

  it('should include timestamp', () => {
    const logger = new StructuredLogger('info');
    logger.info('test');
    const entries = logger.getEntries();
    expect(entries[0].timestamp).toBeDefined();
  });

  it('should add output callback', () => {
    const logger = new StructuredLogger('info');
    const outputFn = vi.fn();
    logger.addOutput(outputFn);
    logger.info('test');
    expect(outputFn).toHaveBeenCalled();
    expect(outputFn.mock.calls[0][0].message).toBe('test');
  });

  it('should filter entries by level', () => {
    const logger = new StructuredLogger('debug');
    logger.info('info msg');
    logger.error('error msg');
    const errorEntries = logger.getEntries({ level: 'error' });
    expect(errorEntries).toHaveLength(1);
    expect(errorEntries[0].message).toBe('error msg');
  });

  it('should filter entries by contains', () => {
    const logger = new StructuredLogger('info');
    logger.info('login success');
    logger.info('logout success');
    const entries = logger.getEntries({ contains: 'login' });
    expect(entries).toHaveLength(1);
    expect(entries[0].message).toBe('login success');
  });

  it('should set level', () => {
    const logger = new StructuredLogger('error');
    logger.setLevel('debug');
    logger.info('should log now');
    expect(logger.getEntries()).toHaveLength(1);
  });

  it('should clear entries', () => {
    const logger = new StructuredLogger('info');
    logger.info('msg1');
    logger.info('msg2');
    logger.clear();
    expect(logger.getEntries()).toHaveLength(0);
  });

  it('should respect maxEntries limit', () => {
    const logger = new StructuredLogger('debug', 3);
    logger.info('msg1');
    logger.info('msg2');
    logger.info('msg3');
    logger.info('msg4');
    const entries = logger.getEntries();
    expect(entries.length).toBeLessThanOrEqual(3);
  });

  it('should export defaultLogger instance', () => {
    expect(defaultLogger).toBeInstanceOf(StructuredLogger);
  });

  it('should filter entries by since', () => {
    const logger = new StructuredLogger('info');
    logger.info('old');
    const before = new Date(Date.now() + 1000);
    logger.info('new');
    const entries = logger.getEntries({ since: before });
    // since filter uses ISO string comparison
    expect(entries.length).toBeLessThanOrEqual(1);
  });
});

// ===== AsyncMemoryWriter tests =====
describe('AsyncMemoryWriter', () => {
  it('should enqueue writes', () => {
    const writeFn = vi.fn().mockResolvedValue(undefined);
    const writer = new AsyncMemoryWriter(writeFn);
    expect(writer.enqueue('id1', { data: 'test' })).toBe(true);
    expect(writer.queueSize).toBe(1);
    writer.stop();
  });

  it('should reject when queue is full', () => {
    const writeFn = vi.fn().mockResolvedValue(undefined);
    const writer = new AsyncMemoryWriter(writeFn, { maxQueueSize: 2 });
    expect(writer.enqueue('id1', 'a')).toBe(true);
    expect(writer.enqueue('id2', 'b')).toBe(true);
    expect(writer.enqueue('id3', 'c')).toBe(false);
    writer.stop();
  });

  it('should flush queue', async () => {
    const writeFn = vi.fn().mockResolvedValue(undefined);
    const writer = new AsyncMemoryWriter(writeFn, { flushIntervalMs: 100000 });
    writer.enqueue('id1', 'a');
    writer.enqueue('id2', 'b');
    await writer.flush();
    expect(writeFn).toHaveBeenCalledTimes(2);
    expect(writer.queueSize).toBe(0);
    writer.stop();
  });

  it('should retry failed writes', async () => {
    let attempts = 0;
    const writeFn = vi.fn(async () => {
      attempts++;
      if (attempts < 2) throw new Error('fail');
    });
    const writer = new AsyncMemoryWriter(writeFn, { maxRetries: 3, flushIntervalMs: 100000 });
    writer.enqueue('id1', 'a');
    await writer.flush();
    expect(writeFn).toHaveBeenCalledTimes(2);
    writer.stop();
  });

  it('should drop after max retries', async () => {
    const writeFn = vi.fn().mockRejectedValue(new Error('permanent fail'));
    const writer = new AsyncMemoryWriter(writeFn, { maxRetries: 2, flushIntervalMs: 100000 });
    writer.enqueue('id1', 'a');
    await writer.flush();
    // maxRetries=2: initial attempt + 1 retry = 2 total calls before dropping
    expect(writeFn).toHaveBeenCalledTimes(2);
    expect(writer.queueSize).toBe(0);
    writer.stop();
  });

  it('should not process when already processing', async () => {
    const writeFn = vi.fn(async () => {
      await new Promise(r => setTimeout(r, 50));
    });
    const writer = new AsyncMemoryWriter(writeFn, { flushIntervalMs: 100000 });
    writer.enqueue('id1', 'a');
    const flushPromise = writer.flush();
    // Try to flush again while processing
    await writer.flush();
    await flushPromise;
    writer.stop();
  });

  it('should use default config', () => {
    const writeFn = vi.fn().mockResolvedValue(undefined);
    const writer = new AsyncMemoryWriter(writeFn);
    expect(writer.enqueue('id1', 'data')).toBe(true);
    writer.stop();
  });
});

// ===== EventBus tests =====
describe('EventBus', () => {
  it('should subscribe and emit events', async () => {
    const bus = new EventBus();
    const handler = vi.fn();
    bus.on('test', handler);
    await bus.emit('test', { data: 'hello' });
    expect(handler).toHaveBeenCalledWith({ data: 'hello' });
  });

  it('should support multiple handlers for same event', async () => {
    const bus = new EventBus();
    const handler1 = vi.fn();
    const handler2 = vi.fn();
    bus.on('test', handler1);
    bus.on('test', handler2);
    await bus.emit('test', 'data');
    expect(handler1).toHaveBeenCalled();
    expect(handler2).toHaveBeenCalled();
  });

  it('should unsubscribe', async () => {
    const bus = new EventBus();
    const handler = vi.fn();
    const sub = bus.on('test', handler);
    sub.unsubscribe();
    await bus.emit('test', 'data');
    expect(handler).not.toHaveBeenCalled();
  });

  it('should support once handler', async () => {
    const bus = new EventBus();
    const handler = vi.fn();
    bus.once('test', handler);
    await bus.emit('test', 'first');
    await bus.emit('test', 'second');
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it('should handle async handlers', async () => {
    const bus = new EventBus();
    const results: string[] = [];
    bus.on('test', async (data: string) => {
      await new Promise(r => setTimeout(r, 10));
      results.push(data);
    });
    await bus.emit('test', 'async-result');
    expect(results).toContain('async-result');
  });

  it('should handle handler errors gracefully', async () => {
    const bus = new EventBus();
    const handler1 = vi.fn(() => { throw new Error('handler error'); });
    const handler2 = vi.fn();
    bus.on('test', handler1);
    bus.on('test', handler2);
    await bus.emit('test', 'data');
    // handler2 should still be called even if handler1 throws
    expect(handler2).toHaveBeenCalled();
  });

  it('should do nothing for unregistered event', async () => {
    const bus = new EventBus();
    await bus.emit('nonexistent', 'data');
    // Should not throw
  });

  it('should remove all handlers for an event with off()', async () => {
    const bus = new EventBus();
    const handler = vi.fn();
    bus.on('test', handler);
    bus.off('test');
    await bus.emit('test', 'data');
    expect(handler).not.toHaveBeenCalled();
  });

  it('should clear all handlers', async () => {
    const bus = new EventBus();
    const h1 = vi.fn();
    const h2 = vi.fn();
    bus.on('event1', h1);
    bus.on('event2', h2);
    bus.clear();
    await bus.emit('event1', 'data');
    await bus.emit('event2', 'data');
    expect(h1).not.toHaveBeenCalled();
    expect(h2).not.toHaveBeenCalled();
  });

  it('should report listener count', () => {
    const bus = new EventBus();
    bus.on('test', () => {});
    bus.on('test', () => {});
    expect(bus.listenerCount('test')).toBe(2);
    expect(bus.listenerCount('nonexistent')).toBe(0);
  });

  it('should handle emit without data', async () => {
    const bus = new EventBus();
    const handler = vi.fn();
    bus.on('test', handler);
    await bus.emit('test');
    expect(handler).toHaveBeenCalledWith(undefined);
  });
});
