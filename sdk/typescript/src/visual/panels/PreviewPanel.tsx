/**
 * PreviewPanel — 实时预览面板（T2-1 可视化构建器）。
 *
 * 以只读形式展示当前图的节点/边统计与 JSON 摘要，随编辑实时更新。
 */

import type { ReactElement } from 'react';
import type { AgentDesignerGraph } from '../AgentDesigner.js';

export interface PreviewPanelProps {
  graph: AgentDesignerGraph;
}

export function PreviewPanel({ graph }: PreviewPanelProps): ReactElement {
  return (
    <section style={{ marginBottom: 16 }}>
      <h4 style={{ margin: '0 0 8px', fontSize: 13, color: '#475569' }}>实时预览</h4>
      <div style={{ fontSize: 12, color: '#475569' }}>
        <div>节点数：{graph.nodes.length}</div>
        <div>边数：{graph.edges.length}</div>
      </div>
      <pre
        style={{
          marginTop: 8,
          maxHeight: 180,
          overflow: 'auto',
          padding: 8,
          background: '#0f172a',
          color: '#e2e8f0',
          borderRadius: 8,
          fontSize: 11,
          lineHeight: 1.5,
        }}
      >
        {JSON.stringify(graph, null, 2)}
      </pre>
    </section>
  );
}
