/**
 * 投机执行 (Speculative Execution) — TS SDK 独有性能优化。
 *
 * 核心思想：在工具执行期间，提前预热下一轮 LLM 调用。
 * 假设工具调用大概率会成功，我们可以：
 * 1. 在工具开始执行的同时，启动一个「预测性」LLM 调用
 * 2. 将工具结果作为「预测输入」注入预测消息
 * 3. 工具完成后，如果预测命中（工具结果与预测一致），直接使用预测结果
 * 4. 如果预测未命中，取消预测并正常执行
 *
 * 适用场景：
 * - 工具延迟较高（如 HTTP 请求、数据库查询）
 * - 工具结果可预测（如文件读取、简单计算）
 * - LLM 调用延迟较高（需要跨网络）
 *
 * 使用方式：
 *   const specExec = new SpeculativeExecutor(provider, toolkit);
 *   const result = await specExec.executeWithSpeculation(messages, toolCalls);
 *
 * 注意：此模块独立于 ReActAgent，可通过 Hook 或包装器集成。
 */

import type { Provider } from '../llm/provider.js';
import type { Message, ToolCall, ToolResult, ToolCallRequest, ToolCallResponse, CompletionRequest, CompletionResponse } from '../types.js';
import type { ToolRegistry } from '../tools/registry.js';

// ===== 类型定义 =====

/** 投机执行配置 */
export interface SpeculativeExecConfig {
  /** 是否启用投机执行，默认 true */
  enabled?: boolean;
  /** 预测命中率阈值：低于此值则自动禁用，默认 0.3 */
  minHitRate?: number;
  /** 最大投机深度：连续投机多少轮后强制等待，默认 2 */
  maxSpecDepth?: number;
  /** 投机超时（毫秒），超时则丢弃预测结果，默认 10000 */
  speculationTimeoutMs?: number;
}

/** 投机执行结果 */
export interface SpeculativeResult {
  /** 最终的 LLM 响应 */
  response: CompletionResponse | ToolCallResponse;
  /** 投机是否命中 */
  speculationHit: boolean;
  /** 投机节省的时间（毫秒） */
  timeSavedMs: number;
  /** 工具执行结果 */
  toolResults: ToolResult[];
}

/** 投机执行统计 */
export interface SpeculationStats {
  /** 总投机次数 */
  totalSpeculations: number;
  /** 命中次数 */
  hits: number;
  /** 命中率 */
  hitRate: number;
  /** 总节省时间（毫秒） */
  totalTimeSavedMs: number;
  /** 平均节省时间（毫秒） */
  avgTimeSavedMs: number;
}

// ===== 工具结果预测器 =====

/**
 * 工具结果预测器 — 基于工具名称和历史模式预测工具结果。
 *
 * 预测策略：
 * 1. 对于确定性工具（如计算、格式转换），预测精确结果
 * 2. 对于 I/O 工具（如文件读取），预测格式模板
 * 3. 对于未知工具，使用上一次成功结果作为参考
 */
export class ToolResultPredictor {
  private history: Map<string, ToolResult[]> = new Map();
  private maxHistoryPerTool = 10;

  /** 记录工具执行结果，用于未来预测 */
  recordResult(toolName: string, result: ToolResult): void {
    if (!this.history.has(toolName)) {
      this.history.set(toolName, []);
    }
    const records = this.history.get(toolName)!;
    records.push(result);
    if (records.length > this.maxHistoryPerTool) records.shift();
  }

  /** 预测工具结果 */
  predict(toolName: string, _args: string): ToolResult | null {
    const records = this.history.get(toolName);
    if (!records || records.length === 0) return null;

    // 简单策略：使用最近一次成功结果作为预测
    const lastSuccess = [...records].reverse().find((r) => !r.isError);
    if (!lastSuccess) return null;

    // 对于确定性工具，如果参数相同则结果大概率相同
    return {
      toolCallId: 'speculative',
      content: lastSuccess.content,
      isError: false,
    };
  }

