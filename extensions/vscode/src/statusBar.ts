/**
 * AgentPrimordia Status Bar — 状态栏组件。
 *
 * 职责：
 * 1. 显示当前 Agent 名 + 状态（空闲/运行/错误）
 * 2. 展示当前会话 token 数（点击查看详情）
  * 3. 展示累计成本（实时更新）
 * 4. 点击可打开完整仪表盘或切换 Agent
 *
 * 设计要点：
 * - 纯逻辑：状态计算与格式化不依赖 vscode API
 * - 创建/更新/销毁委托给 extension.ts 传入的回调
 */

/** Agent 运行状态 */
export type AgentStatus = 'idle' | 'running' | 'error';

/** 状态栏显示数据 */
export interface StatusBarData {
  agentName: string;
  status: AgentStatus;
  tokens: number;
  cost: number;
}

/** 状态栏依赖（vscode API 抽象） */
export interface StatusBarDeps {
  /** 创建底层 StatusBarItem（由 extension.ts 注入） */
  createItem(opts: {
    alignment: number;
    priority: number;
    command?: string;
    tooltip?: string;
  }): any;
  /** 状态栏点击回调 */
  onClick?: (data: StatusBarData) => void;
}

/** 状态枚举 → 图标文本 */
export function statusIcon(status: AgentStatus): string {
  switch (status) {
    case 'idle':
      return '$(debug-pause)';
    case 'running':
      return '$(sync~spin)';
    case 'error':
      return '$(error)';
    default:
      return '$(question)';
  }
}

/** token 数格式化（k 单位） */
export function formatTokens(n: number): string {
  if (n < 1000) return `${n}`;
  return `${(n / 1000).toFixed(1)}k`;
}

/** 成本格式化（美元，4 位小数） */
export function formatCost(n: number): string {
  return `$${n.toFixed(4)}`;
}

/** 状态 → 可读标签（中文） */
export function statusText(status: AgentStatus): string {
  switch (status) {
    case 'idle':
      return '空闲';
    case 'running':
      return '运行中';
    case 'error':
      return '错误';
    default:
      return status;
  }
}

/** 构造状态栏 title 字符串 */
export function buildStatusBarTitle(data: StatusBarData): string {
  const icon = statusIcon(data.status);
  return `${icon} ${data.agentName} | ${formatTokens(data.tokens)} tok | ${formatCost(data.cost)}`;
}

/** 状态栏管理器 */
export class StatusBarManager {
  private item: any = null;
  private data: StatusBarData = {
    agentName: '',
    status: 'idle',
    tokens: 0,
    cost: 0,
  };

  /** 创建状态栏组件并返回引用 */
  create(deps: StatusBarDeps): any {
    const VA: any = (globalThis as any).vscode;
    const alignment = VA?.StatusBarAlignment?.Left ?? 1;
    this.item = deps.createItem({
      alignment,
      priority: 100,
      command: 'agentprimordia.chat.focus',
      tooltip: 'AgentPrimordia',
    });
    this.refresh();
    this.item.show();
    return this.item;
  }

  /** 更新状态数据并刷新显示 */
  update(patch: Partial<StatusBarData>): void {
    this.data = { ...this.data, ...patch };
    this.refresh();
  }

  /** 重新渲染 title */
  refresh(): void {
    if (!this.item) return;
    this.item.text = buildStatusBarTitle(this.item._data ?? this.data);
    this.item.tooltip = [
      `Agent: ${this.data.agentName}`,
      `状态: ${statusText(this.data.status)}`,
      `Token: ${this.data.tokens}`,
      `成本: ${formatCost(this.data.cost)}`,
    ].join('\n');
  }

  /** 销毁状态栏组件 */
  dispose(): void {
    this.item?.dispose();
    this.item = null;
  }

  /** 当前数据（供测试断言） */
  getData(): StatusBarData {
    return { ...this.data };
  }

  /** 获取底层 StatusBarItem（供 extension.ts 注册到 subscriptions） */
  getItem(): any {
    return this.item;
  }
}
