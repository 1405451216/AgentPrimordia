/**
 * 失败记录与诊断（v3.4-6d，TS 侧）
 *
 * 对齐 Go internal/persist/failure.go：FailureRecord 携带完整上下文
 * （阶段/错误/输入/内嵌 checkpoint），MemoryFailureStore 为内存实现，
 * diagnoseFailure 输出结构化诊断与复盘线索。
 */
import type { Checkpoint } from './request-id.js';

/** 失败发生的阶段 */
export type FailurePhase = 'run' | 'plan';

/** 完整失败记录：上下文 + 内嵌 checkpoint，支持一键重放 */
export interface FailureRecord {
  id: string;
  agentId: string;
  sessionId: string;
  phase: FailurePhase;
  error: string;
  turn: number;
  subtaskId?: string;
  input?: string;
  state?: Checkpoint;
  createdAt: string;
}

/** 列表过滤条件 */
export interface FailureFilter {
  agentId?: string;
  sessionId?: string;
}

/** 失败记录存储接口（可替换为持久化实现） */
export interface FailureStore {
  record(rec: FailureRecord): Promise<void>;
  get(id: string): Promise<FailureRecord | null>;
  list(filter?: FailureFilter): Promise<FailureRecord[]>;
  delete(id: string): Promise<boolean>;
}

/** 内存失败存储：按写入顺序倒序返回（最新在前） */
export class MemoryFailureStore implements FailureStore {
  private records = new Map<string, { rec: FailureRecord; seq: number }>();
  private seq = 0;

  async record(rec: FailureRecord): Promise<void> {
    if (!rec.id) throw new Error('failure record id is required');
    this.records.set(rec.id, { rec, seq: this.seq++ });
  }

  async get(id: string): Promise<FailureRecord | null> {
    return this.records.get(id)?.rec ?? null;
  }

  async list(filter?: FailureFilter): Promise<FailureRecord[]> {
    let entries = [...this.records.values()];
    if (filter?.agentId) entries = entries.filter((e) => e.rec.agentId === filter.agentId);
    if (filter?.sessionId) entries = entries.filter((e) => e.rec.sessionId === filter.sessionId);
    return entries.sort((a, b) => b.seq - a.seq).map((e) => e.rec);
  }

  async delete(id: string): Promise<boolean> {
    return this.records.delete(id);
  }
}

/** 结构化诊断失败记录：阶段结论 + 复盘线索 */
export function diagnoseFailure(rec: FailureRecord): string {
  const lines: string[] = [];
  lines.push(`[diagnosis] failure id: ${rec.id}`);
  if (rec.phase === 'plan' && rec.subtaskId) {
    lines.push(`阶段结论: 子任务 ${rec.subtaskId} 规划/执行失败`);
    lines.push('复盘建议: 检查该子任务的依赖与输入，单独重跑验证');
  } else {
    lines.push('阶段结论: LLM 调用或工具执行失败');
    lines.push('复盘建议: 从内嵌 checkpoint 重放，确认错误是否可复现');
  }
  lines.push(`错误信息: ${rec.error}`);
  if (!rec.input) lines.push('输入为空: 疑似无效调用');
  if (!rec.state) lines.push('无内嵌 checkpoint: 仅可上下文重放');
  return lines.join('\n');
}
