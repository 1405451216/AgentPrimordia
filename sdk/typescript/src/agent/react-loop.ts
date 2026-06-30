import type { Message, ToolCall, Response, AgentStatus, ToolResult } from '../types.js';
import type { Provider } from '../llm/provider.js';
import type { ToolRegistry } from '../tools/registry.js';
import type { CostTracker, Checkpoint, CheckpointStore } from './request-id.js';
import type { Memory } from '../memory/store.js';
import type { OTelBridge } from '../metrics/otel-extended.js';
import { validateAgentInput } from '../validate.js';
import { ObjectPool } from '../jsonutil/pool.js';
// P6: Phase 4-5 能力集成
import type { AgentSelfTuner, RunMetrics, TuningSuggestion } from './self-tuning.js';
import type { SpeculativeExecutor } from './speculative-exec.js';
import type { EnhancedToolLearner } from './tool-learning.js';

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
 *   await hooks.fire({ agentID, sessionID, point: 'before_run', turn: 0 });
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

/** 生命周期管理器，控制 Agent 的启动、停止、暂停和恢复。
 *
 * 与 Go 端 Lifecycle 对齐，提供：
 * - 状态管理（idle / running / paused / completed / error）
 * - 停止信号（stop / isStopped / onStop）
 * - 暂停/恢复（pause / resume / waitPause / waitResume）
 */
export class Lifecycle {
  private _status: AgentStatus = 'idle';
  private stopped = false;
  private stopResolvers: (() => void)[] = [];
  private pauseResolvers: (() => void)[] = [];
  private resumeResolvers: (() => void)[] = [];
  private paused = false;

  /** 获取当前状态 */
  get status(): AgentStatus {
    return this._status;
  }

  /** 设置状态 */
  setStatus(s: AgentStatus): void {
    this._status = s;
  }

  /** 发送停止信号，唤醒所有等待停止的 Promise */
  stop(): void {
    this.stopped = true;
    for (const r of this.stopResolvers) r();
    this.stopResolvers = [];
  }

  /** 检查是否已收到停止信号 */
  isStopped(): boolean {
    return this.stopped;
  }

  /** 等待停止信号，返回 Promise 在 stop() 调用时 resolve */
  onStop(): Promise<void> {
    if (this.stopped) return Promise.resolve();
    return new Promise((r) => this.stopResolvers.push(r));
  }

  /** 暂停 Agent，状态从 running 变为 paused */
  pause(): void {
    if (this._status !== 'running') return;
    this._status = 'paused';
    this.paused = true;
    for (const r of this.pauseResolvers) r();
    this.pauseResolvers = [];
  }

  /** 恢复 Agent，状态从 paused 变为 running */
  resume(): void {
    if (this._status !== 'paused') return;
    this._status = 'running';
    this.paused = false;
    for (const r of this.resumeResolvers) r();
    this.resumeResolvers = [];
  }

  /** 等待暂停完成，返回 Promise 在 pause() 调用时 resolve */
  waitPause(): Promise<void> {
    if (this.paused) return Promise.resolve();
    return new Promise((r) => this.pauseResolvers.push(r));
  }

  /** 等待恢复完成，返回 Promise 在 resume() 调用时 resolve */
  waitResume(): Promise<void> {
    if (!this.paused) return Promise.resolve();
    return new Promise((r) => this.resumeResolvers.push(r));
  }
}

// ===== P0-4: 能力缓存 (capCache) =====
// 与 Go 端 resolveCapabilities 对齐，在 Run 入口一次性查找所有能力引用，
// 避免每轮 turn 重复做能力查找（类型断言 / 属性访问）。

/** Agent 运行时能力缓存，在 Run 入口一次性解析后供整个循环使用 */
interface CapabilitiesCache {
  costTracker: CostTracker | null;
  memoryStore: Memory | null;
  checkpointStore: CheckpointStore | null;
  otelBridge: OTelBridge | null;
}

/** Run/StreamEvents 的可选参数 */
export interface RunOptions {
  /** P0-1: AbortSignal 用于取消传播（与 Go 端 context.Context 对齐） */
  signal?: AbortSignal;
}

/** ReActAgent 配置，与 Go 端 ReActConfig 对齐。
 *
 * 字段说明：
 * - name: Agent 名称（必填）
 * - model: LLM Provider（必填）
 * - toolkit: 工具注册表（必填）
 * - maxTurns: 最大轮次，默认 10
 * - maxConsecutiveFailures: 连续工具失败上限，默认 3
 * - systemPrompt: 系统提示词
 * - hooks: 钩子管理器
 * - lifecycle: 生命周期管理器
 * - sessionId: 会话标识
 * - maxMessages: 最大消息缓存数，默认 80
 * - costTracker: 成本追踪器（可选，超预算自动停止）
 * - memoryStore: 记忆存储（可选，支持异步写入 flush）
 * - checkpointStore: Checkpoint 存储（可选，支持断点恢复）
 */
export interface ReActConfig {
  name: string;
  model: Provider;
  toolkit: ToolRegistry;
  maxTurns?: number;
  maxConsecutiveFailures?: number;
  systemPrompt?: string;
  hooks?: HookManager;
  lifecycle?: Lifecycle;
  sessionId?: string;
  maxMessages?: number;
  // P0-5: 成本追踪
  costTracker?: CostTracker;
  // P0-7: 异步记忆写入
  memoryStore?: Memory;
  // P0-3: Checkpoint 存储
  checkpointStore?: CheckpointStore;
  // P3-1: OTel 可观测性桥接（可选，注入后自动创建 turn/LLM/tool 级别 Span）
  otelBridge?: OTelBridge;
  // P4-A1: 并行工具执行（默认 false，开启后同一轮的多个工具调用并行执行）
  parallelToolExecution?: boolean;
  // P4-A1: 并行工具执行的最大并发数（默认无限制）
  maxParallelTools?: number;
  // P6-A1: 投机执行器（注入后在工具执行期间预热下一轮 LLM 调用）
  speculativeExecutor?: SpeculativeExecutor;
  // P6-A2: 自调优器（注入后自动收集 RunMetrics 并在下次运行前应用调优建议）
  selfTuner?: AgentSelfTuner;
  // P6-A3: 增强工具学习器（注入后自动将 few-shot 示例注入 system prompt）
  enhancedToolLearner?: EnhancedToolLearner;
  // P6-A2: 是否自动应用调优建议（默认 true，仅当 selfTuner 已注入时生效）
  autoTune?: boolean;
}

