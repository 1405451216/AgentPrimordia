package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- JSONLoader 测试 ---

func TestJSONLoader_Name(t *testing.T) {
	jl := NewJSONLoader()
	if jl.Name() != "json_loader" {
		t.Errorf("expected 'json_loader', got '%s'", jl.Name())
	}
}

func TestJSONLoader_Description(t *testing.T) {
	jl := NewJSONLoader()
	desc := jl.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "JSON") {
		t.Error("description should mention JSON")
	}
}

func TestJSONLoader_Parameters(t *testing.T) {
	jl := NewJSONLoader()
	params := jl.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("invalid JSON schema: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected type object, got %v", schema["type"])
	}
}

func TestJSONLoader_LoadBasicJSON(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"name": "Alice", "age": 30, "city": "Beijing"}`
	testFile := filepath.Join(tmpDir, "test.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Alice") {
		t.Errorf("expected content containing 'Alice', got '%s'", result.Content)
	}
}

func TestJSONLoader_QuerySimpleKey(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"name": "Alice", "age": 30}`
	testFile := filepath.Join(tmpDir, "query.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "query",
		"path":   testFile,
		"query":  "name",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Alice") {
		t.Errorf("expected content containing 'Alice', got '%s'", result.Content)
	}
}

func TestJSONLoader_QueryNestedKey(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"user": {"name": "Alice", "address": {"city": "Beijing"}}}`
	testFile := filepath.Join(tmpDir, "nested.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "query",
		"path":   testFile,
		"query":  "user.address.city",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Beijing") {
		t.Errorf("expected content containing 'Beijing', got '%s'", result.Content)
	}
}

func TestJSONLoader_QueryArrayIndex(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"users": [{"name": "Alice"}, {"name": "Bob"}]}`
	testFile := filepath.Join(tmpDir, "array.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "query",
		"path":   testFile,
		"query":  "users[0].name",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Alice") {
		t.Errorf("expected content containing 'Alice', got '%s'", result.Content)
	}
}

func TestJSONLoader_QueryArraySecondElement(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"users": [{"name": "Alice"}, {"name": "Bob"}]}`
	testFile := filepath.Join(tmpDir, "array2.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "query",
		"path":   testFile,
		"query":  "users[1].name",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Bob") {
		t.Errorf("expected content containing 'Bob', got '%s'", result.Content)
	}
}

func TestJSONLoader_QueryKeyNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"name": "Alice"}`
	testFile := filepath.Join(tmpDir, "notfound.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "query",
		"path":   testFile,
		"query":  "nonexistent",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent key")
	}
}

func TestJSONLoader_QueryIndexOutOfRange(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"items": [1, 2, 3]}`
	testFile := filepath.Join(tmpDir, "outofrange.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "query",
		"path":   testFile,
		"query":  "items[10]",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for out-of-range index")
	}
}

func TestJSONLoader_FlattenNestedObject(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"user": {"name": "Alice", "address": {"city": "Beijing"}}}`
	testFile := filepath.Join(tmpDir, "flatten.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "flatten",
		"path":   testFile,
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var flattened map[string]any
	if err := json.Unmarshal([]byte(result.Content), &flattened); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if flattened["user.name"] != "Alice" {
		t.Errorf("expected user.name=Alice, got %v", flattened["user.name"])
	}
	if flattened["user.address.city"] != "Beijing" {
		t.Errorf("expected user.address.city=Beijing, got %v", flattened["user.address.city"])
	}
}

func TestJSONLoader_FlattenWithArray(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"items": ["a", "b", "c"]}`
	testFile := filepath.Join(tmpDir, "flatten_arr.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "flatten",
		"path":   testFile,
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var flattened map[string]any
	if err := json.Unmarshal([]byte(result.Content), &flattened); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if flattened["items.0"] != "a" {
		t.Errorf("expected items.0=a, got %v", flattened["items.0"])
	}
	if flattened["items.2"] != "c" {
		t.Errorf("expected items.2=c, got %v", flattened["items.2"])
	}
}

func TestJSONLoader_FlattenCustomSeparator(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"user": {"name": "Alice"}}`
	testFile := filepath.Join(tmpDir, "flatten_sep.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action":            "flatten",
		"path":              testFile,
		"flatten_separator": "_",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var flattened map[string]any
	if err := json.Unmarshal([]byte(result.Content), &flattened); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if flattened["user_name"] != "Alice" {
		t.Errorf("expected user_name=Alice, got %v", flattened["user_name"])
	}
}

func TestJSONLoader_FileNotFound(t *testing.T) {
	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   "/nonexistent/file.json",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent file")
	}
}

func TestJSONLoader_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.json")
	os.WriteFile(testFile, []byte("{invalid json}"), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid JSON")
	}
}

func TestJSONLoader_InvalidAction(t *testing.T) {
	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "invalid",
		"path":   "test.json",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestJSONLoader_MissingPath(t *testing.T) {
	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing path")
	}
}

func TestJSONLoader_QueryMissingQuery(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{"name": "Alice"}`
	testFile := filepath.Join(tmpDir, "noquery.json")
	os.WriteFile(testFile, []byte(content), 0644)

	jl := NewJSONLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "query",
		"path":   testFile,
	})
	result, err := jl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing query")
	}
}

// --- 内部函数测试 ---

func TestSplitJSONPath(t *testing.T) {
	tests := []struct {
		path     string
		expected []string
	}{
		{"name", []string{"name"}},
		{"user.name", []string{"user", "name"}},
		{"users[0].name", []string{"users", "[0]", "name"}},
		{"a.b[2].c", []string{"a", "b", "[2]", "c"}},
		{"items[0]", []string{"items", "[0]"}},
	}
	for _, tt := range tests {
		result := splitJSONPath(tt.path)
		if len(result) != len(tt.expected) {
			t.Errorf("splitJSONPath(%q): expected %v, got %v", tt.path, tt.expected, result)
			continue
		}
		for i, part := range result {
			if part != tt.expected[i] {
				t.Errorf("splitJSONPath(%q)[%d]: expected %q, got %q", tt.path, i, tt.expected[i], part)
			}
		}
	}
}

func TestFlattenJSON(t *testing.T) {
	data := map[string]any{
		"user": map[string]any{
			"name": "Alice",
			"tags": []any{"a", "b"},
		},
	}
	result := flattenJSON(data, "", ".")
	if result["user.name"] != "Alice" {
		t.Errorf("expected user.name=Alice, got %v", result["user.name"])
	}
	if result["user.tags.0"] != "a" {
		t.Errorf("expected user.tags.0=a, got %v", result["user.tags.0"])
	}
}
