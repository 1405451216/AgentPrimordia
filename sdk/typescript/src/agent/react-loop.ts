import type { Message, Response } from '../types.js';
import type { Provider } from '../llm/provider.js';
import type { ToolRegistry } from '../tools/registry.js';
import type { CostTracker, Checkpoint, CheckpointStore } from './request-id.js';
import type { Memory } from '../memory/store.js';
import type { OTelBridge } from '../metrics/otel-extended.js';
import { validateAgentInput } from '../validate.js';
import type { AgentSelfTuner, RunMetrics, TuningSuggestion } from './self-tuning.js';
import type { SpeculativeExecutor } from './speculative-exec.js';
import type { EnhancedToolLearner } from './tool-learning.js';

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
  otelBridge?: OTelBridge;
  parallelToolExecution?: boolean;
  maxParallelTools?: number;
  speculativeExecutor?: SpeculativeExecutor;
  selfTuner?: AgentSelfTuner;
  enhancedToolLearner?: EnhancedToolLearner;
  autoTune?: boolean;
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
  private otelBridge?: OTelBridge;
  private runMu: Promise<void> = Promise.resolve();
  private parallelToolExecution: boolean;
  private maxParallelTools: number;
  private gracefulShutdownFlag = false;
  private speculativeExecutor?: SpeculativeExecutor;
  private selfTuner?: AgentSelfTuner;
  private enhancedToolLearner?: EnhancedToolLearner;
  private autoTune: boolean;
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
    this.maxParallelTools = config.maxParallelTools ?? 0;
    this.speculativeExecutor = config.speculativeExecutor;
    this.selfTuner = config.selfTuner;
    this.enhancedToolLearner = config.enhancedToolLearner;
    this.autoTune = config.autoTune ?? true;
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
      const response = await this.runLoop(startTime, 0, options);

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
