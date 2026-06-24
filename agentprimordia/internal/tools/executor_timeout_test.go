package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestExecutor_ToolTimeout(t *testing.T) {
	registry := NewRegistry()
	slowTool := &mockTimeoutTool{
		name:  "slow",
		delay: 5 * time.Second,
	}
	registry.Register(slowTool)

	exec := NewExecutorWithConfig(registry, ExecutorConfig{
		DefaultTimeout: 100 * time.Millisecond,
	})

	ctx := context.Background()
	_, err := exec.Execute(ctx, &FunctionCall{
		ID:   "call-1",
		Name: "slow",
		Args: "{}",
	})

	if err == nil {
		t.Fatal("期望超时错误")
	}
	// 超时可能返回 context.DeadlineExceeded 或包含它的包装错误
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("错误 = %v, 期望包含 DeadlineExceeded", err)
	}
}

func TestExecutor_PerToolTimeout(t *testing.T) {
	registry := NewRegistry()
	fastTool := &mockTimeoutTool{
		name:  "fast",
		delay: 50 * time.Millisecond,
	}
	slowTool := &mockTimeoutTool{
		name:  "slow",
		delay: 5 * time.Second,
	}
	registry.Register(fastTool)
	registry.Register(slowTool)

	exec := NewExecutorWithConfig(registry, ExecutorConfig{
		DefaultTimeout: 200 * time.Millisecond,
		PerToolTimeout: map[string]time.Duration{
			"fast": 1 * time.Second,   // fast 工具允许更长时间
			"slow": 50 * time.Millisecond, // slow 工具超时更短
		},
	})

	// fast 工具应在 200ms 内完成（50ms delay < 1s per-tool timeout）
	_, err := exec.Execute(context.Background(), &FunctionCall{
		ID:   "call-1",
		Name: "fast",
		Args: "{}",
	})
	if err != nil {
		t.Errorf("fast 工具不应超时: %v", err)
	}

	// slow 工具应超时（5s delay > 50ms per-tool timeout）
	_, err = exec.Execute(context.Background(), &FunctionCall{
		ID:   "call-2",
		Name: "slow",
		Args: "{}",
	})
	if err == nil {
		t.Fatal("slow 工具应超时")
	}
}

// mockTimeoutTool 用于测试超时的模拟工具
type mockTimeoutTool struct {
	name  string
	delay time.Duration
}

func (m *mockTimeoutTool) Name() string                      { return m.name }
func (m *mockTimeoutTool) Description() string               { return "mock tool" }
func (m *mockTimeoutTool) Parameters() json.RawMessage       { return nil }
func (m *mockTimeoutTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	select {
	case <-ctx.Done():
		return NewErrorResult(ctx.Err().Error()), ctx.Err()
	case <-time.After(m.delay):
		return &Result{Content: "done"}, nil
	}
}
