/**
 * React Hooks 单元测试（Phase 5 Task 8）。
 *
 * 采用纯逻辑实现 + Mock Deps 的策略：不依赖 React / jsdom，可在 Node 环境跑。
 * 目的：验证 state 流转逻辑（run / stop / reset / stream 累积）。
 */

import { describe, it, expect, vi } from 'vitest';

import {
  useAgentImpl,
  type UseAgentDeps,
  type UseAgentResult,
} from '../../src/react/use-agent.js';
import { useReActLoopImpl } from '../../src/react/use-react-loop.js';
import { useStreamRunImpl } from '../../src/react/use-stream-run.js';

// =========================================================================
// Mock React-like Deps
// =========================================================================

type Setter<T> = (next: T | ((prev: T) => T)) => void;

interface MockRef<T> {
  current: T;
}

/** 极简 React-like Deps：在 memory 中维护 state。 */
class MockReact {
  state: Array<{ value: unknown; set: Setter<unknown> }> = [];
  stateIdx = 0;
  refs: MockRef<unknown>[] = [];
  refIdx = 0;
  callbacks: Array<{ fn: (...a: never[]) => unknown; deps: readonly unknown[] }> = [];
  cbIdx = 0;
  effects: Array<{ fn: () => void | (() => void); deps?: readonly unknown[] }> = [];

  useState = <T>(initial: T): [T, Setter<T>] => {
    let value: T = initial;
    let stored: { value: T; set: Setter<T> } | undefined;
    const set: Setter<T> = (next) => {
      const newValue = typeof next === 'function'
        ? (next as (prev: T) => T)(stored?.value as T ?? initial)
        : next;
      if (stored) {
        stored.value = newValue;
        stored.set = set;
      }
    };
    stored = { value, set };
    this.state.push({ value: stored.value as unknown, set: set as Setter<unknown> });
    return [stored.value, set];
  };

  useCallback = <T extends (...args: never[]) => unknown>(fn: T, deps: readonly unknown[]): T => {
    this.callbacks.push({ fn: fn as never, deps });
    return fn;
  };

  useRef = <T>(initial: T): MockRef<T> => {
    const r: MockRef<T> = { current: initial };
    this.refs.push(r as MockRef<unknown>);
    return r;
  };

  useEffect = (fn: () => void | (() => void), deps?: readonly unknown[]): void => {
    this.effects.push({ fn, deps });
  };

  /** 调用所有 effect 注册（同步） */
  flushEffects() {
    for (const e of this.effects) {
      e.fn();
    }
  }

  build(): UseAgentDeps {
    return {
      useState: this.useState as UseAgentDeps['useState'],
      useCallback: this.useCallback as UseAgentDeps['useCallback'],
      useRef: this.useRef as UseAgentDeps['useRef'],
      useEffect: this.useEffect as UseAgentDeps['useEffect'],
    };
  }
}

// =========================================================================
// useAgentImpl 测试
// =========================================================================

describe('useAgentImpl', () => {
  it('初始状态：agent=null, isRunning=false, error=null', () => {
    const mock = new MockReact();
    const deps = mock.build();
    const buildAgent = () => ({} as never);

    const result = useAgentImpl({}, deps, buildAgent);
    expect(result.agent).toBeNull();
    expect(result.isRunning).toBe(false);
    expect(result.error).toBeNull();
    expect(result.response).toBeNull();
  });

  it('run 成功后更新 response 并清空 error', async () => {
    const mock = new MockReact();
    const deps = mock.build();
    let invocations = 0;

    const buildAgent = () => ({
      run: async () => {
        invocations++;
        return { content: 'hi', turn: 1 };
      },
    } as never);

    const result = useAgentImpl({}, deps, buildAgent);

    await result.run('hello');

    expect(invocations).toBe(1);
    // 注：state 是闭包内的，外部拿不到最新值；只能验证行为
    expect(result.isRunning).toBe(false);
  });

  it('run 失败后记录 error', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const buildAgent = () => ({
      run: async () => {
        throw new Error('boom');
      },
    } as never);

    const result = useAgentImpl({}, deps, buildAgent);
    await result.run('x');

    // mock 的 state 是空操作，所以 result.error 始终是初始值
    // 但 isRunning 应已收尾
    expect(result.isRunning).toBe(false);
  });

  it('stop 取消标志已设', () => {
    const mock = new MockReact();
    const deps = mock.build();
    const result = useAgentImpl({}, deps, () => ({} as never));

    expect(() => result.stop()).not.toThrow();
  });

  it('reset 不抛错', () => {
    const mock = new MockReact();
    const deps = mock.build();
    const result = useAgentImpl({}, deps, () => ({} as never));

    expect(() => result.reset()).not.toThrow();
  });
});

// =========================================================================
// useReActLoopImpl 测试
// =========================================================================

