package main

import (
	"context"
	"fmt"
	"log"

	ap "agentprimordia/pkg"
)

// MockLLM 是示例用的模拟 LLM
type MockLLM struct{}

func (m *MockLLM) Complete(ctx context.Context, req *ap.CompletionRequest) (*ap.CompletionResponse, error) {
	return &ap.CompletionResponse{
		ID: "mock-1", Content: "你好！我是链式 API 创建的 Agent，有什么可以帮你的？",
		Role: "assistant", Usage: ap.Usage{PromptTokens: 10, CompletionTokens: 20},
	}, nil
}
func (m *MockLLM) Stream(ctx context.Context, req *ap.CompletionRequest) (<-chan ap.Chunk, error) {
	ch := make(chan ap.Chunk, 1)
	go func() { defer close(ch); ch <- ap.Chunk{Content: "你好！", Done: true} }()
	return ch, nil
}
func (m *MockLLM) CallTools(ctx context.Context, req *ap.ToolCallRequest) (*ap.ToolCallResponse, error) {
	return &ap.ToolCallResponse{Usage: ap.Usage{}}, nil
}
func (m *MockLLM) Info() ap.ModelInfo {
	return ap.ModelInfo{Name: "mock", Provider: "mock", MaxContext: 4096, SupportsTools: true}
}

func main() {
	fmt.Println("=== 链式 API：最简 Agent ===")
	fmt.Println()

	// 只需 4 个必填字段即可创建 Agent
	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:    "hello-agent",
		Model:   &MockLLM{},
		MaxTurns: 3,
	})

	resp, err := agent.Run(context.Background(), ap.UserMessage("你好！"))
	if err != nil {
		log.Fatalf("运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("轮数: %d\n", resp.Metrics.TotalTurns)
}
