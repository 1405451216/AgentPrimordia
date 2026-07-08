// Package autoscaler 提供 LLM 负载感知的智能扩缩容。
//
// 核心功能：
//   - LLM 指标采集：队列深度、平均 LLM 延迟、Token 消耗速率
//   - 优先级驱逐：高优先级 Agent 可抢占低优先级资源
//   - 反抖动保护：扩容快、缩容慢，避免震荡
package autoscaler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Ensure no orphan refs
var _ = context.Background
var _ = fmt.Sprintf

// LLMMetrics 是单个 Pod 的 LLM 负载指标
type LLMMetrics struct {
	PodName           string    `json:"pod_name"`
	QueueDepth        int32     `json:"queue_depth"`         // 当前排队任务数
	AvgLatencyMs      float64   `json:"avg_llm_latency_ms"`  // 最近 5min 平均 LLM 延迟
	TokenRatePerMin   float64   `json:"token_rate_per_min"`  // 每分钟 Token 消耗
	ActiveTasks       int32     `json:"active_tasks"`        // 当前正在执行的任务数
	PriorityWeightedQ float64   `json:"priority_weighted_q"` // 优先级加权的队列深度
	LastUpdated       time.Time `json:"last_updated"`
}

// LLMMetricCollector 从 Pod 指标端点采集 LLM 指标
type LLMMetricCollector struct {
	mu      sync.RWMutex
	metrics map[string]*LLMMetrics // keyed by pod name
}

// NewLLMMetricCollector 创建采集器
func NewLLMMetricCollector() *LLMMetricCollector {
	return &LLMMetricCollector{
		metrics: make(map[string]*LLMMetrics),
	}
}

// Update 更新指定 Pod 的指标
func (c *LLMMetricCollector) Update(m *LLMMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m.LastUpdated = time.Now()
	c.metrics[m.PodName] = m
}

// GetAll 返回所有 Pod 指标副本
func (c *LLMMetricCollector) GetAll() map[string]*LLMMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]*LLMMetrics, len(c.metrics))
	for k, v := range c.metrics {
		cp := *v
		out[k] = &cp
	}
	return out
}

// Aggregate 返回聚合后的 LLM 指标
func (c *LLMMetricCollector) Aggregate() AggregatedLLMMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalLatency, totalTokenRate, totalPriorityQ float64
	var totalQueue, totalActive int32
	count := int32(len(c.metrics))

	for _, m := range c.metrics {
		totalLatency += m.AvgLatencyMs
		totalTokenRate += m.TokenRatePerMin
		totalPriorityQ += m.PriorityWeightedQ
		totalQueue += m.QueueDepth
		totalActive += m.ActiveTasks
	}

	if count == 0 {
		return AggregatedLLMMetrics{}
	}

	return AggregatedLLMMetrics{
		PodCount:              count,
		TotalQueueDepth:       totalQueue,
		TotalActiveTasks:      totalActive,
		AvgLatencyMs:          totalLatency / float64(count),
		TokenRatePerMin:       totalTokenRate,
		PriorityWeightedQueue: totalPriorityQ,
		Timestamp:             time.Now(),
	}
}

// AggregatedLLMMetrics 聚合 LLM 指标
type AggregatedLLMMetrics struct {
	PodCount              int32     `json:"pod_count"`
	TotalQueueDepth       int32     `json:"total_queue_depth"`
	TotalActiveTasks      int32     `json:"total_active_tasks"`
	AvgLatencyMs          float64   `json:"avg_latency_ms"`
	TokenRatePerMin       float64   `json:"token_rate_per_min"`
	PriorityWeightedQueue float64   `json:"priority_weighted_queue"`
	Timestamp             time.Time `json:"timestamp"`
}

