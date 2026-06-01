package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustNewFS(t *testing.T, dir string) *FileSystem {
	t.Helper()
	fs, err := NewFileSystem(dir)
	if err != nil {
		t.Fatalf("NewFileSystem(%q) error: %v", dir, err)
	}
	return fs
}

func TestFileSystem_Name(t *testing.T) {
	fs := mustNewFS(t, ".")
	if fs.Name() != "filesystem" {
		t.Errorf("expected 'filesystem', got '%s'", fs.Name())
	}
}

func TestFileSystem_Description(t *testing.T) {
	fs := mustNewFS(t, ".")
	desc := fs.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "file") && !strings.Contains(desc, "File") {
		t.Error("description should mention file operations")
	}
}

func TestFileSystem_Parameters(t *testing.T) {
	fs := mustNewFS(t, ".")
	params := fs.Parameters()
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

func TestReadFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello world\nline2\nline3"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "read",
		"path":   "test.txt",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello world") {
		t.Errorf("expected content containing 'hello world', got '%s'", result.Content)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "read",
		"path":   "nonexistent.txt",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("should be error for missing file, got: %s", result.Content)
	}
}

func TestReadFile_WithLineRange(t *testing.T) {
	tmpDir := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\n"
	os.WriteFile(filepath.Join(tmpDir, "range.txt"), []byte(content), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]any{
		"action":     "read",
		"path":       "range.txt",
		"start_line": float64(2),
		"end_line":   float64(4),
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if strings.Contains(result.Content, "line1") {
		t.Error("should not contain line1")
	}
	if !strings.Contains(result.Content, "line2") {
		t.Error("should contain line2")
	}
	if !strings.Contains(result.Content, "line3") {
		t.Error("should contain line3")
	}
	if !strings.Contains(result.Content, "line4") {
		t.Error("should contain line4")
	}
	if strings.Contains(result.Content, "line5") {
		t.Error("should not contain line5")
	}
}

func TestWriteFile_Create(t *testing.T) {
	tmpDir := t.TempDir()
	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action":  "write",
		"path":    "newfile.txt",
		"content": "brand new content",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "newfile.txt"))
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if string(data) != "brand new content" {
		t.Errorf("expected 'brand new content', got '%s'", string(data))
	}
}

func TestWriteFile_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "overwrite.txt"), []byte("original"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action":  "write",
		"path":    "overwrite.txt",
		"content": "updated content",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "overwrite.txt"))
	if string(data) != "updated content" {
		t.Errorf("expected 'updated content', got '%s'", string(data))
	}
}

func TestEditFile_Replace(t *testing.T) {
	tmpDir := t.TempDir()
	original := "hello old world\nthis is other text"
	os.WriteFile(filepath.Join(tmpDir, "edit.txt"), []byte(original), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action":  "edit",
		"path":    "edit.txt",
		"old_str": "old",
		"new_str": "new",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "edit.txt"))
	if !strings.Contains(string(data), "hello new world") {
		t.Errorf("replacement failed, got: %s", string(data))
	}
	if strings.Contains(string(data), "old") {
		t.Error("'old' should have been replaced")
	}
}

func TestListDir(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("b"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "list_dir",
		"path":   ".",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if !strings.Contains(result.Content, "a.txt") {
		t.Errorf("should list a.txt, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "b.go") {
		t.Errorf("should list b.go, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "subdir") {
		t.Errorf("should list subdir, got: %s", result.Content)
	}
}

func TestSearchInFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := "apple banana cherry\ndate elderberry fig\ngrape honeydew ice"
	os.WriteFile(filepath.Join(tmpDir, "search.txt"), []byte(content), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "search",
		"path":   "search.txt",
		"query":  "berry",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	if !strings.Contains(result.Content, "elderberry") {
		t.Errorf("should find 'elderberry', got: %s", result.Content)
	}
}

func TestFileInfo(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "info.txt")
	os.WriteFile(testFile, []byte("metadata test"), 0644)

	time.Sleep(10 * time.Millisecond)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "file_info",
		"path":   "info.txt",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("error: %v, result: %v", err, result)
	}
	var info map[string]any
	if err := json.Unmarshal([]byte(result.Content), &info); err != nil {
		t.Fatalf("result should be JSON: %v", err)
	}
	if info["name"] != "info.txt" {
		t.Errorf("expected name 'info.txt', got '%v'", info["name"])
	}
	if size, ok := info["size"].(float64); !ok || size == 0 {
		t.Errorf("size should be > 0, got %v", info["size"])
	}
}

func TestPathTraversal_Blocked(t *testing.T) {
	tmpDir := t.TempDir()
	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "read",
		"path":   "../../etc/passwd",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("path traversal should be blocked, got: %s", result.Content)
	}
	if !strings.Contains(strings.ToLower(result.Content), "traversal") &&
		!strings.Contains(strings.ToLower(result.Content), "denied") &&
		!strings.Contains(strings.ToLower(result.Content), "outside") {
		t.Errorf("error message should indicate path traversal, got: %s", result.Content)
	}
}

func TestSensitiveFile_Protected(t *testing.T) {
	tmpDir := t.TempDir()
	sensitivePath := filepath.Join(tmpDir, ".env")
	os.WriteFile(sensitivePath, []byte("SECRET_KEY=abc123"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "read",
		"path":   ".env",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("sensitive file .env should be protected, got: %s", result.Content)
	}
}

func TestInvalidAction(t *testing.T) {
	tmpDir := t.TempDir()
	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "nonexistent_action",
		"path":   "test.txt",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("invalid action should return error, got: %s", result.Content)
	}
}
