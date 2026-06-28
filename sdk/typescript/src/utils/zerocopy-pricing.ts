import type { Message } from '../types.js';

// ===== Zero-Copy Message Optimization =====
// In JavaScript, strings are immutable and passed by reference,
// so "zero-copy" here means avoiding unnecessary string concatenation
// and using direct references where possible.

export class ZeroCopyMessage {
  role: Message['role'];
  contentRef: string;
  slices: string[] | null = null;

  constructor(role: Message['role'], content: string) {
    this.role = role;
    this.contentRef = content;
  }

  /** Returns content without copying (direct reference). */
  content(): string {
    return this.contentRef;
  }

  /** Convert to standard Message (creates a new object but reuses string). */
  toMessage(): Message {
    return { role: this.role, content: this.contentRef };
  }

  /** Append content slice without copying existing content. */
  append(slice: string): void {
    if (this.slices === null) {
      this.slices = [this.contentRef];
    }
    this.slices.push(slice);
    this.contentRef = this.slices.join('');
  }

  /** Prepend content slice. */
  prepend(slice: string): void {
    if (this.slices === null) {
      this.slices = [this.contentRef];
    }
    this.slices.unshift(slice);
    this.contentRef = this.slices.join('');
  }

  /** Get content length without materializing. */
  length(): number {
    if (this.slices === null) return this.contentRef.length;
    return this.slices.reduce((sum, s) => sum + s.length, 0);
  }
}

/** Batch convert to zero-copy messages. */
export function batchConvertToZeroCopy(role: Message['role'], contents: string[]): ZeroCopyMessage[] {
  return contents.map(c => new ZeroCopyMessage(role, c));
}

// ===== Zero-Copy Pool =====
// Object pool to reduce GC pressure

export class ZeroCopyPool {
  private pool: ZeroCopyMessage[] = [];
  private maxSize: number;

  constructor(maxSize: number = 1000) {
    this.maxSize = maxSize;
  }

  acquire(role: Message['role'], content: string): ZeroCopyMessage {
    const msg = this.pool.pop();
    if (msg) {
      // Reuse pooled object
      msg.role = role;
      msg.contentRef = content;
      msg.slices = null;
      return msg;
    }
    return new ZeroCopyMessage(role, content);
  }

  release(msg: ZeroCopyMessage): void {
    if (this.pool.length < this.maxSize) {
      // Clear references for GC
      msg.contentRef = '';
      msg.slices = null;
      this.pool.push(msg);
    }
  }

  get size(): number { return this.pool.length; }
}

// ===== String Builder (efficient string concatenation) =====

export class StringBuilder {
  private parts: string[] = [];
  private cached: string | null = null;

  append(s: string): this {
    this.parts.push(s);
    this.cached = null;
    return this;
  }

  appendLine(s: string = ''): this {
    this.parts.push(s + '\n');
    this.cached = null;
    return this;
  }

  toString(): string {
    if (this.cached !== null) return this.cached;
    this.cached = this.parts.join('');
    return this.cached;
  }

  clear(): void {
    this.parts = [];
    this.cached = null;
  }

  get length(): number {
    return this.parts.reduce((sum, s) => sum + s.length, 0);
  }

  isEmpty(): boolean {
    return this.parts.length === 0 || this.length === 0;
  }
}

// ===== Byte Buffer Pool =====

export class ByteBufferPool {
  private pool: Buffer[] = [];
  private maxSize: number;
  private bufferSize: number;

  constructor(bufferSize: number = 4096, maxPoolSize: number = 100) {
    this.bufferSize = bufferSize;
    this.maxSize = maxPoolSize;
  }

  acquire(): Buffer {
    return this.pool.pop() ?? Buffer.alloc(this.bufferSize);
  }

  release(buf: Buffer): void {
    if (buf.length === this.bufferSize && this.pool.length < this.maxSize) {
      buf.fill(0);
      this.pool.push(buf);
    }
  }

  get size(): number { return this.pool.length; }
}

// ===== Model Pricing Table =====

export interface ModelPricing {
  model: string;
  provider: string;
  promptPricePer1M: number;
  completionPricePer1M: number;
}

