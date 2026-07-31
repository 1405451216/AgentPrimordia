/**
 * Edge Runtime 适配层 — 检测运行时环境并提供统一 API。
 *
 * Stability: Stable
 *
 * 支持：Node.js / Cloudflare Workers / Deno / Bun
 * 与 Go 端不同，TS SDK 可以跑在 Edge Runtime 上（V8 isolates），
 * 这是 TS 相对 Go 的核心优势之一。
 *
 * 使用方式：
 *   const runtime = detectRuntime();
 *   if (runtime.isEdge) { // 使用 Edge 兼容的 API }
 */

import { createRequire } from 'node:module';

const nodeRequire = createRequire(import.meta.url);

// ===== 运行时类型 =====

export type RuntimeName = 'node' | 'cloudflare' | 'deno' | 'bun' | 'browser' | 'unknown';

export interface RuntimeInfo {
  name: RuntimeName;
  version: string;
  isEdge: boolean;
  isNode: boolean;
  isBrowser: boolean;
  /** 是否支持 worker_threads（仅 Node.js） */
  supportsWorkerThreads: boolean;
  /** 是否支持 WebSocket（浏览器、Deno、Bun、Node 22+） */
  supportsWebSocket: boolean;
  /** 是否支持 crypto.subtle（所有现代运行时） */
  supportsCryptoSubtle: boolean;
  /** 是否支持 fetch（所有现代运行时） */
  supportsFetch: boolean;
  /** 是否支持 ReadableStream / WritableStream */
  supportsStreams: boolean;
}

// ===== 运行时检测 =====

let cachedRuntime: RuntimeInfo | null = null;

/** 检测当前运行时环境（结果缓存） */
export function detectRuntime(): RuntimeInfo {
  if (cachedRuntime) return cachedRuntime;

  const info = _detect();
  cachedRuntime = info;
  return info;
}

function _detect(): RuntimeInfo {
  // Cloudflare Workers
  if (typeof globalThis !== 'undefined' && 'caches' in globalThis && typeof (globalThis as Record<string, unknown>).WebSocketPair !== 'undefined') {
    return {
      name: 'cloudflare',
      version: '',
      isEdge: true,
      isNode: false,
      isBrowser: false,
      supportsWorkerThreads: false,
      supportsWebSocket: true,
      supportsCryptoSubtle: true,
      supportsFetch: true,
      supportsStreams: true,
    };
  }

  // Deno
  if (typeof globalThis !== 'undefined' && typeof (globalThis as Record<string, unknown>).Deno !== 'undefined') {
    const deno = (globalThis as { Deno?: { version?: { deno?: string } } }).Deno;
    return {
      name: 'deno',
      version: deno?.version?.deno ?? '',
      isEdge: false,
      isNode: false,
      isBrowser: false,
      supportsWorkerThreads: true, // Deno has Web Workers
      supportsWebSocket: true,
      supportsCryptoSubtle: true,
      supportsFetch: true,
      supportsStreams: true,
    };
  }

  // Bun
  if (typeof globalThis !== 'undefined' && typeof (globalThis as Record<string, unknown>).Bun !== 'undefined') {
    const bun = (globalThis as { Bun?: { version?: string } }).Bun;
    return {
      name: 'bun',
      version: bun?.version ?? '',
      isEdge: false,
      isNode: false,
      isBrowser: false,
      supportsWorkerThreads: true, // Bun supports worker_threads
      supportsWebSocket: true,
      supportsCryptoSubtle: true,
      supportsFetch: true,
      supportsStreams: true,
    };
  }

  // Node.js
  if (typeof process !== 'undefined' && process.versions?.node) {
    const nodeVer = process.versions.node;
    const major = parseInt(nodeVer.split('.')[0]!, 10);
    return {
      name: 'node',
      version: nodeVer,
      isEdge: false,
      isNode: true,
      isBrowser: false,
      supportsWorkerThreads: major >= 18,
      supportsWebSocket: major >= 22, // Node 22+ has global WebSocket
      supportsCryptoSubtle: true,
      supportsFetch: major >= 18,
      supportsStreams: true,
    };
  }

  // Browser
  if (typeof window !== 'undefined' && typeof document !== 'undefined') {
    return {
      name: 'browser',
      version: navigator?.userAgent ?? '',
      isEdge: false,
      isNode: false,
      isBrowser: true,
      supportsWorkerThreads: false, // Browsers use Web Workers, not worker_threads
      supportsWebSocket: true,
      supportsCryptoSubtle: true,
      supportsFetch: true,
      supportsStreams: true,
    };
  }

  return {
    name: 'unknown',
    version: '',
    isEdge: false,
    isNode: false,
    isBrowser: false,
    supportsWorkerThreads: false,
    supportsWebSocket: false,
    supportsCryptoSubtle: false,
    supportsFetch: false,
    supportsStreams: false,
  };
}

// ===== 平台无关工具函数 =====

/** 获取适合当前运行时的 WebSocket 构造器 */
export function getWebSocketConstructor(): typeof WebSocket | null {
  const rt = detectRuntime();

  // Node.js 22+ has global WebSocket
  if (rt.supportsWebSocket) {
    return globalThis.WebSocket ?? null;
  }

  // Node.js < 22: try to load ws package
  if (rt.isNode) {
    try {
      const ws = nodeRequire('ws');
      return ws.WebSocket ?? ws.default ?? ws;
    } catch {
      return null;
    }
  }

  return null;
}

/** 获取适合当前运行时的定时器（Edge Runtime 不支持 setTimeout 长延迟） */
export function createTimer(callback: () => void, ms: number): { clear: () => void } {
  const rt = detectRuntime();

  if (rt.isEdge && ms > 30000) {
    // Edge Runtime: setTimeout 上限 30s，长延迟需要分片
    let cancelled = false;
    let remaining = ms;
    const tick = 30000;

    function schedule() {
      if (cancelled || remaining <= 0) {
        if (!cancelled) callback();
        return;
      }
      const delay = Math.min(remaining, tick);
      remaining -= delay;
      setTimeout(schedule, delay);
    }

    setTimeout(schedule, Math.min(ms, tick));
    return { clear: () => { cancelled = true; } };
  }

  const id = setTimeout(callback, ms);
  return { clear: () => clearTimeout(id) };
}

/** 检查当前运行时是否支持某个 API */
export function supports(api: 'worker_threads' | 'websocket' | 'fetch' | 'streams' | 'crypto'): boolean {
  const rt = detectRuntime();
  switch (api) {
    case 'worker_threads': return rt.supportsWorkerThreads;
    case 'websocket': return rt.supportsWebSocket;
    case 'fetch': return rt.supportsFetch;
    case 'streams': return rt.supportsStreams;
    case 'crypto': return rt.supportsCryptoSubtle;
    default: return false;
  }
}

/** 重置运行时检测缓存（用于测试） */
export function resetRuntimeCache(): void {
  cachedRuntime = null;
}
