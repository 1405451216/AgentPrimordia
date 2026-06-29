import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockProvider } from '../../src/llm/provider.js';
import { APIError } from '../../src/llm/openai.js';
import { ResilientProvider } from '../../src/llm/resilient.js';
import { StructuredOutputExtractor } from '../../src/llm/structured-output.js';
import {
  BatchRequestProcessor,
  defaultBatchConfig,
} from '../../src/llm/batch.js';
import {
  InMemoryCache,
  FingerprintCache,
  CachedProvider,
  StructuredExtractor,
  schemaFromStruct,
  RateLimiter,
  BatchProcessor,
  SentimentSchema,
  ClassificationSchema,
  SummarySchema,
  NERSchema,
  type LLMCache,
  type CacheEntry,
} from '../../src/llm/cache-structured.js';
import {
  MultimodalAdapter,
  OpenAIMultimodalProvider,
  textContent,
  imageUrlContent,
  imageB64Content,
  audioContent,
  videoContent,
} from '../../src/llm/multimodal.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse } from '../../src/types.js';

const sampleMessages = [{ role: 'user' as const, content: 'Hello' }];

// ===== ResilientProvider Tests =====

describe('ResilientProvider', () => {
  it('should return result from primary on success', async () => {
    const primary = new MockProvider({ response: 'primary reply' });
    const resilient = new ResilientProvider(primary, { maxRetries: 2, retryBackoff: 10 });
    const resp = await resilient.complete({ messages: sampleMessages });
    expect(resp.content).toBe('primary reply');
  });

  it('should retry on retryable error and succeed', async () => {
    let calls = 0;
    const primary = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls < 3) throw new APIError('server error', '', '', 500);
        return { id: '1', content: 'ok', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const resilient = new ResilientProvider(primary as any, { maxRetries: 3, retryBackoff: 10 });
    const resp = await resilient.complete({ messages: sampleMessages });
    expect(resp.content).toBe('ok');
    expect(calls).toBe(3);
  });

  it('should not retry on non-retryable error (4xx)', async () => {
    let calls = 0;
    const primary = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        throw new APIError('bad request', '', '', 400);
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const resilient = new ResilientProvider(primary as any, { maxRetries: 3, retryBackoff: 10 });
    await expect(resilient.complete({ messages: sampleMessages })).rejects.toThrow('bad request');
    expect(calls).toBe(1);
  });

  it('should retry on 429 (rate limit)', async () => {
    let calls = 0;
    const primary = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls < 2) throw new APIError('rate limited', '', '', 429);
        return { id: '1', content: 'ok', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const resilient = new ResilientProvider(primary as any, { maxRetries: 3, retryBackoff: 10 });
    const resp = await resilient.complete({ messages: sampleMessages });
    expect(resp.content).toBe('ok');
    expect(calls).toBe(2);
  });

  it('should retry on 408 (timeout)', async () => {
    let calls = 0;
    const primary = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls < 2) throw new APIError('timeout', '', '', 408);
        return { id: '1', content: 'ok', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const resilient = new ResilientProvider(primary as any, { maxRetries: 3, retryBackoff: 10 });
    const resp = await resilient.complete({ messages: sampleMessages });
    expect(resp.content).toBe('ok');
  });

  it('should use fallback when primary fails', async () => {
    const primary = new MockProvider({ error: true });
    const fallback = new MockProvider({ response: 'fallback reply' });
    const resilient = new ResilientProvider(primary, { maxRetries: 1, retryBackoff: 10 });
    resilient.addFallback(fallback);
    const resp = await resilient.complete({ messages: sampleMessages });
    expect(resp.content).toBe('fallback reply');
  });

  it('should throw when all providers fail', async () => {
    const primary = new MockProvider({ error: true });
    const fallback = new MockProvider({ error: true });
    const resilient = new ResilientProvider(primary, { maxRetries: 1, retryBackoff: 10 });
    resilient.addFallback(fallback);
    await expect(resilient.complete({ messages: sampleMessages })).rejects.toThrow('mock error');
  });

  it('should call tools with retry', async () => {
    let calls = 0;
    const primary = {
      complete: vi.fn(),
      callTools: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls < 2) throw new APIError('server error', '', '', 500);
        return { content: 'ok', toolCalls: [], usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const resilient = new ResilientProvider(primary as any, { maxRetries: 3, retryBackoff: 10 });
    const resp = await resilient.callTools({ messages: sampleMessages, tools: [] });
    expect(resp.content).toBe('ok');
    expect(calls).toBe(2);
  });

  it('should open circuit breaker after threshold failures', async () => {
    const primary = new MockProvider({ error: true });
    const resilient = new ResilientProvider(primary, {
      maxRetries: 0,
      retryBackoff: 1,
      circuitThreshold: 3,
      circuitRecoverAfter: 100,
    });

    // First 3 calls should fail with mock error
    for (let i = 0; i < 3; i++) {
      await expect(resilient.complete({ messages: sampleMessages })).rejects.toThrow('mock error');
    }

    // 4th call should fail with circuit breaker open
    await expect(resilient.complete({ messages: sampleMessages })).rejects.toThrow('circuit breaker is open');
  });

  it('should recover circuit breaker after timeout', async () => {
    const primary = new MockProvider({ error: true });
    const resilient = new ResilientProvider(primary, {
      maxRetries: 0,
      retryBackoff: 1,
      circuitThreshold: 2,
      circuitRecoverAfter: 50,
    });

    // Trip the circuit
    for (let i = 0; i < 2; i++) {
      await expect(resilient.complete({ messages: sampleMessages })).rejects.toThrow();
    }
    // Circuit should be open
    await expect(resilient.complete({ messages: sampleMessages })).rejects.toThrow('circuit breaker is open');

    // Wait for recovery
    await new Promise((r) => setTimeout(r, 100));

    // Should allow attempt again (will still fail, but with mock error, not circuit open)
    await expect(resilient.complete({ messages: sampleMessages })).rejects.toThrow('mock error');
  });

  it('should return primary info', () => {
    const primary = new MockProvider();
    const resilient = new ResilientProvider(primary);
    const info = resilient.info();
    expect(info.name).toBe('mock-model');
  });

  it('should handle network errors (retryable)', async () => {
    let calls = 0;
    const primary = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls < 2) throw new Error('network error');
        return { id: '1', content: 'ok', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const resilient = new ResilientProvider(primary as any, { maxRetries: 3, retryBackoff: 10 });
    const resp = await resilient.complete({ messages: sampleMessages });
    expect(resp.content).toBe('ok');
  });
});

// ===== StructuredOutputExtractor Tests =====

describe('StructuredOutputExtractor', () => {
  it('should extract structured data', async () => {
    const provider = new MockProvider({ response: '{"name":"John","age":30}' });
    const extractor = new StructuredOutputExtractor(provider, 'gpt-4');
    const result = await extractor.extract<{ name: string; age: number }>(
      'John is 30 years old',
      { name: 'person', schema: { type: 'object', properties: { name: { type: 'string' }, age: { type: 'number' } } } }
    );
    expect(result.name).toBe('John');
    expect(result.age).toBe(30);
  });

  it('should throw when provider is null', () => {
    expect(() => new StructuredOutputExtractor(null as any, 'gpt-4')).toThrow('provider must not be nil');
  });

  it('should throw when schema is null', async () => {
    const provider = new MockProvider();
    const extractor = new StructuredOutputExtractor(provider, 'gpt-4');
    await expect(extractor.extract('test', null as any)).rejects.toThrow('schema must not be nil');
  });

  it('should retry on invalid JSON', async () => {
    let calls = 0;
    const provider = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls === 1) return { id: '1', content: 'not json', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
        return { id: '2', content: '{"ok":true}', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const extractor = new StructuredOutputExtractor(provider as any, 'gpt-4', { maxRetries: 2 });
    const result = await extractor.extract('test', { name: 'test', schema: { type: 'object' } });
    expect(result).toEqual({ ok: true });
    expect(calls).toBe(2);
  });

  it('should throw after exhausting retries on provider error', async () => {
    const provider = new MockProvider({ error: true });
    const extractor = new StructuredOutputExtractor(provider, 'gpt-4', { maxRetries: 1 });
    await expect(extractor.extract('test', { name: 'test', schema: {} })).rejects.toThrow('结构化提取失败');
  });

  it('should throw after exhausting retries on invalid JSON', async () => {
    const provider = new MockProvider({ response: 'always not json' });
    const extractor = new StructuredOutputExtractor(provider, 'gpt-4', { maxRetries: 1 });
    await expect(extractor.extract('test', { name: 'test', schema: {} })).rejects.toThrow('结构化提取失败');
  });

  it('should use extractInto as alias', async () => {
    const provider = new MockProvider({ response: '{"value":42}' });
    const extractor = new StructuredOutputExtractor(provider, 'gpt-4');
    const result = await extractor.extractInto<{ value: number }>('test', { name: 'test', schema: {} });
    expect(result.value).toBe(42);
  });
});

// ===== BatchRequestProcessor Tests =====

describe('BatchRequestProcessor', () => {
  it('should return default config', () => {
    const cfg = defaultBatchConfig();
    expect(cfg.maxBatchSize).toBe(10);
    expect(cfg.flushTimeout).toBe(100);
  });

  it('should batch and process requests', async () => {
    const provider = new MockProvider({ response: 'batch reply', delay: 10 });
    const bp = new BatchRequestProcessor(provider, { maxBatchSize: 2, flushTimeout: 50 });

    const p1 = bp.complete({ messages: sampleMessages });
    const p2 = bp.complete({ messages: sampleMessages });

    const [r1, r2] = await Promise.all([p1, p2]);
    expect(r1.content).toBe('batch reply');
    expect(r2.content).toBe('batch reply');

    await bp.close();
  });

  it('should flush on timeout', async () => {
    const provider = new MockProvider({ response: 'timeout reply' });
    const bp = new BatchRequestProcessor(provider, { maxBatchSize: 10, flushTimeout: 30 });

    const p = bp.complete({ messages: sampleMessages });
    const r = await p;
    expect(r.content).toBe('timeout reply');

    await bp.close();
  });

  it('should delegate callTools to inner provider', async () => {
    const provider = new MockProvider({ response: 'tools reply', toolCalls: [{ id: '1', name: 'test', arguments: '{}' }] });
    const bp = new BatchRequestProcessor(provider);
    const resp = await bp.callTools({ messages: sampleMessages, tools: [] });
    expect(resp.content).toBe('tools reply');
    await bp.close();
  });

  it('should return inner provider info', () => {
    const provider = new MockProvider();
    const bp = new BatchRequestProcessor(provider);
    expect(bp.info().name).toBe('mock-model');
    bp.close();
  });

  it('should throw when complete called after close', async () => {
    const provider = new MockProvider();
    const bp = new BatchRequestProcessor(provider, { flushTimeout: 1000 });
    await bp.close();
    await expect(bp.complete({ messages: sampleMessages })).rejects.toThrow('batch processor is closed');
  });

  it('should close without error when already closed', async () => {
    const provider = new MockProvider();
    const bp = new BatchRequestProcessor(provider, { flushTimeout: 1000 });
    await bp.close();
    await expect(bp.close()).resolves.not.toThrow();
  });

  it('should pass through stream', async () => {
    const provider = new MockProvider({ response: 'stream test' });
    const bp = new BatchRequestProcessor(provider);
    expect(bp.stream).toBeDefined();
    const chunks = [];
    for await (const chunk of bp.stream!({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThan(0);
    await bp.close();
  });
});

// ===== InMemoryCache Tests =====

describe('InMemoryCache', () => {
  it('should store and retrieve entries', async () => {
    const cache = new InMemoryCache();
    const entry: CacheEntry = {
      content: 'test',
      response: { id: '1', content: 'test', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } },
      timestamp: Date.now(),
    };
    await cache.set('key1', entry);
    const result = await cache.get('key1');
    expect(result).not.toBeNull();
    expect(result!.content).toBe('test');
  });

  it('should return null for missing keys', async () => {
    const cache = new InMemoryCache();
    const result = await cache.get('missing');
    expect(result).toBeNull();
  });

  it('should track stats', async () => {
    const cache = new InMemoryCache();
    await cache.set('k', { content: 'v', response: {} as any, timestamp: 0 });
    await cache.get('k'); // hit
    await cache.get('missing'); // miss
    const stats = cache.stats();
    expect(stats.hits).toBe(1);
    expect(stats.misses).toBe(1);
    expect(stats.hitRate).toBeCloseTo(0.5);
    expect(stats.size).toBe(1);
  });

  it('should evict oldest when at capacity', async () => {
    const cache = new InMemoryCache(2);
    await cache.set('k1', { content: 'v1', response: {} as any, timestamp: 0 });
    await cache.set('k2', { content: 'v2', response: {} as any, timestamp: 0 });
    await cache.set('k3', { content: 'v3', response: {} as any, timestamp: 0 });
    expect(cache.stats().size).toBe(2);
    expect(await cache.get('k1')).toBeNull();
    expect(await cache.get('k3')).not.toBeNull();
  });

  it('should clear all entries', async () => {
    const cache = new InMemoryCache();
    await cache.set('k', { content: 'v', response: {} as any, timestamp: 0 });
    cache.clear();
    expect(cache.stats().size).toBe(0);
    expect(cache.stats().hits).toBe(0);
  });

  it('should invalidate by pattern', async () => {
    const cache = new InMemoryCache();
    await cache.set('user:1', { content: 'v', response: {} as any, timestamp: 0 });
    await cache.set('user:2', { content: 'v', response: {} as any, timestamp: 0 });
    await cache.set('post:1', { content: 'v', response: {} as any, timestamp: 0 });
    cache.invalidate('user:');
    expect(await cache.get('user:1')).toBeNull();
    expect(await cache.get('user:2')).toBeNull();
    expect(await cache.get('post:1')).not.toBeNull();
  });

  it('should invalidate all when no pattern', async () => {
    const cache = new InMemoryCache();
    await cache.set('k1', { content: 'v', response: {} as any, timestamp: 0 });
    await cache.set('k2', { content: 'v', response: {} as any, timestamp: 0 });
    cache.invalidate();
    expect(cache.stats().size).toBe(0);
  });
});

// ===== FingerprintCache Tests =====

describe('FingerprintCache', () => {
  it('should generate fingerprint from messages', () => {
    const fp = FingerprintCache.fingerprint(
      [{ role: 'user', content: 'hello' }],
      'gpt-4'
    );
    expect(fp).toContain('gpt-4');
    expect(fp).toContain('user:hello');
  });

  it('should generate fingerprint without model', () => {
    const fp = FingerprintCache.fingerprint([{ role: 'user', content: 'hi' }]);
    expect(fp).toContain('::');
    expect(fp).toContain('user:hi');
  });

  it('should store and retrieve by fingerprint', async () => {
    const cache = new FingerprintCache();
    const key = FingerprintCache.fingerprint([{ role: 'user', content: 'hello' }], 'gpt-4');
    await cache.set(key, { content: 'reply', response: {} as any, timestamp: 0 });
    const result = await cache.get(key);
    expect(result).not.toBeNull();
    expect(result!.content).toBe('reply');
  });
});

// ===== CachedProvider Tests =====

describe('CachedProvider', () => {
  it('should cache complete results', async () => {
    let calls = 0;
    const inner = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        return { id: '1', content: 'cached reply', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const cache = new InMemoryCache();
    const cached = new CachedProvider(inner as any, cache);

    const r1 = await cached.complete({ messages: sampleMessages });
    const r2 = await cached.complete({ messages: sampleMessages });
    expect(r1.content).toBe('cached reply');
    expect(r2.content).toBe('cached reply');
    expect(calls).toBe(1); // Only called once
  });

  it('should cache callTools results', async () => {
    let calls = 0;
    const inner = {
      complete: vi.fn(),
      callTools: vi.fn().mockImplementation(async () => {
        calls++;
        return { content: 'tools', toolCalls: [], usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const cache = new InMemoryCache();
    const cached = new CachedProvider(inner as any, cache);

    await cached.callTools({ messages: sampleMessages, tools: [] });
    await cached.callTools({ messages: sampleMessages, tools: [] });
    expect(calls).toBe(1);
  });

  it('should not cache streams', async () => {
    const inner = new MockProvider({ response: 'stream reply' });
    const cache = new InMemoryCache();
    const cached = new CachedProvider(inner, cache);

    const chunks = [];
    for await (const chunk of cached.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThan(0);
    // Stream should not be cached
    expect(cache.stats().size).toBe(0);
  });

  it('should fall back to complete for stream when no stream impl', async () => {
    const inner = {
      complete: vi.fn().mockResolvedValue({ id: '1', content: 'no stream', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const cached = new CachedProvider(inner as any, new InMemoryCache());
    const chunks = [];
    for await (const chunk of cached.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks[0].content).toBe('no stream');
    expect(chunks[0].done).toBe(true);
  });

  it('should return inner info', () => {
    const inner = new MockProvider();
    const cached = new CachedProvider(inner, new InMemoryCache());
    expect(cached.info().name).toBe('mock-model');
  });

  it('should expose cache', () => {
    const inner = new MockProvider();
    const cache = new InMemoryCache();
    const cached = new CachedProvider(inner, cache);
    expect(cached.getCache()).toBe(cache);
  });
});

// ===== StructuredExtractor Tests =====

describe('StructuredExtractor', () => {
  it('should extract JSON from direct response', async () => {
    const provider = new MockProvider({ response: '{"name":"test","value":42}' });
    const extractor = new StructuredExtractor({ provider, model: 'gpt-4' });
    const result = await extractor.extract('input', { name: 'test', schema: {} });
    expect(result).toEqual({ name: 'test', value: 42 });
  });

  it('should extract JSON from code block', async () => {
    const provider = new MockProvider({ response: '```json\n{"ok":true}\n```' });
    const extractor = new StructuredExtractor({ provider, model: 'gpt-4' });
    const result = await extractor.extract('input', { name: 'test', schema: {} });
    expect(result).toEqual({ ok: true });
  });

  it('should extract JSON from embedded text', async () => {
    const provider = new MockProvider({ response: 'Here is the result: {"found":true} done' });
    const extractor = new StructuredExtractor({ provider, model: 'gpt-4' });
    const result = await extractor.extract('input', { name: 'test', schema: {} });
    expect(result).toEqual({ found: true });
  });

  it('should extract JSON array from embedded text', async () => {
    const provider = new MockProvider({ response: 'Result: [1, 2, 3] end' });
    const extractor = new StructuredExtractor({ provider, model: 'gpt-4' });
    const result = await extractor.extract('input', { name: 'test', schema: {} });
    expect(result).toEqual([1, 2, 3]);
  });

  it('should retry on extraction failure', async () => {
    let calls = 0;
    const provider = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls < 2) return { id: '1', content: 'not json at all', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
        return { id: '2', content: '{"ok":true}', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const extractor = new StructuredExtractor({ provider: provider as any, model: 'gpt-4', maxRetries: 3 });
    const result = await extractor.extract('input', { name: 'test', schema: {} });
    expect(result).toEqual({ ok: true });
    expect(calls).toBe(2);
  });

  it('should throw after exhausting retries', async () => {
    const provider = new MockProvider({ response: 'not json' });
    const extractor = new StructuredExtractor({ provider, model: 'gpt-4', maxRetries: 2 });
    await expect(extractor.extract('input', { name: 'test', schema: {} })).rejects.toThrow('Could not extract JSON');
  });

  it('should throw on provider error', async () => {
    const provider = new MockProvider({ error: true });
    const extractor = new StructuredExtractor({ provider, model: 'gpt-4', maxRetries: 2 });
    await expect(extractor.extract('input', { name: 'test', schema: {} })).rejects.toThrow('mock error');
  });
});

// ===== Schema Helpers Tests =====

describe('Schema Helpers', () => {
  it('should generate schema from struct', () => {
    const schema = schemaFromStruct('user', {
      name: { type: 'string', description: 'User name' },
      age: { type: 'number' },
      role: { type: 'string', enum: ['admin', 'user'] },
    });
    expect(schema.name).toBe('user');
    expect(schema.strict).toBe(true);
    expect(schema.schema.type).toBe('object');
    const props = schema.schema.properties as Record<string, any>;
    expect(props.name.type).toBe('string');
    expect(props.name.description).toBe('User name');
    expect(props.role.enum).toEqual(['admin', 'user']);
    expect(schema.schema.required).toEqual(['name', 'age', 'role']);
  });

  it('should export predefined schemas', () => {
    expect(SentimentSchema.name).toBe('sentiment');
    expect(ClassificationSchema.name).toBe('classification');
    expect(SummarySchema.name).toBe('summary');
    expect(NERSchema.name).toBe('ner');
  });
});

// ===== RateLimiter Tests =====

describe('RateLimiter', () => {
  it('should acquire tokens', async () => {
    const limiter = new RateLimiter(60);
    await expect(limiter.acquire()).resolves.not.toThrow();
  });

  it('should deplete and wait for tokens', async () => {
    const limiter = new RateLimiter(600); // 10 per second
    await limiter.acquire();
    await limiter.acquire();
    // Third acquire should still work quickly with high rate
    await expect(limiter.acquire()).resolves.not.toThrow();
  });
});

// ===== BatchProcessor Tests =====

describe('BatchProcessor', () => {
  it('should process requests concurrently', async () => {
    const provider = new MockProvider({ response: 'batch', delay: 10 });
    const bp = new BatchProcessor(provider, { maxConcurrent: 3 });

    const requests = [
      { id: '1', messages: sampleMessages },
      { id: '2', messages: sampleMessages },
      { id: '3', messages: sampleMessages },
    ];

    const results = await bp.process(requests);
    expect(results).toHaveLength(3);
    for (const r of results) {
      expect(r.response).toBeDefined();
      expect(r.response!.content).toBe('batch');
    }
  });

  it('should handle errors in batch', async () => {
    let calls = 0;
    const provider = {
      complete: vi.fn().mockImplementation(async () => {
        calls++;
        if (calls === 2) throw new Error('batch error');
        return { id: '1', content: 'ok', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const bp = new BatchProcessor(provider as any, { maxConcurrent: 1 });
    const results = await bp.process([
      { id: '1', messages: sampleMessages },
      { id: '2', messages: sampleMessages },
      { id: '3', messages: sampleMessages },
    ]);

    expect(results).toHaveLength(3);
    expect(results[1].error).toBeDefined();
    expect(results[1].error!.message).toBe('batch error');
    expect(results[0].response).toBeDefined();
    expect(results[2].response).toBeDefined();
  });

  it('should use rate limiter', async () => {
    const provider = new MockProvider({ response: 'limited' });
    const rateLimiter = new RateLimiter(100);
    const bp = new BatchProcessor(provider, { maxConcurrent: 2, rateLimiter });

    const results = await bp.process([
      { id: '1', messages: sampleMessages },
      { id: '2', messages: sampleMessages },
    ]);
    expect(results).toHaveLength(2);
  });

  it('should handle empty request list', async () => {
    const provider = new MockProvider();
    const bp = new BatchProcessor(provider);
    const results = await bp.process([]);
    expect(results).toHaveLength(0);
  });
});

// ===== Multimodal Tests =====

describe('Multimodal Content Builders', () => {
  it('should build text content', () => {
    const content = textContent('hello');
    expect(content.type).toBe('text');
    expect(content.text).toBe('hello');
  });

  it('should build image URL content', () => {
    const content = imageUrlContent('http://example.com/img.png');
    expect(content.type).toBe('image_url');
    expect(content.imageUrl).toBe('http://example.com/img.png');
  });

  it('should build image b64 content', () => {
    const content = imageB64Content('base64data', 'image/jpeg');
    expect(content.type).toBe('image_b64');
    expect(content.imageB64).toBe('base64data');
    expect(content.mimeType).toBe('image/jpeg');
  });

  it('should build image b64 content with default mime', () => {
    const content = imageB64Content('base64data');
    expect(content.mimeType).toBe('image/png');
  });

  it('should build audio content', () => {
    const content = audioContent('audiodata');
    expect(content.type).toBe('audio');
    expect(content.audioData).toBe('audiodata');
    expect(content.mimeType).toBe('audio/wav');
  });

  it('should build video content', () => {
    const content = videoContent('videodata');
    expect(content.type).toBe('video');
    expect(content.videoData).toBe('videodata');
    expect(content.mimeType).toBe('video/mp4');
  });
});

describe('MultimodalAdapter', () => {
  it('should adapt provider with default capabilities', () => {
    const inner = new MockProvider();
    const adapter = new MultimodalAdapter(inner);
    expect(adapter.capabilities).toEqual(['text', 'vision']);
  });

  it('should adapt provider with custom capabilities', () => {
    const inner = new MockProvider();
    const adapter = new MultimodalAdapter(inner, ['text', 'audio']);
    expect(adapter.capabilities).toContain('audio');
  });

  it('should complete multimodal request', async () => {
    const inner = new MockProvider({ response: 'multimodal reply' });
    const adapter = new MultimodalAdapter(inner);

    const resp = await adapter.completeMultimodal({
      messages: [
        {
          role: 'user',
          content: [
            textContent('Hello'),
            imageUrlContent('http://example.com/img.png'),
          ],
        },
      ],
    });

    expect(resp.content).toBe('multimodal reply');
  });

  it('should delegate complete to inner', async () => {
    const inner = new MockProvider({ response: 'direct' });
    const adapter = new MultimodalAdapter(inner);
    const resp = await adapter.complete({ messages: sampleMessages });
    expect(resp.content).toBe('direct');
  });

  it('should delegate stream to inner', async () => {
    const inner = new MockProvider({ response: 'stream test' });
    const adapter = new MultimodalAdapter(inner);
    const chunks = [];
    for await (const chunk of adapter.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThan(0);
  });

  it('should fall back to complete for stream when inner has no stream', async () => {
    const inner = {
      complete: vi.fn().mockResolvedValue({ id: '1', content: 'no stream', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };

    const adapter = new MultimodalAdapter(inner as any);
    const chunks = [];
    for await (const chunk of adapter.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks[0].content).toBe('no stream');
  });

  it('should delegate callTools to inner', async () => {
    const inner = new MockProvider({ response: 'tools' });
    const adapter = new MultimodalAdapter(inner);
    const resp = await adapter.callTools({ messages: sampleMessages, tools: [] });
    expect(resp.content).toBe('tools');
  });

  it('should return inner info', () => {
    const inner = new MockProvider();
    const adapter = new MultimodalAdapter(inner);
    expect(adapter.info().name).toBe('mock-model');
  });
});

describe('OpenAIMultimodalProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct', () => {
    const provider = new OpenAIMultimodalProvider({ apiKey: 'sk-test-key-1234567890' });
    expect(provider.capabilities).toContain('vision');
    expect(provider.capabilities).toContain('text');
  });

  it('should complete multimodal request with text and image', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({
          id: 'mm-1',
          choices: [{ message: { role: 'assistant', content: 'I see an image' } }],
          usage: { prompt_tokens: 50, completion_tokens: 10, total_tokens: 60 },
        }),
        json: async () => ({}),
        body: null,
      } as Response
    );

    const provider = new OpenAIMultimodalProvider({ apiKey: 'sk-test-key-1234567890' });
    const resp = await provider.completeMultimodal({
      messages: [
        {
          role: 'user',
          content: [
            textContent('What is this?'),
            imageUrlContent('http://example.com/img.png'),
          ],
        },
      ],
    });

    expect(resp.content).toBe('I see an image');
    expect(resp.usage.totalTokens).toBe(60);
  });

  it('should handle b64 image content', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({
          id: 'mm-2',
          choices: [{ message: { role: 'assistant', content: 'b64 image' } }],
          usage: { prompt_tokens: 30, completion_tokens: 5, total_tokens: 35 },
        }),
        json: async () => ({}),
        body: null,
      } as Response
    );

    const provider = new OpenAIMultimodalProvider({ apiKey: 'sk-test-key-1234567890' });
    const resp = await provider.completeMultimodal({
      messages: [
        {
          role: 'user',
          content: [
            textContent('Analyze'),
            imageB64Content('aGVsbG8=', 'image/png'),
          ],
        },
      ],
    });

    expect(resp.content).toBe('b64 image');
  });

  it('should handle API error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      {
        ok: false,
        status: 400,
        text: async () => 'Bad request',
        json: async () => ({}),
        body: null,
      } as Response
    );

    const provider = new OpenAIMultimodalProvider({ apiKey: 'sk-test-key-1234567890' });
    await expect(
      provider.completeMultimodal({
        messages: [{ role: 'user', content: [textContent('hi')] }],
      })
    ).rejects.toThrow('OpenAI multimodal error');
  });
});
