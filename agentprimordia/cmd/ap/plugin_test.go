package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ===== Plugin list 测试 =====

func TestPluginList_Empty(t *testing.T) {
	setupTestProject(t)

	err := pluginList()
	if err != nil {
		t.Fatalf("pluginList 失败: %v", err)
	}
}

func TestPluginList_WithPlugins(t *testing.T) {
	dir := setupTestProject(t)

	apYaml := `name: test
plugins:
  - github.com/user/ap-plugin-slack
  - github.com/user/ap-plugin-weather
`
	os.WriteFile(filepath.Join(dir, ".ap.yaml"), []byte(apYaml), 0o644)

	err := pluginList()
	if err != nil {
		t.Fatalf("pluginList 失败: %v", err)
	}
}

// ===== Plugin create 测试 =====

func TestPluginCreate_Success(t *testing.T) {
	dir := setupTestProject(t)

	err := pluginCreate([]string{"ap-plugin-test"})
	if err != nil {
		t.Fatalf("pluginCreate 失败: %v", err)
	}

	// 验证目录结构
	pluginDir := filepath.Join(dir, "ap-plugin-test")
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		t.Fatal("插件目录未创建")
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.go")); os.IsNotExist(err) {
		t.Error("plugin.go 未创建")
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "go.mod")); os.IsNotExist(err) {
		t.Error("go.mod 未创建")
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "README.md")); os.IsNotExist(err) {
		t.Error("README.md 未创建")
	}

	// 验证 go.mod 版本
	modContent, _ := os.ReadFile(filepath.Join(pluginDir, "go.mod"))
	if !contains(string(modContent), "go 1.23") {
		t.Error("plugin go.mod 应包含 go 1.23")
	}

	// 验证 plugin.go 内容
	pluginCode, _ := os.ReadFile(filepath.Join(pluginDir, "plugin.go"))
	if !contains(string(pluginCode), "package ap_plugin_test") {
		t.Error("plugin.go 应包含 package ap_plugin_test")
	}
	if !contains(string(pluginCode), "ap-plugin-test") {
		t.Error("plugin.go 应包含插件名称 ap-plugin-test")
	}
}

func TestPluginCreate_DirExists(t *testing.T) {
	dir := setupTestProject(t)

	// 创建已存在的目录
	os.Mkdir(filepath.Join(dir, "existing-plugin"), 0o755)

	err := pluginCreate([]string{"existing-plugin"})
	if err == nil {
		t.Fatal("期望返回错误（目录已存在），实际返回 nil")
	}
}

func TestPluginCreate_NoName(t *testing.T) {
	setupTestProject(t)

	err := pluginCreate([]string{})
	if err == nil {
		t.Fatal("期望返回错误（未指定名称），实际返回 nil")
	}
}

// ===== Plugin remove 测试 =====

func TestPluginRemove_NotInstalled(t *testing.T) {
	setupTestProject(t)

	err := pluginRemove([]string{"github.com/nonexistent/plugin"})
	if err == nil {
		t.Fatal("期望返回错误（插件未安装），实际返回 nil")
	}
}

func TestPluginRemove_NoName(t *testing.T) {
	setupTestProject(t)

	err := pluginRemove([]string{})
	if err == nil {
		t.Fatal("期望返回错误（未指定名称），实际返回 nil")
	}
}

func TestPluginRemove_Success(t *testing.T) {
	dir := setupTestProject(t)

	// 写入含插件的配置
	apYaml := `name: test
plugins:
  - github.com/user/ap-plugin-test
`
	os.WriteFile(filepath.Join(dir, ".ap.yaml"), []byte(apYaml), 0o644)

	err := pluginRemove([]string{"github.com/user/ap-plugin-test"})
	if err != nil {
		t.Fatalf("pluginRemove 失败: %v", err)
	}

	// 验证已从配置中移除
	config := loadAPConfig()
	for _, p := range config.Plugins {
		if p == "github.com/user/ap-plugin-test" {
			t.Error("插件应已从配置中移除")
		}
	}
}

// ===== Plugin dispatch 测试 =====

func TestRunPlugin_NoArgs(t *testing.T) {
	err := runPlugin([]string{})
	if err != nil {
		t.Errorf("无参数应返回 nil（显示帮助），实际: %v", err)
	}
}

func TestRunPlugin_UnknownSubcommand(t *testing.T) {
	err := runPlugin([]string{"unknown"})
	if err == nil {
		t.Fatal("期望返回错误（未知子命令），实际返回 nil")
	}
}
