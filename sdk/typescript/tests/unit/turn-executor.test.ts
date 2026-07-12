import { describe, it, expect } from 'vitest';
import { TurnExecutor } from '../../src/agent/turn-executor.js';
import type { CapabilitiesCache, TurnState, TurnResult, ToolExecutionResult } from '../../src/agent/turn-executor.js';
import type { ToolCall } from '../../src/types.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { HookManager } from '../../src/agent/hooks.js';

function createState(): TurnState {
  return { consecutiveFailures: 0, totalLLMLatency: 0, totalToolLatency: 0, toolCount: 0, pendingMemoryWrites: [] };
}

const emptyCapCache: CapabilitiesCache = { costTracker: null, memoryStore: null, checkpointStore: null, otelBridge: null };

function createTool(name: string) {
  return { type: 'function' as const, function: { name, description: 'desc', parameters: {} } };
}

function makeExecutor(response: string, opts: { toolCalls?: ToolCall[]; hooks?: HookManager; tools?: any[] } = {}) {
  const provider = new MockProvider({ response, toolCalls: opts.toolCalls });
  const registry = new ToolRegistry();
  for (const t of opts.tools || []) registry.register(t);
  return new TurnExecutor(provider as unknown as any, registry, opts.hooks || new HookManager(), emptyCapCache,
    { name: 'test', sessionId: 'test-session', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
}

describe('TurnExecutor', () => {
  describe('executeTurn - basic', () => {
    it('completes a turn without tools', async () => {
      const executor = makeExecutor('Hello!');
      const result = await executor.executeTurn([{ role: 'user', content: 'Hi' }], 0, createState(), Date.now());
      expect(result.response?.content).toBe('Hello!');
    });

    it('shouldStop when no tool calls', async () => {
      const executor = makeExecutor('Done');
      const result = await executor.executeTurn([{ role: 'user' as const, content: 'q' }], 0, createState(), Date.now());
      expect(result.shouldStop).toBe(true);
    });

    it('should append assistant message to messages', async () => {
      const executor = makeExecutor('Response');
      const messages = [{ role: 'user' as const, content: 'Q' }];
      await executor.executeTurn(messages, 0, createState(), Date.now());
      expect(messages.length).toBeGreaterThanOrEqual(2);
    });
  });

  describe('executeTurn - tools', () => {
    it('should execute tools and continue (not stop)', async () => {
      const toolCalls: ToolCall[] = [{ id: 'tc1', name: 'echo', arguments: '{}' }];
      const executor = makeExecutor('tool result', { toolCalls, tools: [createTool('echo')] });
      const result = await executor.executeTurn([{ role: 'user' as const, content: 'call' }], 0, createState(), Date.now());
      // return tools result and continue
      expect(result.tools).toBeDefined();
      expect(result.shouldStop).toBe(false);
    });

    it('should update tool count in state', async () => {
      const toolCalls: ToolCall[] = [{ id: 'tc1', name: 'echo', arguments: '{}' }];
      const state = createState();
      const executor = makeExecutor('result', { toolCalls, tools: [createTool('echo')] });
      await executor.executeTurn([{ role: 'user' as const, content: 'go' }], 0, state, Date.now());
      expect(state.toolCount).toBe(1);
    });

    it('should not throw when tool not found', async () => {
      const toolCalls: ToolCall[] = [{ id: 'tc1', name: 'nonexistent', arguments: '{}' }];
      const state = createState();
      const executor = makeExecutor('fail', { toolCalls, tools: [] });
      // Should not throw, but handle gracefully
      await expect(executor.executeTurn([{ role: 'user' as const, content: 'err' }], 0, state, Date.now())).resolves.toBeDefined();
    });
  });

  describe('hooks', () => {
    it('should trigger before_turn, after_llm, on_complete and after_turn', async () => {
      const hooks = new HookManager();
      const calls: string[] = [];
      hooks.register('before_turn', () => calls.push('before_turn'));
      hooks.register('after_llm', () => calls.push('after_llm'));
      hooks.register('on_complete', () => calls.push('on_complete'));
      hooks.register('after_turn', () => calls.push('after_turn'));
      const executor = makeExecutor('done', { hooks });
      await executor.executeTurn([{ role: 'user' as const, content: 'hi' }], 0, createState(), Date.now());
      expect(calls).toContain('before_turn');
      expect(calls).toContain('after_llm');
      expect(calls).toContain('on_complete');
      expect(calls).toContain('after_turn');
    });
  });

  describe('error handling', () => {
    it('should propagate LLM error', async () => {
      const provider = new MockProvider({ response: 'err', error: true });
      const registry = new ToolRegistry();
      const executor = new TurnExecutor(provider as unknown as any, registry, new HookManager(), emptyCapCache,
        { name: 'test', sessionId: 'test-session', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
      await expect(executor.executeTurn([{ role: 'user' as const, content: 'err' }], 0, createState(), Date.now())).rejects.toThrow();
    });
  });
});
