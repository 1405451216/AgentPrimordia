import { describe, it, expect, vi } from 'vitest';
import { Session } from '../../src/agent/session.js';
import { LLMPlanner, PlanExecutor } from '../../src/agent/planning.js';
import {
  ExactMatchEvaluator,
  ContainsEvaluator,
  RegexEvaluator,
  LLMEvaluator,
  CompositeEvaluator,
  EvalSuite,
} from '../../src/agent/eval.js';
import { LLMReflector } from '../../src/agent/reflection.js';
import { estimateTokens, estimateTokenCount, ContextWindow } from '../../src/agent/context-compress.js';
import { MemoryToolLearner } from '../../src/agent/tool-learning.js';
import { MermaidGenerator, DOTGenerator, WorkflowVisualizer, VisualEditor, defaultVisualizeConfig } from '../../src/agent/visualize.js';
import { AgentTool } from '../../src/agent/agent-tool.js';

// Helper: create mock Provider
function createMockProvider(responses: string[]): any {
  let idx = 0;
  return {
    complete: vi.fn(async () => {
      const resp = responses[idx] ?? responses[responses.length - 1] ?? '';
      idx++;
      return { content: resp };
    }),
    stream: vi.fn(async function* () {
      for (const r of responses) yield r;
    }),
  };
}

// Helper: create mock ReActAgent
function createMockAgent(name: string, responses: string[]): any {
  let idx = 0;
  return {
    name,
    run: vi.fn(async () => {
      const resp = responses[idx] ?? responses[responses.length - 1] ?? 'default';
      idx++;
      return { content: resp };
    }),
    stream: async function* () {
      for (const r of responses) yield r;
    },
  };
}

// Helper: create mock Memory
function createMockMemory(): any {
  const items: any[] = [];
  return {
    add: vi.fn(async (item: any) => { items.push(item); }),
    search: vi.fn(async () => items),
    get: vi.fn(async (id: string) => items.find(i => i.id === id)),
    all: vi.fn(async () => items),
    remove: vi.fn(async (id: string) => {
      const idx = items.findIndex(i => i.id === id);
      if (idx >= 0) items.splice(idx, 1);
    }),
    clear: vi.fn(() => { items.length = 0; }),
  };
}

// ===== Session tests =====
describe('Session', () => {
  it('should create session with auto-generated id', () => {
    const agent = createMockAgent('test', ['response']);
    const session = new Session(agent);
    expect(session.id).toContain('sess-');
    expect(session.length).toBe(0);
  });

  it('should create session with custom id', () => {
    const agent = createMockAgent('test', ['response']);
    const session = new Session(agent, undefined, { id: 'custom-session' });
    expect(session.id).toBe('custom-session');
  });

  it('should ask agent and track history', async () => {
    const agent = createMockAgent('test', ['hello response']);
    const session = new Session(agent);
    const resp = await session.ask('hello');
    expect(resp.content).toBe('hello response');
    expect(session.length).toBe(2); // user + assistant
    expect(session.getHistory()).toHaveLength(2);
  });

  it('should include previous conversation in context', async () => {
    const agent = createMockAgent('test', ['response1', 'response2']);
    const session = new Session(agent);
    await session.ask('question1');
    // Check that the second call includes previous conversation
    await session.ask('question2');
    expect(agent.run).toHaveBeenCalledTimes(2);
    // The second call should include "Previous conversation"
    const secondCallArg = agent.run.mock.calls[1][0] as string;
    expect(secondCallArg).toContain('Previous conversation');
    expect(secondCallArg).toContain('question1');
  });

  it('should clear history', async () => {
    const agent = createMockAgent('test', ['response']);
    const session = new Session(agent);
    await session.ask('hello');
    expect(session.length).toBe(2);
    session.clear();
    expect(session.length).toBe(0);
    expect(session.getHistory()).toHaveLength(0);
  });

  it('should use custom maxHistory', async () => {
    const agent = createMockAgent('test', ['r1', 'r2', 'r3']);
    const session = new Session(agent, undefined, { maxHistory: 1 });
    await session.ask('q1');
    await session.ask('q2');
    // History should be trimmed
    expect(session.length).toBeLessThanOrEqual(2);
  });

  it('should save to memory if provided', async () => {
    const agent = createMockAgent('test', ['response']);
    const memory = createMockMemory();
    const session = new Session(agent, memory);
    await session.ask('hello');
    expect(memory.add).toHaveBeenCalledTimes(2); // user + assistant
  });

  it('should stream response', async () => {
    const agent = createMockAgent('test', ['chunk1', 'chunk2']);
    const session = new Session(agent);
    const chunks: string[] = [];
    for await (const chunk of session.askStream('hello')) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThan(0);
    expect(session.length).toBe(2);
  });
});

