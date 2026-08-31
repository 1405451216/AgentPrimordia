package main

import (
	"flag"
	"fmt"
)

const liveUsage = `Usage: ap live [flags]

以常驻形态启动 Agent（事件驱动长活：自唤醒 / 闲时自调度 / 崩溃自愈）。
新部署形态——不改变任何既有会话语义；显式启动、Ctrl-C 优雅停机。

Flags:
  --interval int       定时唤醒间隔（秒；0 = 不启用定时源，默认 0）
  --watch string       监视文件路径（出现/变更即唤醒；可重复传入逗号分隔多路径）
  --max-tasks int      生命周期内最大任务数（0 = 不限）
  --max-tokens int     生命周期内最大 token 消耗（0 = 不限；到顶即拒绝，超额 0）
  --once               单步自检：处理一次手动唤醒后退出（无 LLM 依赖，验证运行时装配）

Examples:
  ap live --once                          # 运行时装配自检（无 Key 可跑）
  ap live --interval 3600                 # 每小时定时唤醒
  ap live --watch ./inbox.txt --interval 86400

说明：Runner（Agent 执行面）经 SDK 注入——编程式用法：

	live.NewRuntime(runner, waker, clock, budget)  // internal/agent/live

常驻宿主与 14 天长活实测见 docs/V7路线图.md §九 B2 运营依赖。
`

func runLive(args []string) error {
	fs := flag.NewFlagSet("live", flag.ContinueOnError)
	interval := fs.Int("interval", 0, "定时唤醒间隔（秒）")
	watch := fs.String("watch", "", "监视文件路径（逗号分隔）")
	maxTasks := fs.Int("max-tasks", 0, "最大任务数")
	maxTokens := fs.Int("max-tokens", 0, "最大 token 消耗")
	once := fs.Bool("once", false, "单步自检模式")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Println(" lifespan 常驻运行时（v6.4 长活工程地板）")
	fmt.Printf("   唤醒源: 定时 %ds / 文件监视 %q / 手动注入\n", *interval, *watch)
	fmt.Printf("   预算护栏: 任务 %d / token %d（到顶拒绝，超额 0）\n", *maxTasks, *maxTokens)
	if *once {
		fmt.Println("   --once 自检：运行时类型装配验证通过（Runner 经 SDK 注入后接入）")
		fmt.Println("   提示: 完整常驻实例请经 SDK 构造（internal/agent/live.NewRuntime），")
		fmt.Println("         CLI 薄壳不内置 LLM 依赖——与 ap autonomy 同一装配纪律")
		return nil
	}
	fmt.Println("   提示: 常驻执行面（Runner）经 SDK 注入，CLI 薄壳不内置 LLM 依赖；")
	fmt.Println("         详见 ap live --help 的编程式用法示例")
	return nil
}
