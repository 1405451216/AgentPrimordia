import { describe, it, expect, vi } from 'vitest';
import { TurnExecutor } from '../../src/agent/turn-executor.js';
import type { CapabilitiesCache, TurnState } from '../../src/agent/turn-executor.js';
import { HookManager } from '../../src/agent/hooks.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { InMemoryCheckpointStore, CostTracker } from '../../src/agent/request-id.js';
import type { Tool, ToolCall, Message } from '../../src/types.js';

class EchoTool implements Tool {
  name = 'echo';
  description = 'Echo';
  parameters = { type: 'object' as const, properties: { message: { type: 'string' } }, required: ['message'] };
  async execute(args: Record<string, unknown>): Promise<string> { return `echo: ${args.message}`; }
}
class FailTool implements Tool {
  name = 'fail';
  description = 'Fail';
  parameters = { type: 'object' as const, properties: {}, required: [] };
  async execute(): Promise<string> { throw new Error('fail'); }
}
class CalcTool implements Tool {
  name = 'calc';
  description = 'Calc';
  parameters = { type: 'object' as const, properties: { expr: { type: 'string' } }, required: ['expr'] };
  async execute(args: Record<string, unknown>): Promise<string> { return `result: ${args.expr}`; }
}
class ProgProvider extends MockProvider {
  private responses: Array<{ content: string; toolCalls?: ToolCall[] }>;
  private idx = 0;
  constructor(r: Array<{ content: string; toolCalls?: ToolCall[] }>) { super(); this.responses = r; }
  async callTools() {
    const r = this.responses[this.idx] ?? this.responses[this.responses.length - 1];
    this.idx++;
    return { content: r.content, toolCalls: r.toolCalls ?? [], usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 } };
  }
  async complete() {
    const r = this.responses[this.idx] ?? this.responses[this.responses.length - 1];
    this.idx++;
    return { id: `r${this.idx}`, content: r.content, role: 'assistant' as const, usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 } };
  }
  info() { return { name: 'pm', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }; }
}
function reg(...t: Tool[]) { const r = new ToolRegistry(); for (const x of t) r.register(x); return r; }
function caps(): CapabilitiesCache { return { costTracker: null, memoryStore: null, checkpointStore: null, otelBridge: null }; }
function state(): TurnState { return { consecutiveFailures: 0, totalLLMLatency: 0, totalToolLatency: 0, toolCount: 0, pendingMemoryWrites: [] }; }

