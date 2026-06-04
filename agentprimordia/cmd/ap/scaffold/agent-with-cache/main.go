// {{.ProjectName}} — Agent with cache template
// 展示如何在 Agent 上启用 LLM 响应缓存，避免重复 LLM 调用。
//
// 前置条件：
//   export OPENAI_API_KEY=sk-xxx
//
// 跑法：
//   cd {{.ProjectName}}
//   go mod tidy
//   go run .
//
// 缓存要点：
//   - 相同 prompt (含温度等参数) 的请求会被缓存
//   - 缓存命中率可通过 CacheStats 查询
//   - 适合重复查询场景；不适合每次都不同的随机 prompt
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	ap "agentprimordia/pkg"
)

func main() {
	provider := ap.NewOpenAIProvider(ap.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o",
	})

	// 启用 LLM 缓存（内存版,适合开发）
	cache := ap.NewInMemoryCache(ap.InMemoryCacheFullConfig{
		MaxSize: 1000,
		TTL:     300, // 5 分钟
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
	fmt.Println("\n=== 第二次调用 (应命中缓存) ===")
	resp2, err := agent.Run(context.Background(),
		ap.UserMessage("用一句话介绍 Go 语言"))
	if err != nil {
		log.Fatalf("调用失败: %v", err)
	}
	fmt.Printf("回复: %s\n", resp2.Content)

	// 打印缓存统计
	stats := cache.Stats()
	fmt.Printf("\n缓存统计: 命中 %d / 未命中 %d / 总计 %d\n",
		stats.Hits, stats.Misses, stats.Hits+stats.Misses)
}
