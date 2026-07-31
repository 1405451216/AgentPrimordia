/**
 * bun-agent.ts unit tests
 *
 * Coverage:
 * - BunEdgeAgent construction with memory storage fallback
 * - run() execution and storage persistence
 * - BunSQLiteStorage fallback to memory in non-Bun env
 * - getAgent() returns underlying agent
 * - Custom storage injection
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { BunEdgeAgent, type BunAgentOptions } from '../bun-agent.js';
import { MemoryEdgeStorage, BunSQLiteStorage } from '../edge-storage.js';
import type { Provider } from '../../llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../../types.js';

function createMockProvider(overrides?: Partial<Provider>): Provider {
  return {
    complete: vi.fn(async (_req: CompletionRequest): Promise<CompletionResponse> => ({
      id: 'mock-id',
      content: 'bun-mock-response',
      role: 'assistant',
      usage: { promptTokens: 8, completionTokens: 4, totalTokens: 12 },
    })),
    callTools: vi.fn(async (_req: ToolCallRequest): Promise<ToolCallResponse> => ({
      content: '',
      toolCalls: [],
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    })),
    info: vi.fn((): ModelInfo => ({
      name: 'mock-bun-model',
      provider: 'mock',
      maxContext: 4096,
      supportsTools: false,
      supportsStreaming: false,
    })),
    ...overrides,
  };
}

function createOpts(overrides?: Partial<BunAgentOptions>): BunAgentOptions {
  return {
    name: 'test-bun-agent',
    provider: createMockProvider(),
    storage: new MemoryEdgeStorage(),
    maxTurns: 3,
    systemPrompt: 'You are a test agent',
    ...overrides,
  };
}

describe('BunEdgeAgent', () => {
  let agent: BunEdgeAgent;

  beforeEach(() => {
    agent = new BunEdgeAgent(createOpts());
  });

  describe('constructor', () => {
    it('should create agent with provided storage', () => {
      expect(agent).toBeDefined();
      expect(agent.storage).toBeInstanceOf(MemoryEdgeStorage);
    });

    it('should default to BunSQLiteStorage when no storage provided', () => {
      const a = new BunEdgeAgent({ name: 'default-storage', provider: createMockProvider() });
      expect(a.storage).toBeInstanceOf(BunSQLiteStorage);
    });

    it('should use custom name', () => {
      const inner = agent.getAgent();
      expect(inner.name).toBe('test-bun-agent');
    });

    it('should default name to bun-agent', () => {
      const a = new BunEdgeAgent({ provider: createMockProvider() });
      expect(a.getAgent().name).toBe('bun-agent');
    });
  });

  describe('run()', () => {
    it('should return response content', async () => {
      const result = await agent.run('hello bun');
      expect(typeof result).toBe('string');
      expect(result).toBe('bun-mock-response');
    });

    it('should store last input and output', async () => {
      await agent.run('test-input');
      const storage = agent.storage as MemoryEdgeStorage;
      expect(await storage.get('last:input')).toBe('test-input');
      expect(await storage.get('last:output')).toBe('bun-mock-response');
    });

    it('should call provider.complete()', async () => {
      const provider = createMockProvider();
      const a = new BunEdgeAgent(createOpts({ provider }));
      await a.run('trigger');
      expect(provider.complete).toHaveBeenCalled();
    });

    it('should return error message when provider fails', async () => {
      const provider = createMockProvider({
        complete: vi.fn().mockRejectedValue(new Error('provider fail')),
      });
      const a = new BunEdgeAgent(createOpts({ provider }));
      const result = await a.run('fail');
      expect(result).toContain('provider fail');
    });
  });

  describe('getAgent()', () => {
    it('should return the underlying ReActAgent', () => {
      const inner = agent.getAgent();
      expect(inner).toBeDefined();
      expect(typeof inner.run).toBe('function');
    });
  });
});

describe('BunSQLiteStorage fallback', () => {
  it('should fall back to memory in non-Bun environment', async () => {
    const storage = new BunSQLiteStorage();
    await storage.set('key1', 'value1');
    expect(await storage.get('key1')).toBe('value1');
  });

  it('should support delete in fallback mode', async () => {
    const storage = new BunSQLiteStorage();
    await storage.set('k', 'v');
    await storage.delete('k');
    expect(await storage.get('k')).toBeNull();
  });

  it('should support list in fallback mode', async () => {
    const storage = new BunSQLiteStorage();
    await storage.set('prefix:a', 1);
    await storage.set('prefix:b', 2);
    await storage.set('other:c', 3);
    const entries = await storage.list('prefix:');
    expect(entries).toHaveLength(2);
    expect(entries.map(([k]) => k).sort()).toEqual(['prefix:a', 'prefix:b']);
  });
});
