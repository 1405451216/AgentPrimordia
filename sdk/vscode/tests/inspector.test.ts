/**
 * Agent Inspector 状态机测试（Phase 5 Task 9）。
 */

import { describe, it, expect } from 'vitest';

import {
  createInspectorState,
  applyCommand,
  applyStep,
  serializeState,
  deserializeBreakpoints,
  elapsedMs,
  statusLabel,
  stepKindLabel,
} from '../src/inspector.js';
import {
  formatStep,
  formatStateSummary,
  formatStepHistory,
  formatDuration,
  progressRatio,
  toWebviewPayload,
} from '../src/format.js';

describe('createInspectorState', () => {
  it('初始为空闲状态，无步骤，无断点', () => {
    const s = createInspectorState();
    expect(s.status).toBe('idle');
    expect(s.steps).toEqual([]);
    expect(s.tokens).toBe(0);
    expect(s.error).toBeNull();
    expect(s.startedAt).toBeNull();
    expect(s.endedAt).toBeNull();
    expect(s.breakpoints.size).toBe(0);
  });
});

describe('applyCommand', () => {
  it('start 启动并切换到 running，记录 prompt', () => {
    const s0 = createInspectorState();
    const { state, startRequested } = applyCommand(s0, {
      type: 'start',
      prompt: 'hello',
      maxTurns: 5,
    });
    expect(state.status).toBe('running');
    expect(state.currentPrompt).toBe('hello');
    expect(state.startedAt).not.toBeNull();
    expect(startRequested).toBe(true);
  });

  it('stop 收尾并设置 endedAt', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const { state } = applyCommand(s0, { type: 'stop' });
    expect(state.status).toBe('done');
    expect(state.endedAt).not.toBeNull();
  });

  it('pause 仅在 running 时生效', () => {
    const s0 = createInspectorState();
    const { state: s1 } = applyCommand(s0, { type: 'pause' });
    expect(s1.status).toBe('idle');
    const { state: s2 } = applyCommand(
      applyCommand(s0, { type: 'start', prompt: 'x', maxTurns: 3 }).state,
      { type: 'pause' },
    );
    expect(s2.status).toBe('paused');
  });

  it('resume 仅在 paused 时生效', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'x',
      maxTurns: 3,
    }).state;
    const { state: s1 } = applyCommand(s0, { type: 'resume' });
    expect(s1.status).toBe('running'); // running 不能 resume
    const { state: s2 } = applyCommand(
      applyCommand(s0, { type: 'pause' }).state,
      { type: 'resume' },
    );
    expect(s2.status).toBe('running');
  });

  it('reset 清空所有状态', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'x',
      maxTurns: 3,
    }).state;
    const { state } = applyCommand(s0, { type: 'reset' });
    expect(state.status).toBe('idle');
    expect(state.steps).toEqual([]);
    expect(state.currentPrompt).toBe('');
  });

  it('addBreakpoint 正确添加', () => {
    const s0 = createInspectorState();
    const { state } = applyCommand(s0, { type: 'addBreakpoint', stepIndex: 3 });
    expect(state.breakpoints.has(3)).toBe(true);
  });

  it('removeBreakpoint 删除已存在的断点', () => {
    const s0 = applyCommand(createInspectorState(), { type: 'addBreakpoint', stepIndex: 5 })
      .state;
    const { state } = applyCommand(s0, { type: 'removeBreakpoint', stepIndex: 5 });
    expect(state.breakpoints.has(5)).toBe(false);
  });
});

describe('applyStep', () => {
  it('running 状态下记录 step 并累加 token', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const { state, hitBreakpoint } = applyStep(s0, 'thought', { text: '分析需求' });
    expect(state.steps).toHaveLength(1);
    expect(state.steps[0].kind).toBe('thought');
    expect(state.tokens).toBeGreaterThan(0);
    expect(hitBreakpoint).toBe(false);
  });

  it('done 步骤自动收尾', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const { state } = applyStep(s0, 'done', { text: 'final' });
    expect(state.status).toBe('done');
    expect(state.endedAt).not.toBeNull();
  });

  it('error 步骤切到 error 状态', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const { state } = applyStep(s0, 'error', { text: 'failed' });
    expect(state.status).toBe('error');
    expect(state.error).not.toBeNull();
    expect(state.error?.message).toBe('failed');
  });

  it('在断点处自动暂停', () => {
    const s0 = applyCommand(createInspectorState(), { type: 'addBreakpoint', stepIndex: 1 })
      .state;
    const s1 = applyCommand(s0, { type: 'start', prompt: 'go', maxTurns: 5 }).state;
    const { state, hitBreakpoint } = applyStep(s1, 'thought', { text: 'x' });
    expect(hitBreakpoint).toBe(true);
    expect(state.status).toBe('paused');
  });

  it('非 running 状态下拒绝接收 step', () => {
    const s0 = createInspectorState();
    const { state, hitBreakpoint } = applyStep(s0, 'thought', { text: 'x' });
    expect(state.steps).toHaveLength(0);
    expect(hitBreakpoint).toBe(false);
  });

  it('paused 状态下仍可接收 step 但不重置 status', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const s1 = applyCommand(s0, { type: 'pause' }).state;
    const { state } = applyStep(s1, 'observation', { text: 'ok' });
    expect(state.steps).toHaveLength(1);
    expect(state.status).toBe('paused');
  });
});

