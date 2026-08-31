// backdiff.go — 回溯差异（行动后校验：计划图 vs 实际轨迹图）
//
// ComparePaths 为纯函数：相同输入必得相同输出（确定性可测试）。
// 两条路径均以节点/步骤 ID 序列表示：
//   - 计划图路径 = Plan.Path()（计划步骤序）；
//   - 实际轨迹路径 = StateGraph.PathTo(最新节点)（根→最新事实的图路径）。
package worldmodel

// PlanStep 计划步骤——计划图的最小单位。
// ID 在计划内唯一（回溯差异与预演按此对齐）；Summary 建议与工具调用摘要
// 同格式（"工具名 输入"），使执行后与 tool_call 节点去重收敛到同一节点。
type PlanStep struct {
	ID        string   // 步骤 ID（计划内唯一；空则由 tracker 按序派生）
	Summary   string   // 步骤摘要（建议形如 "工具名 输入摘要"）
	DependsOn []string // 可执行前提：前序步骤 ID 或状态图既有节点 ID
}

// Plan 计划图：Goal 之外仅保留有序步骤——预演门（Rehearse）与
// 回溯差异（ComparePaths）的共同输入。
type Plan struct {
	Goal  string     // 计划目标（规范化后作 plan 节点摘要）
	Steps []PlanStep // 有序步骤
}

// Path 返回计划步骤 ID 序列（即计划路径，供 ComparePaths 使用）。
func (p Plan) Path() []string {
	ids := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		ids[i] = s.ID
	}
	return ids
}

// BackDiff 回溯差异结果——全部字段确定性可复算。
type BackDiff struct {
	// DivergedAt 实际轨迹首次离开计划路径处的锚点 ID：
	//   - 计划与实际完全一致时为 ""；
	//   - 计划尚有剩余步骤时为首个未被实际遵循的计划步骤
	//     （乱序执行也在锚点处暴露，见对抗用例）；
	//   - 计划已走完而实际多走时为首个计划外实际步骤。
	DivergedAt string
	// PlannedButSkipped 计划中、但未在实际轨迹全长任何位置出现的
	// 步骤 ID（保持计划序；无则为 nil）。
	PlannedButSkipped []string
	// ExecutedButUnplanned 实际执行、但不在计划全长任何位置出现的
	// 动作/节点 ID（保持实际序；无则为 nil）。
	ExecutedButUnplanned []string
}

// ComparePaths 对比计划路径与实际轨迹路径，输出回溯差异（纯函数，确定性）：
//
//  1. 求最长公共前缀长度 p；
//  2. DivergedAt 锚点：
//     两序列在 p 处均有剩余 → planned[p]（乱序执行也在此暴露）；
//     仅计划有剩余（计划未走完）→ planned[p]；
//     仅实际有剩余（计划外追加）→ actual[p]；
//     完全一致 → ""；
//  3. PlannedButSkipped = 计划 p 之后、未在实际轨迹全长任何位置出现的 ID（保持计划序）；
//  4. ExecutedButUnplanned = 实际 p 之后、未在计划全长任何位置出现的 ID（保持实际序）。
//
// 重复节点（同 ID 重复执行）按「全长出现过」判定，不重复计入两列表——
// 重复本身由 DivergedAt 锚点暴露（见对抗用例 TestComparePaths）。
func ComparePaths(planned, actual []string) BackDiff {
	p := 0
	for p < len(planned) && p < len(actual) && planned[p] == actual[p] {
		p++
	}
	diff := BackDiff{}
	switch {
	case p == len(planned) && p == len(actual):
		diff.DivergedAt = "" // 完全一致
	case p < len(planned) && p < len(actual):
		diff.DivergedAt = planned[p] // 首个偏离计划路径的计划步骤
	case p < len(planned):
		diff.DivergedAt = planned[p] // 计划未走完
	default:
		diff.DivergedAt = actual[p] // 计划走完后的计划外追加
	}
	// 集合判定用全长序列做成员检查：乱序执行的步骤不算「跳过」，
	// 计划内步骤的重复执行不算「计划外」——两者都由锚点统一暴露。
	diff.PlannedButSkipped = missingFrom(planned[p:], actual)
	diff.ExecutedButUnplanned = missingFrom(actual[p:], planned)
	return diff
}

// missingFrom 返回 seq 中未在 other 任何位置出现的元素（保持 seq 原序）。
func missingFrom(seq, other []string) []string {
	var out []string
	for _, v := range seq {
		if !containsString(other, v) {
			out = append(out, v)
		}
	}
	return out
}

// containsString 线性成员判定（避免引入 map 迭代序不确定性）。
func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
