package agent

import (
	"context"
	"testing"
	"time"

	"agentprimordia/internal/llm"
)

// capTestMemoryStore 是 CapabilityAgent 测试专用的 MemoryStore 实现
type capTestMemoryStore struct {
	episodes []*MemoryEpisode
}

func (m *capTestMemoryStore) Add(ctx context.Context, episode *MemoryEpisode) error {
	m.episodes = append(m.episodes, episode)
	return nil
}

// capTestEventPublisher 是 CapabilityAgent 测试专用的 EventPublisher 实现
type capTestEventPublisher struct {
	events []struct {
		eventType string
		source    string
	}
}

func (m *capTestEventPublisher) PublishAsync(eventType string, source string, payload any) error {
	m.events = append(m.events, struct {
		eventType string
		source    string
	}{eventType, source})
	return nil
}

// capTestMetricsRecorder 是 CapabilityAgent 测试专用的 MetricsRecorder 实现
type capTestMetricsRecorder struct {
	llmCalls  int
	toolCalls int
	turns     int
}

func (m *capTestMetricsRecorder) RecordLLMCall(duration time.Duration, err error)                   { m.llmCalls++ }
func (m *capTestMetricsRecorder) RecordToolCall(duration time.Duration, err error)                  { m.toolCalls++ }
func (m *capTestMetricsRecorder) RecordTurn(duration time.Duration)                                 { m.turns++ }
func (m *capTestMetricsRecorder) RecordTokenUsage(model string, promptTokens, completionTokens int) {}
func (m *capTestMetricsRecorder) IncActiveAgents()                                                  {}
func (m *capTestMetricsRecorder) DecActiveAgents()                                                  {}

// ===== 测试：Capable 接口类型断言 =====

