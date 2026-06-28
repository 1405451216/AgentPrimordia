package eval

import (
	"context"
	"testing"
)

// ===== ExactMatchEvaluator 测试 =====

// TestExactMatchEqual 测试精确匹配通过
func TestExactMatchEqual(t *testing.T) {
	ev := &ExactMatchEvaluator{}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello"},
		Expected:    "hello",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("内容相同应该通过")
	}
	if result.Score != 1.0 {
		t.Fatalf("通过时分数应该为 1.0，实际为 %f", result.Score)
	}
}

// TestExactMatchNotEqual 测试精确匹配不通过
func TestExactMatchNotEqual(t *testing.T) {
	ev := &ExactMatchEvaluator{}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello"},
		Expected:    "world",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if result.Passed {
		t.Fatal("内容不同不应该通过")
	}
	if result.Score != 0.0 {
		t.Fatalf("不通过时分数应该为 0.0，实际为 %f", result.Score)
	}
}

// TestExactMatchCaseInsensitive 测试忽略大小写匹配
func TestExactMatchCaseInsensitive(t *testing.T) {
	ev := &ExactMatchEvaluator{CaseInsensitive: true}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "Hello"},
		Expected:    "hello",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("忽略大小写后内容相同应该通过")
	}
}

// TestExactMatchNormalizeWhitespace 测试规范化空白匹配
func TestExactMatchNormalizeWhitespace(t *testing.T) {
	ev := &ExactMatchEvaluator{NormalizeWhitespace: true}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "  hello   world  "},
		Expected:    "hello world",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("规范化空白后内容相同应该通过")
	}
}

// ===== ContainsEvaluator 测试 =====

// TestContainsAllKeywords 测试包含所有关键词
func TestContainsAllKeywords(t *testing.T) {
	ev := &ContainsEvaluator{Keywords: []string{"hello", "world"}}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello beautiful world"},
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("包含所有关键词应该通过")
	}
}

// TestContainsPartialKeywords 测试部分包含关键词
func TestContainsPartialKeywords(t *testing.T) {
	ev := &ContainsEvaluator{Keywords: []string{"hello", "missing"}}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello world"},
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if result.Passed {
		t.Fatal("缺少关键词不应该通过")
	}
	if result.Score != 0.5 {
		t.Fatalf("1/2 关键词匹配时分数应该为 0.5，实际为 %f", result.Score)
	}
}

// TestContainsNoKeywords 测试无关键词
func TestContainsNoKeywords(t *testing.T) {
	ev := &ContainsEvaluator{Keywords: []string{}}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "anything"},
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("无关键词时应该通过")
	}
}

// TestContainsCaseInsensitive 测试关键词忽略大小写
func TestContainsCaseInsensitive(t *testing.T) {
	ev := &ContainsEvaluator{Keywords: []string{"Hello"}}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "say hello world"},
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("关键词匹配应该忽略大小写")
	}
}

// ===== ToolUsageEvaluator 测试 =====

