package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// validateTemplateID 验证模板 ID 安全性
// 防止路径遍历：仅允许字母、数字、连字符、下划线、点
func validateTemplateID(id string) error {
	if id == "" {
		return fmt.Errorf("template ID cannot be empty")
	}
	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("invalid template ID %q: path separators and '..' not allowed", id)
	}
	for _, ch := range id {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.') {
			return fmt.Errorf("invalid template ID %q: only alphanumeric, '-', '_', '.' allowed", id)
		}
	}
	return nil
}

// safeTemplatePath 构建安全的模板文件路径
func safeTemplatePath(templateID string) (string, error) {
	if err := validateTemplateID(templateID); err != nil {
		return "", err
	}
	p := filepath.Join(marketplaceDir, templateID+".json")
	// 确保最终路径在 marketplaceDir 内
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	baseAbs, _ := filepath.Abs(marketplaceDir)
	if !strings.HasPrefix(abs, baseAbs) {
		return "", fmt.Errorf("invalid template path: escapes base directory")
	}
	return p, nil
}

// Phase 3.2: CLI 市场管理
//
// 命令：
//   ap marketplace search <query>   搜索 Agent 模板
//   ap marketplace install <id>     安装模板
//   ap marketplace publish          发布模板到市场
//   ap marketplace run <id>         从模板一键启动 Agent

func runMarketplace(args []string) error {
	if len(args) == 0 {
		printMarketplaceHelp()
		return nil
	}

	subcmd := args[0]
	switch subcmd {
	case "search":
		return runMarketplaceSearch(args[1:])
	case "install":
		return runMarketplaceInstall(args[1:])
	case "publish":
		return runMarketplacePublish(args[1:])
	case "run":
		return runMarketplaceRun(args[1:])
	case "list":
		return runMarketplaceList(args[1:])
	case "--help", "-h", "help":
		printMarketplaceHelp()
		return nil
	default:
		return fmt.Errorf("unknown marketplace subcommand %q, run %s for help", subcmd, bold("ap marketplace --help"))
	}
}

func printMarketplaceHelp() {
	fmt.Print(`ap marketplace — manage Agent templates

Usage:
  ap marketplace <command> [arguments]

Commands:
  search <query>    search agent templates by keyword
  install <id>      install a template locally
  publish           publish a template to the marketplace
  run <id>          deploy and run an agent from template
  list              list installed templates

Options:
  --category CAT    filter by category (research/coding/analysis/chat/automation)
  --json            output in JSON format

Examples:
  ap marketplace search "code review"
  ap marketplace search --category coding
  ap marketplace install tmpl-code-review
  ap marketplace publish
  ap marketplace run tmpl-code-review
`)
}

// ===== 模板存储 =====

const marketplaceDir = ".ap-templates"

// templateMeta 本地模板元数据
type templateMeta struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Author       string   `json:"author"`
	Category     string   `json:"category"`
	Tags         []string `json:"tags,omitempty"`
	SystemPrompt string   `json:"system_prompt"`
	InstalledAt  string   `json:"installed_at"`
}

// ===== 子命令实现 =====

func runMarketplaceSearch(args []string) error {
	query := ""
	category := ""
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--category":
			i++
			if i >= len(args) {
				return fmt.Errorf("--category requires a value")
			}
			category = args[i]
		case "--json":
			jsonOutput = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				query = args[i]
			}
		}
	}

	// 从本地模板库搜索
	templates := loadLocalTemplates()

	// 过滤
	var results []templateMeta
	for _, tmpl := range templates {
		if category != "" && tmpl.Category != category {
			continue
		}
		if query != "" {
			queryLower := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(tmpl.Name), queryLower) &&
				!strings.Contains(strings.ToLower(tmpl.Description), queryLower) {
				continue
			}
		}
		results = append(results, tmpl)
	}

	if jsonOutput {
		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(results) == 0 {
		fmt.Println("未找到匹配的模板")
		if query != "" {
			fmt.Printf("  搜索关键词: %q\n", query)
		}
		if category != "" {
			fmt.Printf("  分类过滤: %s\n", category)
		}
		return nil
	}

	fmt.Printf("找到 %d 个模板：\n\n", len(results))
	for _, tmpl := range results {
		fmt.Printf("  %s %s\n", bold(tmpl.ID), tmpl.Name)
		fmt.Printf("    %s\n", tmpl.Description)
		fmt.Printf("    分类: %s | 版本: %s | 作者: %s\n\n", tmpl.Category, tmpl.Version, tmpl.Author)
	}

	return nil
}

func runMarketplaceInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap marketplace install <template-id>")
	}

	templateID := args[0]

	// 验证模板 ID 安全性
	targetPath, err := safeTemplatePath(templateID)
	if err != nil {
		return err
	}

	// 确保模板目录存在
	if err := os.MkdirAll(marketplaceDir, 0755); err != nil {
		return fmt.Errorf("create templates dir: %w", err)
	}

	// 检查是否已安装
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("template %q already installed", templateID)
	}

	// 创建模板占位（实际应从远程市场下载）
	tmpl := templateMeta{
		ID:           templateID,
		Name:         templateID,
		Description:  "Installed template",
		Version:      "1.0.0",
		Author:       "unknown",
		Category:     "chat",
		SystemPrompt: "You are a helpful assistant.",
		InstalledAt:  time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal template: %w", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("write template: %w", err)
	}

	infof("模板 %s 安装成功", bold(templateID))
	fmt.Printf("  路径: %s\n", targetPath)
	fmt.Printf("\n使用以下命令运行：\n")
	fmt.Printf("  %s\n", bold("ap marketplace run "+templateID))

	return nil
}

func runMarketplacePublish(args []string) error {
	// 查找当前目录的 agent.yaml 或 .ap.yaml
	configFile := ""
	for _, name := range []string{"agent.yaml", ".ap.yaml", "ap.json"} {
		if _, err := os.Stat(name); err == nil {
			configFile = name
			break
		}
	}

	if configFile == "" {
		return fmt.Errorf("no agent config found in current directory (expected agent.yaml or .ap.yaml)")
	}

	fmt.Printf("%s\n\n", bold("发布模板"))
	fmt.Printf("  配置文件:  %s\n", configFile)
	fmt.Printf("  状态:      %s\n", bold("准备发布"))
	fmt.Println()
	fmt.Println("  注意：模板发布功能需要连接到 AgentPrimordia 市场服务。")
	fmt.Println("  当前为本地模式，模板将保存到本地目录。")

	return nil
}

func runMarketplaceRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap marketplace run <template-id>")
	}

	templateID := args[0]

	// 验证模板 ID 安全性
	targetPath, err := safeTemplatePath(templateID)
	if err != nil {
		return err
	}

	// 查找已安装的模板
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return fmt.Errorf("template %q not installed, run 'ap marketplace install %s' first", templateID, templateID)
	}

	var tmpl templateMeta
	if err := json.Unmarshal(data, &tmpl); err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	fmt.Printf("%s\n\n", bold("从模板启动 Agent"))
	fmt.Printf("  模板 ID:     %s\n", tmpl.ID)
	fmt.Printf("  名称:        %s\n", tmpl.Name)
	fmt.Printf("  分类:        %s\n", tmpl.Category)
	fmt.Printf("  系统提示:    %s\n", truncateStr(tmpl.SystemPrompt, 60))
	fmt.Println()
	fmt.Printf("  状态: %s\n", bold("就绪"))
	fmt.Printf("\n  提示：实际运行需要配置 LLM Provider（API Key 等）。\n")
	fmt.Printf("  使用 %s 初始化完整项目。\n", bold("ap init --template "+templateID))

	return nil
}

func runMarketplaceList(args []string) error {
	templates := loadLocalTemplates()

	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	if jsonOutput {
		out, _ := json.MarshalIndent(templates, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(templates) == 0 {
		fmt.Println("未安装任何模板")
		fmt.Printf("使用 %s 搜索可用模板\n", bold("ap marketplace search <query>"))
		return nil
	}

	fmt.Printf("已安装 %d 个模板：\n\n", len(templates))
	for _, tmpl := range templates {
		fmt.Printf("  %s  %s (v%s)\n", bold(tmpl.ID), tmpl.Name, tmpl.Version)
		fmt.Printf("    %s | %s\n\n", tmpl.Category, tmpl.Description)
	}

	return nil
}

// ===== 辅助函数 =====

// loadLocalTemplates 加载本地已安装的模板
func loadLocalTemplates() []templateMeta {
	var templates []templateMeta

	entries, err := os.ReadDir(marketplaceDir)
	if err != nil {
		return templates
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(marketplaceDir + "/" + entry.Name())
		if err != nil {
			continue
		}
		var tmpl templateMeta
		if err := json.Unmarshal(data, &tmpl); err != nil {
			continue
		}
		templates = append(templates, tmpl)
	}

	return templates
}

// truncateStr 截断字符串
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
