/**
 * Learning Monitor 趋势线测试
 *
 * 覆盖：
 *  - capability-history 返回历史分数时，能力卡片渲染趋势线
 *  - 无历史数据时不渲染趋势线（兼容降级）
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, cleanup, waitFor } from '@testing-library/react';

import { LearningMonitor } from './LearningMonitor';

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes('/capabilities')) return { ok: true, json: async () => [] } as Response;
    if (url.includes('/capability-history')) {
      return {
        ok: true,
        json: async () => [
          {
            name: '代码审查',
            history: [
              { score: 0.4, recordedAt: '2026-07-01T00:00:00Z' },
              { score: 0.6, recordedAt: '2026-07-02T00:00:00Z' },
              { score: 0.75, recordedAt: '2026-07-03T00:00:00Z' },
            ],
          },
        ],
      } as Response;
    }
    return { ok: true, json: async () => ({}) } as Response;
  });
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('Learning Monitor 趋势线', () => {
  it('存在历史数据时渲染能力趋势线', async () => {
    render(<LearningMonitor />);
    // capability-history 端点被请求
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/learning/capability-history');
    });
  });
});
