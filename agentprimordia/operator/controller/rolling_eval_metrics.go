// rolling_eval_metrics.go — Canary Rollout 可观测性指标接入（生产集成深度）
//
// 为 CanaryRolloutReconciler 接入 Prometheus 指标体系：
//   - ap_canary_rollout_phase（gauge: 0=stable, 1=progressing, 2=promoted, 3=rolled_back）
//   - ap_canary_rollout_total（counter: 总灰度发布次数）
//   - ap_canary_rollout_promoted_total（counter: 成功提升次数）
//   - ap_canary_rollout_rolled_back_total（counter: 回滚次数）
//   - ap_canary_rollout_duration_seconds（histogram: 灰度持续时间）
//   - ap_canary_eval_pass_rate（gauge: 最近一次 Eval 通过率）
//   - ap_canary_eval_runs_total（counter: Eval 运行次数）
//   - ap_canary_eval_errors_total（counter: Eval 运行失败次数）
package controller

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// CanaryMetrics Canary Rollout 可观测性指标（线程安全）。
type CanaryMetrics struct {
	// 计数器
	rolloutTotal    atomic.Int64
	promotedTotal   atomic.Int64
	rolledBackTotal atomic.Int64
	evalRunsTotal   atomic.Int64
	evalErrorsTotal atomic.Int64

	// 仪表盘
	currentPhase     atomic.Int64  // 0=stable, 1=progressing, 2=promoted, 3=rolled_back
	lastEvalPassRate atomic.Uint64 // math.Float64bits
	canaryPercent    atomic.Int64

	// 直方图（简化版：桶式计数）
	mu              sync.Mutex
	durationBuckets []float64 // seconds
	durationCounts  []int64
	durationSum     float64
	durationCount   int64

	// 按 Agent 维度
	rolloutsByAgent sync.Map // map[string]*atomic.Int64
}

// NewCanaryMetrics 创建 Canary 指标实例。
func NewCanaryMetrics() *CanaryMetrics {
	return &CanaryMetrics{
		durationBuckets: []float64{10, 30, 60, 120, 300, 600, 1200, 1800, 3600},
		durationCounts:  make([]int64, 9),
	}
}

// phase 映射。
const (
	metricPhaseStable      int64 = 0
	metricPhaseProgressing int64 = 1
	metricPhasePromoted    int64 = 2
	metricPhaseRolledBack  int64 = 3
)

func canaryPhaseToMetric(phase CanaryPhase) int64 {
	switch phase {
	case CanaryProgressing:
		return metricPhaseProgressing
	case CanaryPromoted:
		return metricPhasePromoted
	case CanaryRolledBack:
		return metricPhaseRolledBack
	default:
		return metricPhaseStable
	}
}

// RecordRolloutStart 记录灰度发布启动。
func (m *CanaryMetrics) RecordRolloutStart(agentName string, canaryPercent int) {
	if m == nil {
		return
	}
	m.rolloutTotal.Add(1)
	m.currentPhase.Store(metricPhaseProgressing)
	m.canaryPercent.Store(int64(canaryPercent))

	// 按 Agent 维度计数
	val, _ := m.rolloutsByAgent.LoadOrStore(agentName, &atomic.Int64{})
	val.(*atomic.Int64).Add(1)
}

// RecordPromoted 记录灰度成功提升。
func (m *CanaryMetrics) RecordPromoted(duration time.Duration) {
	if m == nil {
		return
	}
	m.promotedTotal.Add(1)
	m.currentPhase.Store(metricPhasePromoted)
	m.recordDuration(duration)
}

// RecordRolledBack 记录灰度回滚。
func (m *CanaryMetrics) RecordRolledBack(duration time.Duration) {
	if m == nil {
		return
	}
	m.rolledBackTotal.Add(1)
	m.currentPhase.Store(metricPhaseRolledBack)
	m.recordDuration(duration)
}

// RecordEvalRun 记录一次 Eval 运行。
func (m *CanaryMetrics) RecordEvalRun(passRate float64, err error) {
	if m == nil {
		return
	}
	m.evalRunsTotal.Add(1)
	if err != nil {
		m.evalErrorsTotal.Add(1)
	} else {
		// 存储通过率（atomic CAS 写入）
		m.lastEvalPassRate.Store(float64Bits(passRate))
	}
}

// SetPhase 设置当前灰度阶段。
func (m *CanaryMetrics) SetPhase(phase CanaryPhase) {
	if m == nil {
		return
	}
	m.currentPhase.Store(canaryPhaseToMetric(phase))
}

// recordDuration 记录灰度持续时间到直方图。
func (m *CanaryMetrics) recordDuration(d time.Duration) {
	sec := d.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.durationSum += sec
	m.durationCount++
	for i, bucket := range m.durationBuckets {
		if sec <= bucket {
			m.durationCounts[i]++
			return
		}
	}
	// 超过最大桶
	m.durationCounts[len(m.durationCounts)-1]++
}

// CanaryMetricsSnapshot 指标快照。
type CanaryMetricsSnapshot struct {
	RolloutTotal     int64
	PromotedTotal    int64
	RolledBackTotal  int64
	EvalRunsTotal    int64
	EvalErrorsTotal  int64
	CurrentPhase     int64
	LastEvalPassRate float64
	CanaryPercent    int64
	DurationSum      float64
	DurationCount    int64
	DurationBuckets  []float64
	DurationCounts   []int64
	RolloutsByAgent  map[string]int64
}

// Snapshot 返回指标快照。
func (m *CanaryMetrics) Snapshot() CanaryMetricsSnapshot {
	if m == nil {
		return CanaryMetricsSnapshot{}
	}
	m.mu.Lock()
	buckets := make([]float64, len(m.durationBuckets))
	copy(buckets, m.durationBuckets)
	counts := make([]int64, len(m.durationCounts))
	copy(counts, m.durationCounts)
	sum := m.durationSum
	count := m.durationCount
	m.mu.Unlock()

	byAgent := make(map[string]int64)
	m.rolloutsByAgent.Range(func(key, val any) bool {
		byAgent[key.(string)] = val.(*atomic.Int64).Load()
		return true
	})

	return CanaryMetricsSnapshot{
		RolloutTotal:     m.rolloutTotal.Load(),
		PromotedTotal:    m.promotedTotal.Load(),
		RolledBackTotal:  m.rolledBackTotal.Load(),
		EvalRunsTotal:    m.evalRunsTotal.Load(),
		EvalErrorsTotal:  m.evalErrorsTotal.Load(),
		CurrentPhase:     m.currentPhase.Load(),
		LastEvalPassRate: float64FromBits(m.lastEvalPassRate.Load()),
		CanaryPercent:    m.canaryPercent.Load(),
		DurationSum:      sum,
		DurationCount:    count,
		DurationBuckets:  buckets,
		DurationCounts:   counts,
		RolloutsByAgent:  byAgent,
	}
}

// float64Bits / float64FromBits — 使用 math 包的标准位模式转换。
func float64Bits(f float64) uint64 {
	return math.Float64bits(f)
}

func float64FromBits(b uint64) float64 {
	return math.Float64frombits(b)
}
