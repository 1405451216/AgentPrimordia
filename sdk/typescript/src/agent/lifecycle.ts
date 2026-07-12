/**
 * Agent 生命周期管理器 — 从 react-loop.ts 拆分
 *
 * 与 Go 端 Lifecycle 对齐，提供：
 * - 状态管理（idle / running / paused / completed / error）
 * - 停止信号（stop / isStopped / onStop）
 * - 暂停/恢复（pause / resume / waitPause / waitResume）
 */
import type { AgentStatus } from '../types.js';

/** 生命周期管理器，控制 Agent 的启动、停止、暂停和恢复。
 *
 * 与 Go 端 Lifecycle 对齐，提供：
 * - 状态管理（idle / running / paused / completed / error）
 * - 停止信号（stop / isStopped / onStop）
 * - 暂停/恢复（pause / resume / waitPause / waitResume）
 */
export class Lifecycle {
  private _status: AgentStatus = 'idle';
  private stopped = false;
  private stopResolvers: (() => void)[] = [];
  private pauseResolvers: (() => void)[] = [];
  private resumeResolvers: (() => void)[] = [];
  private paused = false;

  /** 获取当前状态 */
  get status(): AgentStatus {
    return this._status;
  }

  /** 设置状态 */
  setStatus(s: AgentStatus): void {
    this._status = s;
  }

  /** 发送停止信号，唤醒所有等待停止的 Promise */
  stop(): void {
    this.stopped = true;
    for (const r of this.stopResolvers) r();
    this.stopResolvers = [];
  }

  /** 检查是否已收到停止信号 */
  isStopped(): boolean {
    return this.stopped;
  }

  /** 等待停止信号，返回 Promise 在 stop() 调用时 resolve */
  onStop(): Promise<void> {
    if (this.stopped) return Promise.resolve();
    return new Promise((r) => this.stopResolvers.push(r));
  }

  /** 暂停 Agent，状态从 running 变为 paused */
  pause(): void {
    if (this._status !== 'running') return;
    this._status = 'paused';
    this.paused = true;
    for (const r of this.pauseResolvers) r();
    this.pauseResolvers = [];
  }

  /** 恢复 Agent，状态从 paused 变为 running */
  resume(): void {
    if (this._status !== 'paused') return;
    this._status = 'running';
    this.paused = false;
    for (const r of this.resumeResolvers) r();
    this.resumeResolvers = [];
  }

  /** 等待暂停完成，返回 Promise 在 pause() 调用时 resolve */
  waitPause(): Promise<void> {
    if (this.paused) return Promise.resolve();
    return new Promise((r) => this.pauseResolvers.push(r));
  }

  /** 等待恢复完成，返回 Promise 在 resume() 调用时 resolve */
  waitResume(): Promise<void> {
    if (!this.paused) return Promise.resolve();
    return new Promise((r) => this.resumeResolvers.push(r));
  }
}
