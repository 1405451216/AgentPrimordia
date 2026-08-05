/**
 * TurnExecutor — ReAct 单轮执行引擎
 *
 * 从 react-loop.ts 拆分而来，封装单轮 ReAct 循环的核心逻辑：
 * - LLM 调用（callTools / complete）
 * - 工具执行（串行 + 并行共用 executeTools）
 * - Hook 触发（before_turn / after_turn / before_tool / after_tool / after_llm / on_complete）
 * - Checkpoint 保存
 * - 成本预算检查
 * - 连续失败停止
 * - Graceful shutdown 检查
 * - OTel span 追踪
 *
 * 被 ReActAgent.runLoop() 和 streamEvents() 共享使用，消除重复代码。
 */
import type { Message, ToolCall, Response, ToolResult } from '../types.js';
import type { Provider } from '../llm/provider.js';
import type { ToolRegistry } from '../tools/registry.js';
import type { CostTracker, Checkpoint, CheckpointStore } from './request-id.js';
import type { Memory } from '../memory/store.js';
import type { OTelBridgeLike } from '../metrics/otel-extended.js';
import type { StreamEvent } from './hooks.js';
import type { HookManager } from './hooks.js';
import { validateLLMCompletion, validateToolCallResponse } from '../schema/index.js';

// ===== 能力缓存接口 =====

/** Agent 运行时能力缓存，在 Run 入口一次性解析后供整个循环使用 */
export interface CapabilitiesCache {
  costTracker: CostTracker | null;
  memoryStore: Memory | null;
  checkpointStore: CheckpointStore | null;
  otelBridge: OTelBridgeLike | null;
}

// ===== TurnExecutor 配置 =====

export interface TurnExecutorConfig {
  name: string;
  sessionId: string;
  maxConsecutiveFailures: number;
  parallelToolExecution: boolean;
  maxParallelTools: number;
  maxMessages: number;
  /** 检查是否请求优雅关闭 */
  isGracefulShutdownRequested?: () => boolean;
  /** 输出端护栏检查函数（v3.4-5）：每轮 LLM 响应写入历史前检查；返回放行与否与脱敏文本，阻断时抛错 */
  outputGuard?: (content: string) => { passed: boolean; modified: string };
}

// ===== 运行时状态 =====

/** 跨 turn 累积的运行时状态（可变引用） */
export interface TurnState {
  consecutiveFailures: number;
  totalLLMLatency: number;
  totalToolLatency: number;
  toolCount: number;
  pendingMemoryWrites: Promise<void>[];
}

// ===== 执行结果 =====

/** 工具执行结果 */
export interface ToolExecutionResult {
  results: ToolResult[];
  totalToolLatency: number;
  toolCount: number;
  shouldStop: boolean;
  events: StreamEvent[];
}

/** 工具执行模式 */
export type ToolExecutionMode = 'serial' | 'parallel';

/** 单轮执行结果 */
export interface TurnResult {
  /** LLM 响应 */
  thought: { content: string; toolCalls?: ToolCall[] };
  /** 工具执行结果（当有工具调用时） */
  tools?: ToolExecutionResult;
  /** 是否应停止循环 */
  shouldStop: boolean;
  /** 停止原因 */
  stopReason?: 'no_more_tools' | 'consecutive_failures' | 'cost_budget' | 'graceful_shutdown' | 'max_turns';
  /** 最终响应（当应停止时） */
  response?: Response;
  /** 收集的事件（供流式模式使用） */
  events: StreamEvent[];
}

// ===== TurnExecutor 类 =====

export class TurnExecutor {
  constructor(
    private model: Provider,
    private toolkit: ToolRegistry,
    private hooks: HookManager,
    private capCache: CapabilitiesCache,
    private config: TurnExecutorConfig,
  ) {}

