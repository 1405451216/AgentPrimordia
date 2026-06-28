import type { ReActAgent } from '../agent/react-loop.js';
import type { Response } from '../types.js';

// ===== DAG Workflow =====

export interface DAGNode {
  id: string;
  handler: (input: string, context: DAGContext) => Promise<string>;
  config?: {
    retryCount?: number;
    timeoutMs?: number;
    condition?: (input: string) => boolean;
  };
}

export interface DAGEdge {
  from: string;
  to: string;
  condition?: (result: string) => boolean;
}

export interface DAGContext {
  nodeResults: Record<string, string>;
  metadata: Record<string, unknown>;
}

export interface DAGNodeResult {
  nodeId: string;
  output: string;
  status: 'completed' | 'skipped' | 'failed';
  duration: number;
  error?: string;
}

export interface DAGResult {
  output: string;
  nodeResults: Record<string, DAGNodeResult>;
  totalDuration: number;
  success: boolean;
}

export class DAGBuilder {
  private nodes: Map<string, DAGNode> = new Map();
  private edges: DAGEdge[] = [];
  private name: string;

  constructor(name: string) {
    this.name = name;
  }

  node(id: string, handler: (input: string, context: DAGContext) => Promise<string>): DAGBuilder {
    this.nodes.set(id, { id, handler });
    return this;
  }

  nodeWithConfig(id: string, handler: (input: string, context: DAGContext) => Promise<string>, config: DAGNode['config']): DAGBuilder {
    this.nodes.set(id, { id, handler, config });
    return this;
  }

  edge(from: string, to: string, condition?: (result: string) => boolean): DAGBuilder {
    this.edges.push({ from, to, condition });
    return this;
  }

  build(): DAGWorkflow {
    return new DAGWorkflow(this.name, Array.from(this.nodes.values()), this.edges);
  }
}

export class DAGWorkflow {
  private nodes: Map<string, DAGNode>;
  private edges: DAGEdge[];
  private name: string;

  constructor(name: string, nodes: DAGNode[], edges: DAGEdge[]) {
    this.name = name;
    this.nodes = new Map(nodes.map((n) => [n.id, n]));
    this.edges = edges;
  }

  async run(input: string): Promise<DAGResult> {
    const startTime = Date.now();
    const context: DAGContext = { nodeResults: {}, metadata: {} };
    const results: Record<string, DAGNodeResult> = {};

    // Topological sort
    const order = this.topologicalSort();
    if (order.length === 0) {
      return { output: '', nodeResults: results, totalDuration: 0, success: false };
    }

    // Find entry nodes (no incoming edges)
    const entryNodes = order.filter((id) => !this.edges.some((e) => e.to === id));

    // Execute nodes
    let currentInput = input;
    const completed = new Set<string>();

    for (const nodeId of order) {
      const node = this.nodes.get(nodeId);
      if (!node) continue;

      // Check condition
      if (node.config?.condition && !node.config.condition(currentInput)) {
        results[nodeId] = { nodeId, output: '', status: 'skipped', duration: 0 };
        completed.add(nodeId);
        continue;
      }

      const nodeStart = Date.now();
      try {
        let output: string;

        // Execute with retry
        const retries = node.config?.retryCount ?? 0;
        let lastError: Error | null = null;

        for (let attempt = 0; attempt <= retries; attempt++) {
          try {
            output = await this.executeWithTimeout(node, currentInput, context, node.config?.timeoutMs);
            lastError = null;
            break;
          } catch (err) {
            lastError = err instanceof Error ? err : new Error(String(err));
            if (attempt < retries) {
              await new Promise((r) => setTimeout(r, 1000 * (attempt + 1)));
            }
          }
        }

        if (lastError) {
          results[nodeId] = { nodeId, output: '', status: 'failed', duration: Date.now() - nodeStart, error: lastError.message };
          // Try to follow fallback edges
          const fallbackEdges = this.edges.filter((e) => e.from === nodeId && e.condition);
          if (fallbackEdges.length === 0) {
            return { output: '', nodeResults: results, totalDuration: Date.now() - startTime, success: false };
          }
        } else {
          output = output!;
          context.nodeResults[nodeId] = output;
          results[nodeId] = { nodeId, output, status: 'completed', duration: Date.now() - nodeStart };
          currentInput = output;
        }
      } catch (err) {
        results[nodeId] = { nodeId, output: '', status: 'failed', duration: Date.now() - nodeStart, error: (err as Error).message };
        return { output: '', nodeResults: results, totalDuration: Date.now() - startTime, success: false };
      }

      completed.add(nodeId);

      // Find next nodes based on edges
      const nextEdges = this.edges.filter((e) => e.from === nodeId);
      for (const edge of nextEdges) {
        if (edge.condition && !edge.condition(currentInput)) {
          continue; // Skip if condition not met
        }
      }
    }

    // Find final output (last completed node)
    const lastCompleted = order.filter((id) => results[id]?.status === 'completed').pop();
    const output = lastCompleted ? results[lastCompleted].output : '';

    return {
      output,
      nodeResults: results,
      totalDuration: Date.now() - startTime,
      success: true,
    };
  }

