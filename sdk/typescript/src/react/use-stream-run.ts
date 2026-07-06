/**
 * useStreamRun Hook — 流式运行，逐 token 累积输出。
 *
 * 使用方式：
 *   const { text, isStreaming, error, run, stop } = useStreamRun();
 *   <pre>{text}</pre>
 *
 * 设计要点（Phase 5 Task 8）：
 * - 与 useAgent 类似，但专注于流式累积
 * - 支持 AbortSignal 取消
 * - 不依赖 React 18+ 新 API（如 use()）以兼容 React 16/17
 */

import type { UseAgentDeps } from './use-agent.js';
import { makeState } from './use-agent.js';

/** useStreamRun 的输入参数 */
export interface StreamRunOptions {
  /** 运行选项（如 agent 配置、prompt prefix） */
  prompt: string;
  /** 是否启用流式（默认 true） */
  enabled?: boolean;
}

/** useStreamRun 返回值 */
export interface UseStreamRunResult {
  /** 当前累积的文本 */
  text: string;
  /** 是否正在流式 */
  isStreaming: boolean;
  /** 错误 */
  error: Error | null;
  /** token 计数（估算） */
  tokenCount: number;
  /** 运行 */
  run: (opts: StreamRunOptions) => Promise<void>;
  /** 重新流（使用最近一次 prompt） */
  rerun: () => Promise<void>;
  /** 取消当前流 */
  stop: () => void;
  /** 清空累积文本 */
  reset: () => void;
}

/** 底层流式提供者 */
export interface StreamProvider {
  /** 返回 AsyncIterable，逐 token 产出字符串 */
  stream(prompt: string, signal: AbortSignal): AsyncIterable<string>;
}

/** useStreamRun 纯逻辑实现。 */
export function useStreamRunImpl(
  deps: UseAgentDeps,
  providerFactory: () => StreamProvider,
): UseStreamRunResult {
  const textRef = makeState<string>(deps, '');
  const isStreamingRef = makeState<boolean>(deps, false);
  const errorRef = makeState<Error | null>(deps, null);
  const tokenCountRef = makeState<number>(deps, 0);
  const lastPromptRef = makeState<string>(deps, '');
  const ctrlRef = deps.useRef<AbortController | null>(null);

  const runImpl = async (opts: StreamRunOptions) => {
    if (!opts.prompt) {
      errorRef.set(new Error('useStreamRun: prompt 不能为空'));
      return
    }
    lastPromptRef.set(opts.prompt);
    textRef.set('');
    errorRef.set(null);
    tokenCountRef.set(0);
    isStreamingRef.set(true);

    const ctrl = new AbortController();
    ctrlRef.current?.abort();
    ctrlRef.current = ctrl;

    const provider = providerFactory();
    try {
      let count = 0;
      let acc = '';
      for await (const chunk of provider.stream(opts.prompt, ctrl.signal)) {
        acc += chunk;
        count += 1;
        textRef.set(acc);
        tokenCountRef.set(count);
      }
    } catch (e) {
      if ((e as Error).name !== 'AbortError') {
        errorRef.set(e as Error);
      }
    } finally {
      isStreamingRef.set(false);
    }
  };

  const run = deps.useCallback(runImpl, []);

  const rerun = deps.useCallback(async () => {
    if (!lastPromptRef.get()) {
      errorRef.set(new Error('useStreamRun: 没有上次 prompt 可重跑'));
      return;
    }
    await run({ prompt: lastPromptRef.get() });
  }, [run]);

  const stop = deps.useCallback(() => {
    ctrlRef.current?.abort();
    isStreamingRef.set(false);
  }, []);

  const reset = deps.useCallback(() => {
    textRef.set('');
    errorRef.set(null);
    tokenCountRef.set(0);
    lastPromptRef.set('');
  }, []);

  return {
    get text() { return textRef.get(); },
    get isStreaming() { return isStreamingRef.get(); },
    get error() { return errorRef.get(); },
    get tokenCount() { return tokenCountRef.get(); },
    run,
    rerun,
    stop,
    reset,
  };
}