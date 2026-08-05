/**
 * Marketplace 加固行为测试
 *
 * 覆盖：
 *  - 部署成功展示成功横幅
 *  - 部署失败展示错误横幅，且关闭后消失
 *  - 模板加载失败展示错误面板 + 重试
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { MarketplacePage } from './MarketplacePage';

const TEMPLATES = [
  {
    id: 't1',
    name: 'CODE REVIEWER',
    description: '自动审查 PR 代码',
    version: '1.0.0',
    author: 'agentprimordia',
    category: 'coding',
    tags: ['code-review'],
    rating: 4.8,
    downloads: 1280,
  },
];

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST' && url.includes('/deploy')) {
      return { ok: true, json: async () => ({}) } as Response;
    }
    if (url.includes('/templates')) {
      return { ok: true, json: async () => TEMPLATES } as Response;
    }
    return { ok: true, json: async () => ({}) } as Response;
  });
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('Marketplace 加固', () => {
  it('部署成功展示成功横幅', async () => {
    const user = userEvent.setup();
    render(<MarketplacePage />);

    const deployBtn = await screen.findByRole('button', { name: '一键部署' });
    await user.click(deployBtn);

    expect(await screen.findByText(/部署成功/)).toBeInTheDocument();
  });

  it('部署失败展示错误横幅，点击关闭后消失', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(input).includes('/deploy')) {
        return { ok: false, status: 503, json: async () => ({}) } as Response;
      }
      if (String(input).includes('/templates')) {
        return { ok: true, json: async () => TEMPLATES } as Response;
      }
      return { ok: true, json: async () => ({}) } as Response;
    });
    const user = userEvent.setup();
    render(<MarketplacePage />);

    const deployBtn = await screen.findByRole('button', { name: '一键部署' });
    await user.click(deployBtn);

    expect(await screen.findByText(/部署失败：HTTP 503/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '关闭' }));
    expect(screen.queryByText(/部署失败：HTTP 503/)).not.toBeInTheDocument();
  });

  it('模板加载失败展示错误面板，点击重试后恢复', async () => {
    let failFirst = true;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      if (String(input).includes('/templates') && failFirst) {
        failFirst = false;
        return { ok: false, status: 500, json: async () => ({}) } as Response;
      }
      return { ok: true, json: async () => TEMPLATES } as Response;
    });
    const user = userEvent.setup();
    render(<MarketplacePage />);

    expect(await screen.findByText(/加载失败：HTTP 500/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '重试' }));
    expect(await screen.findByRole('button', { name: '一键部署' })).toBeInTheDocument();
  });
});