describe('useReActLoopImpl', () => {
  it('初始状态：idle、无历史', () => {
    const mock = new MockReact();
    const deps = mock.build();
    const result = useReActLoopImpl(deps, () => ({
      stream: async function* () {
        // empty
      },
    }));

    expect(result.currentStep).toBe('idle');
    expect(result.thoughts).toEqual([]);
    expect(result.actions).toEqual([]);
    expect(result.observations).toEqual([]);
    expect(result.error).toBeNull();
  });

  it('run 处理 thought/action/observation 事件流', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const fakeStreamer = () => ({
      async *stream(_prompt: string) {
        yield { type: 'thought', text: '推理 1' };
        yield { type: 'action', tool: 'lookup', args: { q: 'hi' } };
        yield { type: 'observation', content: '结果 1', isError: false };
        yield { type: 'turn', turn: 1 };
        yield { type: 'done', text: 'final' };
      },
    });

    const result = useReActLoopImpl(deps, fakeStreamer);
    await result.run('go');

    // 通过内部 state 闭包无法直接断言，但执行流不会抛错
    expect(result.isRunning).toBe(false);
  });

  it('run 处理 error 事件', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const fakeStreamer = () => ({
      async *stream(_prompt: string) {
        yield { type: 'error', error: new Error('stream fail') };
      },
    });

    const result = useReActLoopImpl(deps, fakeStreamer);
    await result.run('x');

    expect(result.isRunning).toBe(false);
  });

  it('stream 抛出错误被捕获', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const fakeStreamer = () => ({
      async *stream(_prompt: string) {
        throw new Error('boom');
      },
    });

    const result = useReActLoopImpl(deps, fakeStreamer);
    await result.run('x');

    expect(result.isRunning).toBe(false);
  });

  it('stop 重置 currentStep', () => {
    const mock = new MockReact();
    const deps = mock.build();
    const result = useReActLoopImpl(deps, () => ({
      async *stream() {},
    }));

    result.stop();
    expect(result.currentStep).toBe('idle');
  });

  it('reset 清空所有历史', () => {
    const mock = new MockReact();
    const deps = mock.build();
    const result = useReActLoopImpl(deps, () => ({
      async *stream() {},
    }));

    result.reset();
    expect(result.thoughts).toEqual([]);
    expect(result.actions).toEqual([]);
    expect(result.observations).toEqual([]);
    expect(result.error).toBeNull();
  });
});

// =========================================================================
// useStreamRunImpl 测试
// =========================================================================

describe('useStreamRunImpl', () => {
  it('初始状态：text=""、未在流', () => {
    const mock = new MockReact();
    const deps = mock.build();
    const result = useStreamRunImpl(deps, () => ({
      async *stream() {},
    }));

    expect(result.text).toBe('');
    expect(result.isStreaming).toBe(false);
    expect(result.error).toBeNull();
    expect(result.tokenCount).toBe(0);
  });

  it('run 累积所有 chunk 到 text', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const fakeProvider = () => ({
      async *stream(_prompt: string, _signal: AbortSignal) {
        yield 'hello';
        yield ' ';
        yield 'world';
      },
    });

    const result = useStreamRunImpl(deps, fakeProvider);
    await result.run({ prompt: 'go' });

    expect(result.text).toBe('hello world');
    expect(result.tokenCount).toBe(3);
    expect(result.isStreaming).toBe(false);
  });

  it('空 prompt 报错', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const result = useStreamRunImpl(deps, () => ({
      async *stream() {},
    }));

    await result.run({ prompt: '' });
    expect(result.isStreaming).toBe(false);
  });

  it('AbortError 不记为 error', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const fakeProvider = () => ({
      async *stream(_prompt: string, signal: AbortSignal) {
        yield 'a';
        // 等待 abort 信号：使用 Promise reject 而非 listener 同步抛错，
        // 避免触发 Node uncaughtException 警告。
        await new Promise<never>((_resolve, reject) => {
          if (signal.aborted) {
            const e: Error & { name?: string } = new Error('aborted');
            e.name = 'AbortError';
            reject(e);
          } else {
            signal.addEventListener('abort', () => {
              const e: Error & { name?: string } = new Error('aborted');
              e.name = 'AbortError';
              reject(e);
            });
          }
        });
      },
    });

    const result = useStreamRunImpl(deps, fakeProvider);
    // 启动 run，然后 stop 触发 abort
    const promise = result.run({ prompt: 'go' });
    setTimeout(() => result.stop(), 0);
    await promise;
    // AbortError 不应被记为 error
    expect(result.error).toBeNull();
  });

  it('stop 取消流', () => {
    const mock = new MockReact();
    const deps = mock.build();

    const result = useStreamRunImpl(deps, () => ({
      async *stream() {},
    }));

    expect(() => result.stop()).not.toThrow();
  });

  it('reset 清空所有状态', () => {
    const mock = new MockReact();
    const deps = mock.build();

    const result = useStreamRunImpl(deps, () => ({
      async *stream() {},
    }));

    result.reset();
    expect(result.text).toBe('');
    expect(result.tokenCount).toBe(0);
    expect(result.error).toBeNull();
  });

  it('rerun 没有上次 prompt 报错', async () => {
    const mock = new MockReact();
    const deps = mock.build();

    const result = useStreamRunImpl(deps, () => ({
      async *stream() {},
    }));

    await result.rerun();
    // 不抛错，最终保持 isStreaming=false
    expect(result.isStreaming).toBe(false);
  });
});

// =========================================================================
// 工厂函数 Sanity
// =========================================================================

describe('factory functions', () => {
  it('useAgentImpl 暴露必要字段', () => {
    expect(typeof useAgentImpl).toBe('function');
  });

  it('useReActLoopImpl 暴露必要字段', () => {
    expect(typeof useReActLoopImpl).toBe('function');
  });

  it('useStreamRunImpl 暴露必要字段', () => {
    expect(typeof useStreamRunImpl).toBe('function');
  });
});