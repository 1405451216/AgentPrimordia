/**
 * Prompt 实验管理器 — 将 v1 的 prompt-ab-test 升级为完整实验平台（T2-2）。
 *
 * 支持：
 * - 多变量实验（A/B/C/...，不再只是 A/B）
 * - 注入式评估器（PromptEvaluator），与具体 LLM/运行环境解耦，便于测试
 * - 基于 statistical-test 的显著性检验与效应量
 * - 自动推荐胜出变体
 */

import { welchTTest, type SignificanceResult } from './statistical-test.js';

/** Prompt 变体 */
export interface PromptVariant {
  name: string;
  content: string;
  metadata?: Record<string, unknown>;
}

/** 单次评估结果 */
export interface ExperimentResult {
  variant: string;
  testCase: string;
  /** 数值评分（越高越好） */
  score: number;
  success: boolean;
  latencyMs?: number;
  cost?: number;
  notes?: string;
}

/** 评估器：对给定变体 + 测试用例给出评估结果 */
export interface PromptEvaluator {
  evaluate(variant: PromptVariant, testCase: string): ExperimentResult | Promise<ExperimentResult>;
}

export interface MultivariateOptions {
  minSamples?: number;
  confidenceLevel?: number;
}

export interface ExperimentSummary {
  name: string;
  winner: string | null;
  results: Record<string, ExperimentResult[]>;
  /** 各变体平均评分 */
  meanScores: Record<string, number>;
  significance: SignificanceResult & { recommendation: string };
  recommendation: string;
  createdAt: string;
}

/** Prompt 实验管理器 */
export class PromptExperimentManager {
  private experiments: Map<string, ExperimentSummary> = new Map();

  /** 运行多变量实验 */
  async runMultivariate(
    name: string,
    variants: PromptVariant[],
    testCases: string[],
    evaluator: PromptEvaluator,
    options: MultivariateOptions = {},
  ): Promise<ExperimentSummary> {
    const minSamples = options.minSamples ?? 1;
    const confidence = options.confidenceLevel ?? 0.95;

    const results: Record<string, ExperimentResult[]> = {};
    for (const v of variants) results[v.name] = [];

    // 对每个测试用例，轮流测试所有变体（平衡顺序偏差）
    for (const testCase of testCases) {
      for (const v of variants) {
        const r = await evaluator.evaluate(v, testCase);
        results[v.name].push(r);
      }
    }

    const summary = this.summarize(name, variants, results, confidence, minSamples);
    this.experiments.set(name, summary);
    return summary;
  }

  /** 运行 A/B 实验（多变量特例） */
  async runAB(
    name: string,
    variantA: PromptVariant,
    variantB: PromptVariant,
    testCases: string[],
    evaluator: PromptEvaluator,
    options: MultivariateOptions = {},
  ): Promise<ExperimentSummary> {
    return this.runMultivariate(name, [variantA, variantB], testCases, evaluator, options);
  }

  /** 基于已收集结果生成摘要与推荐 */
  private summarize(
    name: string,
    variants: PromptVariant[],
    results: Record<string, ExperimentResult[]>,
    confidence: number,
    minSamples: number,
  ): ExperimentSummary {
    const meanScores: Record<string, number> = {};
    let winner: string | null = null;
    let bestMean = -Infinity;

    for (const v of variants) {
      const rs = results[v.name] ?? [];
      if (rs.length < minSamples) {
        meanScores[v.name] = 0;
        continue;
      }
      const mean = rs.reduce((a, r) => a + r.score, 0) / rs.length;
      meanScores[v.name] = mean;
      if (mean > bestMean) {
        bestMean = mean;
        winner = v.name;
      }
    }

    // 显著性：胜出变体 vs 基线（第一个变体）
    let significance: SignificanceResult & { recommendation: string };
    const baseline = variants[0]?.name;
    if (!winner || winner === baseline || (results[winner]?.length ?? 0) < 2 || (results[baseline]?.length ?? 0) < 2) {
      significance = {
        statistic: 0, pValue: 1, isSignificant: false,
        recommendation: winner ? `推荐 ${winner}（尚无显著统计差异，建议增加样本量）` : '无可用结果',
      };
    } else {
      const test = welchTTest(
        results[winner].map((r) => r.score),
        results[baseline].map((r) => r.score),
        confidence,
      );
      const rec = test.isSignificant
        ? `推广 ${winner} 为默认配置（p=${test.pValue.toFixed(4)}，效应量 d=${test.effectSize?.toFixed(2) ?? 'n/a'}）`
        : `差异不显著（p=${test.pValue.toFixed(4)}），建议增加样本量后再决策`;
      significance = {
        statistic: test.statistic,
        df: test.df,
        pValue: test.pValue,
        isSignificant: test.isSignificant,
        effectSize: test.effectSize,
        recommendation: rec,
      };
    }

    return {
      name,
      winner,
      results,
      meanScores,
      significance,
      recommendation: significance.recommendation,
      createdAt: new Date().toISOString(),
    };
  }

  /** 获取实验结果 */
  get(name: string): ExperimentSummary | undefined {
    return this.experiments.get(name);
  }

  /** 列出所有实验名 */
  list(): string[] {
    return Array.from(this.experiments.keys());
  }
}
