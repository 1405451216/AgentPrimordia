import { describe, it, expect } from 'vitest';
import { ReActAgent, MockProvider, ToolRegistry, HookManager, Lifecycle, InMemoryStore, VectorStore, Bus, ACL, Sandbox, MetricsCollector } from '../src/index.js';
import type { Tool, Event } from '../src/index.js';

class EchoTool implements Tool {
  name = 'echo';
  description = 'Echo the input';
  parameters = { type: 'object', properties: { text: { type: 'string' } } };

  async execute(args: Record<string, unknown>): Promise<string> {
    return `Echo: ${args.text}`;
  }
}

describe('ReActAgent', () => {
  it('should run a simple agent', async () => {
    const provider = new MockProvider({ response: 'Hello!' });
    const registry = new ToolRegistry();

    const agent = new ReActAgent({
      name: 'test-agent',
      model: provider,
      toolkit: registry,
      maxTurns: 5,
    });

    const response = await agent.run('Hi');
    expect(response.content).toBe('Hello!');
    expect(response.metrics.totalTurns).toBeGreaterThanOrEqual(1);
  });

  it('should fire hooks', async () => {
    const provider = new MockProvider({ response: 'hook test' });
    const registry = new ToolRegistry();
    const hooks = new HookManager();

    const fired: string[] = [];
    hooks.register('before_run', () => fired.push('before_run'));
    hooks.register('after_llm', () => fired.push('after_llm'));
    hooks.register('on_complete', () => fired.push('on_complete'));

    const agent = new ReActAgent({
      name: 'hook-agent',
      model: provider,
      toolkit: registry,
      hooks,
    });

    await agent.run('test');
    expect(fired).toContain('before_run');
    expect(fired).toContain('after_llm');
    expect(fired).toContain('on_complete');
  });

  it('should respect lifecycle stop', async () => {
    const provider = new MockProvider({ response: 'stopped' });
    const registry = new ToolRegistry();
    const lifecycle = new Lifecycle();

    const agent = new ReActAgent({
      name: 'stop-agent',
      model: provider,
      toolkit: registry,
      lifecycle,
    });

    lifecycle.stop();
    const response = await agent.run('test');
    expect(response.metrics.totalTurns).toBe(0);
  });

  it('should use system prompt', async () => {
    const provider = new MockProvider({ response: 'system response' });
    const registry = new ToolRegistry();

    const agent = new ReActAgent({
      name: 'system-agent',
      model: provider,
      toolkit: registry,
      systemPrompt: 'You are a helpful assistant',
    });

    const response = await agent.run('Hi');
    expect(response.content).toBe('system response');
  });
});

describe('MockProvider', () => {
  it('should return mock response', async () => {
    const provider = new MockProvider({ response: 'test' });
    const resp = await provider.complete({ messages: [{ role: 'user', content: 'Hi' }] });
    expect(resp.content).toBe('test');
  });

  it('should throw error when configured', async () => {
    const provider = new MockProvider({ error: true });
    await expect(provider.complete({ messages: [{ role: 'user', content: 'Hi' }] })).rejects.toThrow('mock error');
  });

  it('should return tool calls', async () => {
    const provider = new MockProvider({
      toolCalls: [{ id: 'c1', name: 'test', arguments: '{}' }],
    });
    const resp = await provider.callTools({
      messages: [{ role: 'user', content: 'Hi' }],
      tools: [{ type: 'function', function: { name: 'test', description: 'desc', parameters: {} } }],
    });
    expect(resp.toolCalls).toHaveLength(1);
  });

  it('should stream response', async () => {
    const provider = new MockProvider({ response: 'hello world' });
    const chunks: string[] = [];
    for await (const chunk of provider.stream!({ messages: [{ role: 'user', content: 'Hi' }] })) {
      chunks.push(chunk.content);
    }
    expect(chunks.join('').trim()).toBe('hello world');
  });
});

describe('ToolRegistry', () => {
  it('should register and execute tools', async () => {
    const registry = new ToolRegistry();
    registry.register(new EchoTool());

    expect(registry.has('echo')).toBe(true);
    const result = await registry.execute({ id: '1', name: 'echo', arguments: '{"text":"hi"}' });
    expect(result.content).toBe('Echo: hi');
    expect(result.isError).toBe(false);
  });

  it('should return error for unknown tool', async () => {
    const registry = new ToolRegistry();
    const result = await registry.execute({ id: '1', name: 'unknown', arguments: '{}' });
    expect(result.isError).toBe(true);
  });

  it('should list definitions', () => {
    const registry = new ToolRegistry();
    registry.register(new EchoTool());
    const defs = registry.definitions();
    expect(defs).toHaveLength(1);
    expect(defs[0].function.name).toBe('echo');
  });
});

describe('HookManager', () => {
  it('should register and fire hooks', async () => {
    const hooks = new HookManager();
    let called = false;
    hooks.register('before_run', () => { called = true; });
    await hooks.fire({ agentID: 'a', sessionID: '', point: 'before_run', turn: 0 });
    expect(called).toBe(true);
  });

  it('should support async hooks', async () => {
    const hooks = new HookManager();
    let called = false;
    hooks.register('after_llm', async () => { called = true; });
    await hooks.fire({ agentID: 'a', sessionID: '', point: 'after_llm', turn: 0 });
    expect(called).toBe(true);
  });

  it('should remove hooks', () => {
    const hooks = new HookManager();
    hooks.register('before_run', () => {});
    hooks.remove('before_run');
    expect(hooks.count('before_run')).toBe(0);
  });
});

