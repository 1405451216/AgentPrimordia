/**
 * AgentPrimordia Run History — 侧边栏树形视图。
 *
 * 职责：
 * 1. 按日期分组展示最近 Agent 运行记录
 * 2. 每条记录显示：模板名、轮次、token、成本、状态
 * 3. 点击打开详情；右键菜单：重新运行 / 比较 / 删除
 *
 * 设计要点：
 * - TreeDataProvider 模式，适配 VS Code 树形视图
 * - 数据源由 StudioApi 提供（可替换为本地缓存 stub）
 */

import type { StudioApi, Run } from './studioApi.js';

/** 树节点类型 */
export type RunTreeNode =
  | { type: 'date'; label: string }
  | { type: 'run'; run: Run };

/** 依赖注入（便于测试替换） */
export interface RunHistoryDeps {
  api: StudioApi;
  /** 最大加载条数 */
  limit?: number;
}

/** 格式化日期分组标签（"今天" / "昨天" / "MM-DD"） */
export function groupLabel(ts: number): string {
  const now = new Date();
  const d = new Date(ts);
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const startOfYesterday = startOfToday - 86400000;
  const startOfDay = new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

  if (startOfDay === startOfToday) return '今天';
  if (startOfDay === startOfYesterday) return '昨天';
  return `${d.getMonth() + 1}-${String(d.getDate()).padStart(2, '0')}`;
}

/** 状态 → 简短图标文本 */
export function runStatusIcon(status: Run['status']): string {
  switch (status) {
    case 'pending':
      return '$(clock)';
    case 'running':
      return '$(sync~spin)';
    case 'done':
      return '$(check)';
    case 'error':
      return '$(error)';
    default:
      return '$(question)';
  }
}

/** 单行摘要 */
export function runSummary(run: Run): string {
  return [
    `模板: ${run.template}`,
    `轮次: ${run.turns}`,
    `Token: ${run.tokens}`,
    `成本: $${run.cost.toFixed(4)}`,
    `状态: ${run.status}`,
  ].join('  |  ');
}

/** 按日期分组 */
export function groupByDate(runs: Run[]): Array<{ label: string; runs: Run[] }> {
  const map = new Map<string, Run[]>();
  for (const r of runs) {
    const label = groupLabel(r.startedAt);
    const list = map.get(label) ?? [];
    list.push(r);
    map.set(label, list);
  }
  return Array.from(map.entries()).map(([label, runs]) => ({ label, runs }));
}

/** 树形视图提供者 */
export class RunHistoryProvider {
  private runs: Run[] = [];
  private onDidChangeTreeData: ((node?: any) => void) | null = null;

  constructor(private readonly deps: RunHistoryDeps) {}

  /** 注册数据变化回调（由 extension.ts 传入） */
  registerListener(cb: (node?: any) => void): void {
    this.onDidChangeTreeData = cb;
  }

  /** 加载运行记录 */
  async refresh(): Promise<void> {
    try {
      const res = await this.deps.api.getRuns(this.deps.limit ?? 20);
      this.runs = res.items;
    } catch {
      this.runs = [];
    }
    this.onDidChangeTreeData?.();
  }

  /** 获取根节点（日期分组） */
  getTreeItem(node: RunTreeNode): any {
    const VA: any = (globalThis as any).vscode;
    if (node.type === 'date') {
      const item = new VA.TreeItem(node.label, VA.TreeItemCollapsibleState.Expanded);
      item.contextValue = 'date';
      return item;
    }
    const run = node.run;
    const item = new VA.TreeItem(`${runStatusIcon(run.status)} ${run.template}`, VA.TreeItemCollapsibleState.None);
    item.description = `${run.turns} 轮 / ${run.tokens} tok / $${run.cost.toFixed(3)}`;
    item.tooltip = runSummary(run);
    item.contextValue = 'run';
    item.command = {
      command: 'agentprimordia.runDetails',
      title: '查看运行详情',
      arguments: [run.id],
    };
    return item;
  }

  /** 获取子节点 */
  getChildren(node?: RunTreeNode): RunTreeNode[] {
    if (!node) {
      // 根：返回日期分组
      return groupByDate(this.runs).map((g) => ({ type: 'date' as const, label: g.label }));
    }
    if (node.type === 'date') {
      // 日期分组下：返回运行记录
      return this.runs
        .filter((r) => groupLabel(r.startedAt) === node.label)
        .map((run) => ({ type: 'run' as const, run }));
    }
    return [];
  }

  /** 删除指定运行（本地，Studio 端需另行调用） */
  removeRun(id: string): void {
    this.runs = this.runs.filter((r) => r.id !== id);
    this.onDidChangeTreeData?.();
  }

  /** 当前数据快照（供测试） */
  getRuns(): Run[] {
    return this.runs.map((r) => ({ ...r }));
  }
}
