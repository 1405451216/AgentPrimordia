package llm

import (
	"context"
	"sync"
	"testing"
	"time"
)

const mockEmbeddingDim = 16

type MockLLM struct {
	mu            sync.Mutex
	responses     []*CompletionResponse
	toolResponses []*ToolCallResponse
	callCount     int
	lastRequest   any
	t             *testing.T
	delay         time.Duration
	err           error
}

func NewMockLLM(t *testing.T) *MockLLM {
	return &MockLLM{
		t:         t,
		responses: make([]*CompletionResponse, 0),
		delay:     0,
	}
}

func (m *MockLLM) WithResponse(content string) *MockLLM {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, &CompletionResponse{
		ID:      "mock-id",
		Content: content,
		Role:    "assistant",
		Usage:   Usage{PromptTokens: 10, CompletionTokens: len(content) / 4, TotalTokens: 10 + len(content)/4},
	})
	return m
}

func (m *MockLLM) WithToolResponse(calls []FunctionCall) *MockLLM {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolResponses = append(m.toolResponses, &ToolCallResponse{
		Content:   "",
		ToolCalls: calls,
		Usage:     Usage{PromptTokens: 20, CompletionTokens: 30, TotalTokens: 50},
	})
	return m
}

func (m *MockLLM) WithDelay(d time.Duration) *MockLLM {
	m.delay = d
	return m
}

func (m *MockLLM) WithError(err error) *MockLLM {
	m.err = err
	return m
}

func (m *MockLLM) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++
	m.lastRequest = req

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.err != nil {
		return nil, m.err
	}

	if len(m.responses) == 0 {
		return &CompletionResponse{
			ID:      "mock-default",
			Content: "This is a default mock response",
			Role:    "assistant",
		}, nil
	}

	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
}

func (m *MockLLM) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	ch := make(chan Chunk, 10)

	go func() {
		defer close(ch)
		resp, err := m.Complete(ctx, req)
		if err != nil {
			return
		}
		ch <- Chunk{Content: resp.Content, Done: true, Usage: &resp.Usage}
	}()

	return ch, nil
}

func (m *MockLLM) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++
	m.lastRequest = req

	if m.err != nil {
		// 即使出错也消耗一个 toolResponse，保持队列顺序一致
		if len(m.toolResponses) > 0 {
			m.toolResponses = m.toolResponses[1:]
		}
		return nil, m.err
	}

	if len(m.toolResponses) == 0 {
		return &ToolCallResponse{
			Content:   "",
			ToolCalls: []FunctionCall{},
			Usage:     Usage{},
		}, nil
	}

	resp := m.toolResponses[0]
	m.toolResponses = m.toolResponses[1:]
	return resp, nil
}

func (m *MockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = make([]float32, mockEmbeddingDim)
	}
	return embeddings, nil
}

func (m *MockLLM) Info() ModelInfo {
	return ModelInfo{
		Name:              "mock-model",
		Provider:          "mock",
		MaxContext:        4096,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}

func (m *MockLLM) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *MockLLM) LastRequest() any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastRequest
}
