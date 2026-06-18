package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockTool struct {
	name        string
	description string
	params      json.RawMessage
	response    string
	shouldFail  bool
	delay       time.Duration
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) Parameters() json.RawMessage {
	if m.params != nil {
		return m.params
	}
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)
}
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (*Result, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.shouldFail {
		return NewErrorResult("intentional failure"), nil
	}
	return NewResult(m.response), nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	tool := &mockTool{name: "test_tool", description: "A test tool", response: "ok"}

	err := reg.Register(tool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, exists := reg.Get("test_tool")
	if !exists {
		t.Fatal("tool should exist after registration")
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected 'test_tool', got '%s'", got.Name())
	}
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	tool := &mockTool{name: "dup", response: "ok"}

	if err := reg.Register(tool); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	err := reg.Register(tool)
	if err != nil {
		t.Errorf("duplicate should be no-op, got: %v", err)
	}
	if reg.Count() != 1 {
		t.Errorf("expected count 1, got %d", reg.Count())
	}
}

func TestRegistry_ListAndCount(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	_ = reg.RegisterMultiple(
		&mockTool{name: "a", response: "a"},
		&mockTool{name: "b", response: "b"},
		&mockTool{name: "c", response: "c"},
	)

	if reg.Count() != 3 {
		t.Errorf("expected 3, got %d", reg.Count())
	}
	names := reg.List()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}
}

func TestRegistry_GetNonExistent(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	_, exists := reg.Get("nonexistent")
	if exists {
		t.Error("should not exist")
	}
}

func TestRegistry_Definitions(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "weather", description: "Get weather", response: "sunny"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	defs := reg.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	fn, ok := defs[0]["function"].(map[string]any)
	if !ok {
		t.Fatal("function should be map")
	}
	if fn["name"] != "weather" {
		t.Errorf("expected 'weather', got '%v'", fn["name"])
	}
}

func TestRegistry_Permissions(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "secure", response: "ok"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	err := reg.SetPermission("secure", Permission{RequireConfirmation: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perm, exists := reg.GetPermission("secure")
	if !exists {
		t.Fatal("permission should exist")
	}
	if !perm.RequireConfirmation {
		t.Error("should be true")
	}
}

func TestExecutor_ExecuteSuccess(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "echo", description: "Echo", response: "hello!"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "echo", Args: `{"query":"test"}`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should not be error, content: %s", result.Content)
	}
	if result.Content != "hello!" {
		t.Errorf("expected 'hello!', got '%s'", result.Content)
	}
}

func TestExecutor_ExecuteNotFound(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	executor := NewExecutor(reg)

	result, err := executor.Execute(context.Background(), &FunctionCall{
		Name: "nonexistent", Args: `{}`,
	})

	if err == nil {
		t.Error("expected error")
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_ExecuteTimeout(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "slow", response: "ok", delay: 200 * time.Millisecond}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	executor := NewExecutor(reg).WithTimeout(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := executor.Execute(ctx, &FunctionCall{Name: "slow", Args: `{}`})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestNewResultHelpers(t *testing.T) {
	t.Parallel()
	success := NewResult("good")
	if success.IsError {
		t.Error("should not be error")
	}
	fail := NewErrorResult("bad")
	if !fail.IsError {
		t.Error("should be error")
	}
}

// ===== 确认回调测试 =====

func TestExecutor_ConfirmationRequired_Denied(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "dangerous", description: "Dangerous", response: "boom"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 设置需要确认但没有回调
	_ = reg.SetPermission("dangerous", Permission{RequireConfirmation: true})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "dangerous", Args: `{}`,
	})

	if err != ErrConfirmDenied {
		t.Errorf("expected ErrConfirmDenied, got: %v", err)
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_ConfirmationCallback_Accepted(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "safe_op", description: "Safe Op", response: "done"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 设置确认回调，始终允许
	_ = reg.SetPermission("safe_op", Permission{
		RequireConfirmation: true,
		ConfirmFunc: func(toolName string, args json.RawMessage) bool {
			return true
		},
	})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "safe_op", Args: `{}`,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("should not be error, content: %s", result.Content)
	}
}

func TestExecutor_ConfirmationCallback_Rejected(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "risky_op", description: "Risky Op", response: "oops"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 设置确认回调，始终拒绝
	_ = reg.SetPermission("risky_op", Permission{
		RequireConfirmation: true,
		ConfirmFunc: func(toolName string, args json.RawMessage) bool {
			return false
		},
	})

	executor := NewExecutor(reg)
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "risky_op", Args: `{}`,
	})

	if err != ErrConfirmDenied {
		t.Errorf("expected ErrConfirmDenied, got: %v", err)
	}
	if !result.IsError {
		t.Error("result should be error")
	}
}

func TestExecutor_ConfirmationConditional(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "cond_op", description: "Conditional Op", response: "ok"}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 条件性确认：只允许特定参数
	_ = reg.SetPermission("cond_op", Permission{
		RequireConfirmation: true,
		ConfirmFunc: func(toolName string, args json.RawMessage) bool {
			var params map[string]any
			_ = json.Unmarshal(args, &params)
			if mode, ok := params["mode"]; ok && mode == "safe" {
				return true
			}
			return false
		},
	})

	executor := NewExecutor(reg)

	// 安全模式 - 允许
	result, err := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_1", Name: "cond_op", Args: `{"mode":"safe"}`,
	})
	if err != nil {
		t.Fatalf("safe mode should be allowed, got: %v", err)
	}
	if result.IsError {
		t.Error("safe mode result should not be error")
	}

	// 危险模式 - 拒绝
	result2, err2 := executor.Execute(context.Background(), &FunctionCall{
		ID: "call_2", Name: "cond_op", Args: `{"mode":"danger"}`,
	})
	if err2 != ErrConfirmDenied {
		t.Errorf("danger mode should be denied, got: %v", err2)
	}
	if !result2.IsError {
		t.Error("danger mode result should be error")
	}
}

