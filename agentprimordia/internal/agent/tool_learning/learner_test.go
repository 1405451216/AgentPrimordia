package tool_learning

import (
	"context"
	"testing"
)

// mockMemoryStore 模拟 MemoryStore
type mockMemoryStore struct {
	episodes []*Episode
}

func (m *mockMemoryStore) Add(ctx context.Context, episode *Episode) error {
	m.episodes = append(m.episodes, episode)
	return nil
}

// TestToolLearnerInterface 验证 ToolLearner 接口定义
func TestToolLearnerInterface(t *testing.T) {
	var _ ToolLearner = (*MemoryToolLearner)(nil)
}

// TestBestPracticeCreation 测试最佳实践创建
func TestBestPracticeCreation(t *testing.T) {
	practice := BestPractice{
		ToolName:    "search",
		Pattern:     "使用关键词搜索",
		Description: "使用精确的关键词进行搜索",
		SuccessRate: 0.85,
		Examples:    []string{"关键词1", "关键词2"},
	}

	if practice.ToolName != "search" {
		t.Errorf("Expected tool name 'search', got '%s'", practice.ToolName)
	}
	if practice.SuccessRate < 0 || practice.SuccessRate > 1 {
		t.Errorf("Success rate should be between 0 and 1, got %f", practice.SuccessRate)
	}
}

// TestSuggestionCreation 测试改进建议创建
func TestSuggestionCreation(t *testing.T) {
	suggestion := Suggestion{
		OriginalArgs: "原始参数",
		ImprovedArgs: "改进参数",
		Reason:       "基于历史经验",
		Confidence:   0.9,
	}

	if suggestion.OriginalArgs == suggestion.ImprovedArgs {
		t.Error("Original and improved args should be different")
	}
	if suggestion.Confidence < 0 || suggestion.Confidence > 1 {
		t.Errorf("Confidence should be between 0 and 1, got %f", suggestion.Confidence)
	}
}

// TestToolUsageRecordCreation 测试工具使用记录创建
func TestToolUsageRecordCreation(t *testing.T) {
	record := ToolUsageRecord{
		ToolName: "calculator",
		Args:     "2+2",
		Result:   "4",
		Success:  true,
	}

	if !record.Success {
		t.Error("Expected success to be true")
	}
	if record.ToolName != "calculator" {
		t.Errorf("Expected tool name 'calculator', got '%s'", record.ToolName)
	}
}

// TestMemoryToolLearnerRecordSuccess 测试记录成功使用
func TestMemoryToolLearnerRecordSuccess(t *testing.T) {
	mem := &mockMemoryStore{}
	learner := NewMemoryToolLearner(mem)

	err := learner.RecordSuccess(context.Background(), "search", "关键词", "搜索结果")
	if err != nil {
		t.Fatalf("RecordSuccess failed: %v", err)
	}

	if len(mem.episodes) != 1 {
		t.Errorf("Expected 1 episode, got %d", len(mem.episodes))
	}
}

// TestMemoryToolLearnerRecordFailure 测试记录失败使用
func TestMemoryToolLearnerRecordFailure(t *testing.T) {
	mem := &mockMemoryStore{}
	learner := NewMemoryToolLearner(mem)

	err := learner.RecordFailure(context.Background(), "search", "关键词", "网络错误")
	if err != nil {
		t.Fatalf("RecordFailure failed: %v", err)
	}

	if len(mem.episodes) != 1 {
		t.Errorf("Expected 1 episode, got %d", len(mem.episodes))
	}
}

// TestMemoryToolLearnerGetBestPractices 测试获取最佳实践
func TestMemoryToolLearnerGetBestPractices(t *testing.T) {
	mem := &mockMemoryStore{}
	learner := NewMemoryToolLearner(mem)

	practices, err := learner.GetBestPractices(context.Background(), "search")
	if err != nil {
		t.Fatalf("GetBestPractices failed: %v", err)
	}

	// 当前实现返回空列表
	if practices == nil {
		t.Error("Expected non-nil practices slice")
	}
}

// TestMemoryToolLearnerSuggestImprovement 测试改进建议
func TestMemoryToolLearnerSuggestImprovement(t *testing.T) {
	mem := &mockMemoryStore{}
	learner := NewMemoryToolLearner(mem)

	suggestion, err := learner.SuggestImprovement(context.Background(), "calculator", "3+3")
	if err != nil {
		t.Fatalf("SuggestImprovement failed: %v", err)
	}

	if suggestion == nil {
		t.Fatal("Expected suggestion, got nil")
	}
	if suggestion.OriginalArgs != "3+3" {
		t.Errorf("Expected original args '3+3', got '%s'", suggestion.OriginalArgs)
	}
	// 没有历史数据时，建议应该等于原始输入
	if suggestion.ImprovedArgs != "3+3" {
		t.Errorf("Expected improved args '3+3' (no history), got '%s'", suggestion.ImprovedArgs)
	}
}
