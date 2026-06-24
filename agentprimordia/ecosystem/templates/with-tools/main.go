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

	// 注意：此模板未设置 Model，请根据需要取消注释并配置你的 LLM Provider。
	var provider ap.Provider // nil — 请替换为真实 Provider

	// 使用 NewAgent 构造器注入工具和记忆能力（v0.7.0 推荐）。
	agent, err := ap.NewAgent("{{.ProjectName}}", "你是一个可以读写文件、执行命令和访问网页的助手。", provider,
		ap.WithMaxTurns(20),
		ap.WithToolkit(registry),
		ap.WithMemory(memory),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

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
