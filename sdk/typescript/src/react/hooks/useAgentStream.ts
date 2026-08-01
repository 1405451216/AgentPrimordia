/**
 * useAgentStream — 订阅 ReActAgent 的流式事件（token / tool_call / tool_result / done / error）。
 *
 * 设计要点（与 react/use-agent.ts 保持一致）：
 * - 不直接 import react，由调用方通过 deps 注入 hooks（React 作为 peerDependency）
 * - 通过 agent.streamEvents() 异步迭代消费事件，支持 AbortController 取消
 * - 返回 content（累计文本）、isStreaming、events（全量事件）、error 以及 start/stop 控制
 *
 * 使用方式（通过 react/index.ts 的 useAgentStreamWithReact 工厂绑定 React）：
 *   const useAgentStream = useAgentStreamWithReact(React, agent, { prompt: '你好' });
 *   const { content, isStreaming, events } = useAgentStream();
 */

import type { ReActAgent, StreamEvent } from '../../agent/react-loop.js';
import { makeState, type UseAgentDeps } from '../use-agent.js';

/** useAgentStream 选项 */
export interface UseAgentStreamOptions {
  /** 初始 prompt；提供则组件 mount 后自动开始流式订阅 */
  prompt?: string;
}

/** useAgentStream 返回值 */
export interface UseAgentStreamResult {
  /** 累计拼接的流式文本内容 */
  content: string;
  /** 是否正在流式输出 */
  isStreaming: boolean;
  /** 收集到的全部结构化事件（含 tool_call / tool_result 等） */
  events: StreamEvent[];
  /** 最近一次错误（如有） */
  error: Error | null;
  /** 主动开始流式订阅某个 prompt */
  start: (prompt: string) => void;
  /** 停止当前订阅 */
  stop: () => void;
}

/** useAgentStream 的纯逻辑实现，不直接依赖 React。 */
export function useAgentStreamImpl(
  deps: UseAgentDeps,
  agent: ReActAgent,
  options?: UseAgentStreamOptions,
): UseAgentStreamResult {
  const contentRef = makeState<string>(deps, '');
  const isStreamingRef = makeState<boolean>(deps, false);
  const eventsRef = makeState<StreamEvent[]>(deps, []);
  const errorRef = makeState<Error | null>(deps, null);
  const abortRef = deps.useRef<AbortController | null>(null);
  const mountedRef = deps.useRef(true);

  deps.useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const stop = deps.useCallback(() => {
    abortRef.current?.abort();
    isStreamingRef.set(false);
  }, []);

  const start = deps.useCallback(
    (prompt: string) => {
      contentRef.set('');
      eventsRef.set([]);
      errorRef.set(null);
      isStreamingRef.set(true);

      const ctrl = new AbortController();
      abortRef.current = ctrl;

      (async () => {
        try {
          const iterable = agent.streamEvents(prompt, { signal: ctrl.signal });
          for await (const event of iterable) {
            if (ctrl.signal.aborted) break;
            if (mountedRef.current) {
              eventsRef.set([...eventsRef.get(), event]);
            }
            if (event.type === 'token' && event.content) {
              if (mountedRef.current) {
                contentRef.set(contentRef.get() + event.content);
              }
            } else if (event.type === 'error') {
              if (mountedRef.current) errorRef.set(event.error);
            }
          }
        } catch (e) {
          if (mountedRef.current) errorRef.set(e as Error);
        } finally {
          if (mountedRef.current) isStreamingRef.set(false);
        }
      })();
    },
    [agent as never],
  );

  // 自动订阅初始 prompt
  deps.useEffect(() => {
    if (options?.prompt) start(options.prompt);
    return () => abortRef.current?.abort();
  }, [options?.prompt]);

  return {
    get content() {
      return contentRef.get();
    },
    get isStreaming() {
      return isStreamingRef.get();
    },
    get events() {
      return eventsRef.get();
    },
    get error() {
      return errorRef.get();
    },
    start,
    stop,
  };
}
