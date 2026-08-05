/**
 * Chaos Lab 加固行为测试
 *
 * 覆盖：
 *  - 破坏性故障类型弹出两步确认
 *  - 确认后 POST，成功展示「已提交」横幅
 *  - POST 失败展示错误信息
 *  - 取消确认不提交
 *  - 历史加载失败展示错误面板 + 重试
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, within, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ChaosLab } from './ChaosLab';

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') {
      return { ok: true, json: async () => ({}) } as Response;
    }
    if (url.includes('/chaos/experiments')) {
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

describe('Chaos Lab 加固', () => {
  it('破坏性故障类型（进程终止）触发两步确认', async () => {
    const user = userEvent.setup();
    render(<ChaosLab />);

    await user.type(screen.getByPlaceholderText('实验名称'), 'kill 实验');
    await user.selectOptions(screen.getByRole('combobox'), 'kill');
    await user.click(screen.getByRole('button', { name: '运行实验' }));

    // 确认对话框出现，且包含破坏性警告
    const dialog = screen.getByRole('dialog');
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getAllByText(/进程终止/).length).toBeGreaterThanOrEqual(1);
    // 尚未提交
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/chaos/experiments', expect.objectContaining({ method: 'POST' }));
  });

  it('确认后提交 POST，成功展示「已提交」横幅', async () => {
    const user = userEvent.setup();
    render(<ChaosLab />);

    await user.type(screen.getByPlaceholderText('实验名称'), '延迟实验');
    await user.click(screen.getByRole('button', { name: '运行实验' }));
    await user.click(screen.getByRole('button', { name: '确认运行' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/chaos/experiments',
        expect.objectContaining({ method: 'POST' }),
      );
    });
    expect(await screen.findByText(/已提交/)).toBeInTheDocument();
  });

  it('取消确认不提交', async () => {
    const user = userEvent.setup();
    render(<ChaosLab />);

    await user.type(screen.getByPlaceholderText('实验名称'), '取消实验');
    await user.click(screen.getByRole('button', { name: '运行实验' }));
    await user.click(screen.getByRole('button', { name: '取消' }));

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalledWith(
      '/api/v1/chaos/experiments',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('POST 失败时展示错误信息', async () => {
    fetchMock.mockImplementation(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return { ok: false, status: 500, json: async () => ({}) } as Response;
      }
      return { ok: true, json: async () => [] } as Response;
    });
    const user = userEvent.setup();
    render(<ChaosLab />);

    await user.type(screen.getByPlaceholderText('实验名称'), '失败实验');
    await user.click(screen.getByRole('button', { name: '运行实验' }));
    await user.click(screen.getByRole('button', { name: '确认运行' }));

    expect(await screen.findByText(/提交失败：HTTP 500/)).toBeInTheDocument();
    // 确认框保留，便于重试
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('历史加载失败展示错误面板，点击重试后恢复', async () => {
    let failFirst = true;
    fetchMock.mockImplementation(async (_input: RequestInfo | URL) => {
      if (String(_input).includes('/chaos/experiments') && failFirst) {
        failFirst = false;
        return { ok: false, status: 500, json: async () => ({}) } as Response;
      }
      return { ok: true, json: async () => [] } as Response;
    });
    const user = userEvent.setup();
    render(<ChaosLab />);

    expect(await screen.findByText(/加载失败：HTTP 500/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '重试' }));
    expect(await screen.findByText('暂无实验记录')).toBeInTheDocument();
  });
});
