import { ReActAgent, HookManager, Lifecycle } from './react-loop.js';
import type { ReActConfig } from './react-loop.js';
import type { Provider } from '../llm/provider.js';
import { ToolRegistry } from '../tools/registry.js';
import type { Memory } from '../memory/store.js';
import type { Message, Response } from '../types.js';
import { PromptTemplate } from './prompt-template.js';
import {
  type CheckpointStore,
  type ContextWindowStrategy,
  type HITLManager,
  type CostTracker,
  InMemoryCheckpointStore,
  KeepLastNStrategy,
} from './request-id.js';
import { Session } from './session.js';

// ===== Capability Interfaces =====

export interface MemoryCapable { getMemory(): Memory | undefined; }
export interface RAGCapable { getRAGProvider(): RAGProvider | undefined; }
export interface HookCapable { getHooks(): HookManager; }
export interface TraceCapable { getTracer(): Tracer | undefined; }
export interface CostCapable { getCostTracker(): CostTracker | undefined; }
export interface CheckpointCapable { getCheckpointStore(): CheckpointStore | undefined; }
export interface HITLCapable { getHITLManager(): HITLManager | undefined; }
export interface ContextWindowCapable { getContextWindowStrategy(): ContextWindowStrategy | undefined; }

// ===== Tracer =====

export type SpanKind = 'internal' | 'client' | 'server';

export interface Span {
  spanContext(): string;
  setAttribute(key: string, value: unknown): void;
  setAttributes(attrs: Record<string, unknown>): void;
  setStatus(status: 'ok' | 'error', description?: string): void;
  end(): void;
}

export interface Tracer {
  start(name: string, kind?: SpanKind, opts?: { parent?: string; attributes?: Record<string, unknown> }): Span;
}

export class NoopTracer implements Tracer {
  start(_name: string, _kind?: SpanKind, _opts?: { parent?: string; attributes?: Record<string, unknown> }): Span {
    return new NoopSpan();
  }
}

class NoopSpan implements Span {
  spanContext(): string { return ''; }
  setAttribute(_key: string, _value: unknown): void {}
  setAttributes(_attrs: Record<string, unknown>): void {}
  setStatus(_status: 'ok' | 'error', _description?: string): void {}
  end(): void {}
}

// ===== RAG Provider =====

export interface RAGDocument {
  id: string;
  content: string;
  score: number;
  source?: string;
  role?: string;
}

export type RAGMode = 'auto' | 'first' | 'on_demand';

export interface RAGConfig {
  provider: RAGProvider;
  mode?: RAGMode;
  topK?: number;
  minScore?: number;
  contextTemplate?: string;
}

export interface RAGProvider {
  search(query: string, topK: number): Promise<RAGDocument[]>;
}

// ===== Capability Agent =====

export interface AgentOption {
  name: string;
  systemPrompt?: string;
  model: Provider;
  maxTurns?: number;
  temperature?: number;
  sessionId?: string;
}

/**
 * CapabilityAgent is a composable Agent wrapper.
 * Inject capabilities via chainable WithXxx() methods.
 */
export class CapabilityAgent {
  private agent: ReActAgent;
  private _memory?: Memory;
  private _hooks?: HookManager;
  private _lifecycle: Lifecycle;
  private _toolkit?: ToolRegistry;
  private _tracer?: Tracer;
  private _costTracker?: CostTracker;
  private _checkpointStore?: CheckpointStore;
  private _contextWindow?: ContextWindowStrategy;
  private _hitl?: HITLManager;
  private _rag?: RAGConfig;
  private _promptTemplate?: PromptTemplate;
  private _maxMessages: number;

  constructor(opts: AgentOption) {
    if (!opts.name?.trim()) throw new Error('Agent name is required');
    if (!opts.model) throw new Error('Model provider is required');

    this._lifecycle = new Lifecycle();
    this._toolkit = new ToolRegistry();
    this._maxMessages = opts.maxTurns ? opts.maxTurns * 8 : 80;

    this.agent = new ReActAgent({
      name: opts.name,
      model: opts.model,
      toolkit: this._toolkit,
      maxTurns: opts.maxTurns ?? 10,
      systemPrompt: opts.systemPrompt ?? '',
      sessionId: opts.sessionId ?? '',
      hooks: this._hooks,
      lifecycle: this._lifecycle,
      maxMessages: this._maxMessages,
    });
  }

  // ===== Chainable capability injectors =====

  withMemory(memory: Memory): CapabilityAgent {
    this._memory = memory;
    return this;
  }

  withToolkit(toolkit: ToolRegistry): CapabilityAgent {
    this._toolkit = toolkit;
    // Re-create agent with new toolkit
    this.rebuildAgent();
    return this;
  }

  withHooks(hooks: HookManager): CapabilityAgent {
    this._hooks = hooks;
    this.rebuildAgent();
    return this;
  }

