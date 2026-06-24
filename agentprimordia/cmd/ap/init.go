package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func runInit(args []string) error {
	var (
		name        string
		template    string
		dryRun      bool
		interactive bool
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--template", "-t":
			i++
			if i >= len(args) {
				return fmt.Errorf("--template 需要指定模板名称")
			}
			template = args[i]
		case "--dry-run":
			dryRun = true
		case "--interactive", "-i":
			interactive = true
		case "--help", "-h":
			fmt.Print(`ap init — create a new agent project

Usage:
  ap init <项目名> [--template NAME] [--dry-run] [--interactive]

Templates:
  quickstart         5分钟快速入门 (推荐新手)
  basic              minimal agent (default)
  with-tools         agent with tools (filesystem + shell + web)
  multi-agent        multi-agent collaboration
  agent-with-cache   agent + LLM response cache
  agent-with-rag     agent + knowledge retrieval (RAG)
  agent-with-metrics agent + Prometheus metrics

Options:
  --dry-run          preview files without creating
  --interactive, -i  interactive wizard mode

Examples:
  ap init my-agent
  ap init my-agent --template with-tools
  ap init my-agent --template agent-with-rag --dry-run
  ap init --interactive
`)
			return nil
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	// 交互式向导模式
	if interactive {
		wizard := NewWizard(os.Stdin, os.Stdout)
		opts, err := wizard.Run()
		if err != nil {
			return err
		}
		name = opts.Name
		template = opts.Template
	}

	if name == "" {
		return fmt.Errorf("please specify project name\nUsage: ap init <name>")
	}
	if template == "" {
		template = "basic"
	}

	// 验证模板（validTemplates 定义在 scaffold.go）
	if !validTemplates[template] {
		return fmt.Errorf("unknown template %q, supported: quickstart, basic, with-tools, multi-agent, agent-with-cache, agent-with-rag, agent-with-metrics", template)
	}

	// dry-run 模式：只预览
	if dryRun {
		fmt.Printf("%s preview mode — files to be created:\n\n", bold("DRY RUN"))
		scaffoldDir := "scaffold/" + template
		fs.WalkDir(scaffoldFS, scaffoldDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			relPath := strings.TrimPrefix(path, scaffoldDir+"/")
			if relPath == scaffoldDir {
				return nil
			}
			infof("%s/%s", name, relPath)
			return nil
		})
		infof("%s/.ap.yaml", name)
		infof("%s/.gitignore", name)
		infof("%s/go.mod", name)
		fmt.Printf("\nTemplates: %s\n", template)
		return nil
	}

	// 检查目标目录
	targetDir := name
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("directory %q already exists", targetDir)
	}

	// 创建目录
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	// 复制模板文件
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

		content := strings.ReplaceAll(string(data), "{{.ProjectName}}", name)
		content = strings.ReplaceAll(content, "{{.ModuleName}}", name)

		return os.WriteFile(targetPath, []byte(content), 0o644)
	})
	if err != nil {
		return fmt.Errorf("create project failed: %w", err)
	}

	// 生成 .ap.yaml
	apConfigYAML := fmt.Sprintf(`# AgentPrimordia 项目配置
name: %s
template: %s

llm:
  provider: openai       # openai | anthropic | gemini | ollama | azure | deepseek | qwen
  model: gpt-4o
  # api_key: "sk-xxx"    # 建议用环境变量 AP_LLM_API_KEY

memory:
  backend: sqlite        # sqlite | memory
  path: ./data/memory.db

agent:
  max_turns: 20
  system_prompt: "you are a helpful assistant"
`, name, template)
	if err := os.WriteFile(filepath.Join(targetDir, ".ap.yaml"), []byte(apConfigYAML), 0o644); err != nil {
		return fmt.Errorf("write .ap.yaml failed: %w", err)
	}

	// 生成 .gitignore
	gitignore := `# AgentPrimordia
*.exe
*.exe~
*.dll
*.so
*.dylib
*agent
data/
*.db
.env
`
	if err := os.WriteFile(filepath.Join(targetDir, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		return fmt.Errorf("write .gitignore failed: %w", err)
	}

	// 生成 go.mod
	goMod := fmt.Sprintf(`module %s

go 1.23

require agentprimordia v0.0.0

replace agentprimordia => ..
`, name)
	if err := os.WriteFile(filepath.Join(targetDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return fmt.Errorf("write go.mod failed: %w", err)
	}

	successf("项目 %q 已创建 (Templates: %s)", name, template)
	fmt.Println()
	fmt.Printf("Next steps:\n")
	infof("cd %s", name)
	infof("go mod tidy")
	infof("set AP_LLM_API_KEY=sk-xxx")
	infof("ap run")
	return nil
}
