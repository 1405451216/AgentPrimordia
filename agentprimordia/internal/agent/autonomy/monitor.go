package autonomy

import (
	"sync"
	"time"
)

// AlertLevel 异常分级
type AlertLevel string

const (
	// AlertWarn 警告级别
	AlertWarn AlertLevel = "warn"
	// AlertError 错误级别
	AlertError AlertLevel = "error"
	// AlertCritical 严重级别
	AlertCritical AlertLevel = "critical"
)

// Alert 异常告警
type Alert struct {
	// GoalID 关联目标
	GoalID string
	// Level 告警级别
	Level AlertLevel
	// Message 告警信息
	Message string
	// Timestamp 时间戳
	Timestamp time.Time
}

// GoalStatus 目标运行状态快照
type GoalStatus struct {
	// GoalID 目标 ID
	GoalID string
	// Progress 当前进度 [0.0, 1.0]
	Progress float64
	// Heartbeats 心跳总数
	Heartbeats int
	// LastHeartbeat 最后心跳时间
	LastHeartbeat time.Time
	// StallCount 连续无进展次数
	StallCount int
}

// MonitorConfig 监控器配置
type MonitorConfig struct {
	// StallThreshold 停滞阈值：连续 N 轮无进展触发告警（默认 5）
	StallThreshold int
}

// Monitor 自治监控器：停滞检测 + 进度追踪 + 异常上报
type Monitor struct {
	mu       sync.RWMutex
	cfg      MonitorConfig
	statuses map[string]*goalMonitorState
	alertFns []func(Alert)
}

// goalMonitorState 单个目标的监控状态
type goalMonitorState struct {
	progress      float64
	lastProgress  float64
	heartbeats    int
	lastHeartbeat time.Time
	stallCount    int
	firstBeat     time.Time
}

// NewMonitor 创建监控器
func NewMonitor(cfg MonitorConfig) *Monitor {
	if cfg.StallThreshold <= 0 {
		cfg.StallThreshold = 5
	}
	return &Monitor{
		cfg:      cfg,
		statuses: make(map[string]*goalMonitorState),
	}
}

// OnAlert 注册告警回调
func (m *Monitor) OnAlert(fn func(Alert)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertFns = append(m.alertFns, fn)
}

// ReportHeartbeat 上报心跳（含当前进度）
func (m *Monitor) ReportHeartbeat(goalID string, progress float64) {
	m.mu.Lock()
	st, ok := m.statuses[goalID]
	if !ok {
		st = &goalMonitorState{firstBeat: time.Now()}
		m.statuses[goalID] = st
	}

	st.heartbeats++
	st.lastHeartbeat = time.Now()
	st.lastProgress = st.progress
	st.progress = progress

	// 停滞检测：进度无变化
	if progress <= st.lastProgress && st.heartbeats > 1 {
		st.stallCount++
	} else {
		st.stallCount = 0
	}

	shouldAlert := st.stallCount >= m.cfg.StallThreshold
	m.mu.Unlock()

	if shouldAlert {
		m.emitAlert(Alert{
			GoalID:    goalID,
			Level:     AlertWarn,
			Message:   "目标执行停滞：连续多轮无进展",
			Timestamp: time.Now(),
		})
		// 重置计数避免重复告警
		m.mu.Lock()
		st.stallCount = 0
		m.mu.Unlock()
	}
}

// ReportAnomaly 上报异常
func (m *Monitor) ReportAnomaly(goalID string, level AlertLevel, message string) {
	m.emitAlert(Alert{
		GoalID:    goalID,
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
	})
}

// GetStatus 获取目标运行状态
func (m *Monitor) GetStatus(goalID string) GoalStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	st, ok := m.statuses[goalID]
	if !ok {
		return GoalStatus{GoalID: goalID}
	}
	return GoalStatus{
		GoalID:        goalID,
		Progress:      st.progress,
		Heartbeats:    st.heartbeats,
		LastHeartbeat: st.lastHeartbeat,
		StallCount:    st.stallCount,
	}
}

// EstimateRemaining 估算剩余执行时间
func (m *Monitor) EstimateRemaining(goalID string) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	st, ok := m.statuses[goalID]
	if !ok || st.progress <= 0 || st.firstBeat.IsZero() {
		return 0
	}

	elapsed := time.Since(st.firstBeat)
	if st.progress >= 1.0 {
		return 0
	}
	// 线性外推：总时间 = 已用时间 / 当前进度
	total := time.Duration(float64(elapsed) / st.progress)
	return total - elapsed
}

// emitAlert 触发告警回调
func (m *Monitor) emitAlert(a Alert) {
	m.mu.RLock()
	fns := make([]func(Alert), len(m.alertFns))
	copy(fns, m.alertFns)
	m.mu.RUnlock()

	for _, fn := range fns {
		fn(a)
	}
}