  withRAG(rag: RAGConfig): CapabilityAgent {
    this._rag = rag;
    return this;
  }

  withTracer(tracer: Tracer): CapabilityAgent {
    this._tracer = tracer;
    return this;
  }

  withCostTracker(tracker: CostTracker): CapabilityAgent {
    this._costTracker = tracker;
    return this;
  }

  withCheckpointStore(store: CheckpointStore): CapabilityAgent {
    this._checkpointStore = store;
    return this;
  }

  withContextWindow(strategy: ContextWindowStrategy): CapabilityAgent {
    this._contextWindow = strategy;
    return this;
  }

  withHITL(hitl: HITLManager): CapabilityAgent {
    this._hitl = hitl;
    return this;
  }

  withPromptTemplate(template: PromptTemplate): CapabilityAgent {
    this._promptTemplate = template;
    return this;
  }

  withMaxTurns(maxTurns: number): CapabilityAgent {
    this._maxMessages = maxTurns * 8;
    this.rebuildAgent();
    return this;
  }

  private rebuildAgent(): void {
    this.agent = new ReActAgent({
      name: this.agent.name,
      model: (this.agent as unknown as { model: Provider }).model,
      toolkit: this._toolkit ?? new ToolRegistry(),
      maxTurns: (this.agent as unknown as { maxTurns: number }).maxTurns,
      systemPrompt: (this.agent as unknown as { systemPrompt: string }).systemPrompt,
      sessionId: (this.agent as unknown as { sessionId: string }).sessionId,
      hooks: this._hooks,
      lifecycle: this._lifecycle,
      maxMessages: this._maxMessages,
    });
  }

  // ===== Run =====

  async run(input: string): Promise<Response> {
    // Check cost budget
    if (this._costTracker?.checkBudget()) {
      return {
        content: 'Agent stopped: cost budget exceeded',
        metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 },
      };
    }

    // Inject RAG context if configured
    let fullInput = input;
    if (this._rag && this._rag.mode !== 'on_demand') {
      const docs = await this._rag.provider.search(input, this._rag.topK ?? 5);
      const filtered = docs.filter((d) => d.score >= (this._rag?.minScore ?? 0.3));
      if (filtered.length > 0) {
        const context = this._rag.contextTemplate ?? defaultRAGTemplate();
        const docText = filtered.map((d, i) =>
          `[${i + 1} | score: ${d.score.toFixed(2)} | ${d.role ?? 'knowledge'}] ${d.content}`
        ).join('\n');
        fullInput = context.replace('{{.Context}}', docText) + '\n\n' + input;
      }
    }

    // Render prompt template if configured
    if (this._promptTemplate) {
      const rendered = this._promptTemplate.withVar('AgentName', this.agent.name).render();
      if (rendered) {
        fullInput = `${rendered}\n\n${fullInput}`;
      }
    }

    return this.agent.run(fullInput);
  }

  async *stream(input: string): AsyncIterable<string> {
    yield* this.agent.stream(input);
  }

  async *streamEvents(input: string): AsyncIterable<import('./react-loop.js').StreamEvent> {
    yield* this.agent.streamEvents(input);
  }

  stop(): void {
    this._lifecycle.stop();
  }

  get name(): string {
    return this.agent.name;
  }

  get lifecycle(): Lifecycle {
    return this._lifecycle;
  }

  // ===== Capability getters =====

  getMemory(): Memory | undefined { return this._memory; }
  getHooks(): HookManager | undefined { return this._hooks; }
  getTracer(): Tracer | undefined { return this._tracer; }
  getCostTracker(): CostTracker | undefined { return this._costTracker; }
  getCheckpointStore(): CheckpointStore | undefined { return this._checkpointStore; }
  getHITLManager(): HITLManager | undefined { return this._hitl; }
  getContextWindowStrategy(): ContextWindowStrategy | undefined { return this._contextWindow; }
  getRAGConfig(): RAGConfig | undefined { return this._rag; }
  getToolkit(): ToolRegistry | undefined { return this._toolkit; }

  // ===== Session =====

  newSession(opts?: { id?: string; maxHistory?: number }): Session {
    // CapabilityAgent wraps ReActAgent; Session needs an agent with run/stream methods
    // We cast to ReActAgent since CapabilityAgent has compatible run/stream interfaces
    return new Session(this as unknown as ReActAgent, this._memory, opts);
  }
}

function defaultRAGTemplate(): string {
  return '=== Relevant Knowledge ===\n{{.Context}}\n=== End Knowledge ===\n';
}

// ===== Convenience factory =====

export function newAgent(
  name: string,
  systemPrompt: string,
  model: Provider,
  opts?: Partial<AgentOption>
): CapabilityAgent {
  return new CapabilityAgent({
    name,
    systemPrompt,
    model,
    maxTurns: opts?.maxTurns ?? 10,
    temperature: opts?.temperature,
    sessionId: opts?.sessionId,
  });
}
