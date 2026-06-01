package persist

import (
	"testing"
	"time"
)

func TestCheckpoint_AgentState_JSONRoundtrip(t *testing.T) {
	state := &AgentState{
		AgentID:   "agent-1",
		SessionID: "session-1",
		Status:    "running",
		Messages: []CheckpointMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
		TurnCount: 3,
		Metrics: CheckpointMetrics{
			TotalTurns: 3,
			TotalTools: 2,
			Duration:   "5s",
		},
		SavedAt: time.Now().Truncate(time.Second),
	}

	data, err := state.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	restored, err := UnmarshalAgentState(data)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if restored.AgentID != state.AgentID {
		t.Errorf("AgentID = %q, want %q", restored.AgentID, state.AgentID)
	}
	if restored.SessionID != state.SessionID {
		t.Errorf("SessionID = %q, want %q", restored.SessionID, state.SessionID)
	}
	if restored.Status != state.Status {
		t.Errorf("Status = %q, want %q", restored.Status, state.Status)
	}
	if len(restored.Messages) != len(state.Messages) {
		t.Errorf("Messages length = %d, want %d", len(restored.Messages), len(state.Messages))
	}
	if restored.TurnCount != state.TurnCount {
		t.Errorf("TurnCount = %d, want %d", restored.TurnCount, state.TurnCount)
	}
	if restored.Metrics.TotalTurns != state.Metrics.TotalTurns {
		t.Errorf("Metrics.TotalTurns = %d, want %d", restored.Metrics.TotalTurns, state.Metrics.TotalTurns)
	}
}

func TestCheckpoint_AgentState_DefaultValues(t *testing.T) {
	state := &AgentState{}

	data, err := state.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	restored, err := UnmarshalAgentState(data)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if restored.AgentID != "" {
		t.Errorf("AgentID = %q, want empty", restored.AgentID)
	}
	if restored.Messages != nil {
		t.Errorf("Messages = %v, want nil", restored.Messages)
	}
	if restored.TurnCount != 0 {
		t.Errorf("TurnCount = %d, want 0", restored.TurnCount)
	}
}

func TestCheckpoint_Unmarshal_InvalidJSON(t *testing.T) {
	_, err := UnmarshalAgentState([]byte("invalid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
