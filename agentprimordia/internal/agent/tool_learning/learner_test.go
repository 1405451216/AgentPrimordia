package tool_learning

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// mockMemoryStore 模拟 MemoryStore
type mockMemoryStore struct {
	episodes []*Episode
}

func (m *mockMemoryStore) Add(ctx context.Context, episode *Episode) error {
	m.episodes = append(m.episodes, episode)
	return nil
}

// Query 按 sessionID + metadata 过滤；mock 仅实现最简单的精确匹配。
// 返回时间倒序（最新在前）。
func (m *mockMemoryStore) Query(ctx context.Context, sessionID string, metadata map[string]string) ([]*Episode, error) {
	out := make([]*Episode, 0, len(m.episodes))
	// 倒序遍历以获得"最新在前"
	for i := len(m.episodes) - 1; i >= 0; i-- {
		ep := m.episodes[i]
		if sessionID != "" && ep.SessionID != sessionID {
			continue
		}
		if !metadataMatches(ep.Metadata, metadata) {
			continue
		}
		out = append(out, ep)
	}
	return out, nil
}

func metadataMatches(have, want map[string]string) bool {
	for k, v := range want {
		if have == nil {
			return false
		}
		if got, ok := have[k]; !ok || got != v {
			return false
		}
	}
	return true
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

// TestMemoryToolLearnerGetBestPractices_Empty 当没有历史记录时应返回空切片而非 nil
func TestMemoryToolLearnerGetBestPractices_Empty(t *testing.T) {
	mem := &mockMemoryStore{}
	learner := NewMemoryToolLearner(mem)

	practices, err := learner.GetBestPractices(context.Background(), "unknown_tool")
	if err != nil {
		t.Fatalf("GetBestPractices failed: %v", err)
	}
	if len(practices) != 0 {
		t.Errorf("Expected 0 practices, got %d", len(practices))
	}
}

// TestMemoryToolLearnerGetBestPractices_WithHistory 验证 BUG-04 修复：
// 给定一组成功/失败混合的历史记录，应聚合为单条 BestPractice 并包含统计信息
func TestMemoryToolLearnerGetBestPractices_WithHistory(t *testing.T) {
	mem := &mockMemoryStore{}
	// 注入 4 次成功 + 1 次失败（80% 成功率）
	for i := 0; i < 4; i++ {
		if err := mem.Add(context.Background(), &Episode{
			ID:        fmt.Sprintf("ok-%d", i),
			SessionID: "tool_learning",
			Role:      "tool_usage",
			Content:   buildRecordJSON("search", "query", "result", true, ""),
			Metadata:  map[string]string{"tool_name": "search", "success": "true"},
		}); err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}
	if err := mem.Add(context.Background(), &Episode{
		ID:        "fail-0",
		SessionID: "tool_learning",
		Role:      "tool_usage",
		Content:   buildRecordJSON("search", "bad query", "", false, "timeout"),
		Metadata:  map[string]string{"tool_name": "search", "success": "false"},
	}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	learner := NewMemoryToolLearner(mem)
	practices, err := learner.GetBestPractices(context.Background(), "search")
	if err != nil {
		t.Fatalf("GetBestPractices failed: %v", err)
	}
	if len(practices) != 1 {
		t.Fatalf("Expected 1 aggregated practice, got %d", len(practices))
	}
	p := practices[0]
	if p.ToolName != "search" {
		t.Errorf("Expected tool name 'search', got %q", p.ToolName)
	}
	// 4/5 = 0.8
	if p.SuccessRate < 0.79 || p.SuccessRate > 0.81 {
		t.Errorf("Expected success rate ~0.8, got %f", p.SuccessRate)
	}
	if len(p.Examples) == 0 {
		t.Error("Expected at least one example")
	}
	if len(p.Examples) > 5 {
		t.Errorf("Expected at most 5 examples, got %d", len(p.Examples))
	}
}

// TestMemoryToolLearnerGetBestPractices_DifferentTools 验证 metadata 过滤生效
// 不同 tool_name 的记录不应混入
func TestMemoryToolLearnerGetBestPractices_DifferentTools(t *testing.T) {
	mem := &mockMemoryStore{}
	// search 工具的成功记录
	_ = mem.Add(context.Background(), &Episode{
		ID:        "search-ok",
		SessionID: "tool_learning",
		Content:   buildRecordJSON("search", "q", "r", true, ""),
		Metadata:  map[string]string{"tool_name": "search", "success": "true"},
	})
	// calculator 工具的成功记录
	_ = mem.Add(context.Background(), &Episode{
		ID:        "calc-ok",
		SessionID: "tool_learning",
		Content:   buildRecordJSON("calculator", "2+2", "4", true, ""),
		Metadata:  map[string]string{"tool_name": "calculator", "success": "true"},
	})

	learner := NewMemoryToolLearner(mem)
	practices, err := learner.GetBestPractices(context.Background(), "search")
	if err != nil {
		t.Fatalf("GetBestPractices failed: %v", err)
	}
	if len(practices) != 1 {
		t.Fatalf("Expected 1 practice for search, got %d", len(practices))
	}
	// search 只有 1 条成功 → 100% 成功率
	if practices[0].SuccessRate != 1.0 {
		t.Errorf("Expected success rate 1.0 for search, got %f", practices[0].SuccessRate)
	}
}

// TestMemoryToolLearnerGetBestPractices_AllFailures 验证全失败场景
func TestMemoryToolLearnerGetBestPractices_AllFailures(t *testing.T) {
	mem := &mockMemoryStore{}
	for i := 0; i < 3; i++ {
		_ = mem.Add(context.Background(), &Episode{
			ID:        fmt.Sprintf("fail-%d", i),
			SessionID: "tool_learning",
			Content:   buildRecordJSON("flaky", "x", "", false, "boom"),
			Metadata:  map[string]string{"tool_name": "flaky", "success": "false"},
		})
	}
	learner := NewMemoryToolLearner(mem)
	practices, err := learner.GetBestPractices(context.Background(), "flaky")
	if err != nil {
		t.Fatalf("GetBestPractices failed: %v", err)
	}
	if len(practices) != 1 {
		t.Fatalf("Expected 1 practice, got %d", len(practices))
	}
	if practices[0].SuccessRate != 0.0 {
		t.Errorf("Expected success rate 0.0, got %f", practices[0].SuccessRate)
	}
	if len(practices[0].Examples) != 0 {
		t.Errorf("Expected 0 examples for all-failure case, got %d", len(practices[0].Examples))
	}
}

// buildRecordJSON 构造 ToolUsageRecord 的 JSON 序列化（测试辅助）
func buildRecordJSON(toolName, args, result string, success bool, errMsg string) string {
	rec := ToolUsageRecord{
		ToolName:  toolName,
		Args:      args,
		Result:    result,
		Error:     errMsg,
		Success:   success,
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(rec)
	return string(data)
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
