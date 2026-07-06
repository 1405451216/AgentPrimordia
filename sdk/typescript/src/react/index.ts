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
import type { Agent, AgentConfig } from '../agent/builder.js';

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