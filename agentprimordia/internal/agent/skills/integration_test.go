package skills

import (
	"context"
	"fmt"
	"testing"
)

// --- mock 集成适配器 ---

type mockToolInvoker struct{ tools map[string]bool }

func (m *mockToolInvoker) Invoke(name string, _ map[string]any) (string, error) {
	if !m.tools[name] {
		return "", fmt.Errorf("tool not found")
	}
	return "result:" + name, nil
}
func (m *mockToolInvoker) HasTool(name string) bool { return m.tools[name] }

type mockKnowledgeProvider struct{}

func (m *mockKnowledgeProvider) GetKnowledge(topic string) ([]string, error) {
	return []string{"fact1:" + topic, "fact2:" + topic}, nil
}

type mockPublisher struct{ published []string }

func (m *mockPublisher) Publish(s *Skill) (string, error) {
	m.published = append(m.published, s.ID)
	return "pub-" + s.ID, nil
}
func (m *mockPublisher) Unpublish(id string) error { return nil }

type mockAutonomyHook struct{}

func (m *mockAutonomyHook) OnGoalComplete(_ string, _ bool, _ string) {}
func (m *mockAutonomyHook) SuggestSkill(desc string) string {
	if desc == "fallback" {
		return "hook-skill"
	}
	return ""
}

type mockRAGSink struct{ stored int }

func (m *mockRAGSink) StoreTestCase(_ string, _ TestCase) error {
	m.stored++
	return nil
}

// --- 测试 ---

func TestToolIntegrationInvoke(t *testing.T) {
	ti := NewToolIntegration(&mockToolInvoker{tools: map[string]bool{"query": true}})

	out, err := ti.InvokeStepTool(StepDef{ID: "s1", ToolName: "query"}, nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out != "result:query" {
		t.Errorf("out = %q", out)
	}

	// 未注册工具
	_, err = ti.InvokeStepTool(StepDef{ID: "s2", ToolName: "missing"}, nil)
	if err == nil {
		t.Fatal("missing tool should error")
	}
}

func TestLearningIntegrationEnrich(t *testing.T) {
	li := NewLearningIntegration(&mockKnowledgeProvider{})
	s := NewSkill("x", "d", []StepDef{{ID: "s1"}})

	if err := li.EnrichSkillDescription(s, "data-fix"); err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if s.Metadata["knowledge_source"] != "data-fix" {
		t.Errorf("metadata = %v", s.Metadata)
	}
	if s.Metadata["knowledge_count"] != "2" {
		t.Errorf("count = %q", s.Metadata["knowledge_count"])
	}
}

func TestMarketplaceIntegrationPublish(t *testing.T) {
	pub := &mockPublisher{}
	mi := NewMarketplaceIntegration(pub)
	s := NewSkill("x", "d", []StepDef{{ID: "s1"}})

	// draft 不可发布
	if _, err := mi.PublishSkill(s); err == nil {
		t.Fatal("draft skill should not publish")
	}

	s.Activate()
	id, err := mi.PublishSkill(s)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if id == "" {
		t.Error("publish id empty")
	}
	if len(pub.published) != 1 {
		t.Errorf("published = %d", len(pub.published))
	}
}

func TestAutonomyIntegrationSuggest(t *testing.T) {
	store := NewStore()
	s := NewSkill("数据修复", "修复异常数据", []StepDef{{ID: "s1"}})
	s.Tags = []string{"数据", "修复"}
	s.Activate()
	store.Save(s)
	matcher := NewMatcher(store, MatcherConfig{})

	ai := NewAutonomyIntegration(&mockAutonomyHook{}, matcher)

	// matcher 命中
	if id := ai.SuggestForGoal("数据修复任务"); id != s.ID {
		t.Errorf("suggest = %q, want %q", id, s.ID)
	}
	// matcher 未命中，回退 hook
	if id := ai.SuggestForGoal("fallback"); id != "hook-skill" {
		t.Errorf("fallback suggest = %q", id)
	}
}

func TestRAGIntegrationSink(t *testing.T) {
	sink := &mockRAGSink{}
	ri := NewRAGIntegration(sink)

	cases := []TestCase{
		{Name: "c1", Input: map[string]any{}, ExpectedOutput: map[string]any{}},
		{Name: "c2", Input: map[string]any{}, ExpectedOutput: map[string]any{}},
	}
	if err := ri.SinkTestCases("s1", cases); err != nil {
		t.Fatalf("sink: %v", err)
	}
	if sink.stored != 2 {
		t.Errorf("stored = %d, want 2", sink.stored)
	}
}

// 确保 context 被引用（集成接口签名需要）
var _ = context.Background
