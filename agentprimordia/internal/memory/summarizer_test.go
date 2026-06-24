package memory

import (
	"context"
	"fmt"
	"testing"
)

// mockSummarizerLLM 实现 SummarizerLLM 接口用于测试
type mockSummarizerLLM struct {
	response string
}

func (m *mockSummarizerLLM) Complete(ctx context.Context, messages []ChatMessageForSummary, model string) (string, error) {
	return m.response, nil
}

func TestSummarizer_ExtractSummary(t *testing.T) {
	mockResp := `这是一段测试内容的摘要
topics: 测试,摘要,提取`

	provider := &mockSummarizerLLM{response: mockResp}
	summarizer := NewSummarizer(provider)

	ctx := context.Background()
	content := "这是一段需要被摘要的测试内容，包含一些关键信息。"

	result, err := summarizer.ExtractSummary(ctx, content)
	if err != nil {
		t.Fatalf("ExtractSummary 失败: %v", err)
	}

	if result.Summary == "" {
		t.Error("摘要为空")
	}

	if result.Topics == "" {
		t.Error("主题为空")
	}

	t.Logf("摘要: %s", result.Summary)
	t.Logf("主题: %s", result.Topics)
}

func TestSummarizer_WithModel(t *testing.T) {
	provider := &mockSummarizerLLM{response: "测试摘要\ntopics: 测试"}
	summarizer := NewSummarizer(provider)
	summarizer.WithModel("gpt-3.5-turbo")

	if summarizer.model != "gpt-3.5-turbo" {
		t.Errorf("模型设置失败，期望: gpt-3.5-turbo, 实际: %s", summarizer.model)
	}
}

func TestParseSummaryResponse(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedSum    string
		expectedTopics string
	}{
		{
			name:           "标准格式",
			input:          "这是摘要\ntopics: 主题1,主题2",
			expectedSum:    "这是摘要",
			expectedTopics: "主题1,主题2",
		},
		{
			name:           "多行摘要",
			input:          "第一行摘要\n第二行补充\ntopics: 主题",
			expectedSum:    "第一行摘要\n第二行补充",
			expectedTopics: "主题",
		},
		{
			name:           "无主题",
			input:          "只有摘要",
			expectedSum:    "只有摘要",
			expectedTopics: "",
		},
		{
			name:           "大写 Topics",
			input:          "摘要内容\nTopics: 主题1,主题2",
			expectedSum:    "摘要内容",
			expectedTopics: "主题1,主题2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, topics := parseSummaryResponse(tt.input)
			if summary != tt.expectedSum {
				t.Errorf("摘要不匹配，期望: %q, 实际: %q", tt.expectedSum, summary)
			}
			if topics != tt.expectedTopics {
				t.Errorf("主题不匹配，期望: %q, 实际: %q", tt.expectedTopics, topics)
			}
		})
	}
}

func TestWindowSummaryStrategy_ShouldSummarize(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	sessionID := "test-session"

	// 添加 5 条记忆
	for i := 0; i < 5; i++ {
		ep := &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: sessionID,
			Role:      "user",
			Content:   fmt.Sprintf("内容 %d", i),
		}
		if err := store.Add(ctx, ep); err != nil {
			t.Fatalf("添加记忆失败: %v", err)
		}
	}

	strategy := NewWindowSummaryStrategy(5)

	should, err := strategy.ShouldSummarize(ctx, store, sessionID)
	if err != nil {
		t.Fatalf("ShouldSummarize 失败: %v", err)
	}

	if !should {
		t.Error("应该触发摘要，因为记忆数量达到窗口大小")
	}

	// 测试未达到窗口大小
	strategy2 := NewWindowSummaryStrategy(10)
	should2, err := strategy2.ShouldSummarize(ctx, store, sessionID)
	if err != nil {
		t.Fatalf("ShouldSummarize 失败: %v", err)
	}

	if should2 {
		t.Error("不应该触发摘要，因为记忆数量未达到窗口大小")
	}
}

func TestWindowSummaryStrategy_SelectEpisodes(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	sessionID := "test-session"

	// 添加 10 条记忆
	for i := 0; i < 10; i++ {
		ep := &Episode{
			ID:         fmt.Sprintf("ep-%d", i),
			SessionID:  sessionID,
			Role:       "user",
			Content:    fmt.Sprintf("内容 %d", i),
			Importance: float64(i) / 10.0,
		}
		if err := store.Add(ctx, ep); err != nil {
			t.Fatalf("添加记忆失败: %v", err)
		}
	}

	strategy := NewWindowSummaryStrategy(5)

	episodes, err := strategy.SelectEpisodes(ctx, store, sessionID)
	if err != nil {
		t.Fatalf("SelectEpisodes 失败: %v", err)
	}

	if len(episodes) != 5 {
		t.Errorf("期望选择 5 条记忆，实际选择 %d 条", len(episodes))
	}
}

