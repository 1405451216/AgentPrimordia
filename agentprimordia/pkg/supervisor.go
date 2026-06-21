// Stability: Experimental — v3.0.0 新增前沿能力，API 可能随使用场景演进而调整。
package ap

import (
	"agentprimordia/internal/orchestration"
)

// Worker 工作者最小接口
type Worker = orchestration.Worker

// SVTask 任务定义
type SVTask = orchestration.Task

// TaskStatus 任务状态
type SVTaskStatus = orchestration.TaskStatus

// TaskResult 任务执行结果
type SVTaskResult = orchestration.TaskResult

// WorkerState worker 在 supervisor 中的运行时状态
type WorkerState = orchestration.WorkerState

// AssignmentStrategy 任务分配策略
type AssignmentStrategy = orchestration.AssignmentStrategy

// RoundRobinStrategy 轮询分配策略
type RoundRobinStrategy = orchestration.RoundRobinStrategy

// LoadBalancedStrategy 基于当前负载分配
type LoadBalancedStrategy = orchestration.LoadBalancedStrategy

// SkillBasedStrategy 基于技能标签匹配分配
type SkillBasedStrategy = orchestration.SkillBasedStrategy

// SupervisorConfig supervisor 配置
type SupervisorConfig = orchestration.SupervisorConfig

// SupervisorEvent supervisor 事件
type SupervisorEvent = orchestration.SupervisorEvent

// Supervisor 监督者：管理多个 Worker 并根据策略分配任务
type Supervisor = orchestration.Supervisor

const (
	// SVTaskStatusPending 任务等待中
	SVTaskStatusPending = orchestration.TaskStatusPending
	// SVTaskStatusRunning 任务运行中
	SVTaskStatusRunning = orchestration.TaskStatusRunning
	// SVTaskStatusCompleted 任务已完成
	SVTaskStatusCompleted = orchestration.TaskStatusCompleted
	// SVTaskStatusFailed 任务失败
	SVTaskStatusFailed = orchestration.TaskStatusFailed
	// SVTaskStatusCancelled 任务已取消
	SVTaskStatusCancelled = orchestration.TaskStatusCancelled
)

var (
	// NewRoundRobinStrategy 创建轮询策略
	NewRoundRobinStrategy = orchestration.NewRoundRobinStrategy
	// NewLoadBalancedStrategy 创建负载均衡策略
	NewLoadBalancedStrategy = orchestration.NewLoadBalancedStrategy
	// NewSkillBasedStrategy 创建技能匹配策略
	NewSkillBasedStrategy = orchestration.NewSkillBasedStrategy
	// NewSupervisor 创建 supervisor
	NewSupervisor = orchestration.NewSupervisor
)
