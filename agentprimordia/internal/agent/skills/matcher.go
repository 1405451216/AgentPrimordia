package skills

// ConfidenceLevel 匹配置信度等级
type ConfidenceLevel string

const (
	// ConfidenceHigh 高置信度：自动调用
	ConfidenceHigh ConfidenceLevel = "high"
	// ConfidenceMedium 中置信度：建议调用（需确认）
	ConfidenceMedium ConfidenceLevel = "medium"
	// ConfidenceLow 低置信度：不匹配
	ConfidenceLow ConfidenceLevel = "low"
)

// MatchResult 匹配结果
type MatchResult struct {
	// Skill 匹配到的技能
	Skill *Skill
	// Score 匹配分数 [0, 1]
	Score float64
	// Confidence 置信度等级
	Confidence ConfidenceLevel
}

// MatcherConfig 匹配器配置
type MatcherConfig struct {
	// HighThreshold 高置信度阈值（默认 0.8）
	HighThreshold float64
	// MediumThreshold 中置信度阈值（默认 0.5）
	MediumThreshold float64
}

// Matcher 技能匹配器：任务描述 → 语义检索匹配技能
type Matcher struct {
	cfg   MatcherConfig
	store *Store
}

// NewMatcher 创建匹配器
func NewMatcher(store *Store, cfg MatcherConfig) *Matcher {
	if cfg.HighThreshold <= 0 {
		cfg.HighThreshold = 0.8
	}
	if cfg.MediumThreshold <= 0 {
		cfg.MediumThreshold = 0.5
	}
	return &Matcher{cfg: cfg, store: store}
}

// Match 为任务描述匹配最佳技能
func (m *Matcher) Match(taskDescription string) *MatchResult {
	active := m.store.ListActive()
	if len(active) == 0 {
		return nil
	}

	var best *MatchResult
	for _, skill := range active {
		score := m.score(skill, taskDescription)
		if best == nil || score > best.Score {
			confidence := m.classify(score)
			best = &MatchResult{Skill: skill, Score: score, Confidence: confidence}
		}
	}

	if best != nil && best.Confidence == ConfidenceLow {
		return nil // 低于阈值不返回
	}
	return best
}

// MatchAll 返回所有匹配结果（按分数降序）
func (m *Matcher) MatchAll(taskDescription string) []MatchResult {
	active := m.store.ListActive()
	var results []MatchResult
	for _, skill := range active {
		score := m.score(skill, taskDescription)
		confidence := m.classify(score)
		if confidence != ConfidenceLow {
			results = append(results, MatchResult{Skill: skill, Score: score, Confidence: confidence})
		}
	}
	// 简单排序
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	return results
}

// score 计算技能与任务描述的匹配分数
func (m *Matcher) score(skill *Skill, task string) float64 {
	score := 0.0
	factors := 0.0

	// 名称匹配
	factors++
	if contains(task, skill.Name) {
		score += 1.0
	} else if tokenOverlap(task, skill.Name) > 0.3 {
		score += 0.4
	}

	// 描述匹配
	factors++
	if tokenOverlap(task, skill.Description) > 0.3 {
		score += tokenOverlap(task, skill.Description)
	}

	// 标签匹配
	factors++
	tagHits := 0
	for _, tag := range skill.Tags {
		if contains(task, tag) {
			tagHits++
		}
	}
	if len(skill.Tags) > 0 {
		score += float64(tagHits) / float64(len(skill.Tags))
	}

	if factors == 0 {
		return 0
	}
	return score / factors
}

// classify 根据分数分类置信度
func (m *Matcher) classify(score float64) ConfidenceLevel {
	if score >= m.cfg.HighThreshold {
		return ConfidenceHigh
	}
	if score >= m.cfg.MediumThreshold {
		return ConfidenceMedium
	}
	return ConfidenceLow
}

func contains(s string, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s string, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
