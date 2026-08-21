// ab_harness.go — v5.2 A/B 对照 harness：双策略同任务集可复现跑分。
//
// 量化验收（V6-ROADMAP §四）：三策略 A/B 跑分可复现——同任务集、确定性
// 引擎工厂，输出成功率 / 平均 token / 平均轮数对照报告（JSON 可序列化）。
package strategy

import (
	"context"
	"fmt"
)

// ABTask A/B 任务用例
type ABTask struct {
	Goal      string // 任务目标
	MaxTurns  int    // 可选轮数上限
}

// EngineFactory 引擎工厂：为第 i 个任务构造独立引擎（确定性/可复现由实现保证）
type EngineFactory func(taskIndex int) Engine

// ABStrategyStats 单策略统计
type ABStrategyStats struct {
	Strategy    string   `json:"strategy"`
	SuccessRate float64  `json:"success_rate"`
	AvgTokens   float64  `json:"avg_tokens"`
	AvgTurns    float64  `json:"avg_turns"`
	Failures    []string `json:"failures,omitempty"`
}

// ABReport 双策略对照报告
type ABReport struct {
	Tasks int             `json:"tasks"`
	A     ABStrategyStats `json:"a"`
	B     ABStrategyStats `json:"b"`
}

// ABCompare 同一任务集分别以策略 A、B 运行（每任务经工厂独立建引擎），返回对照报告。
// 成功判定：Run 无错误且 Output 非空。
func ABCompare(ctx context.Context, a, b Strategy, tasks []ABTask, newEngine EngineFactory) (*ABReport, error) {
	if len(tasks) == 0 {
		return nil, fmt.Errorf("strategy: A/B 任务集为空")
	}
	if newEngine == nil {
		return nil, fmt.Errorf("strategy: A/B 缺少引擎工厂")
	}
	rep := &ABReport{Tasks: len(tasks)}
	rep.A = abRun(ctx, a, tasks, newEngine)
	rep.B = abRun(ctx, b, tasks, newEngine)
	return rep, nil
}

func abRun(ctx context.Context, s Strategy, tasks []ABTask, newEngine EngineFactory) ABStrategyStats {
	st := ABStrategyStats{Strategy: s.Name()}
	for i, t := range tasks {
		res, err := s.Run(ctx, newEngine(i), Task{Goal: t.Goal, MaxTurns: t.MaxTurns})
		if err != nil || res.Output == "" {
			st.Failures = append(st.Failures, fmt.Sprintf("task#%d: %v", i, err))
			continue
		}
		st.SuccessRate++
		st.AvgTokens += float64(res.Usage.TotalTokens)
		st.AvgTurns += float64(res.Turns)
	}
	n := float64(len(tasks))
	st.SuccessRate /= n
	if succ := st.SuccessRate * n; succ > 0 {
		st.AvgTokens /= succ
		st.AvgTurns /= succ
	}
	return st
}
