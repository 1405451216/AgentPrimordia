package kv

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// newTestKVTool 创建用于测试的 KVStoreTool 实例
func newTestKVTool(t *testing.T) *KVStoreTool {
	t.Helper()

	p := New()
	dbPath := t.TempDir() + "\\test_kv.db"
	err := p.Init(map[string]any{"db_path": dbPath})
	if err != nil {
		t.Fatalf("初始化 KV 插件失败: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	return p.tool
}

// TestKVStoreTool_Name 验证工具名称
func TestKVStoreTool_Name(t *testing.T) {
	tool := &KVStoreTool{}
	if tool.Name() != "kv_store" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "kv_store")
	}
}

// TestKVStoreTool_Category 验证工具分类
func TestKVStoreTool_Category(t *testing.T) {
	tool := &KVStoreTool{}
	if tool.Category() != "database" {
		t.Errorf("Category() = %q, want %q", tool.Category(), "database")
	}
}

// TestKVStoreTool_Parameters 验证参数 Schema 可解析
func TestKVStoreTool_Parameters(t *testing.T) {
	tool := &KVStoreTool{}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("Parameters() 不应返回 nil")
	}

	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("Parameters() 返回的 JSON 无效: %v", err)
	}
}

// TestKVStoreTool_SetAndGet 验证 set 和 get 操作
func TestKVStoreTool_SetAndGet(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	// 设置键值对
	setArgs, _ := json.Marshal(map[string]any{
		"action": "set",
		"key":    "greeting",
		"value":  "hello world",
	})
	result, err := tool.Execute(ctx, setArgs)
	if err != nil {
		t.Fatalf("set 执行失败: %v", err)
	}
	if result.IsError {
		t.Fatalf("set 返回错误: %s", result.Content)
	}

	// 获取键值对
	getArgs, _ := json.Marshal(map[string]any{
		"action": "get",
		"key":    "greeting",
	})
	result, err = tool.Execute(ctx, getArgs)
	if err != nil {
		t.Fatalf("get 执行失败: %v", err)
	}
	if result.IsError {
		t.Fatalf("get 返回错误: %s", result.Content)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		t.Fatalf("解析 get 结果失败: %v", err)
	}
	if data["key"] != "greeting" {
		t.Errorf("key = %v, want %q", data["key"], "greeting")
	}
	if data["value"] != "hello world" {
		t.Errorf("value = %v, want %q", data["value"], "hello world")
	}
}

// TestKVStoreTool_GetNonExistent 验证获取不存在的键返回错误
func TestKVStoreTool_GetNonExistent(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	args, _ := json.Marshal(map[string]any{
		"action": "get",
		"key":    "nonexistent",
	})
	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("期望返回错误结果（键不存在）")
	}
}

// TestKVStoreTool_Delete 验证删除操作
func TestKVStoreTool_Delete(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	// 先设置
	setArgs, _ := json.Marshal(map[string]any{
		"action": "set",
		"key":    "to_delete",
		"value":  "temporary",
	})
	_, _ = tool.Execute(ctx, setArgs)

	// 删除
	delArgs, _ := json.Marshal(map[string]any{
		"action": "delete",
		"key":    "to_delete",
	})
	result, err := tool.Execute(ctx, delArgs)
	if err != nil {
		t.Fatalf("delete 执行失败: %v", err)
	}
	if result.IsError {
		t.Fatalf("delete 返回错误: %s", result.Content)
	}

	// 验证已删除
	getArgs, _ := json.Marshal(map[string]any{
		"action": "get",
		"key":    "to_delete",
	})
	result, _ = tool.Execute(ctx, getArgs)
	if !result.IsError {
		t.Error("期望获取已删除的键返回错误")
	}
}

// TestKVStoreTool_DeleteNonExistent 验证删除不存在的键返回错误
func TestKVStoreTool_DeleteNonExistent(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	args, _ := json.Marshal(map[string]any{
		"action": "delete",
		"key":    "nonexistent",
	})
	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("期望删除不存在的键返回错误")
	}
}

