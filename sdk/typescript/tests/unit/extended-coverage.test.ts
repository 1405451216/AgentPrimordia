import { describe, it, expect, vi } from 'vitest';
import {
  basicAgentDeployment,
  multiAgentDeployment,
  withAutoscaling,
  withHealthCheck,
  withMetrics,
  withTracing,
  toYAML,
} from '../../src/operator/crd.js';
import {
  ZeroCopyMessage,
  batchConvertToZeroCopy,
  ZeroCopyPool,
  StringBuilder,
  ByteBufferPool,
  PricingCalculator,
  defaultPricingTable,
} from '../../src/utils/zerocopy-pricing.js';
import { DynamicOrchestrator, Scheduler, StepExecutor } from '../../src/orchestration/extended.js';

// ===== CRD tests =====
describe('CRD Helpers', () => {
  it('basicAgentDeployment should create deployment', () => {
    const dep = basicAgentDeployment('my-agent', {
      provider: 'openai',
      model: 'gpt-4o',
      systemPrompt: 'You are helpful',
    });
    expect(dep.apiVersion).toBe('agentprimordia.io/v1');
    expect(dep.kind).toBe('AgentDeployment');
    expect(dep.metadata.name).toBe('my-agent');
    expect(dep.spec.replicas).toBe(1);
    expect(dep.spec.template.provider).toBe('openai');
    expect(dep.spec.template.model).toBe('gpt-4o');
  });

  it('basicAgentDeployment should use custom replicas', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai',
      model: 'gpt-4o',
      systemPrompt: 'test',
      replicas: 3,
    });
    expect(dep.spec.replicas).toBe(3);
  });

  it('basicAgentDeployment should include apiSecretRef', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai',
      model: 'gpt-4o',
      systemPrompt: 'test',
      apiSecretRef: 'my-secret',
    });
    expect(dep.spec.template.apiSecretRef).toBe('my-secret');
  });

  it('multiAgentDeployment should create multiple deployments', () => {
    const deps = multiAgentDeployment('group', [
      { name: 'agent-1', provider: 'openai', model: 'gpt-4o', systemPrompt: 'a' },
      { name: 'agent-2', provider: 'anthropic', model: 'claude-3', systemPrompt: 'b' },
    ]);
    expect(deps).toHaveLength(2);
    expect(deps[0].metadata.name).toBe('agent-1');
    expect(deps[1].metadata.name).toBe('agent-2');
  });

  it('withAutoscaling should add autoscaling config', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withAutoscaling(dep, { minReplicas: 2, maxReplicas: 20 });
    expect(result.spec.autoscaling).toBeDefined();
    expect(result.spec.autoscaling!.enabled).toBe(true);
    expect(result.spec.autoscaling!.minReplicas).toBe(2);
    expect(result.spec.autoscaling!.maxReplicas).toBe(20);
  });

  it('withAutoscaling should use defaults', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withAutoscaling(dep, {});
    expect(result.spec.autoscaling!.minReplicas).toBe(1);
    expect(result.spec.autoscaling!.maxReplicas).toBe(10);
  });

  it('withHealthCheck should add health check config', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withHealthCheck(dep);
    expect(result.spec.healthCheck).toBeDefined();
    expect(result.spec.healthCheck!.enabled).toBe(true);
    expect(result.spec.healthCheck!.interval).toBe(30);
  });

  it('withHealthCheck should use custom config', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withHealthCheck(dep, { interval: 60, timeout: 10 });
    expect(result.spec.healthCheck!.interval).toBe(60);
    expect(result.spec.healthCheck!.timeout).toBe(10);
  });

  it('withMetrics should add metrics config', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withMetrics(dep);
    expect(result.spec.template.metrics).toBeDefined();
    expect(result.spec.template.metrics!.enabled).toBe(true);
    expect(result.spec.template.metrics!.port).toBe(9090);
  });

  it('withMetrics should use custom config', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withMetrics(dep, { port: 8080, path: '/m' });
    expect(result.spec.template.metrics!.port).toBe(8080);
    expect(result.spec.template.metrics!.path).toBe('/m');
  });

  it('withTracing should add tracing config', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withTracing(dep);
    expect(result.spec.template.tracing).toBeDefined();
    expect(result.spec.template.tracing!.enabled).toBe(true);
    expect(result.spec.template.tracing!.serviceName).toBe('agent');
  });

  it('withTracing should use custom config', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const result = withTracing(dep, { endpoint: 'http://jaeger:4317', samplingRate: 0.5 });
    expect(result.spec.template.tracing!.endpoint).toBe('http://jaeger:4317');
    expect(result.spec.template.tracing!.samplingRate).toBe(0.5);
  });

  it('toYAML should produce YAML string', () => {
    const dep = basicAgentDeployment('agent', {
      provider: 'openai', model: 'gpt-4o', systemPrompt: 'test',
    });
    const yaml = toYAML(dep);
    expect(yaml).toContain('apiVersion:');
    expect(yaml).toContain('kind:');
    expect(yaml).toContain('AgentDeployment');
    expect(yaml).toContain('name: agent');
  });

  it('toYAML should handle arrays', () => {
    const deps = multiAgentDeployment('group', [
      { name: 'a', provider: 'openai', model: 'gpt-4o', systemPrompt: 'test' },
    ]);
    const yaml = toYAML(deps[0]!);
    expect(yaml).toContain('name: a');
  });
});

