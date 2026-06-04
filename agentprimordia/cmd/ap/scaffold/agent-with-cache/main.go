// {{.ProjectName}} — Agent with cache template
// 展示如何在 Agent 上启用 LLM 响应缓存，避免重复 LLM 调用。
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

	// 启用 LLM 缓存（简化配置：仅 maxSize + minScore，无 embedding）
	cache := ap.NewInMemoryCacheWithFullConfig(ap.InMemoryCacheFullConfig{
		MaxSize:  1000,
		MinScore: 0.8,
	})

	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "{{.ProjectName}}",
		SystemPrompt: "你是一个智能助手",
		Model:        provider,
		MaxTurns:     10,
	}).WithCache(cache)

	// 第一次调用 — 真实请求 LLM
	fmt.Println("=== 第一次调用 ===")
	resp1, err := agent.Run(context.Background(),
		ap.UserMessage("用一句话介绍 Go 语言"))
	if err != nil {
		log.Fatalf("调用失败: %v", err)
	}
	fmt.Printf("回复: %s\n", resp1.Content)

	// 第二次相同调用 — 应命中缓存
	fmt.Println("\n=== 第二次调用 ===")
	resp2, err := agent.Run(context.Background(),
		ap.UserMessage("用一句话介绍 Go 语言"))
	if err != nil {
		log.Fatalf("调用失败: %v", err)
	}
	fmt.Printf("回复: %s\n", resp2.Content)

	// 打印缓存统计
	stats := cache.Stats(context.Background())
	fmt.Printf("\n缓存统计: 总查询 %d, 命中 %d, 未命中 %d, 命中率 %.1f%%\n",
		stats.TotalQueries, stats.CacheHits, stats.CacheMisses,
		stats.HitRate*100)
}
