import type { Memory } from '../memory/store.js';

// ===== Tool Learning Types =====

export interface BestPractice {
  toolName: string;
  pattern: string;
  description: string;
  successRate: number;
  examples: string[];
  createdAt: string;
}

export interface Suggestion {
  originalArgs: string;
  improvedArgs: string;
  reason: string;
  confidence: number;
}

export interface ToolUsageRecord {
  toolName: string;
  args: string;
  result?: string;
  error?: string;
  success: boolean;
  timestamp: string;
  /** 可选：执行耗时（毫秒）。未记录时 avgLatencyMs 在聚合时被跳过。 */
  latencyMs?: number;
}

export interface ToolLearner {
  recordSuccess(toolName: string, args: string, result: string): Promise<void>;
  recordFailure(toolName: string, args: string, errorMsg: string): Promise<void>;
  getBestPractices(toolName: string): Promise<BestPractice[]>;
  suggestImprovement(toolName: string, args: string): Promise<Suggestion>;
}

// ===== Memory Tool Learner =====

export class MemoryToolLearner implements ToolLearner {
  // 提升为 protected 以允许子类（EnhancedToolLearner）在 recordSuccessWithTiming
  // / recordFailureWithTiming 中直接复用同一 memory 句柄写入 tool_learning 事件。
  // 修复 TS2341: Property 'memory' is private and only accessible within class 'MemoryToolLearner'.
  // 仅类型可见性变更，运行时行为不变。
  protected memory: Memory;
  private records: Map<string, ToolUsageRecord[]> = new Map();

  constructor(memory: Memory) {
    this.memory = memory;
  }

  async recordSuccess(toolName: string, args: string, result: string): Promise<void> {
    const record: ToolUsageRecord = {
      toolName,
      args,
      result,
      success: true,
      timestamp: new Date().toISOString(),
    };

    this.addToCache(record);
    await this.memory.add({
      id: `tl-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      sessionId: 'tool-learning',
      role: 'system',
      content: JSON.stringify(record),
      metadata: { type: 'tool_learning', toolName, success: 'true' },
      createdAt: new Date().toISOString(),
    });
  }

  async recordFailure(toolName: string, args: string, errorMsg: string): Promise<void> {
    const record: ToolUsageRecord = {
      toolName,
      args,
      error: errorMsg,
      success: false,
      timestamp: new Date().toISOString(),
    };

    this.addToCache(record);
    await this.memory.add({
      id: `tl-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      sessionId: 'tool-learning',
      role: 'system',
      content: JSON.stringify(record),
      metadata: { type: 'tool_learning', toolName, success: 'false' },
      createdAt: new Date().toISOString(),
    });
  }

  async getBestPractices(toolName: string): Promise<BestPractice[]> {
    const records = this.records.get(toolName) ?? [];
    if (records.length === 0) return [];

    const successRecords = records.filter(r => r.success);
    const _successRate = successRecords.length / records.length;

    // Group by similar argument patterns
    const patterns: Map<string, ToolUsageRecord[]> = new Map();
    for (const record of successRecords) {
      const pattern = this.extractPattern(record.args);
      if (!patterns.has(pattern)) patterns.set(pattern, []);
      patterns.get(pattern)!.push(record);
    }

    const practices: BestPractice[] = [];
    for (const [pattern, patternRecords] of patterns) {
      const patternSuccessRate = patternRecords.length / records.filter(r =>
        this.extractPattern(r.args) === pattern
      ).length;

      practices.push({
        toolName,
        pattern,
        description: `Pattern "${pattern}" has ${(patternSuccessRate * 100).toFixed(0)}% success rate`,
        successRate: patternSuccessRate,
        examples: patternRecords.slice(0, 3).map(r => r.args),
        createdAt: patternRecords[0].timestamp,
      });
    }

    return practices.sort((a, b) => b.successRate - a.successRate);
  }

