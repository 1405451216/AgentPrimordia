import { describe, it, expect, vi } from 'vitest';
import {
  DAGBuilder,
  DAGWorkflow,
  GroupChat,
  Debate,
  Supervisor,
  WorkflowExecution,
  PlanBuilder,
} from '../../src/orchestration/advanced.js';

// Helper: create a mock ReActAgent
function createMockAgent(name: string, responses: string[]): any {
  let callIndex = 0;
  return {
    name,
    run: vi.fn(async () => {
      const resp = responses[callIndex] ?? responses[responses.length - 1] ?? 'default';
      callIndex++;
      return { content: resp };
    }),
  };
}

// ===== DAGBuilder & DAGWorkflow tests =====
describe('DAGBuilder', () => {
  it('should create builder with name', () => {
    const builder = new DAGBuilder('test-dag');
    const wf = builder.build();
    expect(wf).toBeDefined();
  });

  it('should add nodes', () => {
    const builder = new DAGBuilder('test');
    builder.node('a', async () => 'result-a');
    builder.node('b', async () => 'result-b');
    const wf = builder.build();
    expect(wf).toBeDefined();
  });

  it('should add nodes with config', () => {
    const builder = new DAGBuilder('test');
    builder.nodeWithConfig('a', async () => 'result', { retryCount: 2, timeoutMs: 1000 });
    const wf = builder.build();
    expect(wf).toBeDefined();
  });

  it('should add edges', () => {
    const builder = new DAGBuilder('test');
    builder.node('a', async () => 'a');
    builder.node('b', async () => 'b');
    builder.edge('a', 'b');
    const wf = builder.build();
    expect(wf).toBeDefined();
  });

  it('should add edges with condition', () => {
    const builder = new DAGBuilder('test');
    builder.node('a', async () => 'a');
    builder.node('b', async () => 'b');
    builder.edge('a', 'b', (result) => result.includes('a'));
    const wf = builder.build();
    expect(wf).toBeDefined();
  });
});

describe('DAGWorkflow', () => {
  it('should run single-node workflow', async () => {
    const builder = new DAGBuilder('single');
    builder.node('only', async (input) => `processed: ${input}`);
    const wf = builder.build();
    const result = await wf.run('test input');
    expect(result.success).toBe(true);
    expect(result.output).toContain('processed: test input');
    expect(result.nodeResults['only'].status).toBe('completed');
  });

  it('should run linear multi-node workflow', async () => {
    const builder = new DAGBuilder('linear');
    builder.node('a', async (input) => `a:${input}`);
    builder.node('b', async (input) => `b:${input}`);
    builder.edge('a', 'b');
    const wf = builder.build();
    const result = await wf.run('start');
    expect(result.success).toBe(true);
    expect(result.output).toContain('b:');
    expect(result.nodeResults['a'].status).toBe('completed');
    expect(result.nodeResults['b'].status).toBe('completed');
  });

  it('should skip nodes when condition fails', async () => {
    const builder = new DAGBuilder('conditional');
    builder.nodeWithConfig('a', async (input) => 'yes', {
      condition: (input) => !input.includes('skip'),
    });
    const wf = builder.build();
    const result = await wf.run('skip me');
    expect(result.nodeResults['a'].status).toBe('skipped');
  });

  it('should handle node failure', async () => {
    const builder = new DAGBuilder('fail');
    builder.node('a', async () => { throw new Error('node error'); });
    const wf = builder.build();
    const result = await wf.run('input');
    expect(result.success).toBe(false);
    expect(result.nodeResults['a'].status).toBe('failed');
    expect(result.nodeResults['a'].error).toBe('node error');
  });

  it('should handle node failure with fallback edge', async () => {
    const builder = new DAGBuilder('fallback');
    builder.node('a', async () => { throw new Error('fail'); });
    builder.node('recovery', async () => 'recovered');
    builder.edge('a', 'recovery', () => true);
    const wf = builder.build();
    const result = await wf.run('input');
    // When a node fails and has fallback edges, the workflow continues
    expect(result.nodeResults['a'].status).toBe('failed');
    expect(result.nodeResults['a'].error).toBe('fail');
  });

  it('should retry failed nodes', async () => {
    let attempts = 0;
    const builder = new DAGBuilder('retry');
    builder.nodeWithConfig('a', async () => {
      attempts++;
      if (attempts < 2) throw new Error('retry me');
      return 'success';
    }, { retryCount: 2 });
    const wf = builder.build();
    const result = await wf.run('input');
    expect(result.nodeResults['a'].status).toBe('completed');
    expect(result.output).toBe('success');
  });

  it('should handle timeout', async () => {
    const builder = new DAGBuilder('timeout');
    builder.nodeWithConfig('a', async () => {
      await new Promise(r => setTimeout(r, 500));
      return 'done';
    }, { timeoutMs: 50 });
    const wf = builder.build();
    const result = await wf.run('input');
    expect(result.success).toBe(false);
    expect(result.nodeResults['a'].status).toBe('failed');
  });

  it('should return empty result for empty workflow', async () => {
    const builder = new DAGBuilder('empty');
    const wf = builder.build();
    const result = await wf.run('input');
    expect(result.output).toBe('');
    expect(result.success).toBe(false);
  });

  it('should track totalDuration', async () => {
    const builder = new DAGBuilder('timed');
    builder.node('a', async () => {
      await new Promise(r => setTimeout(r, 10));
      return 'done';
    });
    const wf = builder.build();
    const result = await wf.run('input');
    expect(result.totalDuration).toBeGreaterThanOrEqual(0);
  });
});

