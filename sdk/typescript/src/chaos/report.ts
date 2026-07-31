/**
 * report.ts — 实验报告生成
 *
 * 对齐 Go 端 internal/chaos/report.go
 * Stability: Experimental
 */

import type { ExperimentResult, ExperimentStatus } from './engine.js';

/** 实验摘要（用于批量报告，对齐 Go ExperimentSummary） */
export interface ExperimentSummary {
  name: string;
  status: ExperimentStatus;
  hypothesisValidated: boolean;
  durationMs: number;
  faultCount: number;
  steadyStateMet: boolean;
}

/** 生成实验摘要（对齐 Go Summarize） */
export function summarize(result: ExperimentResult): ExperimentSummary {
  return {
    name: result.experiment.name,
    status: result.status,
    hypothesisValidated: result.hypothesisValidated,
    durationMs: result.durationMs,
    faultCount: result.faultResults.length,
    steadyStateMet: result.postSteadyState?.met ?? true,
  };
}

/** 将实验结果格式化为 Markdown 报告（对齐 Go FormatReport） */
export function formatReport(result: ExperimentResult): string {
  const lines: string[] = [];

  lines.push('# 混沌实验报告');
  lines.push('');
  lines.push(`**实验名称**: ${result.experiment.name}`);
  lines.push('');

  if (result.experiment.description) {
    lines.push(`**描述**: ${result.experiment.description}`);
    lines.push('');
  }

  lines.push(`**假设**: ${result.experiment.hypothesis}`);
  lines.push('');

  const statusIcon = result.hypothesisValidated ? '✅' : '❌';
  lines.push(`**假设验证**: ${statusIcon} ${result.hypothesisValidated ? '已验证' : '未验证'}`);
  lines.push('');
  lines.push(`**状态**: ${result.status}`);
  lines.push('');
  lines.push(`**持续时间**: ${result.durationMs}ms`);
  lines.push('');
  lines.push(`**开始时间**: ${result.startTime.toISOString()}`);
  lines.push('');
  lines.push(`**结束时间**: ${result.endTime.toISOString()}`);
  lines.push('');

  // 故障注入结果
  if (result.faultResults.length > 0) {
    lines.push('## 注入的故障');
    lines.push('');
    lines.push('| # | 类型 | 描述 | 注入状态 | 注入时间 | 清理时间 | 错误 |');
    lines.push('|---|------|------|----------|----------|----------|------|');
    result.faultResults.forEach((fr, i) => {
      const injected = fr.injected ? '✅ 成功' : '❌ 失败';
      const injectTime = fr.injectTime.toLocaleTimeString();
      const cleanupTime = fr.cleanupTime ? fr.cleanupTime.toLocaleTimeString() : '-';
      const errMsg = fr.error?.message ?? '';
      lines.push(`| ${i + 1} | ${fr.faultType} | ${fr.description} | ${injected} | ${injectTime} | ${cleanupTime} | ${errMsg} |`);
    });
    lines.push('');
  }

  // 稳态检查
  if (result.experiment.steadyState) {
    lines.push('## 稳态检查');
    lines.push('');
    lines.push(`**稳态条件**: ${result.experiment.steadyState.name()}`);
    lines.push('');

    lines.push('### 实验前');
    lines.push('');
    if (result.preSteadyState) {
      formatSteadyStateResult(lines, result.preSteadyState);
    }

    lines.push('### 实验后');
    lines.push('');
    if (result.postSteadyState) {
      formatSteadyStateResult(lines, result.postSteadyState);
    }
  }

  // 标签
  if (result.experiment.tags && result.experiment.tags.length > 0) {
    lines.push('');
    lines.push(`**标签**: ${result.experiment.tags.join(', ')}`);
  }

  return lines.join('\n');
}

function formatSteadyStateResult(lines: string[], r: { met: boolean; message: string; details?: Record<string, unknown> }): void {
  const metIcon = r.met ? '✅' : '❌';
  lines.push(`- **状态**: ${metIcon} ${r.met ? '满足' : '不满足'}`);
  lines.push(`- **消息**: ${r.message}`);
  if (r.details && Object.keys(r.details).length > 0) {
    lines.push('- **详情**:');
    for (const [k, v] of Object.entries(r.details)) {
      lines.push(`  - ${k}: ${v}`);
    }
  }
  lines.push('');
}

/** 将多个实验摘要格式化为表格（对齐 Go FormatSummaryTable） */
export function formatSummaryTable(summaries: ExperimentSummary[]): string {
  const lines: string[] = [];
  lines.push('| 实验 | 状态 | 假设 | 持续时间 | 故障数 | 稳态 |');
  lines.push('|------|------|------|----------|--------|------|');

  for (const s of summaries) {
    const validated = s.hypothesisValidated ? '✅' : '❌';
    const met = s.steadyStateMet ? '✅' : '❌';
    lines.push(`| ${s.name} | ${s.status} | ${validated} | ${s.durationMs}ms | ${s.faultCount} | ${met} |`);
  }

  return lines.join('\n');
}
