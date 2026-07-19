/**
 * 共享消息类型 — 供 background / popup / devtools 引用
 *
 * content script 不使用 Chrome MV3 的 ES 模块（受平台限制），
 * 所以这里不复用本文件内容；content script 在自身内联相同结构。
 * 本文件仅用于提供「消息工厂」，让其它端可以拷贝带类型的消息对象。
 */

/** 代理检测来源 */
export type AgentSource = 'meta' | 'global' | 'manual' | 'none';

/** 带来源信息的检测结果 */
export interface AgentDetectionResult {
    detected: boolean;
    source: AgentSource;
    agentId?: string;
    agentName?: string;
    endpoint?: string;
}

/** 一次运行摘要 */
export interface RunSummary {
    id: string;
    agentId: string;
    startedAt: number;
    endedAt?: number;
    status: 'completed' | 'running' | 'failed';
    totalTokens: number;
    traceCount: number;
}

/** 单个追踪事件 */
export interface TraceEvent {
    id: string;
    type: 'llm_call' | 'tool_call' | 'thinking' | 'error' | 'custom';
    timestamp: number;
    duration?: number;
    tokens?: number;
    label?: string;
}

/** 扩展持久化状态 */
export interface ExtensionState {
    studioUrl: string;
    connected: boolean;
    currentSession: string | null;
    recentRuns: RunSummary[];
}

/** 扩展各组件间传递的消息 —— discriminated union */
export type ExtensionMessage =
    | { type: 'AGENT_DETECTED'; payload: AgentDetectionResult }
    | { type: 'AGENT_NOT_DETECTED' }
    | { type: 'GET_AGENT_STATUS' }
    | { type: 'AGENT_STATUS_RESPONSE'; payload: AgentDetectionResult }
    | { type: 'SEND_MESSAGE'; payload: { text: string; sessionId?: string } }
    | { type: 'MESSAGE_SENT'; payload: { success: boolean; error?: string } }
    | { type: 'GET_RECENT_RUNS' }
    | { type: 'RECENT_RUNS_RESPONSE'; payload: RunSummary[] }
    | { type: 'TRACE_EVENT'; payload: TraceEvent }
    | { type: 'GET_TRACES'; payload: { sessionId: string } }
    | { type: 'TRACES_RESPONSE'; payload: { sessionId: string; events: TraceEvent[] } }
    | { type: 'REFRESH_STATUS' }
    | { type: 'OPEN_STUDIO'; payload: { path?: string } }
    | { type: 'CONNECTION_STATUS'; payload: { connected: boolean; studioUrl: string } };

/** 默认扩展持久化状态工厂 */
export function defaultExtensionState(): ExtensionState {
    return {
        studioUrl: 'http://localhost:8765',
        connected: false,
        currentSession: null,
        recentRuns: [],
    };
}

/**
 * 带类型的消息拷贝辅助：给定 type 与 payload，返回完整的 ExtensionMessage。
 *
 * 让 background / popup / devtools 构造强类型消息更简洁：
 *   send(copy('GET_RECENT_RUNS'));
 *   send(copy('SEND_MESSAGE', { text: 'hi' }));
 */
export function copy<T extends ExtensionMessage['type']>(
    type: T,
    ...args: Extract<ExtensionMessage, { type: T }> extends { payload: infer P }
        ? [payload: P]
        : [payload?: undefined]
): Extract<ExtensionMessage, { type: T }> {
    return { type, payload: args[0] } as Extract<ExtensionMessage, { type: T }>;
}
