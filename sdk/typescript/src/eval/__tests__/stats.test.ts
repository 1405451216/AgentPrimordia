/**
 * stats.test.ts — V7 弧线 S0-1 统计框架数值对账测试（TypeScript 端）。
 *
 * 权威夹具 agentprimordia/internal/eval/testdata/stats_fixtures.json 由 Go 侧
 * 生成（internal/eval/stats_test.go 的 TestWriteStatsFixtures，Go 为权威侧），
 * 本文件逐项断言 TS 实现与之一致：
 * - 跨语言容差 1e-9；
 * - bootstrap 因 xorshift64 RNG 已逐位对齐，point/lower95/upper95 断言位级全等。
 * 边界语义与 Go stats_test.go 同源（报错前缀、单调性、同 seed 复现等）。
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  Z95,
  normalCDF,
  normalQuantile,
  wilsonInterval,
  reportRate,
  formatRate,
  mcnemarExact,
  analyzePaired,
  twoProportionZTest,
  sampleSizeTwoProportion,
  mcnemarPower,
  sampleSizeMcNemar,
  cohenKappa,
  pairedBootstrapCI,
} from '../stats.js';
import type { PairedOutcome } from '../stats.js';

const __dirname = dirname(fileURLToPath(import.meta.url));

/** Go 侧权威夹具（跨语言对账门；字段名与 Go stats_test.go 的 json tag 一致）。 */
const FIXTURES_PATH = resolve(
  __dirname,
  '../../../../../agentprimordia/internal/eval/testdata/stats_fixtures.json',
);

/** 夹具结构（字段名即 Go 侧 json tag，改名即破坏对账门）。 */
interface StatsFixtures {
  wilson: { s: number; n: number; lower95: number; upper95: number }[];
  mcnemar: { b: number; c: number; p: number }[];
  kappa: { rater_a: string[]; rater_b: string[]; kappa: number }[];
  mcnemar_power: { n: number; delta: number; omega: number; power: number }[];
  bootstrap: {
    deltas: number[];
    seed: number;
    iterations: number;
    point: number;
    lower95: number;
    upper95: number;
  }[];
  normal_quantile: { p: number; x: number }[];
}

const FX = JSON.parse(readFileSync(FIXTURES_PATH, 'utf-8')) as StatsFixtures;

/** 与 Go ErrInvalidStatInput 对齐的报错前缀。 */
const ERR_PREFIX = 'eval: 统计入参非法';

/** 断言 fn 抛出且 message 以 Go ErrInvalidStatInput 前缀开头。 */
function expectStatInputError(fn: () => unknown): void {
  expect(fn).toThrow(Error);
  expect(fn).toThrow(ERR_PREFIX);
}

/** 与 Go almost() 同语义的相对容差比较。 */
function almost(got: number, want: number, tol: number): boolean {
  return Math.abs(got - want) <= tol;
}