  async suggestImprovement(toolName: string, args: string): Promise<Suggestion> {
    const practices = await this.getBestPractices(toolName);
    if (practices.length === 0) {
      return {
        originalArgs: args,
        improvedArgs: args,
        reason: 'No historical data available',
        confidence: 0,
      };
    }

    // Find best matching pattern
    const inputPattern = this.extractPattern(args);
    const matching = practices.find(p => p.pattern === inputPattern);

    if (matching && matching.successRate < 0.5) {
      // Find a better pattern
      const better = practices.find(p => p.successRate > 0.7);
      if (better) {
        return {
          originalArgs: args,
          improvedArgs: better.examples[0] ?? args,
          reason: `Current pattern has ${(matching.successRate * 100).toFixed(0)}% success rate. Suggested pattern has ${(better.successRate * 100).toFixed(0)}% success rate.`,
          confidence: better.successRate,
        };
      }
    }

    return {
      originalArgs: args,
      improvedArgs: args,
      reason: 'Current approach looks good',
      confidence: matching?.successRate ?? 0.5,
    };
  }

  protected addToCache(record: ToolUsageRecord): void {
    if (!this.records.has(record.toolName)) {
      this.records.set(record.toolName, []);
    }
    this.records.get(record.toolName)!.push(record);
    // Keep last 100 records per tool
    const records = this.records.get(record.toolName)!;
    if (records.length > 100) records.shift();
  }

  protected extractPattern(args: string): string {
    // Simple pattern extraction: extract action/operation type
    try {
      const parsed = JSON.parse(args);
      if (parsed.action) return `action:${parsed.action}`;
      if (parsed.method) return `method:${parsed.method}`;
      if (parsed.command) return `command:${String(parsed.command).split(' ')[0]}`;
    } catch {}
    return args.slice(0, 50);
  }
}

// ===== P5: Tool Learning 闭环增强 =====
// 从执行结果中学习工具使用模式，自动生成 few-shot 示例。
// 与 Go 端 tool_learning/ 对齐，但增加了 few-shot 自动生成能力。

/** Few-shot 示例 */
export interface FewShotExample {
  /** 工具名称 */
  toolName: string;
  /** 输入（用户意图描述） */
  input: string;
  /** 工具调用参数 */
  toolArgs: string;
  /** 预期结果 */
  expectedResult: string;
  /** 质量分数 [0, 1] */
  quality: number;
  /** 来源（成功记录 ID） */
  source: string;
}

/** 工具使用模式 */
export interface ToolUsagePattern {
  /** 工具名称 */
  toolName: string;
  /** 模式名称 */
  patternName: string;
  /** 参数模板 */
  argTemplate: string;
  /** 使用频率 */
  frequency: number;
  /** 成功率 */
  successRate: number;
  /** 平均执行时间（毫秒） */
  avgLatencyMs: number;
  /** 典型使用场景 */
  scenarios: string[];
}

/** 闭环增强配置 */
export interface EnhancedToolLearningConfig {
  /** 最大 few-shot 示例数，默认 5 */
  maxFewShotExamples?: number;
  /** 质量分数阈值，默认 0.7 */
  qualityThreshold?: number;
  /** 最小记录数才能生成模式，默认 3 */
  minRecordsForPattern?: number;
}

/**
 * 增强版工具学习器 — 在 MemoryToolLearner 基础上增加闭环能力。
 *
 * 核心增强：
 * 1. 自动生成 few-shot 示例，注入到 system prompt 中
 * 2. 识别工具使用模式，生成工具使用指南
 * 3. 检测低效用法，生成改进建议
 * 4. 支持跨会话学习（通过 Memory 持久化）
 *
 * 使用方式：
 *   const learner = new EnhancedToolLearner(memory);
 *   // 记录工具使用
 *   await learner.recordSuccess('search', '{"q":"test"}', '结果: ...');
 *   // 生成 few-shot 示例
 *   const examples = await learner.generateFewShotExamples('search');
 *   // 生成工具使用指南
 *   const patterns = await learner.getUsagePatterns('search');
 */
