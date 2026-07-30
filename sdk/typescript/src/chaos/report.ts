/**
 * report.ts — 实验报告生成
 *
 * 对齐 Go 端 internal/chaos/report.go
 * Stability: Experimental
 */

import type { ExperimentResult, ExperimentStatus } from './engine.js';

/** 实验摘要（用于批量报告） */
export interface ExperimentSummary {
  name: string;
  status: ExperimentStatus;
  hypothesisValidated: boolean;
  durationMs: number;
  faultCount: number;
  steadyStateMet: boolean;
}

/** 生成实验摘要 */
export function summarize(result: ExperimentResult): ExperimentSummary {
  return {
    name: result.experiment.name,
    status: result.status,
    hypothesisValidated: result.hypothesisValidated,
    durationMs: result.durationMs,
    faultCount: result.experiment.faults.length,
    steadyStateMet: result.postSteadyState?.met ?? true,
  };
}

/** 将实验结果格式化为 Markdown 报告 */
export function formatReport(result: ExperimentResult): string {
  const lines: string[] = [];
  const statusIcon = result.status === 'completed' ? '✅' : result.status === 'failed' ? '❌' : '⏭️';

  lines.push(`# 混沌实验报告: ${result.experiment.name}`);
  lines.push('');
  lines.push(`> ${statusIcon} 状态: **${result.status}** | 耗时: ${result.durationMs}ms`);
  lines.push('');

  if (result.experiment.hypothesis) {
    lines.push(`**假设**: ${result.experiment.hypothesis}`);
    lines.push(`**验证结果**: ${result.hypothesisValidated ? '✅ 通过' : '❌ 未通过'}`);
    lines.push('');
  }

  // 故障注入结果
  if (result.faultResults.length > 0) {
    lines.push('## 故障注入');
    lines.push('');
    lines.push('| # | 类型 | 描述 | 状态 |');
    lines.push('|---|------|------|------|');
    result.faultResults.forEach((fr, i) => {
      const icon = fr.injected ? '✅' : '❌';
      lines.push(`| ${i + 1} | ${fr.faultType} | ${fr.description} | ${icon} |`);
    });
    lines.push('');
  }

  // 稳态检查
  if (result.preSteadyState || result.postSteadyState) {
    lines.push('## 稳态验证');
    lines.push('');
    if (result.preSteadyState) {
      lines.push(`- **实验前**: ${result.preSteadyState.met ? '✅' : '❌'} ${result.preSteadyState.message}`);
    }
    if (result.postSteadyState) {
      lines.push(`- **实验后**: ${result.postSteadyState.met ? '✅' : '❌'} ${result.postSteadyState.message}`);
    }
    lines.push('');
  }

  // 错误信息
  if (result.error) {
    lines.push('## 错误');
    lines.push('');
    lines.push(`\`\`\`\n${result.error.message}\n\`\`\``);
    lines.push('');
  }

  return lines.join('\n');
}

/** 将多个实验摘要格式化为表格 */
export function formatSummaryTable(summaries: ExperimentSummary[]): string {
  const lines: string[] = [];
  lines.push('| # | 实验 | 状态 | 假设验证 | 故障数 | 稳态 | 耗时 |');
  lines.push('|---|------|------|----------|--------|------|------|');

  summaries.forEach((s, i) => {
    const statusIcon = s.status === 'completed' ? '✅' : s.status === 'failed' ? '❌' : '⏭️';
    const hypoIcon = s.hypothesisValidated ? '✅' : '❌';
    const ssIcon = s.steadyStateMet ? '✅' : '❌';
    lines.push(`| ${i + 1} | ${s.name} | ${statusIcon} ${s.status} | ${hypoIcon} | ${s.faultCount} | ${ssIcon} | ${s.durationMs}ms |`);
  });

  const passed = summaries.filter(s => s.hypothesisValidated).length;
  lines.push('');
  lines.push(`**总计**: ${passed}/${summaries.length} 假设验证通过`);

  return lines.join('\n');
}