// ===== LLMPlanner tests =====
describe('LLMPlanner', () => {
  it('should decompose task into subtasks', async () => {
    const provider = createMockProvider([
      JSON.stringify([
        { id: '1', description: 'First step', depends_on: [] },
        { id: '2', description: 'Second step', depends_on: ['1'] },
      ]),
    ]);
    const planner = new LLMPlanner(provider);
    const subtasks = await planner.decompose('do something');
    expect(subtasks).toHaveLength(2);
    expect(subtasks[0].id).toBe('1');
    expect(subtasks[0].status).toBe('pending');
    expect(subtasks[1].dependsOn).toEqual(['1']);
  });

  it('should handle invalid JSON response', async () => {
    const provider = createMockProvider(['not json at all']);
    const planner = new LLMPlanner(provider);
    const subtasks = await planner.decompose('task');
    expect(subtasks).toHaveLength(0);
  });

  it('should extract JSON from text with surrounding content', async () => {
    const provider = createMockProvider([
      `Here is the plan:\n[{"id":"1","description":"step","depends_on":[]}]`,
    ]);
    const planner = new LLMPlanner(provider);
    const subtasks = await planner.decompose('task');
    expect(subtasks).toHaveLength(1);
    expect(subtasks[0].id).toBe('1');
  });

  it('should generate plan', async () => {
    const provider = createMockProvider([
      JSON.stringify([{ id: '1', description: 'step', depends_on: [] }]),
    ]);
    const planner = new LLMPlanner(provider);
    const plan = await planner.generatePlan('goal');
    expect(plan.goal).toBe('goal');
    expect(plan.subTasks).toHaveLength(1);
    expect(plan.createdAt).toBeDefined();
  });

  it('should handle dependsOn field name', async () => {
    const provider = createMockProvider([
      JSON.stringify([{ id: '1', description: 'step', dependsOn: [] }]),
    ]);
    const planner = new LLMPlanner(provider);
    const subtasks = await planner.decompose('task');
    expect(subtasks[0].dependsOn).toEqual([]);
  });
});

// ===== PlanExecutor tests =====
describe('PlanExecutor', () => {
  it('should execute plan with no dependencies', async () => {
    const provider = createMockProvider(['result1', 'result2']);
    const executor = new PlanExecutor({ provider });
    const plan = {
      goal: 'test',
      createdAt: new Date().toISOString(),
      subTasks: [
        { id: '1', description: 'task1', dependsOn: [], status: 'pending' as const },
        { id: '2', description: 'task2', dependsOn: [], status: 'pending' as const },
      ],
    };
    const result = await executor.execute(plan);
    expect(result.subTasks[0].status).toBe('completed');
    expect(result.subTasks[1].status).toBe('completed');
  });

  it('should execute tasks with dependencies in order', async () => {
    const provider = createMockProvider(['result1', 'result2-with-context']);
    const executor = new PlanExecutor({ provider });
    const plan = {
      goal: 'test',
      createdAt: new Date().toISOString(),
      subTasks: [
        { id: '1', description: 'first', dependsOn: [], status: 'pending' as const },
        { id: '2', description: 'second', dependsOn: ['1'], status: 'pending' as const },
      ],
    };
    const result = await executor.execute(plan);
    expect(result.subTasks[0].status).toBe('completed');
    expect(result.subTasks[1].status).toBe('completed');
    // Second task should have context from first
    const secondCallArg = provider.complete.mock.calls[1][0];
    expect(secondCallArg.messages[0].content).toContain('result1');
  });

  it('should handle failed dependencies', async () => {
    const provider = {
      complete: vi.fn()
        .mockRejectedValueOnce(new Error('fail'))
        .mockResolvedValueOnce({ content: 'result2' }),
    };
    const executor = new PlanExecutor({ provider: provider as any });
    const plan = {
      goal: 'test',
      createdAt: new Date().toISOString(),
      subTasks: [
        { id: '1', description: 'fail-task', dependsOn: [], status: 'pending' as const },
        { id: '2', description: 'dependent', dependsOn: ['1'], status: 'pending' as const },
      ],
    };
    const result = await executor.execute(plan);
    expect(result.subTasks[0].status).toBe('failed');
    expect(result.subTasks[1].status).toBe('failed');
    expect(result.subTasks[1].result).toBe('Dependency failed');
  });
});

