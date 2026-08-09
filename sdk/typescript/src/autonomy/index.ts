// Autonomy module — Long-horizon autonomous goal execution
// Mirrors Go internal/agent/autonomy/

// ===== Goal State Machine =====

export enum GoalState {
  Created = 'created',
  Planned = 'planned',
  Executing = 'executing',
  Validated = 'validated',
  Done = 'done',
  Failed = 'failed',
}

export enum Priority {
  Low = 0,
  Normal = 1,
  High = 2,
  Critical = 3,
}

const VALID_TRANSITIONS: Record<GoalState, GoalState[]> = {
  [GoalState.Created]: [GoalState.Planned, GoalState.Failed],
  [GoalState.Planned]: [GoalState.Executing, GoalState.Failed],
  [GoalState.Executing]: [GoalState.Validated, GoalState.Failed],
  [GoalState.Validated]: [GoalState.Done, GoalState.Executing],
  [GoalState.Done]: [],
  [GoalState.Failed]: [GoalState.Planned],
};

export interface StateChangeEvent {
  goalId: string;
  from: GoalState;
  to: GoalState;
  timestamp: Date;
  reason?: string;
}

export function isTerminal(state: GoalState): boolean {
  return state === GoalState.Done || state === GoalState.Failed;
}

export function validateTransition(from: GoalState, to: GoalState): boolean {
  return VALID_TRANSITIONS[from]?.includes(to) ?? false;
}

// ===== Agent Goal =====

export interface GoalConfig {
  acceptanceCriteria?: string[];
  priority?: Priority;
  maxRetries?: number;
  deadline?: Date;
  metadata?: Record<string, string>;
}

export interface AgentGoal {
  id: string;
  description: string;
  acceptanceCriteria: string[];
  priority: Priority;
  state: GoalState;
  maxRetries: number;
  retryCount: number;
  deadline?: Date;
  createdAt: Date;
  updatedAt: Date;
  metadata: Record<string, string>;
}

let goalCounter = 0;

export function createGoal(description: string, cfg: GoalConfig = {}): AgentGoal {
  goalCounter++;
  return {
    id: `goal-${Date.now()}-${goalCounter}`,
    description,
    acceptanceCriteria: cfg.acceptanceCriteria ?? [],
    priority: cfg.priority ?? Priority.Normal,
    state: GoalState.Created,
    maxRetries: cfg.maxRetries ?? 3,
    retryCount: 0,
    deadline: cfg.deadline,
    createdAt: new Date(),
    updatedAt: new Date(),
    metadata: cfg.metadata ?? {},
  };
}

export function transitionGoal(goal: AgentGoal, to: GoalState): void {
  if (!validateTransition(goal.state, to)) {
    throw new Error(`autonomy: 非法状态转换 ${goal.state} → ${to}`);
  }
  goal.state = to;
  goal.updatedAt = new Date();
}

export function canRetry(goal: AgentGoal): boolean {
  return goal.retryCount < goal.maxRetries;
}

// ===== Goal Plan =====

export type StepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';

export interface PlanStep {
  id: string;
  description: string;
  dependsOn?: string[];
  status: StepStatus;
  result?: string;
  error?: string;
}

export interface GoalPlan {
  goalId: string;
  steps: PlanStep[];
  version: number;
  createdAt: Date;
  replanReason?: string;
}

export function createPlan(goalId: string, steps: Omit<PlanStep, 'status'>[]): GoalPlan {
  return {
    goalId,
    steps: steps.map(s => ({ ...s, status: 'pending' as StepStatus })),
    version: 1,
    createdAt: new Date(),
  };
}

export function planProgress(plan: GoalPlan): number {
  if (plan.steps.length === 0) return 0;
  const done = plan.steps.filter(s => s.status === 'completed' || s.status === 'skipped').length;
  return done / plan.steps.length;
}

export function isPlanComplete(plan: GoalPlan): boolean {
  return plan.steps.length > 0 && plan.steps.every(s => s.status === 'completed' || s.status === 'skipped');
}

