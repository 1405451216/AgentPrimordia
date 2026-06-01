package agent

import (
	"testing"

	"agentprimordia/internal/llm"
)

func TestAgentInterface_ReActAgent_Implements(t *testing.T) {
	var _ Agent = (*ReActAgent)(nil)
}

func TestAgentInterface_Name(t *testing.T) {
	cfg := ReActConfig{
		Name:  "test-agent",
		Model: llm.NewMockLLM(t),
	}
	agt := NewReActAgent(cfg)

	if agt.Name() != "test-agent" {
		t.Errorf("Name() = %q, want %q", agt.Name(), "test-agent")
	}
}

func TestAgentInterface_Stop(t *testing.T) {
	cfg := ReActConfig{
		Name:  "stop-test",
		Model: llm.NewMockLLM(t),
	}
	agt := NewReActAgent(cfg)

	agt.Stop()

	if !agt.lifecycle.IsStopped() {
		t.Error("after Stop(), lifecycle.IsStopped() = false, want true")
	}
}

func TestAgentInterface_Stats(t *testing.T) {
	cfg := ReActConfig{
		Name:  "stats-test",
		Model: llm.NewMockLLM(t),
	}
	agt := NewReActAgent(cfg)

	stats := agt.Stats()
	if stats.Status != StatusIdle {
		t.Errorf("Stats().Status = %q, want %q", stats.Status, StatusIdle)
	}
}
