package main

import (
	"sort"
	"strings"
	"testing"
)

// --- plugin 模板测试 ---

// TestGenerate_Plugin 验证 plugin 模板生成完整文件清单。
func TestGenerate_Plugin(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "my-plugin",
		Template: "plugin",
	})
	if err != nil {
		t.Fatalf("Generate plugin 失败: %v", err)
	}
	want := []string{
		".ap.yaml",
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
		".gitignore",
		"go.mod",
		"Makefile",
		"README.md",
		"plugin.go",
		"plugin.json",
		"plugin_test.go",
	}
	got := make([]string, 0, len(files))
	for k := range files {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("plugin 文件清单不匹配: got %v, want %v", got, want)
	}
}

// TestGenerate_Plugin_ProjectNameReplaced 验证 {{.ProjectName}} 已被替换。
func TestGenerate_Plugin_ProjectNameReplaced(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "my-plugin",
		Template: "plugin",
	})
	if err != nil {
		t.Fatalf("Generate plugin 失败: %v", err)
	}
	for name, content := range files {
		if strings.Contains(string(content), "{{.ProjectName}}") {
			t.Errorf("%s 仍含未替换的 {{.ProjectName}}", name)
		}
	}
	// plugin.json 中应包含 my-plugin
	pj, ok := files["plugin.json"]
	if !ok {
		t.Fatal("plugin.json 缺失")
	}
	if !strings.Contains(string(pj), "my-plugin") {
		t.Errorf("plugin.json 未包含 my-plugin")
	}
}

// TestGenerate_Plugin_PackageDecl 验证 plugin.go 应有有效的 package 声明。
func TestGenerate_Plugin_PackageDecl(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "my-plugin",
		Template: "plugin",
	})
	if err != nil {
		t.Fatalf("Generate plugin 失败: %v", err)
	}
	pg, ok := files["plugin.go"]
	if !ok {
		t.Fatal("plugin.go 缺失")
	}
	s := string(pg)
	if !strings.Contains(s, "package main") {
		t.Error("plugin.go 缺 package main")
	}
	if !strings.Contains(s, "func New()") {
		t.Error("plugin.go 缺 New() 构造函数")
	}
	if !strings.Contains(s, "func (p *Plugin) Name()") {
		t.Error("plugin.go 缺 Name() 方法")
	}
	if !strings.Contains(s, "func (p *Plugin) Tools()") {
		t.Error("plugin.go 缺 Tools() 方法")
	}
}

// --- provider 模板测试 ---

// TestGenerate_Provider 验证 provider 模板生成完整文件清单。
func TestGenerate_Provider(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "my-provider",
		Template: "provider",
	})
	if err != nil {
		t.Fatalf("Generate provider 失败: %v", err)
	}
	want := []string{
		".ap.yaml",
		".github/workflows/ci.yml",
		".gitignore",
		"go.mod",
		"Makefile",
		"README.md",
		"main.go",
		"main_test.go",
	}
	got := make([]string, 0, len(files))
	for k := range files {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("provider 文件清单不匹配: got %v, want %v", got, want)
	}
}

// TestGenerate_Provider_ProjectNameReplaced 验证 {{.ProjectName}} 已被替换。
func TestGenerate_Provider_ProjectNameReplaced(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "my-provider",
		Template: "provider",
	})
	if err != nil {
		t.Fatalf("Generate provider 失败: %v", err)
	}
	for name, content := range files {
		if strings.Contains(string(content), "{{.ProjectName}}") {
			t.Errorf("%s 仍含未替换的 {{.ProjectName}}", name)
		}
	}
	main, ok := files["main.go"]
	if !ok {
		t.Fatal("main.go 缺失")
	}
	if !strings.Contains(string(main), "my-provider") {
		t.Errorf("main.go 未包含 my-provider")
	}
}