  /** 检查预测是否命中（实际结果与预测一致） */
  isHit(predicted: ToolResult, actual: ToolResult): boolean {
    if (predicted.isError !== actual.isError) return false;
    // 内容相似度检查：如果前 200 字符相同则认为命中
    const predPrefix = predicted.content.slice(0, 200);
    const actualPrefix = actual.content.slice(0, 200);
    return predPrefix === actualPrefix;
  }

  /** 清空历史 */
  reset(): void {
    this.history.clear();
  }
}

// ===== 投机执行器 =====

/**
 * 投机执行器 — 在工具执行期间并行启动下一轮 LLM 调用。
 *
 * 工作流程：
 * 1. LLM 返回工具调用 → 正常执行工具
 * 2. 同时：预测工具结果 → 用预测结果构造消息 → 启动下一轮 LLM 调用
 * 3. 工具执行完成 → 比较实际结果与预测
 * 4. 命中：直接使用步骤 2 的 LLM 结果（节省一轮 LLM 延迟）
 * 5. 未命中：丢弃步骤 2 的结果，用实际结果重新调用 LLM
 */
export class SpeculativeExecutor {
  private provider: Provider;
  private toolkit: ToolRegistry;
  private predictor: ToolResultPredictor;
  private config: Required<SpeculativeExecConfig>;
  private stats: { total: number; hits: number; timeSavedMs: number } = {
    total: 0, hits: 0, timeSavedMs: 0,
  };
  private autoDisabled = false;

  constructor(
    provider: Provider,
    toolkit: ToolRegistry,
    config?: SpeculativeExecConfig,
  ) {
    this.provider = provider;
    this.toolkit = toolkit;
    this.predictor = new ToolResultPredictor();
    this.config = {
      enabled: config?.enabled ?? true,
      minHitRate: config?.minHitRate ?? 0.3,
      maxSpecDepth: config?.maxSpecDepth ?? 2,
      speculationTimeoutMs: config?.speculationTimeoutMs ?? 10000,
    };
  }

  /** 带投机执行的 ReAct 单轮处理 */
  async executeWithSpeculation(
    messages: Message[],
    toolCalls: ToolCall[],
  ): Promise<SpeculativeResult> {
    const startTime = Date.now();

    // 1. 预测所有工具结果
    const predictions = toolCalls.map((tc) => ({
      toolCall: tc,
      predicted: this.predictor.predict(tc.name, tc.arguments),
    }));

    // 检查是否可以投机（至少有一个预测）
    const canSpeculate = this.config.enabled && !this.autoDisabled &&
      predictions.some((p) => p.predicted !== null);

    // 2. 启动工具执行（实际）
    const toolExecPromise = this.executeTools(toolCalls);

    // 3. 如果可以投机，同时启动下一轮 LLM 调用（预测性）
    let speculationPromise: Promise<CompletionResponse | ToolCallResponse> | null = null;
    if (canSpeculate) {
      const speculativeMessages = this.buildSpeculativeMessages(messages, toolCalls, predictions);
      speculationPromise = this.callLLM(speculativeMessages);
    }

    // 4. 等待工具执行完成
    const toolResults = await toolExecPromise;

    // 5. 记录工具结果用于未来预测
    for (const result of toolResults) {
      const tc = toolCalls.find((t) => t.id === result.toolCallId);
      if (tc) {
        this.predictor.recordResult(tc.name, result);
      }
    }

    // 6. 检查投机是否命中
    if (speculationPromise) {
      // 检查所有预测是否命中
      let allHit = true;
      for (let i = 0; i < toolCalls.length; i++) {
        const predicted = predictions[i].predicted;
        if (predicted && i < toolResults.length) {
          if (!this.predictor.isHit(predicted, toolResults[i])) {
            allHit = false;
            break;
          }
        } else if (!predicted) {
          // 没有预测的工具，只要结果不是错误就算命中
          if (toolResults[i].isError) {
            allHit = false;
            break;
          }
        }
      }

      if (allHit) {
        // 投机命中！使用预测的 LLM 结果
        try {
          const specResult = await this.raceWithTimeout(speculationPromise, this.config.speculationTimeoutMs);
          const timeSavedMs = Date.now() - startTime;
          this.stats.total++;
          this.stats.hits++;
          this.stats.timeSavedMs += timeSavedMs;
          return {
            response: specResult,
            speculationHit: true,
            timeSavedMs,
            toolResults,
          };
        } catch {
          // 投机超时或失败，回退到正常流程
        }
      } else {
        // 投机未命中，丢弃预测结果
        this.stats.total++;
        // 自动禁用检查
        if (this.stats.total >= 5) {
          const hitRate = this.stats.hits / this.stats.total;
          if (hitRate < this.config.minHitRate) {
            this.autoDisabled = true;
          }
        }
      }
    }

    // 7. 正常流程：用实际结果调用 LLM
    const finalMessages = this.buildFinalMessages(messages, toolCalls, toolResults);
    const response = await this.callLLM(finalMessages);

    return {
      response,
      speculationHit: false,
      timeSavedMs: 0,
      toolResults,
    };
  }

