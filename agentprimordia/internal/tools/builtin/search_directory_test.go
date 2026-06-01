package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSystem_SearchDirectory_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello world\nfoo bar\nhello again"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("no match here\nhello from b"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "search_directory",
		"path":   ".",
		"query":  "hello",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello") {
		t.Errorf("should contain 'hello', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "a.txt") {
		t.Errorf("should reference a.txt, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "b.txt") {
		t.Errorf("should reference b.txt, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "found 3 match") {
		t.Errorf("should find 3 matches, got: %s", result.Content)
	}
}

func TestFileSystem_SearchDirectory_WithInclude(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "code.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("# Hello\nThis is hello world"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "util.go"), []byte("package util\n// hello util"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action":  "search_directory",
		"path":    ".",
		"query":   "hello",
		"include": "*.go",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if strings.Contains(result.Content, "readme.md") {
		t.Errorf("should not include readme.md (filtered by *.go), got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "code.go") {
		t.Errorf("should include code.go, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "util.go") {
		t.Errorf("should include util.go, got: %s", result.Content)
	}
}

func TestFileSystem_SearchDirectory_NoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("apple banana cherry"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "search_directory",
		"path":   ".",
		"query":  "xyz_not_found",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "no matches found") {
		t.Errorf("should report no matches, got: %s", result.Content)
	}
}

func TestFileSystem_SearchDirectory_DeepNested(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "level1", "level2", "level3")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(tmpDir, "top.txt"), []byte("target at top"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "level1", "mid.txt"), []byte("target at mid"), 0644)
	os.WriteFile(filepath.Join(subDir, "deep.txt"), []byte("target at deep"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "search_directory",
		"path":   ".",
		"query":  "target",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "found 3 match") {
		t.Errorf("should find 3 matches across nested dirs, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "top.txt") {
		t.Errorf("should reference top.txt, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "mid.txt") {
		t.Errorf("should reference mid.txt, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "deep.txt") {
		t.Errorf("should reference deep.txt, got: %s", result.Content)
	}
}

func TestFileSystem_SearchDirectory_EmptyQuery(t *testing.T) {
	tmpDir := t.TempDir()
	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "search_directory",
		"path":   ".",
		"query":  "",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("empty query should return error")
	}
	if !strings.Contains(result.Content, "query is required") {
		t.Errorf("should mention query required, got: %s", result.Content)
	}
}

func TestFileSystem_SearchDirectory_NotDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content"), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]string{
		"action": "search_directory",
		"path":   "file.txt",
		"query":  "content",
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("search_directory on a file should return error")
	}
	if !strings.Contains(result.Content, "not a directory") {
		t.Errorf("should mention not a directory, got: %s", result.Content)
	}
}

func TestFileSystem_SearchDirectory_MaxResults(t *testing.T) {
	tmpDir := t.TempDir()
	var content string
	for i := 0; i < 20; i++ {
		content += "match_line\nother_line\n"
	}
	os.WriteFile(filepath.Join(tmpDir, "big.txt"), []byte(content), 0644)

	fs := mustNewFS(t, tmpDir)
	args, _ := json.Marshal(map[string]any{
		"action":      "search_directory",
		"path":        ".",
		"query":       "match_line",
		"max_results": float64(5),
	})
	result, err := fs.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "found 5 match") {
		t.Errorf("should limit to 5 matches, got: %s", result.Content)
	}
}
