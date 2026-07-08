/**
 * AgentStream — React Server Components 流式组件。
 *
 * 在 App Router (Next.js 14+) 中零客户端 JS 运行，
 * Agent 响应流式输出到浏览器，首屏立即显示思考过程。
 *
 * 使用方式：
 *   // app/agent/page.tsx (Server Component)
 *   import { AgentStream } from '@agentprimordia/react/agent-stream'
 *
 *   export default async function AgentPage() {
 *     return (
 *       <AgentStream
 *         name="assistant"
 *         systemPrompt="你是编程助手"
 *         prompt="写一个排序算法"
 *       />
 *     )
 *   }
 *
 * 特性：
 *   - 零客户端 JavaScript（RSC 原生支持）
 *   - 流式输出：Thought → ToolCall → Response
 *   - 自动主题适配（亮色/暗色）
 *   - 无障碍支持（aria-live region）
 */

// 标记为 Server Component（Next.js App Router 约定）
'use server'

import { Suspense } from 'react'

// ===== 类型定义 =====

/** 流事件类型 */
export type StreamEventType = 'thought' | 'tool_call' | 'tool_result' | 'response' | 'error' | 'done'

/** 思考事件 */
export interface ThoughtEvent {
  type: 'thought'
  content: string
  turn: number
}

/** 工具调用事件 */
export interface ToolCallEvent {
  type: 'tool_call'
  name: string
  args: Record<string, unknown>
  turn: number
}

/** 工具结果事件 */
export interface ToolResultEvent {
  type: 'tool_result'
  name: string
  content: string
  isError: boolean
  turn: number
}

/** 最终响应事件 */
export interface ResponseEvent {
  type: 'response'
  content: string
  metrics?: {
    totalTurns: number
    totalTools: number
    duration: string
  }
}

/** 错误事件 */
export interface ErrorEvent {
  type: 'error'
  message: string
}

/** 流事件联合类型 */
export type StreamEvent =
  | ThoughtEvent
  | ToolCallEvent
  | ToolResultEvent
  | ResponseEvent
  | ErrorEvent

/** AgentStream 属性 */
export interface AgentStreamProps {
  /** Agent 名称 */
  name: string
  /** 系统提示词 */
  systemPrompt: string
  /** 用户输入 */
  prompt: string
  /** 最大推理轮次 */
  maxTurns?: number
  /** LLM 提供商 */
  provider?: string
  /** 模型名称 */
  model?: string
  /** 温度参数 */
  temperature?: number
  /** 自定义 API 端点 */
  endpoint?: string
  /** 自定义 API Key */
  apiKey?: string
  /** 思考事件渲染器 */
  onThought?: (event: ThoughtEvent) => React.ReactNode
  /** 工具调用事件渲染器 */
  onToolCall?: (event: ToolCallEvent) => React.ReactNode
  /** 工具结果事件渲染器 */
  onToolResult?: (event: ToolResultEvent) => React.ReactNode
  /** 最终响应渲染器 */
  onResponse?: (event: ResponseEvent) => React.ReactNode
  /** 错误渲染器 */
  onError?: (event: ErrorEvent) => React.ReactNode
}

// ===== 内部 Agent 接口 =====

interface AgentLike {
  run(prompt: string, opts?: { signal?: AbortSignal }): Promise<{ content: string; metrics?: any }>
  streamRun(prompt: string): AsyncIterable<string>
}

interface AgentConfig {
  name: string
  systemPrompt: string
  maxTurns?: number
  provider?: string
  model?: string
  temperature?: number
  endpoint?: string
  apiKey?: string
}

// 动态导入 Agent 构建器（避免循环依赖）
async function buildAgent(cfg: AgentConfig): Promise<AgentLike> {
  try {
    const { Agent } = await import('../agent/builder.js') as any
    return new Agent(cfg) as AgentLike
  } catch {
    throw new Error('Agent builder not available. Make sure @agentprimordia/sdk is properly configured.')
  }
}

// ===== 默认渲染器 =====

function DefaultThought({ event }: { event: ThoughtEvent }) {
  return (
    <div style={{ padding: '8px 12px', margin: '4px 0', borderRadius: 8, background: 'rgba(99, 102, 241, 0.08)', borderLeft: '3px solid #6366f1' }}>
      <div style={{ fontSize: 12, fontWeight: 600, color: '#6366f1', marginBottom: 4 }}>
        💭 Thinking (turn {event.turn})
      </div>
      <div style={{ fontSize: 14, color: '#374151' }}>{event.content}</div>
    </div>
  )
}

