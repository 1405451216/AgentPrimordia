/**
 * 投机执行 v2 — 依赖轻量的「神经」工具结果预测器。
 *
 * 设计动机（对应 evolution 计划 T3-3）：
 *   v1 的 ToolResultPredictor 仅用「上一次成功结果」做预测，对参数敏感的工具
 *   （如 calculator 传入不同参数返回不同结果）几乎总是预测错误。
 *
 *   本模块在不引入 tfjs 等重依赖的前提下，用两层轻量模型提升预测准确率：
 *   1. 精确记忆层：相同参数（args 哈希）出现过的工具调用，直接返回历史精确结果。
 *   2. 统计/逻辑层：对未见过参数的调用，用每工具的「多数结果」+ 轻量逻辑回归
 *      （仅 4 个启发式特征）预测 isError，避免 v1 总是预测成功导致的系统性偏差。
 *
 * 与 v1 的关系：NeuralToolPredictor 继承 ToolResultPredictor，复用其 isHit()
 * 命中判定与 recordResult() 历史记录，并覆盖 predict() 实现更聪明的策略。
 */

import { ToolResultPredictor } from './speculative-exec.js';
import type { ToolResult } from '../types.js';

/** 工具使用记录，用于训练预测模型 */
export interface ToolUsageRecord {
  toolName: string;
  args: string;
  result: string;
  success: boolean;
  /** 可选时间戳（毫秒），用于老化处理 */
  timestamp?: number;
}

/** 预测特征（仅 4 个低成本启发式特征，无需 tensor 框架） */
interface PredictionFeatures {
  bias: number;
  argLengthNorm: number;
  hasDigits: boolean;
  hasSpecial: boolean;
}

/** 单工具模型数据 */
interface ToolModel {
  /** args 哈希 -> 精确历史结果（精确记忆层） */
  argResults: Map<string, ToolResult>;
  /** 成功结果内容 -> 出现次数（用于多数投票） */
  contentCounts: Map<string, number>;
  successCount: number;
  totalCount: number;
  /** 逻辑回归权重 [bias, argLen, digits, special] */
  weights: number[];
}

function hashString(s: string): string {
  let h = 5381;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) + h + s.charCodeAt(i)) | 0;
  }
  return (h >>> 0).toString(36);
}

function extractFeatures(args: string): PredictionFeatures {
  let hasDigits = false;
  let hasSpecial = false;
  for (let i = 0; i < args.length; i++) {
    const c = args.charCodeAt(i);
    if (c >= 48 && c <= 57) hasDigits = true;
    else if (!(c >= 65 && c <= 90) && !(c >= 97 && c <= 122)) hasSpecial = true;
  }
  return {
    bias: 1,
    argLengthNorm: Math.min(args.length / 1000, 1),
    hasDigits,
    hasSpecial,
  };
}

function sigmoid(z: number): number {
  // 数值稳定版本
  if (z >= 0) return 1 / (1 + Math.exp(-z));
  const ez = Math.exp(z);
  return ez / (1 + ez);
}

function dot(f: PredictionFeatures, w: number[]): number {
  return f.bias * w[0] + f.argLengthNorm * w[1] + (f.hasDigits ? w[2] : 0) + (f.hasSpecial ? w[3] : 0);
}

/**
 * NeuralToolPredictor — 轻量「神经」风格工具结果预测器。
 *
 * 不依赖任何 ML 框架：逻辑回归用梯度下降在纯 TS 中实现，
 * 参数量极小（4 维），训练在毫秒级完成，适合在线/边车场景。
 */
export class NeuralToolPredictor extends ToolResultPredictor {
  private models: Map<string, ToolModel> = new Map();

  /** 记录一次工具执行结果，同时更新精确记忆与统计模型 */
  override recordResult(toolName: string, result: ToolResult): void {
    super.recordResult(toolName, result);
    this.ensureModel(toolName);
    const m = this.models.get(toolName)!;

    // 精确记忆层：按 args 哈希存储
    m.argResults.set(hashString(result.toolCallId === 'speculative' ? '' : toolName + ':' + result.content), result);
    // 注意：recordResult 拿不到 args，仅维护统计层；精确层靠 train() 预填

    // 统计层
    m.totalCount++;
    if (!result.isError) {
      m.successCount++;
      m.contentCounts.set(result.content, (m.contentCounts.get(result.content) ?? 0) + 1);
    }
  }

