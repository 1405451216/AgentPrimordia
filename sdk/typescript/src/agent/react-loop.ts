import type { Message, Response } from '../types.js';
import type { Provider } from '../llm/provider.js';
import type { ToolRegistry } from '../tools/registry.js';
import type { CostTracker, Checkpoint, CheckpointStore } from './request-id.js';
import type { Memory } from '../memory/store.js';
import type { OTelBridgeLike } from '../metrics/otel-extended.js';
import { validateAgentInput } from '../validate.js';
import type { AgentSelfTuner, RunMetrics, TuningSuggestion } from './self-tuning.js';
import type { SpeculativeExecutor } from './speculative-exec.js';
import type { EnhancedToolLearner } from './tool-learning.js';
import type { Planner, SubTask } from './planning.js';
import type { Reflector, Severity } from './reflection.js';
import type { GuardrailEngine } from '../security/guardrails.js';
import type { FailurePhase, FailureRecord, FailureStore } from './failure.js';

// 从拆分模块重新导出，保持向后兼容
export type { StreamEvent, HookPoint, HookContext, HookFunc } from './hooks.js';
export { HookManager } from './hooks.js';
export { Lifecycle } from './lifecycle.js';

import type { StreamEvent } from './hooks.js';
import { HookManager } from './hooks.js';
import { Lifecycle } from './lifecycle.js';

import { TurnExecutor } from './turn-executor.js';
import type { CapabilitiesCache, TurnState } from './turn-executor.js';

export interface RunOptions {
  signal?: AbortSignal;
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
  costTracker?: CostTracker;
  memoryStore?: Memory;
  checkpointStore?: CheckpointStore;
  otelBridge?: OTelBridgeLike;
  parallelToolExecution?: boolean;
  maxParallelTools?: number;
  speculativeExecutor?: SpeculativeExecutor;
  selfTuner?: AgentSelfTuner;
  enhancedToolLearner?: EnhancedToolLearner;
  autoTune?: boolean;
  /** 任务规划器：run 入口将目标分解为子任务 DAG 依次执行（>1 子任务才启用） */
  planner?: Planner;
  /** 自反思器：完成路径对输出批评，severity 达到阈值时改写 */
  reflector?: Reflector;
  /** 触发 improve 改写的最低严重度，默认 high */
  reflectionSeverityThreshold?: Severity;
  /** 护栏引擎（v3.4-5）：入口对用户输入脱敏/拒绝，LLM 输出逐轮检查（对齐 Go InputGuard/OutputGuard） */
  guardrail?: GuardrailEngine;
  /** 失败记录存储（v3.4-6d）：运行失败自动落盘完整上下文，支持一键重放（对齐 Go WithFailureStore） */
  failureStore?: FailureStore;
}

