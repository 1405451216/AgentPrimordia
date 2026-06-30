/**
 * 智能模型路由 — 成本感知 + 任务复杂度路由。
 *
 * 根据输入消息的复杂度、上下文长度、是否需要工具调用等因素，
 * 自动选择最合适的模型（如 GPT-4o 用于复杂推理，GPT-4o-mini 用于简单问答）。
 *
 * 核心策略：
 * - 上下文长度路由：长上下文 → 大窗口模型
 * - 复杂度路由：代码生成/多步推理 → 高能力模型
 * - 成本感知：简单任务 → 低价模型，节省 Token 成本
 * - 降级策略：主模型失败时自动降级到备用模型
 *
 * 使用方式：
 *   const router = new ModelRouter()
 *     .register('simple', cheapProvider, { costPer1K: 0.0005, complexityLimit: 0.3 })
 *     .register('advanced', expensiveProvider, { costPer1K: 0.03, complexityLimit: 1.0 });
 *   const provider = router.route(messages);
 */

import type { Provider } from './provider.js';
import type { Message, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo } from '../types.js';

// ===== 类型定义 =====

/** 模型注册配置 */
export interface ModelRouteConfig {
  /** 模型标识名 */
  name: string;
  /** 模型成本（美元 / 1K Token） */
  costPer1K: number;
  /** 该模型能处理的复杂度上限 [0, 1] */
  complexityLimit: number;
  /** 该模型能处理的最大上下文长度（Token） */
  maxContext: number;
  /** 是否支持工具调用 */
  supportsTools: boolean;
  /** 优先级（数字越小优先级越高，默认 1） */
  priority?: number;
}

/** 路由决策结果 */
export interface RouteDecision {
  /** 选中的模型名 */
  modelName: string;
  /** 选中的 Provider */
  provider: Provider;
  /** 预估复杂度 [0, 1] */
  complexity: number;
  /** 预估成本（美元） */
  estimatedCost: number;
  /** 路由原因 */
  reason: string;
}

/** 路由策略 */
export type RouteStrategy = 'cost-first' | 'quality-first' | 'balanced';

// ===== 复杂度评估器 =====

/**
 * 评估输入消息的复杂度。
 *
 * 启发式规则：
 * - 消息长度 → 长消息复杂度更高
 * - 代码块数量 → 含代码的消息复杂度更高
 * - 推理关键词 → "分析"、"推理"、"设计"、"对比" 等
 * - 数学/逻辑 → 含数学公式或逻辑推理
 * - 多步骤 → 含"步骤"、"计划"、"流程" 等
 */
export class ComplexityEvaluator {
  private static readonly REASONING_KEYWORDS = [
    '分析', '推理', '设计', '对比', '评估', '优化', '重构', '架构',
    'analyze', 'reason', 'design', 'compare', 'evaluate', 'optimize',
    'refactor', 'architecture', 'explain', 'derive', 'prove',
  ];

  private static readonly MULTI_STEP_KEYWORDS = [
    '步骤', '计划', '流程', '分步', '阶段',
    'step', 'plan', 'process', 'pipeline', 'workflow',
  ];

  private static readonly CODE_PATTERNS = [
    /```[\s\S]*?```/g,
    /function\s+\w+/g,
    /class\s+\w+/g,
    /import\s+/g,
    /const\s+\w+\s*=/g,
    /def\s+\w+/g,
  ];

