package main

import (
	"context"
	"fmt"
	"log"

	ap "agentprimordia/pkg"
)

func main() {
	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "{{.ProjectName}}",
		SystemPrompt: "你是一个智能助手，用中文回答问题。",
		MaxTurns:     10,
		// 设置 Model 为你的 LLM Provider:
		// Model: ap.NewOpenAIProvider(ap.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-4o"}),
	})

	prompt := "你好！"
	resp, err := agent.Run(context.Background(), ap.UserMessage(prompt))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("轮数: %d\n", resp.Metrics.TotalTurns)
}