// TestToolUsageAllUsed 测试所有工具都被使用
func TestToolUsageAllUsed(t *testing.T) {
	ev := &ToolUsageEvaluator{ExpectedTools: []string{"search", "read"}}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{
			ToolCalls: []ToolCall{
				{Name: "search", Args: "{}"},
				{Name: "read", Args: "{}"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("所有工具都被使用应该通过")
	}
}

// TestToolUsagePartialUsed 测试部分工具使用
func TestToolUsagePartialUsed(t *testing.T) {
	ev := &ToolUsageEvaluator{ExpectedTools: []string{"search", "read", "write"}}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{
			ToolCalls: []ToolCall{
				{Name: "search", Args: "{}"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if result.Passed {
		t.Fatal("部分工具使用不应该通过")
	}
}

// TestToolUsageNoExpected 测试无期望工具
func TestToolUsageNoExpected(t *testing.T) {
	ev := &ToolUsageEvaluator{ExpectedTools: []string{}}
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "no tools"},
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("无期望工具时应该通过")
	}
}

// ===== CompositeEvaluator 测试 =====

// TestCompositeAllPass 测试 CompositeAll 模式全部通过
func TestCompositeAllPass(t *testing.T) {
	ev := &CompositeEvaluator{
		Evaluators: []Evaluator{
			&ExactMatchEvaluator{},
			&ContainsEvaluator{Keywords: []string{"hello"}},
		},
		Mode: CompositeAll,
	}

	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello"},
		Expected:    "hello",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("所有评估器都通过时 CompositeAll 应该通过")
	}
}

// TestCompositeAllFail 测试 CompositeAll 模式部分失败
func TestCompositeAllFail(t *testing.T) {
	ev := &CompositeEvaluator{
		Evaluators: []Evaluator{
			&ExactMatchEvaluator{},
			&ContainsEvaluator{Keywords: []string{"missing"}},
		},
		Mode: CompositeAll,
	}

	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello"},
		Expected:    "hello",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if result.Passed {
		t.Fatal("部分评估器失败时 CompositeAll 不应该通过")
	}
}

// TestCompositeAnyPass 测试 CompositeAny 模式
func TestCompositeAnyPass(t *testing.T) {
	ev := &CompositeEvaluator{
		Evaluators: []Evaluator{
			&ExactMatchEvaluator{},
			&ContainsEvaluator{Keywords: []string{"hello"}},
		},
		Mode: CompositeAny,
	}

	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello world"},
		Expected:    "wrong",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("任一评估器通过时 CompositeAny 应该通过")
	}
}

// TestCompositeWeighted 测试 CompositeWeighted 模式
func TestCompositeWeighted(t *testing.T) {
	ev := &CompositeEvaluator{
		WeightedEvaluators: []WeightedEvaluator{
			{Evaluator: &ExactMatchEvaluator{}, Weight: 0.7},
			{Evaluator: &ContainsEvaluator{Keywords: []string{"hello"}}, Weight: 0.3},
		},
		Mode: CompositeWeighted,
	}

	// 精确匹配不通过(0.0)，包含通过(1.0)
	// 加权分数 = 0.0*0.7 + 1.0*0.3 = 0.3
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello world"},
		Expected:    "wrong",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if result.Passed {
		t.Fatal("加权分数 0.3 < 0.5 不应该通过")
	}
}

// TestCompositeWeightedPass 测试加权模式通过
func TestCompositeWeightedPass(t *testing.T) {
	ev := &CompositeEvaluator{
		WeightedEvaluators: []WeightedEvaluator{
			{Evaluator: &ExactMatchEvaluator{}, Weight: 0.7},
			{Evaluator: &ContainsEvaluator{Keywords: []string{"hello"}}, Weight: 0.3},
		},
		Mode: CompositeWeighted,
	}

	// 精确匹配通过(1.0)，包含通过(1.0)
	// 加权分数 = 1.0*0.7 + 1.0*0.3 = 1.0
	result, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "hello"},
		Expected:    "hello",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("加权分数 1.0 >= 0.5 应该通过")
	}
}

// ===== LLMEvaluator 测试 =====

// mockLLMProvider 用于测试的模拟 LLM 提供者
type mockLLMProvider struct {
	response string
	err      error
}

