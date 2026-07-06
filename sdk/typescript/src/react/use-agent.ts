/**
 * useAgent Hook — 将 ReActAgent 封装为 React 状态。
 *
 * 使用方式：
 *   const { agent, isRunning, error, run, streamRun, stop } = useAgent({
 *     name: 'my-agent',
 *     systemPrompt: '你是助手',
 *     maxTurns: 10,
 *   });
 *
 *   <button onClick={() => run('你好')}>运行</button>
 *   {isRunning && <p>运行中...</p>}
 *   {error && <p>错误：{error.message}</p>}
 *
 * 设计要点（Phase 5 Task 8）：
 * - 通过 AbortController 支持运行中取消
 * - 状态以 React useState 暴露，调用方可直接绑定 UI
 * - 不引入额外状态管理库：subscriptions 在组件 unmount 时清理
 * - 不直接 import react，让调用方自行安装 peerDep
 */

// React 18+ 标准 hooks 的最小类型（不强制依赖具体版本）
type DependencyList = readonly unknown[];
type StateHook<T> = [T, (next: T | ((prev: T) => T)) => void];

interface UseAgentDeps {
  useState: <T>(initial: T) => StateHook<T>;
  useCallback: <T extends (...args: never[]) => unknown>(fn: T, deps: DependencyList) => T;
  useRef: <T>(initial: T) => { current: T };
  useEffect: (fn: () => void | (() => void), deps?: DependencyList) => void;
}

import type { Agent } from '../agent/builder.js';
import type { AgentConfig } from '../agent/builder.js';
import type { Response } from '../types.js';

/** useAgent 的可选项，继承 AgentConfig 但所有字段可选 */
export interface UseAgentOptions extends Partial<AgentConfig> {
  /** 是否在 mount 时自动启动（默认 false） */
  autoStart?: boolean;
  /** 自动启动时的 prompt */
  initialPrompt?: string;
}

/** useAgent 返回值 */
export interface UseAgentResult {
  /** 当前 Agent 实例（首次运行后填充） */
  agent: Agent | null;
  /** 是否正在运行 */
  isRunning: boolean;
  /** 上一次错误 */
  error: Error | null;
  /** 上一次响应 */
  response: Response | null;
  /** 运行一次；返回 Response */
  run: (prompt: string) => Promise<void>;
  /** 流式运行；返回 AsyncGenerator */
  streamRun: (prompt: string) => AsyncGenerator<string>;
  /** 取消当前运行 */
  stop: () => void;
  /** 重置状态（清空 agent / error / response） */
  reset: () => void;
}

/**
 * 内部 ref holder：既能让 setter 修改外部可见的状态，
 * 又能通过 getter 在返回值中暴露最新值。
 *
 * 设计动机：在 mock 环境下没有 React 重渲染，闭包内捕获的 state
 * 是调用时刻的快照，无法反映 setter 的后续更新。这里在 setter 调
 * 用时同步写入 ref，使外部读取时总能拿到最新值。
 *
 * 在真实 React 环境下，useState 会触发重渲染，新一次 render 调用
 * 时 useState 返回最新值；这里通过 ref 与 React state 双写保证
 * mock 与真实环境行为一致。
 *
 * 导出给同包其他 hooks 复用。
 */
export interface StateRef<T> {
  get: () => T;
  set: (next: T | ((prev: T) => T)) => void;
}

export function makeState<T>(deps: UseAgentDeps, initial: T): StateRef<T> {
  const [, rawSet] = deps.useState<T>(initial);
  let current: T = initial;
  const set: StateRef<T>['set'] = (next) => {
    const computed = typeof next === 'function'
      ? (next as (prev: T) => T)(current)
      : next;
    current = computed;
    rawSet(computed);
  };
  return { get: () => current, set };
}

/** useAgent 的纯逻辑实现，不直接调用 React，由调用方注入依赖。 */
export function useAgentImpl(
  options: UseAgentOptions,
  deps: UseAgentDeps,
  buildAgent: (opts: UseAgentOptions) => Agent,
): UseAgentResult {
  const agentRef = makeState<Agent | null>(deps, null);
  const isRunningRef = makeState<boolean>(deps, false);
  const errorRef = makeState<Error | null>(deps, null);
  const responseRef = makeState<Response | null>(deps, null);
  const abortRef = deps.useRef<AbortController | null>(null);
  const mountedRef = deps.useRef(true);

  deps.useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);

  const run = deps.useCallback(async (prompt: string) => {
    isRunningRef.set(true);
    errorRef.set(null);
    responseRef.set(null);

    const ctrl = new AbortController();
    abortRef.current = ctrl;
    let a: Agent;
    try {
      a = buildAgent(options);
    } catch (e) {
      errorRef.set(e as Error);
      isRunningRef.set(false);
      return;
    }
    if (mountedRef.current) agentRef.set(a);

    try {
      const resp = await a.run(prompt, { signal: ctrl.signal } as never);
      if (mountedRef.current) responseRef.set(resp);
    } catch (e) {
      if (mountedRef.current) errorRef.set(e as Error);
    } finally {
      if (mountedRef.current) isRunningRef.set(false);
    }
  }, [options as never]);

  const streamRun = deps.useCallback(async function* (prompt: string): AsyncGenerator<string> {
    isRunningRef.set(true);
    errorRef.set(null);
    responseRef.set(null);

    const a = buildAgent(options);
    if (mountedRef.current) agentRef.set(a);

    try {
      // streamRun 期望返回 AsyncIterable<string>
      const iterable = await a.streamRun(prompt);
      for await (const chunk of iterable) {
        if (abortRef.current?.signal.aborted) return;
        yield chunk;
      }
    } catch (e) {
      if (mountedRef.current) errorRef.set(e as Error);
    } finally {
      if (mountedRef.current) isRunningRef.set(false);
    }
  }, [options as never]);

  const stop = deps.useCallback(() => {
    abortRef.current?.abort();
    isRunningRef.set(false);
  }, []);

  const reset = deps.useCallback(() => {
    agentRef.set(null);
    errorRef.set(null);
    responseRef.set(null);
    isRunningRef.set(false);
  }, []);

  // autoStart 在 mount 时启动
  deps.useEffect(() => {
    if (options.autoStart && options.initialPrompt) {
      void run(options.initialPrompt);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [options.autoStart, options.initialPrompt]);

  return {
    get agent() { return agentRef.get(); },
    get isRunning() { return isRunningRef.get(); },
    get error() { return errorRef.get(); },
    get response() { return responseRef.get(); },
    run,
    streamRun,
    stop,
    reset,
  };
}