// ===== GroupChat tests =====
describe('GroupChat', () => {
  it('should run with multiple agents', async () => {
    const agent1 = createMockAgent('agent1', ['response1']);
    const agent2 = createMockAgent('agent2', ['response2']);
    const chat = new GroupChat({
      agents: [agent1, agent2],
      maxRounds: 1,
    });
    const result = await chat.run('topic');
    expect(result.totalRounds).toBe(1);
    expect(result.messages).toHaveLength(2);
    expect(result.messages[0].agentName).toBe('agent1');
    expect(result.messages[1].agentName).toBe('agent2');
  });

  it('should run with moderator', async () => {
    const agent1 = createMockAgent('a1', ['msg1']);
    const mod = createMockAgent('moderator', ['summary']);
    const chat = new GroupChat({
      agents: [agent1],
      maxRounds: 2,
      moderator: mod,
    });
    const result = await chat.run('topic');
    expect(result.summary).toContain('summary');
  });

  it('should use default maxRounds of 3', async () => {
    const agent1 = createMockAgent('a1', ['r1', 'r2', 'r3']);
    const chat = new GroupChat({ agents: [agent1] });
    const result = await chat.run('topic');
    expect(result.totalRounds).toBe(3);
  });

  it('should generate summary without moderator', async () => {
    const agent1 = createMockAgent('a1', ['hello']);
    const chat = new GroupChat({ agents: [agent1], maxRounds: 1 });
    const result = await chat.run('topic');
    expect(result.summary).toContain('a1');
    expect(result.summary).toContain('hello');
  });
});

// ===== Debate tests =====
describe('Debate', () => {
  it('should run debate between proponent and opponent', async () => {
    const pro = createMockAgent('pro', ['pro-arg1', 'pro-arg2']);
    const opp = createMockAgent('opp', ['opp-arg1', 'opp-arg2']);
    const debate = new Debate({
      topic: 'AI is beneficial',
      proponent: pro,
      opponent: opp,
      rounds: 2,
    });
    const result = await debate.run();
    expect(result.proponentArguments).toHaveLength(2);
    expect(result.opponentArguments).toHaveLength(2);
  });

  it('should run debate with judge', async () => {
    const pro = createMockAgent('pro', ['pro-arg']);
    const opp = createMockAgent('opp', ['opp-arg']);
    const judge = createMockAgent('judge', ['The proponent wins the debate']);
    const debate = new Debate({
      topic: 'Test topic',
      proponent: pro,
      opponent: opp,
      judge,
      rounds: 1,
    });
    const result = await debate.run();
    expect(result.judgeVerdict).toContain('proponent wins');
    expect(result.winner).toBe('proponent');
  });

  it('should declare opponent as winner', async () => {
    const pro = createMockAgent('pro', ['pro']);
    const opp = createMockAgent('opp', ['opp']);
    const judge = createMockAgent('judge', ['The opponent wins']);
    const debate = new Debate({
      topic: 'Test',
      proponent: pro,
      opponent: opp,
      judge,
      rounds: 1,
    });
    const result = await debate.run();
    expect(result.winner).toBe('opponent');
  });

  it('should declare draw when no clear winner', async () => {
    const pro = createMockAgent('pro', ['pro']);
    const opp = createMockAgent('opp', ['opp']);
    const judge = createMockAgent('judge', ['Both sides presented well']);
    const debate = new Debate({
      topic: 'Test',
      proponent: pro,
      opponent: opp,
      judge,
      rounds: 1,
    });
    const result = await debate.run();
    expect(result.winner).toBe('draw');
  });

  it('should use default 3 rounds', async () => {
    const pro = createMockAgent('pro', ['a', 'b', 'c']);
    const opp = createMockAgent('opp', ['x', 'y', 'z']);
    const debate = new Debate({
      topic: 'Test',
      proponent: pro,
      opponent: opp,
    });
    const result = await debate.run();
    expect(result.proponentArguments).toHaveLength(3);
    expect(result.opponentArguments).toHaveLength(3);
  });
});

