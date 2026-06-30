/**
 * Agent 自省与自我调优 — TS SDK 独有进化能力。
 *
 * Agent 在运行过程中收集性能指标（延迟、工具成功率、Token 使用量等），
 * 基于历史数据自动调整配置参数（maxTurns、并行度、重试策略等）。
 *
 * 这是 TS SDK 相对 Go SDK 的进化优势：
 * - Go 端配置是静态的，需要人工调优
 * - TS 端可以利用运行时指标动态优化，形成「越用越快」的正反馈
 *
 * 使用方式：
 *   const tuner = new AgentSelfTuner();
 *   // 每次运行后记录指标
 *   tuner.recordRun(metrics);
 *   // 获取调优建议
 *   const suggestion = tuner.getSuggestion();
 *   if (suggestion.shouldAdjust) {
 *     agent.maxTurns = suggestion.maxTurns;
 *   }
 */

// ===== 类型定义 =====

/** 单次运行的性能指标 */
export interface RunMetrics {
  /** 总轮次 */
  totalTurns: number;
  /** 总工具调用次数 */
  totalTools: number;
  /** 总耗时（毫秒） */
  duration: number;
  /** LLM 调用总延迟（毫秒） */
  llmLatency: number;
  /** 工具执行总延迟（毫秒） */
  toolLatency: number;
  /** Token 使用量（可选） */
  totalTokens?: number;
  /** 工具失败次数（可选） */
  toolFailures?: number;
  /** 是否成功完成 */
  success: boolean;
}

/** 调优建议 */
export interface TuningSuggestion {
  /** 是否建议调整 */
  shouldAdjust: boolean;
  /** 建议的最大轮次 */
  maxTurns?: number;
  /** 建议是否开启并行工具执行 */
  parallelToolExecution?: boolean;
  /** 建议的最大并行工具数 */
  maxParallelTools?: number;
  /** 建议的最大连续失败次数 */
  maxConsecutiveFailures?: number;
  /** 调整原因 */
  reason: string;
  /** 置信度 [0, 1] */
  confidence: number;
}

/** 自省统计 */
export interface IntrospectionStats {
  /** 总运行次数 */
  totalRuns: number;
  /** 成功运行次数 */
  successfulRuns: number;
  /** 平均耗时（毫秒） */
  avgDuration: number;
  /** 平均轮次 */
  avgTurns: number;
  /** 平均工具调用次数 */
  avgTools: number;
  /** 工具成功率 */
  toolSuccessRate: number;
  /** 平均 LLM 延迟（毫秒） */
  avgLLMLatency: number;
  /** 平均工具延迟（毫秒） */
  avgToolLatency: number;
  /** 平均 Token 使用量 */
  avgTokens: number;
  /** P95 耗时（毫秒） */
  p95Duration: number;
  /** 趋势：最近 5 次 vs 之前 5 次的耗时变化比例 */
  trendRatio: number;
}

// ===== Agent 自省调优器 =====

const MAX_HISTORY = 100;
const MIN_RUNS_FOR_TUNING = 5;

export class AgentSelfTuner {
  private history: RunMetrics[] = [];
  private currentConfig: {
    maxTurns: number;
    parallelToolExecution: boolean;
    maxParallelTools: number;
    maxConsecutiveFailures: number;
  };

  constructor(config?: {
    maxTurns?: number;
    parallelToolExecution?: boolean;
    maxParallelTools?: number;
    maxConsecutiveFailures?: number;
  }) {
    this.currentConfig = {
      maxTurns: config?.maxTurns ?? 10,
      parallelToolExecution: config?.parallelToolExecution ?? false,
      maxParallelTools: config?.maxParallelTools ?? 0,
      maxConsecutiveFailures: config?.maxConsecutiveFailures ?? 3,
    };
  }

  /** 记录一次运行指标 */
  recordRun(metrics: RunMetrics): void {
    this.history.push(metrics);
    if (this.history.length > MAX_HISTORY) {
      this.history.shift();
    }
  }

  /** 获取当前配置 */
  get config() {
    return { ...this.currentConfig };
  }

  /** 获取自省统计 */
  getStats(): IntrospectionStats {
    if (this.history.length === 0) {
      return {
        totalRuns: 0,
        successfulRuns: 0,
        avgDuration: 0,
        avgTurns: 0,
        avgTools: 0,
        toolSuccessRate: 0,
        avgLLMLatency: 0,
        avgToolLatency: 0,
        avgTokens: 0,
        p95Duration: 0,
        trendRatio: 1,
      };
    }

    const runs = this.history;
    const successful = runs.filter((r) => r.success);
    const durations = runs.map((r) => r.duration).sort((a, b) => a - b);
    const p95Idx = Math.floor(durations.length * 0.95);

    const totalTools = runs.reduce((sum, r) => sum + r.totalTools, 0);
    const totalToolFailures = runs.reduce((sum, r) => sum + (r.toolFailures ?? 0), 0);
    const totalTokens = runs.reduce((sum, r) => sum + (r.totalTokens ?? 0), 0);

    // 趋势计算：最近 5 次 vs 之前 5 次
    let trendRatio = 1;
    if (runs.length >= 10) {
      const recent = runs.slice(-5);
      const previous = runs.slice(-10, -5);
      const recentAvg = recent.reduce((s, r) => s + r.duration, 0) / 5;
      const previousAvg = previous.reduce((s, r) => s + r.duration, 0) / 5;
      trendRatio = previousAvg > 0 ? recentAvg / previousAvg : 1;
    }

    return {
      totalRuns: runs.length,
      successfulRuns: successful.length,
      avgDuration: runs.reduce((s, r) => s + r.duration, 0) / runs.length,
      avgTurns: runs.reduce((s, r) => s + r.totalTurns, 0) / runs.length,
      avgTools: totalTools / runs.length,
      toolSuccessRate: totalTools > 0 ? (totalTools - totalToolFailures) / totalTools : 1,
      avgLLMLatency: runs.reduce((s, r) => s + r.llmLatency, 0) / runs.length,
      avgToolLatency: runs.reduce((s, r) => s + r.toolLatency, 0) / runs.length,
      avgTokens: totalTokens / runs.length,
      p95Duration: durations[p95Idx] ?? durations[durations.length - 1],
      trendRatio,
    };
  }