describe('serialize / deserialize', () => {
  it('serializeState 把 Set 转为有序数组', () => {
    const s0 = createInspectorState();
    const s1 = applyCommand(s0, { type: 'addBreakpoint', stepIndex: 5 }).state;
    const s2 = applyCommand(s1, { type: 'addBreakpoint', stepIndex: 2 }).state;
    const ser = serializeState(s2);
    expect(ser.breakpoints).toEqual([2, 5]);
  });

  it('deserializeBreakpoints 过滤非法值', () => {
    const bp = deserializeBreakpoints([1, -1, 0, 2.5, 3]);
    expect(Array.from(bp)).toEqual([1, 3]);
  });
});

describe('elapsedMs', () => {
  it('未启动返回 0', () => {
    expect(elapsedMs(createInspectorState())).toBe(0);
  });

  it('启动后返回 now - startedAt', () => {
    const started = Date.now();
    const s = {
      ...createInspectorState(),
      startedAt: started,
      endedAt: null,
    };
    const elapsed = elapsedMs(s, started + 1000);
    expect(elapsed).toBe(1000);
  });
});

describe('label helpers', () => {
  it('statusLabel 返回中文标签', () => {
    expect(statusLabel('idle')).toBe('空闲');
    expect(statusLabel('running')).toBe('运行中');
    expect(statusLabel('paused')).toBe('已暂停');
    expect(statusLabel('done')).toBe('已完成');
    expect(statusLabel('error')).toBe('错误');
  });

  it('stepKindLabel 返回中文标签', () => {
    expect(stepKindLabel('thought')).toBe('思考');
    expect(stepKindLabel('action')).toBe('工具调用');
    expect(stepKindLabel('observation')).toBe('观察');
    expect(stepKindLabel('turn')).toBe('轮次');
    expect(stepKindLabel('done')).toBe('完成');
    expect(stepKindLabel('error')).toBe('错误');
  });
});

describe('format helpers', () => {
  it('formatStep 输出序号+标签+内容', () => {
    const s = {
      index: 1,
      kind: 'thought' as const,
      text: '思考',
      timestamp: Date.UTC(2026, 6, 6, 12, 0, 0),
    };
    const out = formatStep(s);
    expect(out).toMatch(/#1 思考/);
    expect(out).toContain('思考');
  });

  it('formatStep action 显示工具名和参数', () => {
    const s = {
      index: 2,
      kind: 'action' as const,
      tool: 'search',
      args: { q: 'hi' },
      timestamp: Date.now(),
    };
    const out = formatStep(s);
    expect(out).toMatch(/#2 工具调用/);
    expect(out).toContain('search');
    expect(out).toContain('"q":"hi"');
  });

  it('formatDuration 正确格式化 ms/s/m', () => {
    expect(formatDuration(0)).toMatch(/ms$/);
    expect(formatDuration(1500)).toMatch(/s$/);
    expect(formatDuration(65000)).toMatch(/m/);
  });

  it('formatStateSummary 包含关键字段', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const s1 = applyStep(s0, 'thought', { text: 't' }).state;
    const out = formatStateSummary(s1);
    expect(out).toContain('运行中');
    expect(out).toContain('步骤数: 1');
    expect(out).toContain('Token 估算:');
  });

  it('formatStepHistory 多步以空行分隔', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const s1 = applyStep(s0, 'thought', { text: 't1' }).state;
    const s2 = applyStep(s1, 'observation', { text: 'o1' }).state;
    const out = formatStepHistory(s2);
    expect(out.split('\n\n')).toHaveLength(2);
  });

  it('progressRatio 反映进度', () => {
    expect(progressRatio(createInspectorState())).toBe(0);
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const s1 = applyStep(s0, 'thought', { text: 'x' }).state;
    expect(progressRatio(s1)).toBeGreaterThan(0);
    expect(progressRatio(s1)).toBeLessThan(1);
  });

  it('toWebviewPayload 包含 statusLabel 与 elapsedLabel', () => {
    const s0 = applyCommand(createInspectorState(), {
      type: 'start',
      prompt: 'go',
      maxTurns: 5,
    }).state;
    const payload = toWebviewPayload(s0);
    expect(payload.statusLabel).toBe('运行中');
    expect(payload.elapsedLabel).toMatch(/ms|s$/);
    expect(Array.isArray(payload.breakpoints)).toBe(true);
  });
});