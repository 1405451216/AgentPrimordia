package ap_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/llm"
	ap "agentprimordia/pkg"
)

// ===== MemoryAdapter 测试 =====

func TestMemoryAdapter_Add(t *testing.T) {
	store, err := ap.WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory failed: %v", err)
	}
	defer store.Close()

	adapter := ap.NewMemoryAdapter(store)

	ep := &agent.MemoryEpisode{
		ID:        "ep-test-1",
		SessionID: "session-1",
		Role:      "user",
		Content:   "Hello, world!",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := adapter.Add(context.Background(), ep); err != nil {
		t.Fatalf("MemoryAdapter.Add() error = %v", err)
	}

	// 验证数据写入了底层 store
	count, err := store.Count(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("store.Count() error = %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 episode, got %d", count)
	}
}

func TestMemoryAdapter_AddMultiple(t *testing.T) {
	store, err := ap.WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory failed: %v", err)
	}
	defer store.Close()

	adapter := ap.NewMemoryAdapter(store)

	for i := 0; i < 5; i++ {
		ep := &agent.MemoryEpisode{
			ID:        "ep-test-" + string(rune('0'+i)),
			SessionID: "session-1",
			Role:      "user",
			Content:   "message",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := adapter.Add(context.Background(), ep); err != nil {
			t.Fatalf("MemoryAdapter.Add() error = %v", err)
		}
	}

	count, _ := store.Count(context.Background(), "session-1")
	if count != 5 {
		t.Errorf("expected 5 episodes, got %d", count)
	}
}

// ===== EventBusAdapter 测试 =====

func TestEventBusAdapter_PublishAsync(t *testing.T) {
	bus := ap.NewBus(16)
	defer bus.Close()

	adapter := ap.NewEventBusAdapter(bus)

	ch, subID := bus.Subscribe("agent.start")
	defer bus.Unsubscribe(subID)

	if err := adapter.PublishAsync("agent.start", "test-agent", map[string]string{"key": "value"}); err != nil {
		t.Fatalf("EventBusAdapter.PublishAsync() error = %v", err)
	}

	select {
	case evt := <-ch:
		if evt.Source != "test-agent" {
			t.Errorf("expected source 'test-agent', got %q", evt.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBusAdapter_PublishAsync_ClosedBus(t *testing.T) {
	bus := ap.NewBus(16)
	bus.Close()

	adapter := ap.NewEventBusAdapter(bus)

	err := adapter.PublishAsync("agent.start", "test", nil)
	if err == nil {
		t.Error("expected error when publishing to closed bus")
	}
}

// ===== MetricsAdapter 测试 =====

func TestMetricsAdapter_RecordLLMCall(t *testing.T) {
	m := ap.NewMetrics()
	adapter := ap.NewMetricsAdapter(m)

	adapter.RecordLLMCall(100*time.Millisecond, nil)
	adapter.RecordLLMCall(200*time.Millisecond, errors.New("timeout"))

	snap := m.Snapshot()
	if snap.LLMTotalCalls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", snap.LLMTotalCalls)
	}
	if snap.LLMTotalErrors != 1 {
		t.Errorf("expected 1 LLM error, got %d", snap.LLMTotalErrors)
	}
}

func TestMetricsAdapter_RecordToolCall(t *testing.T) {
	m := ap.NewMetrics()
	adapter := ap.NewMetricsAdapter(m)

	adapter.RecordToolCall(50*time.Millisecond, nil)

	snap := m.Snapshot()
	if snap.ToolTotalCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", snap.ToolTotalCalls)
	}
}

func TestMetricsAdapter_RecordTurn(t *testing.T) {
	m := ap.NewMetrics()
	adapter := ap.NewMetricsAdapter(m)

	adapter.RecordTurn(1 * time.Second)

	snap := m.Snapshot()
	if snap.TotalTurns != 1 {
		t.Errorf("expected 1 turn, got %d", snap.TotalTurns)
	}
}

func TestMetricsAdapter_ActiveAgents(t *testing.T) {
	m := ap.NewMetrics()
	adapter := ap.NewMetricsAdapter(m)

	adapter.IncActiveAgents()
	adapter.IncActiveAgents()
	snap := m.Snapshot()
	if snap.ActiveAgents != 2 {
		t.Errorf("expected 2 active agents, got %d", snap.ActiveAgents)
	}

	adapter.DecActiveAgents()
	snap = m.Snapshot()
	if snap.ActiveAgents != 1 {
		t.Errorf("expected 1 active agent, got %d", snap.ActiveAgents)
	}
}

// ===== 全集成测试 =====

func TestAgentIntegration_WithMemoryEventBusMetrics(t *testing.T) {
	// 创建 Memory
	memStore, err := ap.WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory failed: %v", err)
	}
	defer memStore.Close()

	// 创建 EventBus
	bus := ap.NewBus(64)
	defer bus.Close()

	// 订阅所有事件
	eventCh, subID := bus.SubscribeAll()
	defer bus.Unsubscribe(subID)

	// 创建 Metrics
	m := ap.NewMetrics()

	// 创建 Mock LLM（简单响应）
	mockLLM := &integrationMockLLM{response: "I can help you with that!"}

	// 创建 Agent，集成所有模块
	a := ap.NewReActAgent(ap.ReActConfig{
		Name:           "IntegrationAgent",
		SystemPrompt:   "You are a helpful assistant.",
		Model:          mockLLM,
		Toolkit:        ap.NewToolRegistry(),
		Memory:         ap.NewMemoryAdapter(memStore),
		EventPublisher: ap.NewEventBusAdapter(bus),
		Metrics:        ap.NewMetricsAdapter(m),
		MaxTurns:       10,
	})

	// 运行 Agent
	resp, err := a.Run(context.Background(), ap.UserMessage("Hello!"))
	if err != nil {
		t.Fatalf("Agent.Run() error = %v", err)
	}

	// 验证响应
	if resp.Content != "I can help you with that!" {
		t.Errorf("unexpected content: %s", resp.Content)
	}

	// 验证 Memory 写入
	count, _ := memStore.Count(context.Background(), "")
	if count < 1 { // 至少有 user 或 assistant 消息
		t.Errorf("expected at least 1 memory episode, got %d", count)
	}

	// 验证 EventBus 收到事件
	receivedEvents := []ap.Event{}
	timeout := time.After(2 * time.Second)
	for len(receivedEvents) < 2 {
		select {
		case evt := <-eventCh:
			receivedEvents = append(receivedEvents, evt)
		case <-timeout:
			goto done
		}
	}
done:
	if len(receivedEvents) == 0 {
		t.Error("expected at least 1 event from EventBus")
	}

	// 验证 Metrics 记录
	snap := m.Snapshot()
	if snap.LLMTotalCalls == 0 {
		t.Error("expected at least 1 LLM call in metrics")
	}
	if snap.ActiveAgents != 0 {
		t.Errorf("expected 0 active agents after completion, got %d", snap.ActiveAgents)
	}
}

func TestAgentIntegration_WithCheckpoint(t *testing.T) {
	// 创建 Checkpoint Store
	cpStore, err := ap.InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore failed: %v", err)
	}
	defer cpStore.Close()

	// 创建 Mock LLM
	mockLLM := &integrationMockLLM{response: "Task completed!"}

	// 创建 Agent with Checkpoint
	a := ap.NewReActAgent(ap.ReActConfig{
		Name:            "CheckpointAgent",
		Model:           mockLLM,
		Toolkit:         ap.NewToolRegistry(),
		CheckpointStore: cpStore,
		SessionID:       "session-checkpoint-1",
		MaxTurns:        10,
	})

	resp, err := a.Run(context.Background(), ap.UserMessage("Do something"))
	if err != nil {
		t.Fatalf("Agent.Run() error = %v", err)
	}

	// 验证 Checkpoint 已保存
	state, err := cpStore.Load(context.Background(), "CheckpointAgent")
	if err != nil {
		t.Fatalf("CheckpointStore.Load() error = %v", err)
	}
	if state.Status != "completed" {
		t.Errorf("expected checkpoint status 'completed', got %q", state.Status)
	}
	if state.SessionID != "session-checkpoint-1" {
		t.Errorf("expected session ID 'session-checkpoint-1', got %q", state.SessionID)
	}
	if state.TurnCount != 1 {
		t.Errorf("expected 1 turn in checkpoint, got %d", state.TurnCount)
	}
	if len(state.Messages) < 1 {
		t.Errorf("expected at least 1 message in checkpoint, got %d", len(state.Messages))
	}
	_ = resp
}