  /** 基于历史数据生成调优建议 */
  getSuggestion(): TuningSuggestion {
    if (this.history.length < MIN_RUNS_FOR_TUNING) {
      return {
        shouldAdjust: false,
        reason: `Insufficient data: need ${MIN_RUNS_FOR_TUNING} runs, have ${this.history.length}`,
        confidence: 0,
      };
    }

    const stats = this.getStats();
    const suggestions: string[] = [];
    let confidence = 0.5;

    // 1. maxTurns 调优：如果大多数运行在 3 轮内完成，可以降低 maxTurns
    if (stats.avgTurns < this.currentConfig.maxTurns * 0.5 && stats.successfulRuns / stats.totalRuns > 0.8) {
      const newMaxTurns = Math.max(3, Math.ceil(stats.avgTurns * 1.5));
      if (newMaxTurns < this.currentConfig.maxTurns) {
        suggestions.push(`maxTurns: ${this.currentConfig.maxTurns} → ${newMaxTurns} (avg turns = ${stats.avgTurns.toFixed(1)})`);
        this.currentConfig.maxTurns = newMaxTurns;
        confidence += 0.15;
      }
    } else if (stats.successfulRuns / stats.totalRuns < 0.7 && this.currentConfig.maxTurns < 50) {
      // 成功率低且未达上限，建议增加 maxTurns
      const newMaxTurns = Math.min(50, this.currentConfig.maxTurns + 5);
      suggestions.push(`maxTurns: ${this.currentConfig.maxTurns} → ${newMaxTurns} (success rate = ${(stats.successfulRuns / stats.totalRuns * 100).toFixed(0)}%)`);
      this.currentConfig.maxTurns = newMaxTurns;
      confidence += 0.1;
    }

    // 2. 并行工具执行调优：如果平均工具数 > 2 且工具延迟占比高
    const toolLatencyRatio = stats.avgDuration > 0 ? stats.avgToolLatency / stats.avgDuration : 0;
    if (stats.avgTools > 2 && toolLatencyRatio > 0.4 && !this.currentConfig.parallelToolExecution) {
      suggestions.push(`parallelToolExecution: false → true (avg tools = ${stats.avgTools.toFixed(1)}, tool latency ratio = ${(toolLatencyRatio * 100).toFixed(0)}%)`);
      this.currentConfig.parallelToolExecution = true;
      this.currentConfig.maxParallelTools = Math.ceil(stats.avgTools);
      confidence += 0.2;
    }

    // 3. maxConsecutiveFailures 调优：如果工具成功率低
    if (stats.toolSuccessRate < 0.8 && this.currentConfig.maxConsecutiveFailures < 5) {
      const newMax = Math.min(5, this.currentConfig.maxConsecutiveFailures + 1);
      suggestions.push(`maxConsecutiveFailures: ${this.currentConfig.maxConsecutiveFailures} → ${newMax} (tool success rate = ${(stats.toolSuccessRate * 100).toFixed(0)}%)`);
      this.currentConfig.maxConsecutiveFailures = newMax;
      confidence += 0.1;
    }

    // 4. 趋势分析：如果性能在下降
    if (stats.trendRatio > 1.2) {
      suggestions.push(`Performance degrading: recent runs ${(stats.trendRatio * 100 - 100).toFixed(0)}% slower than previous`);
      confidence += 0.1;
    } else if (stats.trendRatio < 0.9) {
      suggestions.push(`Performance improving: recent runs ${(100 - stats.trendRatio * 100).toFixed(0)}% faster than previous`);
    }

    return {
      shouldAdjust: suggestions.length > 0,
      maxTurns: this.currentConfig.maxTurns,
      parallelToolExecution: this.currentConfig.parallelToolExecution,
      maxParallelTools: this.currentConfig.maxParallelTools,
      maxConsecutiveFailures: this.currentConfig.maxConsecutiveFailures,
      reason: suggestions.length > 0 ? suggestions.join('; ') : 'No adjustments needed',
      confidence: Math.min(1, confidence),
    };
  }

  /** 清空历史记录 */
  reset(): void {
    this.history = [];
  }

  /** 导出历史记录（用于持久化） */
  exportHistory(): RunMetrics[] {
    return [...this.history];
  }

  /** 导入历史记录 */
  importHistory(data: RunMetrics[]): void {
    this.history = [...data].slice(-MAX_HISTORY);
  }
}
