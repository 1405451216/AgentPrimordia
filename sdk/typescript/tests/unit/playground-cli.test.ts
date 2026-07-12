import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { PlaygroundSession, PlaygroundManager } from '../../src/playground/playground-cli.js';
import type { PlaygroundAgent } from '../../src/playground/playground-cli.js';

/** 创建测试用 Agent */
function makeAgent(overrides?: Partial<PlaygroundAgent>): PlaygroundAgent {
  return {
    id: 'test-1',
    name: 'Test',
    model: 'gpt-4',
    status: 'idle',
    turnCount: 0,
    totalTokens: 0,
    ...overrides,
  };
}

// ---- PlaygroundSession 基础测试 ----

describe('PlaygroundSession', () => {
  let session: PlaygroundSession;

  beforeEach(() => {
    session = new PlaygroundSession(makeAgent());
  });

  it('should start with empty messages', () => {
    expect(session.messages.length).toBe(0);
  });

  it('should reference the provided agent', () => {
    expect(session.agent.id).toBe('test-1');
    expect(session.agent.name).toBe('Test');
    expect(session.agent.model).toBe('gpt-4');
  });

  it('should export as JSON', () => {
    session.messages.push({ role: 'user', content: 'hi', timestamp: new Date() });
    const json = session.exportAsJSON();
    expect(json).toContain('test-1');
    expect(json).toContain('hi');
  });

  it('should clear history', () => {
    session.messages.push({ role: 'user', content: 'hi', timestamp: new Date() });
    session.clearHistory();
    expect(session.messages.length).toBe(0);
  });

  it('should set status to thinking during send', async () => {
    // 用 spy 拦截 fetch 并返回快速响应
    const origFetch = globalThis.fetch;
    let fetchCalled = false;
    globalThis.fetch = vi.fn(async () => {
      fetchCalled = true;
      // 在 send 中 status 在 fetch 之前就被改为 thinking
      expect(session.agent.status).toBe('thinking');
      return new Response(JSON.stringify({ response: 'OK' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }) as unknown as typeof fetch;

    const result = await session.send('hello');
    expect(fetchCalled).toBe(true);
    expect(result).toBe('OK');
    expect(session.agent.status).toBe('idle');

    globalThis.fetch = origFetch;
  });

  it('should add user message on send', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ response: 'Hi there' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    ) as unknown as typeof fetch;

    const countBefore = session.messages.length;
    await session.send('hello');
    expect(session.messages.length).toBe(countBefore + 2); // user + assistant
    expect(session.messages[session.messages.length - 2].role).toBe('user');
    expect(session.messages[session.messages.length - 2].content).toBe('hello');
    expect(session.messages[session.messages.length - 1].role).toBe('assistant');
  });

  it('should increment turnCount after successful send', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ response: 'OK' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    ) as unknown as typeof fetch;

    expect(session.agent.turnCount).toBe(0);
    await session.send('first');
    expect(session.agent.turnCount).toBe(1);
    await session.send('second');
    expect(session.agent.turnCount).toBe(2);
  });

  it('should handle HTTP error', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('Internal Server Error', { status: 500 })
    ) as unknown as typeof fetch;

    await expect(session.send('fail')).rejects.toThrow(/500/);
    expect(session.agent.status).toBe('error');
  });

  it('should handle SSE streaming', async () => {
    const encoder = new TextEncoder();
    const sseData = 'data: Hello\ndata:  World\ndata: [DONE]\n';
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(sseData));
        controller.close();
      },
    });

    globalThis.fetch = vi.fn(async () =>
      new Response(stream as any, {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      })
    ) as unknown as typeof fetch;

    const tokens: string[] = [];
    const result = await session.send('stream test', (token) => tokens.push(token));
    expect(tokens).toContain('Hello');
    expect(tokens).toContain(' World');
    expect(result).toBe('Hello World');
  });
});

// ---- PlaygroundSession.streamStats 测试 ----

describe('PlaygroundSession.streamStats', () => {
  let session: PlaygroundSession;

  beforeEach(() => {
    session = new PlaygroundSession(makeAgent());
  });

  it('should update stats when idle', async () => {
    expect(session.agent.status).toBe('idle');
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ turn_count: 5, total_tokens: 1200 }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    ) as unknown as typeof fetch;

    await session.streamStats();
    expect(session.agent.turnCount).toBe(5);
    expect(session.agent.totalTokens).toBe(1200);
  });

  it('should skip when not idle', async () => {
    session.agent.status = 'thinking';
    const spy = vi.fn();
    globalThis.fetch = spy as unknown as typeof fetch;

    await session.streamStats();
    expect(spy).not.toHaveBeenCalled();
  });
});

// ---- PlaygroundManager 测试 ----

describe('PlaygroundManager', () => {
  it('should initialize with empty sessions', () => {
    const mgr = new PlaygroundManager();
    expect(mgr.sessions.size).toBe(0);
  });

  it('should create agent and register session', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ id: 'agent-new' }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' },
      })
    ) as unknown as typeof fetch;

    const mgr = new PlaygroundManager();
    const id = await mgr.createAgent({ name: 'bot', model: 'gpt-4' });
    expect(id).toBe('agent-new');
    expect(mgr.sessions.size).toBe(1);
    expect(mgr.getSession('agent-new')).toBeDefined();
  });

  it('should list agents from remote', async () => {
    const list = [
      { id: 'a1', name: 'A', model: 'gpt-4', status: 'idle', turnCount: 0, totalTokens: 0 },
      { id: 'a2', name: 'B', model: 'gpt-4', status: 'idle', turnCount: 0, totalTokens: 0 },
    ];
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ agents: list }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    ) as unknown as typeof fetch;

    const mgr = new PlaygroundManager();
    const agents = await mgr.listAgents();
    expect(agents).toHaveLength(2);
    expect(agents[0].id).toBe('a1');
  });

  it('should throw on create failure', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('Server Error', { status: 500 })
    ) as unknown as typeof fetch;

    const mgr = new PlaygroundManager();
    await expect(mgr.createAgent({ name: 'bad', model: 'gpt-4' })).rejects.toThrow(/500/);
  });
});