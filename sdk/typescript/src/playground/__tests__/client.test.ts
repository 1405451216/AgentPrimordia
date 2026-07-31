/**
 * PlaygroundClient 单元测试。
 *
 * 覆盖：
 * - Agent CRUD（createAgent / deleteAgent / listAgents / getAgent）
 * - 同步对话 chat
 * - 流式对话 streamChat（SSE 解析）
 * - 重连逻辑与超时
 * - 错误响应（404 / 500 / 网络异常）
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { PlaygroundClient } from '../index.js';
import type { AgentInfo, ChatResponse, AgentStats } from '../components.js';

// ===== mock fetch =====

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

/** 构造一个模拟 Response */
function mockResponse(body: unknown, init: { status?: number; headers?: Record<string, string> } = {}) {
  const status = init.status ?? 200;
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (k: string) => init.headers?.[k] ?? null },
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(typeof body === 'string' ? body : JSON.stringify(body)),
    body: null as ReadableStream<Uint8Array> | null,
  };
}

/** 构造一个 SSE ReadableStream */
function sseStream(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  let idx = 0;
  return new ReadableStream({
    pull(controller) {
      if (idx < chunks.length) {
        controller.enqueue(encoder.encode(chunks[idx++]));
      } else {
        controller.close();
      }
    },
  });
}

// ===== helpers =====

const API_BASE = 'http://localhost:8080';
const MODEL = 'gpt-4';

function makeClient(opts?: { timeoutMs?: number }) {
  return new PlaygroundClient({ apiBase: API_BASE, defaultModel: MODEL }, opts);
}

// ===== tests =====