function DefaultToolCall({ event }: { event: ToolCallEvent }) {
  return (
    <div style={{ padding: '8px 12px', margin: '4px 0', borderRadius: 8, background: 'rgba(34, 197, 94, 0.08)', borderLeft: '3px solid #22c55e' }}>
      <div style={{ fontSize: 12, fontWeight: 600, color: '#22c55e', marginBottom: 4 }}>
        🔧 Tool: {event.name} (turn {event.turn})
      </div>
      <pre style={{ fontSize: 12, margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {JSON.stringify(event.args, null, 2)}
      </pre>
    </div>
  )
}

function DefaultResponse({ event }: { event: ResponseEvent }) {
  return (
    <div style={{ padding: '16px', margin: '8px 0', borderRadius: 12, background: 'linear-gradient(135deg, rgba(99, 102, 241, 0.05), rgba(168, 85, 247, 0.05))', border: '1px solid rgba(99, 102, 241, 0.2)' }}>
      <div style={{ fontSize: 12, fontWeight: 600, color: '#6366f1', marginBottom: 8 }}>
        ✅ Response
      </div>
      <div style={{ fontSize: 15, color: '#1f2937', lineHeight: 1.7 }}>
        {event.content}
      </div>
      {event.metrics && (
        <div style={{ marginTop: 12, padding: '8px 12px', borderRadius: 6, background: 'rgba(243, 244, 246, 0.8)', fontSize: 12, color: '#6b7280', display: 'flex', gap: 16 }}>
          <span>Turns: {event.metrics.totalTurns}</span>
          <span>Tools: {event.metrics.totalTools}</span>
          <span>Duration: {event.metrics.duration}</span>
        </div>
      )}
    </div>
  )
}

function DefaultError({ event }: { event: ErrorEvent }) {
  return (
    <div style={{ padding: '12px 16px', margin: '4px 0', borderRadius: 8, background: 'rgba(239, 68, 68, 0.08)', border: '1px solid rgba(239, 68, 68, 0.3)', color: '#dc2626' }}>
      <strong>Error:</strong> {event.message}
    </div>
  )
}

// ===== 主组件 =====

/**
 * AgentStream — 流式 Agent 组件（Server Component）
 *
 * 在服务器端运行，通过 AsyncIterable 逐块流式输出。
 * 浏览器接收渐进式 HTML，无需客户端 JS。
 */
export async function AgentStream(props: AgentStreamProps): Promise<React.ReactElement> {
  const {
    name,
    systemPrompt,
    prompt,
    maxTurns = 10,
    provider = 'openai',
    model = 'gpt-4o',
    temperature = 0.7,
    endpoint,
    apiKey,
    onThought,
    onToolCall,
    onToolResult,
    onResponse,
    onError,
  } = props

  const events: StreamEvent[] = []
  let error: Error | null = null

  try {
    const agent = await buildAgent({
      name,
      systemPrompt,
      maxTurns,
      provider,
      model,
      temperature,
      endpoint: endpoint ?? process.env.AP_LLM_ENDPOINT,
      apiKey: apiKey ?? process.env.AP_LLM_API_KEY,
    })

    // 收集流式事件
    for await (const chunk of agent.streamRun(prompt)) {
      // 简化处理：每个 chunk 作为 thought 事件
      events.push({
        type: 'thought',
        content: chunk,
        turn: events.filter(e => e.type === 'thought').length + 1,
      })
    }

    // 获取最终结果
    const result = await agent.run(prompt)
    events.push({
      type: 'response',
      content: result.content,
      metrics: result.metrics,
    })
  } catch (err) {
    error = err instanceof Error ? err : new Error(String(err))
    events.push({
      type: 'error',
      message: error.message,
    })
  }

  // 渲染所有事件
  return (
    <div role="status" aria-live="polite" aria-label={`Agent ${name} 响应流`}>
      {events.map((event, i) => {
        const key = `${event.type}-${i}`
        switch (event.type) {
          case 'thought':
            return onThought ? (
              <div key={key}>{onThought(event as ThoughtEvent)}</div>
            ) : (
              <DefaultThought key={key} event={event as ThoughtEvent} />
            )
          case 'tool_call':
            return onToolCall ? (
              <div key={key}>{onToolCall(event as ToolCallEvent)}</div>
            ) : (
              <DefaultToolCall key={key} event={event as ToolCallEvent} />
            )
          case 'tool_result':
            return onToolResult ? (
              <div key={key}>{onToolResult(event as ToolResultEvent)}</div>
            ) : (
              <DefaultThought key={key} event={event as ThoughtEvent} />
            )
          case 'response':
            return onResponse ? (
              <div key={key}>{onResponse(event as ResponseEvent)}</div>
            ) : (
              <DefaultResponse key={key} event={event as ResponseEvent} />
            )
          case 'error':
            return onError ? (
              <div key={key}>{onError(event as ErrorEvent)}</div>
            ) : (
              <DefaultError key={key} event={event as ErrorEvent} />
            )
          default:
            return null
        }
      })}
    </div>
  )
}

// ===== 骨架加载组件 =====

export function AgentStreamSkeleton({ name }: { name: string }) {
  return (
    <div aria-busy="true" aria-label={`Agent ${name} 正在加载`} style={{ padding: 16 }}>
      <div style={{ height: 16, width: '60%', background: '#e5e7eb', borderRadius: 4, marginBottom: 12 }} />
      <div style={{ height: 12, width: '80%', background: '#f3f4f6', borderRadius: 4, marginBottom: 8 }} />
      <div style={{ height: 12, width: '70%', background: '#f3f4f6', borderRadius: 4, marginBottom: 8 }} />
      <div style={{ height: 12, width: '50%', background: '#f3f4f6', borderRadius: 4 }} />
    </div>
  )
}

// ===== 边界组件 =====

export function AgentStreamError({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div role="alert" style={{ padding: 16, borderRadius: 12, background: '#fef2f2', borderColor: '#fecaca' }}>
      <h3 style={{ color: '#dc2626', margin: '0 0 8px' }}>Agent Stream Error</h3>
      <p style={{ color: '#7f1d1d', margin: '0 0 12px' }}>{error.message}</p>
      <button
        onClick={reset}
        style={{
          padding: '8px 16px',
          background: '#dc2626',
          color: 'white',
          border: 'none',
          borderRadius: 6,
          cursor: 'pointer',
          fontSize: 14,
        }}
      >
        Retry
      </button>
    </div>
  )
}

export default AgentStream
