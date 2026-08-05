/**
 * 零样板 React Hooks kit（v3.7-4）。
 *
 * 直接可用的 useAgent / useReActLoop：
 *   import { useAgent, useReActLoop } from '@agentprimordia/sdk/react';
 *
 *   function Chat() {
 *     const { messages, run } = useAgent({ name: 'chat', model: provider });
 *     return <button onClick={() => run('hello')}>发送</button>;
 *   }
 *
 * 无需手动注入 React 实例或构造 Agent——kit 自动绑定。
 * 需要安装 react@>=18（可选 peer 依赖）。
 */
import { useState, useCallback, useRef, useEffect } from 'react';
import { useAgentImpl, type UseAgentOptions, type UseAgentResult, type Agent } from './use-agent.js';
import { useReActLoopImpl, type ReActStreamer, type ReActStreamEvent, type UseReActLoopResult } from './use-react-loop.js';
import { ReActAgent } from '../agent/react-loop.js';

/** 默认 buildAgent：从 options 构造进程内 ReActAgent（零样板默认）。 */
function defaultBuildAgent(opts: UseAgentOptions): Agent {
  if (!opts.name) throw new Error('useAgent: options.name 必填');
  if (!opts.model) throw new Error('useAgent: options.model 必填（LLM Provider）');
  return new ReActAgent(opts as never);
}

/** 零样板 useAgent：直接使用进程内 ReActAgent，无需注入 React/Agent。 */
export function useAgent(options: UseAgentOptions = {}): UseAgentResult {
  return useAgentImpl(options, { useState, useCallback, useRef, useEffect }, defaultBuildAgent);
}

/** 基于 ReActAgent.streamEvents 的默认 ReActStreamer。 */
export class DefaultStreamer implements ReActStreamer {
  constructor(private readonly agent: ReActAgent) {}

  async *stream(prompt: string): AsyncIterable<ReActStreamEvent> {
    for await (const ev of this.agent.streamEvents(prompt)) {
      switch (ev.type) {
        case 'token':
          yield { type: 'thought', text: ev.content };
          break;
        case 'tool_call':
          yield {
            type: 'action',
            tool: ev.toolCall.name,
            args: ev.toolCall.arguments,
          };
          break;
        case 'tool_result':
          yield { type: 'observation', content: ev.result.content, isError: ev.result.isError ?? false };
          break;
        case 'turn_end':
          yield { type: 'turn', turn: ev.turn };
          break;
        case 'done':
          yield { type: 'done', text: ev.response.content };
          break;
        case 'error':
          yield { type: 'error', error: ev.error };
          break;
        default:
          break;
      }
    }
  }
}

/** 零样板 useReActLoop：自动构造 Agent 并展示 ReAct 思考-行动-观察过程。 */
export function useReActLoop(options: UseAgentOptions = {}): UseReActLoopResult {
  const streamerFactory = () => new DefaultStreamer(defaultBuildAgent(options) as ReActAgent);
  return useReActLoopImpl({ useState, useCallback, useRef, useEffect }, streamerFactory);
}

/** useAgent 的 HTTP 版（连接 AgentPrimordia REST 服务），零样板。 */
export { useAgent as useRemoteAgent } from './useAgent.js';
