package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPluginLoader_Load(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader(registry)

	plugin := &mockPlugin{
		name:    "test-plugin",
		version: "1.0.0",
		tools: []Tool{
			&pluginMockTool{name: "tool_a", desc: "Tool A"},
			&pluginMockTool{name: "tool_b", desc: "Tool B"},
		},
	}

	err := loader.Load(plugin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := registry.Get("tool_a"); !ok {
		t.Error("tool_a should be registered")
	}
	if _, ok := registry.Get("tool_b"); !ok {
		t.Error("tool_b should be registered")
	}
}

func TestPluginLoader_LoadDuplicate(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader(registry)

	plugin := &mockPlugin{
		name:    "test-plugin",
		version: "1.0.0",
		tools:   []Tool{&pluginMockTool{name: "tool_a", desc: "Tool A"}},
	}

	if err := loader.Load(plugin); err != nil {
		t.Fatalf("first load failed: %v", err)
	}

	err := loader.Load(plugin)
	if err == nil {
		t.Error("expected error for duplicate plugin load")
	}
}

func TestPluginLoader_Unload(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader(registry)

	plugin := &mockPlugin{
		name:    "test-plugin",
		version: "1.0.0",
		tools:   []Tool{&pluginMockTool{name: "tool_a", desc: "Tool A"}},
	}

	_ = loader.Load(plugin)

	err := loader.Unload("test-plugin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := registry.Get("tool_a"); ok {
		t.Error("tool_a should be unregistered after plugin unload")
	}
}

func TestPluginLoader_UnloadNonExistent(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader(registry)

	err := loader.Unload("nonexistent")
	if err == nil {
		t.Error("expected error for unloading nonexistent plugin")
	}
}

func TestPluginLoader_List(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader(registry)

	_ = loader.Load(&mockPlugin{name: "plugin-a", version: "1.0.0", tools: nil})
	_ = loader.Load(&mockPlugin{name: "plugin-b", version: "2.0.0", tools: nil})

	list := loader.List()
	if len(list) != 2 {
		t.Errorf("List() returned %d plugins, want 2", len(list))
	}

	names := make(map[string]bool)
	for _, info := range list {
		names[info.Name] = true
	}
	if !names["plugin-a"] || !names["plugin-b"] {
		t.Errorf("List() missing expected plugins, got: %+v", list)
	}
}

func TestBuiltinPlugin_Init(t *testing.T) {
	plugin := &BuiltinPlugin{}
	err := plugin.Init(map[string]any{
		"root_dir":  t.TempDir(),
		"enable_fs": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plugin.Name() != "builtin" {
		t.Errorf("Name() = %q, want builtin", plugin.Name())
	}
}

func TestBuiltinPlugin_Tools(t *testing.T) {
	plugin := &BuiltinPlugin{}
	_ = plugin.Init(map[string]any{
		"root_dir":     t.TempDir(),
		"enable_fs":    true,
		"enable_shell": true,
		"enable_utils": true,
	})

	tools := plugin.Tools()
	if len(tools) == 0 {
		t.Error("expected at least one tool from BuiltinPlugin")
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name()] = true
	}
	if !toolNames["filesystem"] {
		t.Error("expected filesystem tool")
	}
}

func TestBuiltinPlugin_Init_NoRootDir(t *testing.T) {
	plugin := &BuiltinPlugin{}
	err := plugin.Init(map[string]any{})
	if err == nil {
		t.Error("expected error when root_dir is missing")
	}
}

func TestNewBuiltinWeb(t *testing.T) {
	tool := newBuiltinWeb()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "web" {
		t.Errorf("expected name 'web', got '%s'", tool.Name())
	}
	if tool.Description() != "Web search and fetch" {
		t.Errorf("expected description 'Web search and fetch', got '%s'", tool.Description())
	}
}

func TestNewBuiltinShell(t *testing.T) {
	tool := newBuiltinShell()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "shell" {
		t.Errorf("expected name 'shell', got '%s'", tool.Name())
	}
}

func TestNewBuiltinCalc(t *testing.T) {
	tool := newBuiltinCalc()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "calculator" {
		t.Errorf("expected name 'calculator', got '%s'", tool.Name())
	}
}

func TestNewBuiltinDateTime(t *testing.T) {
	tool := newBuiltinDateTime()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "datetime" {
		t.Errorf("expected name 'datetime', got '%s'", tool.Name())
	}
}

func TestBuiltinToolAdapter_Execute(t *testing.T) {
	tool := newBuiltinWeb()
	_, err := tool.Execute(context.Background(), nil)
	if err == nil {
		t.Error("expected error from builtin adapter")
	}
}

func TestBuiltinPlugin_Close(t *testing.T) {
	plugin := &BuiltinPlugin{}
	_ = plugin.Init(map[string]any{"root_dir": t.TempDir()})
	err := plugin.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestBuiltinPlugin_Version(t *testing.T) {
	plugin := &BuiltinPlugin{}
	version := plugin.Version()
	if version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", version)
	}
}

func TestNewBuiltinFS(t *testing.T) {
	tool, err := newBuiltinFS(t.TempDir())
	if err != nil {
		t.Fatalf("newBuiltinFS returned error: %v", err)
	}
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name() != "filesystem" {
		t.Errorf("expected name 'filesystem', got '%s'", tool.Name())
	}
}

func TestNewBuiltinFS_InvalidDir(t *testing.T) {
	_, err := newBuiltinFS("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}

func TestBuiltinToolAdapter_Parameters(t *testing.T) {
	tool := &builtinToolAdapter{name: "test", desc: "test tool"}
	params := tool.Parameters()
	if params != nil {
		t.Errorf("expected nil parameters, got %v", params)
	}
}

func TestBuiltinPlugin_Init_WithAllOptions(t *testing.T) {
	plugin := &BuiltinPlugin{}
	err := plugin.Init(map[string]any{
		"root_dir":     t.TempDir(),
		"enable_fs":    true,
		"enable_shell": true,
		"enable_web":   true,
		"enable_utils": true,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	tools := plugin.Tools()
	if len(tools) < 4 {
		t.Errorf("expected at least 4 tools, got %d", len(tools))
	}
}

func TestRegistry_RegisterPlugin(t *testing.T) {
	registry := NewRegistry()
	plugin := &mockPlugin{
		name:    "test-plugin",
		version: "1.0.0",
		tools: []Tool{
			&pluginMockTool{name: "p_tool_1", desc: "Plugin Tool 1"},
			&pluginMockTool{name: "p_tool_2", desc: "Plugin Tool 2"},
		},
	}

	err := registry.RegisterPlugin(plugin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if registry.Count() != 2 {
		t.Errorf("Count() = %d, want 2", registry.Count())
	}
}

func TestRegistry_ToolsByCategory(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(&categorizedTool{name: "fs_read", desc: "Read file", category: "filesystem"})
	_ = registry.Register(&categorizedTool{name: "fs_write", desc: "Write file", category: "filesystem"})
	_ = registry.Register(&categorizedTool{name: "calc", desc: "Calculator", category: "utility"})

	categories := registry.ToolsByCategory()
	if len(categories["filesystem"]) != 2 {
		t.Errorf("filesystem tools = %d, want 2", len(categories["filesystem"]))
	}
	if len(categories["utility"]) != 1 {
		t.Errorf("utility tools = %d, want 1", len(categories["utility"]))
	}
}

func TestRegistry_ToolCategories(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(&categorizedTool{name: "fs_read", desc: "Read file", category: "filesystem"})
	_ = registry.Register(&categorizedTool{name: "calc", desc: "Calculator", category: "utility"})

	cats := registry.ToolCategories()
	if len(cats) != 2 {
		t.Errorf("categories = %d, want 2", len(cats))
	}

	catMap := make(map[string]bool)
	for _, c := range cats {
		catMap[c] = true
	}
	if !catMap["filesystem"] || !catMap["utility"] {
		t.Errorf("missing expected categories, got: %v", cats)
	}
}

func TestPluginLoader_LoadWithInitError(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader(registry)

	plugin := &mockPlugin{
		name:    "failing-plugin",
		version: "1.0.0",
		initErr: ErrInvalidConfig,
	}

	err := loader.Load(plugin)
	if err == nil {
		t.Error("expected error when plugin Init fails")
	}
}

func TestPluginLoader_Close(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader(registry)

	plugin := &mockPlugin{
		name:    "test-plugin",
		version: "1.0.0",
		tools:   []Tool{&pluginMockTool{name: "tool_a", desc: "Tool A"}},
	}

	_ = loader.Load(plugin)
	closed := false
	plugin.closeFunc = func() { closed = true }

	_ = loader.Unload("test-plugin")
	if !closed {
		t.Error("plugin Close() should have been called during Unload")
	}
}

type mockPlugin struct {
	name      string
	version   string
	tools     []Tool
	initErr   error
	closeFunc func()
}

func (p *mockPlugin) Name() string    { return p.name }
func (p *mockPlugin) Version() string { return p.version }
func (p *mockPlugin) Tools() []Tool   { return p.tools }
func (p *mockPlugin) Init(config map[string]any) error {
	if p.initErr != nil {
		return p.initErr
	}
	return nil
}
func (p *mockPlugin) Close() error {
	if p.closeFunc != nil {
		p.closeFunc()
	}
	return nil
}

type pluginMockTool struct {
	name string
	desc string
}

func (t *pluginMockTool) Name() string                { return t.name }
func (t *pluginMockTool) Description() string         { return t.desc }
func (t *pluginMockTool) Parameters() json.RawMessage { return nil }
func (t *pluginMockTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return NewResult("mock result"), nil
}

type categorizedTool struct {
	name     string
	desc     string
	category string
}

func (t *categorizedTool) Name() string                { return t.name }
func (t *categorizedTool) Description() string         { return t.desc }
func (t *categorizedTool) Parameters() json.RawMessage { return nil }
func (t *categorizedTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	return NewResult("result"), nil
}
func (t *categorizedTool) Category() string { return t.category }
