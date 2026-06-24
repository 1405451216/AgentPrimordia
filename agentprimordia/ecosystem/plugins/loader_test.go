package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// createTestPluginDir 在临时目录中创建测试插件目录结构
// tempdir/
//
//	test-plugin/
//	  plugin.json
func createTestPluginDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("创建插件目录失败: %v", err)
	}

	manifest := Plugin{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Type:        PluginTypeTool,
		Description: "测试插件",
		Author:      "tester",
		Entry:       "main.go",
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("序列化 plugin.json 失败: %v", err)
	}

	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("写入 plugin.json 失败: %v", err)
	}

	return dir
}

// TestPluginLoader_Discover 测试从目录中发现插件
func TestPluginLoader_Discover(t *testing.T) {
	dir := createTestPluginDir(t)

	loader := NewLoader(LoaderConfig{PluginDir: dir})
	plugins, err := loader.Discover()
	if err != nil {
		t.Fatalf("Discover() 报错: %v", err)
	}

	if len(plugins) != 1 {
		t.Fatalf("Discover() 返回 %d 个插件, want 1", len(plugins))
	}

	p := plugins[0]
	if p.Name != "test-plugin" {
		t.Errorf("Name = %q, want %q", p.Name, "test-plugin")
	}
	if p.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", p.Version, "1.0.0")
	}
	if p.Type != PluginTypeTool {
		t.Errorf("Type = %q, want %q", p.Type, PluginTypeTool)
	}
	if p.Description != "测试插件" {
		t.Errorf("Description = %q, want %q", p.Description, "测试插件")
	}
	if p.Author != "tester" {
		t.Errorf("Author = %q, want %q", p.Author, "tester")
	}
	if p.Entry != "main.go" {
		t.Errorf("Entry = %q, want %q", p.Entry, "main.go")
	}
}

// TestPluginLoader_Load 测试按名称加载特定插件
func TestPluginLoader_Load(t *testing.T) {
	dir := createTestPluginDir(t)

	loader := NewLoader(LoaderConfig{PluginDir: dir})
	p, err := loader.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load() 报错: %v", err)
	}

	if p.Name != "test-plugin" {
		t.Errorf("Name = %q, want %q", p.Name, "test-plugin")
	}
	if p.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", p.Version, "1.0.0")
	}
	if p.Type != PluginTypeTool {
		t.Errorf("Type = %q, want %q", p.Type, PluginTypeTool)
	}
}

// TestPluginLoader_LoadNotFound 测试加载不存在的插件应返回错误
func TestPluginLoader_LoadNotFound(t *testing.T) {
	dir := t.TempDir()

	loader := NewLoader(LoaderConfig{PluginDir: dir})
	_, err := loader.Load("nonexistent")
	if err == nil {
		t.Fatal("Load() 对不存在的插件应返回错误, got nil")
	}
}

// TestPluginLoader_Validate 测试插件元数据校验（合法数据应通过）
func TestPluginLoader_Validate(t *testing.T) {
	p := Plugin{
		Name:    "valid-plugin",
		Version: "1.0.0",
		Type:    PluginTypeTool,
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() 对合法插件应返回 nil, got %v", err)
	}
}

// TestPlugin_Validate_Invalid 测试空名称的插件应校验失败
func TestPlugin_Validate_Invalid(t *testing.T) {
	p := Plugin{
		Name:    "",
		Version: "1.0.0",
		Type:    PluginTypeTool,
	}

	if err := p.Validate(); err == nil {
		t.Fatal("Validate() 对空名称应返回错误, got nil")
	}
}