  /** 评估消息复杂度，返回 [0, 1] */
  evaluate(messages: Message[]): number {
    if (messages.length === 0) return 0;

    const lastUser = [...messages].reverse().find((m) => m.role === 'user');
    if (!lastUser) return 0.2;

    const content = lastUser.content ?? '';
    let score = 0;

    // 1. 长度因素（最长 2000 字符 → 满分）
    score += Math.min(content.length / 2000, 1) * 0.25;

    // 2. 代码块因素
    let codeBlocks = 0;
    for (const pattern of ComplexityEvaluator.CODE_PATTERNS) {
      const matches = content.match(pattern);
      if (matches) codeBlocks += matches.length;
    }
    score += Math.min(codeBlocks / 5, 1) * 0.3;

    // 3. 推理关键词
    let reasoningHits = 0;
    const lowerContent = content.toLowerCase();
    for (const kw of ComplexityEvaluator.REASONING_KEYWORDS) {
      if (lowerContent.includes(kw.toLowerCase())) reasoningHits++;
    }
    score += Math.min(reasoningHits / 3, 1) * 0.25;

    // 4. 多步骤因素
    let multiStepHits = 0;
    for (const kw of ComplexityEvaluator.MULTI_STEP_KEYWORDS) {
      if (lowerContent.includes(kw.toLowerCase())) multiStepHits++;
    }
    score += Math.min(multiStepHits / 2, 1) * 0.1;

    // 5. 上下文消息数
    score += Math.min(messages.length / 20, 1) * 0.1;

    return Math.min(score, 1);
  }
}

// ===== Token 估算器 =====

/** 粗略估算 Token 数量（4 字符 ≈ 1 Token） */
export function estimateTokens(messages: Message[]): number {
  let chars = 0;
  for (const msg of messages) {
    chars += (msg.content ?? '').length;
    if (msg.toolCalls) {
      for (const tc of msg.toolCalls) {
        chars += JSON.stringify(tc.arguments ?? {}).length;
      }
    }
  }
  return Math.ceil(chars / 4);
}

// ===== 模型路由器 =====

/**
 * 智能模型路由器。
 *
 * 注册多个模型 Provider，根据任务复杂度和成本自动路由。
 * 实现 Provider 接口，可直接注入 ReActAgent。
 */
export class ModelRouter implements Provider {
  private models: Map<string, { provider: Provider; config: ModelRouteConfig }> = new Map();
  private evaluator = new ComplexityEvaluator();
  private strategy: RouteStrategy;
  private fallbackModel?: string;
  private routeHistory: RouteDecision[] = [];

  constructor(strategy: RouteStrategy = 'balanced') {
    this.strategy = strategy;
  }

  /** 注册一个模型 */
  register(name: string, provider: Provider, config: Omit<ModelRouteConfig, 'name'>): this {
    this.models.set(name, {
      provider,
      config: { ...config, name },
    });
    return this;
  }

  /** 设置降级模型 */
  setFallback(name: string): this {
    this.fallbackModel = name;
    return this;
  }

  /** 设置路由策略 */
  setStrategy(strategy: RouteStrategy): this {
    this.strategy = strategy;
    return this;
  }

  /** 获取路由历史 */
  getHistory(): readonly RouteDecision[] {
    return this.routeHistory;
  }

