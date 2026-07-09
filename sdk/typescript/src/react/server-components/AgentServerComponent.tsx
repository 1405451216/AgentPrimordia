/**
 * AgentServerComponent — React 19 Server Component (RSC) 兼容的 Agent 渲染组件。
 *
 * 该组件在服务器渲染阶段直接运行 Agent（await agent.run(input)），将最终响应与
 * 运行指标作为 RSC payload 流式下发，充分发挥 React 19 并发/流式渲染能力。
 *
 * 特点：
 * - 异步组件（async function component），天然适配 Suspense 与流式 SSR
 * - 不依赖任何客户端 hooks，可在纯 Server 环境下渲染
 * - 通过 createAgent Builder DSL 构造 Agent，类型安全且无第三方框架依赖
 *
 * 使用方式：
 *   // app/page.tsx (Server Component)
 *   import { AgentServerComponent } from '@agentprimordia/sdk/react/server-components';
 *   export default function Page() {
 *     return <AgentServerComponent input="你好" provider={provider} toolkit={toolkit} />;
 *   }
 */

import { createAgent } from '../../agent/builder.js';
import type { Provider } from '../../llm/provider.js';
import type { ToolRegistry } from '../../tools/registry.js';
import type { AgentMetrics, Response } from '../../types.js';
import type { ReactElement } from 'react';

/** AgentServerComponent 的 props */
export interface AgentServerComponentProps {
  /** 用户输入 prompt */
  input: string;
  /** LLM Provider（服务端注入） */
  provider: Provider;
  /** 工具注册表（服务端注入） */
  toolkit: ToolRegistry;
  /** 系统提示词（可选） */
  systemPrompt?: string;
  /** 最大轮次（可选，默认 10） */
  maxTurns?: number;
}

/**
 * 渲染 Agent 运行结果的服务端组件。
 *
 * 注意：作为 RSC，本组件在服务器执行，await agent.run() 期间会触发 Suspense 边界，
 * 因此调用方应在外层包裹 <Suspense fallback={...}>。
 */
export async function AgentServerComponent({
  input,
  provider,
  toolkit,
  systemPrompt,
  maxTurns,
}: AgentServerComponentProps): Promise<ReactElement> {
  const agent = createAgent('server-agent')
    .withProvider(provider)
    .withToolkit(toolkit)
    .withSystemPrompt(systemPrompt ?? '')
    .withMaxTurns(maxTurns ?? 10)
    .build();

  const response: Response = await agent.run(input);

  return (
    <div className="ap-agent-server-component">
      <p className="ap-agent-response">{response.content}</p>
      <AgentMetrics metrics={response.metrics} />
    </div>
  );
}

/** Agent 运行指标展示（纯展示，无副作用） */
export function AgentMetrics({ metrics }: { metrics: AgentMetrics }): ReactElement {
  return (
    <dl className="ap-agent-metrics">
      <dt>Turns</dt>
      <dd>{metrics.totalTurns}</dd>
      <dt>Tools</dt>
      <dd>{metrics.totalTools}</dd>
      <dt>Duration</dt>
      <dd>{metrics.duration} ms</dd>
      <dt>LLM latency</dt>
      <dd>{metrics.llmLatency} ms</dd>
      <dt>Tool latency</dt>
      <dd>{metrics.toolLatency} ms</dd>
    </dl>
  );
}
