package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"agentprimordia/cmd/example/demo"
	ap "agentprimordia/pkg"
)

func main() {
	fmt.Println("🚀 最简 Agent 示例 - 无需 API Key!")
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println()

	demoLLM := demo.NewDemoLLM(
		"你好!我是AI助手,很高兴为你服务!",
	)

	simpleAgent := ap.NewAgent("SimpleBot", "你是一个友好的AI助手,用中文简洁回答。", demoLLM, ap.WithMaxTurns(3))

	fmt.Println("📝 用户输入: 你好")
	fmt.Println("🤖 Agent 正在思考...")

	startTime := time.Now()
	resp, err := simpleAgent.Run(context.Background(), ap.UserMessage("你好"))
	if err != nil {
		log.Fatalf("❌ 运行失败: %v", err)
	}

	duration := time.Since(startTime)

	fmt.Println()
	fmt.Println("✅ Agent 回复:", resp.Content)
	fmt.Println("⏱️  耗时:", duration)
	fmt.Println("🔄 执行轮数:", resp.Metrics.TotalTurns)
	fmt.Println()

	stats := simpleAgent.Stats()
	fmt.Println("📊 Agent 状态:", stats.Status)
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println("🎉 示例运行成功! 框架工作正常!")
}
