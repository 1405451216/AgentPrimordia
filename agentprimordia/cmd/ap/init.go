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
//go:embed scaffold/agent-with-cache scaffold/agent-with-rag scaffold/agent-with-metrics
//go:embed scaffold/quickstart
var scaffoldFS embed.FS

func runInit(args []string) {
	var (
		name     string
		template string
		dryRun   bool
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--template", "-t":
			i++
			if i >= len(args) {
				errorf("--template 需要指定模板名称")
				os.Exit(1)
			}
			template = args[i]
		case "--dry-run":
			dryRun = true
		case "--help", "-h":
			fmt.Print(`ap init — create a new agent project

Usage:
  ap init <项目名> [--template NAME] [--dry-run]

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

Examples:
  ap init my-agent
  ap init my-agent --template with-tools
  ap init my-agent --template agent-with-rag --dry-run
`)
			return
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	if name == "" {
		errorf("please specify project name\nUsage: ap init <name>")
		os.Exit(1)
	}
	if template == "" {
		template = "basic"
	}

	// 验证模板
	validTemplates := map[string]bool{
		"quickstart":         true,
		"basic":              true,
		"with-tools":         true,
		"multi-agent":        true,
		"agent-with-cache":   true,
		"agent-with-rag":     true,
		"agent-with-metrics": true,
	}
	if !validTemplates[template] {
		errorf("unknown template %q, supported: quickstart, basic, with-tools, multi-agent, agent-with-cache, agent-with-rag, agent-with-metrics", template)
		os.Exit(1)
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
		return
	}

	// 检查目标目录
	targetDir := name
	if _, err := os.Stat(targetDir); err == nil {
		errorf("directory %q already exists", targetDir)
		os.Exit(1)
	}

	// 创建目录
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		errorf("create directory failed: %v", err)
		os.Exit(1)
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
		errorf("create project failed: %v", err)
		os.Exit(1)
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
	os.WriteFile(filepath.Join(targetDir, ".ap.yaml"), []byte(apConfigYAML), 0o644)

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
	os.WriteFile(filepath.Join(targetDir, ".gitignore"), []byte(gitignore), 0o644)

	// 生成 go.mod
	goMod := fmt.Sprintf(`module %s

go 1.22

require agentprimordia v0.0.0

replace agentprimordia => ..
`, name)
	os.WriteFile(filepath.Join(targetDir, "go.mod"), []byte(goMod), 0o644)

	successf("项目 %q 已创建 (Templates: %s)", name, template)
	fmt.Println()
	fmt.Printf("Next steps:\n")
	infof("cd %s", name)
	infof("go mod tidy")
	infof("set AP_LLM_API_KEY=sk-xxx")
	infof("ap run")
}
