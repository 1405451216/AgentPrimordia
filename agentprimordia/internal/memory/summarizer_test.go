package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"agentprimordia/internal/llm"
)

func TestSummarizer_ExtractSummary(t *testing.T) {
	mockLLM := llm.NewMockLLM(t).WithResponse("这是一段关于代码重构的摘要\ntopics: 重构,代码质量")

	summarizer := NewSummarizer(mockLLM)
	result, err := summarizer.ExtractSummary(context.Background(), "我需要重构这段代码，提高可读性和性能")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestSummarizer_ExtractSummary_EmptyContent(t *testing.T) {
	mockLLM := llm.NewMockLLM(t).WithResponse("空内容")

	summarizer := NewSummarizer(mockLLM)
	result, err := summarizer.ExtractSummary(context.Background(), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary even for empty content")
	}
}

func TestSummarizer_ExtractSummary_Error(t *testing.T) {
	mockLLM := llm.NewMockLLM(t).WithError(errors.New("LLM unavailable"))

	summarizer := NewSummarizer(mockLLM)
	_, err := summarizer.ExtractSummary(context.Background(), "some content")

	if err == nil {
		t.Error("expected error when LLM fails")
	}
}

func TestSummarizer_WithModel(t *testing.T) {
	mockLLM := llm.NewMockLLM(t).WithResponse("摘要\ntopics: 测试")

	summarizer := NewSummarizer(mockLLM).WithModel("gpt-4o-mini")

	if summarizer.model != "gpt-4o-mini" {
		t.Errorf("model = %q, want %q", summarizer.model, "gpt-4o-mini")
	}

	result, err := summarizer.ExtractSummary(context.Background(), "测试内容")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestSQLiteStore_StartAutoCleanup(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	ep := &Episode{
		ID:        "old-1",
		SessionID: "s1",
		Role:      "user",
		Content:   "old content",
	}
	store.Add(ctx, ep)

	stop := store.StartAutoCleanup(CleanupConfig{
		MaxAgeDays:    0,
		Interval:      100 * time.Millisecond,
		PreserveRoles: []string{"tool"},
	})

	time.Sleep(300 * time.Millisecond)
	stop()

	count, _ := store.Count(ctx, "")
	if count != 0 {
		t.Errorf("expected 0 episodes after cleanup, got %d", count)
	}
}

func TestSQLiteStore_StartAutoCleanup_PreserveTool(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	toolEp := &Episode{
		ID:        "tool-1",
		SessionID: "s1",
		Role:      "tool",
		Content:   "tool output",
	}
	store.Add(ctx, toolEp)

	stop := store.StartAutoCleanup(CleanupConfig{
		MaxAgeDays:    0,
		Interval:      100 * time.Millisecond,
		PreserveRoles: []string{"tool"},
	})

	time.Sleep(300 * time.Millisecond)
	stop()

	count, _ := store.Count(ctx, "")
	if count != 1 {
		t.Errorf("expected 1 tool episode preserved, got %d", count)
	}
}

func TestSQLiteStore_ExtractSummaryAsync(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	ep := &Episode{
		ID:        "async-1",
		SessionID: "s1",
		Role:      "assistant",
		Content:   "这是一段需要摘要的内容",
	}
	store.Add(ctx, ep)

	mockLLM := llm.NewMockLLM(t).WithResponse("这是摘要内容\ntopics: 测试,摘要")
	summarizer := NewSummarizer(mockLLM)

	errCh := store.ExtractSummaryAsync(ctx, "async-1", summarizer)

	var asyncErr error
	select {
	case asyncErr = <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for async summary")
	}

	if asyncErr != nil {
		t.Fatalf("unexpected async error: %v", asyncErr)
	}

	updated, _ := store.Get(ctx, "async-1")
	if updated.Summary == "" {
		t.Error("expected summary to be updated after async extraction")
	}
}

func TestSQLiteStore_ExtractSummaryAsync_Error(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	ep := &Episode{
		ID:        "async-err-1",
		SessionID: "s1",
		Role:      "assistant",
		Content:   "content",
	}
	store.Add(ctx, ep)

	mockLLM := llm.NewMockLLM(t).WithError(errors.New("LLM failed"))
	summarizer := NewSummarizer(mockLLM)

	errCh := store.ExtractSummaryAsync(ctx, "async-err-1", summarizer)

	var asyncErr error
	select {
	case asyncErr = <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for async summary")
	}

	if asyncErr == nil {
		t.Error("expected error from async summary when LLM fails")
	}

	original, _ := store.Get(ctx, "async-err-1")
	if original.Summary != "" {
		t.Error("summary should not be updated when extraction fails")
	}
}

func TestCleanupConfig_Defaults(t *testing.T) {
	cfg := DefaultCleanupConfig()
	if cfg.MaxAgeDays != 30 {
		t.Errorf("MaxAgeDays = %d, want 30", cfg.MaxAgeDays)
	}
	if cfg.Interval != 24*time.Hour {
		t.Errorf("Interval = %v, want 24h", cfg.Interval)
	}
	if len(cfg.PreserveRoles) != 1 || cfg.PreserveRoles[0] != "tool" {
		t.Errorf("PreserveRoles = %v, want [tool]", cfg.PreserveRoles)
	}
}

func TestSummarizer_ParseSummaryResponse(t *testing.T) {
	tests := []struct {
		input   string
		wantSum string
		wantTop string
	}{
		{
			"这是摘要\ntopics: 重构,性能",
			"这是摘要",
			"重构,性能",
		},
		{
			"纯摘要没有标签",
			"纯摘要没有标签",
			"",
		},
		{
			"摘要行1\n摘要行2\ntopics: 标签1,标签2",
			"摘要行1\n摘要行2",
			"标签1,标签2",
		},
	}

	for _, tt := range tests {
		sum, top := parseSummaryResponse(tt.input)
		if strings.TrimSpace(sum) != tt.wantSum {
			t.Errorf("summary = %q, want %q", sum, tt.wantSum)
		}
		if top != tt.wantTop {
			t.Errorf("topics = %q, want %q", top, tt.wantTop)
		}
	}
}

func TestWindowSummaryStrategy_ShouldSummarize(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	strategy := NewWindowSummaryStrategy(5)

	should, err := strategy.ShouldSummarize(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if should {
		t.Error("should not summarize with 0 episodes")
	}

	for i := 0; i < 5; i++ {
		store.Add(ctx, &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: "s1",
			Role:      "user",
			Content:   fmt.Sprintf("content %d", i),
		})
	}

	should, err = strategy.ShouldSummarize(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !should {
		t.Error("should summarize with 5 episodes and window size 5")
	}
}

func TestWindowSummaryStrategy_SelectEpisodes(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		store.Add(ctx, &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: "s1",
			Role:      "user",
			Content:   fmt.Sprintf("content %d", i),
		})
	}

	strategy := NewWindowSummaryStrategy(3)
	episodes, err := strategy.SelectEpisodes(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(episodes) != 3 {
		t.Errorf("episodes count = %d, want 3", len(episodes))
	}
}

func TestWindowSummaryStrategy_RoleFilter(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	store.Add(ctx, &Episode{ID: "ep-1", SessionID: "s1", Role: "user", Content: "user msg"})
	store.Add(ctx, &Episode{ID: "ep-2", SessionID: "s1", Role: "assistant", Content: "assistant msg"})
	store.Add(ctx, &Episode{ID: "ep-3", SessionID: "s1", Role: "user", Content: "user msg 2"})

	strategy := &WindowSummaryStrategy{WindowSize: 5, MinEpisodes: 1, RoleFilter: "user"}
	episodes, err := strategy.SelectEpisodes(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(episodes) != 2 {
		t.Errorf("episodes count = %d, want 2 (user only)", len(episodes))
	}
}

func TestImportanceSummaryStrategy(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	store.Add(ctx, &Episode{ID: "ep-1", SessionID: "s1", Role: "user", Content: "low importance", Importance: 0.2})
	store.Add(ctx, &Episode{ID: "ep-2", SessionID: "s1", Role: "user", Content: "high importance", Importance: 0.9})
	store.Add(ctx, &Episode{ID: "ep-3", SessionID: "s1", Role: "user", Content: "medium importance", Importance: 0.7})

	strategy := NewImportanceSummaryStrategy(0.5)

	should, err := strategy.ShouldSummarize(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !should {
		t.Error("should summarize with 2 important episodes")
	}

	episodes, err := strategy.SelectEpisodes(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(episodes) != 2 {
		t.Errorf("episodes count = %d, want 2 (importance >= 0.5)", len(episodes))
	}
}

func TestSessionSummaryStrategy(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	strategy := NewSessionSummaryStrategy()

	should, err := strategy.ShouldSummarize(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if should {
		t.Error("should not summarize with 0 episodes")
	}

	for i := 0; i < 4; i++ {
		store.Add(ctx, &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: "s1",
			Role:      "user",
			Content:   fmt.Sprintf("content %d", i),
		})
	}

	should, err = strategy.ShouldSummarize(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !should {
		t.Error("should summarize with 4 episodes (>= 3 min)")
	}

	episodes, err := strategy.SelectEpisodes(ctx, store, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(episodes) != 4 {
		t.Errorf("episodes count = %d, want 4", len(episodes))
	}
}

func TestSummaryEngine_Run(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		store.Add(ctx, &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: "s1",
			Role:      "user",
			Content:   fmt.Sprintf("这是第%d条记忆内容", i+1),
		})
	}

	mockLLM := llm.NewMockLLM(t).WithResponse("这是会话摘要\ntopics: 记忆,会话")
	summarizer := NewSummarizer(mockLLM)
	strategy := NewWindowSummaryStrategy(5)

	engine := NewSummaryEngine(strategy, summarizer, store)
	result, err := engine.Run(ctx, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestSummaryEngine_RunAndStore(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		store.Add(ctx, &Episode{
			ID:        fmt.Sprintf("ep-%d", i),
			SessionID: "s1",
			Role:      "user",
			Content:   fmt.Sprintf("content %d", i),
		})
	}

	mockLLM := llm.NewMockLLM(t).WithResponse("摘要内容\ntopics: 测试")
	summarizer := NewSummarizer(mockLLM)
	strategy := NewWindowSummaryStrategy(5)

	engine := NewSummaryEngine(strategy, summarizer, store)
	result, err := engine.RunAndStore(ctx, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	count, _ := store.Count(ctx, "s1")
	if count != 6 {
		t.Errorf("count = %d, want 6 (5 original + 1 summary)", count)
	}
}

func TestSummaryEngine_NotNeeded(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	mockLLM := llm.NewMockLLM(t).WithResponse("should not be called")
	summarizer := NewSummarizer(mockLLM)
	strategy := NewWindowSummaryStrategy(10)

	engine := NewSummaryEngine(strategy, summarizer, store)
	result, err := engine.Run(ctx, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result when summarization not needed")
	}
}
