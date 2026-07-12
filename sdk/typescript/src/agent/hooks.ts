/**
 * Agent 钩子系统 — 从 react-loop.ts 拆分
 *
 * 包含：StreamEvent、HookPoint、HookContext、HookFunc、HookManager
 *
 * 与 Go 端 hooks 包对齐，提供 Agent 生命周期钩子注册与触发机制。
 * HookManager 使用 ObjectPool 复用 HookContext，减少热路径 GC 压力。
 */
import type { Message, ToolCall, Response, ToolResult } from '../types.js';
import { ObjectPool } from '../jsonutil/pool.js';

// ===== 流式事件类型 =====

/** 流式事件联合类型，表示 Agent 运行过程中产生的各类事件。
 *
 * 事件类型：
 * - token: LLM 输出的文本片段（流式 token）
 * - tool_call: LLM 请求调用工具
 * - tool_result: 工具执行完成
 * - turn_end: 一个 ReAct 循环轮次结束
 * - done: Agent 运行完成，包含最终响应
 * - error: 运行过程中发生错误
 */
export type StreamEvent =
  | { type: 'token'; content: string }
  | { type: 'tool_call'; toolCall: ToolCall; turn: number }
  | { type: 'tool_result'; result: ToolResult; turn: number }
  | { type: 'turn_end'; turn: number }
  | { type: 'done'; response: Response }
  | { type: 'error'; error: Error };

/** 钩子触发点类型，覆盖 Agent 生命周期的关键节点。
 *
 * 与 Go 端 HookPoint 对齐，支持以下阶段：
 * - 运行阶段：before_run / after_run / on_complete / on_error
 * - 轮次阶段：before_turn / after_turn
 * - LLM 阶段：before_llm / after_llm
 * - 工具阶段：before_tool / after_tool / before_tool_parse / after_tool_parse
 * - 编排阶段：before_pipeline_step / after_pipeline_step / before_handoff / after_handoff
 * - 并行阶段：before_parallel_agent / after_parallel_agent / before_dag_node / after_dag_node
 * - 流式阶段：on_stream / on_stream_start / on_stream_end
 * - 记忆阶段：before_memory_read / after_memory_read / before_memory_write / after_memory_write
 * - 上下文窗口：context_window_update / context_window_full
 * - 指标阶段：on_metrics_collect
 * - 生命周期：before_shutdown / after_shutdown / on_state_change
 * - RAG 阶段：before_rag / after_rag
 */
export type HookPoint =
  | 'before_run'
  | 'after_run'
  | 'before_turn'
  | 'after_turn'
  | 'before_llm'
  | 'after_llm'
  | 'before_tool'
  | 'after_tool'
  | 'on_error'
  | 'on_complete'
  // Extended hook points (matching Go framework)
  | 'before_rag'
  | 'after_rag'
  | 'before_pipeline_step'
  | 'after_pipeline_step'
  | 'before_handoff'
  | 'after_handoff'
  | 'before_parallel_agent'
  | 'after_parallel_agent'
  | 'before_dag_node'
  | 'after_dag_node'
  | 'on_stream'
  | 'on_stream_start'
  | 'on_stream_end'
  | 'before_memory_read'
  | 'after_memory_read'
  | 'before_memory_write'
  | 'after_memory_write'
  | 'context_window_update'
  | 'context_window_full'
  | 'before_tool_parse'
  | 'after_tool_parse'
  | 'on_metrics_collect'
  | 'before_shutdown'
  | 'after_shutdown'
  | 'on_state_change';

/** 钩子上下文，包含 Agent 当前运行状态快照，传递给各钩子函数。
 *
 * 与 Go 端 HookContext 对齐，字段含义：
 * - agentID: Agent 标识
 * - sessionID: 会话标识
 * - point: 触发钩子的事件点
 * - turn: 当前轮次编号
 * - message: 当前消息（可选）
 * - response: 当前响应（可选）
 * - toolCall: 当前工具调用（可选）
 * - toolResult: 工具执行结果（可选）
 * - error: 错误信息（可选）
 * - metadata: 附加元数据（可选）
 * - requestID: 请求 ID，用于可观测性关联
 * - streamChunk: 流式数据块
 * - duration: 当前阶段耗时（毫秒）
 * - oldState: 状态变更前状态
 * - newState: 状态变更后状态
 * - reason: 变更原因
 * - memoryQuery: 记忆查询语句
 * - memoryResult: 记忆查询结果
 * - contextWindowUsage: 当前上下文窗口使用量
 * - contextWindowLimit: 上下文窗口上限
 */
export interface HookContext {
  agentID: string;
  sessionID: string;
  point: HookPoint;
  turn: number;
  message?: Message;
  response?: Response;
  toolCall?: ToolCall;
  toolResult?: ToolResult;
  error?: Error;
  metadata?: Record<string, unknown>;
  // Extended fields (matching Go framework)
  requestID?: string;
  streamChunk?: StreamEvent;
  duration?: number;
  oldState?: string;
  newState?: string;
  reason?: string;
  memoryQuery?: string;
  memoryResult?: unknown;
  contextWindowUsage?: number;
  contextWindowLimit?: number;
}

