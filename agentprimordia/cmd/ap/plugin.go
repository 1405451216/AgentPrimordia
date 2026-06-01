package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runPlugin(args []string) {
	if len(args) == 0 {
		fmt.Print(`ap plugin — 管理插件

用法:
  ap plugin <subcommand> [arguments]

子命令:
  install <module>   从 Go Module 安装插件
  list               列出已安装插件
  create <name>      创建插件项目骨架
  remove <name>      移除插件

示例:
  ap plugin install github.com/user/ap-plugin-slack
  ap plugin list
  ap plugin create ap-plugin-weather
`)
		return
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "install":
		pluginInstall(subargs)
	case "list":
		pluginList()
	case "create":
		pluginCreate(subargs)
	case "remove":
		pluginRemove(subargs)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n", subcmd)
		os.Exit(1)
	}
}

func pluginInstall(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 Go Module 路径\n用法: ap plugin install github.com/user/ap-plugin-xxx")
		os.Exit(1)
	}

	module := args[0]
	dir, err := findProjectDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// go get 安装模块
	fmt.Printf("安装插件: %s\n", module)
	cmd := exec.Command("go", "get", module)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "安装失败: %v\n", err)
		os.Exit(1)
	}

	// 更新配置
	config := loadAPConfig()
	if config.Plugins == nil {
		config.Plugins = []string{}
	}
	config.Plugins = append(config.Plugins, module)
	if err := saveAPConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 保存配置失败: %v\n", err)
	}

	fmt.Printf("✓ 插件 %s 已安装\n", module)
	fmt.Println()
	fmt.Println("在代码中引入插件:")
	fmt.Printf("  import _ %q\n", module)
	fmt.Printf("  // 然后在 init() 中: pluginLoader.Load(%q.NewPlugin())\n", module)
	fmt.Println()
	fmt.Println("运行 ap run 使插件生效")
}

func pluginList() {
	config := loadAPConfig()
	if len(config.Plugins) == 0 {
		fmt.Println("未安装任何插件")
		fmt.Println()
		fmt.Println("使用 ap plugin install <module> 安装插件")
		return
	}

	fmt.Printf("%-40s %s\n", "模块路径", "状态")
	fmt.Println(strings.Repeat("-", 60))
	for _, p := range config.Plugins {
		status := "已安装"
		fmt.Printf("%-40s %s\n", p, status)
	}
}

func pluginCreate(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定插件名称\n用法: ap plugin create ap-plugin-xxx")
		os.Exit(1)
	}

	name := args[0]
	if _, err := os.Stat(name); err == nil {
		fmt.Fprintf(os.Stderr, "错误: 目录 %q 已存在\n", name)
		os.Exit(1)
	}

	// 创建目录结构
	dirs := []string{
		name,
		filepath.Join(name, "tools"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 创建目录失败: %v\n", err)
			os.Exit(1)
		}
	}

	// 生成 go.mod
	goMod := fmt.Sprintf(`module %s

go 1.22

require agentprimordia v0.0.0
`, name)
	os.WriteFile(filepath.Join(name, "go.mod"), []byte(goMod), 0o644)

	// 生成插件入口 plugin.go
	pluginCode := fmt.Sprintf(`package %s

import (
	ap "agentprimordia/pkg"
)

// Plugin 实现 ap.ToolPlugin 接口
type Plugin struct{}

// NewPlugin 创建插件实例（入口点）
func NewPlugin() ap.ToolPlugin {
	return &Plugin{}
}

// Name 返回插件名称
func (p *Plugin) Name() string {
	return %q
}

// Version 返回插件版本
func (p *Plugin) Version() string {
	return "0.1.0"
}

// Tools 返回插件提供的工具列表
func (p *Plugin) Tools() []ap.Tool {
	return []ap.Tool{
		// 在此注册你的工具
	}
}

// Init 初始化插件
func (p *Plugin) Init(config map[string]any) error {
	return nil
}

// Close 清理资源
func (p *Plugin) Close() error {
	return nil
}
`, strings.ReplaceAll(name, "-", "_"), name)
	os.WriteFile(filepath.Join(name, "plugin.go"), []byte(pluginCode), 0o644)

	// 生成 README
	readme := "# " + name + "\n\nAgentPrimordia 插件。\n\n" +
		"## 安装\n\n" +
		"```bash\nap plugin install " + name + "\n```\n\n" +
		"## 使用\n\n" +
		"```go\nimport _ \"" + name + "\"\n\n// 插件会自动注册到 ToolRegistry\n```\n\n" +
		"## 开发\n\n" +
		"```bash\ncd " + name + "\ngo mod tidy\ngo test ./...\n```\n"
	os.WriteFile(filepath.Join(name, "README.md"), []byte(readme), 0o644)

	fmt.Printf("✓ 插件项目 %q 已创建\n\n", name)
	fmt.Printf("目录结构:\n")
	fmt.Printf("  %s/\n", name)
	fmt.Printf("  ├── plugin.go    — 插件入口（实现 ToolPlugin 接口）\n")
	fmt.Printf("  ├── go.mod\n")
	fmt.Printf("  └── README.md\n\n")
	fmt.Printf("下一步:\n")
	fmt.Printf("  cd %s\n", name)
	fmt.Printf("  # 编辑 plugin.go 添加你的工具\n")
	fmt.Printf("  go mod tidy\n")
}

func pluginRemove(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定插件名称")
		os.Exit(1)
	}

	module := args[0]
	config := loadAPConfig()

	found := false
	var newPlugins []string
	for _, p := range config.Plugins {
		if p == module {
			found = true
			continue
		}
		newPlugins = append(newPlugins, p)
	}

	if !found {
		fmt.Fprintf(os.Stderr, "错误: 插件 %q 未安装\n", module)
		os.Exit(1)
	}

	config.Plugins = newPlugins
	if err := saveAPConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 插件 %q 已从配置中移除\n", module)
	fmt.Println("提示: 运行 go mod tidy 清理依赖")
}