func (m *mockLLMProvider) Evaluate(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// TestLLMEvaluatorPass 测试 LLM 评估器通过
func TestLLMEvaluatorPass(t *testing.T) {
	ev := &LLMEvaluator{
		Provider: &mockLLMProvider{
			response: `{"score": 0.9, "passed": true, "reason": "good answer"}`,
		},
	}

	result, err := ev.Evaluate(context.Background(), EvalInput{
		Task:        "test task",
		AgentOutput: &Response{Content: "good answer"},
		Expected:    "good answer",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("LLM 评估通过时应该通过")
	}
	if result.Score != 0.9 {
		t.Fatalf("分数应该为 0.9，实际为 %f", result.Score)
	}
}

// TestLLMEvaluatorFail 测试 LLM 评估器不通过
func TestLLMEvaluatorFail(t *testing.T) {
	ev := &LLMEvaluator{
		Provider: &mockLLMProvider{
			response: `{"score": 0.3, "passed": false, "reason": "bad answer"}`,
		},
	}

	result, err := ev.Evaluate(context.Background(), EvalInput{
		Task:        "test task",
		AgentOutput: &Response{Content: "bad answer"},
		Expected:    "good answer",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if result.Passed {
		t.Fatal("LLM 评估不通过时不应该通过")
	}
}

// TestLLMEvaluatorProviderError 测试 LLM 提供者返回错误
func TestLLMEvaluatorProviderError(t *testing.T) {
	ev := &LLMEvaluator{
		Provider: &mockLLMProvider{err: context.DeadlineExceeded},
	}

	_, err := ev.Evaluate(context.Background(), EvalInput{
		AgentOutput: &Response{Content: "test"},
	})
	if err == nil {
		t.Fatal("LLM 提供者返回错误时 Evaluate 应该返回错误")
	}
}

// TestLLMEvaluatorCustomTemplate 测试自定义提示词模板
func TestLLMEvaluatorCustomTemplate(t *testing.T) {
	ev := &LLMEvaluator{
		Provider: &mockLLMProvider{
			response: `{"score": 1.0, "passed": true, "reason": "ok"}`,
		},
		PromptTemplate: "Task: {{.Task}} Output: {{.Output}} Expected: {{.Expected}}",
	}

	result, err := ev.Evaluate(context.Background(), EvalInput{
		Task:        "my task",
		AgentOutput: &Response{Content: "my output"},
		Expected:    "my expected",
	})
	if err != nil {
		t.Fatalf("Evaluate 返回错误: %v", err)
	}
	if !result.Passed {
		t.Fatal("应该通过")
	}
}

// ===== parseLLMEvalResponse 测试 =====

// TestParseLLMEvalResponseComplete 测试解析完整的 LLM 评估响应
func TestParseLLMEvalResponseComplete(t *testing.T) {
	resp := `Here is my evaluation: {"score": 0.85, "passed": true, "reason": "mostly correct"}`
	result, err := parseLLMEvalResponse(resp)
	if err != nil {
		t.Fatalf("parseLLMEvalResponse 返回错误: %v", err)
	}
	if result.Score != 0.85 {
		t.Fatalf("分数应该为 0.85，实际为 %f", result.Score)
	}
	if !result.Passed {
		t.Fatal("passed 应该为 true")
	}
}

// TestParseLLMEvalResponseIncomplete 测试解析不完整的 LLM 响应
func TestParseLLMEvalResponseIncomplete(t *testing.T) {
	resp := `{"score": 0.5}` // 缺少 passed 和 reason 字段
	result, err := parseLLMEvalResponse(resp)
	if err != nil {
		t.Fatalf("parseLLMEvalResponse 返回错误: %v", err)
	}
	// 不完整响应应该返回默认值
	if result.Passed {
		t.Fatal("不完整响应不应该通过")
	}
}

// TestParseLLMEvalResponseEmpty 测试解析空响应
func TestParseLLMEvalResponseEmpty(t *testing.T) {
	result, err := parseLLMEvalResponse("")
	if err != nil {
		t.Fatalf("parseLLMEvalResponse 返回错误: %v", err)
	}
	if result.Passed {
		t.Fatal("空响应不应该通过")
	}
	if result.Score != 0.0 {
		t.Fatalf("空响应分数应该为 0.0，实际为 %f", result.Score)
	}
}

// ===== EvalRunner 测试 =====

// mockEvalAgent 用于测试的模拟评估 Agent
type mockEvalAgent struct {
	response *Response
	err      error
}

func (m *mockEvalAgent) Run(ctx context.Context, input string) (*Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

// TestEvalRunnerSuite 测试评估套件运行
func TestEvalRunnerSuite(t *testing.T) {
	runner := NewEvalRunner(&ExactMatchEvaluator{})

	agent := &mockEvalAgent{
		response: &Response{Content: "hello"},
	}

	cases := []EvalCase{
		{Task: "greet", Input: "say hello", Expected: "hello"},
		{Task: "fail", Input: "say world", Expected: "world"},
	}

	result, err := runner.RunSuite(context.Background(), agent, cases)
	if err != nil {
		t.Fatalf("RunSuite 返回错误: %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("总数应该为 2，实际为 %d", result.Total)
	}
	if result.Passed != 1 {
		t.Fatalf("通过数应该为 1，实际为 %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Fatalf("失败数应该为 1，实际为 %d", result.Failed)
	}
	if result.PassRate != 0.5 {
		t.Fatalf("通过率应该为 0.5，实际为 %f", result.PassRate)
	}
}

// TestEvalRunnerAgentError 测试 Agent 运行失败
func TestEvalRunnerAgentError(t *testing.T) {
	runner := NewEvalRunner(&ExactMatchEvaluator{})

	agent := &mockEvalAgent{err: context.DeadlineExceeded}

	cases := []EvalCase{
		{Task: "fail", Input: "test", Expected: "test"},
	}

	result, err := runner.RunSuite(context.Background(), agent, cases)
	if err != nil {
		t.Fatalf("RunSuite 不应该返回错误: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("Agent 失败时用例应该标记为失败，实际失败数为 %d", result.Failed)
	}
	if len(result.Results) != 1 {
		t.Fatal("结果列表应该有 1 条记录")
	}
	if result.Results[0].Error == nil {
		t.Fatal("失败用例的 Error 不应该为 nil")
	}
}

// TestEvalRunnerEmptyCases 测试空用例列表
func TestEvalRunnerEmptyCases(t *testing.T) {
	runner := NewEvalRunner(&ExactMatchEvaluator{})
	agent := &mockEvalAgent{response: &Response{Content: "ok"}}

	result, err := runner.RunSuite(context.Background(), agent, nil)
	if err != nil {
		t.Fatalf("RunSuite 返回错误: %v", err)
	}
	if result.Total != 0 {
		t.Fatalf("空用例总数应该为 0，实际为 %d", result.Total)
	}
	if result.PassRate != 0 {
		t.Fatalf("空用例通过率应该为 0，实际为 %f", result.PassRate)
	}
}

// TestEvalRunnerNoEvaluators 测试无评估器
func TestEvalRunnerNoEvaluators(t *testing.T) {
	runner := NewEvalRunner()
	agent := &mockEvalAgent{response: &Response{Content: "ok"}}

	cases := []EvalCase{
		{Task: "test", Input: "test", Expected: "ok"},
	}

	result, err := runner.RunSuite(context.Background(), agent, cases)
	if err != nil {
		t.Fatalf("RunSuite 返回错误: %v", err)
	}
	// 无评估器时默认通过
	if result.Passed != 1 {
		t.Fatalf("无评估器时应该默认通过，实际通过数为 %d", result.Passed)
	}
}

// ===== 辅助函数测试 =====

// TestNormalizeWhitespace 测试空白规范化
func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello   world  ", "hello world"},
		{"hello\t\tworld", "hello world"},
		{"hello\n\nworld", "hello world"},
		{"  a  b  c  ", "a b c"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		result := normalizeWhitespace(tt.input)
		if result != tt.expected {
			t.Errorf("输入 %q: 期望 %q，实际 %q", tt.input, tt.expected, result)
		}
	}
}

// TestBoolToScore 测试布尔值转分数
func TestBoolToScore(t *testing.T) {
	if boolToScore(true) != 1.0 {
		t.Fatal("true 应该转为 1.0")
	}
	if boolToScore(false) != 0.0 {
		t.Fatal("false 应该转为 0.0")
	}
}

// TestFoundNotFound 测试 found/not_found 字符串
func TestFoundNotFound(t *testing.T) {
	if foundNotFound(true) != "found" {
		t.Fatal("true 应该返回 'found'")
	}
	if foundNotFound(false) != "not found" {
		t.Fatal("false 应该返回 'not found'")
	}
}

// ===== 接口满足性测试 =====

// TestEvaluatorInterface 测试 Evaluator 接口满足
func TestEvaluatorInterface(t *testing.T) {
	var _ Evaluator = (*ExactMatchEvaluator)(nil)
	var _ Evaluator = (*ContainsEvaluator)(nil)
	var _ Evaluator = (*ToolUsageEvaluator)(nil)
	var _ Evaluator = (*CompositeEvaluator)(nil)
	var _ Evaluator = (*LLMEvaluator)(nil)
}

// TestAgentInterface 测试 Agent 接口满足
func TestAgentInterface(t *testing.T) {
	var _ Agent = (*mockEvalAgent)(nil)
}

// ===== JSON 解析辅助函数测试 =====

// TestExtractJSONNumber 测试提取 JSON 数字
func TestExtractJSONNumber(t *testing.T) {
	tests := []struct {
		json     string
		key      string
		expected string
	}{
		{`"score": 0.85`, `"score"`, "0.85"},
		{`"score": 1`, `"score"`, "1"},
		{`"count": 42`, `"count"`, "42"},
		{`"other": 1`, `"score"`, ""},
	}

	for _, tt := range tests {
		result := extractJSONNumber(tt.json, tt.key)
		if result != tt.expected {
			t.Errorf("extractJSONNumber(%q, %q): 期望 %q，实际 %q", tt.json, tt.key, tt.expected, result)
		}
	}
}

// TestExtractJSONValue 测试提取 JSON 值
func TestExtractJSONValue(t *testing.T) {
	tests := []struct {
		json     string
		key      string
		expected string
	}{
		{`"passed": true`, `"passed"`, "true"},
		{`"passed": false`, `"passed"`, "false"},
		{`"other": true`, `"passed"`, ""},
	}

	for _, tt := range tests {
		result := extractJSONValue(tt.json, tt.key)
		if result != tt.expected {
			t.Errorf("extractJSONValue(%q, %q): 期望 %q，实际 %q", tt.json, tt.key, tt.expected, result)
		}
	}
}

// TestExtractJSONString 测试提取 JSON 字符串
func TestExtractJSONString(t *testing.T) {
	tests := []struct {
		json     string
		key      string
		expected string
	}{
		{`"reason": "good answer"`, `"reason"`, "good answer"},
		{`"reason": ""`, `"reason"`, ""},
		{`"other": "test"`, `"reason"`, ""},
	}

	for _, tt := range tests {
		result := extractJSONString(tt.json, tt.key)
		if result != tt.expected {
			t.Errorf("extractJSONString(%q, %q): 期望 %q，实际 %q", tt.json, tt.key, tt.expected, result)
		}
	}
}
