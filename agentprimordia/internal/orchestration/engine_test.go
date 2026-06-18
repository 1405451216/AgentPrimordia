package orchestration

import (
	"context"
	"testing"

	"agentprimordia/cmd/example/demo"
	"agentprimordia/internal/agent"
)

func TestExecutionEngine_Parallel(t *testing.T) {
	ag := agent.NewReActAgent(agent.ReActConfig{
		Name:         "Test-Agent",
		SystemPrompt: "你是测试助手",
		Model:        demo.NewDemoLLM("ok"),
		MaxTurns:     1,
	})

	steps := []*AgentStep{
		{ID: "p1", Name: "p1", Agent: ag, Prompt: "go"},
		{ID: "p2", Name: "p2", Agent: ag, Prompt: "go"},
		{ID: "p3", Name: "p3", Agent: ag, Prompt: "go"},
	}

	engine := NewExecutionEngine(ExecutionEngineConfig{MaxConcurrency: 2})
	result, err := engine.Run(context.Background(), ParallelMode, steps, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	if len(result.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(result.Steps))
	}
}