// ===== ExactMatchEvaluator tests =====
describe('ExactMatchEvaluator', () => {
  it('should match exact text', async () => {
    const evaluator = new ExactMatchEvaluator();
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'hello world', toolCalls: [] },
      expected: 'hello world',
    });
    expect(result.passed).toBe(true);
    expect(result.score).toBe(1.0);
  });

  it('should not match different text', async () => {
    const evaluator = new ExactMatchEvaluator();
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'hello', toolCalls: [] },
      expected: 'world',
    });
    expect(result.passed).toBe(false);
    expect(result.score).toBe(0.0);
  });

  it('should support case insensitive matching', async () => {
    const evaluator = new ExactMatchEvaluator({ caseInsensitive: true });
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'Hello World', toolCalls: [] },
      expected: 'hello world',
    });
    expect(result.passed).toBe(true);
  });

  it('should normalize whitespace by default', async () => {
    const evaluator = new ExactMatchEvaluator();
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'hello   world', toolCalls: [] },
      expected: 'hello world',
    });
    expect(result.passed).toBe(true);
  });
});

// ===== ContainsEvaluator tests =====
describe('ContainsEvaluator', () => {
  it('should check if output contains expected', async () => {
    const evaluator = new ContainsEvaluator();
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'hello world foo', toolCalls: [] },
      expected: 'world',
    });
    expect(result.passed).toBe(true);
  });

  it('should fail if not contained', async () => {
    const evaluator = new ContainsEvaluator();
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'hello', toolCalls: [] },
      expected: 'world',
    });
    expect(result.passed).toBe(false);
  });

  it('should support case insensitive', async () => {
    const evaluator = new ContainsEvaluator({ caseInsensitive: true });
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'Hello World', toolCalls: [] },
      expected: 'WORLD',
    });
    expect(result.passed).toBe(true);
  });
});

// ===== RegexEvaluator tests =====
describe('RegexEvaluator', () => {
  it('should match pattern', async () => {
    const evaluator = new RegexEvaluator('\\d+');
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'abc123def', toolCalls: [] },
      expected: '',
    });
    expect(result.passed).toBe(true);
  });

  it('should not match when pattern fails', async () => {
    const evaluator = new RegexEvaluator('^\\d+$');
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'abc', toolCalls: [] },
      expected: '',
    });
    expect(result.passed).toBe(false);
  });

  it('should accept RegExp object', async () => {
    const evaluator = new RegexEvaluator(/foo/i);
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'FOO bar', toolCalls: [] },
      expected: '',
    });
    expect(result.passed).toBe(true);
  });
});

// ===== LLMEvaluator tests =====
describe('LLMEvaluator', () => {
  it('should parse LLM evaluation result', async () => {
    const provider = createMockProvider([
      JSON.stringify({
        score: 0.8,
        passed: true,
        criteria: [{ name: 'relevance', score: 0.8, passed: true, reason: 'relevant' }],
      }),
    ]);
    const evaluator = new LLMEvaluator(provider, ['relevance']);
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'response', toolCalls: [] },
      expected: 'expected',
    });
    expect(result.score).toBe(0.8);
    expect(result.passed).toBe(true);
    expect(result.criteria).toHaveLength(1);
  });

  it('should handle invalid LLM response', async () => {
    const provider = createMockProvider(['not json']);
    const evaluator = new LLMEvaluator(provider, ['criterion']);
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'response', toolCalls: [] },
      expected: 'expected',
    });
    expect(result.passed).toBe(false);
    expect(result.criteria[0].name).toBe('parse_error');
  });
});

