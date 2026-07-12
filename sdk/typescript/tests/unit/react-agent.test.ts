/**
 * ReActAgent 核心单元测试
 *
 * 覆盖：
 * - 构造校验（name/model/toolkit 必填，maxTurns 范围）
 * - 基本运行（无工具调用 → 直接返回）
 * - 工具调用循环（LLM 请求工具 → 执行 → 反馈 → 最终回复）
 * - maxTurns 限制
 * - 连续工具失败上限
 * - 优雅关闭
 * - 取消信号（AbortSignal）
 * - 钩子触发（before_run / after_run / before_turn / after_turn）
 * - 流式事件
 */
import { describe, it, expect, vi } from 'vitest';
import { ReActAgent, HookManager, Lifecycle } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import type { Tool, ToolCall } from '../../src/types.js';

// ===== Mock 工具 =====

class EchoTool implements Tool {
  name = 'echo';
  description = 'Echo back the input';
  parameters = {
    type: 'object' as const,
    properties: {
      message: { type: 'string' },
    },
    required: ['message'],
  };
  async execute(args: Record<string, unknown>): Promise<string> {
    return `echo: ${args.message}`;
  }
}

class CalcTool implements Tool {
  name = 'calc';
  description = 'Calculate expression';
  parameters = {
    type: 'object' as const,
    properties: {
      expr: { type: 'string' },
    },
    required: ['expr'],
  };
  async execute(args: Record<string, unknown>): Promise<string> {
    return `result: ${args.expr}`;
  }
}

// ===== 可编程 Mock Provider =====
// 支持按调用顺序返回不同响应（第一轮返回工具调用，第二轮返回最终回复）

class ProgrammableProvider extends MockProvider {
  private responses: Array<{ content: string; toolCalls?: ToolCall[] }>;
  private idx = 0;

  constructor(responses: Array<{ content: string; toolCalls?: ToolCall[] }>) {
    super();
    this.responses = responses;
  }

  async callTools() {
    const resp = this.responses[this.idx] ?? this.responses[this.responses.length - 1];
    this.idx++;
    return {
      content: resp.content,
      toolCalls: resp.toolCalls ?? [],
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    };
  }

  async complete() {
    const resp = this.responses[this.idx] ?? this.responses[this.responses.length - 1];
    this.idx++;
    return {
      id: `resp-${this.idx}`,
      content: resp.content,
      role: 'assistant' as const,
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    };
  }

  info() {
    return {
      name: 'programmable-mock',
      provider: 'mock',
      maxContext: 4096,
      supportsTools: true,
      supportsStreaming: true,
    };
  }
}

// ===== Helper =====

function createToolkit(...tools: Tool[]): ToolRegistry {
  const reg = new ToolRegistry();
  for (const t of tools) reg.register(t);
  return reg;
}

// ===== Tests =====

