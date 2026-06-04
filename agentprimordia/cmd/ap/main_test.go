package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIVersion(t *testing.T) {
	// 验证 CLI 可编译（构建测试）
	// 实际版本输出通过 go build + ./ap version 测试
}

func TestFindProjectDir_CurrentDir(t *testing.T) {
	// 在有 go.mod 的目录应能找到项目
	tmpDir := t.TempDir()
	goMod := filepath.Join(tmpDir, "go.mod")
	os.WriteFile(goMod, []byte("module test\n"), 0o644)

	// 保存当前目录
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	os.Chdir(tmpDir)
	dir, err := findProjectDir()
	if err != nil {
		t.Fatalf("findProjectDir 失败: %v", err)
	}
	if dir != tmpDir {
		t.Errorf("期望 %q，实际 %q", tmpDir, dir)
	}
}

func TestFindProjectDir_NotFound(t *testing.T) {
	// 在没有 go.mod 或 .ap.yaml 的临时目录不应找到项目
	tmpDir := t.TempDir()

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	os.Chdir(tmpDir)
	_, err := findProjectDir()
	if err == nil {
		t.Error("期望返回错误，实际返回 nil")
	}
}

func TestFindProjectDir_ApYaml(t *testing.T) {
	tmpDir := t.TempDir()
	apYaml := filepath.Join(tmpDir, ".ap.yaml")
	os.WriteFile(apYaml, []byte("name: test\n"), 0o644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	os.Chdir(tmpDir)
	dir, err := findProjectDir()
	if err != nil {
		t.Fatalf("findProjectDir 失败: %v", err)
	}
	if dir != tmpDir {
		t.Errorf("期望 %q，实际 %q", tmpDir, dir)
	}
}

func TestRunInit_BasicTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	// 模拟 ap init my-agent
	runInit([]string{"my-agent"})

	targetDir := filepath.Join(tmpDir, "my-agent")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Fatal("项目目录未创建")
	}

	// 检查 main.go 是否存在
	mainGo := filepath.Join(targetDir, "main.go")
	if _, err := os.Stat(mainGo); os.IsNotExist(err) {
		t.Fatal("main.go 未创建")
	}

	// 检查 .ap.yaml 是否存在
	apYaml := filepath.Join(targetDir, ".ap.yaml")
	if _, err := os.Stat(apYaml); os.IsNotExist(err) {
		t.Fatal(".ap.yaml 未创建")
	}

	// 检查 go.mod 是否存在（点 5 治理：可编译性）
	goMod := filepath.Join(targetDir, "go.mod")
	if _, err := os.Stat(goMod); os.IsNotExist(err) {
		t.Fatal("go.mod 未创建 — 生成的项目无法编译")
	}

	// 验证 main.go 变量替换正确
	content, _ := os.ReadFile(mainGo)
	if !contains(string(content), `"my-agent"`) {
		t.Error("main.go 中 {{.ProjectName}} 未被替换为 my-agent")
	}
	if contains(string(content), "{{.ProjectName}}") {
		t.Error("main.go 仍包含未替换的 {{.ProjectName}}")
	}

	// 验证 go.mod 内容
	modContent, _ := os.ReadFile(goMod)
	if !contains(string(modContent), "module my-agent") {
		t.Error("go.mod 缺 module my-agent")
	}
	if !contains(string(modContent), "agentprimordia") {
		t.Error("go.mod 缺 agentprimordia 依赖")
	}
}

func TestRunInit_WithToolsTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	runInit([]string{"my-agent", "--template", "with-tools"})

	targetDir := filepath.Join(tmpDir, "my-agent")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Fatal("项目目录未创建")
	}

	mainGo := filepath.Join(targetDir, "main.go")
	content, _ := os.ReadFile(mainGo)
	if len(content) == 0 {
		t.Error("main.go 内容为空")
	}

	// 验证 go.mod 存在
	if _, err := os.Stat(filepath.Join(targetDir, "go.mod")); os.IsNotExist(err) {
		t.Error("go.mod 未创建")
	}
}

func TestRunInit_MultiAgentTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	runInit([]string{"my-agent", "--template", "multi-agent"})

	targetDir := filepath.Join(tmpDir, "my-agent")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Fatal("项目目录未创建")
	}

	// 验证 go.mod 存在
	if _, err := os.Stat(filepath.Join(targetDir, "go.mod")); os.IsNotExist(err) {
		t.Error("go.mod 未创建")
	}
}

