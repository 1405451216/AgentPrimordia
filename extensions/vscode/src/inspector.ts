/**
 * Agent Inspector 核心状态机。
 *
 * 这是与 VS Code API 解耦的纯逻辑实现：消费 ReAct 步骤事件并维护
 * InspectorState。InspectorView（webview）与 InspectorHost（vscode
 * extension）通过 applyCommand / applyStep 接口交互。
 */

import type {
  InspectorCommand,
  InspectorState,
  InspectorStatus,
  InspectorStep,
  InspectorStepKind,
} from './types.js';

/** 构造初始状态 */
export function createInspectorState(): InspectorState {
  return {
    status: 'idle',
    steps: [],
    currentPrompt: '',
    tokens: 0,
    error: null,
    startedAt: null,
    endedAt: null,
    breakpoints: new Set<number>(),
  };
}

/**
 * 不可变更新辅助：返回新 state 对象，不修改原对象。
 *
 * Webview 通过 === 比较 state 引用决定是否重渲染，所以每次更新
 * 必须返回新对象。但内部引用类型（breakpoints）允许原地修改并返
 * 回新 Set 以触发引用变化。
 */
function transition(
  state: InspectorState,
  patch: Partial<Omit<InspectorState, 'breakpoints' | 'steps'>> & {
    steps?: InspectorStep[];
    breakpoints?: Set<number>;
  },
): InspectorState {
  return {
    ...state,
    ...patch,
    steps: patch.steps ?? state.steps,
    breakpoints: patch.breakpoints ?? state.breakpoints,
  };
}

/** 应用 InspectorCommand，返回新 state 与是否需要触发副作用。 */
export interface InspectorCommandResult {
  state: InspectorState;
  /** 是否需要在断点处暂停（host 应等待 resume） */
  pauseRequested: boolean;
  /** 命令是否为 start（host 应启动 agent 运行） */
  startRequested: boolean;
}

export function applyCommand(
  state: InspectorState,
  cmd: InspectorCommand,
): InspectorCommandResult {
  let pauseRequested = false;
  let startRequested = false;

  switch (cmd.type) {
    case 'start': {
      const now = Date.now();
      const next: InspectorState = transition(state, {
        status: 'running',
        currentPrompt: cmd.prompt,
        steps: [],
        tokens: 0,
        error: null,
        startedAt: now,
        endedAt: null,
      });
      startRequested = true;
      return { state: next, pauseRequested, startRequested };
    }
    case 'stop': {
      const next: InspectorState = transition(state, {
        status: 'done',
        endedAt: Date.now(),
      });
      return { state: next, pauseRequested, startRequested };
    }
    case 'pause': {
      if (state.status !== 'running') {
        return { state, pauseRequested: false, startRequested: false };
      }
      const next = transition(state, { status: 'paused' });
      return { state: next, pauseRequested: false, startRequested: false };
    }
    case 'resume': {
      if (state.status !== 'paused') {
        return { state, pauseRequested: false, startRequested: false };
      }
      const next = transition(state, { status: 'running' });
      return { state: next, pauseRequested: false, startRequested: false };
    }
    case 'reset': {
      const next = createInspectorState();
      return { state: next, pauseRequested: false, startRequested: false };
    }
    case 'addBreakpoint': {
      const bp = new Set(state.breakpoints);
      bp.add(cmd.stepIndex);
      const next = transition(state, { breakpoints: bp });
      return { state: next, pauseRequested, startRequested };
    }
    case 'removeBreakpoint': {
      const bp = new Set(state.breakpoints);
      bp.delete(cmd.stepIndex);
      const next = transition(state, { breakpoints: bp });
      return { state: next, pauseRequested, startRequested };
    }
    default: {
      // 兜底：未知命令直接返回原 state
      return { state, pauseRequested: false, startRequested: false };
    }
  }
}

/**
 * 应用单步事件（来自 ReAct 流）。
 *
 * 返回：
 * - state: 新 state
 * - hitBreakpoint: 是否遇到断点（host 应自动 pause）
 */
export function applyStep(
  state: InspectorState,
  kind: InspectorStepKind,
  payload: { text?: string; tool?: string; args?: unknown },
): { state: InspectorState; hitBreakpoint: boolean } {
  if (state.status !== 'running' && state.status !== 'paused') {
    // 非运行/暂停状态不接受 step
    return { state, hitBreakpoint: false };
  }

  const step: InspectorStep = {
    index: state.steps.length + 1,
    kind,
    text: payload.text,
    tool: payload.tool,
    args: payload.args,
    timestamp: Date.now(),
  };

  const steps = [...state.steps, step];

  // 估算 token：thought/action/observation 文本长度按 4 字符/token
  const added = (payload.text ?? '').length + (payload.tool ?? '').length;
  const tokens = state.tokens + Math.max(1, Math.ceil(added / 4));

  const next = transition(state, { steps, tokens });

  // 检查断点：每个新 step 的 index 加入检查
  if (next.breakpoints.has(step.index)) {
    return {
      state: transition(next, { status: 'paused' }),
      hitBreakpoint: true,
    };
  }

  // done / error 状态收尾
  if (kind === 'done') {
    return {
      state: transition(next, { status: 'done', endedAt: Date.now() }),
      hitBreakpoint: false,
    };
  }
  if (kind === 'error') {
    const err = new Error(payload.text ?? 'unknown error');
    return {
      state: transition(next, {
        status: 'error',
        error: err,
        endedAt: Date.now(),
      }),
      hitBreakpoint: false,
    };
  }

  return { state: next, hitBreakpoint: false };
}

/** 序列化 state 用于 Webview 传输（breakpoints 转为数组） */
export function serializeState(state: InspectorState): Omit<InspectorState, 'breakpoints'> & {
  breakpoints: number[];
} {
  return {
    ...state,
    breakpoints: Array.from(state.breakpoints).sort((a, b) => a - b),
  };
}

/** 从 serialized state 反序列化（接收 webview 命令） */
export function deserializeBreakpoints(arr: number[]): Set<number> {
  return new Set(arr.filter((n) => Number.isInteger(n) && n > 0));
}

/** 估算步骤运行时长（ms）。未启动或未结束时返回 0。 */
export function elapsedMs(state: InspectorState, now: number = Date.now()): number {
  if (state.startedAt === null) return 0;
  const end = state.endedAt ?? now;
  return Math.max(0, end - state.startedAt);
}

/** 状态枚举到人可读标签 */
export function statusLabel(status: InspectorStatus): string {
  switch (status) {
    case 'idle':
      return '空闲';
    case 'running':
      return '运行中';
    case 'paused':
      return '已暂停';
    case 'done':
      return '已完成';
    case 'error':
      return '错误';
    default:
      return status;
  }
}

/** 步骤类型到人可读标签 */
export function stepKindLabel(kind: InspectorStepKind): string {
  switch (kind) {
    case 'thought':
      return '思考';
    case 'action':
      return '工具调用';
    case 'observation':
      return '观察';
    case 'turn':
      return '轮次';
    case 'done':
      return '完成';
    case 'error':
      return '错误';
    default:
      return kind;
  }
}