/**
 * ExportPanel — 一键导出面板（T2-1 可视化构建器）。
 *
 * 将当前图序列化为 Agent 配置 JSON，提供下载按钮与只读预览。
 */

import { useState } from 'react';
import type { ReactElement } from 'react';
import type { AgentDesignerGraph } from '../AgentDesigner.js';

export interface ExportPanelProps {
  graph: AgentDesignerGraph;
}

function serializeConfig(graph: AgentDesignerGraph): string {
  const config = {
    version: 1,
    nodes: graph.nodes.map((n) => ({
      id: n.id,
      type: n.type,
      label: n.label,
      position: { x: n.x, y: n.y },
      config: n.config,
    })),
    edges: graph.edges.map((e) => ({ id: e.id, source: e.source, target: e.target })),
  };
  return JSON.stringify(config, null, 2);
}

export function ExportPanel({ graph }: ExportPanelProps): ReactElement {
  const [exported, setExported] = useState<string | null>(null);

  const handleExport = () => {
    const json = serializeConfig(graph);
    setExported(json);
    if (typeof document !== 'undefined') {
      const blob = new Blob([json], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'agent-config.json';
      a.click();
      URL.revokeObjectURL(url);
    }
  };

  return (
    <section>
      <h4 style={{ margin: '0 0 8px', fontSize: 13, color: '#475569' }}>导出</h4>
      <button
        onClick={handleExport}
        style={{
          width: '100%',
          padding: '8px 10px',
          borderRadius: 8,
          border: 'none',
          background: '#6366f1',
          color: '#fff',
          fontSize: 13,
          cursor: 'pointer',
        }}
      >
        导出 agent-config.json
      </button>
      {exported && (
        <pre
          style={{
            marginTop: 8,
            maxHeight: 160,
            overflow: 'auto',
            padding: 8,
            background: '#0f172a',
            color: '#e2e8f0',
            borderRadius: 8,
            fontSize: 11,
          }}
        >
          {exported}
        </pre>
      )}
    </section>
  );
}