  private async executeWithTimeout(
    node: DAGNode,
    input: string,
    context: DAGContext,
    timeoutMs?: number
  ): Promise<string> {
    if (!timeoutMs) return node.handler(input, context);

    return Promise.race([
      node.handler(input, context),
      new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error(`Node "${node.id}" timed out after ${timeoutMs}ms`)), timeoutMs)
      ),
    ]);
  }

  private topologicalSort(): string[] {
    const inDegree: Map<string, number> = new Map();
    const adjList: Map<string, string[]> = new Map();

    for (const nodeId of this.nodes.keys()) {
      inDegree.set(nodeId, 0);
      adjList.set(nodeId, []);
    }

    for (const edge of this.edges) {
      if (!adjList.has(edge.from)) adjList.set(edge.from, []);
      adjList.get(edge.from)!.push(edge.to);
      inDegree.set(edge.to, (inDegree.get(edge.to) ?? 0) + 1);
    }

    const queue: string[] = [];
    for (const [id, deg] of inDegree) {
      if (deg === 0) queue.push(id);
    }

    const result: string[] = [];
    while (queue.length > 0) {
      const current = queue.shift()!;
      result.push(current);
      const neighbors = adjList.get(current) ?? [];
      for (const neighbor of neighbors) {
        const newDeg = (inDegree.get(neighbor) ?? 0) - 1;
        inDegree.set(neighbor, newDeg);
        if (newDeg === 0) queue.push(neighbor);
      }
    }

    return result;
  }

  get name_(): string { return this.name; }
}

// ===== GroupChat =====

export interface GroupChatConfig {
  agents: ReActAgent[];
  maxRounds?: number;
  moderator?: ReActAgent;
  topic?: string;
}

export interface GroupChatMessage {
  agentName: string;
  round: number;
  content: string;
}

export interface GroupChatResult {
  messages: GroupChatMessage[];
  summary: string;
  totalRounds: number;
}

export class GroupChat {
  private config: GroupChatConfig;

  constructor(config: GroupChatConfig) {
    this.config = config;
  }

  async run(topic: string): Promise<GroupChatResult> {
    const messages: GroupChatMessage[] = [];
    const maxRounds = this.config.maxRounds ?? 3;
    let currentTopic = topic;

    for (let round = 0; round < maxRounds; round++) {
      for (const agent of this.config.agents) {
        const prompt = round === 0
          ? `Topic: ${currentTopic}\n\nPlease share your thoughts on this topic.`
          : `Previous discussion:\n${messages.map((m) => `${m.agentName}: ${m.content}`).join('\n')}\n\nPlease contribute to the discussion.`;

        const resp = await agent.run(prompt);
        messages.push({
          agentName: agent.name,
          round,
          content: resp.content,
        });
      }

      // Moderator summarizes after each round
      if (this.config.moderator && round < maxRounds - 1) {
        const modPrompt = `Summarize the discussion so far and suggest the next direction:\n\n${messages.map((m) => `${m.agentName}: ${m.content}`).join('\n')}`;
        const modResp = await this.config.moderator.run(modPrompt);
        currentTopic = modResp.content;
      }
    }

    // Final summary
    let summary = '';
    if (this.config.moderator) {
      const summaryPrompt = `Provide a final summary of the group discussion:\n\n${messages.map((m) => `${m.agentName}: ${m.content}`).join('\n')}`;
      const resp = await this.config.moderator.run(summaryPrompt);
      summary = resp.content;
    } else {
      summary = messages.map((m) => `${m.agentName}: ${m.content}`).join('\n');
    }

    return { messages, summary, totalRounds: maxRounds };
  }
}

// ===== Debate =====

