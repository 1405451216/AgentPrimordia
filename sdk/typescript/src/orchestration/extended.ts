import type { ReActAgent } from '../agent/react-loop.js';

// ===== Dynamic Orchestration =====

export type DynamicRouter = (input: string, context: Record<string, unknown>) => Promise<string>;

export interface DynamicRoute {
  name: string;
  match: (input: string) => boolean;
  agentName: string;
  priority: number;
}

export class DynamicOrchestrator {
  private routes: DynamicRoute[] = [];
  private agents: Map<string, ReActAgent> = new Map();
  private defaultAgent: string | null = null;

  registerAgent(name: string, agent: ReActAgent): void {
    this.agents.set(name, agent);
  }

  addRoute(route: DynamicRoute): void {
    this.routes.push(route);
    // Sort by priority (higher first)
    this.routes.sort((a, b) => b.priority - a.priority);
  }

  setDefault(agentName: string): void {
    this.defaultAgent = agentName;
  }

  async route(input: string): Promise<string> {
    // Find matching route
    for (const route of this.routes) {
      if (route.match(input)) {
        const agent = this.agents.get(route.agentName);
        if (agent) {
          const resp = await agent.run(input);
          return resp.content;
        }
      }
    }

    // Fall back to default agent
    if (this.defaultAgent) {
      const agent = this.agents.get(this.defaultAgent);
      if (agent) {
        const resp = await agent.run(input);
        return resp.content;
      }
    }

    throw new Error('No matching route or default agent found');
  }

  getRoutes(): DynamicRoute[] {
    return [...this.routes];
  }

  getAgentNames(): string[] {
    return Array.from(this.agents.keys());
  }
}

// ===== Scheduler =====

export type TaskPriority = 'low' | 'normal' | 'high' | 'critical';

export interface ScheduledTask {
  id: string;
  name: string;
  priority: TaskPriority;
  fn: () => Promise<unknown>;
  scheduledAt: number;
  startedAt?: number;
  completedAt?: number;
  status: 'pending' | 'running' | 'completed' | 'failed';
  result?: unknown;
  error?: Error;
}

export class Scheduler {
  private tasks: ScheduledTask[] = [];
  private running: boolean = false;
  private maxConcurrent: number;
  private activeCount: number = 0;

  constructor(maxConcurrent: number = 5) {
    this.maxConcurrent = maxConcurrent;
  }

