// Stability: Experimental — v3.0.0 新增前沿能力，API 可能随使用场景演进而调整。
package ap

import (
	"agentprimordia/internal/agent/planning"
)

// Planner 定义任务规划和分解接口
type Planner = planning.Planner

// SubTask 表示一个子任务
type SubTask = planning.SubTask

// Plan 表示执行计划
type Plan = planning.Plan

// TaskStatus 任务状态
type TaskStatus = planning.TaskStatus

// LLMPlanner 使用 LLM 进行任务规划
type LLMPlanner = planning.LLMPlanner

const (
	// TaskPending 任务等待中
	TaskPending = planning.TaskPending
	// TaskRunning 任务运行中
	TaskRunning = planning.TaskRunning
	// TaskCompleted 任务已完成
	TaskCompleted = planning.TaskCompleted
	// TaskFailed 任务失败
	TaskFailed = planning.TaskFailed
)

var (
	// NewLLMPlanner 创建 LLMPlanner 实例
	NewLLMPlanner = planning.NewLLMPlanner
)
