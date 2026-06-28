import type { Message, ToolCall, Response, AgentMetrics, AgentStatus, ToolResult } from '../types.js';
import type { Provider } from '../llm/provider.js';
import type { ToolRegistry } from '../tools/registry.js';
import { validateAgentInput, requirePositiveInt, requireNonEmpty } from '../validate.js';

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

  /** 注册钩子函数到指定触发点 */
  register(point: HookPoint, fn: HookFunc): void {
    if (!this.hooks.has(point)) {
      this.hooks.set(point, []);
    }
    this.hooks.get(point)!.push(fn);
  }

  /** 触发指定钩子点的所有注册函数 */
  async fire(ctx: HookContext): Promise<void> {
    const fns = this.hooks.get(ctx.point) ?? [];
    for (const fn of fns) {
      await fn(ctx);
    }
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
  }

  /** 执行 ReAct 循环，返回最终响应。
 *
 * 参数：
 * - input: 用户输入文本
 *
 * 返回：包含最终内容和指标的 Response 对象
 */
  async run(input: string): Promise<Response> {
    validateAgentInput(input);
    this.lifecycle.setStatus('running');
    const startTime = Date.now();
    let totalLLMLatency = 0;
    let totalToolLatency = 0;
    let toolCount = 0;
    this.consecutiveFailures = 0;

    this.messages = [];
    if (this.systemPrompt) {
      this.messages.push({ role: 'system', content: this.systemPrompt });
    }
    this.messages.push({ role: 'user', content: input });

    await this.hooks.fire({
      agentID: this.name,
      sessionID: this.sessionId,
      point: 'before_run',
      turn: 0,
    });

    let turn = 0;
    for (; turn < this.maxTurns; turn++) {
      if (this.lifecycle.isStopped()) break;

      await this.hooks.fire({
        agentID: this.name,
        sessionID: this.sessionId,
        point: 'before_turn',
        turn,
      });

      this.trimMessages();

      const llmStart = Date.now();
      const thought = await this.callLLM();
      totalLLMLatency += Date.now() - llmStart;

      await this.hooks.fire({
        agentID: this.name,
        sessionID: this.sessionId,
        point: 'after_llm',
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

        this.lifecycle.setStatus('completed');
        await this.hooks.fire({
          agentID: this.name,
          sessionID: this.sessionId,
          point: 'on_complete',
          turn,
          response,
        });
        await this.hooks.fire({
          agentID: this.name,
          sessionID: this.sessionId,
          point: 'after_turn',
          turn,
        });

        return response;
      }

      for (const tc of thought.toolCalls) {
        await this.hooks.fire({
          agentID: this.name,
          sessionID: this.sessionId,
          point: 'before_tool',
          turn,
          toolCall: tc,
        });

        const toolStart = Date.now();
        const result = await this.toolkit.execute(tc);
        totalToolLatency += Date.now() - toolStart;
        toolCount++;

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

        await this.hooks.fire({
          agentID: this.name,
          sessionID: this.sessionId,
          point: 'after_tool',
          turn,
          toolResult: result,
        });
      }

      await this.hooks.fire({
        agentID: this.name,
        sessionID: this.sessionId,
        point: 'after_turn',
        turn,
      });
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

  /**
   * Stream the agent's response as text tokens.
   * 
   * - When no tools are registered: streams LLM tokens directly (true token-by-token).
   * - When tools are registered: runs the full ReAct loop, yielding content from each turn.
   *   Tool results are NOT yielded (use streamEvents() for structured events).
   */
  async *stream(input: string): AsyncIterable<string> {
    for await (const event of this.streamEvents(input)) {
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
  async *streamEvents(input: string): AsyncIterable<StreamEvent> {
    validateAgentInput(input);
    this.lifecycle.setStatus('running');
    const startTime = Date.now();
    let totalLLMLatency = 0;
    let totalToolLatency = 0;
    let toolCount = 0;
    this.consecutiveFailures = 0;

    this.messages = [];
    if (this.systemPrompt) {
      this.messages.push({ role: 'system', content: this.systemPrompt });
    }
    this.messages.push({ role: 'user', content: input });

    await this.hooks.fire({
      agentID: this.name, sessionID: this.sessionId, point: 'before_run', turn: 0,
    });

    const hasTools = this.toolkit.size() > 0;

    let turn = 0;
    for (; turn < this.maxTurns; turn++) {
      if (this.lifecycle.isStopped()) break;

      await this.hooks.fire({
        agentID: this.name, sessionID: this.sessionId, point: 'before_turn', turn,
      });

      this.trimMessages();

      const llmStart = Date.now();

      if (!hasTools) {
        // No tools: stream directly from LLM if supported
        if (this.model.stream) {
          let fullContent = '';
          for await (const chunk of this.model.stream({ messages: this.messages })) {
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

      await this.hooks.fire({
        agentID: this.name, sessionID: this.sessionId, point: 'after_llm', turn,
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

        await this.hooks.fire({
          agentID: this.name, sessionID: this.sessionId,
          point: 'on_complete', turn, response,
        });
        await this.hooks.fire({
          agentID: this.name, sessionID: this.sessionId, point: 'after_turn', turn,
        });

        yield { type: 'done', response };
        return;
      }

      // Execute tool calls
      for (const tc of resp.toolCalls) {
        await this.hooks.fire({
          agentID: this.name, sessionID: this.sessionId,
          point: 'before_tool', turn, toolCall: tc,
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

        await this.hooks.fire({
          agentID: this.name, sessionID: this.sessionId,
          point: 'after_tool', turn, toolResult: result,
        });
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
}