export function readySteps(plan: GoalPlan): PlanStep[] {
  const completed = new Set(plan.steps.filter(s => s.status === 'completed' || s.status === 'skipped').map(s => s.id));
  return plan.steps.filter(s =>
    s.status === 'pending' && (s.dependsOn ?? []).every(dep => completed.has(dep))
  );
}

// ===== Autonomy Runtime（v4.7-1 TS 自治运行时，对齐 Go AutonomyRuntime） =====

export interface CheckpointStore {
  saveCheckpoint(cp: Checkpoint): Promise<void>;
  loadCheckpoint(goalId: string): Promise<Checkpoint | undefined>;
  listIncomplete(): Promise<Checkpoint[]>;
}

export interface Checkpoint {
  goalId: string;
  goalDescription: string;
  state: GoalState;
  lastCompletedStep: string;
  plan: GoalPlan | null;
  completed: boolean;
}

export interface StepExecutor {
  executeStep(step: PlanStep): Promise<string>;
}

export interface AutonomyRuntimeConfig {
  stepExecutor: StepExecutor;
  checkpointStore?: CheckpointStore;
}

/** 自治运行时：目标生命周期 + 计划执行 + 崩溃恢复（与 Go 语义对齐）。 */
export class AutonomyRuntime {
  private goals = new Map<string, AgentGoal>();
  private plans = new Map<string, GoalPlan>();
  private resume: CheckpointStore | null;

  constructor(private cfg: AutonomyRuntimeConfig) {
    this.resume = cfg.checkpointStore ?? null;
  }

  submitGoal(description: string, cfg: GoalConfig = {}): AgentGoal {
    const goal = createGoal(description, cfg);
    this.goals.set(goal.id, goal);
    return goal;
  }

  setPlan(goalId: string, plan: GoalPlan): void {
    if (!this.goals.has(goalId)) throw new Error(`目标 ${goalId} 不存在`);
    this.plans.set(goalId, plan);
  }

  getGoal(goalId: string): AgentGoal | undefined {
    return this.goals.get(goalId);
  }

  getPlan(goalId: string): GoalPlan | undefined {
    return this.plans.get(goalId);
  }

  /** 执行目标：按就绪步骤循环执行直至完成（跳过已完成步骤，支持断点续跑）。 */
  async executeGoal(goalId: string): Promise<void> {
    const goal = this.goals.get(goalId);
    if (!goal) throw new Error(`目标 ${goalId} 不存在`);
    const plan = this.plans.get(goalId);
    if (!plan) throw new Error(`目标 ${goalId} 无计划`);
    // 与 Go 一致：planned → executing
    if (goal.state === GoalState.Created) transitionGoal(goal, GoalState.Planned);
    transitionGoal(goal, GoalState.Executing);

    let step = readySteps(plan).shift();
    while (step) {
      await this.cfg.stepExecutor.executeStep(step);
      step.status = 'completed';
      step = readySteps(plan).shift();
    }
    if (!isPlanComplete(plan)) {
      transitionGoal(goal, GoalState.Failed);
      throw new Error(`目标 ${goalId} 计划未完成`);
    }
    transitionGoal(goal, GoalState.Validated);
  }

  completeGoal(goalId: string): void {
    const goal = this.goals.get(goalId);
    if (!goal) throw new Error(`目标 ${goalId} 不存在`);
    transitionGoal(goal, GoalState.Done);
  }

  /** 崩溃恢复：扫描未完成 checkpoint → 重建目标 + 恢复计划（与 Go ResumeIncomplete 对齐）。 */
  async resumeIncomplete(): Promise<string[]> {
    if (!this.resume) return [];
    const checkpoints = await this.resume.listIncomplete();
    const resumed: string[] = [];
    for (const cp of checkpoints) {
      if (cp.plan) this.plans.set(cp.goalId, cp.plan);
      if (!this.goals.has(cp.goalId)) {
        const goal = createGoal(cp.goalDescription || '恢复目标');
        (goal as { id: string }).id = cp.goalId;
        transitionGoal(goal, GoalState.Planned);
        this.goals.set(cp.goalId, goal);
      }
      resumed.push(cp.goalId);
    }
    return resumed;
  }
}
