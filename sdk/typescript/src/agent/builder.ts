/**
 * 类型安全的 Agent Builder DSL — 编译期链式调用验证。
 *
 * 利用 TypeScript 条件类型实现编译期能力检查：
 * - 未注入 Provider 时调用 build() 会报类型错误
 * - 未注入 Toolkit 时调用 build() 会报类型错误
 * - 每一步 withXxx() 返回增强后的类型，只暴露可用方法
 *
 * 这是 TS SDK 相对 Go SDK 的独有优势 — Go 的链式调用无法在编译期验证。
 *
 * 使用方式：
 *   const agent = await createAgent('my-agent')
 *     .withProvider(provider)
 *     .withToolkit(toolkit)
 *     .withMemory(memory)
 *     .withHooks(hooks)
 *     .build();
 */

import { ReActAgent } from './react-loop.js';
import type { ReActConfig, HookManager } from './react-loop.js';
import type { Provider } from '../llm/provider.js';
import type { ToolRegistry } from '../tools/registry.js';
import type { Memory } from '../memory/store.js';
import type { CostTracker, CheckpointStore } from './request-id.js';
import type { OTelBridge } from '../metrics/otel-extended.js';
import { AgentSelfTuner } from './self-tuning.js';
import type { SpeculativeExecutor } from './speculative-exec.js';
import type { EnhancedToolLearner } from './tool-learning.js';

// ===== Builder 状态类型 =====

/** 标记类型：表示某能力是否已注入 */
type Has<T> = { readonly __has: T };
type Missing = { readonly __missing: true };

/** Builder 接口 — 根据已注入能力动态暴露方法 */
interface AgentBuilder<
  P = Missing, // Provider
  T = Missing, // Toolkit
> {
  // 必填项 — 注入后返回增强类型
  withProvider(provider: Provider): AgentBuilder<Has<Provider>, T>;
  withToolkit(toolkit: ToolRegistry): AgentBuilder<P, Has<ToolRegistry>>;

  // 可选项 — 始终可用
  withName(name: string): AgentBuilder<P, T>;
  withMaxTurns(maxTurns: number): AgentBuilder<P, T>;
  withSystemPrompt(prompt: string): AgentBuilder<P, T>;
  withSessionId(id: string): AgentBuilder<P, T>;
  withMaxMessages(max: number): AgentBuilder<P, T>;
  withHooks(hooks: HookManager): AgentBuilder<P, T>;
  withMemory(memory: Memory): AgentBuilder<P, T>;
  withCostTracker(tracker: CostTracker): AgentBuilder<P, T>;
  withCheckpointStore(store: CheckpointStore): AgentBuilder<P, T>;
  // P3-1: OTel 可观测性
  withOTel(otel: OTelBridge): AgentBuilder<P, T>;
  // P4-A1: 并行工具执行
  withParallelTools(maxParallel?: number): AgentBuilder<P, T>;
  // P6-A1: 投机执行
  withSpeculativeExec(executor: SpeculativeExecutor): AgentBuilder<P, T>;
  // P6-A2: 自调优
  withSelfTuning(tuner?: AgentSelfTuner, autoTune?: boolean): AgentBuilder<P, T>;
  // P6-A3: 增强工具学习
  withEnhancedToolLearner(learner: EnhancedToolLearner): AgentBuilder<P, T>;

  // build() — 仅当 P 和 T 都已注入时才可用
  build: P extends Has<Provider>
    ? T extends Has<ToolRegistry>
      ? () => ReActAgent
      : never
    : never;
}

// ===== Builder 实现 =====

class AgentBuilderImpl<
  P = Missing,
  T = Missing,