  /**
   * 执行单轮 ReAct 循环。
   *
   * 包含：成本预算检查 → before_turn hook → LLM 调用 → after_llm hook →
   * 工具执行（如有） → checkpoint 保存 → after_turn hook。
   *
   * @param messages 消息历史（会被修改：追加 assistant/tool 消息）
   * @param turn 当前轮次编号
   * @param state 跨 turn 累积的可变状态
   * @param startTime 运行开始时间（用于计算 duration）
   */
  async executeTurn(
    messages: Message[],
    turn: number,
    state: TurnState,
    startTime: number,
  ): Promise<TurnResult> {
    const events: StreamEvent[] = [];

    // P0-5: 成本预算检查（循环内每轮）
    if (this.capCache.costTracker?.checkBudget()) {
      return {
        thought: { content: '' },
        shouldStop: true,
        stopReason: 'cost_budget',
        response: {
          content: 'Agent stopped: cost budget exceeded',
          metrics: {
            totalTurns: turn,
            totalTools: state.toolCount,
            duration: Date.now() - startTime,
            llmLatency: state.totalLLMLatency,
            toolLatency: state.totalToolLatency,
          },
        },
        events,
      };
    }

    // 消息裁剪（保持上下文窗口）
    this.trimMessages(messages);

    // 触发 before_turn hook
    await this.hooks.fireHook('before_turn', {
      agentID: this.config.name,
      sessionID: this.config.sessionId,
      turn,
    });

    // ===== LLM 调用 =====
    const hasTools = this.toolkit.size() > 0;
    const llmSpanId = this.capCache.otelBridge?.startSpan('agent.llm_call', {
      'agent.name': this.config.name,
      'agent.turn': turn,
    });
    const llmStart = Date.now();

    let thought: { content: string; toolCalls?: ToolCall[] };
    if (hasTools) {
      const rawResp = await this.model.callTools({
        messages,
        tools: this.toolkit.definitions(),
      });
      const validation = validateToolCallResponse(rawResp);
      if (!validation.ok) {
        console.warn('[TurnExecutor] callTools 响应验证失败:', validation.errors.join('; '));
      }
      const resp = validation.ok ? validation.data : (rawResp as { content: string; toolCalls: ToolCall[] });
      thought = { content: resp.content, toolCalls: resp.toolCalls };
    } else {
      const rawResp = await this.model.complete({ messages });
      const validation = validateLLMCompletion(rawResp);
      if (!validation.ok) {
        console.warn('[TurnExecutor] complete 响应验证失败:', validation.errors.join('; '));
      }
      const resp = validation.ok ? validation.data : (rawResp as { content: string });
      thought = { content: resp.content };
    }

    // v3.4-5：输出端护栏——逐轮检查 LLM 响应，写入消息历史前脱敏/拒绝（对齐 Go OutputGuard）
    if (this.config.outputGuard && thought.content) {
      const guarded = this.config.outputGuard(thought.content);
      if (!guarded.passed) {
        throw new Error('output blocked by guardrail');
      }
      thought.content = guarded.modified;
    }

    if (hasTools) {
      messages.push({
        role: 'assistant',
        content: thought.content,
        toolCalls: thought.toolCalls && thought.toolCalls.length > 0 ? thought.toolCalls : undefined,
      });
    } else {
      messages.push({ role: 'assistant', content: thought.content });
    }

    state.totalLLMLatency += Date.now() - llmStart;

    // OTel: 记录 LLM 延迟和工具调用标记
    if (llmSpanId) {
      this.capCache.otelBridge?.addAttribute(llmSpanId, 'llm.latency_ms', Date.now() - llmStart);
      this.capCache.otelBridge?.addAttribute(
        llmSpanId,
        'llm.has_tool_calls',
        !!(thought.toolCalls && thought.toolCalls.length > 0),
      );
      this.capCache.otelBridge?.endSpan(llmSpanId, 'ok');
    }

    // 触发 after_llm hook
    await this.hooks.fireHook('after_llm', {
      agentID: this.config.name,
      sessionID: this.config.sessionId,
      turn,
    });

    // 收集 token 事件（流式模式使用）
    if (thought.content) {
      events.push({ type: 'token', content: thought.content });
    }

    // 无工具调用 → 返回最终答案
    if (!thought.toolCalls || thought.toolCalls.length === 0) {
      const response: Response = {
        content: thought.content,
        metrics: {
          totalTurns: turn + 1,
          totalTools: state.toolCount,
          duration: Date.now() - startTime,
          llmLatency: state.totalLLMLatency,
          toolLatency: state.totalToolLatency,
        },
      };

      // 保存 checkpoint
      await this.saveCheckpoint(turn + 1, messages, response);

      // 触发 on_complete hook
      await this.hooks.fireHook('on_complete', {
        agentID: this.config.name,
        sessionID: this.config.sessionId,
        turn,
        response,
      });

      // 触发 after_turn hook
      await this.hooks.fireHook('after_turn', {
        agentID: this.config.name,
        sessionID: this.config.sessionId,
        turn,
      });

      return { thought, shouldStop: true, stopReason: 'no_more_tools', response, events };
    }

    // ===== 工具执行 =====
    const mode: ToolExecutionMode =
      this.config.parallelToolExecution && thought.toolCalls.length > 1 ? 'parallel' : 'serial';
    const toolExec = await this.executeTools(messages, thought.toolCalls, turn, mode, state);
    events.push(...toolExec.events);

    // P4-A5: Graceful shutdown 检查
    if (this.config.isGracefulShutdownRequested?.()) {
      const response: Response = {
        content: 'Agent graceful shutdown: completed after tool execution',
        metrics: {
          totalTurns: turn + 1,
          totalTools: toolExec.toolCount,
          duration: Date.now() - startTime,
          llmLatency: state.totalLLMLatency,
          toolLatency: toolExec.totalToolLatency,
        },
      };
      return { thought, tools: toolExec, shouldStop: true, stopReason: 'graceful_shutdown', response, events };
    }

    // 连续失败检查
    if (toolExec.shouldStop) {
      const response: Response = {
        content: `Agent stopped: ${state.consecutiveFailures} consecutive tool failures`,
        metrics: {
          totalTurns: turn + 1,
          totalTools: toolExec.toolCount,
          duration: Date.now() - startTime,
          llmLatency: state.totalLLMLatency,
          toolLatency: toolExec.totalToolLatency,
        },
      };
      return { thought, tools: toolExec, shouldStop: true, stopReason: 'consecutive_failures', response, events };
    }

    // 保存 checkpoint（每轮结束）
    await this.saveCheckpoint(turn + 1, messages, null);

    // 触发 after_turn hook
    await this.hooks.fireHook('after_turn', {
      agentID: this.config.name,
      sessionID: this.config.sessionId,
      turn,
    });

    return { thought, tools: toolExec, shouldStop: false, events };
  }

