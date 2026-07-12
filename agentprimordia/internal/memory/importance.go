// importance.go — 记忆重要度评分器（分层记忆深化的评分层）。
//
// 对 Episode 计算综合重要度得分，由四个维度加权：
//   - Recency：时间衰减（指数衰减 exp(-λΔt)）
//   - Frequency：归一化访问频率
//   - Relevance：与当前上下文的关键词重叠度
//   - Emotional：情感强度（成功/失败标记）

package memory

import (
	"context"
	"math"
	"strings"
	"time"
)

// decayLambda 是时间衰减系数（越大衰减越快）。
const decayLambda = 0.0001

// ImportanceScore 包含各维度得分和加权总分。
type ImportanceScore struct {
	Recency   float64
	Frequency float64
	Relevance float64
	Emotional float64
	Total     float64
}

// ImportanceScorer 是重要度评分器。
type ImportanceScorer struct {
	WeightRecency   float64
	WeightFrequency float64
	WeightRelevance float64
	WeightEmotional float64
}

// NewImportanceScorer 创建评分器并使用默认权重。
func NewImportanceScorer() *ImportanceScorer {
	return &ImportanceScorer{
		WeightRecency:   0.25,
		WeightFrequency: 0.25,
		WeightRelevance: 0.30,
		WeightEmotional: 0.20,
	}
}

// AgentState 表示当前 Agent 的上下文状态，用于相关性计算。
type AgentState struct {
	SessionID      string
	RecentKeywords []string
	CurrentTask    string
}

// Score 计算 Episode 的综合重要度。
func (s *ImportanceScorer) Score(ctx context.Context, mem *Episode, currentState *AgentState) ImportanceScore {
	recency := s.scoreRecency(mem)
	frequency := s.scoreFrequency(mem)
	relevance := s.scoreRelevance(mem, currentState)
	emotional := s.scoreEmotional(mem)

	total := recency*s.WeightRecency +
		frequency*s.WeightFrequency +
		relevance*s.WeightRelevance +
		emotional*s.WeightEmotional

	return ImportanceScore{
		Recency:   recency,
		Frequency: frequency,
		Relevance: relevance,
		Emotional: emotional,
		Total:     total,
	}
}

func (s *ImportanceScorer) scoreRecency(mem *Episode) float64 {
	if mem.CreatedAt == "" {
		return 0
	}
	createdAt, err := time.Parse(time.RFC3339, mem.CreatedAt)
	if err != nil {
		return 0
	}
	elapsed := time.Since(createdAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	return math.Exp(-decayLambda * elapsed)
}

func (s *ImportanceScorer) scoreFrequency(mem *Episode) float64 {
	if mem.Importance > 0 && mem.Importance <= 1 {
		return mem.Importance
	}
	return 0
}

func (s *ImportanceScorer) scoreRelevance(mem *Episode, currentState *AgentState) float64 {
	if currentState == nil || len(currentState.RecentKeywords) == 0 {
		return 0
	}

	epKeywords := extractKeywords(mem.Content + " " + mem.Summary + " " + mem.Topics)
	if len(epKeywords) == 0 {
		return 0
	}

	currentSet := make(map[string]struct{}, len(currentState.RecentKeywords))
	for _, kw := range currentState.RecentKeywords {
		currentSet[strings.ToLower(kw)] = struct{}{}
	}

	intersection := 0
	union := make(map[string]struct{}, len(epKeywords)+len(currentSet))
	for kw := range epKeywords {
		union[kw] = struct{}{}
		if _, ok := currentSet[kw]; ok {
			intersection++
		}
	}
	for kw := range currentSet {
		union[kw] = struct{}{}
	}

	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

func (s *ImportanceScorer) scoreEmotional(mem *Episode) float64 {
	if mem.Metadata == nil {
		return 0
	}

	if v, ok := mem.Metadata["emotional"]; ok {
		switch strings.ToLower(v) {
		case "high":
			return 1.0
		case "medium":
			return 0.5
		case "low":
			return 0.1
		}
	}

	if v, ok := mem.Metadata["success"]; ok && isTruthy(v) {
		return 0.8
	}
	if _, ok := mem.Metadata["failure"]; ok {
		return 0.7
	}
	if _, ok := mem.Metadata["error"]; ok {
		return 0.7
	}

	return 0
}

func extractKeywords(text string) map[string]struct{} {
	tokens := tokenizeRe.Split(strings.ToLower(text), -1)
	result := make(map[string]struct{}, len(tokens))
	for _, tok := range tokens {
		if tok != "" {
			result[tok] = struct{}{}
		}
	}
	return result
}

func isTruthy(s string) bool {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "success":
		return true
	}
	return false
}