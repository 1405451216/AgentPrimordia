/**
 * AgentDesigner — 拖拽式 Agent 可视化设计器（T2-1）。
 *
 * 自包含实现（不依赖 reactflow 等图形库）：
 * - 左侧节点工具箱（LLM / Tool / Reflect / Condition）
 * - 中央画布：节点可拖拽定位、点头部的输出端口进入连线模式后点击目标节点完成连接
 * - 右侧三块面板：ConfigPanel（配置）、PreviewPanel（实时预览）、ExportPanel（一键导出 JSON）
 *
 * 设计遵循 react/ 子包的“懒加载 react”约定（React 作为 peerDependency），
 * 直接 import react 属客户端 UI 的正常用法。
 */

import { useState, useRef, useCallback } from 'react';
import type { PointerEvent as ReactPointerEvent, ReactElement, ReactNode } from 'react';
import { LLMNode } from './nodes/LLMNode.js';
import { ToolNode } from './nodes/ToolNode.js';
import { ReflectNode } from './nodes/ReflectNode.js';
import { ConditionNode } from './nodes/ConditionNode.js';
import { DataEdge } from './edges/DataEdge.js';
import { ConfigPanel } from './panels/ConfigPanel.js';
import { PreviewPanel } from './panels/PreviewPanel.js';
import { ExportPanel } from './panels/ExportPanel.js';

/** 节点类型 */
export type AgentNodeType = 'llm' | 'tool' | 'reflect' | 'condition';

/** 设计器中的节点 */
export interface DesignerNode {
  id: string;
  type: AgentNodeType;
  label: string;
  x: number;
  y: number;
  config: Record<string, unknown>;
}

/** 设计器中的边（数据流） */
export interface DesignerEdge {
  id: string;
  source: string;
  target: string;
}

/** 完整 Agent 图（可导出配置） */
export interface AgentDesignerGraph {
  nodes: DesignerNode[];
  edges: DesignerEdge[];
}

export const NODE_W = 200;
export const NODE_H = 76;

const PALETTE: { type: AgentNodeType; label: string; accent: string }[] = [
  { type: 'llm', label: 'LLM', accent: '#6d8bff' },
  { type: 'tool', label: 'Tool', accent: '#34d399' },
  { type: 'reflect', label: 'Reflect', accent: '#fbbf24' },
  { type: 'condition', label: 'Condition', accent: '#f472b6' },
];

function defaultConfig(type: AgentNodeType): Record<string, unknown> {
  switch (type) {
    case 'llm':
      return { model: 'gpt-4o', systemPrompt: '', temperature: 0.7 };
    case 'tool':
      return { toolkit: 'default', allowedTools: [] };
    case 'reflect':
      return { enabled: true, minTurns: 3 };
    case 'condition':
      return { expression: '', branches: 2 };
  }
}

let idSeq = 0;
function nextId(prefix: string): string {
  idSeq += 1;
  return `${prefix}-${idSeq}`;
}

/** 通用节点外壳：处理拖拽、选中、连线起点的公共交互 */
export interface BaseNodeProps {
  node: DesignerNode;
  selected: boolean;
  accent: string;
  onSelect: () => void;
  onPointerDownBody: (e: ReactPointerEvent) => void;
  onStartConnect: () => void;
  children?: ReactNode;
}

export function BaseNode({
  node,
  selected,
  accent,
  onSelect,
  onPointerDownBody,
  onStartConnect,
  children,
}: BaseNodeProps): ReactElement {
  return (
    <div
      className={`ap-node${selected ? ' ap-node-selected' : ''}`}
      style={{
        position: 'absolute',
        left: node.x,
        top: node.y,
        width: NODE_W,
        minHeight: NODE_H,
        background: 'rgba(255,255,255,0.92)',
        border: selected ? '2px solid #6366f1' : '1px solid #e2e8f0',
        borderLeft: `4px solid ${accent}`,
        borderRadius: 12,
        boxShadow: selected
          ? '0 8px 24px rgba(99,102,241,0.25)'
          : '0 2px 8px rgba(15,23,42,0.08)',
        padding: 10,
        cursor: 'grab',
        userSelect: 'none',
        transition: 'box-shadow 0.2s ease, transform 0.05s ease',
      }}
      onPointerDown={(e) => {
        e.stopPropagation();
        onPointerDownBody(e);
      }}
      onClick={(e) => {
        e.stopPropagation();
        onSelect();
      }}
    >
      <div style={{ fontWeight: 600, fontSize: 13, color: '#0f172a' }}>{node.label}</div>
      <div style={{ marginTop: 6, fontSize: 12, color: '#64748b' }}>{children}</div>
      <div
        title="点击后选择目标节点以连线"
        onPointerDown={(e) => {
          e.stopPropagation();
          onStartConnect();
        }}
        style={{
          position: 'absolute',
          right: -8,
          top: '50%',
          transform: 'translateY(-50%)',
          width: 14,
          height: 14,
          borderRadius: '50%',
          background: accent,
          border: '2px solid #fff',
          cursor: 'crosshair',
        }}
      />
    </div>
  );
}

