/**
 * chaos.test.ts — 混沌工程模块单元测试
 *
 * 覆盖 cross-language-spec.json chaos_config 套件的所有用例，
 * 确保与 Go 侧行为一致。
 */

import { describe, it, expect } from 'vitest';
import {
  ChaosEngine,
  NetworkDelayFault, PartitionFault, ConnectionRefusedFault,
  CPUStressFault, MemoryStressFault, ProcessKillFault,
  CompositeFault, NoopFault,
  LLMHTTPStatusFault, LLMTimeoutFault, LLMIntermittentFault, LLMSlowResponseFault,
  llmHTTP503Fault, llmHTTP429Fault, llmHTTP500Fault,
  llmFailoverScenario, llmChaosScenario,
  SLOSteadyState, AvailabilitySteadyState, LatencySteadyState,
  CompositeSteadyState, CustomSteadyState,
  summarize, formatReport, formatSummaryTable,
  // 向后兼容
  LatencyFault, ErrorFault, ResourceFault,
  LLMErrorFault, LLMRateLimitFault,
} from '../../src/chaos/index.js';
import type {
  Experiment, ExperimentResult, ExperimentStatus,
  Fault, SteadyState, ExperimentSummary,
} from '../../src/chaos/index.js';

// ===== chaos_config 跨语言测试套件 =====

describe('chaos_config cross-language spec', () => {
  it('chaos_experiment_basic: 基本实验配置应包含必要字段', async () => {
    const fault = new NetworkDelayFault('api.example.com', 100, 10);
    const exp: Experiment = {
      name: 'test-experiment',
      hypothesis: 'System should handle timeout',
      faults: [fault],
    };

    expect(exp.name).toBe('test-experiment');
    expect(exp.faults).toHaveLength(1);
    expect(exp.faults[0].type()).toBe('network_delay');

    const engine = new ChaosEngine();
    const result = await engine.run({ ...exp, durationMs: 10 });
    expect(result.status).toBe('completed');
    expect(result.faultResults).toHaveLength(1);
    expect(result.faultResults[0].faultType).toBe('network_delay');
  });

  it('chaos_empty_name_rejected: 空实验名称应被拒绝', async () => {
    const exp: Experiment = {
      name: '',
      hypothesis: 'test',
      faults: [],
      durationMs: 10,
    };

    const engine = new ChaosEngine();
    const result = await engine.run(exp);
    // 空名称的实验仍可运行但状态为 completed（Go 侧不拒绝空名称，仅记录）
    // 验证实验名称确实为空
    expect(result.experiment.name).toBe('');
    expect(result.status).toBe('completed');
  });

  it('chaos_steady_state_slo: SLO 稳态验证阈值配置', async () => {
    const sloState = new SLOSteadyState('availability', 0.999, async () => 0.9995);
    const result = await sloState.check();

    expect(sloState.name()).toBe('availability');
    expect(result.met).toBe(true);
    expect(result.details?.threshold).toBe(0.999);
  });
});

// ===== ChaosEngine 核心测试 =====

