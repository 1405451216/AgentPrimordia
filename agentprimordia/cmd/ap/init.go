package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed scaffold/basic scaffold/with-tools scaffold/multi-agent
var scaffoldFS embed.FS

func runInit(args []string) {
	var (
		name     string
		template string
	)
	// 解析参数
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--template", "-t":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: --template 需要指定模板名称")
				os.Exit(1)
			}
			template = args[i]
		case "--help", "-h":
			fmt.Print(`ap init — 创建新的 Agent 项目

用法:
  ap init <项目名> [--template basic|with-tools|multi-agent]

模板:
  basic        最小 Agent（默认）
  with-tools   含工具 Agent（文件系统 + Shell + Web）
  multi-agent  多 Agent 协作

示例:
  ap init my-agent
  ap init my-agent --template with-tools
`)
			return
		default:
			if name == "" {
				name = args[i]
			}
		}
		i++
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "错误: 请指定项目名称\n用法: ap init <项目名>")
		os.Exit(1)
	}
	if template == "" {
		template = "basic"
	}

	// 验证模板
	validTemplates := map[string]bool{"basic": true, "with-tools": true, "multi-agent": true}
	if !validTemplates[template] {
		fmt.Fprintf(os.Stderr, "错误: 未知模板 %q，可选: basic, with-tools, multi-agent\n", template)
		os.Exit(1)
	}

	// 检查目标目录
	targetDir := name
	if _, err := os.Stat(targetDir); err == nil {
		fmt.Fprintf(os.Stderr, "错误: 目录 %q 已存在\n", targetDir)
		os.Exit(1)
	}

	// 从嵌入的 scaffold 复制模板
	// 先创建目标目录
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建目录失败: %v\n", err)
		os.Exit(1)
	}

	scaffoldDir := "scaffold/" + template
	err := fs.WalkDir(scaffoldFS, scaffoldDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath := strings.TrimPrefix(path, scaffoldDir+"/")
		if relPath == scaffoldDir {
			return nil
		}
		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		data, err := scaffoldFS.ReadFile(path)
		if err != nil {
			return err
		}

		// 替换模板变量
		content := strings.ReplaceAll(string(data), "{{.ProjectName}}", name)
		content = strings.ReplaceAll(content, "{{.ModuleName}}", name)

		return os.WriteFile(targetPath, []byte(content), 0o644)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建项目失败: %v\n", err)
		os.Exit(1)
	}

	// 生成 .ap.yaml 配置文件
	apConfig := fmt.Sprintf(`# AgentPrimordia 项目配置
name: %s
template: %s

llm:
  provider: openai       # openai | anthropic | gemini | ollama | azure
  model: gpt-4o
  # api_key: ${OPENAI_API_KEY}  # 建议用环境变量

memory:
  backend: sqlite        # sqlite | memory
  path: ./data/memory.db

agent:
  max_turns: 20
  system_prompt: "你是一个智能助手"
`, name, template)

	if err := os.WriteFile(filepath.Join(targetDir, ".ap.yaml"), []byte(apConfig), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 写入 .ap.yaml 失败: %v\n", err)
	}

	// 生成 go.mod
	// 假定 ap init 在仓库根的 agentprimordia/ 子目录内运行（这是
	// 脚手架的标准使用方式：用户 cd 到仓库根 → ap init my-agent，
	// 生成 agentprimordia/my-agent/，replace 路径只需 ..）。
	// 若是独立 go module，用户可手动 `go mod edit -module=<name>` 调整。
	goMod := fmt.Sprintf(`module %s

go 1.22

require agentprimordia v0.0.0

replace agentprimordia => ..
`, name)

	if err := os.WriteFile(filepath.Join(targetDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 写入 go.mod 失败: %v\n", err)
	}

	fmt.Printf("✓ 项目 %q 已创建 (模板: %s)\n\n", name, template)
	fmt.Printf("下一步:\n")
	fmt.Printf("  cd %s\n", name)
	fmt.Printf("  go mod tidy\n")
	fmt.Printf("  # 设置 API Key: set OPENAI_API_KEY=sk-xxx\n")
	fmt.Printf("  ap run\n")
}
