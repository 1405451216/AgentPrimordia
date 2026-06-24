package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- HTMLLoader 测试 ---

func TestHTMLLoader_Name(t *testing.T) {
	hl := NewHTMLLoader()
	if hl.Name() != "html_loader" {
		t.Errorf("expected 'html_loader', got '%s'", hl.Name())
	}
}

func TestHTMLLoader_Description(t *testing.T) {
	hl := NewHTMLLoader()
	desc := hl.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "HTML") {
		t.Error("description should mention HTML")
	}
}

func TestHTMLLoader_Parameters(t *testing.T) {
	hl := NewHTMLLoader()
	params := hl.Parameters()
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

func TestHTMLLoader_LoadBasicHTML(t *testing.T) {
	tmpDir := t.TempDir()
	content := `<!DOCTYPE html>
<html>
<head>
<title>测试页面</title>
<meta name="description" content="这是一个测试页面">
<meta name="author" content="张三">
</head>
<body>
<h1>主标题</h1>
<p>这是段落内容。</p>
<h2>二级标题</h2>
<p>更多内容。</p>
</body>
</html>`
	testFile := filepath.Join(tmpDir, "test.html")
	os.WriteFile(testFile, []byte(content), 0644)

	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc HTMLDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.Title != "测试页面" {
		t.Errorf("expected title '测试页面', got '%s'", doc.Title)
	}
	if doc.Meta["description"] != "这是一个测试页面" {
		t.Errorf("expected meta description '这是一个测试页面', got '%s'", doc.Meta["description"])
	}
	if doc.Meta["author"] != "张三" {
		t.Errorf("expected meta author '张三', got '%s'", doc.Meta["author"])
	}
	if len(doc.Headings) < 2 {
		t.Errorf("expected at least 2 headings, got %d", len(doc.Headings))
	}
	if doc.Headings[0].Level != 1 || doc.Headings[0].Content != "主标题" {
		t.Errorf("expected heading level 1 '主标题', got level %d '%s'", doc.Headings[0].Level, doc.Headings[0].Content)
	}
	if !strings.Contains(doc.Content, "段落内容") {
		t.Errorf("content should contain '段落内容', got '%s'", doc.Content)
	}
}

func TestHTMLLoader_Links(t *testing.T) {
	tmpDir := t.TempDir()
	content := `<html><body>
<a href="https://example.com">示例链接</a>
<a href="https://docs.example.com">文档</a>
</body></html>`
	testFile := filepath.Join(tmpDir, "links.html")
	os.WriteFile(testFile, []byte(content), 0644)

	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc HTMLDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(doc.Links) < 2 {
		t.Errorf("expected at least 2 links, got %d", len(doc.Links))
	}
	foundExample := false
	foundDocs := false
	for _, link := range doc.Links {
		if link.Href == "https://example.com" && link.Text == "示例链接" {
			foundExample = true
		}
		if link.Href == "https://docs.example.com" && link.Text == "文档" {
			foundDocs = true
		}
	}
	if !foundExample {
		t.Error("expected to find link to example.com")
	}
	if !foundDocs {
		t.Error("expected to find link to docs.example.com")
	}
}

func TestHTMLLoader_Images(t *testing.T) {
	tmpDir := t.TempDir()
	content := `<html><body>
<img src="https://example.com/img1.png" alt="图片1">
<img alt="图片2" src="https://example.com/img2.png">
</body></html>`
	testFile := filepath.Join(tmpDir, "images.html")
	os.WriteFile(testFile, []byte(content), 0644)

	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc HTMLDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(doc.Images) < 2 {
		t.Errorf("expected at least 2 images, got %d: %+v", len(doc.Images), doc.Images)
	}
	foundImg1 := false
	foundImg2 := false
	for _, img := range doc.Images {
		if img.Src == "https://example.com/img1.png" && img.Alt == "图片1" {
			foundImg1 = true
		}
		if img.Src == "https://example.com/img2.png" && img.Alt == "图片2" {
			foundImg2 = true
		}
	}
	if !foundImg1 {
		t.Error("expected to find image img1.png with alt '图片1'")
	}
	if !foundImg2 {
		t.Error("expected to find image img2.png with alt '图片2'")
	}
}

func TestHTMLLoader_Headings(t *testing.T) {
	tmpDir := t.TempDir()
	content := `<html><body>
<h1>一级标题</h1>
<h2>二级标题</h2>
<h3>三级标题</h3>
<h4>四级标题</h4>
<h5>五级标题</h5>
<h6>六级标题</h6>
</body></html>`
	testFile := filepath.Join(tmpDir, "headings.html")
	os.WriteFile(testFile, []byte(content), 0644)

	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc HTMLDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(doc.Headings) != 6 {
		t.Fatalf("expected 6 headings, got %d", len(doc.Headings))
	}
	expectedHeadings := []struct {
		level   int
		content string
	}{
		{1, "一级标题"}, {2, "二级标题"}, {3, "三级标题"},
		{4, "四级标题"}, {5, "五级标题"}, {6, "六级标题"},
	}
	for i, exp := range expectedHeadings {
		if doc.Headings[i].Level != exp.level {
			t.Errorf("heading %d: expected level %d, got %d", i, exp.level, doc.Headings[i].Level)
		}
		if doc.Headings[i].Content != exp.content {
			t.Errorf("heading %d: expected '%s', got '%s'", i, exp.content, doc.Headings[i].Content)
		}
	}
}

func TestHTMLLoader_ScriptStyleRemoval(t *testing.T) {
	tmpDir := t.TempDir()
	content := `<html><head>
<style>body { color: red; }</style>
</head><body>
<script>alert('hello');</script>
<p>可见内容</p>
</body></html>`
	testFile := filepath.Join(tmpDir, "script.html")
	os.WriteFile(testFile, []byte(content), 0644)

	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc HTMLDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if strings.Contains(doc.Content, "alert") {
		t.Errorf("content should not contain script content: %s", doc.Content)
	}
	if strings.Contains(doc.Content, "color: red") {
		t.Errorf("content should not contain style content: %s", doc.Content)
	}
	if !strings.Contains(doc.Content, "可见内容") {
		t.Errorf("content should contain '可见内容': %s", doc.Content)
	}
}

func TestHTMLLoader_HTMLEntities(t *testing.T) {
	tmpDir := t.TempDir()
	content := `<html><body>
<p>&amp; &lt; &gt; &quot; &#39; &nbsp;</p>
</body></html>`
	testFile := filepath.Join(tmpDir, "entities.html")
	os.WriteFile(testFile, []byte(content), 0644)

	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc HTMLDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if !strings.Contains(doc.Content, "&") {
		t.Errorf("content should contain decoded '&': %s", doc.Content)
	}
	if !strings.Contains(doc.Content, "<") {
		t.Errorf("content should contain decoded '<': %s", doc.Content)
	}
	if !strings.Contains(doc.Content, ">") {
		t.Errorf("content should contain decoded '>': %s", doc.Content)
	}
}

func TestHTMLLoader_FileNotFound(t *testing.T) {
	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   "/nonexistent/file.html",
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent file")
	}
}