  submit(name: string, fn: () => Promise<unknown>, priority: TaskPriority = 'normal'): string {
    const task: ScheduledTask = {
      id: `task-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      name,
      priority,
      fn,
      scheduledAt: Date.now(),
      status: 'pending',
    };
    this.tasks.push(task);
    return task.id;
  }

  async start(): Promise<void> {
    this.running = true;
    this.processQueue();
  }

  stop(): void {
    this.running = false;
  }

  async waitAll(): Promise<void> {
    while (this.tasks.some(t => t.status === 'pending' || t.status === 'running')) {
      await new Promise(r => setTimeout(r, 100));
    }
  }

  getTask(id: string): ScheduledTask | undefined {
    return this.tasks.find(t => t.id === id);
  }

  getPendingCount(): number {
    return this.tasks.filter(t => t.status === 'pending').length;
  }

  getStats(): { total: number; pending: number; running: number; completed: number; failed: number } {
    return {
      total: this.tasks.length,
      pending: this.tasks.filter(t => t.status === 'pending').length,
      running: this.tasks.filter(t => t.status === 'running').length,
      completed: this.tasks.filter(t => t.status === 'completed').length,
      failed: this.tasks.filter(t => t.status === 'failed').length,
    };
  }

  private async processQueue(): Promise<void> {
    while (this.running) {
      if (this.activeCount >= this.maxConcurrent) {
        await new Promise(r => setTimeout(r, 50));
        continue;
      }

      // Get next task by priority
      const pending = this.tasks.filter(t => t.status === 'pending');
      if (pending.length === 0) {
        if (this.activeCount === 0) break;
        await new Promise(r => setTimeout(r, 50));
        continue;
      }

      const priorityOrder = { critical: 0, high: 1, normal: 2, low: 3 };
      pending.sort((a, b) => priorityOrder[a.priority] - priorityOrder[b.priority]);
      const task = pending[0]!;

      task.status = 'running';
      task.startedAt = Date.now();
      this.activeCount++;

      this.executeTask(task);
    }
  }

  private async executeTask(task: ScheduledTask): Promise<void> {
    try {
      task.result = await task.fn();
      task.status = 'completed';
    } catch (err) {
      task.error = err instanceof Error ? err : new Error(String(err));
      task.status = 'failed';
    } finally {
      task.completedAt = Date.now();
      this.activeCount--;
    }
  }
}

// ===== Step Executor =====

export type StepType = 'llm' | 'tool' | 'parallel' | 'sequential' | 'conditional' | 'loop' | 'wait';

export interface WorkflowStep {
  id: string;
  name: string;
  type: StepType;
  config: StepConfig;
  next?: string[];        // Next step IDs
  onError?: string;       // Step to execute on error
  retryCount?: number;
  timeoutMs?: number;
}

export interface StepConfig {
  // LLM step
  prompt?: string;
  model?: string;
  // Tool step
  toolName?: string;
  toolArgs?: Record<string, unknown>;
  // Parallel step
  parallelSteps?: string[];
  // Conditional step
  condition?: (result: string) => boolean;
  trueStep?: string;
  falseStep?: string;
  // Loop step
  loopSteps?: string[];
  loopCondition?: (result: string) => boolean;
  maxIterations?: number;
  // Wait step
  waitMs?: number;
}

export interface StepResult {
  stepId: string;
  output: string;
  status: 'completed' | 'skipped' | 'failed';
  duration: number;
  error?: string;
}

export class StepExecutor {
  private steps: Map<string, WorkflowStep> = new Map();
  private startStep: string | null = null;

  addStep(step: WorkflowStep): this {
    this.steps.set(step.id, step);
    if (!this.startStep) this.startStep = step.id;
    return this;
  }

  setStart(stepId: string): this {
    this.startStep = stepId;
    return this;
  }

  async execute(
    input: string,
    context?: { agent?: ReActAgent; getTool?: (name: string) => { execute: (args: Record<string, unknown>) => Promise<string> } }
  ): Promise<{ results: StepResult[]; finalOutput: string }> {
    if (!this.startStep) throw new Error('No start step defined');

    const results: StepResult[] = [];
    let currentId = this.startStep;
    let currentInput = input;
    let finalOutput = input;

    const executed = new Set<string>();

    while (currentId && !executed.has(currentId)) {
      executed.add(currentId);
      const step = this.steps.get(currentId);
      if (!step) break;

      const startTime = Date.now();
      const result = await this.executeStep(step, currentInput, context);
      result.duration = Date.now() - startTime;
      results.push(result);

      if (result.status === 'failed' && step.onError) {
        currentId = step.onError;
        continue;
      }

      if (result.status === 'completed') {
        currentInput = result.output;
        finalOutput = result.output;
      }

      // Determine next step
      if (step.type === 'conditional' && step.config.condition) {
        const conditionMet = step.config.condition(currentInput);
        currentId = conditionMet ? (step.config.trueStep ?? '') : (step.config.falseStep ?? '');
      } else if (step.type === 'loop') {
        const maxIter = step.config.maxIterations ?? 10;
        let iter = 0;
        while (iter < maxIter && step.config.loopCondition?.(currentInput)) {
          for (const loopStepId of step.config.loopSteps ?? []) {
            const loopStep = this.steps.get(loopStepId);
            if (loopStep) {
              const loopStart = Date.now();
              const loopResult = await this.executeStep(loopStep, currentInput, context);
              loopResult.duration = Date.now() - loopStart;
              results.push(loopResult);
              if (loopResult.status === 'completed') currentInput = loopResult.output;
            }
          }
          iter++;
        }
        currentId = step.next?.[0] ?? '';
      } else {
        currentId = step.next?.[0] ?? '';
      }
    }

    return { results, finalOutput };
  }

  private async executeStep(
    step: WorkflowStep,
    input: string,
    context?: { agent?: ReActAgent; getTool?: (name: string) => { execute: (args: Record<string, unknown>) => Promise<string> } }
  ): Promise<StepResult> {
    try {
      let output = '';

      switch (step.type) {
        case 'llm':
          if (context?.agent) {
            const resp = await context.agent.run(step.config.prompt ?? input);
            output = resp.content;
          } else {
            output = input;
          }
          break;

        case 'tool':
          if (context?.getTool && step.config.toolName) {
            const tool = context.getTool(step.config.toolName);
            output = await tool.execute(step.config.toolArgs ?? {});
          }
          break;

        case 'parallel': {
          // Execute parallel steps concurrently
          const parallelResults = await Promise.all(
            (step.config.parallelSteps ?? []).map(async stepId => {
              const s = this.steps.get(stepId);
              if (!s) return '';
              const r = await this.executeStep(s, input, context);
              return r.output;
            })
          );
          output = parallelResults.join('\n');
          break;
        }

        case 'sequential':
          output = input;
          break;

        case 'wait':
          await new Promise(r => setTimeout(r, step.config.waitMs ?? 1000));
          output = input;
          break;

        default:
          output = input;
      }

      return { stepId: step.id, output, status: 'completed', duration: 0 };
    } catch (err) {
      return {
        stepId: step.id,
        output: '',
        status: 'failed',
        duration: 0,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }
}

// ===== Worker Pool =====

export interface WorkerPoolConfig {
  minWorkers: number;
  maxWorkers: number;
  queueSize: number;
  idleTimeoutMs: number;
}

export interface PoolTask {
  id: string;
  fn: () => Promise<unknown>;
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
}

export class WorkerPool {
  private config: WorkerPoolConfig;
  private workers: number = 0;
  private queue: PoolTask[] = [];
  private activeTasks: number = 0;
  private running: boolean = true;

  constructor(config?: Partial<WorkerPoolConfig>) {
    this.config = {
      minWorkers: config?.minWorkers ?? 1,
      maxWorkers: config?.maxWorkers ?? 10,
      queueSize: config?.queueSize ?? 1000,
      idleTimeoutMs: config?.idleTimeoutMs ?? 60000,
    };
  }

  async submit<T>(fn: () => Promise<T>): Promise<T> {
    if (this.queue.length >= this.config.queueSize) {
      throw new Error('Worker pool queue is full');
    }

    return new Promise<T>((resolve, reject) => {
      this.queue.push({
        id: `pool-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        fn,
        resolve: resolve as (value: unknown) => void,
        reject,
      });
      this.tryDispatch();
    });
  }

  private tryDispatch(): void {
    while (this.running && this.queue.length > 0 && this.activeTasks < this.config.maxWorkers) {
      const task = this.queue.shift();
      if (!task) break;

      this.activeTasks++;
      if (this.activeTasks > this.workers) {
        this.workers = this.activeTasks;
      }

      this.executeTask(task);
    }
  }

  private async executeTask(task: PoolTask): Promise<void> {
    try {
      const result = await task.fn();
      task.resolve(result);
    } catch (err) {
      task.reject(err instanceof Error ? err : new Error(String(err)));
    } finally {
      this.activeTasks--;
      // Try to dispatch next task
      if (this.queue.length > 0) {
        this.tryDispatch();
      }
    }
  }

  async drain(): Promise<void> {
    this.running = false;
    while (this.activeTasks > 0 || this.queue.length > 0) {
      await new Promise(r => setTimeout(r, 100));
    }
  }

  getStats(): { workers: number; activeTasks: number; queueSize: number } {
    return {
      workers: this.workers,
      activeTasks: this.activeTasks,
      queueSize: this.queue.length,
    };
  }

  getQueueLength(): number { return this.queue.length; }
  getActiveWorkers(): number { return this.activeTasks; }
}

