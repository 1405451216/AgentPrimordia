import type { Message, ToolCall, Response, AgentMetrics, AgentStatus, ToolResult } from '../types.js';
import type { Provider } from '../llm/provider.js';
import type { ToolRegistry } from '../tools/registry.js';
import { validateAgentInput, requirePositiveInt, requireNonEmpty } from '../validate.js';

// ===== Stream Event Types =====

export type StreamEvent =
  | { type: 'token'; content: string }
  | { type: 'tool_call'; toolCall: ToolCall; turn: number }
  | { type: 'tool_result'; result: ToolResult; turn: number }
  | { type: 'turn_end'; turn: number }
  | { type: 'done'; response: Response }
  | { type: 'error'; error: Error };

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

export type HookFunc = (ctx: HookContext) => Promise<void> | void;

export class HookManager {
  private hooks: Map<HookPoint, HookFunc[]> = new Map();

  register(point: HookPoint, fn: HookFunc): void {
    if (!this.hooks.has(point)) {
      this.hooks.set(point, []);
    }
    this.hooks.get(point)!.push(fn);
  }

  async fire(ctx: HookContext): Promise<void> {
    const fns = this.hooks.get(ctx.point) ?? [];
    for (const fn of fns) {
      await fn(ctx);
    }
  }

  remove(point: HookPoint): void {
    this.hooks.delete(point);
  }

  clear(): void {
    this.hooks.clear();
  }

  count(point: HookPoint): number {
    return this.hooks.get(point)?.length ?? 0;
  }
}

export class Lifecycle {
  private _status: AgentStatus = 'idle';
  private stopped = false;
  private stopResolvers: (() => void)[] = [];
  private pauseResolvers: (() => void)[] = [];
  private resumeResolvers: (() => void)[] = [];
  private paused = false;

  get status(): AgentStatus {
    return this._status;
  }

  setStatus(s: AgentStatus): void {
    this._status = s;
  }

  stop(): void {
    this.stopped = true;
    for (const r of this.stopResolvers) r();
    this.stopResolvers = [];
  }

  isStopped(): boolean {
    return this.stopped;
  }

  onStop(): Promise<void> {
    if (this.stopped) return Promise.resolve();
    return new Promise((r) => this.stopResolvers.push(r));
  }

  pause(): void {
    if (this._status !== 'running') return;
    this._status = 'paused';
    this.paused = true;
    for (const r of this.pauseResolvers) r();
    this.pauseResolvers = [];
  }

  resume(): void {
    if (this._status !== 'paused') return;
    this._status = 'running';
    this.paused = false;
    for (const r of this.resumeResolvers) r();
    this.resumeResolvers = [];
  }

  waitPause(): Promise<void> {
    if (this.paused) return Promise.resolve();
    return new Promise((r) => this.pauseResolvers.push(r));
  }

  waitResume(): Promise<void> {
    if (!this.paused) return Promise.resolve();
    return new Promise((r) => this.resumeResolvers.push(r));
  }
}

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
