/**
 * VS Code Agent 管理视图（抽象接口）。
 *
 * 不依赖 vscode 模块，以便在非 VS Code 环境下也可测试。
 * 使用抽象 TreeItem 和 TreeItemCollapsibleState 接口，
 * 由实际的 VS Code 扩展注入 vscode.TreeItem 实现。
 */

import type { PlaygroundClient } from '../playground/index.js';

/** TreeItem 折叠状态（与 vscode.TreeItemCollapsibleState 对齐） */
export enum TreeItemCollapsibleState {
  None = 0,
  Collapsed = 1,
  Expanded = 2,
}

/** 抽象 TreeItem 接口 */
export interface TreeItem {
  label: string;
  description?: string;
  collapsibleState: TreeItemCollapsibleState;
  command?: { command: string; title: string; arguments?: unknown[] };
  iconPath?: string | { light: string; dark: string };
  tooltip?: string;
  contextValue?: string;
}

/** Agent 树节点项 */
export interface AgentItem {
  id: string;
  label: string;
  status: 'running' | 'idle' | 'error';
  collapsibleState: TreeItemCollapsibleState;
  model?: string;
  details?: string;
}

/**
 * Provider 接口——VS Code TreeView 所需的抽象接口。
 * 实际使用时由 vscode 扩展包装本接口。
 */
export interface AgentViewProvider {
  getChildren(element?: AgentItem): Promise<AgentItem[]>;
  getTreeItem(element: AgentItem): TreeItem;
  refresh(): void;
}

/**
 * 基于 PlaygroundClient 的 AgentViewProvider 实现。
 */
export class PlaygroundAgentViewProvider implements AgentViewProvider {
  private agents: AgentItem[] = [];
  private listeners: Array<() => void> = [];
  private lastError?: Error;

  constructor(private readonly client: PlaygroundClient) {}

  /** 获取子节点：根节点返回全部 Agent */
  async getChildren(element?: AgentItem): Promise<AgentItem[]> {
    if (element) {
      // 目前只有一层，子节点返回空
      return [];
    }
    return this.agents;
  }

  /** 将 AgentItem 转为 VS Code TreeItem */
  getTreeItem(element: AgentItem): TreeItem {
    const icon =
      element.status === 'running' ? '$(sync~spin)' :
      element.status === 'error'   ? '$(error)' :
                                     '$(circle-outline)';
    return {
      label: element.label,
      description: element.model ?? element.status,
      collapsibleState: element.collapsibleState,
      iconPath: icon,
      tooltip: element.details ?? `Agent ${element.id} (${element.status})`,
      contextValue: 'agent',
    };
  }

  /** 刷新 Agent 列表 */
  async refresh(): Promise<void> {
    try {
      const list = await this.client.listAgents();
      this.agents = list.map((a) => ({
        id: a.id,
        label: a.id,
        status: a.status as AgentItem['status'],
        collapsibleState: TreeItemCollapsibleState.None,
        model: a.model,
        details: `${a.model} [${a.status}]`,
      }));
      this.lastError = undefined;
    } catch (err) {
      this.lastError = err instanceof Error ? err : new Error(String(err));
      this.agents = [];
    }
    for (const fn of this.listeners) fn();
  }

  /** 注册变更订阅 */
  onDidChange(fn: () => void): () => void {
    this.listeners.push(fn);
    return () => {
      this.listeners = this.listeners.filter((l) => l !== fn);
    };
  }

  getLastError(): Error | undefined {
    return this.lastError;
  }
}

/** 将任意字符串归一化为 AgentItem['status'] */
export function normalizeAgentStatus(s: string): AgentItem['status'] {
  if (s === 'running' || s === 'idle' || s === 'error') return s;
  return 'error';
}
