package main

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestProject 创建临时项目目录（含 go.mod），返回路径并 chdir 到该目录。
// 调用方需 defer os.Chdir(origDir) 恢复。
func setupTestProject(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n"), 0o644)
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	_ = os.Chdir(tmpDir)
	return tmpDir
}

// ===== MCP list 测试 =====

func TestMCPList_Empty(t *testing.T) {
	setupTestProject(t)

	err := mcpList()
	if err != nil {
		t.Fatalf("mcpList 失败: %v", err)
	}
}

func TestMCPList_WithServers(t *testing.T) {
	dir := setupTestProject(t)

	// 写入包含 MCP server 的 .ap.yaml
	apYaml := `name: test
mcp:
  servers:
    filesystem:
      command: npx
      args:
        - "@modelcontextprotocol/server-filesystem"
        - "/tmp"
      auto_start: true
    remote:
      base_url: "http://localhost:3001/mcp"
`
	_ = os.WriteFile(filepath.Join(dir, ".ap.yaml"), []byte(apYaml), 0o644)

	err := mcpList()
	if err != nil {
		t.Fatalf("mcpList 失败: %v", err)
	}
}

// ===== MCP add 测试 =====

func TestMCPAdd_NoName(t *testing.T) {
	setupTestProject(t)

	err := mcpAdd([]string{})
	if err == nil {
		t.Fatal("期望返回错误（未指定 server name），实际返回 nil")
	}
}

func TestMCPAdd_CommandOnly(t *testing.T) {
	setupTestProject(t)

	err := mcpAdd([]string{"filesystem", "--command", "npx", "--args", "@mcp/server-filesystem,/tmp"})
	if err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	// 验证配置已保存
	config := loadAPConfig()
	srv, ok := config.MCP.Servers["filesystem"]
	if !ok {
		t.Fatal("MCP server 'filesystem' 未保存到配置")
	}
	if srv.Command != "npx" {
		t.Errorf("Command 期望 npx，实际 %s", srv.Command)
	}
}

func TestMCPAdd_URLOnly(t *testing.T) {
	setupTestProject(t)

	err := mcpAdd([]string{"remote", "--url", "http://localhost:3001/mcp"})
	if err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	config := loadAPConfig()
	srv, ok := config.MCP.Servers["remote"]
	if !ok {
		t.Fatal("MCP server 'remote' 未保存到配置")
	}
	if srv.BaseURL != "http://localhost:3001/mcp" {
		t.Errorf("BaseURL 期望 http://localhost:3001/mcp，实际 %s", srv.BaseURL)
	}
}

func TestMCPAdd_NoCommandOrURL(t *testing.T) {
	setupTestProject(t)

	err := mcpAdd([]string{"bad-server"})
	if err == nil {
		t.Fatal("期望返回错误（未提供 --command 或 --url），实际返回 nil")
	}
}

func TestMCPAdd_AutoStart(t *testing.T) {
	setupTestProject(t)

	err := mcpAdd([]string{"fs", "--command", "npx", "--auto-start"})
	if err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	config := loadAPConfig()
	srv := config.MCP.Servers["fs"]
	if !srv.AutoStart {
		t.Error("AutoStart 应为 true")
	}
}

func TestMCPAdd_EnvVars(t *testing.T) {
	setupTestProject(t)

	err := mcpAdd([]string{"fs", "--command", "npx", "--env", "API_KEY=secret,DEBUG=1"})
	if err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	config := loadAPConfig()
	srv := config.MCP.Servers["fs"]
	if srv.Env["API_KEY"] != "secret" {
		t.Errorf("Env[API_KEY] 期望 secret，实际 %s", srv.Env["API_KEY"])
	}
	if srv.Env["DEBUG"] != "1" {
		t.Errorf("Env[DEBUG] 期望 1，实际 %s", srv.Env["DEBUG"])
	}
}

// ===== MCP remove 测试 =====

func TestMCPRemove_Success(t *testing.T) {
	setupTestProject(t)

	// 先添加
	if err := mcpAdd([]string{"temp-server", "--command", "npx"}); err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	// 再删除
	err := mcpRemove([]string{"temp-server"})
	if err != nil {
		t.Fatalf("mcpRemove 失败: %v", err)
	}

	// 验证已删除
	config := loadAPConfig()
	if _, ok := config.MCP.Servers["temp-server"]; ok {
		t.Error("server 应已被删除")
	}
}

