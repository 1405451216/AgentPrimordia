package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	ap "agentprimordia/pkg"
)

type SimpleMockLLM struct {
	mu        sync.Mutex
	responses []string
	index     int
}

func NewSimpleMockLLM(responses ...string) *SimpleMockLLM {
	return &SimpleMockLLM{responses: responses}
}

func (m *SimpleMockLLM) Complete(ctx context.Context, req *ap.CompletionRequest) (*ap.CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	content := "I'm a mock assistant. How can I help?"
	if m.index < len(m.responses) {
		content = m.responses[m.index]
		m.index++
	}

	return &ap.CompletionResponse{
		ID:      "mock-1",
		Content: content,
		Role:    "assistant",
		Usage:   ap.Usage{PromptTokens: 10, CompletionTokens: 20},
	}, nil
}

func (m *SimpleMockLLM) Stream(ctx context.Context, req *ap.CompletionRequest) (<-chan ap.Chunk, error) {
	ch := make(chan ap.Chunk, 1)
	go func() {
		defer close(ch)
		resp, _ := m.Complete(ctx, req)
		if resp != nil {
			ch <- ap.Chunk{Content: resp.Content, Done: true}
		}
	}()
	return ch, nil
}

func (m *SimpleMockLLM) CallTools(ctx context.Context, req *ap.ToolCallRequest) (*ap.ToolCallResponse, error) {
	return &ap.ToolCallResponse{Usage: ap.Usage{}}, nil
}

func (m *SimpleMockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (m *SimpleMockLLM) Info() ap.ModelInfo {
	return ap.ModelInfo{Name: "mock", Provider: "mock", MaxContext: 4096, SupportsTools: true}
}

func main() {
	fmt.Println("=== AgentPrimordia: 基础示例 ===")
	fmt.Println()

	// 使用 PromptTemplate 构建系统提示词
	tmpl := ap.NewPromptTemplate("你是一个{{.Role}}助手，专注于{{.Domain}}领域。")
	tmpl.WithVar("Role", "Go开发").WithVar("Domain", "Agent框架")
	systemPrompt, err := tmpl.Render()
	if err != nil {
		log.Fatalf("渲染模板失败: %v", err)
	}
	fmt.Printf("系统提示词: %s\n\n", systemPrompt)

	mockLLM := NewSimpleMockLLM("你好！我是 AgentPrimordia 助手，有什么可以帮你的？")

	agent := ap.NewReActAgent(ap.ReActConfig{
		Name:         "BasicAgent",
		SystemPrompt: systemPrompt,
		Model:        mockLLM,
		MaxTurns:     3,
	})

	resp, err := agent.Run(context.Background(), ap.UserMessage("你好！"))
	if err != nil {
		log.Fatalf("Agent 运行失败: %v", err)
	}

	fmt.Printf("回复: %s\n", resp.Content)
	fmt.Printf("轮数: %d\n", resp.Metrics.TotalTurns)
	fmt.Printf("版本: %s\n", ap.Version)
}
