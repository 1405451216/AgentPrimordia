import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { PlaygroundClient } from '../../src/playground/index.js';

function mockFetch(handlers: Record<string, Response | (() => Response)>): void {
  const fn = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit): Promise<Response> => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : (input as Request).url;
    for (const key of Object.keys(handlers)) {
      if (url.includes(key)) {
        const h = handlers[key];
        return typeof h === 'function' ? h() : h;
      }
    }
    return new Response('not found', { status: 404 });
  });
  globalThis.fetch = fn as unknown as typeof fetch;
}

function sseStream(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
}

describe('PlaygroundClient', () => {
  const BASE = 'http://localhost:8080';
  const MODEL = 'gpt-4';

  beforeEach(() => { vi.restoreAllMocks(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('creates an agent', async () => {
    mockFetch({
      '/api/playground/agents': new Response(JSON.stringify({ id: 'agent-1', model: MODEL, status: 'idle' }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' },
      }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    const agent = await pg.createAgent({ name: 'test-agent' });
    expect(agent).toEqual({ id: 'agent-1', model: MODEL, status: 'idle' });
  });

  it('lists agents', async () => {
    const list = [
      { id: 'a1', model: MODEL, status: 'idle' },
      { id: 'a2', model: MODEL, status: 'running' },
    ];
    mockFetch({
      '/api/playground/agents': new Response(JSON.stringify(list), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    const agents = await pg.listAgents();
    expect(agents).toHaveLength(2);
    expect(agents[0].id).toBe('a1');
  });

  it('chats synchronously', async () => {
    mockFetch({
      '/chat': new Response(JSON.stringify({ response: 'Hello, user!', tokens: 42 }), { status: 200 }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    const reply = await pg.chat('agent-1', 'Hi');
    expect(reply.response).toBe('Hello, user!');
    expect(reply.tokens).toBe(42);
  });

  it('streams chat with SSE token events', async () => {
    const sseData = [
      'event: token\\ndata: {"content":"Hello"}\\n\\n',
      'event: token\\ndata: {"content":" world"}\\n\\n',
      'event: done\\ndata: [DONE]\\n\\n',
    ];
    mockFetch({
      '/stream': new Response(sseStream(sseData) as any, { status: 200 }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    const events: any[] = [];
    for await (const ev of pg.streamChat('agent-1', 'Hi')) {
      events.push(ev);
    }
    expect(events).toEqual([
      { type: 'token', content: 'Hello' },
      { type: 'token', content: ' world' },
      { type: 'done' },
    ]);
  });

  it('streams chat with tool_call events', async () => {
    const sseData = [
      'event: token\\ndata: {"content":"Let me check"}\\n\\n',
      'event: tool_call\\ndata: {"tool":"shell","args":{"cmd":"ls"}}\\n\\n',
      'event: tool_call\\ndata: {"tool":"filesystem","args":{"path":"/tmp"}}\\n\\n',
      'event: done\\ndata: [DONE]\\n\\n',
    ];
    mockFetch({
      '/stream': new Response(sseStream(sseData) as any, { status: 200 }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    const events: any[] = [];
    for await (const ev of pg.streamChat('agent-1', 'run ls')) {
      events.push(ev);
    }
    expect(events).toContainEqual({ type: 'tool_call', tool: 'shell', args: { cmd: 'ls' } });
    expect(events).toContainEqual({ type: 'tool_call', tool: 'filesystem', args: { path: '/tmp' } });
  });

  it('streamEvents subscribes to Agent events', async () => {
    const sseData = [
      'event: token\\ndata: {"content":"thinking..."}\\n\\n',
      'event: error\\ndata: {"message":"timeout"}\\n\\n',
      'event: done\\ndata: [DONE]\\n\\n',
    ];
    mockFetch({
      '/events': new Response(sseStream(sseData) as any, { status: 200 }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    const events: any[] = [];
    for await (const ev of pg.streamEvents('agent-1')) {
      events.push(ev);
    }
    expect(events).toEqual([
      { type: 'token', content: 'thinking...' },
      { type: 'error', message: 'timeout' },
      { type: 'done' },
    ]);
  });

  it('gets agent stats', async () => {
    mockFetch({
      '/stats': new Response(JSON.stringify({ turnCount: 5, totalTokens: 1200 }), { status: 200 }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    const stats = await pg.getStats('agent-1');
    expect(stats.turnCount).toBe(5);
    expect(stats.totalTokens).toBe(1200);
  });

  it('deletes an agent', async () => {
    mockFetch({
      '/api/playground/agents/agent-1': new Response(null, { status: 204 }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    await expect(pg.deleteAgent('agent-1')).resolves.toBeUndefined();
  });

  it('throws on API error', async () => {
    mockFetch({
      '/api/playground/agents': new Response('Internal Server Error', { status: 500 }),
    });
    const pg = new PlaygroundClient({ apiBase: BASE, defaultModel: MODEL });
    await expect(pg.createAgent({ name: 'fail' })).rejects.toThrow(/500/);
  });
});
