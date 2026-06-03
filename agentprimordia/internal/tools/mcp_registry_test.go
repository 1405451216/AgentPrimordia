package tools

import (
	"context"
	"os"
	"testing"
)

func TestMCPRegistry_Register(t *testing.T) {
	r := NewMCPRegistry()

	r.Register(MCPClientConfig{
		Name:    "test-server",
		BaseURL: "http://localhost:3000",
	})

	entries := r.List()
	if len(entries) != 1 {
		t.Fatalf("期望 1 个条目，实际 %d 个", len(entries))
	}
	if entries[0].Config.Name != "test-server" {
		t.Errorf("名称不匹配: got %q", entries[0].Config.Name)
	}
	if entries[0].Status != MCPClientStopped {
		t.Errorf("初始状态应为 stopped，实际 %s", entries[0].Status)
	}
}

func TestMCPRegistry_Unregister(t *testing.T) {
	r := NewMCPRegistry()
	r.Register(MCPClientConfig{Name: "server-1", BaseURL: "http://localhost:3000"})
	r.Register(MCPClientConfig{Name: "server-2", BaseURL: "http://localhost:3001"})

	if err := r.Unregister("server-1"); err != nil {
		t.Fatalf("Unregister 失败: %v", err)
	}

	entries := r.List()
	if len(entries) != 1 {
		t.Fatalf("期望 1 个条目，实际 %d 个", len(entries))
	}
	if entries[0].Config.Name != "server-2" {
		t.Errorf("剩余条目名称不匹配: got %q", entries[0].Config.Name)
	}
}

func TestMCPRegistry_Unregister_NotFound(t *testing.T) {
	r := NewMCPRegistry()
	err := r.Unregister("nonexistent")
	if err == nil {
		t.Error("期望返回错误，实际返回 nil")
	}
}

func TestMCPRegistry_Get(t *testing.T) {
	r := NewMCPRegistry()
	r.Register(MCPClientConfig{
		Name:    "test-server",
		BaseURL: "http://localhost:3000",
	})

	entry, ok := r.Get("test-server")
	if !ok {
		t.Fatal("未找到已注册的 server")
	}
	if entry.Config.BaseURL != "http://localhost:3000" {
		t.Errorf("BaseURL 不匹配: got %q", entry.Config.BaseURL)
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("不应找到未注册的 server")
	}
}

func TestMCPRegistry_Stop_NotRunning(t *testing.T) {
	r := NewMCPRegistry()
	r.Register(MCPClientConfig{Name: "test-server"})

	if err := r.Stop("test-server"); err != nil {
		t.Fatalf("Stop 未运行的 server 不应报错: %v", err)
	}
}

func TestMCPRegistry_Stop_NotFound(t *testing.T) {
	r := NewMCPRegistry()
	err := r.Stop("nonexistent")
	if err == nil {
		t.Error("期望返回错误，实际返回 nil")
	}
}

func TestMCPRegistry_StopAll(t *testing.T) {
	r := NewMCPRegistry()
	r.Register(MCPClientConfig{Name: "s1"})
	r.Register(MCPClientConfig{Name: "s2"})
	r.StopAll() // 不应 panic
}

func TestMCPRegistry_RegisterIntoRegistry_NotRunning(t *testing.T) {
	r := NewMCPRegistry()
	r.Register(MCPClientConfig{Name: "test-server"})
	toolReg := NewRegistry()

	if err := r.RegisterIntoRegistry(toolReg); err != nil {
		t.Fatalf("RegisterIntoRegistry 不应报错: %v", err)
	}
}

func TestMCPRegistry_LoadFromConfig_NotFound(t *testing.T) {
	r := NewMCPRegistry()
	err := r.LoadFromConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Error("期望返回错误，实际返回 nil")
	}
}

func TestMCPRegistry_LoadFromConfig_InvalidJSON(t *testing.T) {
	r := NewMCPRegistry()
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	_ = os.WriteFile(configPath, []byte("invalid json"), 0o644)

	err := r.LoadFromConfig(configPath)
	if err == nil {
		t.Error("期望返回错误，实际返回 nil")
	}
}

func TestMCPRegistry_LoadFromConfig_Valid(t *testing.T) {
	r := NewMCPRegistry()
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"

	config := `{
		"mcp": {
			"servers": {
				"filesystem": {
					"name": "filesystem",
					"command": "npx",
					"args": ["@mcp/server-filesystem", "/tmp"],
					"autoStart": true
				},
				"github": {
					"name": "github",
					"baseUrl": "http://localhost:3001/mcp"
				}
			}
		}
	}`
	_ = os.WriteFile(configPath, []byte(config), 0o644)

	err := r.LoadFromConfig(configPath)
	if err != nil {
		t.Fatalf("LoadFromConfig 失败: %v", err)
	}

	entries := r.List()
	if len(entries) != 2 {
		t.Fatalf("期望 2 个条目，实际 %d 个", len(entries))
	}
}

func TestMCPRegistry_Test_NotStarted(t *testing.T) {
	r := NewMCPRegistry()
	r.Register(MCPClientConfig{Name: "test-server"})

	ctx := context.Background()
	err := r.Test(ctx, "test-server")
	if err == nil {
		t.Error("未启动的 server 不应通过测试")
	}
}

func TestMCPRegistry_Test_NotFound(t *testing.T) {
	r := NewMCPRegistry()
	ctx := context.Background()
	err := r.Test(ctx, "nonexistent")
	if err == nil {
		t.Error("不存在的 server 不应通过测试")
	}
}
