// think_budget.go — v5.2 自适应思考深度：test-time compute 预算。
//
// 难题深想、易题浅想——预算由任务信号驱动：
//   - 复杂度信号（多步/并发/迁移/设计等关键词、目标长度）
//   - 历史失败信号（重试次数越多预算越充裕）
//
// 量化验收（V6-ROADMAP §四 任务 3）：同任务集成功率不降且成本下降
// ——简单任务被压到浅预算，复杂任务保留深预算。
package strategy

import "strings"

// ThinkBudget 思考预算
type ThinkBudget struct {
	MaxTurns       int // 最大推理轮数
	MaxCorrections int // 最大修正轮数
}

// TaskSignals 任务复杂度与历史信号
type TaskSignals struct {
	Goal          string // 目标文本
	HistoryLength int    // 既有历史消息数
	FailureCount  int    // 该任务此前失败次数
}

// 复杂度关键词：命中越多越「难」
var complexityKeywords = []string{
	"迁移", "架构", "设计", "分布式", "并发", "性能", "安全",
	"重构", "集成", "优化", "多阶段", "依赖", "事务", "兼容",
}

// AdaptiveBudget 按任务信号计算思考预算：
//   - 简单任务（无关键词命中、短目标、无失败史）→ 浅预算（省 token）
//   - 复杂任务 / 有失败史 → 深预算（保成功率）
func AdaptiveBudget(sig TaskSignals) ThinkBudget {
	complexity := 0
	lower := strings.ToLower(sig.Goal)
	for _, kw := range complexityKeywords {
		if strings.Contains(lower, kw) {
			complexity++
		}
	}
	if len(sig.Goal) > 200 {
		complexity++
	}
	if sig.HistoryLength > 10 {
		complexity++
	}

	switch {
	case complexity == 0 && sig.FailureCount == 0:
		return ThinkBudget{MaxTurns: 4, MaxCorrections: 0} // 浅：直出+不修正
	case complexity <= 2 && sig.FailureCount <= 1:
		return ThinkBudget{MaxTurns: 8, MaxCorrections: 1} // 中
	default:
		return ThinkBudget{MaxTurns: 16, MaxCorrections: 3} // 深
	}
}

// Apply 将预算写入任务（策略消费 MaxTurns；验证循环消费 MaxCorrections）
func (b ThinkBudget) Apply(t *Task, v *VerificationLoopStrategy) {
	t.MaxTurns = b.MaxTurns
	if v != nil {
		v.MaxCorrections = b.MaxCorrections
	}
}
