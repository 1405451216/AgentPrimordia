/**
 * cloudflare-adapter.ts unit tests
 *
 * Coverage:
 * - CloudflareAdapter routing: POST /chat, GET /session/:id, DELETE /session/:id, GET /health
 * - CORS preflight (OPTIONS)
 * - 404 for unknown routes
 * - adaptStorage() KV-to-EdgeStorage bridge
 * - MemoryKVEdgeAdapter fallback (no KV namespace)
 * - createWorkerHandler() convenience function
 * - Error handling (500)
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  CloudflareAdapter,
  createWorkerHandler,
  type EdgeConfig,
  type EdgeStorageAdapter,
  type ExportedHandler,
  type ExecutionContext,
} from '../cloudflare-adapter.js';

function makeRequest(url: string, init?: RequestInit): Request {
  return new Request(url, init);
}

const mockCtx: ExecutionContext = {
  waitUntil: vi.fn(),
  passThroughOnException: vi.fn(),
};

function createMockKV(): EdgeStorageAdapter {
  const store = new Map<string, { value: string; expires?: number }>();
  return {
    get: vi.fn(async (key: string) => {
      const e = store.get(key);
      if (!e) return null;
      if (e.expires && Date.now() > e.expires) { store.delete(key); return null; }
      return e.value;
    }),
    put: vi.fn(async (key: string, value: string, opts?: { expirationTtl?: number }) => {
      const expires = opts?.expirationTtl ? Date.now() + opts.expirationTtl * 1000 : undefined;
      store.set(key, { value, expires });
    }),
    delete: vi.fn(async (key: string) => { store.delete(key); }),
    list: vi.fn(async (opts?: { prefix?: string; limit?: number }) => {
      const keys: string[] = [];
      for (const [k] of store) {
        if (!opts?.prefix || k.startsWith(opts.prefix)) keys.push(k);
      }
      return keys.slice(0, opts?.limit ?? 1000);
    }),
  };
}

const BASE_CONFIG: EdgeConfig = { name: 'test-worker' };

describe('CloudflareAdapter', () => {
  let kv: EdgeStorageAdapter;
  let handler: ExportedHandler;

  beforeEach(() => {
    kv = createMockKV();
    const adapter = new CloudflareAdapter({ ...BASE_CONFIG, kvNamespace: kv });
    handler = adapter.createHandler();
  });

  describe('GET /health', () => {
    it('should return ok status', async () => {
      const req = makeRequest('https://worker.test/health');
      const resp = await handler.fetch(req, {}, mockCtx);
      expect(resp.status).toBe(200);
      const body = await resp.json() as Record<string, unknown>;
      expect(body.status).toBe('ok');
      expect(body.name).toBe('test-worker');
    });
  });

  describe('POST /chat', () => {
    it('should return a response with sessionId', async () => {
      const req = makeRequest('https://worker.test/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: 'hello', sessionId: 'sess-1' }),
      });
      const resp = await handler.fetch(req, {}, mockCtx);
      expect(resp.status).toBe(200);
      const body = await resp.json() as Record<string, unknown>;
      expect(body.response).toContain('test-worker');
      expect(body.response).toContain('hello');
      expect(body.sessionId).toBe('sess-1');
    });

    it('should auto-generate sessionId when not provided', async () => {
      const req = makeRequest('https://worker.test/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: 'hi' }),
      });
      const resp = await handler.fetch(req, {}, mockCtx);
      const body = await resp.json() as Record<string, unknown>;
      expect(body.sessionId).toBeTruthy();
      expect(typeof body.sessionId).toBe('string');
    });

    it('should persist session history in KV', async () => {
      const req = makeRequest('https://worker.test/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: 'test', sessionId: 'sess-hist' }),
      });
      await handler.fetch(req, {}, mockCtx);
      expect(kv.put).toHaveBeenCalledWith('session:sess-hist', expect.any(String), { expirationTtl: 86400 });
    });
  });

  describe('GET /session/:id', () => {
    it('should return session history', async () => {
      const chatReq = makeRequest('https://worker.test/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: 'msg1', sessionId: 'sess-2' }),
      });
      await handler.fetch(chatReq, {}, mockCtx);
      const getReq = makeRequest('https://worker.test/session/sess-2');
      const resp = await handler.fetch(getReq, {}, mockCtx);
      expect(resp.status).toBe(200);
      const body = await resp.json() as Record<string, unknown>;
      expect(body.sessionId).toBe('sess-2');
      expect(Array.isArray(body.history)).toBe(true);
      expect((body.history as unknown[]).length).toBeGreaterThan(0);
    });

    it('should return empty history for unknown session', async () => {
      const req = makeRequest('https://worker.test/session/unknown');
      const resp = await handler.fetch(req, {}, mockCtx);
      expect(resp.status).toBe(200);
      const body = await resp.json() as Record<string, unknown>;
      expect(body.history).toEqual([]);
    });
  });

  describe('DELETE /session/:id', () => {
    it('should delete a session', async () => {
      const req = makeRequest('https://worker.test/session/sess-del', { method: 'DELETE' });
      const resp = await handler.fetch(req, {}, mockCtx);
      expect(resp.status).toBe(200);
      const body = await resp.json() as Record<string, unknown>;
      expect(body.deleted).toBe(true);
      expect(kv.delete).toHaveBeenCalledWith('session:sess-del');
    });
  });

  describe('OPTIONS (CORS preflight)', () => {
    it('should return 204 with CORS headers', async () => {
      const req = makeRequest('https://worker.test/chat', { method: 'OPTIONS' });
      const resp = await handler.fetch(req, {}, mockCtx);
      expect(resp.status).toBe(204);
      expect(resp.headers.get('Access-Control-Allow-Origin')).toBe('*');
    });
  });

  describe('404 for unknown routes', () => {
    it('should return 404', async () => {
      const req = makeRequest('https://worker.test/unknown');
      const resp = await handler.fetch(req, {}, mockCtx);
      expect(resp.status).toBe(404);
    });
  });

  describe('Error handling', () => {
    it('should return 500 on internal error', async () => {
      const badKV = createMockKV();
      (badKV.get as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('KV failure'));
      const adapter2 = new CloudflareAdapter({ ...BASE_CONFIG, kvNamespace: badKV });
      const handler2 = adapter2.createHandler();
      const req = makeRequest('https://worker.test/session/err');
      const resp = await handler2.fetch(req, {}, mockCtx);
      expect(resp.status).toBe(500);
      const body = await resp.json() as Record<string, unknown>;
      expect(body.error).toContain('KV failure');
    });
  });
});

describe('CloudflareAdapter.adaptStorage', () => {
  it('should bridge EdgeStorageAdapter to EdgeStorage', async () => {
    const kv = createMockKV();
    const adapter = new CloudflareAdapter(BASE_CONFIG);
    const storage = adapter.adaptStorage(kv);
    await storage.set('key1', { hello: 'world' });
    expect(kv.put).toHaveBeenCalledWith('key1', JSON.stringify({ hello: 'world' }));
    const val = await storage.get('key1');
    expect(val).toEqual({ hello: 'world' });
    await storage.delete('key1');
    expect(kv.delete).toHaveBeenCalledWith('key1');
  });

  it('should return null for missing keys', async () => {
    const kv = createMockKV();
    const adapter = new CloudflareAdapter(BASE_CONFIG);
    const storage = adapter.adaptStorage(kv);
    expect(await storage.get('missing')).toBeNull();
  });

  it('should list with prefix and parse values', async () => {
    const kv = createMockKV();
    await kv.put('p:1', JSON.stringify({ a: 1 }));
    await kv.put('p:2', JSON.stringify({ b: 2 }));
    const adapter = new CloudflareAdapter(BASE_CONFIG);
    const storage = adapter.adaptStorage(kv);
    const entries = await storage.list('p:');
    expect(entries).toHaveLength(2);
    expect(entries[0]![1]).toEqual({ a: 1 });
  });
});

describe('MemoryKVEdgeAdapter fallback', () => {
  it('should use in-memory storage when no KV provided', async () => {
    const adapter = new CloudflareAdapter({ name: 'mem-worker' });
    const handler = adapter.createHandler();
    const req = makeRequest('https://worker.test/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: 'test' }),
    });
    const resp = await handler.fetch(req, {}, mockCtx);
    expect(resp.status).toBe(200);
  });
});

describe('createWorkerHandler', () => {
  it('should return a handler with fetch method', () => {
    const h = createWorkerHandler({ name: 'quick-worker' });
    expect(typeof h.fetch).toBe('function');
  });

  it('should handle health check', async () => {
    const h = createWorkerHandler({ name: 'quick-worker' });
    const req = makeRequest('https://worker.test/health');
    const resp = await h.fetch(req, {}, mockCtx);
    expect(resp.status).toBe(200);
    const body = await resp.json() as Record<string, unknown>;
    expect(body.name).toBe('quick-worker');
  });
});

describe('createHandler with config override', () => {
  it('should merge config overrides', async () => {
    const kv = createMockKV();
    const adapter = new CloudflareAdapter({ name: 'base' });
    const handler = adapter.createHandler({ name: 'overridden', kvNamespace: kv });
    const req = makeRequest('https://worker.test/health');
    const resp = await handler.fetch(req, {}, mockCtx);
    const body = await resp.json() as Record<string, unknown>;
    expect(body.name).toBe('overridden');
  });
});
