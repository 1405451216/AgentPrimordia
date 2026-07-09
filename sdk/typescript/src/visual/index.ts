/**
 * 可视化构建器子包入口（T2-1）。
 *
 * 自包含实现（不依赖 reactflow），提供拖拽式 Agent 设计器及节点/边/面板组件。
 */

export { AgentDesigner, BaseNode, NODE_W, NODE_H } from './AgentDesigner.js';
export type {
  AgentNodeType,
  DesignerNode,
  DesignerEdge,
  AgentDesignerGraph,
  BaseNodeProps,
} from './AgentDesigner.js';

export { LLMNode } from './nodes/LLMNode.js';
export { ToolNode } from './nodes/ToolNode.js';
export { ReflectNode } from './nodes/ReflectNode.js';
export { ConditionNode } from './nodes/ConditionNode.js';

export { DataEdge } from './edges/DataEdge.js';
export type { DataEdgeProps } from './edges/DataEdge.js';

export { ConfigPanel } from './panels/ConfigPanel.js';
export type { ConfigPanelProps } from './panels/ConfigPanel.js';
export { PreviewPanel } from './panels/PreviewPanel.js';
export type { PreviewPanelProps } from './panels/PreviewPanel.js';
export { ExportPanel } from './panels/ExportPanel.js';
export type { ExportPanelProps } from './panels/ExportPanel.js';
