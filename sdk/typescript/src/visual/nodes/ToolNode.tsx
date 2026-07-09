/**
 * Tool 节点 — 工具调用节点（T2-1 可视化构建器）。
 */

import type { PointerEvent as ReactPointerEvent, ReactElement } from 'react';
import { BaseNode, type DesignerNode } from '../AgentDesigner.js';

export interface ToolNodeProps {
  node: DesignerNode;
  selected: boolean;
  onSelect: () => void;
  onPointerDownBody: (e: ReactPointerEvent) => void;
  onStartConnect: () => void;
}

export function ToolNode(props: ToolNodeProps): ReactElement {
  const toolkit = (props.node.config.toolkit as string) ?? 'default';
  return (
    <BaseNode {...props} accent="#34d399">
      <div>工具调用节点</div>
      <div style={{ marginTop: 4, fontSize: 11 }}>toolkit: {toolkit}</div>
    </BaseNode>
  );
}