/** 钩子函数类型，接收 HookContext 并返回 void 或 Promise<void> */
export type HookFunc = (ctx: HookContext) => Promise<void> | void;

/** 钩子管理器，负责注册、触发和移除钩子函数。
 *
 * 使用方式：
 *   const hooks = new HookManager();
 *   hooks.register('before_run', (ctx) => { console.log(ctx.turn); });
 *   await hooks.fireHook({ agentID, sessionID, point: 'before_run', turn: 0 });
 */
export class HookManager {
  private hooks: Map<HookPoint, HookFunc[]> = new Map();
  /** HookContext 对象池，复用热路径对象减少 GC 压力（与 Go 端 sync.Pool 对齐） */
  private ctxPool = new ObjectPool<HookContext>(
    () => ({ agentID: '', sessionID: '', point: 'before_run', turn: 0 }),
    (ctx) => {
      ctx.agentID = '';
      ctx.sessionID = '';
      ctx.point = 'before_run';
      ctx.turn = 0;
      ctx.message = undefined;
      ctx.response = undefined;
      ctx.toolCall = undefined;
      ctx.toolResult = undefined;
      ctx.error = undefined;
      ctx.metadata = undefined;
      ctx.requestID = undefined;
      ctx.streamChunk = undefined;
      ctx.duration = undefined;
      ctx.oldState = undefined;
      ctx.newState = undefined;
      ctx.reason = undefined;
      ctx.memoryQuery = undefined;
      ctx.memoryResult = undefined;
      ctx.contextWindowUsage = undefined;
      ctx.contextWindowLimit = undefined;
    },
    64,
  );

  /** 注册钩子函数到指定触发点 */
  register(point: HookPoint, fn: HookFunc): void {
    if (!this.hooks.has(point)) {
      this.hooks.set(point, []);
    }
    this.hooks.get(point)!.push(fn);
  }

  /** 触发指定钩子点的所有注册函数（使用对象池减少 GC）
   *
   * 与 Go 端 fireHookWithPool 对齐，在 ReAct 热路径中复用 HookContext 对象。
   */
  async fireHook(point: HookPoint, opts: Omit<Partial<HookContext>, 'point'>): Promise<void> {
    // P0-6: 事件订阅者检查 — 无订阅者时跳过 payload 构造，避免热路径上的无用开销
    if (!this.hasSubscriber(point)) return;
    const ctx = this.ctxPool.get();
    ctx.point = point;
    if (opts.agentID !== undefined) ctx.agentID = opts.agentID;
    if (opts.sessionID !== undefined) ctx.sessionID = opts.sessionID;
    if (opts.turn !== undefined) ctx.turn = opts.turn;
    if (opts.message !== undefined) ctx.message = opts.message;
    if (opts.response !== undefined) ctx.response = opts.response;
    if (opts.toolCall !== undefined) ctx.toolCall = opts.toolCall;
    if (opts.toolResult !== undefined) ctx.toolResult = opts.toolResult;
    if (opts.error !== undefined) ctx.error = opts.error;
    if (opts.metadata !== undefined) ctx.metadata = opts.metadata;
    if (opts.requestID !== undefined) ctx.requestID = opts.requestID;
    if (opts.streamChunk !== undefined) ctx.streamChunk = opts.streamChunk;
    if (opts.duration !== undefined) ctx.duration = opts.duration;
    if (opts.oldState !== undefined) ctx.oldState = opts.oldState;
    if (opts.newState !== undefined) ctx.newState = opts.newState;
    if (opts.reason !== undefined) ctx.reason = opts.reason;
    if (opts.memoryQuery !== undefined) ctx.memoryQuery = opts.memoryQuery;
    if (opts.memoryResult !== undefined) ctx.memoryResult = opts.memoryResult;
    if (opts.contextWindowUsage !== undefined) ctx.contextWindowUsage = opts.contextWindowUsage;
    if (opts.contextWindowLimit !== undefined) ctx.contextWindowLimit = opts.contextWindowLimit;
    try {
      await this.fire(ctx);
    } finally {
      this.ctxPool.put(ctx);
    }
  }

  /** 触发指定钩子点的所有注册函数 */
  async fire(ctx: HookContext): Promise<void> {
    const fns = this.hooks.get(ctx.point) ?? [];
    for (const fn of fns) {
      await fn(ctx);
    }
  }

  /** P0-6: 检查指定钩子点是否有订阅者（与 Go 端 hasEventSubscriber 对齐） */
  hasSubscriber(point: HookPoint): boolean {
    return (this.hooks.get(point)?.length ?? 0) > 0;
  }

  /** 移除指定钩子点的所有注册函数 */
  remove(point: HookPoint): void {
    this.hooks.delete(point);
  }

  /** 清空所有钩子注册 */
  clear(): void {
    this.hooks.clear();
  }

  /** 查询指定钩子点的注册函数数量 */
  count(point: HookPoint): number {
    return this.hooks.get(point)?.length ?? 0;
  }
}
