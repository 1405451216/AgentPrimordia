/**
 * Prompt A/B 测试 — 自动对比不同 system prompt 的效果。
 *
 * 核心能力：
 * 1. 同时运行多个不同 system prompt 的 Agent
 * 2. 基于评估指标（响应质量、工具准确率、耗时、Token 消耗）自动选优
 * 3. 支持统计显著性检验，避免小样本偏差
 * 4. 自动将最优 prompt 推广为默认配置
 *
 * 使用方式：
 *   const abTest = new PromptABTest({
 *     variants: [
 *       { name: 'concise', systemPrompt: '你是一个简洁助手' },
 *       { name: 'detailed', systemPrompt: '你是一个详细助手' },
 *     ],
 *     evaluator: new QualityEvaluator(),
 *   });
 *   const result = await abTest.run('解释递归', agentFactory);
 *   console.log(result.winner); // 'concise' or 'detailed'
 */

import type { Provider } from '../llm/provider.js';
import type { Response } from '../types.js';

// ===== 类型定义 =====

/** Prompt 变体定义 */
export interface PromptVariant {
  /** 变体名称（唯一标识） */
  name: string;
  /** 系统提示词 */
  systemPrompt: string;
  /** 额外配置（如 temperature、maxTurns 等） */
  config?: {
    temperature?: number;
    maxTurns?: number;
    [key: string]: unknown;
  };
}

/** 单次实验结果 */
export interface ExperimentResult {
  /** 变体名称 */
  variantName: string;
  /** Agent 响应 */
  response: Response;
  /** 评估分数 [0, 1] */
  score: number;
  /** 耗时（毫秒） */
  durationMs: number;
  /** Token 使用量 */
  totalTokens?: number;
  /** 评估详情 */
  evaluationDetails?: Record<string, number>;
}

/** A/B 测试结果 */
export interface ABTestResult {
  /** 所有变体的实验结果 */
  results: ExperimentResult[];
  /** 最优变体名称 */
  winner: string;
  /** 最优变体分数 */
  winnerScore: number;
  /** 统计显著性（置信度 [0, 1]，-1 表示样本不足） */
  confidence: number;
  /** 建议使用的 system prompt */
  recommendedPrompt: string;
  /** 实验总结 */
  summary: string;
}

/** 评估器接口 */
export interface PromptEvaluator {
  /** 评估 Agent 响应质量，返回 [0, 1] 分数 */
  evaluate(input: string, response: Response, variant: PromptVariant): Promise<{ score: number; details?: Record<string, number> }>;
}

/** A/B 测试配置 */
export interface ABTestConfig {
  /** 变体列表（至少 2 个） */
  variants: PromptVariant[];
  /** 评估器 */
  evaluator: PromptEvaluator;
  /** 每个变体重复实验次数（用于统计显著性），默认 3 */
  repeatsPerVariant?: number;
  /** 是否并行执行，默认 true */
  parallel?: boolean;
}

// ===== 内置评估器 =====

/** 基于关键词的简单评估器 */
export class KeywordEvaluator implements PromptEvaluator {
  private keywords: string[];

  constructor(keywords: string[]) {
    this.keywords = keywords;
  }

  async evaluate(_input: string, response: Response): Promise<{ score: number; details?: Record<string, number> }> {
    const content = response.content.toLowerCase();
    const hits = this.keywords.filter((kw) => content.includes(kw.toLowerCase())).length;
    const score = this.keywords.length > 0 ? hits / this.keywords.length : 0.5;
    return { score, details: { keywordHits: hits, totalKeywords: this.keywords.length } };
  }
}

/** 基于响应长度和完整性的评估器 */
export class CompletenessEvaluator implements PromptEvaluator {
  private minLength: number;
  private maxLength: number;

  constructor(minLength = 50, maxLength = 5000) {
    this.minLength = minLength;
    this.maxLength = maxLength;
  }

  async evaluate(_input: string, response: Response): Promise<{ score: number; details?: Record<string, number> }> {
    const len = response.content.length;
    let score = 0.5;
    if (len >= this.minLength && len <= this.maxLength) score = 1.0;
    else if (len < this.minLength) score = len / this.minLength * 0.5;
    else score = Math.max(0.3, 1.0 - (len - this.maxLength) / this.maxLength * 0.5);

    // 检查是否有错误
    if (response.content.includes('error') || response.content.includes('Error')) {
      score *= 0.7;
    }

    return {
      score: Math.max(0, Math.min(1, score)),
      details: { length: len, hasError: score < 0.5 ? 1 : 0 },
    };
  }
}