// ===== ZeroCopyMessage tests =====
describe('ZeroCopyMessage', () => {
  it('should create message', () => {
    const msg = new ZeroCopyMessage('user', 'hello');
    expect(msg.content()).toBe('hello');
    expect(msg.role).toBe('user');
  });

  it('should convert to Message', () => {
    const msg = new ZeroCopyMessage('system', 'prompt');
    const m = msg.toMessage();
    expect(m).toEqual({ role: 'system', content: 'prompt' });
  });

  it('should append content', () => {
    const msg = new ZeroCopyMessage('user', 'hello');
    msg.append(' world');
    expect(msg.content()).toBe('hello world');
  });

  it('should prepend content', () => {
    const msg = new ZeroCopyMessage('user', 'world');
    msg.prepend('hello ');
    expect(msg.content()).toBe('hello world');
  });

  it('should get length without slices', () => {
    const msg = new ZeroCopyMessage('user', 'hello');
    expect(msg.length()).toBe(5);
  });

  it('should get length with slices', () => {
    const msg = new ZeroCopyMessage('user', 'hello');
    msg.append(' world');
    expect(msg.length()).toBe(11);
  });

  it('should handle multiple appends', () => {
    const msg = new ZeroCopyMessage('user', 'a');
    msg.append('b');
    msg.append('c');
    expect(msg.content()).toBe('abc');
  });
});

describe('batchConvertToZeroCopy', () => {
  it('should convert array of strings', () => {
    const msgs = batchConvertToZeroCopy('user', ['a', 'b', 'c']);
    expect(msgs).toHaveLength(3);
    expect(msgs[0].content()).toBe('a');
    expect(msgs[1].content()).toBe('b');
    expect(msgs[2].content()).toBe('c');
  });

  it('should handle empty array', () => {
    const msgs = batchConvertToZeroCopy('user', []);
    expect(msgs).toHaveLength(0);
  });
});

describe('ZeroCopyPool', () => {
  it('should acquire and release', () => {
    const pool = new ZeroCopyPool(10);
    const msg = pool.acquire('user', 'test');
    expect(msg.content()).toBe('test');
    pool.release(msg);
    expect(pool.size).toBe(1);
  });

  it('should reuse pooled objects', () => {
    const pool = new ZeroCopyPool(10);
    const msg1 = pool.acquire('user', 'first');
    pool.release(msg1);
    const msg2 = pool.acquire('assistant', 'second');
    expect(msg2).toBe(msg1); // Same object
    expect(msg2.role).toBe('assistant');
    expect(msg2.content()).toBe('second');
  });

  it('should create new when pool empty', () => {
    const pool = new ZeroCopyPool(10);
    const msg = pool.acquire('user', 'test');
    expect(msg).toBeDefined();
    expect(pool.size).toBe(0);
  });

  it('should not exceed max size', () => {
    const pool = new ZeroCopyPool(2);
    const msg1 = pool.acquire('user', 'a');
    const msg2 = pool.acquire('user', 'b');
    const msg3 = pool.acquire('user', 'c');
    pool.release(msg1);
    pool.release(msg2);
    pool.release(msg3);
    expect(pool.size).toBe(2);
  });
});

