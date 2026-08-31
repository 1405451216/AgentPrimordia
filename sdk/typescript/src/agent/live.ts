/**
 * live.ts — 常驻运行时 TS 端（矩阵 #4 对等：Node 长循环 + idle 调度）
 *
 * 与 Go internal/agent/live/ 逐语义对齐（单线程事件循环下天然串行）：
 *   - 多源自唤醒（timer/file-manual 注入；webhook 为调用方注入面）；
 *   - 预算护栏确定性不变式：MaxTokens 账面硬顶（到顶钳制、超额 0）+
 *     MaxTasks 任务数上限；闲时代谢只受 token 闸约束；
 *   - Guardian 守护执行：异常捕获 = 崩溃自愈（crashed outcome + 审计）；
 *   - IdleScheduler 循环代谢：优先级 + 冷却间隔，失败不进冷却每步重试；
 *   - Runtime 逐步驱动（handleWake/idleStep 可单步调用——确定性 harness
 *     无需真实时间模拟任意时长常驻）；链式哈希审计链 verify。
 */

// ===== 类型（与 Go types.go 对齐）=====

export const WAKE_SOURCES = ['timer', 'file', 'webhook', 'manual', 'idle_tick'] as const;
export type WakeSource = (typeof WAKE_SOURCES)[number];

export interface WakeEvent {
  source: WakeSource;
  detail?: string;
  payload?: string;
  at?: number; // epoch ms（缺省取时钟）
}

export interface TaskSpec {
  id: string;
  input: string;
  wake: WakeEvent;
}

export interface TaskOutcome {
  taskId: string;
  wake: WakeEvent;
  success: boolean;
  output: string;
  errText?: string;
  crashed: boolean;
  tokens: number;
  at: number;
}

/** 预算（确定性不变式：超额 0）。 */
export class Budget {
  private tasksDoneN = 0;
  private tokensSpentN = 0;

  constructor(
    public readonly maxTasks = 0,
    public readonly maxTokens = 0,
  ) {}

  exhausted(): boolean {
    if (this.maxTasks > 0 && this.tasksDoneN >= this.maxTasks) {
      return true;
    }
    if (this.maxTokens > 0 && this.tokensSpentN >= this.maxTokens) {
      return true;
    }
    return false;
  }

  /** token 预算闸（闲时代谢以此为准；任务数上限不拦运行时自身代谢）。 */
  tokensExhausted(): boolean {
    return this.maxTokens > 0 && this.tokensSpentN >= this.maxTokens;
  }

  record(tokens: number): void {
    this.tasksDoneN++;
    this.tokensSpentN += tokens;
    if (this.maxTokens > 0 && this.tokensSpentN > this.maxTokens) {
      this.tokensSpentN = this.maxTokens; // 钳制：账面不越上限
    }
  }

  snapshot(): { tasksDone: number; tokensSpent: number; exhausted: boolean } {
    return { tasksDone: this.tasksDoneN, tokensSpent: this.tokensSpentN, exhausted: this.exhausted() };
  }
}

/** 执行面（Agent 的窄投影；同步/异步皆可）。 */
export type RunnerFn = (task: TaskSpec) => Promise<{ output: string; tokens: number; err?: Error }> | { output: string; tokens: number; err?: Error };

/** 时钟抽象（确定性测试注入）。 */
export interface Clock {
  now(): number;
}

/** 审计链节点（链式哈希，与 Go 同模式）。 */
export interface AuditEntry {
  seq: number;
  stage: string;
  detail: string;
  prevHash: string;
  hash: string;
  at: number;
}

import { createHash } from 'node:crypto';

function auditHash(e: Omit<AuditEntry, 'hash'>): string {
  return createHash('sha256')
    .update(`${e.seq}|${new Date(e.at).toISOString()}|${e.stage}|${e.detail}|${e.prevHash}`)
    .digest('hex');
}

/** 闲时代谢作业（循环复发：cooldownMs 冷却间隔，失败不进冷却）。 */
export interface IdleJob {
  name: string;
  priority: number; // 数值越小越先
  cooldownMs?: number;
  run: (now: number) => Promise<string> | string; // 返回摘要；抛错 = 本步失败
}

// ===== Waker：多源自唤醒 =====

