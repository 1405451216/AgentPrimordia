package memory

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestImportanceScorer_New(t *testing.T) {
	s := NewImportanceScorer()
	if s.WeightRecency != 0.25 || s.WeightFrequency != 0.25 || s.WeightRelevance != 0.30 || s.WeightEmotional != 0.20 {
		t.Errorf("unexpected default weights: %+v", s)
	}
}

func TestImportanceScorer_ScoreRecency(t *testing.T) {
	s := NewImportanceScorer()

	// 刚创建的 Episode：Recency 应接近 1
	ep1, _ := NewEpisode("sess1", "user", "hello")
	score1 := s.Score(context.Background(), ep1, nil)
	if score1.Recency < 0.99 {
		t.Errorf("fresh episode recency = %f, want >= 0.99", score1.Recency)
	}
	if score1.Recency > 1.0 {
		t.Errorf("recency = %f, want <= 1.0", score1.Recency)
	}

	// 2 小时前创建的 Episode
	oldEp, _ := NewEpisode("sess1", "user", "old message")
	oldCreatedAt := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	oldEp.CreatedAt = oldCreatedAt
	score2 := s.Score(context.Background(), oldEp, nil)

	// 2 小时 = 7200 秒，exp(-0.0001 * 7200) = exp(-0.72) ≈ 0.487
	expected := math.Exp(-0.0001 * 7200)
	if math.Abs(score2.Recency-expected) > 0.01 {
		t.Errorf("2-hour-old recency = %f, want ~%f", score2.Recency, expected)
	}

	if score1.Recency <= score2.Recency {
		t.Error("fresh episode should have higher recency than old episode")
	}
}

func TestImportanceScorer_ScoreEmotional(t *testing.T) {
	s := NewImportanceScorer()

	tests := []struct {
		name     string
		metadata map[string]string
		wantMin  float64
		wantMax  float64
	}{
		{"high emotional", map[string]string{"emotional": "high"}, 1.0, 1.0},
		{"medium emotional", map[string]string{"emotional": "medium"}, 0.5, 0.5},
		{"low emotional", map[string]string{"emotional": "low"}, 0.1, 0.1},
		{"success", map[string]string{"success": "true"}, 0.8, 0.8},
		{"failure", map[string]string{"failure": "true"}, 0.7, 0.7},
		{"error", map[string]string{"error": "some error"}, 0.7, 0.7},
		{"no metadata", nil, 0.0, 0.0},
		{"empty metadata", map[string]string{}, 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, _ := NewEpisode("s1", "user", "test")
			ep.Metadata = tt.metadata
			score := s.Score(context.Background(), ep, nil)
			if score.Emotional < tt.wantMin || score.Emotional > tt.wantMax {
				t.Errorf("Emotional = %f, want [%f, %f]", score.Emotional, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestImportanceScorer_ScoreRelevance(t *testing.T) {
	s := NewImportanceScorer()

	state := &AgentState{
		RecentKeywords: []string{"weather", "beach", "vacation"},
	}

	ep, _ := NewEpisode("s1", "user", "beach vacation is great")
	ep.Topics = "travel"
	score := s.Score(context.Background(), ep, state)

	// beach 和 vacation 匹配 → intersection=2, union≈4 → jaccard≈0.5
	if score.Relevance <= 0 || score.Relevance > 1 {
		t.Errorf("Relevance = %f, want (0, 1]", score.Relevance)
	}

	// 与当前状态无关的 Episode
	ep2, _ := NewEpisode("s1", "user", "machine learning algorithms")
	score2 := s.Score(context.Background(), ep2, state)
	if score2.Relevance >= score.Relevance {
		t.Logf("Note: irrelevant episode relevance=%f vs relevant=%f", score2.Relevance, score.Relevance)
	}
}

func TestImportanceScorer_ScoreRelevance_NilState(t *testing.T) {
	s := NewImportanceScorer()
	ep, _ := NewEpisode("s1", "user", "test content")
	score := s.Score(context.Background(), ep, nil)
	if score.Relevance != 0 {
		t.Errorf("Relevance with nil state = %f, want 0", score.Relevance)
	}
}

func TestImportanceScorer_Total(t *testing.T) {
	s := NewImportanceScorer()

	// 高情感 + 高相关性的 Episode 总分应高于低情感 + 低相关性的
	ep1, _ := NewEpisode("s1", "user", "beach vacation is wonderful")
	ep1.Topics = "travel beach"
	ep1.Metadata = map[string]string{"emotional": "high"}

	state := &AgentState{RecentKeywords: []string{"beach", "vacation"}}

	ep2, _ := NewEpisode("s1", "user", "machine learning algorithms")

	score1 := s.Score(context.Background(), ep1, state)
	score2 := s.Score(context.Background(), ep2, nil)

	if score1.Total <= score2.Total {
		t.Errorf("emotional+relevant total=%f should be > neutral total=%f", score1.Total, score2.Total)
	}
}

func TestImportanceScorer_Frequency(t *testing.T) {
	s := NewImportanceScorer()

	ep1, _ := NewEpisode("s1", "user", "test")
	ep1.Importance = 0.8
	score1 := s.Score(context.Background(), ep1, nil)

	ep2, _ := NewEpisode("s1", "user", "test")
	ep2.Importance = 0.0
	score2 := s.Score(context.Background(), ep2, nil)

	if score1.Frequency != 0.8 {
		t.Errorf("Frequency = %f, want 0.8", score1.Frequency)
	}
	if score2.Frequency != 0.0 {
		t.Errorf("Frequency = %f, want 0.0", score2.Frequency)
	}
}

func TestImportanceScorer_InvalidTimestamp(t *testing.T) {
	s := NewImportanceScorer()
	ep, _ := NewEpisode("s1", "user", "test")
	ep.CreatedAt = "not-a-timestamp"
	score := s.Score(context.Background(), ep, nil)
	if score.Recency != 0 {
		t.Errorf("invalid timestamp recency = %f, want 0", score.Recency)
	}
}

func TestImportanceScorer_TotalRange(t *testing.T) {
	s := NewImportanceScorer()
	ep, _ := NewEpisode("s1", "user", "test")
	ep.Metadata = map[string]string{"emotional": "high"}
	score := s.Score(context.Background(), ep, nil)

	if score.Total < 0 || score.Total > 1 {
		t.Errorf("Total = %f, want [0, 1]", score.Total)
	}
}
