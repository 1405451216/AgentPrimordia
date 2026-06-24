//go:build ignore

package agent

import (
	"context"
	"testing"
)

func TestPipeline_ConditionalStep_Skip(t *testing.T) {
	agent1 := &mockAgentForOrch{name: "step1", output: "out1"}
	agent2 := &mockAgentForOrch{name: "step2", output: "out2"}
	agent3 := &mockAgentForOrch{name: "step3", output: "out3"}

	pipeline := NewPipeline(
		PipelineStep{Name: "step1", Agent: agent1},
		PipelineStep{
			Name:  "step2",
			Agent: agent2,
			Condition: func(_ context.Context, _ *StepResult) bool {
				return false
			},
		},
		PipelineStep{Name: "step3", Agent: agent3},
	)

	result, err := pipeline.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Pipeline.Run() error = %v", err)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Steps))
	}
	if !result.Steps[1].Skipped {
		t.Error("step2 should be skipped")
	}
	if result.Steps[1].Output != "" {
		t.Errorf("skipped step output should be empty, got %q", result.Steps[1].Output)
	}
	if result.Steps[0].Skipped {
		t.Error("step1 should not be skipped")
	}
	if result.Steps[2].Skipped {
		t.Error("step3 should not be skipped")
	}
	if result.Final != "out3" {
		t.Errorf("expected final output 'out3', got %q", result.Final)
	}
}

func TestPipeline_ConditionalStep_Execute(t *testing.T) {
	agent1 := &mockAgentForOrch{name: "step1", output: "out1"}
	agent2 := &mockAgentForOrch{name: "step2", output: "out2"}

	pipeline := NewPipeline(
		PipelineStep{Name: "step1", Agent: agent1},
		PipelineStep{
			Name:  "step2",
			Agent: agent2,
			Condition: func(_ context.Context, _ *StepResult) bool {
				return true
			},
		},
	)

	result, err := pipeline.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Pipeline.Run() error = %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[1].Skipped {
		t.Error("step2 should not be skipped when Condition returns true")
	}
	if result.Steps[1].Output != "out2" {
		t.Errorf("step2 output = %q, want 'out2'", result.Steps[1].Output)
	}
	if result.Final != "out2" {
		t.Errorf("expected final output 'out2', got %q", result.Final)
	}
}

func TestPipeline_ConditionalStep_Nil(t *testing.T) {
	agent1 := &mockAgentForOrch{name: "step1", output: "out1"}
	agent2 := &mockAgentForOrch{name: "step2", output: "out2"}

	pipeline := NewPipeline(
		PipelineStep{Name: "step1", Agent: agent1, Condition: nil},
		PipelineStep{Name: "step2", Agent: agent2, Condition: nil},
	)

	result, err := pipeline.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Pipeline.Run() error = %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[0].Skipped {
		t.Error("step1 should not be skipped with nil Condition")
	}
	if result.Steps[1].Skipped {
		t.Error("step2 should not be skipped with nil Condition")
	}
	if result.Steps[0].Output != "out1" {
		t.Errorf("step1 output = %q, want 'out1'", result.Steps[0].Output)
	}
	if result.Steps[1].Output != "out2" {
		t.Errorf("step2 output = %q, want 'out2'", result.Steps[1].Output)
	}
}
