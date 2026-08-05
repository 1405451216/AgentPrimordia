/**
 * Marketplace 加固行为测试
 *
 * 覆盖：
 *  - 部署成功展示成功横幅
 *  - 部署失败展示错误横幅，且关闭后消失
 *  - 模板加载失败展示错误面板 + 重试
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup, waitFor } from '@testing-library/react';
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
    if (url.includes('/deployments')) {
      return { ok: true, json: async () => [] } as Response;
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
  it('部署需两步确认，确认后展示成功横幅', async () => {
    const user = userEvent.setup();
    render(<MarketplacePage />);

    const deployBtn = await screen.findByRole('button', { name: '一键部署' });
    await user.click(deployBtn);

    // 确认对话框出现
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '确认部署' }));

    expect(await screen.findByText(/部署成功/)).toBeInTheDocument();
  });

  it('部署确认框可取消，不发起请求', async () => {
    const user = userEvent.setup();
    render(<MarketplacePage />);

    const deployBtn = await screen.findByRole('button', { name: '一键部署' });
    await user.click(deployBtn);

    await screen.findByRole('dialog');
    await user.click(screen.getByRole('button', { name: '取消' }));

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalledWith(
      expect.stringContaining('/deploy'),
      expect.objectContaining({ method: 'POST' }),
    );
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

    await screen.findByRole('dialog');
    await user.click(screen.getByRole('button', { name: '确认部署' }));

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

  it('清空搜索框后重置为全量列表（防抖恒发请求）', async () => {
    // mock 按 q 参数过滤，模拟服务端搜索
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/templates')) {
        const q = new URL(url, 'http://localhost').searchParams.get('q') ?? '';
        const list = q
          ? TEMPLATES.filter((t) => t.name.toLowerCase().includes(q.toLowerCase()))
          : TEMPLATES;
        return { ok: true, json: async () => list } as Response;
      }
      return { ok: true, json: async () => ({}) } as Response;
    });
    const user = userEvent.setup();
    render(<MarketplacePage />);

    // 初始全量列表
    await screen.findByRole('button', { name: '一键部署' });

    // 输入查询词 → 防抖后显示空结果
    await user.type(screen.getByPlaceholderText('搜索模板...'), '不存在');
    await waitFor(() => {
      expect(screen.getByText('未找到模板')).toBeInTheDocument();
    });

    // 清空搜索框 → 防抖后恢复全量列表
    await user.clear(screen.getByPlaceholderText('搜索模板...'));
    await waitFor(() => {
      expect(screen.getByRole('button', { name: '一键部署' })).toBeInTheDocument();
    });
  });

  it('竞态防护：旧请求被新请求中止，慢响应不覆盖新结果', async () => {
    let resolveSlow!: (v: Response) => void;
    const slowGate = new Promise<Response>((r) => { resolveSlow = r; });
    let slowMade = false;

    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/templates') && init?.signal) {
        // 第一个搜索请求标记为「慢」，挂起等待手动完成
        if (!slowMade) {
          slowMade = true;
          return slowGate;
        }
      }
      return { ok: true, json: async () => TEMPLATES } as Response;
    });
    const user = userEvent.setup();
    render(<MarketplacePage />);

    // 触发第一次搜索（慢请求挂起中）
    await user.type(screen.getByPlaceholderText('搜索模板...'), 'x');
    await new Promise((r) => setTimeout(r, 350)); // 等防抖触发第一次搜索

    // 触发第二次搜索（快请求，AbortController 应中止慢请求）
    await user.type(screen.getByPlaceholderText('搜索模板...'), 'y');
    await new Promise((r) => setTimeout(r, 350));

    // 即使慢请求随后完成，也不应覆盖快结果——由 AbortController 的 signal 保证
    resolveSlow!({ ok: true, json: async () => [] } as Response);
    await waitFor(() => {
      // 快请求的模板仍在（未被慢响应 [] 覆盖）
      expect(screen.getByRole('button', { name: '一键部署' })).toBeInTheDocument();
    });
  });

  it('已部署面板展示运行中 Agent，可点击停止', async () => {
    const runningDep = {
      id: 'dep-1',
      templateId: 'code-reviewer',
      name: 'Code Reviewer',
      version: '1.0.0',
      category: 'coding',
      status: 'running',
      deployedAt: new Date().toISOString(),
    };
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/deployments/') && url.includes('/stop') && init?.method === 'POST') {
        return { ok: true, json: async () => ({ status: 'stopped', id: 'dep-1' }) } as Response;
      }
      if (url.includes('/deployments')) {
        // 停止后返回 stopped 状态
        if (String(input).includes('/stop')) {
          return { ok: true, json: async () => [] } as Response;
        }
        return { ok: true, json: async () => [runningDep] } as Response;
      }
      if (url.includes('/templates')) {
        return { ok: true, json: async () => TEMPLATES } as Response;
      }
      return { ok: true, json: async () => ({}) } as Response;
    });
    const user = userEvent.setup();
    render(<MarketplacePage />);

    expect(await screen.findByText('已部署 Agent（1）')).toBeInTheDocument();
    expect(screen.getByText('运行中')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '停止' }));
    // 两步确认
    await screen.findByRole('dialog');
    await user.click(screen.getByRole('button', { name: '确认停止' }));
    expect(await screen.findByText(/已停止/)).toBeInTheDocument();
  });
});
