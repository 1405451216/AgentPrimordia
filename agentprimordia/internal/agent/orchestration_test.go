//go:build ignore

package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"agentprimordia/internal/llm"
	"agentprimordia/internal/tools"
)

// ===== Pipeline 测试 =====

func TestPipeline_SimpleSequence(t *testing.T) {
	mock1 := &orchestrationMockLLM{response: "Step 1 done"}
	mock2 := &orchestrationMockLLM{response: "Step 2 done"}

	agent1, err := NewAgent("step1-agent", "", mock1, WithMaxTurns(10))
	if err != nil {
		t.Fatal(err)
	}
	agent1 = agent1.WithToolkit(tools.NewRegistry())
	agent2, err := NewAgent("step2-agent", "", mock2, WithMaxTurns(10))
	if err != nil {
		t.Fatal(err)
	}
	agent2 = agent2.WithToolkit(tools.NewRegistry())

	pipeline := NewPipeline(
		PipelineStep{Name: "step1", Agent: agent1},
		PipelineStep{Name: "step2", Agent: agent2},
	)

	result, err := pipeline.Run(context.Background(), "Start")
	if err != nil {
		t.Fatalf("Pipeline.Run() error = %v", err)
	}
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Final != "Step 2 done" {
		t.Errorf("expected 'Step 2 done', got '%s'", result.Final)
	}
	if result.Steps[0].Output != "Step 1 done" {
		t.Errorf("step 1 output: %s", result.Steps[0].Output)
	}
}

func TestPipeline_StepFailure(t *testing.T) {
	failMock := &orchestrationMockLLM{shouldFail: true}

	agent1, err := NewAgent("fail-agent", "", failMock, WithMaxTurns(10))
	if err != nil {
		t.Fatal(err)
	}
	agent1 = agent1.WithToolkit(tools.NewRegistry())

	pipeline := NewPipeline(
		PipelineStep{Name: "fail-step", Agent: agent1},
	)

	_, err = pipeline.Run(context.Background(), "Start")
	if err == nil {
		t.Error("expected error from failing pipeline step")
	}
}

// ===== Handoff 测试 =====

func TestHandoff_SingleAgent(t *testing.T) {
	mock := &orchestrationMockLLM{response: "Handled!"}

	agent1, err := NewAgent("handler-agent", "", mock, WithMaxTurns(10))
	if err != nil {
		t.Fatal(err)
	}
	agent1 = agent1.WithToolkit(tools.NewRegistry())

	handoff := NewHandoff(HandoffConfig{
		Agents: []Agent{agent1},
		Router: func(ctx context.Context, input string) int {
			return 0 // 总是路由到第一个 Agent
		},
	})

	result, err := handoff.Run(context.Background(), "Help me")
	if err != nil {
		t.Fatalf("Handoff.Run() error = %v", err)
	}
	if result.Output != "Handled!" {
		t.Errorf("expected 'Handled!', got '%s'", result.Output)
	}
}

func TestHandoff_NoAgentCanHandle(t *testing.T) {
	handoff := NewHandoff(HandoffConfig{
		Agents: []Agent{},
		Router: func(ctx context.Context, input string) int {
			return -1
		},
	})

	_, err := handoff.Run(context.Background(), "Help me")
	if err == nil {
		t.Error("expected error when no agent can handle")
	}
}

// ===== Parallel 测试 =====

