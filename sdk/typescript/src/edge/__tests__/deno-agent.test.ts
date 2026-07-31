/**
 * deno-agent.ts unit tests
 *
 * Coverage:
 * - DenoEdgeAgent.create() with memory storage fallback
 * - run() / runWithDetails() execution
 * - RateLimiter enforcement
 * - Health status tracking
 * - close() cleanup
 * - Storage error degradation (atomicWrite flag)
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { DenoEdgeAgent, type DenoAgentOptions } from '../deno-agent.js';
import { MemoryEdgeStorage } from '../edge-storage.js';
import type { Provider } from '../../llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../../types.js';

function createMockProvider(overrides?: Partial<Provider>): Provider {
  return {
    complete: vi.fn(async (_req: CompletionRequest): Promise<CompletionResponse> => ({
      id: 'mock-id',
      content: 'mock-response',
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    })),
    callTools: vi.fn(async (_req: ToolCallRequest): Promise<ToolCallResponse> => ({
      content: '',
      toolCalls: [],
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    })),
    info: vi.fn((): ModelInfo => ({
      name: 'mock-model',
      provider: 'mock',
      maxContext: 4096,
      supportsTools: false,
      supportsStreaming: false,
    })),
    ...overrides,
  };
}

function createOpts(overrides?: Partial<DenoAgentOptions>): DenoAgentOptions {
  return {
    name: 'test-deno-agent',
    provider: createMockProvider(),
    storage: new MemoryEdgeStorage(),
    maxTurns: 3,
    requestTimeoutMs: 5000,
    maxRetries: 1,
    retryBaseDelayMs: 10,
    rateLimitPerMinute: 100,
    cleanupIntervalMs: 999999,
    ...overrides,
  };
}

describe('DenoEdgeAgent', () => {
  let agent: DenoEdgeAgent;

  beforeEach(async () => {
    agent = await DenoEdgeAgent.create(createOpts());
  });

  afterEach(() => {
    agent.close();
  });

  describe('create()', () => {
    it('should create an agent with memory storage fallback', async () => {
      expect(agent).toBeDefined();
      expect(agent.storage).toBeInstanceOf(MemoryEdgeStorage);
    });

    it('should use provided storage', async () => {
      const storage = new MemoryEdgeStorage();
      const a = await DenoEdgeAgent.create(createOpts({ storage }));
      expect(a.storage).toBe(storage);
      a.close();
    });
  });

  describe('run()', () => {
    it('should return a string response', async () => {
      const result = await agent.run('hello');
      expect(typeof result).toBe('string');
      expect(result.length).toBeGreaterThan(0);
    });

    it('should store last input/output in storage', async () => {
      await agent.run('test-input');
      const storage = agent.storage as MemoryEdgeStorage;
      expect(await storage.get('last:input')).toBe('test-input');
      expect(await storage.get('last:output')).toBeTruthy();
    });
  });

  describe('runWithDetails()', () => {
    it('should return detailed result', async () => {
      const result = await agent.runWithDetails('hello');
      expect(result.content).toBeTruthy();
      expect(typeof result.durationMs).toBe('number');
      expect(result.durationMs).toBeGreaterThanOrEqual(0);
      expect(typeof result.retries).toBe('number');
      expect(typeof result.atomicWrite).toBe('boolean');
    });

    it('should report retries=0 on success', async () => {
      const result = await agent.runWithDetails('simple');
      expect(result.retries).toBe(0);
      expect(result.content).toBeTruthy();
    });
  });

  describe('RateLimiter', () => {
    it('should throw when rate limit exceeded', async () => {
      const a = await DenoEdgeAgent.create(createOpts({ rateLimitPerMinute: 2 }));
      await a.run('req1');
      await a.run('req2');
      await expect(a.run('req3')).rejects.toThrow('Rate limit');
      a.close();
    });
  });

  describe('getHealth()', () => {
    it('should return health status', () => {
      const health = agent.getHealth();
      expect(health).toHaveProperty('healthy');
      expect(health).toHaveProperty('totalRequests');
      expect(health).toHaveProperty('totalErrors');
      expect(health).toHaveProperty('uptimeMs');
      expect(health).toHaveProperty('kvConnected');
      expect(health.kvConnected).toBe(false);
    });

    it('should track request counts', async () => {
      await agent.run('a');
      await agent.run('b');
      const health = agent.getHealth();
      expect(health.totalRequests).toBe(2);
      expect(health.totalErrors).toBe(0);
      expect(health.healthy).toBe(true);
    });
  });

  describe('getAgent()', () => {
    it('should return the underlying ReActAgent', () => {
      const inner = agent.getAgent();
      expect(inner).toBeDefined();
      expect(inner.name).toBe('test-deno-agent');
    });
  });

  describe('close()', () => {
    it('should clean up timers without error', () => {
      expect(() => agent.close()).not.toThrow();
    });

    it('should be idempotent', () => {
      agent.close();
      expect(() => agent.close()).not.toThrow();
    });
  });

  describe('storage error degradation', () => {
    it('should set atomicWrite=false when storage fails', async () => {
      const failingStorage = new MemoryEdgeStorage();
      vi.spyOn(failingStorage, 'set').mockRejectedValue(new Error('storage down'));
      const a = await DenoEdgeAgent.create(createOpts({ storage: failingStorage }));
      const result = await a.runWithDetails('fail-storage');
      expect(result.content).toBeTruthy();
      expect(result.atomicWrite).toBe(false);
      a.close();
    });
  });
});
