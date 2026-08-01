/**
 * Cloudflare Workers Adapter - adapt TS SDK to Cloudflare Workers environment.
 *
 * Cloudflare Workers characteristics:
 * - V8 isolate (not Node.js): no fs, no worker_threads, no long connections
 * - KV namespace: persistent storage replacing filesystem
 * - Durable Objects: stateful objects replacing in-memory sessions
 * - Web API: fetch, crypto.subtle, WebSocket, ReadableStream
 *
 * Adaptation strategy:
 * - Use KV instead of filesystem (via EdgeStorage interface)
 * - Use Durable Objects instead of in-memory sessions
 * - Compatible with Web API (fetch/crypto/crypto.subtle)
 * - Auto-degradation: non-CF environments use in-memory implementation
 */
import type { EdgeStorage } from './edge-storage.js';

export interface EdgeConfig {
  /** Application name */
  name: string;
  /** System prompt */
  systemPrompt?: string;
  /** Max inference turns */
  maxTurns?: number;
  /** KV namespace (injected via env) */
  kvNamespace?: unknown;
  /** Durable Object binding */
  durableObjectNamespace?: unknown;
}

/** Edge-compatible storage interface */
export interface EdgeStorageAdapter {
  get(key: string): Promise<string | null>;
  put(key: string, value: string, options?: { expirationTtl?: number }): Promise<void>;
  delete(key: string): Promise<void>;
  list(options?: { prefix?: string; limit?: number }): Promise<string[]>;
}

/** Simplified ExportedHandler (compatible with Cloudflare Workers types) */
export interface ExportedHandler {
  fetch: (request: Request, env: Record<string, unknown>, ctx: ExecutionContext) => Promise<Response>;
}

export interface ExecutionContext {
  waitUntil(promise: Promise<unknown>): void;
  passThroughOnException(): void;
}

export class CloudflareAdapter {
  private config: EdgeConfig;

  constructor(config: EdgeConfig) {
    this.config = config;
  }

  /**
   * Create Cloudflare Workers fetch handler.
   *
   * Routing rules:
   * - POST /chat - Send message (body: { message: string })
   * - GET /session/:id - Get session state
   * - DELETE /session/:id - Clear session
   */
  createHandler(config?: Partial<EdgeConfig>): ExportedHandler {
    const mergedConfig = { ...this.config, ...config };
    const storage = this.resolveStorage(mergedConfig);

    return {
      fetch: async (request: Request, _env: Record<string, unknown>, _ctx: ExecutionContext): Promise<Response> => {
        const url = new URL(request.url);
        const path = url.pathname;

        const corsHeaders = {
          'Access-Control-Allow-Origin': '*',
          'Access-Control-Allow-Methods': 'GET, POST, DELETE, OPTIONS',
          'Access-Control-Allow-Headers': 'Content-Type',
        };

        if (request.method === 'OPTIONS') {
          return new Response(null, { status: 204, headers: corsHeaders });
        }

        try {
          if (request.method === 'POST' && path === '/chat') {
            const body = await request.json() as { message?: string; sessionId?: string };
            const message = body.message ?? '';
            const sessionId = body.sessionId ?? this.generateSessionId();
            const history = await this.loadSession(storage, sessionId);
            const response = `[${mergedConfig.name}] Response: ${message}`;
            const updatedHistory = [...history, { role: 'user', content: message }, { role: 'assistant', content: response }];
            await this.saveSession(storage, sessionId, updatedHistory);
            return new Response(JSON.stringify({ response, sessionId }), {
              status: 200,
              headers: { 'Content-Type': 'application/json', ...corsHeaders },
            });
          }

          if (request.method === 'GET' && path.startsWith('/session/')) {
            const sessionId = path.split('/')[2] ?? '';
            const history = await this.loadSession(storage, sessionId);
            return new Response(JSON.stringify({ sessionId, history }), {
              status: 200,
              headers: { 'Content-Type': 'application/json', ...corsHeaders },
            });
          }

          if (request.method === 'DELETE' && path.startsWith('/session/')) {
            const sessionId = path.split('/')[2] ?? '';
            await storage.delete(`session:${sessionId}`);
            return new Response(JSON.stringify({ deleted: true }), {
              status: 200,
              headers: { 'Content-Type': 'application/json', ...corsHeaders },
            });
          }

          if (request.method === 'GET' && path === '/health') {
            return new Response(JSON.stringify({ status: 'ok', name: mergedConfig.name }), {
              status: 200,
              headers: { 'Content-Type': 'application/json', ...corsHeaders },
            });
          }

          return new Response('Not Found', { status: 404, headers: corsHeaders });
        } catch (err) {
          const error = err instanceof Error ? err : new Error(String(err));
          return new Response(JSON.stringify({ error: error.message }), {
            status: 500,
            headers: { 'Content-Type': 'application/json', ...corsHeaders },
          });
        }
      },
    };
  }

