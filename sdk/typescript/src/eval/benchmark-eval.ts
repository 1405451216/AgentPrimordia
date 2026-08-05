/**
 * 真实 harness 基准集评估器与运行器（TypeScript 端，v3.5-1）。
 *
 * 与 Go 端 internal/eval/benchmark_evaluator.go 保持功能一致：
 * - CodeConstructEvaluator 等价实现：output 必须同时包含 case.requires 全部片段
 * - BenchmarkRunner 等价实现：按阶段/语言聚合，产出 JSON 兼容基准报告
 */
import type { EvalCase } from './shared-cases.js';
import type { EvalAgent } from './shared-cases.js';

// ===== 阶段/语言常量 =====

export const PhasePlan = 'plan';
export const PhaseImplement = 'implement';
export const PhaseTest = 'test';
export const PhaseReview = 'review';
export const PhaseRelease = 'release';
export const PhaseGuard = 'guard';
export const PhaseMemory = 'memory';
export const PhaseTool = 'tool';

export const LangGo = 'go';
export const LangTS = 'ts';
export const LangMulti = 'multi';

// ===== 评估器 =====

export interface CodeScore {
  score: number;
  passed: boolean;
}

/** 编码任务评估：output 必须同时包含 case.requires 全部片段；requires 为空时退化为 expected 关键词匹配。 */
export function codeConstructScore(c: EvalCase, output: string): CodeScore {
  if (!c.requires || c.requires.length === 0) {
    const found = !!output && output.toLowerCase().includes(c.expected.toLowerCase());
    return { score: found ? 1.0 : 0.0, passed: found };
  }
  const lower = output.toLowerCase();
  let matched = 0;
  for (const frag of c.requires) {
    if (lower.includes(frag.toLowerCase())) {
      matched++;
    }
  }
  const score = matched / c.requires.length;
  return { score, passed: score >= c.threshold };
}

// ===== 报告类型 =====

export interface PhaseSummary {
  total: number;
  passed: number;
  failed: number;
  pass_rate: number;
}

export interface BenchmarkCaseResult {
  case_id: string;
  name: string;
  phase?: string;
  lang?: string;
  passed: boolean;
  score: number;
  duration_ms: number;
  error?: string;
}

export interface BenchmarkReport {
  version: string;
  total: number;
  passed: number;
  failed: number;
  pass_rate: number;
  total_ms: number;
  by_phase: Record<string, PhaseSummary>;
  by_lang: Record<string, PhaseSummary>;
  results: BenchmarkCaseResult[];
  generated: string;
}

// ===== 运行器 =====

/** 对给定 Agent 运行基准集并生成报告（与 Go 端 BenchmarkRunner 行为一致）。 */
export async function runBenchmark(
  agent: EvalAgent,
  version: string,
  cases: EvalCase[],
): Promise<BenchmarkReport> {
  const report: BenchmarkReport = {
    version,
    total: cases.length,
    passed: 0,
    failed: 0,
    pass_rate: 0,
    total_ms: 0,
    by_phase: {},
    by_lang: {},
    results: [],
    generated: '',
  };

  const startAll = Date.now();
  for (const c of cases) {
    const start = Date.now();
    let output = '';
    let error: string | undefined;
    try {
      const resp = await agent.run({ input: c.input });
      output = resp.output;
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
    const duration = Date.now() - start;

    let score = 0;
    let passed = false;
    if (!error) {
      const r = codeConstructScore(c, output);
      score = r.score;
      passed = r.passed;
    }

    report.results.push({
      case_id: c.id,
      name: c.name,
      phase: c.harness_phase,
      lang: c.lang,
      passed,
      score,
      duration_ms: duration,
      error,
    });

    if (error || !passed) {
      report.failed++;
    } else {
      report.passed++;
    }
    accumulate(report, c, passed);
  }
  report.total_ms = Date.now() - startAll;
  report.pass_rate = report.total > 0 ? report.passed / report.total : 0;
  report.generated = new Date().toISOString();
  return report;
}

function accumulate(report: BenchmarkReport, c: EvalCase, passed: boolean): void {
  const phases = [c.harness_phase ?? '', c.lang ?? ''];
  for (let i = 0; i < 2; i++) {
    const key = phases[i];
    if (!key) continue;
    const bucket = i === 0 ? report.by_phase : report.by_lang;
    let sum = bucket[key];
    if (!sum) {
      sum = { total: 0, passed: 0, failed: 0, pass_rate: 0 };
      bucket[key] = sum;
    }
    sum.total++;
    if (passed) {
      sum.passed++;
    } else {
      sum.failed++;
    }
    sum.pass_rate = sum.total > 0 ? sum.passed / sum.total : 0;
  }
}