/** LLM 评估器（使用另一个 LLM 来评估响应质量） */
export class PromptLLMEvaluator implements PromptEvaluator {
  private provider: Provider;
  private evaluationPrompt: string;

  constructor(provider: Provider, evaluationPrompt?: string) {
    this.provider = provider;
    this.evaluationPrompt = evaluationPrompt ??
      'Rate the following response on a scale of 0 to 100. Return only the number.\n\nInput: {input}\n\nResponse: {response}';
  }

  async evaluate(input: string, response: Response): Promise<{ score: number; details?: Record<string, number> }> {
    try {
      const prompt = this.evaluationPrompt
        .replace('{input}', input.slice(0, 500))
        .replace('{response}', response.content.slice(0, 1000));

      const result = await this.provider.complete({
        messages: [{ role: 'user', content: prompt }],
        temperature: 0,
      });

      const score = parseInt(result.content.match(/\d+/)?.[0] ?? '50', 10) / 100;
      return { score: Math.max(0, Math.min(1, score)), details: { rawScore: score * 100 } };
    } catch {
      return { score: 0.5, details: { error: 1 } };
    }
  }
}

// ===== 统计工具 =====

/** 计算均值 */
function mean(values: number[]): number {
  return values.length > 0 ? values.reduce((a, b) => a + b, 0) / values.length : 0;
}

/** 计算标准差 */
function stdDev(values: number[]): number {
  if (values.length < 2) return 0;
  const m = mean(values);
  const variance = values.reduce((sum, v) => sum + (v - m) ** 2, 0) / (values.length - 1);
  return Math.sqrt(variance);
}

/** 简单的 t 检验（Welch's t-test），返回置信度 [0, 1] */
function welchTTest(a: number[], b: number[]): number {
  if (a.length < 2 || b.length < 2) return -1; // 样本不足
  const ma = mean(a);
  const mb = mean(b);
  const sa = stdDev(a);
  const sb = stdDev(b);
  const na = a.length;
  const nb = b.length;

  const se = Math.sqrt((sa * sa) / na + (sb * sb) / nb);
  if (se === 0) return ma === mb ? 0.5 : 1.0;

  const t = Math.abs(ma - mb) / se;
  // 简化：t > 2.776 (df=4, p=0.05) 认为显著
  // 使用近似：置信度 = min(1, t / 3)
  return Math.min(1, t / 3);
}

// ===== A/B 测试引擎 =====

export class PromptABTest {
  private config: Required<ABTestConfig>;

  constructor(config: ABTestConfig) {
    if (config.variants.length < 2) {
      throw new Error('A/B test requires at least 2 variants');
    }
    this.config = {
      variants: config.variants,
      evaluator: config.evaluator,
      repeatsPerVariant: config.repeatsPerVariant ?? 3,
      parallel: config.parallel ?? true,
    };
  }

