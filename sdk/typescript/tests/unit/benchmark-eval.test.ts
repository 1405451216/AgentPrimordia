/**
 * 真实 harness 基准集测试（TypeScript 端，v3.5-1）。
 * 验证：≥50 条、字段完整性、阶段/语言覆盖、与 Go 权威 JSON 双线一致、评估器与运行器行为。
 */
import { describe, it, expect, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { HARNESS_BENCHMARK_CASES } from '../../src/eval/benchmark-cases.js';
import {
  codeConstructScore,
  runBenchmark,
  PhasePlan,
  PhaseImplement,
  PhaseTest,
  PhaseReview,
  PhaseRelease,
  PhaseGuard,
  PhaseMemory,
  PhaseTool,
  LangGo,
  LangTS,
  LangMulti,
} from '../../src/eval/benchmark-eval.js';
import type { EvalAgent, EvalCase } from '../../src/eval/shared-cases.js';

const __dirname = dirname(fileURLToPath(import.meta.url));

// 读取 Go 权威 JSON 用于双线一致性校验
function goBenchmarkCases(): EvalCase[] {
  const jsonPath = join(__dirname, '..', '..', '..', '..', 'agentprimordia', 'internal', 'eval', 'benchmark_cases.json');
  return JSON.parse(readFileSync(jsonPath, 'utf8')) as EvalCase[];
}

describe('HARNESS_BENCHMARK_CASES', () => {
  it('should contain at least 50 real tasks', () => {
    expect(HARNESS_BENCHMARK_CASES.length).toBeGreaterThanOrEqual(50);
  });

  it('should have unique IDs and required fields', () => {
    const ids = new Set<string>();
    for (const c of HARNESS_BENCHMARK_CASES) {
      expect(c.id).toBeTruthy();
      expect(ids.has(c.id)).toBe(false);
      ids.add(c.id);
      expect(c.name).toBeTruthy();
      expect(c.input).toBeTruthy();
      expect(c.expected).toBeTruthy();
      expect(c.threshold).toBeGreaterThan(0);
      expect(c.threshold).toBeLessThanOrEqual(1);
      // 真实任务必须可被评估
      expect(c.requires?.length || c.expected).toBeTruthy();
    }
  });

  it('should cover all harness phases', () => {
    const phases = new Set(HARNESS_BENCHMARK_CASES.map((c) => c.harness_phase));
    for (const p of [PhasePlan, PhaseImplement, PhaseTest, PhaseReview, PhaseRelease, PhaseGuard, PhaseMemory, PhaseTool]) {
      expect(phases.has(p), `缺少阶段 ${p}`).toBe(true);
    }
  });

  it('should cover both Go and TS lines with >=15 cases each', () => {
    const go = HARNESS_BENCHMARK_CASES.filter((c) => c.lang === LangGo);
    const ts = HARNESS_BENCHMARK_CASES.filter((c) => c.lang === LangTS);
    expect(go.length).toBeGreaterThanOrEqual(15);
    expect(ts.length).toBeGreaterThanOrEqual(15);
  });

  it('should have all implement cases declaring code constructs', () => {
    for (const c of HARNESS_BENCHMARK_CASES) {
      if (c.harness_phase === PhaseImplement) {
        expect(c.requires?.length, `implement 用例 ${c.id} 必须声明 requires`).toBeGreaterThan(0);
      }
    }
  });

  it('should be identical to Go authoritative JSON (parity)', () => {
    const goCases = goBenchmarkCases();
    expect(goCases.length).toBe(HARNESS_BENCHMARK_CASES.length);

    const goById = new Map(goCases.map((c) => [c.id, c]));
    for (const tsCase of HARNESS_BENCHMARK_CASES) {
      const goCase = goById.get(tsCase.id);
      expect(goCase, `Go 端缺少用例 ${tsCase.id}`).toBeDefined();
      expect(tsCase.harness_phase).toBe(goCase!.harness_phase);
      expect(tsCase.lang).toBe(goCase!.lang);
      expect(tsCase.expected).toBe(goCase!.expected);
      expect(tsCase.threshold).toBe(goCase!.threshold);
      expect(tsCase.requires).toEqual(goCase!.requires);
    }
  });
});

describe('codeConstructScore', () => {
  it('should pass when all required constructs present', () => {
    const c: EvalCase = { id: 'fib', name: '', input: '', expected: '', metrics: [], threshold: 0.8, requires: ['func Fibonacci(', 'if n < 0', 'return'] };
    const r = codeConstructScore(c, 'func Fibonacci(n int) int { if n < 0 { return -1 }; if n < 2 { return n }; return Fibonacci(n-1)+Fibonacci(n-2) }');
    expect(r.passed).toBe(true);
    expect(r.score).toBe(1.0);
  });

  it('should fail when a required construct is missing', () => {
    const c: EvalCase = { id: 'fib', name: '', input: '', expected: '', metrics: [], threshold: 0.8, requires: ['func Fibonacci(', 'if n < 0', 'return'] };
    const r = codeConstructScore(c, 'func Fibonacci(n int) int { if n < 2 { return n }; return Fibonacci(n-1)+Fibonacci(n-2) }');
    expect(r.passed).toBe(false);
    expect(r.score).toBeLessThan(0.8);
  });

  it('should fall back to expected keyword when requires empty', () => {
    const c: EvalCase = { id: 'kw', name: '', input: '', expected: 'hello', metrics: [], threshold: 0.5 };
    const r = codeConstructScore(c, 'Well hello there');
    expect(r.passed).toBe(true);
    expect(r.score).toBe(1.0);
  });

  it('should treat "|" in a fragment as OR alternation', () => {
    const c: EvalCase = {
      id: 'guard-exfil', name: '', input: '', expected: '', metrics: [], threshold: 0.9,
      requires: ['拒绝|不透露', 'system prompt|系统提示'],
    };
    const r = codeConstructScore(c, '我应当拒绝透露我的 system prompt 内容。');
    expect(r.passed).toBe(true);
    expect(r.score).toBe(1.0);
  });

  it('should count a "|" fragment as missing when no alternative matches', () => {
    const c: EvalCase = {
      id: 'alt-miss', name: '', input: '', expected: '', metrics: [], threshold: 0.8,
      requires: ['拒绝|不透露', 'absent-xyz|也没有'],
    };
    const r = codeConstructScore(c, '我会拒绝这个请求。');
    expect(r.passed).toBe(false);
    expect(r.score).toBe(0.5);
  });
});

describe('runBenchmark', () => {
  function mockAgent(): EvalAgent {
    return {
      run: vi.fn(async ({ input }) => {
        if (input.includes('Fibonacci')) {
          return { output: 'func Fibonacci(n int) int { if n < 0 { return -1 }; if n < 2 { return n }; return Fibonacci(n-1)+Fibonacci(n-2) }' };
        }
        if (input.includes('拦截提示词注入')) {
          return { output: 'block: 检测到提示词注入, 拒绝执行' };
        }
        if (input.includes('Hello!')) {
          return { output: 'Hello there!' };
        }
        return { output: '' };
      }),
    };
  }

  it('should aggregate report by phase and lang', async () => {
    const report = await runBenchmark(mockAgent(), 'v3.5.0-test', HARNESS_BENCHMARK_CASES);
    expect(report.total).toBe(HARNESS_BENCHMARK_CASES.length);
    expect(report.results.length).toBe(HARNESS_BENCHMARK_CASES.length);
    expect(report.version).toBe('v3.5.0-test');
    expect(report.generated).toBeTruthy();
    expect(report.pass_rate).toBeGreaterThan(0);
    expect(report.pass_rate).toBeLessThanOrEqual(1);

    for (const p of [PhasePlan, PhaseImplement, PhaseTest, PhaseReview, PhaseRelease, PhaseGuard, PhaseMemory, PhaseTool]) {
      expect(report.by_phase[p]?.total).toBeGreaterThan(0);
    }
    for (const l of [LangGo, LangTS, LangMulti]) {
      expect(report.by_lang[l]?.total).toBeGreaterThan(0);
    }
    // implement 阶段应至少有 1 条通过（fibonacci）
    expect(report.by_phase[PhaseImplement]?.passed).toBeGreaterThan(0);
  });

  it('should count agent errors as failed with error info', async () => {
    const badAgent: EvalAgent = {
      run: vi.fn(async () => {
        throw new Error('agent down');
      }),
    };
    const report = await runBenchmark(badAgent, 'v3.5.0', HARNESS_BENCHMARK_CASES.slice(0, 3));
    expect(report.failed).toBe(3);
    for (const r of report.results) {
      expect(r.error).toBe('agent down');
    }
  });
});
