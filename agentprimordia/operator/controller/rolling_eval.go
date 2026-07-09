// rolling_eval.go — 基于 Eval 结果的自动滚动发布决策（G2-4 扩展）
//
// 在 operator/rolling.go 的滚动升级策略（MaxUnavailable/MaxSurge/preStop）
// 基础上，增加"灰度发布 + Eval 门禁"的决策逻辑：先灰度部分流量，等待
// Eval 套件结果，通过则继续滚动 / 失败则自动回滚。
//
// 决策逻辑被提取为纯函数，便于单元测试，不依赖 controller-runtime。
package controller

import "fmt"

// RolloutAction 灰度决策动作。
type RolloutAction string

const (
	// ActionPromote 继续/完成滚动发布（Eval 通过）。
	ActionPromote RolloutAction = "promote"
	// ActionRollback 回滚到稳定版本（Eval 失败）。
	ActionRollback RolloutAction = "rollback"
	// ActionHold 保持现状（Eval 未成功运行，无法决策）。
	ActionHold RolloutAction = "hold"
)

// EvalResult 一次 Eval 运行的结果。
type EvalResult struct {
	// RanOK 是否成功运行（false 表示 Eval 自身出错，不应据此决策）
	RanOK bool
	// PassRate 通过率 [0,1]
	PassRate float64
	// Threshold 通过率阈值（>= 视为通过）
	Threshold float64
}

// RolloutDecision 灰度决策结果。
type RolloutDecision struct {
	Action        RolloutAction
	Reason        string
	CanaryPercent int
}

// DecideRollout 根据 Eval 结果决定灰度动作。
//
// 规则：
//   - Eval 未成功运行 → Hold（不贸然发布或回滚）。
//   - PassRate >= Threshold → Promote。
//   - 否则 → Rollback。
func DecideRollout(canaryPercent int, eval EvalResult) RolloutDecision {
	if !eval.RanOK {
		return RolloutDecision{
			Action:        ActionHold,
			Reason:        "Eval 未成功运行，保持现状等待下一次评估",
			CanaryPercent: canaryPercent,
		}
	}
	if eval.PassRate >= eval.Threshold {
		return RolloutDecision{
			Action:        ActionPromote,
			Reason:        fmt.Sprintf("Eval 通过率 %.2f ≥ 阈值 %.2f，继续滚动发布", eval.PassRate, eval.Threshold),
			CanaryPercent: canaryPercent,
		}
	}
	return RolloutDecision{
		Action:        ActionRollback,
		Reason:        fmt.Sprintf("Eval 通过率 %.2f < 阈值 %.2f，自动回滚", eval.PassRate, eval.Threshold),
		CanaryPercent: canaryPercent,
	}
}

// RollingUpdateWithEval 模拟一次完整灰度发布轮次：
// 先灰度 canaryPercent 流量，再依据 Eval 结果决定 promote / rollback。
//
// 返回决策；真实实现（operator/agent_controller.go 的 Reconcile 循环）应据此
// 调用 K8s API 调整 Deployment 副本与流量权重。
func RollingUpdateWithEval(agentName string, canaryPercent int, eval EvalResult) (RolloutDecision, error) {
	if canaryPercent <= 0 || canaryPercent > 100 {
		return RolloutDecision{}, fmt.Errorf("rolling: canaryPercent %d 超出 (0,100]", canaryPercent)
	}
	_ = agentName
	return DecideRollout(canaryPercent, eval), nil
}
