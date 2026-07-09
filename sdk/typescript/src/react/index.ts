/**
 * React Hooks 子包入口（Phase 5 Task 8）。
 *
 * 通过懒加载 react，让 import 该模块时不强制安装 react 依赖。
 * 用户需要：
 * 1. 安装 react: `npm install react@>=18`
 * 2. 调用 useAgentXxxWithReact() 工厂方法
 */

export {
  useAgentImpl,
  type UseAgentDeps,
  type UseAgentOptions,
  type UseAgentResult,
} from './use-agent.js';

export {
  useReActLoopImpl,
  type ReActAction,
  type ReActObservation,
  type ReActStep,
  type ReActStreamEvent,
  type ReActStreamer,
  type UseReActLoopResult,
} from './use-react-loop.js';

export {
  useStreamRunImpl,
  type StreamProvider,
  type StreamRunOptions,
  type UseStreamRunResult,
} from './use-stream-run.js';

import {
  useAgentImpl,
  type UseAgentOptions,
  type UseAgentResult,
  type UseAgentDeps,
} from './use-agent.js';
import type { Agent, AgentConfig } from './use-agent.js';
import { useReActLoopImpl } from './use-react-loop.js';
import { useStreamRunImpl } from './use-stream-run.js';

/**
 * useAgentWithReact：构造绑定到具体 React 实例的 useAgent Hook。
 *
 * 用法：
 *   import * as React from 'react';
 *   import { useAgentWithReact } from '@agentprimordia/sdk/react';
 *   const useAgent = useAgentWithReact(React);
 *   const { run, isRunning } = useAgent({ name: 'my-agent' });
 */
export function useAgentWithReact(
  React: {
    useState: UseAgentDeps['useState'];
    useCallback: UseAgentDeps['useCallback'];
    useRef: UseAgentDeps['useRef'];
    useEffect: UseAgentDeps['useEffect'];
  },
  buildAgent: (opts: UseAgentOptions) => Agent,
) {
  return function useAgent(options: UseAgentOptions = {}): UseAgentResult {
    return useAgentImpl(options, React, buildAgent);
  };
}

/**
 * useReActLoopWithReact：构造绑定到具体 React 实例的 useReActLoop Hook。
 */
export function useReActLoopWithReact(
  React: {
    useState: UseAgentDeps['useState'];
    useCallback: UseAgentDeps['useCallback'];
    useRef: UseAgentDeps['useRef'];
    useEffect: UseAgentDeps['useEffect'];
  },
  streamerFactory: () => import('./use-react-loop.js').ReActStreamer,
) {
  return function useReActLoop() {
    return useReActLoopImpl(React, streamerFactory);
  };
}

/**
 * useStreamRunWithReact：构造绑定到具体 React 实例的 useStreamRun Hook。
 */
export function useStreamRunWithReact(
  React: {
    useState: UseAgentDeps['useState'];
    useCallback: UseAgentDeps['useCallback'];
    useRef: UseAgentDeps['useRef'];
    useEffect: UseAgentDeps['useEffect'];
  },
  providerFactory: () => import('./use-stream-run.js').StreamProvider,
) {
  return function useStreamRun() {
    return useStreamRunImpl(React, providerFactory);
  };
}

export type { Agent, AgentConfig };

// ===== T2-4: React 19 深度集成 =====

import { useAgentStreamImpl, type UseAgentStreamOptions, type UseAgentStreamResult } from './hooks/useAgentStream.js';
import { useAgentSuspenseImpl, type UseAgentSuspenseDeps } from './hooks/useAgentSuspense.js';
import type { ReActAgent } from '../agent/react-loop.js';

/**
 * useAgentStreamWithReact：构造绑定到具体 React 实例的 useAgentStream Hook。
 *
 * 用法：
 *   const useAgentStream = useAgentStreamWithReact(React, agent, { prompt: '你好' });
 *   const { content, isStreaming } = useAgentStream();
 */
export function useAgentStreamWithReact(
  React: {
    useState: UseAgentDeps['useState'];
    useCallback: UseAgentDeps['useCallback'];
    useRef: UseAgentDeps['useRef'];
    useEffect: UseAgentDeps['useEffect'];
  },
  agent: ReActAgent,
  options?: UseAgentStreamOptions,
) {
  return function useAgentStream(opts?: UseAgentStreamOptions): UseAgentStreamResult {
    return useAgentStreamImpl(React, agent, opts ?? options);
  };
}

/**
 * useAgentSuspenseWithReact：构造绑定到具体 React 19 实例的 useAgentSuspense Hook。
 *
 * 用法：
 *   const useAgentSuspense = useAgentSuspenseWithReact(React, agent);
 *   function View() { const content = useAgentSuspense('介绍一下你自己'); return <p>{content}</p>; }
 */
export function useAgentSuspenseWithReact(
  React: {
    useState: UseAgentDeps['useState'];
    useCallback: UseAgentDeps['useCallback'];
    useRef: UseAgentDeps['useRef'];
    useEffect: UseAgentDeps['useEffect'];
    use: <T>(promise: Promise<T>) => T;
  },
  agent: ReActAgent,
) {
  return function useAgentSuspense(input: string): string {
    return useAgentSuspenseImpl(React as UseAgentSuspenseDeps, agent, input);
  };
}

export { useAgentStreamImpl, useAgentSuspenseImpl };

export type {
  UseAgentStreamOptions,
  UseAgentStreamResult,
} from './hooks/useAgentStream.js';
export type { UseAgentSuspenseDeps } from './hooks/useAgentSuspense.js';
// AgentServerComponent 为 RSC（依赖 react 运行时），仅导出类型以避免破坏 react 子包的懒加载约定
export type { AgentServerComponentProps } from './server-components/AgentServerComponent.js';

// ===== T3-4: 实时多 Agent 协作 UI =====

export { CollaborationView } from './collaboration/CollaborationView.js';
export { AgentNode as CollaborationAgentNode, STATUS_LABEL } from './collaboration/AgentNode.js';
export { MessageFlow } from './collaboration/MessageFlow.js';
export { HITLPanel } from './collaboration/HITLPanel.js';
export { CollaborationReplay } from './collaboration/CollaborationReplay.js';
export type {
  CollabAgentStatus,
  CollaborationAgent,
  CollaborationMessage,
  ApprovalRequest,
  CollaborationSession,
} from './collaboration/CollaborationView.js';
export type { AgentNodeProps } from './collaboration/AgentNode.js';
export type { MessageFlowProps } from './collaboration/MessageFlow.js';
export type { HITLPanelProps } from './collaboration/HITLPanel.js';
export type { CollaborationReplayProps } from './collaboration/CollaborationReplay.js';