func TestCapabilityAgent_ImplementsMemoryCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test response")

	a := NewReActAgent(ReActConfig{
		Name:     "test-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	mem := &capTestMemoryStore{}
	capAgent := a.WithMemory(mem)

	// 验证 CapabilityAgent 实现了 MemoryCapable 接口
	var _ MemoryCapable = capAgent

	// 验证接口发现
	if c, ok := any(capAgent).(MemoryCapable); !ok {
		t.Error("CapabilityAgent 应实现 MemoryCapable 接口")
	} else if c.GetMemoryStore() != mem {
		t.Error("GetMemoryStore 应返回注入的 MemoryStore")
	}
}

func TestCapabilityAgent_ImplementsRAGCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test response")

	a := NewReActAgent(ReActConfig{
		Name:     "test-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	ragCfg := RAGConfig{Mode: RAGModeAuto, TopK: 3}
	capAgent := a.WithRAG(ragCfg)

	var _ RAGCapable = capAgent

	if c, ok := any(capAgent).(RAGCapable); !ok {
		t.Error("CapabilityAgent 应实现 RAGCapable 接口")
	} else if c.GetRAGConfig().TopK != 3 {
		t.Error("GetRAGConfig 应返回注入的 RAGConfig")
	}
}

func TestCapabilityAgent_ImplementsEventCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test response")

	a := NewReActAgent(ReActConfig{
		Name:     "test-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	ep := &capTestEventPublisher{}
	capAgent := a.WithEvents(ep)

	var _ EventCapable = capAgent

	if c, ok := any(capAgent).(EventCapable); !ok {
		t.Error("CapabilityAgent 应实现 EventCapable 接口")
	} else if c.GetEventPublisher() != ep {
		t.Error("GetEventPublisher 应返回注入的 EventPublisher")
	}
}

func TestCapabilityAgent_ImplementsMetricsCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test response")

	a := NewReActAgent(ReActConfig{
		Name:     "test-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	mr := &capTestMetricsRecorder{}
	capAgent := a.WithMetrics(mr)

	var _ MetricsCapable = capAgent

	if c, ok := any(capAgent).(MetricsCapable); !ok {
		t.Error("CapabilityAgent 应实现 MetricsCapable 接口")
	} else if c.GetMetricsRecorder() != mr {
		t.Error("GetMetricsRecorder 应返回注入的 MetricsRecorder")
	}
}

// ===== 测试：链式 API =====

func TestChainAPI_MultipleCapabilities(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test response")

	mem := &capTestMemoryStore{}
	ep := &capTestEventPublisher{}
	mr := &capTestMetricsRecorder{}

	capAgent := NewReActAgent(ReActConfig{
		Name:     "chain-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	}).WithMemory(mem).WithEvents(ep).WithMetrics(mr)

	// 验证已注入的能力可发现且非 nil
	if c, ok := any(capAgent).(MemoryCapable); !ok {
		t.Error("应实现 MemoryCapable")
	} else if c.GetMemoryStore() == nil {
		t.Error("MemoryStore 不应为 nil")
	}
	if c, ok := any(capAgent).(EventCapable); !ok {
		t.Error("应实现 EventCapable")
	} else if c.GetEventPublisher() == nil {
		t.Error("EventPublisher 不应为 nil")
	}
	if c, ok := any(capAgent).(MetricsCapable); !ok {
		t.Error("应实现 MetricsCapable")
	} else if c.GetMetricsRecorder() == nil {
		t.Error("MetricsRecorder 不应为 nil")
	}

	// 验证未注入的能力返回 nil（Go 接口满足性：方法存在但返回 nil）
	// 引擎通过 if c, ok := agent.(XxxCapable); ok && c.GetXxx() != nil 检测
	if c, ok := any(capAgent).(RAGCapable); !ok {
		t.Error("CapabilityAgent 应满足 RAGCapable 接口（方法存在）")
	} else if c.GetRAGConfig() != nil {
		t.Error("未注入 RAG 时 GetRAGConfig 应返回 nil")
	}
	if c, ok := any(capAgent).(HITLCapable); !ok {
		t.Error("CapabilityAgent 应满足 HITLCapable 接口（方法存在）")
	} else if c.GetHITLConfig() != nil {
		t.Error("未注入 HITL 时 GetHITLConfig 应返回 nil")
	}
}

func TestChainAPI_SyncsToInnerConfig(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test response")

	mem := &capTestMemoryStore{}
	mr := &capTestMetricsRecorder{}

	a := NewReActAgent(ReActConfig{
		Name:     "sync-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	capAgent := a.WithMemory(mem).WithMetrics(mr)

	// 验证内部 ReActAgent 的配置已同步
	if a.config.Memory != mem {
		t.Error("内部 ReActAgent 的 Memory 应已同步")
	}
	if a.config.Metrics != mr {
		t.Error("内部 ReActAgent 的 Metrics 应已同步")
	}

	// 验证通过 CapabilityAgent 也能获取
	if capAgent.GetMemoryStore() != mem {
		t.Error("GetMemoryStore 应返回注入的 MemoryStore")
	}
	if capAgent.GetMetricsRecorder() != mr {
		t.Error("GetMetricsRecorder 应返回注入的 MetricsRecorder")
	}
}

// ===== 测试：Agent 接口委托 =====

func TestCapabilityAgent_DelegatesAgentInterface(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("delegated response")

	a := NewReActAgent(ReActConfig{
		Name:     "delegate-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	mem := &capTestMemoryStore{}
	capAgent := a.WithMemory(mem)

	// 验证 Agent 接口
	var _ Agent = capAgent

	// 验证 Name 委托
	if capAgent.Name() != "delegate-agent" {
		t.Errorf("Name 应为 delegate-agent，实际为 %s", capAgent.Name())
	}

	// 验证 Run 委托
	resp, err := capAgent.Run(context.Background(), UserMessage("hello"))
	if err != nil {
		t.Errorf("Run 不应返回错误: %v", err)
	}
	if resp.Content != "delegated response" {
		t.Errorf("Response 内容应为 'delegated response'，实际为 %s", resp.Content)
	}

	// 验证记忆已保存
	if len(mem.episodes) == 0 {
		t.Error("记忆应已保存")
	}
}

func TestCapabilityAgent_InnerMethod(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test")

	a := NewReActAgent(ReActConfig{
		Name:     "inner-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	capAgent := a.WithMemory(&capTestMemoryStore{})

	// 验证 Inner 返回原始 ReActAgent
	if capAgent.Inner() != a {
		t.Error("Inner 应返回原始 ReActAgent")
	}
}

// ===== 测试：ReActAgent 未包装时不实现 Capable =====

func TestReActAgent_DoesNotImplementCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)

	a := NewReActAgent(ReActConfig{
		Name:     "bare-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	// ReActAgent 本身不实现 Capable 接口（没有 GetXxx 方法）
	// 这是协议式微内核的关键：只有通过 WithXxx 包装后才具备 Capable 接口
	if _, ok := any(a).(MemoryCapable); ok {
		t.Error("ReActAgent 不应满足 MemoryCapable 接口（无 GetMemoryStore 方法）")
	}
	if _, ok := any(a).(RAGCapable); ok {
		t.Error("ReActAgent 不应满足 RAGCapable 接口（无 GetRAGConfig 方法）")
	}
}

// ===== 测试：CapabilityAgent 上继续链式注入 =====

func TestChainAPI_ContinueOnCapabilityAgent(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test response")

	mem := &capTestMemoryStore{}
	ep := &capTestEventPublisher{}

	// 先用 ReActAgent.WithMemory 创建 CapabilityAgent
	capAgent := NewReActAgent(ReActConfig{
		Name:     "continue-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	}).WithMemory(mem)

	// 在 CapabilityAgent 上继续注入
	capAgent = capAgent.WithEvents(ep)

	// 验证两个能力都存在
	if capAgent.GetMemoryStore() != mem {
		t.Error("Memory 应保留")
	}
	if capAgent.GetEventPublisher() != ep {
		t.Error("EventPublisher 应已注入")
	}
}

// ===== 测试：ContextWindowCapable =====

func TestCapabilityAgent_ImplementsContextWindowCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test")

	a := NewReActAgent(ReActConfig{
		Name:     "ctx-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	cw := NewDefaultStrategy(50)
	capAgent := a.WithContextWindow(cw)

	var _ ContextWindowCapable = capAgent

	if c, ok := any(capAgent).(ContextWindowCapable); !ok {
		t.Error("CapabilityAgent 应实现 ContextWindowCapable")
	} else if c.GetContextWindowStrategy() != cw {
		t.Error("GetContextWindowStrategy 应返回注入的策略")
	}
}

// ===== 测试：HookCapable =====

func TestCapabilityAgent_ImplementsHookCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test")

	a := NewReActAgent(ReActConfig{
		Name:     "hook-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	hooks := NewHookManager()
	capAgent := a.WithHooks(hooks)

	var _ HookCapable = capAgent

	if c, ok := any(capAgent).(HookCapable); !ok {
		t.Error("CapabilityAgent 应实现 HookCapable")
	} else if c.GetHooks() != hooks {
		t.Error("GetHooks 应返回注入的 Hooks")
	}
}

// ===== 测试：FileScopeCapable =====

func TestCapabilityAgent_ImplementsFileScopeCapable(t *testing.T) {
	mockLLM := llm.NewMockLLM(t)
	mockLLM.WithResponse("test")

	a := NewReActAgent(ReActConfig{
		Name:     "scope-agent",
		Model:    mockLLM,
		MaxTurns: 5,
	})

	scopes := []string{"/workspace/data", "/workspace/src"}
	capAgent := a.WithFileScope(scopes)

	var _ FileScopeCapable = capAgent

	if c, ok := any(capAgent).(FileScopeCapable); !ok {
		t.Error("CapabilityAgent 应实现 FileScopeCapable")
	} else {
		fs := c.GetFileScope()
		if len(fs) != 2 || fs[0] != "/workspace/data" {
			t.Errorf("GetFileScope 应返回注入的作用域，实际: %v", fs)
		}
	}
}
