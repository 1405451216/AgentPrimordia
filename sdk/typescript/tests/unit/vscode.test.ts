import { describe, it, expect, vi } from 'vitest';
import {
  PlaygroundAgentViewProvider,
  TreeItemCollapsibleState,
  normalizeAgentStatus,
  type AgentItem,
} from '../../src/vscode/agents-view.js';
import {
  DagVisualizer,
  MockWebview,
  STATUS_COLORS,
  makeNodeId,
  type DagWorkflow,
  type DagNodeStatus,
} from '../../src/vscode/dag-visualizer.js';
import type { PlaygroundClient } from '../../src/playground/index.js';

/** 构造一个 mock PlaygroundClient */
function mockClient(data: { listAgents?: any }): PlaygroundClient {
  return {
    listAgents: vi.fn(async () => data.listAgents ?? []),
    createAgent: vi.fn(),
    deleteAgent: vi.fn(),
    chat: vi.fn(),
    getStats: vi.fn(),
    getAgent: vi.fn(),
    streamChat: vi.fn(),
    streamEvents: vi.fn(),
  } as unknown as PlaygroundClient;
}

describe('VS Code Agents View', () => {
  it('getChildren returns all agents at root', async () => {
    const client = mockClient({
      listAgents: [
        { id: 'a1', model: 'gpt-4', status: 'idle' },
        { id: 'a2', model: 'gpt-4', status: 'running' },
      ],
    });
    const provider = new PlaygroundAgentViewProvider(client);
    await provider.refresh();

    const children = await provider.getChildren();
    expect(children).toHaveLength(2);
    expect(children[0].id).toBe('a1');
    expect(children[1].status).toBe('running');
  });

  it('getChildren returns empty for child element', async () => {
    const client = mockClient({ listAgents: [{ id: 'a1', model: 'gpt-4', status: 'idle' }] });
    const provider = new PlaygroundAgentViewProvider(client);
    await provider.refresh();

    const item: AgentItem = {
      id: 'a1', label: 'a1', status: 'idle',
      collapsibleState: TreeItemCollapsibleState.None,
    };
    const children = await provider.getChildren(item);
    expect(children).toHaveLength(0);
  });

  it('getTreeItem returns correct icon per status', async () => {
    const client = mockClient({ listAgents: [] });
    const provider = new PlaygroundAgentViewProvider(client);

    const idle: AgentItem = { id: 'x', label: 'x', status: 'idle', collapsibleState: TreeItemCollapsibleState.None };
    expect(provider.getTreeItem(idle).iconPath).toContain('circle-outline');

    const running: AgentItem = { id: 'x', label: 'x', status: 'running', collapsibleState: TreeItemCollapsibleState.None };
    expect(provider.getTreeItem(running).iconPath).toContain('sync');

    const errorItem: AgentItem = { id: 'x', label: 'x', status: 'error', collapsibleState: TreeItemCollapsibleState.None };
    expect(provider.getTreeItem(errorItem).iconPath).toContain('error');
  });

  it('normalizeAgentStatus maps correctly', () => {
    expect(normalizeAgentStatus('running')).toBe('running');
    expect(normalizeAgentStatus('idle')).toBe('idle');
    expect(normalizeAgentStatus('error')).toBe('error');
    expect(normalizeAgentStatus('unknown')).toBe('error');
  });

  it('refresh handles API errors gracefully', async () => {
    const client = mockClient({});
    client.listAgents = vi.fn(async () => { throw new Error('API down'); });
    const provider = new PlaygroundAgentViewProvider(client);

    await provider.refresh();
    expect(provider.getLastError()?.message).toBe('API down');
  });

  it('notifies listeners on refresh', async () => {
    const client = mockClient({ listAgents: [] });
    const provider = new PlaygroundAgentViewProvider(client);
    const spy = vi.fn();
    provider.onDidChange(spy);

    await provider.refresh();
    expect(spy).toHaveBeenCalledTimes(1);
  });
});

describe('VS Code DAG Visualizer', () => {
  const sampleWorkflow: DagWorkflow = {
    nodes: [
      { id: 'n1', label: 'Start', status: 'pending' },
      { id: 'n2', label: 'Task A', status: 'running' },
      { id: 'n3', label: 'Task B', status: 'done' },
    ],
    edges: [
      { from: 'n1', to: 'n2' },
      { from: 'n1', to: 'n3' },
    ],
  };

  it('createPanel sends init message', () => {
    const mock = new MockWebview();
    DagVisualizer.createPanel(mock, sampleWorkflow);

    expect(mock.messages).toHaveLength(1);
    expect(mock.messages[0]).toEqual({ type: 'init', workflow: sampleWorkflow });
  });

  it('updateNodeStatus posts message', () => {
    const mock = new MockWebview();
    const viz = DagVisualizer.createPanel(mock, sampleWorkflow);
    mock.messages.length = 0;

    viz.updateNodeStatus('n1', 'done');
    expect(mock.messages).toEqual([{ type: 'node_status', nodeId: 'n1', status: 'done' }]);
    expect(viz.getWorkflow().nodes[0].status).toBe('done');
  });

  it('highlightEdge posts message', () => {
    const mock = new MockWebview();
    const viz = DagVisualizer.createPanel(mock, sampleWorkflow);
    mock.messages.length = 0;

    viz.highlightEdge('n1', 'n2');
    expect(mock.messages).toEqual([{ type: 'edge_highlight', from: 'n1', to: 'n2' }]);
  });

  it('MockWebview simulates messages', () => {
    const mock = new MockWebview();
    const viz = DagVisualizer.createPanel(mock, sampleWorkflow);

    const handler = vi.fn();
    viz.onEvent(handler);
    mock.simulateMessage({ type: 'node_status', nodeId: 'n1', status: 'running' });

    expect(handler).toHaveBeenCalledWith({ type: 'node_status', nodeId: 'n1', status: 'running' });
  });

  it('STATUS_COLORS maps all statuses', () => {
    expect(STATUS_COLORS.pending).toBe('#9ca3af');
    expect(STATUS_COLORS.running).toBe('#3b82f6');
    expect(STATUS_COLORS.done).toBe('#22c55e');
    expect(STATUS_COLORS.failed).toBe('#ef4444');
  });

  it('makeNodeId formats correctly', () => {
    expect(makeNodeId('step', 0)).toBe('step_0');
    expect(makeNodeId('node', 5)).toBe('node_5');
  });
});
