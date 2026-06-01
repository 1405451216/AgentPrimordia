package main

import (
	"fmt"
	"os"
)

const (
	usage = `AgentPrimordia (ap) — Go Agent 开发框架命令行工具

用法:
  ap <command> [arguments]

命令:
  init     创建新的 Agent 项目
  run      编译并运行当前项目
  debug    启动调试服务器
  test     运行 eval 测试套件
  mcp      管理 MCP Server
  plugin   管理插件
  version  显示版本号

使用 "ap <command> --help" 查看子命令详情。
`
)

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		runInit(args)
	case "run":
		runRun(args)
	case "debug":
		runDebug(args)
	case "test":
		runTest(args)
	case "mcp":
		runMCP(args)
	case "plugin":
		runPlugin(args)
	case "version", "-v", "--version":
		fmt.Println("AgentPrimordia CLI v0.1.0")
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		fmt.Print(usage)
		os.Exit(1)
	}
}
