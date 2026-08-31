package learning

import (
	"context"
	"testing"
	"time"
)

func TestKnowledgeDistillerDistill(t *testing.T) {
	distiller := NewKnowledgeDistiller()

	interaction := Interaction{
		ID:          "test-1",
		UserInput:   "What is the capital of France?",
		AgentOutput: "The capital of France is Paris. Paris is a beautiful city.",
		Success:     true,
		Timestamp:   time.Now(),
	}

	items, err := distiller.Distill(context.Background(), interaction)
	if err != nil {
		t.Fatalf("Distill 失败: %v", err)
	}

	if len(items) == 0 {
		t.Error("应蒸馏出至少一个知识项")
	}

	// 检查是否包含事实
	hasFact := false
	for _, item := range items {
		if item.Category == "fact" {
			hasFact = true
			if item.Confidence < 0.7 {
				t.Errorf("Confidence = %f, 应 >= 0.7", item.Confidence)
			}
		}
	}
	if !hasFact {
		t.Error("应包含事实类知识")
	}
}

func TestKnowledgeDistillerWithFeedback(t *testing.T) {
	distiller := NewKnowledgeDistiller()

	interaction := Interaction{
		ID:          "test-2",
		UserInput:   "Tell me about Go",
		AgentOutput: "Go is a programming language. It is compiled.",
		Feedback:    "Great answer, very helpful!",
		Success:     true,
		Timestamp:   time.Now(),
	}

	items, err := distiller.Distill(context.Background(), interaction)
	if err != nil {
		t.Fatalf("Distill 失败: %v", err)
	}

	// 应包含偏好
	hasPref := false
	for _, item := range items {
		if item.Category == "preference" {
			hasPref = true
		}
	}
	if !hasPref {
		t.Error("应包含偏好类知识")
	}
}

func TestKnowledgeDistillerSearch(t *testing.T) {
	distiller := NewKnowledgeDistiller()

	interaction := Interaction{
		ID:          "search-test",
		UserInput:   "What is AI?",
		AgentOutput: "AI is artificial intelligence. It is a field of computer science.",
		Success:     true,
		Timestamp:   time.Now(),
	}

	_, _ = distiller.Distill(context.Background(), interaction)

	// 搜索事实
	results := distiller.SearchKnowledge("fact", "")
	if len(results) == 0 {
		t.Error("搜索事实应返回结果")
	}

	// 搜索不存在的类别
	results = distiller.SearchKnowledge("nonexistent", "")
	if len(results) != 0 {
		t.Error("搜索不存在的类别应返回空")
	}
}

func TestKnowledgeDistillerGetKnowledge(t *testing.T) {
	distiller := NewKnowledgeDistiller()

	interaction := Interaction{
		ID:          "get-test",
		UserInput:   "What is 1+1?",
		AgentOutput: "1+1 equals 2. This is a basic math fact.",
		Success:     true,
		Timestamp:   time.Now(),
	}

	items, _ := distiller.Distill(context.Background(), interaction)
	if len(items) == 0 {
		t.Fatal("未蒸馏出知识")
	}

	// 按ID获取
	got, exists := distiller.GetKnowledge(items[0].ID)
	if !exists {
		t.Error("应能获取知识项")
	}
	if got.Pattern != items[0].Pattern {
		t.Error("Pattern 不匹配")
	}

	// 获取不存在的
	_, exists = distiller.GetKnowledge("nonexistent")
	if exists {
		t.Error("不存在的 ID 应返回 false")
	}
}

func TestKnowledgeDistillerStats(t *testing.T) {
	distiller := NewKnowledgeDistiller()

	for i := 0; i < 5; i++ {
		_, _ = distiller.Distill(context.Background(), Interaction{
			ID:          "stat-" + string(rune('0'+i)),
			UserInput:   "What is X?",
			AgentOutput: "X is a variable. It is commonly used in math.",
			Success:     true,
			Timestamp:   time.Now(),
		})
	}

	stats := distiller.GetStats()
	if stats.TotalInteractions != 5 {
		t.Errorf("TotalInteractions = %d, 期望 5", stats.TotalInteractions)
	}
	if stats.TotalDistilled == 0 {
		t.Error("TotalDistilled 应 > 0")
	}
}

func TestCapabilityEvolverRegister(t *testing.T) {
	evolver := NewCapabilityEvolver()

	evolver.Register(Capability{
		Name:        "reasoning",
		Description: "Logical reasoning ability",
		Score:       0.5,
	})

	cap, exists := evolver.GetCapability("reasoning")
	if !exists {
		t.Fatal("能力不存在")
	}
	if cap.Score != 0.5 {
		t.Errorf("Score = %f", cap.Score)
	}
}

