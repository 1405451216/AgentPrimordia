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