export interface DebateConfig {
  topic: string;
  proponent: ReActAgent;
  opponent: ReActAgent;
  judge?: ReActAgent;
  rounds?: number;
}

export interface DebateResult {
  proponentArguments: string[];
  opponentArguments: string[];
  judgeVerdict?: string;
  winner?: 'proponent' | 'opponent' | 'draw';
}

export class Debate {
  private config: DebateConfig;

  constructor(config: DebateConfig) {
    this.config = config;
  }

  async run(): Promise<DebateResult> {
    const rounds = this.config.rounds ?? 3;
    const proponentArgs: string[] = [];
    const opponentArgs: string[] = [];

    for (let round = 0; round < rounds; round++) {
      // Proponent argues
      const proPrompt = round === 0
        ? `Debate topic: ${this.config.topic}\n\nPresent your opening argument in favor.`
        : `Your opponent argues: "${opponentArgs[opponentArgs.length - 1]}"\n\nRebut and present your next argument.`;
      const proResp = await this.config.proponent.run(proPrompt);
      proponentArgs.push(proResp.content);

      // Opponent argues
      const oppPrompt = round === 0
        ? `Debate topic: ${this.config.topic}\n\nPresent your opening argument against.`
        : `Your opponent argues: "${proponentArgs[proponentArgs.length - 1]}"\n\nRebut and present your next argument.`;
      const oppResp = await this.config.opponent.run(oppPrompt);
      opponentArgs.push(oppResp.content);
    }

    // Judge verdict
    let judgeVerdict: string | undefined;
    let winner: 'proponent' | 'opponent' | 'draw' | undefined;

    if (this.config.judge) {
      const judgePrompt = `You are judging a debate on: "${this.config.topic}".\n\nProponent arguments:\n${proponentArgs.map((a, i) => `Round ${i + 1}: ${a}`).join('\n')}\n\nOpponent arguments:\n${opponentArgs.map((a, i) => `Round ${i + 1}: ${a}`).join('\n')}\n\nProvide your verdict and declare a winner.`;
      const resp = await this.config.judge.run(judgePrompt);
      judgeVerdict = resp.content;

      // Try to determine winner
      if (resp.content.toLowerCase().includes('proponent wins')) winner = 'proponent';
      else if (resp.content.toLowerCase().includes('opponent wins')) winner = 'opponent';
      else winner = 'draw';
    }

    return { proponentArguments: proponentArgs, opponentArguments: opponentArgs, judgeVerdict, winner };
  }
}

// ===== Supervisor =====

export interface SupervisorConfig {
  supervisor: ReActAgent;
  workers: Map<string, ReActAgent>;
  maxIterations?: number;
}

export interface SupervisorResult {
  output: string;
  workerResults: Record<string, string>;
  iterations: number;
}

export class Supervisor {
  private config: SupervisorConfig;

  constructor(config: SupervisorConfig) {
    this.config = config;
  }

  async run(task: string): Promise<SupervisorResult> {
    const maxIter = this.config.maxIterations ?? 10;
    const workerResults: Record<string, string> = {};
    let currentTask = task;
    let output = '';

    for (let i = 0; i < maxIter; i++) {
      // Supervisor decides which worker to use next
      const workerList = Array.from(this.config.workers.keys());
      const supervisorPrompt = `Task: ${currentTask}\n\nAvailable workers: ${workerList.join(', ')}\n\nWhich worker should handle the next subtask? Reply with just the worker name, or "DONE" if the task is complete.`;

      const resp = await this.config.supervisor.run(supervisorPrompt);
      const decision = resp.content.trim().toLowerCase();

      if (decision === 'done' || decision.includes('done')) {
        output = resp.content;
        break;
      }

      // Find matching worker
      const workerName = workerList.find((name) => decision.includes(name.toLowerCase()));
      if (!workerName) {
        output = `Supervisor could not determine next worker. Last response: ${resp.content}`;
        break;
      }

      const worker = this.config.workers.get(workerName)!;
      const workerResp = await worker.run(currentTask);
      workerResults[workerName] = workerResp.content;
      currentTask = workerResp.content;

      if (i === maxIter - 1) {
        const finalResp = await this.config.supervisor.run(`Summarize the results:\n${JSON.stringify(workerResults, null, 2)}`);
        output = finalResp.content;
      }
    }

    return { output, workerResults, iterations: maxIter };
  }
}

// ===== Workflow Engine =====