  /**
   * 工具执行（串行 + 并行共用）。
   *
   * @param messages 消息历史
   * @param toolCalls 待执行的工具调用列表
   * @param turn 当前轮次
   * @param mode 执行模式：serial 或 parallel
   * @param state 跨 turn 累积的可变状态
   */
  async executeTools(
    messages: Message[],
    toolCalls: ToolCall[],
    turn: number,
    mode: ToolExecutionMode,
    state: TurnState,
  ): Promise<ToolExecutionResult> {
    return mode === 'parallel'
      ? this.executeToolsParallel(messages, toolCalls, turn, state)
      : this.executeToolsSerial(messages, toolCalls, turn, state);
  }

  // ===== 串行工具执行 =====

  private async executeToolsSerial(
    messages: Message[],
    toolCalls: ToolCall[],
    turn: number,
    state: TurnState,
  ): Promise<ToolExecutionResult> {
    const events: StreamEvent[] = [];
    const results: ToolResult[] = [];
    let totalToolLatency = state.totalToolLatency;
    let toolCount = state.toolCount;
    let shouldStop = false;

    for (const tc of toolCalls) {
      // before_tool hook
      await this.hooks.fireHook('before_tool', {
        agentID: this.config.name,
        sessionID: this.config.sessionId,
        turn,
        toolCall: tc,
      });
      events.push({ type: 'tool_call', toolCall: tc, turn });

      // OTel: 创建 tool call Span
      const toolSpanId = this.capCache.otelBridge?.startSpan('agent.tool_call', {
        'agent.name': this.config.name,
        'tool.name': tc.name,
        'agent.turn': turn,
      });
      const toolStart = Date.now();
      const result = await this.toolkit.execute(tc);
      const latency = Date.now() - toolStart;
      totalToolLatency += latency;
      toolCount++;

      // OTel: 记录工具延迟和错误标记
      if (toolSpanId) {
        this.capCache.otelBridge?.addAttribute(toolSpanId, 'tool.latency_ms', latency);
        this.capCache.otelBridge?.addAttribute(toolSpanId, 'tool.is_error', !!result.isError);
        this.capCache.otelBridge?.endSpan(toolSpanId, result.isError ? 'error' : 'ok');
      }

      events.push({ type: 'tool_result', result, turn });

      // 连续失败计数
      if (result.isError) {
        state.consecutiveFailures++;
        if (state.consecutiveFailures >= this.config.maxConsecutiveFailures) {
          shouldStop = true;
        }
      } else {
        state.consecutiveFailures = 0;
      }

      // 追加工具结果到消息
      messages.push({
        role: 'tool',
        content: result.content,
        toolCallId: tc.id,
        name: tc.name,
      });

      // 异步记忆写入（不阻塞主循环）
      if (this.capCache.memoryStore) {
        const mem = this.capCache.memoryStore;
        state.pendingMemoryWrites.push(
          mem.add({
            id: `${this.config.name}-${this.config.sessionId}-${turn}-${tc.id}`,
            sessionId: this.config.sessionId,
            role: 'tool',
            content: result.content,
            metadata: { toolCallId: tc.id, toolName: tc.name },
            createdAt: new Date().toISOString(),
          }).catch(() => {}),
        );
      }

      // after_tool hook
      await this.hooks.fireHook('after_tool', {
        agentID: this.config.name,
        sessionID: this.config.sessionId,
        turn,
        toolResult: result,
      });
    }

    state.totalToolLatency = totalToolLatency;
    state.toolCount = toolCount;

    return { results, totalToolLatency, toolCount, shouldStop, events };
  }

