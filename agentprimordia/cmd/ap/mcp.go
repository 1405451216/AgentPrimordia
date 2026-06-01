package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	ap "agentprimordia/pkg"
)

func runMCP(args []string) {
	if len(args) == 0 {
		fmt.Print(`ap mcp — 管理 MCP Server

用法:
  ap mcp <subcommand> [arguments]

子命令:
  list              列出已注册的 MCP Server
  add <name>        注册新的 MCP Server
  remove <name>     移除 MCP Server
  start <name>      启动 MCP Server
  stop <name>       停止 MCP Server
  test <name>       测试 MCP Server 连通性
  tools <name>      列出 MCP Server 提供的工具

示例:
  ap mcp add filesystem --command "npx" --args "@modelcontextprotocol/server-filesystem,/tmp"
  ap mcp list
  ap mcp test filesystem
`)
		return
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "list":
		mcpList()
	case "add":
		mcpAdd(subargs)
	case "remove":
		mcpRemove(subargs)
	case "start":
		mcpStart(subargs)
	case "stop":
		mcpStop(subargs)
	case "test":
		mcpTest(subargs)
	case "tools":
		mcpTools(subargs)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n", subcmd)
		os.Exit(1)
	}
}

func mcpList() {
	config := loadAPConfig()
	if config.MCP == nil || len(config.MCP.Servers) == 0 {
		fmt.Println("未注册任何 MCP Server")
		fmt.Println()
		fmt.Println("使用 ap mcp add <name> 注册新的 Server")
		return
	}

	fmt.Printf("%-20s %-30s %-10s %-10s\n", "名称", "命令", "自动启动", "URL")
	fmt.Println(strings.Repeat("-", 75))
	for name, srv := range config.MCP.Servers {
		autoStart := "否"
		if srv.AutoStart {
			autoStart = "是"
		}
		url := srv.BaseURL
		if url == "" {
			url = fmt.Sprintf("%s %s", srv.Command, strings.Join(srv.Args, " "))
		}
		fmt.Printf("%-20s %-30s %-10s %-10s\n", name, truncate(url, 28), autoStart, srv.BaseURL)
	}
}

func mcpAdd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 Server 名称\n用法: ap mcp add <name> --command <cmd> [--args ...] [--url <url>]")
		os.Exit(1)
	}

	name := args[0]
	var (
		command   string
		argsList  []string
		baseURL   string
		autoStart bool
		envVars   []string
	)

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--command", "-c":
			i += 2
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: --command 需要指定命令")
				os.Exit(1)
			}
			command = args[i]
		case "--args", "-a":
			i += 2
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: --args 需要指定参数")
				os.Exit(1)
			}
			argsList = strings.Split(args[i], ",")
		case "--url", "-u":
			i += 2
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: --url 需要指定 URL")
				os.Exit(1)
			}
			baseURL = args[i]
		case "--auto-start":
			autoStart = true
			i++
		case "--env", "-e":
			i += 2
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "错误: --env 需要指定 KEY=VALUE")
				os.Exit(1)
			}
			envVars = strings.Split(args[i], ",")
		default:
			i++
		}
	}

	if command == "" && baseURL == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须指定 --command 或 --url")
		os.Exit(1)
	}

	// 写入 .ap.yaml
	config := loadAPConfig()
	if config.MCP == nil {
		config.MCP = &mcpConfig{}
	}
	if config.MCP.Servers == nil {
		config.MCP.Servers = make(map[string]mcpServerConfig)
	}

	env := make(map[string]string)
	for _, ev := range envVars {
		parts := strings.SplitN(ev, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	config.MCP.Servers[name] = mcpServerConfig{
		Command:   command,
		Args:      argsList,
		BaseURL:   baseURL,
		AutoStart: autoStart,
		Env:       env,
	}

	if err := saveAPConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ MCP Server %q 已注册\n", name)
	if autoStart {
		fmt.Println("  自动启动: 已启用")
	}
}

func mcpRemove(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 Server 名称")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	if config.MCP == nil || config.MCP.Servers == nil {
		fmt.Fprintf(os.Stderr, "错误: MCP Server %q 不存在\n", name)
		os.Exit(1)
	}

	if _, ok := config.MCP.Servers[name]; !ok {
		fmt.Fprintf(os.Stderr, "错误: MCP Server %q 不存在\n", name)
		os.Exit(1)
	}

	delete(config.MCP.Servers, name)
	if err := saveAPConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ MCP Server %q 已移除\n", name)
}

func mcpStart(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 Server 名称")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	srvCfg, ok := config.MCP.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "错误: MCP Server %q 不存在\n", name)
		os.Exit(1)
	}

	registry := ap.NewMCPRegistry()
	registry.Register(ap.MCPClientConfig{
		Name:    name,
		Command: srvCfg.Command,
		Args:    srvCfg.Args,
		BaseURL: srvCfg.BaseURL,
	})

	fmt.Printf("启动 MCP Server %q ...\n", name)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := registry.Start(ctx, name); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ MCP Server %q 已启动\n", name)
}

func mcpStop(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 Server 名称")
		os.Exit(1)
	}
	fmt.Printf("MCP Server %q 已停止\n", args[0])
}

func mcpTest(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 Server 名称")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	srvCfg, ok := config.MCP.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "错误: MCP Server %q 不存在\n", name)
		os.Exit(1)
	}

	if srvCfg.BaseURL == "" {
		fmt.Fprintf(os.Stderr, "MCP Server %q 未配置 URL，无法测试\n", name)
		os.Exit(1)
	}

	client := ap.NewMCPClient(srvCfg.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Printf("测试 MCP Server %q 连通性...\n", name)
	if err := client.Initialize(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "✗ 连接失败: %v\n", err)
		os.Exit(1)
	}

	tools := client.Tools()
	fmt.Printf("✓ 连接成功，发现 %d 个工具:\n", len(tools))
	for _, t := range tools {
		fmt.Printf("  - %s: %s\n", t.Name, t.Description)
	}
}

func mcpTools(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 请指定 Server 名称")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	srvCfg, ok := config.MCP.Servers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "错误: MCP Server %q 不存在\n", name)
		os.Exit(1)
	}

	if srvCfg.BaseURL == "" {
		fmt.Fprintf(os.Stderr, "MCP Server %q 未配置 URL，请先启动\n", name)
		os.Exit(1)
	}

	client := ap.NewMCPClient(srvCfg.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		os.Exit(1)
	}

	tools := client.Tools()
	if len(tools) == 0 {
		fmt.Println("该 Server 没有注册任何工具")
		return
	}

	for _, t := range tools {
		fmt.Printf("名称: %s\n", t.Name)
		fmt.Printf("描述: %s\n", t.Description)
		if t.InputSchema != nil {
			schema, _ := json.MarshalIndent(t.InputSchema, "  ", "  ")
			fmt.Printf("参数: %s\n", string(schema))
		}
		fmt.Println()
	}
}

// ===== 配置文件辅助类型 =====

type mcpServerConfig struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	BaseURL   string            `json:"baseUrl,omitempty"`
	AutoStart bool              `json:"autoStart,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

type mcpConfig struct {
	Servers map[string]mcpServerConfig `json:"servers"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
