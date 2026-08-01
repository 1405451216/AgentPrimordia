/**
 * collaboration.test.ts — 协作组件类型导出与 props 验证测试。
 *
 * 测试 collaboration 子模块导出的组件和类型：
 * - CollaborationView
 * - AgentNode (CollaborationAgentNode)
 * - MessageFlow
 * - HITLPanel
 * - CollaborationReplay
 *
 * 由于运行在 node 环境（无 DOM），采用静态分析方式验证：
 * - 导出存在性
 * - 类型定义完整性
 * - 组件 props 接口结构
 */

import { describe, it, expect } from 'vitest';
import {
  CollaborationView,
  CollaborationAgentNode,
  MessageFlow,
  HITLPanel,
  CollaborationReplay,
  STATUS_LABEL,
} from '../index.js';

import type {
  CollabAgentStatus,
  CollaborationAgent,
  CollaborationMessage,
  ApprovalRequest,
  CollaborationSession,
  AgentNodeProps,
  MessageFlowProps,
  HITLPanelProps,
  CollaborationReplayProps,
} from '../index.js';

// ===== 导出存在性测试 =====

describe('collaboration module exports', () => {
  it('should export CollaborationView as a function', () => {
    expect(typeof CollaborationView).toBe('function');
  });

  it('should export AgentNode as CollaborationAgentNode', () => {
    expect(typeof CollaborationAgentNode).toBe('function');
  });

  it('should export MessageFlow as a function', () => {
    expect(typeof MessageFlow).toBe('function');
  });

  it('should export HITLPanel as a function', () => {
    expect(typeof HITLPanel).toBe('function');
  });

  it('should export CollaborationReplay as a function', () => {
    expect(typeof CollaborationReplay).toBe('function');
  });

  it('should export STATUS_LABEL constant', () => {
    expect(STATUS_LABEL).toBeDefined();
    expect(typeof STATUS_LABEL).toBe('object');
  });
});

// ===== STATUS_LABEL 完整性测试 =====

describe('STATUS_LABEL', () => {
  const allStatuses: CollabAgentStatus[] = ['idle', 'thinking', 'working', 'waiting', 'error', 'done'];

  it('should have labels for all CollabAgentStatus values', () => {
    for (const status of allStatuses) {
      expect(STATUS_LABEL[status]).toBeDefined();
      expect(typeof STATUS_LABEL[status]).toBe('string');
      expect(STATUS_LABEL[status].length).toBeGreaterThan(0);
    }
  });

  it('should have correct Chinese labels', () => {
    expect(STATUS_LABEL.idle).toBe('空闲');
    expect(STATUS_LABEL.thinking).toBe('思考中');
    expect(STATUS_LABEL.working).toBe('工作中');
    expect(STATUS_LABEL.waiting).toBe('等待中');
    expect(STATUS_LABEL.error).toBe('错误');
    expect(STATUS_LABEL.done).toBe('已完成');
  });
});

// ===== 类型结构验证（通过构造合法数据验证接口兼容性）=====

describe('CollaborationAgent type structure', () => {
  it('should accept valid CollaborationAgent objects', () => {
    const agent: CollaborationAgent = {
      id: 'agent-1',
      name: 'Test Agent',
      status: 'idle',
    };
    expect(agent.id).toBe('agent-1');
    expect(agent.name).toBe('Test Agent');
    expect(agent.status).toBe('idle');
  });

  it('should accept CollaborationAgent with optional fields', () => {
    const agent: CollaborationAgent = {
      id: 'agent-2',
      name: 'Full Agent',
      status: 'working',
      role: 'researcher',
      activity: 'Searching documents',
      accent: '#ff0000',
    };
    expect(agent.role).toBe('researcher');
    expect(agent.activity).toBe('Searching documents');
    expect(agent.accent).toBe('#ff0000');
  });
});

describe('CollaborationMessage type structure', () => {
  it('should accept valid CollaborationMessage objects', () => {
    const msg: CollaborationMessage = {
      id: 'msg-1',
      from: 'agent-1',
      to: 'agent-2',
      content: 'Hello from agent 1',
      timestamp: Date.now(),
      kind: 'message',
    };
    expect(msg.id).toBe('msg-1');
    expect(msg.from).toBe('agent-1');
    expect(msg.kind).toBe('message');
  });

  it('should support all message kinds', () => {
    const kinds: Array<CollaborationMessage['kind']> = ['message', 'tool_call', 'tool_result', 'error'];
    for (const kind of kinds) {
      const msg: CollaborationMessage = {
        id: `msg-${kind}`,
        from: 'system',
        content: 'test',
        kind,
      };
      expect(msg.kind).toBe(kind);
    }
  });
});

describe('ApprovalRequest type structure', () => {
  it('should accept valid ApprovalRequest objects', () => {
    const approval: ApprovalRequest = {
      id: 'approval-1',
      agentId: 'agent-1',
      title: 'Approve deployment',
      detail: 'Deploy to production?',
      createdAt: Date.now(),
    };
    expect(approval.id).toBe('approval-1');
    expect(approval.agentId).toBe('agent-1');
    expect(approval.title).toBe('Approve deployment');
  });
});

describe('CollaborationSession type structure', () => {
  it('should accept a complete CollaborationSession', () => {
    const session: CollaborationSession = {
      id: 'session-1',
      title: 'Test Session',
      agents: [
        { id: 'a1', name: 'Agent 1', status: 'idle' },
        { id: 'a2', name: 'Agent 2', status: 'working' },
      ],
      messages: [
        { id: 'm1', from: 'a1', to: 'a2', content: 'Start task' },
      ],
      pendingApprovals: [
        { id: 'ap1', agentId: 'a1', title: 'Confirm' },
      ],
      websocketUrl: 'ws://localhost:8080/ws',
    };
    expect(session.id).toBe('session-1');
    expect(session.agents.length).toBe(2);
    expect(session.messages.length).toBe(1);
    expect(session.pendingApprovals.length).toBe(1);
  });
});

// ===== Props 接口验证 =====

describe('Component props interfaces', () => {
  it('AgentNodeProps should require agent field', () => {
    const props: AgentNodeProps = {
      agent: { id: 'a1', name: 'Test', status: 'idle' },
    };
    expect(props.agent).toBeDefined();
    expect(props.agent.id).toBe('a1');
  });

  it('MessageFlowProps should require messages and agents', () => {
    const props: MessageFlowProps = {
      messages: [],
      agents: [],
    };
    expect(Array.isArray(props.messages)).toBe(true);
    expect(Array.isArray(props.agents)).toBe(true);
  });

  it('HITLPanelProps should require approvals', () => {
    const props: HITLPanelProps = {
      approvals: [],
    };
    expect(Array.isArray(props.approvals)).toBe(true);
  });

  it('HITLPanelProps should accept optional callbacks', () => {
    const props: HITLPanelProps = {
      approvals: [],
      onApprove: (_id: string) => { /* noop */ },
      onReject: (_id: string) => { /* noop */ },
    };
    expect(typeof props.onApprove).toBe('function');
    expect(typeof props.onReject).toBe('function');
  });

  it('CollaborationReplayProps should accept messages and optional config', () => {
    const props: CollaborationReplayProps = {
      messages: [
        { id: 'm1', from: 'user', content: 'hello' },
      ],
      agents: [{ id: 'a1', name: 'Agent', status: 'idle' as CollabAgentStatus }],
      intervalMs: 500,
    };
    expect(props.messages.length).toBe(1);
    expect(props.intervalMs).toBe(500);
  });
});
