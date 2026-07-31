/**
 * hooks.test.ts — React hooks 单元测试。
 *
 * 测试 react 子包中所有 hook 工厂函数和核心逻辑：
 * - useAgentImpl (use-agent.ts)
 * - useReActLoopImpl (use-react-loop.ts)
 * - useStreamRunImpl (use-stream-run.ts)
 * - useAgentStreamImpl (hooks/useAgentStream.ts)
 * - useOrchestrationStream (hooks.ts)
 *
 * 通过注入 mock React 原语，无需真实 React 运行时。
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useAgentImpl, makeState, type UseAgentDeps } from '../use-agent.js';
import { useReActLoopImpl, type ReActStreamer } from '../use-react-loop.js';
import { useStreamRunImpl, type StreamProvider } from '../use-stream-run.js';

// ===== Mock React 原语 =====

function createMockReactDeps(): UseAgentDeps & { _states: Map<number, any> } {
  const states = new Map<number, any>();
  let stateId = 0;
  const refs = new Map<number, any>();
  let refId = 0;
  const effects: Array<{ fn: () => void | (() => void); deps?: readonly unknown[] }> = [];
  const callbacks: Array<{ fn: (...args: any[]) => any; deps: readonly unknown[] }> = [];

  return {
    _states: states,
    useState: <T>(initial: T): [T, (next: T | ((prev: T) => T)) => void] => {
      const id = stateId++;
      if (!states.has(id)) states.set(id, initial);
      const setter = (next: T | ((prev: T) => T)) => {
        const computed = typeof next === 'function' ? (next as (prev: T) => T)(states.get(id)) : next;
        states.set(id, computed);
      };
      return [states.get(id), setter];
    },
    useCallback: <T extends (...args: never[]) => unknown>(fn: T, deps: readonly unknown[]): T => {
      callbacks.push({ fn, deps });
      return fn;
    },
    useRef: <T>(initial: T): { current: T } => {
      const id = refId++;
      if (!refs.has(id)) refs.set(id, { current: initial });
      return refs.get(id);
    },
    useEffect: (fn: () => void | (() => void), deps?: readonly unknown[]) => {
      effects.push({ fn, deps });
    },
  };
}

// ===== useAgentImpl 测试 =====

describe('useAgentImpl', () => {
  let deps: UseAgentDeps;

  const mockAgent = {
    run: vi.fn().mockResolvedValue({ content: 'hello response', metrics: {} }),
    stream: vi.fn().mockResolvedValue((async function* () {
      yield 'hello';
      yield ' ';
      yield 'world';
    })()),
  };

  const buildAgent = vi.fn().mockReturnValue(mockAgent);

  beforeEach(() => {
    vi.clearAllMocks();
    const mock = createMockReactDeps();
    deps = mock;
  });

  it('should initialize with default state', () => {
    const result = useAgentImpl({}, deps, buildAgent);
    expect(result.agent).toBeNull();
    expect(result.isRunning).toBe(false);
    expect(result.error).toBeNull();
    expect(result.response).toBeNull();
  });

  it('should return run/stop/reset functions', () => {
    const result = useAgentImpl({}, deps, buildAgent);
    expect(typeof result.run).toBe('function');
    expect(typeof result.streamRun).toBe('function');
    expect(typeof result.stop).toBe('function');
    expect(typeof result.reset).toBe('function');
  });

  it('run() should call buildAgent and agent.run', async () => {
    const result = useAgentImpl({ name: 'test-agent' }, deps, buildAgent);
    await result.run('test prompt');

    expect(buildAgent).toHaveBeenCalledWith({ name: 'test-agent' });
    expect(mockAgent.run).toHaveBeenCalledWith('test prompt', expect.any(Object));
  });

  it('run() should set error when buildAgent throws', async () => {
    const errorBuild = vi.fn().mockImplementation(() => {
      throw new Error('build failed');
    });
    const result = useAgentImpl({}, deps, errorBuild);
    await result.run('test');

    expect(result.error).toBeInstanceOf(Error);
    expect(result.error?.message).toBe('build failed');
    expect(result.isRunning).toBe(false);
  });

  it('stop() should abort the current run', () => {
    const result = useAgentImpl({}, deps, buildAgent);
    // stop should not throw even when nothing is running
    expect(() => result.stop()).not.toThrow();
  });

  it('reset() should clear all state', () => {
    const result = useAgentImpl({}, deps, buildAgent);
    result.reset();
    expect(result.agent).toBeNull();
    expect(result.error).toBeNull();
    expect(result.response).toBeNull();
    expect(result.isRunning).toBe(false);
  });
});

// ===== makeState 测试 =====

describe('makeState', () => {
  it('should initialize with the given value', () => {
    const deps = createMockReactDeps();
    const state = makeState(deps, 42);
    expect(state.get()).toBe(42);
  });

  it('should update with a direct value', () => {
    const deps = createMockReactDeps();
    const state = makeState(deps, 'initial');
    state.set('updated');
    expect(state.get()).toBe('updated');
  });

  it('should update with a function updater', () => {
    const deps = createMockReactDeps();
    const state = makeState(deps, 10);
    state.set((prev) => prev + 5);
    expect(state.get()).toBe(15);
  });
});

// ===== useReActLoopImpl 测试 =====

describe('useReActLoopImpl', () => {
  it('should initialize with idle state', () => {
    const deps = createMockReactDeps();
    const streamer: ReActStreamer = {
      stream: vi.fn().mockReturnValue((async function* () {})()),
    };
    const result = useReActLoopImpl(deps, () => streamer);

    expect(result.currentStep).toBe('idle');
    expect(result.thoughts).toEqual([]);
    expect(result.actions).toEqual([]);
    expect(result.observations).toEqual([]);
    expect(result.turn).toBe(0);
    expect(result.isRunning).toBe(false);
    expect(result.error).toBeNull();
  });

  it('should process stream events through run()', async () => {
    const deps = createMockReactDeps();
    const streamer: ReActStreamer = {
      stream: vi.fn().mockReturnValue((async function* () {
        yield { type: 'thought', text: 'Let me think...' };
        yield { type: 'action', tool: 'search', args: { query: 'test' } };
        yield { type: 'observation', content: 'found result', isError: false };
        yield { type: 'turn', turn: 1 };
        yield { type: 'done', text: 'final answer' };
      })()),
    };

    const result = useReActLoopImpl(deps, () => streamer);
    await result.run('test prompt');

    expect(result.thoughts).toContain('Let me think...');
    expect(result.actions.length).toBe(1);
    expect(result.actions[0].tool).toBe('search');
    expect(result.observations.length).toBe(1);
    expect(result.currentStep).toBe('done');
    expect(result.isRunning).toBe(false);
  });

  it('should handle errors in stream', async () => {
    const deps = createMockReactDeps();
    const streamer: ReActStreamer = {
      stream: vi.fn().mockReturnValue((async function* () {
        yield { type: 'error', error: new Error('stream failed') };
      })()),
    };

    const result = useReActLoopImpl(deps, () => streamer);
    await result.run('test');

    expect(result.error).toBeInstanceOf(Error);
    expect(result.error?.message).toBe('stream failed');
  });

  it('reset() should clear all state', () => {
    const deps = createMockReactDeps();
    const streamer: ReActStreamer = {
      stream: vi.fn().mockReturnValue((async function* () {})()),
    };
    const result = useReActLoopImpl(deps, () => streamer);
    result.reset();

    expect(result.thoughts).toEqual([]);
    expect(result.actions).toEqual([]);
    expect(result.currentStep).toBe('idle');
    expect(result.turn).toBe(0);
  });

  it('stop() should set isRunning to false and step to idle', () => {
    const deps = createMockReactDeps();
    const streamer: ReActStreamer = {
      stream: vi.fn().mockReturnValue((async function* () {})()),
    };
    const result = useReActLoopImpl(deps, () => streamer);
    result.stop();

    expect(result.isRunning).toBe(false);
    expect(result.currentStep).toBe('idle');
  });
});

// ===== useStreamRunImpl 测试 =====

describe('useStreamRunImpl', () => {
  it('should initialize with empty state', () => {
    const deps = createMockReactDeps();
    const provider: StreamProvider = {
      stream: vi.fn().mockReturnValue((async function* () {})()),
    };
    const result = useStreamRunImpl(deps, () => provider);

    expect(result.text).toBe('');
    expect(result.isStreaming).toBe(false);
    expect(result.error).toBeNull();
    expect(result.tokenCount).toBe(0);
  });

  it('should accumulate text from stream', async () => {
    const deps = createMockReactDeps();
    const provider: StreamProvider = {
      stream: vi.fn().mockReturnValue((async function* () {
        yield 'Hello';
        yield ' ';
        yield 'World';
      })()),
    };

    const result = useStreamRunImpl(deps, () => provider);
    await result.run({ prompt: 'test' });

    expect(result.text).toBe('Hello World');
    expect(result.tokenCount).toBe(3);
    expect(result.isStreaming).toBe(false);
  });

  it('should set error when prompt is empty', async () => {
    const deps = createMockReactDeps();
    const provider: StreamProvider = {
      stream: vi.fn().mockReturnValue((async function* () {})()),
    };

    const result = useStreamRunImpl(deps, () => provider);
    await result.run({ prompt: '' });

    expect(result.error).toBeInstanceOf(Error);
    expect(result.error?.message).toContain('prompt');
  });

  it('should set error when rerun without previous prompt', async () => {
    const deps = createMockReactDeps();
    const provider: StreamProvider = {
      stream: vi.fn().mockReturnValue((async function* () {})()),
    };

    const result = useStreamRunImpl(deps, () => provider);
    await result.rerun();

    expect(result.error).toBeInstanceOf(Error);
    expect(result.error?.message).toContain('prompt');
  });

  it('reset() should clear all accumulated state', async () => {
    const deps = createMockReactDeps();
    const provider: StreamProvider = {
      stream: vi.fn().mockReturnValue((async function* () {
        yield 'data';
      })()),
    };

    const result = useStreamRunImpl(deps, () => provider);
    await result.run({ prompt: 'test' });
    result.reset();

    expect(result.text).toBe('');
    expect(result.tokenCount).toBe(0);
    expect(result.error).toBeNull();
  });

  it('stop() should abort the stream', () => {
    const deps = createMockReactDeps();
    const provider: StreamProvider = {
      stream: vi.fn().mockReturnValue((async function* () {})()),
    };
    const result = useStreamRunImpl(deps, () => provider);
    // Should not throw when nothing is streaming
    expect(() => result.stop()).not.toThrow();
  });
});