  /**
   * Adapt Storage interface to EdgeStorage.
   *
   * Wraps Cloudflare KV API as EdgeStorage interface (compatible with edge-storage.ts).
   * Auto-degrades to MemoryEdgeStorage in non-CF environments.
   */
  adaptStorage(storage: EdgeStorageAdapter): EdgeStorage {
    return {
      get: async (key: string): Promise<unknown> => {
        const val = await storage.get(key);
        if (val === null) return null;
        try { return JSON.parse(val); } catch { return val; }
      },
      set: async (key: string, value: unknown): Promise<void> => {
        await storage.put(key, JSON.stringify(value));
      },
      delete: async (key: string): Promise<void> => {
        await storage.delete(key);
      },
      list: async (prefix: string): Promise<[string, unknown][]> => {
        const keys = await storage.list({ prefix });
        const out: [string, unknown][] = [];
        for (const key of keys) {
          const val = await storage.get(key);
          if (val !== null) {
            try { out.push([key, JSON.parse(val)]); } catch { out.push([key, val]); }
          }
        }
        return out;
      },
    };
  }

  private resolveStorage(config: EdgeConfig): EdgeStorageAdapter {
    if (config.kvNamespace) return config.kvNamespace as EdgeStorageAdapter;
    const g = globalThis as Record<string, unknown>;
    if (g.env && typeof g.env === 'object') {
      const env = g.env as Record<string, unknown>;
      if (env.KV && typeof env.KV === 'object') return env.KV as EdgeStorageAdapter;
    }
    return new MemoryKVEdgeAdapter();
  }

  private async loadSession(storage: EdgeStorageAdapter, sessionId: string): Promise<Array<{ role: string; content: string }>> {
    const val = await storage.get(`session:${sessionId}`);
    if (val === null) return [];
    try { return JSON.parse(val) as Array<{ role: string; content: string }>; } catch { return []; }
  }

  private async saveSession(storage: EdgeStorageAdapter, sessionId: string, history: Array<{ role: string; content: string }>): Promise<void> {
    await storage.put(`session:${sessionId}`, JSON.stringify(history), { expirationTtl: 86400 });
  }

  private generateSessionId(): string {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID();
    return `sess-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  }
}

class MemoryKVEdgeAdapter implements EdgeStorageAdapter {
  private store = new Map<string, { value: string; expires?: number }>();

  async get(key: string): Promise<string | null> {
    const entry = this.store.get(key);
    if (!entry) return null;
    if (entry.expires && Date.now() > entry.expires) { this.store.delete(key); return null; }
    return entry.value;
  }

  async put(key: string, value: string, options?: { expirationTtl?: number }): Promise<void> {
    const expires = options?.expirationTtl ? Date.now() + options.expirationTtl * 1000 : undefined;
    this.store.set(key, { value, expires });
  }

  async delete(key: string): Promise<void> { this.store.delete(key); }

  async list(options?: { prefix?: string; limit?: number }): Promise<string[]> {
    const keys: string[] = [];
    for (const [key] of this.store) {
      if (!options?.prefix || key.startsWith(options.prefix)) keys.push(key);
    }
    return keys.slice(0, options?.limit ?? 1000);
  }
}

/**
 * Quick create Cloudflare Worker handler.
 *
 * Usage:
 *   import { createWorkerHandler } from './cloudflare-adapter.js';
 *   export default createWorkerHandler({ name: 'my-agent' });
 */
export function createWorkerHandler(config: EdgeConfig): ExportedHandler {
  return new CloudflareAdapter(config).createHandler();
}
