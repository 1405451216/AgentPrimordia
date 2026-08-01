/**
 * Studio 壳层与路由渲染测试
 *
 * 验证：
 *  - 侧边导航渲染 4 个入口
 *  - 根路由渲染 Chaos Lab
 *  - 点击导航可切换 Cluster / Learning / Marketplace 页面
 *  - 深链路径直接渲染对应页面
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';

import StudioApp from './router';

// 页面通过 fetch('/api/v1/...') 拉取数据；测试环境按端点返回
// 形状正确的空数据，让页面渲染空态 UI（后端未实现端点时的行为）。
function stubFetchByURL(url: string) {
  if (url.includes('/chaos/experiments')) return [];
  if (url.includes('/cluster/status')) {
    return { nodes: [], leaderId: '', hashRingSize: 0, totalShards: 0 };
  }
  if (url.includes('/learning/capabilities')) return [];
  if (url.includes('/learning/')) return {};
  if (url.includes('/marketplace/templates')) return [];
  return {};
}

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      return { ok: true, json: async () => stubFetchByURL(url) } as Response;
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderStudio(initialPath = '/') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <StudioApp />
    </MemoryRouter>,
  );
}

describe('Studio App 壳层', () => {
  it('渲染侧边导航的 4 个入口', () => {
    renderStudio();
    const nav = screen.getByRole('navigation');
    expect(nav).toHaveTextContent('Chaos Lab');
    expect(nav).toHaveTextContent('Cluster');
    expect(nav).toHaveTextContent('Learning');
    expect(nav).toHaveTextContent('Marketplace');
  });

  it('根路由默认渲染 Chaos Lab 页面', () => {
    renderStudio();
    expect(
      screen.getByRole('heading', { name: 'Chaos Lab' }),
    ).toBeInTheDocument();
    expect(screen.getByText('新建实验')).toBeInTheDocument();
  });

  it('点击 Cluster 导航切换到 Cluster Dashboard', async () => {
    const user = userEvent.setup();
    renderStudio();
    await user.click(screen.getByRole('link', { name: /Cluster/ }));
    expect(
      screen.getByRole('heading', { name: 'Cluster Dashboard' }),
    ).toBeInTheDocument();
  });

  it('点击 Learning 导航切换到 Learning Monitor', async () => {
    const user = userEvent.setup();
    renderStudio();
    await user.click(screen.getByRole('link', { name: /Learning/ }));
    expect(
      screen.getByRole('heading', { name: 'Learning Monitor' }),
    ).toBeInTheDocument();
  });

  it('点击 Marketplace 导航切换到 Agent Marketplace', async () => {
    const user = userEvent.setup();
    renderStudio();
    await user.click(screen.getByRole('link', { name: /Marketplace/ }));
    expect(
      screen.getByRole('heading', { name: 'Agent Marketplace' }),
    ).toBeInTheDocument();
  });

  it('直接访问 /cluster 路径时渲染 Cluster Dashboard（深链）', async () => {
    renderStudio('/cluster');
    // ClusterDashboard 初始为加载态，待 fetch 返回后渲染标题，故用 findByRole 等待
    expect(
      await screen.findByRole('heading', { name: 'Cluster Dashboard' }),
    ).toBeInTheDocument();
  });
});