func TestMCPRemove_NotFound(t *testing.T) {
	setupTestProject(t)

	err := mcpRemove([]string{"nonexistent"})
	if err == nil {
		t.Fatal("期望返回错误（server 不存在），实际返回 nil")
	}
}

func TestMCPRemove_NoName(t *testing.T) {
	setupTestProject(t)

	err := mcpRemove([]string{})
	if err == nil {
		t.Fatal("期望返回错误（未指定 name），实际返回 nil")
	}
}

// ===== MCP test 测试 =====

func TestMCPTest_NotFound(t *testing.T) {
	setupTestProject(t)

	err := mcpTest([]string{"nonexistent"})
	if err == nil {
		t.Fatal("期望返回错误（server 不存在），实际返回 nil")
	}
}

func TestMCPTest_NoURL(t *testing.T) {
	setupTestProject(t)

	// 添加一个无 URL 的 server
	if err := mcpAdd([]string{"cmd-only", "--command", "npx"}); err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	err := mcpTest([]string{"cmd-only"})
	if err == nil {
		t.Fatal("期望返回错误（无 URL），实际返回 nil")
	}
}

func TestMCPTest_NoName(t *testing.T) {
	setupTestProject(t)

	err := mcpTest([]string{})
	if err == nil {
		t.Fatal("期望返回错误（未指定 name），实际返回 nil")
	}
}

// ===== MCP start 测试 =====

func TestMCPStart_NotFound(t *testing.T) {
	setupTestProject(t)

	err := mcpStart([]string{"nonexistent"})
	if err == nil {
		t.Fatal("期望返回错误（server 不存在），实际返回 nil")
	}
}

func TestMCPStart_NoName(t *testing.T) {
	setupTestProject(t)

	err := mcpStart([]string{})
	if err == nil {
		t.Fatal("期望返回错误（未指定 name），实际返回 nil")
	}
}

// ===== MCP stop 测试 =====

func TestMCPStop_NotFound(t *testing.T) {
	setupTestProject(t)

	err := mcpStop([]string{"nonexistent"})
	if err == nil {
		t.Fatal("期望返回错误（server 不存在），实际返回 nil")
	}
}

func TestMCPStop_NoName(t *testing.T) {
	setupTestProject(t)

	err := mcpStop([]string{})
	if err == nil {
		t.Fatal("期望返回错误（未指定 name），实际返回 nil")
	}
}

func TestMCPStop_DisableAutoStart(t *testing.T) {
	setupTestProject(t)

	// 添加一个 auto-start server
	if err := mcpAdd([]string{"fs", "--command", "npx", "--auto-start"}); err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	// stop 应禁用 auto-start
	err := mcpStop([]string{"fs"})
	if err != nil {
		t.Fatalf("mcpStop 失败: %v", err)
	}

	config := loadAPConfig()
	srv := config.MCP.Servers["fs"]
	if srv.AutoStart {
		t.Error("stop 后 AutoStart 应为 false")
	}
}

// ===== MCP tools 测试 =====

func TestMCPTools_NotFound(t *testing.T) {
	setupTestProject(t)

	err := mcpTools([]string{"nonexistent"})
	if err == nil {
		t.Fatal("期望返回错误（server 不存在），实际返回 nil")
	}
}

func TestMCPTools_NoURL(t *testing.T) {
	setupTestProject(t)

	if err := mcpAdd([]string{"cmd-only", "--command", "npx"}); err != nil {
		t.Fatalf("mcpAdd 失败: %v", err)
	}

	err := mcpTools([]string{"cmd-only"})
	if err == nil {
		t.Fatal("期望返回错误（无 URL），实际返回 nil")
	}
}

// ===== MCP dispatch 测试 =====

func TestRunMCP_NoArgs(t *testing.T) {
	err := runMCP([]string{})
	if err != nil {
		t.Errorf("无参数应返回 nil（显示帮助），实际: %v", err)
	}
}

func TestRunMCP_UnknownSubcommand(t *testing.T) {
	err := runMCP([]string{"unknown"})
	if err == nil {
		t.Fatal("期望返回错误（未知子命令），实际返回 nil")
	}
}

// ===== truncate 测试 =====

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hi", 3, "hi"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}
