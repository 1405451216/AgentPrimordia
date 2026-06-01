package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/cmd/example/demo"
)

func main() {
	fmt.Println("=== AgentPrimordia Level 1: Hello Agent ===")
	fmt.Println()

	demoLLM := demo.NewDemoLLM("Hello! How can I help you today?")

	agentName := "HelloAgent"
	helloAgent := agent.NewReActAgent(agent.ReActConfig{
		Name:         agentName,
		SystemPrompt: "You are a helpful assistant. Respond concisely and politely.",
		Model:        demoLLM,
		MaxTurns:     5,
	})

	startTime := time.Now()
	resp, err := helloAgent.Run(context.Background(), agent.UserMessage("Hello!"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	duration := time.Since(startTime)

	fmt.Printf("Agent 名称: %s\n", agentName)
	fmt.Printf("用户输入: Hello!\n")
	fmt.Printf("Agent 回复: %s\n", resp.Content)
	fmt.Printf("执行轮数: %d\n", resp.Metrics.TotalTurns)
	fmt.Printf("总耗时: %v\n", duration)
	fmt.Printf("LLM 延迟: %v\n", resp.Metrics.LLMLatency)

	stats := helloAgent.Stats()
	fmt.Printf("Agent 状态: %s\n", stats.Status)

	fmt.Println()
	fmt.Println("--- Level 1 示例运行完成 ---")
}
