package main

import (
	"context"
	"fmt"
	"log"

	ap "agentprimordia/pkg"
)

type MockLLM struct{}

func (m *MockLLM) Complete(ctx context.Context, req *ap.CompletionRequest) (*ap.CompletionResponse, error) {
	return &ap.CompletionResponse{
		ID:      "mock-1",
		Content: "让我帮你读取文件内容。",
		Role:    "assistant",
		Usage:   ap.Usage{PromptTokens: 10, CompletionTokens: 15},
	}, nil
}

func (m *MockLLM) Stream(ctx context.Context, req *ap.CompletionRequest) (<-chan ap.Chunk, error) {
	ch := make(chan ap.Chunk, 1)
	go func() {
		defer close(ch)
		ch <- ap.Chunk{Content: "让我帮你读取文件内容。", Done: true}
	}()
	return ch, nil
}

func (m *MockLLM) CallTools(ctx context.Context, req *ap.ToolCallRequest) (*ap.ToolCallResponse, error) {
	return &ap.ToolCallResponse{Usage: ap.Usage{}}, nil
}

func (m *MockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (m *MockLLM) Info() ap.ModelInfo {
	return ap.ModelInfo{Name: "mock", Provider: "mock", MaxContext: 4096, SupportsTools: true}
}

func main() {
	fmt.Println("=== AgentPrimordia: 工具 + 记忆示例 ===")
	fmt.Println()

	// 使用 DefaultToolkit 快速配置工具集
	registry, err := ap.DefaultToolkit(ap.ToolkitConfig{
		RootDir:     ".",
		EnableFS:    true,
		EnableShell: true,
		EnableWeb:   true,
	})
	if err != nil {
		log.Fatalf("创建 DefaultToolkit 失败: %v", err)
	}

	memory, err := ap.WithInMemory()
	if err != nil {
		log.Fatalf("创建 Memory 失败: %v", err)
	}
	defer memory.Close()

	hooks := ap.NewHookManager()
	hooks.Register(ap.HookBeforeRun, func(ctx context.Context, hctx *ap.HookContext) error {
		fmt.Printf("[Hook] Agent %s 开始运行\n", hctx.AgentID)
		return nil
	})
	hooks.Register(ap.HookAfterRun, func(ctx context.Context, hctx *ap.HookContext) error {
		fmt.Printf("[Hook] Agent %s 运行完成\n", hctx.AgentID)
		return nil
	})

	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "TooledAgent",
		SystemPrompt: "你是一个可以读写文件、执行命令和访问网页的助手",
		Model:        &MockLLM{},
		Toolkit:      registry,
		Memory:       ap.NewMemoryAdapter(memory),
		MaxTurns:     5,
		Hooks:        hooks,
	})

	resp, err := agent.Run(context.Background(), ap.UserMessage("读取当前目录的文件"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("工具调用次数: %d\n", resp.Metrics.TotalTools)

	episodes, _ := memory.Search(context.Background(), "文件", nil)
	fmt.Printf("记忆条目数: %d\n", len(episodes))
}
