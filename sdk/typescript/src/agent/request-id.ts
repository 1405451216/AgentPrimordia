import { randomBytes } from 'node:crypto';
import { AsyncLocalStorage } from 'node:async_hooks';

// ===== Request ID Propagation =====

const requestIDStorage = new AsyncLocalStorage<string>();

/** Generate a unique request ID (32-char hex). */
export function newRequestID(): string {
  return randomBytes(16).toString('hex');
}

/**
 * Run a function with a request ID in context.
 * If no ID is provided, a new one is generated.
 * The ID is propagated through async calls via AsyncLocalStorage.
 */
export function withRequestID<T>(fn: () => Promise<T>, reqID?: string): Promise<T> {
  const id = reqID ?? newRequestID();
  return requestIDStorage.run(id, fn);
}

/** Get the current request ID from async context. */
export function getRequestID(): string {
  return requestIDStorage.getStore() ?? '';
}

// ===== Context Window Strategy =====

export interface ContextWindowStrategy {
  /** Trim messages to fit within a token budget. */
  trim(messages: import('../types.js').Message[], maxTokens: number): import('../types.js').Message[];
}

/** Keep last N messages strategy. */
export class KeepLastNStrategy implements ContextWindowStrategy {
  constructor(private n: number = 20) {}

  trim(messages: import('../types.js').Message[], _maxTokens: number): import('../types.js').Message[] {
    if (messages.length <= this.n) return messages;
    const system = messages.filter((m) => m.role === 'system');
    const rest = messages.filter((m) => m.role !== 'system');
    const keep = Math.max(1, this.n - system.length);
    return [...system, ...rest.slice(-keep)];
  }
}

/** Token estimation strategy (rough estimate: 1 token ≈ 4 chars). */
export class TokenBudgetStrategy implements ContextWindowStrategy {
  constructor(private charsPerToken: number = 4) {}

  trim(messages: import('../types.js').Message[], maxTokens: number): import('../types.js').Message[] {
    const maxChars = maxTokens * this.charsPerToken;
    const system = messages.filter((m) => m.role === 'system');
    const rest = messages.filter((m) => m.role !== 'system');

    // Always keep system messages
    let systemChars = 0;
    for (const m of system) systemChars += m.content.length;
    const budget = maxChars - systemChars;
    if (budget <= 0) return system;

    // Keep as many recent messages as fit
    const kept: import('../types.js').Message[] = [];
    let used = 0;
    for (let i = rest.length - 1; i >= 0; i--) {
      const msgChars = rest[i].content.length;
      if (used + msgChars > budget) break;
      kept.unshift(rest[i]);
      used += msgChars;
    }
    return [...system, ...kept];
  }
}

// ===== Checkpoint Persistence =====

export interface Checkpoint {
  id: string;
  sessionID: string;
  turn: number;
  messages: import('../types.js').Message[];
  metrics: import('../types.js').AgentMetrics;
  createdAt: string;
}

export interface CheckpointStore {
  save(checkpoint: Checkpoint): Promise<void>;
  load(id: string): Promise<Checkpoint | null>;
  list(sessionID: string): Promise<Checkpoint[]>;
  delete(id: string): Promise<void>;
}

/** In-memory checkpoint store. */
export class InMemoryCheckpointStore implements CheckpointStore {
  private checkpoints: Map<string, Checkpoint> = new Map();

  async save(checkpoint: Checkpoint): Promise<void> {
    this.checkpoints.set(checkpoint.id, checkpoint);
  }

  async load(id: string): Promise<Checkpoint | null> {
    return this.checkpoints.get(id) ?? null;
  }

  async list(sessionID: string): Promise<Checkpoint[]> {
    const result: Checkpoint[] = [];
    for (const cp of this.checkpoints.values()) {
      if (cp.sessionID === sessionID) result.push(cp);
    }
    return result.sort((a, b) => a.turn - b.turn);
  }

  async delete(id: string): Promise<void> {
    this.checkpoints.delete(id);
  }

  clear(): void {
    this.checkpoints.clear();
  }
}

// ===== HITL (Human-in-the-Loop) =====

export type InterruptReason = 'tool_confirm' | 'user_input' | 'approval';

export interface InterruptRequest {
  reason: InterruptReason;
  message: string;
  data?: Record<string, unknown>;
  turn: number;
}

export interface InterruptResponse {
  approved: boolean;
  input?: string;
  modified?: Record<string, unknown>;
}

export type InterruptHandler = (req: InterruptRequest) => Promise<InterruptResponse>;

export interface HITLConfig {
  /** Tools that require human confirmation before execution. */
  confirmTools: string[] | '*';
  /** Handler for interrupt requests. */
  handler: InterruptHandler;
  /** Timeout in ms for waiting for human response (0 = no timeout). */
  timeoutMs?: number;
}

export class HITLManager {
  private config: HITLConfig;

  constructor(config: HITLConfig) {
    this.config = config;
  }

