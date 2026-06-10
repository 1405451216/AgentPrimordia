package main

import (
	"context"
	"fmt"
	"log"

	ap "agentprimordia/pkg"
	"agentprimordia/testutil"
)

func main() {
	fmt.Println("=== AgentPrimordia: 基础示例 ===")
	fmt.Println()

	// 使用 PromptTemplate 构建系统提示词
	tmpl := ap.NewPromptTemplate("你是一个{{.Role}}助手，专注于{{.Domain}}领域。")
	tmpl.WithVar("Role", "Go开发").WithVar("Domain", "Agent框架")
	systemPrompt, err := tmpl.Render()
	if err != nil {
		log.Fatalf("渲染模板失败: %v", err)
	}
	fmt.Printf("系统提示词: %s\n\n", systemPrompt)

	mockLLM := testutil.NewMockProvider("你好！我是 AgentPrimordia 助手，有什么可以帮你的？")

	agent := ap.NewAgent("BasicAgent", systemPrompt, mockLLM, ap.WithMaxTurns(3))

	resp, err := agent.Run(context.Background(), ap.UserMessage("你好！"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("轮数: %d\n", resp.Metrics.TotalTurns)
	fmt.Printf("版本: %s\n", ap.Version)
}
