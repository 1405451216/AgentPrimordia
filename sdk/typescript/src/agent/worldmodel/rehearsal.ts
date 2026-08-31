/**
 * rehearsal.ts — 预演门（行动前静态检查：计划中每个步骤是否在图上有可执行前提）
 *
 * 与 Go 端 internal/agent/worldmodel/rehearsal.go 逐语义对齐（矩阵 #1 对等）：
 * 纯图算法、无 LLM、确定性输出；缺陷文案（中文）与 Go 逐字一致——跨语言
 * 对账夹具覆盖。本模块只提供检查结果，不拦截任何默认路径。
 */
import { StateGraph } from './graph.js';
import { Plan, PlanStep } from './backdiff.js';

/** 预演结果。 */
export interface RehearsalReport {
  /** 全部步骤的可执行前提均满足时为 true（含空计划）。 */
  Pass: boolean;
  /** 全部缺陷条目（中文、确定性顺序：按计划步骤序 → 步骤内 DependsOn 声明序）。 */
  MissingPreconditions: string[];
}

/** 依赖存在性判定回调（graph 允许为 null，视作空图）。 */
type depLookup = (dep: string) => boolean;

/**
 * rehearse 静态预演：对 plan 的每个步骤逐一检查 DependsOn 是否可满足。
 * 确定性规则（与 Go 一致）：
 *   - 步骤 ID 计划内重复 → 后续出现记缺陷（首现参与判定）；
 *   - 依赖自身 → 缺陷；
 *   - 依赖计划内更晚步骤 → 缺陷（顺序倒置）；
 *   - 依赖更早但预演失败的步骤 → 缺陷（传递失败，逐层上报）；
 *   - 依赖既非计划步骤也非图节点 → 缺陷（前提缺失）。
 */
export function rehearse(plan: Plan, graph: StateGraph | null): RehearsalReport {
  const report: RehearsalReport = { Pass: true, MissingPreconditions: [] };
  const steps: PlanStep[] = plan.Steps ?? [];
  const firstIndex = new Map<string, number>();
  steps.forEach((s, i) => {
    if (!firstIndex.has(s.ID)) {
      firstIndex.set(s.ID, i);
    }
  });
  const executable = new Map<string, boolean>();
  const hasNode: depLookup = graph
    ? (dep) => graph.node(dep) !== undefined
    : () => false;

  steps.forEach((step, i) => {
    const first = firstIndex.get(step.ID)!;
    if (first !== i) {
      report.MissingPreconditions.push(
        `步骤 ${step.ID}（第 ${i + 1} 位）与第 ${first + 1} 位步骤 ID 重复，重复步骤不参与预演`,
      );
      return;
    }
    let ok = true;
    for (const dep of step.DependsOn ?? []) {
      if (dep === step.ID) {
        ok = false;
        report.MissingPreconditions.push(`步骤 ${step.ID} 依赖自身（自依赖不可满足）`);
        continue;
      }
      const j = firstIndex.get(dep);
      if (j !== undefined) {
        if (j > i) {
          ok = false;
          report.MissingPreconditions.push(
            `步骤 ${step.ID} 依赖其后的步骤 ${dep}（顺序倒置，当前序位不可能已执行）`,
          );
        } else if (!executable.get(dep)) {
          ok = false;
          report.MissingPreconditions.push(`步骤 ${step.ID} 依赖的步骤 ${dep} 预演未通过（传递失败）`);
        }
      } else if (!hasNode(dep)) {
        ok = false;
        report.MissingPreconditions.push(
          `步骤 ${step.ID} 缺少可执行前提：${dep}（既非计划内前序步骤，也不在状态图中）`,
        );
      }
    }
    executable.set(step.ID, ok);
  });

  report.Pass = report.MissingPreconditions.length === 0;
  return report;
}