// ===== validateGitURL 测试 =====

func TestValidateGitURL_Valid(t *testing.T) {
	t.Parallel()
	validURLs := []string{
		"https://github.com/user/repo.git",
		"http://github.com/user/repo.git",
		"git://github.com/user/repo.git",
		"ssh://git@github.com/user/repo.git",
		"git@github.com:user/repo.git",
	}
	for _, u := range validURLs {
		if err := validateGitURL(u); err != nil {
			t.Errorf("validateGitURL(%q) should be valid, got: %v", u, err)
		}
	}
}

func TestValidateGitURL_Invalid(t *testing.T) {
	t.Parallel()
	invalidURLs := []string{
		"ftp://example.com/repo.git",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"",
		"not-a-url",
	}
	for _, u := range invalidURLs {
		if err := validateGitURL(u); err == nil {
			t.Errorf("validateGitURL(%q) should be invalid", u)
		}
	}
}

// ===== getDataType 测试 =====

func TestGetDataType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    any
		expected string
	}{
		{map[string]any{"a": 1}, "object"},
		{[]any{1, 2, 3}, "array"},
		{"hello", "string"},
		{float64(3.14), "number"},
		{true, "boolean"},
		{false, "boolean"},
		{nil, "null"},
		{42, "unknown"},         // int 不是 JSON 标准类型
		{struct{}{}, "unknown"}, // 结构体也是 unknown
	}
	for _, tc := range cases {
		got := getDataType(tc.input)
		if got != tc.expected {
			t.Errorf("getDataType(%v) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ===== isValidTableName 测试 =====

func TestIsValidTableName(t *testing.T) {
	t.Parallel()
	valid := []string{"users", "User_Data", "table1", "A", "a_b_c_123"}
	for _, name := range valid {
		if !isValidTableName(name) {
			t.Errorf("isValidTableName(%q) should be true", name)
		}
	}
	invalid := []string{"", "user data", "table;drop", "a-b", "表名", "foo.bar"}
	for _, name := range invalid {
		if isValidTableName(name) {
			t.Errorf("isValidTableName(%q) should be false", name)
		}
	}
}

// ===== toStringSlice 测试 =====

func TestToStringSlice(t *testing.T) {
	t.Parallel()
	items := []any{"hello", 42, true, 3.14}
	got := toStringSlice(items)
	expected := []string{"hello", "42", "true", "3.14"}
	if len(got) != len(expected) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(expected))
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("toStringSlice[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
	// 空切片
	empty := toStringSlice([]any{})
	if len(empty) != 0 {
		t.Errorf("empty slice should have length 0, got %d", len(empty))
	}
}

// ===== applyTransform 测试 =====

func TestApplyTransform_UppercaseKeys(t *testing.T) {
	t.Parallel()
	data := map[string]any{"name": "test", "age": float64(25)}
	result := applyTransform(data, "uppercase_keys")
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}
	if _, exists := m["NAME"]; !exists {
		t.Error("expected key 'NAME'")
	}
	if _, exists := m["AGE"]; !exists {
		t.Error("expected key 'AGE'")
	}
}

func TestApplyTransform_Flatten(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": map[string]any{
			"b": "deep",
		},
		"c": "top",
	}
	result := applyTransform(data, "flatten")
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}
	if m["a.b"] != "deep" {
		t.Errorf("expected 'a.b' = 'deep', got %v", m["a.b"])
	}
	if m["c"] != "top" {
		t.Errorf("expected 'c' = 'top', got %v", m["c"])
	}
}

func TestApplyTransform_UnknownRule(t *testing.T) {
	t.Parallel()
	data := map[string]any{"key": "value"}
	result := applyTransform(data, "nonexistent_rule")
	// 未知规则应原样返回
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be original data")
	}
	if m["key"] != "value" {
		t.Error("unknown rule should return data unchanged")
	}
}

