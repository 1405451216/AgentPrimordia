import { describe, it, expect, vi } from 'vitest';
import { CircuitBreaker, Retry, ResilientWrapper, FallbackHandler } from '../../src/resilience/circuit-retry.js';

describe('CircuitBreaker', () => {
  it('should start in closed state', () => {
    const cb = new CircuitBreaker();
    expect(cb.getState()).toBe('closed');
    expect(cb.getFailureCount()).toBe(0);
  });

  it('should execute successful calls', async () => {
    const cb = new CircuitBreaker();
    const result = await cb.execute(async () => 42);
    expect(result).toBe(42);
    expect(cb.getState()).toBe('closed');
  });

  it('should open after maxFailures', async () => {
    const cb = new CircuitBreaker({ maxFailures: 3, resetTimeoutMs: 999999 });
    for (let i = 0; i < 3; i++) {
      await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    }
    expect(cb.getState()).toBe('open');
    expect(cb.getFailureCount()).toBe(3);
  });

  it('should reject calls when open', async () => {
    const cb = new CircuitBreaker({ maxFailures: 1, resetTimeoutMs: 999999 });
    await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    expect(cb.getState()).toBe('open');
    await expect(cb.execute(async () => 42)).rejects.toThrow('Circuit breaker is open');
  });

  it('should transition to half_open after resetTimeout', async () => {
    const cb = new CircuitBreaker({ maxFailures: 1, resetTimeoutMs: 50 });
    await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    expect(cb.getState()).toBe('open');
    
    await new Promise(r => setTimeout(r, 60));
    const result = await cb.execute(async () => 'recovered');
    expect(result).toBe('recovered');
    expect(cb.getState()).toBe('half_open');
  });

  it('should close after successThreshold in half_open', async () => {
    const cb = new CircuitBreaker({ maxFailures: 1, resetTimeoutMs: 50, successThreshold: 2 });
    await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    
    await new Promise(r => setTimeout(r, 60));
    await cb.execute(async () => 'ok1');
    expect(cb.getState()).toBe('half_open');
    await cb.execute(async () => 'ok2');
    expect(cb.getState()).toBe('closed');
  });

  it('should re-open on failure in half_open', async () => {
    const cb = new CircuitBreaker({ maxFailures: 1, resetTimeoutMs: 50 });
    await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    
    await new Promise(r => setTimeout(r, 60));
    expect(cb.getState()).toBe('open');
    
    await expect(cb.execute(async () => { throw new Error('fail2'); })).rejects.toThrow('fail2');
    expect(cb.getState()).toBe('open');
  });

  it('should limit half_open calls', async () => {
    const cb = new CircuitBreaker({ maxFailures: 1, resetTimeoutMs: 50, halfOpenMaxCalls: 1 });
    await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    
    await new Promise(r => setTimeout(r, 60));
    
    // First call is allowed (halfOpenCalls becomes 1)
    const p = cb.execute(async () => {
      await new Promise(r => setTimeout(r, 100));
      return 'ok';
    });
    
    // Second call should be rejected
    await expect(cb.execute(async () => 'ok')).rejects.toThrow('too many concurrent calls');
    
    await p;
  });

  it('reset() should restore closed state', async () => {
    const cb = new CircuitBreaker({ maxFailures: 1 });
    await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    expect(cb.getState()).toBe('open');
    
    cb.reset();
    expect(cb.getState()).toBe('closed');
    expect(cb.getFailureCount()).toBe(0);
  });

  it('should use default config', () => {
    const cb = new CircuitBreaker();
    expect(cb.getState()).toBe('closed');
  });

  it('should use custom config', async () => {
    const cb = new CircuitBreaker({ maxFailures: 10, resetTimeoutMs: 5000, halfOpenMaxCalls: 5, successThreshold: 3 });
    for (let i = 0; i < 10; i++) {
      await expect(cb.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    }
    expect(cb.getState()).toBe('open');
  });
});

describe('Retry', () => {
  it('should return result on first success', async () => {
    const retry = new Retry();
    const fn = vi.fn().mockResolvedValue('ok');
    const result = await retry.execute(fn);
    expect(result).toBe('ok');
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it('should retry on failure and eventually succeed', async () => {
    const retry = new Retry({ maxRetries: 3, initialDelayMs: 1, jitter: false });
    let attempts = 0;
    const fn = async () => {
      attempts++;
      if (attempts < 3) throw new Error('fail');
      return 'success';
    };
    const result = await retry.execute(fn);
    expect(result).toBe('success');
    expect(attempts).toBe(3);
  });

  it('should throw after maxRetries', async () => {
    const retry = new Retry({ maxRetries: 2, initialDelayMs: 1, jitter: false });
    const fn = vi.fn().mockRejectedValue(new Error('always fail'));
    await expect(retry.execute(fn)).rejects.toThrow('always fail');
    expect(fn).toHaveBeenCalledTimes(3); // initial + 2 retries
  });

  it('should apply exponential backoff', async () => {
    const retry = new Retry({ maxRetries: 3, initialDelayMs: 10, multiplier: 2, maxDelayMs: 1000, jitter: false });
    const fn = vi.fn().mockRejectedValue(new Error('fail'));
    const start = Date.now();
    await expect(retry.execute(fn)).rejects.toThrow('fail');
    const elapsed = Date.now() - start;
    // 10 + 20 + 40 = 70ms minimum
    expect(elapsed).toBeGreaterThanOrEqual(60);
  });

  it('should apply jitter', async () => {
    const retry = new Retry({ maxRetries: 1, initialDelayMs: 100, jitter: true });
    const fn = vi.fn().mockRejectedValue(new Error('fail'));
    const start = Date.now();
    await expect(retry.execute(fn)).rejects.toThrow('fail');
    const elapsed = Date.now() - start;
    // With jitter, delay is random 0-100ms
    expect(elapsed).toBeLessThan(150);
  });

  it('should filter retryable errors', async () => {
    const retry = new Retry({ maxRetries: 3, initialDelayMs: 1, jitter: false, retryableErrors: ['timeout'] });
    const fn = vi.fn().mockRejectedValue(new Error('connection refused'));
    await expect(retry.execute(fn)).rejects.toThrow('connection refused');
    expect(fn).toHaveBeenCalledTimes(1); // not retried
  });

  it('should retry matching error patterns', async () => {
    const retry = new Retry({ maxRetries: 2, initialDelayMs: 1, jitter: false, retryableErrors: ['timeout'] });
    let attempts = 0;
    const fn = async () => {
      attempts++;
      if (attempts < 2) throw new Error('request timeout');
      return 'ok';
    };
    const result = await retry.execute(fn);
    expect(result).toBe('ok');
    expect(attempts).toBe(2);
  });

  it('should handle non-Error throws', async () => {
    const retry = new Retry({ maxRetries: 1, initialDelayMs: 1, jitter: false });
    const fn = vi.fn().mockRejectedValue('string error');
    await expect(retry.execute(fn)).rejects.toThrow();
  });

  it('should cap delay at maxDelayMs', async () => {
    const retry = new Retry({ maxRetries: 3, initialDelayMs: 100, multiplier: 10, maxDelayMs: 200, jitter: false });
    const fn = vi.fn().mockRejectedValue(new Error('fail'));
    const start = Date.now();
    await expect(retry.execute(fn)).rejects.toThrow('fail');
    const elapsed = Date.now() - start;
    // 100 + 200 + 200 = 500ms
    expect(elapsed).toBeLessThan(600);
  });
});

describe('ResilientWrapper', () => {
  it('should combine circuit breaker and retry', async () => {
    const wrapper = new ResilientWrapper({
      circuitBreaker: { maxFailures: 5 },
      retry: { maxRetries: 2, initialDelayMs: 1, jitter: false },
    });

    let attempts = 0;
    const fn = async () => {
      attempts++;
      if (attempts < 3) throw new Error('fail');
      return 'ok';
    };

    const result = await wrapper.execute(fn);
    expect(result).toBe('ok');
    expect(wrapper.circuitState).toBe('closed');
  });

  it('should apply timeout', async () => {
    const wrapper = new ResilientWrapper({
      retry: { maxRetries: 0, initialDelayMs: 1, jitter: false },
      timeoutMs: 50,
    });

    const fn = async () => {
      await new Promise(r => setTimeout(r, 200));
      return 'slow';
    };

    await expect(wrapper.execute(fn)).rejects.toThrow('timed out');
  });

  it('should work without timeout', async () => {
    const wrapper = new ResilientWrapper({
      retry: { maxRetries: 0, initialDelayMs: 1, jitter: false },
    });

    const result = await wrapper.execute(async () => 'ok');
    expect(result).toBe('ok');
  });

  it('should open circuit after repeated failures', async () => {
    const wrapper = new ResilientWrapper({
      circuitBreaker: { maxFailures: 1, resetTimeoutMs: 999999 },
      retry: { maxRetries: 0, initialDelayMs: 1, jitter: false },
    });

    await expect(wrapper.execute(async () => { throw new Error('fail'); })).rejects.toThrow('fail');
    expect(wrapper.circuitState).toBe('open');
  });
});

describe('FallbackHandler', () => {
  it('should use first successful provider', async () => {
    const handler = new FallbackHandler<string>()
      .add('primary', async () => 'primary result')
      .add('secondary', async () => 'secondary result');

    const { result, provider } = await handler.execute();
    expect(result).toBe('primary result');
    expect(provider).toBe('primary');
  });

  it('should fall through to next provider on failure', async () => {
    const handler = new FallbackHandler<string>()
      .add('primary', async () => { throw new Error('fail'); })
      .add('secondary', async () => 'secondary result');

    const { result, provider } = await handler.execute();
    expect(result).toBe('secondary result');
    expect(provider).toBe('secondary');
  });

  it('should throw if all providers fail', async () => {
    const handler = new FallbackHandler<string>()
      .add('a', async () => { throw new Error('a fail'); })
      .add('b', async () => { throw new Error('b fail'); });

    await expect(handler.execute()).rejects.toThrow();
  });

  it('should handle non-Error throws', async () => {
    const handler = new FallbackHandler<string>()
      .add('a', async () => { throw 'string error'; });

    await expect(handler.execute()).rejects.toThrow();
  });

  it('should throw if no providers', async () => {
    const handler = new FallbackHandler<string>();
    await expect(handler.execute()).rejects.toBeUndefined();
  });
});
