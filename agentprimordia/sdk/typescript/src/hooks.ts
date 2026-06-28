// hooks.ts — 钩子管理器，镜像 Go 的 internal/agent/hooks.go
// 所有注释使用中文

/** HookPoint 钩子点（35个，与Go完全对齐） */
export type HookPoint =
  | "before_run" | "after_run"
  | "before_turn" | "after_turn"
  | "before_llm" | "after_llm"
  | "before_tool" | "after_tool"
  | "on_error" | "on_complete"
  | "before_rag" | "after_rag"
  | "before_pipeline_step" | "after_pipeline_step"
  | "before_handoff" | "after_handoff"
  | "before_parallel_agent" | "after_parallel_agent"
  | "before_dag_node" | "after_dag_node"
  | "on_stream" | "on_stream_start" | "on_stream_end"
  | "before_memory_read" | "after_memory_read"
  | "before_memory_write" | "after_memory_write"
  | "context_window_update" | "context_window_full"
  | "before_tool_parse" | "after_tool_parse"
  | "on_metrics_collect"
  | "before_shutdown" | "after_shutdown"
  | "on_state_change";

/** HookPhase 钩子执行阶段，决定执行顺序 */
export enum HookPhase {
  /** 护栏阶段：Guardrails 固定在此 */
  Validation = 0,
  /** 预处理：日志、指标 */
  PreProcessing = 1,
  /** 执行：业务逻辑 */
  Execution = 2,
  /** 后处理：通知、缓存 */
  PostProcessing = 3,
}

/** 阶段执行顺序（与Go的phaseOrder一致） */
const PHASE_ORDER: HookPhase[] = [
  HookPhase.Validation,
  HookPhase.PreProcessing,
  HookPhase.Execution,
  HookPhase.PostProcessing,
];

/** HookContext 钩子上下文 */
export interface HookContext {
  agentId?: string;
  requestId?: string;
  sessionId?: string;
  point: HookPoint;
  turn?: number;
  message?: unknown;
  response?: unknown;
  toolCall?: unknown;
  toolResult?: unknown;
  error?: Error;
  metadata?: Record<string, unknown>;
  streamChunk?: unknown;
  duration?: number;
  oldState?: string;
  newState?: string;
  reason?: string;
  memoryQuery?: string;
  memoryResult?: unknown;
  contextWindowUsage?: number;
  contextWindowLimit?: number;
}

/** HookCondition 钩子条件，满足条件才执行 */
export type HookCondition = (ctx: HookContext) => boolean;

/** HookFunc 钩子函数 */
export type HookFunc = (ctx: HookContext) => Promise<void> | void;

/** Hook 钩子定义 */
export interface Hook {
  point: HookPoint;
  func: HookFunc;
  priority: number;
  condition: HookCondition | null;
  id: string;
  phase: HookPhase;
}

/** HookMiddleware 中间件，可在 Fire 前后添加横切逻辑 */
export interface HookMiddleware {
  before(ctx: HookContext): Promise<void> | void;
  after(ctx: HookContext, err: Error | null): Promise<void> | void;
}

/** HookStats 钩子执行统计 */
export interface HookStats {
  totalFired: number;
  totalErrors: number;
  byPoint: Record<string, number>;
  byErrors: Record<string, number>;
}

/**
 * HookManager 钩子管理器
 *
 * 管理钩子的注册、触发和统计。Fire 方法按阶段顺序执行钩子，
 * 支持条件过滤和中间件横切逻辑。
 */
export class HookManager {
  /** 按钩子点分组的钩子列表 */
  private hooks: Map<HookPoint, Hook[]> = new Map();
  /** 中间件列表 */
  private middleware: HookMiddleware[] = [];
  /** 统计数据 */
  private stats: HookStats = {
    totalFired: 0,
    totalErrors: 0,
    byPoint: {},
    byErrors: {},
  };

  constructor() {}

  // ===== 注册方法 =====

  /** Register 注册钩子（默认优先级0，Execution阶段） */
  register(point: HookPoint, fn: HookFunc): void {
    this.registerWithPriority(point, fn, 0);
  }

  /** RegisterWithPriority 注册带优先级的钩子 */
  registerWithPriority(point: HookPoint, fn: HookFunc, priority: number): void {
    this.registerConditional(point, fn, priority, null, "");
  }

  /** RegisterConditional 注册条件钩子（默认Execution阶段） */
  registerConditional(
    point: HookPoint,
    fn: HookFunc,
    priority: number,
    condition: HookCondition | null,
    id: string,
  ): void {
    this.registerConditionalInPhase(HookPhase.Execution, point, fn, priority, condition, id);
  }

  /** RegisterInPhase 在指定阶段注册钩子 */
  registerInPhase(phase: HookPhase, point: HookPoint, fn: HookFunc): void {
    this.registerConditionalInPhase(phase, point, fn, 0, null, "");
  }

  /** RegisterConditionalInPhase 在指定阶段注册条件钩子（完整参数版） */
  registerConditionalInPhase(
    phase: HookPhase,
    point: HookPoint,
    fn: HookFunc,
    priority: number,
    condition: HookCondition | null,
    id: string,
  ): void {
    const hook: Hook = {
      point,
      func: fn,
      priority,
      condition,
      id,
      phase,
    };

    let list = this.hooks.get(point);
    if (!list) {
      list = [];
      this.hooks.set(point, list);
    }
    list.push(hook);

    // 按优先级升序插入排序（与Go的插入排序逻辑一致）
    for (let i = list.length - 1; i > 0; i--) {
      if (list[i].priority < list[i - 1].priority) {
        [list[i], list[i - 1]] = [list[i - 1], list[i]];
      } else {
        break;
      }
    }
  }

  // ===== 中间件 =====

  /** Use 添加中间件 */
  use(middleware: HookMiddleware): void {
    this.middleware.push(middleware);
  }

  // ===== 触发 =====

  /**
   * Fire 触发钩子
   *
   * 执行流程：
   * 1. 运行中间件 before 钩子
   * 2. 按阶段顺序执行钩子（Validation → PreProcessing → Execution → PostProcessing）
   * 3. 每个阶段内按优先级升序执行
   * 4. 检查条件后执行钩子
   * 5. 如果钩子抛出异常，停止执行并记录错误
   * 6. 按逆序运行中间件 after 钩子
   * 7. 记录统计
   */
  async fire(ctx: HookContext): Promise<void> {
    // 获取该钩子点的所有钩子（快照拷贝，避免迭代中修改）
    const hooks = this.hooks.get(ctx.point)
      ? [...this.hooks.get(ctx.point)!]
      : [];
    const mids = [...this.middleware];