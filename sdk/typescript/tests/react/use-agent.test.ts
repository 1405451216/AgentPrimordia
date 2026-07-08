/**
 * useAgent Hook 测试
 *
 * 测试策略：
 * - 使用 mock React hooks 测试 useAgentImpl 逻辑
 * - 不依赖真实 React 环境
 * - 验证状态管理、abort、reset 等核心逻辑
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useAgentImpl, makeState, type UseAgentDeps, type StateRef } from '../../src/react/use-agent.js';
import type { Response } from '../../src/types.js';

// ===== Mock React Hooks =====

function createMockDeps(): UseAgentDeps & { states: Map<string, unknown> } {
  const states = new Map<string, unknown>();

  return {
    states,
    useState: <T>(initial: T): [T, (next: T | ((prev: T) => T)) => void] => {
      const key = `state_${states.size}`;
      if (!states.has(key)) {
        states.set(key, initial);
      }
      const setter = (next: T | ((prev: T) => T)) => {
        const current = states.get(key) as T;
        const computed = typeof next === 'function' ? (next as (prev: T) => T)(current) : next;
        states.set(key, computed);
      };
      return [states.get(key) as T, setter];
    },
    useCallback: <T extends (...args: never[]) => unknown>(fn: T): T => fn,
    useRef: <T>(initial: T): { current: T } => ({ current: initial }),
    useEffect: (_fn: () => void | (() => void), _deps?: unknown[]): void => {},
  };
}

// ===== Mock Agent =====

function createMockAgent(name: string) {
  return {
    name,
    run: vi.fn().mockResolvedValue({ content: `response from ${name}` }),
    streamRun: vi.fn().mockImplementation(async function* () {
      yield `chunk1 from ${name}`;
      yield `chunk2 from ${name}`;
    }),
  };
}

// ===== 测试用例 =====

describe('makeState', () => {
  it('should return current value via get()', () => {
    const deps = createMockDeps();
    const ref = makeState(deps, 'initial');
    expect(ref.get()).toBe('initial');
  });

  it('should update value via set()', () => {
    const deps = createMockDeps();
    const ref = makeState(deps, 'initial');
    ref.set('updated');
    expect(ref.get()).toBe('updated');
  });

  it('should support function updater', () => {
    const deps = createMockDeps();
    const ref = makeState(deps, 0);
    ref.set((prev) => prev + 1);
    expect(ref.get()).toBe(1);
  });
});

describe('useAgentImpl', () => {
  it('should return initial state', () => {
    const deps = createMockDeps();
    const result = useAgentImpl({ name: 'test' }, deps, () => createMockAgent('test'));

    expect(result.agent).toBeNull();
    expect(result.isRunning).toBe(false);
    expect(result.error).toBeNull();
    expect(result.response).toBeNull();
  });

  it('should set isRunning during run', async () => {
    const deps = createMockDeps();
    let resolveRun: (value: Response) => void = () => {};
    const mockAgent = {
      name: 'test',
      run: vi.fn().mockImplementation(() => new Promise<Response>((resolve) => {
        resolveRun = resolve;
      })),
      streamRun: vi.fn(),
    };

    const result = useAgentImpl({ name: 'test' }, deps, () => mockAgent);

    // 开始运行
    const runPromise = result.run('hello');
    expect(result.isRunning).toBe(true);

    // 完成运行
    resolveRun({ content: 'done' });
    await runPromise;

    expect(result.isRunning).toBe(false);
    expect(result.response).toEqual({ content: 'done' });
  });

  it('should handle run error', async () => {
    const deps = createMockDeps();
    const mockAgent = {
      name: 'test',
      run: vi.fn().mockRejectedValue(new Error('LLM failed')),
      streamRun: vi.fn(),
    };

    const result = useAgentImpl({ name: 'test' }, deps, () => mockAgent);
    await result.run('hello');

    expect(result.isRunning).toBe(false);
    expect(result.error).toBeInstanceOf(Error);
    expect(result.error?.message).toBe('LLM failed');
  });

  it('should support stop()', async () => {
    const deps = createMockDeps();
    let resolveRun: (value: Response) => void = () => {};
    const mockAgent = {
      name: 'test',
      run: vi.fn().mockImplementation(() => new Promise<Response>((resolve) => {
        resolveRun = resolve;
      })),
      streamRun: vi.fn(),
    };

    const result = useAgentImpl({ name: 'test' }, deps, () => mockAgent);

    const runPromise = result.run('hello');
    result.stop(); // 取消

    // 即使 resolve，也应该保持 isRunning = false
    resolveRun({ content: 'done' });
    await runPromise;

    expect(result.isRunning).toBe(false);
  });

  it('should support reset()', async () => {
    const deps = createMockDeps();
    const mockAgent = {
      name: 'test',
      run: vi.fn().mockResolvedValue({ content: 'done' }),
      streamRun: vi.fn(),
    };

    const result = useAgentImpl({ name: 'test' }, deps, () => mockAgent);
    await result.run('hello');

    expect(result.response).not.toBeNull();
    result.reset();

    expect(result.agent).toBeNull();
    expect(result.error).toBeNull();
    expect(result.response).toBeNull();
    expect(result.isRunning).toBe(false);
  });

  it('should handle streamRun', async () => {
    const deps = createMockDeps();
    const mockAgent = {
      name: 'test',
      run: vi.fn(),
      streamRun: vi.fn().mockImplementation(async function* () {
        yield 'chunk1';
        yield 'chunk2';
        yield 'chunk3';
      }),
    };

    const result = useAgentImpl({ name: 'test' }, deps, () => mockAgent);

    const chunks: string[] = [];
    for await (const chunk of result.streamRun('hello')) {
      chunks.push(chunk);
    }

    expect(chunks).toEqual(['chunk1', 'chunk2', 'chunk3']);
  });

  it('should set agent after first run', async () => {
    const deps = createMockDeps();
    const mockAgent = createMockAgent('test');

    const result = useAgentImpl({ name: 'test' }, deps, () => mockAgent);
    expect(result.agent).toBeNull();

    await result.run('hello');
    expect(result.agent).not.toBeNull();
  });
});
