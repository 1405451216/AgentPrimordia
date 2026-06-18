package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// errorTool 执行时返回错误的工具
type errorTool struct {
	name string
}

func (e *errorTool) Name() string        { return e.name }
func (e *errorTool) Description() string { return "always errors" }
func (e *errorTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (e *errorTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return NewErrorResult("tool execution failed"), ErrToolExecution
}

// panicTool 执行时 panic 的工具（用于测试 perf-v5 Task 1 的 panic recover）
type panicTool struct {
	name string
}

func (p *panicTool) Name() string        { return p.name }
func (p *panicTool) Description() string { return "panics on execute" }
func (p *panicTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (p *panicTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	panic("intentional tool panic for testing")
}

// perf-v5 Task 1：测试 Execute panic recover
func TestExecutor_Execute_PanicRecover(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&panicTool{name: "panic_tool"})
	executor := NewExecutor(reg)

	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_panic",
		Name: "panic_tool",
		Args: `{}`,
	})

	// panic 应被捕获并转为 error 返回
	if err == nil {
		t.Fatal("expected error from panic recovery, got nil")
	}
	if result == nil {
		t.Fatal("result should not be nil after panic recovery")
	}
	if !result.IsError {
		t.Error("result should be marked as error")
	}
	if result.Content == "" {
		t.Error("error result should have content describing the panic")
	}
}

func TestExecutor_Execute_UnknownTool(t *testing.T) {
	reg := NewRegistry()
	executor := NewExecutor(reg)

	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_1",
		Name: "nonexistent_tool",
		Args: `{}`,
	})

	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if !result.IsError {
		t.Error("result should be error")
	}
	if result.Content == "" {
		t.Error("error result should have content")
	}
}

func TestExecutor_Execute_InvalidArgs(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", description: "Echo", response: "ok"})

	executor := NewExecutor(reg)

	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_1",
		Name: "echo",
		Args: `{invalid json`,
	})

	// mockTool 不校验参数，所以即使参数无效也不会报错
	// 但 extractPathFromArgs 应该处理无效 JSON
	if err != nil {
		t.Logf("invalid args returned error: %v (acceptable)", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestExecutor_Execute_ToolError(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&errorTool{name: "fail_tool"})

	executor := NewExecutor(reg)

	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_1",
		Name: "fail_tool",
		Args: `{}`,
	})

	if !errors.Is(err, ErrToolExecution) {
		t.Errorf("expected ErrToolExecution, got: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_Execute_ContextCancelled(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "slow", response: "ok", delay: 5 * time.Second})

	executor := NewExecutor(reg).WithTimeout(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	result, err := executor.Execute(ctx, &FunctionCall{
		ID:   "call_1",
		Name: "slow",
		Args: `{}`,
	})

	if err == nil {
		t.Error("expected error from cancelled context")
	}
	if result != nil && result.IsError {
		t.Logf("cancelled context returned error result: %s", result.Content)
	}
}

type nilResultTool struct{}

func (n *nilResultTool) Name() string        { return "nil_result" }
func (n *nilResultTool) Description() string { return "returns nil result" }
func (n *nilResultTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (n *nilResultTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return nil, ErrToolExecution
}

func TestExecutor_Execute_ToolReturnsNilResultWithError(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&nilResultTool{})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_1",
		Name: "nil_result",
		Args: `{}`,
	})

	if err == nil {
		t.Error("expected error")
	}
	if result == nil {
		t.Error("executor should create error result when tool returns nil")
	}
	if result != nil && !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_ExecuteBatch_Empty(t *testing.T) {
	reg := NewRegistry()
	executor := NewExecutor(reg)

	results, err := executor.ExecuteBatch(context.Background(), nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty batch, got: %v", results)
	}
}

func TestExecutor_ExecuteBatch_WithNilCall(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", response: "ok"})

	executor := NewExecutor(reg)

	results, err := executor.ExecuteBatch(context.Background(), []*FunctionCall{
		{ID: "1", Name: "echo", Args: `{}`},
		nil,
	})

	if err == nil {
		t.Error("expected error for nil tool call in batch")
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestExecutor_ExecuteBatch_Success(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", response: "ok"})

	executor := NewExecutor(reg)

	results, err := executor.ExecuteBatch(context.Background(), []*FunctionCall{
		{ID: "1", Name: "echo", Args: `{}`},
		{ID: "2", Name: "echo", Args: `{}`},
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if r == nil {
			t.Errorf("result %d is nil", i)
		}
	}
}

func TestExecutor_Execute_MetadataSet(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", response: "ok"})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_1",
		Name: "echo",
		Args: `{}`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata == nil {
		t.Fatal("metadata should be set")
	}
	if _, ok := result.Metadata["duration_ms"]; !ok {
		t.Error("metadata should contain duration_ms")
	}
	if result.Metadata["tool_name"] != "echo" {
		t.Errorf("metadata tool_name should be 'echo', got %v", result.Metadata["tool_name"])
	}
}

func TestExtractPathFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected string
	}{
		{"valid_path", `{"path": "/src/main.go"}`, "/src/main.go"},
		{"no_path", `{"query": "test"}`, ""},
		{"invalid_json", `{invalid}`, ""},
		{"empty_string", `{"path": ""}`, ""},
		{"path_is_number", `{"path": 123}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathFromArgs(tt.args)
			if got != tt.expected {
				t.Errorf("extractPathFromArgs(%q) = %q, want %q", tt.args, got, tt.expected)
			}
		})
	}
}

func TestExecutor_Execute_ScopeDenied_NoPath(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", response: "ok"})

	policy := NewFileScopePolicy()
	policy.SetScope("agent-1", []string{"/src/"})

	executor := NewExecutor(reg).WithScopePolicy(policy, "agent-1")

	// 参数中没有 path 字段，scope 检查应该跳过
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_1",
		Name: "echo",
		Args: `{"query": "test"}`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should succeed when no path in args, got error: %s", result.Content)
	}
}

func TestExecutor_Execute_RequireConfirmation_Logging(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "confirm_tool", response: "ok"})

	perm := Permission{
		RequireConfirmation: true,
		ConfirmFunc:         func(toolName string, args json.RawMessage) bool { return true },
	}
	_ = reg.SetPermission("confirm_tool", perm)

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID:   "call_1",
		Name: "confirm_tool",
		Args: `{}`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should succeed with confirmed callback, got: %s", result.Content)
	}
}