describe('TurnExec', () => {
  it('no tool calls - stops', async () => {
    const p = new ProgProvider([{ content: 'Hello!' }]);
    const e = new TurnExecutor(p, reg(), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const r = await e.executeTurn([{ role: 'user' as const, content: 'hi' }], 0, state(), Date.now());
    expect(r.shouldStop).toBe(true);
    expect(r.stopReason).toBe('no_more_tools');
    expect(r.response?.content).toBe('Hello!');
  });

  it('emits token event', async () => {
    const p = new ProgProvider([{ content: 'tok' }]);
    const e = new TurnExecutor(p, reg(), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const r = await e.executeTurn([{ role: 'user' as const, content: 'hi' }], 0, state(), Date.now());
    expect(r.events).toContainEqual({ type: 'token', content: 'tok' });
  });

  it('serial tool execution', async () => {
    const tc: ToolCall = { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'hi' }) };
    const p = new ProgProvider([{ content: 'calling', toolCalls: [tc] }]);
    const e = new TurnExecutor(p, reg(new EchoTool()), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const msgs = [{ role: 'user' as const, content: 'use echo' }];
    const s = state();
    const r = await e.executeTurn(msgs, 0, s, Date.now());
    expect(r.shouldStop).toBe(false);
    expect(s.toolCount).toBe(1);
    expect(msgs.length).toBe(3);
    expect(msgs[2]).toMatchObject({ role: 'tool', content: 'echo: hi' });
  });

  it('emits tool_call and tool_result events', async () => {
    const tc: ToolCall = { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'x' }) };
    const p = new ProgProvider([{ content: 'c', toolCalls: [tc] }]);
    const e = new TurnExecutor(p, reg(new EchoTool()), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const r = await e.executeTurn([{ role: 'user' as const, content: 'use' }], 0, state(), Date.now());
    expect(r.events).toContainEqual({ type: 'tool_call', toolCall: tc, turn: 0 });
    const tr = r.events.find((e: any) => e.type === 'tool_result') as any;
    expect(tr.result.content).toBe('echo: x');
  });

  it('parallel tool execution', async () => {
    const calls: ToolCall[] = [
      { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'a' }) },
      { id: 'tc2', name: 'calc', arguments: JSON.stringify({ expr: '1' }) },
    ];
    const p = new ProgProvider([{ content: 'two', toolCalls: calls }]);
    const e = new TurnExecutor(p, reg(new EchoTool(), new CalcTool()), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: true, maxParallelTools: 0, maxMessages: 80 });
    const s = state();
    const r = await e.executeTurn([{ role: 'user' as const, content: 'two' }], 0, s, Date.now());
    expect(r.shouldStop).toBe(false);
    expect(s.toolCount).toBe(2);
  });

  it('consecutive failures stop', async () => {
    const calls: ToolCall[] = [
      { id: 'tc1', name: 'fail', arguments: '{}' },
      { id: 'tc2', name: 'fail', arguments: '{}' },
      { id: 'tc3', name: 'fail', arguments: '{}' },
    ];
    const p = new ProgProvider([{ content: 'f', toolCalls: calls }]);
    const e = new TurnExecutor(p, reg(new FailTool()), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const r = await e.executeTurn([{ role: 'user' as const, content: 'f' }], 0, state(), Date.now());
    expect(r.shouldStop).toBe(true);
    expect(r.stopReason).toBe('consecutive_failures');
    expect(r.response?.content).toContain('3 consecutive tool failures');
  });

  it('checkpoint saved at final answer', async () => {
    const cp = new InMemoryCheckpointStore();
    const c = caps(); c.checkpointStore = cp;
    const p = new ProgProvider([{ content: 'done' }]);
    const e = new TurnExecutor(p, reg(), new HookManager(), c,
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    await e.executeTurn([{ role: 'user' as const, content: 'hi' }], 0, state(), Date.now());
    const cps = await cp.list('s1');
    expect(cps.length).toBe(1);
    expect(cps[0].turn).toBe(1);
  });

  it('checkpoint saved after tool execution', async () => {
    const cp = new InMemoryCheckpointStore();
    const c = caps(); c.checkpointStore = cp;
    const tc: ToolCall = { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'x' }) };
    const p = new ProgProvider([{ content: 'c', toolCalls: [tc] }]);
    const e = new TurnExecutor(p, reg(new EchoTool()), new HookManager(), c,
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    await e.executeTurn([{ role: 'user' as const, content: 'e' }], 0, state(), Date.now());
    const cps = await cp.list('s1');
    expect(cps.length).toBe(1);
  });

  it('cost budget stops', async () => {
    const ct = new CostTracker(undefined, { maxCost: 0.001 });
    ct.record('t', 'm', 1000000, 0);
    const c = caps(); c.costTracker = ct;
    const p = new ProgProvider([{ content: 'nope' }]);
    const e = new TurnExecutor(p, reg(), new HookManager(), c,
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const r = await e.executeTurn([{ role: 'user' as const, content: 'hi' }], 0, state(), Date.now());
    expect(r.shouldStop).toBe(true);
    expect(r.stopReason).toBe('cost_budget');
  });

  it('graceful shutdown stops', async () => {
    const tc: ToolCall = { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'x' }) };
    const p = new ProgProvider([{ content: 'c', toolCalls: [tc] }]);
    const hk = new HookManager();
    let gs = false;
    const e = new TurnExecutor(p, reg(new EchoTool()), hk, caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80,
        isGracefulShutdownRequested: () => gs });
    gs = true;
    const r = await e.executeTurn([{ role: 'user' as const, content: 'e' }], 0, state(), Date.now());
    expect(r.shouldStop).toBe(true);
    expect(r.stopReason).toBe('graceful_shutdown');
  });

  it('fires hooks', async () => {
    const p = new ProgProvider([{ content: 'd' }]);
    const hk = new HookManager();
    const bt = vi.fn(async () => {});
    const at = vi.fn(async () => {});
    hk.register('before_turn', bt);
    hk.register('after_turn', at);
    const e = new TurnExecutor(p, reg(), hk, caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    await e.executeTurn([{ role: 'user' as const, content: 'h' }], 0, state(), Date.now());
    expect(bt).toHaveBeenCalledTimes(1);
    expect(at).toHaveBeenCalledTimes(1);
  });

  it('fires tool hooks', async () => {
    const tc: ToolCall = { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'x' }) };
    const p = new ProgProvider([{ content: 'c', toolCalls: [tc] }]);
    const hk = new HookManager();
    const bft = vi.fn(async () => {});
    const aft = vi.fn(async () => {});
    hk.register('before_tool', bft);
    hk.register('after_tool', aft);
    const e = new TurnExecutor(p, reg(new EchoTool()), hk, caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    await e.executeTurn([{ role: 'user' as const, content: 'e' }], 0, state(), Date.now());
    expect(bft).toHaveBeenCalledTimes(1);
    expect(aft).toHaveBeenCalledTimes(1);
  });

  it('fires on_complete hook', async () => {
    const p = new ProgProvider([{ content: 'f' }]);
    const hk = new HookManager();
    const oc = vi.fn(async () => {});
    hk.register('on_complete', oc);
    const e = new TurnExecutor(p, reg(), hk, caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    await e.executeTurn([{ role: 'user' as const, content: 'h' }], 0, state(), Date.now());
    expect(oc).toHaveBeenCalledTimes(1);
  });

  it('executeTools serial', async () => {
    const calls: ToolCall[] = [
      { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'a' }) },
      { id: 'tc2', name: 'echo', arguments: JSON.stringify({ message: 'b' }) },
    ];
    const p = new MockProvider();
    const e = new TurnExecutor(p, reg(new EchoTool()), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const msgs: Message[] = [{ role: 'user', content: 'hi' }];
    const r = await e.executeTools(msgs, calls, 0, 'serial', state());
    expect(r.toolCount).toBe(2);
    expect(r.shouldStop).toBe(false);
  });

  it('executeTools parallel', async () => {
    const calls: ToolCall[] = [
      { id: 'tc1', name: 'echo', arguments: JSON.stringify({ message: 'a' }) },
      { id: 'tc2', name: 'echo', arguments: JSON.stringify({ message: 'b' }) },
    ];
    const p = new MockProvider();
    const e = new TurnExecutor(p, reg(new EchoTool()), new HookManager(), caps(),
      { name: 't', sessionId: 's1', maxConsecutiveFailures: 3, parallelToolExecution: false, maxParallelTools: 0, maxMessages: 80 });
    const msgs: Message[] = [{ role: 'user', content: 'hi' }];
    const r = await e.executeTools(msgs, calls, 0, 'parallel', state());
    expect(r.toolCount).toBe(2);
    expect(r.shouldStop).toBe(false);
  });
});
