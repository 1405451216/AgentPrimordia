/**
 * Inspector 展示层工具。
 *
 * 与 inspector.ts 解耦：负责把 InspectorState / InspectorStep 渲染
 * 成可读字符串，供 Webview 渲染层与日志层共用。
 */

import type { InspectorState, InspectorStep, InspectorStepKind } from './types.js';
import { stepKindLabel, statusLabel, elapsedMs } from './inspector.js';

/** 把单步渲染成 markdown 风格文本（含时间戳与序号） */
export function formatStep(step: InspectorStep): string {
  const ts = new Date(step.timestamp).toISOString().slice(11, 23);
  const head = `[${ts}] #${step.index} ${stepKindLabel(step.kind)}`;
  switch (step.kind) {
    case 'thought':
      return `${head}\n${step.text ?? ''}`;
    case 'action':
      return `${head}\n${step.tool ?? ''}(${formatArgs(step.args)})`;
    case 'observation':
      return `${head}\n${step.text ?? ''}${step.tool ? ` (${step.tool})` : ''}`;
    case 'turn':
      return `${head} turn=${(step.args as { turn?: number } | undefined)?.turn ?? '?'}`;
    case 'done':
      return `${head}\n${step.text ?? ''}`;
    case 'error':
      return `${head}\n${step.text ?? 'unknown error'}`;
    default:
      return `${head}`;
  }
}

function formatArgs(args: unknown): string {
  if (args === undefined || args === null) return '';
  try {
    const json = JSON.stringify(args);
    return json.length > 200 ? json.slice(0, 200) + '…' : json;
  } catch {
    return String(args);
  }
}

/** 渲染整个 Inspector 状态为多行摘要（供 status bar / 输出通道） */
export function formatStateSummary(state: InspectorState): string {
  const parts: string[] = [];
  parts.push(`状态: ${statusLabel(state.status)}`);
  parts.push(`步骤数: ${state.steps.length}`);
  parts.push(`Token 估算: ${state.tokens}`);
  parts.push(`断点: ${Array.from(state.breakpoints).sort((a, b) => a - b).join(', ') || '(无)'}`);
  if (state.currentPrompt) parts.push(`Prompt: ${state.currentPrompt}`);
  if (state.startedAt) {
    parts.push(`运行: ${formatDuration(elapsedMs(state))}`);
  }
  if (state.error) parts.push(`错误: ${state.error.message}`);
  return parts.join('\n');
}

/** 渲染步骤历史为多行（步骤用 formatStep 渲染，步骤间用空行隔开） */
export function formatStepHistory(state: InspectorState): string {
  return state.steps.map(formatStep).join('\n\n');
}

/** 毫秒时长格式化为 "1m23.456s" / "456ms" */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const sec = ms / 1000;
  if (sec < 60) return `${sec.toFixed(3)}s`;
  const min = Math.floor(sec / 60);
  const rem = sec - min * 60;
  return `${min}m${rem.toFixed(3)}s`;
}

/** 把状态机事件折叠为简洁的 step 类型列表（用于断点 UI） */
export function listStepKinds(state: InspectorState): InspectorStepKind[] {
  return state.steps.map((s) => s.kind);
}

/** 计算当前 step 在所有步骤中的进度（0-1）。无步骤返回 0。 */
export function progressRatio(state: InspectorState): number {
  if (state.steps.length === 0) return 0;
  if (state.status === 'done' || state.status === 'error') return 1;
  return Math.min(1, state.steps.length / Math.max(1, state.steps.length + 1));
}

/** 把 InspectorState 转成 Webview HTML 友好的 JSON（用于 postMessage） */
export function toWebviewPayload(state: InspectorState): Record<string, unknown> {
  return {
    status: state.status,
    statusLabel: statusLabel(state.status),
    steps: state.steps.map((s) => ({
      index: s.index,
      kind: s.kind,
      kindLabel: stepKindLabel(s.kind),
      text: s.text,
      tool: s.tool,
      args: s.args,
      timestamp: s.timestamp,
    })),
    currentPrompt: state.currentPrompt,
    tokens: state.tokens,
    error: state.error ? { message: state.error.message } : null,
    startedAt: state.startedAt,
    endedAt: state.endedAt,
    elapsedMs: elapsedMs(state),
    elapsedLabel: formatDuration(elapsedMs(state)),
    breakpoints: Array.from(state.breakpoints).sort((a, b) => a - b),
    progress: progressRatio(state),
  };
}