/** AgentDesigner 主组件 */
export function AgentDesigner(): ReactElement {
  const [nodes, setNodes] = useState<DesignerNode[]>([]);
  const [edges, setEdges] = useState<DesignerEdge[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [connectFrom, setConnectFrom] = useState<string | null>(null);
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const dragRef = useRef<{ id: string; dx: number; dy: number } | null>(null);
  const connectFromRef = useRef<string | null>(null);

  const addNode = useCallback(
    (type: AgentNodeType) => {
      const count = nodes.length;
      const node: DesignerNode = {
        id: nextId(type),
        type,
        label: type.toUpperCase(),
        x: 60 + (count % 5) * 36,
        y: 60 + (count % 5) * 36,
        config: defaultConfig(type),
      };
      setNodes((prev) => [...prev, node]);
    },
    [nodes.length],
  );

  const updateNodePos = useCallback((id: string, x: number, y: number) => {
    setNodes((prev) => prev.map((n) => (n.id === id ? { ...n, x, y } : n)));
  }, []);

  const updateNodeConfig = useCallback((id: string, config: Record<string, unknown>) => {
    setNodes((prev) => prev.map((n) => (n.id === id ? { ...n, config } : n)));
  }, []);

  const startDrag = useCallback(
    (e: ReactPointerEvent, id: string) => {
      const rect = canvasRef.current?.getBoundingClientRect();
      const node = nodes.find((n) => n.id === id);
      if (!rect || !node) return;
      dragRef.current = {
        id,
        dx: e.clientX - rect.left - node.x,
        dy: e.clientY - rect.top - node.y,
      };
      const move = (ev: PointerEvent) => {
        if (!dragRef.current) return;
        const nx = Math.max(0, ev.clientX - rect.left - dragRef.current.dx);
        const ny = Math.max(0, ev.clientY - rect.top - dragRef.current.dy);
        updateNodePos(dragRef.current.id, nx, ny);
      };
      const up = () => {
        dragRef.current = null;
        window.removeEventListener('pointermove', move);
        window.removeEventListener('pointerup', up);
      };
      window.addEventListener('pointermove', move);
      window.addEventListener('pointerup', up);
    },
    [nodes, updateNodePos],
  );

  const startConnect = useCallback((fromId: string) => {
    connectFromRef.current = fromId;
    setConnectFrom(fromId);
  }, []);

  const onNodeClick = useCallback((id: string) => {
    if (connectFromRef.current && connectFromRef.current !== id) {
      const from = connectFromRef.current;
      setEdges((prev) => [...prev, { id: nextId('edge'), source: from, target: id }]);
      connectFromRef.current = null;
      setConnectFrom(null);
      return;
    }
    setSelectedId(id);
  }, []);

  const selectedNode = nodes.find((n) => n.id === selectedId) ?? null;

  const nodeById = new Map(nodes.map((n) => [n.id, n]));

  const renderNode = (n: DesignerNode): ReactElement => {
    const common = {
      node: n,
      selected: n.id === selectedId,
      onSelect: () => onNodeClick(n.id),
      onPointerDownBody: (e: ReactPointerEvent) => startDrag(e, n.id),
      onStartConnect: () => startConnect(n.id),
    };
    switch (n.type) {
      case 'llm':
        return <LLMNode key={n.id} {...common} />;
      case 'tool':
        return <ToolNode key={n.id} {...common} />;
      case 'reflect':
        return <ReflectNode key={n.id} {...common} />;
      case 'condition':
        return <ConditionNode key={n.id} {...common} />;
    }
  };

  const graph: AgentDesignerGraph = { nodes, edges };

  return (
    <div style={{ display: 'flex', height: '100%', minHeight: 480, fontFamily: 'system-ui, sans-serif' }}>
      {/* 节点工具箱 */}
      <aside style={{ width: 168, padding: 12, borderRight: '1px solid #e2e8f0', background: '#f8fafc' }}>
        <div style={{ fontSize: 12, fontWeight: 700, color: '#475569', marginBottom: 8 }}>节点</div>
        {PALETTE.map((p) => (
          <button
            key={p.type}
            onClick={() => addNode(p.type)}
            style={{
              display: 'block',
              width: '100%',
              textAlign: 'left',
              marginBottom: 8,
              padding: '8px 10px',
              borderRadius: 8,
              border: '1px solid #e2e8f0',
              borderLeft: `4px solid ${p.accent}`,
              background: '#fff',
              cursor: 'pointer',
              fontSize: 13,
            }}
          >
            + {p.label}
          </button>
        ))}
        {connectFrom && (
          <div style={{ marginTop: 12, fontSize: 12, color: '#6366f1' }}>
            连线模式：点击目标节点完成连接
          </div>
        )}
      </aside>

      {/* 画布 */}
      <main
        ref={canvasRef}
        onClick={() => {
          setSelectedId(null);
          connectFromRef.current = null;
          setConnectFrom(null);
        }}
        style={{
          position: 'relative',
          flex: 1,
          overflow: 'auto',
          background:
            'radial-gradient(circle, #e2e8f0 1px, transparent 1px) 0 0 / 22px 22px, #fff',
        }}
      >
        <svg
          style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', pointerEvents: 'none' }}
        >
          <defs>
            <marker
              id="ap-arrow"
              markerWidth="10"
              markerHeight="10"
              refX="8"
              refY="3"
              orient="auto"
              markerUnits="strokeWidth"
            >
              <path d="M0,0 L8,3 L0,6 Z" fill="#94a3b8" />
            </marker>
          </defs>
          {edges.map((edge) => {
            const s = nodeById.get(edge.source);
            const t = nodeById.get(edge.target);
            if (!s || !t) return null;
            return (
              <DataEdge
                key={edge.id}
                x1={s.x + NODE_W}
                y1={s.y + NODE_H / 2}
                x2={t.x}
                y2={t.y + NODE_H / 2}
              />
            );
          })}
        </svg>
        {nodes.map(renderNode)}
      </main>

      {/* 右侧面板 */}
      <aside
        style={{
          width: 320,
          padding: 12,
          borderLeft: '1px solid #e2e8f0',
          background: '#f8fafc',
          overflowY: 'auto',
        }}
      >
        <ConfigPanel node={selectedNode} onChange={updateNodeConfig} />
        <PreviewPanel graph={graph} />
        <ExportPanel graph={graph} />
      </aside>
    </div>
  );
}