describe('stats — 跨语言数值对账门（stats_fixtures.json，容差 1e-9）', () => {
  it('Z95 常量与 Go 一致', () => {
    expect(Z95).toBe(1.959963984540054);
  });

  it('wilson：全部条目与 Go 权威值一致（≤1e-9）', () => {
    expect(FX.wilson.length).toBeGreaterThan(0);
    for (const c of FX.wilson) {
      const { lower, upper } = wilsonInterval(c.s, c.n, Z95);
      expect(almost(lower, c.lower95, 1e-9), 'wilson lower s=' + c.s + ' n=' + c.n).toBe(true);
      expect(almost(upper, c.upper95, 1e-9), 'wilson upper s=' + c.s + ' n=' + c.n).toBe(true);
    }
  });

  it('mcnemar：全部条目与 Go 权威值一致（≤1e-9）', () => {
    expect(FX.mcnemar.length).toBeGreaterThan(0);
    for (const c of FX.mcnemar) {
      expect(almost(mcnemarExact(c.b, c.c), c.p, 1e-9), 'mcnemar b=' + c.b + ' c=' + c.c).toBe(true);
    }
  });

  it('kappa：全部条目与 Go 权威值一致（≤1e-9）', () => {
    expect(FX.kappa.length).toBeGreaterThan(0);
    for (const c of FX.kappa) {
      expect(almost(cohenKappa(c.rater_a, c.rater_b), c.kappa, 1e-9)).toBe(true);
    }
  });

  it('mcnemar_power：全部条目与 Go 权威值一致（≤1e-9）', () => {
    expect(FX.mcnemar_power.length).toBeGreaterThan(0);
    for (const c of FX.mcnemar_power) {
      const pw = mcnemarPower(c.n, c.delta, c.omega, 0.05);
      expect(almost(pw, c.power, 1e-9), 'power n=' + c.n).toBe(true);
    }
  });

  it('bootstrap：RNG 已固定，point/lower95/upper95 必须逐位一致', () => {
    expect(FX.bootstrap.length).toBeGreaterThan(0);
    for (const c of FX.bootstrap) {
      const ci = pairedBootstrapCI(c.deltas, c.iterations, c.seed);
      // 位级全等（比 1e-9 容差更严：RNG 与求和/排序/分位取数全部确定性）
      expect(Object.is(ci.point, c.point), 'bootstrap point seed=' + c.seed).toBe(true);
      expect(Object.is(ci.lower, c.lower95), 'bootstrap lower seed=' + c.seed).toBe(true);
      expect(Object.is(ci.upper, c.upper95), 'bootstrap upper seed=' + c.seed).toBe(true);
    }
  });

  it('normal_quantile：全部条目与 Go 权威值一致（≤1e-9）', () => {
    expect(FX.normal_quantile.length).toBeGreaterThan(0);
    for (const c of FX.normal_quantile) {
      expect(almost(normalQuantile(c.p), c.x, 1e-9), 'quantile p=' + c.p).toBe(true);
    }
  });

  it('normalCDF：Φ(0)=0.5，Φ(Z95)≈0.975（erfc 移植自检）', () => {
    expect(normalCDF(0)).toBe(0.5);
    expect(almost(normalCDF(Z95), 0.975, 1e-12)).toBe(true);
  });
});

