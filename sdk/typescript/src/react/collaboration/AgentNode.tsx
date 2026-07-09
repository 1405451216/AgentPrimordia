/**
 * AgentNode — 协作视图中的 Agent 状态卡片（T3-4）。
 */

import type { ReactElement } from 'react';
import type { CollaborationAgent, CollabAgentStatus } from './CollaborationView.js';

/** 人类可读的状态文案 */
export const STATUS_LABEL: Record<CollabAgentStatus, string> = {
  idle: '空闲',
  thinking: '思考中',
  working: '工作中',
  waiting: '等待中',
  error: '错误',
  done: '已完成',
};

export interface AgentNodeProps {
  agent: CollaborationAgent;
}

const STATUS_COLOR: Record<string, string> = {
  idle: '#94a3b8',
  thinking: '#6366f1',
  working: '#34d399',
  waiting: '#fbbf24',
  error: '#ef4444',
  done: '#22c55e',
};

export function AgentNode({ agent }: AgentNodeProps): ReactElement {
  const accent = agent.accent ?? STATUS_COLOR[agent.status] ?? '#6366f1';
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '6px 10px',
        borderRadius: 10,
        background: '#f8fafc',
        border: `1px solid ${accent}55`,
        minWidth: 140,
      }}
    >
      <span
        style={{
          width: 10,
          height: 10,
          borderRadius: '50%',
          background: accent,
          boxShadow: `0 0 0 3px ${accent}22`,
        }}
      />
      <div>
        <div style={{ fontSize: 13, fontWeight: 600, color: '#0f172a' }}>{agent.name}</div>
        <div style={{ fontSize: 11, color: '#64748b' }}>
          {agent.role ? `${agent.role} · ` : ''}
          {STATUS_LABEL[agent.status]}
          {agent.activity ? ` · ${agent.activity}` : ''}
        </div>
      </div>
    </div>
  );
}