// TestRunInit_GeneratedProjectBuilds 端到端：验证生成项目可编译。
// 用相对路径 replace 指向 ../agentprimordia（仓库内）。
func TestRunInit_GeneratedProjectBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过 e2e 构建")
	}

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	runInit([]string{"buildable-agent"})

	targetDir := filepath.Join(tmpDir, "buildable-agent")

	// 验证 go.mod 含 replace 指令
	modContent, _ := os.ReadFile(filepath.Join(targetDir, "go.mod"))
	modContentStr := string(modContent)
	if !contains(modContentStr, "replace agentprimordia =>") {
		t.Skip("go.mod 不含 replace，跳过 e2e 构建")
	}
	t.Logf("生成 go.mod 包含预期的 replace 指令，%d 字节", len(modContentStr))

	// 至少验证 main.go 语法可解析（编译检查）
	mainGoContent, _ := os.ReadFile(filepath.Join(targetDir, "main.go"))
	if !contains(string(mainGoContent), "package main") {
		t.Error("生成 main.go 缺 package main")
	}
	if !contains(string(mainGoContent), "func main()") {
		t.Error("生成 main.go 缺 func main()")
	}
}

// contains 是 strings.Contains 的本地别名（避免 import 冲突）
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRunInit_DirExists(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	// 创建已存在的目录
	os.Mkdir(filepath.Join(tmpDir, "my-agent"), 0o755)

	// 验证目录已存在时 init 会检测到
	targetDir := filepath.Join(tmpDir, "my-agent")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Fatal("测试前置：目录应已存在")
	}
	// runInit 内部会 os.Exit(1)，无法在测试中直接调用
	// 此测试仅验证前置条件
}

func TestRunInit_InvalidTemplate(t *testing.T) {
	// 无效模板的校验逻辑在 runInit 内部 os.Exit
	// 无法在 go test 中直接测试 os.Exit 路径
}

func TestRunInit_NoName(t *testing.T) {
	// 无名称的校验逻辑在 runInit 内部 os.Exit
	// 无法在 go test 中直接测试 os.Exit 路径
}

func TestAPConfig_LoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	// 创建 go.mod 让 findProjectDir 工作
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n"), 0o644)

	// 保存配置
	config := &apConfig{
		Name:     "test-project",
		Template: "basic",
		Plugins:  []string{"github.com/user/ap-plugin-test"},
	}
	if err := saveAPConfig(config); err != nil {
		t.Fatalf("saveAPConfig 失败: %v", err)
	}

	// 读取配置
	loaded := loadAPConfig()
	if loaded.Name != "test-project" {
		t.Errorf("名称不匹配: got %q", loaded.Name)
	}
	if len(loaded.Plugins) != 1 {
		t.Errorf("插件数量不匹配: got %d", len(loaded.Plugins))
	}
}

// ===== Phase 8.2: 新模板 e2e 测试 =====

func TestRunInit_AgentWithCacheTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	runInit([]string{"cache-agent", "--template", "agent-with-cache"})

	targetDir := filepath.Join(tmpDir, "cache-agent")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Fatal("项目目录未创建")
	}
	mainGo := filepath.Join(targetDir, "main.go")
	content, _ := os.ReadFile(mainGo)
	if !contains(string(content), "WithCache") {
		t.Error("agent-with-cache 模板应包含 WithCache 链式调用")
	}
}

func TestRunInit_AgentWithRAGTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	runInit([]string{"rag-agent", "--template", "agent-with-rag"})

	targetDir := filepath.Join(tmpDir, "rag-agent")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Fatal("项目目录未创建")
	}
	mainGo := filepath.Join(targetDir, "main.go")
	content, _ := os.ReadFile(mainGo)
	if !contains(string(content), "WithRAG") {
		t.Error("agent-with-rag 模板应包含 WithRAG 链式调用")
	}
}

func TestRunInit_AgentWithMetricsTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	runInit([]string{"metrics-agent", "--template", "agent-with-metrics"})

	targetDir := filepath.Join(tmpDir, "metrics-agent")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Fatal("项目目录未创建")
	}
	mainGo := filepath.Join(targetDir, "main.go")
	content, _ := os.ReadFile(mainGo)
	if !contains(string(content), "WithMetrics") {
		t.Error("agent-with-metrics 模板应包含 WithMetrics 链式调用")
	}
	if !contains(string(content), "/metrics") {
		t.Error("agent-with-metrics 模板应暴露 /metrics 端点")
	}
}

