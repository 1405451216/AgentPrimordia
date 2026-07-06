package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// memoryAuditLogger 测试用内存审计 logger
type memoryAuditLogger struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (m *memoryAuditLogger) Log(_ context.Context, event AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *memoryAuditLogger) Events() []AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]AuditEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

func (m *memoryAuditLogger) CountByAction(action string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.events {
		if e.Action == action {
			n++
		}
	}
	return n
}

// TestReActLoop_AuditAgentStartStop 验证 AgentStart/AgentStop 审计事件被写入
func TestReActLoop_AuditAgentStartStop(t *testing.T) {
	mockProvider := &outputGuardMockProvider{content: "完成"}
	audit := &memoryAuditLogger{}

	agent := newReActAgent(ReActConfig{
		Name:     "audit-test",
		Model:    mockProvider,
		MaxTurns: 1,
	}).WithAuditLogger(audit)

	_, err := agent.Run(context.Background(), Message{Role: RoleUser, Content: "测试"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if n := audit.CountByAction(auditActionAgentStart); n != 1 {
		t.Errorf("AgentStart 事件数 = %d, want 1", n)
	}
	if n := audit.CountByAction(auditActionAgentStop); n != 1 {
		t.Errorf("AgentStop 事件数 = %d, want 1", n)
	}
}

// TestReActLoop_AuditLLMCall 验证 LLM 调用审计事件
func TestReActLoop_AuditLLMCall(t *testing.T) {
	mockProvider := &outputGuardMockProvider{content: "完成"}
	audit := &memoryAuditLogger{}

	agent := newReActAgent(ReActConfig{
		Name:     "audit-llm",
		Model:    mockProvider,
		MaxTurns: 1,
	}).WithAuditLogger(audit)

	_, err := agent.Run(context.Background(), Message{Role: RoleUser, Content: "测试"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if n := audit.CountByAction(auditActionLLMCall); n != 1 {
		t.Errorf("LLMCall 事件数 = %d, want 1", n)
	}
	// 验证事件内容
	for _, e := range audit.Events() {
		if e.Action == auditActionLLMCall {
			if e.Result != auditResultSuccess {
				t.Errorf("LLMCall Result = %q, want success", e.Result)
			}
			if e.Actor != "audit-llm" {
				t.Errorf("Actor = %q, want audit-llm", e.Actor)
			}
			if e.Details == nil {
				t.Error("Details 不应为空")
			}
			if _, ok := e.Details["turn"]; !ok {
				t.Error("Details 应包含 turn 字段")
			}
		}
	}
}

// TestReActLoop_AuditGuardrailSanitize 验证 GuardrailSanitize 审计事件
func TestReActLoop_AuditGuardrailSanitize(t *testing.T) {
	mockProvider := &outputGuardMockProvider{content: "邮箱 zhangsan@example.com"}
	audit := &memoryAuditLogger{}

	guard := OutputGuard(func(content string) (string, bool, error) {
		sanitized := "邮箱 [REDACTED]"
		return sanitized, false, nil
	})

	agent := newReActAgent(ReActConfig{
		Name:     "audit-sanitize",
		Model:    mockProvider,
		MaxTurns: 1,
	}).WithOutputGuard(guard).WithAuditLogger(audit)

	_, err := agent.Run(context.Background(), Message{Role: RoleUser, Content: "测试"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if n := audit.CountByAction(auditActionGuardrailSanitize); n != 1 {
		t.Errorf("GuardrailSanitize 事件数 = %d, want 1", n)
	}
}

// TestReActLoop_AuditGuardrailBlock 验证 GuardrailBlock 审计事件
func TestReActLoop_AuditGuardrailBlock(t *testing.T) {
	mockProvider := &outputGuardMockProvider{content: "正常内容"}
	audit := &memoryAuditLogger{}

	guard := OutputGuard(func(_ string) (string, bool, error) {
		return "", true, nil // 拒绝
	})

	agent := newReActAgent(ReActConfig{
		Name:     "audit-block",
		Model:    mockProvider,
		MaxTurns: 1,
	}).WithOutputGuard(guard).WithAuditLogger(audit)

	_, err := agent.Run(context.Background(), Message{Role: RoleUser, Content: "测试"})
	if err == nil {
		t.Error("期望返回 blocked 错误")
	}

	if n := audit.CountByAction(auditActionGuardrailBlock); n != 1 {
		t.Errorf("GuardrailBlock 事件数 = %d, want 1", n)
	}
}

// TestReActLoop_NoAuditLogger 验证未配置 auditLogger 时正常运行
func TestReActLoop_NoAuditLogger(t *testing.T) {
	mockProvider := &outputGuardMockProvider{content: "完成"}

	agent := newReActAgent(ReActConfig{
		Name:     "no-audit",
		Model:    mockProvider,
		MaxTurns: 1,
	})

	resp, err := agent.Run(context.Background(), Message{Role: RoleUser, Content: "测试"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if resp.Content != "完成" {
		t.Errorf("Content = %q, want 完成", resp.Content)
	}
}

// TestReActLoop_AuditLoggerError 验证 auditLogger 出错时不影响主流程
func TestReActLoop_AuditLoggerError(t *testing.T) {
	mockProvider := &outputGuardMockProvider{content: "完成"}

	failingLogger := failingAuditLogger{}
	agent := newReActAgent(ReActConfig{
		Name:     "failing-audit",
		Model:    mockProvider,
		MaxTurns: 1,
	}).WithAuditLogger(failingLogger)

	// 即使 audit logger 失败，主流程仍应正常返回
	resp, err := agent.Run(context.Background(), Message{Role: RoleUser, Content: "测试"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if resp.Content != "完成" {
		t.Errorf("Content = %q, want 完成", resp.Content)
	}
}

type failingAuditLogger struct{}

func (failingAuditLogger) Log(_ context.Context, _ AuditEvent) error {
	return errors.New("audit write failed")
}
