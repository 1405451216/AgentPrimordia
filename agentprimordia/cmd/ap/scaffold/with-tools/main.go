package main

import (
	"context"
	"fmt"
	"log"
	"os"

	ap "agentprimordia/pkg"
)

func main() {
	// 配置工具集
	registry, err := ap.DefaultToolkit(ap.ToolkitConfig{
		RootDir:     ".",
		EnableFS:    true,
		EnableShell: true,
		EnableWeb:   true,
	})
	if err != nil {
		log.Fatalf("创建工具集失败: %v", err)
	}

	// 配置记忆存储
	memory, err := ap.WithInMemory()
	if err != nil {
		log.Fatalf("创建记忆存储失败: %v", err)
	}
	defer memory.Close()

	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "{{.ProjectName}}",
		SystemPrompt: "你是一个可以读写文件、执行命令和访问网页的助手。",
		MaxTurns:     20,
		Toolkit:      registry,
		Memory:       ap.NewMemoryAdapter(memory),
		// 设置 Model 为你的 LLM Provider:
		// Model: ap.NewOpenAIProvider(ap.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-4o"}),
	})

	prompt := "列出当前目录的文件"
	if envPrompt := os.Getenv("AP_PROMPT"); envPrompt != "" {
		prompt = envPrompt
	}

	resp, err := agent.Run(context.Background(), ap.UserMessage(prompt))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("工具调用: %d 次\n", resp.Metrics.TotalTools)
}
