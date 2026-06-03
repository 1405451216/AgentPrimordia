package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type mockTool struct {
	name        string
	description string
	params      json.RawMessage
	response    string
	shouldFail  bool
	delay       time.Duration
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) Parameters() json.RawMessage {
	if m.params != nil {
		return m.params
	}
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)
}
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.shouldFail {
		return NewErrorResult("intentional failure"), nil
	}
	return NewResult(m.response), nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	tool := &mockTool{name: "test_tool", description: "A test tool", response: "ok"}

	err := reg.Register(tool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, exists := reg.Get("test_tool")
	if !exists {
		t.Fatal("tool should exist after registration")
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected 'test_tool', got '%s'", got.Name())
	}
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	reg := NewRegistry()
	tool := &mockTool{name: "dup", response: "ok"}

	_ = reg.Register(tool)
	err := reg.Register(tool)
	if err != nil {
		t.Errorf("duplicate should be no-op, got: %v", err)
	}
	if reg.Count() != 1 {
		t.Errorf("expected count 1, got %d", reg.Count())
	}
}

func TestRegistry_ListAndCount(t *testing.T) {
	reg := NewRegistry()
	_ = reg.RegisterMultiple(
		&mockTool{name: "a", response: "a"},
		&mockTool{name: "b", response: "b"},
		&mockTool{name: "c", response: "c"},
	)

	if reg.Count() != 3 {
		t.Errorf("expected 3, got %d", reg.Count())
	}
	names := reg.List()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}
}

func TestRegistry_GetNonExistent(t *testing.T) {
	reg := NewRegistry()
	_, exists := reg.Get("nonexistent")
	if exists {
		t.Error("should not exist")
	}
}

func TestRegistry_Definitions(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "weather", description: "Get weather", response: "sunny"})

	defs := reg.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	fn, ok := defs[0]["function"].(map[string]any)
	if !ok {
		t.Fatal("function should be map")
	}
	if fn["name"] != "weather" {
		t.Errorf("expected 'weather', got '%v'", fn["name"])
	}
}

func TestRegistry_Permissions(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "secure", response: "ok"})

	err := reg.SetPermission("secure", Permission{RequireConfirmation: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perm, exists := reg.GetPermission("secure")
	if !exists {
		t.Fatal("permission should exist")
	}
	if !perm.RequireConfirmation {
		t.Error("should be true")
	}
}

func TestExecutor_ExecuteSuccess(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "echo", description: "Echo", response: "hello!"})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "echo", Args: `{"query":"test"}`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should not be error, content: %s", result.Content)
	}
	if result.Content != "hello!" {
		t.Errorf("expected 'hello!', got '%s'", result.Content)
	}
}

func TestExecutor_ExecuteNotFound(t *testing.T) {
	reg := NewRegistry()
	executor := NewExecutor(reg)

	result, err := executor.Execute(context.Background(), &FunctionCall{
		Name: "nonexistent", Args: `{}`,
	})

	if err == nil {
		t.Error("expected error")
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_ExecuteTimeout(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "slow", response: "ok", delay: 200 * time.Millisecond})

	executor := NewExecutor(reg).WithTimeout(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := executor.Execute(ctx, &FunctionCall{Name: "slow", Args: `{}`})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestNewResultHelpers(t *testing.T) {
	success := NewResult("good")
	if success.IsError {
		t.Error("should not be error")
	}
	fail := NewErrorResult("bad")
	if !fail.IsError {
		t.Error("should be error")
	}
}

// ===== 确认回调测试 =====

func TestExecutor_ConfirmationRequired_Denied(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "dangerous", description: "Dangerous", response: "boom"})

	// 设置需要确认但没有回调
	_ = reg.SetPermission("dangerous", Permission{RequireConfirmation: true})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "dangerous", Args: `{}`,
	})

	if err != ErrConfirmDenied {
		t.Errorf("expected ErrConfirmDenied, got: %v", err)
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_ConfirmationCallback_Accepted(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "safe_op", description: "Safe Op", response: "done"})

	// 设置确认回调，始终允许
	_ = reg.SetPermission("safe_op", Permission{
		RequireConfirmation: true,
		ConfirmFunc: func(toolName string, args json.RawMessage) bool {
			return true
		},
	})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "safe_op", Args: `{}`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should not be error, content: %s", result.Content)
	}
}

func TestExecutor_ConfirmationCallback_Rejected(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "risky_op", description: "Risky Op", response: "oops"})

	// 设置确认回调，始终拒绝
	_ = reg.SetPermission("risky_op", Permission{
		RequireConfirmation: true,
		ConfirmFunc: func(toolName string, args json.RawMessage) bool {
			return false
		},
	})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "risky_op", Args: `{}`,
	})

	if err != ErrConfirmDenied {
		t.Errorf("expected ErrConfirmDenied, got: %v", err)
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_ConfirmationConditional(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "cond_op", description: "Conditional Op", response: "ok"})

	// 条件性确认：只允许特定参数
	_ = reg.SetPermission("cond_op", Permission{
		RequireConfirmation: true,
		ConfirmFunc: func(toolName string, args json.RawMessage) bool {
			var params map[string]any
			_ = json.Unmarshal(args, &params)
			if mode, ok := params["mode"]; ok && mode == "safe" {
				return true
			}
			return false
		},
	})

	executor := NewExecutor(reg)

	// 安全模式 - 允许
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "cond_op", Args: `{"mode":"safe"}`,
	})
	if err != nil {
		t.Fatalf("safe mode should be allowed, got: %v", err)
	}
	if result.IsError {
		t.Error("safe mode result should not be error")
	}

	// 危险模式 - 拒绝
	result2, err2 := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_2", Name: "cond_op", Args: `{"mode":"danger"}`,
	})
	if err2 != ErrConfirmDenied {
		t.Errorf("danger mode should be denied, got: %v", err2)
	}
	if !result2.IsError {
		t.Error("danger mode result should be error")
	}
}