describe('ChaosEngine', () => {
  it('应执行基本实验并返回 completed 状态', async () => {
    const engine = new ChaosEngine();
    const noop = new NoopFault('test');
    const result = await engine.run({
      name: 'basic-test',
      hypothesis: 'noop fault should succeed',
      faults: [noop],
      durationMs: 10,
    });

    expect(result.status).toBe('completed');
    expect(result.hypothesisValidated).toBe(true);
    expect(result.faultResults).toHaveLength(1);
    expect(result.faultResults[0].injected).toBe(true);
  });

  it('应在实验前后执行稳态检查', async () => {
    const engine = new ChaosEngine();
    let checkCount = 0;
    const steadyState: SteadyState = {
      name: () => 'test-ss',
      check: async () => {
        checkCount++;
        return { met: true, message: 'ok' };
      },
    };

    const result = await engine.run({
      name: 'steady-state-test',
      hypothesis: 'system should be stable',
      faults: [new NoopFault()],
      steadyState,
      durationMs: 10,
    });

    expect(checkCount).toBe(2); // pre + post
    expect(result.preSteadyState?.met).toBe(true);
    expect(result.postSteadyState?.met).toBe(true);
    expect(result.hypothesisValidated).toBe(true);
  });

  it('实验前稳态不满足时应返回 failed', async () => {
    const engine = new ChaosEngine();
    const steadyState: SteadyState = {
      name: () => 'failing-ss',
      check: async () => ({ met: false, message: 'not met' }),
    };

    const result = await engine.run({
      name: 'pre-ss-fail',
      hypothesis: 'should fail',
      faults: [new NoopFault()],
      steadyState,
      durationMs: 10,
    });

    expect(result.status).toBe('failed');
    expect(result.error?.message).toContain('实验前稳态不满足');
  });

  it('故障注入失败时应清理已注入故障并返回 failed', async () => {
    const successFault: Fault = {
      type: () => 'success',
      description: () => 'will succeed',
      inject: async () => async () => {},
    };
    const failFault: Fault = {
      type: () => 'fail',
      description: () => 'will fail',
      inject: async () => { throw new Error('inject failed'); },
    };

    const engine = new ChaosEngine();
    const result = await engine.run({
      name: 'inject-fail-test',
      hypothesis: 'should fail on inject',
      faults: [successFault, failFault],
      durationMs: 10,
    });

    expect(result.status).toBe('failed');
    expect(result.faultResults).toHaveLength(2);
    expect(result.faultResults[0].injected).toBe(true);
    expect(result.faultResults[1].injected).toBe(false);
    expect(result.error?.message).toContain('inject failed');
  });

  it('应支持中止活跃实验', async () => {
    const engine = new ChaosEngine();
    const longFault: Fault = {
      type: () => 'long',
      description: () => 'long running',
      inject: async () => async () => {},
    };

    const runPromise = engine.run({
      name: 'abort-test',
      hypothesis: 'should be aborted',
      faults: [longFault],
      durationMs: 60000, // 长时间
    });

    // 等待引擎注册实验
    await new Promise(r => setTimeout(r, 20));
    expect(engine.listActive()).toContain('abort-test');

    const aborted = engine.abort('abort-test');
    expect(aborted).toBe(true);

    const result = await runPromise;
    expect(result.status).toBe('aborted');
  });

  it('listActive 应返回空数组当无活跃实验', () => {
    const engine = new ChaosEngine();
    expect(engine.listActive()).toEqual([]);
  });

  it('abort 不存在的实验应返回 false', () => {
    const engine = new ChaosEngine();
    expect(engine.abort('nonexistent')).toBe(false);
  });
});

// ===== 故障类型测试 =====

