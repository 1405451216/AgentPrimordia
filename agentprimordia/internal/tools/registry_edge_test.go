package tools

import (
	"encoding/json"
	"testing"
)

func TestRegistry_DuplicateRegister(t *testing.T) {
	reg := NewRegistry()
	tool1 := &mockTool{name: "dup_tool", description: "first", response: "v1"}
	tool2 := &mockTool{name: "dup_tool", description: "second", response: "v2"}

	if err := reg.Register(tool1); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := reg.Register(tool2); err != nil {
		t.Fatalf("duplicate register should be idempotent, got: %v", err)
	}
	if reg.Count() != 1 {
		t.Errorf("expected count 1 after duplicate, got %d", reg.Count())
	}

	got, exists := reg.Get("dup_tool")
	if !exists {
		t.Fatal("tool should exist")
	}
	// 重复注册应该覆盖
	if got.Description() != "second" {
		t.Errorf("expected description 'second' after overwrite, got '%s'", got.Description())
	}
}

func TestRegistry_Unregister_NotSupported(t *testing.T) {
	// Registry 当前没有 Unregister 方法，此测试验证行为一致性
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "keep", response: "ok"})

	if reg.Count() != 1 {
		t.Errorf("expected 1 tool, got %d", reg.Count())
	}

	// 验证工具仍然存在
	_, exists := reg.Get("keep")
	if !exists {
		t.Error("tool should still exist (no unregister method)")
	}
}

func TestRegistry_ListEmpty(t *testing.T) {
	reg := NewRegistry()

	names := reg.List()
	if names == nil {
		t.Error("List() should return empty slice, not nil")
	}
	if len(names) != 0 {
		t.Errorf("expected empty list, got %d items", len(names))
	}

	count := reg.Count()
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	defs := reg.Definitions()
	if len(defs) != 0 {
		t.Errorf("expected empty definitions, got %d", len(defs))
	}
}

func TestRegistry_RegisterEmptyName(t *testing.T) {
	reg := NewRegistry()
	tool := &mockTool{name: "", description: "no name", response: "ok"}

	err := reg.Register(tool)
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig for empty name, got: %v", err)
	}
}

func TestRegistry_RegisterMultiple_WithInvalid(t *testing.T) {
	reg := NewRegistry()

	err := reg.RegisterMultiple(
		&mockTool{name: "valid", response: "ok"},
		&mockTool{name: "", response: "ok"},
	)
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}

	// 第一个有效工具应该已注册
	if reg.Count() != 1 {
		t.Errorf("expected 1 tool (valid one), got %d", reg.Count())
	}
}

func TestRegistry_SetPermission_NonExistent(t *testing.T) {
	reg := NewRegistry()

	err := reg.SetPermission("nonexistent", Permission{RequireConfirmation: true})
	if err != ErrToolNotFound {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
}

func TestRegistry_GetPermission_NonExistent(t *testing.T) {
	reg := NewRegistry()

	_, exists := reg.GetPermission("nonexistent")
	if exists {
		t.Error("permission should not exist for unregistered tool")
	}
}

func TestRegistry_GetPermission_DefaultPermission(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "tool_with_perm", response: "ok"})

	perm, exists := reg.GetPermission("tool_with_perm")
	if !exists {
		t.Error("default permission should exist after registration")
	}
	if perm == nil {
		t.Fatal("permission should not be nil")
	}
	if perm.RequireConfirmation {
		t.Error("default permission should not require confirmation")
	}
}

func TestRegistry_Definitions_NilParameters(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "nil_params", description: "No params", params: nil, response: "ok"})

	defs := reg.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}

	fn, ok := defs[0]["function"].(map[string]any)
	if !ok {
		t.Fatal("function should be map")
	}
	if fn["name"] != "nil_params" {
		t.Errorf("expected name 'nil_params', got '%v'", fn["name"])
	}
}

func TestRegistry_Definitions_WithParameters(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{
		name:        "param_tool",
		description: "Has params",
		params:      json.RawMessage(`{"type":"object","properties":{"x":{"type":"number"}}}`),
		response:    "ok",
	})

	defs := reg.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}

	fn, ok := defs[0]["function"].(map[string]any)
	if !ok {
		t.Fatal("function should be map")
	}
	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatal("parameters should be map")
	}
	if params["type"] != "object" {
		t.Errorf("expected type 'object', got '%v'", params["type"])
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry()

	done := make(chan bool, 100)

	for i := 0; i < 50; i++ {
		go func(idx int) {
			name := string(rune('a' + idx%26))
			_ = reg.Register(&mockTool{name: name, response: "ok"})
			done <- true
		}(i)
	}

	for i := 0; i < 50; i++ {
		go func() {
			reg.List()
			reg.Count()
			reg.Definitions()
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}
