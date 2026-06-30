/**
 * Phase 5 高级进化能力 — 单元测试
 *
 * 覆盖：
 * 1. 投机执行 (Speculative Execution)
 * 2. Prompt A/B 测试
 * 3. Tool Learning 闭环增强
 * 4. Edge-Native 冷启动优化
 * 5. 分布式 Agent 编排
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { InMemoryStore } from '../../src/memory/store.js';
import {
  SpeculativeExecutor,
  ToolResultPredictor,
} from '../../src/agent/speculative-exec.js';
import {
  PromptABTest,
  KeywordEvaluator,
  CompletenessEvaluator,
} from '../../src/agent/prompt-ab-test.js';
import { EnhancedToolLearner } from '../../src/agent/tool-learning.js';
import {
  ColdStartOptimizer,
  LazyLoader,
  ConnectionPrewarmer,
  lazyLoad,
  initEdgeRuntime,
} from '../../src/edge/cold-start.js';
import { DistributedOrchestrator } from '../../src/agent/distributed-orchestration.js';
import type { ToolCall, ToolResult, Response } from '../../src/types.js';

// ===== Mock 工具 =====
class MockTool {
  name = 'mock_tool';
  description = 'A mock tool for testing';
  parameters = { type: 'object' as const, properties: {} };
  async execute(args: Record<string, unknown>): Promise<string> {
    return `result: ${JSON.stringify(args)}`;
  }
}

// ===== 1. 投机执行测试 =====

describe('Phase 5.1: Speculative Execution', () => {
  describe('ToolResultPredictor', () => {
    it('should predict based on historical results', () => {
      const predictor = new ToolResultPredictor();
      const result: ToolResult = {
        toolCallId: 'tc-1',
        content: 'file content here',
        isError: false,
      };

      predictor.recordResult('read_file', result);
      const prediction = predictor.predict('read_file', '{"path":"test.txt"}');

      expect(prediction).not.toBeNull();
      expect(prediction!.content).toBe('file content here');
      expect(prediction!.isError).toBe(false);
    });

    it('should return null when no history', () => {
      const predictor = new ToolResultPredictor();
      const prediction = predictor.predict('unknown_tool', '{}');
      expect(prediction).toBeNull();
    });

    it('should detect hit when content matches', () => {
      const predictor = new ToolResultPredictor();
      const predicted: ToolResult = {
        toolCallId: 'spec',
        content: 'same content here',
        isError: false,
      };
      const actual: ToolResult = {
        toolCallId: 'tc-1',
        content: 'same content here',
        isError: false,
      };
      expect(predictor.isHit(predicted, actual)).toBe(true);
    });

    it('should detect miss when content differs', () => {
      const predictor = new ToolResultPredictor();
      const predicted: ToolResult = {
        toolCallId: 'spec',
        content: 'predicted content',
        isError: false,
      };
      const actual: ToolResult = {
        toolCallId: 'tc-1',
        content: 'different content',
        isError: false,
      };
      expect(predictor.isHit(predicted, actual)).toBe(false);
    });

    it('should detect miss when error status differs', () => {
      const predictor = new ToolResultPredictor();
      const predicted: ToolResult = {
        toolCallId: 'spec',
        content: 'content',
        isError: false,
      };
      const actual: ToolResult = {
        toolCallId: 'tc-1',
        content: 'content',
        isError: true,
      };
      expect(predictor.isHit(predicted, actual)).toBe(false);
    });

    it('should clear history on reset', () => {
      const predictor = new ToolResultPredictor();
      predictor.recordResult('tool', { toolCallId: '1', content: 'x', isError: false });
      predictor.reset();
      expect(predictor.predict('tool', '{}')).toBeNull();
    });
  });

  describe('SpeculativeExecutor', () => {
    it('should execute tools and return LLM response', async () => {
      const provider = new MockProvider({ response: 'final answer' });
      const toolkit = new ToolRegistry();
      toolkit.register(new MockTool());

      const executor = new SpeculativeExecutor(provider, toolkit);
      const toolCalls: ToolCall[] = [
        { id: 'tc-1', name: 'mock_tool', arguments: '{"input":"test"}' },
      ];

      const result = await executor.executeWithSpeculation([], toolCalls);

      expect(result.toolResults).toHaveLength(1);
      expect(result.toolResults[0]!.content).toContain('result:');
      expect(result.toolResults[0]!.isError).toBe(false);
    });

    it('should track statistics', async () => {
      const provider = new MockProvider({ response: 'answer' });
      const toolkit = new ToolRegistry();
      toolkit.register(new MockTool());

      const executor = new SpeculativeExecutor(provider, toolkit);
      const stats = executor.getStats();

      expect(stats.totalSpeculations).toBe(0);
      expect(stats.hitRate).toBe(0);
    });

    it('should reset stats and predictor', () => {
      const provider = new MockProvider({ response: 'answer' });
      const toolkit = new ToolRegistry();
      const executor = new SpeculativeExecutor(provider, toolkit);

      executor.reset();
      const stats = executor.getStats();
      expect(stats.totalSpeculations).toBe(0);
    });

    it('should auto-disable when hit rate is too low', async () => {
      const provider = new MockProvider({ response: 'answer' });
      const toolkit = new ToolRegistry();
      toolkit.register(new MockTool());

      const executor = new SpeculativeExecutor(provider, toolkit, {
        enabled: true,
        minHitRate: 0.99, // Very high threshold to trigger auto-disable
      });

      // Record some results to enable speculation, then miss repeatedly
      const predictor = new ToolResultPredictor();
      predictor.recordResult('mock_tool', { toolCallId: '1', content: 'old', isError: false });

      // Execute multiple times with mismatched results
      for (let i = 0; i < 6; i++) {
        await executor.executeWithSpeculation([], [
          { id: `tc-${i}`, name: 'mock_tool', arguments: `{"i":${i}}` },
        ]);
      }

      // After 5+ speculations with low hit rate, should be auto-disabled
      expect(executor.isAutoDisabled()).toBe(true);
    });
  });
});

// ===== 2. Prompt A/B 测试 =====

describe('Phase 5.2: Prompt A/B Testing', () => {
  describe('KeywordEvaluator', () => {
    it('should score based on keyword matches', async () => {
      const evaluator = new KeywordEvaluator(['hello', 'world']);
      const response: Response = {
        content: 'hello world test',
        metrics: { totalTurns: 1, totalTools: 0, duration: 100, llmLatency: 50, toolLatency: 0 },
      };

      const { score } = await evaluator.evaluate('test', response, {
        name: 'test',
        systemPrompt: 'test',
      });
      expect(score).toBe(1.0);
    });

    it('should return partial score for partial matches', async () => {
      const evaluator = new KeywordEvaluator(['hello', 'world', 'foo']);
      const response: Response = {
        content: 'hello world',
        metrics: { totalTurns: 1, totalTools: 0, duration: 100, llmLatency: 50, toolLatency: 0 },
      };

      const { score } = await evaluator.evaluate('test', response, {
        name: 'test',
        systemPrompt: 'test',
      });
      expect(score).toBeCloseTo(2 / 3, 2);
    });
  });

  describe('CompletenessEvaluator', () => {
    it('should score 1.0 for optimal length', async () => {
      const evaluator = new CompletenessEvaluator(10, 1000);
      const response: Response = {
        content: 'a'.repeat(100),
        metrics: { totalTurns: 1, totalTools: 0, duration: 100, llmLatency: 50, toolLatency: 0 },
      };

      const { score } = await evaluator.evaluate('test', response, {
        name: 'test',
        systemPrompt: 'test',
      });
      expect(score).toBe(1.0);
    });

    it('should penalize too short responses', async () => {
      const evaluator = new CompletenessEvaluator(100, 1000);
      const response: Response = {
        content: 'short',
        metrics: { totalTurns: 1, totalTools: 0, duration: 100, llmLatency: 50, toolLatency: 0 },
      };

      const { score } = await evaluator.evaluate('test', response, {
        name: 'test',
        systemPrompt: 'test',
      });
      expect(score).toBeLessThan(0.5);
    });
  });

  describe('PromptABTest', () => {
    it('should throw with less than 2 variants', () => {
      expect(() => new PromptABTest({
        variants: [{ name: 'only', systemPrompt: 'test' }],
        evaluator: new KeywordEvaluator(['test']),
      })).toThrow();
    });

    it('should run A/B test and pick winner', async () => {
      const evaluator = new KeywordEvaluator(['detailed', 'comprehensive']);

      const abTest = new PromptABTest({
        variants: [
          { name: 'concise', systemPrompt: 'Be concise' },
          { name: 'detailed', systemPrompt: 'Be detailed and comprehensive' },
        ],
        evaluator,
        repeatsPerVariant: 2,
      });

      const agentFactory = (variant: { name: string; systemPrompt: string }) => ({
        run: async (_input: string): Promise<Response> => {
          // 'detailed' variant returns content with keywords
          const content = variant.name === 'detailed'
            ? 'This is a detailed and comprehensive response'
            : 'Short response';
          return {
            content,
            metrics: { totalTurns: 1, totalTools: 0, duration: 100, llmLatency: 50, toolLatency: 0 },
          };
        },
      });

      const result = await abTest.run('test input', agentFactory);

      expect(result.winner).toBe('detailed');
      expect(result.winnerScore).toBeGreaterThan(0);
      expect(result.results).toHaveLength(4); // 2 variants * 2 repeats
      expect(result.recommendedPrompt).toBe('Be detailed and comprehensive');
      expect(result.summary).toContain('A/B Test Summary');
    });

    it('should run batch tests', async () => {
      const evaluator = new KeywordEvaluator(['hello']);
      const abTest = new PromptABTest({
        variants: [
          { name: 'a', systemPrompt: 'A' },
          { name: 'b', systemPrompt: 'B' },
        ],
        evaluator,
        repeatsPerVariant: 1,
      });

      const agentFactory = () => ({
        run: async (): Promise<Response> => ({
          content: 'hello world',
          metrics: { totalTurns: 1, totalTools: 0, duration: 50, llmLatency: 25, toolLatency: 0 },
        }),
      });

      const results = await abTest.runBatch(['input1', 'input2'], agentFactory);
      expect(results).toHaveLength(2);
    });
  });
});

// ===== 3. Tool Learning 闭环增强测试 =====

describe('Phase 5.3: Enhanced Tool Learning', () => {
  let memory: InMemoryStore;
  let learner: EnhancedToolLearner;

  beforeEach(() => {
    memory = new InMemoryStore();
    learner = new EnhancedToolLearner(memory, { minRecordsForPattern: 1 });
  });

  it('should record success and generate best practices', async () => {
    await learner.recordSuccess('search', '{"query":"test"}', 'results found');
    await learner.recordSuccess('search', '{"query":"test"}', 'results found');
    await learner.recordSuccess('search', '{"query":"test"}', 'results found');

    const practices = await learner.getBestPractices('search');
    expect(practices.length).toBeGreaterThan(0);
    expect(practices[0]!.toolName).toBe('search');
  });

  it('should generate few-shot examples from successful records', async () => {
    // Record enough successful executions
    for (let i = 0; i < 5; i++) {
      await learner.recordSuccess('calculator', '{"action":"add","a":1,"b":2}', '3');
    }

    const examples = await learner.generateFewShotExamples('calculator');
    expect(examples.length).toBeGreaterThan(0);
    expect(examples[0]!.toolName).toBe('calculator');
    expect(examples[0]!.toolArgs).toContain('action');
  });

  it('should generate usage patterns', async () => {
    for (let i = 0; i < 5; i++) {
      await learner.recordSuccess('filesystem', '{"action":"read","path":"/test"}', 'content');
    }

    const patterns = await learner.getUsagePatterns('filesystem');
    expect(patterns.length).toBeGreaterThan(0);
    expect(patterns[0]!.toolName).toBe('filesystem');
    expect(patterns[0]!.successRate).toBeGreaterThan(0);
  });

  it('should generate usage guide', async () => {
    for (let i = 0; i < 5; i++) {
      await learner.recordSuccess('search', '{"query":"test"}', 'found');
    }

    const guide = await learner.generateUsageGuide(['search']);
    expect(guide).toContain('Tool Usage Guide');
    expect(guide).toContain('search');
  });

  it('should detect inefficiencies', async () => {
    // Record mix of success and failure
    for (let i = 0; i < 5; i++) {
      await learner.recordSuccess('tool', '{"action":"good"}', 'ok');
    }
    for (let i = 0; i < 5; i++) {
      await learner.recordFailure('tool', '{"action":"bad"}', 'failed');
    }

    const suggestions = await learner.detectInefficiencies('tool');
    expect(suggestions.length).toBeGreaterThan(0);
  });

  it('should clear cache', async () => {
    await learner.recordSuccess('tool', '{"action":"test"}', 'ok');
    await learner.generateFewShotExamples('tool');
    learner.clearCache();
    // After clearing cache, should regenerate
    const examples = await learner.generateFewShotExamples('tool');
    expect(examples).toBeDefined();
  });
});

// ===== 4. Edge-Native 冷启动优化测试 =====

describe('Phase 5.4: Edge-Native Cold Start Optimization', () => {
  describe('LazyLoader', () => {
    it('should lazily load module on first access', async () => {
      let loaded = false;
      const loader = new LazyLoader(async () => {
        loaded = true;
        return { value: 42 };
      });

      expect(loaded).toBe(false);
      expect(loader.isLoaded()).toBe(false);

      const mod = await loader.get();
      expect(loaded).toBe(true);
      expect(loader.isLoaded()).toBe(true);
      expect(mod.value).toBe(42);
    });

    it('should cache loaded module', async () => {
      let loadCount = 0;
      const loader = new LazyLoader(async () => {
        loadCount++;
        return { count: loadCount };
      });

      await loader.get();
      await loader.get();

      expect(loadCount).toBe(1);
    });

    it('should preload without blocking', async () => {
      let loaded = false;
      const loader = new LazyLoader(async () => {
        await new Promise((r) => setTimeout(r, 10));
        loaded = true;
        return { data: 'loaded' };
      });

      loader.preload();
      expect(loaded).toBe(false);
      await loader.get();
      expect(loaded).toBe(true);
    });
  });

  describe('lazyLoad factory', () => {
    it('should create LazyLoader instance', () => {
      const loader = lazyLoad(async () => ({ test: true }));
      expect(loader).toBeInstanceOf(LazyLoader);
    });
  });

  describe('ConnectionPrewarmer', () => {
    it('should track prewarmed endpoints', async () => {
      const prewarmer = new ConnectionPrewarmer();
      // Mock fetch
      const originalFetch = globalThis.fetch;
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true }) as never;

      await prewarmer.prewarmLLM('https://api.test.com');
      expect(prewarmer.isPrewarmed('https://api.test.com')).toBe(true);

      globalThis.fetch = originalFetch;
    });

    it('should prewarm multiple endpoints', async () => {
      const prewarmer = new ConnectionPrewarmer();
      const originalFetch = globalThis.fetch;
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true }) as never;

      await prewarmer.prewarmAll(['https://a.com', 'https://b.com']);
      expect(prewarmer.isPrewarmed('https://a.com')).toBe(true);
      expect(prewarmer.isPrewarmed('https://b.com')).toBe(true);

      globalThis.fetch = originalFetch;
    });
  });

  describe('ColdStartOptimizer', () => {
    it('should analyze and return report', async () => {
      const optimizer = new ColdStartOptimizer();
      const report = await optimizer.analyze();

      expect(report).toBeDefined();
      expect(report.runtime).toBeDefined();
      expect(report.coldStartMs).toBeGreaterThanOrEqual(0);
      expect(report.suggestions).toBeInstanceOf(Array);
      expect(report.lazyLoadCandidates).toBeInstanceOf(Array);
    });

    it('should generate suggestions with priority', async () => {
      const optimizer = new ColdStartOptimizer();
      const report = await optimizer.analyze();

      expect(report.suggestions.length).toBeGreaterThan(0);
      for (const suggestion of report.suggestions) {
        expect(suggestion.priority).toBeGreaterThanOrEqual(1);
        expect(suggestion.priority).toBeLessThanOrEqual(5);
        expect(suggestion.estimatedSavingMs).toBeGreaterThan(0);
      }
    });

    it('should generate Cloudflare config', () => {
      const optimizer = new ColdStartOptimizer();
      const config = optimizer.generateCloudflareConfig();
      expect(config).toContain('wrangler.toml');
      expect(config).toContain('agentprimordia-worker');
    });

    it('should generate Deno config', () => {
      const optimizer = new ColdStartOptimizer();
      const config = optimizer.generateDenoConfig();
      expect(config).toContain('deno.json');
      expect(config).toContain('deno-server');
    });

    it('should generate lazy-load wrapper code', () => {
      const optimizer = new ColdStartOptimizer();
      const wrapper = optimizer.generateLazyLoadWrapper('llm/openai.js', ['OpenAIProvider']);
      expect(wrapper).toContain('lazyLoad');
      expect(wrapper).toContain('OpenAIProvider');
    });

    it('should record module loads', () => {
      const optimizer = new ColdStartOptimizer();
      optimizer.recordModuleLoad('test.js', 5, 10);
      // No direct assertion, but should not throw
    });
  });

  describe('initEdgeRuntime', () => {
    it('should initialize and return optimizer/prewarmer/report', async () => {
      const originalFetch = globalThis.fetch;
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true }) as never;

      const { optimizer, prewarmer, report } = await initEdgeRuntime({
        prewarmEndpoints: ['https://api.test.com'],
      });

      expect(optimizer).toBeInstanceOf(ColdStartOptimizer);
      expect(prewarmer).toBeInstanceOf(ConnectionPrewarmer);
      expect(report).toBeDefined();
      expect(report.runtime).toBeDefined();

      globalThis.fetch = originalFetch;
    });
  });
});

// ===== 5. 分布式 Agent 编排测试 =====

describe('Phase 5.5: Distributed Agent Orchestration', () => {
  describe('DistributedOrchestrator', () => {
    it('should construct with config', () => {
      const mockTransport = {
        connect: vi.fn().mockResolvedValue(undefined),
        onMessage: vi.fn(),
        send: vi.fn().mockResolvedValue(undefined),
        close: vi.fn(),
      };

      const orchestrator = new DistributedOrchestrator(
        {
          agentId: 'test-agent',
          name: 'TestAgent',
          roles: ['worker'],
          serverUrl: 'ws://localhost:8080',
        },
        mockTransport as never,
      );

      expect(orchestrator).toBeDefined();
      expect(orchestrator.getNodes()).toHaveLength(0); // Not started yet
    });

    it('should start and register self', async () => {
      const mockTransport = {
        connect: vi.fn().mockResolvedValue(undefined),
        onMessage: vi.fn(),
        send: vi.fn().mockResolvedValue(undefined),
        close: vi.fn(),
      };

      const orchestrator = new DistributedOrchestrator(
        {
          agentId: 'worker-1',
          name: 'Worker1',
          roles: ['researcher'],
          serverUrl: 'ws://localhost:8080',
        },
        mockTransport as never,
      );

      await orchestrator.start();

      const nodes = orchestrator.getNodes();
      expect(nodes).toHaveLength(1);
      expect(nodes[0]!.id).toBe('worker-1');
      expect(nodes[0]!.status).toBe('online');

      await orchestrator.stop();
    });

    it('should execute local task when no remote agents', async () => {
      const mockTransport = {
        connect: vi.fn().mockResolvedValue(undefined),
        onMessage: vi.fn(),
        send: vi.fn().mockResolvedValue(undefined),
        close: vi.fn(),
      };

      const orchestrator = new DistributedOrchestrator(
        {
          agentId: 'solo-agent',
          name: 'Solo',
          roles: ['worker'],
          serverUrl: 'ws://localhost:8080',
          taskTimeoutMs: 5000,
        },
        mockTransport as never,
      );

      await orchestrator.start();
      orchestrator.onTask(async (input) => `processed: ${input}`);

      const result = await orchestrator.submitTask('test input');

      expect(result.success).toBe(true);
      expect(result.output).toBe('processed: test input');
      expect(result.agentId).toBe('solo-agent');

      await orchestrator.stop();
    });

    it('should run MapReduce locally when alone', async () => {
      const mockTransport = {
        connect: vi.fn().mockResolvedValue(undefined),
        onMessage: vi.fn(),
        send: vi.fn().mockResolvedValue(undefined),
        close: vi.fn(),
      };

      const orchestrator = new DistributedOrchestrator(
        {
          agentId: 'map-agent',
          name: 'Map',
          roles: ['worker'],
          serverUrl: 'ws://localhost:8080',
        },
        mockTransport as never,
      );

      await orchestrator.start();

      const inputs = ['task1', 'task2', 'task3'];
      const result = await orchestrator.submitMapReduce(
        inputs,
        async (input) => `mapped:${input}`,
        (outputs) => outputs.join(','),
      );

      expect(result.mapResults).toHaveLength(3);
      expect(result.mapResults.every((r) => r.success)).toBe(true);
      expect(result.reduceResult).toBe('mapped:task1,mapped:task2,mapped:task3');
      expect(result.totalDurationMs).toBeGreaterThanOrEqual(0);

      await orchestrator.stop();
    });

    it('should handle task timeout', async () => {
      const mockTransport = {
        connect: vi.fn().mockResolvedValue(undefined),
        onMessage: vi.fn(),
        send: vi.fn().mockResolvedValue(undefined),
        close: vi.fn(),
      };

      const orchestrator = new DistributedOrchestrator(
        {
          agentId: 'timeout-agent',
          name: 'Timeout',
          roles: ['worker'],
          serverUrl: 'ws://localhost:8080',
          taskTimeoutMs: 50, // Very short timeout
        },
        mockTransport as never,
      );

      await orchestrator.start();

      // Register a fake remote agent that never responds
      // The transport.send is mocked, so the response will never come
      orchestrator['nodes'].set('remote-agent', {
        id: 'remote-agent',
        name: 'Remote',
        address: 'ws://remote:8080',
        roles: ['worker'],
        status: 'online',
        load: 0,
        lastHeartbeat: Date.now(),
      });

      await expect(
        orchestrator.submitTask('test', { targetAgentId: 'remote-agent' }),
      ).rejects.toThrow('timed out');

      await orchestrator.stop();
    });

    it('should broadcast to all agents', async () => {
      const mockTransport = {
        connect: vi.fn().mockResolvedValue(undefined),
        onMessage: vi.fn(),
        send: vi.fn().mockResolvedValue(undefined),
        close: vi.fn(),
      };

      const orchestrator = new DistributedOrchestrator(
        {
          agentId: 'broadcast-agent',
          name: 'Broadcast',
          roles: ['worker'],
          serverUrl: 'ws://localhost:8080',
        },
        mockTransport as never,
      );

      await orchestrator.start();
      orchestrator.onTask(async (input) => `got: ${input}`);

      // No remote agents, so broadcast will use local execution
      const results = await orchestrator.broadcast('hello');

      // With no remote agents, broadcast returns empty (self is filtered out)
      expect(results).toHaveLength(0);

      await orchestrator.stop();
    });
  });
});