// ===== flattenObject / flattenRecursive 测试 =====

func TestFlattenObject_Nested(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"x": map[string]any{
			"y": map[string]any{
				"z": float64(42),
			},
		},
		"top": "level",
	}
	result := flattenObject(data)
	if result["x.y.z"] != float64(42) {
		t.Errorf("expected x.y.z = 42, got %v", result["x.y.z"])
	}
	if result["top"] != "level" {
		t.Errorf("expected top = 'level', got %v", result["top"])
	}
}

func TestFlattenObject_NonMap(t *testing.T) {
	t.Parallel()
	// 非 map 输入，没有前缀，结果为空
	result := flattenObject("just a string")
	if len(result) != 0 {
		t.Errorf("non-map input should produce empty result, got %v", result)
	}
}

// ===== CSV aggregate 测试 =====

// createTestCSV 创建测试用 CSV 文件
func createTestCSV(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "test.csv")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test CSV: %v", err)
	}
	return path
}

const testCSVContent = `name,score,grade
Alice,90,A
Bob,80,B
Charlie,70,C
`

func TestCSVTool_Aggregate_Avg(t *testing.T) {
	t.Parallel()
	csvPath := createTestCSV(t, t.TempDir(), testCSVContent)
	tool := NewCSVTool()
	args := `{"action":"aggregate","file_path":` + jsonString(csvPath) + `,"aggregate_column":"score","aggregate_func":"avg"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(result.Content), &out)
	// avg = (90+80+70)/3 = 80
	if val, ok := out["value"].(float64); !ok || val != 80 {
		t.Errorf("expected avg = 80, got %v", out["value"])
	}
}

func TestCSVTool_Aggregate_Min(t *testing.T) {
	t.Parallel()
	csvPath := createTestCSV(t, t.TempDir(), testCSVContent)
	tool := NewCSVTool()
	args := `{"action":"aggregate","file_path":` + jsonString(csvPath) + `,"aggregate_column":"score","aggregate_func":"min"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(result.Content), &out)
	if val, ok := out["value"].(float64); !ok || val != 70 {
		t.Errorf("expected min = 70, got %v", out["value"])
	}
}

func TestCSVTool_Aggregate_Max(t *testing.T) {
	t.Parallel()
	csvPath := createTestCSV(t, t.TempDir(), testCSVContent)
	tool := NewCSVTool()
	args := `{"action":"aggregate","file_path":` + jsonString(csvPath) + `,"aggregate_column":"score","aggregate_func":"max"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(result.Content), &out)
	if val, ok := out["value"].(float64); !ok || val != 90 {
		t.Errorf("expected max = 90, got %v", out["value"])
	}
}

func TestCSVTool_Aggregate_Count(t *testing.T) {
	t.Parallel()
	csvPath := createTestCSV(t, t.TempDir(), testCSVContent)
	tool := NewCSVTool()
	args := `{"action":"aggregate","file_path":` + jsonString(csvPath) + `,"aggregate_column":"score","aggregate_func":"count"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(result.Content), &out)
	if val, ok := out["value"].(float64); !ok || val != 3 {
		t.Errorf("expected count = 3, got %v", out["value"])
	}
}

// ===== SQLiteTool 测试 =====

// setupTestDB 创建测试用 SQLite 数据库并插入数据
func setupTestDB(t *testing.T) (*SQLiteTool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	tool, err := NewSQLiteTool(dbPath)
	if err != nil {
		t.Fatalf("failed to create SQLite tool: %v", err)
	}
	t.Cleanup(func() { _ = tool.Close() })

	// 直接通过 DB 创建表和插入数据
	_, err = tool.db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	_, err = tool.db.Exec(`INSERT INTO users (name, age) VALUES ('Alice', 30), ('Bob', 25)`)
	if err != nil {
		t.Fatalf("failed to insert data: %v", err)
	}
	return tool, dbPath
}

func TestSQLiteTool_Query(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	args := `{"action":"query","sql":"SELECT * FROM users ORDER BY id"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("query should succeed, got error: %s", result.Content)
	}
	var rows []map[string]any
	_ = json.Unmarshal([]byte(result.Content), &rows)
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestSQLiteTool_Tables(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	args := `{"action":"tables"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tables should succeed, got error: %s", result.Content)
	}
	var rows []map[string]any
	_ = json.Unmarshal([]byte(result.Content), &rows)
	if len(rows) == 0 {
		t.Error("expected at least one table")
	}
}

