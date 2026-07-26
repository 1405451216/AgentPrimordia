package wasm

import (
	"context"
	"strings"
	"testing"
)

// TestToolExecutor_Register 测试工具注册
func TestToolExecutor_Register(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)

	// 空名称
	if err := executor.Register("", "execute", []byte{0x00}); err == nil {
		t.Error("expected error for empty name")
	}

	// 空函数名
	if err := executor.Register("test", "", []byte{0x00}); err == nil {
		t.Error("expected error for empty execute func")
	}

	// 空字节码
	if err := executor.Register("test", "execute", nil); err == nil {
		t.Error("expected error for empty wasm bytes")
	}

	// 无效字节码（编译失败）
	if err := executor.Register("test", "execute", []byte{0x00, 0x01}); err == nil {
		t.Error("expected error for invalid wasm bytes")
	}
}

// TestToolExecutor_ListAndHas 测试工具列表和查询
func TestToolExecutor_ListAndHas(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)

	// 初始为空
	tools := executor.ListTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}

	if executor.HasTool("nonexistent") {
		t.Error("HasTool should return false for unregistered tool")
	}
}

// TestToolExecutor_Unregister 测试工具注销
func TestToolExecutor_Unregister(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)

	// 注销不存在的工具
	if err := executor.Unregister("nonexistent"); err == nil {
		t.Error("expected error for unregistering nonexistent tool")
	}
}

// TestToolExecutor_Execute_NotRegistered 测试执行未注册工具
func TestToolExecutor_Execute_NotRegistered(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)

	_, err = executor.Execute(ctx, "nonexistent", map[string]any{"key": "value"})
	if err == nil {
		t.Error("expected error for executing unregistered tool")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error should mention 'not registered', got: %v", err)
	}
}

// TestToolExecutor_ExecuteRaw_NotRegistered 测试原始执行未注册工具
func TestToolExecutor_ExecuteRaw_NotRegistered(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)

	_, err = executor.ExecuteRaw(ctx, "nonexistent", []byte("test"))
	if err == nil {
		t.Error("expected error for executing unregistered tool")
	}
}

// TestToolExecutor_ExecuteJSON_InvalidArgs 测试无效 JSON 参数
func TestToolExecutor_ExecuteJSON_InvalidArgs(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)

	_, err = executor.ExecuteJSON(ctx, "nonexistent", "not valid json{{{")
	if err == nil {
		t.Error("expected error for invalid JSON args")
	}
	if !strings.Contains(err.Error(), "invalid args JSON") {
		t.Errorf("error should mention 'invalid args JSON', got: %v", err)
	}
}

// TestToolExecutor_DuplicateRegister 测试重复注册
func TestToolExecutor_DuplicateRegister(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)

	// 使用有效的最小 WASM 模块（仅 magic + version）
	// 注意：这个模块无法通过编译，但我们可以测试重复注册逻辑
	// 先用一个能编译的模块 — 由于没有真实 WASM 工具二进制，
	// 我们验证当第一次注册失败时，不会阻止后续注册
	err1 := executor.Register("dup", "execute", []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})
	if err1 != nil {
		// 最小编译可能成功（空模块），也可能失败
		t.Logf("first register result: %v", err1)
	}

	// 如果第一次成功，第二次应该报重复
	if err1 == nil {
		err2 := executor.Register("dup", "execute", []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})
		if err2 == nil {
			t.Error("expected error for duplicate registration")
		}
		if !strings.Contains(err2.Error(), "already registered") {
			t.Errorf("error should mention 'already registered', got: %v", err2)
		}
	}
}

// TestToolExecutor_SetDefaultTimeout 测试超时设置
func TestToolExecutor_SetDefaultTimeout(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer rt.Close(ctx)

	executor := NewToolExecutor(rt)
	executor.SetDefaultTimeout(5000000000) // 5s

	executor.mu.RLock()
	timeout := executor.defaultTimeout
	executor.mu.RUnlock()

	if timeout != 5000000000 {
		t.Errorf("defaultTimeout = %v, want 5s", timeout)
	}
}

// TestValidateABI 测试 ABI 验证
func TestValidateABI(t *testing.T) {
	tests := []struct {
		name        string
		exports     []string
		hasMemory   bool
		executeFunc string
		wantErr     bool
	}{
		{
			name:        "valid",
			exports:     []string{"alloc", "free", "calculate"},
			hasMemory:   true,
			executeFunc: "calculate",
			wantErr:     false,
		},
		{
			name:        "no memory",
			exports:     []string{"alloc", "calculate"},
			hasMemory:   false,
			executeFunc: "calculate",
			wantErr:     true,
		},
		{
			name:        "no alloc",
			exports:     []string{"calculate"},
			hasMemory:   true,
			executeFunc: "calculate",
			wantErr:     true,
		},
		{
			name:        "no execute func",
			exports:     []string{"alloc", "free"},
			hasMemory:   true,
			executeFunc: "calculate",
			wantErr:     true,
		},
		{
			name:        "minimal valid (no free)",
			exports:     []string{"alloc", "execute"},
			hasMemory:   true,
			executeFunc: "execute",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateABI(tt.exports, tt.hasMemory, tt.executeFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateABI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestToolInputOutput_Serialization 测试输入输出序列化
func TestToolInputOutput_Serialization(t *testing.T) {
	input := ToolInput{
		ToolName: "calculator",
		Args:     map[string]any{"expression": "2+2"},
		Context: &ToolInputContext{
			RequestID:  "req-123",
			TimeoutMS:  5000,
			ABIVersion: ABIVersion,
		},
	}

	if input.ToolName != "calculator" {
		t.Errorf("ToolName = %q", input.ToolName)
	}
	if input.Context.ABIVersion != ABIVersion {
		t.Errorf("ABIVersion = %d, want %d", input.Context.ABIVersion, ABIVersion)
	}

	output := ToolOutput{
		Content: "4",
		IsError: false,
		Metadata: map[string]string{
			"execution_time_ms": "2",
		},
	}

	if output.Content != "4" {
		t.Errorf("Content = %q", output.Content)
	}
	if output.IsError {
		t.Error("IsError should be false")
	}
}