describe('stats — 边界语义（与 Go stats_test.go 同语义）', () => {
  it('R3 口径：n=24 全对只能宣称 Wilson 下界（≈0.862）而非裸 100%', () => {
    const r = reportRate(24, 24);
    expect(r.point).toBe(1);
    expect(r.wilsonLower).toBeGreaterThan(0.86);
    expect(r.wilsonLower).toBeLessThan(0.87);
    expect(r.wilsonLower).toBeCloseTo(0.8620237953250198, 9);
    expect(formatRate(r)).toBe('1.000 (Wilson95 下界 0.862, n=24)');
  });

  it('formatRate 文案与 Go RatePoint.String() 完全一致', () => {
    expect(formatRate(reportRate(90, 100))).toBe('0.900 (Wilson95 下界 0.826, n=100)');
    expect(reportRate(90, 100).toString()).toBe(formatRate(reportRate(90, 100)));
  });

  it('Wilson：非法入参报错（trials=0 / s>n / z=0）', () => {
    expectStatInputError(() => wilsonInterval(3, 0, Z95));
    expectStatInputError(() => wilsonInterval(11, 10, Z95));
    expectStatInputError(() => wilsonInterval(1, 10, 0));
  });

  it('normalQuantile：p 越界与 NaN 报错', () => {
    expectStatInputError(() => normalQuantile(0));
    expectStatInputError(() => normalQuantile(1));
    expectStatInputError(() => normalQuantile(-0.5));
    expectStatInputError(() => normalQuantile(Number.NaN));
  });

  it('mcnemarExact：对称性、退化输入与决策边界', () => {
    expect(mcnemarExact(0, 0)).toBe(1); // 无不一致对：无法拒绝
    expect(mcnemarExact(3, 3)).toBe(1); // 完全均衡
    expect(mcnemarExact(25, 10)).toBe(mcnemarExact(10, 25)); // 对称性
    expect(mcnemarExact(8, 21)).toBeLessThan(0.05); // 8 vs 21 应显著
    expectStatInputError(() => mcnemarExact(-1, 3)); // 负数
  });

  it('analyzePaired：计数、lift 与直接 McNemar 一致（Go TestAnalyzePaired 同例）', () => {
    const specs: PairedOutcome[] = [
      { taskId: 't1', baseline: true, treatment: true },
      { taskId: 't2', baseline: false, treatment: true },
      { taskId: 't3', baseline: false, treatment: true },
      { taskId: 't4', baseline: true, treatment: false },
      { taskId: 't5', baseline: true, treatment: true },
      { taskId: 't6', baseline: false, treatment: false },
      { taskId: 't7', baseline: false, treatment: true },
      { taskId: 't8', baseline: true, treatment: true },
    ];
    const a = analyzePaired(specs);
    expect(a.n).toBe(8);
    expect(a.discB).toBe(1);
    expect(a.discC).toBe(3);
    expect(a.concordant).toBe(4);
    expect(a.baselineRate.successes).toBe(4);
    expect(a.treatmentRate.successes).toBe(6);
    expect(almost(a.lift, 0.25, 1e-12)).toBe(true);
    expect(a.pValue).toBe(mcnemarExact(1, 3));
    expectStatInputError(() => analyzePaired([])); // 空输入
  });

  it('twoProportionZTest 与 sampleSizeTwoProportion：Go 同表数值', () => {
    const { diff, z, pValue } = twoProportionZTest(30, 100, 50, 100);
    expect(almost(diff, 0.2, 1e-12)).toBe(true);
    expect(almost(z, 2.8867513, 1e-5)).toBe(true);
    expect(almost(pValue, 0.003889, 1e-4)).toBe(true);
    expectStatInputError(() => twoProportionZTest(1, 0, 1, 10)); // n=0
    expect(sampleSizeTwoProportion(0.5, 0.65, 0.05, 0.8)).toBe(170);
    expect(sampleSizeTwoProportion(0.5, 0.7, 0.05, 0.8)).toBe(93);
    expect(sampleSizeTwoProportion(0.5, 0.8, 0.05, 0.8)).toBe(39);
    expect(sampleSizeTwoProportion(0.3, 0.5, 0.05, 0.8)).toBe(93);
    expectStatInputError(() => sampleSizeTwoProportion(0.5, 0.5, 0.05, 0.8)); // p1==p2
  });

  it('mcnemarPower：关键点位 + 随 n 单调不减 + |delta|≥omega 报错', () => {
    const p71 = mcnemarPower(71, 0.15, 0.3, 0.05);
    expect(almost(p71, 0.5806320470332901, 1e-9)).toBe(true);
    expect(mcnemarPower(108, 0.15, 0.3, 0.05)).toBeGreaterThanOrEqual(0.8);
    let prev = -1;
    for (const n of [30, 60, 90, 120, 150]) {
      const p = mcnemarPower(n, 0.15, 0.3, 0.05);
      expect(p).toBeGreaterThanOrEqual(prev - 1e-9); // 离散非严格单调，容差对齐 Go
      prev = p;
    }
    expectStatInputError(() => mcnemarPower(50, 0.4, 0.3, 0.05)); // |delta|>=omega
    expect(() => mcnemarPower(50, 0.4, 0.3, 0.05)).toThrow('|delta|=0.4 须 < omega=0.3');
    expectStatInputError(() => mcnemarPower(0, 0.15, 0.3, 0.05)); // n=0
  });

  it('sampleSizeMcNemar：R2 四个已验证点位逐位复现（108/80/34/59）', () => {
    expect(sampleSizeMcNemar(0.15, 0.3, 0.05, 0.8)).toBe(108);
    expect(sampleSizeMcNemar(0.2, 0.4, 0.05, 0.8)).toBe(80);
    expect(sampleSizeMcNemar(0.3, 0.4, 0.05, 0.8)).toBe(34);
    expect(sampleSizeMcNemar(0.2, 0.3, 0.05, 0.8)).toBe(59);
    // 最小性：n-1 未达标
    expect(mcnemarPower(107, 0.15, 0.3, 0.05)).toBeLessThan(0.8);
    expectStatInputError(() => sampleSizeMcNemar(0, 0.3, 0.05, 0.8)); // delta=0
    expectStatInputError(() => sampleSizeMcNemar(0.3, 0.3, 0.05, 0.8)); // |delta|>=omega
    expectStatInputError(() => sampleSizeMcNemar(0.15, 0.3, 0.05, 1)); // power 越界
  });

  it('cohenKappa：单一类别 κ 无定义报错 + 长度不等报错', () => {
    expectStatInputError(() => cohenKappa([], []));
    expectStatInputError(() => cohenKappa(['a'], ['b', 'c']));
    expectStatInputError(() => cohenKappa(['a', 'a'], ['a', 'a']));
    // 完全一致但多于一个类别：po=1，κ 为有限值
    expect(cohenKappa(['a', 'b'], ['a', 'b'])).toBe(1);
  });

  it('pairedBootstrapCI：同 seed 结果全等 + CI 覆盖点估计 + 非法入参报错', () => {
    const deltas = Array.from({ length: 40 }, (_, i) => (i % 7) - 2.5);
    const c1 = pairedBootstrapCI(deltas, 2000, 20260831);
    const c2 = pairedBootstrapCI(deltas, 2000, 20260831);
    expect(c1.point).toBe(c2.point);
    expect(c1.lower).toBe(c2.lower);
    expect(c1.upper).toBe(c2.upper);
    expect(c1.lower).toBeLessThanOrEqual(c1.point);
    expect(c1.point).toBeLessThanOrEqual(c1.upper);
    expect(c1.point).toBeGreaterThanOrEqual(-1);
    expect(c1.point).toBeLessThanOrEqual(1);
    expect(c1.iterations).toBe(2000);
    expectStatInputError(() => pairedBootstrapCI(deltas, 50, 1)); // iterations<100
    expectStatInputError(() => pairedBootstrapCI([Number.NaN], 200, 1)); // NaN
    expectStatInputError(() => pairedBootstrapCI([Infinity], 200, 1)); // Inf
    expectStatInputError(() => pairedBootstrapCI([], 200, 1)); // 空序列
  });

  it('toJSON：snake_case 字段与 Go 落盘报告可互换', () => {
    const r = reportRate(90, 100);
    const rJson = JSON.parse(JSON.stringify(r)) as Record<string, unknown>;
    expect(Object.keys(rJson)).toEqual(['successes', 'trials', 'point', 'wilson_lower95', 'wilson_upper95']);
    expect(rJson['successes']).toBe(90);
    expect(rJson['trials']).toBe(100);
    expect(rJson['point']).toBe(0.9);
    expect(rJson['wilson_lower95']).toBe(r.wilsonLower);
    expect(rJson['wilson_upper95']).toBe(r.wilsonUpper);

    const specs: PairedOutcome[] = [
      { taskId: 't1', baseline: true, treatment: true },
      { taskId: 't2', baseline: false, treatment: true },
      { taskId: 't3', baseline: false, treatment: true },
      { taskId: 't4', baseline: true, treatment: false },
    ];
    const aJson = JSON.parse(JSON.stringify(analyzePaired(specs))) as Record<string, unknown>;
    expect(Object.keys(aJson)).toEqual([
      'n',
      'concordant',
      'disc_b',
      'disc_c',
      'lift',
      'p_value',
      'baseline_rate',
      'treatment_rate',
    ]);
    expect(aJson['disc_b']).toBe(1);
    expect(aJson['disc_c']).toBe(2);

    const ci = pairedBootstrapCI(Array.from({ length: 40 }, (_, i) => (i % 7) - 2.5), 2000, 7);
    const ciJson = JSON.parse(JSON.stringify(ci)) as Record<string, unknown>;
    expect(Object.keys(ciJson)).toEqual(['point', 'lower95', 'upper95', 'iterations', 'seed']);
    expect(ciJson['lower95']).toBe(ci.lower);
    expect(ciJson['upper95']).toBe(ci.upper);
    expect(ciJson['seed']).toBe(7);
  });
});
