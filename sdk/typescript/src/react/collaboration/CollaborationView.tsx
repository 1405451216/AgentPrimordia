/**
 * CollaborationView — 实时多 Agent 协作主视图（T3-4）。
 *
 * 自包含实现（不依赖 reactflow 等图形库）：
 * - 顶部：参与 Agent 的状态卡片栏（AgentNode）
 * - 中部：消息流可视化（MessageFlow）
 * - 底部：人类介入面板（HITLPanel）
 *
 * 组件为受控/展示型：实时数据由消费方通过 props 注入（WebSocket / SSE 等）。
 */

import type { ReactElement } from 'react';
import { AgentNode } from './AgentNode.js';
import { MessageFlow } from './MessageFlow.js';
import { HITLPanel } from './HITLPanel.js';

/** Agent 协作状态 */
export type CollabAgentStatus =
  | 'idle'
  | 'thinking'
  | 'working'
  | 'waiting'
  | 'error'
  | 'done';

/** 参与协作的 Agent */
export interface CollaborationAgent {
  id: string;
  name: string;
  role?: string;
  status: CollabAgentStatus;
  /** 当前正在执行的任务描述 */
  activity?: string;
  /** 主题色（可选） */
  accent?: string;
}

/** 协作消息 */
export interface CollaborationMessage {
  id: string;
  /** 发送方 agent id / 'user' / 'system' */
  from: string;
  /** 接收方 agent id（可选） */
  to?: string;
  content: string;
  timestamp?: number;
  kind?: 'message' | 'tool_call' | 'tool_result' | 'error';
}

/** 人类介入审批请求 */
export interface ApprovalRequest {
  id: string;
  agentId: string;
  title: string;
  detail?: string;
  createdAt?: number;
}

/** 协作会话（数据由消费方注入） */
export interface CollaborationSession {
  id: string;
  title?: string;
  agents: CollaborationAgent[];
  messages: CollaborationMessage[];
  pendingApprovals: ApprovalRequest[];
  websocketUrl?: string;
  approve?: (id: string) => void | Promise<void>;
  reject?: (id: string) => void | Promise<void>;
}

/** 人类可读的状态文案（定义见 AgentNode.tsx，避免循环依赖） */

export interface CollaborationViewProps {
  session: CollaborationSession;
  onApprove?: (id: string) => void;
  onReject?: (id: string) => void;
}

export function CollaborationView({
  session,
  onApprove,
  onReject,
}: CollaborationViewProps): ReactElement {
  const approve = onApprove ?? session.approve;
  const reject = onReject ?? session.reject;

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: 480,
        fontFamily: 'system-ui, sans-serif',
        background: '#f8fafc',
      }}
    >
      <header
        style={{
          padding: '12px 16px',
          borderBottom: '1px solid #e2e8f0',
          background: '#fff',
        }}
      >
        <div style={{ fontSize: 15, fontWeight: 700, color: '#0f172a' }}>
          {session.title ?? '多 Agent 协作'}
        </div>
        <div style={{ display: 'flex', gap: 12, marginTop: 12, flexWrap: 'wrap' }}>
          {session.agents.map((a) => (
            <AgentNode key={a.id} agent={a} />
          ))}
        </div>
      </header>

      <main style={{ flex: 1, overflow: 'auto', padding: 16 }}>
        <MessageFlow messages={session.messages} agents={session.agents} />
      </main>

      <HITLPanel approvals={session.pendingApprovals} onApprove={approve} onReject={reject} />
    </div>
  );
}
