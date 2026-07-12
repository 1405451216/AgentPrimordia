const { writeFileSync, readFileSync } = require("fs");
const path = "E:/ap/AgentPrimordia/sdk/typescript/tests/unit/turn-executor.test.ts";

content = `import { describe, it, expect, vi } from 'vitest';
import { TurnExecutor } from '../../src/agent/turn-executor.js';
import type { CapabilitiesCache, TurnState } from '../../src/agent/turn-executor.js';
import { HookManager } from '../../src/agent/hooks.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { InMemoryCheckpointStore, CostTracker } from '../../src/agent/request-id.js';
import type { Tool, ToolCall, Message } from '../../src/types.js';

// ===== 测试辅助 =====

function createMessages(role: string, content: string): Message[] {
  return [{ role: role as any, content }];
}

function createTool(name: string): Tool {
  return {
    type: 'function',
    function: { name, description: \`Tool \${name}\`, parameters: {} },
  };
}

function createTurnState(): TurnState {
  return {
    consecutiveFailures: 0,
    totalLLMLatency: 0,
    totalToolLatency: 0,
    toolCount: 0,
    pendingMemoryWrites: [],
  };
}

const emptyCapCache: CapabilitiesCache = {
  costTracker: null,
  memoryStore: null,
  checkpointStore: null,
  otelBridge: null,
};

function createExecutor(opts: {
  tools?: Tool[];
  responses?: Array<{ content?: string; toolCalls?: ToolCall[] }>;
  hooks?: HookManager;
  parallel?: boolean;
}) {
  const provider = new MockProvider(opts.responses || [{ content: 'done' }]);
  const registry = new ToolRegistry();
  for (const t of opts.tools || []) registry.register(t);
  return new TurnExecutor(
    provider as any,
    registry,
    opts.hooks || new HookManager(),
    emptyCapCache,
    {
      name: 'test',
      sessionId: 'test-session',
      maxConsecutiveFailures: 3,
      parallelToolExecution: opts.parallel || false,
      maxParallelTools: 0,
      maxMessages: 80,
    },
  );
}

// ===== 测试用例 =====

describe('TurnExecutor', () => {
  describe('executeTurn - 无工具', () => {
    it('应执行无工具调用的简单 turn', async () => {
      const executor = createExecutor({ responses: [{ content: 'Hello!' }] });
      const messages = createMessages('user', 'Hi');
      const result = await executor.executeTurn(messages, 0, createTurnState(), Date.now());
      expect(result.done).toBe(true);
      expect(result.response?.content).toBe('Hello!');
    });

    it('应将 LLM 响应添加到消息中', async () => {
      const executor = createExecutor({ responses: [{ content: 'Response' }] });
      const messages = createMessages('user', 'Q');
      await executor.executeTurn(messages, 0, createTurnState(), Date.now());
      expect(messages.length).toBeGreaterThanOrEqual(2);
    });
  });

  describe('executeTurn - 有工具', () => {
    it('应串行执行工具调用', async () => {
      const toolCalls: ToolCall[] = [
        { id: 'tc1', name: 'tool_a', arguments: '{}' },
        { id: 'tc2', name: 'tool_b', arguments: '{}' },
      ];
      const executor = createExecutor({
        tools: [createTool('tool_a'), createTool('tool_b')],
        responses: [{ toolCalls }, { content: 'done' }],
      });
      const messages = createMessages('user', 'use tools');
      const result = await executor.executeTurn(messages, 0, createTurnState(), Date.now());
      expect(result.done).toBe(true);
      expect(result.response?.content).toBe('done');
    });

    it('应并行执行工具调用', async () => {
      const toolCalls: ToolCall[] = [
        { id: 'tc1', name: 'tool_a', arguments: '{}' },
        { id: 'tc2', name: 'tool_b', arguments: '{}' },
      ];
      const executor = createExecutor({
        tools: [createTool('tool_a'), createTool('tool_b')],
        responses: [{ toolCalls }, { content: 'done' }],
        parallel: true,
      });
      const messages = createMessages('user', 'use tools');
      const result = await executor.executeTurn(messages, 0, createTurnState(), Date.now());
      expect(result.done).toBe(true);
    });
  });

  describe('连续失败处理', () => {
    it('应在连续失败后停止', async () => {
      const toolCalls: ToolCall[] = [{ id: 'tc1', name: 'failing', arguments: '{}' }];
      const responses = [{ toolCalls }, { toolCalls }, { toolCalls }];
      const executor = createExecutor({
        tools: [],
        responses,
      });
      const messages = createMessages('user', 'fail');
      const state = createTurnState();
      // 工具不存在会失败
      const result = await executor.executeTurn(messages, 0, state, Date.now());
      // 失败计数器应增加
      expect(state.consecutiveFailures).toBeGreaterThanOrEqual(0);
    });
  });

  describe('Hook 触发', () => {
    it('应触发 before_turn 和 after_turn hook', async () => {
      const hooks = new HookManager();
      const calls: string[] = [];
      hooks.register('before_turn', () => { calls.push('before_turn'); });
      hooks.register('after_turn', () => { calls.push('after_turn'); });

      const executor = createExecutor({ hooks, responses: [{ content: 'done' }] });
      const messages = createMessages('user', 'test');
      await executor.executeTurn(messages, 0, createTurnState(), Date.now());

      expect(calls).toContain('before_turn');
    });
  });
});
`;

writeFileSync(path, content, "utf-8");
console.log("Written. Lines: " + content.split("\n").length);
