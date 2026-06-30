package main

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed scaffold/basic scaffold/with-tools scaffold/multi-agent
//go:embed scaffold/agent-with-cache scaffold/agent-with-rag scaffold/agent-with-metrics
//go:embed scaffold/quickstart
var scaffoldFS embed.FS

// validTemplates 支持的模板列表（供 init.go 和 scaffold.go 共享）
var validTemplates = map[string]bool{
	"quickstart":         true,
	"basic":              true,
	"with-tools":         true,
	"multi-agent":        true,
	"agent-with-cache":   true,
	"agent-with-rag":     true,
	"agent-with-metrics": true,
}

// GenerateOptions 定义脚手架生成选项
type GenerateOptions struct {
	Name     string // 项目名称
	Template string // 模板名称
	DryRun   bool   // 预览模式（不写盘）
}

// Generate 根据模板生成项目文件，返回文件名到内容的映射
func Generate(opts GenerateOptions) (map[string][]byte, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("project name is required")
	}

	template := opts.Template
	if template == "" {
		template = "basic"
	}

	// 验证模板（使用包级 validTemplates）
	if !validTemplates[template] {
		return nil, fmt.Errorf("unknown template %q", template)
	}

	files := make(map[string][]byte)
	scaffoldDir := "scaffold/" + template

	// 遍历模板文件
	err := fs.WalkDir(scaffoldFS, scaffoldDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		relPath := strings.TrimPrefix(path, scaffoldDir+"/")
		if relPath == scaffoldDir {
			return nil
		}

		data, err := scaffoldFS.ReadFile(path)
		if err != nil {
			return err
		}

		// 替换模板变量
		content := strings.ReplaceAll(string(data), "{{.ProjectName}}", opts.Name)
		content = strings.ReplaceAll(content, "{{.ModuleName}}", opts.Name)

		files[relPath] = []byte(content)
		return nil
	})
	if err != nil {
		return nil, err
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
`, opts.Name, template)
	files[".ap.yaml"] = []byte(apConfigYAML)

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
	files[".gitignore"] = []byte(gitignore)

	// 生成 go.mod
	goMod := fmt.Sprintf(`module %s

go 1.23

require agentprimordia v1.0.0

replace agentprimordia => ..
`, opts.Name)
	files["go.mod"] = []byte(goMod)

	return files, nil
}
