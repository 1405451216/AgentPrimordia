/**
 * Reflect 节点 — 自我反思节点（T2-1 可视化构建器）。
 */

import type { PointerEvent as ReactPointerEvent, ReactElement } from 'react';
import { BaseNode, type DesignerNode } from '../AgentDesigner.js';

export interface ReflectNodeProps {
  node: DesignerNode;
  selected: boolean;
  onSelect: () => void;
  onPointerDownBody: (e: ReactPointerEvent) => void;
  onStartConnect: () => void;
}

export function ReflectNode(props: ReflectNodeProps): ReactElement {
  const enabled = props.node.config.enabled !== false;
  return (
    <BaseNode {...props} accent="#fbbf24">
      <div>自我反思节点</div>
      <div style={{ marginTop: 4, fontSize: 11 }}>{enabled ? '已启用' : '已禁用'}</div>
    </BaseNode>
  );
}