// TestGenerate_Provider_Interface 验证 provider 实现 Chat 接口。
func TestGenerate_Provider_Interface(t *testing.T) {
	files, err := Generate(GenerateOptions{
		Name:     "my-provider",
		Template: "provider",
	})
	if err != nil {
		t.Fatalf("Generate provider 失败: %v", err)
	}
	mg, ok := files["main.go"]
	if !ok {
		t.Fatal("main.go 缺失")
	}
	s := string(mg)
	for _, want := range []string{
		"func NewProvider(",
		"func (p *Provider) Name()",
		"func (p *Provider) Chat(",
		"func (p *Provider) Close()",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("main.go 缺 %q", want)
		}
	}
}

// --- runInit --type 参数测试 ---

// TestRunInit_TypePlugin 验证 `ap init X --type plugin` 在 dry-run 下正确工作。
func TestRunInit_TypePlugin(t *testing.T) {
	err := runInit([]string{"test-plugin", "--type", "plugin", "--dry-run"})
	if err != nil {
		t.Fatalf("runInit --type=plugin 失败: %v", err)
	}
}

// TestRunInit_TypeProvider 验证 `ap init X --type provider` 在 dry-run 下正确工作。
func TestRunInit_TypeProvider(t *testing.T) {
	err := runInit([]string{"test-provider", "--type", "provider", "--dry-run"})
	if err != nil {
		t.Fatalf("runInit --type=provider 失败: %v", err)
	}
}

// TestRunInit_TypeUnknown 验证未知 --type 返回错误。
func TestRunInit_TypeUnknown(t *testing.T) {
	err := runInit([]string{"test", "--type", "unknown", "--dry-run"})
	if err == nil {
		t.Fatal("期望返回未知类型错误")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("错误信息应提及 unknown，实际: %v", err)
	}
}

// TestRunInit_TypePluginWithWrongTemplate 验证 --type=plugin + 错误 template 报错。
func TestRunInit_TypePluginWithWrongTemplate(t *testing.T) {
	err := runInit([]string{"test", "--type", "plugin", "--template", "basic", "--dry-run"})
	if err == nil {
		t.Fatal("期望返回 template 不匹配的错误")
	}
}

// TestWizard_TypeSelection 验证 Wizard.Run() 返回 GenerateOptions 且含 Type 字段。
func TestWizard_TypeSelection(t *testing.T) {
	input := strings.NewReader("my-agent\n1\n\n") // agent + template=quickstart
	w := &strings.Builder{}
	wizard := NewWizard(input, w)
	opts, err := wizard.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() 失败: %v", err)
	}
	if opts.Name != "my-agent" {
		t.Errorf("期望 Name=my-agent, got %q", opts.Name)
	}
	if opts.Type != "agent" {
		t.Errorf("期望 Type=agent, got %q", opts.Type)
	}
	if opts.Template != "quickstart" {
		t.Errorf("期望 Template=quickstart, got %q", opts.Template)
	}
}

// TestWizard_PluginType 验证 Wizard.Run() 选择 plugin 时 template 也被设为 plugin。
func TestWizard_PluginType(t *testing.T) {
	input := strings.NewReader("my-plugin\n2\n") // plugin type → auto template=plugin
	w := &strings.Builder{}
	wizard := NewWizard(input, w)
	opts, err := wizard.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() 失败: %v", err)
	}
	if opts.Type != "plugin" {
		t.Errorf("期望 Type=plugin, got %q", opts.Type)
	}
	if opts.Template != "plugin" {
		t.Errorf("期望 Template=plugin, got %q", opts.Template)
	}
}

// TestWizard_ProviderType 验证 Wizard.Run() 选择 provider 时 template 也被设为 provider。
func TestWizard_ProviderType(t *testing.T) {
	input := strings.NewReader("my-provider\n3\n") // provider type → auto template=provider
	w := &strings.Builder{}
	wizard := NewWizard(input, w)
	opts, err := wizard.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() 失败: %v", err)
	}
	if opts.Type != "provider" {
		t.Errorf("期望 Type=provider, got %q", opts.Type)
	}
	if opts.Template != "provider" {
		t.Errorf("期望 Template=provider, got %q", opts.Template)
	}
}
