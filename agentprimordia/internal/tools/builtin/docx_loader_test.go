package builtin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- DOCXLoader 测试 ---

func TestDOCXLoader_Name(t *testing.T) {
	dl := NewDOCXLoader()
	if dl.Name() != "docx_loader" {
		t.Errorf("expected 'docx_loader', got '%s'", dl.Name())
	}
}

func TestDOCXLoader_Description(t *testing.T) {
	dl := NewDOCXLoader()
	desc := dl.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "DOCX") {
		t.Error("description should mention DOCX")
	}
}

func TestDOCXLoader_Parameters(t *testing.T) {
	dl := NewDOCXLoader()
	params := dl.Parameters()
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

func TestDOCXLoader_LoadBasicDocument(t *testing.T) {
	tmpDir := t.TempDir()
	docxData := buildMinimalDOCX(
		[]DOCXParagraph{
			{Text: "Hello World", Style: ""},
			{Text: "第一章", Style: "Heading1", IsHeading: true},
			{Text: "这是正文内容。", Style: ""},
		},
		DOCXMetadata{
			Title:   "测试文档",
			Author:  "张三",
			Created: "2024-01-01T00:00:00Z",
		},
	)
	testFile := filepath.Join(tmpDir, "test.docx")
	os.WriteFile(testFile, docxData, 0644)

	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc DOCXDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(doc.Paragraphs) < 3 {
		t.Errorf("expected at least 3 paragraphs, got %d", len(doc.Paragraphs))
	}
	if doc.Paragraphs[0].Text != "Hello World" {
		t.Errorf("expected first paragraph 'Hello World', got '%s'", doc.Paragraphs[0].Text)
	}
	if doc.Metadata.Title != "测试文档" {
		t.Errorf("expected title '测试文档', got '%s'", doc.Metadata.Title)
	}
	if doc.Metadata.Author != "张三" {
		t.Errorf("expected author '张三', got '%s'", doc.Metadata.Author)
	}
}

func TestDOCXLoader_Headings(t *testing.T) {
	tmpDir := t.TempDir()
	docxData := buildMinimalDOCX(
		[]DOCXParagraph{
			{Text: "一级标题", Style: "Heading1", IsHeading: true},
			{Text: "正文段落", Style: ""},
			{Text: "二级标题", Style: "Heading2", IsHeading: true},
			{Text: "更多正文", Style: ""},
		},
		DOCXMetadata{},
	)
	testFile := filepath.Join(tmpDir, "headings.docx")
	os.WriteFile(testFile, docxData, 0644)

	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc DOCXDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	headingCount := 0
	for _, p := range doc.Paragraphs {
		if p.IsHeading {
			headingCount++
		}
	}
	if headingCount != 2 {
		t.Errorf("expected 2 headings, got %d", headingCount)
	}

	// 验证标题内容
	foundH1 := false
	foundH2 := false
	for _, p := range doc.Paragraphs {
		if p.Style == "Heading1" && p.Text == "一级标题" {
			foundH1 = true
		}
		if p.Style == "Heading2" && p.Text == "二级标题" {
			foundH2 = true
		}
	}
	if !foundH1 {
		t.Error("expected to find Heading1 '一级标题'")
	}
	if !foundH2 {
		t.Error("expected to find Heading2 '二级标题'")
	}
}

func TestDOCXLoader_Metadata(t *testing.T) {
	tmpDir := t.TempDir()
	docxData := buildMinimalDOCX(
		[]DOCXParagraph{
			{Text: "内容", Style: ""},
		},
		DOCXMetadata{
			Title:    "文档标题",
			Author:   "作者",
			Created:  "2024-06-01T10:00:00Z",
			Modified: "2024-06-15T14:30:00Z",
		},
	)
	testFile := filepath.Join(tmpDir, "meta.docx")
	os.WriteFile(testFile, docxData, 0644)

	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc DOCXDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.Metadata.Title != "文档标题" {
		t.Errorf("expected title '文档标题', got '%s'", doc.Metadata.Title)
	}
	if doc.Metadata.Author != "作者" {
		t.Errorf("expected author '作者', got '%s'", doc.Metadata.Author)
	}
	if doc.Metadata.Created != "2024-06-01T10:00:00Z" {
		t.Errorf("expected created '2024-06-01T10:00:00Z', got '%s'", doc.Metadata.Created)
	}
	if doc.Metadata.Modified != "2024-06-15T14:30:00Z" {
		t.Errorf("expected modified '2024-06-15T14:30:00Z', got '%s'", doc.Metadata.Modified)
	}
}

func TestDOCXLoader_FileNotFound(t *testing.T) {
	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   "/nonexistent/file.docx",
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent file")
	}
}

func TestDOCXLoader_InvalidAction(t *testing.T) {
	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "invalid",
		"path":   "test.docx",
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestDOCXLoader_MissingPath(t *testing.T) {
	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing path")
	}
}

func TestDOCXLoader_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.docx")
	os.WriteFile(testFile, []byte(""), 0644)

	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for empty file")
	}
}

func TestDOCXLoader_InvalidZIP(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.docx")
	os.WriteFile(testFile, []byte("This is not a ZIP file"), 0644)

	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid ZIP")
	}
}

func TestDOCXLoader_MissingDocumentXML(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建一个有效的 ZIP 但不包含 word/document.xml
	docxData := buildMinimalDOCXMissingDocument()
	testFile := filepath.Join(tmpDir, "nodoc.docx")
	os.WriteFile(testFile, docxData, 0644)

	dl := NewDOCXLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := dl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing document.xml")
	}
}

// --- DOCX 内部函数测试 ---

func TestExtractXMLTag(t *testing.T) {
	tests := []struct {
		content  string
		tag      string
		expected string
	}{
		{"<title>Hello</title>", "title", "Hello"},
		{"<dc:title>World</dc:title>", "dc:title", "World"},
		{"<name attr='val'>Content</name>", "name", "Content"},
		{"no tag here", "title", ""},
		{"<title></title>", "title", ""},
	}
	for _, tt := range tests {
		got := extractXMLTag(tt.content, tt.tag)
		if got != tt.expected {
			t.Errorf("extractXMLTag(%q, %q) = %q, want %q", tt.content, tt.tag, got, tt.expected)
		}
	}
}

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"<tag>", "&lt;tag&gt;"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"it's", "it&apos;s"},
	}
	for _, tt := range tests {
		got := escapeXML(tt.input)
		if got != tt.expected {
			t.Errorf("escapeXML(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseDocumentXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:pPr><w:pStyle w:val="Heading1"/></w:pPr>
      <w:r><w:t>标题文本</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>正文内容</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	paragraphs, err := parseDocumentXML([]byte(xmlData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paragraphs) < 2 {
		t.Fatalf("expected at least 2 paragraphs, got %d", len(paragraphs))
	}
	if paragraphs[0].Text != "标题文本" {
		t.Errorf("expected '标题文本', got '%s'", paragraphs[0].Text)
	}
	if !paragraphs[0].IsHeading {
		t.Error("first paragraph should be a heading")
	}
	if paragraphs[1].Text != "正文内容" {
		t.Errorf("expected '正文内容', got '%s'", paragraphs[1].Text)
	}
}

// buildMinimalDOCXMissingDocument 构建缺少 document.xml 的 DOCX
func buildMinimalDOCXMissingDocument() []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
</Types>`
	addZipFile(w, "[Content_Types].xml", contentTypes)

	w.Close()
	return buf.Bytes()
}