// ===== CompositeEvaluator tests =====
describe('CompositeEvaluator', () => {
  it('should combine multiple evaluators', async () => {
    const evaluator = new CompositeEvaluator([
      { evaluator: new ExactMatchEvaluator(), weight: 0.5 },
      { evaluator: new ContainsEvaluator(), weight: 0.5 },
    ]);
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'hello world', toolCalls: [] },
      expected: 'hello world',
    });
    expect(result.passed).toBe(true);
    expect(result.score).toBe(1.0);
    expect(result.criteria).toHaveLength(2);
  });

  it('should handle partial pass', async () => {
    const evaluator = new CompositeEvaluator([
      { evaluator: new ExactMatchEvaluator(), weight: 0.5 },
      { evaluator: new ContainsEvaluator(), weight: 0.5 },
    ]);
    const result = await evaluator.evaluate({
      task: 'test',
      agentOutput: { content: 'hello world extra', toolCalls: [] },
      expected: 'hello world',
    });
    expect(result.passed).toBe(false); // exact match fails
    expect(result.score).toBe(0.5); // contains passes
  });
});

// ===== EvalSuite tests =====
describe('EvalSuite', () => {
  it('should run all test cases', async () => {
    const evaluator = new ExactMatchEvaluator();
    const suite = new EvalSuite({
      evaluator,
      cases: [
        { task: 't1', input: 'q1', expected: 'a1' },
        { task: 't2', input: 'q2', expected: 'a2' },
      ],
      agentRun: async () => ({ content: 'a1', toolCalls: [] }),
    });
    const result = await suite.run();
    expect(result.total).toBe(2);
    expect(result.passed).toBe(1);
    expect(result.failed).toBe(1);
    expect(result.passRate).toBe(0.5);
  });

  it('should handle agent errors', async () => {
    const evaluator = new ExactMatchEvaluator();
    const suite = new EvalSuite({
      evaluator,
      cases: [{ task: 't1', input: 'q1', expected: 'a1' }],
      agentRun: async () => { throw new Error('agent error'); },
    });
    const result = await suite.run();
    expect(result.total).toBe(1);
    expect(result.failed).toBe(1);
    expect(result.results[0].error).toBeDefined();
  });

  it('should handle no agentRun', async () => {
    const evaluator = new ContainsEvaluator();
    const suite = new EvalSuite({
      evaluator,
      cases: [{ task: 't1', input: 'q1', expected: '' }],
    });
    const result = await suite.run();
    expect(result.total).toBe(1);
    expect(result.passed).toBe(1); // empty string is contained in empty content
  });

  it('should handle empty cases', async () => {
    const evaluator = new ExactMatchEvaluator();
    const suite = new EvalSuite({ evaluator, cases: [] });
    const result = await suite.run();
    expect(result.total).toBe(0);
    expect(result.passRate).toBe(0);
  });
});

// ===== LLMReflector tests =====
describe('LLMReflector', () => {
  it('should parse reflection result', async () => {
    const provider = createMockProvider([
      JSON.stringify({
        strengths: ['good logic'],
        weaknesses: ['too verbose'],
        suggestions: ['be concise'],
        confidence: 0.8,
      }),
    ]);
    const reflector = new LLMReflector(provider);
    const reflection = await reflector.reflect('input', 'output');
    expect(reflection.strengths).toContain('good logic');
    expect(reflection.weaknesses).toContain('too verbose');
    expect(reflection.suggestions).toContain('be concise');
    expect(reflection.confidence).toBe(0.8);
  });

  it('should handle invalid reflection response', async () => {
    const provider = createMockProvider(['not json']);
    const reflector = new LLMReflector(provider);
    const reflection = await reflector.reflect('input', 'output');
    expect(reflection.strengths).toHaveLength(0);
    expect(reflection.confidence).toBe(0.5);
  });

  it('should parse critique result', async () => {
    const provider = createMockProvider([
      JSON.stringify({
        issues: [{ description: 'typo', location: 'line 5', severity: 'low' }],
        severity: 'medium',
        corrections: [{ original: 'teh', corrected: 'the', reason: 'spelling' }],
      }),
    ]);
    const reflector = new LLMReflector(provider);
    const critique = await reflector.critique('output with teh');
    expect(critique.issues).toHaveLength(1);
    expect(critique.issues[0].severity).toBe('low');
    expect(critique.severity).toBe('medium');
    expect(critique.corrections).toHaveLength(1);
    expect(critique.corrections[0].corrected).toBe('the');
  });

  it('should improve output with feedback', async () => {
    const provider = createMockProvider(['improved output']);
    const reflector = new LLMReflector(provider);
    const improved = await reflector.improve('original output', {
      issues: [],
      severity: 'low',
      corrections: [{ original: 'original', corrected: 'improved', reason: 'better' }],
    });
    expect(improved).toBe('improved output');
  });

  it('should handle invalid critique response', async () => {
    const provider = createMockProvider(['not json']);
    const reflector = new LLMReflector(provider);
    const critique = await reflector.critique('output');
    expect(critique.issues).toHaveLength(0);
    expect(critique.severity).toBe('medium');
  });
});

