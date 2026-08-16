package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileSystem_Edit_UniqueMatch(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	testFile := filepath.Join(dir, "unique.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	args, _ := json.Marshal(map[string]string{
		"action":  "edit",
		"path":    "unique.txt",
		"old_str": "hello",
		"new_str": "goodbye",
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success for unique match, got error: %s", result.Content)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "goodbye world" {
		t.Errorf("content = %q, want %q", string(data), "goodbye world")
	}
}

func TestFileSystem_Edit_NoMatch(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	testFile := filepath.Join(dir, "nomatch.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	args, _ := json.Marshal(map[string]string{
		"action":  "edit",
		"path":    "nomatch.txt",
		"old_str": "not_found",
		"new_str": "replacement",
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for no match")
	}
}

func TestFileSystem_Edit_MultipleMatches(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	testFile := filepath.Join(dir, "multi.txt")
	os.WriteFile(testFile, []byte("aaa bbb aaa"), 0644)

	args, _ := json.Marshal(map[string]string{
		"action":  "edit",
		"path":    "multi.txt",
		"old_str": "aaa",
		"new_str": "ccc",
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for multiple matches")
	}
	if !contains(result.Content, "2 times") {
		t.Errorf("expected '2 times' in error message, got %q", result.Content)
	}
}

func TestFileSystem_Read_MaxSize(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	bigFile := filepath.Join(dir, "big.txt")
	bigContent := make([]byte, 5*1024*1024+1)
	os.WriteFile(bigFile, bigContent, 0644)

	args, _ := json.Marshal(map[string]string{
		"action": "read",
		"path":   "big.txt",
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for file too large")
	}
}

func TestFileSystem_Write_MaxSize(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	bigContent := make([]byte, 11*1024*1024+1)
	for i := range bigContent {
		bigContent[i] = 'a'
	}

	args, _ := json.Marshal(map[string]string{
		"action":  "write",
		"path":    "big.txt",
		"content": string(bigContent),
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for content too large")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFileSystem_SymlinkEscape_Read(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests skipped on Windows (requires developer mode)")
	}
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	// 在 root 外创建一个文件
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	os.WriteFile(outsideFile, []byte("secret data"), 0644)

	// 创建指向 root 外的 symlink
	linkPath := filepath.Join(dir, "escape_link")
	os.Symlink(outsideFile, linkPath)

	args, _ := json.Marshal(map[string]string{
		"action": "read",
		"path":   "escape_link",
	})

	result, err := fs.Execute(context.Background(), args)
	// 逃逸必须被拦截：以 result.IsError 表达拒绝（err 可能同时非 nil，
	// 与 scope 拒绝路径行为一致——重构后不再要求 err == nil）
	if result == nil || !result.IsError {
		t.Errorf("expected symlink escape to be blocked, result=%v err=%v", result, err)
	}
}

func TestFileSystem_SymlinkEscape_Write(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests skipped on Windows (requires developer mode)")
	}
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	// 在 root 外创建一个目录
	outsideDir := t.TempDir()

	// 创建指向 root 外的 symlink
	linkPath := filepath.Join(dir, "escape_link")
	os.Symlink(outsideDir, linkPath)

	args, _ := json.Marshal(map[string]string{
		"action":  "write",
		"path":    "escape_link/evil.txt",
		"content": "malicious data",
	})

	result, err := fs.Execute(context.Background(), args)
	// 逃逸写入必须被拦截：以 result.IsError 表达拒绝（err 可同时非 nil）
	if result == nil || !result.IsError {
		t.Errorf("expected symlink escape write to be blocked, result=%v err=%v", result, err)
	}
}

func TestFileSystem_SymlinkWithinRoot_Read(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests skipped on Windows (requires developer mode)")
	}
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	// 在 root 内创建文件和 symlink
	targetFile := filepath.Join(dir, "real.txt")
	os.WriteFile(targetFile, []byte("safe content"), 0644)

	linkPath := filepath.Join(dir, "safe_link")
	os.Symlink(targetFile, linkPath)

	args, _ := json.Marshal(map[string]string{
		"action": "read",
		"path":   "safe_link",
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected symlink within root to be allowed, got: %s", result.Content)
	}
}

func TestFileSystem_SymlinkWithinRoot_Edit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests skipped on Windows (requires developer mode)")
	}
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	// 在 root 内创建文件和 symlink
	targetFile := filepath.Join(dir, "edit_real.txt")
	os.WriteFile(targetFile, []byte("hello world"), 0644)

	linkPath := filepath.Join(dir, "edit_link")
	os.Symlink(targetFile, linkPath)

	args, _ := json.Marshal(map[string]string{
		"action":  "edit",
		"path":    "edit_link",
		"old_str": "hello",
		"new_str": "goodbye",
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected symlink edit within root to be allowed, got: %s", result.Content)
	}
}

func TestFileSystem_Search_ReDoSProtection(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("aaa bbb ccc"), 0644)

	// 嵌套重复量词应被拒绝
	args, _ := json.Marshal(map[string]any{
		"action": "search",
		"path":   "test.txt",
		"query":  "(a+)+b",
		"regex":  true,
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected ReDoS pattern to be rejected")
	}
	if !contains(result.Content, "backtracking") {
		t.Errorf("expected backtracking warning, got: %s", result.Content)
	}
}

func TestFileSystem_Search_ValidRegex(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileSystem(dir)

	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello world\nfoo bar"), 0644)

	args, _ := json.Marshal(map[string]any{
		"action": "search",
		"path":   "test.txt",
		"query":  "hel+o",
		"regex":  true,
	})

	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected valid regex to work, got: %s", result.Content)
	}
}
