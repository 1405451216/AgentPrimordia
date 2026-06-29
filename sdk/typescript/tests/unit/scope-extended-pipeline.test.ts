import { describe, it, expect, vi } from 'vitest';
import { FileScopePolicy as ExtendedFileScopePolicy, ToolPermission, ScopedExecutor } from '../../src/tools/scope-extended.js';
import { Pipeline, ParallelRun, Handoff } from '../../src/orchestration/pipeline.js';
import type { ReActAgent } from '../../src/agent/react-loop.js';
import type { Response } from '../../src/types.js';

// ===== Extended FileScopePolicy tests =====
describe('ExtendedFileScopePolicy', () => {
  it('should allow paths for agent', () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allow('agent-1', ['/home/project']);
    expect(policy.checkPath('agent-1', '/home/project/file.ts')).toBe(true);
    expect(policy.checkPath('agent-1', '/etc/passwd')).toBe(false);
  });

  it('should allow all paths when no restrictions', () => {
    const policy = new ExtendedFileScopePolicy();
    expect(policy.checkPath('agent-1', '/any/path')).toBe(true);
  });

  it('should allow all paths when empty array', () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allow('agent-1', []);
    expect(policy.checkPath('agent-1', '/any/path')).toBe(true);
  });

  it('should match exact path', () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allow('agent-1', ['/exact/path']);
    expect(policy.checkPath('agent-1', '/exact/path')).toBe(true);
  });

  it('should allow commands for agent', () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allowCommands('agent-1', ['git', 'npm']);
    expect(policy.checkCommand('agent-1', 'git commit')).toBe(true);
    expect(policy.checkCommand('agent-1', 'npm install')).toBe(true);
    expect(policy.checkCommand('agent-1', 'rm -rf /')).toBe(false);
  });

  it('should allow all commands when no restrictions', () => {
    const policy = new ExtendedFileScopePolicy();
    expect(policy.checkCommand('agent-1', 'anything')).toBe(true);
  });

  it('should get rules', () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allow('agent-1', ['/path1']);
    policy.allowCommands('agent-1', ['git']);
    const rules = policy.getRules('agent-1');
    expect(rules).toEqual({ paths: ['/path1'], commands: ['git'] });
  });

  it('should get rules for unknown agent', () => {
    const policy = new ExtendedFileScopePolicy();
    const rules = policy.getRules('unknown');
    expect(rules).toEqual({ paths: [], commands: [] });
  });
});

// ===== ToolPermission tests =====
describe('ToolPermission', () => {
  it('should allow without confirmation required', async () => {
    const perm = new ToolPermission();
    const result = await perm.check({ toolName: 'shell', agentID: 'a1', args: {} });
    expect(result.allowed).toBe(true);
  });

  it('should allow when no handler set', async () => {
    const perm = new ToolPermission();
    perm.requireConfirm('shell');
    const result = await perm.check({ toolName: 'shell', agentID: 'a1', args: {} });
    expect(result.allowed).toBe(true);
  });

  it('should call handler for confirmed tools', async () => {
    const perm = new ToolPermission();
    perm.requireConfirm('shell');
    perm.setHandler(async () => ({ allowed: false, reason: 'blocked' }));
    const result = await perm.check({ toolName: 'shell', agentID: 'a1', args: {} });
    expect(result.allowed).toBe(false);
    expect(result.reason).toBe('blocked');
  });

  it('should pass through non-confirmed tools', async () => {
    const perm = new ToolPermission();
    perm.requireConfirm('shell');
    perm.setHandler(async () => ({ allowed: false }));
    const result = await perm.check({ toolName: 'filesystem', agentID: 'a1', args: {} });
    expect(result.allowed).toBe(true);
  });

  it('should handle modified args from handler', async () => {
    const perm = new ToolPermission();
    perm.requireConfirm('shell');
    perm.setHandler(async (req) => ({
      allowed: true,
      modifiedArgs: { ...req.args, extra: 'added' },
    }));
    const result = await perm.check({ toolName: 'shell', agentID: 'a1', args: { cmd: 'ls' } });
    expect(result.allowed).toBe(true);
    expect(result.modifiedArgs).toEqual({ cmd: 'ls', extra: 'added' });
  });
});

