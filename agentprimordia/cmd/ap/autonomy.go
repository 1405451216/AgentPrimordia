package main

import (
	"fmt"
)

const autonomyUsage = `Usage: ap autonomy <subcommand> [arguments]

Subcommands:
  run <goal>       提交并执行自治目标
  list             列出所有目标及状态
  resume <id>      恢复未完成目标
  status <id>      查看目标详细状态

Examples:
  ap autonomy run "监控数据异常并自动修复"
  ap autonomy list
  ap autonomy resume goal-abc123
  ap autonomy status goal-abc123
`

func runAutonomy(args []string) error {
	if len(args) == 0 {
		fmt.Print(autonomyUsage)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "run":
		return runAutonomyRun(subArgs)
	case "list":
		return runAutonomyList(subArgs)
	case "resume":
		return runAutonomyResume(subArgs)
	case "status":
		return runAutonomyStatus(subArgs)
	case "--help", "-h", "help":
		fmt.Print(autonomyUsage)
		return nil
	default:
		return fmt.Errorf("unknown autonomy subcommand %q, run \"ap autonomy --help\"", sub)
	}
}

func runAutonomyRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap autonomy run <goal description>")
	}
	goal := args[0]
	fmt.Printf("🚀 提交自治目标: %s\n", goal)
	fmt.Println("   状态: created → 等待规划")
	fmt.Println("   提示: 自治运行时需要配置 StepExecutor，请通过 SDK 编程式使用")
	fmt.Println("   示例: pkg.NewAutonomyRuntime(pkg.RuntimeConfig{...})")
	return nil
}

func runAutonomyList(args []string) error {
	_ = args
	fmt.Println("📋 自治目标列表:")
	fmt.Println("   (暂无活跃目标 — 通过 SDK 或 'ap autonomy run' 提交)")
	return nil
}

func runAutonomyResume(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap autonomy resume <goal-id>")
	}
	goalID := args[0]
	fmt.Printf("🔄 恢复目标: %s\n", goalID)
	fmt.Println("   提示: 恢复需要配置 CheckpointStore，请通过 SDK 编程式使用")
	return nil
}

func runAutonomyStatus(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap autonomy status <goal-id>")
	}
	goalID := args[0]
	fmt.Printf("📊 目标状态: %s\n", goalID)
	fmt.Println("   提示: 状态查询需要运行时实例，请通过 SDK 编程式使用")
	return nil
}
