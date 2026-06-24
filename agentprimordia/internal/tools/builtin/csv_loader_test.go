package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- CSVLoader 测试 ---

func TestCSVLoader_Name(t *testing.T) {
	cl := NewCSVLoader()
	if cl.Name() != "csv_loader" {
		t.Errorf("expected 'csv_loader', got '%s'", cl.Name())
	}
}

func TestCSVLoader_Description(t *testing.T) {
	cl := NewCSVLoader()
	desc := cl.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "CSV") {
		t.Error("description should mention CSV")
	}
}

func TestCSVLoader_Parameters(t *testing.T) {
	cl := NewCSVLoader()
	params := cl.Parameters()
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

func TestCSVLoader_LoadBasicCSV(t *testing.T) {
	tmpDir := t.TempDir()
	content := "name,age,city\nAlice,30,Beijing\nBob,25,Shanghai\n"
	testFile := filepath.Join(tmpDir, "test.csv")
	os.WriteFile(testFile, []byte(content), 0644)

	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var data CSVData
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(data.Headers) != 3 {
		t.Errorf("expected 3 headers, got %d: %v", len(data.Headers), data.Headers)
	}
	if data.Total != 2 {
		t.Errorf("expected 2 rows, got %d", data.Total)
	}
	if data.Rows[0]["name"] != "Alice" {
		t.Errorf("expected first row name 'Alice', got '%s'", data.Rows[0]["name"])
	}
	if data.Rows[1]["city"] != "Shanghai" {
		t.Errorf("expected second row city 'Shanghai', got '%s'", data.Rows[1]["city"])
	}
}

func TestCSVLoader_CustomDelimiter(t *testing.T) {
	tmpDir := t.TempDir()
	content := "name;age;city\nAlice;30;Beijing\n"
	testFile := filepath.Join(tmpDir, "test.csv")
	os.WriteFile(testFile, []byte(content), 0644)

	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action":    "load",
		"path":      testFile,
		"delimiter": ";",
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var data CSVData
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(data.Headers) != 3 {
		t.Errorf("expected 3 headers, got %d", len(data.Headers))
	}
	if data.Rows[0]["name"] != "Alice" {
		t.Errorf("expected name 'Alice', got '%s'", data.Rows[0]["name"])
	}
}

func TestCSVLoader_NoHeader(t *testing.T) {
	tmpDir := t.TempDir()
	content := "Alice,30,Beijing\nBob,25,Shanghai\n"
	testFile := filepath.Join(tmpDir, "noheader.csv")
	os.WriteFile(testFile, []byte(content), 0644)

	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action":     "load",
		"path":       testFile,
		"has_header": false,
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var data CSVData
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	// 无标题行时自动生成 col_0, col_1, ...
	if data.Headers[0] != "col_0" {
		t.Errorf("expected auto-generated header 'col_0', got '%s'", data.Headers[0])
	}
	if data.Total != 2 {
		t.Errorf("expected 2 rows, got %d", data.Total)
	}
}

func TestCSVLoader_ColumnFilter(t *testing.T) {
	tmpDir := t.TempDir()
	content := "name,age,city\nAlice,30,Beijing\nBob,25,Shanghai\n"
	testFile := filepath.Join(tmpDir, "filter.csv")
	os.WriteFile(testFile, []byte(content), 0644)

	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action":  "load",
		"path":    testFile,
		"columns": []string{"name", "city"},
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var data CSVData
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(data.Headers) != 2 {
		t.Errorf("expected 2 filtered headers, got %d: %v", len(data.Headers), data.Headers)
	}
	// 过滤后不应包含 age 字段
	if _, ok := data.Rows[0]["age"]; ok {
		t.Error("filtered result should not contain 'age' field")
	}
	if data.Rows[0]["name"] != "Alice" {
		t.Errorf("expected name 'Alice', got '%s'", data.Rows[0]["name"])
	}
}

func TestCSVLoader_QuotedFields(t *testing.T) {
	tmpDir := t.TempDir()
	content := `name,description
Alice,"Hello, World"
Bob,"He said ""hi"""
`
	testFile := filepath.Join(tmpDir, "quoted.csv")
	os.WriteFile(testFile, []byte(content), 0644)

	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var data CSVData
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if data.Rows[0]["description"] != "Hello, World" {
		t.Errorf("expected 'Hello, World', got '%s'", data.Rows[0]["description"])
	}
	if data.Rows[1]["description"] != `He said "hi"` {
		t.Errorf("expected 'He said \"hi\"', got '%s'", data.Rows[1]["description"])
	}
}

func TestCSVLoader_FileNotFound(t *testing.T) {
	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   "/nonexistent/file.csv",
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent file")
	}
}

func TestCSVLoader_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.csv")
	os.WriteFile(testFile, []byte(""), 0644)

	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for empty file")
	}
}

func TestCSVLoader_InvalidAction(t *testing.T) {
	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "invalid",
		"path":   "test.csv",
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestCSVLoader_MissingPath(t *testing.T) {
	cl := NewCSVLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
	})
	result, err := cl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing path")
	}
}

func TestParseCSVLine_Simple(t *testing.T) {
	fields := parseCSVLine("a,b,c", ",")
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	if fields[0] != "a" || fields[1] != "b" || fields[2] != "c" {
		t.Errorf("expected [a,b,c], got %v", fields)
	}
}

func TestParseCSVLine_QuotedField(t *testing.T) {
	fields := parseCSVLine(`a,"b,c",d`, ",")
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(fields), fields)
	}
	if fields[1] != "b,c" {
		t.Errorf("expected 'b,c', got '%s'", fields[1])
	}
}

func TestParseCSVLine_EscapedQuote(t *testing.T) {
	fields := parseCSVLine(`a,"b""c",d`, ",")
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(fields), fields)
	}
	if fields[1] != `b"c` {
		t.Errorf("expected 'b\"c', got '%s'", fields[1])
	}
}

func TestParseCSVLine_TabDelimiter(t *testing.T) {
	fields := parseCSVLine("a\tb\tc", "\t")
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	if fields[0] != "a" || fields[1] != "b" || fields[2] != "c" {
		t.Errorf("expected [a,b,c], got %v", fields)
	}
}
