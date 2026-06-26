package main

import (
	"sort"
	"strings"
	"testing"
)

// TestGenerate_Basic 验证 basic 模板渲染出预期文件清单。
func TestGenerate_Basic(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "demo",
		Template: "basic",
	})
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	want := []string{".ap.yaml", ".gitignore", "go.mod", "main.go"}
	got := make([]string, 0, len(files))
	for k := range files {
		got = append(got, k)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("文件清单不匹配: got %v, want %v", got, want)
	}
}

// TestGenerate_ProjectName 验证 {{.ProjectName}} 已被替换。
func TestGenerate_ProjectName(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "hello-world",
		Template: "basic",
	})
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	main, ok := files["main.go"]
	if !ok {
		t.Fatal("缺 main.go")
	}
	if !strings.Contains(string(main), `"hello-world"`) {
		t.Error("main.go 未包含替换后的 ProjectName")
	}
	if strings.Contains(string(main), "{{.ProjectName}}") {
		t.Error("main.go 仍含未替换的 {{.ProjectName}}")
	}
}

// TestGenerate_UnknownTemplate 验证未知模板返回错误。
func TestGenerate_UnknownTemplate(t *testing.T) {
	_, err := Generate(GenerateOptions{
		Name:     "demo",
		Template: "does-not-exist",
	})
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
}

// TestGenerate_EmptyName 验证空 name 返回错误。
func TestGenerate_EmptyName(t *testing.T) {
	_, err := Generate(GenerateOptions{
		Name:     "",
		Template: "basic",
	})
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
}

// TestGenerate_DryRunFlag 验证 DryRun 选项不会让 Generate 失败（行为与正式模式相同，
// 实际"不写盘"是 runInit 包装层的事，Generate 本身只生成文件树）。
func TestGenerate_DryRunFlag(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "demo",
		Template: "basic",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	if len(files) == 0 {
		t.Error("DryRun 也应返回文件清单")
	}
}

// TestGenerate_ApYamlContent 验证 .ap.yaml 关键字段存在。
func TestGenerate_ApYamlContent(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "demo",
		Template: "basic",
	})
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	ap, ok := files[".ap.yaml"]
	if !ok {
		t.Fatal("缺 .ap.yaml")
	}
	s := string(ap)
	if !strings.Contains(s, "name: demo") {
		t.Error(".ap.yaml 缺 name: demo")
	}
	if !strings.Contains(s, "template: basic") {
		t.Error(".ap.yaml 缺 template: basic")
	}
}
