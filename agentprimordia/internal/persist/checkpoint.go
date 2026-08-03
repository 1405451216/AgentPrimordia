package persist

import (
	"context"
	"encoding/json"
	"time"
)

// CheckpointMessage 是检查点中的消息表示（独立于 agent.Message 避免循环依赖）
type CheckpointMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	AgentID   string              `json:"agent_id"`
	SessionID string              `json:"session_id"`
	Status    string              `json:"status"`
	Messages  []CheckpointMessage `json:"messages"`
	TurnCount int                 `json:"turn_count"`
	Metrics   CheckpointMetrics   `json:"metrics"`
	// Plan 保存 Plan（executePlan）执行的中间状态；非空表示从计划断点恢复。
	Plan    *CheckpointPlan `json:"plan,omitempty"`
	SavedAt time.Time       `json:"saved_at"`
}

// CheckpointPlan Plan 执行的持久化进度，用于断点续跑整个计划。
type CheckpointPlan struct {
	Subtasks     []CheckpointSubTask `json:"subtasks"`
	Completed    []string            `json:"completed"`
	Results      map[string]string   `json:"results"`
	TotalTools   int                 `json:"total_tools"`
	LLMLatencyNs int64               `json:"llm_latency_ns"`
	ToolLatencyNs int64              `json:"tool_latency_ns"`
}

// CheckpointSubTask Plan 子任务的持久化表示。
type CheckpointSubTask struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on"`
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
