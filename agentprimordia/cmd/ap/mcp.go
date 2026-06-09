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
		fmt.Print(`ap mcp — manage MCP servers

Usage:
  ap mcp <subcommand> [arguments]

Subcommands:
  list              list registered MCP servers
  add <name>        register a new MCP server
  remove <name>     remove MCP server
  start <name>      start MCP server
  stop <name>       stop MCP server
  test <name>       test MCP server connectivity
  tools <name>      list tools provided by MCP server

Examples:
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
		errorf("unknown subcommand %q, run %s for help", subcmd, bold("ap mcp --help"))
		os.Exit(1)
	}
}

func mcpList() {
	config := loadAPConfig()
	if config.MCP == nil || len(config.MCP.Servers) == 0 {
		fmt.Println("no MCP servers registered")
		fmt.Println()
		fmt.Println("use ap mcp add <name> to register a server")
		return
	}

	fmt.Printf("%-20s %-30s %-10s %-10s\n", "Name", "Command", "AutoStart", "URL")
	fmt.Println(strings.Repeat("-", 75))
	for name, srv := range config.MCP.Servers {
		autoStart := "no"
		if srv.AutoStart {
			autoStart = "yes"
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
		errorf("please specify server name\nUsage: ap mcp add <name> --command <cmd> [--args ...] [--url <url>]")
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
			i++
			if i >= len(args) {
				errorf("--command requires a value")
				os.Exit(1)
			}
			command = args[i]
		case "--args", "-a":
			i++
			if i >= len(args) {
				errorf("--args requires a value")
				os.Exit(1)
			}
			argsList = strings.Split(args[i], ",")
		case "--url", "-u":
			i++
			if i >= len(args) {
				errorf("--url requires a value")
				os.Exit(1)
			}
			baseURL = args[i]
		case "--auto-start":
			autoStart = true
		case "--env", "-e":
			i++
			if i >= len(args) {
				errorf("--env requires KEY=VALUE")
				os.Exit(1)
			}
			envVars = strings.Split(args[i], ",")
		}
	}

	if command == "" && baseURL == "" {
		errorf("either --command or --url is required")
		os.Exit(1)
	}

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
		errorf("save config failed: %v", err)
		os.Exit(1)
	}

	successf("MCP server %q registered", name)
	if autoStart {
		infof("auto-start: enabled")
	}
}

func mcpRemove(args []string) {
	if len(args) == 0 {
		errorf("please specify server name")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	if config.MCP == nil || config.MCP.Servers == nil {
		errorf("MCP server %q not found", name)
		os.Exit(1)
	}

	if _, ok := config.MCP.Servers[name]; !ok {
		errorf("MCP server %q not found", name)
		os.Exit(1)
	}

	delete(config.MCP.Servers, name)
	if err := saveAPConfig(config); err != nil {
		errorf("save config failed: %v", err)
		os.Exit(1)
	}

	successf("MCP server %q removed", name)
}

func mcpStart(args []string) {
	if len(args) == 0 {
		errorf("please specify server name")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	srvCfg, ok := config.MCP.Servers[name]
	if !ok {
		errorf("MCP server %q not found", name)
		os.Exit(1)
	}

	registry := ap.NewMCPRegistry()
	registry.Register(ap.MCPClientConfig{
		Name:    name,
		Command: srvCfg.Command,
		Args:    srvCfg.Args,
		BaseURL: srvCfg.BaseURL,
	})

	infof("starting MCP server %q ...", name)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := registry.Start(ctx, name); err != nil {
		errorf("start failed: %v", err)
		os.Exit(1)
	}

	successf("MCP server %q started", name)
}

func mcpStop(args []string) {
	if len(args) == 0 {
		errorf("please specify server name")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	if config.MCP == nil || config.MCP.Servers == nil {
		errorf("MCP server %q not found", name)
		os.Exit(1)
	}

	srvCfg, ok := config.MCP.Servers[name]
	if !ok {
		errorf("MCP server %q not found", name)
		os.Exit(1)
	}

	// 对于 URL 类型的 server，从配置中移除 auto-start
	// 对于 command 类型的 server，目前没有进程管理，仅标记
	if srvCfg.AutoStart {
		srvCfg.AutoStart = false
		config.MCP.Servers[name] = srvCfg
		if err := saveAPConfig(config); err != nil {
			warnf("save config failed: %v", err)
		}
	}

	successf("MCP server %q stopped (auto-start disabled)", name)
	infof("note: command-based servers cannot be stopped remotely; restart with 'ap mcp start %s'", name)
}

func mcpTest(args []string) {
	if len(args) == 0 {
		errorf("please specify server name")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	srvCfg, ok := config.MCP.Servers[name]
	if !ok {
		errorf("MCP server %q not found", name)
		os.Exit(1)
	}

	if srvCfg.BaseURL == "" {
		errorf("MCP server %q has no URL configured, cannot test", name)
		os.Exit(1)
	}

	client := ap.NewMCPClient(srvCfg.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	infof("testing MCP server %q connectivity...", name)
	if err := client.Initialize(ctx); err != nil {
		errorf("connection failed: %v", err)
		os.Exit(1)
	}

	tools := client.Tools()
	successf("connected, found %d tools:", len(tools))
	for _, t := range tools {
		fmt.Printf("  - %s: %s\n", t.Name, t.Description)
	}
}

func mcpTools(args []string) {
	if len(args) == 0 {
		errorf("please specify server name")
		os.Exit(1)
	}

	name := args[0]
	config := loadAPConfig()
	srvCfg, ok := config.MCP.Servers[name]
	if !ok {
		errorf("MCP server %q not found", name)
		os.Exit(1)
	}

	if srvCfg.BaseURL == "" {
		errorf("MCP server %q has no URL, please start it first", name)
		os.Exit(1)
	}

	client := ap.NewMCPClient(srvCfg.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		errorf("initialize failed: %v", err)
		os.Exit(1)
	}

	tools := client.Tools()
	if len(tools) == 0 {
		fmt.Println("no tools registered on this server")
		return
	}

	for _, t := range tools {
		fmt.Printf("Name: %s\n", t.Name)
		fmt.Printf("Description: %s\n", t.Description)
		if t.InputSchema != nil {
			schema, _ := json.MarshalIndent(t.InputSchema, "  ", "  ")
			fmt.Printf("Parameters: %s\n", string(schema))
		}
		fmt.Println()
	}
}

// ===== Config types =====

type mcpServerConfig struct {
	Command   string            `json:"command" yaml:"command"`
	Args      []string          `json:"args,omitempty" yaml:"args,omitempty"`
	BaseURL   string            `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	AutoStart bool              `json:"auto_start,omitempty" yaml:"auto_start,omitempty"`
	Env       map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

type mcpConfig struct {
	Servers map[string]mcpServerConfig `json:"servers" yaml:"servers"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