// ===== ScopedExecutor tests =====
describe('ScopedExecutor', () => {
  it('should execute tool without scope restrictions', async () => {
    const policy = new ExtendedFileScopePolicy();
    const executor = new ScopedExecutor(policy);
    const mockTool = {
      name: 'test',
      description: 'test',
      parameters: {},
      execute: vi.fn().mockResolvedValue('result'),
    };
    const result = await executor.execute(mockTool, {}, 'agent-1');
    expect(result.content).toBe('result');
    expect(result.isError).toBe(false);
  });

  it('should block paths outside scope', async () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allow('agent-1', ['/allowed']);
    const executor = new ScopedExecutor(policy);
    const mockTool = {
      name: 'fs',
      description: 'fs',
      parameters: {},
      execute: vi.fn().mockResolvedValue('ok'),
    };
    const result = await executor.execute(mockTool, { path: '/forbidden' }, 'agent-1');
    expect(result.isError).toBe(true);
    expect(result.content).toContain('outside allowed scope');
  });

  it('should allow paths in scope', async () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allow('agent-1', ['/allowed']);
    const executor = new ScopedExecutor(policy);
    const mockTool = {
      name: 'fs',
      description: 'fs',
      parameters: {},
      execute: vi.fn().mockResolvedValue('ok'),
    };
    const result = await executor.execute(mockTool, { path: '/allowed/file.txt' }, 'agent-1');
    expect(result.isError).toBe(false);
  });

  it('should block commands outside scope', async () => {
    const policy = new ExtendedFileScopePolicy();
    policy.allowCommands('agent-1', ['git']);
    const executor = new ScopedExecutor(policy);
    const mockTool = {
      name: 'shell',
      description: 'shell',
      parameters: {},
      execute: vi.fn().mockResolvedValue('ok'),
    };
    const result = await executor.execute(mockTool, { command: 'rm -rf /' }, 'agent-1');
    expect(result.isError).toBe(true);
    expect(result.content).toContain('not allowed');
  });

  it('should handle tool execution errors', async () => {
    const policy = new ExtendedFileScopePolicy();
    const executor = new ScopedExecutor(policy);
    const mockTool = {
      name: 'fail',
      description: 'fail',
      parameters: {},
      execute: vi.fn().mockRejectedValue(new Error('tool failed')),
    };
    const result = await executor.execute(mockTool, {}, 'agent-1');
    expect(result.isError).toBe(true);
    expect(result.content).toContain('tool failed');
  });

  it('should use custom timeout', async () => {
    const policy = new ExtendedFileScopePolicy();
    const executor = new ScopedExecutor(policy, 5000);
    const mockTool = {
      name: 'slow',
      description: 'slow',
      parameters: {},
      execute: vi.fn().mockImplementation(async () => {
        await new Promise(r => setTimeout(r, 100));
        return 'done';
      }),
    };
    const result = await executor.execute(mockTool, {}, 'agent-1', { timeoutMs: 50 });
    expect(result.isError).toBe(true);
    expect(result.content).toContain('timed out');
  });
});

// ===== Pipeline tests =====
function mockAgent(name: string, response: string): ReActAgent {
  return {
    name,
    run: vi.fn().mockResolvedValue({ content: response, metrics: { totalTurns: 1, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } } as Response),
  } as unknown as ReActAgent;
}