func TestHTMLLoader_InvalidAction(t *testing.T) {
	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "invalid",
		"path":   "test.html",
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestHTMLLoader_MissingPath(t *testing.T) {
	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing path")
	}
}

func TestHTMLLoader_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.html")
	os.WriteFile(testFile, []byte(""), 0644)

	hl := NewHTMLLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := hl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for empty file")
	}
}

// --- parseHTML 内部函数测试 ---

func TestParseHTML_BasicExtraction(t *testing.T) {
	html := `<html><head><title>标题</title></head><body><p>内容</p></body></html>`
	doc := parseHTML(html)
	if doc.Title != "标题" {
		t.Errorf("expected title '标题', got '%s'", doc.Title)
	}
	if !strings.Contains(doc.Content, "内容") {
		t.Errorf("content should contain '内容', got '%s'", doc.Content)
	}
}

func TestParseHTML_MetaTags(t *testing.T) {
	html := `<html><head>
<meta name="keywords" content="go,agent,ai">
<meta name="description" content="测试描述">
</head></html>`
	doc := parseHTML(html)
	if doc.Meta["keywords"] != "go,agent,ai" {
		t.Errorf("expected keywords 'go,agent,ai', got '%s'", doc.Meta["keywords"])
	}
	if doc.Meta["description"] != "测试描述" {
		t.Errorf("expected description '测试描述', got '%s'", doc.Meta["description"])
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"&amp;", "&"},
		{"&lt;", "<"},
		{"&gt;", ">"},
		{"&quot;", "\""},
		{"&#39;", "'"},
		{"&apos;", "'"},
		{"&nbsp;", " "},
		{"a &amp; b", "a & b"},
	}
	for _, tt := range tests {
		got := decodeHTMLEntities(tt.input)
		if got != tt.expected {
			t.Errorf("decodeHTMLEntities(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestStripTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>hello</p>", "hello"},
		{"<div><span>text</span></div>", "text"},
		{"no tags here", "no tags here"},
		{"<br/>", ""},
	}
	for _, tt := range tests {
		got := stripTags(tt.input)
		if got != tt.expected {
			t.Errorf("stripTags(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCollapseWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello   world", "hello world"},
		{"line1\nline2", "line1 line2"},
		{"  spaces  ", " spaces "},
		{"tab\there", "tab here"},
	}
	for _, tt := range tests {
		got := collapseWhitespace(tt.input)
		if got != tt.expected {
			t.Errorf("collapseWhitespace(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
