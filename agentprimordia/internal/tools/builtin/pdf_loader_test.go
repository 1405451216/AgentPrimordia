package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- PDFLoader 测试 ---

func TestPDFLoader_Name(t *testing.T) {
	pl := NewPDFLoader()
	if pl.Name() != "pdf_loader" {
		t.Errorf("expected 'pdf_loader', got '%s'", pl.Name())
	}
}

func TestPDFLoader_Description(t *testing.T) {
	pl := NewPDFLoader()
	desc := pl.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if !strings.Contains(desc, "PDF") {
		t.Error("description should mention PDF")
	}
}

func TestPDFLoader_Parameters(t *testing.T) {
	pl := NewPDFLoader()
	params := pl.Parameters()
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

func TestPDFLoader_LoadBasicPDF(t *testing.T) {
	tmpDir := t.TempDir()
	pdfData := buildMinimalPDF([]string{"Hello World", "Second Page"})
	testFile := filepath.Join(tmpDir, "test.pdf")
	os.WriteFile(testFile, pdfData, 0644)

	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc PDFDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.PageCount != 2 {
		t.Errorf("expected 2 pages, got %d", doc.PageCount)
	}
	if doc.Metadata["pdf_version"] != "1.4" {
		t.Errorf("expected pdf version '1.4', got '%s'", doc.Metadata["pdf_version"])
	}
}

func TestPDFLoader_Metadata(t *testing.T) {
	tmpDir := t.TempDir()
	pdfData := buildMinimalPDF([]string{"Content"})
	testFile := filepath.Join(tmpDir, "meta.pdf")
	os.WriteFile(testFile, pdfData, 0644)

	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc PDFDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.Metadata["title"] != "Test PDF" {
		t.Errorf("expected title 'Test PDF', got '%s'", doc.Metadata["title"])
	}
	if doc.Metadata["author"] != "Test Author" {
		t.Errorf("expected author 'Test Author', got '%s'", doc.Metadata["author"])
	}
}

func TestPDFLoader_TextExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	pdfData := buildMinimalPDF([]string{"Hello PDF World"})
	testFile := filepath.Join(tmpDir, "text.pdf")
	os.WriteFile(testFile, pdfData, 0644)

	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc PDFDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.PageCount < 1 {
		t.Fatalf("expected at least 1 page, got %d", doc.PageCount)
	}
	if !strings.Contains(doc.Pages[0].Content, "Hello PDF World") {
		t.Errorf("page 1 content should contain 'Hello PDF World', got '%s'", doc.Pages[0].Content)
	}
}

func TestPDFLoader_MultiPage(t *testing.T) {
	tmpDir := t.TempDir()
	pdfData := buildMinimalPDF([]string{"Page One", "Page Two", "Page Three"})
	testFile := filepath.Join(tmpDir, "multi.pdf")
	os.WriteFile(testFile, pdfData, 0644)

	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("should not be error: %s", result.Content)
	}

	var doc PDFDocument
	if err := json.Unmarshal([]byte(result.Content), &doc); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if doc.PageCount != 3 {
		t.Errorf("expected 3 pages, got %d", doc.PageCount)
	}
}

func TestPDFLoader_FileNotFound(t *testing.T) {
	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   "/nonexistent/file.pdf",
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent file")
	}
}

func TestPDFLoader_InvalidAction(t *testing.T) {
	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "invalid",
		"path":   "test.pdf",
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestPDFLoader_MissingPath(t *testing.T) {
	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing path")
	}
}

func TestPDFLoader_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.pdf")
	os.WriteFile(testFile, []byte(""), 0644)

	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for empty file")
	}
}

func TestPDFLoader_InvalidPDF(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.pdf")
	os.WriteFile(testFile, []byte("This is not a PDF file"), 0644)

	pl := NewPDFLoader()
	args, _ := json.Marshal(map[string]any{
		"action": "load",
		"path":   testFile,
	})
	result, err := pl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid PDF")
	}
}

// --- PDF 内部函数测试 ---

func TestDecodePDFString_PlainText(t *testing.T) {
	result := decodePDFString([]byte("Hello World"))
	if result != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", result)
	}
}

func TestDecodePDFString_EscapeSequences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`Hello\nWorld`, "Hello\nWorld"},
		{`Hello\rWorld`, "Hello\rWorld"},
		{`Hello\tWorld`, "Hello\tWorld"},
		{`Hello\bWorld`, "Hello\bWorld"},
		{`Hello\fWorld`, "Hello\fWorld"},
		{`Hello\(World`, "Hello(World"},
		{`Hello\)World`, "Hello)World"},
		{`Hello\\World`, "Hello\\World"},
	}
	for _, tt := range tests {
		got := decodePDFString([]byte(tt.input))
		if got != tt.expected {
			t.Errorf("decodePDFString(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDecodePDFString_OctalEscape(t *testing.T) {
	// 八进制转义 \101 = 'A'
	result := decodePDFString([]byte(`\101`))
	if result != "A" {
		t.Errorf("expected 'A', got '%s'", result)
	}
}

func TestDecodePDFString_UTF16BE(t *testing.T) {
	// UTF-16BE BOM + "Hi" (H=0x0048, i=0x0069)
	data := []byte{0xFE, 0xFF, 0x00, 0x48, 0x00, 0x69}
	result := decodePDFString(data)
	if result != "Hi" {
		t.Errorf("expected 'Hi', got '%s'", result)
	}
}

func TestDecodePDFString_Empty(t *testing.T) {
	result := decodePDFString([]byte{})
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func TestEscapePDFString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello (world)", `hello \(world\)`},
		{"back\\slash", `back\\slash`},
	}
	for _, tt := range tests {
		got := escapePDFString(tt.input)
		if got != tt.expected {
			t.Errorf("escapePDFString(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildMinimalPDF(t *testing.T) {
	// 验证生成的 PDF 包含正确的头和基本结构
	pdfData := buildMinimalPDF([]string{"Test"})
	if len(pdfData) == 0 {
		t.Fatal("buildMinimalPDF should produce non-empty data")
	}
	if !strings.HasPrefix(string(pdfData), "%PDF-") {
		t.Errorf("PDF should start with %%PDF- header")
	}
	if !strings.Contains(string(pdfData), "%%EOF") {
		t.Errorf("PDF should end with %%%%EOF marker")
	}
}
