/**
 * Cluster Dashboard 告警横幅与排序测试
 *
 * 覆盖：
 *  - 存在离线/离开节点时展示告警横幅
 *  - 全节点在线时不展示告警横幅
 *  - 点击状态表头可排序
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ClusterDashboard } from './ClusterDashboard';

const OFFLINE_CLUSTER = {
  nodes: [
    { id: 'node-a', address: '1', role: 'leader', status: 'online', capabilities: [], lastSeen: new Date().toISOString() },
    { id: 'node-b', address: '2', role: 'follower', status: 'offline', capabilities: [], lastSeen: new Date().toISOString() },
  ],
  leaderId: 'node-a',
  hashRingSize: 128,
  totalShards: 8,
};

const ONLINE_CLUSTER = {
  nodes: [
    { id: 'node-a', address: '1', role: 'leader', status: 'online', capabilities: [], lastSeen: new Date().toISOString() },
  ],
  leaderId: 'node-a',
  hashRingSize: 128,
  totalShards: 8,
};

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async () => ({
    ok: true,
    json: async () => ONLINE_CLUSTER,
  }) as Response);
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('Cluster Dashboard 告警与排序', () => {
  it('存在离线节点时展示告警横幅', async () => {
    fetchMock.mockImplementation(async () => ({
      ok: true,
      json: async () => OFFLINE_CLUSTER,
    }) as Response);
    render(<ClusterDashboard />);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('1 个节点状态异常');
    expect(alert).toHaveTextContent(/node-b/);
  });

  it('全节点在线时不展示告警横幅', async () => {
    render(<ClusterDashboard />);

    // 等待节点渲染，确认无告警
    await waitFor(() => {
      expect(screen.getAllByText('node-a').length).toBeGreaterThan(0);
    });
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('点击状态表头按状态排序', async () => {
    const mixedCluster = {
      nodes: [
        { id: 'z-node', address: '1', role: 'follower', status: 'online', capabilities: [], lastSeen: new Date().toISOString() },
        { id: 'a-node', address: '2', role: 'follower', status: 'offline', capabilities: [], lastSeen: new Date().toISOString() },
      ],
      leaderId: '',
      hashRingSize: 128,
      totalShards: 8,
    };
    fetchMock.mockImplementation(async () => ({
      ok: true,
      json: async () => mixedCluster,
    }) as Response);
    const user = userEvent.setup();
    render(<ClusterDashboard />);

    await waitFor(() => {
      expect(screen.getAllByText('a-node').length).toBeGreaterThan(0);
    });
    // 点击状态表头升序：offline 在 online 前
    await user.click(screen.getByRole('button', { name: /状态/ }));
    const rows = screen.getAllByRole('row');
    expect(rows[1]).toHaveTextContent('a-node');
  });
});
