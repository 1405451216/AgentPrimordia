/**
 * MessageFlow — 协作消息流可视化（T3-4）。
 *
 * 自包含时间线渲染：每个消息显示为带发送方/接收方与类型徽标的气泡，
 * 通过左侧连接线串联，呈现多 Agent 协作过程。不依赖 reactflow。
 */

import type { ReactElement } from 'react';
import type { CollaborationAgent, CollaborationMessage } from './CollaborationView.js';

export interface MessageFlowProps {
  messages: CollaborationMessage[];
  agents: CollaborationAgent[];
}

const KIND_BADGE: Record<string, { label: string; color: string }> = {
  message: { label: '消息', color: '#6366f1' },
  tool_call: { label: '工具调用', color: '#0ea5e9' },
  tool_result: { label: '工具结果', color: '#34d399' },
  error: { label: '错误', color: '#ef4444' },
};

export function MessageFlow({ messages, agents }: MessageFlowProps): ReactElement {
  const nameById = new Map(agents.map((a) => [a.id, a.name]));

  const nameOf = (id: string): string => {
    if (id === 'user') return '用户';
    if (id === 'system') return '系统';
    return nameById.get(id) ?? id;
  };

  if (messages.length === 0) {
    return (
      <div style={{ color: '#94a3b8', fontSize: 13, padding: 24, textAlign: 'center' }}>
        暂无协作消息
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {messages.map((msg) => {
        const badge = KIND_BADGE[msg.kind ?? 'message'] ?? KIND_BADGE.message;
        return (
          <div key={msg.id} style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
            <div
              style={{
                width: 8,
                height: 8,
                marginTop: 8,
                borderRadius: '50%',
                background: badge.color,
                flexShrink: 0,
              }}
            />
            <div
              style={{
                flex: 1,
                padding: '8px 12px',
                borderRadius: 10,
                background: '#fff',
                border: '1px solid #e2e8f0',
                boxShadow: '0 1px 3px rgba(15,23,42,0.06)',
              }}
            >
              <div style={{ fontSize: 12, color: '#64748b', marginBottom: 4 }}>
                <strong style={{ color: '#0f172a' }}>{nameOf(msg.from)}</strong>
                {msg.to ? ` → ${nameOf(msg.to)}` : ''}
                <span
                  style={{
                    marginLeft: 8,
                    padding: '1px 6px',
                    borderRadius: 6,
                    fontSize: 10,
                    color: '#fff',
                    background: badge.color,
                  }}
                >
                  {badge.label}
                </span>
              </div>
              <div style={{ fontSize: 13, color: '#1e293b', whiteSpace: 'pre-wrap' }}>
                {msg.content}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