const PRICE_PER_MILLION = 1_000_000;

export function defaultPricingTable(): Map<string, ModelPricing> {
  return new Map(Object.entries({
    'gpt-4o': { model: 'gpt-4o', provider: 'openai', promptPricePer1M: 2.5, completionPricePer1M: 10.0 },
    'gpt-4o-mini': { model: 'gpt-4o-mini', provider: 'openai', promptPricePer1M: 0.15, completionPricePer1M: 0.6 },
    'gpt-4-turbo': { model: 'gpt-4-turbo', provider: 'openai', promptPricePer1M: 10.0, completionPricePer1M: 30.0 },
    'gpt-3.5-turbo': { model: 'gpt-3.5-turbo', provider: 'openai', promptPricePer1M: 0.5, completionPricePer1M: 1.5 },
    'claude-3-5-sonnet-20241022': { model: 'claude-3-5-sonnet-20241022', provider: 'anthropic', promptPricePer1M: 3.0, completionPricePer1M: 15.0 },
    'claude-3-haiku-20240307': { model: 'claude-3-haiku-20240307', provider: 'anthropic', promptPricePer1M: 0.25, completionPricePer1M: 1.25 },
    'claude-3-opus-20240229': { model: 'claude-3-opus-20240229', provider: 'anthropic', promptPricePer1M: 15.0, completionPricePer1M: 75.0 },
    'gemini-1.5-pro': { model: 'gemini-1.5-pro', provider: 'google', promptPricePer1M: 1.25, completionPricePer1M: 5.0 },
    'gemini-1.5-flash': { model: 'gemini-1.5-flash', provider: 'google', promptPricePer1M: 0.075, completionPricePer1M: 0.3 },
    'deepseek-chat': { model: 'deepseek-chat', provider: 'deepseek', promptPricePer1M: 0.14, completionPricePer1M: 0.28 },
    'deepseek-coder': { model: 'deepseek-coder', provider: 'deepseek', promptPricePer1M: 0.14, completionPricePer1M: 0.28 },
    'qwen-plus': { model: 'qwen-plus', provider: 'alibaba', promptPricePer1M: 0.4, completionPricePer1M: 1.2 },
    'qwen-turbo': { model: 'qwen-turbo', provider: 'alibaba', promptPricePer1M: 0.05, completionPricePer1M: 0.2 },
    'glm-4': { model: 'glm-4', provider: 'zhipu', promptPricePer1M: 0.5, completionPricePer1M: 0.5 },
    'mistral-large-latest': { model: 'mistral-large-latest', provider: 'mistral', promptPricePer1M: 2.0, completionPricePer1M: 6.0 },
    'mistral-small-latest': { model: 'mistral-small-latest', provider: 'mistral', promptPricePer1M: 0.2, completionPricePer1M: 0.6 },
    'command-r-plus': { model: 'command-r-plus', provider: 'cohere', promptPricePer1M: 2.5, completionPricePer1M: 10.0 },
    'command-r': { model: 'command-r', provider: 'cohere', promptPricePer1M: 0.15, completionPricePer1M: 0.6 },
  }));
}

export class PricingCalculator {
  private table: Map<string, ModelPricing>;

  constructor(customTable?: Map<string, ModelPricing>) {
    this.table = customTable ?? defaultPricingTable();
  }

  /** Calculate cost for a given model and token usage. */
  calculate(model: string, promptTokens: number, completionTokens: number): number {
    const pricing = this.table.get(model);
    if (!pricing) return 0;

    const promptCost = (promptTokens / PRICE_PER_MILLION) * pricing.promptPricePer1M;
    const completionCost = (completionTokens / PRICE_PER_MILLION) * pricing.completionPricePer1M;

    return promptCost + completionCost;
  }

  /** Add or update a model's pricing. */
  setPricing(pricing: ModelPricing): void {
    this.table.set(pricing.model, pricing);
  }

  /** Get pricing for a model. */
  getPricing(model: string): ModelPricing | undefined {
    return this.table.get(model);
  }

  /** List all known models. */
  listModels(): string[] {
    return Array.from(this.table.keys());
  }
}
