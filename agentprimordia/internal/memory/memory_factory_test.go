package memory

import (
	"context"
	"strings"
	"testing"
)

// TestNewMemory_InMemoryBackend 验证内存后端工厂
func TestNewMemory_InMemoryBackend(t *testing.T) {
	store, err := NewMemory(Config{Type: BackendMemory})
	if err != nil {
		t.Fatalf("NewMemory(BackendMemory) 报错: %v", err)
	}
	if store == nil {
		t.Fatal("返回 nil store")
	}

	// 验证可立即使用
	ctx := context.Background()
	ep := &Episode{
		ID:        "ep-1",
		SessionID: "test",
		Role:      "user",
		Content:   "hello",
	}
	if err := store.Add(ctx, ep); err != nil {
		t.Fatalf("Add 报错: %v", err)
	}
}

// TestNewMemory_SQLiteBackend 验证 SQLite 后端工厂
func TestNewMemory_SQLiteBackend(t *testing.T) {
	dbPath := t.TempDir() + "factory_test.db"
	store, err := NewMemory(Config{Type: BackendSQLite, Path: dbPath})
	if err != nil {
		t.Fatalf("NewMemory(BackendSQLite) 报错: %v", err)
	}
	if store == nil {
		t.Fatal("返回 nil store")
	}
	t.Cleanup(func() { store.Close() })

	// 验证表创建
	ctx := context.Background()
	ep := &Episode{ID: "ep-1", SessionID: "s1", Role: "user", Content: "test"}
	if err := store.Add(ctx, ep); err != nil {
		t.Fatalf("Add 报错: %v", err)
	}
}

// TestNewMemory_SQLiteRequiresPath 验证 SQLite 缺 Path 报错
func TestNewMemory_SQLiteRequiresPath(t *testing.T) {
	_, err := NewMemory(Config{Type: BackendSQLite})
	if err == nil {
		t.Error("SQLite 缺 Path 应返回错误")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Errorf("错误信息应提及 path, got: %v", err)
	}
}

// TestNewMemory_UnsupportedBackend 验证未知 Backend 报错
func TestNewMemory_UnsupportedBackend(t *testing.T) {
	_, err := NewMemory(Config{Type: "redis"})
	if err == nil {
		t.Error("未知 backend 应返回错误")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("错误信息应提及 unsupported, got: %v", err)
	}
}

// TestBackendType_Constants 验证常量值
func TestBackendType_Constants(t *testing.T) {
	if BackendSQLite != "sqlite" {
		t.Errorf("BackendSQLite = %q, want %q", BackendSQLite, "sqlite")
	}
	if BackendMemory != "memory" {
		t.Errorf("BackendMemory = %q, want %q", BackendMemory, "memory")
	}
}

// TestDefaultSearchLimit 验证默认搜索限制
func TestDefaultSearchLimit(t *testing.T) {
	if defaultSearchLimit <= 0 {
		t.Errorf("defaultSearchLimit 应 > 0, got %d", defaultSearchLimit)
	}
}

// Episode 构造测试
func TestEpisode_Fields(t *testing.T) {
	ep := Episode{
		ID:         "e1",
		SessionID:  "s1",
		Role:       "user",
		Content:    "hello world",
		CreatedAt:  "2026-06-05T10:00:00Z",
		Importance: 0.5,
	}
	if ep.SessionID != "s1" {
		t.Error("SessionID 未设置")
	}
	if ep.Importance != 0.5 {
		t.Error("Importance 未设置")
	}
	if ep.CreatedAt == "" {
		t.Error("CreatedAt 未设置")
	}
}