// TestKVStoreTool_List 验证列表操作
func TestKVStoreTool_List(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	// 设置多个键值对
	for i := 0; i < 3; i++ {
		args, _ := json.Marshal(map[string]any{
			"action": "set",
			"key":    fmt.Sprintf("key_%d", i),
			"value":  fmt.Sprintf("value_%d", i),
		})
		_, _ = tool.Execute(ctx, args)
	}

	// 列出所有
	listArgs, _ := json.Marshal(map[string]any{"action": "list"})
	result, err := tool.Execute(ctx, listArgs)
	if err != nil {
		t.Fatalf("list 执行失败: %v", err)
	}
	if result.IsError {
		t.Fatalf("list 返回错误: %s", result.Content)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		t.Fatalf("解析 list 结果失败: %v", err)
	}

	count, ok := data["count"].(float64)
	if !ok || int(count) != 3 {
		t.Errorf("count = %v, want 3", data["count"])
	}
}

// TestKVStoreTool_Update 验证更新操作（set 同一 key 覆盖）
func TestKVStoreTool_Update(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	// 设置初始值
	setArgs, _ := json.Marshal(map[string]any{
		"action": "set",
		"key":    "counter",
		"value":  "1",
	})
	_, _ = tool.Execute(ctx, setArgs)

	// 更新值
	setArgs, _ = json.Marshal(map[string]any{
		"action": "set",
		"key":    "counter",
		"value":  "2",
	})
	_, _ = tool.Execute(ctx, setArgs)

	// 验证值已更新
	getArgs, _ := json.Marshal(map[string]any{
		"action": "get",
		"key":    "counter",
	})
	result, _ := tool.Execute(ctx, getArgs)

	var data map[string]any
	json.Unmarshal([]byte(result.Content), &data)
	if data["value"] != "2" {
		t.Errorf("value = %v, want %q", data["value"], "2")
	}
}

// TestKVStoreTool_InvalidAction 验证无效操作返回错误
func TestKVStoreTool_InvalidAction(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	args, _ := json.Marshal(map[string]any{
		"action": "invalid",
	})
	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("期望无效操作返回错误结果")
	}
}

// TestKVStoreTool_MissingKey 验证 get/set/delete 缺少 key 参数时返回错误
func TestKVStoreTool_MissingKey(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	tests := []string{"get", "set", "delete"}
	for _, action := range tests {
		args, _ := json.Marshal(map[string]any{
			"action": action,
		})
		result, err := tool.Execute(ctx, args)
		if err != nil {
			t.Fatalf("%s unexpected error: %v", action, err)
		}
		if !result.IsError {
			t.Errorf("%s: 期望缺少 key 时返回错误", action)
		}
	}
}

// TestKVStoreTool_Execute_InvalidJSON 验证无效 JSON 输入
func TestKVStoreTool_Execute_InvalidJSON(t *testing.T) {
	tool := newTestKVTool(t)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("期望解析错误，但未返回错误")
	}
}

// TestPlugin_Metadata 验证插件元数据
func TestPlugin_Metadata(t *testing.T) {
	p := New()
	if p.Name() != "kv" {
		t.Errorf("Name() = %q, want %q", p.Name(), "kv")
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.1.0")
	}
	if len(p.Tools()) != 1 {
		t.Errorf("Tools() 返回 %d 项, want 1 项", len(p.Tools()))
	}
}

// TestPlugin_Init_DefaultDbPath 验证默认数据库路径
func TestPlugin_Init_DefaultDbPath(t *testing.T) {
	p := New()
	dbPath := t.TempDir() + "\\default.db"
	err := p.Init(map[string]any{"db_path": dbPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	if p.tool.db == nil {
		t.Error("期望 db 不为 nil")
	}
}

// TestPlugin_Init_CreatesTable 验证初始化时创建 KV 表
func TestPlugin_Init_CreatesTable(t *testing.T) {
	p := New()
	dbPath := t.TempDir() + "\\create_table.db"
	err := p.Init(map[string]any{"db_path": dbPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	// 验证表存在
	var tableName string
	err = p.tool.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='ap_kv_store'").Scan(&tableName)
	if err != nil {
		t.Fatalf("KV 表未创建: %v", err)
	}
	if tableName != "ap_kv_store" {
		t.Errorf("tableName = %q, want %q", tableName, "ap_kv_store")
	}
}

// TestKVStoreTool_EmptyList 验证空存储的列表操作
func TestKVStoreTool_EmptyList(t *testing.T) {
	tool := newTestKVTool(t)
	ctx := context.Background()

	args, _ := json.Marshal(map[string]any{"action": "list"})
	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("list 执行失败: %v", err)
	}

	var data map[string]any
	json.Unmarshal([]byte(result.Content), &data)
	count, _ := data["count"].(float64)
	if int(count) != 0 {
		t.Errorf("空存储 count = %v, want 0", data["count"])
	}
}
