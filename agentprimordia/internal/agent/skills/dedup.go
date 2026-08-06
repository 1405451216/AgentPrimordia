package skills

// Deduplicator 技能去重与合并
type Deduplicator struct {
	// SimilarityThreshold 相似度阈值 [0, 1]（超过则视为重复）
	SimilarityThreshold float64
}

// NewDeduplicator 创建去重器
func NewDeduplicator(threshold float64) *Deduplicator {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.8
	}
	return &Deduplicator{SimilarityThreshold: threshold}
}

// FindDuplicates 在技能库中查找与候选技能相似的已有技能
func (d *Deduplicator) FindDuplicates(candidate *Skill, existing []*Skill) []*Skill {
	var duplicates []*Skill
	for _, s := range existing {
		if s.ID == candidate.ID {
			continue
		}
		sim := d.similarity(candidate, s)
		if sim >= d.SimilarityThreshold {
			duplicates = append(duplicates, s)
		}
	}
	return duplicates
}

// similarity 计算两个技能的相似度（基于名称/描述/步骤/标签）
func (d *Deduplicator) similarity(a *Skill, b *Skill) float64 {
	score := 0.0
	total := 0.0

	// 名称相似度
	total += 1.0
	if a.Name == b.Name {
		score += 1.0
	} else if tokenOverlap(a.Name, b.Name) > 0.5 {
		score += 0.5
	}

	// 步骤工具重叠度
	total += 1.0
	toolsA := toolSet(a)
	toolsB := toolSet(b)
	if len(toolsA) > 0 && len(toolsB) > 0 {
		overlap := 0
		for t := range toolsA {
			if toolsB[t] {
				overlap++
			}
		}
		maxLen := len(toolsA)
		if len(toolsB) > maxLen {
			maxLen = len(toolsB)
		}
		score += float64(overlap) / float64(maxLen)
	}

	// 标签重叠度
	tagsA := tagSet(a)
	tagsB := tagSet(b)
	if len(tagsA) > 0 || len(tagsB) > 0 {
		total += 1.0
		overlap := 0
		for t := range tagsA {
			if tagsB[t] {
				overlap++
			}
		}
		maxLen := len(tagsA)
		if len(tagsB) > maxLen {
			maxLen = len(tagsB)
		}
		if maxLen > 0 {
			score += float64(overlap) / float64(maxLen)
		}
	}

	if total == 0 {
		return 0
	}
	return score / total
}

func toolSet(s *Skill) map[string]bool {
	set := make(map[string]bool)
	for _, step := range s.Steps {
		if step.ToolName != "" {
			set[step.ToolName] = true
		}
	}
	return set
}

func tagSet(s *Skill) map[string]bool {
	set := make(map[string]bool)
	for _, t := range s.Tags {
		set[t] = true
	}
	return set
}

func tokenOverlap(a string, b string) float64 {
	if a == b {
		return 1.0
	}
	// 简化：字符级重叠
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	common := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			common++
		}
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	return float64(common) / float64(maxLen)
}
