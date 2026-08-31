/**
 * backdiff.ts — 回溯差异（行动后校验：计划图 vs 实际轨迹图）
 *
 * 与 Go 端 internal/agent/worldmodel/backdiff.go 逐语义对齐（矩阵 #1 对等承诺）：
 * ComparePaths 为纯函数，相同输入必得相同输出（确定性可测试）。
 * 跨语言对账门：agentprimordia/internal/agent/worldmodel/testdata/worldmodel_fixtures.json
 * （Go 为权威生成方），由 src/agent/__tests__/worldmodel.test.ts 逐项断言。
 */

/** 计划步骤——计划图的最小单位（ID 与工具调用节点 ID 同一确定性 ID 空间）。 */
export interface PlanStep {
  /** 步骤 ID（计划内唯一；空则由 tracker 按序派生 step-N） */
  ID: string;
  /** 步骤摘要（建议形如 "工具名 输入摘要"） */
  Summary: string;
  /** 可执行前提：前序步骤 ID 或状态图既有节点 ID */
  DependsOn: string[] | null;
}

/** 计划图：Goal 之外仅保留有序步骤（预演门与回溯差异的共同输入）。 */
export interface Plan {
  Goal: string;
  Steps: PlanStep[] | null;
}

/** 计划步骤 ID 序列（即计划路径，供 ComparePaths 使用）。 */
export function planPath(p: Plan): string[] {
  const ids: string[] = [];
  for (const s of p.Steps ?? []) {
    ids.push(s.ID);
  }
  return ids;
}

/** 回溯差异结果——全部字段确定性可复算。 */
export interface BackDiff {
  /** 实际轨迹首次离开计划路径处的锚点 ID；完全一致时为 "" */
  DivergedAt: string;
  /** 计划中、但未在实际轨迹全长任何位置出现的步骤 ID（保持计划序） */
  PlannedButSkipped: string[] | null;
  /** 实际执行、但不在计划全长任何位置出现的节点 ID（保持实际序） */
  ExecutedButUnplanned: string[] | null;
}

/**
 * ComparePaths 对比计划路径与实际轨迹路径（纯函数，确定性）：
 *  1. 求最长公共前缀长度 p；
 *  2. DivergedAt 锚点：两序列在 p 处均有剩余 → planned[p]；仅计划有剩余 →
 *     planned[p]；仅实际有剩余 → actual[p]；完全一致 → ""；
 *  3. PlannedButSkipped = 计划 p 之后、未在实际全长出现的 ID（保持计划序）；
 *  4. ExecutedButUnplanned = 实际 p 之后、未在计划全长出现的 ID（保持实际序）。
 * 重复节点按「全长出现过」判定，不重复计入两列表——重复本身由锚点暴露。
 */
export function comparePaths(planned: string[], actual: string[]): BackDiff {
  let p = 0;
  while (p < planned.length && p < actual.length && planned[p] === actual[p]) {
    p++;
  }
  const diff: BackDiff = { DivergedAt: '', PlannedButSkipped: null, ExecutedButUnplanned: null };
  if (p === planned.length && p === actual.length) {
    diff.DivergedAt = '';
  } else if (p < planned.length && p < actual.length) {
    diff.DivergedAt = planned[p];
  } else if (p < planned.length) {
    diff.DivergedAt = planned[p];
  } else {
    diff.DivergedAt = actual[p];
  }
  diff.PlannedButSkipped = missingFrom(planned.slice(p), actual);
  diff.ExecutedButUnplanned = missingFrom(actual.slice(p), planned);
  return diff;
}

/** missingFrom 返回 seq 中未在 other 任何位置出现的元素（保持 seq 原序）。 */
function missingFrom(seq: string[], other: string[]): string[] | null {
  const out: string[] = [];
  for (const v of seq) {
    if (!other.includes(v)) {
      out.push(v);
    }
  }
  return out.length > 0 ? out : null;
}
