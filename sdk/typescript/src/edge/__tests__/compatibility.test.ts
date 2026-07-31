/**
 * compatibility.ts unit tests
 *
 * Coverage:
 * - detectEnv() environment detection mapping
 * - isEdgeRuntime() / isNodeRuntime()
 * - createKVStore() InMemory fallback (put/get/delete/list/TTL)
 * - edgeFetch() header injection for edge
 * - createAgent() simplified agent creation
 */
import { describe, it, expect, afterEach, vi } from 'vitest';
import {
  detectEnv,
  isEdgeRuntime,
  isNodeRuntime,
  createKVStore,
  edgeFetch,
  createAgent,
} from '../compatibility.js';
import { resetRuntimeCache } from '../runtime.js';

const g = globalThis as Record<string, unknown>;

function cleanGlobals() {
  delete g.Deno;
  delete g.Bun;
  delete g.caches;
  delete g.WebSocketPair;
}

describe('detectEnv', () => {
  afterEach(() => { cleanGlobals(); resetRuntimeCache(); });

  it('should return "node" in Node.js', () => {
    resetRuntimeCache();
    expect(detectEnv()).toBe('node');
  });

  it('should return "cloudflare-workers" when CF markers present', () => {
    g.caches = {};
    g.WebSocketPair = class {};
    resetRuntimeCache();
    expect(detectEnv()).toBe('cloudflare-workers');
  });

  it('should return "deno" when Deno global present', () => {
    g.Deno = { version: { deno: '1.40.0' } };
    resetRuntimeCache();
    expect(detectEnv()).toBe('deno');
  });

  it('should return "bun" when Bun global present', () => {
    g.Bun = { version: '1.0.0' };
    resetRuntimeCache();
    expect(detectEnv()).toBe('bun');
  });
});

describe('isEdgeRuntime', () => {
  afterEach(() => { cleanGlobals(); resetRuntimeCache(); });

  it('should return false in Node.js', () => {
    resetRuntimeCache();
    expect(isEdgeRuntime()).toBe(false);
  });

  it('should return true for Cloudflare Workers', () => {
    g.caches = {};
    g.WebSocketPair = class {};
    resetRuntimeCache();
    expect(isEdgeRuntime()).toBe(true);
  });
});

describe('isNodeRuntime', () => {
  afterEach(() => { cleanGlobals(); resetRuntimeCache(); });

  it('should return true in Node.js', () => {
    resetRuntimeCache();
    expect(isNodeRuntime()).toBe(true);
  });

  it('should return false for Deno', () => {
    g.Deno = { version: { deno: '1.0.0' } };
    resetRuntimeCache();
    expect(isNodeRuntime()).toBe(false);
  });
});

describe('createKVStore (InMemory fallback)', () => {
  afterEach(() => { cleanGlobals(); resetRuntimeCache(); });

  it('should create an InMemoryKVStore in Node.js (no KV env)', async () => {
    resetRuntimeCache();
    const kv = await createKVStore();
    expect(kv).toBeDefined();
    expect(typeof kv.get).toBe('function');
    expect(typeof kv.put).toBe('function');
    expect(typeof kv.delete).toBe('function');
    expect(typeof kv.list).toBe('function');
  });

  it('should put and get a value', async () => {
    resetRuntimeCache();
    const kv = await createKVStore();
    await kv.put('key1', 'value1');
    expect(await kv.get('key1')).toBe('value1');
  });

  it('should return null for missing key', async () => {
    resetRuntimeCache();
    const kv = await createKVStore();
    expect(await kv.get('nonexistent')).toBeNull();
  });

  it('should delete a key', async () => {
    resetRuntimeCache();
    const kv = await createKVStore();
    await kv.put('key1', 'value1');
    await kv.delete('key1');
    expect(await kv.get('key1')).toBeNull();
  });

  it('should list keys with prefix', async () => {
    resetRuntimeCache();
    const kv = await createKVStore();
    await kv.put('user:1', 'a');
    await kv.put('user:2', 'b');
    await kv.put('post:1', 'c');
    const keys = await kv.list({ prefix: 'user:' });
    expect(keys).toHaveLength(2);
    expect(keys.sort()).toEqual(['user:1', 'user:2']);
  });

  it('should respect list limit', async () => {
    resetRuntimeCache();
    const kv = await createKVStore();
    await kv.put('a', '1');
    await kv.put('b', '2');
    await kv.put('c', '3');
    const keys = await kv.list({ limit: 2 });
    expect(keys).toHaveLength(2);
  });

  it('should expire keys with TTL', async () => {
    resetRuntimeCache();
    const kv = await createKVStore();
    await kv.put('ephemeral', 'data', { expirationTtl: 1 });
    expect(await kv.get('ephemeral')).toBe('data');
    await new Promise((r) => setTimeout(r, 1100));
    expect(await kv.get('ephemeral')).toBeNull();
  });
});

describe('edgeFetch', () => {
  afterEach(() => { cleanGlobals(); resetRuntimeCache(); vi.restoreAllMocks(); });

  it('should call fetch directly in Node.js', async () => {
    resetRuntimeCache();
    const mockResp = new Response('ok', { status: 200 });
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(mockResp);
    const resp = await edgeFetch('https://example.com/test');
    expect(resp.status).toBe(200);
    expect(fetchSpy).toHaveBeenCalledWith('https://example.com/test', {});
  });

  it('should add Edge headers when in CF runtime', async () => {
    g.caches = {};
    g.WebSocketPair = class {};
    resetRuntimeCache();
    const mockResp = new Response('ok', { status: 200 });
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(mockResp);
    await edgeFetch('https://example.com/api');
    const callArgs = fetchSpy.mock.calls[0];
    const init = callArgs![1] as RequestInit;
    const headers = new Headers(init.headers);
    expect(headers.get('User-Agent')).toBe('AgentPrimordia-Edge/1.0');
    expect(headers.get('Connection')).toBe('close');
  });
});

describe('createAgent', () => {
  afterEach(() => { cleanGlobals(); resetRuntimeCache(); });

  it('should create an agent with name', async () => {
    resetRuntimeCache();
    const agent = await createAgent({ name: 'test-agent' });
    expect(agent.name).toBe('test-agent');
    expect(typeof agent.run).toBe('function');
    expect(typeof agent.streamRun).toBe('function');
  });

  it('should return a response from run()', async () => {
    resetRuntimeCache();
    const agent = await createAgent({ name: 'test-agent' });
    const result = await agent.run('hello');
    expect(result).toContain('test-agent');
    expect(result).toContain('hello');
  });

  it('should yield chunks from streamRun()', async () => {
    resetRuntimeCache();
    const agent = await createAgent({ name: 'stream-agent' });
    const chunks: string[] = [];
    for await (const chunk of agent.streamRun('hi')) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThan(0);
    expect(chunks.join('')).toContain('stream-agent');
  });
});
