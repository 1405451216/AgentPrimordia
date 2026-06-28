import type { Provider } from './provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo } from '../types.js';

// ===== LLM Cache =====

export interface CacheStats {
  hits: number;
  misses: number;
  hitRate: number;
  size: number;
}

export interface CacheEntry {
  content: string;
  response: CompletionResponse | ToolCallResponse;
  timestamp: number;
  embedding?: number[];
}

export interface LLMCache {
  get(key: string): Promise<CacheEntry | null>;
  set(key: string, entry: CacheEntry): Promise<void>;
  stats(): CacheStats;
  clear(): void;
  invalidate(pattern?: string): void;
}

// ===== In-Memory Cache (exact match) =====

export class InMemoryCache implements LLMCache {
  private store: Map<string, CacheEntry> = new Map();
  private hits = 0;
  private misses = 0;
  private maxSize: number;

  constructor(maxSize: number = 1000) {
    this.maxSize = maxSize;
  }

  async get(key: string): Promise<CacheEntry | null> {
    const entry = this.store.get(key);
    if (entry) { this.hits++; return entry; }
    this.misses++;
    return null;
  }

  async set(key: string, entry: CacheEntry): Promise<void> {
    if (this.store.size >= this.maxSize) {
      // Evict oldest entry
      const oldest = this.store.keys().next().value;
      if (oldest) this.store.delete(oldest);
    }
    this.store.set(key, entry);
  }

  stats(): CacheStats {
    const total = this.hits + this.misses;
    return {
      hits: this.hits,
      misses: this.misses,
      hitRate: total > 0 ? this.hits / total : 0,
      size: this.store.size,
    };
  }

  clear(): void {
    this.store.clear();
    this.hits = 0;
    this.misses = 0;
  }

  invalidate(pattern?: string): void {
    if (!pattern) { this.clear(); return; }
    for (const key of this.store.keys()) {
      if (key.includes(pattern)) this.store.delete(key);
    }
  }
}

// ===== Fingerprint Cache =====
// Generates a hash key from request messages for exact matching

export class FingerprintCache implements LLMCache {
  private inner: InMemoryCache;

  constructor(maxSize: number = 1000) {
    this.inner = new InMemoryCache(maxSize);
  }

  static fingerprint(messages: import('../types.js').Message[], model?: string): string {
    const parts = messages.map((m) => `${m.role}:${m.content}`).join('|');
    return `${model ?? ''}::${parts}`;
  }

  async get(key: string): Promise<CacheEntry | null> { return this.inner.get(key); }
  async set(key: string, entry: CacheEntry): Promise<void> { return this.inner.set(key, entry); }
  stats(): CacheStats { return this.inner.stats(); }
  clear(): void { this.inner.clear(); }
  invalidate(pattern?: string): void { this.inner.invalidate(pattern); }
}

// ===== Cached Provider =====
// Wraps any provider with caching

export class CachedProvider implements Provider {
  private inner: Provider;
  private cache: LLMCache;

  constructor(inner: Provider, cache: LLMCache) {
    this.inner = inner;
    this.cache = cache;
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const key = FingerprintCache.fingerprint(req.messages, req.model);
    const cached = await this.cache.get(key);
    if (cached && cached.response && 'id' in cached.response) {
      return cached.response as CompletionResponse;
    }
    const resp = await this.inner.complete(req);
    await this.cache.set(key, { content: resp.content, response: resp, timestamp: Date.now() });
    return resp;
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    // Don't cache streams
    if (this.inner.stream) {
      yield* this.inner.stream(req);
    } else {
      const resp = await this.inner.complete(req);
      yield { content: resp.content, done: true, usage: resp.usage };
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const key = 'tools::' + FingerprintCache.fingerprint(req.messages, req.model);
    const cached = await this.cache.get(key);
    if (cached && cached.response && 'toolCalls' in cached.response) {
      return cached.response as ToolCallResponse;
    }
    const resp = await this.inner.callTools(req);
    await this.cache.set(key, { content: resp.content, response: resp, timestamp: Date.now() });
    return resp;
  }

  info(): ModelInfo {
    return this.inner.info();
  }

  getCache(): LLMCache {
    return this.cache;
  }
}

// ===== Structured Output =====

export interface SchemaDef {
  name: string;
  description?: string;
  schema: Record<string, unknown>;
  strict?: boolean;
}

export interface ExtractorConfig {
  provider: Provider;
  model?: string;
  temperature?: number;
  maxRetries?: number;
}

export class StructuredExtractor {
  private config: ExtractorConfig;

  constructor(config: ExtractorConfig) {
    this.config = config;
  }

  /** Extract structured data from text using a JSON schema. */
  async extract<T = unknown>(input: string, schema: SchemaDef): Promise<T> {
    const systemPrompt = `You are a data extraction assistant. Extract structured data according to the following JSON schema. Return ONLY valid JSON, no explanation.\n\nSchema: ${JSON.stringify(schema.schema, null, 2)}`;

    const retries = this.config.maxRetries ?? 3;
    let lastError: Error | null = null;

    for (let attempt = 0; attempt < retries; attempt++) {
      try {
        const resp = await this.config.provider.complete({
          messages: [
            { role: 'system', content: systemPrompt },
            { role: 'user', content: input },
          ],
          model: this.config.model,
          temperature: this.config.temperature ?? 0,
        });

        // Try to parse JSON from response
        const json = this.extractJSON(resp.content);
        return json as T;
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));
      }
    }