export type WorkflowType = 'linear' | 'conditional' | 'loop' | 'parallel_fork_join' | 'state_machine';
export type WorkflowStatus = 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';

export interface WorkflowNode {
  id: string;
  type: 'task' | 'condition' | 'parallel' | 'loop_start' | 'loop_end' | 'fallback' | 'sub_workflow';
  handler?: (input: string, context: Record<string, unknown>) => Promise<string>;
  config?: Record<string, unknown>;
}

export interface WorkflowConfig {
  type: WorkflowType;
  nodes: WorkflowNode[];
  transitions: { from: string; to: string; condition?: (result: string) => boolean }[];
  maxRetries?: number;
  retryBackoffMs?: number;
  loopCondition?: (input: string, iteration: number) => boolean;
  loopMaxIterations?: number;
}

export interface WorkflowResult {
  status: WorkflowStatus;
  output: string;
  nodeResults: Record<string, { output: string; status: string; duration: number }>;
  totalDuration: number;
  error?: Error;
}

export class WorkflowExecution {
  private config: WorkflowConfig;
  private status: WorkflowStatus = 'pending';
  private paused = false;
  private cancelled = false;

  constructor(config: WorkflowConfig) {
    this.config = config;
  }

  async run(input: string): Promise<WorkflowResult> {
    const startTime = Date.now();
    this.status = 'running';
    const nodeResults: Record<string, { output: string; status: string; duration: number }> = {};
    let currentInput = input;

    try {
      switch (this.config.type) {
        case 'linear':
          currentInput = await this.runLinear(currentInput, nodeResults);
          break;
        case 'conditional':
          currentInput = await this.runConditional(currentInput, nodeResults);
          break;
        case 'loop':
          currentInput = await this.runLoop(currentInput, nodeResults);
          break;
        case 'parallel_fork_join':
          currentInput = await this.runParallelForkJoin(currentInput, nodeResults);
          break;
        case 'state_machine':
          currentInput = await this.runStateMachine(currentInput, nodeResults);
          break;
      }

      this.status = 'completed';
      return {
        status: this.status,
        output: currentInput,
        nodeResults,
        totalDuration: Date.now() - startTime,
      };
    } catch (err) {
      this.status = 'failed';
      return {
        status: this.status,
        output: '',
        nodeResults,
        totalDuration: Date.now() - startTime,
        error: err instanceof Error ? err : new Error(String(err)),
      };
    }
  }

  pause(): void { this.paused = true; this.status = 'paused'; }
  resume(): void { this.paused = false; this.status = 'running'; }
  cancel(): void { this.cancelled = true; this.status = 'cancelled'; }

  get status_(): WorkflowStatus { return this.status; }

  private async runLinear(input: string, results: Record<string, { output: string; status: string; duration: number }>): Promise<string> {
    let current = input;
    for (const node of this.config.nodes) {
      if (this.cancelled) throw new Error('Workflow cancelled');
      if (this.paused) await this.waitForResume();
      if (!node.handler) continue;
      const start = Date.now();
      const output = await node.handler(current, {});
      results[node.id] = { output, status: 'completed', duration: Date.now() - start };
      current = output;
    }
    return current;
  }

  private async runConditional(input: string, results: Record<string, { output: string; status: string; duration: number }>): Promise<string> {
    let current = input;
    let currentNodeId = this.config.nodes[0]?.id;

    while (currentNodeId) {
      if (this.cancelled) throw new Error('Workflow cancelled');
      const node = this.config.nodes.find((n) => n.id === currentNodeId);
      if (!node || !node.handler) break;

      const start = Date.now();
      const output = await node.handler(current, {});
      results[node.id] = { output, status: 'completed', duration: Date.now() - start };
      current = output;

      // Find next node based on transitions
      const transitions = this.config.transitions.filter((t) => t.from === currentNodeId);
      let nextNodeId: string | undefined;
      for (const t of transitions) {
        if (!t.condition || t.condition(output)) {
          nextNodeId = t.to;
          break;
        }
      }
      currentNodeId = nextNodeId ?? '';
      if (!currentNodeId) break;
    }

    return current;
  }