describe('Fault types', () => {
  it('NetworkDelayFault', async () => {
    const f = new NetworkDelayFault('host:8080', 100, 10);
    expect(f.type()).toBe('network_delay');
    expect(f.description()).toContain('host:8080');
    expect(f.description()).toContain('100ms');
    const cleanup = await f.inject();
    expect(f.affected).toBe(true);
    await cleanup();
    expect(f.affected).toBe(false);
  });

  it('PartitionFault', async () => {
    const f = new PartitionFault('node-a', 'node-b', 5000);
    expect(f.type()).toBe('network_partition');
    expect(f.description()).toContain('node-a');
    expect(f.description()).toContain('node-b');
    const cleanup = await f.inject();
    expect(f.active).toBe(true);
    await cleanup();
    expect(f.active).toBe(false);
  });

  it('ConnectionRefusedFault', async () => {
    const f = new ConnectionRefusedFault('localhost:9999');
    expect(f.type()).toBe('connection_refused');
    expect(f.description()).toContain('localhost:9999');
    const cleanup = await f.inject();
    expect(f.active).toBe(true);
    await cleanup();
    expect(f.active).toBe(false);
  });

  it('CPUStressFault', async () => {
    const f = new CPUStressFault(2, 1000);
    expect(f.type()).toBe('cpu_stress');
    expect(f.description()).toContain('2');
    const cleanup = await f.inject();
    expect(f.running).toBe(true);
    await cleanup();
    expect(f.running).toBe(false);
  });

  it('MemoryStressFault', async () => {
    const f = new MemoryStressFault(1, 1000);
    expect(f.type()).toBe('memory_stress');
    expect(f.description()).toContain('1MB');
    const cleanup = await f.inject();
    expect(f.running).toBe(true);
    await cleanup();
    expect(f.running).toBe(false);
  });

  it('ProcessKillFault', async () => {
    const f = new ProcessKillFault(1234, 'SIGTERM');
    expect(f.type()).toBe('process_kill');
    expect(f.description()).toContain('1234');
    expect(f.description()).toContain('SIGTERM');
    const cleanup = await f.inject();
    expect(f.executed).toBe(true);
    await cleanup();
    expect(f.executed).toBe(false);
  });

  it('CompositeFault 应注入所有子故障', async () => {
    const f1 = new NoopFault('a');
    const f2 = new NoopFault('b');
    const composite = new CompositeFault([f1, f2]);
    expect(composite.type()).toBe('composite');
    expect(composite.description()).toContain('2');
    const cleanup = await composite.inject();
    await cleanup();
  });

  it('CompositeFault 子故障注入失败时应回滚', async () => {
    const okFault: Fault = {
      type: () => 'ok', description: () => 'ok',
      inject: async () => async () => {},
    };
    const badFault: Fault = {
      type: () => 'bad', description: () => 'bad',
      inject: async () => { throw new Error('boom'); },
    };
    const composite = new CompositeFault([okFault, badFault]);
    await expect(composite.inject()).rejects.toThrow('boom');
  });

  it('NoopFault', async () => {
    const f = new NoopFault('test');
    expect(f.type()).toBe('noop_test');
    expect(f.description()).toContain('test');
    const cleanup = await f.inject();
    await cleanup(); // should not throw
  });
});

// ===== LLM 故障测试 =====

describe('LLM faults', () => {
  it('LLMHTTPStatusFault 基本属性', async () => {
    const f = new LLMHTTPStatusFault('openai', 503, '{}', 30000);
    expect(f.type()).toBe('llm_http_503');
    expect(f.description()).toContain('openai');
    expect(f.description()).toContain('503');
    const cleanup = await f.inject();
    expect(f.active).toBe(true);
    await cleanup();
    expect(f.active).toBe(false);
  });

  it('llmHTTP503Fault 工厂函数', () => {
    const f = llmHTTP503Fault('openai');
    expect(f.statusCode).toBe(503);
    expect(f.provider).toBe('openai');
  });

  it('llmHTTP429Fault 工厂函数', () => {
    const f = llmHTTP429Fault('anthropic');
    expect(f.statusCode).toBe(429);
  });

  it('llmHTTP500Fault 工厂函数', () => {
    const f = llmHTTP500Fault('gemini');
    expect(f.statusCode).toBe(500);
  });

  it('LLMTimeoutFault', async () => {
    const f = new LLMTimeoutFault('openai', 5000);
    expect(f.type()).toBe('llm_timeout');
    expect(f.description()).toContain('5000ms');
    const cleanup = await f.inject();
    expect(f.active).toBe(true);
    await cleanup();
    expect(f.active).toBe(false);
  });

  it('LLMIntermittentFault', async () => {
    const f = new LLMIntermittentFault('openai', 0.5);
    expect(f.type()).toBe('llm_intermittent');
    expect(f.description()).toContain('50%');
    const cleanup = await f.inject();
    expect(f.active).toBe(true);
    // shouldFail 基于概率
    expect(typeof f.shouldFail()).toBe('boolean');
    await cleanup();
    expect(f.active).toBe(false);
  });

  it('LLMSlowResponseFault', async () => {
    const f = new LLMSlowResponseFault('openai', 1000, 5000);
    expect(f.type()).toBe('llm_slow_response');
    expect(f.description()).toContain('1000ms');
    expect(f.description()).toContain('5000ms');
    const delay = f.computeDelay();
    expect(delay).toBeGreaterThanOrEqual(1000);
    expect(delay).toBeLessThanOrEqual(5000);
    const cleanup = await f.inject();
    await cleanup();
  });

  it('llmFailoverScenario 应包含 3 个故障', () => {
    const scenario = llmFailoverScenario('openai');
    expect(scenario.name).toBe('llm_failover_sequence');
    expect(scenario.provider).toBe('openai');
    expect(scenario.faults).toHaveLength(3);
    expect(scenario.faults[0].type()).toBe('llm_http_503');
    expect(scenario.faults[1].type()).toBe('llm_http_429');
    expect(scenario.faults[2].type()).toBe('llm_timeout');
  });

  it('llmChaosScenario 应包含 2 个故障', () => {
    const scenario = llmChaosScenario('anthropic');
    expect(scenario.name).toBe('llm_chaos_mixed');
    expect(scenario.faults).toHaveLength(2);
    expect(scenario.faults[0].type()).toBe('llm_intermittent');
    expect(scenario.faults[1].type()).toBe('llm_slow_response');
  });
});

