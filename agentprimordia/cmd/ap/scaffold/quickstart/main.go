package main

import (
	"context"
	"fmt"
	"log"
	"os"

	ap "agentprimordia/pkg"
)

// 🚀 AgentPrimordia 快速入门示例
// 这是一个最小化的Agent示例，帮助你在5分钟内体验核心功能

func main() {
	fmt.Println("🚀 AgentPrimordia 快速入门")
	fmt.Println("=========================")
	fmt.Println()

	// 步骤1: 配置LLM提供者
	// 从环境变量读取API密钥（推荐做法）
	apiKey := os.Getenv("AP_LLM_API_KEY")
	if apiKey == "" {
		apiKey = "your-api-key-here" // 开发时使用，生产环境必须设置环境变量
		fmt.Println("⚠️  提示: 设置环境变量 AP_LLM_API_KEY 以使用真实LLM")
		fmt.Println("   当前使用演示模式")
		fmt.Println()
	}

	// 创建LLM提供者（这里使用OpenAI兼容接口）
	provider, err := ap.NewOpenAIProvider(ap.Config{
		APIKey:  apiKey,
		Model:   "gpt-4o-mini", // 快速且经济
		BaseURL: "https://api.openai.com/v1",
	})
	if err != nil {
		log.Fatalf("❌ 创建LLM失败: %v", err)
	}

	// 步骤2: 创建Agent
	// 使用NewAgent，配置基本参数
	myAgent, err := ap.NewAgent("QuickStartAgent", "你是一个友好的AI助手，用简洁的中文回答问题。", provider,
		ap.WithMaxTurns(10), // 最大对话轮数
	)
	if err != nil {
		log.Fatalf("❌ 创建Agent失败: %v", err)
	}

	fmt.Println("✅ Agent已创建")
	fmt.Printf("   名称: %s\n", myAgent.Name())
	fmt.Println()

	// 步骤3: 运行Agent
	fmt.Println("💬 开始对话...")
	fmt.Println()

	ctx := context.Background()

	// 发送第一条消息
	userMessage := ap.UserMessage("你好！请用一句话介绍自己")
	response, err := myAgent.Run(ctx, userMessage)
	if err != nil {
		log.Fatalf("❌ Agent运行失败: %v", err)
	}

	fmt.Printf("👤 用户: %s\n", userMessage.Content)
	fmt.Printf("🤖 助手: %s\n", response.Content)
	fmt.Println()

	// 步骤4: 查看统计信息
	stats := myAgent.Stats()
	fmt.Println("📊 运行统计:")
	fmt.Printf("   状态: %s\n", stats.Status)
	fmt.Printf("   当前轮数: %d\n", stats.CurrentTurn)
	fmt.Printf("   消息总数: %d\n", stats.TotalMessages)
	fmt.Printf("   工具调用: %v\n", stats.ToolsCalled)
	fmt.Println()

	fmt.Println("🎉 恭喜！你已成功运行第一个Agent")
	fmt.Println()
	fmt.Println("📚 下一步:")
	fmt.Println("   1. 尝试修改SystemPrompt，改变Agent的行为")
	fmt.Println("   2. 添加多轮对话，体验上下文记忆")
	fmt.Println("   3. 使用 --template with-tools 创建带工具的Agent")
	fmt.Println("   4. 访问文档了解更多: https://github.com/AgentPrimordia/docs")
}