func TestImportanceSummaryStrategy_ShouldSummarize(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	// 添加不同重要性的记忆
	for i := 0; i < 10; i++ {
		ep := &Episode{
			ID:         fmt.Sprintf("ep-%d", i),
			SessionID:  "test-session",
			Role:       "user",
			Content:    fmt.Sprintf("内容 %d", i),
			Importance: float64(i) / 10.0, // 0.0, 0.1, 0.2, ..., 0.9
		}
		if err := store.Add(ctx, ep); err != nil {
			t.Fatalf("添加记忆失败: %v", err)
		}
	}

	// 阈值 0.5，应该有 4 条记忆（0.5, 0.6, 0.7, 0.8, 0.9）
	strategy := NewImportanceSummaryStrategy(0.5)

	should, err := strategy.ShouldSummarize(ctx, store, "test-session")
	if err != nil {
		t.Fatalf("ShouldSummarize 失败: %v", err)
	}

	if !should {
		t.Error("应该触发摘要，因为有足够的高重要性记忆")
	}

	// 阈值 0.9，只有 1 条记忆
	strategy2 := NewImportanceSummaryStrategy(0.9)
	should2, err := strategy2.ShouldSummarize(ctx, store, "test-session")
	if err != nil {
		t.Fatalf("ShouldSummarize 失败: %v", err)
	}

	if should2 {
		t.Error("不应该触发摘要，因为高重要性记忆太少")
	}
}

func TestSessionSummaryStrategy_ShouldSummarize(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	sessionID := "test-session"

	// 添加 3 条记忆
	for i := 0; i < 3; i++ {
		ep := &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: sessionID,
			Role:      "user",
			Content:   fmt.Sprintf("内容 %d", i),
		}
		if err := store.Add(ctx, ep); err != nil {
			t.Fatalf("添加记忆失败: %v", err)
		}
	}

	strategy := NewSessionSummaryStrategy()

	should, err := strategy.ShouldSummarize(ctx, store, sessionID)
	if err != nil {
		t.Fatalf("ShouldSummarize 失败: %v", err)
	}

	if !should {
		t.Error("应该触发摘要，因为会话记忆数量达到最小值")
	}

	// 测试空 sessionID
	should2, err := strategy.ShouldSummarize(ctx, store, "")
	if err != nil {
		t.Fatalf("ShouldSummarize 失败: %v", err)
	}

	if should2 {
		t.Error("空 sessionID 不应该触发摘要")
	}
}

func TestSummaryEngine_Run(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	sessionID := "test-session"

	// 添加 5 条记忆
	for i := 0; i < 5; i++ {
		ep := &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: sessionID,
			Role:      "user",
			Content:   fmt.Sprintf("内容 %d", i),
		}
		if err := store.Add(ctx, ep); err != nil {
			t.Fatalf("添加记忆失败: %v", err)
		}
	}

	mockResp := "这是自动生成的摘要\ntopics: 测试,自动摘要"
	provider := &mockSummarizerLLM{response: mockResp}
	summarizer := NewSummarizer(provider)

	strategy := NewWindowSummaryStrategy(5)
	engine := NewSummaryEngine(strategy, summarizer, store)

	result, err := engine.Run(ctx, sessionID)
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	if result == nil {
		t.Fatal("结果不应为空")
	}

	if result.Summary == "" {
		t.Error("摘要为空")
	}

	if result.Topics == "" {
		t.Error("主题为空")
	}

	t.Logf("摘要: %s", result.Summary)
	t.Logf("主题: %s", result.Topics)
}

func TestSummaryEngine_RunAndStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	sessionID := "test-session"

	// 添加 5 条记忆
	for i := 0; i < 5; i++ {
		ep := &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: sessionID,
			Role:      "user",
			Content:   fmt.Sprintf("内容 %d", i),
		}
		if err := store.Add(ctx, ep); err != nil {
			t.Fatalf("添加记忆失败: %v", err)
		}
	}

	mockResp := "这是自动生成的摘要\ntopics: 测试,自动摘要"
	provider := &mockSummarizerLLM{response: mockResp}
	summarizer := NewSummarizer(provider)

	strategy := NewWindowSummaryStrategy(5)
	engine := NewSummaryEngine(strategy, summarizer, store)

	result, err := engine.RunAndStore(ctx, sessionID)
	if err != nil {
		t.Fatalf("RunAndStore 失败: %v", err)
	}

	if result == nil {
		t.Fatal("结果不应为空")
	}

	// 验证摘要已存储
	opts := &ListOptions{
		SessionID: sessionID,
	}
	episodes, err := store.List(ctx, opts)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}

	found := false
	for _, ep := range episodes {
		if ep.Metadata["type"] == "auto_summary" {
			found = true
			if ep.Summary != result.Summary {
				t.Errorf("存储的摘要不匹配")
			}
		}
	}

	if !found {
		t.Error("未找到自动生成的摘要")
	}
}
