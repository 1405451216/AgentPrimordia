package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// mockFuzzTool 用于 Fuzz 测试的简单工具实现
type mockFuzzTool struct {
	name string
}

func (m *mockFuzzTool) Name() string                { return m.name }
func (m *mockFuzzTool) Description() string         { return "fuzz test tool" }
func (m *mockFuzzTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m *mockFuzzTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return NewResult("ok"), nil
}

// FuzzExecutorExecute 模糊测试工具执行器。
// 确保对任意工具名和参数组合不 panic。
func FuzzExecutorExecute(f *testing.F) {
	seedCalls := []struct {
		name string
		args string
	}{
		{"echo", `{"text":"hello"}`},
		{"nonexistent", `{"key":"value"}`},
		{"", `{"key":"value"}`},
		{"echo", ""},
		{"echo", "invalid json"},
		{"echo", `{"nested":{"deep":{"value":123}}}`},
		{"echo", string(make([]byte, 100))},
	}
	for _, s := range seedCalls {
		f.Add(s.name, s.args)
	}

	f.Fuzz(func(t *testing.T, toolName, args string) {
		registry := NewRegistry()
		_ = registry.Register(&mockFuzzTool{name: "echo"})
		executor := NewExecutor(registry)

		ctx := context.Background()
		tc := &FunctionCall{
			ID:   "test-call",
			Name: toolName,
			Args: args,
		}

		// Execute 不应 panic
		result, err := executor.Execute(ctx, tc)

		// 不存在的工具应返回错误
		if toolName != "echo" && err == nil && result != nil && !result.IsError {
			// 非注册工具不应成功执行
			// 注意：executor 对不存在的工具返回 ErrorResult 而非 error
			t.Errorf("非注册工具 %q 不应成功执行", toolName)
		}

		// 空工具名应返回错误
		if toolName == "" && err == nil && result != nil && !result.IsError {
			t.Errorf("空工具名应返回错误")
		}
	})
}

// FuzzRegistryRegister 模糊测试工具注册。
// 确保对任意工具名不 panic。
func FuzzRegistryRegister(f *testing.F) {
	seedNames := []string{
		"echo",
		"calculate",
		"",
		"tool_with_special_chars!@#",
		"very_long_tool_name_" + string(make([]byte, 50)),
		"中文工具",
	}
	for _, s := range seedNames {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		registry := NewRegistry()

		// Register 不应 panic
		err := registry.Register(&mockFuzzTool{name: name})

		// 空名应返回 ErrInvalidConfig
		if name == "" && err == nil {
			t.Errorf("空工具名应返回 ErrInvalidConfig")
		}

		// 非空名注册后应可获取
		if name != "" && err == nil {
			tool, exists := registry.Get(name)
			if !exists {
				t.Errorf("注册后工具 %q 不可获取", name)
			}
			if tool == nil {
				t.Errorf("注册后工具 %q 为 nil", name)
			}
		}
	})
}

// FuzzExecuteBatch 模糊测试批量工具执行。
// 确保对任意调用列表不 panic。
func FuzzExecuteBatch(f *testing.F) {
	seedBatches := [][]struct {
		name string
		args string
	}{
		{{"echo", `{"text":"a"}`}, {"echo", `{"text":"b"}`}},
		{{"nonexistent", `{}`}},
		{{"echo", "invalid"}, {"echo", `{"text":"ok"}`}},
		{},
	}
	for _, batch := range seedBatches {
		// 将 struct 转为可添加的参数
		names := make([]string, len(batch))
		argsList := make([]string, len(batch))
		for i, call := range batch {
			names[i] = call.name
			argsList[i] = call.args
		}
		f.Add(strings.Join(names, "\x00"), strings.Join(argsList, "\x00"))
	}

	f.Fuzz(func(t *testing.T, namesStr, argsStr string) {
		registry := NewRegistry()
		_ = registry.Register(&mockFuzzTool{name: "echo"})
		executor := NewExecutor(registry)

		// 解析输入
		names := splitNull(namesStr)
		argsList := splitNull(argsStr)

		if len(names) != len(argsList) {
			return // 长度不匹配，跳过
		}

		calls := make([]*FunctionCall, len(names))
		for i := range names {
			calls[i] = &FunctionCall{
				ID:   "batch-call",
				Name: names[i],
				Args: argsList[i],
			}
		}

		ctx := context.Background()
		// ExecuteBatch 不应 panic
		results, err := executor.ExecuteBatch(ctx, calls)

		if err != nil {
			return
		}

		// 结果数应等于调用数
		if len(results) != len(calls) {
			t.Errorf("结果数 %d != 调用数 %d", len(results), len(calls))
		}
	})
}

// splitNull 按空字节分割字符串
func splitNull(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	current := ""
	for _, r := range s {
		if r == 0 {
			result = append(result, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	result = append(result, current)
	return result
}
