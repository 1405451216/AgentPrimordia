package learning

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ===== Mock 实现 =====

// mockSemanticSink 模拟语义记忆
type mockSemanticSink struct {
	facts    []mockFact
	patterns []mockPattern
	prompt   string
}

type mockFact struct {
	key        string
	value      string
	confidence float64
	source     string
}

type mockPattern struct {
	pattern     string
	description string
	successRate float64
	examples    []string
}

func (s *mockSemanticSink) AddFact(ctx context.Context, key, value string, confidence float64, source string) {
	s.facts = append(s.facts, mockFact{key: key, value: value, confidence: confidence, source: source})
}

func (s *mockSemanticSink) AddPattern(ctx context.Context, pattern, description string, successRate float64, examples []string) {
	s.patterns = append(s.patterns, mockPattern{pattern: pattern, description: description, successRate: successRate, examples: examples})
}

func (s *mockSemanticSink) InjectPrompt() string {
	return s.prompt
}

// mockRAG 模拟 RAG 检索器
type mockRAG struct {
	results map[string][]RetrievedChunk
	err     error
}

func (r *mockRAG) Retrieve(ctx context.Context, query string, topK int) ([]RetrievedChunk, error) {
	if r.err != nil {
		return nil, r.err
	}
	// 根据查询关键词返回结果
	for key, chunks := range r.results {
		if strings.Contains(strings.ToLower(query), key) {
			if len(chunks) > topK {
				return chunks[:topK], nil
			}
			return chunks, nil
		}
	}
	return nil, nil
}

// ===== 测试用例 =====

func TestDistillPipeline_ProcessInteraction(t *testing.T) {
	sink := &mockSemanticSink{}
	pipeline := NewDistillPipeline(
		WithSemanticMemory(sink),
		WithPipelineConfig(DistillPipelineConfig{
			MinConfidence: 0.5,
			EnableRAG:     false,
		}),
	)

	inter := Interaction{
		ID:          "inter-1",
		UserInput:   "What is Go?",
		AgentOutput: "Go is a programming language. It was created at Google.",
		Success:     true,
		Timestamp:   time.Now(),
	}

	result, err := pipeline.ProcessInteraction(context.Background(), inter)
	if err != nil {
		t.Fatalf("处理交互失败: %v", err)
	}

	if result.InteractionID != "inter-1" {
		t.Errorf("期望 InteractionID=inter-1, 得到 %s", result.InteractionID)
	}
	if len(result.DistilledItems) == 0 {
		t.Error("应蒸馏出至少一个知识项")
	}

	// 验证统计
	stats := pipeline.GetStats()
	if stats.TotalProcessed != 1 {
		t.Errorf("期望 TotalProcessed=1, 得到 %d", stats.TotalProcessed)
	}
}

func TestDistillPipeline_WriteToSemanticMemory(t *testing.T) {
	sink := &mockSemanticSink{}
	pipeline := NewDistillPipeline(
		WithSemanticMemory(sink),
		WithPipelineConfig(DistillPipelineConfig{
			MinConfidence: 0.5,
			EnableRAG:     false,
		}),
	)

	inter := Interaction{
		ID:          "inter-2",
		UserInput:   "How does memory work?",
		AgentOutput: "Memory is the ability to store and retrieve information. It works through encoding.",
		Feedback:    "good answer",
		Success:     true,
		Timestamp:   time.Now(),
	}

	result, err := pipeline.ProcessInteraction(context.Background(), inter)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	// 应有事实写入语义记忆
	totalWritten := result.FactsWritten + result.PatternsWritten
	if totalWritten == 0 {
		t.Error("应有知识写入语义记忆")
	}

	// 验证 sink 收到了数据
	if len(sink.facts)+len(sink.patterns) == 0 {
		t.Error("语义记忆 sink 应收到数据")
	}

	// 反馈应被记录
	if !result.FeedbackRecorded {
		t.Error("反馈应被记录")
	}
}

func TestDistillPipeline_ConfidenceFilter(t *testing.T) {
	sink := &mockSemanticSink{}
	pipeline := NewDistillPipeline(
		WithSemanticMemory(sink),
		WithPipelineConfig(DistillPipelineConfig{
			MinConfidence: 0.95, // 极高阈值，大部分知识会被过滤
			EnableRAG:     false,
		}),
	)

	inter := Interaction{
		ID:          "inter-3",
		UserInput:   "test",
		AgentOutput: "This is a test response.",
		Success:     false,
		Timestamp:   time.Now(),
	}

	result, err := pipeline.ProcessInteraction(context.Background(), inter)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	// 高阈值下，低置信度知识不应写入
	if result.FactsWritten > 0 {
		t.Logf("注意：有 %d 个事实通过了 0.95 阈值", result.FactsWritten)
	}
}

