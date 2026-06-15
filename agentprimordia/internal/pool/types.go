package pool

import (
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/tools"
)

type PoolTaskStatus string

const (
	PoolTaskQueued    PoolTaskStatus = "queued"
	PoolTaskRunning   PoolTaskStatus = "running"
	PoolTaskCompleted PoolTaskStatus = "completed"
	PoolTaskFailed    PoolTaskStatus = "failed"
	PoolTaskCancelled PoolTaskStatus = "cancelled"
)

type TaskConfig struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Prompt     string            `json:"prompt"`
	SessionID  string            `json:"session_id,omitempty"`
	Tools      []tools.Tool      `json:"tools,omitempty"`
	FilesScope []string          `json:"files_scope,omitempty"`
	MaxTurns   int               `json:"max_turns,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type TaskResult struct {
	TaskID   string          `json:"task_id"`
	Task     TaskConfig      `json:"task"`
	Response *agent.Response `json:"response,omitempty"`
	Error    error           `json:"error,omitempty"`
	Duration time.Duration   `json:"duration"`
	Status   PoolTaskStatus  `json:"status"`
}

type PoolEvent struct {
	Type      string    `json:"type"`
	TaskID    string    `json:"task_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data,omitempty"`
}

type PoolStats struct {
	TotalTasks        int `json:"total_tasks"`
	CompletedTasks    int `json:"completed_tasks"`
	FailedTasks       int `json:"failed_tasks"`
	RunningTasks      int `json:"running_tasks"`
	QueuedTasks       int `json:"queued_tasks"`
	MaxConcurrency    int `json:"max_concurrency"`
	ActiveConcurrency int `json:"active_concurrency"`
}

type PoolConfig struct {
	MaxConcurrency int           `json:"max_concurrency"`
	Timeout        time.Duration `json:"timeout"`
	RetryPolicy    RetryPolicy   `json:"retry_policy,omitempty"`
	// MaxRetainedTasks 保留的已完成任务数上限，超过时自动清理最早的终态任务（M8 修复）。
	// 0 表示不自动清理（向后兼容）。生产环境建议设置（如 1000）避免长期运行内存泄漏。
	MaxRetainedTasks int              `json:"max_retained_tasks,omitempty"`
	DefaultAgent     ReActAgentConfig `json:"default_agent"`
}

type RetryPolicy struct {
	MaxRetries      int           `json:"max_retries"`
	Backoff         time.Duration `json:"backoff"`
	RetryableErrors []string      `json:"retryable_errors"`
}

type ReActAgentConfig struct {
	SystemPrompt string  `json:"system_prompt"`
	MaxTurns     int     `json:"max_turns"`
	Temperature  float64 `json:"temperature"`
}

// AgentFactory 创建 Agent 实例的工厂函数
// Pool 使用此工厂为每个任务创建 Agent，确保 Agent 接收完整配置
type AgentFactory func(config AgentFactoryConfig) agent.Agent

// AgentFactoryConfig 是创建 Agent 时传递的完整配置
// 相比 ReActAgentConfig，包含 Memory/Scope/FileLock 等关键依赖
type AgentFactoryConfig struct {
	Name         string
	SystemPrompt string
	MaxTurns     int
	Temperature  float64
	FilesScope   []string
	SessionID    string
	Metadata     map[string]string
}