describe('Pipeline', () => {
  it('should run steps sequentially', async () => {
    const pipeline = new Pipeline([
      { name: 'step1', agent: mockAgent('a1', 'output1'), input: 'input1' },
      { name: 'step2', agent: mockAgent('a2', 'output2'), input: 'input2' },
    ]);
    const results = await pipeline.run();
    expect(results).toHaveLength(2);
    expect(results[0].stepName).toBe('step1');
    expect(results[0].response.content).toBe('output1');
    expect(results[1].stepName).toBe('step2');
    expect(results[1].skipped).toBe(false);
  });

  it('should use initialInput for first step', async () => {
    const agent = mockAgent('a1', 'result');
    const pipeline = new Pipeline([
      { name: 'step1', agent, input: 'default' },
    ]);
    await pipeline.run('custom input');
    expect(agent.run).toHaveBeenCalledWith('custom input');
  });

  it('should use step input for non-first steps', async () => {
    const agent1 = mockAgent('a1', 'r1');
    const agent2 = mockAgent('a2', 'r2');
    const pipeline = new Pipeline([
      { name: 's1', agent: agent1, input: 'in1' },
      { name: 's2', agent: agent2, input: 'in2' },
    ]);
    await pipeline.run();
    expect(agent2.run).toHaveBeenCalledWith('in2');
  });

  it('should skip steps when condition is false', async () => {
    const pipeline = new Pipeline([
      { name: 's1', agent: mockAgent('a1', 'r1'), input: 'in1' },
      { name: 's2', agent: mockAgent('a2', 'r2'), input: 'in2', condition: () => false },
    ]);
    const results = await pipeline.run();
    expect(results[1].skipped).toBe(true);
    expect(results[1].response.content).toBe('');
  });

  it('should run steps when condition is true', async () => {
    const pipeline = new Pipeline([
      { name: 's1', agent: mockAgent('a1', 'r1'), input: 'in1' },
      { name: 's2', agent: mockAgent('a2', 'r2'), input: 'in2', condition: () => true },
    ]);
    const results = await pipeline.run();
    expect(results[1].skipped).toBe(false);
  });

  it('should handle empty pipeline', async () => {
    const pipeline = new Pipeline([]);
    const results = await pipeline.run();
    expect(results).toHaveLength(0);
  });
});

describe('ParallelRun', () => {
  it('should run all steps in parallel', async () => {
    const pipeline = new ParallelRun([
      { name: 's1', agent: mockAgent('a1', 'r1'), input: 'in1' },
      { name: 's2', agent: mockAgent('a2', 'r2'), input: 'in2' },
    ]);
    const results = await pipeline.run();
    expect(results).toHaveLength(2);
    expect(results[0].response.content).toBe('r1');
    expect(results[1].response.content).toBe('r2');
    expect(results.every(r => !r.skipped)).toBe(true);
  });

  it('should handle empty parallel', async () => {
    const pipeline = new ParallelRun([]);
    const results = await pipeline.run();
    expect(results).toHaveLength(0);
  });
});

describe('Handoff', () => {
  it('should run agents in sequence for maxRounds', async () => {
    const agent1 = mockAgent('a1', 'response1');
    const agent2 = mockAgent('a2', 'response2');
    const handoff = new Handoff([agent1 as ReActAgent, agent2 as ReActAgent], 2);
    const results = await handoff.run('initial');
    // 2 agents * 2 rounds = 4 results
    expect(results).toHaveLength(4);
    expect(results[0].stepName).toBe('a1');
    expect(results[1].stepName).toBe('a2');
    expect(results[2].stepName).toBe('a1');
    expect(results[3].stepName).toBe('a2');
  });

  it('should pass output as input to next agent', async () => {
    const agent1 = mockAgent('a1', 'first output');
    const agent2 = mockAgent('a2', 'second output');
    const handoff = new Handoff([agent1 as ReActAgent, agent2 as ReActAgent], 1);
    await handoff.run('initial');
    expect(agent1.run).toHaveBeenCalledWith('initial');
    expect(agent2.run).toHaveBeenCalledWith('first output');
  });

  it('should use default maxRounds', async () => {
    const agent = mockAgent('a', 'r');
    const handoff = new Handoff([agent as ReActAgent]);
    const results = await handoff.run('test');
    expect(results).toHaveLength(3); // default 3 rounds
  });

  it('should handle single agent', async () => {
    const agent = mockAgent('a', 'response');
    const handoff = new Handoff([agent as ReActAgent], 1);
    const results = await handoff.run('input');
    expect(results).toHaveLength(1);
  });
});
