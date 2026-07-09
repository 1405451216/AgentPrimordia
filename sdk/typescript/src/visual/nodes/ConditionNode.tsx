/**
 * Condition 节点 — 条件分支节点（T2-1 可视化构建器）。
 */

import type { PointerEvent as ReactPointerEvent, ReactElement } from 'react';
import { BaseNode, type DesignerNode } from '../AgentDesigner.js';

export interface ConditionNodeProps {
  node: DesignerNode;
  selected: boolean;
  onSelect: () => void;
  onPointerDownBody: (e: ReactPointerEvent) => void;
  onStartConnect: () => void;
}

export function ConditionNode(props: ConditionNodeProps): ReactElement {
  const branches = (props.node.config.branches as number) ?? 2;
  return (
    <BaseNode {...props} accent="#f472b6">
      <div>条件分支节点</div>
      <div style={{ marginTop: 4, fontSize: 11 }}>分支数: {branches}</div>
    </BaseNode>
  );
}
