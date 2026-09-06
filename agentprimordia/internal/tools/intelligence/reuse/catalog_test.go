// catalog_test.go — 工具目录测试
package reuse

import "testing"

// TestToolCatalog_RegisterAndGet 测试注册和获取
func TestToolCatalog_RegisterAndGet(t *testing.T) {
	catalog := NewToolCatalog()

	// 注册工具
	entry := ToolEntry{
		ID:          "shell",
		Name:        "shell",
		Description: "执行 shell 命令",
		Domain:      "system",
	}
	catalog.Register(entry)

	// 获取已注册工具
	got, ok := catalog.GetByID("shell")
	if !ok {
		t.Fatal("期望找到 shell 工具")
	}
	if got.ID != "shell" {
		t.Errorf("期望 ID=shell，实际=%s", got.ID)
	}
	if got.Name != "shell" {
		t.Errorf("期望 Name=shell，实际=%s", got.Name)
	}
	if got.Domain != "system" {
		t.Errorf("期望 Domain=system，实际=%s", got.Domain)
	}

	// 获取不存在的工具
	_, ok = catalog.GetByID("nonexistent")
	if ok {
		t.Error("期望找不到 nonexistent 工具")
	}
}

// TestToolCatalog_List 测试列出所有工具
func TestToolCatalog_List(t *testing.T) {
	catalog := NewToolCatalog()

	// 空目录
	tools := catalog.List()
	if len(tools) != 0 {
		t.Errorf("期望 0 个工具，实际=%d", len(tools))
	}

	// 注册多个工具
	catalog.Register(ToolEntry{ID: "shell", Name: "shell", Description: "执行命令"})
	catalog.Register(ToolEntry{ID: "file", Name: "file", Description: "文件操作"})
	catalog.Register(ToolEntry{ID: "web", Name: "web", Description: "网络请求"})

	tools = catalog.List()
	if len(tools) != 3 {
		t.Errorf("期望 3 个工具，实际=%d", len(tools))
	}

	// 验证所有工具都存在
	ids := make(map[string]bool)
	for _, tool := range tools {
		ids[tool.ID] = true
	}
	if !ids["shell"] || !ids["file"] || !ids["web"] {
		t.Error("期望包含所有注册的工具")
	}
}

// TestToolCatalog_ConcurrentAccess 测试并发访问
func TestToolCatalog_ConcurrentAccess(t *testing.T) {
	catalog := NewToolCatalog()

	// 并发注册
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			catalog.Register(ToolEntry{
				ID:   "tool" + string(rune('0'+id)),
				Name: "tool",
			})
			done <- true
		}(i)
	}

	// 等待所有注册完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证
	tools := catalog.List()
	if len(tools) != 10 {
		t.Errorf("期望 10 个工具，实际=%d", len(tools))
	}
}
