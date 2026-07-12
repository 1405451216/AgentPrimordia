package eval

import (
	"context"
	"encoding/json"
	"strings"
	"fmt"
	"testing"
)

// mockEvalAgent 用于测试的模拟 Agent。
type mockEvalAgent struct {
	responses map[string]string
}

func (m *mockEvalAgent) Run(_ context.Context, input string) (string, error) {
	for kw, resp := range m.responses {
		if strings.Contains(input, kw) {
			return resp, nil
		}
	}
	return "default response", nil
}

// TestSharedEvalCases 验证共享用例定义完整性。
func TestSharedEvalCases(t *testing.T) {
	cases := SharedEvalCases()
	if len(cases) < 5 {
		t.Fatalf("expected at least 5 cases, got %d", len(cases))
	}

	// 验证必需字段
	for _, c := range cases {
		if c.ID == "" {
			t.Errorf("case has empty ID")
		}
		if c.Name == "" {
			t.Errorf("case %s has empty Name", c.ID)
		}
		if c.Input == "" {
			t.Errorf("case %s has empty Input", c.ID)
		}
		if c.Expected == "" {
			t.Errorf("case %s has empty Expected", c.ID)
		}
		if c.Category == "" {
			t.Errorf("case %s has empty Category", c.ID)
		}
		if c.Threshold <= 0 || c.Threshold > 1 {
			t.Errorf("case %s Threshold = %f, want (0,1]", c.ID, c.Threshold)
		}
	}

	// 验证 ID 唯一性
	ids := make(map[string]bool)
	for _, c := range cases {
		if ids[c.ID] {
			t.Errorf("duplicate case ID: %s", c.ID)
		}
		ids[c.ID] = true
	}
}

// TestSharedEvalCasesJSON 验证用例 JSON 兼容性。
func TestSharedEvalCasesJSON(t *testing.T) {
	cases := SharedEvalCases()

	data, err := CompileCases(cases)
	if err != nil {
		t.Fatalf("CompileCases failed: %v", err)
	}

	// 验证 JSON 可被反序列化
	var restored []EvalCase
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(restored) != len(cases) {
		t.Errorf("restored count = %d, want %d", len(restored), len(cases))
	}

	// 验证字段映射正确（camelCase → snake_case）
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"id"`) {
		t.Errorf("JSON should contain 'id' field")
	}
	if !strings.Contains(jsonStr, `"category"`) {
		t.Errorf("JSON should contain 'category' field")
	}
}

// TestSimpleEvaluator 验证简单评估器。
func TestSimpleEvaluator(t *testing.T) {
	e := &SimpleEvaluator{}
	c := EvalCase{
		ID:       "test",
		Expected: "hello",
	}

	score, passed, err := e.Evaluate(context.Background(), c, "Well hello there!")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if !passed {
		t.Error("expected keyword should pass")
	}
	if score != 1.0 {
		t.Errorf("score = %f, want 1.0", score)
	}
}

// TestSharedEvalRunner 验证执行器完整流程。
func TestSharedEvalRunner(t *testing.T) {
	agent := &mockEvalAgent{
		responses: map[string]string{
			"Hello":   "Hello there! Nice to meet you.",
			"Search":  "web_search results for Go release",
			"memory":  "memory_recall: your preference is saved",
			"SSN":     "block: PII detected",
			"Plan":    "decompose: step 1, step 2, step 3",
		},
	}

	evaluator := &SimpleEvaluator{}
	runner := NewSharedEvalRunner(evaluator)
	result, err := runner.RunSharedEval(context.Background(), agent)
	if err != nil {
		t.Fatalf("RunSharedEval failed: %v", err)
	}

	if result.Total != len(SharedEvalCases()) {
		t.Errorf("Total = %d, want %d", result.Total, len(SharedEvalCases()))
	}
	if result.Passed == 0 {
		t.Error("expected at least one passed case")
	}
	if result.PassRate <= 0 || result.PassRate > 1 {
		t.Errorf("PassRate = %f, want (0,1]", result.PassRate)
	}
}

// TestSharedEvalRunnerWithErrorAgent 验证错误处理。
func TestSharedEvalRunnerWithErrorAgent(t *testing.T) {
	errorAgent := &errorMockAgent{}
	evaluator := &SimpleEvaluator{}
	runner := NewSharedEvalRunner(evaluator)
	result, err := runner.RunSharedEvalWithCases(context.Background(), errorAgent, SharedEvalCases()[:1])
	if err != nil {
		t.Fatalf("RunSharedEvalWithCases failed: %v", err)
	}

	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if result.Results[0].Error == "" {
		t.Error("expected error in result")
	}
}

// TestContainsAnyEvaluator 验证多关键词评估器。
func TestContainsAnyEvaluator(t *testing.T) {
	e := &ContainsAnyEvaluator{Keywords: []string{"search", "lookup", "find"}}
	c := EvalCase{ID: "kw", Expected: "search"}

	score, passed, err := e.Evaluate(context.Background(), c, "I will lookup the database")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if !passed {
		t.Error("should pass with matching keyword 'lookup'")
	}
	if score != 1.0 {
		t.Errorf("score = %f, want 1.0", score)
	}
}

type errorMockAgent struct{}

func (m *errorMockAgent) Run(_ context.Context, input string) (string, error) {
	return "", fmt.Errorf("mock agent error")
}

// fmt is imported for errorMockAgent
var _ = fmt.Sprintf
