/**
 * DAG 可视化面板（抽象接口）。
 *
 * 不依赖 vscode 模块，以便也可用于其他 Web 视图框架。
 * Node 状态：pending / running / done / failed
 */

/** DAG 节点状态 */
export type DagNodeStatus = 'pending' | 'running' | 'done' | 'failed';

/** DAG 工作流节点 */
export interface DagNode {
  id: string;
  label: string;
  status: DagNodeStatus;
  /** 节点在画布中的位置 */
  x?: number;
  y?: number;
  metadata?: Record<string, unknown>;
}

/** DAG 工作流边 */
export interface DagEdge {
  from: string;
  to: string;
  label?: string;
  highlight?: boolean;
}

/** DAG 工作流 */
export interface DagWorkflow {
  nodes: DagNode[];
  edges: DagEdge[];
}

/** 抽象 Webview 接口 */
export interface Webview {
  postMessage(message: unknown): void;
  onMessage(handler: (msg: unknown) => void): void;
}

/** 节点状态更新事件 */
export interface NodeStatusUpdate {
  type: 'node_status';
  nodeId: string;
  status: DagNodeStatus;
}

/** 边高亮事件 */
export interface EdgeHighlight {
  type: 'edge_highlight';
  from: string;
  to: string;
}

/** 全部事件类型 */
export type DagVisualizerEvent = NodeStatusUpdate | EdgeHighlight;

/**
 * DAG 可视化器 — 管理 Webview 面板中的 DAG 渲染。
 */
export class DagVisualizer {
  private workflow: DagWorkflow;
  private webview: Webview;
  private messageHandlers: Array<(msg: DagVisualizerEvent) => void> = [];

  private constructor(webview: Webview, workflow: DagWorkflow) {
    this.webview = webview;
    this.workflow = workflow;

    // 监听来自 webview 的消息
    this.webview.onMessage((msg: unknown) => {
      const m = msg as DagVisualizerEvent;
      for (const h of this.messageHandlers) h(m);
    });
  }

  /** 创建新的可视化面板 */
  static createPanel(webview: Webview, workflow: DagWorkflow): DagVisualizer {
    const viz = new DagVisualizer(webview, workflow);
    viz.render();
    return viz;
  }

  /** 更新节点状态 */
  updateNodeStatus(nodeId: string, status: DagNodeStatus): void {
    const node = this.workflow.nodes.find((n) => n.id === nodeId);
    if (node) {
      node.status = status;
      this.webview.postMessage({ type: 'node_status', nodeId, status } satisfies NodeStatusUpdate);
    }
  }

  /** 高亮边 */
  highlightEdge(from: string, to: string): void {
    const edge = this.workflow.edges.find((e) => e.from === from && e.to === to);
    if (edge) {
      edge.highlight = true;
      this.webview.postMessage({ type: 'edge_highlight', from, to } satisfies EdgeHighlight);
    }
  }

  /** 注册事件监听 */
  onEvent(handler: (msg: DagVisualizerEvent) => void): () => void {
    this.messageHandlers.push(handler);
    return () => {
      this.messageHandlers = this.messageHandlers.filter((h) => h !== handler);
    };
  }

  /** 获取当前工作流 */
  getWorkflow(): DagWorkflow {
    return { nodes: [...this.workflow.nodes], edges: [...this.workflow.edges] };
  }

  /** 重置所有边的 highlight */
  clearHighlights(): void {
    for (const e of this.workflow.edges) {
      e.highlight = false;
    }
  }

  private render(): void {
    this.webview.postMessage({
      type: 'init',
      workflow: this.workflow,
    });
  }
}

/** 用于测试的 MockWebview 实现 */
export class MockWebview implements Webview {
  readonly messages: unknown[] = [];
  private handlers: Array<(msg: unknown) => void> = [];

  postMessage(message: unknown): void {
    this.messages.push(message);
  }

  onMessage(handler: (msg: unknown) => void): void {
    this.handlers.push(handler);
  }

  /** 模拟从 webview 发送消息到扩展 */
  simulateMessage(msg: unknown): void {
    for (const h of this.handlers) h(msg);
  }
}

/** 节点状态的显示颜色映射 */
export const STATUS_COLORS: Record<DagNodeStatus, string> = {
  pending: '#9ca3af',
  running: '#3b82f6',
  done: '#22c55e',
  failed: '#ef4444',
};

/** 构建简单的 DAG 节点 ID */
export function makeNodeId(prefix: string, index: number): string {
  return `${prefix}_${index}`;
}