describe('StringBuilder', () => {
  it('should append strings', () => {
    const sb = new StringBuilder();
    sb.append('hello').append(' world');
    expect(sb.toString()).toBe('hello world');
  });

  it('should append lines', () => {
    const sb = new StringBuilder();
    sb.appendLine('line1').appendLine('line2');
    expect(sb.toString()).toBe('line1\nline2\n');
  });

  it('should append empty line', () => {
    const sb = new StringBuilder();
    sb.appendLine();
    expect(sb.toString()).toBe('\n');
  });

  it('should cache toString result', () => {
    const sb = new StringBuilder();
    sb.append('test');
    const first = sb.toString();
    const second = sb.toString();
    expect(first).toBe(second);
  });

  it('should clear', () => {
    const sb = new StringBuilder();
    sb.append('test');
    sb.clear();
    expect(sb.toString()).toBe('');
    expect(sb.isEmpty()).toBe(true);
  });

  it('should get length', () => {
    const sb = new StringBuilder();
    sb.append('hello').append(' world');
    expect(sb.length).toBe(11);
  });

  it('should check isEmpty', () => {
    const sb = new StringBuilder();
    expect(sb.isEmpty()).toBe(true);
    sb.append('test');
    expect(sb.isEmpty()).toBe(false);
  });

  it('should handle empty string append', () => {
    const sb = new StringBuilder();
    sb.append('');
    expect(sb.isEmpty()).toBe(true);
  });
});

describe('ByteBufferPool', () => {
  it('should acquire buffer', () => {
    const pool = new ByteBufferPool(1024, 10);
    const buf = pool.acquire();
    expect(buf.length).toBe(1024);
  });

  it('should release and reuse', () => {
    const pool = new ByteBufferPool(1024, 10);
    const buf1 = pool.acquire();
    buf1.write('test', 0);
    pool.release(buf1);
    const buf2 = pool.acquire();
    expect(buf2).toBe(buf1);
    // Buffer should be zeroed
    expect(buf2[0]).toBe(0);
  });

  it('should not exceed max pool size', () => {
    const pool = new ByteBufferPool(1024, 2);
    const bufs = [pool.acquire(), pool.acquire(), pool.acquire()];
    for (const buf of bufs) pool.release(buf);
    expect(pool.size).toBe(2);
  });

  it('should not pool different-sized buffers', () => {
    const pool = new ByteBufferPool(1024, 10);
    const buf = Buffer.alloc(512);
    pool.release(buf);
    expect(pool.size).toBe(0);
  });
});

describe('PricingCalculator', () => {
  it('should calculate cost for known model', () => {
    const calc = new PricingCalculator();
    const cost = calc.calculate('gpt-4o', 1000, 500);
    // gpt-4o: $2.50/1M prompt, $10.00/1M completion
    // 1000/1M * 2.5 + 500/1M * 10 = 0.0025 + 0.005 = 0.0075
    expect(cost).toBeCloseTo(0.0075, 5);
  });

  it('should return 0 for unknown model', () => {
    const calc = new PricingCalculator();
    expect(calc.calculate('unknown-model', 1000, 500)).toBe(0);
  });

  it('should set custom pricing', () => {
    const calc = new PricingCalculator();
    calc.setPricing({
      model: 'custom-model',
      provider: 'test',
      promptPricePer1M: 1.0,
      completionPricePer1M: 2.0,
    });
    const pricing = calc.getPricing('custom-model');
    expect(pricing).toBeDefined();
    expect(pricing!.promptPricePer1M).toBe(1.0);
  });

  it('should get pricing for known model', () => {
    const calc = new PricingCalculator();
    const pricing = calc.getPricing('gpt-4o');
    expect(pricing).toBeDefined();
    expect(pricing!.provider).toBe('openai');
  });

  it('should return undefined for unknown pricing', () => {
    const calc = new PricingCalculator();
    expect(calc.getPricing('unknown')).toBeUndefined();
  });

  it('should list models', () => {
    const calc = new PricingCalculator();
    const models = calc.listModels();
    expect(models.length).toBeGreaterThan(5);
    expect(models).toContain('gpt-4o');
  });

  it('should use custom table', () => {
    const customTable = new Map([
      ['custom', { model: 'custom', provider: 'test', promptPricePer1M: 1, completionPricePer1M: 2 }],
    ]);
    const calc = new PricingCalculator(customTable);
    expect(calc.calculate('custom', 1000000, 500000)).toBeCloseTo(2.0, 5);
    expect(calc.getPricing('gpt-4o')).toBeUndefined();
  });
});