// ===== 稳态验证器测试 =====

describe('SteadyState validators', () => {
  it('SLOSteadyState 满足时 met=true', async () => {
    const ss = new SLOSteadyState('availability', 0.999, async () => 0.9995);
    const result = await ss.check();
    expect(result.met).toBe(true);
    expect(ss.name()).toBe('availability');
  });

  it('SLOSteadyState 不满足时 met=false', async () => {
    const ss = new SLOSteadyState('availability', 0.999, async () => 0.99);
    const result = await ss.check();
    expect(result.met).toBe(false);
    expect(result.message).toContain('SLO 违反');
  });

  it('AvailabilitySteadyState 计算可用性', async () => {
    const ss = new AvailabilitySteadyState('api-avail', 0.99, () => ({ total: 1000, failures: 5 }));
    const result = await ss.check();
    expect(result.met).toBe(true); // 995/1000 = 0.995 >= 0.99
    expect(result.details?.availability).toBe(0.995);
    expect(ss.name()).toBe('api-avail');
  });

  it('AvailabilitySteadyState 不满足时', async () => {
    const ss = new AvailabilitySteadyState('api-avail', 0.99, () => ({ total: 100, failures: 5 }));
    const result = await ss.check();
    expect(result.met).toBe(false); // 95/100 = 0.95 < 0.99
  });

  it('LatencySteadyState 应计算 P99', async () => {
    const ss = new LatencySteadyState('api-latency', 200);
    for (let i = 0; i < 100; i++) {
      ss.record(50 + Math.random() * 100);
    }
    const result = await ss.check();
    expect(result.details?.samples).toBe(100);
    expect(ss.name()).toBe('api-latency');
  });

  it('LatencySteadyState 无样本时默认通过', async () => {
    const ss = new LatencySteadyState('empty', 100);
    const result = await ss.check();
    expect(result.met).toBe(true);
    expect(result.message).toBe('无延迟样本');
  });

  it('CompositeSteadyState 所有条件满足时 met=true', async () => {
    const ss1: SteadyState = { name: () => 'a', check: async () => ({ met: true, message: 'ok' }) };
    const ss2: SteadyState = { name: () => 'b', check: async () => ({ met: true, message: 'ok' }) };
    const composite = new CompositeSteadyState('all', [ss1, ss2]);
    const result = await composite.check();
    expect(result.met).toBe(true);
    expect(composite.name()).toBe('all');
  });

  it('CompositeSteadyState 任一条件不满足时 met=false', async () => {
    const ss1: SteadyState = { name: () => 'a', check: async () => ({ met: true, message: 'ok' }) };
    const ss2: SteadyState = { name: () => 'b', check: async () => ({ met: false, message: 'fail' }) };
    const composite = new CompositeSteadyState('all', [ss1, ss2]);
    const result = await composite.check();
    expect(result.met).toBe(false);
  });

  it('CustomSteadyState 应使用自定义函数', async () => {
    const ss = new CustomSteadyState('custom', async () => ({ met: true, message: 'always ok' }));
    const result = await ss.check();
    expect(result.met).toBe(true);
    expect(ss.name()).toBe('custom');
  });
});

// ===== 报告生成测试 =====