// ===== Context compression tests =====
describe('ContextWindow', () => {
  it('should estimate tokens', () => {
    const tokens = estimateTokens([{ role: 'user', content: 'hello world' }]);
    expect(tokens).toBe(3); // ceil(11/4) = 3
  });

  it('should estimate token count', () => {
    expect(estimateTokenCount('hello')).toBe(2); // ceil(5/4) = 2
    expect(estimateTokenCount('')).toBe(0);
  });

  it('should return messages within budget', async () => {
    const cw = new ContextWindow({ maxTokens: 100 });
    const messages = [{ role: 'user', content: 'short' }];
    const result = await cw.manage(messages);
    expect(result).toEqual(messages);
  });

  it('should trim messages exceeding budget', () => {
    const cw = new ContextWindow({ maxTokens: 5 });
    const messages = [
      { role: 'system', content: 'sys' },
      { role: 'user', content: 'a'.repeat(100) },
      { role: 'assistant', content: 'b'.repeat(100) },
      { role: 'user', content: 'recent short' },
    ];
    const result = cw.simpleTrim(messages);
    expect(result.length).toBeLessThan(messages.length);
    expect(result.some(m => m.role === 'system')).toBe(true);
  });

  it('should measure token usage', () => {
    const cw = new ContextWindow({ maxTokens: 100 });
    const messages = [{ role: 'user', content: 'a'.repeat(400) }];
    const measurement = cw.measure(messages);
    expect(measurement.tokens).toBe(100);
    expect(measurement.budget).toBe(100);
    expect(measurement.usage).toBe(100);
  });
});

// ===== MemoryToolLearner tests =====
describe('MemoryToolLearner', () => {
  it('should record success', async () => {
    const memory = createMockMemory();
    const learner = new MemoryToolLearner(memory);
    await learner.recordSuccess('search', '{"query":"test"}', 'found results');
    expect(memory.add).toHaveBeenCalledTimes(1);
  });

  it('should record failure', async () => {
    const memory = createMockMemory();
    const learner = new MemoryToolLearner(memory);
    await learner.recordFailure('search', '{"query":"test"}', 'error occurred');
    expect(memory.add).toHaveBeenCalledTimes(1);
  });

  it('should get best practices', async () => {
    const memory = createMockMemory();
    const learner = new MemoryToolLearner(memory);
    await learner.recordSuccess('search', '{"action":"find","query":"test"}', 'result1');
    await learner.recordSuccess('search', '{"action":"find","query":"test2"}', 'result2');
    await learner.recordFailure('search', '{"action":"find","query":"bad"}', 'error');
    const practices = await learner.getBestPractices('search');
    expect(practices.length).toBeGreaterThan(0);
    expect(practices[0].toolName).toBe('search');
  });

  it('should return empty practices for unknown tool', async () => {
    const memory = createMockMemory();
    const learner = new MemoryToolLearner(memory);
    const practices = await learner.getBestPractices('unknown');
    expect(practices).toHaveLength(0);
  });

  it('should suggest improvement with no data', async () => {
    const memory = createMockMemory();
    const learner = new MemoryToolLearner(memory);
    const suggestion = await learner.suggestImprovement('search', '{"query":"test"}');
    expect(suggestion.confidence).toBe(0);
    expect(suggestion.reason).toContain('No historical data');
  });

  it('should suggest improvement with good pattern', async () => {
    const memory = createMockMemory();
    const learner = new MemoryToolLearner(memory);
    await learner.recordSuccess('search', '{"action":"good"}', 'result');
    const suggestion = await learner.suggestImprovement('search', '{"action":"good"}');
    expect(suggestion.reason).toContain('looks good');
  });

  it('should extract pattern from args', async () => {
    const memory = createMockMemory();
    const learner = new MemoryToolLearner(memory);
    await learner.recordSuccess('tool', '{"method":"GET"}', 'result');
    const practices = await learner.getBestPractices('tool');
    expect(practices[0].pattern).toBe('method:GET');
  });
});

