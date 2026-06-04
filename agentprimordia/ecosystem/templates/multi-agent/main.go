package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	ap "agentprimordia/pkg"
)

type mockLLM struct {
	mu    sync.Mutex
	count int
}

func (m *mockLLM) Complete(ctx context.Context, req *ap.CompletionRequest) (*ap.CompletionResponse, error) {
	m.mu.Lock()
	m.count++
	n := m.count
	m.mu.Unlock()
	return &ap.CompletionResponse{
		ID:      fmt.Sprintf("mock-%d", n),
		Content: fmt.Sprintf("任务 %d 处理完成", n),
		Role:    "assistant",
		Usage:   ap.Usage{PromptTokens: 10, CompletionTokens: 20},
	}, nil
}

func (m *mockLLM) Stream(ctx context.Context, req *ap.CompletionRequest) (<-chan ap.Chunk, error) {
	ch := make(chan ap.Chunk, 1)
	go func() {
		defer close(ch)
		ch <- ap.Chunk{Content: "streaming response", Done: true}
	}()
	return ch, nil
}

func (m *mockLLM) CallTools(ctx context.Context, req *ap.ToolCallRequest) (*ap.ToolCallResponse, error) {
	return &ap.ToolCallResponse{Usage: ap.Usage{}}, nil
}

func (m *mockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (m *mockLLM) Info() ap.ModelInfo {
	return ap.ModelInfo{Name: "mock", Provider: "mock", MaxContext: 4096, SupportsTools: true}
}

func main() {
	fmt.Println("=== {{.ProjectName}}: 多 Agent 协作 ===")

	// 使用 Pool 进行多 Agent 调度
	pool := ap.NewPool(ap.PoolConfig{
		MaxConcurrency: 3,
		DefaultAgent: ap.ReActAgentConfig{
			SystemPrompt: "你是一个任务处理助手",
			MaxTurns:     5,
		},
	})
	defer pool.Close()

	// 替换为你的 LLM Provider:
	// pool.SetModel(ap.NewOpenAIProvider(ap.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-4o"}))
	pool.SetModel(&mockLLM{})

	tasks := []ap.TaskConfig{
		{ID: "task-1", Title: "数据收集", Prompt: "收集相关数据", SessionID: "session-001"},
		{ID: "task-2", Title: "分析处理", Prompt: "分析收集的数据", SessionID: "session-001"},
		{ID: "task-3", Title: "报告生成", Prompt: "生成分析报告", SessionID: "session-001"},
	}

	results, err := pool.Dispatch(context.Background(), tasks)
	if err != nil {
		log.Fatalf("Pool 调度失败: %v", err)
	}

	for _, r := range results {
		status := "成功"
		if r.Error != nil {
			status = r.Error.Error()
		}
		fmt.Printf("任务 [%s] %s: %s (耗时 %v)\n", r.TaskID, r.Task.Title, status, r.Duration)
	}

	stats := pool.Stats()
	fmt.Printf("\n统计: 完成=%d, 失败=%d\n", stats.CompletedTasks, stats.FailedTasks)
}