  private async runLoop(input: string, results: Record<string, { output: string; status: string; duration: number }>): Promise<string> {
    let current = input;
    const maxIter = this.config.loopMaxIterations ?? 10;
    const loopNodes = this.config.nodes.filter((n) => n.type !== 'loop_start' && n.type !== 'loop_end');

    for (let i = 0; i < maxIter; i++) {
      if (this.cancelled) throw new Error('Workflow cancelled');
      if (this.config.loopCondition && !this.config.loopCondition(current, i)) break;

      for (const node of loopNodes) {
        if (!node.handler) continue;
        const start = Date.now();
        const output = await node.handler(current, { iteration: i });
        results[`${node.id}_iter_${i}`] = { output, status: 'completed', duration: Date.now() - start };
        current = output;
      }
    }

    return current;
  }

  private async runParallelForkJoin(input: string, results: Record<string, { output: string; status: string; duration: number }>): Promise<string> {
    const parallelNodes = this.config.nodes.filter((n) => n.type === 'parallel' || n.type === 'task');
    const promises = parallelNodes.map(async (node) => {
      if (!node.handler) return { id: node.id, output: '', duration: 0 };
      const start = Date.now();
      const output = await node.handler(input, {});
      return { id: node.id, output, duration: Date.now() - start };
    });

    const outputs = await Promise.all(promises);
    for (const { id, output, duration } of outputs) {
      results[id] = { output, status: 'completed', duration };
    }

    // Join: concatenate all outputs
    return outputs.map((o) => o.output).join('\n\n');
  }

  private async runStateMachine(input: string, results: Record<string, { output: string; status: string; duration: number }>): Promise<string> {
    let current = input;
    let currentNodeId = this.config.nodes[0]?.id;
    const visited = new Set<string>();

    while (currentNodeId && !visited.has(currentNodeId)) {
      if (this.cancelled) throw new Error('Workflow cancelled');
      visited.add(currentNodeId);

      const node = this.config.nodes.find((n) => n.id === currentNodeId);
      if (!node || !node.handler) break;

      const start = Date.now();
      const output = await node.handler(current, {});
      results[node.id] = { output, status: 'completed', duration: Date.now() - start };
      current = output;

      const transitions = this.config.transitions.filter((t) => t.from === currentNodeId);
      let nextNodeId: string | undefined;
      for (const t of transitions) {
        if (!t.condition || t.condition(output)) {
          nextNodeId = t.to;
          break;
        }
      }
      currentNodeId = nextNodeId ?? '';
      if (!currentNodeId) break;
    }

    return current;
  }

  private waitForResume(): Promise<void> {
    return new Promise((resolve) => {
      const check = () => {
        if (!this.paused || this.cancelled) resolve();
        else setTimeout(check, 100);
      };
      check();
    });
  }
}

// ===== Plan Builder =====

export interface PlanStep {
  id: string;
  description: string;
  agent?: string;
  dependencies: string[];
  status: 'pending' | 'in_progress' | 'completed' | 'failed';
  result?: string;
}

export interface Plan {
  id: string;
  goal: string;
  steps: PlanStep[];
  createdAt: Date;
}

export class PlanBuilder {
  private steps: PlanStep[] = [];
  private goal: string;
  private id: string;

  constructor(goal: string) {
    this.goal = goal;
    this.id = `plan-${Date.now()}`;
  }

  step(id: string, description: string, opts?: { agent?: string; dependsOn?: string[] }): PlanBuilder {
    this.steps.push({
      id,
      description,
      agent: opts?.agent,
      dependencies: opts?.dependsOn ?? [],
      status: 'pending',
    });
    return this;
  }

  build(): Plan {
    // Validate dependencies
    const stepIds = new Set(this.steps.map((s) => s.id));
    for (const step of this.steps) {
      for (const dep of step.dependencies) {
        if (!stepIds.has(dep)) {
          throw new Error(`Step "${step.id}" depends on unknown step "${dep}"`);
        }
      }
    }

    // Check for cycles
    this.detectCycles();

    return {
      id: this.id,
      goal: this.goal,
      steps: [...this.steps],
      createdAt: new Date(),
    };
  }

  private detectCycles(): void {
    const visited = new Set<string>();
    const stack = new Set<string>();

    const visit = (stepId: string): void => {
      if (stack.has(stepId)) throw new Error(`Cycle detected at step "${stepId}"`);
      if (visited.has(stepId)) return;

      visited.add(stepId);
      stack.add(stepId);

      const step = this.steps.find((s) => s.id === stepId);
      if (step) {
        for (const dep of step.dependencies) {
          visit(dep);
        }
      }

      stack.delete(stepId);
    };

    for (const step of this.steps) {
      visit(step.id);
    }
  }
}