// ===== Orchestration Visualizer =====

export interface OrchNode {
  id: string;
  label: string;
  type: 'agent' | 'tool' | 'decision' | 'parallel' | 'start' | 'end';
}

export interface OrchEdge {
  from: string;
  to: string;
  label?: string;
}

export class OrchestrationVisualizer {
  private nodes: Map<string, OrchNode> = new Map();
  private edges: OrchEdge[] = [];

  addNode(node: OrchNode): this {
    this.nodes.set(node.id, node);
    return this;
  }

  addEdge(edge: OrchEdge): this {
    this.edges.push(edge);
    return this;
  }

  toMermaid(): string {
    const lines: string[] = ['graph TD'];

    for (const [id, node] of this.nodes) {
      const shape = this.getShape(node.type);
      lines.push(`  ${id}${shape.start}${node.label}${shape.end}`);
    }

    for (const edge of this.edges) {
      const label = edge.label ? `|${edge.label}|` : '';
      lines.push(`  ${edge.from} -->${label} ${edge.to}`);
    }

    return lines.join('\n');
  }

  toJSON(): string {
    return JSON.stringify({
      nodes: Array.from(this.nodes.entries()).map(([id, node]) => ({ id, label: node.label, type: node.type })),
      edges: this.edges,
    }, null, 2);
  }

