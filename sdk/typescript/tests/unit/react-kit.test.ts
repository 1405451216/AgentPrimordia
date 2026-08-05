/**
 * 零样板 React Hooks kit 测试（v3.7-4）。
 */
import { describe, it, expect } from 'vitest';
import { useAgent, useReActLoop, useRemoteAgent, DefaultStreamer } from '../../src/react/kit.js';
import { ReActAgent } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';

describe('React kit exports', () => {
  it('should export zero-boilerplate hooks as functions', () => {
    expect(typeof useAgent).toBe('function');
    expect(typeof useReActLoop).toBe('function');
    expect(typeof useRemoteAgent).toBe('function');
  });
});

describe('DefaultStreamer', () => {
  function buildAgent(provider: MockProvider): ReActAgent {
    return new ReActAgent({
      name: 'kit-agent',
      model: provider,
      toolkit: new ToolRegistry(),
      maxTurns: 3,
    });
  }

  it('should map streamEvents complete to done event', async () => {
    const provider = new MockProvider({ response: 'kit done' });
    const streamer = new DefaultStreamer(buildAgent(provider));

    const events: string[] = [];
    for await (const ev of streamer.stream('hello')) {
      events.push(ev.type);
    }
    expect(events).toContain('done');
    expect(events).toContain('thought');
  });

  it('should yield action events for tool calls', async () => {
    const provider = new MockProvider({
      toolCalls: [{ id: 't1', name: 'echo', arguments: '{"text":"hi"}' }],
    });
    const registry = new ToolRegistry();
    registry.register({
      name: 'echo',
      description: 'Echo',
      parameters: { type: 'object', properties: { text: { type: 'string' } } },
      async execute(args: Record<string, unknown>) {
        return `echo: ${args.text ?? ''}`;
      },
    });
    const agent = new ReActAgent({
      name: 'kit-agent-tool',
      model: provider,
      toolkit: registry,
      maxTurns: 3,
    });
    const streamer = new DefaultStreamer(agent);

    const actions: string[] = [];
    for await (const ev of streamer.stream('use tool')) {
      if (ev.type === 'action') actions.push(ev.tool);
    }
    expect(actions).toContain('echo');
  });

  it('should map errors to error events', async () => {
    const provider = new MockProvider({ error: true });
    const streamer = new DefaultStreamer(buildAgent(provider));

    const errors: string[] = [];
    for await (const ev of streamer.stream('boom')) {
      if (ev.type === 'error') errors.push(ev.error.message);
    }
    expect(errors.length).toBeGreaterThanOrEqual(1);
  });
});