  /** 批量训练：填充精确记忆层并拟合逻辑回归权重 */
  async train(records: ToolUsageRecord[]): Promise<void> {
    // 按工具分组
    const byTool = new Map<string, ToolUsageRecord[]>();
    for (const r of records) {
      if (!byTool.has(r.toolName)) byTool.set(r.toolName, []);
      byTool.get(r.toolName)!.push(r);
    }

    for (const [toolName, recs] of byTool) {
      this.ensureModel(toolName);
      const m = this.models.get(toolName)!;

      for (const r of recs) {
        const tr: ToolResult = {
          toolCallId: 'speculative',
          content: r.result,
          isError: !r.success,
        };
        // 精确记忆层
        m.argResults.set(hashString(toolName + '::' + r.args), tr);
        // 统计层
        m.totalCount++;
        if (r.success) {
          m.successCount++;
          m.contentCounts.set(r.result, (m.contentCounts.get(r.result) ?? 0) + 1);
        }
      }

      // 拟合逻辑回归（标签 = isError?1:0）
      this.fitLogistic(m, recs);
    }
  }

  /** 预测工具结果 */
  override predict(toolName: string, args: string): ToolResult | null {
    const m = this.models.get(toolName);
    if (!m) {
      // 该工具尚无模型，回退到 v1 策略（最后一次成功结果）
      return super.predict(toolName, args);
    }

    // 精确记忆层命中
    const exact = m.argResults.get(hashString(toolName + '::' + args));
    if (exact) {
      return { toolCallId: 'speculative', content: exact.content, isError: exact.isError };
    }

    // 统计/逻辑层
    const features = extractFeatures(args);
    const pError = sigmoid(dot(features, m.weights));
    const isError = pError > 0.5;
    // 多数成功结果作为内容预测（未命中精确层时使用）
    const majority = this.majorityContent(m);
    return {
      toolCallId: 'speculative',
      content: majority ?? (isError ? 'error' : '[predicted]'),
      isError,
    };
  }

  /** 返回某工具的命中率估计（用于监控/自动降级） */
  modelStats(toolName: string): { total: number; successRate: number; exactEntries: number } | null {
    const m = this.models.get(toolName);
    if (!m) return null;
    return {
      total: m.totalCount,
      successRate: m.totalCount > 0 ? m.successCount / m.totalCount : 0,
      exactEntries: m.argResults.size,
    };
  }

  /** 列出已训练的工具 */
  trainedTools(): string[] {
    return Array.from(this.models.keys());
  }

  override reset(): void {
    super.reset();
    this.models.clear();
  }

  // ===== 内部方法 =====

  private ensureModel(toolName: string): void {
    if (!this.models.has(toolName)) {
      this.models.set(toolName, {
        argResults: new Map(),
        contentCounts: new Map(),
        successCount: 0,
        totalCount: 0,
        weights: [0, 0, 0, 0],
      });
    }
  }

  private majorityContent(m: ToolModel): string | null {
    let best: string | null = null;
    let bestCount = -1;
    for (const [content, count] of m.contentCounts) {
      if (count > bestCount) {
        bestCount = count;
        best = content;
      }
    }
    return best;
  }

  private fitLogistic(m: ToolModel, recs: ToolUsageRecord[]): void {
    const lr = 0.1;
    const epochs = 200;
    const w = m.weights;
    const n = recs.length;
    if (n === 0) return;

    for (let e = 0; e < epochs; e++) {
      let gw0 = 0, gw1 = 0, gw2 = 0, gw3 = 0;
      for (const r of recs) {
        const f = extractFeatures(r.args);
        const z = dot(f, w);
        const pred = sigmoid(z);
        const label = r.success ? 0 : 1; // 预测 isError
        const err = pred - label;
        gw0 += err * f.bias;
        gw1 += err * f.argLengthNorm;
        gw2 += err * (f.hasDigits ? 1 : 0);
        gw3 += err * (f.hasSpecial ? 1 : 0);
      }
      w[0] -= (lr * gw0) / n;
      w[1] -= (lr * gw1) / n;
      w[2] -= (lr * gw2) / n;
      w[3] -= (lr * gw3) / n;
    }
    m.weights = w;
  }
}
