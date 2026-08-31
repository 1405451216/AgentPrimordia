package autonomy

import (
	"context"
	"time"
)

// --- 自治 × RAG 集成 ---

// RAGRetriever RAG 知识检索接口（由外部 react_rag.go 适配）
type RAGRetriever interface {
	// Retrieve 根据查询检索相关知识
	Retrieve(ctx context.Context, query string, topK int) ([]RAGDocument, error)
}

// RAGDocument RAG 检索结果文档
type RAGDocument struct {
	Content string  `json:"content"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"`
}

// RAGIntegration 自治执行的 RAG 集成
type RAGIntegration struct {
	retriever RAGRetriever
	topK      int
}

// NewRAGIntegration 创建 RAG 集成
func NewRAGIntegration(retriever RAGRetriever, topK int) *RAGIntegration {
	if topK <= 0 {
		topK = 5
	}
	return &RAGIntegration{retriever: retriever, topK: topK}
}

// EnrichStepContext 为步骤执行注入 RAG 知识上下文
func (r *RAGIntegration) EnrichStepContext(ctx context.Context, step PlanStep) ([]RAGDocument, error) {
	return r.retriever.Retrieve(ctx, step.Description, r.topK)
}

// --- 自治 × Pool 集成 ---

// PoolDispatcher 多目标并发调度接口（由外部 pool/ 适配）
type PoolDispatcher interface {
	// Dispatch 投递目标执行任务到池
	Dispatch(ctx context.Context, goalID string, fn func() error) error
	// ActiveCount 当前活跃任务数
	ActiveCount() int
}

// PoolIntegration 自治执行的 Pool 集成
type PoolIntegration struct {
	dispatcher PoolDispatcher
}

// NewPoolIntegration 创建 Pool 集成
func NewPoolIntegration(dispatcher PoolDispatcher) *PoolIntegration {
	return &PoolIntegration{dispatcher: dispatcher}
}

// DispatchGoal 将目标执行投递到调度池
func (p *PoolIntegration) DispatchGoal(ctx context.Context, goalID string, fn func() error) error {
	return p.dispatcher.Dispatch(ctx, goalID, fn)
}

// --- 自治 × 集群 集成 ---

// ClusterSync 分布式状态同步接口（由外部 cluster/ 适配）
type ClusterSync interface {
	// SyncState 同步目标状态到集群
	SyncState(ctx context.Context, goalID string, state GoalState, metadata map[string]string) error
	// AcquireOwnership 获取目标执行权（分布式锁）
	AcquireOwnership(ctx context.Context, goalID string, nodeID string) (bool, error)
	// ReleaseOwnership 释放目标执行权
	ReleaseOwnership(ctx context.Context, goalID string, nodeID string) error
}

// ClusterIntegration 自治执行的集群集成
type ClusterIntegration struct {
	sync   ClusterSync
	nodeID string
}

// NewClusterIntegration 创建集群集成
func NewClusterIntegration(sync ClusterSync, nodeID string) *ClusterIntegration {
	return &ClusterIntegration{sync: sync, nodeID: nodeID}
}

// SyncGoalState 同步目标状态到集群
func (c *ClusterIntegration) SyncGoalState(ctx context.Context, goalID string, state GoalState) error {
	return c.sync.SyncState(ctx, goalID, state, map[string]string{"node": c.nodeID})
}

// AcquireGoal 获取目标执行权
func (c *ClusterIntegration) AcquireGoal(ctx context.Context, goalID string) (bool, error) {
	return c.sync.AcquireOwnership(ctx, goalID, c.nodeID)
}

// --- 自治 × 可观测 集成 ---

// GoalMetrics 目标级指标记录接口（由外部 metrics/ + otel/ 适配）
type GoalMetrics interface {
	// RecordGoalLifecycle 记录目标生命周期事件
	RecordGoalLifecycle(goalID string, state GoalState, duration time.Duration)
	// RecordStepExecution 记录步骤执行指标
	RecordStepExecution(goalID string, stepID string, duration time.Duration, err error)
	// RecordReplan 记录重规划事件
	RecordReplan(goalID string, reason string)
}

// ObservabilityIntegration 自治执行的可观测集成
type ObservabilityIntegration struct {
	metrics GoalMetrics
}

// NewObservabilityIntegration 创建可观测集成
func NewObservabilityIntegration(metrics GoalMetrics) *ObservabilityIntegration {
	return &ObservabilityIntegration{metrics: metrics}
}

// RecordLifecycle 记录目标生命周期
func (o *ObservabilityIntegration) RecordLifecycle(goalID string, state GoalState, duration time.Duration) {
	o.metrics.RecordGoalLifecycle(goalID, state, duration)
}

// RecordStep 记录步骤执行
func (o *ObservabilityIntegration) RecordStep(goalID string, stepID string, duration time.Duration, err error) {
	o.metrics.RecordStepExecution(goalID, stepID, duration, err)
}

// --- 自治 × 守卫 集成 ---

// StepGuardrail 步骤级护栏检查接口（由外部 guardrail/ 适配）
type StepGuardrail interface {
	// CheckStep 检查步骤是否允许执行
	CheckStep(ctx context.Context, goalID string, step PlanStep) (allowed bool, reason string, err error)
	// CheckOutput 检查步骤输出是否合规
	CheckOutput(ctx context.Context, goalID string, output string) (sanitized string, blocked bool, err error)
}

// GuardrailIntegration 自治执行的护栏集成
type GuardrailIntegration struct {
	guard StepGuardrail
}

// NewGuardrailIntegration 创建护栏集成
func NewGuardrailIntegration(guard StepGuardrail) *GuardrailIntegration {
	return &GuardrailIntegration{guard: guard}
}

// ValidateStep 执行前校验步骤
func (g *GuardrailIntegration) ValidateStep(ctx context.Context, goalID string, step PlanStep) error {
	allowed, reason, err := g.guard.CheckStep(ctx, goalID, step)
	if err != nil {
		return err
	}
	if !allowed {
		return &GuardrailViolation{GoalID: goalID, StepID: step.ID, Reason: reason}
	}
	return nil
}

// SanitizeOutput 执行后校验输出
func (g *GuardrailIntegration) SanitizeOutput(ctx context.Context, goalID string, output string) (string, error) {
	sanitized, blocked, err := g.guard.CheckOutput(ctx, goalID, output)
	if err != nil {
		return output, err
	}
	if blocked {
		return "", &GuardrailViolation{GoalID: goalID, Reason: "输出被护栏拦截"}
	}
	if sanitized != "" {
		return sanitized, nil
	}
	return output, nil
}

// GuardrailViolation 护栏违规错误
type GuardrailViolation struct {
	GoalID string
	StepID string
	Reason string
}

func (e *GuardrailViolation) Error() string {
	if e.StepID != "" {
		return "autonomy: 护栏拦截 [goal=" + e.GoalID + ", step=" + e.StepID + "]: " + e.Reason
	}
	return "autonomy: 护栏拦截 [goal=" + e.GoalID + "]: " + e.Reason
}
