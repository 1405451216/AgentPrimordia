// {{.ProjectName}} — Agent with metrics template
// 展示如何为 Agent 接入 Prometheus 指标,跟踪 LLM 调用、工具调用、
// 轮次延迟等关键指标。
//
// 前置条件：
//   export OPENAI_API_KEY=sk-xxx
//
// 跑法：
//   cd {{.ProjectName}}
//   go mod tidy
//   go run .
//   # 另开终端：
//   curl http://localhost:9090/metrics
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	ap "agentprimordia/pkg"
)

func main() {
	provider := ap.NewOpenAIProvider(ap.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o",
	})

	// 创建 Prometheus 指标收集器
	metrics := ap.NewMetrics()

	// 启动 metrics HTTP 端点
	handler := ap.NewPrometheusHandler(metrics)
	go func() {
		http.Handle("/metrics", handler)
		fmt.Println("📊 Metrics 端点: http://localhost:9090/metrics")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	// 接入 Agent
	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "{{.ProjectName}}",
		SystemPrompt: "你是一个智能助手",
		Model:        provider,
		MaxTurns:     10,
	}).WithMetrics(metrics)

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

	// 等 metrics server 启动
	time.Sleep(100 * time.Millisecond)
	fmt.Println("\n查询指标: curl http://localhost:9090/metrics")
	fmt.Println("按 Enter 退出...")
	fmt.Scanln()
}