// ===== Visualize tests =====
describe('Visualize', () => {
  function createWorkflow() {
    return {
      nodes: new Map([
        ['start', { id: 'start', name: 'Start', type: 'start' }],
        ['task1', { id: 'task1', name: 'Task 1', type: 'task' }],
        ['cond', { id: 'cond', name: 'Check?', type: 'condition' }],
        ['end', { id: 'end', name: 'End', type: 'end' }],
      ]),
      transitions: [
        { from: 'start', to: 'task1' },
        { from: 'task1', to: 'cond', condition: 'done' },
        { from: 'cond', to: 'end', condition: 'yes' },
      ],
      startNodeID: 'start',
    };
  }

  it('should generate Mermaid diagram', () => {
    const gen = new MermaidGenerator();
    const result = gen.generate(createWorkflow());
    expect(result).toContain('graph TD');
    expect(result).toContain('start');
    expect(result).toContain('-->');
    expect(result).toContain('classDef');
  });

  it('should generate Mermaid with LR direction', () => {
    const gen = new MermaidGenerator();
    const result = gen.generate(createWorkflow(), { direction: 'LR', highlightPath: [], failedNodes: [], showLabels: true });
    expect(result).toContain('graph LR');
  });

  it('should highlight and mark failed nodes', () => {
    const gen = new MermaidGenerator();
    const result = gen.generate(createWorkflow(), {
      direction: 'TD',
      highlightPath: ['task1'],
      failedNodes: ['cond'],
      showLabels: true,
    });
    expect(result).toContain(':::failed');
    expect(result).toContain(':::highlight');
  });

  it('should hide labels when showLabels is false', () => {
    const gen = new MermaidGenerator();
    const result = gen.generate(createWorkflow(), { direction: 'TD', highlightPath: [], failedNodes: [], showLabels: false });
    expect(result).not.toContain('|done|');
  });

  it('should generate DOT diagram', () => {
    const gen = new DOTGenerator();
    const result = gen.generate(createWorkflow());
    expect(result).toContain('digraph workflow');
    expect(result).toContain('rankdir=TB');
    expect(result).toContain('->');
  });

  it('should generate DOT with LR direction', () => {
    const gen = new DOTGenerator();
    const result = gen.generate(createWorkflow(), { direction: 'LR', highlightPath: [], failedNodes: [], showLabels: true });
    expect(result).toContain('rankdir=LR');
  });

  it('should generate DOT with colors', () => {
    const gen = new DOTGenerator();
    const result = gen.generate(createWorkflow(), {
      direction: 'TD',
      highlightPath: ['task1'],
      failedNodes: ['cond'],
      showLabels: true,
    });
    expect(result).toContain('fillcolor=red');
    expect(result).toContain('fillcolor=lightgreen');
  });

  it('should use defaultVisualizeConfig', () => {
    const cfg = defaultVisualizeConfig();
    expect(cfg.direction).toBe('TD');
    expect(cfg.highlightPath).toEqual([]);
    expect(cfg.failedNodes).toEqual([]);
    expect(cfg.showLabels).toBe(true);
  });

  it('should generate all formats via WorkflowVisualizer', () => {
    const viz = new WorkflowVisualizer();
    const wf = createWorkflow();
    expect(viz.toMermaid(wf)).toContain('graph');
    expect(viz.toDOT(wf)).toContain('digraph');
    expect(viz.toJSON(wf)).toContain('"nodes"');
    expect(viz.toHTML(wf)).toContain('<html>');
  });
});