export type { CapabilitiesCache };

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
  private costTracker?: CostTracker;
  private memoryStore?: Memory;
  private checkpointStore?: CheckpointStore;
  private messages: Message[] = [];
  private capCache: CapabilitiesCache | null = null;
  private pendingMemoryWrites: Promise<void>[] = [];
  private otelBridge?: OTelBridgeLike;
  private runMu: Promise<void> = Promise.resolve();
  private parallelToolExecution: boolean;
  private maxParallelTools: number;
  private gracefulShutdownFlag = false;
  private speculativeExecutor?: SpeculativeExecutor;
  private selfTuner?: AgentSelfTuner;
  private enhancedToolLearner?: EnhancedToolLearner;
  private autoTune: boolean;
  private lastRunMetrics?: RunMetrics;
  private planner?: Planner;
  private reflector?: Reflector;
  private reflectionSeverityThreshold: Severity;
  private guardrail?: GuardrailEngine;
  private failureStore?: FailureStore;
  private currentTurn = 0;
  private currentSubtaskId?: string;
  private failureSeq = 0;

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
    this.maxTurns = config.maxTurns ?? 50;
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
    this.maxParallelTools = config.maxParallelTools ?? 0;
    this.speculativeExecutor = config.speculativeExecutor;
    this.selfTuner = config.selfTuner;
    this.enhancedToolLearner = config.enhancedToolLearner;
    this.autoTune = config.autoTune ?? true;
    this.planner = config.planner;
    this.reflector = config.reflector;
    this.reflectionSeverityThreshold = config.reflectionSeverityThreshold ?? 'high';
    this.guardrail = config.guardrail;
    this.failureStore = config.failureStore;
  }

  requestGracefulShutdown(): void {
    this.gracefulShutdownFlag = true;
  }

  isGracefulShutdownRequested(): boolean {
    return this.gracefulShutdownFlag;
  }

  private resolveCapabilities(): CapabilitiesCache {
    return {
      costTracker: this.costTracker ?? null,
      memoryStore: this.memoryStore ?? null,
      checkpointStore: this.checkpointStore ?? null,
      otelBridge: this.otelBridge ?? null,
    };
  }

  private async flushMemoryWriter(): Promise<void> {
    if (this.pendingMemoryWrites.length === 0) return;
    await Promise.allSettled(this.pendingMemoryWrites);
    this.pendingMemoryWrites = [];
  }

  private checkSignal(signal?: AbortSignal): void {
    if (signal?.aborted) {
      throw new DOMException('Agent run aborted', 'AbortError');
    }
  }

  private async publishEvent(eventType: string, payload: Record<string, unknown>): Promise<void> {
    if (this.hooks.hasSubscriber('on_metrics_collect')) {
      await this.hooks.fireHook('on_metrics_collect', {
        agentID: this.name,
        sessionID: this.sessionId,
        turn: 0,
        metadata: { eventType, ...payload },
      });
    }
  }

  async run(input: string, options?: RunOptions): Promise<Response> {
    validateAgentInput(input);
    const runRelease = await this.acquireRunLock();
    try {
      return await this.runEngine(input, options);
    } finally {
      runRelease();
    }
  }

  async streamRun(input: string, options?: RunOptions): Promise<Response> {
    let finalResponse: Response | undefined;
    for await (const event of this.streamEvents(input, options)) {
      if (event.type === 'done' && event.response) {
        finalResponse = event.response;
      }
    }
    if (finalResponse) return finalResponse;
    return this.run(input, options);
  }

  private async acquireRunLock(): Promise<() => void> {
    const oldMu = this.runMu;
    let release!: () => void;
    this.runMu = new Promise<void>((resolve) => { release = resolve; });
    await oldMu;
    return release;
  }

  private async runEngine(input: string, options?: RunOptions): Promise<Response> {
    this.lifecycle.setStatus('running');
    const startTime = Date.now();
    this.consecutiveFailures = 0;
    this.capCache = this.resolveCapabilities();
    const runSpanId = this.capCache.otelBridge?.startSpan('agent.run', {
      'agent.name': this.name,
      'agent.session_id': this.sessionId,
    });

    this.messages = [];
    if (this.systemPrompt) {
      this.messages.push({ role: 'system', content: this.systemPrompt });
    }

    if (this.enhancedToolLearner && this.toolkit.size() > 0) {
      try {
        const toolNames = this.toolkit.list().map((t) => t.name);
        const guide = await this.enhancedToolLearner.generateUsageGuide(toolNames);
        if (guide && this.messages.length > 0 && this.messages[0]!.role === 'system') {
          this.messages[0]!.content += '\n\n' + guide;
        }
      } catch {}
    }

    if (this.selfTuner && this.autoTune && this.lastRunMetrics) {
      const suggestion = this.selfTuner.getSuggestion();
      if (suggestion.shouldAdjust) {
        this.applyTuningSuggestion(suggestion);
      }
    }

    // v3.4-5：输入端护栏——用户输入进入循环前检查（脱敏或拒绝，对齐 Go InputGuard）
    if (this.guardrail) {
      const gi = this.guardrail.checkInput(input);
      if (!gi.passed) {
        this.lifecycle.setStatus('error');
        this.capCache.otelBridge?.endSpan(runSpanId!, 'error');
        return {
          content: 'input blocked by guardrail',
          metrics: { totalTurns: 0, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
        };
      }
      input = gi.modifiedInput;
    }

    this.messages.push({ role: 'user', content: input });

    await this.hooks.fireHook('before_run', {
      agentID: this.name, sessionID: this.sessionId, turn: 0,
    });

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
      // 计划分支：Planner 分解出 >1 子任务时走 DAG 执行，否则降级普通循环
      let response: Response | undefined;
      if (this.planner) {
        let subtasks: SubTask[] = [];
        try {
          subtasks = await this.planner.decompose(input);
        } catch {
          // 规划失败回退普通 ReAct 循环
        }
        if (subtasks.length > 1) {
          response = await this.executePlan(subtasks, startTime, options);
        }
      }

      if (!response) {
        response = await this.runLoop(startTime, 0, options);
        // 完成路径自反思：批评最终输出，必要时改写
        if (this.reflector) {
          response.content = this.guardOutput(await this.reflectAndImprove(response.content));
        }
      }

      await this.hooks.fireHook('after_run', {
        agentID: this.name, sessionID: this.sessionId, turn: 0, response,
      });

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
        agentID: this.name, sessionID: this.sessionId, turn: 0, error,
      });
      // v3.4-6d：失败自动落盘（取消不记录），内嵌最近 checkpoint 供一键重放
      await this.recordFailure(input, error);
      if (error.name === 'AbortError') {
        return {
          content: 'Agent run cancelled',
          metrics: { totalTurns: 0, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
        };
      }
      return {
        content: `Agent error: ${error.message}`,
        metrics: { totalTurns: 0, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
      };
    } finally {
      await this.flushMemoryWriter();
      this.capCache = null;
      this.currentSubtaskId = undefined;
    }
  }

  private async runLoop(startTime: number, startTurn: number, options?: RunOptions): Promise<Response> {
    const executor = this.createTurnExecutor();
    const state: TurnState = {
      consecutiveFailures: 0, totalLLMLatency: 0, totalToolLatency: 0, toolCount: 0,
      pendingMemoryWrites: this.pendingMemoryWrites,
    };

    let turn: number;
    for (turn = startTurn; turn < this.maxTurns; turn++) {
      this.currentTurn = turn;
      this.checkSignal(options?.signal);
      if (this.lifecycle.isStopped()) break;

      const result = await executor.executeTurn(this.messages, turn, state, startTime);

      if (result.shouldStop) {
        this.lifecycle.setStatus('completed');
        return result.response!;
      }
    }

    const duration = Date.now() - startTime;
    this.lifecycle.setStatus('completed');
    return {
      content: this.messages[this.messages.length - 1]?.content ?? '',
      metrics: {
        totalTurns: turn,
        totalTools: state.toolCount,
        duration,
        llmLatency: state.totalLLMLatency,
        toolLatency: state.totalToolLatency,
      },
    };
  }

  // ===== 计划与自反思 =====

  /** 严重度排序，用于阈值比较 */
  private severityRank(s: Severity): number {
    switch (s) {
      case 'low': return 0;
      case 'medium': return 1;
      case 'high': return 2;
      case 'critical': return 3;
      default: return 1;
    }
  }

  /** v3.4-5：输出端护栏对改写后的内容复检（阻断时抛错由 runEngine 统一处理） */
  private guardOutput(content: string): string {
    if (!this.guardrail || !content) return content;
    const result = this.guardrail.checkOutput(content);
    if (!result.passed) {
      throw new Error('output blocked by guardrail');
    }
    return result.modifiedOutput;
  }

  /** 完成路径批评输出；severity ≥ 阈值时调用 improve 改写 */
  private async reflectAndImprove(content: string): Promise<string> {
    if (!this.reflector) return content;
    try {
      const critique = await this.reflector.critique(content);
      if (this.severityRank(critique.severity) >= this.severityRank(this.reflectionSeverityThreshold)) {
        return await this.reflector.improve(content, critique);
      }
    } catch {
      // 反思失败不影响主流程
    }
    return content;
  }

  /** 子任务拓扑分层（Kahn）；遇环兜底按原顺序执行剩余子任务 */
  private topoLayers(subtasks: SubTask[]): SubTask[][] {
    const ids = new Set(subtasks.map((t) => t.id));
    const done = new Set<string>();
    const layers: SubTask[][] = [];
    let remaining = [...subtasks];
    while (remaining.length > 0) {
      const ready = remaining.filter((t) => t.dependsOn.every((d) => done.has(d) || !ids.has(d)));
      if (ready.length === 0) {
        layers.push(remaining);
        break;
      }
      layers.push(ready);
      for (const t of ready) done.add(t.id);
      remaining = remaining.filter((t) => !done.has(t.id));
    }
    return layers;
  }

  /** 按计划分层执行子任务；同层内顺序执行，保证队列式 Mock 的确定性 */
  private async executePlan(subtasks: SubTask[], startTime: number, options?: RunOptions): Promise<Response> {
    const results = new Map<string, string>();
    let totalTurns = 0;
    let totalTools = 0;
    let llmLatency = 0;
    let toolLatency = 0;
    let finalContent = '';

    outer: for (const layer of this.topoLayers(subtasks)) {
      for (const task of layer) {
        this.checkSignal(options?.signal);
        if (this.lifecycle.isStopped()) break outer;
        task.status = 'running';
        // v3.4-6d：记录当前子任务，失败时可定位到 plan 阶段的具体子任务
        this.currentSubtaskId = task.id;

        // 组装子任务上下文：依赖结果 + 当前描述
        const context = task.dependsOn
          .map((d) => results.get(d))
          .filter((v): v is string => !!v)
          .join('\n');
        const subInput = context
          ? `前置子任务结果：\n${context}\n\n执行当前子任务：${task.description}`
          : `执行当前子任务：${task.description}`;

        // 子任务独立重建消息上下文，各自走完整 ReAct 循环
        this.messages = [];
        if (this.systemPrompt) {
          this.messages.push({ role: 'system', content: this.systemPrompt });
        }
        this.messages.push({ role: 'user', content: subInput });

        const resp = await this.runLoop(startTime, 0, options);
        let content = resp.content;
        if (this.reflector) {
          content = this.guardOutput(await this.reflectAndImprove(content));
        }

        task.status = 'completed';
        task.result = content;
        results.set(task.id, content);
        finalContent = content;
        totalTurns += resp.metrics.totalTurns;
        totalTools += resp.metrics.totalTools;
        llmLatency += resp.metrics.llmLatency;
        toolLatency += resp.metrics.toolLatency;
      }
    }

    this.lifecycle.setStatus('completed');
    return {
      content: finalContent,
      metrics: {
        totalTurns,
        totalTools,
        duration: Date.now() - startTime,
        llmLatency,
        toolLatency,
      },
    };
  }

  async resumeFromCheckpoint(checkpoint: Checkpoint, options?: RunOptions): Promise<Response> {
    const runRelease = await this.acquireRunLock();
    try {
      this.lifecycle.setStatus('running');
      const startTime = Date.now();
      this.consecutiveFailures = 0;
      this.capCache = this.resolveCapabilities();

      const runSpanId = this.capCache.otelBridge?.startSpan('agent.resume', {
        'agent.name': this.name, 'agent.session_id': this.sessionId, 'agent.resume_turn': checkpoint.turn,
      });

      this.messages = [...checkpoint.messages];
      this.checkSignal(options?.signal);

      await this.hooks.fireHook('before_run', {
        agentID: this.name, sessionID: this.sessionId, turn: checkpoint.turn,
      });

      try {
        const response = await this.runLoop(startTime, checkpoint.turn, options);
        await this.hooks.fireHook('after_run', {
          agentID: this.name, sessionID: this.sessionId, turn: checkpoint.turn, response,
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
          agentID: this.name, sessionID: this.sessionId, turn: checkpoint.turn, error,
        });
        return {
          content: `Agent error: ${error.message}`,
          metrics: { totalTurns: checkpoint.turn, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
        };
      } finally {
        await this.flushMemoryWriter();
        this.capCache = null;
      }
    } finally {
      runRelease();
    }
  }

  // ===== 失败记录与一键重放（v3.4-6d） =====

  /** 失败自动落盘：跳过取消（AbortError）；内嵌最近 checkpoint 供重放（对齐 Go recordFailure） */
  private async recordFailure(input: string, error: Error): Promise<void> {
    if (!this.failureStore || error.name === 'AbortError') return;
    try {
      let state: Checkpoint | undefined;
      const cpStore = this.capCache?.checkpointStore;
      if (cpStore) {
        const cps = await cpStore.list(this.sessionId);
        if (cps.length > 0) state = cps[cps.length - 1];
      }
      const phase: FailurePhase = this.currentSubtaskId ? 'plan' : 'run';
      const rec: FailureRecord = {
        id: `fail-${Date.now()}-${this.failureSeq++}`,
        agentId: this.name,
        sessionId: this.sessionId,
        phase,
        error: error.message,
        turn: this.currentTurn,
        input,
        createdAt: new Date().toISOString(),
      };
      if (this.currentSubtaskId) rec.subtaskId = this.currentSubtaskId;
      if (state) rec.state = state;
      await this.failureStore.record(rec);
    } catch {
      // 失败记录本身不影响主流程
    }
  }

  /** 一键重放：从失败记录内嵌的 checkpoint 恢复运行（对齐 Go ReplayFailure） */
  async replayFailure(failureID: string, options?: RunOptions): Promise<Response> {
    if (!this.failureStore) throw new Error('no failure store configured');
    const rec = await this.failureStore.get(failureID);
    if (!rec) throw new Error(`failure record not found: ${failureID}`);
    if (!rec.state) throw new Error(`failure record has no embedded checkpoint: ${failureID}`);
    return this.resumeFromCheckpoint(rec.state, options);
  }

  async *stream(input: string, options?: RunOptions): AsyncIterable<string> {
    for await (const event of this.streamEvents(input, options)) {
      if (event.type === 'token' && event.content) {
        yield event.content;
      }
    }
  }

  async *streamEvents(input: string, options?: RunOptions): AsyncIterable<StreamEvent> {
    validateAgentInput(input);
    this.lifecycle.setStatus('running');
    const startTime = Date.now();
    this.consecutiveFailures = 0;
    this.capCache = this.resolveCapabilities();

    this.messages = [];
    if (this.systemPrompt) {
      this.messages.push({ role: 'system', content: this.systemPrompt });
    }

    if (this.enhancedToolLearner && this.toolkit.size() > 0) {
      try {
        const toolNames = this.toolkit.list().map((t) => t.name);
        const guide = await this.enhancedToolLearner.generateUsageGuide(toolNames);
        if (guide && this.messages.length > 0 && this.messages[0]!.role === 'system') {
          this.messages[0]!.content += '\n\n' + guide;
        }
      } catch {}
    }

    if (this.selfTuner && this.autoTune && this.lastRunMetrics) {
      const suggestion = this.selfTuner.getSuggestion();
      if (suggestion.shouldAdjust) {
        this.applyTuningSuggestion(suggestion);
      }
    }

    // v3.4-5：流式入口同样执行输入端护栏
    if (this.guardrail) {
      const gi = this.guardrail.checkInput(input);
      if (!gi.passed) {
        this.lifecycle.setStatus('error');
        yield {
          type: 'done',
          response: {
            content: 'input blocked by guardrail',
            metrics: { totalTurns: 0, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
          },
        };
        return;
      }
      input = gi.modifiedInput;
    }

    this.messages.push({ role: 'user', content: input });

    await this.hooks.fireHook('before_run', {
      agentID: this.name, sessionID: this.sessionId, turn: 0,
    });

    try {
      const hasTools = this.toolkit.size() > 0;

      if (!hasTools) {
        yield* this.streamWithoutTools(startTime);
        return;
      }

      yield* this.streamWithTools(startTime, options);
    } catch (err) {
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
            metrics: { totalTurns: 0, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
          },
        };
      } else {
        yield { type: 'error', error };
      }
    } finally {
      await this.flushMemoryWriter();
      this.capCache = null;
    }
  }

  private async *streamWithoutTools(startTime: number): AsyncIterable<StreamEvent> {
    if (this.model.stream) {
      let fullContent = '';
      for await (const chunk of this.model.stream({ messages: this.messages })) {
        if (chunk.content) {
          fullContent += chunk.content;
          yield { type: 'token', content: chunk.content };
        }
        if (chunk.done) break;
      }
      this.messages.push({ role: 'assistant', content: fullContent });
      this.lifecycle.setStatus('completed');
      yield {
        type: 'done',
        response: {
          content: fullContent,
          metrics: { totalTurns: 1, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
        },
      };
    } else {
      const resp = await this.model.complete({ messages: this.messages });
      this.messages.push({ role: 'assistant', content: resp.content });
      yield { type: 'token', content: resp.content };
      this.lifecycle.setStatus('completed');
      yield {
        type: 'done',
        response: {
          content: resp.content,
          metrics: { totalTurns: 1, totalTools: 0, duration: Date.now() - startTime, llmLatency: 0, toolLatency: 0 },
        },
      };
    }
  }

  private async *streamWithTools(startTime: number, options?: RunOptions): AsyncIterable<StreamEvent> {
    const executor = this.createTurnExecutor();
    const state: TurnState = {
      consecutiveFailures: 0, totalLLMLatency: 0, totalToolLatency: 0, toolCount: 0,
      pendingMemoryWrites: this.pendingMemoryWrites,
    };

    for (let turn = 0; turn < this.maxTurns; turn++) {
      this.checkSignal(options?.signal);
      if (this.lifecycle.isStopped()) break;

      const result = await executor.executeTurn(this.messages, turn, state, startTime);

      for (const ev of result.events) {
        yield ev;
      }

      if (result.shouldStop) {
        this.lifecycle.setStatus('completed');
        yield { type: 'done', response: result.response! };
        return;
      }

      yield { type: 'turn_end', turn };
    }

    this.lifecycle.setStatus('completed');
    yield {
      type: 'done',
      response: {
        content: this.messages[this.messages.length - 1]?.content ?? '',
        metrics: {
          totalTurns: this.maxTurns,
          totalTools: state.toolCount,
          duration: Date.now() - startTime,
          llmLatency: state.totalLLMLatency,
          toolLatency: state.totalToolLatency,
        },
      },
    };
  }

  private createTurnExecutor(): TurnExecutor {
    return new TurnExecutor(
      this.model, this.toolkit, this.hooks, this.capCache!,
      {
        name: this.name, sessionId: this.sessionId,
        maxConsecutiveFailures: this.maxConsecutiveFailures,
        parallelToolExecution: this.parallelToolExecution,
        maxParallelTools: this.maxParallelTools,
        maxMessages: this.maxMessages,
        isGracefulShutdownRequested: () => this.gracefulShutdownFlag,
        // v3.4-5：输出端护栏逐轮检查（对齐 Go OutputGuard）
        outputGuard: this.guardrail ? (content) => {
          const result = this.guardrail!.checkOutput(content);
          return { passed: result.passed, modified: result.modifiedOutput };
        } : undefined,
      },
    );
  }

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