export class Waker {
  private readonly ch: WakeEvent[] = [];
  private closed = false;
  private readonly watching = new Map<string, { mtimeMs: number; exists: boolean }>();
  private lastTick = 0;

  constructor(
    private readonly clock: Clock,
    public readonly intervalMs = 0,
  ) {}

  /** pollTimer 定时源巡检步（由宿主长循环周期调用）。 */
  pollTimer(): WakeEvent | null {
    if (this.closed || this.intervalMs <= 0) {
      return null;
    }
    const now = this.clock.now();
    if (this.lastTick === 0) {
      this.lastTick = now; // 基线
      return null;
    }
    if (now - this.lastTick >= this.intervalMs) {
      this.lastTick = now;
      const ev: WakeEvent = { source: 'timer', detail: `${this.intervalMs}ms`, at: now };
      return this.emit(ev) ? ev : null;
    }
    return null;
  }

  /** emit 注入一次唤醒（manual/webhook；通道容量 16，满则丢新保旧——唤醒风暴防御）。 */
  emit(ev: WakeEvent): boolean {
    if (this.closed) {
      return false;
    }
    if (!ev.at) {
      ev.at = this.clock.now();
    }
    if (this.ch.length >= 16) {
      return false;
    }
    this.ch.push(ev);
    return true;
  }

  /** drain 取出全部待处理唤醒（主循环消费）。 */
  drain(): WakeEvent[] {
    return this.ch.splice(0, this.ch.length);
  }

  get length(): number {
    return this.ch.length;
  }

  get isClosed(): boolean {
    return this.closed;
  }

  close(): void {
    this.closed = true;
  }
}

// ===== Runtime：常驻主循环 + Guardian + IdleScheduler =====

export interface LiveStats {
  tasksDone: number;
  tasksSucceeded: number;
  crashesHealed: number;
  idleRuns: number;
  budgetTasks: number;
  budgetTokens: number;
  budgetExhausted: boolean;
  uptimeDays: number;
  auditCount: number;
}

export class LiveRuntime {
  private seq = 0;
  private readonly audit: AuditEntry[] = [];
  private readonly idleLastRun = new Map<string, number>();
  private idleRunsN = 0;
  private tasksDoneN = 0;
  private tasksSucceededN = 0;
  private crashesHealedN = 0;
  private readonly startedAt: number;
  private lastHeartbeat = 0;
  private heartbeatsN = 0;
  private readonly idleJobs: IdleJob[] = [];

  constructor(
    private readonly runner: RunnerFn,
    private readonly waker: Waker,
    private readonly clock: Clock,
    private readonly budget: Budget = new Budget(),
  ) {
    this.startedAt = clock.now();
  }

  registerIdleJob(j: IdleJob): void {
    const i = this.idleJobs.findIndex((x) => x.name === j.name);
    if (i >= 0) {
      this.idleJobs[i] = j;
    } else {
      this.idleJobs.push(j);
    }
    this.idleJobs.sort((a, b) => a.priority - b.priority);
  }

  /** 处理一批待处理唤醒（主循环步进；返回逐任务 outcome，nil 语义 = 预算拒绝）。 */
  async processWakes(): Promise<(TaskOutcome | null)[]> {
    const outs: (TaskOutcome | null)[] = [];
    for (const ev of this.waker.drain()) {
      outs.push(await this.handleWake(ev));
    }
    return outs;
  }

  /** handleWake 处理一次唤醒（预算耗尽返回 null——超额 0 不变式）。 */
  async handleWake(ev: WakeEvent): Promise<TaskOutcome | null> {
    this.heartbeatsN++;
    this.lastHeartbeat = this.clock.now();
    if (this.budget.exhausted()) {
      this.appendAudit('budget_block', `唤醒 ${ev.source} 被预算护栏拒绝（超额 0 不变式）`);
      return null;
    }
    this.seq++;
    const task: TaskSpec = { id: `run-${String(this.seq).padStart(6, '0')}`, input: ev.payload ?? '', wake: ev };
    const outcome = await this.guardedRun(task);
    this.tasksDoneN++;
    if (outcome.crashed) {
      this.crashesHealedN++;
      this.budget.record(0);
      this.appendAudit('self_heal', `${task.id} 崩溃已恢复：${outcome.errText}`);
    } else if (outcome.success) {
      this.tasksSucceededN++;
      this.budget.record(outcome.tokens);
      this.appendAudit('task', `${task.id} 成功（唤醒 ${ev.source}，tokens ${outcome.tokens}）`);
    } else {
      this.budget.record(outcome.tokens);
      this.appendAudit('task', `${task.id} 失败：${outcome.errText}`);
    }
    return outcome;
  }

