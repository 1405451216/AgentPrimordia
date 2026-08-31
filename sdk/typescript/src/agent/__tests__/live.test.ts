/**
 * live.test.ts — 常驻运行时 TS 端测试（矩阵 #4 对等；与 Go runtime_test 同语义）。
 * 确定性闭环无真实睡眠：崩溃自愈 / 预算超额 0 / idle 冷却代谢 / 伪时钟 14 天模拟。
 */
import { describe, expect, it } from 'vitest';
import { Budget, LiveRuntime, Waker, type Clock, type IdleJob, type RunnerFn } from '../live.js';

const DAY = 86400000;

class FakeClock implements Clock {
  constructor(public t = Date.UTC(2026, 8, 1)) {}
  now(): number {
    return this.t;
  }
  advance(ms: number): void {
    this.t += ms;
  }
}

function runner(panicOn: Set<string>, failOn: Set<string>, tokens: Record<string, number>): RunnerFn {
  return async (task) => {
    if (panicOn.has(task.input)) {
      throw new Error(`注入崩溃：${task.input}`);
    }
    if (failOn.has(task.input)) {
      return { output: '', tokens: tokens[task.input] ?? 0, err: new Error('任务失败') };
    }
    return { output: `完成 ${task.input}`, tokens: tokens[task.input] ?? 0 };
  };
}

describe('常驻运行时（矩阵 #4 对等）', () => {
  it('崩溃注入 ×3 全部自愈，运行时存活，审计链可追溯', async () => {
    const clock = new FakeClock();
    const rt = new LiveRuntime(runner(new Set(['坏1', '坏2', '坏3']), new Set(), { 好: 100 }), new Waker(clock), clock, new Budget());
    for (const input of ['坏1', '坏2', '坏3', '好']) {
      const out = await rt.handleWake({ source: 'manual', payload: input });
      expect(out).not.toBeNull();
    }
    const s = rt.stats();
    expect(s.tasksDone).toBe(4);
    expect(s.crashesHealed).toBe(3);
    expect(s.tasksSucceeded).toBe(1);
    expect(rt.heartbeat().count).toBe(4);
    expect(rt.verifyAudit()).toBeNull();
    expect(rt.auditEntries().filter((e) => e.stage === 'self_heal')).toHaveLength(3);
  });

  it('预算不变式：账面钳制到顶、到顶拒绝（超额 0）', async () => {
    const clock = new FakeClock();
    const rt = new LiveRuntime(runner(new Set(), new Set(), { a: 60, b: 60, c: 60 }), new Waker(clock), clock, new Budget(0, 100));
    expect(await rt.handleWake({ source: 'manual', payload: 'a' })).not.toBeNull();
    expect(await rt.handleWake({ source: 'manual', payload: 'b' })).not.toBeNull();
    const snap = rt.budgetState();
    expect(snap.tokensSpent).toBe(100); // 钳制：绝不越限
    expect(snap.exhausted).toBe(true);
    expect(await rt.handleWake({ source: 'manual', payload: 'c' })).toBeNull(); // 到顶拒绝
    expect(rt.verifyAudit()).toBeNull();
  });

  it('idle 代谢：优先级 + 冷却 + 失败重试', async () => {
    const clock = new FakeClock();
    const rt = new LiveRuntime(runner(new Set(), new Set(), {}), new Waker(clock), clock, new Budget());
    const order: string[] = [];
    const cool = DAY;
    const jobs: IdleJob[] = [
      { name: '蒸馏学习', priority: 5, cooldownMs: cool, run: () => (order.push('蒸馏学习'), '闭环完成') },
      { name: '模型整理', priority: 1, cooldownMs: cool, run: () => (order.push('模型整理'), '整理完成') },
      { name: '工具制造', priority: 3, cooldownMs: cool, run: () => (order.push('工具制造'), Promise.reject(new Error('无候选'))) },
    ];
    for (const j of jobs) {
      rt.registerIdleJob(j);
    }
    expect(await rt.idleStep()).toBe('模型整理: 整理完成');
    expect(await rt.idleStep()).toBe('蒸馏学习: 闭环完成'); // 工具制造失败跳过
    expect(await rt.idleStep()).toBeNull(); // 冷却期内真闲
    clock.advance(cool);
    expect(await rt.idleStep()).toBe('模型整理: 整理完成');
    expect(await rt.idleStep()).toBe('蒸馏学习: 闭环完成');
    // 步序：整理(1) 制造败+学习(2) 冷却检查步制造重试(1) 整理(1) 制造败+学习(2) = 7
    expect(rt.stats().idleRuns).toBe(7);
    expect(rt.auditEntries().filter((e) => e.stage === 'idle' && e.detail.includes('工具制造 失败'))).toHaveLength(3);
    expect(rt.verifyAudit()).toBeNull();
  });

  it('伪时钟 14 天常驻模拟：14 任务 / 2 崩溃自愈 / 14 次闲时学习 / 审计完整', async () => {
    const clock = new FakeClock();
    let day = 0;
    const rt = new LiveRuntime(
      async () => {
        day++;
        if (day === 5 || day === 9) {
          throw new Error(`第 ${day} 天崩溃注入`);
        }
        return { output: '日巡检完成', tokens: 50 };
      },
      new Waker(clock, DAY),
      clock,
      new Budget(14),
    );
    rt.registerIdleJob({ name: '夜间学习', priority: 1, cooldownMs: DAY, run: () => '轨迹蒸馏一轮' });

    for (let d = 0; d < 14; d++) {
      const out = await rt.handleWake({ source: 'timer', detail: '日巡检', payload: 'day' });
      expect(out).not.toBeNull();
      expect(await rt.idleStep()).not.toBeNull();
      clock.advance(DAY);
    }
    const s = rt.stats();
    expect(s.uptimeDays).toBeGreaterThan(13);
    expect(s.uptimeDays).toBeLessThanOrEqual(14.01);
    expect(s.tasksDone).toBe(14);
    expect(s.crashesHealed).toBe(2);
    expect(s.tasksSucceeded).toBe(12);
    expect(s.idleRuns).toBe(14);
    expect(s.budgetTokens).toBe(600); // 12 成功 × 50，崩溃任务不记账
    expect(rt.verifyAudit()).toBeNull();
  });

  it('Waker 定时源：基线周期不唤醒、到期唤醒一次', () => {
    const clock = new FakeClock();
    const w = new Waker(clock, DAY);
    expect(w.pollTimer()).toBeNull(); // 基线
    clock.advance(2 * DAY);
    const ev = w.pollTimer();
    expect(ev?.source).toBe('timer');
    expect(w.pollTimer()).toBeNull(); // 未到期
    w.close();
    expect(w.pollTimer()).toBeNull();
  });
});
