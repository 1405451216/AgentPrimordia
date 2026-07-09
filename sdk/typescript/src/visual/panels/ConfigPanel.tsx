/**
 * ConfigPanel — 右侧节点配置面板（T2-1 可视化构建器）。
 *
 * 根据选中节点的 config 动态渲染可编辑字段，编辑后通过 onChange 回写。
 */

import type { ReactElement } from 'react';
import type { DesignerNode } from '../AgentDesigner.js';

export interface ConfigPanelProps {
  node: DesignerNode | null;
  onChange: (id: string, config: Record<string, unknown>) => void;
}

export function ConfigPanel({ node, onChange }: ConfigPanelProps): ReactElement {
  if (!node) {
    return (
      <section style={{ marginBottom: 16 }}>
        <h4 style={{ margin: '0 0 8px', fontSize: 13, color: '#475569' }}>配置</h4>
        <p style={{ fontSize: 12, color: '#94a3b8' }}>选择一个节点以编辑配置。</p>
      </section>
    );
  }

  const entries = Object.entries(node.config);

  const update = (key: string, raw: string) => {
    const original = node.config[key];
    let value: unknown = raw;
    if (typeof original === 'number') {
      const n = Number(raw);
      value = Number.isNaN(n) ? raw : n;
    } else if (Array.isArray(original)) {
      value = raw
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);
    } else if (typeof original === 'boolean') {
      value = raw === 'true';
    }
    onChange(node.id, { ...node.config, [key]: value });
  };

  return (
    <section style={{ marginBottom: 16 }}>
      <h4 style={{ margin: '0 0 8px', fontSize: 13, color: '#475569' }}>
        配置 · {node.label}
      </h4>
      {entries.length === 0 && (
        <p style={{ fontSize: 12, color: '#94a3b8' }}>该节点无配置项。</p>
      )}
      {entries.map(([key, value]) => (
        <label key={key} style={{ display: 'block', marginBottom: 8, fontSize: 12 }}>
          <span style={{ color: '#64748b' }}>{key}</span>
          <input
            value={Array.isArray(value) ? value.join(', ') : String(value)}
            onChange={(e) => update(key, e.target.value)}
            style={{
              display: 'block',
              width: '100%',
              marginTop: 2,
              padding: '6px 8px',
              border: '1px solid #cbd5e1',
              borderRadius: 6,
              fontSize: 12,
              boxSizing: 'border-box',
            }}
          />
        </label>
      ))}
    </section>
  );
}