  /** 路由决策 — 根据消息选择最佳模型 */
  route(messages: Message[], needsTools: boolean = false): RouteDecision {
    const complexity = this.evaluator.evaluate(messages);
    const tokenEstimate = estimateTokens(messages);

    // 筛选满足条件的模型
    const candidates: Array<{ provider: Provider; config: ModelRouteConfig }> = [];
    for (const { provider, config } of this.models.values()) {
      if (needsTools && !config.supportsTools) continue;
      if (tokenEstimate > config.maxContext) continue;
      if (complexity > config.complexityLimit) continue;
      candidates.push({ provider, config });
    }

    // 如果没有满足条件的候选，尝试放宽限制
    if (candidates.length === 0) {
      for (const { provider, config } of this.models.values()) {
        if (needsTools && !config.supportsTools) continue;
        candidates.push({ provider, config });
      }
    }

    // 仍然没有候选 → 使用 fallback 或第一个
    if (candidates.length === 0) {
      const fallbackName = this.fallbackModel ?? this.models.keys().next().value;
      if (!fallbackName) {
        throw new Error('ModelRouter: no models registered');
      }
      const entry = this.models.get(fallbackName)!;
      const decision: RouteDecision = {
        modelName: fallbackName,
        provider: entry.provider,
        complexity,
        estimatedCost: (tokenEstimate / 1000) * entry.config.costPer1K,
        reason: 'fallback (no suitable model found)',
      };
      this.routeHistory.push(decision);
      return decision;
    }

    // 根据策略排序
    let selected: { provider: Provider; config: ModelRouteConfig };
    let reason: string;

    if (this.strategy === 'cost-first') {
      // 最低成本优先
      candidates.sort((a, b) => a.config.costPer1K - b.config.costPer1K);
      selected = candidates[0]!;
      reason = `cost-first: selected cheapest model ($${selected.config.costPer1K}/1K)`;
    } else if (this.strategy === 'quality-first') {
      // 最高复杂度上限优先（通常对应最强模型）
      candidates.sort((a, b) => b.config.complexityLimit - a.config.complexityLimit);
      selected = candidates[0]!;
      reason = `quality-first: selected most capable model (limit=${selected.config.complexityLimit})`;
    } else {
      // balanced: 选择能覆盖复杂度的最低成本模型
      candidates.sort((a, b) => {
        // 优先选择刚好能覆盖复杂度的
        const aFit = a.config.complexityLimit - complexity;
        const bFit = b.config.complexityLimit - complexity;
        if (aFit >= 0 && bFit >= 0) {
          // 都能覆盖 → 选更便宜的
          return a.config.costPer1K - b.config.costPer1K;
        }
        // 选更接近的
        return bFit - aFit;
      });
      selected = candidates[0]!;
      reason = `balanced: complexity=${complexity.toFixed(2)}, selected ${selected.config.name} (limit=${selected.config.complexityLimit}, cost=$${selected.config.costPer1K}/1K)`;
    }

    const estimatedCost = (tokenEstimate / 1000) * selected.config.costPer1K;
    const decision: RouteDecision = {
      modelName: selected.config.name,
      provider: selected.provider,
      complexity,
      estimatedCost,
      reason,
    };

    this.routeHistory.push(decision);
    // 保留最近 100 条路由记录
    if (this.routeHistory.length > 100) {
      this.routeHistory.shift();
    }

    return decision;
  }

  // ===== Provider 接口实现 =====

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const decision = this.route(req.messages, false);
    try {
      return await decision.provider.complete(req);
    } catch (err) {
      // 降级到 fallback
      if (this.fallbackModel && this.fallbackModel !== decision.modelName) {
        const fallback = this.models.get(this.fallbackModel);
        if (fallback) {
          return await fallback.provider.complete(req);
        }
      }
      throw err;
    }
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const decision = this.route(req.messages, false);
    const provider = decision.provider;
    if (!provider.stream) {
      // 降级为非流式
      const resp = await provider.complete(req);
      yield { content: resp.content, done: true, usage: resp.usage };
      return;
    }
    try {
      yield* provider.stream(req);
    } catch (err) {
      if (this.fallbackModel && this.fallbackModel !== decision.modelName) {
        const fallback = this.models.get(this.fallbackModel);
        if (fallback?.provider.stream) {
          yield* fallback.provider.stream(req);
          return;
        }
      }
      throw err;
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const decision = this.route(req.messages, true);
    try {
      return await decision.provider.callTools(req);
    } catch (err) {
      if (this.fallbackModel && this.fallbackModel !== decision.modelName) {
        const fallback = this.models.get(this.fallbackModel);
        if (fallback) {
          return await fallback.provider.callTools(req);
        }
      }
      throw err;
    }
  }

  info(): ModelInfo {
    // 返回默认模型的信息（第一个注册的）
    const first = this.models.values().next().value;
    if (!first) {
      return {
        name: 'router-empty',
        provider: 'router',
        maxContext: 0,
        supportsTools: false,
        supportsStreaming: false,
      };
    }
    return first.provider.info();
  }
}