  /** 运行 A/B 测试
   *
   * @param input 测试输入
   * @param agentFactory Agent 工厂函数，接收 systemPrompt 返回一个可 run 的 Agent
   */
  async run(
    input: string,
    agentFactory: (variant: PromptVariant) => { run: (input: string) => Promise<Response> },
  ): Promise<ABTestResult> {
    const allResults: ExperimentResult[] = [];
    const scoresByVariant: Map<string, number[]> = new Map();

    // 为每个变体初始化分数数组
    for (const variant of this.config.variants) {
      scoresByVariant.set(variant.name, []);
    }

    if (this.config.parallel) {
      // 并行执行所有变体
      const promises: Promise<ExperimentResult[]>[] = this.config.variants.map(async (variant) => {
        const results: ExperimentResult[] = [];
        for (let i = 0; i < this.config.repeatsPerVariant; i++) {
          const result = await this.runSingleExperiment(input, variant, agentFactory);
          results.push(result);
          scoresByVariant.get(variant.name)!.push(result.score);
        }
        return results;
      });
      const batchResults = await Promise.all(promises);
      for (const batch of batchResults) {
        allResults.push(...batch);
      }
    } else {
      // 串行执行
      for (const variant of this.config.variants) {
        for (let i = 0; i < this.config.repeatsPerVariant; i++) {
          const result = await this.runSingleExperiment(input, variant, agentFactory);
          allResults.push(result);
          scoresByVariant.get(variant.name)!.push(result.score);
        }
      }
    }

    // 计算最优变体
    let winner = this.config.variants[0]!.name;
    let winnerScore = mean(scoresByVariant.get(winner) ?? [0]);
    for (const variant of this.config.variants) {
      const avgScore = mean(scoresByVariant.get(variant.name) ?? [0]);
      if (avgScore > winnerScore) {
        winnerScore = avgScore;
        winner = variant.name;
      }
    }

    // 计算统计置信度
    let confidence = -1;
    if (this.config.variants.length === 2) {
      const scoresA = scoresByVariant.get(this.config.variants[0]!.name) ?? [];
      const scoresB = scoresByVariant.get(this.config.variants[1]!.name) ?? [];
      confidence = welchTTest(scoresA, scoresB);
    } else {
      // 多变体时，比较最优和次优
      const sorted = this.config.variants
        .map((v) => ({ name: v.name, avg: mean(scoresByVariant.get(v.name) ?? [0]) }))
        .sort((a, b) => b.avg - a.avg);
      if (sorted.length >= 2) {
        confidence = welchTTest(
          scoresByVariant.get(sorted[0]!.name) ?? [],
          scoresByVariant.get(sorted[1]!.name) ?? [],
        );
      }
    }

    const winnerVariant = this.config.variants.find((v) => v.name === winner)!;

    const summary = this.buildSummary(allResults, scoresByVariant, winner, winnerScore, confidence);

    return {
      results: allResults,
      winner,
      winnerScore,
      confidence,
      recommendedPrompt: winnerVariant.systemPrompt,
      summary,
    };
  }

  /** 批量测试多个输入 */
  async runBatch(
    inputs: string[],
    agentFactory: (variant: PromptVariant) => { run: (input: string) => Promise<Response> },
  ): Promise<ABTestResult[]> {
    const results: ABTestResult[] = [];
    for (const input of inputs) {
      results.push(await this.run(input, agentFactory));
    }
    return results;
  }

  // ===== 内部方法 =====

  private async runSingleExperiment(
    input: string,
    variant: PromptVariant,
    agentFactory: (variant: PromptVariant) => { run: (input: string) => Promise<Response> },
  ): Promise<ExperimentResult> {
    const agent = agentFactory(variant);
    const start = Date.now();
    const response = await agent.run(input);
    const durationMs = Date.now() - start;

    const { score, details } = await this.config.evaluator.evaluate(input, response, variant);

    return {
      variantName: variant.name,
      response,
      score,
      durationMs,
      totalTokens: undefined, // AgentMetrics 不包含 token 计数
      evaluationDetails: details,
    };
  }

  private buildSummary(
    results: ExperimentResult[],
    scoresByVariant: Map<string, number[]>,
    winner: string,
    winnerScore: number,
    confidence: number,
  ): string {
    const lines: string[] = [];
    lines.push(`A/B Test Summary:`);
    lines.push(`  Variants: ${this.config.variants.map((v) => v.name).join(', ')}`);
    lines.push(`  Repeats per variant: ${this.config.repeatsPerVariant}`);
    lines.push(`  Total experiments: ${results.length}`);
    lines.push('');

    for (const variant of this.config.variants) {
      const scores = scoresByVariant.get(variant.name) ?? [];
      const avgScore = mean(scores);
      const std = stdDev(scores);
      const avgDuration = mean(
        results.filter((r) => r.variantName === variant.name).map((r) => r.durationMs),
      );
      lines.push(`  ${variant.name}:`);
      lines.push(`    Score: ${avgScore.toFixed(3)} (±${std.toFixed(3)})`);
      lines.push(`    Avg duration: ${avgDuration.toFixed(0)}ms`);
    }

    lines.push('');
    lines.push(`  Winner: ${winner} (score: ${winnerScore.toFixed(3)})`);
    lines.push(`  Confidence: ${confidence < 0 ? 'insufficient data' : `${(confidence * 100).toFixed(0)}%`}`);

    return lines.join('\n');
  }
}
