/**
 * useOrchestrationStream — SSE Hook，实时订阅编排执行事件。
 *
 * 使用方式：
 *   const { events, isConnected, error } = useOrchestrationStream(orchID)
 *
 * 特性：
 *   - 自动重连（指数退避）
 *   - 类型安全的事件回调
 *   - 与 React 18 use() / Suspense 兼容
 */

'use client'

import { useEffect, useRef, useState, useCallback } from 'react'

// ===== 类型定义 =====

/** 编排事件类型 */
export type OrchestrationEventType =
  | 'step_started'
  | 'step_completed'
  | 'step_failed'
  | 'step_skipped'
  | 'execution_completed'
  | 'execution_error'

/** 编排步骤状态 */
export type StepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped'

/** 编排事件 */
export interface OrchestrationEvent {
  type: OrchestrationEventType
  timestamp: string
  step_id?: string
  step_name?: string
  data?: Record<string, unknown>
  error?: string
}

/** Hook 返回值 */
export interface UseOrchestrationStreamResult {
  events: OrchestrationEvent[]
  isConnected: boolean
  error: Error | null
  clear: () => void
}

/**
 * useOrchestrationStream 订阅编排实时事件
 *
 * @param orchestratorId 编排 ID（null 表示不订阅）
 * @param options.maxReconnectAttempts 最大重连次数，默认 5
 * @param options.onEvent 事件回调（可选）
 */
export function useOrchestrationStream(
  orchestratorId: string | null,
  options: {
    maxReconnectAttempts?: number
    onEvent?: (event: OrchestrationEvent) => void
  } = {}
): UseOrchestrationStreamResult {
  const { maxReconnectAttempts = 5, onEvent } = options
  const [events, setEvents] = useState<OrchestrationEvent[]>([])
  const [isConnected, setIsConnected] = useState(false)
  const [error, setError] = useState<Error | null>(null)
  const retryCount = useRef(0)
  const abortRef = useRef<AbortController | null>(null)
  const onEventRef = useRef(onEvent)

  // 保持 onEvent ref 最新
  useEffect(() => {
    onEventRef.current = onEvent
  }, [onEvent])

  const clear = useCallback(() => {
    setEvents([])
    setError(null)
  }, [])

  useEffect(() => {
    if (!orchestratorId) return

    const abortController = new AbortController()
    abortRef.current = abortController

    const connect = () => {
      if (abortController.signal.aborted) return

      const url = `/events?orchestrator_id=${encodeURIComponent(orchestratorId)}`

      fetch(url, {
        headers: { Accept: 'text/event-stream' },
        signal: abortController.signal,
      })
        .then((response) => {
          if (!response.ok) {
            throw new Error(`SSE request failed: ${response.status}`)
          }
          setIsConnected(true)
          setError(null)
          retryCount.current = 0

          const reader = response.body?.getReader()
          if (!reader) {
            throw new Error('No response body')
          }

          const decoder = new TextDecoder()
          let buffer = ''

          const readChunk = () => {
            reader.read().then(({ done, value }) => {
              if (done || abortController.signal.aborted) return

              buffer += decoder.decode(value, { stream: true })
              const lines = buffer.split('\n\n')
              buffer = lines.pop() ?? ''

              for (const line of lines) {
                if (line.startsWith('data: ')) {
                  try {
                    const event = JSON.parse(line.slice(6)) as OrchestrationEvent
                    setEvents((prev) => [...prev, event])
                    onEventRef.current?.(event)
                  } catch (e) {
                    console.error('Failed to parse SSE event:', e)
                  }
                }
              }
              readChunk()
            }).catch((err) => {
              if (abortController.signal.aborted) return
              setIsConnected(false)
              if (err.name !== 'AbortError') {
                setError(err)
                scheduleReconnect()
              }
            })
          }
          readChunk()
        })
        .catch((err) => {
          if (err.name === 'AbortError' || abortController.signal.aborted) return
          setIsConnected(false)
          setError(err)
          scheduleReconnect()
        })
    }

    const scheduleReconnect = () => {
      if (retryCount.current >= maxReconnectAttempts) {
        setError(new Error('Max reconnection attempts reached'))
        return
      }
      retryCount.current++
      const delay = Math.min(1000 * Math.pow(2, retryCount.current - 1), 30000)
      setTimeout(connect, delay)
    }

    connect()

    return () => {
      abortController.abort()
      setIsConnected(false)
    }
  }, [orchestratorId, maxReconnectAttempts])

  return { events, isConnected, error, clear }
}

/**
 * useOrchestratorGraph — 从事件流构建 React Flow 图的 hook。
 *
 * 维护实时的节点状态和边列表，用于驱动 React Flow 渲染。
 */
export interface GraphNode {
  id: string
  type: string
  data: Record<string, unknown>
  position: { x: number; y: number }
}

export interface GraphEdge {
  id: string
  source: string
  target: string
}

export interface GraphData {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export function useOrchestratorGraph(
  orchestratorId: string | null,
  initialNodes: GraphNode[],
  initialEdges: GraphEdge[]
): GraphData {
  const { events } = useOrchestrationStream(orchestratorId)
  const [graph, setGraph] = useState<GraphData>({
    nodes: initialNodes,
    edges: initialEdges,
  })

  useEffect(() => {
    if (events.length === 0) return
    const latest = events[events.length - 1]
    if (!latest?.step_id) return

    setGraph((prev) => {
      const nodes = prev.nodes.map((node) => {
        if (node.id === latest.step_id) {
          const data = { ...node.data }
          if (latest.type === 'step_started') data.status = 'running'
          if (latest.type === 'step_completed') data.status = 'completed'
          if (latest.type === 'step_failed') data.status = 'failed'
          if (latest.error) data.error = latest.error
          return { ...node, data }
        }
        return node
      })
      return { ...prev, nodes }
    })
  }, [events])

  return graph
}
