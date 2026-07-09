/**
 * LLM 节点 — 推理 / 规划节点（T2-1 可视化构建器）。
 */

import type { PointerEvent as ReactPointerEvent, ReactElement } from 'react';
import { BaseNode, type DesignerNode } from '../AgentDesigner.js';

export interface LLMNodeProps {
  node: DesignerNode;
  selected: boolean;
  onSelect: () => void;
  onPointerDownBody: (e: ReactPointerEvent) => void;
  onStartConnect: () => void;
}

export function LLMNode(props: LLMNodeProps): ReactElement {
  const model = (props.node.config.model as string) ?? 'gpt-4o';
  return (
    <BaseNode {...props} accent="#6d8bff">
      <div>LLM 推理节点</div>
      <div style={{ marginTop: 4, fontSize: 11 }}>model: {model}</div>
    </BaseNode>
  );
}