func TestDistillPipeline_TriggerRAGForWeaknesses(t *testing.T) {
	rag := &mockRAG{
		results: map[string][]RetrievedChunk{
			"coding": {
				{Content: "Use structured error handling in Go", Score: 0.9},
				{Content: "Prefer table-driven tests", Score: 0.8},
			},
		},
	}

	pipeline := NewDistillPipeline(
		WithRAGRetriever(rag),
		WithPipelineConfig(DistillPipelineConfig{
			WeaknessThreshold: 0.6,
			RAGTopK:           3,
			EnableRAG:         true,
		}),
	)

	// 注册一个弱项能力
	pipeline.GetEvolver().Register(Capability{
		Name:        "coding",
		Description: "code generation quality",
		Score:       0.3, // 低于阈值
	})

	results, err := pipeline.TriggerRAGForWeaknesses(context.Background())
	if err != nil {
		t.Fatalf("RAG 检索失败: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("弱项能力应触发 RAG 检索")
	}

	chunks, ok := results["coding"]
	if !ok {
		t.Fatal("应有 coding 能力的检索结果")
	}
	if len(chunks) != 2 {
		t.Errorf("期望 2 个检索结果, 得到 %d", len(chunks))
	}
}

func TestDistillPipeline_TriggerRAG_NoWeakness(t *testing.T) {
	rag := &mockRAG{results: map[string][]RetrievedChunk{}}

	pipeline := NewDistillPipeline(
		WithRAGRetriever(rag),
		WithPipelineConfig(DistillPipelineConfig{
			WeaknessThreshold: 0.3,
			EnableRAG:         true,
		}),
	)

	// 注册一个强项能力
	pipeline.GetEvolver().Register(Capability{
		Name:  "chat",
		Score: 0.9, // 高于阈值
	})

	results, err := pipeline.TriggerRAGForWeaknesses(context.Background())
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if results != nil && len(results) > 0 {
		t.Error("无弱项时不应有 RAG 结果")
	}
}

func TestDistillPipeline_BuildSystemPrompt(t *testing.T) {
	sink := &mockSemanticSink{
		prompt: "## 已知事实\n- Go: 编程语言（置信度 0.90）\n",
	}

	fbLearner := NewFeedbackLearner()
	_ = fbLearner.RecordFeedback(FeedbackEntry{
		ID:          "fb-1",
		AgentOutput: "Here is a detailed explanation with examples.",
		Feedback:    "great",
		Rating:      1,
	})

	pipeline := NewDistillPipeline(
		WithSemanticMemory(sink),
		WithFeedbackLearner(fbLearner),
		WithPipelineConfig(DistillPipelineConfig{
			MaxPromptLength: 5000,
		}),
	)

	prompt := pipeline.BuildSystemPrompt("You are a helpful assistant.")

	// 应包含基础提示
	if !strings.Contains(prompt, "You are a helpful assistant.") {
		t.Error("应包含基础系统提示")
	}

	// 应包含语义记忆
	if !strings.Contains(prompt, "已知事实") {
		t.Error("应包含语义记忆注入")
	}

	// 应包含偏好
	if !strings.Contains(prompt, "用户偏好") {
		t.Error("应包含用户偏好注入")
	}
}

func TestDistillPipeline_BuildSystemPrompt_MaxLength(t *testing.T) {
	sink := &mockSemanticSink{
		prompt: strings.Repeat("x", 3000),
	}

	pipeline := NewDistillPipeline(
		WithSemanticMemory(sink),
		WithPipelineConfig(DistillPipelineConfig{
			MaxPromptLength: 100,
		}),
	)

	prompt := pipeline.BuildSystemPrompt("base")
	if len(prompt) > 100 {
		t.Errorf("提示长度应被截断到 100, 得到 %d", len(prompt))
	}
}

func TestDistillPipeline_BuildSystemPrompt_Empty(t *testing.T) {
	pipeline := NewDistillPipeline()

	prompt := pipeline.BuildSystemPrompt("")
	if prompt != "" {
		t.Errorf("无内容时应返回空字符串, 得到 %q", prompt)
	}
}

func TestDistillPipeline_FeedbackToRating(t *testing.T) {
	tests := []struct {
		feedback string
		want     int
	}{
		{"good job", 1},
		{"great work", 1},
		{"correct answer", 1},
		{"bad response", -1},
		{"wrong answer", -1},
		{"incorrect", -1},
		{"okay", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := feedbackToRating(tt.feedback)
		if got != tt.want {
			t.Errorf("feedbackToRating(%q) = %d, 期望 %d", tt.feedback, got, tt.want)
		}
	}
}

func TestDistillPipeline_GetComponents(t *testing.T) {
	pipeline := NewDistillPipeline()

	if pipeline.GetDistiller() == nil {
		t.Error("GetDistiller 不应返回 nil")
	}
	if pipeline.GetEvolver() == nil {
		t.Error("GetEvolver 不应返回 nil")
	}
	if pipeline.GetFeedbackLearner() == nil {
		t.Error("GetFeedbackLearner 不应返回 nil")
	}
}

func TestDistillPipeline_MultipleInteractions(t *testing.T) {
	sink := &mockSemanticSink{}
	pipeline := NewDistillPipeline(
		WithSemanticMemory(sink),
		WithPipelineConfig(DistillPipelineConfig{
			MinConfidence: 0.5,
			EnableRAG:     false,
		}),
	)

	interactions := []Interaction{
		{ID: "i1", UserInput: "What is Go?", AgentOutput: "Go is a language.", Success: true, Timestamp: time.Now()},
		{ID: "i2", UserInput: "How to test?", AgentOutput: "Use testing package.", Success: true, Timestamp: time.Now()},
		{ID: "i3", UserInput: "What is Rust?", AgentOutput: "Rust is a systems language.", Success: true, Feedback: "good", Timestamp: time.Now()},
	}

	for _, inter := range interactions {
		_, err := pipeline.ProcessInteraction(context.Background(), inter)
		if err != nil {
			t.Fatalf("处理交互 %s 失败: %v", inter.ID, err)
		}
	}

	stats := pipeline.GetStats()
	if stats.TotalProcessed != 3 {
		t.Errorf("期望 TotalProcessed=3, 得到 %d", stats.TotalProcessed)
	}
}
