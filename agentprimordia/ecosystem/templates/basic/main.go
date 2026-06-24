package main

import (
	"context"
	"fmt"
	"log"

	ap "agentprimordia/pkg"
)

func main() {
	// 设置 Model 为你的 LLM Provider:
	// provider, err := ap.NewOpenAIProvider(ap.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-4o"})
	// if err != nil {
	// 	log.Fatalf("创建 Provider 失败: %v", err)
	// }
	var provider ap.Provider // nil — 请替换为真实 Provider

	agent, err := ap.NewAgent("{{.ProjectName}}", "你是一个智能助手，用中文回答问题。", provider,
		ap.WithMaxTurns(10),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	prompt := "你好！"
	resp, err := agent.Run(context.Background(), ap.UserMessage(prompt))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("轮数: %d\n", resp.Metrics.TotalTurns)
}
