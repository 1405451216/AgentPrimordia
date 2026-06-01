import type { Provider } from './provider.js';
import type { ToolCallRequest, ToolCallResponse, CompletionRequest, CompletionResponse, Chunk, ModelInfo } from '../types.js';

export class ResilientProvider implements Provider {
  private primary: Provider;
  private fallbacks: Provider[] = [];
  private maxRetries: number;
  private retryBackoff: number;
  private maxBackoff: number;
  private circuitThreshold: number;
  private circuitRecoverAfter: number;
  private failures = 0;
  private lastFailTime = 0;
  private circuitOpen = false;

  constructor(primary: Provider, opts?: { maxRetries?: number; retryBackoff?: number; maxBackoff?: number; circuitThreshold?: number; circuitRecoverAfter?: number }) {
    this.primary = primary;
    this.maxRetries = opts?.maxRetries ?? 3;
    this.retryBackoff = opts?.retryBackoff ?? 500;
    this.maxBackoff = opts?.maxBackoff ?? 10000;
    this.circuitThreshold = opts?.circuitThreshold ?? 5;
    this.circuitRecoverAfter = opts?.circuitRecoverAfter ?? 30000;
  }

  addFallback(provider: Provider): void {
    this.fallbacks.push(provider);
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    this.checkCircuit();
    try {
      const result = await this.executeWithRetry(() => this.primary.complete(req));
      this.recordSuccess();
      return result;
    } catch (err) {
      this.recordFailure();
      throw err;
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    this.checkCircuit();
    try {
      const result = await this.executeWithRetry(() => this.primary.callTools(req));
      this.recordSuccess();
      return result;
    } catch (err) {
      this.recordFailure();
      throw err;
    }
  }

  info(): ModelInfo {
    return this.primary.info();
  }

  private checkCircuit(): void {
    if (this.circuitOpen && Date.now() - this.lastFailTime > this.circuitRecoverAfter) {
      this.circuitOpen = false;
    }
    if (this.circuitOpen) {
      throw new Error('circuit breaker is open');
    }
  }

  private recordSuccess(): void {
    this.failures = 0;
    this.circuitOpen = false;
  }

  private recordFailure(): void {
    this.failures++;
    this.lastFailTime = Date.now();
    if (this.failures >= this.circuitThreshold) {
      this.circuitOpen = true;
    }
  }

  private async executeWithRetry<T>(fn: () => Promise<T>): Promise<T> {
    let lastErr: Error | null = null;
    for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
      if (attempt > 0) {
        const backoff = Math.min(this.retryBackoff * Math.pow(2, attempt - 1), this.maxBackoff);
        await new Promise((r) => setTimeout(r, backoff));
      }
      try {
        return await fn();
      } catch (err: any) {
        lastErr = err;
      }
    }
    for (const fallback of this.fallbacks) {
      try {
        return await fn.call(null);
      } catch (err: any) {
        lastErr = err;
      }
    }
    throw lastErr ?? new Error('all providers failed');
  }
}