  // ===== 并行工具执行 =====

  private async executeToolsParallel(
    messages: Message[],
    toolCalls: ToolCall[],
    turn: number,
    state: TurnState,
  ): Promise<ToolExecutionResult> {
    const events: StreamEvent[] = [];
    const _totalToolLatency = state.totalToolLatency;

    // 并行执行单个工具调用（含 before_tool hook + OTel span）
    const executeOne = async (tc: ToolCall): Promise<ToolResult> => {
      await this.hooks.fireHook('before_tool', {
        agentID: this.config.name,
        sessionID: this.config.sessionId,
        turn,
        toolCall: tc,
      });
      events.push({ type: 'tool_call', toolCall: tc, turn });

      const toolSpanId = this.capCache.otelBridge?.startSpan('agent.tool_call', {
        'agent.name': this.config.name,
        'tool.name': tc.name,
        'agent.turn': turn,
      });
      const toolStart = Date.now();
      const result = await this.toolkit.execute(tc);
      const latency = Date.now() - toolStart;

      if (toolSpanId) {
        this.capCache.otelBridge?.addAttribute(toolSpanId, 'tool.latency_ms', latency);
        this.capCache.otelBridge?.addAttribute(toolSpanId, 'tool.is_error', !!result.isError);
        this.capCache.otelBridge?.endSpan(toolSpanId, result.isError ? 'error' : 'ok');
      }

      return result;
    };

    // 支持分批执行（maxParallelTools 限制）
    const toolBatchStart = Date.now();
    let results: ToolResult[];
    if (this.config.maxParallelTools > 0 && toolCalls.length > this.config.maxParallelTools) {
      results = [];
      for (let i = 0; i < toolCalls.length; i += this.config.maxParallelTools) {
        const batch = toolCalls.slice(i, i + this.config.maxParallelTools);
        const batchResults = await Promise.all(batch.map(executeOne));
        results.push(...batchResults);
      }
    } else {
      results = await Promise.all(toolCalls.map(executeOne));
    }

    // 串行处理结果（保持消息顺序、更新统计、触发 after_tool hook）
    let toolCount = state.toolCount;
    let shouldStop = false;

    for (let i = 0; i < toolCalls.length; i++) {
      const tc = toolCalls[i];
      const result = results[i];
      toolCount++;

      events.push({ type: 'tool_result', result, turn });

      // 连续失败计数
      if (result.isError) {
        state.consecutiveFailures++;
        if (state.consecutiveFailures >= this.config.maxConsecutiveFailures) {
          shouldStop = true;
        }
      } else {
        state.consecutiveFailures = 0;
      }

      messages.push({
        role: 'tool',
        content: result.content,
        toolCallId: tc.id,
        name: tc.name,
      });

      // 异步记忆写入
      if (this.capCache.memoryStore) {
        const mem = this.capCache.memoryStore;
        state.pendingMemoryWrites.push(
          mem.add({
            id: `${this.config.name}-${this.config.sessionId}-${turn}-${tc.id}`,
            sessionId: this.config.sessionId,
            role: 'tool',
            content: result.content,
            metadata: { toolCallId: tc.id, toolName: tc.name },
            createdAt: new Date().toISOString(),
          }).catch(() => {}),
        );
      }

      // after_tool hook
      await this.hooks.fireHook('after_tool', {
        agentID: this.config.name,
        sessionID: this.config.sessionId,
        turn,
        toolResult: result,
      });
    }

    const totalToolLatency = _totalToolLatency + (Date.now() - toolBatchStart);
    state.totalToolLatency = totalToolLatency;
    state.toolCount = toolCount;

    return { results, totalToolLatency, toolCount, shouldStop, events };
  }

  // ===== Checkpoint 保存 =====

  /** 保存当前轮次的 checkpoint */
  async saveCheckpoint(turn: number, messages: Message[], finalResponse?: Response | null): Promise<void> {
    if (!this.capCache.checkpointStore) return;
    const checkpoint: Checkpoint = {
      id: `${this.config.name}-${this.config.sessionId}-${turn}`,
      sessionID: this.config.sessionId,
      turn,
      messages: [...messages],
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

  // ===== 消息裁剪 =====

  private trimMessages(messages: Message[]): void {
    if (messages.length <= this.config.maxMessages) return;
    const system = messages.filter((m) => m.role === 'system');
    const rest = messages.filter((m) => m.role !== 'system');
    const keep = this.config.maxMessages - system.length;
    messages.splice(0, messages.length, ...[...system, ...rest.slice(-keep)]);
  }
}