  /** Check if a tool requires human confirmation. */
  shouldInterrupt(toolName: string): boolean {
    if (this.config.confirmTools === '*') return true;
    return this.config.confirmTools.includes(toolName);
  }

  /** Request human confirmation for a tool call. */
  async requestInterrupt(req: InterruptRequest): Promise<InterruptResponse> {
    if (this.config.timeoutMs && this.config.timeoutMs > 0) {
      return Promise.race([
        this.config.handler(req),
        new Promise<InterruptResponse>((_, reject) =>
          setTimeout(() => reject(new Error('HITL timeout')), this.config.timeoutMs)
        ),
      ]);
    }
    return this.config.handler(req);
  }

  /** Update the configuration. */
  update(config: Partial<HITLConfig>): void {
    Object.assign(this.config, config);
  }
}

// ===== Cost Tracker =====

export interface ModelPricing {
  inputPer1K: number;  // cost per 1K input tokens
  outputPer1K: number; // cost per 1K output tokens
}

export interface CostRecord {
  model: string;
  provider: string;
  inputTokens: number;
  outputTokens: number;
  cost: number;
  timestamp: Date;
}

export interface BudgetConfig {
  maxCost: number;
  onBudgetExceeded?: (total: number) => void;
}

export interface CostSummary {
  totalCost: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  byModel: Map<string, { cost: number; calls: number }>;
  byProvider: Map<string, { cost: number; calls: number }>;
}

const DEFAULT_PRICING: Record<string, ModelPricing> = {
  'gpt-4o': { inputPer1K: 0.0025, outputPer1K: 0.01 },
  'gpt-4o-mini': { inputPer1K: 0.00015, outputPer1K: 0.0006 },
  'gpt-4-turbo': { inputPer1K: 0.01, outputPer1K: 0.03 },
  'gpt-3.5-turbo': { inputPer1K: 0.0005, outputPer1K: 0.0015 },
  'claude-3-5-sonnet': { inputPer1K: 0.003, outputPer1K: 0.015 },
  'claude-3-opus': { inputPer1K: 0.015, outputPer1K: 0.075 },
  'claude-3-sonnet': { inputPer1K: 0.003, outputPer1K: 0.015 },
  'claude-3-haiku': { inputPer1K: 0.00025, outputPer1K: 0.00125 },
  'gemini-1.5-pro': { inputPer1K: 0.00125, outputPer1K: 0.005 },
  'gemini-1.5-flash': { inputPer1K: 0.000075, outputPer1K: 0.0003 },
  'deepseek-chat': { inputPer1K: 0.00014, outputPer1K: 0.00028 },
  'deepseek-reasoner': { inputPer1K: 0.00055, outputPer1K: 0.00219 },
};

export class CostTracker {
  private pricing: Record<string, ModelPricing>;
  private budget?: BudgetConfig;
  private records: CostRecord[] = [];
  private totalCost = 0;

  constructor(pricing?: Record<string, ModelPricing>, budget?: BudgetConfig) {
    this.pricing = pricing ?? DEFAULT_PRICING;
    this.budget = budget;
  }

  /** Record an LLM call's cost. */
  record(model: string, provider: string, inputTokens: number, outputTokens: number): number {
    const p = this.pricing[model] ?? { inputPer1K: 0.001, outputPer1K: 0.002 };
    const cost = (inputTokens / 1000) * p.inputPer1K + (outputTokens / 1000) * p.outputPer1K;
    this.totalCost += cost;
    this.records.push({ model, provider, inputTokens, outputTokens, cost, timestamp: new Date() });
    return cost;
  }

  /** Check if budget has been exceeded. */
  checkBudget(): boolean {
    if (!this.budget) return false;
    return this.totalCost >= this.budget.maxCost;
  }

  /** Get cost summary. */
  summary(): CostSummary {
    const byModel = new Map<string, { cost: number; calls: number }>();
    const byProvider = new Map<string, { cost: number; calls: number }>();
    let totalInput = 0;
    let totalOutput = 0;

    for (const r of this.records) {
      totalInput += r.inputTokens;
      totalOutput += r.outputTokens;

      const m = byModel.get(r.model) ?? { cost: 0, calls: 0 };
      m.cost += r.cost;
      m.calls++;
      byModel.set(r.model, m);

      const p = byProvider.get(r.provider) ?? { cost: 0, calls: 0 };
      p.cost += r.cost;
      p.calls++;
      byProvider.set(r.provider, p);
    }

    return {
      totalCost: this.totalCost,
      totalInputTokens: totalInput,
      totalOutputTokens: totalOutput,
      byModel,
      byProvider,
    };
  }

  /** Get all cost records. */
  getRecords(): CostRecord[] {
    return [...this.records];
  }

  /** Reset the tracker. */
  reset(): void {
    this.records = [];
    this.totalCost = 0;
  }

  /** Add or update a model's pricing. */
  setPricing(model: string, pricing: ModelPricing): void {
    this.pricing[model] = pricing;
  }
}
