package sqlplugin

import (
	"context"
	"encoding/json"
	"testing"
)

// TestPlugin_Metadata 验证插件元数据
func TestPlugin_Metadata(t *testing.T) {
	p := New()
	if p.Name() != "sql" {
		t.Errorf("Name() = %q, want %q", p.Name(), "sql")
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.1.0")
	}
}

func TestPlugin_Tools_NilBeforeInit(t *testing.T) {
	p := New()
	if got := p.Tools(); got != nil {
		t.Errorf("Init 前 Tools() 应为 nil, got %v", got)
	}
}

func TestPlugin_Init_DefaultDbPath(t *testing.T) {
	p := New()
	dbPath := t.TempDir() + "test.db"
	if err := p.Init(map[string]any{"db_path": dbPath}); err != nil {
		t.Fatalf("Init 报错: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	tools := p.Tools()
	if len(tools) != 1 {
		t.Errorf("Init 后 Tools() = %d 项, want 1", len(tools))
	}
}

func TestPlugin_Close_NotInitialized(t *testing.T) {
	p := New()
	// Close 在未 Init 时应安全
	if err := p.Close(); err != nil {
		t.Errorf("Close 在未 Init 时报错: %v", err)
	}
}

func TestPlugin_Close_AfterInit(t *testing.T) {
	p := New()
	dbPath := t.TempDir() + "close.db"
	if err := p.Init(map[string]any{"db_path": dbPath}); err != nil {
		t.Fatalf("Init 报错: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close 报错: %v", err)
	}
}

// TestSQLiteTool_EndToEnd_ListTables 验证 tables + schema action
// 注意：SQLiteTool 出于安全考虑只支持 SELECT / PRAGMA，
// 详见 internal/tools/data_tools.go validateSQLSafety。
func TestSQLiteTool_EndToEnd_ListTables(t *testing.T) {
	dbPath := t.TempDir() + "query.db"
	p := New()
	if err := p.Init(map[string]any{"db_path": dbPath}); err != nil {
		t.Fatalf("Init 报错: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	tool := p.Tools()[0]

	// 列出表（空 DB）
	tablesArgs, _ := json.Marshal(map[string]any{
		"action": "tables",
	})
	result, err := tool.Execute(context.Background(), tablesArgs)
	if err != nil {
		t.Fatalf("tables 报错: %v", err)
	}
	if result.IsError {
		t.Fatalf("tables 失败: %s", result.Content)
	}
	if result.Content == "" {
		t.Error("tables 结果 Content 不应为空")
	}
}

// TestSQLiteTool_RejectsNonSelect 验证安全检查：非 SELECT/PRAGMA 被拒
func TestSQLiteTool_RejectsNonSelect(t *testing.T) {
	dbPath := t.TempDir() + "safe.db"
	p := New()
	if err := p.Init(map[string]any{"db_path": dbPath}); err != nil {
		t.Fatalf("Init 报错: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	tool := p.Tools()[0]

	dangerous := []string{
		"CREATE TABLE foo (x INT)",
		"INSERT INTO foo VALUES (1)",
		"DROP TABLE foo",
		"DELETE FROM foo",
		"UPDATE foo SET x = 1",
	}
	for _, sql := range dangerous {
		args, _ := json.Marshal(map[string]any{
			"action": "query",
			"sql":    sql,
		})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("%s 报错: %v", sql, err)
		}
		if !result.IsError {
			t.Errorf("危险 SQL %q 应被拒绝, got Content=%q", sql, result.Content)
		}
	}
}
