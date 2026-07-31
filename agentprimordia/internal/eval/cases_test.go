package eval

import (
	"encoding/json"
	"strings"
	"testing"
)

// ===== EvalCase JSON 序列化/反序列化测试 =====

func TestEvalCase_JSONRoundTrip(t *testing.T) {
	original := EvalCase{
		ID:        "test_case",
		Name:      "Test Case",
		Category:  CategoryTool,
		Input:     "test input",
		Expected:  "test output",
		Metrics:   []string{MetricAccuracy, MetricLatency},
		Threshold: 0.8,
		Metadata:  map[string]string{"key": "value"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	var restored EvalCase
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID = %q, 期望 %q", restored.ID, original.ID)
	}
	if restored.Name != original.Name {
		t.Errorf("Name = %q, 期望 %q", restored.Name, original.Name)
	}
	if restored.Category != original.Category {
		t.Errorf("Category = %q, 期望 %q", restored.Category, original.Category)
	}
	if restored.Input != original.Input {
		t.Errorf("Input = %q, 期望 %q", restored.Input, original.Input)
	}
	if restored.Expected != original.Expected {
		t.Errorf("Expected = %q, 期望 %q", restored.Expected, original.Expected)
	}
	if restored.Threshold != original.Threshold {
		t.Errorf("Threshold = %f, 期望 %f", restored.Threshold, original.Threshold)
	}
	if len(restored.Metrics) != len(original.Metrics) {
		t.Errorf("Metrics 长度 = %d, 期望 %d", len(restored.Metrics), len(original.Metrics))
	}
	if restored.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %q, 期望 value", restored.Metadata["key"])
	}
}

func TestEvalCase_JSONFieldNames(t *testing.T) {
	c := EvalCase{
		ID:       "json_test",
		Name:     "JSON Test",
		Category: CategoryChat,
		Input:    "hello",
		Expected: "hi",
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	jsonStr := string(data)

	// 验证 JSON 字段名使用小写
	expectedFields := []string{`"id"`, `"name"`, `"category"`, `"input"`, `"expected"`, `"metrics"`, `"threshold"`}
	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("JSON 应包含字段 %s", field)
		}
	}
}

func TestEvalCase_OmitEmptyMetadata(t *testing.T) {
	c := EvalCase{
		ID:       "no_meta",
		Name:     "No Metadata",
		Category: CategoryTool,
		Input:    "test",
		Expected: "result",
		// Metadata 为零值
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	jsonStr := string(data)
	if strings.Contains(jsonStr, "metadata") {
		t.Error("零值 Metadata 应被 omitempty 省略")
	}
}

// ===== EvalResult JSON 测试 =====

func TestEvalResult_JSONRoundTrip(t *testing.T) {
	r := EvalResult{
		CaseID:   "case1",
		Passed:   true,
		Score:    0.95,
		Duration: 150,
		Error:    "",
		Metadata: map[string]string{"env": "test"},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	var restored EvalResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	if restored.CaseID != r.CaseID {
		t.Errorf("CaseID = %q, 期望 %q", restored.CaseID, r.CaseID)
	}
	if restored.Passed != r.Passed {
		t.Errorf("Passed = %v, 期望 %v", restored.Passed, r.Passed)
	}
	if restored.Score != r.Score {
		t.Errorf("Score = %f, 期望 %f", restored.Score, r.Score)
	}
	if restored.Duration != r.Duration {
		t.Errorf("Duration = %d, 期望 %d", restored.Duration, r.Duration)
	}
}

func TestEvalResult_OmitEmptyError(t *testing.T) {
	r := EvalResult{
		CaseID: "case1",
		Passed: true,
		Score:  1.0,
		// Error 为空
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	jsonStr := string(data)
	if strings.Contains(jsonStr, "error") {
		t.Error("空 Error 应被 omitempty 省略")
	}
}

// ===== EvalSuiteResult JSON 测试 =====

func TestEvalSuiteResult_JSONRoundTrip(t *testing.T) {
	suite := EvalSuiteResult{
		Total:    3,
		Passed:   2,
		Failed:   1,
		PassRate: 0.6667,
		Results: []EvalResult{
			{CaseID: "c1", Passed: true, Score: 1.0},
			{CaseID: "c2", Passed: true, Score: 0.8},
			{CaseID: "c3", Passed: false, Score: 0.0, Error: "timeout"},
		},
	}

	data, err := json.Marshal(suite)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	var restored EvalSuiteResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	if restored.Total != suite.Total {
		t.Errorf("Total = %d, 期望 %d", restored.Total, suite.Total)
	}
	if restored.Passed != suite.Passed {
		t.Errorf("Passed = %d, 期望 %d", restored.Passed, suite.Passed)
	}
	if restored.Failed != suite.Failed {
		t.Errorf("Failed = %d, 期望 %d", restored.Failed, suite.Failed)
	}
	if len(restored.Results) != 3 {
		t.Errorf("Results 长度 = %d, 期望 3", len(restored.Results))
	}
}

// ===== 分类和指标常量测试 =====

func TestCategoryConstants(t *testing.T) {
	// 验证常量值不为空且唯一
	categories := map[string]string{
		"CategoryTool":     CategoryTool,
		"CategoryMemory":   CategoryMemory,
		"CategoryPlanning": CategoryPlanning,
		"CategorySafety":   CategorySafety,
		"CategoryChat":     CategoryChat,
	}

	seen := make(map[string]bool)
	for name, val := range categories {
		if val == "" {
			t.Errorf("%s 不应为空", name)
		}
		if seen[val] {
			t.Errorf("%s 值 %q 与其他常量重复", name, val)
		}
		seen[val] = true
	}
}

func TestMetricConstants(t *testing.T) {
	metrics := map[string]string{
		"MetricAccuracy":  MetricAccuracy,
		"MetricLatency":   MetricLatency,
		"MetricSafety":    MetricSafety,
		"MetricRelevance": MetricRelevance,
	}

	seen := make(map[string]bool)
	for name, val := range metrics {
		if val == "" {
			t.Errorf("%s 不应为空", name)
		}
		if seen[val] {
			t.Errorf("%s 值 %q 与其他常量重复", name, val)
		}
		seen[val] = true
	}
}

// ===== SharedEvalCases 分类覆盖测试 =====

func TestSharedEvalCases_CategoryCoverage(t *testing.T) {
	cases := SharedEvalCases()

	categories := make(map[string]bool)
	for _, c := range cases {
		categories[c.Category] = true
	}

	// 验证至少覆盖了 4 个分类
	if len(categories) < 4 {
		t.Errorf("分类覆盖 = %d, 期望至少 4 个", len(categories))
	}
}

// ===== CompileCases 测试 =====

func TestCompileCases_EmptyCases(t *testing.T) {
	data, err := CompileCases([]EvalCase{})
	if err != nil {
		t.Fatalf("CompileCases 空列表失败: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("空列表应序列化为 [], 得到 %q", string(data))
	}
}

func TestCompileCases_SingleCase(t *testing.T) {
	cases := []EvalCase{
		{
			ID:        "single",
			Name:      "Single",
			Category:  CategoryChat,
			Input:     "hi",
			Expected:  "hello",
			Threshold: 0.5,
		},
	}
	data, err := CompileCases(cases)
	if err != nil {
		t.Fatalf("CompileCases 失败: %v", err)
	}

	var restored []EvalCase
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(restored) != 1 {
		t.Errorf("期望 1 个用例, 得到 %d", len(restored))
	}
	if restored[0].ID != "single" {
		t.Errorf("ID = %q, 期望 single", restored[0].ID)
	}
}
