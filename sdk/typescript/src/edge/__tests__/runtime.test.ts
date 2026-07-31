/**
 * runtime.ts unit tests
 *
 * Coverage:
 * - detectRuntime() for node / deno / bun / cloudflare
 * - resetRuntimeCache() clears cached detection
 * - supports() API capability check
 * - createTimer() callback and clear
 * - getWebSocketConstructor()
 */
import { describe, it, expect, afterEach, vi } from 'vitest';
import {
  detectRuntime,
  resetRuntimeCache,
  supports,
  createTimer,
  getWebSocketConstructor,
  type RuntimeInfo,
} from '../runtime.js';

const g = globalThis as Record<string, unknown>;

function cleanGlobals() {
  delete g.Deno;
  delete g.Bun;
  delete g.caches;
  delete g.WebSocketPair;
  delete g.window;
  delete g.document;
}

describe('detectRuntime', () => {
  afterEach(() => {
    cleanGlobals();
    resetRuntimeCache();
  });

  it('should detect Node.js runtime', () => {
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('node');
    expect(info.isNode).toBe(true);
    expect(info.isEdge).toBe(false);
    expect(info.isBrowser).toBe(false);
    expect(info.version).toBeTruthy();
    expect(info.supportsFetch).toBe(true);
    expect(info.supportsCryptoSubtle).toBe(true);
    expect(info.supportsStreams).toBe(true);
  });

  it('should detect Cloudflare Workers runtime', () => {
    g.caches = {};
    g.WebSocketPair = class {};
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('cloudflare');
    expect(info.isEdge).toBe(true);
    expect(info.isNode).toBe(false);
    expect(info.isBrowser).toBe(false);
    expect(info.supportsWebSocket).toBe(true);
    expect(info.supportsWorkerThreads).toBe(false);
  });

  it('should detect Deno runtime', () => {
    g.Deno = { version: { deno: '1.40.0' } };
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('deno');
    expect(info.version).toBe('1.40.0');
    expect(info.isEdge).toBe(false);
    expect(info.isNode).toBe(false);
    expect(info.supportsWorkerThreads).toBe(true);
  });

  it('should detect Bun runtime', () => {
    g.Bun = { version: '1.0.25' };
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('bun');
    expect(info.version).toBe('1.0.25');
    expect(info.isEdge).toBe(false);
    expect(info.isNode).toBe(false);
    expect(info.supportsWorkerThreads).toBe(true);
  });

  it('should cache the result on second call', () => {
    resetRuntimeCache();
    const first = detectRuntime();
    const second = detectRuntime();
    expect(first).toBe(second);
  });

  it('should re-detect after resetRuntimeCache', () => {
    resetRuntimeCache();
    const info1 = detectRuntime();
    expect(info1.name).toBe('node');

    g.Deno = { version: { deno: '2.0.0' } };
    resetRuntimeCache();
    const info2 = detectRuntime();
    expect(info2.name).toBe('deno');
  });

  it('should prioritize Cloudflare over Deno when both markers present', () => {
    g.caches = {};
    g.WebSocketPair = class {};
    g.Deno = { version: { deno: '1.0.0' } };
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('cloudflare');
  });

  it('should prioritize Deno over Bun when both markers present', () => {
    g.Deno = { version: { deno: '1.0.0' } };
    g.Bun = { version: '1.0.0' };
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('deno');
  });

  it('should prioritize Bun over Node when both markers present', () => {
    g.Bun = { version: '1.0.0' };
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('bun');
  });

  it('should handle Deno without version info', () => {
    g.Deno = {};
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('deno');
    expect(info.version).toBe('');
  });

  it('should handle Bun without version info', () => {
    g.Bun = {};
    resetRuntimeCache();
    const info = detectRuntime();
    expect(info.name).toBe('bun');
    expect(info.version).toBe('');
  });
});

describe('supports', () => {
  afterEach(() => {
    resetRuntimeCache();
  });

  it('should report fetch support in Node', () => {
    resetRuntimeCache();
    expect(supports('fetch')).toBe(true);
    expect(supports('streams')).toBe(true);
    expect(supports('crypto')).toBe(true);
  });

  it('should report worker_threads support in Node 18+', () => {
    resetRuntimeCache();
    expect(supports('worker_threads')).toBe(true);
  });

  it('should report websocket support', () => {
    resetRuntimeCache();
    const result = supports('websocket');
    expect(typeof result).toBe('boolean');
  });
});

describe('createTimer', () => {
  afterEach(() => {
    resetRuntimeCache();
  });

  it('should call callback after timeout', async () => {
    resetRuntimeCache();
    const cb = vi.fn();
    const timer = createTimer(cb, 50);
    await new Promise((r) => setTimeout(r, 120));
    expect(cb).toHaveBeenCalledOnce();
    timer.clear();
  });

  it('should not call callback if cleared', async () => {
    resetRuntimeCache();
    const cb = vi.fn();
    const timer = createTimer(cb, 50);
    timer.clear();
    await new Promise((r) => setTimeout(r, 120));
    expect(cb).not.toHaveBeenCalled();
  });
});

describe('getWebSocketConstructor', () => {
  afterEach(() => {
    resetRuntimeCache();
  });

  it('should return null or WebSocket in Node', () => {
    resetRuntimeCache();
    const ws = getWebSocketConstructor();
    expect(ws === null || typeof ws === 'function').toBe(true);
  });
});
