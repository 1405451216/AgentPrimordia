/**
 * VisualEditor — 零依赖可视化 Agent 编排编辑器。
 *
 * 纯 React + 原生 Mouse/Touch 事件实现拖拽和连线。
 * 不引入任何第三方库（无 react-flow / @xyflow）。
 *
 * 交互：
 *   - 拖拽节点：按住节点拖动
 *   - 连线：点击"连接"按钮进入连线模式，依次点击源节点和目标节点
 *   - 删除：选中节点后按 Delete 或点击删除按钮
 *   - 编辑属性：选中节点后在右侧面板编辑名称/Prompt
 *   - 滚轮缩放（未来可扩展）
 *
 * 支持模式：Pipeline / Handoff / DAG / GroupChat / Debate
 */

'use client'

import { useState, useCallback, useRef, useEffect } from 'react'

// ===== 类型定义 =====

export type EditorMode = 'pipeline' | 'handoff' | 'dag' | 'groupchat' | 'debate'

export type NodeType = 'agent' | 'tool' | 'condition' | 'trigger'

export interface EditorNode {
  id: string
  type: NodeType
  name: string
  prompt?: string
  position: { x: number; y: number }
  data?: Record<string, unknown>
}

export interface EditorEdge {
  id: string
  source: string
  target: string
  label?: string
  condition?: string
}

export interface EditorGraph {
  nodes: EditorNode[]
  edges: EditorEdge[]
}

export interface VisualEditorProps {
  mode: EditorMode
  initialGraph?: EditorGraph
  onSave?: (graph: EditorGraph) => void
  onRun?: (graph: EditorGraph) => void
  height?: number | string
  readOnly?: boolean
}

// ===== 默认图数据 =====

const defaultGraphs: Record<EditorMode, EditorGraph> = {
  pipeline: {
    nodes: [
      { id: 'n1', type: 'trigger', name: 'Start', position: { x: 100, y: 100 } },
      { id: 'n2', type: 'agent', name: 'Research Agent', prompt: 'Research the topic', position: { x: 300, y: 100 } },
      { id: 'n3', type: 'agent', name: 'Writer Agent', prompt: 'Write a summary', position: { x: 500, y: 100 } },
    ],
    edges: [
      { id: 'e1', source: 'n1', target: 'n2' },
      { id: 'e2', source: 'n2', target: 'n3' },
    ],
  },
  handoff: {
    nodes: [
      { id: 'n1', type: 'agent', name: 'Triage Agent', prompt: 'Analyze request', position: { x: 300, y: 80 } },
      { id: 'n2', type: 'agent', name: 'Tech Support', prompt: 'Handle tech issues', position: { x: 150, y: 200 } },
      { id: 'n3', type: 'agent', name: 'Billing Support', prompt: 'Handle billing', position: { x: 450, y: 200 } },
    ],
    edges: [
      { id: 'e1', source: 'n1', target: 'n2', label: 'tech issue' },
      { id: 'e2', source: 'n1', target: 'n3', label: 'billing issue' },
    ],
  },
  dag: {
    nodes: [
      { id: 'n1', type: 'trigger', name: 'Input', position: { x: 100, y: 120 } },
      { id: 'n2', type: 'agent', name: 'Parallel Task 1', position: { x: 300, y: 60 } },
      { id: 'n3', type: 'agent', name: 'Parallel Task 2', position: { x: 300, y: 180 } },
      { id: 'n4', type: 'agent', name: 'Merge Results', position: { x: 500, y: 120 } },
    ],
    edges: [
      { id: 'e1', source: 'n1', target: 'n2' },
      { id: 'e2', source: 'n1', target: 'n3' },
      { id: 'e3', source: 'n2', target: 'n4' },
      { id: 'e4', source: 'n3', target: 'n4' },
    ],
  },
  groupchat: {
    nodes: [
      { id: 'n1', type: 'agent', name: 'Moderator', prompt: 'Facilitate discussion', position: { x: 300, y: 60 } },
      { id: 'n2', type: 'agent', name: 'Expert A', position: { x: 150, y: 180 } },
      { id: 'n3', type: 'agent', name: 'Expert B', position: { x: 450, y: 180 } },
    ],
    edges: [
      { id: 'e1', source: 'n1', target: 'n2' },
      { id: 'e2', source: 'n1', target: 'n3' },
    ],
  },
  debate: {
    nodes: [
      { id: 'n1', type: 'agent', name: 'Judge', prompt: 'Evaluate arguments', position: { x: 300, y: 60 } },
      { id: 'n2', type: 'agent', name: 'Pro Side', position: { x: 150, y: 180 } },
      { id: 'n3', type: 'agent', name: 'Con Side', position: { x: 450, y: 180 } },
    ],
    edges: [
      { id: 'e1', source: 'n2', target: 'n1' },
      { id: 'e2', source: 'n3', target: 'n1' },
    ],
  },
}