func TestSQLiteTool_Schema(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	args := `{"action":"schema","table_name":"users"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("schema should succeed, got error: %s", result.Content)
	}
	var cols []map[string]any
	_ = json.Unmarshal([]byte(result.Content), &cols)
	if len(cols) != 3 {
		t.Errorf("expected 3 columns (id, name, age), got %d", len(cols))
	}
	if result.Metadata["table_name"] != "users" {
		t.Errorf("expected table_name metadata = 'users', got %v", result.Metadata["table_name"])
	}
}

func TestSQLiteTool_Execute_SELECT(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	// execute action 也受 SQL 安全检查限制，只能执行 SELECT/PRAGMA
	args := `{"action":"execute","sql":"SELECT COUNT(*) as cnt FROM users"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("execute SELECT should succeed, got error: %s", result.Content)
	}
}

func TestSQLiteTool_Execute_DangerousSQL(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	args := `{"action":"execute","sql":"DROP TABLE users"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("dangerous SQL should return error result")
	}
}

func TestSQLiteTool_UnknownAction(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	args := `{"action":"unknown_action"}`
	_, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Error("unknown action should return error")
	}
}

func TestSQLiteTool_Schema_InvalidTableName(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	args := `{"action":"schema","table_name":"users; DROP TABLE"}`
	_, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Error("invalid table name should return error")
	}
}

// ===== JSONTool 测试 =====

func TestJSONTool_Transform(t *testing.T) {
	t.Parallel()
	tool := NewJSONTool()
	args := `{"action":"transform","data":"{\"name\":\"test\",\"value\":42}","transform_rule":"uppercase_keys"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(result.Content), &out)
	if _, exists := out["NAME"]; !exists {
		t.Error("expected uppercase key 'NAME'")
	}
	if result.Metadata["transform_rule"] != "uppercase_keys" {
		t.Errorf("expected transform_rule metadata = 'uppercase_keys', got %v", result.Metadata["transform_rule"])
	}
}

func TestJSONTool_UnknownAction(t *testing.T) {
	t.Parallel()
	tool := NewJSONTool()
	args := `{"action":"nonexistent","data":"{}"}`
	_, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err == nil {
		t.Error("unknown action should return error")
	}
}

// ===== 工具元数据测试 =====

func TestHTTPClientTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := NewHTTPClientTool()
	if tool.Name() != "http_client" {
		t.Errorf("expected name 'http_client', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("parameters should not be empty")
	}
}

func TestGitTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := NewGitTool(t.TempDir())
	if tool.Name() != "git_tool" {
		t.Errorf("expected name 'git_tool', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("parameters should not be empty")
	}
}

func TestGitTool_Execute_InvalidParams(t *testing.T) {
	t.Parallel()
	tool := NewGitTool(t.TempDir())

	// 测试无效 JSON
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("invalid JSON should return error")
	}

	// 测试缺少 action
	_, err = tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Error("missing action should return error")
	}

	// 测试未知 action
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"action":"unknown"}`))
	if err == nil {
		t.Error("unknown action should return error")
	}
}

func TestGitTool_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	tool := NewGitTool(t.TempDir())

	// commit 缺少 message
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"commit"}`))
	if err == nil {
		t.Error("commit without message should return error")
	}

	// checkout 缺少 branch
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"action":"checkout"}`))
	if err == nil {
		t.Error("checkout without branch should return error")
	}
}

func TestSearchTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := NewSearchTool()
	if tool.Name() != "web_search" {
		t.Errorf("expected name 'web_search', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("parameters should not be empty")
	}
}

func TestCSVTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := NewCSVTool()
	if tool.Name() != "csv_processor" {
		t.Errorf("expected name 'csv_processor', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("parameters should not be empty")
	}
}

func TestJSONTool_Metadata(t *testing.T) {
	t.Parallel()
	tool := NewJSONTool()
	if tool.Name() != "json_processor" {
		t.Errorf("expected name 'json_processor', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("parameters should not be empty")
	}
}

func TestSQLiteTool_Metadata(t *testing.T) {
	t.Parallel()
	tool, _ := setupTestDB(t)
	if tool.Name() != "sqlite_processor" {
		t.Errorf("expected name 'sqlite_processor', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("parameters should not be empty")
	}
}

// ===== Plugin 测试 =====

func TestPluginLoader_Get(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	loader := NewPluginLoader(reg)
	plugin := &mockPlugin{name: "test_plugin", version: "1.0.0"}
	_ = loader.Load(plugin)

	got, ok := loader.Get("test_plugin")
	if !ok {
		t.Fatal("plugin should exist")
	}
	if got.Name() != "test_plugin" {
		t.Errorf("expected name 'test_plugin', got '%s'", got.Name())
	}
}

func TestPluginLoader_Get_NotFound(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	loader := NewPluginLoader(reg)
	_, ok := loader.Get("nonexistent")
	if ok {
		t.Error("plugin should not exist")
	}
}

// ===== 辅助函数 =====

// jsonString 将字符串转为 JSON 格式的带引号字符串
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