describe('PlaygroundClient', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  // ----- Agent CRUD -----

  describe('createAgent', () => {
    it('应 POST 到正确路径并返回 AgentInfo', async () => {
      const agent: AgentInfo = { id: 'a1', model: MODEL, status: 'idle' };
      mockFetch.mockResolvedValueOnce(mockResponse(agent));

      const client = makeClient();
      const result = await client.createAgent({ name: 'test-agent' });

      expect(result).toEqual(agent);
      expect(mockFetch).toHaveBeenCalledOnce();
      const [url, init] = mockFetch.mock.calls[0];
      expect(url).toBe(`${API_BASE}/api/playground/agents`);
      expect(init.method).toBe('POST');
      const body = JSON.parse(init.body);
      expect(body.name).toBe('test-agent');
      expect(body.model).toBe(MODEL);
    });

    it('应使用 config 中指定的 model 覆盖 defaultModel', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse({ id: 'a2', model: 'claude-3', status: 'idle' }));

      const client = makeClient();
      await client.createAgent({ name: 'custom', model: 'claude-3' });

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.model).toBe('claude-3');
    });
  });

  describe('deleteAgent', () => {
    it('应发送 DELETE 请求', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse(null, { status: 204 }));

      const client = makeClient();
      await client.deleteAgent('a1');

      const [url, init] = mockFetch.mock.calls[0];
      expect(url).toBe(`${API_BASE}/api/playground/agents/a1`);
      expect(init.method).toBe('DELETE');
    });
  });

  describe('listAgents', () => {
    it('应返回 AgentInfo 数组', async () => {
      const agents: AgentInfo[] = [
        { id: 'a1', model: MODEL, status: 'idle' },
        { id: 'a2', model: MODEL, status: 'busy' },
      ];
      mockFetch.mockResolvedValueOnce(mockResponse(agents));

      const client = makeClient();
      const result = await client.listAgents();

      expect(result).toEqual(agents);
      expect(mockFetch.mock.calls[0][1].method).toBe('GET');
    });
  });

  describe('getAgent', () => {
    it('应返回单个 AgentInfo', async () => {
      const agent: AgentInfo = { id: 'a1', model: MODEL, status: 'idle' };
      mockFetch.mockResolvedValueOnce(mockResponse(agent));

      const client = makeClient();
      const result = await client.getAgent('a1');

      expect(result).toEqual(agent);
    });
  });

  // ----- chat -----

  describe('chat', () => {
    it('应 POST 消息并返回 ChatResponse', async () => {
      const chatResp: ChatResponse = { response: 'Hello!', tokens: 10 };
      mockFetch.mockResolvedValueOnce(mockResponse(chatResp));

      const client = makeClient();
      const result = await client.chat('a1', 'Hi');

      expect(result.response).toBe('Hello!');
      expect(result.tokens).toBe(10);
      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.message).toBe('Hi');
    });
  });

  // ----- streamChat (SSE) — 不使用 fake timers -----

  describe('streamChat', () => {
    it('应解析 SSE 事件流', async () => {
      const sseData = [
        'event: token\ndata: {"content":"Hello"}\n\n',
        'event: token\ndata: {"content":" world"}\n\n',
        'event: done\ndata: [DONE]\n\n',
      ];
      const resp = mockResponse(null);
      resp.body = sseStream(sseData);
      mockFetch.mockResolvedValueOnce(resp);

      const client = makeClient();
      const events = [];
      for await (const evt of client.streamChat('a1', 'Hi')) {
        events.push(evt);
      }

      expect(events).toHaveLength(3);
      expect(events[0]).toEqual({ type: 'token', content: 'Hello' });
      expect(events[1]).toEqual({ type: 'token', content: ' world' });
      expect(events[2]).toEqual({ type: 'done' });
    });

    it('应解析 tool_call 事件', async () => {
      const sseData = [
        'event: tool_call\ndata: {"tool":"search","args":{"q":"test"}}\n\n',
        'event: done\ndata: [DONE]\n\n',
      ];
      const resp = mockResponse(null);
      resp.body = sseStream(sseData);
      mockFetch.mockResolvedValueOnce(resp);

      const client = makeClient();
      const events = [];
      for await (const evt of client.streamChat('a1', 'search')) {
        events.push(evt);
      }

      expect(events[0]).toEqual({ type: 'tool_call', tool: 'search', args: { q: 'test' } });
    });

    it('应解析 error 事件', async () => {
      const sseData = ['event: error\ndata: {"message":"rate limited"}\n\n'];
      const resp = mockResponse(null);
      resp.body = sseStream(sseData);
      mockFetch.mockResolvedValueOnce(resp);

      const client = makeClient();
      const events = [];
      for await (const evt of client.streamChat('a1', 'test')) {
        events.push(evt);
      }

      expect(events[0]).toEqual({ type: 'error', message: 'rate limited' });
    });

    it('应在 HTTP 错误时抛出', async () => {
      const resp = mockResponse('Internal Server Error', { status: 500 });
      mockFetch.mockResolvedValueOnce(resp);

      const client = makeClient();
      await expect(async () => {
        for await (const _evt of client.streamChat('a1', 'test')) { /* noop */ }
      }).rejects.toThrow(/HTTP 500/);
    });

    it('应跳过非 JSON 的 data 行', async () => {
      const sseData = [
        'event: token\ndata: not-json\n\n',
        'event: token\ndata: {"content":"ok"}\n\n',
      ];
      const resp = mockResponse(null);
      resp.body = sseStream(sseData);
      mockFetch.mockResolvedValueOnce(resp);

      const client = makeClient();
      const events = [];
      for await (const evt of client.streamChat('a1', 'test')) {
        events.push(evt);
      }

      expect(events).toHaveLength(1);
      expect(events[0]).toEqual({ type: 'token', content: 'ok' });
    });
  });

  // ----- getStats -----

  describe('getStats', () => {
    it('应返回 AgentStats', async () => {
      const stats: AgentStats = { turnCount: 5, totalTokens: 200 };
      mockFetch.mockResolvedValueOnce(mockResponse(stats));

      const client = makeClient();
      const result = await client.getStats('a1');

      expect(result.turnCount).toBe(5);
      expect(result.totalTokens).toBe(200);
    });
  });

  // ----- 错误处理（HTTP 错误不重试） -----

  describe('error handling', () => {
    it('HTTP 404 不应重试，直接抛出', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse('not found', { status: 404 }));

      const client = makeClient();
      await expect(client.getAgent('missing')).rejects.toThrow(/HTTP 404/);
      expect(mockFetch).toHaveBeenCalledOnce();
    });

    it('HTTP 500 不应重试，直接抛出', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse('error', { status: 500 }));

      const client = makeClient();
      await expect(client.chat('a1', 'test')).rejects.toThrow(/HTTP 500/);
      expect(mockFetch).toHaveBeenCalledOnce();
    });
  });

  // ----- 重连（mock setTimeout 加速） -----

  describe('reconnection', () => {
    let originalSetTimeout: typeof globalThis.setTimeout;

    beforeEach(() => {
      originalSetTimeout = globalThis.setTimeout;
      // 将 setTimeout 替换为立即执行，加速重连延迟
      globalThis.setTimeout = ((fn: () => void) => {
        // 使用 queueMicrotask 确保在当前微任务之后执行
        queueMicrotask(fn);
        return 0 as any;
      }) as any;
    });

    afterEach(() => {
      globalThis.setTimeout = originalSetTimeout;
    });

    it('网络错误应重试最多 3 次后抛出', async () => {
      mockFetch.mockRejectedValue(new TypeError('fetch failed'));

      const client = makeClient();
      await expect(client.listAgents()).rejects.toThrow('fetch failed');
      // 1 初始 + 3 重试 = 4 次
      expect(mockFetch).toHaveBeenCalledTimes(4);
    });

    it('网络错误后重连成功应返回结果', async () => {
      const agents: AgentInfo[] = [{ id: 'a1', model: MODEL, status: 'idle' }];
      mockFetch
        .mockRejectedValueOnce(new TypeError('network down'))
        .mockResolvedValueOnce(mockResponse(agents));

      const client = makeClient();
      const result = await client.listAgents();
      expect(result).toEqual(agents);
    });
  });

  // ----- 超时 -----

  describe('timeout', () => {
    it('应使用自定义超时', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse([]));

      const client = makeClient({ timeoutMs: 5000 });
      await client.listAgents();

      const init = mockFetch.mock.calls[0][1];
      expect(init.signal).toBeDefined();
    });
  });

  // ----- apiBase 尾部斜杠处理 -----

  describe('apiBase normalization', () => {
    it('应去除尾部斜杠', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse([]));

      const client = new PlaygroundClient({ apiBase: 'http://localhost:8080/', defaultModel: MODEL });
      await client.listAgents();

      const url = mockFetch.mock.calls[0][0];
      expect(url).toBe('http://localhost:8080/api/playground/agents');
    });
  });
});
