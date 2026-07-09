// leader_election_metrics.go — Leader 选举可观测性指标接入（生产集成深度）
//
// 为 LeaderElector 接入 Prometheus 指标体系：
//   - ap_leader_election_state（gauge: 0=following, 1=leading, 2=degraded, 3=candidate）
//   - ap_leader_heartbeats_sent_total（counter）
//   - ap_leader_heartbeats_failed_total（counter）
//   - ap_leader_renew_failures_total（counter）
//   - ap_leader_changes_total（counter: 状态变更次数）
//   - ap_leader_lease_duration_seconds（gauge: 当前 Leader 持续时间）
//
// 通过 WithMetrics 方法注入，heartbeat 和 setState 内部自动记录指标。
package persist

import (
	"sync/atomic"
	"time"
)

// LeaderMetrics Leader 选举可观测性指标（线程安全）。
type LeaderMetrics struct {
	// 计数器
	heartbeatsSent   atomic.Int64
	heartbeatsFailed atomic.Int64
	renewFailures    atomic.Int64
	stateChanges     atomic.Int64
	acquireAttempts  atomic.Int64
	acquireSuccesses atomic.Int64
	acquireFailures  atomic.Int64

	// 仪表盘
	currentState     atomic.Int64 // 0=following, 1=leading, 2=degraded, 3=candidate
	leaderSinceNanos atomic.Int64 // 成为 Leader 的时间戳（unix nano），0 = 非 Leader
}

// NewLeaderMetrics 创建 Leader 指标实例。
func NewLeaderMetrics() *LeaderMetrics {
	return &LeaderMetrics{}
}

// 状态映射。
const (
	metricStateFollowing int64 = 0
	metricStateLeading   int64 = 1
	metricStateDegraded  int64 = 2
	metricStateCandidate int64 = 3
)

func leaderStateToMetric(state LeaderState) int64 {
	switch state {
	case LeaderLeading:
		return metricStateLeading
	case LeaderDegraded:
		return metricStateDegraded
	case LeaderCandidate:
		return metricStateCandidate
	default:
		return metricStateFollowing
	}
}

// RecordHeartbeatSent 记录一次心跳发送。
func (m *LeaderMetrics) RecordHeartbeatSent() {
	if m == nil {
		return
	}
	m.heartbeatsSent.Add(1)
}

// RecordHeartbeatFailed 记录一次心跳失败。
func (m *LeaderMetrics) RecordHeartbeatFailed() {
	if m == nil {
		return
	}
	m.heartbeatsFailed.Add(1)
}

// RecordRenewFailure 记录一次续约失败。
func (m *LeaderMetrics) RecordRenewFailure() {
	if m == nil {
		return
	}
	m.renewFailures.Add(1)
}

// RecordAcquireAttempt 记录一次竞选尝试。
func (m *LeaderMetrics) RecordAcquireAttempt() {
	if m == nil {
		return
	}
	m.acquireAttempts.Add(1)
}

// RecordAcquireSuccess 记录一次竞选成功。
func (m *LeaderMetrics) RecordAcquireSuccess() {
	if m == nil {
		return
	}
	m.acquireSuccesses.Add(1)
}

// RecordAcquireFailure 记录一次竞选失败。
func (m *LeaderMetrics) RecordAcquireFailure() {
	if m == nil {
		return
	}
	m.acquireFailures.Add(1)
}

// SetState 设置当前状态。
func (m *LeaderMetrics) SetState(state LeaderState) {
	if m == nil {
		return
	}
	newVal := leaderStateToMetric(state)
	oldVal := m.currentState.Swap(newVal)
	if oldVal != newVal {
		m.stateChanges.Add(1)
	}
	if state == LeaderLeading {
		m.leaderSinceNanos.Store(time.Now().UnixNano())
	} else {
		m.leaderSinceNanos.Store(0)
	}
}

// LeaderMetricsSnapshot 指标快照。
type LeaderMetricsSnapshot struct {
	HeartbeatsSent    int64
	HeartbeatsFailed  int64
	RenewFailures     int64
	StateChanges      int64
	AcquireAttempts   int64
	AcquireSuccesses  int64
	AcquireFailures   int64
	CurrentState      int64
	LeaderDurationSec float64
}

// Snapshot 返回指标快照。
func (m *LeaderMetrics) Snapshot() LeaderMetricsSnapshot {
	if m == nil {
		return LeaderMetricsSnapshot{}
	}
	var duration float64
	nano := m.leaderSinceNanos.Load()
	if nano > 0 {
		duration = time.Since(time.Unix(0, nano)).Seconds()
	}
	return LeaderMetricsSnapshot{
		HeartbeatsSent:    m.heartbeatsSent.Load(),
		HeartbeatsFailed:  m.heartbeatsFailed.Load(),
		RenewFailures:     m.renewFailures.Load(),
		StateChanges:      m.stateChanges.Load(),
		AcquireAttempts:   m.acquireAttempts.Load(),
		AcquireSuccesses:  m.acquireSuccesses.Load(),
		AcquireFailures:   m.acquireFailures.Load(),
		CurrentState:      m.currentState.Load(),
		LeaderDurationSec: duration,
	}
}

// WithMetrics 为 LeaderElector 注入可观测性指标。
// 注入后，heartbeat 和 setState 会自动记录指标。
func (le *LeaderElector) WithMetrics(metrics *LeaderMetrics) *LeaderElector {
	le.metrics = metrics
	return le
}