export class EnhancedToolLearner extends MemoryToolLearner {
  private fewShotCache: Map<string, FewShotExample[]> = new Map();
  private patternCache: Map<string, ToolUsagePattern[]> = new Map();
  private config: Required<EnhancedToolLearningConfig>;
  private enhancedRecords: Map<string, ToolUsageRecord[]> = new Map();

  constructor(memory: Memory, config?: EnhancedToolLearningConfig) {
    super(memory);
    this.config = {
      maxFewShotExamples: config?.maxFewShotExamples ?? 5,
      qualityThreshold: config?.qualityThreshold ?? 0.7,
      minRecordsForPattern: config?.minRecordsForPattern ?? 3,
    };
  }

  /** 覆盖 recordSuccess，同时记录到本地缓存 */
  async recordSuccess(toolName: string, args: string, result: string): Promise<void> {
    await super.recordSuccess(toolName, args, result);
    this.addEnhancedRecord({
      toolName, args, result, success: true,
      timestamp: new Date().toISOString(),
    });
  }

  /** 带执行耗时的 recordSuccess 入口（推荐使用，avgLatencyMs 才有真实数据） */
  async recordSuccessWithTiming(
    toolName: string, args: string, result: string, latencyMs: number,
  ): Promise<void> {
    // 直接写父类 cache 以保留 latencyMs
    const ts = new Date().toISOString();
    this.addToCache({ toolName, args, result, success: true, timestamp: ts, latencyMs });
    await this.memory.add({
      id: `tl-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      sessionId: 'tool-learning',
      role: 'system',
      content: JSON.stringify({ toolName, args, result, success: true, timestamp: ts, latencyMs }),
      metadata: { type: 'tool_learning', toolName, success: 'true' },
      createdAt: ts,
    });
    this.addEnhancedRecord({
      toolName, args, result, success: true,
      timestamp: ts, latencyMs,
    });
  }

  /** 覆盖 recordFailure，同时记录到本地缓存 */
  async recordFailure(toolName: string, args: string, errorMsg: string): Promise<void> {
    await super.recordFailure(toolName, args, errorMsg);
    this.addEnhancedRecord({
      toolName, args, error: errorMsg, success: false,
      timestamp: new Date().toISOString(),
    });
  }

  /** 带执行耗时的 recordFailure 入口 */
  async recordFailureWithTiming(
    toolName: string, args: string, errorMsg: string, latencyMs: number,
  ): Promise<void> {
    // 直接写父类 cache 以保留 latencyMs
    const ts = new Date().toISOString();
    this.addToCache({ toolName, args, error: errorMsg, success: false, timestamp: ts, latencyMs });
    await this.memory.add({
      id: `tl-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      sessionId: 'tool-learning',
      role: 'system',
      content: JSON.stringify({ toolName, args, error: errorMsg, success: false, timestamp: ts, latencyMs }),
      metadata: { type: 'tool_learning', toolName, success: 'false' },
      createdAt: ts,
    });
    this.addEnhancedRecord({
      toolName, args, error: errorMsg, success: false,
      timestamp: ts, latencyMs,
    });
  }

  private addEnhancedRecord(record: ToolUsageRecord): void {
    if (!this.enhancedRecords.has(record.toolName)) {
      this.enhancedRecords.set(record.toolName, []);
    }
    this.enhancedRecords.get(record.toolName)!.push(record);
    const records = this.enhancedRecords.get(record.toolName)!;
    if (records.length > 200) records.shift();
  }

  /** 生成 few-shot 示例，可注入到 system prompt */
  async generateFewShotExamples(toolName: string): Promise<FewShotExample[]> {
    // 检查缓存
    const cached = this.fewShotCache.get(toolName);
    if (cached) return cached;

    const practices = await this.getBestPractices(toolName);
    if (practices.length === 0) return [];

    // 从最佳实践中提取高质量示例
    const examples: FewShotExample[] = [];
    for (const practice of practices) {
      if (practice.successRate < this.config.qualityThreshold) continue;
      for (const exampleArgs of practice.examples) {
        // 提取输入意图
        const input = this.extractIntent(exampleArgs);
        // 查找对应的成功结果
        const result = await this.findResult(toolName, exampleArgs);

        examples.push({
          toolName,
          input,
          toolArgs: exampleArgs,
          expectedResult: result ?? '[successful execution]',
          quality: practice.successRate,
          source: `${toolName}-${practice.pattern}`,
        });
      }
    }

    // 按质量排序，取 top N
    const sorted = examples
      .sort((a, b) => b.quality - a.quality)
      .slice(0, this.config.maxFewShotExamples);

    this.fewShotCache.set(toolName, sorted);
    return sorted;
  }

