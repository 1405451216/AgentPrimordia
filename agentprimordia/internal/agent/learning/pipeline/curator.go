// curator.go — 课程筛选（管道第二段：轨迹 → 训练候选）
//
// 确定性规则筛选（无 LLM、同输入必同输出）：
//   - 只收成功轨迹（失败轨迹的「怎么不做」留给失败库，不进权重）；
//   - 轮次下限（太浅的任务没有蒸馏价值）与上限（异常长轨迹通常是循环病）；
//   - 窄域过滤（命题 1 单域达标制：单域数据集）；
//   - 域内去重（同域同首条 user 输入视为重复任务）；
//   - 课程权重 = f(轮次适中度, token 效率)，[0,1] 确定性打分。
package pipeline

import (
	"fmt"
	"sort"
)

// CuratorConfig 课程筛选配置（零值 = 全默认）。
type CuratorConfig struct {
	// MinTurns / MaxTurns 轮次窗口（默认 1 / 40）
	MinTurns int
	MaxTurns int
	// AllowFailures 允许失败轨迹入课程（默认 false——失败轨迹蒸馏会教坏权重；
	// 仅显式开启并配人工复审时使用）
	AllowFailures bool // MaxPerRun 单轮筛选产出上限（0 = 不限；防单次异常采集淹没课程）
	MaxPerRun     int
}

// CuratedCandidate 筛选产出：样例 + 课程权重 + 入选理由（可审计）。
type CuratedCandidate struct {
	Trajectory Trajectory
	Weight     float64
	Reason     string
}

// Curate 执行课程筛选。domain 为目标窄域（空 = 不过滤域）。
// 确定性：候选按（权重降序, 轨迹 ID 升序）排序，MaxPerRun 截断。
func Curate(trajectories []Trajectory, domain string, cfg CuratorConfig) ([]CuratedCandidate, []string) {
	minTurns := cfg.MinTurns
	if minTurns <= 0 {
		minTurns = 1
	}
	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 40
	}
	var rejected []string
	seenTask := make(map[string]bool)
	var candidates []CuratedCandidate

	// 先按创建时间 + ID 稳定排序，保证去重命中次序确定
	ordered := make([]Trajectory, len(trajectories))
	copy(ordered, trajectories)
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})

	for _, t := range ordered {
		switch {
		case !cfg.AllowFailures && !t.Success:
			rejected = append(rejected, fmt.Sprintf("轨迹 %s：未成功（失败轨迹不进权重）", t.ID))
			continue
		case len(t.Turns) < minTurns:
			rejected = append(rejected, fmt.Sprintf("轨迹 %s：轮次 %d 低于下限 %d", t.ID, len(t.Turns), minTurns))
			continue
		case len(t.Turns) > maxTurns:
			rejected = append(rejected, fmt.Sprintf("轨迹 %s：轮次 %d 超上限 %d（疑似循环病）", t.ID, len(t.Turns), maxTurns))
			continue
		case domain != "" && t.Domain != domain:
			rejected = append(rejected, fmt.Sprintf("轨迹 %s：域 %s ≠ 目标域 %s", t.ID, t.Domain, domain))
			continue
		}
		taskKey := firstUserInput(t)
		if taskKey != "" && seenTask[taskKey] {
			rejected = append(rejected, fmt.Sprintf("轨迹 %s：域内重复任务（首条 user 输入重复）", t.ID))
			continue
		}
		if taskKey != "" {
			seenTask[taskKey] = true
		}
		w := courseWeight(t, minTurns, maxTurns)
		candidates = append(candidates, CuratedCandidate{
			Trajectory: t,
			Weight:     w,
			Reason:     fmt.Sprintf("域 %s，%d 轮，权重 %.3f", t.Domain, len(t.Turns), w),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Weight != candidates[j].Weight {
			return candidates[i].Weight > candidates[j].Weight
		}
		return candidates[i].Trajectory.ID < candidates[j].Trajectory.ID
	})
	if cfg.MaxPerRun > 0 && len(candidates) > cfg.MaxPerRun {
		for _, c := range candidates[cfg.MaxPerRun:] {
			rejected = append(rejected, fmt.Sprintf("轨迹 %s：超出单轮产出上限 %d", c.Trajectory.ID, cfg.MaxPerRun))
		}
		candidates = candidates[:cfg.MaxPerRun]
	}
	return candidates, rejected
}

// firstUserInput 提取首条 user 轮内容（域内任务去重键）。
func firstUserInput(t Trajectory) string {
	for _, tt := range t.Turns {
		if tt.Role == "user" {
			return tt.Content
		}
	}
	return ""
}

// courseWeight 课程权重（确定性）：
//   - 轮次适中度：以窗口中点为峰的三角函数式衰减；
//   - token 效率：轮均 token 越低越高（封顶 0.3 贡献）；
//   - 综合 ∈ (0,1]。
func courseWeight(t Trajectory, minTurns, maxTurns int) float64 {
	n := len(t.Turns)
	mid := float64(minTurns+maxTurns) / 2.0
	turnScore := 1.0
	if mid > 0 {
		turnScore = 1.0 - absFloat(float64(n)-mid)/mid
		if turnScore < 0 {
			turnScore = 0
		}
	}
	effScore := 0.3
	if n > 0 {
		per := float64(t.Tokens) / float64(n)
		if per <= 0 {
			effScore = 0.3
		} else if per < 500 {
			effScore = 0.3 - 0.3*(per/500)
		} else {
			effScore = 0
		}
	}
	w := 0.7*turnScore + effScore
	if w > 1 {
		w = 1
	}
	if w < 0 {
		w = 0
	}
	return w
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