func TestParallelRun(t *testing.T) {
	mock1 := &orchestrationMockLLM{response: "Agent 1 result"}
	mock2 := &orchestrationMockLLM{response: "Agent 2 result"}

	agent1, err := NewAgent("parallel-agent-1", "", mock1, WithMaxTurns(10))
	if err != nil {
		t.Fatal(err)
	}
	agent1 = agent1.WithToolkit(tools.NewRegistry())
	agent2, err := NewAgent("parallel-agent-2", "", mock2, WithMaxTurns(10))
	if err != nil {
		t.Fatal(err)
	}
	agent2 = agent2.WithToolkit(tools.NewRegistry())

	result, err := ParallelRun(context.Background(), []Agent{agent1, agent2}, "Test input", nil)
	if err != nil {
		t.Fatalf("ParallelRun() error = %v", err)
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Output != "Agent 1 result" {
		t.Errorf("agent 1 output: %s", result.Results[0].Output)
	}
	if result.Results[1].Output != "Agent 2 result" {
		t.Errorf("agent 2 output: %s", result.Results[1].Output)
	}
}

// ===== StreamRun 测试 =====

func TestStreamRun_SimpleCompletion(t *testing.T) {
	mock := &orchestrationStreamLLM{chunks: []string{"Hello", " world", "!"}}

	agent, err := NewAgent("stream-agent", "", mock, WithMaxTurns(10))
	if err != nil {
		t.Fatal(err)
	}
	agent = agent.WithToolkit(tools.NewRegistry())

	ch, err := agent.StreamRun(context.Background(), UserMessage("Hi"))
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}

	var tokens []string
	var gotComplete bool
	for evt := range ch {
		switch evt.Type {
		case StreamEventToken:
			tokens = append(tokens, evt.Content)
		case StreamEventComplete:
			gotComplete = true
		}
	}

	if !gotComplete {
		t.Error("expected StreamEventComplete")
	}
	if len(tokens) == 0 {
		t.Error("expected at least 1 token event")
	}
}

// ===== Mock LLM for orchestration tests =====

type orchestrationMockLLM struct {
	response   string
	shouldFail bool
}

func (m *orchestrationMockLLM) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock error")
	}
	return &llm.CompletionResponse{
		ID:      "mock-orch-id",
		Content: m.response,
		Role:    "assistant",
		Usage:   llm.Usage{PromptTokens: 5, CompletionTokens: 10},
	}, nil
}

func (m *orchestrationMockLLM) Stream(ctx context.Context, req *llm.CompletionRequest) (<-chan llm.Chunk, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *orchestrationMockLLM) CallTools(ctx context.Context, req *llm.ToolCallRequest) (*llm.ToolCallResponse, error) {
	return &llm.ToolCallResponse{
		Content: m.response,
		Usage:   llm.Usage{},
	}, nil
}

func (m *orchestrationMockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *orchestrationMockLLM) Info() llm.ModelInfo {
	return llm.ModelInfo{Name: "orchestration-mock", Provider: "mock", MaxContext: 4096, SupportsTools: true}
}

// streaming mock
type orchestrationStreamLLM struct {
	chunks []string
}

func (m *orchestrationStreamLLM) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		ID:      "mock-stream-id",
		Content: "Hello world!",
		Role:    "assistant",
		Usage:   llm.Usage{PromptTokens: 5, CompletionTokens: 3},
	}, nil
}

func (m *orchestrationStreamLLM) Stream(ctx context.Context, req *llm.CompletionRequest) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk, len(m.chunks)+1)
	go func() {
		defer close(ch)
		for _, c := range m.chunks {
			ch <- llm.Chunk{Content: c}
		}
		ch <- llm.Chunk{Done: true, Usage: &llm.Usage{PromptTokens: 5, CompletionTokens: 3}}
	}()
	return ch, nil
}

func (m *orchestrationStreamLLM) CallTools(ctx context.Context, req *llm.ToolCallRequest) (*llm.ToolCallResponse, error) {
	return &llm.ToolCallResponse{
		Content: "Hello world!",
		Usage:   llm.Usage{},
	}, nil
}

func (m *orchestrationStreamLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *orchestrationStreamLLM) Info() llm.ModelInfo {
	return llm.ModelInfo{Name: "stream-mock", Provider: "mock", MaxContext: 4096, SupportsTools: true, SupportsStreaming: true}
}

// ===== Hook 编排测试 =====

type mockAgentForOrch struct {
	name   string
	output string
}

func (m *mockAgentForOrch) Run(_ context.Context, _ Message) (*Response, error) {
	return &Response{Content: m.output}, nil
}

func (m *mockAgentForOrch) StreamRun(_ context.Context, _ Message) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: StreamEventComplete, Content: m.output}
	close(ch)
	return ch, nil
}

func (m *mockAgentForOrch) Stop() {}

func (m *mockAgentForOrch) Stats() AgentStats {
	return AgentStats{Status: StatusIdle}
}

func (m *mockAgentForOrch) Name() string { return m.name }