  /** 生成工具使用模式报告 */
  async getUsagePatterns(toolName: string): Promise<ToolUsagePattern[]> {
    const cached = this.patternCache.get(toolName);
    if (cached) return cached;

    const practices = await this.getBestPractices(toolName);
    if (practices.length < this.config.minRecordsForPattern) return [];

    // 从 enhancedRecords 中按 pattern 分组，计算真实平均延迟
    const records = this.enhancedRecords.get(toolName) ?? [];
    const patternLatencies = new Map<string, number[]>();
    for (const r of records) {
      if (r.latencyMs == null) continue;
      const pattern = this.extractPattern(r.args);
      if (!patternLatencies.has(pattern)) patternLatencies.set(pattern, []);
      patternLatencies.get(pattern)!.push(r.latencyMs);
    }

    const patterns: ToolUsagePattern[] = practices.map((p) => {
      const latencies = patternLatencies.get(p.pattern) ?? [];
      const avgLatencyMs = latencies.length > 0
        ? Math.round(latencies.reduce((a, b) => a + b, 0) / latencies.length)
        : 0;
      return {
        toolName,
        patternName: p.pattern,
        argTemplate: this.extractArgTemplate(p.examples[0] ?? ''),
        frequency: p.examples.length,
        successRate: p.successRate,
        avgLatencyMs,
        scenarios: this.extractScenarios(p),
      };
    });

    const sorted = patterns.sort((a, b) => b.successRate * b.frequency - a.successRate * a.frequency);
    this.patternCache.set(toolName, sorted);
    return sorted;
  }

  /** 生成工具使用指南（可注入 system prompt） */
  async generateUsageGuide(toolNames: string[]): Promise<string> {
    const sections: string[] = [];

    for (const toolName of toolNames) {
      const patterns = await this.getUsagePatterns(toolName);
      if (patterns.length === 0) continue;

      const examples = await this.generateFewShotExamples(toolName);
      sections.push(this.buildToolGuideSection(toolName, patterns, examples));
    }

    if (sections.length === 0) return '';

    return [
      '## Tool Usage Guide (Auto-Generated from Historical Data)\n',
      ...sections,
    ].join('\n');
  }

  /** 检测低效用法并生成改进建议 */
  async detectInefficiencies(toolName: string): Promise<string[]> {
    const suggestions: string[] = [];

    // 使用增强记录（包含成功和失败）来计算各模式的真实成功率
    const records = this.enhancedRecords.get(toolName) ?? [];
    if (records.length === 0) return suggestions;

    // 按模式分组
    const patternGroups: Map<string, ToolUsageRecord[]> = new Map();
    for (const record of records) {
      const pattern = this.enhancedExtractPattern(record.args);
      if (!patternGroups.has(pattern)) patternGroups.set(pattern, []);
      patternGroups.get(pattern)!.push(record);
    }

    // 计算每个模式的成功率
    const patternStats: { pattern: string; successRate: number; total: number }[] = [];
    for (const [pattern, groupRecords] of patternGroups) {
      const successCount = groupRecords.filter((r) => r.success).length;
      const successRate = successCount / groupRecords.length;
      patternStats.push({ pattern, successRate, total: groupRecords.length });
    }

    // 检测低成功率模式
    for (const stat of patternStats) {
      if (stat.successRate < 0.5) {
        suggestions.push(
          `Pattern "${stat.pattern}" has low success rate (${(stat.successRate * 100).toFixed(0)}%). ` +
          `Consider using a different approach.`,
        );
      }
    }

    // 检查是否有更好的替代模式
    const sorted = [...patternStats].sort((a, b) => b.successRate - a.successRate);
    if (sorted.length >= 2 && sorted[0]!.successRate > 0.8 && sorted[sorted.length - 1]!.successRate < 0.4) {
      suggestions.push(
        `Consider using pattern "${sorted[0]!.pattern}" (success rate: ${(sorted[0]!.successRate * 100).toFixed(0)}%) ` +
        `instead of "${sorted[sorted.length - 1]!.pattern}" (success rate: ${(sorted[sorted.length - 1]!.successRate * 100).toFixed(0)}%).`,
      );
    }

    return suggestions;
  }