// ===== 主组件 =====

export function VisualEditor({
  mode,
  initialGraph,
  onSave,
  onRun,
  height = 600,
  readOnly = false,
}: VisualEditorProps) {
  const [graph, setGraph] = useState<EditorGraph>(initialGraph ?? defaultGraphs[mode])
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const [isConnecting, setIsConnecting] = useState(false)
  const [connectSource, setConnectSource] = useState<string | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [draggedNode, setDraggedNode] = useState<string | null>(null)
  const canvasRef = useRef<HTMLDivElement>(null)

  const selectedNodeData = graph.nodes.find((n) => n.id === selectedNode)

  // ===== 节点拖拽 =====

  const handleMouseDown = useCallback((e: React.MouseEvent, nodeId: string) => {
    if (readOnly) return
    if (isConnecting) return

    e.stopPropagation()
    const node = graph.nodes.find((n) => n.id === nodeId)
    if (!node) return

    setIsDragging(true)
    setDraggedNode(nodeId)
    setDragOffset({
      x: e.clientX - node.position.x,
      y: e.clientY - node.position.y,
    })
    setSelectedNode(nodeId)
  }, [graph.nodes, isConnecting, readOnly])

  const handleMouseMove = useCallback((e: React.MouseEvent) => {
    if (!isDragging || !draggedNode) return

    const canvasRect = canvasRef.current?.getBoundingClientRect()
    if (!canvasRect) return

    const x = e.clientX - dragOffset.x
    const y = e.clientY - dragOffset.y

    setGraph((prev) => ({
      ...prev,
      nodes: prev.nodes.map((n) =>
        n.id === draggedNode ? { ...n, position: { x: Math.max(0, x), y: Math.max(0, y) } } : n
      ),
    }))
  }, [isDragging, draggedNode, dragOffset])

  const handleMouseUp = useCallback(() => {
    setIsDragging(false)
    setDraggedNode(null)
  }, [])

  // ===== 连线模式 =====

  const handleNodeClick = useCallback((nodeId: string) => {
    if (readOnly) return
    if (!isConnecting) {
      setSelectedNode(nodeId)
      return
    }

    if (!connectSource) {
      setConnectSource(nodeId)
    } else {
      if (connectSource !== nodeId) {
        const edgeId = `e_${connectSource}_${nodeId}`
        // 避免重复连线
        setGraph((prev) => {
          if (prev.edges.some((e) => e.source === connectSource && e.target === nodeId)) {
            return prev
          }
          return {
            ...prev,
            edges: [...prev.edges, { id: edgeId, source: connectSource!, target: nodeId }],
          }
        })
      }
      setConnectSource(null)
      setIsConnecting(false)
    }
  }, [isConnecting, connectSource, readOnly])

  const startConnecting = useCallback(() => {
    setIsConnecting(true)
    setConnectSource(null)
    setSelectedNode(null)
  }, [])

  const cancelConnecting = useCallback(() => {
    setIsConnecting(false)
    setConnectSource(null)
  }, [])

  // ===== 节点操作 =====

  const addNode = useCallback((type: NodeType) => {
    const id = `n_${Date.now()}`
    const newNode: EditorNode = {
      id,
      type,
      name: `New ${type}`,
      position: { x: 150 + Math.random() * 200, y: 150 + Math.random() * 200 },
    }
    setGraph((prev) => ({ ...prev, nodes: [...prev.nodes, newNode] }))
    setSelectedNode(id)
  }, [])

  const deleteNode = useCallback((id: string) => {
    setGraph((prev) => ({
      nodes: prev.nodes.filter((n) => n.id !== id),
      edges: prev.edges.filter((e) => e.source !== id && e.target !== id),
    }))
    if (selectedNode === id) setSelectedNode(null)
  }, [selectedNode])

  const deleteEdge = useCallback((id: string) => {
    setGraph((prev) => ({ ...prev, edges: prev.edges.filter((e) => e.id !== id) }))
  }, [])

  const updateNode = useCallback((id: string, updates: Partial<EditorNode>) => {
    setGraph((prev) => ({
      ...prev,
      nodes: prev.nodes.map((n) => (n.id === id ? { ...n, ...updates } : n)),
    }))
  }, [])

  // ===== 键盘快捷键 =====

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Delete' || e.key === 'Backspace') {
        if (selectedNode && !readOnly) {
          deleteNode(selectedNode)
        }
      }
      if (e.key === 'Escape') {
        if (isConnecting) cancelConnecting()
        setSelectedNode(null)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [selectedNode, readOnly, deleteNode, isConnecting, cancelConnecting])

  const handleSave = useCallback(() => {
    onSave?.(graph)
  }, [graph, onSave])

  const handleRun = useCallback(() => {
    onRun?.(graph)
  }, [graph, onRun])

  // ===== 渲染 =====

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height,
        border: '1px solid #e5e7eb',
        borderRadius: 12,
        overflow: 'hidden',
        fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
      }}
    >
      {/* 工具栏 */}
      <div
        style={{
          display: 'flex',
          gap: 8,
          padding: '10px 14px',
          borderBottom: '1px solid #e5e7eb',
          background: '#f9fafb',
          alignItems: 'center',
        }}
      >
        <button onClick={() => addNode('agent')} style={btnStyle}>
          + Agent
        </button>
        <button onClick={() => addNode('tool')} style={btnStyle}>
          + Tool
        </button>
        <button onClick={() => addNode('condition')} style={btnStyle}>
          + Condition
        </button>
        <div style={{ width: 1, height: 20, background: '#d1d5db', margin: '0 4px' }} />
        {isConnecting ? (
          <button onClick={cancelConnecting} style={{ ...btnStyle, background: '#fef3c7', borderColor: '#f59e0b' }}>
            ✕ Cancel Connection
          </button>
        ) : (
          <button onClick={startConnecting} style={{ ...btnStyle, background: '#ecfdf5', borderColor: '#10b981' }}>
            🔗 Connect
          </button>
        )}
        <div style={{ flex: 1 }} />
        <button onClick={handleSave} style={{ ...btnStyle, background: '#6366f1', color: 'white', borderColor: '#6366f1' }}>
          💾 Save
        </button>
        <button onClick={handleRun} style={{ ...btnStyle, background: '#22c55e', color: 'white', borderColor: '#22c55e' }}>
          ▶ Run
        </button>
      </div>

      {/* 主体区域 */}
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* 画布 */}
        <div
          ref={canvasRef}
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseUp}
          onClick={() => { if (!isConnecting) setSelectedNode(null) }}
          style={{
            flex: 1,
            position: 'relative',
            background: isConnecting ? '#faf5ff' : '#fafafa',
            cursor: isConnecting ? 'crosshair' : 'default',
            overflow: 'hidden',
          }}
        >
          {/* 网格背景 */}
          <div
            style={{
              position: 'absolute',
              inset: 0,
              backgroundImage: 'radial-gradient(circle, #d1d5db 1px, transparent 1px)',
              backgroundSize: '20px 20px',
              opacity: 0.3,
            }}
          />

          {/* 连线模式提示 */}
          {isConnecting && (
            <div
              style={{
                position: 'absolute',
                top: 12,
                left: '50%',
                transform: 'translateX(-50%)',
                padding: '6px 16px',
                background: '#7c3aed',
                color: 'white',
                borderRadius: 20,
                fontSize: 13,
                fontWeight: 500,
                zIndex: 10,
              }}
            >
              {connectSource ? '🎯 Click target node to complete connection' : '👆 Click source node to start connection'}
            </div>
          )}

          {/* SVG 连线层 */}
          <svg
            style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', pointerEvents: 'none' }}
          >
            <defs>
              <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
                <polygon points="0 0, 10 3.5, 0 7" fill="#9ca3af" />
              </marker>
              <marker id="arrowhead-active" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
                <polygon points="0 0, 10 3.5, 0 7" fill="#6366f1" />
              </marker>
            </defs>
            {graph.edges.map((edge) => {
              const source = graph.nodes.find((n) => n.id === edge.source)
              const target = graph.nodes.find((n) => n.id === edge.target)
              if (!source || !target) return null
              const x1 = source.position.x + 75
              const y1 = source.position.y + 30
              const x2 = target.position.x + 75
              const y2 = target.position.y + 30
              return (
                <g key={edge.id}>
                  <line
                    x1={x1} y1={y1} x2={x2} y2={y2}
                    stroke="#9ca3af" strokeWidth={2} markerEnd="url(#arrowhead)"
                  />
                  {edge.label && (
                    <text x={(x1 + x2) / 2} y={(y1 + y2) / 2 - 8} textAnchor="middle" fontSize={11} fill="#6b7280">
                      {edge.label}
                    </text>
                  )}
                </g>
              )
            })}
          </svg>

          {/* 节点层 */}
          {graph.nodes.map((node) => {
            const isSelected = selectedNode === node.id
            const isConnectSource = connectSource === node.id
            const nodeColor = getNodeColor(node.type)
            return (
              <div
                key={node.id}
                onMouseDown={(e) => handleMouseDown(e, node.id)}
                onClick={(e) => { e.stopPropagation(); handleNodeClick(node.id) }}
                style={{
                  position: 'absolute',
                  left: node.position.x,
                  top: node.position.y,
                  padding: '10px 14px',
                  borderRadius: 10,
                  border: isConnectSource ? '2px solid #7c3aed' : isSelected ? '2px solid #6366f1' : '1px solid #d1d5db',
                  background: isConnectSource ? '#f3e8ff' : nodeColor,
                  cursor: isConnecting ? 'crosshair' : isDragging && draggedNode === node.id ? 'grabbing' : 'grab',
                  minWidth: 140,
                  boxShadow: isSelected
                    ? '0 4px 12px rgba(99, 102, 241, 0.3)'
                    : '0 2px 6px rgba(0,0,0,0.06)',
                  userSelect: 'none',
                  transition: isDragging && draggedNode === node.id ? 'none' : 'box-shadow 0.15s ease',
                }}
              >
                <div style={{ fontSize: 12, fontWeight: 600, color: '#374151', display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span>{getNodeIcon(node.type)}</span>
                  <span>{node.name}</span>
                </div>
                {node.prompt && (
                  <div style={{ fontSize: 11, color: '#6b7280', marginTop: 4, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 160 }}>
                    {node.prompt}
                  </div>
                )}
                {!readOnly && isSelected && (
                  <button
                    onClick={(e) => { e.stopPropagation(); deleteNode(node.id) }}
                    style={{
                      position: 'absolute', top: -8, right: -8, width: 22, height: 22,
                      borderRadius: 11, border: 'none', background: '#ef4444', color: 'white',
                      fontSize: 13, cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center',
                    }}
                  >
                    ×
                  </button>
                )}
              </div>
            )
          })}
        </div>

        {/* 属性面板 */}
        {selectedNodeData && !readOnly && (
          <div
            style={{
              width: 260,
              borderLeft: '1px solid #e5e7eb',
              padding: '16px',
              background: '#fff',
              overflowY: 'auto',
            }}
          >
            <h4 style={{ margin: '0 0 12px', fontSize: 14, fontWeight: 600, color: '#374151' }}>
              Edit Node
            </h4>
            <label style={labelStyle}>Name</label>
            <input
              value={selectedNodeData.name}
              onChange={(e) => updateNode(selectedNodeData.id, { name: e.target.value })}
              style={inputStyle}
            />
            <label style={labelStyle}>Type</label>
            <select
              value={selectedNodeData.type}
              onChange={(e) => updateNode(selectedNodeData.id, { type: e.target.value as NodeType })}
              style={inputStyle}
            >
              <option value="agent">Agent</option>
              <option value="tool">Tool</option>
              <option value="condition">Condition</option>
              <option value="trigger">Trigger</option>
            </select>
            <label style={labelStyle}>Prompt</label>
            <textarea
              value={selectedNodeData.prompt ?? ''}
              onChange={(e) => updateNode(selectedNodeData.id, { prompt: e.target.value })}
              placeholder="Enter agent prompt..."
              style={{ ...inputStyle, minHeight: 80, resize: 'vertical' }}
            />
            <div style={{ marginTop: 16, display: 'flex', gap: 8 }}>
              <button
                onClick={() => deleteNode(selectedNodeData.id)}
                style={{ ...btnStyle, background: '#fef2f2', borderColor: '#fca5a5', color: '#dc2626' }}
              >
                🗑 Delete
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// ===== 辅助函数 =====

function getNodeIcon(type: NodeType): string {
  switch (type) {
    case 'agent': return '🤖'
    case 'tool': return '🔧'
    case 'condition': return '❓'
    case 'trigger': return '⚡'
    default: return '📦'
  }
}

function getNodeColor(type: NodeType): string {
  switch (type) {
    case 'agent': return '#eef2ff'
    case 'tool': return '#f0fdf4'
    case 'condition': return '#fef3c7'
    case 'trigger': return '#fdf2f8'
    default: return '#f9fafb'
  }
}

const btnStyle: React.CSSProperties = {
  padding: '6px 12px',
  border: '1px solid #d1d5db',
  borderRadius: 6,
  background: 'white',
  fontSize: 13,
  cursor: 'pointer',
  transition: 'background 0.15s',
}

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: 12,
  fontWeight: 600,
  color: '#374151',
  marginBottom: 4,
  marginTop: 12,
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '7px 10px',
  border: '1px solid #d1d5db',
  borderRadius: 6,
  fontSize: 13,
  boxSizing: 'border-box',
  outline: 'none',
}

export default VisualEditor