  /** guardedRun 守护执行（异常捕获 = 崩溃自愈核心）。 */
  private async guardedRun(task: TaskSpec): Promise<TaskOutcome> {
    const outcome: TaskOutcome = {
      taskId: task.id,
      wake: task.wake,
      success: false,
      output: '',
      crashed: false,
      tokens: 0,
      at: this.clock.now(),
    };
    try {
      const r = await this.runner(task);
      outcome.output = r.output;
      outcome.tokens = r.tokens;
      if (r.err) {
        outcome.errText = r.err.message;
      } else {
        outcome.success = true;
      }
    } catch (e) {
      outcome.crashed = true;
      outcome.errText = `panic recovered: ${(e as Error).message}`;
      outcome.tokens = 0;
    }
    return outcome;
  }

  /** idleStep 闲时代谢步（执行至多一个合格作业；无合格作业返回 null）。 */
  async idleStep(): Promise<string | null> {
    if (this.budget.tokensExhausted()) {
      this.appendAudit('idle', 'token 预算耗尽，闲时代谢暂停');
      return null;
    }
    const now = this.clock.now();
    for (const j of this.idleJobs) {
      const last = this.idleLastRun.get(j.name);
      if (j.cooldownMs && last !== undefined && now - last < j.cooldownMs) {
        continue; // 冷却中
      }
      this.idleRunsN++;
      let summary: string;
      try {
        summary = await j.run(now);
      } catch (e) {
        this.appendAudit('idle', `作业 ${j.name} 失败：${(e as Error).message}`);
        continue; // 失败不进冷却——下步重试
      }
      this.idleLastRun.set(j.name, now);
      this.appendAudit('idle', `作业 ${j.name}：${summary}`);
      return `${j.name}: ${summary}`;
    }
    return null;
  }

  heartbeat(): { last: number; count: number } {
    return { last: this.lastHeartbeat, count: this.heartbeatsN };
  }

  /** budgetState 预算快照（可观测；与 Go Budget.Snapshot 同口径）。 */
  budgetState(): { tasksDone: number; tokensSpent: number; exhausted: boolean } {
    return this.budget.snapshot();
  }

  stats(): LiveStats {
    const snap = this.budget.snapshot();
    return {
      tasksDone: this.tasksDoneN,
      tasksSucceeded: this.tasksSucceededN,
      crashesHealed: this.crashesHealedN,
      idleRuns: this.idleRunsN,
      budgetTasks: snap.tasksDone,
      budgetTokens: snap.tokensSpent,
      budgetExhausted: snap.exhausted,
      uptimeDays: (this.clock.now() - this.startedAt) / 86400000,
      auditCount: this.audit.length,
    };
  }

  /** appendAudit 追加审计节点（链式哈希）。 */
  private appendAudit(stage: string, detail: string): void {
    const prev = this.audit.length > 0 ? this.audit[this.audit.length - 1].hash : 'genesis';
    const e: Omit<AuditEntry, 'hash'> = {
      seq: this.audit.length + 1,
      stage,
      detail,
      prevHash: prev,
      at: this.clock.now(),
    };
    this.audit.push({ ...e, hash: auditHash(e) });
  }

  auditEntries(): AuditEntry[] {
    return this.audit.map((e) => ({ ...e }));
  }

  /** verifyAudit 全链校验。 */
  verifyAudit(): Error | null {
    let prev = 'genesis';
    for (let i = 0; i < this.audit.length; i++) {
      const e = this.audit[i];
      if (e.prevHash !== prev || e.seq !== i + 1 || e.hash !== auditHash(e)) {
        return new Error(`live: 审计链第 ${i + 1} 节点校验失败`);
      }
      prev = e.hash;
    }
    return null;
  }
}

/** hexToBytes re-export guard（防 tree-shaking 误删；供宿主工具复用）。 */
export const LIVE_PROTOCOL_VERSION = 'ap-live-v1';