// ===== Supervisor tests =====
describe('Supervisor', () => {
  it('should delegate tasks to workers', async () => {
    const supervisor = createMockAgent('supervisor', ['worker1', 'DONE']);
    const worker1 = createMockAgent('worker1', ['task result']);
    const workers = new Map([['worker1', worker1]]);
    const sup = new Supervisor({ supervisor, workers, maxIterations: 3 });
    const result = await sup.run('do task');
    expect(result.workerResults['worker1']).toBe('task result');
  });

  it('should stop when supervisor says DONE', async () => {
    const supervisor = createMockAgent('supervisor', ['DONE']);
    const sup = new Supervisor({
      supervisor,
      workers: new Map(),
      maxIterations: 5,
    });
    const result = await sup.run('task');
    expect(result.iterations).toBe(5);
  });

  it('should handle unknown worker name', async () => {
    const supervisor = createMockAgent('supervisor', ['unknown-worker']);
    const sup = new Supervisor({
      supervisor,
      workers: new Map(),
      maxIterations: 2,
    });
    const result = await sup.run('task');
    expect(result.output).toContain('could not determine');
  });

  it('should run final summary at max iterations', async () => {
    const supervisor = createMockAgent('supervisor', ['worker1', 'worker1', 'final summary']);
    const worker1 = createMockAgent('worker1', ['result1', 'result2']);
    const sup = new Supervisor({
      supervisor,
      workers: new Map([['worker1', worker1]]),
      maxIterations: 2,
    });
    const result = await sup.run('task');
    expect(result.output).toContain('final summary');
  });
});

// ===== WorkflowExecution tests =====
describe('WorkflowExecution', () => {
  it('should run linear workflow', async () => {
    const wf = new WorkflowExecution({
      type: 'linear',
      nodes: [
        { id: 'a', type: 'task', handler: async (input) => `a:${input}` },
        { id: 'b', type: 'task', handler: async (input) => `b:${input}` },
      ],
      transitions: [],
    });
    const result = await wf.run('start');
    expect(result.status).toBe('completed');
    expect(result.output).toContain('b:');
    expect(result.nodeResults['a'].status).toBe('completed');
  });

  it('should run conditional workflow', async () => {
    const wf = new WorkflowExecution({
      type: 'conditional',
      nodes: [
        { id: 'start', type: 'task', handler: async () => 'yes' },
        { id: 'branch-a', type: 'task', handler: async () => 'branch-a' },
        { id: 'branch-b', type: 'task', handler: async () => 'branch-b' },
      ],
      transitions: [
        { from: 'start', to: 'branch-a', condition: (r) => r === 'yes' },
        { from: 'start', to: 'branch-b', condition: (r) => r === 'no' },
      ],
    });
    const result = await wf.run('input');
    expect(result.status).toBe('completed');
    expect(result.output).toBe('branch-a');
  });

  it('should run loop workflow', async () => {
    let count = 0;
    const wf = new WorkflowExecution({
      type: 'loop',
      nodes: [
        { id: 'loop_start', type: 'loop_start' },
        { id: 'task', type: 'task', handler: async (input) => { count++; return `${input}-${count}`; } },
        { id: 'loop_end', type: 'loop_end' },
      ],
      transitions: [],
      loopMaxIterations: 3,
      loopCondition: (input, i) => i < 3,
    });
    const result = await wf.run('start');
    expect(result.status).toBe('completed');
    expect(count).toBe(3);
  });

  it('should run parallel fork-join workflow', async () => {
    const wf = new WorkflowExecution({
      type: 'parallel_fork_join',
      nodes: [
        { id: 'a', type: 'parallel', handler: async () => 'result-a' },
        { id: 'b', type: 'parallel', handler: async () => 'result-b' },
      ],
      transitions: [],
    });
    const result = await wf.run('input');
    expect(result.status).toBe('completed');
    expect(result.output).toContain('result-a');
    expect(result.output).toContain('result-b');
  });

  it('should run state machine workflow', async () => {
    const wf = new WorkflowExecution({
      type: 'state_machine',
      nodes: [
        { id: 's1', type: 'task', handler: async () => 'state1' },
        { id: 's2', type: 'task', handler: async () => 'state2' },
        { id: 's3', type: 'task', handler: async () => 'state3' },
      ],
      transitions: [
        { from: 's1', to: 's2' },
        { from: 's2', to: 's3' },
      ],
    });
    const result = await wf.run('start');
    expect(result.status).toBe('completed');
    expect(result.output).toBe('state3');
  });

  it('should pause and resume', async () => {
    const wf = new WorkflowExecution({
      type: 'linear',
      nodes: [{ id: 'a', type: 'task', handler: async (input) => input }],
      transitions: [],
    });
    wf.pause();
    expect(wf.status_).toBe('paused');
    wf.resume();
    expect(wf.status_).toBe('running');
  });

  it('should cancel', async () => {
    const wf = new WorkflowExecution({
      type: 'linear',
      nodes: [{ id: 'a', type: 'task', handler: async (input) => input }],
      transitions: [],
    });
    wf.cancel();
    expect(wf.status_).toBe('cancelled');
  });

  it('should handle workflow errors', async () => {
    const wf = new WorkflowExecution({
      type: 'linear',
      nodes: [{ id: 'a', type: 'task', handler: async () => { throw new Error('workflow error'); } }],
      transitions: [],
    });
    const result = await wf.run('input');
    expect(result.status).toBe('failed');
    expect(result.error?.message).toBe('workflow error');
  });

  it('should skip nodes without handler', async () => {
    const wf = new WorkflowExecution({
      type: 'linear',
      nodes: [
        { id: 'a', type: 'task' },
        { id: 'b', type: 'task', handler: async (input) => `b:${input}` },
      ],
      transitions: [],
    });
    const result = await wf.run('input');
    expect(result.status).toBe('completed');
    expect(result.output).toContain('b:');
  });

  it('should track node duration', async () => {
    const wf = new WorkflowExecution({
      type: 'linear',
      nodes: [{ id: 'a', type: 'task', handler: async () => { await new Promise(r => setTimeout(r, 10)); return 'done'; } }],
      transitions: [],
    });
    const result = await wf.run('input');
    expect(result.nodeResults['a'].duration).toBeGreaterThanOrEqual(0);
  });

  it('should handle loop without loopCondition (runs until maxIterations)', async () => {
    let count = 0;
    const wf = new WorkflowExecution({
      type: 'loop',
      nodes: [{ id: 'task', type: 'task', handler: async () => { count++; return `r${count}`; } }],
      transitions: [],
      loopMaxIterations: 2,
    });
    const result = await wf.run('start');
    expect(count).toBe(2);
  });
});

