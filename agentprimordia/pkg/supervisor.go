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

// ===== Orchestration Tracing 编排追踪 =====

// OrchestrationTracer 编排层追踪器接口
type OrchestrationTracer = orchestration.Tracer

// OrchestrationTracingConfig 编排追踪配置
type OrchestrationTracingConfig = orchestration.TracingConfig

// OrchestrationTracingStepExecutor 自动追踪的 StepExecutor 装饰器
type OrchestrationTracingStepExecutor = orchestration.TracingStepExecutor

// OrchestrationTracingPipeline 自动追踪的 Pipeline 装饰器
type OrchestrationTracingPipeline = orchestration.TracingPipeline

// OrchestrationTracingHandoffRecorder 自动追踪的 Handoff 装饰器
type OrchestrationTracingHandoffRecorder = orchestration.TracingHandoffRecorder

var (
	// NewOrchestrationNoopTracer 创建不记录任何 span 的 tracer（默认）
	NewOrchestrationNoopTracer = orchestration.NewNoopTracer
	// NewOrchestrationTracingStepExecutor 创建追踪 StepExecutor 装饰器
	NewOrchestrationTracingStepExecutor = orchestration.NewTracingStepExecutor
	// NewOrchestrationTracingPipeline 创建追踪 Pipeline 装饰器
	NewOrchestrationTracingPipeline = orchestration.NewTracingPipeline
	// NewOrchestrationTracingHandoffRecorder 创建追踪 Handoff 包装
	NewOrchestrationTracingHandoffRecorder = orchestration.NewTracingHandoffRecorder
	// OrchestrationWithTracer 构造启用追踪的 TracingConfig
	OrchestrationWithTracer = orchestration.WithTracer
	// OrchestrationFromAgentTracer 把 agent.Tracer 适配为 orchestration.Tracer
	OrchestrationFromAgentTracer = orchestration.FromAgentTracer
	// OrchestrationDefaultTracingConfig 返回关闭追踪的默认配置
	OrchestrationDefaultTracingConfig = orchestration.DefaultTracingConfig
	// OrchestrationConfigureTracing 在 Orchestrator 上启用追踪（接口预留）
	OrchestrationConfigureTracing = orchestration.ConfigureOrchestratorTracing
)
