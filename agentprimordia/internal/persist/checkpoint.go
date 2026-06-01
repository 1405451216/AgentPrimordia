package persist

import (
	"context"
	"encoding/json"
	"time"
)

// CheckpointMessage 是检查点中的消息表示（独立于 agent.Message 避免循环依赖）
type CheckpointMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
}

// CheckpointMetrics 是检查点中的指标表示
type CheckpointMetrics struct {
	TotalTurns  int    `json:"total_turns"`
	TotalTools  int    `json:"total_tools_called"`
	Duration    string `json:"duration"`
	LLMLatency  string `json:"llm_latency_ms"`
	ToolLatency string `json:"tool_latency_ms"`
}

type AgentState struct {
	AgentID   string             `json:"agent_id"`
	SessionID string             `json:"session_id"`
	Status    string             `json:"status"`
	Messages  []CheckpointMessage `json:"messages"`
	TurnCount int                `json:"turn_count"`
	Metrics   CheckpointMetrics  `json:"metrics"`
	SavedAt   time.Time          `json:"saved_at"`
}

func (s *AgentState) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

func UnmarshalAgentState(data []byte) (*AgentState, error) {
	var state AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

type CheckpointStore interface {
	Save(ctx context.Context, state *AgentState) error
	Load(ctx context.Context, agentID string) (*AgentState, error)
	List(ctx context.Context, sessionID string) ([]*AgentState, error)
	Delete(ctx context.Context, agentID string) error
}
