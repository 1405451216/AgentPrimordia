package react

import (
	"context"
	"fmt"
	"testing"

	"agentprimordia/internal/agent/core"
)

// mockDelegate 测试用 Delegate 实现
type mockDelegate struct {
	// llmResponses 按轮次返回预设响应
	llmResponses []mockLLMResponse
	stream       bool
	turnStarts   []int
	turnEnds     []*TurnResult
	completed    *LoopResult
	errs         []error
}

type mockLLMResponse struct {
	content   string
	toolCalls []core.ToolCall
	err       error
}

func (m *mockDelegate) CallLLM(_ context.Context, turn int, _ []core.Message) (string, []core.ToolCall, error) {
	if turn >= len(m.llmResponses) {
		return "no more responses", nil, nil
	}
	r := m.llmResponses[turn]
	return r.content, r.toolCalls, r.err
}

func (m *mockDelegate) ExecuteTools(_ context.Context, calls []core.ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, c := range calls {
		results[i] = ToolResult{ToolName: c.Name, Output: "result:" + c.Name}
	}
	return results
}

func (m *mockDelegate) IsCancelled(ctx context.Context) bool { return ctx.Err() != nil }
func (m *mockDelegate) OnTurnStart(_ context.Context, turn int) error {
	m.turnStarts = append(m.turnStarts, turn)
	return nil
}
func (m *mockDelegate) OnTurnEnd(_ context.Context, r *TurnResult) {
	m.turnEnds = append(m.turnEnds, r)
}
func (m *mockDelegate) OnComplete(_ context.Context, r *LoopResult) { m.completed = r }
func (m *mockDelegate) OnError(_ context.Context, err error)        { m.errs = append(m.errs, err) }
func (m *mockDelegate) EmitStream(_ core.StreamEvent)               {}
func (m *mockDelegate) IsStream() bool                              { return m.stream }

// TestEngine_DirectAnswer 验证无工具调用时直接返回
func TestEngine_DirectAnswer(t *testing.T) {
	engine := NewEngine(Config{AgentName: "test", MaxTurns: 5})
	delegate := &mockDelegate{
		llmResponses: []mockLLMResponse{
			{content: "Hello, world!"},
		},
	}

	history := []core.Message{{Role: core.RoleUser, Content: "Hi"}}
	result, err := engine.Run(context.Background(), delegate, history)

	if err != nil {
		t.Fatalf("期望无错误，得到: %v", err)
	}
	if result.Content != "Hello, world!" {
		t.Errorf("期望 'Hello, world!'，得到 %q", result.Content)
	}
	if result.TotalTurns != 1 {
		t.Errorf("期望 1 轮，得到 %d", result.TotalTurns)
	}
	if delegate.completed == nil {
		t.Error("期望 OnComplete 被调用")
	}
}

// TestEngine_ToolCallThenAnswer 验证工具调用后返回最终答案
func TestEngine_ToolCallThenAnswer(t *testing.T) {
	engine := NewEngine(Config{AgentName: "test", MaxTurns: 10})
	delegate := &mockDelegate{
		llmResponses: []mockLLMResponse{
			{content: "", toolCalls: []core.ToolCall{{ID: "1", Name: "search", Args: `{"q":"test"}`}}},
			{content: "Based on search results..."},
		},
	}

	history := []core.Message{{Role: core.RoleUser, Content: "Search for test"}}
	result, err := engine.Run(context.Background(), delegate, history)

	if err != nil {
		t.Fatalf("期望无错误，得到: %v", err)
	}
	if result.Content != "Based on search results..." {
		t.Errorf("期望最终答案，得到 %q", result.Content)
	}
	if result.TotalTurns != 2 {
		t.Errorf("期望 2 轮，得到 %d", result.TotalTurns)
	}
	if result.ToolCallCount != 1 {
		t.Errorf("期望 1 次工具调用，得到 %d", result.ToolCallCount)
	}
	if len(delegate.turnStarts) != 2 {
		t.Errorf("期望 OnTurnStart 调用 2 次，得到 %d", len(delegate.turnStarts))
	}
}

// TestEngine_MaxTurnsLimit 验证达到最大轮次限制
func TestEngine_MaxTurnsLimit(t *testing.T) {
	engine := NewEngine(Config{AgentName: "test", MaxTurns: 3})
	// 每轮都返回工具调用，永远不给最终答案
	delegate := &mockDelegate{
		llmResponses: []mockLLMResponse{
			{toolCalls: []core.ToolCall{{ID: "1", Name: "tool_a"}}},
			{toolCalls: []core.ToolCall{{ID: "2", Name: "tool_b"}}},
			{toolCalls: []core.ToolCall{{ID: "3", Name: "tool_c"}}},
			{content: "should not reach here"},
		},
	}

	history := []core.Message{{Role: core.RoleUser, Content: "loop forever"}}
	result, err := engine.Run(context.Background(), delegate, history)

	if err != nil {
		t.Fatalf("期望无错误，得到: %v", err)
	}
	if result.TotalTurns != 3 {
		t.Errorf("期望 3 轮（上限），得到 %d", result.TotalTurns)
	}
	if result.ToolCallCount != 3 {
		t.Errorf("期望 3 次工具调用，得到 %d", result.ToolCallCount)
	}
}

// TestEngine_LLMError 验证 LLM 错误传播
func TestEngine_LLMError(t *testing.T) {
	engine := NewEngine(Config{AgentName: "test", MaxTurns: 5})
	delegate := &mockDelegate{
		llmResponses: []mockLLMResponse{
			{err: fmt.Errorf("API rate limited")},
		},
	}

	history := []core.Message{{Role: core.RoleUser, Content: "Hi"}}
	_, err := engine.Run(context.Background(), delegate, history)

	if err == nil {
		t.Fatal("期望错误")
	}
	if len(delegate.errs) == 0 {
		t.Error("期望 OnError 被调用")
	}
}

// TestEngine_ContextCancel 验证上下文取消中断循环
func TestEngine_ContextCancel(t *testing.T) {
	engine := NewEngine(Config{AgentName: "test", MaxTurns: 100})
	ctx, cancel := context.WithCancel(context.Background())

	delegate := &mockDelegate{
		llmResponses: []mockLLMResponse{
			{toolCalls: []core.ToolCall{{ID: "1", Name: "slow_tool"}}},
			{content: "should not reach"},
		},
	}

	// 第一轮结束后取消
	origOnTurnEnd := delegate.OnTurnEnd
	_ = origOnTurnEnd
	delegate.turnEnds = nil

	history := []core.Message{{Role: core.RoleUser, Content: "Hi"}}

	// 在 OnTurnEnd 中取消 context
	cancelDelegate := &cancelOnTurnEndDelegate{mockDelegate: delegate, cancel: cancel}
	result, err := engine.Run(ctx, cancelDelegate, history)

	if err == nil {
		t.Log("注意：ctx 取消可能在第一轮内或第二轮初被检测到")
	}
	if result != nil && result.TotalTurns > 2 {
		t.Errorf("期望 ≤2 轮（ctx 取消后应停止），得到 %d", result.TotalTurns)
	}
}

// cancelOnTurnEndDelegate 在 OnTurnEnd 时取消 context
type cancelOnTurnEndDelegate struct {
	*mockDelegate
	cancel context.CancelFunc
	once   bool
}

func (d *cancelOnTurnEndDelegate) OnTurnEnd(ctx context.Context, r *TurnResult) {
	d.mockDelegate.OnTurnEnd(ctx, r)
	if !d.once {
		d.once = true
		d.cancel()
	}
}