> {
  private config: Partial<ReActConfig> = {};

  constructor(name: string) {
    this.config.name = name;
  }

  withProvider(provider: Provider): AgentBuilder<Has<Provider>, T> {
    this.config.model = provider;
    return this as unknown as AgentBuilder<Has<Provider>, T>;
  }

  withToolkit(toolkit: ToolRegistry): AgentBuilder<P, Has<ToolRegistry>> {
    this.config.toolkit = toolkit;
    return this as unknown as AgentBuilder<P, Has<ToolRegistry>>;
  }

  withName(name: string): AgentBuilder<P, T> {
    this.config.name = name;
    return this as unknown as AgentBuilder<P, T>;
  }

  withMaxTurns(maxTurns: number): AgentBuilder<P, T> {
    this.config.maxTurns = maxTurns;
    return this as unknown as AgentBuilder<P, T>;
  }

  withSystemPrompt(prompt: string): AgentBuilder<P, T> {
    this.config.systemPrompt = prompt;
    return this as unknown as AgentBuilder<P, T>;
  }

  withSessionId(id: string): AgentBuilder<P, T> {
    this.config.sessionId = id;
    return this as unknown as AgentBuilder<P, T>;
  }

  withMaxMessages(max: number): AgentBuilder<P, T> {
    this.config.maxMessages = max;
    return this as unknown as AgentBuilder<P, T>;
  }

  withHooks(hooks: HookManager): AgentBuilder<P, T> {
    this.config.hooks = hooks;
    return this as unknown as AgentBuilder<P, T>;
  }

  withMemory(memory: Memory): AgentBuilder<P, T> {
    this.config.memoryStore = memory;
    return this as unknown as AgentBuilder<P, T>;
  }

  withCostTracker(tracker: CostTracker): AgentBuilder<P, T> {
    this.config.costTracker = tracker;
    return this as unknown as AgentBuilder<P, T>;
  }

  withCheckpointStore(store: CheckpointStore): AgentBuilder<P, T> {
    this.config.checkpointStore = store;
    return this as unknown as AgentBuilder<P, T>;
  }

  withOTel(otel: OTelBridge): AgentBuilder<P, T> {
    this.config.otelBridge = otel;
    return this as unknown as AgentBuilder<P, T>;
  }

  withParallelTools(maxParallel: number = 0): AgentBuilder<P, T> {
    this.config.parallelToolExecution = true;
    this.config.maxParallelTools = maxParallel;
    return this as unknown as AgentBuilder<P, T>;
  }

  withSpeculativeExec(executor: SpeculativeExecutor): AgentBuilder<P, T> {
    this.config.speculativeExecutor = executor;
    return this as unknown as AgentBuilder<P, T>;
  }

  withSelfTuning(tuner?: AgentSelfTuner, autoTune: boolean = true): AgentBuilder<P, T> {
    this.config.selfTuner = tuner ?? new AgentSelfTuner();
    this.config.autoTune = autoTune;
    return this as unknown as AgentBuilder<P, T>;
  }

  withEnhancedToolLearner(learner: EnhancedToolLearner): AgentBuilder<P, T> {
    this.config.enhancedToolLearner = learner;
    return this as unknown as AgentBuilder<P, T>;
  }

  build(): ReActAgent {
    return new ReActAgent(this.config as ReActConfig);
  }
}

// ===== 入口函数 =====

/**
 * 创建类型安全的 Agent Builder。
 *
 * 编译期保证：必须在调用 build() 前注入 Provider 和 Toolkit，
 * 否则 build 属性类型为 never，TypeScript 编译报错。
 *
 * @example
 * ```ts
 * const agent = createAgent('my-agent')
 *   .withProvider(provider)    // 注入后 build 仍不可用
 *   .withToolkit(toolkit)      // 注入后 build 变为可用
 *   .withMemory(memory)        // 可选
 *   .withMaxTurns(20)          // 可选
 *   .build();                  // 现在可以调用
 * ```
 */
export function createAgent(name: string): AgentBuilder {
  return new AgentBuilderImpl(name) as unknown as AgentBuilder;
}

// ===== 预设模板 =====

/** 快速创建基础 Agent */
export function createBasicAgent(
  name: string,
  provider: Provider,
  toolkit: ToolRegistry,
  opts?: Partial<ReActConfig>,
): ReActAgent {
  return new ReActAgent({
    name,
    model: provider,
    toolkit,
    maxTurns: opts?.maxTurns ?? 10,
    ...opts,
  });
}

/** 快速创建带记忆的 Agent */
export function createAgentWithMemory(
  name: string,
  provider: Provider,
  toolkit: ToolRegistry,
  memory: Memory,
  opts?: Partial<ReActConfig>,
): ReActAgent {
  return new ReActAgent({
    name,
    model: provider,
    toolkit,
    memoryStore: memory,
    ...opts,
  });
}

/** 快速创建带成本控制的 Agent */
export function createAgentWithBudget(
  name: string,
  provider: Provider,
  toolkit: ToolRegistry,
  costTracker: CostTracker,
  opts?: Partial<ReActConfig>,
): ReActAgent {
  return new ReActAgent({
    name,
    model: provider,
    toolkit,
    costTracker,
    ...opts,
  });
}
