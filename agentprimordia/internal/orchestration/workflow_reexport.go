package orchestration

import "agentprimordia/internal/agent"

// Re-exports of the workflow types now living in internal/agent (moved there
// in Phase 6 so they can be exposed via pkg/). These aliases keep existing
// internal/orchestration tests compiling without modification.

type (
	WorkflowExecution       = agent.WorkflowExecution
	WorkflowConfig          = agent.WorkflowConfig
	WorkflowType            = agent.WorkflowType
	WorkflowStatus          = agent.WorkflowStatus
	WorkflowNode            = agent.WorkflowNode
	WorkflowEvent           = agent.WorkflowEvent
	WorkflowResult          = agent.WorkflowResult
	WorkflowMetrics         = agent.WorkflowMetrics
	NodeType                = agent.NodeType
	NodeCondition           = agent.NodeCondition
	NodeConfig              = agent.NodeConfig
	NodeExecutionStatus     = agent.NodeExecutionStatus
	Transition              = agent.Transition
	TransitionCondition     = agent.TransitionCondition
	ErrorHandling           = agent.ErrorHandling
	ExecutionRecord         = agent.ExecutionRecord
)

const (
	LinearWorkflow       = agent.LinearWorkflow
	ConditionalWorkflow  = agent.ConditionalWorkflow
	LoopWorkflow         = agent.LoopWorkflow
	ParallelForkJoin     = agent.ParallelForkJoin
	StateMachine         = agent.StateMachine

	WfStatusPending      = agent.WfStatusPending
	WfStatusRunning      = agent.WfStatusRunning
	WfStatusPaused       = agent.WfStatusPaused
	WfStatusCompleted    = agent.WfStatusCompleted
	WfStatusFailed       = agent.WfStatusFailed
	WfStatusCancelled    = agent.WfStatusCancelled

	TaskNode             = agent.TaskNode
	ConditionNode        = agent.ConditionNode
	ParallelNode         = agent.ParallelNode
	LoopStartNode        = agent.LoopStartNode
	LoopEndNode          = agent.LoopEndNode
	FallbackNode         = agent.FallbackNode

	NodePending          = agent.NodePending
	NodeRunning          = agent.NodeRunning
	NodeCompleted        = agent.NodeCompleted
	NodeSkipped          = agent.NodeSkipped
	NodeFailed           = agent.NodeFailed
)

var NewWorkflowExecution = agent.NewWorkflowExecution
