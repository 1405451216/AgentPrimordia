// rehearsal.go — 预演门（行动前静态检查：计划中每个步骤是否在图上有可执行前提）
//
// 纯图算法、无 LLM、确定性输出（提案 §2.1：预演 gate 在工具执行前）。
// 本切片只提供检查结果，不拦截任何默认路径——如何处置由接入方决定
// （v6.1 opt-in：预演不过 → 接入方写失败库，见 options.go 接线点⑤）。
package worldmodel

import "fmt"

// RehearsalReport 预演结果。
type RehearsalReport struct {
	// Pass 全部步骤的可执行前提均满足时为 true（含空计划）。
	Pass bool
	// MissingPreconditions 全部缺陷条目（中文、确定性顺序：
	// 按计划步骤序 → 步骤内 DependsOn 声明序）；无缺陷时为 nil。
	MissingPreconditions []string
}

// Rehearse 静态预演：对 plan 的每个步骤逐一检查 DependsOn 是否可满足。
// 可满足 = 前提引用的是「计划内更早的、且自身可满足的步骤」，
// 或「状态图 graph 中已存在的节点」；graph 允许为 nil（视作空图）。
//
// 确定性规则（对抗用例覆盖：自依赖/前向依赖/环/重复步骤）：
//   - 步骤 ID 在计划内重复 → 第二次及以后出现记为缺陷（首现参与判定）；
//   - 依赖自身 → 缺陷（自依赖不可满足）；
//   - 依赖计划内更晚步骤 → 缺陷（顺序倒置：当前序位不可能已执行）；
//   - 依赖更早但预演失败的步骤 → 缺陷（传递失败，逐层上报）；
//   - 依赖既非计划步骤也非图节点 → 缺陷（前提缺失）。
//   - 判定优先级：计划内步骤 → 图节点；两者皆非才报缺失。
func Rehearse(plan Plan, graph *StateGraph) RehearsalReport {
	report := RehearsalReport{}
	// 全计划步骤首现索引：支持「依赖更晚步骤」的前向判定
	firstIndex := make(map[string]int, len(plan.Steps))
	for i, s := range plan.Steps {
		if _, seen := firstIndex[s.ID]; !seen {
			firstIndex[s.ID] = i
		}
	}
	executable := make(map[string]bool, len(plan.Steps))
	for i, step := range plan.Steps {
		if first := firstIndex[step.ID]; first != i {
			// 重复步骤 ID：仅首现参与预演，后续出现记缺陷
			report.MissingPreconditions = append(report.MissingPreconditions,
				fmt.Sprintf("步骤 %s（第 %d 位）与第 %d 位步骤 ID 重复，重复步骤不参与预演", step.ID, i+1, first+1))
			continue
		}
		ok := true
		for _, dep := range step.DependsOn {
			switch {
			case dep == step.ID:
				ok = false
				report.MissingPreconditions = append(report.MissingPreconditions,
					fmt.Sprintf("步骤 %s 依赖自身（自依赖不可满足）", step.ID))
			default:
				// dep != step.ID，故命中 firstIndex 时必有 j != i
				j, inPlan := firstIndex[dep]
				if inPlan {
					if j > i {
						ok = false
						report.MissingPreconditions = append(report.MissingPreconditions,
							fmt.Sprintf("步骤 %s 依赖其后的步骤 %s（顺序倒置，当前序位不可能已执行）", step.ID, dep))
					} else if !executable[dep] {
						ok = false
						report.MissingPreconditions = append(report.MissingPreconditions,
							fmt.Sprintf("步骤 %s 依赖的步骤 %s 预演未通过（传递失败）", step.ID, dep))
					}
				} else if graph == nil {
					ok = false
					report.MissingPreconditions = append(report.MissingPreconditions,
						fmt.Sprintf("步骤 %s 缺少可执行前提：%s（既非计划内前序步骤，也不在状态图中）", step.ID, dep))
				} else if _, onGraph := graph.Node(dep); !onGraph {
					ok = false
					report.MissingPreconditions = append(report.MissingPreconditions,
						fmt.Sprintf("步骤 %s 缺少可执行前提：%s（既非计划内前序步骤，也不在状态图中）", step.ID, dep))
				}
			}
		}
		executable[step.ID] = ok
	}
	report.Pass = len(report.MissingPreconditions) == 0
	return report
}