  /** 清除缓存 */
  clearCache(): void {
    this.fewShotCache.clear();
    this.patternCache.clear();
    // 不清除 enhancedRecords，因为它是数据源而非缓存
  }

  // ===== 内部方法 =====

  /** 参数模式提取（与 MemoryToolLearner.extractPattern 逻辑一致） */
  private enhancedExtractPattern(args: string): string {
    try {
      const parsed = JSON.parse(args);
      if (parsed.action) return `action:${parsed.action}`;
      if (parsed.method) return `method:${parsed.method}`;
      if (parsed.command) return `command:${String(parsed.command).split(' ')[0]}`;
    } catch {}
    return args.slice(0, 50);
  }

  private extractIntent(args: string): string {
    try {
      const parsed = JSON.parse(args);
      if (parsed.query) return `Search for: ${parsed.query}`;
      if (parsed.command) return `Execute: ${parsed.command}`;
      if (parsed.path) return `Access: ${parsed.path}`;
      if (parsed.action) return `Action: ${parsed.action}`;
      return `Use ${Object.keys(parsed).join(', ')}`;
    } catch {
      return args.slice(0, 100);
    }
  }

  private async findResult(toolName: string, args: string): Promise<string | null> {
    // 从 MemoryToolLearner 的记录中查找
    // 注意：这里简化实现，实际需要从 memory 中查询
    void toolName;
    void args;
    return null;
  }

  private extractArgTemplate(exampleArgs: string): string {
    try {
      const parsed = JSON.parse(exampleArgs);
      const template: Record<string, string> = {};
      for (const [key, value] of Object.entries(parsed)) {
        template[key] = typeof value === 'string' ? `<${key}>` : JSON.stringify(value);
      }
      return JSON.stringify(template);
    } catch {
      return '<args>';
    }
  }

  private extractScenarios(practice: BestPractice): string[] {
    const scenarios: string[] = [];
    if (practice.pattern.startsWith('action:')) {
      scenarios.push(`When performing ${practice.pattern.slice(7)}`);
    }
    if (practice.pattern.startsWith('command:')) {
      scenarios.push(`When running ${practice.pattern.slice(8)} commands`);
    }
    if (practice.pattern.startsWith('method:')) {
      scenarios.push(`When using ${practice.pattern.slice(7)} method`);
    }
    return scenarios.length > 0 ? scenarios : ['General use'];
  }

  private buildToolGuideSection(
    toolName: string,
    patterns: ToolUsagePattern[],
    examples: FewShotExample[],
  ): string {
    const lines: string[] = [`### ${toolName}\n`];

    if (patterns.length > 0) {
      lines.push('**Best patterns:**');
      for (const p of patterns.slice(0, 3)) {
        lines.push(`- \`${p.argTemplate}\` — success: ${(p.successRate * 100).toFixed(0)}%, uses: ${p.frequency}`);
      }
      lines.push('');
    }

    if (examples.length > 0) {
      lines.push('**Examples:**');
      for (const ex of examples.slice(0, 3)) {
        lines.push(`- Input: ${ex.input}`);
        lines.push(`  Args: \`${ex.toolArgs}\``);
        lines.push(`  Result: ${ex.expectedResult.slice(0, 100)}`);
      }
      lines.push('');
    }

    return lines.join('\n');
  }
}

