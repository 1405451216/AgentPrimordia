/**
 * Agent Inspector 公共类型定义。
 *
 * 设计动机：把 Inspector Webview 的状态与事件抽象成与 VS Code API
 * 无关的数据结构，便于在 Node 环境用 vitest 单元测试。
 * extension.ts 仅作为 vscode.WebviewPanel ↔ Inspector 之间的粘合。
 */

/** Agent 步骤类型（与 ReAct Loop 一致） */
export type InspectorStepKind =
  | 'thought'
  | 'action'
  | 'observation'
  | 'turn'
  | 'done'
  | 'error';

/** 单步事件 */
export interface InspectorStep {
  /** 序号（从 1 开始） */
  index: number;
  /** 步骤类型 */
  kind: InspectorStepKind;
  /** 步骤文本内容（用于 thought/observation/done/error） */
  text?: string;
  /** 工具调用（action 步骤） */
  tool?: string;
  /** 工具参数（action 步骤） */
  args?: unknown;
  /** 时间戳（毫秒） */
  timestamp: number;
}

// ===== DAG 工作流可视化（v4.4-4：VS Code Inspector 完善） =====
// 与 sdk/typescript/src/vscode/dag-visualizer.ts 的 DagWorkflow 形状一致。

/** DAG 节点状态 */
export type WorkflowDagNodeStatus = 'pending' | 'running' | 'done' | 'failed';

/** DAG 工作流节点 */
export interface WorkflowDagNode {
  id: string;
  label: string;
  status: WorkflowDagNodeStatus;
  metadata?: Record<string, unknown>;
}

/** DAG 工作流边 */
export interface WorkflowDagEdge {
  from: string;
  to: string;
  label?: string;
  highlight?: boolean;
}

/** DAG 工作流（plan 执行图） */
export interface WorkflowDag {
  nodes: WorkflowDagNode[];
  edges: WorkflowDagEdge[];
}

/** Inspector 状态 */
export type InspectorStatus = 'idle' | 'running' | 'paused' | 'done' | 'error';

/** Inspector 主状态 */
export interface InspectorState {
  status: InspectorStatus;
  steps: InspectorStep[];
  currentPrompt: string;
  /** 当前累计 token 数（粗略估算） */
  tokens: number;
  /** 错误信息 */
  error: Error | null;
  /** 起始时间 */
  startedAt: number | null;
  /** 结束时间 */
  endedAt: number | null;
  /** 断点序号列表（在断点处的 step index） */
  breakpoints: Set<number>;
  /** 计划 DAG 工作流（v4.4-4：plan 事件到达时更新） */
  plan: WorkflowDag | null;
}

/** Inspector 命令（从 Webview → Extension Host） */
export type InspectorCommand =
  | { type: 'start'; prompt: string; maxTurns: number }
  | { type: 'stop' }
  | { type: 'pause' }
  | { type: 'resume' }
  | { type: 'reset' }
  | { type: 'addBreakpoint'; stepIndex: number }
  | { type: 'removeBreakpoint'; stepIndex: number };

/** Inspector 输出消息（从 Extension Host → Webview） */
export type InspectorMessage =
  | { type: 'state'; state: InspectorState }
  | { type: 'log'; level: 'info' | 'warn' | 'error'; text: string }
  | { type: 'streamChunk'; text: string };

/** 调试配置 */
export interface AgentDebugConfig {
  /** 配置名称 */
  name: string;
  /** Agent 名称（从 .ap.yaml 推断） */
  agentName: string;
  /** 系统 prompt */
  systemPrompt: string;
  /** 启动 prompt */
  initialPrompt: string;
  /** 最大轮次 */
  maxTurns: number;
  /** 是否启用 trace 模式 */
  trace: boolean;
  /** 工作目录 */
  cwd: string;
}

/** .ap.yaml 顶层结构（最小子集） */
export interface ApYamlConfig {
  /** Agent 名称 */
  name?: string;
  /** 系统 prompt */
  systemPrompt?: string;
  /** 启动 prompt */
  initialPrompt?: string;
  /** 最大轮次 */
  maxTurns?: number;
  /** 工具列表 */
  tools?: string[];
  /** 其它字段透传 */
  [key: string]: unknown;
}