  /** 获取投机执行统计 */
  getStats(): SpeculationStats {
    const total = this.stats.total;
    const hits = this.stats.hits;
    return {
      totalSpeculations: total,
      hits,
      hitRate: total > 0 ? hits / total : 0,
      totalTimeSavedMs: this.stats.timeSavedMs,
      avgTimeSavedMs: total > 0 ? this.stats.timeSavedMs / total : 0,
    };
  }

  /** 重置统计和预测器 */
  reset(): void {
    this.stats = { total: 0, hits: 0, timeSavedMs: 0 };
    this.predictor.reset();
    this.autoDisabled = false;
  }

  /** 是否已自动禁用 */
  isAutoDisabled(): boolean {
    return this.autoDisabled;
  }

  // ===== 内部方法 =====

  private async executeTools(toolCalls: ToolCall[]): Promise<ToolResult[]> {
    const results: ToolResult[] = [];
    for (const tc of toolCalls) {
      const result = await this.toolkit.execute(tc);
      results.push(result);
    }
    return results;
  }

  private buildSpeculativeMessages(
    messages: Message[],
    toolCalls: ToolCall[],
    predictions: { toolCall: ToolCall; predicted: ToolResult | null }[],
  ): Message[] {
    const specMessages = [...messages];

    // 添加 assistant 消息（工具调用）
    specMessages.push({
      role: 'assistant',
      content: '',
      toolCalls,
    });

    // 添加预测的工具结果
    for (const { toolCall, predicted } of predictions) {
      specMessages.push({
        role: 'tool',
        content: predicted?.content ?? '[executing...]',
        toolCallId: toolCall.id,
        name: toolCall.name,
      });
    }

    return specMessages;
  }

  private buildFinalMessages(
    messages: Message[],
    toolCalls: ToolCall[],
    toolResults: ToolResult[],
  ): Message[] {
    const finalMessages = [...messages];

    // 添加 assistant 消息（工具调用）
    finalMessages.push({
      role: 'assistant',
      content: '',
      toolCalls,
    });

    // 添加实际工具结果
    for (let i = 0; i < toolCalls.length; i++) {
      const tc = toolCalls[i];
      const result = toolResults[i];
      finalMessages.push({
        role: 'tool',
        content: result.content,
        toolCallId: tc.id,
        name: tc.name,
      });
    }

    return finalMessages;
  }

  private async callLLM(
    messages: Message[],
  ): Promise<CompletionResponse | ToolCallResponse> {
    if (this.toolkit.size() > 0) {
      return this.provider.callTools({
        messages,
        tools: this.toolkit.definitions(),
      } as ToolCallRequest);
    }
    return this.provider.complete({ messages } as CompletionRequest);
  }

  private async raceWithTimeout<T>(
    promise: Promise<T>,
    timeoutMs: number,
  ): Promise<T> {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const timeout = new Promise<never>((_, reject) => {
      timer = setTimeout(() => reject(new Error('Speculation timeout')), timeoutMs);
    });
    try {
      return await Promise.race([promise, timeout]);
    } finally {
      if (timer) clearTimeout(timer);
    }
  }
}