describe('defaultPricingTable', () => {
  it('should return map with known models', () => {
    const table = defaultPricingTable();
    expect(table.size).toBeGreaterThan(10);
    expect(table.has('gpt-4o')).toBe(true);
    expect(table.has('claude-3-5-sonnet-20241022')).toBe(true);
  });
});

// ===== DynamicOrchestrator tests =====
describe('DynamicOrchestrator', () => {
  it('should register agents', () => {
    const orch = new DynamicOrchestrator();
    const mockAgent = { run: vi.fn().mockResolvedValue({ content: 'response' }) } as unknown;
    orch.registerAgent('agent-1', mockAgent as any);
    expect(orch.getAgentNames()).toEqual(['agent-1']);
  });

  it('should add and sort routes by priority', () => {
    const orch = new DynamicOrchestrator();
    orch.addRoute({ name: 'low', match: () => true, agentName: 'a', priority: 1 });
    orch.addRoute({ name: 'high', match: () => true, agentName: 'b', priority: 10 });
    const routes = orch.getRoutes();
    expect(routes[0].name).toBe('high');
    expect(routes[1].name).toBe('low');
  });

  it('should set default agent', () => {
    const orch = new DynamicOrchestrator();
    const mockAgent = { run: vi.fn().mockResolvedValue({ content: 'default response' }) } as unknown;
    orch.registerAgent('default', mockAgent as any);
    orch.setDefault('default');
  });

  it('should route to matching agent', async () => {
    const orch = new DynamicOrchestrator();
    const mockAgent = { run: vi.fn().mockResolvedValue({ content: 'matched' }) } as unknown;
    orch.registerAgent('agent-1', mockAgent as any);
    orch.addRoute({ name: 'route1', match: (input) => input.includes('hello'), agentName: 'agent-1', priority: 1 });
    const result = await orch.route('hello world');
    expect(result).toBe('matched');
  });

  it('should fall back to default agent', async () => {
    const orch = new DynamicOrchestrator();
    const mockAgent = { run: vi.fn().mockResolvedValue({ content: 'default' }) } as unknown;
    orch.registerAgent('default', mockAgent as any);
    orch.setDefault('default');
    const result = await orch.route('no match');
    expect(result).toBe('default');
  });

  it('should throw when no route matches and no default', async () => {
    const orch = new DynamicOrchestrator();
    await expect(orch.route('test')).rejects.toThrow('No matching route');
  });

  it('should skip route if agent not found', async () => {
    const orch = new DynamicOrchestrator();
    orch.addRoute({ name: 'route1', match: () => true, agentName: 'missing', priority: 1 });
    await expect(orch.route('test')).rejects.toThrow();
  });
});

// ===== Scheduler tests =====
describe('Scheduler', () => {
  it('should submit tasks', () => {
    const scheduler = new Scheduler(3);
    const id = scheduler.submit('task1', async () => 'result');
    expect(id).toBeDefined();
    expect(scheduler.getTask(id)).toBeDefined();
    expect(scheduler.getPendingCount()).toBe(1);
  });

  it('should execute tasks', async () => {
    const scheduler = new Scheduler(3);
    const id = scheduler.submit('task1', async () => 'done');
    scheduler.start();
    await scheduler.waitAll();
    scheduler.stop();
    const task = scheduler.getTask(id);
    expect(task!.status).toBe('completed');
    expect(task!.result).toBe('done');
  });

  it('should handle task failures', async () => {
    const scheduler = new Scheduler(3);
    const id = scheduler.submit('fail', async () => { throw new Error('task error'); });
    scheduler.start();
    await scheduler.waitAll();
    scheduler.stop();
    const task = scheduler.getTask(id);
    expect(task!.status).toBe('failed');
    expect(task!.error).toBeDefined();
  });

  it('should get stats', () => {
    const scheduler = new Scheduler(3);
    scheduler.submit('t1', async () => 'a');
    scheduler.submit('t2', async () => 'b');
    const stats = scheduler.getStats();
    expect(stats.total).toBe(2);
    expect(stats.pending).toBe(2);
  });

  it('should stop processing', () => {
    const scheduler = new Scheduler(1);
    scheduler.stop();
    // Should not throw
  });

  it('should use default maxConcurrent', () => {
    const scheduler = new Scheduler();
    expect(scheduler.getStats().total).toBe(0);
  });
});

