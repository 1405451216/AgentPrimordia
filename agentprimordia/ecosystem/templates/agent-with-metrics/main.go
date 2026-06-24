// {{.ProjectName}} — Agent with metrics template
// 展示如何为 Agent 接入 Prometheus 指标，暴露 /metrics HTTP 端点。
//
// 前置条件：
//
//	export OPENAI_API_KEY=sk-xxx
//
// 跑法：
//
//	cd {{.ProjectName}}
//	go mod tidy
//	go run .
//	# 另开终端：
//	curl http://localhost:9090/metrics
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	ap "agentprimordia/pkg"
)

func main() {
	provider, err := ap.NewOpenAIProvider(ap.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o",
	})
	if err != nil {
		log.Fatalf("创建 Provider 失败: %v", err)
	}

	// 创建 Prometheus 指标收集器
	metrics := ap.NewMetrics()

	// 启动 metrics HTTP server
	handler := ap.NewPrometheusHandler(metrics, ":9090")
	go func() {
		if err := handler.Start(); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()
	fmt.Println("📊 Metrics 端点: http://localhost:9090/metrics")

	// 接入 Agent
	agent, err := ap.NewAgent("{{.ProjectName}}", "你是一个智能助手", provider,
		ap.WithMaxTurns(10),
		ap.WithMetrics(metrics),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	// 跑几轮对话
	questions := []string{
		"你好",
		"今天天气如何？",
		"再见",
	}
	for _, q := range questions {
		fmt.Printf("\n问: %s\n", q)
		resp, err := agent.Run(context.Background(), ap.UserMessage(q))
		if err != nil {
			log.Printf("调用失败: %v", err)
			continue
		}
		fmt.Printf("答: %s\n", resp.Content)
	}

	fmt.Println("\n查询指标: curl http://localhost:9090/metrics")
	fmt.Println("按 Enter 退出...")
	fmt.Scanln()
}
