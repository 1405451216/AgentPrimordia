/**
 * Pipeline 编排模式单元测试
 *
 * 覆盖：
 * - Pipeline：顺序执行、条件跳过、初始输入传递
 * - ParallelRun：并行执行所有步骤
 * - Handoff：多 Agent 轮次交接
 */
import { describe, it, expect, vi } from 'vitest';
import { Pipeline, ParallelRun, Handoff } from '../../src/orchestration/pipeline.js';
import type { ReActAgent } from '../../src/agent/react-loop.js';
import type { Response } from '../../src/types.js';

// ===== Mock Agent Factory =====

function createMockAgent(name: string, responses: string[]): ReActAgent {
  let idx = 0;
  return {
    name,
    run: vi.fn(async (input: string): Promise<Response> => {
      const resp = responses[idx] ?? responses[responses.length - 1] ?? `response-${idx}`;
      idx++;
      return {
        content: typeof resp === 'string' ? resp.replace('{input}', input) : resp,
        metrics: { totalTurns: 1, totalTools: 0, duration: 10, llmLatency: 5, toolLatency: 0 },
      };
    }),
  } as unknown as ReActAgent;
}

// ===== Pipeline Tests =====

describe('Pipeline', () => {
  it('should execute steps sequentially', async () => {
    const agent1 = createMockAgent('agent-1', ['result-1']);
    const agent2 = createMockAgent('agent-2', ['result-2']);

    const pipeline = new Pipeline([
      { name: 'step-1', agent: agent1, input: 'input-1' },
      { name: 'step-2', agent: agent2, input: 'input-2' },
    ]);

    const results = await pipeline.run();

    expect(results.length).toBe(2);
    expect(results[0].stepName).toBe('step-1');
    expect(results[0].response.content).toBe('result-1');
    expect(results[0].skipped).toBe(false);
    expect(results[1].stepName).toBe('step-2');
    expect(results[1].response.content).toBe('result-2');
    expect(results[1].skipped).toBe(false);
  });

  it('should pass initial input to first step', async () => {
    const agent = createMockAgent('agent-1', ['processed: {input}']);

    const pipeline = new Pipeline([
      { name: 'first', agent, input: 'default' },
    ]);

    const results = await pipeline.run('custom-initial');

    expect(results[0].response.content).toContain('custom-initial');
  });

  it('should use step.input for subsequent steps (not initialInput)', async () => {
    const agent1 = createMockAgent('a1', ['first']);
    const agent2 = createMockAgent('a2', ['second: {input}']);

    const pipeline = new Pipeline([
      { name: 's1', agent: agent1, input: 'ignored' },
      { name: 's2', agent: agent2, input: 'step-input' },
    ]);

    const results = await pipeline.run('initial');

    // First step uses initialInput
    expect(agent1.run).toHaveBeenCalledWith('initial');
    // Second step uses its own input
    expect(agent2.run).toHaveBeenCalledWith('step-input');
  });

  it('should skip step when condition returns false', async () => {
    const agent1 = createMockAgent('a1', ['result-1']);
    const agent2 = createMockAgent('a2', ['should-not-run']);

    const pipeline = new Pipeline([
      { name: 's1', agent: agent1, input: 'in' },
      {
        name: 's2',
        agent: agent2,
        input: 'in',
        condition: (_prev) => false, // always skip
      },
    ]);

    const results = await pipeline.run();

    expect(results.length).toBe(2);
    expect(results[0].skipped).toBe(false);
    expect(results[1].skipped).toBe(true);
    expect(results[1].response.content).toBe('');
    expect(agent2.run).not.toHaveBeenCalled();
  });

  it('should execute step when condition returns true', async () => {
    const agent1 = createMockAgent('a1', ['result-1']);
    const agent2 = createMockAgent('a2', ['result-2']);

    const pipeline = new Pipeline([
      { name: 's1', agent: agent1, input: 'in' },
      {
        name: 's2',
        agent: agent2,
        input: 'in',
        condition: (prev) => prev !== null && prev.response.content === 'result-1',
      },
    ]);

    const results = await pipeline.run();

    expect(results[1].skipped).toBe(false);
    expect(results[1].response.content).toBe('result-2');
  });

  it('should handle empty pipeline', async () => {
    const pipeline = new Pipeline([]);
    const results = await pipeline.run();
    expect(results).toEqual([]);
  });

  it('should handle single step pipeline', async () => {
    const agent = createMockAgent('solo', ['only-result']);
    const pipeline = new Pipeline([{ name: 'only', agent, input: 'go' }]);

    const results = await pipeline.run();
    expect(results.length).toBe(1);
    expect(results[0].response.content).toBe('only-result');
  });
});

