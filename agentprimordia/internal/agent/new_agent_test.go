package agent

import (
	"context"
	"testing"

	"agentprimordia/internal/llm"
)

func TestNewAgent_Basic(t *testing.T) {
	mock := llm.NewMockLLM(t).WithResponse("Hello, I am a test agent.")

	agent := NewAgent("test-bot", "you are helpful", mock, WithMaxTurns(5))

	if agent == nil {
		t.Fatal("NewAgent returned nil")
	}

	// 验证底层 ReActAgent 配置
	inner := agent.inner
	if inner.config.Name != "test-bot" {
		t.Errorf("name = %q, want %q", inner.config.Name, "test-bot")
	}
	if inner.config.SystemPrompt != "you are helpful" {
		t.Errorf("systemPrompt = %q, want %q", inner.config.SystemPrompt, "you are helpful")
	}
	if inner.config.MaxTurns != 5 {
		t.Errorf("maxTurns = %d, want 5", inner.config.MaxTurns)
	}
}

func TestNewAgent_WithAllOptions(t *testing.T) {
	mock := llm.NewMockLLM(t).WithResponse("ok")

	agent := NewAgent("opt-bot", "be helpful", mock,
		WithMaxTurns(20),
		WithTemperature(0.5),
		WithSessionID("session-42"),
	)

	inner := agent.inner
	if inner.config.MaxTurns != 20 {
		t.Errorf("maxTurns = %d, want 20", inner.config.MaxTurns)
	}
	if inner.config.Temperature != 0.5 {
		t.Errorf("temperature = %f, want 0.5", inner.config.Temperature)
	}
	if inner.config.SessionID != "session-42" {
		t.Errorf("sessionID = %q, want %q", inner.config.SessionID, "session-42")
	}
}

func TestNewAgent_Defaults(t *testing.T) {
	mock := llm.NewMockLLM(t).WithResponse("ok")

	agent := NewAgent("default-bot", "be helpful", mock)

	inner := agent.inner
	if inner.config.MaxTurns != 50 {
		t.Errorf("maxTurns = %d, want 50 (default applied by NewReActAgent)", inner.config.MaxTurns)
	}
}

func TestNewAgent_CanRun(t *testing.T) {
	mock := llm.NewMockLLM(t).WithResponse("Hello from test agent!")

	agent := NewAgent("run-bot", "you are helpful", mock, WithMaxTurns(3))

	resp, err := agent.Run(context.Background(), UserMessage("hi"))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if resp.Content != "Hello from test agent!" {
		t.Errorf("content = %q, want %q", resp.Content, "Hello from test agent!")
	}
}

func TestNewAgent_ChainAPI(t *testing.T) {
	mock := llm.NewMockLLM(t).WithResponse("ok")

	// 验证 NewAgent 返回的是 CapabilityAgent，可链式调用
	agent := NewAgent("chain-bot", "be helpful", mock)

	// 链式调用不应 panic
	agent.WithMemory(nil)
	agent.WithHooks(nil)

	// 再次链式调用
	agent2 := agent.WithTracer(NewNoopTracer())
	if agent2 == nil {
		t.Fatal("chain API returned nil")
	}
}

func TestNewAgent_EquivalentToNewReActAgent(t *testing.T) {
	mock1 := llm.NewMockLLM(t).WithResponse("response 1")
	mock2 := llm.NewMockLLM(t).WithResponse("response 2")

	// 方式 1：NewAgent
	agent1 := NewAgent("equivalent", "prompt", mock1, WithMaxTurns(7))

	// 方式 2：NewReActAgent（旧方式）
	agent2 := NewReActAgent(ReActConfig{
		Name:         "equivalent",
		SystemPrompt: "prompt",
		Model:        mock2,
		MaxTurns:     7,
	})

	resp1, err1 := agent1.Run(context.Background(), UserMessage("hi"))
	if err1 != nil {
		t.Fatalf("NewAgent Run failed: %v", err1)
	}

	resp2, err2 := agent2.Run(context.Background(), UserMessage("hi"))
	if err2 != nil {
		t.Fatalf("NewReActAgent Run failed: %v", err2)
	}

	if resp1.Content != "response 1" {
		t.Errorf("NewAgent response = %q", resp1.Content)
	}
	if resp2.Content != "response 2" {
		t.Errorf("NewReActAgent response = %q", resp2.Content)
	}

	// 验证两种方式创建的 agent 都有正确的配置
	if agent1.inner.config.MaxTurns != agent2.config.MaxTurns {
		t.Errorf("MaxTurns mismatch: %d vs %d", agent1.inner.config.MaxTurns, agent2.config.MaxTurns)
	}
}
