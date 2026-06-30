
// ===== Graceful Shutdown =====

export type AgentStatus = 'idle' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';

export interface AgentStats {
  status: AgentStatus;
  requestID?: string;
  currentTurn: number;
  totalMessages: number;
  toolsCalled: Record<string, number>;
  startTime?: Date;
}

export class LifecycleManager {
  private _status: AgentStatus = 'idle';
  private _statusReason = '';
  private stopped = false;
  private gracefulShutdown = false;
  private stopResolvers: (() => void)[] = [];

  get status(): AgentStatus { return this._status; }
  get statusReason(): string { return this._statusReason; }

  setStatus(s: AgentStatus): void { this._status = s; }
  setStatusWithReason(s: AgentStatus, reason: string): void {
    this._status = s;
    this._statusReason = reason;
  }

  stop(): void {
    this.stopped = true;
    this._status = 'cancelled';
    for (const r of this.stopResolvers) r();
    this.stopResolvers = [];
  }

  isStopped(): boolean { return this.stopped; }

  requestGracefulShutdown(): void {
    this.gracefulShutdown = true;
  }

  isGracefulShutdown(): boolean { return this.gracefulShutdown; }

  onStop(): Promise<void> {
    if (this.stopped) return Promise.resolve();
    return new Promise((r) => this.stopResolvers.push(r));
  }
}

// ===== Tracer ===== (already defined in capability-agent.ts, this re-exports)

export type { Tracer, Span, SpanKind, NoopTracer } from './capability-agent.js';

// ===== Workflow Types =====

export type WorkflowType = 'linear' | 'conditional' | 'loop' | 'parallel_fork_join' | 'state_machine';
export type WorkflowStatus = 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';

export interface WorkflowNode {
  id: string;
  type: 'task' | 'condition' | 'parallel' | 'loop_start' | 'loop_end' | 'fallback' | 'sub_workflow';
  handler?: (input: string, context: WorkflowContext) => Promise<string>;
  config?: Record<string, unknown>;
}

export interface WorkflowTransition {
  from: string;
  to: string;
  condition?: (result: string) => boolean;
}

export interface WorkflowConfig {
  type: WorkflowType;
  nodes: WorkflowNode[];
  transitions: WorkflowTransition[];
  maxRetries?: number;
  retryBackoffMs?: number;
}

export interface WorkflowContext {
  nodeResults: Record<string, string>;
  currentTurn: number;
  metadata: Record<string, unknown>;
}

export interface WorkflowResult {
  status: WorkflowStatus;
  output: string;
  nodeResults: Record<string, { output: string; status: string; duration: number }>;
  totalDuration: number;
  error?: Error;
}

export interface WorkflowEvent {
  type: 'node_start' | 'node_complete' | 'node_error' | 'workflow_complete' | 'workflow_error';
  nodeId?: string;
  timestamp: Date;
  data?: Record<string, unknown>;
}