// ===== ParallelRun Tests =====

describe('ParallelRun', () => {
  it('should execute all steps in parallel', async () => {
    const agent1 = createMockAgent('a1', ['par-1']);
    const agent2 = createMockAgent('a2', ['par-2']);
    const agent3 = createMockAgent('a3', ['par-3']);

    const parallel = new ParallelRun([
      { name: 'p1', agent: agent1, input: 'i1' },
      { name: 'p2', agent: agent2, input: 'i2' },
      { name: 'p3', agent: agent3, input: 'i3' },
    ]);

    const results = await parallel.run();

    expect(results.length).toBe(3);
    expect(results.map((r) => r.response.content).sort()).toEqual(['par-1', 'par-2', 'par-3']);
    for (const r of results) {
      expect(r.skipped).toBe(false);
    }
  });

  it('should handle empty parallel run', async () => {
    const parallel = new ParallelRun([]);
    const results = await parallel.run();
    expect(results).toEqual([]);
  });

  it('should handle single step', async () => {
    const agent = createMockAgent('solo', ['solo-result']);
    const parallel = new ParallelRun([{ name: 'only', agent, input: 'go' }]);

    const results = await parallel.run();
    expect(results.length).toBe(1);
    expect(results[0].response.content).toBe('solo-result');
  });
});

// ===== Handoff Tests =====

describe('Handoff', () => {
  it('should pass output from one agent to the next', async () => {
    const agent1 = createMockAgent('a1', ['intermediate']);
    const agent2 = createMockAgent('a2', ['final: {input}']);

    const handoff = new Handoff([agent1, agent2], 1);

    const results = await handoff.run('start');

    expect(results.length).toBe(2);
    // Agent 1 receives the initial input
    expect(agent1.run).toHaveBeenCalledWith('start');
    // Agent 2 receives agent 1's output
    expect(agent2.run).toHaveBeenCalledWith('intermediate');
    expect(results[1].response.content).toBe('final: intermediate');
  });

  it('should run for multiple rounds', async () => {
    const agent1 = createMockAgent('a1', ['round1-a1', 'round2-a1']);
    const agent2 = createMockAgent('a2', ['round1-a2', 'round2-a2']);

    const handoff = new Handoff([agent1, agent2], 2);

    const results = await handoff.run('start');

    expect(results.length).toBe(4); // 2 agents × 2 rounds
    // Verify execution order: a1 → a2 → a1 → a2
    expect(results[0].stepName).toBe('a1');
    expect(results[1].stepName).toBe('a2');
    expect(results[2].stepName).toBe('a1');
    expect(results[3].stepName).toBe('a2');
  });

  it('should use default maxRounds of 3', async () => {
    const agent = createMockAgent('a', ['resp']);
    const handoff = new Handoff([agent]);

    const results = await handoff.run('start');
    expect(results.length).toBe(3); // 1 agent × 3 rounds (default)
  });

  it('should handle single agent single round', async () => {
    const agent = createMockAgent('solo', ['solo-response']);
    const handoff = new Handoff([agent], 1);

    const results = await handoff.run('start');
    expect(results.length).toBe(1);
    expect(results[0].response.content).toBe('solo-response');
  });

  it('should chain output through rounds', async () => {
    const agent = createMockAgent('chain', ['step1', 'step2', 'step3']);
    const handoff = new Handoff([agent], 3);

    const results = await handoff.run('initial');

    expect(agent.run).toHaveBeenCalledTimes(3);
    expect(agent.run).toHaveBeenNthCalledWith(1, 'initial');
    expect(agent.run).toHaveBeenNthCalledWith(2, 'step1');
    expect(agent.run).toHaveBeenNthCalledWith(3, 'step2');
    expect(results[2].response.content).toBe('step3');
  });
});