describe('Lifecycle', () => {
  it('should start as idle', () => {
    const lc = new Lifecycle();
    expect(lc.status).toBe('idle');
  });

  it('should transition status', () => {
    const lc = new Lifecycle();
    lc.setStatus('running');
    expect(lc.status).toBe('running');
  });

  it('should support stop', () => {
    const lc = new Lifecycle();
    lc.stop();
    expect(lc.isStopped()).toBe(true);
  });

  it('should support pause and resume', () => {
    const lc = new Lifecycle();
    lc.setStatus('running');
    lc.pause();
    expect(lc.status).toBe('paused');
    lc.resume();
    expect(lc.status).toBe('running');
  });
});

describe('InMemoryStore', () => {
  it('should add and get episodes', async () => {
    const store = new InMemoryStore();
    await store.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello', createdAt: new Date().toISOString() });
    const ep = await store.get('1');
    expect(ep?.content).toBe('hello');
  });

  it('should search episodes', async () => {
    const store = new InMemoryStore();
    await store.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello world', createdAt: new Date().toISOString() });
    await store.add({ id: '2', sessionId: 's1', role: 'user', content: 'goodbye', createdAt: new Date().toISOString() });
    const results = await store.search('hello');
    expect(results).toHaveLength(1);
  });

  it('should return stats', async () => {
    const store = new InMemoryStore();
    await store.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello', createdAt: new Date().toISOString() });
    const stats = await store.stats();
    expect(stats.totalEpisodes).toBe(1);
  });
});

describe('VectorStore', () => {
  it('should add and search vectors', () => {
    const store = new VectorStore(4);
    store.add('doc1', [1, 0, 0, 0]);
    store.add('doc2', [0, 1, 0, 0]);
    const results = store.search([1, 0, 0, 0], 2);
    expect(results[0].id).toBe('doc1');
    expect(results[0].score).toBeCloseTo(1.0);
  });

  it('should reject dimension mismatch', () => {
    const store = new VectorStore(4);
    expect(() => store.add('bad', [1, 0, 0])).toThrow('dimension mismatch');
  });
});

describe('EventBus', () => {
  it('should publish and receive events', () => {
    const bus = new Bus();
    const received: Event[] = [];
    bus.subscribe('agent.start', (e) => received.push(e));
    bus.publish({ id: '1', type: 'agent.start', source: 'test', timestamp: new Date() });
    expect(received).toHaveLength(1);
  });

  it('should support wildcard subscription', () => {
    const bus = new Bus();
    const received: Event[] = [];
    bus.subscribeAll((e) => received.push(e));
    bus.publish({ id: '1', type: 'agent.start', source: 'test', timestamp: new Date() });
    bus.publish({ id: '2', type: 'tool.call', source: 'test', timestamp: new Date() });
    expect(received).toHaveLength(2);
  });
});

describe('Security', () => {
  it('should enforce ACL rules', () => {
    const acl = new ACL();
    acl.allow('agent-1', '/src/', 'read');
    expect(acl.check('agent-1', '/src/main.go', 'read')).toBe(true);
    expect(acl.check('agent-1', '/src/main.go', 'write')).toBe(false);
    expect(acl.check('agent-2', '/src/main.go', 'read')).toBe(false);
  });

  it('should enforce deny rules', () => {
    const acl = new ACL();
    acl.allow('agent-1', '/src/', 'all');
    acl.deny('agent-1', '/src/secret.key');
    expect(acl.check('agent-1', '/src/secret.key', 'read')).toBe(false);
    expect(acl.check('agent-1', '/src/main.go', 'read')).toBe(true);
  });

  it('should sandbox commands', () => {
    const acl = new ACL();
    const sb = new Sandbox(acl);
    sb.allowCommand('ls');
    sb.blockCommand('rm');
    expect(sb.canExecute('agent-1', 'ls')).toBeNull();
    expect(sb.canExecute('agent-1', 'rm')?.message).toContain('blocked');
  });

  it('should detect path traversal', () => {
    const acl = new ACL();
    acl.allow('agent-1', '/workspace/', 'all');
    const sb = new Sandbox(acl);
    expect(sb.validatePath('agent-1', '/workspace/../../../etc/passwd', 'read')?.message).toContain('invalid path');
  });
});

describe('MetricsCollector', () => {
  it('should record metrics', () => {
    const m = new MetricsCollector();
    m.recordLLMCall(100);
    m.recordLLMCall(200, new Error('fail'));
    m.recordToolCall(50);
    m.incActiveAgents();

    const snap = m.snapshot();
    expect(snap.llm_total_calls).toBe(2);
    expect(snap.llm_total_errors).toBe(1);
    expect(snap.tool_total_calls).toBe(1);
    expect(snap.active_agents).toBe(1);
    expect(snap.avg_llm_latency_ms).toBe(150);
  });
});