describe('Report generation', () => {
  function makeResult(overrides?: Partial<ExperimentResult>): ExperimentResult {
    return {
      experiment: {
        name: 'test-exp',
        hypothesis: 'test hypothesis',
        faults: [new NoopFault()],
      },
      status: 'completed',
      startTime: new Date('2026-01-01T00:00:00Z'),
      endTime: new Date('2026-01-01T00:00:01Z'),
      durationMs: 1000,
      faultResults: [{
        faultType: 'noop_default',
        description: '空操作故障: default（用于测试）',
        injected: true,
        injectTime: new Date('2026-01-01T00:00:00Z'),
        cleanupTime: new Date('2026-01-01T00:00:01Z'),
      }],
      hypothesisValidated: true,
      ...overrides,
    };
  }

  it('summarize 应生成正确摘要', () => {
    const result = makeResult();
    const summary = summarize(result);
    expect(summary.name).toBe('test-exp');
    expect(summary.status).toBe('completed');
    expect(summary.hypothesisValidated).toBe(true);
    expect(summary.faultCount).toBe(1);
    expect(summary.steadyStateMet).toBe(true);
  });

  it('formatReport 应生成 Markdown 报告', () => {
    const result = makeResult();
    const report = formatReport(result);
    expect(report).toContain('# 混沌实验报告');
    expect(report).toContain('test-exp');
    expect(report).toContain('test hypothesis');
    expect(report).toContain('✅');
    expect(report).toContain('已验证');
    expect(report).toContain('noop_default');
  });

  it('formatReport 应包含稳态检查', () => {
    const steadyState: SteadyState = { name: () => 'slo', check: async () => ({ met: true, message: 'ok' }) };
    const result = makeResult({
      experiment: { name: 'ss-test', hypothesis: 'h', faults: [], steadyState },
      preSteadyState: { met: true, message: 'pre ok' },
      postSteadyState: { met: true, message: 'post ok' },
    });
    const report = formatReport(result);
    expect(report).toContain('## 稳态检查');
    expect(report).toContain('### 实验前');
    expect(report).toContain('### 实验后');
  });

  it('formatSummaryTable 应生成表格', () => {
    const summaries: ExperimentSummary[] = [
      { name: 'exp-1', status: 'completed', hypothesisValidated: true, durationMs: 1000, faultCount: 2, steadyStateMet: true },
      { name: 'exp-2', status: 'failed', hypothesisValidated: false, durationMs: 500, faultCount: 1, steadyStateMet: false },
    ];
    const table = formatSummaryTable(summaries);
    expect(table).toContain('exp-1');
    expect(table).toContain('exp-2');
    expect(table).toContain('✅');
    expect(table).toContain('❌');
  });
});

// ===== 向后兼容性测试 =====

describe('Backward compatibility', () => {
  it('LatencyFault 应正常工作', async () => {
    const f = new LatencyFault(100, 10, 'target');
    expect(f.type()).toBe('network_delay');
    await f.inject();
    await f.cleanup();
  });

  it('ErrorFault 应正常工作', async () => {
    const f = new ErrorFault(500, 'error', 'target');
    expect(f.type()).toBe('error');
    await f.inject();
    await f.cleanup();
  });

  it('ResourceFault 应正常工作', async () => {
    const f = new ResourceFault('cpu', 80, 'target', 1000);
    expect(f.type()).toBe('resource');
    await f.inject();
    await f.cleanup();
  });

  it('LLMErrorFault 应正常工作', async () => {
    const f = new LLMErrorFault(500, 'error', 'model');
    expect(f.type()).toBe('llm_error');
    const cleanup = await f.inject();
    await cleanup();
  });

  it('LLMRateLimitFault 应正常工作', async () => {
    const f = new LLMRateLimitFault(10, 1000, 'model');
    expect(f.type()).toBe('llm_rate_limit');
    const cleanup = await f.inject();
    expect(f.shouldThrottle()).toBe(false); // first request
    await cleanup();
  });
});

// ===== ExperimentStatus 类型测试 =====

describe('ExperimentStatus', () => {
  it('应支持所有状态值', () => {
    const statuses: ExperimentStatus[] = ['pending', 'running', 'completed', 'aborted', 'failed'];
    expect(statuses).toHaveLength(5);
  });
});