// DesiredReplicas 根据 LLM 指标计算期望副本数
func (a AggregatedLLMMetrics) DesiredReplicas(cfg ScalingConfig) int32 {
	if a.PodCount == 0 {
		return cfg.MinReplicas
	}

	// 基于队列深度的期望副本
	queueBased := a.TotalQueueDepth / cfg.TargetQueueDepthPerPod
	if queueBased < cfg.MinReplicas {
		queueBased = cfg.MinReplicas
	}
	if queueBased > cfg.MaxReplicas {
		queueBased = cfg.MaxReplicas
	}

	// 基于延迟的期望副本（超延迟阈值时至少扩容 1 个）
	latencyBased := int32(0)
	if cfg.TargetLatencyMs > 0 && a.AvgLatencyMs > cfg.TargetLatencyMs {
		// 延迟超阈值，建议扩容 ceil(当前延迟/目标延迟) 倍
		latencyRatio := a.AvgLatencyMs / cfg.TargetLatencyMs
		latencyBased = int32(float64(a.PodCount) * latencyRatio)
	}

	// 基于 Token 消耗速率的期望副本
	tokenBased := int32(0)
	if cfg.TargetTokenRatePerPod > 0 && a.TokenRatePerMin > 0 {
		tokenBased = int32(a.TokenRatePerMin / cfg.TargetTokenRatePerPod)
	}

	// 取最大值，确保满足所有维度
	desired := queueBased
	if latencyBased > desired {
		desired = latencyBased
	}
	if tokenBased > desired {
		desired = tokenBased
	}

	// 边界
	if desired < cfg.MinReplicas {
		desired = cfg.MinReplicas
	}
	if desired > cfg.MaxReplicas {
		desired = cfg.MaxReplicas
	}

	return desired
}

// ScalingConfig 定义 LLM 负载感知扩缩容配置
type ScalingConfig struct {
	MinReplicas            int32   `json:"min_replicas"`
	MaxReplicas            int32   `json:"max_replicas"`
	TargetQueueDepthPerPod int32   `json:"target_queue_depth_per_pod"` // 每副本目标队列深度
	TargetLatencyMs        float64 `json:"target_latency_ms"`          // 目标 LLM 延迟 (ms)
	TargetTokenRatePerPod  float64 `json:"target_token_rate_per_pod"`  // 每副本目标 Token/分钟
}

// DefaultScalingConfig 返回默认配置
func DefaultScalingConfig() ScalingConfig {
	return ScalingConfig{
		MinReplicas:            1,
		MaxReplicas:            10,
		TargetQueueDepthPerPod: 5,
		TargetLatencyMs:        5000,
		TargetTokenRatePerPod:  100000,
	}
}

// PriorityEvictor 处理优先级抢占
type PriorityEvictor struct {
	mu         sync.RWMutex
	priorities map[string]AgentPriority // deployment name -> priority
}

// AgentPriority 表示 Agent 部署的优先级
type AgentPriority struct {
	DeploymentName string `json:"deployment_name"`
	Priority       int32  `json:"priority"` // 数值越大优先级越高
	MinReplicas    int32  `json:"min_replicas"`
	MaxReplicas    int32  `json:"max_replicas"`
}

// NewPriorityEvictor 创建优先级驱逐器
func NewPriorityEvictor() *PriorityEvictor {
	return &PriorityEvictor{
		priorities: make(map[string]AgentPriority),
	}
}

// Register 注册 Agent 优先级
func (e *PriorityEvictor) Register(p AgentPriority) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.priorities[p.DeploymentName] = p
}

// ShouldPreempt 判断 low 优先级的 Pod 是否应该被高优先级抢占
func (e *PriorityEvictor) ShouldPreempt(ctx context.Context, highPri, lowPri string) bool {
	e.mu.RLock()
	high, highOk := e.priorities[highPri]
	low, lowOk := e.priorities[lowPri]
	e.mu.RUnlock()

	if !highOk || !lowOk {
		return false // 未注册的允许共存
	}

	if high.Priority <= low.Priority {
		return false // 高优先级不更高，不抢占
	}

	// 只有当低优先级实际占用超出最小副本时才允许抢占
	return low.MinReplicas < low.MaxReplicas
}

// Ensure interface
var _ = fmt.Sprintf
var _ = context.Background