// ===== VisualEditor tests =====
describe('VisualEditor', () => {
  function createSimpleWorkflow() {
    return {
      nodes: new Map([
        ['a', { id: 'a', name: 'A', type: 'task' }],
      ]),
      transitions: [],
      startNodeID: 'a',
    };
  }

  it('should add node', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    expect(editor.getWorkflow().nodes.has('b')).toBe(true);
  });

  it('should remove node', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    editor.removeNode('b');
    expect(editor.getWorkflow().nodes.has('b')).toBe(false);
  });

  it('should remove connected edges when removing node', () => {
    const wf = createSimpleWorkflow();
    const editor = new VisualEditor(wf);
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    editor.addEdge({ from: 'a', to: 'b' });
    editor.removeNode('b');
    expect(editor.getWorkflow().transitions).toHaveLength(0);
  });

  it('should add edge', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    editor.addEdge({ from: 'a', to: 'b' });
    expect(editor.getWorkflow().transitions).toHaveLength(1);
  });

  it('should remove edge', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    editor.addEdge({ from: 'a', to: 'b' });
    editor.removeEdge('a', 'b');
    expect(editor.getWorkflow().transitions).toHaveLength(0);
  });

  it('should update node', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.updateNode('a', { name: 'Updated' });
    expect(editor.getWorkflow().nodes.get('a')?.name).toBe('Updated');
  });

  it('should undo add node', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    expect(editor.undo()).toBe(true);
    expect(editor.getWorkflow().nodes.has('b')).toBe(false);
  });

  it('should redo add node', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    editor.undo();
    expect(editor.redo()).toBe(true);
    expect(editor.getWorkflow().nodes.has('b')).toBe(true);
  });

  it('should undo remove node', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.removeNode('a');
    editor.undo();
    expect(editor.getWorkflow().nodes.has('a')).toBe(true);
  });

  it('should undo add edge', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.addNode({ id: 'b', name: 'B', type: 'task' });
    editor.addEdge({ from: 'a', to: 'b' });
    editor.undo();
    expect(editor.getWorkflow().transitions).toHaveLength(0);
  });

  it('should undo update node', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    editor.updateNode('a', { name: 'Updated' });
    editor.undo();
    expect(editor.getWorkflow().nodes.get('a')?.name).toBe('A');
  });

  it('should return false when nothing to undo', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    expect(editor.undo()).toBe(false);
  });

  it('should return false when nothing to redo', () => {
    const editor = new VisualEditor(createSimpleWorkflow());
    expect(editor.redo()).toBe(false);
  });
});

// ===== AgentTool tests =====
describe('AgentTool', () => {
  it('should create tool with agent name', () => {
    const agent = createMockAgent('math', []);
    const tool = new AgentTool(agent);
    expect(tool.name).toBe('agent_math');
    expect(tool.description).toContain('math');
    expect(tool.parameters).toBeDefined();
  });

  it('should use custom description', () => {
    const agent = createMockAgent('test', []);
    const tool = new AgentTool(agent, { description: 'Custom description' });
    expect(tool.description).toBe('Custom description');
  });

  it('should use custom param schema', () => {
    const agent = createMockAgent('test', []);
    const tool = new AgentTool(agent, { paramSchema: { type: 'object' } });
    expect(tool.parameters).toEqual({ type: 'object' });
  });

  it('should execute and return agent response', async () => {
    const agent = createMockAgent('test', ['agent response']);
    const tool = new AgentTool(agent);
    const result = await tool.execute({ input: 'do something' });
    expect(result).toBe('agent response');
  });

  it('should throw when input is missing', async () => {
    const agent = createMockAgent('test', []);
    const tool = new AgentTool(agent);
    await expect(tool.execute({})).rejects.toThrow("缺少必需参数 'input'");
  });

  it('should throw when input is empty', async () => {
    const agent = createMockAgent('test', []);
    const tool = new AgentTool(agent);
    await expect(tool.execute({ input: '  ' })).rejects.toThrow("缺少必需参数 'input'");
  });

  it('should wrap agent errors', async () => {
    const agent = {
      name: 'fail-agent',
      run: vi.fn().mockRejectedValue(new Error('agent failed')),
    };
    const tool = new AgentTool(agent as any);
    await expect(tool.execute({ input: 'test' })).rejects.toThrow('子 Agent [fail-agent] 执行失败: agent failed');
  });
});