func TestPipeline_Hooks_Fired(t *testing.T) {
	agent1 := &mockAgentForOrch{name: "step1", output: "out1"}
	agent2 := &mockAgentForOrch{name: "step2", output: "out2"}

	hm := NewHookManager()
	var fired []HookPoint
	hm.Register(HookBeforePipelineStep, func(_ context.Context, _ *HookContext) error {
		fired = append(fired, HookBeforePipelineStep)
		return nil
	})
	hm.Register(HookAfterPipelineStep, func(_ context.Context, _ *HookContext) error {
		fired = append(fired, HookAfterPipelineStep)
		return nil
	})

	pipeline := NewPipeline(
		PipelineStep{Name: "step1", Agent: agent1},
		PipelineStep{Name: "step2", Agent: agent2},
	)
	pipeline.SetHooks(hm)

	_, err := pipeline.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Pipeline.Run() error = %v", err)
	}

	if len(fired) != 4 {
		t.Fatalf("expected 4 hook firings, got %d: %v", len(fired), fired)
	}
	if fired[0] != HookBeforePipelineStep {
		t.Errorf("fired[0] = %v, want HookBeforePipelineStep", fired[0])
	}
	if fired[1] != HookAfterPipelineStep {
		t.Errorf("fired[1] = %v, want HookAfterPipelineStep", fired[1])
	}
	if fired[2] != HookBeforePipelineStep {
		t.Errorf("fired[2] = %v, want HookBeforePipelineStep", fired[2])
	}
	if fired[3] != HookAfterPipelineStep {
		t.Errorf("fired[3] = %v, want HookAfterPipelineStep", fired[3])
	}
}

func TestHandoff_Hooks_Fired(t *testing.T) {
	agent1 := &mockAgentForOrch{name: "handler", output: "handled"}

	hm := NewHookManager()
	var fired []HookPoint
	hm.Register(HookBeforeHandoff, func(_ context.Context, _ *HookContext) error {
		fired = append(fired, HookBeforeHandoff)
		return nil
	})
	hm.Register(HookAfterHandoff, func(_ context.Context, _ *HookContext) error {
		fired = append(fired, HookAfterHandoff)
		return nil
	})

	handoff := NewHandoff(HandoffConfig{
		Agents: []Agent{agent1},
		Router: func(_ context.Context, _ string) int { return 0 },
	})
	handoff.SetHooks(hm)

	_, err := handoff.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Handoff.Run() error = %v", err)
	}

	if len(fired) != 2 {
		t.Fatalf("expected 2 hook firings, got %d: %v", len(fired), fired)
	}
	if fired[0] != HookBeforeHandoff {
		t.Errorf("fired[0] = %v, want HookBeforeHandoff", fired[0])
	}
	if fired[1] != HookAfterHandoff {
		t.Errorf("fired[1] = %v, want HookAfterHandoff", fired[1])
	}
}

func TestParallelRun_Hooks_Fired(t *testing.T) {
	agent1 := &mockAgentForOrch{name: "p-agent-1", output: "r1"}
	agent2 := &mockAgentForOrch{name: "p-agent-2", output: "r2"}

	hm := NewHookManager()
	var fired []HookPoint
	var mu sync.Mutex
	hm.Register(HookBeforeParallelAgent, func(_ context.Context, _ *HookContext) error {
		mu.Lock()
		fired = append(fired, HookBeforeParallelAgent)
		mu.Unlock()
		return nil
	})
	hm.Register(HookAfterParallelAgent, func(_ context.Context, _ *HookContext) error {
		mu.Lock()
		fired = append(fired, HookAfterParallelAgent)
		mu.Unlock()
		return nil
	})

	_, err := ParallelRun(context.Background(), []Agent{agent1, agent2}, "input", hm)
	if err != nil {
		t.Fatalf("ParallelRun() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(fired) != 4 {
		t.Fatalf("expected 4 hook firings, got %d: %v", len(fired), fired)
	}

	var beforeCount, afterCount int
	for _, p := range fired {
		switch p {
		case HookBeforeParallelAgent:
			beforeCount++
		case HookAfterParallelAgent:
			afterCount++
		}
	}
	if beforeCount != 2 {
		t.Errorf("beforeCount = %d, want 2", beforeCount)
	}
	if afterCount != 2 {
		t.Errorf("afterCount = %d, want 2", afterCount)
	}
}