/** ReActAgent 是基于 ReAct（推理+行动）循环的 Agent 实现。
 *
 * 与 Go 端 ReActAgent 对齐，核心流程：
 * 1. 接收用户输入
 * 2. 调用 LLM 获取推理结果（可能包含工具调用）
 * 3. 执行工具调用，将结果反馈给 LLM
 * 4. 重复直到 LLM 返回最终答案或达到最大轮次
 *
 * 使用方式：
 *   const agent = new ReActAgent({
 *     name: 'my-agent',
 *     model: provider,
 *     toolkit: registry,
 *     maxTurns: 10,
 *   });
 *   const response = await agent.run('你好');
 *   // 或流式：for await (const event of agent.streamEvents('你好')) { ... }
 *   // 支持取消：const ctrl = new AbortController(); agent.run('你好', { signal: ctrl.signal });
 *   // 支持断点恢复：await agent.resumeFromCheckpoint(checkpoint);
 */
export class ReActAgent {
  readonly name: string;
  private model: Provider;
  private toolkit: ToolRegistry;
  private maxTurns: number;
  private maxConsecutiveFailures: number;
  private maxMessages: number;
  private consecutiveFailures = 0;
  private systemPrompt: string;
  private sessionId: string;
  private hooks: HookManager;
  private lifecycle: Lifecycle;
  private messages: Message[] = [];
  // P0-5: 成本追踪器
  private costTracker?: CostTracker;
  // P0-7: 异步记忆存储
  private memoryStore?: Memory;
  // P0-3: Checkpoint 存储
  private checkpointStore?: CheckpointStore;
  // P0-4: 能力缓存（Run 入口一次性解析，避免每轮重复查找）
  private capCache: CapabilitiesCache | null = null;
  // P0-7: 异步记忆写入队列
  private pendingMemoryWrites: Promise<void>[] = [];
  // P3-1: OTel 可观测性桥接
  private otelBridge?: OTelBridge;
  // P3-4: runMu 互斥锁 — 防止并发 run() 导致状态竞争（与 Go 端 runMu 对齐）
  private runMu: Promise<void> = Promise.resolve();
  // P4-A1: 并行工具执行配置
  private parallelToolExecution: boolean;
  private maxParallelTools: number;
  // P4-A5: Graceful shutdown 标志 — 完成当前轮次后再停止
  private gracefulShutdownFlag = false;
  // P6-A1: 投机执行器
  private speculativeExecutor?: SpeculativeExecutor;
  // P6-A2: 自调优器
  private selfTuner?: AgentSelfTuner;
  // P6-A3: 增强工具学习器
  private enhancedToolLearner?: EnhancedToolLearner;
  // P6-A2: 是否自动应用调优建议
  private autoTune: boolean;
  // P6-A2: 最近一次运行指标（用于自调优）
  private lastRunMetrics?: RunMetrics;

  constructor(config: ReActConfig) {
    if (!config.name?.trim()) throw new Error('Agent name is required');
    if (!config.model) throw new Error('Model provider is required');
    if (!config.toolkit) throw new Error('Toolkit is required');
    if (config.maxTurns !== undefined && (config.maxTurns < 1 || config.maxTurns > 100)) {
      throw new Error('maxTurns must be between 1 and 100');
    }
    this.name = config.name;
    this.model = config.model;
    this.toolkit = config.toolkit;
    this.maxTurns = config.maxTurns ?? 10;
    this.maxConsecutiveFailures = config.maxConsecutiveFailures ?? 3;
    this.maxMessages = config.maxMessages ?? 80;
    this.systemPrompt = config.systemPrompt ?? '';
    this.sessionId = config.sessionId ?? '';
    this.hooks = config.hooks ?? new HookManager();
    this.lifecycle = config.lifecycle ?? new Lifecycle();
    this.costTracker = config.costTracker;
    this.memoryStore = config.memoryStore;
    this.checkpointStore = config.checkpointStore;
    this.otelBridge = config.otelBridge;
    this.parallelToolExecution = config.parallelToolExecution ?? false;
    this.maxParallelTools = config.maxParallelTools ?? 0; // 0 = 无限制
    // P6: Phase 4-5 能力注入
    this.speculativeExecutor = config.speculativeExecutor;
    this.selfTuner = config.selfTuner;
    this.enhancedToolLearner = config.enhancedToolLearner;
    this.autoTune = config.autoTune ?? true;
  }

  // ===== P4-A5: Graceful Shutdown =====

  /** 请求优雅关闭：完成当前轮次后停止 Agent，而非立即中断。
   *
   * 与 Go 端 IsGracefulShutdown() 对齐。
   * 与 stop() 的区别：stop() 立即中断，gracefulShutdown() 等当前 turn 完成。
   */
  requestGracefulShutdown(): void {
    this.gracefulShutdownFlag = true;
  }

  /** 检查是否已请求优雅关闭 */
  isGracefulShutdownRequested(): boolean {
    return this.gracefulShutdownFlag;
  }

  // ===== P0-4: 能力缓存 =====
  // 与 Go 端 resolveCapabilities 对齐，在 Run 入口一次性查找所有能力引用
  private resolveCapabilities(): CapabilitiesCache {
    return {
      costTracker: this.costTracker ?? null,
      memoryStore: this.memoryStore ?? null,
      checkpointStore: this.checkpointStore ?? null,
      otelBridge: this.otelBridge ?? null,
    };
  }

