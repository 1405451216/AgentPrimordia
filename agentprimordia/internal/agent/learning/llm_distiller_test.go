// llm_distiller_test.go — LLM 知识蒸馏器测试
package learning

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockLLMProvider 模拟 LLM 提供者
type mockLLMProvider struct {
	responses []string
	callCount int
	err       error
}

func (m *mockLLMProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.callCount >= len(m.responses) {
		return `{"knowledge": [], "summary": "no more responses"}`, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

// TestLLMDistiller_DistillWithLLM 测试 LLM 知识蒸馏
func TestLLMDistiller_DistillWithLLM(t *testing.T) {
	mock := &mockLLMProvider{
		responses: []string{
			`{
				"knowledge": [
					{
						"category": "fact",
						"pattern": "Go 是一种静态类型编程语言",
						"context": "编程语言讨论",
						"confidence": 0.95
					},
					{
						"category": "skill",
						"pattern": "使用 go test 运行测试",
						"context": "Go 开发",
						"confidence": 0.9
					},
					{
						"category": "preference",
						"pattern": "用户偏好简洁的代码风格",
						"context": "代码审查",
						"confidence": 0.4
					}
				],
				"summary": "提取了 3 条知识"
			}`,
		},
	}

	distiller := NewLLMDistiller(LLMDistillerConfig{
		Provider:      mock,
		MinConfidence: 0.5,
	})

	interactions := []Interaction{
		{
			ID:          "inter_1",
			UserInput:   "什么是 Go 语言？",
			AgentOutput: "Go 是一种静态类型编程语言，由 Google 开发。使用 go test 可以运行测试。",
			Success:     true,
			Timestamp:   time.Now(),
		},
	}

	items, err := distiller.DistillWithLLM(context.Background(), interactions)
	if err != nil {
		t.Fatalf("DistillWithLLM failed: %v", err)
	}

	// 应该过滤掉置信度 < 0.5 的条目（preference 的 0.4）
	if len(items) != 2 {
		t.Errorf("expected 2 items (filtered low confidence), got %d", len(items))
	}

	// 验证第一条知识
	if len(items) > 0 {
		if items[0].Category != "fact" {
			t.Errorf("first item category = %q, want 'fact'", items[0].Category)
		}
		if items[0].Confidence != 0.95 {
			t.Errorf("first item confidence = %f, want 0.95", items[0].Confidence)
		}
	}

	// 验证统计
	stats := distiller.GetStats()
	if stats.TotalInteractions != 1 {
		t.Errorf("TotalInteractions = %d, want 1", stats.TotalInteractions)
	}
	if stats.TotalLLMCalls != 1 {
		t.Errorf("TotalLLMCalls = %d, want 1", stats.TotalLLMCalls)
	}
	if stats.TotalDistilled != 2 {
		t.Errorf("TotalDistilled = %d, want 2", stats.TotalDistilled)
	}
}

// TestLLMDistiller_EmptyInteractions 测试空交互
func TestLLMDistiller_EmptyInteractions(t *testing.T) {
	mock := &mockLLMProvider{}
	distiller := NewLLMDistiller(LLMDistillerConfig{Provider: mock})

	items, err := distiller.DistillWithLLM(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items for empty interactions, got %d", len(items))
	}
}

// TestLLMDistiller_LLMError 测试 LLM 调用失败
func TestLLMDistiller_LLMError(t *testing.T) {
	mock := &mockLLMProvider{
		err: fmt.Errorf("LLM service unavailable"),
	}
	distiller := NewLLMDistiller(LLMDistillerConfig{Provider: mock})

	interactions := []Interaction{
		{ID: "test", UserInput: "hello", AgentOutput: "hi", Success: true},
	}

	_, err := distiller.DistillWithLLM(context.Background(), interactions)
	if err == nil {
		t.Fatal("expected error for LLM failure")
	}

	stats := distiller.GetStats()
	if stats.TotalErrors != 1 {
		t.Errorf("TotalErrors = %d, want 1", stats.TotalErrors)
	}
}

// TestLLMDistiller_InvalidJSON 测试 LLM 返回无效 JSON
func TestLLMDistiller_InvalidJSON(t *testing.T) {
	mock := &mockLLMProvider{
		responses: []string{"This is not JSON at all"},
	}
	distiller := NewLLMDistiller(LLMDistillerConfig{Provider: mock})

	interactions := []Interaction{
		{ID: "test", UserInput: "hello", AgentOutput: "hi", Success: true},
	}

	_, err := distiller.DistillWithLLM(context.Background(), interactions)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

// TestLLMDistiller_MarkdownWrappedJSON 测试 markdown 包裹的 JSON
func TestLLMDistiller_MarkdownWrappedJSON(t *testing.T) {
	mock := &mockLLMProvider{
		responses: []string{
			"Here is the extraction:\n```json\n{\"knowledge\": [{\"category\": \"fact\", \"pattern\": \"test\", \"context\": \"ctx\", \"confidence\": 0.8}], \"summary\": \"ok\"}\n```",
		},
	}
	distiller := NewLLMDistiller(LLMDistillerConfig{
		Provider:      mock,
		MinConfidence: 0.5,
	})

	interactions := []Interaction{
		{ID: "test", UserInput: "hello", AgentOutput: "world", Success: true},
	}

	items, err := distiller.DistillWithLLM(context.Background(), interactions)
	if err != nil {
		t.Fatalf("DistillWithLLM failed: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

// TestLLMDistiller_BatchSizeLimit 测试批次大小限制
func TestLLMDistiller_BatchSizeLimit(t *testing.T) {
	mock := &mockLLMProvider{
		responses: []string{
			`{"knowledge": [{"category": "fact", "pattern": "batch test", "context": "ctx", "confidence": 0.9}], "summary": "ok"}`,
		},
	}
	distiller := NewLLMDistiller(LLMDistillerConfig{
		Provider:     mock,
		MaxBatchSize: 2,
	})

	// 传入 5 个交互，应被截断为 2 个
	interactions := make([]Interaction, 5)
	for i := range interactions {
		interactions[i] = Interaction{
			ID:          fmt.Sprintf("inter_%d", i),
			UserInput:   "question",
			AgentOutput: "answer",
			Success:     true,
		}
	}

	_, err := distiller.DistillWithLLM(context.Background(), interactions)
	if err != nil {
		t.Fatalf("DistillWithLLM failed: %v", err)
	}

	// 统计应记录所有 5 个交互（即使只处理了 2 个）
	stats := distiller.GetStats()
	if stats.TotalInteractions != 2 {
		t.Errorf("TotalInteractions = %d, want 2 (batch limited)", stats.TotalInteractions)
	}
}

// TestLLMDistiller_GenerateCapabilityTests 测试能力测试生成
func TestLLMDistiller_GenerateCapabilityTests(t *testing.T) {
	mock := &mockLLMProvider{
		responses: []string{
			`[
				{"capability_name": "code_review", "test_input": "Review this function", "expected_output": "Identify bugs", "difficulty": "easy"},
				{"capability_name": "code_review", "test_input": "Analyze architecture", "expected_output": "Suggest improvements", "difficulty": "hard"}
			]`,
		},
	}
	distiller := NewLLMDistiller(LLMDistillerConfig{Provider: mock})

	tests, err := distiller.GenerateCapabilityTests(context.Background(), "code_review", "代码审查能力", 2)
	if err != nil {
		t.Fatalf("GenerateCapabilityTests failed: %v", err)
	}

	if len(tests) != 2 {
		t.Errorf("expected 2 tests, got %d", len(tests))
	}

	if len(tests) > 0 {
		if tests[0].CapabilityName != "code_review" {
			t.Errorf("test capability = %q, want 'code_review'", tests[0].CapabilityName)
		}
		if tests[0].Difficulty != "easy" {
			t.Errorf("test difficulty = %q, want 'easy'", tests[0].Difficulty)
		}
	}
}

// TestLLMDistiller_SearchKnowledge 测试知识搜索
func TestLLMDistiller_SearchKnowledge(t *testing.T) {
	mock := &mockLLMProvider{
		responses: []string{
			`{
				"knowledge": [
					{"category": "fact", "pattern": "Go uses garbage collection", "context": "programming", "confidence": 0.9},
					{"category": "skill", "pattern": "Use channels for concurrency", "context": "Go programming", "confidence": 0.85},
					{"category": "preference", "pattern": "User prefers tabs over spaces", "context": "code style", "confidence": 0.8}
				],
				"summary": "3 items"
			}`,
		},
	}
	distiller := NewLLMDistiller(LLMDistillerConfig{
		Provider:      mock,
		MinConfidence: 0.5,
	})

	interactions := []Interaction{
		{ID: "search_test", UserInput: "q", AgentOutput: "a", Success: true},
	}
	_, _ = distiller.DistillWithLLM(context.Background(), interactions)

	// 按类别搜索
	facts := distiller.SearchKnowledge("fact", "")
	if len(facts) != 1 {
		t.Errorf("expected 1 fact, got %d", len(facts))
	}

	// 按关键词搜索
	goItems := distiller.SearchKnowledge("", "Go")
	if len(goItems) < 1 {
		t.Error("expected at least 1 item matching 'Go'")
	}

	// 无结果
	empty := distiller.SearchKnowledge("nonexistent", "")
	if len(empty) != 0 {
		t.Errorf("expected 0 items for nonexistent category, got %d", len(empty))
	}
}

// TestExtractJSON 测试 JSON 提取
func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantJSON bool
	}{
		{"pure JSON object", `{"key": "value"}`, true},
		{"pure JSON array", `[{"key": "value"}]`, true},
		{"markdown wrapped", "```json\n{\"key\": \"value\"}\n```", true},
		{"text before JSON", "Here is the result: {\"key\": \"value\"}", true},
		{"no JSON", "This is just text", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			hasJSON := result != ""
			if hasJSON != tt.wantJSON {
				t.Errorf("extractJSON(%q) = %q, wantJSON %v", tt.input, result, tt.wantJSON)
			}
		})
	}
}