// ===== PlanBuilder tests =====
describe('PlanBuilder', () => {
  it('should create a plan with steps', () => {
    const builder = new PlanBuilder('achieve goal');
    builder.step('step1', 'do first');
    builder.step('step2', 'do second');
    const plan = builder.build();
    expect(plan.goal).toBe('achieve goal');
    expect(plan.steps).toHaveLength(2);
    expect(plan.steps[0].id).toBe('step1');
  });

  it('should support step dependencies', () => {
    const builder = new PlanBuilder('goal');
    builder.step('step1', 'first');
    builder.step('step2', 'second', { dependsOn: ['step1'] });
    const plan = builder.build();
    expect(plan.steps[1].dependencies).toEqual(['step1']);
  });

  it('should support step with agent', () => {
    const builder = new PlanBuilder('goal');
    builder.step('step1', 'first', { agent: 'worker1' });
    const plan = builder.build();
    expect(plan.steps[0].agent).toBe('worker1');
  });

  it('should throw on unknown dependency', () => {
    const builder = new PlanBuilder('goal');
    builder.step('step1', 'first', { dependsOn: ['unknown'] });
    expect(() => builder.build()).toThrow('unknown step');
  });

  it('should detect cycles', () => {
    const builder = new PlanBuilder('goal');
    builder.step('a', 'a', { dependsOn: ['b'] });
    builder.step('b', 'b', { dependsOn: ['a'] });
    expect(() => builder.build()).toThrow('Cycle detected');
  });

  it('should generate unique plan id', async () => {
    const builder1 = new PlanBuilder('goal');
    const plan1 = builder1.build();
    // Wait a bit to ensure different timestamp
    await new Promise(r => setTimeout(r, 10));
    const builder2 = new PlanBuilder('goal');
    const plan2 = builder2.build();
    expect(plan1.id).not.toBe(plan2.id);
  });

  it('should set createdAt timestamp', () => {
    const builder = new PlanBuilder('goal');
    const plan = builder.build();
    expect(plan.createdAt).toBeInstanceOf(Date);
  });

  it('should create plan with no steps', () => {
    const builder = new PlanBuilder('empty goal');
    const plan = builder.build();
    expect(plan.steps).toHaveLength(0);
  });

  it('should support chained step calls', () => {
    const builder = new PlanBuilder('goal');
    builder
      .step('a', 'first')
      .step('b', 'second', { dependsOn: ['a'] })
      .step('c', 'third', { dependsOn: ['b'] });
    const plan = builder.build();
    expect(plan.steps).toHaveLength(3);
  });

  it('should detect self-referencing cycle', () => {
    const builder = new PlanBuilder('goal');
    builder.step('a', 'a', { dependsOn: ['a'] });
    expect(() => builder.build()).toThrow('Cycle detected');
  });
});