  // ===== P0-7: 异步记忆写入 flush =====
  // 与 Go 端 flushMemoryWriter 对齐，确保所有 saveMemory 调用完成
  private async flushMemoryWriter(): Promise<void> {
    if (this.pendingMemoryWrites.length === 0) return;
    await Promise.allSettled(this.pendingMemoryWrites);
    this.pendingMemoryWrites = [];
  }

  // ===== P0-1: 取消信号检查 =====
  private checkSignal(signal?: AbortSignal): void {
    if (signal?.aborted) {
      throw new DOMException('Agent run aborted', 'AbortError');
    }
  }

  // ===== P0-6: 条件事件发布（仅在有订阅者时构造 payload） =====
  private async publishEvent(eventType: string, payload: Record<string, unknown>): Promise<void> {
    // 简单实现：通过 hooks 的 hasSubscriber 检查
    if (this.hooks.hasSubscriber('on_metrics_collect')) {
      await this.hooks.fireHook('on_metrics_collect', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn: 0,
        metadata: { eventType, ...payload },
      });
    }
  }

  /** 执行 ReAct 循环，返回最终响应。
   *
   * 参数：
   * - input: 用户输入文本
   * - options: 可选参数，支持 AbortSignal 取消
   *
   * 返回：包含最终内容和指标的 Response 对象
   */
  async run(input: string, options?: RunOptions): Promise<Response> {
    validateAgentInput(input);
    // P3-4: runMu 互斥锁 — 防止并发 run() 导致状态竞争（与 Go 端 runMu 对齐）
    const runRelease = await this.acquireRunLock();
    try {
      return await this.runEngine(input, options);
    } finally {
      runRelease();
    }
  }

  // P3-4: Promise-based 互斥锁实现
  private async acquireRunLock(): Promise<() => void> {
    const oldMu = this.runMu;
    let release!: () => void;
    this.runMu = new Promise<void>((resolve) => { release = resolve; });
    await oldMu;
    return release;
  }

  // ===== P4-A1: 并行工具执行 =====
  // 同一轮的多个工具调用并行执行，适用于 I/O 密集型工具场景。
  // 使用 Promise.all 确保所有工具完成后才进入下一轮。
  // 注意：消息按原始 toolCalls 顺序追加，保证 LLM 看到的上下文一致性。
  private async executeToolsParallel(
    toolCalls: ToolCall[],
    turn: number,
    startTime: number,
    totalLLMLatency: number,
    _totalToolLatency: number,
    _toolCount: number,
  ): Promise<{ totalToolLatency: number; toolCount: number; shouldStop: boolean }> {
    let totalToolLatency = _totalToolLatency;
    let toolCount = _toolCount;
    let shouldStop = false;

    // 并行执行所有工具调用
    const executeOne = async (tc: ToolCall): Promise<ToolResult> => {
      await this.hooks.fireHook('before_tool', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn,
        toolCall: tc,
      });

      const toolSpanId = this.capCache?.otelBridge?.startSpan(`agent.tool_call`, {
        'agent.name': this.name,
        'tool.name': tc.name,
        'agent.turn': turn,
      });
      const toolStart = Date.now();
      const result = await this.toolkit.execute(tc);
      const latency = Date.now() - toolStart;

      this.capCache?.otelBridge?.addAttribute(toolSpanId!, 'tool.latency_ms', latency);
      this.capCache?.otelBridge?.addAttribute(toolSpanId!, 'tool.is_error', !!result.isError);
      this.capCache?.otelBridge?.endSpan(toolSpanId!, result.isError ? 'error' : 'ok');

      return result;
    };

    // 如果配置了最大并发数，使用分批执行
    let results: ToolResult[];
    if (this.maxParallelTools > 0 && toolCalls.length > this.maxParallelTools) {
      results = [];
      for (let i = 0; i < toolCalls.length; i += this.maxParallelTools) {
        const batch = toolCalls.slice(i, i + this.maxParallelTools);
        const batchResults = await Promise.all(batch.map(executeOne));
        results.push(...batchResults);
      }
    } else {
      results = await Promise.all(toolCalls.map(executeOne));
    }

    // 串行处理结果（保持消息顺序、更新统计、触发 after_tool hook）
    const toolBatchStart = Date.now();
    for (let i = 0; i < toolCalls.length; i++) {
      const tc = toolCalls[i];
      const result = results[i];
      totalToolLatency += Date.now() - toolBatchStart; // 近似：并行总时间
      toolCount++;

      if (result.isError) {
        this.consecutiveFailures++;
        if (this.consecutiveFailures >= this.maxConsecutiveFailures) {
          shouldStop = true;
        }
      } else {
        this.consecutiveFailures = 0;
      }

      this.messages.push({
        role: 'tool',
        content: result.content,
        toolCallId: tc.id,
        name: tc.name,
      });

      // P0-7: 异步记忆写入
      if (this.capCache?.memoryStore) {
        const mem = this.capCache.memoryStore;
        this.pendingMemoryWrites.push(
          mem.add({
            id: `${this.name}-${this.sessionId}-${turn}-${tc.id}`,
            sessionId: this.sessionId,
            role: 'tool',
            content: result.content,
            metadata: { toolCallId: tc.id, toolName: tc.name },
            createdAt: new Date().toISOString(),
          }).catch(() => {}),
        );
      }

      await this.hooks.fireHook('after_tool', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn,
        toolResult: result,
      });
    }

    // 并行模式下，工具延迟 = 并行执行总时间（而非各工具延迟之和）
    totalToolLatency = _totalToolLatency + (Date.now() - toolBatchStart);

    void startTime; void totalLLMLatency;
    return { totalToolLatency, toolCount, shouldStop };
  }

  // ===== P0-2: Panic Recovery + 引擎入口 =====
  // 与 Go 端 reactLoopEngine 对齐，统一处理异常恢复、能力缓存、记忆 flush
  private async runEngine(input: string, options?: RunOptions): Promise<Response> {
    this.lifecycle.setStatus('running');
    const startTime = Date.now();
    const totalLLMLatency = 0;
    const totalToolLatency = 0;
    const toolCount = 0;
    this.consecutiveFailures = 0;

    // P0-4: Run 入口一次性查找所有能力引用
    this.capCache = this.resolveCapabilities();

    // P3-1: 创建 run-level Span
    const runSpanId = this.capCache.otelBridge?.startSpan(`agent.run`, {
      'agent.name': this.name,
      'agent.session_id': this.sessionId,
    });

    this.messages = [];
    if (this.systemPrompt) {
      this.messages.push({ role: 'system', content: this.systemPrompt });
    }

    // P6-A3: Tool Learning 闭环 — 自动将 few-shot 示例注入 system prompt
    if (this.enhancedToolLearner && this.toolkit.size() > 0) {
      try {
        const toolNames = this.toolkit.list().map((t) => t.name);
        const guide = await this.enhancedToolLearner.generateUsageGuide(toolNames);
        if (guide && this.messages.length > 0 && this.messages[0]!.role === 'system') {
          this.messages[0]!.content += '\n\n' + guide;
        }
      } catch {
        // few-shot 注入失败不影响主流程
      }
    }

    // P6-A2: 自调优 — 在运行前应用上次的调优建议
    if (this.selfTuner && this.autoTune && this.lastRunMetrics) {
      const suggestion = this.selfTuner.getSuggestion();
      if (suggestion.shouldAdjust) {
        this.applyTuningSuggestion(suggestion);
      }
    }

    this.messages.push({ role: 'user', content: input });

    await this.hooks.fireHook('before_run', {
      agentID: this.name,
      sessionID: this.sessionId,
      turn: 0,
    });

    // P0-5: 成本预算检查（Run 入口）
    if (this.capCache.costTracker?.checkBudget()) {
      const response: Response = {
        content: 'Agent stopped: cost budget exceeded',
        metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 },
      };
      this.lifecycle.setStatus('completed');
      this.capCache.otelBridge?.endSpan(runSpanId!, 'ok');
      return response;
    }

    try {
      const response = await this.runLoop(
        startTime, totalLLMLatency, totalToolLatency, toolCount, 0, options,
      );

      await this.hooks.fireHook('after_run', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn: 0,
        response,
      });

      // P6-A2: 自调优 — 记录本次运行指标
      if (this.selfTuner) {
        this.lastRunMetrics = {
          totalTurns: response.metrics.totalTurns,
          totalTools: response.metrics.totalTools,
          duration: response.metrics.duration,
          llmLatency: response.metrics.llmLatency,
          toolLatency: response.metrics.toolLatency,
          success: true,
        };
        this.selfTuner.recordRun(this.lastRunMetrics);
      }

      // P3-1: 结束 run-level Span
      this.capCache.otelBridge?.endSpan(runSpanId!, 'ok');

      return response;
    } catch (err) {
      // P0-2: Panic Recovery — 异常时状态回退 + fire on_error hook
      const error = err instanceof Error ? err : new Error(String(err));
      this.lifecycle.setStatus('error');

      // P3-1: 记录错误事件并结束 Span
      if (runSpanId) {
        this.capCache.otelBridge?.addEvent(runSpanId, 'error', { message: error.message });
        this.capCache.otelBridge?.endSpan(runSpanId, 'error');
      }

      await this.hooks.fireHook('on_error', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn: 0,
        error,
      });

      // 返回错误响应而非抛出，与 Go 端 panic recover 行为对齐
      if (error.name === 'AbortError') {
        return {
          content: 'Agent run cancelled',
          metrics: {
            totalTurns: 0, totalTools: 0,
            duration: Date.now() - startTime,
            llmLatency: 0, toolLatency: 0,
          },
        };
      }
      return {
        content: `Agent error: ${error.message}`,
        metrics: {
          totalTurns: 0, totalTools: 0,
          duration: Date.now() - startTime,
          llmLatency: 0, toolLatency: 0,
        },
      };
    } finally {
      // P0-7: flush 异步记忆写入队列，确保所有 saveMemory 调用完成
      await this.flushMemoryWriter();
      // 清理 capCache，避免下次 Run() 误用旧引用
      this.capCache = null;
    }
  }

  // ===== P0-3: runLoop 共享循环体 =====
  // 与 Go 端 runLoop 对齐，被 runEngine 和 resumeFromCheckpoint 共享
  private async runLoop(
    startTime: number,
    _totalLLMLatency: number,
    _totalToolLatency: number,
    _toolCount: number,
    startTurn: number,
    options?: RunOptions,
  ): Promise<Response> {
    let totalLLMLatency = _totalLLMLatency;
    let totalToolLatency = _totalToolLatency;
    let toolCount = _toolCount;
    let turn = startTurn;

    for (; turn < this.maxTurns; turn++) {
      // P0-1: 取消信号检查
      this.checkSignal(options?.signal);

      if (this.lifecycle.isStopped()) break;

      // P3-1: 创建 turn-level Span
      const turnSpanId = this.capCache?.otelBridge?.startSpan(`agent.turn`, {
        'agent.name': this.name,
        'agent.turn': turn,
      });

      // P0-5: 成本预算检查（循环内每轮）
      if (this.capCache?.costTracker?.checkBudget()) {
        const response: Response = {
          content: 'Agent stopped: cost budget exceeded',
          metrics: {
            totalTurns: turn, totalTools: toolCount,
            duration: Date.now() - startTime,
            llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
          },
        };
        this.lifecycle.setStatus('completed');
        this.capCache?.otelBridge?.endSpan(turnSpanId!, 'ok');
        return response;
      }

      // P0-6: 仅在有订阅者时触发 hook
      await this.hooks.fireHook('before_turn', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn,
      });

      this.trimMessages();

      // P3-1: 创建 LLM call Span
      const llmSpanId = this.capCache?.otelBridge?.startSpan(`agent.llm_call`, {
        'agent.name': this.name,
        'agent.turn': turn,
      });
      const llmStart = Date.now();
      const thought = await this.callLLM();
      totalLLMLatency += Date.now() - llmStart;
      this.capCache?.otelBridge?.addAttribute(llmSpanId!, 'llm.latency_ms', Date.now() - llmStart);
      this.capCache?.otelBridge?.addAttribute(llmSpanId!, 'llm.has_tool_calls', !!(thought.toolCalls && thought.toolCalls.length > 0));
      this.capCache?.otelBridge?.endSpan(llmSpanId!, 'ok');

      await this.hooks.fireHook('after_llm', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn,
      });

      if (!thought.toolCalls || thought.toolCalls.length === 0) {
        const duration = Date.now() - startTime;
        const response: Response = {
          content: thought.content,
          metrics: {
            totalTurns: turn + 1,
            totalTools: toolCount,
            duration,
            llmLatency: totalLLMLatency,
            toolLatency: totalToolLatency,
          },
        };

        // P0-3: 保存 checkpoint（如果配置了 checkpointStore）
        if (this.capCache?.checkpointStore) {
          await this.saveCheckpoint(turn + 1, response).catch(() => {});
        }

        this.lifecycle.setStatus('completed');
        await this.hooks.fireHook('on_complete', {
          agentID: this.name,
          sessionID: this.sessionId,
          turn,
          response,
        });
        await this.hooks.fireHook('after_turn', {
          agentID: this.name,
          sessionID: this.sessionId,
          turn,
        });

        // P3-1: 结束 turn-level Span
        this.capCache?.otelBridge?.endSpan(turnSpanId!, 'ok');

        return response;
      }

      // P4-A1: 并行工具执行 — 同一轮的多个工具调用可以并行执行
      // 这在 I/O 密集型工具（如 HTTP 请求、数据库查询）场景下可显著降低延迟
      if (this.parallelToolExecution && thought.toolCalls.length > 1) {
        const toolResults = await this.executeToolsParallel(
          thought.toolCalls, turn, startTime, totalLLMLatency, totalToolLatency, toolCount,
        );
        totalToolLatency = toolResults.totalToolLatency;
        toolCount = toolResults.toolCount;

        // P4-A5: 并行执行后检查 graceful shutdown
        if (this.gracefulShutdownFlag) {
          const response: Response = {
            content: 'Agent graceful shutdown: completed after tool execution',
            metrics: {
              totalTurns: turn + 1, totalTools: toolCount,
              duration: Date.now() - startTime,
              llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
            },
          };
          this.lifecycle.setStatus('completed');
          return response;
        }

        // 检查是否有连续失败导致的停止
        if (toolResults.shouldStop) {
          const response: Response = {
            content: `Agent stopped: ${this.consecutiveFailures} consecutive tool failures`,
            metrics: {
              totalTurns: turn + 1, totalTools: toolCount,
              duration: Date.now() - startTime,
              llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
            },
          };
          this.lifecycle.setStatus('completed');
          return response;
        }
      } else {
        // 串行执行（默认行为，与 Go 端一致）
        for (const tc of thought.toolCalls) {
          await this.hooks.fireHook('before_tool', {
            agentID: this.name,
            sessionID: this.sessionId,
            turn,
            toolCall: tc,
          });

          // P3-1: 创建 tool call Span
          const toolSpanId = this.capCache?.otelBridge?.startSpan(`agent.tool_call`, {
            'agent.name': this.name,
            'tool.name': tc.name,
            'agent.turn': turn,
          });
          const toolStart = Date.now();
          const result = await this.toolkit.execute(tc);
          totalToolLatency += Date.now() - toolStart;
          toolCount++;
          this.capCache?.otelBridge?.addAttribute(toolSpanId!, 'tool.latency_ms', Date.now() - toolStart);
          this.capCache?.otelBridge?.addAttribute(toolSpanId!, 'tool.is_error', !!result.isError);
          this.capCache?.otelBridge?.endSpan(toolSpanId!, result.isError ? 'error' : 'ok');

          if (result.isError) {
            this.consecutiveFailures++;
            if (this.consecutiveFailures >= this.maxConsecutiveFailures) {
              const response: Response = {
                content: `Agent stopped: ${this.consecutiveFailures} consecutive tool failures`,
                metrics: {
                  totalTurns: turn + 1,
                  totalTools: toolCount,
                  duration: Date.now() - startTime,
                  llmLatency: totalLLMLatency,
                  toolLatency: totalToolLatency,
                },
              };
              this.lifecycle.setStatus('completed');
              return response;
            }
          } else {
            this.consecutiveFailures = 0;
          }

          this.messages.push({
            role: 'tool',
            content: result.content,
            toolCallId: tc.id,
            name: tc.name,
          });

          // P0-7: 异步记忆写入（不阻塞主循环）
          if (this.capCache?.memoryStore) {
            const mem = this.capCache.memoryStore;
            this.pendingMemoryWrites.push(
              mem.add({
                id: `${this.name}-${this.sessionId}-${turn}-${tc.id}`,
                sessionId: this.sessionId,
                role: 'tool',
                content: result.content,
                metadata: { toolCallId: tc.id, toolName: tc.name },
                createdAt: new Date().toISOString(),
              }).catch(() => {}),
            );
          }

          await this.hooks.fireHook('after_tool', {
            agentID: this.name,
            sessionID: this.sessionId,
            turn,
            toolResult: result,
          });
        }
      }

      // P0-3: 每轮结束保存 checkpoint
      if (this.capCache?.checkpointStore) {
        await this.saveCheckpoint(turn + 1, null).catch(() => {});
      }

      await this.hooks.fireHook('after_turn', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn,
      });

      // P3-1: 结束 turn-level Span
      this.capCache?.otelBridge?.endSpan(turnSpanId!, 'ok');
    }

    const duration = Date.now() - startTime;
    const response: Response = {
      content: this.messages[this.messages.length - 1]?.content ?? '',
      metrics: {
        totalTurns: turn,
        totalTools: toolCount,
        duration,
        llmLatency: totalLLMLatency,
        toolLatency: totalToolLatency,
      },
    };

    this.lifecycle.setStatus('completed');
    return response;
  }

  // ===== P0-3: Checkpoint 保存与恢复 =====

  private async saveCheckpoint(turn: number, finalResponse: Response | null): Promise<void> {
    if (!this.capCache?.checkpointStore) return;
    const checkpoint: Checkpoint = {
      id: `${this.name}-${this.sessionId}-${turn}`,
      sessionID: this.sessionId,
      turn,
      messages: [...this.messages],
      metrics: finalResponse?.metrics ?? {
        totalTurns: turn,
        totalTools: 0,
        duration: 0,
        llmLatency: 0,
        toolLatency: 0,
      },
      createdAt: new Date().toISOString(),
    };
    await this.capCache.checkpointStore.save(checkpoint);
  }

  /** P0-3: 从 Checkpoint 恢复并继续运行。
   *
   * 与 Go 端 ResumeFromCheckpoint 对齐，从指定 checkpoint 的 turn 和 messages 继续 ReAct 循环。
   *
   * 参数：
   * - checkpoint: 之前保存的 Checkpoint 对象
   * - options: 可选参数，支持 AbortSignal 取消
   *
   * 返回：包含最终内容和指标的 Response 对象
   */
  async resumeFromCheckpoint(checkpoint: Checkpoint, options?: RunOptions): Promise<Response> {
    // P3-4: runMu 互斥锁
    const runRelease = await this.acquireRunLock();
    try {
      this.lifecycle.setStatus('running');
      const startTime = Date.now();
      this.consecutiveFailures = 0;
      this.capCache = this.resolveCapabilities();

      // P3-1: 创建 run-level Span
      const runSpanId = this.capCache.otelBridge?.startSpan('agent.resume', {
        'agent.name': this.name,
        'agent.session_id': this.sessionId,
        'agent.resume_turn': checkpoint.turn,
      });

      // 恢复消息历史
      this.messages = [...checkpoint.messages];

      this.checkSignal(options?.signal);

      await this.hooks.fireHook('before_run', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn: checkpoint.turn,
      });

      try {
        const response = await this.runLoop(
          startTime, 0, 0, 0, checkpoint.turn, options,
        );

        await this.hooks.fireHook('after_run', {
          agentID: this.name,
          sessionID: this.sessionId,
          turn: checkpoint.turn,
          response,
        });

        this.capCache.otelBridge?.endSpan(runSpanId!, 'ok');

        return response;
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        this.lifecycle.setStatus('error');
        if (runSpanId) {
          this.capCache.otelBridge?.addEvent(runSpanId, 'error', { message: error.message });
          this.capCache.otelBridge?.endSpan(runSpanId, 'error');
        }
        await this.hooks.fireHook('on_error', {
          agentID: this.name,
          sessionID: this.sessionId,
          turn: checkpoint.turn,
          error,
        });
        return {
          content: `Agent error: ${error.message}`,
          metrics: {
            totalTurns: checkpoint.turn, totalTools: 0,
            duration: Date.now() - startTime,
            llmLatency: 0, toolLatency: 0,
          },
        };
      } finally {
        await this.flushMemoryWriter();
        this.capCache = null;
      }
    } finally {
      runRelease();
    }
  }

  /**
   * Stream the agent's response as text tokens.
   *
   * - When no tools are registered: streams LLM tokens directly (true token-by-token).
   * - When tools are registered: runs the full ReAct loop, yielding content from each turn.
   *   Tool results are NOT yielded (use streamEvents() for structured events).
   */
  async *stream(input: string, options?: RunOptions): AsyncIterable<string> {
    for await (const event of this.streamEvents(input, options)) {
      if (event.type === 'token' && event.content) {
        yield event.content;
      }
    }
  }

  /**
   * Stream structured events from the full ReAct loop.
   *
   * Events include:
   * - token: LLM text output (streamed token by token when possible)
   * - tool_call: a tool was invoked by the LLM
   * - tool_result: a tool execution completed
   * - turn_end: a ReAct turn completed
   * - done: the agent finished with a final response
   * - error: an error occurred
   */
  async *streamEvents(input: string, options?: RunOptions): AsyncIterable<StreamEvent> {
    validateAgentInput(input);
    this.lifecycle.setStatus('running');
    const startTime = Date.now();
    let totalLLMLatency = 0;
    let totalToolLatency = 0;
    let toolCount = 0;
    this.consecutiveFailures = 0;

    // P0-4: 能力缓存
    this.capCache = this.resolveCapabilities();

    this.messages = [];
    if (this.systemPrompt) {
      this.messages.push({ role: 'system', content: this.systemPrompt });
    }

    // P6-A3: Tool Learning 闭环 — 流式模式同样注入 few-shot 示例
    if (this.enhancedToolLearner && this.toolkit.size() > 0) {
      try {
        const toolNames = this.toolkit.list().map((t) => t.name);
        const guide = await this.enhancedToolLearner.generateUsageGuide(toolNames);
        if (guide && this.messages.length > 0 && this.messages[0]!.role === 'system') {
          this.messages[0]!.content += '\n\n' + guide;
        }
      } catch {
        // few-shot 注入失败不影响主流程
      }
    }

    // P6-A2: 自调优 — 流式模式同样应用调优建议
    if (this.selfTuner && this.autoTune && this.lastRunMetrics) {
      const suggestion = this.selfTuner.getSuggestion();
      if (suggestion.shouldAdjust) {
        this.applyTuningSuggestion(suggestion);
      }
    }

    this.messages.push({ role: 'user', content: input });

    await this.hooks.fireHook('before_run', {
      agentID: this.name, sessionID: this.sessionId, turn: 0,
    });

    // P0-2: Panic Recovery 包裹流式循环
    try {
      const hasTools = this.toolkit.size() > 0;

      let turn = 0;
      for (; turn < this.maxTurns; turn++) {
        // P0-1: 取消信号检查
        this.checkSignal(options?.signal);

        if (this.lifecycle.isStopped()) break;

        // P0-5: 成本预算检查
        if (this.capCache?.costTracker?.checkBudget()) {
          const response: Response = {
            content: 'Agent stopped: cost budget exceeded',
            metrics: {
              totalTurns: turn + 1, totalTools: toolCount,
              duration: Date.now() - startTime,
              llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
            },
          };
          this.lifecycle.setStatus('completed');
          yield { type: 'done', response };
          return;
        }

        await this.hooks.fireHook('before_turn', {
          agentID: this.name, sessionID: this.sessionId, turn,
        });

        this.trimMessages();

        const llmStart = Date.now();

        if (!hasTools) {
          // No tools: stream directly from LLM if supported
          if (this.model.stream) {
            let fullContent = '';
            for await (const chunk of this.model.stream({ messages: this.messages })) {
              // P0-1: 流式取消
              this.checkSignal(options?.signal);
              if (chunk.content) {
                fullContent += chunk.content;
                yield { type: 'token', content: chunk.content };
              }
              if (chunk.done) break;
            }
            totalLLMLatency += Date.now() - llmStart;
            this.messages.push({ role: 'assistant', content: fullContent });

            const response: Response = {
              content: fullContent,
              metrics: {
                totalTurns: turn + 1, totalTools: 0,
                duration: Date.now() - startTime,
                llmLatency: totalLLMLatency, toolLatency: 0,
              },
            };
            this.lifecycle.setStatus('completed');
            yield { type: 'done', response };
            return;
          } else {
            const resp = await this.model.complete({ messages: this.messages });
            totalLLMLatency += Date.now() - llmStart;
            this.messages.push({ role: 'assistant', content: resp.content });
            yield { type: 'token', content: resp.content };

            const response: Response = {
              content: resp.content,
              metrics: {
                totalTurns: turn + 1, totalTools: 0,
                duration: Date.now() - startTime,
                llmLatency: totalLLMLatency, toolLatency: 0,
              },
            };
            this.lifecycle.setStatus('completed');
            yield { type: 'done', response };
            return;
          }
        }

        // Tools available: use callTools (non-streaming) for each turn
        const resp = await this.model.callTools({
          messages: this.messages,
          tools: this.toolkit.definitions(),
        });
        totalLLMLatency += Date.now() - llmStart;

        await this.hooks.fireHook('after_llm', {
          agentID: this.name, sessionID: this.sessionId, turn,
        });

        // Yield the LLM's thinking content as a token
        if (resp.content) {
          yield { type: 'token', content: resp.content };
        }

        this.messages.push({
          role: 'assistant',
          content: resp.content,
          toolCalls: resp.toolCalls.length > 0 ? resp.toolCalls : undefined,
        });

        // No tool calls → final answer
        if (resp.toolCalls.length === 0) {
          const response: Response = {
            content: resp.content,
            metrics: {
              totalTurns: turn + 1, totalTools: toolCount,
              duration: Date.now() - startTime,
              llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
            },
          };
          this.lifecycle.setStatus('completed');

          // P0-3: 保存最终 checkpoint
          if (this.capCache?.checkpointStore) {
            await this.saveCheckpoint(turn + 1, response).catch(() => {});
          }

          await this.hooks.fireHook('on_complete', {
            agentID: this.name, sessionID: this.sessionId, turn, response,
          });
          await this.hooks.fireHook('after_turn', {
            agentID: this.name, sessionID: this.sessionId, turn,
          });

          yield { type: 'done', response };
          return;
        }

        // P4-A1: 并行工具执行（流式模式）
        // 在流式模式下，tool_call 事件先全部 yield，然后并行执行
        if (this.parallelToolExecution && resp.toolCalls.length > 1) {
          // 先 yield 所有 tool_call 事件
          for (const tc of resp.toolCalls) {
            await this.hooks.fireHook('before_tool', {
              agentID: this.name, sessionID: this.sessionId, turn, toolCall: tc,
            });
            yield { type: 'tool_call', toolCall: tc, turn };
          }

          // 并行执行所有工具
          const toolBatchStart = Date.now();
          const results = await Promise.all(
            resp.toolCalls.map((tc) => this.toolkit.execute(tc)),
          );
          totalToolLatency += Date.now() - toolBatchStart;
          toolCount += results.length;

          // 串行处理结果
          for (let i = 0; i < resp.toolCalls.length; i++) {
            const tc = resp.toolCalls[i];
            const result = results[i];

            yield { type: 'tool_result', result, turn };

            if (result.isError) {
              this.consecutiveFailures++;
              if (this.consecutiveFailures >= this.maxConsecutiveFailures) {
                const response: Response = {
                  content: `Agent stopped: ${this.consecutiveFailures} consecutive tool failures`,
                  metrics: {
                    totalTurns: turn + 1, totalTools: toolCount,
                    duration: Date.now() - startTime,
                    llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
                  },
                };
                this.lifecycle.setStatus('completed');
                yield { type: 'done', response };
                return;
              }
            } else {
              this.consecutiveFailures = 0;
            }

            this.messages.push({
              role: 'tool', content: result.content,
              toolCallId: tc.id, name: tc.name,
            });

            if (this.capCache?.memoryStore) {
              const mem = this.capCache.memoryStore;
              this.pendingMemoryWrites.push(
                mem.add({
                  id: `${this.name}-${this.sessionId}-${turn}-${tc.id}`,
                  sessionId: this.sessionId,
                  role: 'tool',
                  content: result.content,
                  metadata: { toolCallId: tc.id, toolName: tc.name },
                  createdAt: new Date().toISOString(),
                }).catch(() => {}),
              );
            }

            await this.hooks.fireHook('after_tool', {
              agentID: this.name, sessionID: this.sessionId, turn, toolResult: result,
            });
          }

          // P4-A5: 流式模式下检查 graceful shutdown
          if (this.gracefulShutdownFlag) {
            const response: Response = {
              content: 'Agent graceful shutdown: completed after tool execution',
              metrics: {
                totalTurns: turn + 1, totalTools: toolCount,
                duration: Date.now() - startTime,
                llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
              },
            };
            this.lifecycle.setStatus('completed');
            yield { type: 'done', response };
            return;
          }
        } else {
          // 串行执行（默认行为）
          for (const tc of resp.toolCalls) {
            await this.hooks.fireHook('before_tool', {
              agentID: this.name, sessionID: this.sessionId, turn, toolCall: tc,
            });

            yield { type: 'tool_call', toolCall: tc, turn };

            const toolStart = Date.now();
            const result = await this.toolkit.execute(tc);
            totalToolLatency += Date.now() - toolStart;
            toolCount++;

            yield { type: 'tool_result', result, turn };

            if (result.isError) {
              this.consecutiveFailures++;
              if (this.consecutiveFailures >= this.maxConsecutiveFailures) {
                const response: Response = {
                  content: `Agent stopped: ${this.consecutiveFailures} consecutive tool failures`,
                  metrics: {
                    totalTurns: turn + 1, totalTools: toolCount,
                    duration: Date.now() - startTime,
                    llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
                  },
                };
                this.lifecycle.setStatus('completed');
                yield { type: 'done', response };
                return;
              }
            } else {
              this.consecutiveFailures = 0;
            }

            this.messages.push({
              role: 'tool', content: result.content,
              toolCallId: tc.id, name: tc.name,
            });

            // P0-7: 异步记忆写入
            if (this.capCache?.memoryStore) {
              const mem = this.capCache.memoryStore;
              this.pendingMemoryWrites.push(
                mem.add({
                  id: `${this.name}-${this.sessionId}-${turn}-${tc.id}`,
                  sessionId: this.sessionId,
                  role: 'tool',
                  content: result.content,
                  metadata: { toolCallId: tc.id, toolName: tc.name },
                  createdAt: new Date().toISOString(),
                }).catch(() => {}),
              );
            }

            await this.hooks.fireHook('after_tool', {
              agentID: this.name, sessionID: this.sessionId, turn, toolResult: result,
            });
          }
        }

        yield { type: 'turn_end', turn };

        await this.hooks.fire({
          agentID: this.name, sessionID: this.sessionId, point: 'after_turn', turn,
        });
      }

      // Max turns exceeded
      const response: Response = {
        content: this.messages[this.messages.length - 1]?.content ?? '',
        metrics: {
          totalTurns: turn, totalTools: toolCount,
          duration: Date.now() - startTime,
          llmLatency: totalLLMLatency, toolLatency: totalToolLatency,
        },
      };
      this.lifecycle.setStatus('completed');
      yield { type: 'done', response };
    } catch (err) {
      // P0-2: Panic Recovery
      const error = err instanceof Error ? err : new Error(String(err));
      this.lifecycle.setStatus('error');
      await this.hooks.fireHook('on_error', {
        agentID: this.name, sessionID: this.sessionId, turn: 0, error,
      });

      if (error.name === 'AbortError') {
        yield {
          type: 'done',
          response: {
            content: 'Agent run cancelled',
            metrics: {
              totalTurns: 0, totalTools: 0,
              duration: Date.now() - startTime,
              llmLatency: 0, toolLatency: 0,
            },
          },
        };
      } else {
        yield { type: 'error', error };
      }
    } finally {
      // P0-7: flush 异步记忆
      await this.flushMemoryWriter();
      this.capCache = null;
    }
  }

  private async callLLM(): Promise<{ content: string; toolCalls?: ToolCall[] }> {
    if (this.toolkit.size() > 0) {
      const resp = await this.model.callTools({
        messages: this.messages,
        tools: this.toolkit.definitions(),
      });
      this.messages.push({
        role: 'assistant',
        content: resp.content,
        toolCalls: resp.toolCalls.length > 0 ? resp.toolCalls : undefined,
      });
      return { content: resp.content, toolCalls: resp.toolCalls };
    }

    const resp = await this.model.complete({ messages: this.messages });
    this.messages.push({ role: 'assistant', content: resp.content });
    return { content: resp.content };
  }

  private trimMessages(): void {
    if (this.messages.length <= this.maxMessages) return;
    const system = this.messages.filter((m) => m.role === 'system');
    const rest = this.messages.filter((m) => m.role !== 'system');
    const keep = this.maxMessages - system.length;
    this.messages = [...system, ...rest.slice(-keep)];
  }

  // P6-A2: 应用自调优建议到当前 Agent 配置
  private applyTuningSuggestion(suggestion: TuningSuggestion): void {
    if (suggestion.maxTurns !== undefined && suggestion.maxTurns >= 1 && suggestion.maxTurns <= 100) {
      this.maxTurns = suggestion.maxTurns;
    }
    if (suggestion.parallelToolExecution !== undefined) {
      this.parallelToolExecution = suggestion.parallelToolExecution;
    }
    if (suggestion.maxParallelTools !== undefined && suggestion.maxParallelTools >= 0) {
      this.maxParallelTools = suggestion.maxParallelTools;
    }
    if (suggestion.maxConsecutiveFailures !== undefined && suggestion.maxConsecutiveFailures >= 1) {
      this.maxConsecutiveFailures = suggestion.maxConsecutiveFailures;
    }
  }
}