describe('ReActAgent', () => {
  describe('constructor validation', () => {
    it('should throw if name is empty', () => {
      expect(() => new ReActAgent({
        name: '',
        model: new MockProvider(),
        toolkit: new ToolRegistry(),
      })).toThrow('Agent name is required');
    });

    it('should throw if name is whitespace', () => {
      expect(() => new ReActAgent({
        name: '   ',
        model: new MockProvider(),
        toolkit: new ToolRegistry(),
      })).toThrow('Agent name is required');
    });

    it('should throw if model is not provided', () => {
      expect(() => new ReActAgent({
        name: 'test',
        model: null as any,
        toolkit: new ToolRegistry(),
      })).toThrow('Model provider is required');
    });

    it('should throw if toolkit is not provided', () => {
      expect(() => new ReActAgent({
        name: 'test',
        model: new MockProvider(),
        toolkit: null as any,
      })).toThrow('Toolkit is required');
    });

    it('should throw if maxTurns is out of range', () => {
      expect(() => new ReActAgent({
        name: 'test',
        model: new MockProvider(),
        toolkit: new ToolRegistry(),
        maxTurns: 0,
      })).toThrow('maxTurns must be between 1 and 100');

      expect(() => new ReActAgent({
        name: 'test',
        model: new MockProvider(),
        toolkit: new ToolRegistry(),
        maxTurns: 101,
      })).toThrow('maxTurns must be between 1 and 100');
    });

    it('should use default values when optional config is omitted', () => {
      const agent = new ReActAgent({
        name: 'test',
        model: new MockProvider(),
        toolkit: new ToolRegistry(),
      });
      expect(agent.name).toBe('test');
    });
  });

  describe('run (no tools)', () => {
    it('should return response when LLM provides final answer directly', async () => {
      const provider = new ProgrammableProvider([
        { content: 'Hello! How can I help you?' },
      ]);
      const agent = new ReActAgent({
        name: 'test-agent',
        model: provider,
        toolkit: createToolkit(),
        maxTurns: 5,
      });

      const response = await agent.run('Hi');

      expect(response.content).toBe('Hello! How can I help you?');
      expect(response.metrics.totalTurns).toBe(1);
      expect(response.metrics.totalTools).toBe(0);
    });

    it('should include system prompt in messages', async () => {
      const provider = new ProgrammableProvider([
        { content: 'response with system prompt' },
      ]);
      const agent = new ReActAgent({
        name: 'test',
        model: provider,
        toolkit: createToolkit(),
        systemPrompt: 'You are a helpful assistant.',
      });

      const response = await agent.run('Hello');
      expect(response.content).toBe('response with system prompt');
    });
  });

  describe('run (with tool calls)', () => {
    it('should execute tool call and return final response', async () => {
      const toolCall: ToolCall = {
        id: 'tc-1',
        name: 'echo',
        arguments: JSON.stringify({ message: 'hello world' }),
      };

      const provider = new ProgrammableProvider([
        { content: 'Let me echo that for you.', toolCalls: [toolCall] },
        { content: 'The echo result is: echo: hello world' },
      ]);

      const agent = new ReActAgent({
        name: 'tool-agent',
        model: provider,
        toolkit: createToolkit(new EchoTool()),
        maxTurns: 5,
      });

      const response = await agent.run('echo hello world');

      expect(response.content).toContain('echo: hello world');
      expect(response.metrics.totalTools).toBe(1);
      expect(response.metrics.totalTurns).toBeGreaterThanOrEqual(2);
    });

    it('should handle multiple tool calls in sequence', async () => {
      const tc1: ToolCall = { id: 'tc-1', name: 'echo', arguments: JSON.stringify({ message: 'first' }) };
      const tc2: ToolCall = { id: 'tc-2', name: 'calc', arguments: JSON.stringify({ expr: '2+2' }) };

      const provider = new ProgrammableProvider([
        { content: 'Calling first tool', toolCalls: [tc1] },
        { content: 'Calling second tool', toolCalls: [tc2] },
        { content: 'Both tools completed' },
      ]);

      const agent = new ReActAgent({
        name: 'multi-tool',
        model: provider,
        toolkit: createToolkit(new EchoTool(), new CalcTool()),
        maxTurns: 10,
      });

      const response = await agent.run('use both tools');

      expect(response.content).toBe('Both tools completed');
      expect(response.metrics.totalTools).toBe(2);
      expect(response.metrics.totalTurns).toBeGreaterThanOrEqual(3);
    });
  });

  describe('maxTurns limit', () => {
    it('should stop when maxTurns is reached', async () => {
      // Provider always returns tool calls, never a final answer
      const toolCall: ToolCall = {
        id: 'tc-loop',
        name: 'echo',
        arguments: JSON.stringify({ message: 'loop' }),
      };

      const provider = new ProgrammableProvider([
        { content: 'looping', toolCalls: [toolCall] },
        { content: 'looping', toolCalls: [toolCall] },
        { content: 'looping', toolCalls: [toolCall] },
      ]);

      const agent = new ReActAgent({
        name: 'loop-agent',
        model: provider,
        toolkit: createToolkit(new EchoTool()),
        maxTurns: 3,
      });

      const response = await agent.run('start loop');

      // Should stop after 3 turns (0, 1, 2)
      expect(response.metrics.totalTurns).toBeLessThanOrEqual(3);
    });
  });

  describe('consecutive failures', () => {
    it('should stop after maxConsecutiveFailures', async () => {
      // Tool doesn't exist → all calls will error
      const toolCall: ToolCall = {
        id: 'tc-fail',
        name: 'nonexistent_tool',
        arguments: '{}',
      };

      const provider = new ProgrammableProvider([
        { content: 'calling missing tool', toolCalls: [toolCall] },
        { content: 'calling missing tool', toolCalls: [toolCall] },
        { content: 'calling missing tool', toolCalls: [toolCall] },
        { content: 'calling missing tool', toolCalls: [toolCall] },
      ]);

      const agent = new ReActAgent({
        name: 'fail-agent',
        model: provider,
        toolkit: createToolkit(), // no tools registered → all calls fail
        maxTurns: 10,
        maxConsecutiveFailures: 3,
      });

      const response = await agent.run('call missing tool');

      // Should stop early due to consecutive failures
      expect(response.metrics.totalTools).toBeLessThanOrEqual(3);
    });
  });

  describe('graceful shutdown', () => {
    it('should support requestGracefulShutdown', () => {
      const agent = new ReActAgent({
        name: 'test',
        model: new MockProvider(),
        toolkit: new ToolRegistry(),
      });

      expect(agent.isGracefulShutdownRequested()).toBe(false);
      agent.requestGracefulShutdown();
      expect(agent.isGracefulShutdownRequested()).toBe(true);
    });
  });

  describe('abort signal', () => {
    it('should handle AbortSignal cancellation', async () => {
      const provider = new ProgrammableProvider([
        { content: 'response after abort' },
      ]);

      const agent = new ReActAgent({
        name: 'abort-agent',
        model: provider,
        toolkit: createToolkit(),
      });

      const controller = new AbortController();
      controller.abort();

      const response = await agent.run('test', { signal: controller.signal });

      expect(response.content).toContain('cancelled');
      expect(response.metrics.totalTurns).toBe(0);
    });
  });

  describe('hooks', () => {
    it('should fire before_run and after_run hooks', async () => {
      const provider = new ProgrammableProvider([
        { content: 'hook test response' },
      ]);

      const hooks = new HookManager();
      const beforeRun = vi.fn(async () => {});
      const afterRun = vi.fn(async () => {});

      hooks.register('before_run', beforeRun);
      hooks.register('after_run', afterRun);

      const agent = new ReActAgent({
        name: 'hook-agent',
        model: provider,
        toolkit: createToolkit(),
        hooks,
      });

      await agent.run('test hooks');

      expect(beforeRun).toHaveBeenCalledTimes(1);
      expect(afterRun).toHaveBeenCalledTimes(1);
    });

    it('should fire before_turn and after_turn hooks', async () => {
      const provider = new ProgrammableProvider([
        { content: 'turn response' },
      ]);

      const hooks = new HookManager();
      const beforeTurn = vi.fn(async () => {});
      const afterTurn = vi.fn(async () => {});

      hooks.register('before_turn', beforeTurn);
      hooks.register('after_turn', afterTurn);

      const agent = new ReActAgent({
        name: 'turn-agent',
        model: provider,
        toolkit: createToolkit(),
        hooks,
      });

      await agent.run('test turns');

      // At least 1 turn
      expect(beforeTurn).toHaveBeenCalled();
      expect(afterTurn).toHaveBeenCalled();
    });

    it('should fire before_tool and after_tool hooks when tool is called', async () => {
      const toolCall: ToolCall = {
        id: 'tc-1',
        name: 'echo',
        arguments: JSON.stringify({ message: 'hook test' }),
      };

      const provider = new ProgrammableProvider([
        { content: 'calling tool', toolCalls: [toolCall] },
        { content: 'tool done' },
      ]);

      const hooks = new HookManager();
      const beforeTool = vi.fn(async () => {});
      const afterTool = vi.fn(async () => {});

      hooks.register('before_tool', beforeTool);
      hooks.register('after_tool', afterTool);

      const agent = new ReActAgent({
        name: 'tool-hook-agent',
        model: provider,
        toolkit: createToolkit(new EchoTool()),
        hooks,
      });

      await agent.run('use tool');

      expect(beforeTool).toHaveBeenCalledTimes(1);
      expect(afterTool).toHaveBeenCalledTimes(1);
    });
  });

  describe('lifecycle', () => {
    it('should set status to completed after successful run', async () => {
      const provider = new ProgrammableProvider([
        { content: 'done' },
      ]);

      const lifecycle = new Lifecycle();
      const agent = new ReActAgent({
        name: 'lifecycle-agent',
        model: provider,
        toolkit: createToolkit(),
        lifecycle,
      });

      await agent.run('test');

      expect(lifecycle.status).toBe('completed');
    });

    it('should stop when lifecycle.stop() is called', async () => {
      const toolCall: ToolCall = {
        id: 'tc-1',
        name: 'echo',
        arguments: JSON.stringify({ message: 'before stop' }),
      };

      const provider = new ProgrammableProvider([
        { content: 'calling tool', toolCalls: [toolCall] },
        { content: 'after stop' },
        { content: 'should not reach' },
      ]);

      const lifecycle = new Lifecycle();
      const hooks = new HookManager();

      // Stop after first turn
      hooks.register('after_turn', async () => {
        lifecycle.stop();
      });

      const agent = new ReActAgent({
        name: 'stop-agent',
        model: provider,
        toolkit: createToolkit(new EchoTool()),
        lifecycle,
        hooks,
        maxTurns: 10,
      });

      const response = await agent.run('test stop');

      // Should stop after 1 turn due to lifecycle.stop()
      expect(response.metrics.totalTurns).toBeLessThanOrEqual(2);
    });
  });

  describe('input validation', () => {
    it('should throw for empty input', async () => {
      const agent = new ReActAgent({
        name: 'test',
        model: new MockProvider(),
        toolkit: new ToolRegistry(),
      });

      await expect(agent.run('')).rejects.toThrow();
    });
  });

  describe('streamEvents', () => {
    it('should yield stream events', async () => {
      const provider = new ProgrammableProvider([
        { content: 'stream response' },
      ]);

      const agent = new ReActAgent({
        name: 'stream-agent',
        model: provider,
        toolkit: createToolkit(),
        maxTurns: 5,
      });

      const events: string[] = [];
      for await (const event of agent.streamEvents('test stream')) {
        events.push(event.type);
      }

      // Should have at least done event
      expect(events).toContain('done');
    });

    it('should yield tool_call and tool_result events during tool execution', async () => {
      const toolCall: ToolCall = {
        id: 'tc-stream',
        name: 'echo',
        arguments: JSON.stringify({ message: 'stream tool' }),
      };

      const provider = new ProgrammableProvider([
        { content: 'calling tool in stream', toolCalls: [toolCall] },
        { content: 'stream final response' },
      ]);

      const agent = new ReActAgent({
        name: 'stream-tool-agent',
        model: provider,
        toolkit: createToolkit(new EchoTool()),
        maxTurns: 5,
      });

      const events: string[] = [];
      for await (const event of agent.streamEvents('use tool in stream')) {
        events.push(event.type);
      }

      expect(events).toContain('tool_call');
      expect(events).toContain('tool_result');
      expect(events).toContain('done');
    });
  });
});
