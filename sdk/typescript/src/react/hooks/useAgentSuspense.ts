/**
 * useAgentSuspense — 基于 React 19 `use()` API 的 Suspense 集成。
 *
 * 在渲染期间读取 agent.run(input) 返回的 Promise，触发 Suspense 边界（fallback）。
 * 对同一组件实例 + 相同 input 缓存 Promise，避免重复执行。
 *
 * 设计要点（与 react/use-agent.ts 保持一致）：
 * - 不直接 import react，由调用方通过 deps 注入 useState/useRef/use（React 作为 peerDependency）
 * - `use` 为 React 19 的并发读取 API，需在渲染期调用（不可在 effect 中调用）
 *
 * 使用方式（通过 react/index.ts 的 useAgentSuspenseWithReact 工厂绑定 React）：
 *   const useAgentSuspense = useAgentSuspenseWithReact(React, agent);
 *   function Profile() {
 *     const content = useAgentSuspense('请介绍一下你自己');
 *     return <p>{content}</p>;
 *   }
 */

import type { ReActAgent } from '../../agent/react-loop.js';
import type { Response } from '../../types.js';
import type { UseAgentDeps } from '../use-agent.js';

/** 注入 React 19 的 use 能力后的依赖集合 */
export type UseAgentSuspenseDeps = UseAgentDeps & {
  /** React 19 的 use(promise) 并发读取 API */
  use: <T>(promise: Promise<T>) => T;
};

/** useAgentSuspense 的纯逻辑实现，不直接依赖 React。 */
export function useAgentSuspenseImpl(
  deps: UseAgentSuspenseDeps,
  agent: ReActAgent,
  input: string,
): string {
  const promiseRef = deps.useRef<Promise<Response> | null>(null);
  const inputRef = deps.useRef<string>(input);

  // 同一组件实例 + 相同 input 复用 Promise，input 变化则重新运行
  if (promiseRef.current === null || inputRef.current !== input) {
    inputRef.current = input;
    promiseRef.current = agent.run(input);
  }

  const response = deps.use(promiseRef.current);
  return response.content;
}
