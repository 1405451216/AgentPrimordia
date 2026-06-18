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

func TestRegistry_Definitions_CacheOverwrite(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "cached", description: "first", response: "ok"})

	defs1 := reg.Definitions()
	if len(defs1) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs1))
	}

	// 覆盖注册，验证缓存更新
	_ = reg.Register(&mockTool{name: "cached", description: "second", response: "ok"})
	defs2 := reg.Definitions()
	fn, ok := defs2[0]["function"].(map[string]any)
	if !ok {
		t.Fatal("function should be map")
	}
	if fn["description"] != "second" {
		t.Errorf("expected description 'second' after overwrite, got '%v'", fn["description"])
	}

	// 验证返回的是深拷贝：修改 defs1 不影响 defs2
	defs1[0]["type"] = "modified"
	if defs2[0]["type"] != "function" {
		t.Error("Definitions 应该返回深拷贝，避免调用者污染缓存")
	}
}

func TestRegistry_Definitions_CacheIsolation(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{
		name:        "iso",
		description: "iso tool",
		params:      json.RawMessage(`{"type":"object","properties":{"x":{"type":"number"}}}`),
		response:    "ok",
	})

	defs := reg.Definitions()
	fn, _ := defs[0]["function"].(map[string]any)
	params, _ := fn["parameters"].(map[string]any)
	params["type"] = "modified"

	defs2 := reg.Definitions()
	fn2, _ := defs2[0]["function"].(map[string]any)
	params2, _ := fn2["parameters"].(map[string]any)
	if params2["type"] != "object" {
		t.Error("parameters 也应该被深拷贝")
	}
}

// TestRegistry_TypeAssertionSafety 验证 Registry 内部 sync.Map 若被异常值污染，
// 各查询方法不会 panic，而是跳过或返回 false。
func TestRegistry_TypeAssertionSafety(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&mockTool{name: "valid", response: "ok"})

	// 模拟异常值污染
	reg.tools.Store("bad-tool", "not a Tool")
	reg.tools.Store(123, &mockTool{name: "int-key", response: "ok"}) // 非 string key
	reg.toolDefs.Store("bad-def", "not a map")
	reg.permissions.Store("bad-perm", "not a Permission")

	// Get 应返回 false 而不是 panic
	if _, exists := reg.Get("bad-tool"); exists {
		t.Error("Get should return false for non-Tool value")
	}
	if _, exists := reg.GetPermission("bad-perm"); exists {
		t.Error("GetPermission should return false for non-Permission value")
	}

	// List 应跳过非 string key（顺序不保证）
	names := reg.List()
	hasValid := false
	for _, n := range names {
		if n == "valid" {
			hasValid = true
		}
	}
	if !hasValid {
		t.Errorf("List should include valid key, got %v", names)
	}

	// Definitions 应跳过异常 def
	defs := reg.Definitions()
	if len(defs) != 1 {
		t.Errorf("Definitions should skip invalid defs, got %d", len(defs))
	}

	// ToolsByCategory / ToolCategories 应跳过异常 tool
	byCat := reg.ToolsByCategory()
	if len(byCat) != 1 {
		t.Errorf("ToolsByCategory should skip invalid tools, got %v", byCat)
	}
	cats := reg.ToolCategories()
	if len(cats) != 1 {
		t.Errorf("ToolCategories should skip invalid tools, got %v", cats)
	}
}

func BenchmarkRegistry_Definitions(b *testing.B) {
	reg := NewRegistry()
	for i := 0; i < 20; i++ {
		name := "tool_" + string(rune('a'+i%26))
		_ = reg.Register(&mockTool{
			name:        name,
			description: "benchmark tool",
			params:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			response:    "ok",
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.Definitions()
	}
}
