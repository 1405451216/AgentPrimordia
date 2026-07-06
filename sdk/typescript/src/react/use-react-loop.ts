/**
 * useReActLoop Hook — 实时显示 ReAct 思考-行动-观察过程。
 *
 * 使用方式：
 *   const { thoughts, actions, observations, currentStep, run } = useReActLoop();
 *   <button onClick={() => run('分析这段代码')}>运行</button>
 *
 * 设计要点（Phase 5 Task 8）：
 * - 暴露 ReAct 三段论（Thought / Action / Observation）
 * - currentStep 标识当前阶段（方便在 UI 显示 loading）
 * - 纯逻辑实现，React 依赖由调用方注入
 */

import type { UseAgentDeps } from './use-agent.js';
import type { UseAgentResult } from './use-agent.js';
import { makeState } from './use-agent.js';

/** ReAct Loop 的工具调用 */
export interface ReActAction {
  /** 工具名称 */
  tool: string;
  /** 工具参数（已解析的 JSON） */
  args: unknown;
  /** 调用产生时间戳（自 epoch 起的毫秒） */
  timestamp: number;
}

/** ReAct Loop 的观察结果 */
export interface ReActObservation {
  /** 工具返回内容 */
  content: string;
  /** 是否为错误 */
  isError: boolean;
  /** 时间戳 */
  timestamp: number;
}

/** ReAct 当前阶段 */
export type ReActStep = 'idle' | 'thinking' | 'acting' | 'observing' | 'done';

/** useReActLoop 返回值 */
export interface UseReActLoopResult {
  /** 历次 LLM 思考文本 */
  thoughts: string[];
  /** 历次工具调用 */
  actions: ReActAction[];
  /** 历次观察 */
  observations: ReActObservation[];
  /** 当前阶段 */
  currentStep: ReActStep;
  /** 当前轮次（从 1 开始） */
  turn: number;
  /** 错误（如有） */
  error: Error | null;
  /** 是否运行中 */
  isRunning: boolean;
  /** 启动一次 ReAct 运行；与 useAgent 共享结果 */
  run: UseAgentResult['run'];
  /** 取消 */
  stop: UseAgentResult['stop'];
  /** 清空所有历史 */
  reset: () => void;
}

/** 模拟 Agent 流式事件的接口（可在测试中注入） */
export interface ReActStreamer {
  /** 返回 AsyncIterable，每次 yield 一个事件 */
  stream(prompt: string): AsyncIterable<ReActStreamEvent>;
}

/** ReAct 流式事件 */
export type ReActStreamEvent =
  | { type: 'thought'; text: string }
  | { type: 'action'; tool: string; args: unknown }
  | { type: 'observation'; content: string; isError: boolean }
  | { type: 'turn'; turn: number }
  | { type: 'done'; text: string }
  | { type: 'error'; error: Error };

/** useReActLoop 纯逻辑实现。 */
export function useReActLoopImpl(
  deps: UseAgentDeps,
  streamerFactory: () => ReActStreamer,
): UseReActLoopResult {
  const thoughtsRef = makeState<string[]>(deps, []);
  const actionsRef = makeState<ReActAction[]>(deps, []);
  const observationsRef = makeState<ReActObservation[]>(deps, []);
  const currentStepRef = makeState<ReActStep>(deps, 'idle');
  const turnRef = makeState<number>(deps, 0);
  const errorRef = makeState<Error | null>(deps, null);
  const isRunningRef = makeState<boolean>(deps, false);

  const runImpl = async (prompt: string) => {
    if (isRunningRef.get()) return;
    isRunningRef.set(true);
    errorRef.set(null);
    thoughtsRef.set([]);
    actionsRef.set([]);
    observationsRef.set([]);
    turnRef.set(0);
    currentStepRef.set('thinking');

    try {
      const streamer = streamerFactory();
      const events = streamer.stream(prompt);
      for await (const ev of events) {
        switch (ev.type) {
          case 'thought':
            thoughtsRef.set((prev) => [...prev, ev.text]);
            currentStepRef.set('acting');
            break;
          case 'action':
            actionsRef.set((prev) => [...prev, {
              tool: ev.tool,
              args: ev.args,
              timestamp: Date.now(),
            }]);
            currentStepRef.set('observing');
            break;
          case 'observation':
            observationsRef.set((prev) => [...prev, {
              content: ev.content,
              isError: ev.isError,
              timestamp: Date.now(),
            }]);
            currentStepRef.set('thinking');
            break;
          case 'turn':
            turnRef.set(ev.turn);
            break;
          case 'done':
            currentStepRef.set('done');
            break;
          case 'error':
            errorRef.set(ev.error);
            break;
        }
      }
      currentStepRef.set('done');
    } catch (e) {
      errorRef.set(e as Error);
    } finally {
      isRunningRef.set(false);
    }
  };

  const run = deps.useCallback(runImpl, []);

  const stop = deps.useCallback(() => {
    isRunningRef.set(false);
    currentStepRef.set('idle');
  }, []);

  const reset = deps.useCallback(() => {
    thoughtsRef.set([]);
    actionsRef.set([]);
    observationsRef.set([]);
    currentStepRef.set('idle');
    turnRef.set(0);
    errorRef.set(null);
    isRunningRef.set(false);
  }, []);

  return {
    get thoughts() { return thoughtsRef.get(); },
    get actions() { return actionsRef.get(); },
    get observations() { return observationsRef.get(); },
    get currentStep() { return currentStepRef.get(); },
    get turn() { return turnRef.get(); },
    get error() { return errorRef.get(); },
    get isRunning() { return isRunningRef.get(); },
    run,
    stop,
    reset,
  };
}