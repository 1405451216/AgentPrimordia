package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- MarkdownLoader 测试 ---

func TestMarkdownLoader_Name(t *testing.T) {
	ml := NewMarkdownLoader()
	if ml.Name() != "markdown_loader" {
		t.Errorf("expected 'markdown_loader', got '%s'", ml.Name())
	}
}

func TestMarkdownLoader_Description(t *testing.T) {
	ml := NewMarkdownLoader()
	desc := ml.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "Markdown") {
		t.Error("description should mention Markdown")
	}
}

func TestMarkdownLoader_Parameters(t *testing.T) {
	ml := NewMarkdownLoader()
	params := ml.Parameters()
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

func TestMarkdownLoader_LoadBasicDocument(t *testing.T) {
	tmpDir := t.TempDir()
	content := `# 主标题

这是第一段内容。

## 二级标题

这是二级标题下的内容。

### 三级标题

三级标题下的内容。
`
	testFile := filepath.Join(tmpDir, "test.md")
	os.WriteFile(testFile, []byte(content), 0644)

	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
		"path":   testFile,
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc MarkdownDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.Title != "主标题" {
		t.Errorf("expected title '主标题', got '%s'", doc.Title)
	}
	if len(doc.Sections) < 3 {
		t.Errorf("expected at least 3 sections, got %d", len(doc.Sections))
	}
}

func TestMarkdownLoader_CodeBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	content := `# 代码示例

下面是 Go 代码：

` + "```go" + `
package main

func main() {
    println("hello")
}
` + "```" + `

下面是 Python 代码：

` + "```python" + `
print("hello")
` + "```" + `
`
	testFile := filepath.Join(tmpDir, "code.md")
	os.WriteFile(testFile, []byte(content), 0644)

	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
		"path":   testFile,
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc MarkdownDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(doc.CodeBlocks) < 2 {
		t.Errorf("expected at least 2 code blocks, got %d", len(doc.CodeBlocks))
	}
	foundGo := false
	foundPython := false
	for _, cb := range doc.CodeBlocks {
		if cb.Language == "go" {
			foundGo = true
			if !strings.Contains(cb.Code, "package main") {
				t.Errorf("go code block should contain 'package main', got '%s'", cb.Code)
			}
		}
		if cb.Language == "python" {
			foundPython = true
		}
	}
	if !foundGo {
		t.Error("expected to find a Go code block")
	}
	if !foundPython {
		t.Error("expected to find a Python code block")
	}
}

func TestMarkdownLoader_LinksAndImages(t *testing.T) {
	tmpDir := t.TempDir()
	content := `# 链接与图片

这是一个 [示例链接](https://example.com) 和另一个 [文档](https://docs.example.com)。

![图片描述](https://example.com/image.png)

普通文本。
`
	testFile := filepath.Join(tmpDir, "links.md")
	os.WriteFile(testFile, []byte(content), 0644)

	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
		"path":   testFile,
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc MarkdownDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(doc.Links) < 2 {
		t.Errorf("expected at least 2 links, got %d: %v", len(doc.Links), doc.Links)
	}
	if len(doc.Images) < 1 {
		t.Errorf("expected at least 1 image, got %d: %v", len(doc.Images), doc.Images)
	}
}

func TestMarkdownLoader_FileNotFound(t *testing.T) {
	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
		"path":   "/nonexistent/file.md",
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent file")
	}
}

func TestMarkdownLoader_InvalidAction(t *testing.T) {
	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "invalid",
		"path":   "test.md",
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestMarkdownLoader_MissingPath(t *testing.T) {
	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing path")
	}
}

func TestMarkdownLoader_RawContent(t *testing.T) {
	tmpDir := t.TempDir()
	content := "# 标题\n\n内容\n"
	testFile := filepath.Join(tmpDir, "raw.md")
	os.WriteFile(testFile, []byte(content), 0644)

	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
		"path":   testFile,
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc MarkdownDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.RawContent != content {
		t.Errorf("raw content mismatch: expected %q, got %q", content, doc.RawContent)
	}
}

func TestMarkdownLoader_SectionLevels(t *testing.T) {
	tmpDir := t.TempDir()
	content := `# 一级

## 二级

### 三级

#### 四级

##### 五级

###### 六级
`
	testFile := filepath.Join(tmpDir, "levels.md")
	os.WriteFile(testFile, []byte(content), 0644)

	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
		"path":   testFile,
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc MarkdownDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	expectedLevels := []int{1, 2, 3, 4, 5, 6}
	if len(doc.Sections) < len(expectedLevels) {
		t.Fatalf("expected at least %d sections, got %d", len(expectedLevels), len(doc.Sections))
	}
	for i, level := range expectedLevels {
		if doc.Sections[i].Level != level {
			t.Errorf("section %d: expected level %d, got %d", i, level, doc.Sections[i].Level)
		}
	}
}

func TestMarkdownLoader_CodeBlockWithoutLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	content := `# 无语言标记的代码块

` + "```" + `
some code here
` + "```" + `
`
	testFile := filepath.Join(tmpDir, "nolang.md")
	os.WriteFile(testFile, []byte(content), 0644)

	ml := NewMarkdownLoader()
	args, _ := json.Marshal(map[string]string{
		"action": "load",
		"path":   testFile,
	})
	result, err := ml.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc MarkdownDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(doc.CodeBlocks) < 1 {
		t.Fatal("expected at least 1 code block")
	}
	if doc.CodeBlocks[0].Language != "" {
		t.Errorf("expected empty language, got '%s'", doc.CodeBlocks[0].Language)
	}
	if !strings.Contains(doc.CodeBlocks[0].Code, "some code here") {
		t.Errorf("code block should contain 'some code here', got '%s'", doc.CodeBlocks[0].Code)
	}
}
