// ===== Resilience: Circuit Breaker + Retry =====

export type CircuitState = 'closed' | 'open' | 'half_open';

export interface CircuitBreakerConfig {
  maxFailures: number;
  resetTimeoutMs: number;
  halfOpenMaxCalls: number;
  successThreshold: number;
}

export class CircuitBreaker {
  private config: CircuitBreakerConfig;
  private state: CircuitState = 'closed';
  private failureCount = 0;
  private successCount = 0;
  private halfOpenCalls = 0;
  private lastFailureTime: number = 0;

  constructor(config?: Partial<CircuitBreakerConfig>) {
    this.config = {
      maxFailures: config?.maxFailures ?? 5,
      resetTimeoutMs: config?.resetTimeoutMs ?? 30000,
      halfOpenMaxCalls: config?.halfOpenMaxCalls ?? 3,
      successThreshold: config?.successThreshold ?? 2,
    };
  }

  async execute<T>(fn: () => Promise<T>): Promise<T> {
    if (this.state === 'open') {
      if (Date.now() - this.lastFailureTime >= this.config.resetTimeoutMs) {
        this.state = 'half_open';
        this.halfOpenCalls = 0;
        this.successCount = 0;
      } else {
        throw new Error('Circuit breaker is open');
      }
    }

    if (this.state === 'half_open' && this.halfOpenCalls >= this.config.halfOpenMaxCalls) {
      throw new Error('Circuit breaker half-open: too many concurrent calls');
    }

    this.halfOpenCalls++;

    try {
      const result = await fn();
      this.onSuccess();
      return result;
    } catch (err) {
      this.onFailure();
      throw err;
    }
  }

  private onSuccess(): void {
    this.failureCount = 0;
    if (this.state === 'half_open') {
      this.successCount++;
      if (this.successCount >= this.config.successThreshold) {
        this.state = 'closed';
        this.halfOpenCalls = 0;
      }
    }
  }

  private onFailure(): void {
    this.lastFailureTime = Date.now();
    if (this.state === 'half_open') {
      this.state = 'open';
      this.halfOpenCalls = 0;
    } else {
      this.failureCount++;
      if (this.failureCount >= this.config.maxFailures) {
        this.state = 'open';
      }
    }
  }

  getState(): CircuitState { return this.state; }
  getFailureCount(): number { return this.failureCount; }
  reset(): void {
    this.state = 'closed';
    this.failureCount = 0;
    this.successCount = 0;
    this.halfOpenCalls = 0;
  }
}

// ===== Retry =====

export interface RetryConfig {
  maxRetries: number;
  initialDelayMs: number;
  maxDelayMs: number;
  multiplier: number;
  jitter: boolean;
  retryableErrors?: string[];
}

export class Retry {
  private config: RetryConfig;

  constructor(config?: Partial<RetryConfig>) {
    this.config = {
      maxRetries: config?.maxRetries ?? 3,
      initialDelayMs: config?.initialDelayMs ?? 1000,
      maxDelayMs: config?.maxDelayMs ?? 30000,
      multiplier: config?.multiplier ?? 2,
      jitter: config?.jitter ?? true,
      retryableErrors: config?.retryableErrors,
    };
  }

  async execute<T>(fn: () => Promise<T>): Promise<T> {
    let lastError: Error | undefined;
    let delay = this.config.initialDelayMs;

    for (let attempt = 0; attempt <= this.config.maxRetries; attempt++) {
      try {
        return await fn();
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));

        if (attempt === this.config.maxRetries) break;
        if (this.config.retryableErrors && !this.isRetryable(lastError)) break;

        // Calculate delay with optional jitter
        let sleepTime = delay;
        if (this.config.jitter) {
          sleepTime = Math.floor(Math.random() * delay);
        }

        await new Promise(r => setTimeout(r, sleepTime));
        delay = Math.min(delay * this.config.multiplier, this.config.maxDelayMs);
      }
    }

    throw lastError;
  }

  private isRetryable(error: Error): boolean {
    if (!this.config.retryableErrors) return true;
    return this.config.retryableErrors.some(pattern =>
      error.message.includes(pattern) || error.name.includes(pattern)
    );
  }
}

// ===== Combined Resilient Wrapper =====

export interface ResilientConfig {
  circuitBreaker?: Partial<CircuitBreakerConfig>;
  retry?: Partial<RetryConfig>;
  timeoutMs?: number;
}

export class ResilientWrapper {
  private breaker: CircuitBreaker;
  private retry: Retry;
  private timeoutMs?: number;

  constructor(config?: ResilientConfig) {
    this.breaker = new CircuitBreaker(config?.circuitBreaker);
    this.retry = new Retry(config?.retry);
    this.timeoutMs = config?.timeoutMs;
  }

  async execute<T>(fn: () => Promise<T>): Promise<T> {
    return this.breaker.execute(() => {
      return this.retry.execute(() => {
        if (this.timeoutMs) {
          return this.withTimeout(fn, this.timeoutMs);
        }
        return fn();
      });
    });
  }

  private async withTimeout<T>(fn: () => Promise<T>, timeoutMs: number): Promise<T> {
    return Promise.race([
      fn(),
      new Promise<T>((_, reject) =>
        setTimeout(() => reject(new Error(`Operation timed out after ${timeoutMs}ms`)), timeoutMs)
      ),
    ]);
  }

  get circuitState(): CircuitState { return this.breaker.getState(); }
}

// ===== Fallback Provider =====

export class FallbackHandler<T> {
  private providers: Array<{ name: string; fn: () => Promise<T> }> = [];

  add(name: string, fn: () => Promise<T>): this {
    this.providers.push({ name, fn });
    return this;
  }

  async execute(): Promise<{ result: T; provider: string }> {
    let lastError: Error | undefined;
    for (const { name, fn } of this.providers) {
      try {
        const result = await fn();
        return { result, provider: name };
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));
      }
    }
    throw lastError;
  }
}