func TestAgentIntegration_WithContextWindow(t *testing.T) {
	mockLLM := &integrationMockLLM{response: "Summarized!"}

	strategy := ap.NewDefaultStrategy(3) // 只保留最后3条

	a := ap.NewReActAgent(ap.ReActConfig{
		Name:          "ContextWindowAgent",
		Model:         mockLLM,
		Toolkit:       ap.NewToolRegistry(),
		ContextWindow: strategy,
		MaxTurns:      10,
	})

	resp, err := a.Run(context.Background(), ap.UserMessage("Test"))
	if err != nil {
		t.Fatalf("Agent.Run() error = %v", err)
	}
	if resp.Content != "Summarized!" {
		t.Errorf("unexpected content: %s", resp.Content)
	}
}

func TestAgentIntegration_Stop(t *testing.T) {
	mockLLM := &integrationMockLLM{response: "Running..."}

	a := ap.NewReActAgent(ap.ReActConfig{
		Name:     "StopAgent",
		Model:    mockLLM,
		Toolkit:  ap.NewToolRegistry(),
		MaxTurns: 10,
	})

	// 在另一个 goroutine 中停止
	go func() {
		time.Sleep(100 * time.Millisecond)
		a.Stop()
	}()

	// 即使被停止，Run 也应该返回（不 panic）
	_, _ = a.Run(context.Background(), ap.UserMessage("test"))
}

// ===== 辅助 Mock LLM =====

type integrationMockLLM struct {
	response string
}

func (m *integrationMockLLM) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		ID:      "mock-integration-id",
		Content: m.response,
		Role:    "assistant",
		Usage:   llm.Usage{PromptTokens: 10, CompletionTokens: 20},
	}, nil
}

func (m *integrationMockLLM) Stream(ctx context.Context, req *llm.CompletionRequest) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk, 1)
	go func() {
		defer close(ch)
		ch <- llm.Chunk{Content: m.response, Done: true}
	}()
	return ch, nil
}

func (m *integrationMockLLM) CallTools(ctx context.Context, req *llm.ToolCallRequest) (*llm.ToolCallResponse, error) {
	return &llm.ToolCallResponse{
		Content:   m.response,
		ToolCalls: []llm.FunctionCall{},
		Usage:     llm.Usage{},
	}, nil
}

func (m *integrationMockLLM) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (m *integrationMockLLM) Info() llm.ModelInfo {
	return llm.ModelInfo{
		Name:              "integration-mock",
		Provider:          "mock",
		MaxContext:        4096,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}
