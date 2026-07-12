package health

import (
	"sync"
	"time"
)

// SLOStatus 单个 SLO 的达成状态。
type SLOStatus struct {
	// Name SLO 名称
	Name string
	// Current 当前达成率（如 0.9995）
	Current float64
	// Target 目标达成率（如 0.999）
	Target float64
	// BurnRate 错误预算消耗速率（1.0 = 线性消耗）
	BurnRate float64
	// ErrorBudget 剩余错误预算
	ErrorBudget float64
	// Violated 是否违反 SLO
	Violated bool
}

// CheckSLO 检查一组 SLI 指标是否满足 SLO 目标。
//
// 对每个 SLIMetric，计算其 BurnRate、ErrorBudget 和是否违反 SLO。
// ErrorBudget = (1 - Target) - (1 - Current)
// BurnRate = (1 - Current) / (1 - Target)
func CheckSLO(metrics []SLIMetric, target float64) []SLOStatus {
	statuses := make([]SLOStatus, 0, len(metrics))
	for _, m := range metrics {
		current := m.Value
		// 限制在 [0, 1] 范围
		if current < 0 {
			current = 0
		}
		if current > 1 {
			current = 1
		}

		t := target
		if t <= 0 {
			t = m.Target
		}
		if t <= 0 || t >= 1 {
			t = 0.999 // 默认目标
		}

		errorRate := 1.0 - current
		allowedError := 1.0 - t

		var burnRate float64
		if allowedError > 0 {
			burnRate = errorRate / allowedError
		}
		errorBudget := allowedError - errorRate

		statuses = append(statuses, SLOStatus{
			Name:        m.Name,
			Current:     current,
			Target:      t,
			BurnRate:    burnRate,
			ErrorBudget: errorBudget,
			Violated:    current < t,
		})
	}
	return statuses
}

// CheckSLOsWithDefaults 为每个指标使用自身 Target 检查 SLO。
func CheckSLOsWithDefaults(metrics []SLIMetric) []SLOStatus {
	statuses := make([]SLOStatus, 0, len(metrics))
	for _, m := range metrics {
		statuses = append(statuses, checkSingleSLO(m))
	}
	return statuses
}

// checkSingleSLO 检查单个 SLI 指标。
func checkSingleSLO(m SLIMetric) SLOStatus {
	current := m.Value
	if current < 0 {
		current = 0
	}
	if current > 1 {
		current = 1
	}

	t := m.Target
	if t <= 0 || t >= 1 {
		t = 0.999
	}

	errorRate := 1.0 - current
	allowedError := 1.0 - t

	var burnRate float64
	if allowedError > 0 {
		burnRate = errorRate / allowedError
	}
	errorBudget := allowedError - errorRate

	return SLOStatus{
		Name:        m.Name,
		Current:     current,
		Target:      t,
		BurnRate:    burnRate,
		ErrorBudget: errorBudget,
		Violated:    current < t,
	}
}

// SLORegistry SLO 指标注册表，用于收集和监控多组 SLI。
//
// 注意：本实现不引入 prometheus 等第三方依赖。
// 如需集成外部监控系统，使用 ExportSLOStatus 获取状态后自行推送。
type SLORegistry struct {
	mu       sync.RWMutex
	metrics  map[string]SLIMetric
	target   float64
}

// NewSLORegistry 创建 SLO 注册表。
func NewSLORegistry(target float64) *SLORegistry {
	if target <= 0 || target >= 1 {
		target = 0.999
	}
	return &SLORegistry{
		metrics: make(map[string]SLIMetric),
		target:  target,
	}
}

// UpdateMetric 更新一个 SLI 指标。
func (r *SLORegistry) UpdateMetric(m SLIMetric) {
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	r.mu.Lock()
	r.metrics[m.Name] = m
	r.mu.Unlock()
}

// GetSLOStatus 返回当前所有 SLO 的达成状态。
func (r *SLORegistry) GetSLOStatus() []SLOStatus {
	r.mu.RLock()
	metrics := make([]SLIMetric, 0, len(r.metrics))
	for _, m := range r.metrics {
		metrics = append(metrics, m)
	}
	r.mu.RUnlock()
	return CheckSLO(metrics, r.target)
}

// RegisterSLOMetrics 注册 SLO 指标到外部 Registry 接口。
//
// 本方法接受一个回调函数而非 *prometheus.Registry，以避免引入第三方依赖。
// 回调函数会收到每个 SLO 指标的当前 BurnRate 和 ErrorBudget，
// 可用于推送至 Prometheus、OpenTelemetry、Datadog 等系统。
func RegisterSLOMetrics(registry *SLORegistry, callback func(status SLOStatus)) {
	if registry == nil {
		return
	}
	for _, s := range registry.GetSLOStatus() {
		callback(s)
	}
}
