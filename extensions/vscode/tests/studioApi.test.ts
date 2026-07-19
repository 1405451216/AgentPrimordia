/**
 * StudioApi 客户端测试。
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { StudioApi, StudioApiError, type Run } from '../src/studioApi.js';

// 全局 fetch stub
const mockFetch = vi.fn();
global.fetch = mockFetch;

function jsonResponse(data: unknown, ok = true, status = 200): Response {
  return {
    ok,
    status,
    statusText: ok ? 'OK' : 'Error',
    json: async () => data,
    text: async () => (typeof data === 'string' ? data : JSON.stringify(data)),
  } as Response;
}

describe('StudioApi', () => {
  let api: StudioApi;

  beforeEach(() => {
    mockFetch.mockReset();
    api = new StudioApi('http://localhost:8765', 'test-key');
  });

  it('getRuns 发送 GET 请求并解析响应', async () => {
    const fake = { items: [{ id: 'r1' } as Run], total: 1 };
    mockFetch.mockResolvedValue(jsonResponse(fake));

    const res = await api.getRuns(10);
    expect(res.items).toHaveLength(1);
    expect(res.items[0].id).toBe('r1');
    expect(mockFetch).toHaveBeenCalled();
    const [url, init] = mockFetch.mock.calls[0];
    expect(url).toContain('/api/runs?limit=10');
    expect(init.headers['Authorization']).toBe('Bearer test-key');
  });

  it('getRun 按 id 查询', async () => {
    const fake = { id: 'r2', template: 't' } as Run;
    mockFetch.mockResolvedValue(jsonResponse(fake));

    const res = await api.getRun('r2');
    expect(res.id).toBe('r2');
  });

  it('startRun 发送 POST 请求', async () => {
    const fake = { id: 'r3', template: 'tpl' } as Run;
    mockFetch.mockResolvedValue(jsonResponse(fake));

    const res = await api.startRun('tpl', 'hello');
    expect(res.id).toBe('r3');
    const [, init] = mockFetch.mock.calls[0];
    expect(init.method).toBe('POST');
    expect(init.body).toContain('hello');
  });

  it('HTTP 错误抛出 StudioApiError', async () => {
    mockFetch.mockResolvedValue(jsonResponse('not found', false, 404));
    await expect(api.getRun('nope')).rejects.toBeInstanceOf(StudioApiError);
    await expect(api.getRun('nope')).rejects.toMatchObject({ status: 404 });
  });

  it('无 baseUrl 尾部斜杠也能正常工作', async () => {
    mockFetch.mockResolvedValue(jsonResponse({ items: [], total: 0 }));
    await new StudioApi('http://host/', '').getRuns();
    const [url] = mockFetch.mock.calls[0];
    expect(url).toBe('http://host/api/runs?limit=20');
  });

  it('streamRun 返回 Response', async () => {
    mockFetch.mockResolvedValue(new Response());
    const res = await api.streamRun('r1');
    expect(res).toBeInstanceOf(Response);
  });
});