func TestCapabilityEvolverEvaluate(t *testing.T) {
	evolver := NewCapabilityEvolver()
	evolver.Register(Capability{
		Name:  "coding",
		Score: 0.5,
	})

	// 评估通过
	_ = evolver.Evaluate("coding", true)

	cap, _ := evolver.GetCapability("coding")
	if cap.TimesTested != 1 {
		t.Errorf("TimesTested = %d, 期望 1", cap.TimesTested)
	}
	if cap.TimesPassed != 1 {
		t.Errorf("TimesPassed = %d, 期望 1", cap.TimesPassed)
	}
	if cap.Score <= 0.5 {
		t.Error("通过后 Score 应 > 0.5")
	}

	// 评估失败
	_ = evolver.Evaluate("coding", false)
	cap, _ = evolver.GetCapability("coding")
	if cap.TimesTested != 2 {
		t.Errorf("TimesTested = %d, 期望 2", cap.TimesTested)
	}
	if cap.TimesPassed != 1 {
		t.Errorf("TimesPassed = %d, 期望 1", cap.TimesPassed)
	}
}

func TestCapabilityEvolverGetWeaknesses(t *testing.T) {
	evolver := NewCapabilityEvolver()
	evolver.Register(Capability{
		Name:  "strong",
		Score: 0.9,
	})
	evolver.Register(Capability{
		Name:  "weak",
		Score: 0.3,
	})
	evolver.Register(Capability{
		Name:  "medium",
		Score: 0.6,
	})

	// 获取弱项（阈值 0.5）
	weaknesses := evolver.GetWeaknesses(0.5)
	if len(weaknesses) != 1 {
		t.Fatalf("弱项数 = %d, 期望 1", len(weaknesses))
	}
	if weaknesses[0].Name != "weak" {
		t.Errorf("弱项名称 = %s, 期望 weak", weaknesses[0].Name)
	}
}

func TestCapabilityEvolverListCapabilities(t *testing.T) {
	evolver := NewCapabilityEvolver()
	evolver.Register(Capability{Name: "a", Score: 0.5})
	evolver.Register(Capability{Name: "b", Score: 0.7})

	caps := evolver.ListCapabilities()
	if len(caps) != 2 {
		t.Errorf("能力数 = %d, 期望 2", len(caps))
	}
}

func TestFeedbackLearnerRecordPositive(t *testing.T) {
	learner := NewFeedbackLearner()

	err := learner.RecordFeedback(FeedbackEntry{
		UserInput:   "test input",
		AgentOutput: "This is a good answer because it is clear.",
		Feedback:    "Great, very helpful!",
		Rating:      1,
	})
	if err != nil {
		t.Fatalf("RecordFeedback 失败: %v", err)
	}

	prefs := learner.GetPreferences()
	if prefs.TotalFeedback != 1 {
		t.Errorf("TotalFeedback = %d, 期望 1", prefs.TotalFeedback)
	}
	if len(prefs.PositivePatterns) == 0 {
		t.Error("应记录正面模式")
	}
}

func TestFeedbackLearnerRecordNegative(t *testing.T) {
	learner := NewFeedbackLearner()

	_ = learner.RecordFeedback(FeedbackEntry{
		UserInput:   "test input",
		AgentOutput: "This is a bad answer.",
		Feedback:    "Incorrect and unhelpful.",
		Rating:      -1,
	})

	prefs := learner.GetPreferences()
	if len(prefs.NegativePatterns) == 0 {
		t.Error("应记录负面模式")
	}
}

func TestFeedbackLearnerShouldPrefer(t *testing.T) {
	learner := NewFeedbackLearner()

	// 训练正面偏好
	_ = learner.RecordFeedback(FeedbackEntry{
		UserInput:   "How to code?",
		AgentOutput: "Use clean code practices. It is recommended.",
		Feedback:    "Good advice!",
		Rating:      1,
	})

	// 训练负面偏好
	_ = learner.RecordFeedback(FeedbackEntry{
		UserInput:   "How to code?",
		AgentOutput: "Just write spaghetti code. It is fast.",
		Feedback:    "Bad advice, wrong!",
		Rating:      -1,
	})

	// 测试偏好判断
	positiveOutput := "Use clean code practices. It is recommended."
	score := learner.ShouldPrefer(positiveOutput)
	if score <= 0.5 {
		t.Errorf("正面输出的偏好分数 = %f, 应 > 0.5", score)
	}

	negativeOutput := "Just write spaghetti code. It is fast."
	score = learner.ShouldPrefer(negativeOutput)
	if score >= 0.5 {
		t.Errorf("负面输出的偏好分数 = %f, 应 < 0.5", score)
	}
}

func TestFeedbackLearnerHistory(t *testing.T) {
	learner := NewFeedbackLearner()

	for i := 0; i < 5; i++ {
		_ = learner.RecordFeedback(FeedbackEntry{
			UserInput:   "test",
			AgentOutput: "test output",
			Feedback:    "ok",
			Rating:      0,
		})
	}

	history := learner.GetFeedbackHistory()
	if len(history) != 5 {
		t.Errorf("历史记录数 = %d, 期望 5", len(history))
	}
}

func TestSplitSentences(t *testing.T) {
	sentences := splitSentences("Hello world. How are you? I am fine!")
	if len(sentences) != 3 {
		t.Errorf("句子数 = %d, 期望 3", len(sentences))
	}
}

func TestSplitSentencesEmpty(t *testing.T) {
	sentences := splitSentences("")
	if len(sentences) != 0 {
		t.Error("空文本应返回空")
	}
}
