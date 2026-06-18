package orchestration

import (
	"context"
	"testing"

	"agentprimordia/cmd/example/demo"
	"agentprimordia/internal/agent"
)

func TestDefaultStepExecutor_ExecutesAgent(t *testing.T) {
	ag := agent.NewReActAgent(agent.ReActConfig{
		Name:         "Step-Agent",
		SystemPrompt: "你是测试助手",
		Model:        demo.NewDemoLLM("hello"),
		MaxTurns:     1,
	})

	step := &AgentStep{ID: "s1", Name: "s1", Agent: ag, Prompt: "say hello"}
	exec := NewDefaultStepExecutor(nil)
	result := exec.Execute(context.Background(), step, nil)

	if result.Status != StepCompleted {
		t.Errorf("expected completed, got %s, error: %v", result.Status, result.Error)
	}
	if result.Output["content"] != "hello" {
		t.Errorf("unexpected output: %v", result.Output)
	}
}