    throw lastError ?? new Error('Structured extraction failed');
  }

  private extractJSON(text: string): unknown {
    // Try direct parse first
    try { return JSON.parse(text); } catch {}

    // Try to find JSON in code blocks
    const codeBlockMatch = text.match(/```(?:json)?\s*([\s\S]*?)```/);
    if (codeBlockMatch) {
      try { return JSON.parse(codeBlockMatch[1].trim()); } catch {}
    }

    // Try to find first { and last }
    const firstBrace = text.indexOf('{');
    const lastBrace = text.lastIndexOf('}');
    if (firstBrace !== -1 && lastBrace !== -1 && lastBrace > firstBrace) {
      try { return JSON.parse(text.slice(firstBrace, lastBrace + 1)); } catch {}
    }

    // Try to find first [ and last ]
    const firstBracket = text.indexOf('[');
    const lastBracket = text.lastIndexOf(']');
    if (firstBracket !== -1 && lastBracket !== -1 && lastBracket > firstBracket) {
      try { return JSON.parse(text.slice(firstBracket, lastBracket + 1)); } catch {}
    }

    throw new Error('Could not extract JSON from response');
  }
}

/** Generate a JSON Schema from a TypeScript-like type description. */
export function schemaFromStruct(name: string, properties: Record<string, { type: string; description?: string; enum?: string[] }>): SchemaDef {
  const schema: Record<string, unknown> = {
    type: 'object',
    properties: {},
    required: Object.keys(properties),
  };

  for (const [key, prop] of Object.entries(properties)) {
    (schema.properties as Record<string, unknown>)[key] = {
      type: prop.type,
      ...(prop.description ? { description: prop.description } : {}),
      ...(prop.enum ? { enum: prop.enum } : {}),
    };
  }

  return { name, schema, strict: true };
}

// ===== Predefined Schemas =====

export const SentimentSchema: SchemaDef = {
  name: 'sentiment',
  schema: {
    type: 'object',
    properties: {
      sentiment: { type: 'string', enum: ['positive', 'negative', 'neutral'] },
      confidence: { type: 'number' },
    },
    required: ['sentiment', 'confidence'],
  },
};

export const ClassificationSchema: SchemaDef = {
  name: 'classification',
  schema: {
    type: 'object',
    properties: {
      category: { type: 'string' },
      confidence: { type: 'number' },
      labels: { type: 'array', items: { type: 'string' } },
    },
    required: ['category', 'confidence'],
  },
};

export const SummarySchema: SchemaDef = {
  name: 'summary',
  schema: {
    type: 'object',
    properties: {
      summary: { type: 'string' },
      keyPoints: { type: 'array', items: { type: 'string' } },
    },
    required: ['summary'],
  },
};

export const NERSchema: SchemaDef = {
  name: 'ner',
  schema: {
    type: 'object',
    properties: {
      entities: {
        type: 'array',
        items: {
          type: 'object',
          properties: {
            text: { type: 'string' },
            type: { type: 'string', enum: ['person', 'organization', 'location', 'date', 'other'] },
            startPos: { type: 'number' },
            endPos: { type: 'number' },
          },
          required: ['text', 'type'],
        },
      },
    },
    required: ['entities'],
  },
};

// ===== Rate Limiter =====

export class RateLimiter {
  private tokens: number;
  private maxTokens: number;
  private refillRate: number; // tokens per second
  private lastRefill: number;

  constructor(maxRequestsPerMinute: number = 60) {
    this.maxTokens = maxRequestsPerMinute;
    this.tokens = maxRequestsPerMinute;
    this.refillRate = maxRequestsPerMinute / 60;
    this.lastRefill = Date.now();
  }

  async acquire(): Promise<void> {
    this.refill();
    if (this.tokens >= 1) {
      this.tokens -= 1;
      return;
    }
    // Wait for next token
    const waitMs = Math.ceil((1 - this.tokens) / this.refillRate * 1000);
    await new Promise((r) => setTimeout(r, waitMs));
    this.refill();
    this.tokens -= 1;
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = (now - this.lastRefill) / 1000;
    this.tokens = Math.min(this.maxTokens, this.tokens + elapsed * this.refillRate);
    this.lastRefill = now;
  }
}

// ===== Batch Processing =====

export interface BatchRequest {
  id: string;
  messages: import('../types.js').Message[];
  model?: string;
}

export interface BatchResult {
  id: string;
  response?: CompletionResponse;
  error?: Error;
}

export class BatchProcessor {
  private provider: Provider;
  private maxConcurrent: number;
  private rateLimiter?: RateLimiter;

  constructor(provider: Provider, opts?: { maxConcurrent?: number; rateLimiter?: RateLimiter }) {
    this.provider = provider;
    this.maxConcurrent = opts?.maxConcurrent ?? 5;
    this.rateLimiter = opts?.rateLimiter;
  }

  async process(requests: BatchRequest[]): Promise<BatchResult[]> {
    const results: BatchResult[] = [];
    let nextIndex = 0;

    const worker = async (): Promise<void> => {
      while (nextIndex < requests.length) {
        const currentIndex = nextIndex++;
        const req = requests[currentIndex];
        if (!req) break;

        try {
          if (this.rateLimiter) await this.rateLimiter.acquire();
          const response = await this.provider.complete({
            messages: req.messages,
            model: req.model,
          });
          results.push({ id: req.id, response });
        } catch (err) {
          results.push({ id: req.id, error: err instanceof Error ? err : new Error(String(err)) });
        }
      }
    };

    const workers = Array.from({ length: Math.min(this.maxConcurrent, requests.length) }, () => worker());
    await Promise.all(workers);

    return results;
  }
}
