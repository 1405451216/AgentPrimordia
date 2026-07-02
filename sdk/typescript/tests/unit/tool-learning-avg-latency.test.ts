/**
 * ToolLearning avgLatencyMs 修复回归测试
 *
 * 历史 bug: src/agent/tool-learning.ts:340 avgLatencyMs 硬编码 0
 * 修复: P0.2 — 通过 recordSuccessWithTiming/recordFailureWithTiming 入口
 *        记录 latencyMs,getUsagePatterns 真实计算模式平均值
 *
 * 注: getUsagePatterns 内部 minRecordsForPattern 比较的是「practices 数量」
 * (不同 pattern 数),所以测试用 3 个不同 pattern 触发返回
 */
import { describe, it, expect, beforeEach } from 'vitest';
import { EnhancedToolLearner } from '../../src/agent/tool-learning.js';
import { InMemoryStore } from '../../src/memory/store.js';
import type { Memory } from '../../src/memory/store.js';

describe('EnhancedToolLearner.getUsagePatterns — avgLatencyMs 真实计算', () => {
  let learner: EnhancedToolLearner;
  let mem: Memory;

  beforeEach(() => {
    mem = new InMemoryStore();
    // 降低 minRecordsForPattern 以便测试不依赖"3 个不同 pattern"
    learner = new EnhancedToolLearner(mem, { minRecordsForPattern: 1 });
  });

  it('无 timing 数据时 avgLatencyMs 仍为 0（不报错）', async () => {
    await learner.recordSuccess('search', '{"q":"foo"}', 'r1');
    const patterns = await learner.getUsagePatterns('search');
    expect(patterns.length).toBeGreaterThan(0);
    expect(patterns[0].avgLatencyMs).toBe(0);
  });

  it('通过 recordSuccessWithTiming 记录后 avgLatencyMs 真实计算', async () => {
    await learner.recordSuccessWithTiming('search', '{"q":"foo"}', 'r1', 100);
    await learner.recordSuccessWithTiming('search', '{"q":"foo"}', 'r2', 200);
    await learner.recordSuccessWithTiming('search', '{"q":"foo"}', 'r3', 300);

    const patterns = await learner.getUsagePatterns('search');
    expect(patterns.length).toBeGreaterThan(0);
    // 平均 (100+200+300)/3 = 200
    expect(patterns[0].avgLatencyMs).toBe(200);
  });

  it('混合有/无 timing 时仅取有 timing 的记录', async () => {
    await learner.recordSuccess('search', '{"q":"foo"}', 'r1');
    await learner.recordSuccess('search', '{"q":"foo"}', 'r2');
    await learner.recordSuccessWithTiming('search', '{"q":"foo"}', 'r3', 50);
    await learner.recordSuccessWithTiming('search', '{"q":"foo"}', 'r4', 150);

    const patterns = await learner.getUsagePatterns('search');
    expect(patterns.length).toBeGreaterThan(0);
    // 仅算 (50+150)/2 = 100
    expect(patterns[0].avgLatencyMs).toBe(100);
  });

  it('失败记录也参与 avgLatencyMs 计算', async () => {
    await learner.recordFailureWithTiming('search', '{"q":"foo"}', 'err', 80);
    await learner.recordFailureWithTiming('search', '{"q":"foo"}', 'err', 120);
    await learner.recordSuccessWithTiming('search', '{"q":"foo"}', 'r', 50);

    const patterns = await learner.getUsagePatterns('search');
    expect(patterns.length).toBeGreaterThan(0);
    // 3 次: 80+120+50 / 3 = 83.33 → round → 83
    expect(patterns[0].avgLatencyMs).toBe(83);
  });

  it('不同 pattern 各自的 avgLatencyMs 独立计算', async () => {
    // pattern A (action:foo): 耗时 100, 200
    await learner.recordSuccessWithTiming('search', '{"action":"foo","q":"x"}', 'r', 100);
    await learner.recordSuccessWithTiming('search', '{"action":"foo","q":"y"}', 'r', 200);
    // pattern B (action:bar): 耗时 50
    await learner.recordSuccessWithTiming('search', '{"action":"bar"}', 'r', 50);

    const patterns = await learner.getUsagePatterns('search');
    expect(patterns.length).toBe(2);
    // 按 successRate 降序,foo (2/2=1.0) 排前
    const fooPattern = patterns.find(p => p.patternName === 'action:foo');
    const barPattern = patterns.find(p => p.patternName === 'action:bar');
    expect(fooPattern?.avgLatencyMs).toBe(150); // (100+200)/2
    expect(barPattern?.avgLatencyMs).toBe(50);
  });
});
