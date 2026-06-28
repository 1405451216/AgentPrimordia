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
}

export interface ToolLearner {
  recordSuccess(toolName: string, args: string, result: string): Promise<void>;
  recordFailure(toolName: string, args: string, errorMsg: string): Promise<void>;
  getBestPractices(toolName: string): Promise<BestPractice[]>;
  suggestImprovement(toolName: string, args: string): Promise<Suggestion>;
}

// ===== Memory Tool Learner =====

export class MemoryToolLearner implements ToolLearner {
  private memory: Memory;
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
    const successRate = successRecords.length / records.length;

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

  private addToCache(record: ToolUsageRecord): void {
    if (!this.records.has(record.toolName)) {
      this.records.set(record.toolName, []);
    }
    this.records.get(record.toolName)!.push(record);
    // Keep last 100 records per tool
    const records = this.records.get(record.toolName)!;
    if (records.length > 100) records.shift();
  }

  private extractPattern(args: string): string {
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