// ===== StepExecutor tests =====
describe('StepExecutor', () => {
  it('should add steps and set start', () => {
    const executor = new StepExecutor();
    executor.addStep({
      id: 'step1',
      name: 'First',
      type: 'llm',
      config: { prompt: 'Hello' },
    });
    expect(executor).toBeDefined();
  });

  it('should set start step', () => {
    const executor = new StepExecutor();
    executor.addStep({ id: 's1', name: 'S1', type: 'wait', config: { waitMs: 1 } });
    executor.addStep({ id: 's2', name: 'S2', type: 'wait', config: { waitMs: 1 } });
    executor.setStart('s2');
  });

  it('should throw when no start step', async () => {
    const executor = new StepExecutor();
    await expect(executor.execute('input')).rejects.toThrow('No start step');
  });

  it('should execute wait step', async () => {
    const executor = new StepExecutor();
    executor.addStep({
      id: 'wait1',
      name: 'Wait',
      type: 'wait',
      config: { waitMs: 10 },
    });
    const { results, finalOutput } = await executor.execute('test input');
    expect(results).toHaveLength(1);
    expect(results[0].status).toBe('completed');
  });

  it('should execute sequential steps', async () => {
    const executor = new StepExecutor();
    executor.addStep({
      id: 's1',
      name: 'Step 1',
      type: 'wait',
      config: { waitMs: 1 },
      next: ['s2'],
    });
    executor.addStep({
      id: 's2',
      name: 'Step 2',
      type: 'wait',
      config: { waitMs: 1 },
    });
    const { results } = await executor.execute('input');
    expect(results).toHaveLength(2);
  });

  it('should handle conditional step', async () => {
    const executor = new StepExecutor();
    executor.addStep({
      id: 'cond',
      name: 'Condition',
      type: 'conditional',
      config: {
        condition: (result: string) => result.includes('yes'),
        trueStep: 'true-branch',
        falseStep: 'false-branch',
      },
    });
    executor.addStep({ id: 'true-branch', name: 'True', type: 'wait', config: { waitMs: 1 } });
    executor.addStep({ id: 'false-branch', name: 'False', type: 'wait', config: { waitMs: 1 } });

    const { results } = await executor.execute('yes');
    expect(results).toHaveLength(2);
    expect(results[1].stepId).toBe('true-branch');
  });

  it('should handle tool step with getTool', async () => {
    const executor = new StepExecutor();
    executor.addStep({
      id: 'tool1',
      name: 'Tool Step',
      type: 'tool',
      config: { toolName: 'echo', toolArgs: { text: 'hello' } },
    });

    const mockTool = {
      execute: vi.fn().mockResolvedValue('tool result'),
    };
    const { results } = await executor.execute('input', {
      getTool: () => mockTool,
    });
    expect(results[0].status).toBe('completed');
    expect(results[0].output).toBe('tool result');
  });

  it('should handle error with onError', async () => {
    const executor = new StepExecutor();
    executor.addStep({
      id: 'fail-step',
      name: 'Fail',
      type: 'tool',
      config: { toolName: 'bad' },
      onError: 'recovery',
    });
    executor.addStep({ id: 'recovery', name: 'Recovery', type: 'wait', config: { waitMs: 1 } });

    const { results } = await executor.execute('input', {
      getTool: () => ({ execute: async () => { throw new Error('tool error'); } }),
    });
    expect(results.some(r => r.status === 'failed')).toBe(true);
    expect(results.some(r => r.stepId === 'recovery')).toBe(true);
  });
});
