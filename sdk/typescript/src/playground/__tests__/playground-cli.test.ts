/**
 * PlaygroundSession / PlaygroundManager 单元测试。
 *
 * 覆盖：
 * - PlaygroundSession: send (JSON / SSE)、abort、clearHistory、exportAsJSON、streamStats
 * - PlaygroundManager: createAgent、listAgents、getSession
 * - 错误处理
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { PlaygroundSession, PlaygroundManager } from '../playground-cli.js';
import type { PlaygroundAgent } from '../playground-cli.js';

// ===== mock fetch =====

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function mockResponse(body: unknown, init: { status?: number; headers?: Record<string, string> } = {}) {
  const status = init.status ?? 200;
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get: (key: string) => init.headers?.[key] ?? null,
    },
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(typeof body === 'string' ? body : JSON.stringify(body)),
    body: null as ReadableStream<Uint8Array> | null,
  };
}

function makeAgent(overrides: Partial<PlaygroundAgent> = {}): PlaygroundAgent {
  return {
    id: 'agent-1',
    name: 'test-agent',
    model: 'gpt-4',
    status: 'idle',
    turnCount: 0,
    totalTokens: 0,
    ...overrides,
  };
}

// ===== tests =====

describe('PlaygroundSession', () => {
  beforeEach(() => {
    mockFetch.mockReset();
    delete process.env.PLAYGROUND_API;
  });

  describe('send — JSON response', () => {
    it('应发送消息并返回 JSON 回复', async () => {
      mockFetch.mockResolvedValueOnce(
        mockResponse({ response: 'Hi there!' })
      );

      const agent = makeAgent();
      const session = new PlaygroundSession(agent);
      const reply = await session.send('Hello');

      expect(reply).toBe('Hi there!');
      expect(session.messages).toHaveLength(2);
      expect(session.messages[0].role).toBe('user');
      expect(session.messages[1].role).toBe('assistant');
      expect(agent.turnCount).toBe(1);
      expect(agent.status).toBe('idle');
    });

    it('应支持 content 字段作为回复', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse({ content: 'From content field' }));

      const session = new PlaygroundSession(makeAgent());
      const reply = await session.send('test');
      expect(reply).toBe('From content field');
    });
  });

  describe('send — SSE response', () => {
    it('应逐 token 解析流式响应', async () => {
      const encoder = new TextEncoder();
      const chunks = [
        'data: Hello\n\n',
        'data:  world\n\n',
        'data: [DONE]\n\n',
      ];
      let idx = 0;
      const stream = new ReadableStream({
        pull(controller) {
          if (idx < chunks.length) {
            controller.enqueue(encoder.encode(chunks[idx++]));
          } else {
            controller.close();
          }
        },
      });

      const resp = mockResponse(null, {
        headers: { 'content-type': 'text/event-stream' },
      });
      resp.body = stream;
      mockFetch.mockResolvedValueOnce(resp);

      const tokens: string[] = [];
      const session = new PlaygroundSession(makeAgent());
      const reply = await session.send('Hi', (t) => tokens.push(t));

      expect(reply).toBe('Hello world');
      expect(tokens).toEqual(['Hello', ' world']);
      expect(session.messages).toHaveLength(2);
    });
  });

  describe('send — error handling', () => {
    it('应在 HTTP 错误时设置 status 为 error', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse('fail', { status: 500 }));

      const agent = makeAgent();
      const session = new PlaygroundSession(agent);
      await expect(session.send('test')).rejects.toThrow(/HTTP 500/);
      expect(agent.status).toBe('error');
    });
  });

  describe('abort', () => {
    it('应中断请求并返回已收集内容', async () => {
      // 模拟一个永远不会完成的响应
      mockFetch.mockImplementationOnce((_url: string, init: RequestInit) => {
        return new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => {
            const err = new Error('Aborted');
            err.name = 'AbortError';
            reject(err);
          });
        });
      });

      const agent = makeAgent();
      const session = new PlaygroundSession(agent);

      const sendPromise = session.send('test');
      session.abort();

      const result = await sendPromise;
      expect(result).toContain('[aborted]');
      expect(agent.status).toBe('idle');
    });
  });

  describe('clearHistory', () => {
    it('应清空消息列表', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse({ response: 'ok' }));

      const session = new PlaygroundSession(makeAgent());
      await session.send('hello');
      expect(session.messages).toHaveLength(2);

      session.clearHistory();
      expect(session.messages).toHaveLength(0);
    });
  });

  describe('exportAsJSON', () => {
    it('应导出包含 agent 和 messages 的 JSON', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse({ response: 'hi' }));

      const session = new PlaygroundSession(makeAgent());
      await session.send('hello');

      const json = session.exportAsJSON();
      const parsed = JSON.parse(json);
      expect(parsed.agent.id).toBe('agent-1');
      expect(parsed.messages).toHaveLength(2);
    });
  });

  describe('streamStats', () => {
    it('应在 idle 状态下刷新统计', async () => {
      mockFetch.mockResolvedValueOnce(
        mockResponse({ turn_count: 10, total_tokens: 500 })
      );

      const agent = makeAgent();
      const session = new PlaygroundSession(agent);
      await session.streamStats();

      expect(agent.turnCount).toBe(10);
      expect(agent.totalTokens).toBe(500);
    });

    it('非 idle 状态应跳过', async () => {
      const agent = makeAgent({ status: 'thinking' });
      const session = new PlaygroundSession(agent);
      await session.streamStats();

      expect(mockFetch).not.toHaveBeenCalled();
    });
  });
});

describe('PlaygroundManager', () => {
  beforeEach(() => {
    mockFetch.mockReset();
    delete process.env.PLAYGROUND_API;
  });

  describe('createAgent', () => {
    it('应创建 Agent 并返回 ID', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse({ id: 'new-agent' }));

      const manager = new PlaygroundManager();
      const id = await manager.createAgent({ name: 'bot', model: 'gpt-4' });

      expect(id).toBe('new-agent');
      expect(manager.getSession('new-agent')).toBeDefined();
    });

    it('HTTP 错误应抛出', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse('err', { status: 500 }));

      const manager = new PlaygroundManager();
      await expect(
        manager.createAgent({ name: 'bot', model: 'gpt-4' })
      ).rejects.toThrow(/HTTP 500/);
    });
  });

  describe('listAgents', () => {
    it('应返回 Agent 列表', async () => {
      const agents = [
        { id: 'a1', name: 'bot1', model: 'gpt-4', status: 'idle', turnCount: 0, totalTokens: 0 },
      ];
      mockFetch.mockResolvedValueOnce(mockResponse({ agents }));

      const manager = new PlaygroundManager();
      const result = await manager.listAgents();

      expect(result).toEqual(agents);
    });

    it('空列表应返回空数组', async () => {
      mockFetch.mockResolvedValueOnce(mockResponse({}));

      const manager = new PlaygroundManager();
      const result = await manager.listAgents();

      expect(result).toEqual([]);
    });
  });

  describe('getSession', () => {
    it('未找到应返回 undefined', () => {
      const manager = new PlaygroundManager();
      expect(manager.getSession('nonexistent')).toBeUndefined();
    });
  });
});