  private getShape(type: string): { start: string; end: string } {
    switch (type) {
      case 'start': return { start: '([', end: '])' };
      case 'end': return { start: '[[', end: ']]' };
      case 'decision': return { start: '{', end: '}' };
      case 'parallel': return { start: '[/', end: '/]' };
      default: return { start: '[', end: ']' };
    }
  }
}

// ===== Collaboration Patterns =====

export interface CollaborationMessage {
  from: string;
  to: string;
  content: string;
  timestamp: string;
}

export class CollaborationHub {
  private participants: Map<string, { name: string; send: (msg: CollaborationMessage) => Promise<void> }> = new Map();
  private messages: CollaborationMessage[] = [];
  private messageHandlers: Map<string, ((msg: CollaborationMessage) => void)[]> = new Map();

  register(name: string, send: (msg: CollaborationMessage) => Promise<void>): void {
    this.participants.set(name, { name, send });
  }

  unregister(name: string): void {
    this.participants.delete(name);
    this.messageHandlers.delete(name);
  }

  async send(from: string, to: string, content: string): Promise<void> {
    const msg: CollaborationMessage = {
      from, to, content,
      timestamp: new Date().toISOString(),
    };
    this.messages.push(msg);

    if (to === 'broadcast') {
      // Send to all participants
      for (const [name, participant] of this.participants) {
        if (name !== from) {
          await participant.send(msg);
        }
      }
    } else {
      const participant = this.participants.get(to);
      if (participant) await participant.send(msg);
    }

    // Notify handlers
    const handlers = this.messageHandlers.get(to);
    if (handlers) for (const handler of handlers) handler(msg);
  }

  onMessage(name: string, handler: (msg: CollaborationMessage) => void): () => void {
    if (!this.messageHandlers.has(name)) this.messageHandlers.set(name, []);
    this.messageHandlers.get(name)!.push(handler);
    return () => {
      const handlers = this.messageHandlers.get(name);
      if (handlers) {
        const idx = handlers.indexOf(handler);
        if (idx >= 0) handlers.splice(idx, 1);
      }
    };
  }

  getHistory(participant?: string): CollaborationMessage[] {
    if (!participant) return [...this.messages];
    return this.messages.filter(m => m.from === participant || m.to === participant || m.to === 'broadcast');
  }

  getParticipants(): string[] {
    return Array.from(this.participants.keys());
  }
}
